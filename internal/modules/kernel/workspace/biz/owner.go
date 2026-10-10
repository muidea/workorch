package workspace

import (
	"context"
	"log/slog"
	"sync"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	modulecommon "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/common"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID

const workspaceTemplateDefaultsCommandSource = "kernel/system"

type Dependencies struct {
	OwnerRepositories repository.WorkspaceOwnerRepositories
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	manager            *goalDoDManager
	workspaces         repository.WorkspaceRepository
	cleaner            repository.WorkspaceDeletionMaterialCleaner
	goals              repository.GoalRepository
	dods               repository.DoDRepository
	templates          repository.WorkspaceTemplateRepository
	topics             []string
	started            bool
	templateMutationMu sync.Mutex
	// deletionIngressMu serializes the initial Workspace deletion admission
	// against owner public mutation ingresses. Persisted CAS remains the
	// authority; this lock only closes same-process non-Workspace side-effect
	// windows such as Goal/DoD writes.
	deletionIngressMu sync.RWMutex
	deletionWorkMu    sync.Mutex

	runProjectionStateMu    sync.Mutex
	runProjectionWorkMu     sync.Mutex
	runProjectionCtx        context.Context
	runProjectionCancel     context.CancelFunc
	runProjectionPending    bool
	runProjectionGeneration uint64
	runProjectionLastError  error
}

func New(h magicevent.Hub, b task.BackgroundRoutine, deps Dependencies) (*Biz, error) {
	if deps.EventHub == nil || h == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := deps.OwnerRepositories.Validate(); err != nil {
		return nil, err
	}
	workspaces := deps.OwnerRepositories.Workspaces()
	goals := deps.OwnerRepositories.Goals()
	dods := deps.OwnerRepositories.DoDs()
	bizPtr := &Biz{
		Base:       basebiz.New(modulecommon.ModuleID, h, b),
		workspaces: workspaces,
		cleaner:    deps.OwnerRepositories.DeletionMaterialCleaner(),
		goals:      goals,
		dods:       dods,
		templates:  deps.OwnerRepositories.Templates(),
	}
	bizPtr.manager = newGoalDoDManagerWithPublisher(workspaces, goals, dods, bizPtr.PostEvent)
	return bizPtr, nil
}

func (m *Biz) Start() error {
	if m == nil || m.manager == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	if m.started {
		return nil
	}
	m.topics = workspaceCommandTopics()
	for _, topic := range m.topics {
		m.SubscribeFunc(topic, m.handleCommand)
	}
	for _, topic := range m.lifecycleTopics() {
		m.SubscribeFunc(topic, m.handleLifecycleCommand)
		m.topics = append(m.topics, topic)
	}
	for _, topic := range definitionAdmissionTopics() {
		m.SubscribeFunc(topic, m.handleDefinitionAdmissionCommand)
		m.topics = append(m.topics, topic)
	}
	for _, topic := range definitionSpecificationTopics() {
		m.SubscribeFunc(topic, m.handleDefinitionSpecificationCommand)
		m.topics = append(m.topics, topic)
	}
	m.SubscribeFunc(workspaceevents.TopicWorkspaceCommandExecutionIntent, m.handleExecutionIntentCommand)
	m.topics = append(m.topics, workspaceevents.TopicWorkspaceCommandExecutionIntent)
	// Run owner is the sole publisher of lifecycle facts. Workspace consumes a
	// fact and posts an owner-local projection command; it never accepts an
	// Orchestrator-written Workspace model.
	m.SubscribeMatchedFunc(ModuleID+"/run-projection", runevents.NotificationDestination, runevents.TopicRunStateChanged, m.handleRunStateChanged)
	m.SubscribeMatchedFunc(ModuleID+"/run-projection", runevents.NotificationDestination, runevents.TopicRunRuntimeCommandsReady, m.handleRunRuntimeCommandsReady)
	m.topics = append(m.topics, workspaceevents.TopicWorkspaceTemplateCommandEnsureDefaults)
	m.SubscribeFunc(workspaceevents.TopicWorkspaceTemplateCommandEnsureDefaults, m.handleSystemDefaultsCommand)
	for _, topic := range []string{workspaceevents.TopicWorkspaceTemplateCommandQueryBuiltinDefaults, workspaceevents.TopicWorkspaceTemplateCommandUpgradeBuiltinDefault} {
		m.topics = append(m.topics, topic)
		m.SubscribeFunc(topic, m.handleBuiltinTemplateDefaultsCommand)
	}
	if err := m.startRunProjectionReconciliation(); err != nil {
		m.unsubscribeOwnerTopics()
		return err
	}
	m.started = true
	return nil
}

func (m *Biz) handleSystemDefaultsCommand(ev magicevent.Event, result magicevent.Result) {
	if result == nil {
		return
	}
	if ev == nil || ev.Source() != workspaceTemplateDefaultsCommandSource || m == nil || m.templates == nil {
		setSystemWorkspaceTemplateDefaultsResult(result, common.ErrInvalidArgument)
		return
	}
	if _, ok := ev.Data().(workspaceevents.EnsureWorkspaceTemplateDefaultsCommand); !ok {
		setSystemWorkspaceTemplateDefaultsResult(result, common.ErrInvalidArgument)
		return
	}
	setSystemWorkspaceTemplateDefaultsResult(result, m.ensureCurrentBuiltinWorkspaceTemplateDefaults(ev.Context()))
}

func setSystemWorkspaceTemplateDefaultsResult(result magicevent.Result, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(workspaceevents.EnsureWorkspaceTemplateDefaultsResult{Ensured: true}, nil)
}

func (m *Biz) Teardown(context.Context) {
	if m == nil {
		return
	}
	m.stopRunProjectionReconciliation()
	m.unsubscribeOwnerTopics()
	m.manager = nil
	m.workspaces = nil
	m.cleaner = nil
	m.goals = nil
	m.dods = nil
	m.templates = nil
	m.started = false
}

func (m *Biz) unsubscribeOwnerTopics() {
	if m == nil {
		return
	}
	for idx := len(m.topics) - 1; idx >= 0; idx-- {
		m.UnsubscribeFunc(m.topics[idx])
	}
	m.UnsubscribeMatchedFunc(ModuleID+"/run-projection", runevents.TopicRunStateChanged)
	m.UnsubscribeMatchedFunc(ModuleID+"/run-projection", runevents.TopicRunRuntimeCommandsReady)
	m.topics = nil
}

func (m *Biz) handleRunStateChanged(ev magicevent.Event, _ magicevent.Result) {
	data, ok := ev.Data().(runevents.RunStateChangedData)
	if !ok || ev.Source() != runevents.CommandDestination || m == nil || m.EventHub() == nil {
		return
	}
	if err := data.Validate(); err != nil {
		// The notification is only a delivery hint. Keep reconciliation dirty so
		// a malformed or incomplete hint cannot make the Workspace projection
		// silently miss the Run owner's current typed fact.
		slog.Warn("Workspace Run projection notification rejected; scheduling current-fact reconciliation", "workspace", data.WorkspaceID, "run", data.RunID, "err", err)
		m.requestRunProjectionReconciliation(err)
		return
	}
	if runProjectionTerminalState(data.State) {
		// Workspace terminal release is driven only by the sealed Orchestrator
		// delivery. Reconcile/clear any prior notification retry without ever
		// translating a terminal Run hint into a Workspace target state.
		m.requestRunProjectionReconciliation(nil)
		return
	}
	if err := m.applyRunStateChangedNotification(ev.Context(), data); err != nil {
		// Post has no acknowledgement channel. Preserve the failure as an
		// owner-local retry request and re-read the current Run-owner fact;
		// never treat a failed projection CAS as successful delivery.
		slog.Warn("Workspace Run projection delivery failed; scheduling current-fact reconciliation", "workspace", data.WorkspaceID, "run", data.RunID, "err", err)
		m.requestRunProjectionReconciliation(err)
	}
}

func runProjectionTerminalState(state common.RunState) bool {
	switch state {
	case common.RunStateCompleted, common.RunStateFailed, common.RunStateCancelled, common.RunStateSuperseded:
		return true
	default:
		return false
	}
}

func (m *Biz) handleRunRuntimeCommandsReady(ev magicevent.Event, _ magicevent.Result) {
	data, ok := ev.Data().(runevents.RuntimeCommandsReadyData)
	if !ok || ev.Source() != runevents.CommandDestination || data.Validate() != nil || m == nil {
		return
	}
	m.requestRunProjectionReconciliation(nil)
}

func (m *Biz) publishWorkspaceUpdated(ctx context.Context, item *workspacemodel.Workspace) {
	if m == nil || m.EventHub() == nil || item == nil {
		return
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceStateChanged, modulecommon.ModuleID, workspaceevents.NotificationDestination, magicevent.NewHeader(), ctx, workspaceChangedNotification(item))
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), event.ID(), eventcontract.LaneScope{SubjectType: "workspace", SubjectID: string(item.ID), WorkspaceID: string(item.ID)}))
	m.PostEvent(event)
}

// workspaceChangedNotification is the public Workspace state projection used
// by EventHub, Eventstream and transport consumers. Owner-private fields such
// as RunProjectionCursor and persistence Revision are intentionally absent.
func workspaceChangedNotification(item *workspacemodel.Workspace) workspaceevents.WorkspaceChangedData {
	if item == nil {
		return workspaceevents.WorkspaceChangedData{}
	}
	return workspaceevents.WorkspaceChangedData{
		WorkspaceID: item.ID, Title: item.Title, Description: item.Description,
		DeliverableDirectory: item.DeliverableDirectory, InternalDirectory: item.InternalDirectory,
		State: item.State, ActiveRunID: item.ActiveRunID, DefaultLLMSelection: runtimecontract.CloneProviderInferenceSelection(item.DefaultLLMSelection),
	}
}

func sameWorkspaceChangedNotification(left, right workspaceevents.WorkspaceChangedData) bool {
	if left.WorkspaceID != right.WorkspaceID || left.Title != right.Title || left.Description != right.Description ||
		left.DeliverableDirectory != right.DeliverableDirectory || left.InternalDirectory != right.InternalDirectory ||
		left.State != right.State || left.ActiveRunID != right.ActiveRunID || left.RunID != right.RunID {
		return false
	}
	if left.DefaultLLMSelection == nil || right.DefaultLLMSelection == nil {
		return left.DefaultLLMSelection == nil && right.DefaultLLMSelection == nil
	}
	return left.DefaultLLMSelection.Same(*right.DefaultLLMSelection)
}

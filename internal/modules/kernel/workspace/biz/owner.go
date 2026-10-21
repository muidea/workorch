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
	subscriptions      []ownerSubscription
	lifecycle          basebiz.Lifecycle
	shutdownMu         sync.Mutex
	releasing          bool
	released           bool
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
	leave, err := m.enterOwner(false)
	if err != nil {
		return err
	}
	defer leave()
	if m.started {
		return nil
	}
	if len(m.subscriptions) != 0 {
		return common.ErrConflict
	}
	for _, topic := range workspaceCommandTopics() {
		if err := m.subscribeOwner(topic, m.handleCommand, false); err != nil {
			return err
		}
	}
	for _, topic := range m.lifecycleTopics() {
		if err := m.subscribeOwner(topic, m.handleLifecycleCommand, false); err != nil {
			return err
		}
	}
	for _, topic := range definitionAdmissionTopics() {
		if err := m.subscribeOwner(topic, m.handleDefinitionAdmissionCommand, false); err != nil {
			return err
		}
	}
	for _, topic := range definitionSpecificationTopics() {
		if err := m.subscribeOwner(topic, m.handleDefinitionSpecificationCommand, false); err != nil {
			return err
		}
	}
	if err := m.subscribeOwner(workspaceevents.TopicWorkspaceExactConfirmedStartupQuery, m.handleExactConfirmedStartupQuery, false); err != nil {
		return err
	}
	if err := m.subscribeOwner(workspaceevents.TopicWorkspacePreparationStateQuery, m.handlePreparationStateQuery, false); err != nil {
		return err
	}
	if err := m.subscribeOwner(workspaceevents.TopicWorkspaceCommandExecutionIntent, m.handleExecutionIntentCommand, false); err != nil {
		return err
	}
	// Run 事实仅供原 owner 投影合同，不从通知重建执行权威。
	if err := m.subscribeOwner(runevents.TopicRunStateChanged, m.handleRunStateChanged, true); err != nil {
		return err
	}
	if err := m.subscribeOwner(runevents.TopicRunRuntimeCommandsReady, m.handleRunRuntimeCommandsReady, true); err != nil {
		return err
	}
	if err := m.subscribeOwner(workspaceevents.TopicWorkspaceTemplateCommandEnsureDefaults, m.handleSystemDefaultsCommand, false); err != nil {
		return err
	}
	for _, topic := range []string{workspaceevents.TopicWorkspaceTemplateCommandQueryBuiltinDefaults, workspaceevents.TopicWorkspaceTemplateCommandUpgradeBuiltinDefault} {
		if err := m.subscribeOwner(topic, m.handleBuiltinTemplateDefaultsCommand, false); err != nil {
			return err
		}
	}
	if err := m.startRunProjectionReconciliation(); err != nil {
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

func (m *Biz) Teardown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.BeginShutdown(ctx)
	if err := m.Quiesce(ctx); err != nil {
		return err
	}
	m.shutdownMu.Lock()
	if m.releasing {
		m.shutdownMu.Unlock()
		return common.ErrConflict
	}
	m.releasing = true
	m.shutdownMu.Unlock()
	defer func() { m.shutdownMu.Lock(); m.releasing = false; m.shutdownMu.Unlock() }()
	if err := m.Quiesce(ctx); err != nil {
		return err
	}
	for len(m.subscriptions) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		last := len(m.subscriptions) - 1
		entry := m.subscriptions[last]
		var err error
		if entry.matched {
			err = m.UnsubscribeMatchedFuncChecked(ModuleID+"/run-projection", entry.topic)
		} else {
			err = m.UnsubscribeFuncChecked(entry.topic)
		}
		if err != nil {
			return err
		}
		m.subscriptions = m.subscriptions[:last]
	}
	m.manager = nil
	m.workspaces = nil
	m.cleaner = nil
	m.goals = nil
	m.dods = nil
	m.templates = nil
	m.started = false
	m.runProjectionStateMu.Lock()
	m.runProjectionCancel = nil
	m.runProjectionCtx = nil
	m.runProjectionPending = false
	m.runProjectionStateMu.Unlock()
	m.shutdownMu.Lock()
	m.released = true
	m.shutdownMu.Unlock()
	return nil
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

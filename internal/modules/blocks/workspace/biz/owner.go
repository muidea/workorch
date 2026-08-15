package workspace

import (
	"context"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/common"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const ModuleID = modulecommon.ModuleID

type Dependencies struct {
	Workspaces        repository.WorkspaceRepository
	Goals             repository.GoalRepository
	DoDs              repository.DoDRepository
	Templates         repository.WorkspaceTemplateRepository
	EventHub          magicevent.Hub
	BackgroundRoutine task.BackgroundRoutine
}

type Biz struct {
	basebiz.Base
	manager    *GoalDoDManager
	workspaces repository.WorkspaceRepository
	goals      repository.GoalRepository
	dods       repository.DoDRepository
	templates  repository.WorkspaceTemplateRepository
	topics     []string
	started    bool
}

func New(h magicevent.Hub, b task.BackgroundRoutine, deps Dependencies) (*Biz, error) {
	if deps.Workspaces == nil || deps.Goals == nil || deps.DoDs == nil || deps.Templates == nil || deps.EventHub == nil || h == nil {
		return nil, common.ErrInvalidArgument
	}
	bizPtr := &Biz{Base: basebiz.New(modulecommon.ModuleID, h, b), workspaces: deps.Workspaces, goals: deps.Goals, dods: deps.DoDs, templates: deps.Templates}
	bizPtr.manager = NewGoalDoDManagerWithPublisher(deps.Workspaces, deps.Goals, deps.DoDs, bizPtr.PostEvent)
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
	m.SubscribeFunc(workspaceevents.TopicWorkspaceCommandExecutionIntent, m.handleExecutionIntentCommand)
	m.topics = append(m.topics, workspaceevents.TopicWorkspaceCommandExecutionIntent)
	// Run owner is the sole publisher of lifecycle facts. Workspace consumes a
	// fact and posts an owner-local projection command; it never accepts an
	// Orchestrator-written Workspace model.
	m.SubscribeMatchedFunc(ModuleID+"/run-projection", runevents.NotificationDestination, runevents.TopicRunStateChanged, m.handleRunStateChanged)
	m.topics = append(m.topics, workspaceevents.TopicWorkspaceTemplateCommandEnsureDefaults)
	m.SubscribeFunc(workspaceevents.TopicWorkspaceTemplateCommandEnsureDefaults, m.handleSystemDefaultsCommand)
	m.started = true
	return nil
}

func (m *Biz) handleSystemDefaultsCommand(ev magicevent.Event, result magicevent.Result) {
	if _, ok := ev.Data().(workspaceevents.EnsureWorkspaceTemplateDefaultsCommand); !ok || m == nil || m.templates == nil {
		setSystemWorkspaceTemplateDefaultsResult(result, common.ErrInvalidArgument)
		return
	}
	setSystemWorkspaceTemplateDefaultsResult(result, ensureBuiltinWorkspaceTemplateDefaults(ev.Context(), m.templates))
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
	for idx := len(m.topics) - 1; idx >= 0; idx-- {
		m.UnsubscribeFunc(m.topics[idx])
	}
	m.UnsubscribeMatchedFunc(ModuleID+"/run-projection", runevents.TopicRunStateChanged)
	m.topics = nil
	m.manager = nil
	m.workspaces = nil
	m.goals = nil
	m.dods = nil
	m.templates = nil
	m.started = false
}

func (m *Biz) handleRunStateChanged(ev magicevent.Event, _ magicevent.Result) {
	data, ok := ev.Data().(runevents.RunStateChangedData)
	if !ok || ev.Source() != runevents.CommandDestination || data.Validate() != nil || m == nil || m.EventHub() == nil {
		return
	}
	switch data.State {
	case common.RunStatePending, common.RunStateActive, common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval:
		// A fresh-activation-required fact remains the active Run until the user
		// explicitly requests restart. Releasing this slot from a notification
		// would let a later strict resume create a new Run implicitly.
		if data.RequiresFreshActivation() {
			return
		}
		cmd := workspaceevents.RunProjectionFact{
			WorkspaceID:          data.WorkspaceID,
			RunID:                data.RunID,
			SourceOwner:          runevents.CommandDestination,
			CausationID:          data.CausationID,
			RunState:             data.State,
			RuntimeStateRevision: data.RuntimeStateRevision,
			RuntimeStateRef:      data.RuntimeStateRef,
		}
		// RunStateChanged is already an immutable cross-owner fact. Convert it
		// directly into the Workspace owner's private reducer input instead of
		// exposing a second public write command back to this owner.
		_, _ = m.submitWorkspaceLifecycleInternalEvent(ev.Context(), workspaceLifecycleInternalEvent{
			Kind:          workspaceLifecycleInternalEventRunProjectionObserved,
			RunProjection: &cmd,
		})
	}
}

func (m *Biz) publishWorkspaceUpdated(ctx context.Context, item *workspacemodel.Workspace) {
	if m == nil || m.EventHub() == nil || item == nil {
		return
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceStateChanged, modulecommon.ModuleID, workspaceevents.NotificationDestination, magicevent.NewHeader(), ctx, workspaceevents.WorkspaceChangedData{
		WorkspaceID: item.ID, Title: item.Title, Description: item.Description,
		DeliverableDirectory: item.DeliverableDirectory, InternalDirectory: item.InternalDirectory,
		State: item.State, ActiveRunID: item.ActiveRunID, DefaultLLMSelection: runtimecontract.CloneProviderInferenceSelection(item.DefaultLLMSelection),
	})
	event.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), event.ID(), eventcontract.LaneScope{SubjectType: "workspace", SubjectID: string(item.ID), WorkspaceID: string(item.ID)}))
	m.PostEvent(event)
}

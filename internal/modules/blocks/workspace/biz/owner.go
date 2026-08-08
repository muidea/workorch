package workspace

import (
	"context"
	"fmt"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/common"
	workspaceeventmapping "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/eventmapping"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/soulbinding"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
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
	m.topics = []string{
		workspaceevents.TopicWorkspaceCommandCreate,
		workspaceevents.TopicWorkspaceCommandGet,
		workspaceevents.TopicWorkspaceCommandList,
		workspaceevents.TopicWorkspaceCommandUpdate,
		workspaceevents.TopicWorkspaceCommandUpdateLLMSelection,
		workspaceevents.TopicWorkspaceCommandDelete,
		workspaceevents.TopicWorkspaceCommandDefineGoal,
		workspaceevents.TopicWorkspaceCommandUpdateGoal,
		workspaceevents.TopicWorkspaceCommandDeleteGoal,
		workspaceevents.TopicWorkspaceCommandGetGoal,
		workspaceevents.TopicWorkspaceCommandListGoalsByWS,
		workspaceevents.TopicWorkspaceCommandDefineDoD,
		workspaceevents.TopicWorkspaceCommandUpdateDoD,
		workspaceevents.TopicWorkspaceCommandDeleteDoD,
		workspaceevents.TopicWorkspaceCommandGetDoD,
		workspaceevents.TopicWorkspaceCommandListDoDsByWS,
		workspaceevents.TopicWorkspaceTemplateCommandCreate,
		workspaceevents.TopicWorkspaceTemplateCommandGet,
		workspaceevents.TopicWorkspaceTemplateCommandList,
		workspaceevents.TopicWorkspaceTemplateCommandUpdate,
		workspaceevents.TopicWorkspaceTemplateCommandDelete,
	}
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

func (m *Biz) handleCommand(ev magicevent.Event, result magicevent.Result) {
	response, err := m.executeCommand(ev)
	setWorkspaceMagicResult(result, response, err)
}

func (m *Biz) executeCommand(ev magicevent.Event) (workspaceevents.WorkspaceCommandResult, error) {
	if m == nil || m.manager == nil {
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
	}
	workspaceID, err := workspaceScopeFromCommand(ev)
	if err != nil {
		return workspaceevents.WorkspaceCommandResult{}, err
	}
	ctx := ev.Context()
	if workspaceID != "" {
		ctx, err = common.BindRuntimeWorkspaceContext(ctx, workspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
	}
	switch ev.ID() {
	case workspaceevents.TopicWorkspaceCommandCreate:
		cmd, ok := ev.Data().(workspaceevents.CreateWorkspaceCommand)
		if !ok {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.manager.CreateWorkspace(ctx, workspaceevents.CreateWorkspaceCommand{
			WorkspaceID: cmd.WorkspaceID, Title: cmd.Title, Description: cmd.Description,
			DeliverableDirectory: cmd.DeliverableDirectory, InternalDirectory: cmd.InternalDirectory,
		}); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{}, nil
	case workspaceevents.TopicWorkspaceCommandGet:
		cmd, ok := ev.Data().(workspaceevents.GetWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" || m.workspaces == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Workspace: workspaceeventmapping.WorkspaceDTO(item)}, nil
	case workspaceevents.TopicWorkspaceCommandList:
		if _, ok := ev.Data().(workspaceevents.ListWorkspacesCommand); !ok || m.workspaces == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.workspaces.List(ctx)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Workspaces: workspaceeventmapping.WorkspaceDTOs(items)}, nil
	case workspaceevents.TopicWorkspaceCommandUpdate:
		cmd, ok := ev.Data().(workspaceevents.UpdateWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" || m.workspaces == nil || soulbinding.Validate(cmd.SoulBindings) != nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		current, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		// Generic metadata updates deliberately preserve lifecycle-owned fields.
		// Workspace.State and ActiveRunID may only be changed by lifecycle_reducer.
		item := workspaceFromUpdateCommand(current, cmd)
		if err := m.workspaces.CompareAndSwap(ctx, item, item.Revision); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		m.publishWorkspaceUpdated(ctx, item)
		return workspaceevents.WorkspaceCommandResult{Workspace: workspaceeventmapping.WorkspaceDTO(item)}, nil
	case workspaceevents.TopicWorkspaceCommandUpdateLLMSelection:
		cmd, ok := ev.Data().(workspaceevents.UpdateWorkspaceLLMSelectionCommand)
		if !ok || cmd.WorkspaceID == "" || m.workspaces == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.workspaces.Get(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		if cmd.ExpectedRevision == 0 || item.Revision != cmd.ExpectedRevision {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrConflict
		}
		currentSelection := runtimecontract.CloneProviderInferenceSelection(item.DefaultLLMSelection)
		if cmd.Selection == nil {
			item.DefaultLLMSelection = nil
		} else {
			selection := cmd.Selection.Normalized()
			if err := selection.Validate(); err != nil {
				return workspaceevents.WorkspaceCommandResult{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
			}
			item.DefaultLLMSelection = &selection
		}
		if (currentSelection == nil && item.DefaultLLMSelection == nil) ||
			(currentSelection != nil && item.DefaultLLMSelection != nil && currentSelection.Same(*item.DefaultLLMSelection)) {
			return workspaceevents.WorkspaceCommandResult{Workspace: workspaceeventmapping.WorkspaceDTO(item), Changed: false}, nil
		}
		if err := m.workspaces.CompareAndSwap(ctx, item, cmd.ExpectedRevision); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		m.publishWorkspaceUpdated(ctx, item)
		return workspaceevents.WorkspaceCommandResult{Workspace: workspaceeventmapping.WorkspaceDTO(item), Changed: true}, nil
	case workspaceevents.TopicWorkspaceCommandDelete:
		cmd, ok := ev.Data().(workspaceevents.DeleteWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.manager.DeleteWorkspace(ctx, cmd.WorkspaceID); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{}, nil
	case workspaceevents.TopicWorkspaceCommandDefineGoal:
		cmd, ok := ev.Data().(workspaceevents.DefineGoalCommand)
		if !ok || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.manager.DefineGoal(ctx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		item, err := m.goals.Get(ctx, cmd.GoalID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Goal: workspacemodel.CloneGoal(item)}, nil
	case workspaceevents.TopicWorkspaceCommandUpdateGoal:
		cmd, ok := ev.Data().(workspaceevents.UpdateGoalCommand)
		if !ok || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.manager.UpdateGoal(ctx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		item, err := m.goals.Get(ctx, cmd.GoalID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Goal: workspacemodel.CloneGoal(item)}, nil
	case workspaceevents.TopicWorkspaceCommandDeleteGoal:
		cmd, ok := ev.Data().(workspaceevents.DeleteGoalCommand)
		if !ok || cmd.GoalID == "" || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.goals.Delete(ctx, cmd.GoalID); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{}, nil
	case workspaceevents.TopicWorkspaceCommandGetGoal:
		cmd, ok := ev.Data().(workspaceevents.GetGoalCommand)
		if !ok || cmd.GoalID == "" || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.goals.Get(ctx, cmd.GoalID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Goal: workspacemodel.CloneGoal(item)}, nil
	case workspaceevents.TopicWorkspaceCommandListGoalsByWS:
		cmd, ok := ev.Data().(workspaceevents.ListGoalsByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" || m.goals == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.goals.ListByWorkspaceID(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Goals: workspacemodel.CloneGoals(items)}, nil
	case workspaceevents.TopicWorkspaceCommandDefineDoD:
		cmd, ok := ev.Data().(workspaceevents.DefineDoDCommand)
		if !ok || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.manager.DefineDoD(ctx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		item, err := m.dods.Get(ctx, cmd.DoDID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{DoD: workspacemodel.CloneDoD(item)}, nil
	case workspaceevents.TopicWorkspaceCommandUpdateDoD:
		cmd, ok := ev.Data().(workspaceevents.UpdateDoDCommand)
		if !ok || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.manager.UpdateDoD(ctx, cmd); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		item, err := m.dods.Get(ctx, cmd.DoDID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{DoD: workspacemodel.CloneDoD(item)}, nil
	case workspaceevents.TopicWorkspaceCommandDeleteDoD:
		cmd, ok := ev.Data().(workspaceevents.DeleteDoDCommand)
		if !ok || cmd.DoDID == "" || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.dods.Delete(ctx, cmd.DoDID); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{}, nil
	case workspaceevents.TopicWorkspaceCommandGetDoD:
		cmd, ok := ev.Data().(workspaceevents.GetDoDCommand)
		if !ok || cmd.DoDID == "" || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.dods.Get(ctx, cmd.DoDID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{DoD: workspacemodel.CloneDoD(item)}, nil
	case workspaceevents.TopicWorkspaceCommandListDoDsByWS:
		cmd, ok := ev.Data().(workspaceevents.ListDoDsByWorkspaceCommand)
		if !ok || cmd.WorkspaceID == "" || m.dods == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.dods.ListByWorkspaceID(ctx, cmd.WorkspaceID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{DoDs: workspacemodel.CloneDoDs(items)}, nil
	case workspaceevents.TopicWorkspaceTemplateCommandCreate:
		cmd, ok := ev.Data().(workspaceevents.CreateWorkspaceTemplateCommand)
		if !ok || m.templates == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		item, err := workspaceTemplateFromCommand(workspaceevents.CreateWorkspaceTemplateCommand(cmd))
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		if err := m.templates.Create(ctx, item); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Template: workspaceTemplateFromRepository(item)}, nil
	case workspaceevents.TopicWorkspaceTemplateCommandGet:
		cmd, ok := ev.Data().(workspaceevents.GetWorkspaceTemplateCommand)
		if !ok || cmd.TemplateID == "" || m.templates == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		item, err := m.templates.Get(ctx, cmd.TemplateID)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Template: workspaceTemplateFromRepository(item)}, nil
	case workspaceevents.TopicWorkspaceTemplateCommandList:
		if _, ok := ev.Data().(workspaceevents.ListWorkspaceTemplatesCommand); !ok || m.templates == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		items, err := m.templates.List(ctx)
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Templates: workspaceTemplatesFromRepository(items)}, nil
	case workspaceevents.TopicWorkspaceTemplateCommandUpdate:
		cmd, ok := ev.Data().(workspaceevents.UpdateWorkspaceTemplateCommand)
		if !ok || m.templates == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		item, err := workspaceTemplateFromCommand(workspaceevents.CreateWorkspaceTemplateCommand(cmd))
		if err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		if err := m.templates.Update(ctx, item); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{Template: workspaceTemplateFromRepository(item)}, nil
	case workspaceevents.TopicWorkspaceTemplateCommandDelete:
		cmd, ok := ev.Data().(workspaceevents.DeleteWorkspaceTemplateCommand)
		if !ok || cmd.TemplateID == "" || m.templates == nil {
			return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
		}
		if err := m.templates.Delete(ctx, cmd.TemplateID); err != nil {
			return workspaceevents.WorkspaceCommandResult{}, err
		}
		return workspaceevents.WorkspaceCommandResult{}, nil
	default:
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
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

func workspaceScopeFromCommand(ev magicevent.Event) (common.WorkspaceID, error) {
	if ev == nil {
		return "", common.ErrInvalidArgument
	}
	switch cmd := ev.Data().(type) {
	case workspaceevents.CreateWorkspaceCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.GetWorkspaceCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.UpdateWorkspaceCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.UpdateWorkspaceLLMSelectionCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.DeleteWorkspaceCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.DefineGoalCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.UpdateGoalCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.DeleteGoalCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.GetGoalCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.ListGoalsByWorkspaceCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.DefineDoDCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.UpdateDoDCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.DeleteDoDCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.GetDoDCommand:
		return cmd.WorkspaceID, nil
	case workspaceevents.ListDoDsByWorkspaceCommand:
		return cmd.WorkspaceID, nil
	default:
		if ev.ID() == "" {
			return "", common.ErrInvalidArgument
		}
		return "", nil
	}
}

func setWorkspaceMagicResult(result magicevent.Result, data workspaceevents.WorkspaceCommandResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}

func workspaceTemplateFromCommand(cmd workspaceevents.CreateWorkspaceTemplateCommand) (*workspacemodel.WorkspaceTemplate, error) {
	if cmd.TemplateID == "" || soulbinding.Validate(cmd.SoulBindings) != nil {
		return nil, common.ErrInvalidArgument
	}
	return &workspacemodel.WorkspaceTemplate{
		ID:               cmd.TemplateID,
		Name:             cmd.Name,
		Description:      cmd.Description,
		GoalText:         cmd.GoalText,
		DoDItems:         append([]string(nil), cmd.DoDItems...),
		DefaultSoulID:    cmd.DefaultSoulID,
		SoulBindings:     soulbinding.StringMap(cmd.SoulBindings),
		ContextHints:     cmd.ContextHints.Clone(),
		EvidencePolicies: workorcheventpayload.CloneSlice(cmd.EvidencePolicies),
	}, nil
}

func workspaceTemplateFromRepository(item *workspacemodel.WorkspaceTemplate) *workspaceevents.WorkspaceTemplate {
	if item == nil {
		return nil
	}
	return &workspaceevents.WorkspaceTemplate{
		ID:               item.ID,
		Name:             item.Name,
		Description:      item.Description,
		GoalText:         item.GoalText,
		DoDItems:         append([]string(nil), item.DoDItems...),
		DefaultSoulID:    item.DefaultSoulID,
		SoulBindings:     soulbinding.FromStringMap(item.SoulBindings),
		ContextHints:     item.ContextHints.Clone(),
		EvidencePolicies: workorcheventpayload.CloneSlice(item.EvidencePolicies),
	}
}

func workspaceTemplatesFromRepository(items []*workspacemodel.WorkspaceTemplate) []*workspaceevents.WorkspaceTemplate {
	if items == nil {
		return nil
	}
	ret := make([]*workspaceevents.WorkspaceTemplate, 0, len(items))
	for _, item := range items {
		ret = append(ret, workspaceTemplateFromRepository(item))
	}
	return ret
}

func workspaceFromUpdateCommand(current *workspacemodel.Workspace, cmd workspaceevents.UpdateWorkspaceCommand) *workspacemodel.Workspace {
	if current == nil {
		return nil
	}
	item := *current
	item.Title = cmd.Title
	item.Description = cmd.Description
	item.DeliverableDirectory = cmd.DeliverableDirectory
	item.InternalDirectory = cmd.InternalDirectory
	item.TemplateID = cmd.TemplateID
	item.ProfileID = cmd.ProfileID
	item.DefaultLLMSelection = runtimecontract.CloneProviderInferenceSelection(cmd.DefaultLLMSelection)
	item.DefaultSoulID = cmd.DefaultSoulID
	item.SoulBindings = soulbinding.SoulIDMap(cmd.SoulBindings)
	item.ContextHints = cmd.ContextHints.Clone()
	item.EvidencePolicies = workorcheventpayload.CloneSlice(cmd.EvidencePolicies)
	return &item
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneSoulBindingMap(input map[string]common.SoulID) map[string]common.SoulID {
	if input == nil {
		return nil
	}
	ret := make(map[string]common.SoulID, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneMapSlice(input []map[string]any) []map[string]any {
	if input == nil {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		ret = append(ret, cloneMap(item))
	}
	return ret
}

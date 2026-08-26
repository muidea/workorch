package workspace

import (
	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type workspaceCommandFamily uint8

// workspaceCommandResponse is private reducer plumbing. It allows the four
// command families to share implementation helpers without leaking a tagged-
// by-populated-field union through EventHub.
type workspaceCommandResponse struct {
	Workspace  *workspaceevents.Workspace
	Workspaces []*workspaceevents.Workspace
	Changed    bool
	Goal       *workspacemodel.Goal
	Goals      []*workspacemodel.Goal
	DoD        *workspacemodel.DoD
	DoDs       []*workspacemodel.DoD
	Template   *workspaceevents.WorkspaceTemplate
	Templates  []*workspaceevents.WorkspaceTemplate
}

const (
	workspaceCommandFamilyDefinition workspaceCommandFamily = iota + 1
	workspaceCommandFamilyGoal
	workspaceCommandFamilyDoD
	workspaceCommandFamilyTemplate
)

func workspaceCommandTopics() []string {
	return []string{
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
}

func workspaceCommandFamilyForTopic(topic string) (workspaceCommandFamily, bool) {
	switch topic {
	case workspaceevents.TopicWorkspaceCommandCreate,
		workspaceevents.TopicWorkspaceCommandGet,
		workspaceevents.TopicWorkspaceCommandList,
		workspaceevents.TopicWorkspaceCommandUpdate,
		workspaceevents.TopicWorkspaceCommandUpdateLLMSelection,
		workspaceevents.TopicWorkspaceCommandDelete:
		return workspaceCommandFamilyDefinition, true
	case workspaceevents.TopicWorkspaceCommandDefineGoal,
		workspaceevents.TopicWorkspaceCommandUpdateGoal,
		workspaceevents.TopicWorkspaceCommandDeleteGoal,
		workspaceevents.TopicWorkspaceCommandGetGoal,
		workspaceevents.TopicWorkspaceCommandListGoalsByWS:
		return workspaceCommandFamilyGoal, true
	case workspaceevents.TopicWorkspaceCommandDefineDoD,
		workspaceevents.TopicWorkspaceCommandUpdateDoD,
		workspaceevents.TopicWorkspaceCommandDeleteDoD,
		workspaceevents.TopicWorkspaceCommandGetDoD,
		workspaceevents.TopicWorkspaceCommandListDoDsByWS:
		return workspaceCommandFamilyDoD, true
	case workspaceevents.TopicWorkspaceTemplateCommandCreate,
		workspaceevents.TopicWorkspaceTemplateCommandGet,
		workspaceevents.TopicWorkspaceTemplateCommandList,
		workspaceevents.TopicWorkspaceTemplateCommandUpdate,
		workspaceevents.TopicWorkspaceTemplateCommandDelete:
		return workspaceCommandFamilyTemplate, true
	default:
		return 0, false
	}
}

func (m *Biz) handleCommand(ev magicevent.Event, result magicevent.Result) {
	if ev != nil && ev.ID() == workspaceevents.TopicWorkspaceCommandDelete {
		if m == nil {
			setWorkspaceDeletionMagicResult(result, workspaceevents.DeleteWorkspaceResult{}, common.ErrInvalidArgument)
			return
		}
		m.deletionIngressMu.Lock()
		response, err := m.deleteWorkspace(ev.Context(), ev)
		m.deletionIngressMu.Unlock()
		setWorkspaceDeletionMagicResult(result, response, err)
		return
	}
	if m != nil && workspaceMutationTopic(ev) {
		m.deletionIngressMu.RLock()
		defer m.deletionIngressMu.RUnlock()
	}
	response, err := m.executeCommand(ev)
	setWorkspaceMagicResult(result, ev, response, err)
}

func workspaceMutationTopic(ev magicevent.Event) bool {
	if ev == nil {
		return false
	}
	switch ev.ID() {
	case workspaceevents.TopicWorkspaceCommandUpdate,
		workspaceevents.TopicWorkspaceCommandUpdateLLMSelection,
		workspaceevents.TopicWorkspaceCommandDefineGoal,
		workspaceevents.TopicWorkspaceCommandUpdateGoal,
		workspaceevents.TopicWorkspaceCommandDeleteGoal,
		workspaceevents.TopicWorkspaceCommandDefineDoD,
		workspaceevents.TopicWorkspaceCommandUpdateDoD,
		workspaceevents.TopicWorkspaceCommandDeleteDoD:
		return true
	default:
		return false
	}
}

func (m *Biz) executeCommand(ev magicevent.Event) (workspaceCommandResponse, error) {
	if m == nil || m.manager == nil {
		return workspaceCommandResponse{}, common.ErrInvalidArgument
	}
	workspaceID, err := workspaceScopeFromCommand(ev)
	if err != nil {
		return workspaceCommandResponse{}, err
	}
	ctx := ev.Context()
	if workspaceID != "" {
		ctx, err = common.BindRuntimeWorkspaceContext(ctx, workspaceID)
		if err != nil {
			return workspaceCommandResponse{}, err
		}
	}
	family, ok := workspaceCommandFamilyForTopic(ev.ID())
	if !ok {
		return workspaceCommandResponse{}, common.ErrInvalidArgument
	}
	switch family {
	case workspaceCommandFamilyDefinition:
		return m.executeWorkspaceDefinitionCommand(ctx, ev)
	case workspaceCommandFamilyGoal:
		return m.executeGoalCommand(ctx, ev)
	case workspaceCommandFamilyDoD:
		return m.executeDoDCommand(ctx, ev)
	case workspaceCommandFamilyTemplate:
		return m.executeWorkspaceTemplateCommand(ctx, ev)
	default:
		return workspaceCommandResponse{}, common.ErrInvalidArgument
	}
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

func setWorkspaceMagicResult(result magicevent.Result, ev magicevent.Event, data workspaceCommandResponse, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	if ev == nil {
		setWorkspaceCommandError(result, common.ErrInvalidArgument)
		return
	}
	switch ev.ID() {
	case workspaceevents.TopicWorkspaceCommandCreate,
		workspaceevents.TopicWorkspaceCommandGet,
		workspaceevents.TopicWorkspaceCommandUpdate:
		command, ok := ev.Data().(workspaceevents.WorkspaceResultCommand)
		response := workspaceevents.WorkspaceResult{Workspace: data.Workspace}
		if !ok || response.ValidateFor(command) != nil {
			setWorkspaceCommandError(result, common.ErrInvalidArgument)
			return
		}
		result.Set(response, nil)
	case workspaceevents.TopicWorkspaceCommandList:
		command, ok := ev.Data().(workspaceevents.ListWorkspacesCommand)
		response := workspaceevents.WorkspaceListResult{Workspaces: data.Workspaces}
		if !ok || response.ValidateFor(command) != nil {
			setWorkspaceCommandError(result, common.ErrInvalidArgument)
			return
		}
		result.Set(response, nil)
	case workspaceevents.TopicWorkspaceCommandUpdateLLMSelection:
		command, ok := ev.Data().(workspaceevents.UpdateWorkspaceLLMSelectionCommand)
		response := workspaceevents.WorkspaceLLMSelectionResult{Workspace: data.Workspace, Changed: data.Changed}
		if !ok || response.ValidateFor(command) != nil {
			setWorkspaceCommandError(result, common.ErrInvalidArgument)
			return
		}
		result.Set(response, nil)
	case workspaceevents.TopicWorkspaceCommandDefineGoal,
		workspaceevents.TopicWorkspaceCommandUpdateGoal,
		workspaceevents.TopicWorkspaceCommandGetGoal:
		command, ok := ev.Data().(workspaceevents.GoalResultCommand)
		response := workspaceevents.GoalResult{Goal: data.Goal}
		if !ok || response.ValidateFor(command) != nil {
			setWorkspaceCommandError(result, common.ErrInvalidArgument)
			return
		}
		result.Set(response, nil)
	case workspaceevents.TopicWorkspaceCommandListGoalsByWS:
		command, ok := ev.Data().(workspaceevents.ListGoalsByWorkspaceCommand)
		response := workspaceevents.GoalListResult{Goals: data.Goals}
		if !ok || response.ValidateFor(command) != nil {
			setWorkspaceCommandError(result, common.ErrInvalidArgument)
			return
		}
		result.Set(response, nil)
	case workspaceevents.TopicWorkspaceCommandDeleteGoal:
		command, ok := ev.Data().(workspaceevents.DeleteGoalCommand)
		response := workspaceevents.DeleteGoalResult{WorkspaceID: command.WorkspaceID, GoalID: command.GoalID}
		if !ok || response.ValidateFor(command) != nil {
			setWorkspaceCommandError(result, common.ErrInvalidArgument)
			return
		}
		result.Set(response, nil)
	case workspaceevents.TopicWorkspaceCommandDefineDoD,
		workspaceevents.TopicWorkspaceCommandUpdateDoD,
		workspaceevents.TopicWorkspaceCommandGetDoD:
		command, ok := ev.Data().(workspaceevents.DoDResultCommand)
		response := workspaceevents.DoDResult{DoD: data.DoD}
		if !ok || response.ValidateFor(command) != nil {
			setWorkspaceCommandError(result, common.ErrInvalidArgument)
			return
		}
		result.Set(response, nil)
	case workspaceevents.TopicWorkspaceCommandListDoDsByWS:
		command, ok := ev.Data().(workspaceevents.ListDoDsByWorkspaceCommand)
		response := workspaceevents.DoDListResult{DoDs: data.DoDs}
		if !ok || response.ValidateFor(command) != nil {
			setWorkspaceCommandError(result, common.ErrInvalidArgument)
			return
		}
		result.Set(response, nil)
	case workspaceevents.TopicWorkspaceCommandDeleteDoD:
		command, ok := ev.Data().(workspaceevents.DeleteDoDCommand)
		response := workspaceevents.DeleteDoDResult{WorkspaceID: command.WorkspaceID, DoDID: command.DoDID}
		if !ok || response.ValidateFor(command) != nil {
			setWorkspaceCommandError(result, common.ErrInvalidArgument)
			return
		}
		result.Set(response, nil)
	case workspaceevents.TopicWorkspaceTemplateCommandCreate,
		workspaceevents.TopicWorkspaceTemplateCommandGet,
		workspaceevents.TopicWorkspaceTemplateCommandUpdate:
		command, ok := ev.Data().(workspaceevents.WorkspaceTemplateResultCommand)
		response := workspaceevents.WorkspaceTemplateResult{Template: data.Template}
		if !ok || response.ValidateFor(command) != nil {
			setWorkspaceCommandError(result, common.ErrInvalidArgument)
			return
		}
		result.Set(response, nil)
	case workspaceevents.TopicWorkspaceTemplateCommandList:
		command, ok := ev.Data().(workspaceevents.ListWorkspaceTemplatesCommand)
		response := workspaceevents.WorkspaceTemplateListResult{Templates: data.Templates}
		if !ok || response.ValidateFor(command) != nil {
			setWorkspaceCommandError(result, common.ErrInvalidArgument)
			return
		}
		result.Set(response, nil)
	case workspaceevents.TopicWorkspaceTemplateCommandDelete:
		command, ok := ev.Data().(workspaceevents.DeleteWorkspaceTemplateCommand)
		response := workspaceevents.DeleteWorkspaceTemplateResult{TemplateID: command.TemplateID}
		if !ok || response.ValidateFor(command) != nil {
			setWorkspaceCommandError(result, common.ErrInvalidArgument)
			return
		}
		result.Set(response, nil)
	default:
		setWorkspaceCommandError(result, common.ErrInvalidArgument)
	}
}

func setWorkspaceCommandError(result magicevent.Result, err error) {
	result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
}

func setWorkspaceDeletionMagicResult(result magicevent.Result, data workspaceevents.DeleteWorkspaceResult, err error) {
	if result == nil {
		return
	}
	if err != nil {
		result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: err.Error(), Cause: err})
		return
	}
	result.Set(data, nil)
}

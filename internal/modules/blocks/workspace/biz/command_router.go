package workspace

import (
	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

type workspaceCommandFamily uint8

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
	setWorkspaceMagicResult(result, response, err)
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
	family, ok := workspaceCommandFamilyForTopic(ev.ID())
	if !ok {
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
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
		return workspaceevents.WorkspaceCommandResult{}, common.ErrInvalidArgument
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

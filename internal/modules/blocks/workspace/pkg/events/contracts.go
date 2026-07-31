package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("workspace", eventcontract.CommandClassNotify, eventcontract.LanePolicyState,
		TopicWorkspaceCreated,
		TopicWorkspaceStateChanged,
		TopicWorkspaceDeleted,
		TopicGoalDefined,
		TopicDoDDefined,
	)...)
	ret = append(ret, eventcontract.Build("workspace", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicWorkspaceCommandGet,
		TopicWorkspaceCommandList,
		TopicWorkspaceCommandGetGoal,
		TopicWorkspaceCommandListGoalsByWS,
		TopicWorkspaceCommandGetDoD,
		TopicWorkspaceCommandListDoDsByWS,
		TopicWorkspaceTemplateCommandGet,
		TopicWorkspaceTemplateCommandList,
	)...)
	ret = append(ret, eventcontract.Build("workspace", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicWorkspaceCommandCreate,
		TopicWorkspaceCommandEnsure,
		TopicWorkspaceCommandUpdate,
		TopicWorkspaceCommandUpdateLLMTarget,
		TopicWorkspaceCommandDelete,
		TopicWorkspaceCommandPurge,
		TopicWorkspaceCommandDefineGoal,
		TopicWorkspaceCommandUpdateGoal,
		TopicWorkspaceCommandDeleteGoal,
		TopicWorkspaceCommandDefineDoD,
		TopicWorkspaceCommandUpdateDoD,
		TopicWorkspaceCommandDeleteDoD,
		TopicWorkspaceTemplateCommandCreate,
		TopicWorkspaceTemplateCommandUpdate,
		TopicWorkspaceTemplateCommandDelete,
		TopicWorkspaceCommandApplyRunProjectionFact,
		TopicWorkspaceCommandExecutionIntent,
		TopicWorkspaceCommandReleaseFreshActivationRunSlot,
		TopicWorkspaceCommandApplyTerminalDelivery,
	)...)
	return ret
}

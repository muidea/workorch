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
		TopicWorkspaceCommandUpdate,
		TopicWorkspaceCommandUpdateLLMSelection,
		TopicWorkspaceCommandDelete,
		TopicWorkspaceCommandDefineGoal,
		TopicWorkspaceCommandUpdateGoal,
		TopicWorkspaceCommandDeleteGoal,
		TopicWorkspaceCommandDefineDoD,
		TopicWorkspaceCommandUpdateDoD,
		TopicWorkspaceCommandDeleteDoD,
		TopicWorkspaceTemplateCommandCreate,
		TopicWorkspaceTemplateCommandUpdate,
		TopicWorkspaceTemplateCommandDelete,
		TopicWorkspaceTemplateCommandEnsureDefaults,
		TopicWorkspaceCommandExecutionIntent,
		TopicWorkspaceFactTerminalDelivery,
		TopicWorkspaceCommandSubmitDefinitionAdmissionCandidate,
		TopicWorkspaceCommandPromoteDefinitionAdmissionCandidate,
		TopicWorkspaceCommandAcknowledgeDefinitionClarification,
	)...)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassWrite, eventcontract.StateMachineSM1, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicWorkspaceCommandExecutionIntent)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassWrite, eventcontract.StateMachineSM4, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicWorkspaceFactTerminalDelivery)
	return ret
}

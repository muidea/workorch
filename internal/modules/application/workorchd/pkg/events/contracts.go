package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := eventcontract.Build("workorchd.service", eventcontract.CommandClassNotify, eventcontract.LanePolicyTopic,
		TopicWorkorchDServiceStopped,
	)
	ret = append(ret, eventcontract.Build("workorchd.scheduler", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicWorkorchDCommandWorkspaceRunRequested,
	)...)
	ret = append(ret, eventcontract.Build("workorchd.workspace_runtime_view", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicWorkorchDCommandReadWorkspaceRuntimeView,
	)...)
	ret = append(ret, eventcontract.Build("workorchd.workspace_projection", eventcontract.CommandClassNotify, eventcontract.LanePolicyState,
		TopicWorkspaceProjectionChanged,
	)...)
	ret = append(ret, eventcontract.Build("cli.workspace", eventcontract.CommandClassFacade, eventcontract.LanePolicyTopic, TopicCLIWorkspaceCreate, TopicCLIWorkspaceGet, TopicCLIWorkspaceList, TopicCLIWorkspaceSnapshot, TopicCLIWorkspaceDelete, TopicCLIWorkspaceUpdateLLMTarget)...)
	ret = append(ret, eventcontract.Build("cli.goal", eventcontract.CommandClassFacade, eventcontract.LanePolicyTopic, TopicCLIGoalDefine, TopicCLIGoalList, TopicCLIGoalGet, TopicCLIGoalUpdate)...)
	ret = append(ret, eventcontract.Build("cli.dod", eventcontract.CommandClassFacade, eventcontract.LanePolicyTopic, TopicCLIDoDDefine, TopicCLIDoDList, TopicCLIDoDGet, TopicCLIDoDUpdate)...)
	ret = append(ret, eventcontract.Build("cli.run", eventcontract.CommandClassFacade, eventcontract.LanePolicyTopic, TopicCLIRunGet, TopicCLIRunList, TopicCLIRunStatus)...)
	ret = append(ret, eventcontract.Build("cli.decision", eventcontract.CommandClassFacade, eventcontract.LanePolicyTopic, TopicCLIDecisionResolve, TopicCLIDecisionBatchResolve, TopicCLIDecisionList, TopicCLIDecisionGet)...)
	ret = append(ret, eventcontract.Build("cli.approval", eventcontract.CommandClassFacade, eventcontract.LanePolicyTopic, TopicCLIApprovalApprove, TopicCLIApprovalReject, TopicCLIApprovalBatchResolve, TopicCLIApprovalList, TopicCLIApprovalGet)...)
	ret = append(ret, eventcontract.Build("cli.soul", eventcontract.CommandClassFacade, eventcontract.LanePolicyTopic, TopicCLISoulCreate, TopicCLISoulGet, TopicCLISoulList, TopicCLISoulUpdate, TopicCLISoulDelete)...)
	ret = append(ret, eventcontract.Build("cli.skill", eventcontract.CommandClassFacade, eventcontract.LanePolicyTopic, TopicCLISkillCreate, TopicCLISkillGet, TopicCLISkillList)...)
	ret = append(ret, eventcontract.Build("cli.mcp", eventcontract.CommandClassFacade, eventcontract.LanePolicyTopic, TopicCLIMCPServerCreate, TopicCLIMCPServerGet, TopicCLIMCPServerList, TopicCLIMCPServerUpdate, TopicCLIMCPCapabilityList)...)
	ret = append(ret, eventcontract.Build("cli.memory", eventcontract.CommandClassFacade, eventcontract.LanePolicyTopic, TopicCLIMemoryCreate, TopicCLIMemoryGet, TopicCLIMemoryUpdate, TopicCLIMemoryDelete, TopicCLIMemoryList, TopicCLIMemorySearch)...)
	ret = append(ret, eventcontract.Build("cli.template", eventcontract.CommandClassFacade, eventcontract.LanePolicyTopic, TopicCLITemplateCreate, TopicCLITemplateGet, TopicCLITemplateList, TopicCLITemplateUpdate, TopicCLITemplateDelete)...)
	ret = append(ret, eventcontract.Build("cli.configuration", eventcontract.CommandClassFacade, eventcontract.LanePolicyTopic, TopicCLIConfigurationGet, TopicCLIConfigurationRefreshChatModels)...)
	ret = append(ret, eventcontract.Build("cli", eventcontract.CommandClassNotify, eventcontract.LanePolicyTopic,
		TopicCLIProgress,
	)...)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassWrite, eventcontract.StateMachineSM1, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicWorkorchDCommandWorkspaceRunRequested)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassRead, eventcontract.StateMachineSM4, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, false), TopicWorkorchDCommandReadWorkspaceRuntimeView)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassNotify, eventcontract.StateMachineSM4, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, false), TopicWorkspaceProjectionChanged)
	return ret
}

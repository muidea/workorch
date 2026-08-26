package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("run", eventcontract.CommandClassNotify, eventcontract.LanePolicyState,
		TopicRunCreated,
		TopicRunStarted,
		TopicRunBlockedOnDecision,
		TopicRunBlockedOnApproval,
		TopicRunCompleted,
		TopicRunFailed,
		TopicRunCancelled,
		TopicRunSuperseded,
		TopicRunStateChanged,
		TopicRunContinuationRequested,
		TopicRunRuntimeCommandsReady,
	)...)
	ret = append(ret, eventcontract.Build("run", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicRunCommandGet,
		TopicRunCommandList,
		TopicRunCommandListByWorkspace,
		TopicRunCommandLifecycleFactInventory,
		TopicRunCommandRuntimeRecoveryInventory,
		TopicRunCommandWorkspaceRetirementFact,
		TopicRunCommandWorkspaceRetirementInventory,
		TopicRunCommandRuntimeStateRecordReadiness,
		TopicRunCommandRuntimeStateRecord,
		TopicRunCommandGetStep,
		TopicRunCommandListStepsByRun,
		TopicRunCommandListStepSummariesByRun,
		TopicRunCommandGetRuntimeCheckpoint,
		TopicRunCommandListCheckpointsByRun,
	)...)
	ret = append(ret, eventcontract.Build("run", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicRunCommandStoreRuntimeStateRecord,
		TopicRunCommandPrepareWorkspaceRetirement,
		TopicRunCommandFinalizeWorkspaceRetirement,
		TopicRunCommandPurgeWorkspace,
		TopicRunCommandWorkspaceExecutionIntent,
		TopicRunFactGovernanceWait,
		TopicRunFactContinuationEligible,
		TopicRunFactTerminalDelivery,
	)...)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassNotify, eventcontract.StateMachineSM1, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true),
		TopicRunCreated, TopicRunStarted, TopicRunBlockedOnDecision, TopicRunBlockedOnApproval,
		TopicRunCompleted, TopicRunFailed, TopicRunCancelled, TopicRunSuperseded, TopicRunStateChanged,
		TopicRunContinuationRequested,
	)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassNotify, eventcontract.StateMachineSM4, eventcontract.ScopePolicyProcess, eventcontract.IdentityPolicyProcessLifecycle, false), TopicRunRuntimeCommandsReady)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassRead, eventcontract.StateMachineSM1, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicRunCommandLifecycleFactInventory)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassRead, eventcontract.StateMachineSM4, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true),
		TopicRunCommandRuntimeRecoveryInventory, TopicRunCommandRuntimeStateRecordReadiness, TopicRunCommandRuntimeStateRecord,
		TopicRunCommandGetRuntimeCheckpoint, TopicRunCommandListCheckpointsByRun,
	)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassWrite, eventcontract.StateMachineSM1, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true),
		TopicRunCommandWorkspaceExecutionIntent,
		TopicRunFactGovernanceWait, TopicRunFactContinuationEligible,
		TopicRunFactTerminalDelivery,
	)
	ret = eventcontract.Annotate(ret, eventcontract.OpaquePersistenceMetadata(eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority), TopicRunCommandStoreRuntimeStateRecord)
	return ret
}

package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("run", eventcontract.CommandClassNotify, eventcontract.LanePolicyState,
		TopicRunCreated,
		TopicRunStarted,
		TopicRunResumed,
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
		TopicRunCommandRuntimeRecoveryInventory,
		TopicRunCommandRuntimeStateRecordReadiness,
		TopicRunCommandRuntimeStateRecord,
		TopicRunCommandGetStep,
		TopicRunCommandListStepsByRun,
		TopicRunCommandListStepSummariesByRun,
		TopicRunCommandGetRuntimeCheckpoint,
		TopicRunCommandListCheckpointsByRun,
	)...)
	ret = append(ret, eventcontract.Build("run", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicRunCommandCreate,
		TopicRunCommandEnsure,
		TopicRunCommandPurgeRuntimeResources,
		TopicRunCommandUpdate,
		TopicRunCommandLinkSuccessor,
		TopicRunCommandDelete,
		TopicRunCommandActivate,
		TopicRunCommandWorkspaceExecutionIntent,
		TopicRunCommandContinue,
		TopicRunCommandApplyGovernanceWait,
		TopicRunCommandApplyContinuationEligible,
		TopicRunCommandApplyTerminalDelivery,
		TopicRunCommandApplyRuntimeStateIncompatible,
	)...)
	return ret
}

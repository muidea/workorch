package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("orchestrator", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicOrchestratorCommandListToolCallsByRun,
		TopicOrchestratorCommandListPatchesByRun,
		TopicOrchestratorCommandListValidationResultsByRun,
		TopicOrchestratorCommandListCoverageWorkItemsByRun,
		TopicOrchestratorCommandListArtifactContinuationsByRun,
		TopicOrchestratorQueryRuntimeOperationReceipt,
	)...)
	ret = append(ret, eventcontract.Build("orchestrator", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicOrchestratorCommandPurgeRun,
		TopicOrchestratorCommitRuntimeGovernanceResolution,
		TopicOrchestratorCommitRuntimeGovernanceTicket,
		TopicOrchestratorRequestRuntimeTermination,
		TopicOrchestratorCommitRuntimeOperationReceipt,
	)...)
	ret = append(ret, eventcontract.Build("orchestrator", eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute,
		TopicOrchestratorDriveRuntime,
	)...)
	ret = append(ret, eventcontract.Build("orchestrator", eventcontract.CommandClassNotify, eventcontract.LanePolicyState,
		TopicOrchestratorRetryRuntimeDrive,
		TopicPatchPreviewed,
		TopicPatchApplied,
		TopicPatchFailed,
		TopicPatchReverted,
		TopicArtifactContinuationCreated,
		TopicArtifactContinuationChunkRecorded,
		TopicArtifactContinuationChunkRejected,
		TopicArtifactContinuationAssembled,
		TopicArtifactContinuationApplied,
		TopicArtifactContinuationFailed,
		TopicOrchestratorRuntimeGovernanceWaitRecorded,
		TopicOrchestratorRuntimeContinuationEligible,
		TopicOrchestratorRuntimeTerminalDeliveryRequested,
		TopicOrchestratorRuntimeDriveObserved,
	)...)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassRead, eventcontract.StateMachineSM4, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true),
		TopicOrchestratorCommandListToolCallsByRun, TopicOrchestratorCommandListPatchesByRun,
		TopicOrchestratorCommandListValidationResultsByRun, TopicOrchestratorCommandListCoverageWorkItemsByRun,
		TopicOrchestratorCommandListArtifactContinuationsByRun, TopicOrchestratorQueryRuntimeOperationReceipt,
	)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassWrite, eventcontract.StateMachineSM2, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true),
		TopicOrchestratorCommitRuntimeGovernanceResolution, TopicOrchestratorCommitRuntimeGovernanceTicket,
	)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassWrite, eventcontract.StateMachineSM3, eventcontract.ScopePolicyWorkspaceRunStep, eventcontract.IdentityPolicyRuntimeOperation, true), TopicOrchestratorCommitRuntimeOperationReceipt)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassWrite, eventcontract.StateMachineSM4, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicOrchestratorCommandPurgeRun, TopicOrchestratorRequestRuntimeTermination)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassExecute, eventcontract.StateMachineSM4, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicOrchestratorDriveRuntime)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassNotify, eventcontract.StateMachineSM3, eventcontract.ScopePolicyWorkspaceRunStep, eventcontract.IdentityPolicyRuntimeOperation, true),
		TopicPatchPreviewed, TopicPatchApplied, TopicPatchFailed, TopicPatchReverted,
		TopicArtifactContinuationCreated, TopicArtifactContinuationChunkRecorded, TopicArtifactContinuationChunkRejected,
		TopicArtifactContinuationAssembled, TopicArtifactContinuationApplied, TopicArtifactContinuationFailed,
	)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassNotify, eventcontract.StateMachineSM2, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicOrchestratorRuntimeGovernanceWaitRecorded, TopicOrchestratorRuntimeContinuationEligible)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassNotify, eventcontract.StateMachineSM4, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true), TopicOrchestratorRetryRuntimeDrive, TopicOrchestratorRuntimeTerminalDeliveryRequested, TopicOrchestratorRuntimeDriveObserved)
	return ret
}

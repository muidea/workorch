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
		TopicOrchestratorAcquireTargetSnapshot,
		TopicOrchestratorAcquireWorkspaceManifest,
		TopicOrchestratorAcquireProviderCeiling,
		TopicOrchestratorAcquireRuntimeCapabilities,
	)...)
	ret = append(ret, eventcontract.Build("orchestrator", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicOrchestratorCommandStartRun,
		TopicOrchestratorCommandBlockDecision,
		TopicOrchestratorCommandBlockApproval,
		TopicOrchestratorCommandResumeRun,
		TopicOrchestratorCommandCancelRun,
		TopicOrchestratorCommandSupersedeRun,
		TopicOrchestratorCommandFailRun,
		TopicOrchestratorCommandPurgeRun,
		TopicOrchestratorCommandReleaseRuntimeV1Resources,
		TopicOrchestratorCommitRuntimeGovernanceResolution,
		TopicOrchestratorCommitRuntimeGovernanceTicket,
		TopicOrchestratorOpenRuntimeGovernanceRequest,
	)...)
	ret = append(ret, eventcontract.Build("orchestrator", eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute,
		TopicOrchestratorDriveRuntimeV2,
		TopicOrchestratorRequestRuntimeCandidate,
		TopicOrchestratorExecuteRuntimeOperation,
		TopicOrchestratorExecuteFileEditOperation,
		TopicOrchestratorExecutePatchApplyOperation,
	)...)
	ret = append(ret, eventcontract.Build("orchestrator", eventcontract.CommandClassNotify, eventcontract.LanePolicyState,
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
	)...)
	return ret
}

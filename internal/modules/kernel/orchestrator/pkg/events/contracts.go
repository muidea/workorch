package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := []eventcontract.Contract{}
	ret = append(ret, eventcontract.Build("orchestrator", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicOrchestratorCommandListToolCallsByRun,
		TopicOrchestratorCommandListPatchesByRun,
		TopicOrchestratorCommandListValidationResultsByRun,
		TopicOrchestratorCommandListArtifactContinuationsByRun,
	)...)
	ret = append(ret, eventcontract.Build("orchestrator", eventcontract.CommandClassWrite, eventcontract.LanePolicyState,
		TopicOrchestratorCommandStartRun,
		TopicOrchestratorCommandBlockDecision,
		TopicOrchestratorCommandBlockApproval,
		TopicOrchestratorCommandResumeRun,
		TopicOrchestratorCommandValidateRun,
		TopicOrchestratorCommandCompleteRun,
		TopicOrchestratorCommandCancelRun,
		TopicOrchestratorCommandSupersedeRun,
		TopicOrchestratorCommandFailRun,
		TopicOrchestratorCommandPurgeRun,
		TopicOrchestratorCommandUpdateRuntimePolicy,
	)...)
	ret = append(ret, eventcontract.Build("orchestrator", eventcontract.CommandClassNotify, eventcontract.LanePolicyTopic,
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

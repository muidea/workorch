package events

import "github.com/muidea/workorch/internal/pkg/eventcontract"

func Contracts() []eventcontract.Contract {
	ret := eventcontract.Build("file", eventcontract.CommandClassRead, eventcontract.LanePolicyQuery,
		TopicAcquireRuntimeCapabilities,
		TopicAcquireRuntimeOperationAdmissibility,
		TopicValidateRuntimeOperationCandidateConformance,
		TopicProbeRuntimeOperationFeasibility, TopicProbeRuntimeSemanticProofFeasibility, TopicQueryRuntimeCWIReconciliation,
		TopicQueryRuntimeFileReadDependencies,
		TopicAcquireTargetSnapshot,
		TopicAcquireRuntimeTargetSelection,
		TopicAcquireWorkspaceManifest,
		TopicConfirmRuntimeRepairSurface,
		TopicAcquireAuthoringResources,
		TopicVerifyAuthoringResource,
		TopicObservePreparationDirectories,
		TopicObservePreparationResources,
		TopicObservePreparationManagedConfigs,
		TopicReobservePreparationFiles,
		TopicQueryPreparationEpoch,
	)
	ret = append(ret, eventcontract.Build("file", eventcontract.CommandClassExecute, eventcontract.LanePolicyExecute,
		TopicExecuteFileListOperation,
		TopicExecuteFileReadOperation,
		TopicExecuteFileEditOperation,
		TopicExecuteFileWriteOperation,
		TopicExecutePatchApplyOperation,
		TopicExecutePatchPreviewOperation,
	)...)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassRead, eventcontract.StateMachineSM2, eventcontract.ScopePolicyWorkspaceRun, eventcontract.IdentityPolicyRuntimeAuthority, true),
		TopicAcquireRuntimeCapabilities, TopicAcquireRuntimeOperationAdmissibility, TopicProbeRuntimeOperationFeasibility,
		TopicProbeRuntimeSemanticProofFeasibility, TopicQueryRuntimeCWIReconciliation, TopicAcquireTargetSnapshot,
		TopicAcquireRuntimeTargetSelection, TopicAcquireWorkspaceManifest, TopicConfirmRuntimeRepairSurface,
	)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassRead, eventcontract.StateMachineSM3, eventcontract.ScopePolicyWorkspaceRunStep, eventcontract.IdentityPolicyRuntimeOperation, true),
		TopicValidateRuntimeOperationCandidateConformance, TopicQueryRuntimeFileReadDependencies,
	)
	ret = eventcontract.Annotate(ret, eventcontract.RuntimeMetadata(eventcontract.CommandClassExecute, eventcontract.StateMachineSM3, eventcontract.ScopePolicyWorkspaceRunStep, eventcontract.IdentityPolicyRuntimeOperation, true),
		TopicExecuteFileListOperation, TopicExecuteFileReadOperation, TopicExecuteFileEditOperation,
		TopicExecuteFileWriteOperation, TopicExecutePatchApplyOperation, TopicExecutePatchPreviewOperation,
	)
	return ret
}

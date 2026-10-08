package runtime

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// CloneRaw 仅为测试 repository 提供独立副本，不规范化、不校验或重建 authority。
// 独立前像 CAS 不能读取被调用方通过共享指针或 slice 提前改写的存储值。
func (a RuntimeAggregate) CloneRaw() RuntimeAggregate {
	record := runtimecontract.RuntimeStateRecord{
		ResultFeedbackVersion: a.ResultFeedbackVersion,
		StateRevision: a.StateRevision, EvidenceSequence: a.EvidenceSequence,
		Definition: a.Definition, Facts: a.Facts, Decision: a.Decision,
		SemanticPlanning: a.SemanticPlanning, ImplementationWork: a.ImplementationWork,
		Reconciliation: a.Reconciliation, CoverageProofLineage: a.CoverageProofLineage,
		ProcessedEventIDs: a.ProcessedEventIDs, RepairSurfaceConfirmations: a.RepairSurfaceConfirmations,
		Diagnostics: a.Diagnostics, CandidateRequestSessions: a.CandidateRequestSessions,
		CandidateAttempts: a.CandidateAttempts, ExecutionBudget: a.ExecutionBudget,
		RecoverableWaits: a.RecoverableWaits, OperationRecoveryAuthorizations: a.OperationRecoveryAuthorizations,
		DefinitionClarification: a.DefinitionClarification, GovernanceRequests: a.GovernanceRequests,
		GovernanceResolutions: a.GovernanceResolutions, OperationGovernanceBindings: a.OperationGovernanceBindings,
		OperationReceipts: a.OperationReceipts, DeliveryAcknowledgements: a.DeliveryAcknowledgements,
		TerminalMaterialSeal: a.TerminalMaterialSeal, PrerequisiteInvalidations: a.PrerequisiteInvalidations,
		DriveLease: a.DriveLease, Termination: a.Termination,
	}.Clone()
	out := RuntimeAggregate{
		ResultFeedbackVersion: record.ResultFeedbackVersion,
		StateRevision: record.StateRevision, EvidenceSequence: record.EvidenceSequence,
		Definition: record.Definition, Facts: record.Facts, Decision: record.Decision,
		SemanticPlanning: record.SemanticPlanning, ImplementationWork: record.ImplementationWork,
		Reconciliation: record.Reconciliation, CoverageProofLineage: record.CoverageProofLineage,
		ProcessedEventIDs: record.ProcessedEventIDs, RepairSurfaceConfirmations: record.RepairSurfaceConfirmations,
		Diagnostics: record.Diagnostics, CandidateRequestSessions: record.CandidateRequestSessions,
		CandidateAttempts: record.CandidateAttempts, ExecutionBudget: record.ExecutionBudget,
		RecoverableWaits: record.RecoverableWaits, OperationRecoveryAuthorizations: record.OperationRecoveryAuthorizations,
		DefinitionClarification: record.DefinitionClarification, GovernanceRequests: record.GovernanceRequests,
		GovernanceResolutions: record.GovernanceResolutions, OperationGovernanceBindings: record.OperationGovernanceBindings,
		OperationReceipts: record.OperationReceipts, DeliveryAcknowledgements: record.DeliveryAcknowledgements,
		TerminalMaterialSeal: record.TerminalMaterialSeal, PrerequisiteInvalidations: record.PrerequisiteInvalidations,
		DriveLease: record.DriveLease, Termination: record.Termination,
	}
	if a.ActiveDispatch != nil {
		value := cloneSealedDispatchRaw(*a.ActiveDispatch)
		out.ActiveDispatch = &value
	}
	if a.CompletedDispatches != nil {
		out.CompletedDispatches = make([]SealedDispatch, len(a.CompletedDispatches))
		for index, value := range a.CompletedDispatches {
			out.CompletedDispatches[index] = cloneSealedDispatchRaw(value)
		}
	}
	return out
}

func cloneSealedDispatchRaw(value SealedDispatch) SealedDispatch {
	record := runtimecontract.CloneRuntimeDispatchRecordRaw(runtimecontract.RuntimeDispatchRecord{
		ID: value.ID, WorkspaceID: value.WorkspaceID, RunID: value.RunID,
		Transaction: value.Transaction, CandidateHash: value.CandidateHash, Candidate: value.Candidate,
		CandidateManifestHash: value.CandidateManifestHash,
		ImplementationWorkCommitment: value.ImplementationWorkCommitment,
		CoverageBindingHash: value.CoverageBindingHash, CoverageBindings: value.CoverageBindings,
		OperationTargetAuthorizations: value.OperationTargetAuthorizations, FocusTargetRole: value.FocusTargetRole,
		ValidationRecipes: value.ValidationRecipes,
		RepairCommitmentHash: value.RepairCommitmentHash, RepairCommitment: value.RepairCommitment,
		AssessmentAuthorityInputHash: value.AssessmentAuthorityInputHash, AssessmentAuthorityInput: value.AssessmentAuthorityInput,
		AssessmentAdjudicationHash: value.AssessmentAdjudicationHash, AssessmentAdjudication: value.AssessmentAdjudication,
		OperationAdmissibilities: value.OperationAdmissibilities,
		OperationAdmissibilityObservations: value.OperationAdmissibilityObservations,
		OperationPolicies: value.OperationPolicies, State: runtimecontract.RuntimeDispatchState(value.State),
		CompletedOperationIDs: value.CompletedOperationIDs, OperationExecutions: value.OperationExecutions,
		OperationAuthorityHash: value.OperationAuthorityHash, DispatchAuthorityHash: value.DispatchAuthorityHash,
	})
	return SealedDispatch{
		ID: record.ID, WorkspaceID: record.WorkspaceID, RunID: record.RunID,
		Transaction: record.Transaction, CandidateHash: record.CandidateHash, Candidate: record.Candidate,
		CandidateManifestHash: record.CandidateManifestHash,
		ImplementationWorkCommitment: record.ImplementationWorkCommitment,
		CoverageBindingHash: record.CoverageBindingHash, CoverageBindings: record.CoverageBindings,
		OperationTargetAuthorizations: record.OperationTargetAuthorizations, FocusTargetRole: record.FocusTargetRole,
		ValidationRecipes: record.ValidationRecipes,
		RepairCommitmentHash: record.RepairCommitmentHash, RepairCommitment: record.RepairCommitment,
		AssessmentAuthorityInputHash: record.AssessmentAuthorityInputHash, AssessmentAuthorityInput: record.AssessmentAuthorityInput,
		AssessmentAdjudicationHash: record.AssessmentAdjudicationHash, AssessmentAdjudication: record.AssessmentAdjudication,
		OperationAdmissibilities: record.OperationAdmissibilities,
		OperationAdmissibilityObservations: record.OperationAdmissibilityObservations,
		OperationPolicies: record.OperationPolicies, State: DispatchState(record.State),
		CompletedOperationIDs: record.CompletedOperationIDs, OperationExecutions: record.OperationExecutions,
		OperationAuthorityHash: record.OperationAuthorityHash, DispatchAuthorityHash: record.DispatchAuthorityHash,
	}
}

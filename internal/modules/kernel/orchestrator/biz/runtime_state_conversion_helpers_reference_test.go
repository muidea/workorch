package orchestrator

import (
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Frozen pre-closure conversion helpers used only by independent test oracles.
func cloneRuntimeDeliveryAcknowledgements(values []runtimecontract.RuntimeDeliveryAcknowledgement) []runtimecontract.RuntimeDeliveryAcknowledgement {
	if values == nil {
		return nil
	}
	ret := make([]runtimecontract.RuntimeDeliveryAcknowledgement, len(values))
	for index := range values {
		ret[index] = values[index].CloneRaw()
	}
	return ret
}

func cloneRuntimeRecoverableWaits(values []runtimecontract.RuntimeRecoverableWaitRecord) []runtimecontract.RuntimeRecoverableWaitRecord {
	if values == nil {
		return nil
	}
	ret := make([]runtimecontract.RuntimeRecoverableWaitRecord, len(values))
	for idx := range values {
		ret[idx] = values[idx].Clone()
	}
	return ret
}

func cloneRuntimeOperationRecoveryAuthorizations(values []runtimecontract.RuntimeOperationRecoveryAuthorization) []runtimecontract.RuntimeOperationRecoveryAuthorization {
	if values == nil {
		return nil
	}
	ret := make([]runtimecontract.RuntimeOperationRecoveryAuthorization, len(values))
	for idx := range values {
		ret[idx] = values[idx].Clone()
	}
	return ret
}

func cloneRuntimeRepairSurfaceConfirmations(values []runtimecontract.RuntimeRepairSurfaceConfirmationRecord) []runtimecontract.RuntimeRepairSurfaceConfirmationRecord {
	if len(values) == 0 {
		return nil
	}
	ret := make([]runtimecontract.RuntimeRepairSurfaceConfirmationRecord, len(values))
	for idx, value := range values {
		ret[idx] = value.Clone()
	}
	return ret
}

func cloneRuntimeOperationGovernanceBindings(values []runtimecontract.RuntimeOperationGovernanceBinding) []runtimecontract.RuntimeOperationGovernanceBinding {
	if len(values) == 0 {
		return nil
	}
	ret := make([]runtimecontract.RuntimeOperationGovernanceBinding, len(values))
	for idx := range values {
		ret[idx] = values[idx].Clone()
	}
	return ret
}

func cloneRuntimeImplementationWorkCommitmentRecord(round *runtimecontract.RuntimeImplementationWorkRound) *runtimecontract.RuntimeImplementationWorkRound {
	if round == nil {
		return nil
	}
	copy := round.Clone()
	return &copy
}

// cloneRuntimeCandidateRequestSessions keeps the Run-owner record conversion
// ownership-neutral. Request-session state is advanced only by runtime's
// controller; this adapter merely preserves its persisted value across a
// file-backed reload and must never share fragment metadata backing arrays.
func cloneRuntimeCandidateRequestSessions(values []runtimecontract.RuntimeCandidateRequestSession) []runtimecontract.RuntimeCandidateRequestSession {
	if len(values) == 0 {
		return nil
	}
	ret := make([]runtimecontract.RuntimeCandidateRequestSession, len(values))
	for idx := range values {
		ret[idx] = values[idx].Clone()
	}
	return ret
}

func runtimeDispatchRecordFromAggregate(dispatch runtimecontract.RuntimeDispatchRecord) runtimecontract.RuntimeDispatchRecord {
	return runtimecontract.RuntimeDispatchRecord{
		ID: dispatch.ID, WorkspaceID: dispatch.WorkspaceID, RunID: dispatch.RunID,
		Transaction: dispatch.Transaction, CandidateHash: dispatch.CandidateHash, Candidate: dispatch.Candidate.CloneRaw(),
		CandidateManifestHash:              dispatch.CandidateManifestHash,
		ImplementationWorkCommitment:       cloneRuntimeImplementationWorkCommitmentRecord(dispatch.ImplementationWorkCommitment),
		CoverageBindingHash:                dispatch.CoverageBindingHash,
		CoverageBindings:                   runtimecontract.CloneCoverageEvidenceBindings(dispatch.CoverageBindings),
		OperationTargetAuthorizations:      runtimecontract.CloneRuntimeDispatchOperationTargetAuthorizations(dispatch.OperationTargetAuthorizations),
		FocusTargetRole:                    dispatch.FocusTargetRole,
		ValidationRecipes:                  runtimecontract.CloneRuntimeValidationRecipes(dispatch.ValidationRecipes),
		RepairCommitmentHash:               dispatch.RepairCommitmentHash,
		RepairCommitment:                   cloneRuntimeDispatchRepairCommitmentRecord(dispatch.RepairCommitment),
		AssessmentAuthorityInputHash:       dispatch.AssessmentAuthorityInputHash,
		AssessmentAuthorityInput:           cloneRuntimeAssessmentAuthorityInputRecord(dispatch.AssessmentAuthorityInput),
		AssessmentAdjudicationHash:         dispatch.AssessmentAdjudicationHash,
		AssessmentAdjudication:             cloneRuntimeAssessmentAdjudicationRecord(dispatch.AssessmentAdjudication),
		OperationAdmissibilities:           runtimecontract.CloneRuntimeOperationAdmissibilities(dispatch.OperationAdmissibilities),
		OperationAdmissibilityObservations: runtimecontract.CloneRuntimePrerequisiteObservations(dispatch.OperationAdmissibilityObservations),
		OperationPolicies:                  runtimecontract.NormalizeRuntimeOperationPolicyFacts(dispatch.OperationPolicies),
		State:                              runtimecontract.RuntimeDispatchState(dispatch.State), CompletedOperationIDs: append([]string(nil), dispatch.CompletedOperationIDs...),
		OperationExecutions: runtimecontract.CloneRuntimeOperationExecutionRecords(dispatch.OperationExecutions), OperationAuthorityHash: dispatch.OperationAuthorityHash,
		DispatchAuthorityHash: dispatch.DispatchAuthorityHash,
	}
}

func runtimeDispatchFromRecord(dispatch runtimecontract.RuntimeDispatchRecord) runtimecontract.RuntimeDispatchRecord {
	return runtimecontract.RuntimeDispatchRecord{
		ID: dispatch.ID, WorkspaceID: dispatch.WorkspaceID, RunID: dispatch.RunID,
		Transaction: dispatch.Transaction, CandidateHash: dispatch.CandidateHash, Candidate: dispatch.Candidate.CloneRaw(),
		CandidateManifestHash:              dispatch.CandidateManifestHash,
		ImplementationWorkCommitment:       cloneRuntimeImplementationWorkCommitmentRecord(dispatch.ImplementationWorkCommitment),
		CoverageBindingHash:                dispatch.CoverageBindingHash,
		CoverageBindings:                   runtimecontract.CloneCoverageEvidenceBindings(dispatch.CoverageBindings),
		OperationTargetAuthorizations:      runtimecontract.CloneRuntimeDispatchOperationTargetAuthorizations(dispatch.OperationTargetAuthorizations),
		FocusTargetRole:                    dispatch.FocusTargetRole,
		ValidationRecipes:                  runtimecontract.CloneRuntimeValidationRecipes(dispatch.ValidationRecipes),
		RepairCommitmentHash:               dispatch.RepairCommitmentHash,
		RepairCommitment:                   cloneRuntimeDispatchRepairCommitmentRecord(dispatch.RepairCommitment),
		AssessmentAuthorityInputHash:       dispatch.AssessmentAuthorityInputHash,
		AssessmentAuthorityInput:           cloneRuntimeAssessmentAuthorityInputRecord(dispatch.AssessmentAuthorityInput),
		AssessmentAdjudicationHash:         dispatch.AssessmentAdjudicationHash,
		AssessmentAdjudication:             cloneRuntimeAssessmentAdjudicationRecord(dispatch.AssessmentAdjudication),
		OperationAdmissibilities:           runtimecontract.CloneRuntimeOperationAdmissibilities(dispatch.OperationAdmissibilities),
		OperationAdmissibilityObservations: runtimecontract.CloneRuntimePrerequisiteObservations(dispatch.OperationAdmissibilityObservations),
		OperationPolicies:                  runtimecontract.NormalizeRuntimeOperationPolicyFacts(dispatch.OperationPolicies),
		State:                              runtimecontract.RuntimeDispatchState(dispatch.State), CompletedOperationIDs: append([]string(nil), dispatch.CompletedOperationIDs...),
		OperationExecutions: runtimecontract.CloneRuntimeOperationExecutionRecords(dispatch.OperationExecutions), OperationAuthorityHash: dispatch.OperationAuthorityHash,
		DispatchAuthorityHash: dispatch.DispatchAuthorityHash,
	}
}

func cloneRuntimeDispatchRepairCommitmentRecord(value *runtimecontract.RuntimeDispatchRepairCommitment) *runtimecontract.RuntimeDispatchRepairCommitment {
	if value == nil {
		return nil
	}
	copy := value.Clone()
	return &copy
}

func cloneRuntimeAssessmentAuthorityInputRecord(value *runtimecontract.RuntimeAssessmentAuthorityInput) *runtimecontract.RuntimeAssessmentAuthorityInput {
	if value == nil {
		return nil
	}
	copy := value.Clone()
	return &copy
}

func cloneRuntimeAssessmentAdjudicationRecord(value *runtimecontract.RuntimeAssessmentAdjudication) *runtimecontract.RuntimeAssessmentAdjudication {
	if value == nil {
		return nil
	}
	copy := value.Clone()
	return &copy
}

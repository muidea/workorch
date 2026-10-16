package orchestrator

// Frozen pre-reuse conversion oracle; no validation gate is delegated to it.
import (
	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func referenceRuntimeStateRecordFromAggregate(aggregate runtime.RuntimeAggregate) runtimecontract.RuntimeStateRecord {
	ret := runtimecontract.RuntimeStateRecord{
		ResultFeedbackVersion:           aggregate.ResultFeedbackVersion,
		StateRevision:                   aggregate.StateRevision,
		EvidenceSequence:                aggregate.EvidenceSequence,
		Definition:                      aggregate.Definition,
		Facts:                           aggregate.Facts,
		SemanticPlanning:                aggregate.SemanticPlanning.Clone(),
		ImplementationWork:              aggregate.ImplementationWork.Clone(),
		Reconciliation:                  aggregate.Reconciliation.Clone(),
		CoverageProofLineage:            runtimecontract.NormalizeRuntimeCoverageProofLineage(aggregate.CoverageProofLineage),
		Decision:                        aggregate.Decision,
		ProcessedEventIDs:               append([]string(nil), aggregate.ProcessedEventIDs...),
		RepairSurfaceConfirmations:      cloneRuntimeRepairSurfaceConfirmations(aggregate.RepairSurfaceConfirmations),
		CandidateRequestSessions:        cloneRuntimeCandidateRequestSessions(aggregate.CandidateRequestSessions),
		CandidateAttempts:               append([]runtimecontract.RuntimeCandidateAttemptRecord(nil), aggregate.CandidateAttempts...),
		ExecutionBudget:                 aggregate.ExecutionBudget.Clone(),
		RecoverableWaits:                cloneRuntimeRecoverableWaits(aggregate.RecoverableWaits),
		OperationRecoveryAuthorizations: cloneRuntimeOperationRecoveryAuthorizations(aggregate.OperationRecoveryAuthorizations),
		GovernanceRequests:              append([]runtimecontract.RuntimeGovernanceRequestRecord(nil), aggregate.GovernanceRequests...),
		GovernanceResolutions:           runtimecontract.CloneRuntimeGovernanceResolutionRecords(aggregate.GovernanceResolutions),
		OperationGovernanceBindings:     cloneRuntimeOperationGovernanceBindings(aggregate.OperationGovernanceBindings),
		OperationReceipts:               runtimecontract.NormalizeRuntimeOperationReceipts(aggregate.OperationReceipts),
		DeliveryAcknowledgements:        cloneRuntimeDeliveryAcknowledgements(aggregate.DeliveryAcknowledgements),
		PrerequisiteInvalidations:       runtimecontract.NormalizeRuntimePrerequisiteInvalidations(aggregate.PrerequisiteInvalidations),
	}
	if aggregate.TerminalMaterialSeal != nil {
		value := aggregate.TerminalMaterialSeal.CloneRaw()
		ret.TerminalMaterialSeal = &value
	}
	if aggregate.DefinitionClarification != nil {
		value := aggregate.DefinitionClarification.Clone()
		ret.DefinitionClarification = &value
	}
	if aggregate.DriveLease != nil {
		lease := aggregate.DriveLease.Clone()
		ret.DriveLease = &lease
	}
	if aggregate.Termination != nil {
		term := aggregate.Termination.Clone()
		ret.Termination = &term
	}
	if aggregate.ActiveDispatch != nil {
		value := referenceRuntimeDispatchRecordFromAggregate(*aggregate.ActiveDispatch)
		ret.ActiveDispatch = &value
	}
	ret.CompletedDispatches = make([]runtimecontract.RuntimeDispatchRecord, len(aggregate.CompletedDispatches))
	for idx := range aggregate.CompletedDispatches {
		ret.CompletedDispatches[idx] = referenceRuntimeDispatchRecordFromAggregate(aggregate.CompletedDispatches[idx])
	}
	ret.Diagnostics = append([]runtimecontract.RuntimeDiagnosticRecord(nil), aggregate.Diagnostics...)
	return ret.Clone()
}

func referenceRuntimeAggregateFromRecord(record runtimecontract.RuntimeStateRecord) runtime.RuntimeAggregate {
	ret := runtime.RuntimeAggregate{
		ResultFeedbackVersion:           record.ResultFeedbackVersion,
		StateRevision:                   record.StateRevision,
		EvidenceSequence:                record.EvidenceSequence,
		Definition:                      record.Definition,
		Facts:                           record.Facts,
		SemanticPlanning:                record.SemanticPlanning.Clone(),
		ImplementationWork:              record.ImplementationWork.Clone(),
		Reconciliation:                  record.Reconciliation.Clone(),
		CoverageProofLineage:            runtimecontract.NormalizeRuntimeCoverageProofLineage(record.CoverageProofLineage),
		Decision:                        record.Decision,
		ProcessedEventIDs:               append([]string(nil), record.ProcessedEventIDs...),
		RepairSurfaceConfirmations:      cloneRuntimeRepairSurfaceConfirmations(record.RepairSurfaceConfirmations),
		CandidateRequestSessions:        cloneRuntimeCandidateRequestSessions(record.CandidateRequestSessions),
		CandidateAttempts:               append([]runtimecontract.RuntimeCandidateAttemptRecord(nil), record.CandidateAttempts...),
		ExecutionBudget:                 record.ExecutionBudget.Clone(),
		RecoverableWaits:                cloneRuntimeRecoverableWaits(record.RecoverableWaits),
		OperationRecoveryAuthorizations: cloneRuntimeOperationRecoveryAuthorizations(record.OperationRecoveryAuthorizations),
		GovernanceRequests:              append([]runtimecontract.RuntimeGovernanceRequestRecord(nil), record.GovernanceRequests...),
		GovernanceResolutions:           runtimecontract.CloneRuntimeGovernanceResolutionRecords(record.GovernanceResolutions),
		OperationGovernanceBindings:     cloneRuntimeOperationGovernanceBindings(record.OperationGovernanceBindings),
		OperationReceipts:               runtimecontract.NormalizeRuntimeOperationReceipts(record.OperationReceipts),
		DeliveryAcknowledgements:        cloneRuntimeDeliveryAcknowledgements(record.DeliveryAcknowledgements),
		PrerequisiteInvalidations:       runtimecontract.NormalizeRuntimePrerequisiteInvalidations(record.PrerequisiteInvalidations),
	}
	if record.TerminalMaterialSeal != nil {
		value := record.TerminalMaterialSeal.CloneRaw()
		ret.TerminalMaterialSeal = &value
	}
	if record.DefinitionClarification != nil {
		value := record.DefinitionClarification.Clone()
		ret.DefinitionClarification = &value
	}
	if record.DriveLease != nil {
		lease := record.DriveLease.Clone()
		ret.DriveLease = &lease
	}
	if record.Termination != nil {
		term := record.Termination.Clone()
		ret.Termination = &term
	}
	if record.ActiveDispatch != nil {
		value := referenceRuntimeDispatchFromRecord(*record.ActiveDispatch)
		ret.ActiveDispatch = &value
	}
	ret.CompletedDispatches = make([]runtime.SealedDispatch, len(record.CompletedDispatches))
	for idx := range record.CompletedDispatches {
		ret.CompletedDispatches[idx] = referenceRuntimeDispatchFromRecord(record.CompletedDispatches[idx])
	}
	ret.Diagnostics = append([]runtimecontract.RuntimeDiagnosticRecord(nil), record.Diagnostics...)
	return ret
}

func referenceRuntimeDispatchRecordFromAggregate(dispatch runtime.SealedDispatch) runtimecontract.RuntimeDispatchRecord {
	return runtimecontract.RuntimeDispatchRecord{
		ID: dispatch.ID, WorkspaceID: dispatch.WorkspaceID, RunID: dispatch.RunID,
		Transaction: dispatch.Transaction, CandidateHash: dispatch.CandidateHash, Candidate: dispatch.Candidate,
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

func referenceRuntimeDispatchFromRecord(dispatch runtimecontract.RuntimeDispatchRecord) runtime.SealedDispatch {
	return runtime.SealedDispatch{
		ID: dispatch.ID, WorkspaceID: dispatch.WorkspaceID, RunID: dispatch.RunID,
		Transaction: dispatch.Transaction, CandidateHash: dispatch.CandidateHash, Candidate: dispatch.Candidate,
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
		State:                              runtime.DispatchState(dispatch.State), CompletedOperationIDs: append([]string(nil), dispatch.CompletedOperationIDs...),
		OperationExecutions: runtimecontract.CloneRuntimeOperationExecutionRecords(dispatch.OperationExecutions), OperationAuthorityHash: dispatch.OperationAuthorityHash,
		DispatchAuthorityHash: dispatch.DispatchAuthorityHash,
	}
}

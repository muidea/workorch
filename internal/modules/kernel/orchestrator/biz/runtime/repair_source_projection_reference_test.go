package runtime

import (
	"fmt"
	"reflect"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Complete pre-change algorithms. Whole-Facts normalization remains here as
// the independent oracle for projection, ordering and first-error behavior.
func repairProjectionReferenceValidateConfirmedRepairMandatoryCommitments(aggregate RuntimeAggregate) error {
	dispatches := make([]runtimecontract.RuntimeDispatchRecord, 0, len(aggregate.CompletedDispatches)+1)
	dispatches = append(dispatches, aggregate.CompletedDispatches...)
	if aggregate.ActiveDispatch != nil {
		dispatches = append(dispatches, *aggregate.ActiveDispatch)
	}
	for _, dispatch := range dispatches {
		required, err := repairProjectionReferenceConfirmedRepairCommitmentRequiredByRejectedObservation(aggregate, dispatch)
		if err != nil {
			return err
		}
		if required && dispatch.RepairCommitment == nil {
			return fmt.Errorf("runtime aggregate confirmed owner repair dispatch %q omitted its mandatory rejected-validation commitment", dispatch.ID)
		}
	}
	return nil
}

func repairProjectionReferenceConfirmedRepairCommitmentRequiredByRejectedObservation(aggregate RuntimeAggregate, dispatch runtimecontract.RuntimeDispatchRecord) (bool, error) {
	if dispatch.Candidate.StateChange == nil || dispatch.Transaction.State != runtimecontract.ConvergenceStateNeedStateChange {
		return false, nil
	}
	targetRefs, repairOwnerModuleID, err := confirmedRepairDispatchTargets(dispatch)
	if err != nil || repairOwnerModuleID == "" {
		return false, err
	}
	operations := candidateRuntimeOperations(dispatch.Candidate)
	matches := 0
	facts := aggregate.Facts.Normalized()
	for _, fact := range facts.Evidence.Rejected {
		if fact.Kind != runtimecontract.EvidenceKindValidation || fact.FocusID != dispatch.Transaction.FocusID ||
			fact.Revision != dispatch.Transaction.EvidenceRevision || fact.DispatchID == "" || fact.OperationID == "" {
			continue
		}
		var sourceDispatch *runtimecontract.RuntimeDispatchRecord
		for index := range aggregate.CompletedDispatches {
			candidate := &aggregate.CompletedDispatches[index]
			if candidate.ID != fact.DispatchID || candidate.State != DispatchStateCommitted ||
				!containsString(candidate.CompletedOperationIDs, fact.OperationID) {
				continue
			}
			if sourceDispatch != nil {
				return false, fmt.Errorf("%w: confirmed owner repair mandatory source dispatch is duplicated", common.ErrConflict)
			}
			sourceDispatch = candidate
		}
		if sourceDispatch == nil {
			continue
		}
		sourceOperation, found := candidateOperationByID(sourceDispatch.Candidate, fact.OperationID)
		if !found {
			continue
		}
		execution, found := operationExecutionRecord(sourceDispatch.OperationExecutions, sourceOperation.ID)
		if !found || execution.State != runtimecontract.OperationExecutionEvidenceCommitted || execution.Fence == 0 {
			continue
		}
		receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(
			aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence,
		)
		if !found || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected {
			continue
		}
		failure := failureFactFromOperationReceiptFeedback(receipt)
		observation := runtimeValidationFailureObservationFromFailure(failure)
		if observation == nil || observation.DispatchID != fact.DispatchID || observation.OperationID != fact.OperationID ||
			observation.AuthoritySnapshotID != fact.AuthoritySnapshotID || observation.FailureSignature != fact.FailureSignature ||
			observation.TargetRole != dispatch.FocusTargetRole || !runtimeTargetRefsWithin(targetRefs, observation.CandidateTargetRefs) {
			continue
		}
		expectedOwner, ownerErr := runtimeExpectedRepairSurfaceOwner(aggregate, *observation)
		if ownerErr != nil || expectedOwner != repairOwnerModuleID {
			continue
		}
		// Reuse the full receipt/evidence reverse proof without depending on the
		// mutable confirmation ledger: the sealed Profile/binding owner plus source
		// rejection is sufficient to prove that a commitment was mandatory. The
		// commitment remains the reverse key for the exact confirmation itself.
		sourceFact, _, exactSourceOperation, sourceErr := repairProjectionReferenceConfirmedRepairSourceRejection(
			aggregate,
			runtimecontract.RuntimeRepairSurfaceConfirmation{Observation: observation.Clone()},
		)
		if sourceErr != nil {
			return false, sourceErr
		}
		if sourceFact.ID != fact.ID || sourceFact.Revision != dispatch.Transaction.EvidenceRevision ||
			!sameRuntimeStringSet(sourceFact.BindingRefs, exactSourceOperation.BindingRefs) {
			return false, fmt.Errorf("%w: confirmed owner repair mandatory source evidence drifted", common.ErrConflict)
		}
		for _, operation := range operations {
			operation = operation.Normalized()
			if !sameRuntimeStringSet(operation.BindingRefs, exactSourceOperation.BindingRefs) ||
				!sameCoverageIDs(operation.CoverageWorkItemIDs, exactSourceOperation.CoverageWorkItemIDs) {
				return false, fmt.Errorf("%w: confirmed owner repair mandatory dispatch does not retain its source binding", common.ErrConflict)
			}
		}
		matches++
	}
	if matches > 1 {
		return false, fmt.Errorf("%w: confirmed owner repair dispatch has multiple mandatory rejected-validation sources", common.ErrConflict)
	}
	return matches == 1, nil
}

// validatePendingConfirmedRepairTransaction is the fresh-load proof for a
// partially completed single-owner all-of dispatch. It validates only immutable
// transaction lineage and explicitly rejects restoration of the superseded
// target/directive.
func repairProjectionReferenceConfirmedRepairSourceRejection(
	aggregate RuntimeAggregate,
	confirmation runtimecontract.RuntimeRepairSurfaceConfirmation,
) (runtimecontract.EvidenceFact, runtimecontract.RuntimeOperationReceipt, runtimecontract.RuntimeOperation, error) {
	confirmation = confirmation.Normalized()
	observation := confirmation.Observation.Normalized()
	var dispatch *runtimecontract.RuntimeDispatchRecord
	for idx := range aggregate.CompletedDispatches {
		candidate := &aggregate.CompletedDispatches[idx]
		if candidate.ID == observation.DispatchID && candidate.State == DispatchStateCommitted &&
			containsString(candidate.CompletedOperationIDs, observation.OperationID) {
			if dispatch != nil {
				return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair observation matches multiple source dispatches", common.ErrConflict)
			}
			dispatch = candidate
		}
	}
	if dispatch == nil || !sealedValidationFailureObservationMatchesDispatch(*dispatch, observation) {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair has no exact sealed validation source", common.ErrConflict)
	}
	operation, found := candidateOperationByID(dispatch.Candidate, observation.OperationID)
	if !found {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source operation is not sealed", common.ErrConflict)
	}
	execution, found := operationExecutionRecord(dispatch.OperationExecutions, observation.OperationID)
	if !found || execution.State != runtimecontract.OperationExecutionEvidenceCommitted || execution.Fence == 0 {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source has no evidence-committed execution", common.ErrConflict)
	}
	receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
	if !found || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || receipt.OperationID != operation.ID ||
		execution.OwnerReceiptRef != "runtime-receipt/"+receipt.ExecutionKey ||
		execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) ||
		!runtimeValidationFailureObservationHasPersistedReceipt(aggregate, observation) {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source has no exact rejected validation-owner receipt", common.ErrConflict)
	}
	expectedFailure := failureFactFromOperationReceiptFeedback(receipt)
	if expectedFailure == nil || expectedFailure.Signature != observation.FailureSignature {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source receipt failure identity mismatch", common.ErrConflict)
	}
	var fact runtimecontract.EvidenceFact
	matches := 0
	for _, candidate := range aggregate.Facts.Normalized().Evidence.Rejected {
		if candidate.DispatchID != dispatch.ID || candidate.OperationID != operation.ID {
			continue
		}
		fact, matches = candidate, matches+1
	}
	if matches != 1 {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source requires one receipt-derived rejected evidence fact", common.ErrConflict)
	}
	receiptHash := runtimecontract.StableRuntimeOperationReceiptHash(receipt)
	eventID := string(fact.ID)
	recoveryEventID := "runtime-receipt|" + receiptHash
	recoverySourceRef := "runtime-receipt/" + receipt.ExecutionKey
	if eventID != recoveryEventID || fact.SourceRef != recoverySourceRef {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source evidence has no canonical persisted receipt-recovery identity", common.ErrConflict)
	}
	processed := 0
	for _, eventID := range aggregate.ProcessedEventIDs {
		if eventID == string(fact.ID) {
			processed++
		}
	}
	receiptEvidence := receipt.Normalized().Evidence
	if processed != 1 {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source evidence has no unique processed-event fence", common.ErrConflict)
	}
	if fact.Kind != runtimecontract.EvidenceKindValidation || fact.SubjectRef != operation.ID ||
		fact.Revision == "" || !runtimecontract.EvidenceRevisionAfter(fact.Revision, dispatch.Transaction.EvidenceRevision) ||
		fact.AuthoritySnapshotID != dispatch.Transaction.AuthoritySnapshotID ||
		fact.DispatchID != dispatch.ID || fact.OperationID != operation.ID {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source evidence scope/revision does not match its sealed operation", common.ErrConflict)
	}
	if !sameRuntimeTargetRefs(fact.TargetRefs, operation.TargetRefs) ||
		!sameRuntimeStringSet(fact.BindingRefs, operation.BindingRefs) ||
		!sameCoverageIDs(fact.CoverageWorkItemIDs, operation.CoverageWorkItemIDs) {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source evidence target/binding/coverage does not match its sealed operation", common.ErrConflict)
	}
	if fact.FailureClass != expectedFailure.Class || fact.FailureCode != expectedFailure.Code ||
		fact.FailureSignature != expectedFailure.Signature || fact.FailureMessage != expectedFailure.Message {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source failure does not match its exact validation-owner receipt", common.ErrConflict)
	}
	if fact.Summary != receiptEvidence.Summary || fact.ContentHash != receiptEvidence.ContentHash ||
		!reflect.DeepEqual(fact.OwnerRevisionSet, receiptEvidence.OwnerRevisionSet) ||
		!reflect.DeepEqual(fact.ApplicabilityDependencies, receiptEvidence.ApplicabilityDependencies) ||
		!reflect.DeepEqual(fact.ContentAddress, receiptEvidence.ContentAddress) ||
		!reflect.DeepEqual(fact.ExitCode, receiptEvidence.ExitCode) || !sameRuntimeTargetRefs(fact.EvidenceRefs, receiptEvidence.EvidenceRefs) ||
		fact.Quality != receiptEvidence.Quality || fact.ZeroWork != receiptEvidence.ZeroWork ||
		!sameRuntimeTargetRefs(fact.EvidenceTags, receiptEvidence.EvidenceTags) ||
		!reflect.DeepEqual(fact.Validation, receiptEvidence.Validation) {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source evidence does not match its exact validation-owner receipt", common.ErrConflict)
	}
	if fact.EvidenceReason != receiptEvidence.EvidenceReason {
		return runtimecontract.EvidenceFact{}, runtimecontract.RuntimeOperationReceipt{}, runtimecontract.RuntimeOperation{}, fmt.Errorf("%w: confirmed owner repair source evidence reason does not match its exact validation-owner receipt", common.ErrConflict)
	}
	return fact, receipt, operation, nil
}

func repairProjectionReferenceConfirmedRepairGovernanceEvidenceRevision(aggregate RuntimeAggregate, binding runtimecontract.RuntimeOperationGovernanceBinding) (string, error) {
	binding = binding.Normalized()
	if binding.Grant == nil {
		return "", fmt.Errorf("%w: confirmed owner repair governance hop has no grant", common.ErrConflict)
	}
	grant := binding.Grant.Normalized()
	wantID := common.EvidenceRecordID("governance-resolution:" + string(grant.ResolutionID) + ":" + binding.GovernanceSnapshotID)
	wantSource := "governance-resolution/" + string(grant.ResolutionID)
	var matched runtimecontract.EvidenceFact
	matches := 0
	for _, fact := range aggregate.Facts.Normalized().Evidence.Accepted {
		if fact.ID != wantID || fact.Kind != runtimecontract.EvidenceKindGovernance || fact.SourceRef != wantSource ||
			fact.DispatchID != binding.GovernanceDispatchID || fact.AuthoritySnapshotID != binding.GovernanceSnapshotID ||
			fact.ContentHash != grant.ResolutionHash || !sameRuntimeTargetRefs(fact.EvidenceRefs, grant.EvidenceRefs) {
			continue
		}
		matched, matches = fact, matches+1
	}
	processed := 0
	for _, eventID := range aggregate.ProcessedEventIDs {
		if eventID == string(wantID) {
			processed++
		}
	}
	if matches != 1 || processed != 1 || matched.Revision == "" {
		return "", fmt.Errorf("%w: confirmed owner repair governance hop has no unique accepted resolution evidence", common.ErrConflict)
	}
	return matched.Revision, nil
}

// completedConfirmedRepairTransactions reconstructs the immutable source
// and effect chain for committed confirmed-owner repairs. It deliberately does
// not consult RepairDirective or FailureTarget: those mutable projections are
// what this reverse proof is responsible for validating.

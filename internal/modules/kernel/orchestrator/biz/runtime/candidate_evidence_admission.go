package runtime

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type executionEvidenceAdmission struct {
	Evidence      ExecutionEvidence
	Operation     runtimecontract.RuntimeOperation
	ExpectedScope runtimecontract.RuntimeEffectScope
}

type executionEvidenceCreditRole uint8

const (
	executionEvidenceCreditRoleProof executionEvidenceCreditRole = iota
	executionEvidenceCreditRoleConfirmedRepairCausation
)

type executionEvidenceCreditFinding struct {
	BindingRefs         []string
	CoverageWorkItemIDs []common.CoverageWorkItemID
	Diagnostic          string
	Role                executionEvidenceCreditRole
}

func admitExecutionEvidenceIdentity(aggregate RuntimeAggregate, evidence ExecutionEvidence) (executionEvidenceAdmission, error) {
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.ID != evidence.DispatchID || aggregate.ActiveDispatch.Transaction.AuthoritySnapshotID != evidence.AuthoritySnapshotID {
		return executionEvidenceAdmission{}, fmt.Errorf("execution evidence does not match the active sealed dispatch")
	}
	if !candidateContainsOperation(aggregate.ActiveDispatch.Candidate, evidence.OperationID) {
		return executionEvidenceAdmission{}, fmt.Errorf("execution evidence operation %q is not present in the sealed candidate", evidence.OperationID)
	}
	if err := validateRuntimeExecutionPurposeFence(aggregate); err != nil {
		return executionEvidenceAdmission{}, fmt.Errorf("%w: execution evidence binding fence: %v", common.ErrConflict, err)
	}

	boundEvidence, err := bindPersistedOwnerReceipt(aggregate, evidence)
	if err != nil {
		return executionEvidenceAdmission{}, err
	}
	operation, found := candidateOperationForEvidence(aggregate.ActiveDispatch.Candidate, evidence.OperationID)
	if !found {
		return executionEvidenceAdmission{}, fmt.Errorf("execution evidence operation %q is not present in the sealed candidate", evidence.OperationID)
	}
	if boundEvidence.Receipt != nil {
		if err := validateOperationReceiptFeedback(aggregate, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID, aggregate.ActiveDispatch.ID, *boundEvidence.Receipt); err != nil {
			return executionEvidenceAdmission{}, fmt.Errorf("execution evidence owner receipt: %w", err)
		}
	}
	descriptor, descriptorFound := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !descriptorFound {
		return executionEvidenceAdmission{}, fmt.Errorf("execution evidence operation %q has no descriptor", operation.Kind)
	}
	if descriptor.RequiresOwnerReceipt && boundEvidence.Receipt == nil {
		return executionEvidenceAdmission{}, fmt.Errorf("execution evidence operation %q requires persisted owner receipt", operation.Kind)
	}
	expectedScope, hasExecution := expectedEffectScopeForOperation(*aggregate.ActiveDispatch, evidence.OperationID, aggregate.Facts.Activation)
	if !hasExecution {
		return executionEvidenceAdmission{}, fmt.Errorf("execution evidence operation %q has no sealed execution record", evidence.OperationID)
	}
	return executionEvidenceAdmission{Evidence: boundEvidence, Operation: operation, ExpectedScope: expectedScope}, nil
}

func bindPersistedOwnerReceipt(aggregate RuntimeAggregate, evidence ExecutionEvidence) (ExecutionEvidence, error) {
	if evidence.Receipt == nil {
		return evidence, nil
	}
	persisted, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, evidence.Receipt.ExecutionKey, evidence.Receipt.Fence)
	if !found || !runtimecontract.SameRuntimeOperationReceipt(persisted, *evidence.Receipt) {
		return ExecutionEvidence{}, fmt.Errorf("execution evidence owner receipt is not persisted")
	}
	execution, found := operationExecutionRecord(aggregate.ActiveDispatch.OperationExecutions, evidence.OperationID)
	if !found || execution.State != runtimecontract.OperationExecutionOwnerCommitted || execution.ExecutionKey != persisted.ExecutionKey ||
		execution.Fence != persisted.Fence || execution.OwnerReceiptRef != "runtime-receipt/"+persisted.ExecutionKey ||
		execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(persisted) {
		return ExecutionEvidence{}, fmt.Errorf("execution evidence requires owner_committed execution state")
	}
	// The persisted owner receipt is the sole evidence-fact source. The
	// surrounding internal event carries routing/status information only.
	evidence.Receipt = &persisted
	evidence = canonicalPersistedOwnerReceiptEvidenceIdentity(aggregate, evidence)
	evidence.Evidence = persisted.Evidence
	evidence.ObservedEffectScope = persisted.ObservedEffectScope
	evidence.NoOp = persisted.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp
	if persisted.Outcome == runtimecontract.RuntimeOperationOutcomeRejected {
		if evidence.Status != ExecutionEvidenceStatusRejected {
			return ExecutionEvidence{}, fmt.Errorf("rejected owner receipt cannot produce accepted execution evidence")
		}
		failure := failureFactFromOperationReceiptFeedback(persisted)
		if failure == nil {
			return ExecutionEvidence{}, fmt.Errorf("rejected owner receipt has no typed owner failure")
		}
		// The owner receipt is the sole diagnostic authority just as it is the
		// sole effect/evidence authority above. The internal event's Failure copy
		// is routing validation only and cannot erase typed OriginalDiagnostic or
		// substitute an ungrounded target.
		evidence.Failure = failure
	}
	return evidence, nil
}

// canonicalPersistedOwnerReceiptEvidenceIdentity gives every ordinary owner
// receipt one durable evidence identity, independent of whether Runtime saw
// the live response or recovered an already-persisted receipt after restart.
// Publishers may use a live operation event for transport correlation, but
// the persisted SM-3 fact is always keyed by the owner receipt hash/fence.
func canonicalPersistedOwnerReceiptEvidenceIdentity(aggregate RuntimeAggregate, evidence ExecutionEvidence) ExecutionEvidence {
	if evidence.Receipt == nil {
		return evidence
	}
	persisted, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, evidence.Receipt.ExecutionKey, evidence.Receipt.Fence)
	if !found || !runtimecontract.SameRuntimeOperationReceipt(persisted, *evidence.Receipt) {
		return evidence
	}
	hash := runtimecontract.StableRuntimeOperationReceiptHash(persisted)
	evidence.EventID = "runtime-receipt|" + hash
	evidence.SourceRef = "runtime-receipt/" + persisted.ExecutionKey
	evidence.OwnerReceiptRef = evidence.SourceRef
	evidence.OwnerReceiptHash = hash
	evidence.Receipt = &persisted
	return evidence
}

func candidateOperationForEvidence(candidate runtimecontract.RuntimeCandidate, operationID string) (runtimecontract.RuntimeOperation, bool) {
	for _, operation := range candidateRuntimeOperations(candidate) {
		operation = operation.Normalized()
		if operation.ID == operationID {
			return operation, true
		}
	}
	return runtimecontract.RuntimeOperation{}, false
}

func deriveExecutionEvidenceFact(aggregate RuntimeAggregate, admission executionEvidenceAdmission) (runtimecontract.EvidenceFact, executionEvidenceCreditFinding, error) {
	evidenceKind, err := adjudicateEvidenceKind(aggregate.Decision, admission.ExpectedScope, true, admission.Evidence)
	if err != nil {
		return runtimecontract.EvidenceFact{}, executionEvidenceCreditFinding{}, err
	}
	bindings := runtimecontract.CloneCoverageEvidenceBindings(aggregate.Facts.Focus.CoverageEvidenceBindings)
	if aggregate.ActiveDispatch != nil {
		bindings = runtimecontract.CloneCoverageEvidenceBindings(aggregate.ActiveDispatch.CoverageBindings)
	}
	finding := findExecutionEvidenceCredit(bindings, admission.Operation, evidenceKind, admission.Evidence.Receipt)
	evidence := admission.Evidence
	if evidence.Status == ExecutionEvidenceStatusAccepted && aggregate.ActiveDispatch != nil &&
		confirmedRepairCausationOnlyBinding(*aggregate.ActiveDispatch, bindings, admission.Operation, evidenceKind) {
		// A confirmed validation repair deliberately reuses the rejected
		// validation binding as immutable causation. When that binding has no
		// state-change proof, the absence is not a receipt mismatch and must not
		// be converted into terminal CWI credit or a reducer diagnostic.
		finding.CoverageWorkItemIDs = nil
		finding.Diagnostic = ""
		finding.Role = executionEvidenceCreditRoleConfirmedRepairCausation
	}
	if evidence.Status == ExecutionEvidenceStatusRejected {
		// A rejected owner receipt can retain the sealed binding lineage for
		// reverse proof, but it never receives reducer-derived CWI credit.
		finding.BindingRefs = append([]string(nil), admission.Operation.BindingRefs...)
		finding.CoverageWorkItemIDs = nil
	}
	fact := runtimecontract.EvidenceFact{
		ID: common.EvidenceRecordID(evidence.EventID), Kind: evidenceKind, SubjectRef: strings.TrimSpace(evidence.OperationID),
		FocusID:   aggregate.ActiveDispatch.Transaction.FocusID,
		SourceRef: strings.TrimSpace(evidence.SourceRef), AuthoritySnapshotID: evidence.AuthoritySnapshotID,
		DispatchID: evidence.DispatchID, OperationID: evidence.OperationID, Revision: nextEvidenceRevision(aggregate.EvidenceSequence + 1),
		Summary: evidence.Evidence.Summary, ContentHash: evidence.Evidence.ContentHash,
		ContentAddress:            cloneRuntimeEvidenceContentAddress(evidence.Evidence.ContentAddress),
		OwnerRevisionSet:          runtimecontract.CloneRuntimeOwnerRevisionSet(evidence.Evidence.OwnerRevisionSet),
		ApplicabilityDependencies: runtimecontract.CloneRuntimeEvidenceApplicabilityDependencies(evidence.Evidence.ApplicabilityDependencies),
		ExitCode:                  evidence.Evidence.ExitCode, EvidenceRefs: append([]string(nil), evidence.Evidence.EvidenceRefs...),
		TargetRefs:          append([]string(nil), admission.Operation.TargetRefs...),
		BindingRefs:         finding.BindingRefs,
		CoverageWorkItemIDs: finding.CoverageWorkItemIDs,
		Quality:             evidence.Evidence.Quality, ZeroWork: evidence.Evidence.ZeroWork,
		EvidenceReason: evidence.Evidence.EvidenceReason, EvidenceTags: append([]string(nil), evidence.Evidence.EvidenceTags...),
	}
	// EvidenceFact is an exact owner-receipt projection. Reducer diagnostics
	// belong to FailureFact/OriginalDiagnostic and must never be appended to an
	// owner-authored reason: doing so breaks receipt reverse proof and makes
	// recovery depend on mutable credit adjudication.
	if evidence.Evidence.Validation != nil {
		observation := evidence.Evidence.Validation.Clone()
		fact.Validation = &observation
	}
	return fact, finding, nil
}

func cloneRuntimeEvidenceContentAddress(value *runtimecontract.RuntimeEvidenceContentAddress) *runtimecontract.RuntimeEvidenceContentAddress {
	if value == nil {
		return nil
	}
	cloned := value.Clone()
	return &cloned
}

func findExecutionEvidenceCredit(bindings []runtimecontract.CoverageEvidenceBinding, operation runtimecontract.RuntimeOperation, evidenceKind string, receipt *runtimecontract.RuntimeOperationReceipt) executionEvidenceCreditFinding {
	bindingRefs, coverageIDs, diagnostic := coverageEvidenceCreditForReceipt(bindings, operation, evidenceKind, receipt)
	return executionEvidenceCreditFinding{
		BindingRefs:         bindingRefs,
		CoverageWorkItemIDs: coverageIDs,
		Diagnostic:          diagnostic,
	}
}

// confirmedRepairCausationOnlyBinding recognizes the one legitimate case in
// which a sealed operation retains a binding that does not authorize its
// evidence kind. The immutable repair commitment requires that source binding
// for validation-failure causation, while fresh validation remains the only
// terminal proof. All other no-credit findings keep the ordinary mismatch
// semantics.
func confirmedRepairCausationOnlyBinding(
	dispatch SealedDispatch,
	bindings []runtimecontract.CoverageEvidenceBinding,
	operation runtimecontract.RuntimeOperation,
	evidenceKind string,
) bool {
	if dispatch.RepairCommitment == nil ||
		dispatch.RepairCommitment.Normalized().Purpose != runtimecontract.RuntimeDispatchRepairPurposeConfirmedValidation ||
		strings.TrimSpace(evidenceKind) != runtimecontract.EvidenceKindStateChange {
		return false
	}
	operation = operation.Normalized()
	if len(operation.BindingRefs) != 1 {
		return false
	}
	binding, found := runtimecontract.CoverageEvidenceBindingForRef(bindings, operation.BindingRefs[0])
	if !found || !coverageWorkItemIDIncluded(dispatch.RepairCommitment.Closure.CoverageWorkItemIDs, binding.CoverageWorkItemID) {
		return false
	}
	_, grantsOperationKindCredit := runtimecontract.CoverageEvidenceProofForKind(binding, evidenceKind)
	return !grantsOperationKindCredit
}

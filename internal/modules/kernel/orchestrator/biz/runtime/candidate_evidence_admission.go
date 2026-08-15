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

type executionEvidenceCreditFinding struct {
	BindingRefs         []string
	CoverageWorkItemIDs []common.CoverageWorkItemID
	Diagnostic          string
}

func admitExecutionEvidenceIdentity(aggregate RuntimeAggregate, evidence ExecutionEvidence) (executionEvidenceAdmission, error) {
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.ID != evidence.DispatchID || aggregate.ActiveDispatch.Transaction.AuthoritySnapshotID != evidence.AuthoritySnapshotID {
		return executionEvidenceAdmission{}, fmt.Errorf("execution evidence does not match the active sealed dispatch")
	}
	if !candidateContainsOperation(aggregate.ActiveDispatch.Candidate, evidence.OperationID) {
		return executionEvidenceAdmission{}, fmt.Errorf("execution evidence operation %q is not present in the sealed candidate", evidence.OperationID)
	}
	if err := validateCurrentCWIBindingFenceForEvidenceCommit(aggregate.Facts); err != nil {
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
	if !found || execution.State != runtimecontract.OperationExecutionOwnerCommitted || execution.ExecutionKey != persisted.ExecutionKey {
		return ExecutionEvidence{}, fmt.Errorf("execution evidence requires owner_committed execution state")
	}
	// The persisted owner receipt is the sole evidence-fact source. The
	// surrounding internal event carries routing/status information only.
	evidence.Receipt = &persisted
	evidence.Evidence = persisted.Evidence
	evidence.ObservedEffectScope = persisted.ObservedEffectScope
	evidence.NoOp = persisted.Outcome == runtimecontract.RuntimeOperationOutcomeNoOp
	return evidence, nil
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
	finding := findExecutionEvidenceCredit(aggregate.Facts, admission.Operation, evidenceKind, admission.Evidence.Receipt)
	evidence := admission.Evidence
	fact := runtimecontract.EvidenceFact{
		ID: common.EvidenceRecordID(evidence.EventID), Kind: evidenceKind, SubjectRef: strings.TrimSpace(evidence.OperationID),
		SourceRef: strings.TrimSpace(evidence.SourceRef), AuthoritySnapshotID: evidence.AuthoritySnapshotID,
		DispatchID: evidence.DispatchID, OperationID: evidence.OperationID, Revision: nextEvidenceRevision(aggregate.EvidenceSequence + 1),
		Summary: evidence.Evidence.Summary, ContentHash: evidence.Evidence.ContentHash,
		ExitCode: evidence.Evidence.ExitCode, EvidenceRefs: append([]string(nil), evidence.Evidence.EvidenceRefs...),
		TargetRefs:          append([]string(nil), admission.Operation.TargetRefs...),
		BindingRefs:         finding.BindingRefs,
		CoverageWorkItemIDs: finding.CoverageWorkItemIDs,
		Quality:             evidence.Evidence.Quality, ZeroWork: evidence.Evidence.ZeroWork,
		EvidenceReason: evidence.Evidence.EvidenceReason, EvidenceTags: append([]string(nil), evidence.Evidence.EvidenceTags...),
	}
	if finding.Diagnostic != "" {
		fact.EvidenceReason = strings.TrimSpace(strings.Join([]string{fact.EvidenceReason, finding.Diagnostic}, "; "))
	}
	if evidence.Evidence.Validation != nil {
		observation := evidence.Evidence.Validation.Clone()
		fact.Validation = &observation
	}
	return fact, finding, nil
}

func findExecutionEvidenceCredit(facts runtimecontract.RuntimeFacts, operation runtimecontract.RuntimeOperation, evidenceKind string, receipt *runtimecontract.RuntimeOperationReceipt) executionEvidenceCreditFinding {
	bindingRefs, coverageIDs, diagnostic := coverageEvidenceCreditForReceipt(facts, operation, evidenceKind, receipt)
	return executionEvidenceCreditFinding{
		BindingRefs:         bindingRefs,
		CoverageWorkItemIDs: coverageIDs,
		Diagnostic:          diagnostic,
	}
}

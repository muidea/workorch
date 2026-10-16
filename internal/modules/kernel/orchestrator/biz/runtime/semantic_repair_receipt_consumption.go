package runtime

import (
	"fmt"
	"slices"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Only the receipt CAS consumes the exact target-free semantic handoff.
// Current applicability remains an independent sampling debt: consumption
// cannot settle a CWI, restore stale credit or authorize another validation.
func consumeSemanticRepairFreshValidationReceipt(aggregate *RuntimeAggregate, dispatch SealedDispatch, fact runtimecontract.EvidenceFact) error {
	facts := aggregate.Facts
	if !runtimeValidationFailureRepairOwnsFreshValidationDirective(facts) || fact.Kind != runtimecontract.EvidenceKindValidation {
		return nil
	}
	directive := facts.RepairDirective.Clone()
	if len(facts.Focus.CoverageWorkItemIDs) != 1 || directive.Closure == nil ||
		dispatch.State != DispatchStateCommitted || dispatch.Candidate.Validation == nil || dispatch.RepairCommitment != nil || dispatch.ImplementationWorkCommitment != nil ||
		fact.DispatchID != dispatch.ID || !runtimecontract.EvidenceRevisionAfter(fact.Revision, directive.SupersedesEvidenceRevision) {
		return nil
	}
	index := runtimeCWICoverageIndex(facts.Coverage, facts.Focus.CoverageWorkItemIDs[0])
	if index < 0 || facts.Coverage[index].CurrentBindingRef == nil || !containsString(fact.BindingRefs, facts.Coverage[index].CurrentBindingRef.BindingID) {
		return nil
	}
	manifests := 0
	for _, session := range aggregate.CandidateRequestSessions {
		m := session.CandidateManifest
		if m != nil && m.Hash == dispatch.CandidateManifestHash && session.Transaction == dispatch.Transaction &&
			m.RepairClosureID == directive.Closure.ID && m.RepairPurpose == "" {
			manifests++
		}
	}
	if manifests != 1 {
		return fmt.Errorf("semantic repair receipt consumption lacks exact ordinary validation manifest")
	}
	execution, found := operationExecutionRecord(dispatch.OperationExecutions, fact.OperationID)
	if !found || execution.State != runtimecontract.OperationExecutionEvidenceCommitted || !containsString(dispatch.CompletedOperationIDs, fact.OperationID) {
		return fmt.Errorf("semantic repair receipt consumption lacks original completed execution")
	}
	receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
	if !found || (receipt.Feedback != nil && receipt.Feedback.DispatchID != dispatch.ID) || receipt.OperationID != fact.OperationID || execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) ||
		(receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted && receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected) {
		return fmt.Errorf("semantic repair receipt consumption lacks original owner receipt")
	}
	// Reconstruct the still-outstanding handoff by removing only this admitted
	// event from a detached view; all original repair lineage is independently
	// reverse-proved by the existing compiler, never inferred from its ID.
	previous := aggregate.CloneRaw()
	previous.Facts.Evidence.Accepted = slices.DeleteFunc(previous.Facts.Evidence.Accepted, func(e runtimecontract.EvidenceFact) bool { return e.ID == fact.ID })
	previous.Facts.Evidence.Rejected = slices.DeleteFunc(previous.Facts.Evidence.Rejected, func(e runtimecontract.EvidenceFact) bool { return e.ID == fact.ID })
	barrier, err := semanticRepairFreshValidationEvidenceRevision(previous)
	if err != nil || barrier != directive.SupersedesEvidenceRevision {
		return fmt.Errorf("semantic repair receipt consumption lacks original handoff: %v", err)
	}
	aggregate.Facts.RepairDirective = nil
	return nil
}

package runtime

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/proof"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runtimeProspectiveCoverageReceiptSetFinding projects one persisted sealed
// dispatch into the owner-neutral pure collective matcher. It never combines
// receipts across dispatches or rebuilds operation authority from current
// catalogs.
func runtimeProspectiveCoverageReceiptSetFinding(aggregate RuntimeAggregate, binding runtimecontract.CoverageEvidenceBinding, evidenceKind, dispatchID string) proof.ProspectiveCoverageReceiptSetFinding {
	binding = binding.Normalized()
	dispatchID = strings.TrimSpace(dispatchID)
	dispatch, found := runtimeDispatchForCollectiveReceiptSet(aggregate, dispatchID)
	if !found {
		return proof.ProspectiveCoverageReceiptSetFinding{
			RequiredTargetRefs: append([]string(nil), binding.ProspectiveTargetRefs...),
			MissingTargetRefs:  append([]string(nil), binding.ProspectiveTargetRefs...),
			Diagnostic:         "prospective coverage evidence has no matching sealed dispatch",
		}
	}
	members := make([]proof.ProspectiveCoverageReceiptMember, 0)
	for _, operation := range candidateRuntimeOperations(dispatch.Candidate) {
		execution, executionFound := operationExecutionRecord(dispatch.OperationExecutions, operation.ID)
		if !executionFound {
			continue
		}
		receipt, receiptFound := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
		if !receiptFound {
			continue
		}
		acceptedEvidence, evidenceFound := runtimeAcceptedEvidenceForProspectiveReceiptMember(
			aggregate.Facts.Evidence.Accepted,
			binding,
			evidenceKind,
			dispatch.ID,
			operation,
		)
		if !evidenceFound {
			continue
		}
		members = append(members, proof.ProspectiveCoverageReceiptMember{
			DispatchID: dispatch.ID, Operation: operation, Execution: execution, Receipt: receipt,
			AcceptedEvidence: &acceptedEvidence,
		})
	}
	return proof.MatchProspectiveCoverageReceiptSet(binding, evidenceKind, members)
}

func runtimeAcceptedEvidenceForProspectiveReceiptMember(
	evidence []runtimecontract.EvidenceFact,
	binding runtimecontract.CoverageEvidenceBinding,
	evidenceKind string,
	dispatchID string,
	operation runtimecontract.RuntimeOperation,
) (runtimecontract.EvidenceFact, bool) {
	for _, fact := range evidence {
		if strings.TrimSpace(fact.DispatchID) != dispatchID || strings.TrimSpace(fact.OperationID) != operation.ID ||
			strings.TrimSpace(fact.Kind) != evidenceKind ||
			!coverageWorkItemIDIncluded(fact.CoverageWorkItemIDs, binding.CoverageWorkItemID) ||
			!proof.EvidenceReferencesBinding(fact, binding.ID) || proof.WeakEvidence(fact) {
			continue
		}
		return fact, true
	}
	return runtimecontract.EvidenceFact{}, false
}

func runtimeDispatchForCollectiveReceiptSet(aggregate RuntimeAggregate, dispatchID string) (SealedDispatch, bool) {
	if dispatchID == "" {
		return SealedDispatch{}, false
	}
	if aggregate.ActiveDispatch != nil && aggregate.ActiveDispatch.ID == dispatchID {
		return *aggregate.ActiveDispatch, true
	}
	for _, dispatch := range aggregate.CompletedDispatches {
		if dispatch.ID == dispatchID {
			return dispatch, true
		}
	}
	return SealedDispatch{}, false
}

// runtimeProspectiveCoverageKindComplete requires one and the same sealed
// dispatch to carry the complete target set for an evidence kind. Accepted
// facts from separate repair/restart generations cannot be unioned into a new
// proof authority.
func runtimeProspectiveCoverageKindComplete(aggregate RuntimeAggregate, binding runtimecontract.CoverageEvidenceBinding, coverageID common.CoverageWorkItemID, evidenceKind string, evidence []runtimecontract.EvidenceFact) (string, bool) {
	binding = binding.Normalized()
	if len(binding.ProspectiveTargetRefs) <= 1 {
		return "", true
	}
	seenDispatches := make(map[string]struct{})
	for _, fact := range evidence {
		if fact.Kind != evidenceKind || !coverageWorkItemIDIncluded(fact.CoverageWorkItemIDs, coverageID) ||
			!proof.EvidenceReferencesBinding(fact, binding.ID) {
			continue
		}
		dispatchID := strings.TrimSpace(fact.DispatchID)
		if dispatchID == "" {
			continue
		}
		if _, duplicate := seenDispatches[dispatchID]; duplicate {
			continue
		}
		seenDispatches[dispatchID] = struct{}{}
		if runtimeProspectiveCoverageReceiptSetFinding(aggregate, binding, evidenceKind, dispatchID).Complete() {
			return dispatchID, true
		}
	}
	return "", false
}

// validateRuntimeProspectiveCoverageCollectiveCredit is the current-shape
// persistence fence. A process restart may load partial receipt lineage, but a
// satisfied/settled multi-target prospective CWI is invalid unless one sealed
// dispatch proves the complete all-of set for every required evidence kind.
func validateRuntimeProspectiveCoverageCollectiveCredit(aggregate RuntimeAggregate) error {
	focuses := append([]runtimecontract.CoverageFocus{aggregate.Facts.Focus}, aggregate.Facts.RemainingFocuses...)
	focuses = append(focuses, aggregate.Facts.CompletedFocuses...)
	bindings := make(map[common.CoverageWorkItemID]runtimecontract.CoverageEvidenceBinding)
	for _, focus := range focuses {
		for _, binding := range focus.Normalized().CoverageEvidenceBindings {
			if len(binding.ProspectiveTargetRefs) > 1 {
				bindings[binding.CoverageWorkItemID] = binding
			}
		}
	}
	for _, item := range aggregate.Facts.Coverage {
		if item.State != runtimecontract.RuntimeCoverageItemStateSatisfied && item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateSettled {
			continue
		}
		binding, found := bindings[item.ID]
		if !found {
			continue
		}
		for _, evidenceKind := range binding.EvidenceKinds() {
			if _, complete := runtimeProspectiveCoverageKindComplete(aggregate, binding, item.ID, evidenceKind, aggregate.Facts.Evidence.Accepted); !complete {
				return fmt.Errorf("runtime CWI %q settled without complete prospective all-of receipt set for binding %q evidence kind %q", item.ID, binding.ID, evidenceKind)
			}
		}
	}
	return nil
}

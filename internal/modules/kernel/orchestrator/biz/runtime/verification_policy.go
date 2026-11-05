package runtime

import (
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Non-assessment receipt closure uses the same policy implementation, with
// its own current eligible proof set. No assessment result is produced here.
func runtimeVerificationPolicySatisfied(aggregate RuntimeAggregate, binding runtimecontract.CoverageEvidenceBinding, coverageID common.CoverageWorkItemID, refs []string) bool {
	if binding.VerificationPolicy == nil {
		return true
	}
	evidence := acceptedEvidenceFactsForRefs(aggregate.Facts, coverageID, refs)
	eligible := make([]runtimecontract.EvidenceFact, 0, len(evidence))
	for _, fact := range evidence {
		if weakRuntimeEvidence(fact) || !runtimecontract.RuntimeEvidenceHasFreshProofApplicability(fact, aggregate.Facts.EvidenceApplicabilityObservations, aggregate.Facts.EvidenceRevision, aggregate.Reconciliation.RequestHash) || !runtimeEvidenceAppliesToCoverageWorkItemWithLineage(aggregate.Facts, fact, coverageID, aggregate.CoverageProofLineage) {
			continue
		}
		operation, found := runtimeOperationForEvidence(aggregate, fact)
		if !found || !runtimeOperationAllowedForFocus(aggregate.Facts.Focus, operation) {
			continue
		}
		eligible = append(eligible, fact)
	}
	dispatches := aggregate.CompletedDispatches
	if aggregate.ActiveDispatch != nil {
		dispatches = append(append([]runtimecontract.RuntimeDispatchRecord(nil), dispatches...), *aggregate.ActiveDispatch)
	}
	return runtimecontract.RuntimeVerificationEvidencePolicySatisfied(binding, eligible, (dispatches), aggregate.OperationReceipts)
}

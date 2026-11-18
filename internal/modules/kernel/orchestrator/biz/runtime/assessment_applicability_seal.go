package runtime

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// A new SM-3 assessment seal must consume the exact independently loaded SM-2
// observation prefix and current Resume generation. Existing dispatches retain
// their original fence; later matching observations never rewrite history.
func validateAssessmentApplicabilitySealTransition(previous, next RuntimeAggregate) error {
	known := make(map[string]bool, len(previous.CompletedDispatches)+1)
	for _, dispatch := range previous.CompletedDispatches {
		known[dispatch.ID] = true
	}
	if previous.ActiveDispatch != nil {
		known[previous.ActiveDispatch.ID] = true
	}
	checkDispatch := func(dispatch runtimecontract.RuntimeDispatchRecord) error {
		if known[dispatch.ID] || dispatch.Candidate.Assessment == nil {
			return nil
		}
		authority := dispatch.AssessmentAuthorityInput
		if authority == nil || authority.ApplicabilityReconciliationRequestHash != previous.Reconciliation.RequestHash ||
			!runtimecontract.RuntimeRawEqual(authority.ApplicabilityObservations, previous.Facts.EvidenceApplicabilityObservations) {
			return fmt.Errorf("%w: assessment applicability seal differs from independent current-generation preimage", common.ErrConflict)
		}
		if _, required, err := nextRuntimeEvidenceApplicabilityRequest(previous); err != nil || required {
			return fmt.Errorf("%w: assessment seal precedes fresh owner applicability observation: %v", common.ErrConflict, err)
		}
		return nil
	}
	for i := range next.CompletedDispatches {
		if err := checkDispatch(next.CompletedDispatches[i]); err != nil {
			return err
		}
	}
	if next.ActiveDispatch != nil {
		if err := checkDispatch(*next.ActiveDispatch); err != nil {
			return err
		}
	}
	return nil
}

package runtime

import (
	"fmt"
	"reflect"

	"github.com/muidea/workorch/internal/pkg/common"
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
	dispatches := append([]SealedDispatch(nil), next.CompletedDispatches...)
	if next.ActiveDispatch != nil {
		dispatches = append(dispatches, *next.ActiveDispatch)
	}
	for _, dispatch := range dispatches {
		if known[dispatch.ID] || dispatch.Candidate.Assessment == nil {
			continue
		}
		authority := dispatch.AssessmentAuthorityInput
		if authority == nil || authority.ApplicabilityReconciliationRequestHash != previous.Reconciliation.RequestHash ||
			!reflect.DeepEqual(authority.ApplicabilityObservations, previous.Facts.EvidenceApplicabilityObservations) {
			return fmt.Errorf("%w: assessment applicability seal differs from independent current-generation preimage", common.ErrConflict)
		}
		if _, required, err := NextRuntimeEvidenceApplicabilityRequest(previous); err != nil || required {
			return fmt.Errorf("%w: assessment seal precedes fresh owner applicability observation: %v", common.ErrConflict, err)
		}
	}
	return nil
}

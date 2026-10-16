package workorchruntimecontract

import (
	"fmt"
	"reflect"
)

// Retirement is rooted in the same stored SM-2 observation, not a free-form
// invalidation reason. The owner-private CAS separately proves the old current
// transaction and zero unresolved allocation retirement.
func ValidateRuntimeApplicabilityCandidateInvalidations(observations []RuntimeEvidenceApplicabilityObservation, attempts []RuntimeCandidateAttemptRecord, sessions []RuntimeCandidateRequestSession) error {
	requests := make(map[string]RuntimeEvidenceApplicabilityRequest, len(observations))
	for _, observation := range observations {
		requests[observation.Request.RequestID] = observation.Request
	}
	for _, attempt := range attempts {
		if attempt.ApplicabilityInvalidationRequest == nil {
			continue
		}
		request := *attempt.ApplicabilityInvalidationRequest
		original, found := requests[request.RequestID]
		if !found || !reflect.DeepEqual(original, request) || attempt.State != RuntimeCandidateAttemptStateInvalidated || attempt.Transaction.EvidenceRevision != request.EvidenceRevision {
			return fmt.Errorf("candidate applicability invalidation has no exact original SM-2 observation")
		}
	}
	for _, session := range sessions {
		if session.LastOutcome.Code != "evidence_applicability_changed" {
			continue
		}
		request, found := requests[session.LastOutcome.Signature]
		if !found || session.State != RuntimeCandidateRequestSessionTerminal || session.ProviderSubmitted || session.LastOutcome.Class != RuntimeCandidateRequestOutcomePrerequisiteStale || session.Transaction.EvidenceRevision != request.EvidenceRevision {
			return fmt.Errorf("request applicability retirement has no exact original SM-2 observation")
		}
	}
	return nil
}

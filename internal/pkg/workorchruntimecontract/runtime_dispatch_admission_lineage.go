package workorchruntimecontract

import "fmt"

// ValidateRuntimeDispatchAdmissionLineage proves that each executable SM-3
// dispatch entered through exactly one durable admission source: an accepted
// provider candidate attempt or one granted governance continuation. A
// self-consistent dispatch record alone is not execution authority.
func ValidateRuntimeDispatchAdmissionLineage(
	active *RuntimeDispatchRecord,
	completed []RuntimeDispatchRecord,
	attempts []RuntimeCandidateAttemptRecord,
	bindings []RuntimeOperationGovernanceBinding,
) error {
	dispatches := make([]RuntimeDispatchRecord, 0, len(completed)+1)
	dispatches = append(dispatches, completed...)
	if active != nil {
		dispatches = append(dispatches, *active)
	}
	for _, dispatch := range dispatches {
		if dispatch.Candidate.StateChange == nil && dispatch.Candidate.Validation == nil {
			continue
		}
		acceptedMatches := 0
		for _, attempt := range attempts {
			if attempt.State == RuntimeCandidateAttemptStateAccepted && attempt.Transaction == dispatch.Transaction &&
				attempt.DispatchID == dispatch.ID && attempt.CandidateHash == dispatch.CandidateHash &&
				attempt.DispatchAuthorityHash == dispatch.DispatchAuthorityHash {
				acceptedMatches++
			}
		}
		resumeMatches := 0
		for _, rawBinding := range bindings {
			binding := rawBinding.Normalized()
			if binding.State == RuntimeOperationGovernanceBindingGranted && binding.Grant != nil &&
				binding.Grant.Normalized().ResumeDispatchID == dispatch.ID {
				resumeMatches++
			}
		}
		if acceptedMatches+resumeMatches != 1 {
			return fmt.Errorf("runtime operation dispatch %q requires exactly one accepted-attempt or governance-resume admission, got accepted=%d resume=%d", dispatch.ID, acceptedMatches, resumeMatches)
		}
	}
	return nil
}

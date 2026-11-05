package workorchruntimecontract

import "strings"

// Select only proof groups whose exact inputs differ from this CAS's formal
// preimage. This is a local projection, not a retained successful computation.
// Collection uniqueness is checked against the complete current collections;
// an ambiguous member must enter the ordinary rejecting admission path.
func runtimeCandidateSourcesRequiringAdmission(previous *RuntimeStateRecord, active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord, attempts []RuntimeCandidateAttemptRecord, sessions []RuntimeCandidateRequestSession) ([]RuntimeCandidateAttemptRecord, []RuntimeCandidateRequestSession) {
	dispatches := make(map[string]RuntimeDispatchRecord, len(completed)+1)
	dispatchCounts := make(map[string]int, len(completed)+1)
	for _, d := range completed {
		dispatches[d.ID], dispatchCounts[d.ID] = d, dispatchCounts[d.ID]+1
	}
	if active != nil {
		dispatches[active.ID], dispatchCounts[active.ID] = *active, dispatchCounts[active.ID]+1
	}
	requests := make(map[string]RuntimeCandidateRequestSession, len(sessions))
	requestCounts := make(map[string]int, len(sessions))
	for _, session := range sessions {
		id := strings.TrimSpace(session.LogicalRequestID)
		requests[id], requestCounts[id] = session, requestCounts[id]+1
	}
	type attemptIdentity struct {
		snapshot string
		ordinal  int
	}
	attemptCounts := make(map[attemptIdentity]int, len(attempts))
	for _, attempt := range attempts {
		attemptCounts[attemptIdentity{attempt.Transaction.AuthoritySnapshotID, attempt.Attempt}]++
	}
	var pending []RuntimeCandidateAttemptRecord
	neededRequests := make(map[string]bool)
	for _, attempt := range attempts {
		if attempt.State != RuntimeCandidateAttemptStateAccepted {
			continue
		}
		id := strings.TrimSpace(attempt.LogicalRequestID)
		if attemptCounts[attemptIdentity{attempt.Transaction.AuthoritySnapshotID, attempt.Attempt}] == 1 && dispatchCounts[attempt.DispatchID] == 1 && requestCounts[id] == 1 &&
			committedRuntimeCandidateSource(previous, dispatches[attempt.DispatchID], attempt, requests[id]) {
			continue
		}
		pending = append(pending, attempt)
		neededRequests[id] = true
	}
	var inputs []RuntimeCandidateRequestSession
	for _, session := range sessions {
		if neededRequests[strings.TrimSpace(session.LogicalRequestID)] {
			inputs = append(inputs, session)
		}
	}
	return pending, inputs
}

func committedRuntimeCandidateSource(previous *RuntimeStateRecord, dispatch RuntimeDispatchRecord, attempt RuntimeCandidateAttemptRecord, session RuntimeCandidateRequestSession) bool {
	if previous == nil {
		return false
	}
	return committedRuntimeCandidateAttempt(previous, attempt) && committedRuntimeDispatch(previous, dispatch) && committedRuntimeCandidateRequest(previous, session)
}

func committedRuntimeCandidateAttempt(previous *RuntimeStateRecord, attempt RuntimeCandidateAttemptRecord) bool {
	if previous != nil {
		for _, old := range previous.CandidateAttempts {
			if old.Transaction.AuthoritySnapshotID == attempt.Transaction.AuthoritySnapshotID && old.Attempt == attempt.Attempt && runtimeRawEqual(old, attempt) {
				return true
			}
		}
	}
	return false
}

func committedRuntimeCandidateRequest(previous *RuntimeStateRecord, session RuntimeCandidateRequestSession) bool {
	if previous != nil {
		for _, old := range previous.CandidateRequestSessions {
			if old.LogicalRequestID == session.LogicalRequestID && runtimeRawEqual(old, session) {
				return true
			}
		}
	}
	return false
}

func committedRuntimeDispatch(previous *RuntimeStateRecord, dispatch RuntimeDispatchRecord) bool {
	if previous == nil {
		return false
	}
	if previous.ActiveDispatch != nil && previous.ActiveDispatch.ID == dispatch.ID && runtimeRawEqual(*previous.ActiveDispatch, dispatch) {
		return true
	}
	for _, old := range previous.CompletedDispatches {
		if old.ID == dispatch.ID && runtimeRawEqual(old, dispatch) {
			return true
		}
	}
	return false
}

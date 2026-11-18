package workorchruntimecontract

import (
	"slices"
	"strings"
)

type runtimeCandidateAdmissionsCache struct {
	previous  *RuntimeStateRecord
	active    *RuntimeDispatchRecord
	completed []RuntimeDispatchRecord
	attempts  []RuntimeCandidateAttemptRecord
	sessions  []RuntimeCandidateRequestSession
	pending   []RuntimeCandidateAttemptRecord
	inputs    []RuntimeCandidateRequestSession
}

func runtimeCandidateSourcesRequiringAdmissionInSession(session *runtimeSemanticValidationSession, active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord, attempts []RuntimeCandidateAttemptRecord, sessions []RuntimeCandidateRequestSession) ([]RuntimeCandidateAttemptRecord, []RuntimeCandidateRequestSession) {
	if session == nil || session.committedPreimage == nil {
		return runtimeCandidateSourcesRequiringAdmission(nil, active, completed, attempts, sessions)
	}
	if cache := session.candidateAdmissions; cache != nil &&
		cache.previous == session.committedPreimage &&
		runtimeRawEqual(cache.active, active) &&
		len(cache.completed) == len(completed) &&
		len(cache.attempts) == len(attempts) &&
		len(cache.sessions) == len(sessions) &&
		runtimeRawEqual(cache.completed, completed) &&
		runtimeRawEqual(cache.attempts, attempts) &&
		runtimeRawEqual(cache.sessions, sessions) {
		return cache.pending, cache.inputs
	}
	pending, inputs := runtimeCandidateSourcesRequiringAdmission(session.committedPreimage, active, completed, attempts, sessions)
	session.candidateAdmissions = &runtimeCandidateAdmissionsCache{
		previous:  session.committedPreimage,
		active:    active,
		completed: completed,
		attempts:  attempts,
		sessions:  sessions,
		pending:   pending,
		inputs:    inputs,
	}
	return pending, inputs
}

// Select only proof groups whose exact inputs differ from this CAS's formal
// preimage. This is a local projection, not a retained successful computation.
// Collection uniqueness is checked against the complete current collections;
// an ambiguous member must enter the ordinary rejecting admission path.
func runtimeCandidateSourcesRequiringAdmission(previous *RuntimeStateRecord, active *RuntimeDispatchRecord, completed []RuntimeDispatchRecord, attempts []RuntimeCandidateAttemptRecord, sessions []RuntimeCandidateRequestSession) ([]RuntimeCandidateAttemptRecord, []RuntimeCandidateRequestSession) {
	hasAccepted := false
	for i := range attempts {
		if attempts[i].State == RuntimeCandidateAttemptStateAccepted {
			hasAccepted = true
			break
		}
	}
	if !hasAccepted {
		return nil, nil
	}
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
	var prevAttempts map[attemptIdentity][]int
	var prevRequests map[string][]int
	var prevCompleted map[string][]int
	if previous != nil {
		prevAttempts = make(map[attemptIdentity][]int, len(previous.CandidateAttempts))
		for i := range previous.CandidateAttempts {
			old := &previous.CandidateAttempts[i]
			key := attemptIdentity{old.Transaction.AuthoritySnapshotID, old.Attempt}
			prevAttempts[key] = append(prevAttempts[key], i)
		}
		prevRequests = make(map[string][]int, len(previous.CandidateRequestSessions))
		for i := range previous.CandidateRequestSessions {
			id := previous.CandidateRequestSessions[i].LogicalRequestID
			prevRequests[id] = append(prevRequests[id], i)
		}
		prevCompleted = make(map[string][]int, len(previous.CompletedDispatches))
		for i := range previous.CompletedDispatches {
			id := previous.CompletedDispatches[i].ID
			prevCompleted[id] = append(prevCompleted[id], i)
		}
	}
	attemptCommitted := func(attempt RuntimeCandidateAttemptRecord) bool {
		if previous == nil {
			return false
		}
		for _, idx := range prevAttempts[attemptIdentity{attempt.Transaction.AuthoritySnapshotID, attempt.Attempt}] {
			if runtimeRawEqual(previous.CandidateAttempts[idx], attempt) {
				return true
			}
		}
		return false
	}
	requestCommitted := func(session RuntimeCandidateRequestSession) bool {
		if previous == nil {
			return false
		}
		for _, idx := range prevRequests[session.LogicalRequestID] {
			if runtimeRawEqual(previous.CandidateRequestSessions[idx], session) {
				return true
			}
		}
		return false
	}
	dispatchCommitted := func(dispatch RuntimeDispatchRecord) bool {
		if previous == nil {
			return false
		}
		if previous.ActiveDispatch != nil && sameRuntimeDispatchPreSealLineage(*previous.ActiveDispatch, dispatch) {
			return true
		}
		for _, idx := range prevCompleted[dispatch.ID] {
			if sameRuntimeDispatchPreSealLineage(previous.CompletedDispatches[idx], dispatch) {
				return true
			}
		}
		return false
	}
	var pending []RuntimeCandidateAttemptRecord
	neededRequests := make(map[string]bool)
	for _, attempt := range attempts {
		if attempt.State != RuntimeCandidateAttemptStateAccepted {
			continue
		}
		id := strings.TrimSpace(attempt.LogicalRequestID)
		if attemptCounts[attemptIdentity{attempt.Transaction.AuthoritySnapshotID, attempt.Attempt}] == 1 &&
			dispatchCounts[attempt.DispatchID] == 1 &&
			requestCounts[id] == 1 &&
			attemptCommitted(attempt) &&
			requestCommitted(requests[id]) &&
			dispatchCommitted(dispatches[attempt.DispatchID]) {
			continue
		}
		pending = append(pending, attempt)
		neededRequests[id] = true
	}
	if len(pending) == 0 {
		return nil, nil
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
	return committedRuntimeCandidateAttempt(previous, attempt) && committedRuntimeDispatchPreSealLineage(previous, dispatch) && committedRuntimeCandidateRequest(previous, session)
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

func stripRuntimeDispatchPostSealProgress(in RuntimeDispatchRecord) RuntimeDispatchRecord {
	in.State = ""
	in.CompletedOperationIDs = nil
	in.AssessmentAdjudicationHash = ""
	in.AssessmentAdjudication = nil
	if len(in.OperationExecutions) > 0 {
		execs := slices.Clone(in.OperationExecutions)
		for i := range execs {
			execs[i].State = ""
			execs[i].Fence = 0
			execs[i].OwnerReceiptRef = ""
			execs[i].OwnerReceiptHash = ""
			execs[i].EvidenceFactID = ""
			execs[i].EvidenceFactHash = ""
			execs[i].ClaimedAt = ""
		}
		in.OperationExecutions = execs
	}
	return in
}

func sameRuntimeDispatchPreSealLineage(a, b RuntimeDispatchRecord) bool {
	if a.ID != b.ID || a.CandidateHash != b.CandidateHash || a.CandidateManifestHash != b.CandidateManifestHash || a.DispatchAuthorityHash != b.DispatchAuthorityHash {
		return false
	}
	if runtimeRawEqual(a, b) {
		return true
	}
	return runtimeRawEqual(stripRuntimeDispatchPostSealProgress(a), stripRuntimeDispatchPostSealProgress(b))
}

func committedRuntimeDispatchPreSealLineage(previous *RuntimeStateRecord, dispatch RuntimeDispatchRecord) bool {
	if previous == nil {
		return false
	}
	if previous.ActiveDispatch != nil && sameRuntimeDispatchPreSealLineage(*previous.ActiveDispatch, dispatch) {
		return true
	}
	for _, old := range previous.CompletedDispatches {
		if sameRuntimeDispatchPreSealLineage(old, dispatch) {
			return true
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

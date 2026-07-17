package workorchruntimecontract

import "testing"

func TestRuntimeCandidateAttemptScopeValidationSupportsLegacyRetryButRejectsNewDuplicate(t *testing.T) {
	facts := testRuntimeFacts()
	decision := validDecisionForState(ConvergenceStateNeedStateChange)
	current := decision.AuthorityTransaction()
	legacy := current
	legacy.AuthoritySnapshotID = "legacy-resealed-snapshot"
	legacy.Attempt = 7
	contractID := StableAuthorityContractID(facts.WorkspaceID, facts.RunID, current)
	failure := &RuntimeCandidateFailure{Class: RuntimeCandidateFailureClassContract, Reason: "invalid candidate"}
	values := []RuntimeCandidateAttemptRecord{
		{Transaction: legacy, Attempt: 0, StablePrefixHash: "stable-prefix", State: RuntimeCandidateAttemptStateFailed, Failure: failure},
		{AuthorityContractID: contractID, Transaction: current, Attempt: 1, StablePrefixHash: "stable-prefix", State: RuntimeCandidateAttemptStatePending},
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, values); err != nil {
		t.Fatalf("legacy same-contract retry rejected: %v", err)
	}
	duplicate := current
	duplicate.AuthoritySnapshotID = "duplicate-snapshot"
	duplicate.Attempt = 1
	values = []RuntimeCandidateAttemptRecord{
		{AuthorityContractID: contractID, Transaction: current, Attempt: 0, StablePrefixHash: "stable-prefix", State: RuntimeCandidateAttemptStateFailed, Failure: failure},
		{AuthorityContractID: contractID, Transaction: duplicate, Attempt: 0, StablePrefixHash: "stable-prefix", State: RuntimeCandidateAttemptStateFailed, Failure: failure},
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, values); err == nil {
		t.Fatal("new duplicate attempt 0 was accepted across snapshots")
	}
}

func TestRuntimeCandidateAttemptScopeValidationKeepsLegacyRunawayLoadable(t *testing.T) {
	facts := testRuntimeFacts()
	transaction := validDecisionForState(ConvergenceStateNeedStateChange).AuthorityTransaction()
	failure := &RuntimeCandidateFailure{Class: RuntimeCandidateFailureClassContract, Reason: "invalid candidate"}
	values := make([]RuntimeCandidateAttemptRecord, 0, 3)
	for attempt := 0; attempt < 3; attempt++ {
		legacy := transaction
		legacy.AuthoritySnapshotID = "legacy-snapshot-" + string(rune('a'+attempt))
		legacy.Attempt = attempt
		values = append(values, RuntimeCandidateAttemptRecord{
			Transaction: legacy, Attempt: 0, StablePrefixHash: "stable-prefix",
			State: RuntimeCandidateAttemptStateFailed, Failure: failure,
		})
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, values); err != nil {
		t.Fatalf("legacy runaway must remain loadable for formal exhaustion: %v", err)
	}
}

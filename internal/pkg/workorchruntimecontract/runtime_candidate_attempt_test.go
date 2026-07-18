package workorchruntimecontract

import "testing"

func TestRuntimeCandidateAttemptScopeValidationRequiresExplicitContractAndRejectsDuplicate(t *testing.T) {
	facts := testRuntimeFacts()
	decision := validDecisionForState(ConvergenceStateNeedStateChange)
	current := decision.AuthorityTransaction()
	resealed := current
	resealed.AuthoritySnapshotID = "resealed-snapshot"
	resealed.Attempt = 7
	contractID := StableAuthorityContractID(facts.WorkspaceID, facts.RunID, current)
	failure := &RuntimeCandidateFailure{Class: RuntimeCandidateFailureClassContract, Reason: "invalid candidate"}
	values := []RuntimeCandidateAttemptRecord{
		{Transaction: resealed, Attempt: 0, StablePrefixHash: "stable-prefix", State: RuntimeCandidateAttemptStateFailed, Failure: failure},
		{AuthorityContractID: contractID, Transaction: current, Attempt: 1, StablePrefixHash: "stable-prefix", State: RuntimeCandidateAttemptStatePending},
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, values); err == nil {
		t.Fatal("candidate attempt without authority contract id was accepted")
	}
	values[0].AuthorityContractID = contractID
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, values); err != nil {
		t.Fatalf("explicit same-contract retry rejected: %v", err)
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

func TestRuntimeCandidateAttemptScopeValidationRejectsRunawayAttempts(t *testing.T) {
	facts := testRuntimeFacts()
	transaction := validDecisionForState(ConvergenceStateNeedStateChange).AuthorityTransaction()
	failure := &RuntimeCandidateFailure{Class: RuntimeCandidateFailureClassContract, Reason: "invalid candidate"}
	values := make([]RuntimeCandidateAttemptRecord, 0, 3)
	contractID := StableAuthorityContractID(facts.WorkspaceID, facts.RunID, transaction)
	for attempt := 0; attempt < 3; attempt++ {
		resealed := transaction
		resealed.AuthoritySnapshotID = "resealed-snapshot-" + string(rune('a'+attempt))
		resealed.Attempt = attempt
		values = append(values, RuntimeCandidateAttemptRecord{
			AuthorityContractID: contractID, Transaction: resealed, Attempt: 0, StablePrefixHash: "stable-prefix",
			State: RuntimeCandidateAttemptStateFailed, Failure: failure,
		})
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, values); err == nil {
		t.Fatal("runaway candidate attempts were accepted")
	}
}

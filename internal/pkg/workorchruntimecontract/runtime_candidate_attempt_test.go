package workorchruntimecontract

import (
	"strings"
	"testing"
)

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
		{Transaction: resealed, Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-0", State: RuntimeCandidateAttemptStateFailed, Failure: failure},
		{AuthorityContractID: contractID, Transaction: current, Attempt: 1, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-1", State: RuntimeCandidateAttemptStatePending},
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
		{AuthorityContractID: contractID, Transaction: current, Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-0", State: RuntimeCandidateAttemptStateFailed, Failure: failure},
		{AuthorityContractID: contractID, Transaction: duplicate, Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-duplicate", State: RuntimeCandidateAttemptStateFailed, Failure: failure},
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
			AuthorityContractID: contractID, Transaction: resealed, Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-runaway",
			State: RuntimeCandidateAttemptStateFailed, Failure: failure,
		})
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, values); err == nil {
		t.Fatal("runaway candidate attempts were accepted")
	}
}

func TestRuntimeCandidateFeasibilityRecordValidationRejectsAmbiguousOrAcceptedLedger(t *testing.T) {
	failure := &FailureFact{Class: FailureClassCandidateContractFailure, Code: "target_invalid", Signature: "file/target_invalid", Message: "target is invalid"}
	values := []RuntimeCandidateFeasibilityRecord{
		{OperationID: "operation-1", AdmissibilitySnapshotID: "inventory-1", Feasible: false, Failure: failure},
		{OperationID: "operation-1", AdmissibilitySnapshotID: "inventory-1", Feasible: true},
	}
	if err := ValidateRuntimeCandidateFeasibilityRecords(values); err == nil {
		t.Fatal("duplicate candidate feasibility operation was accepted")
	}
	facts := testRuntimeFacts()
	transaction := validDecisionForState(ConvergenceStateNeedStateChange).AuthorityTransaction()
	values = values[:1]
	attempt := RuntimeCandidateAttemptRecord{
		AuthorityContractID: StableAuthorityContractID(facts.WorkspaceID, facts.RunID, transaction), Transaction: transaction,
		Attempt: 0, StablePrefixHash: "stable-prefix", ManifestHash: "manifest-1", LogicalRequestID: "request-0", State: RuntimeCandidateAttemptStateAccepted, Feasibilities: values,
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(facts.WorkspaceID, facts.RunID, []RuntimeCandidateAttemptRecord{attempt}); err == nil {
		t.Fatal("accepted candidate attempt retained an unsealed feasibility ledger")
	}
}

func TestRuntimeStateRecordRejectsTerminalDecisionWithPendingCandidateAttempt(t *testing.T) {
	facts := testRuntimeFacts()
	decision := validDecisionForState(ConvergenceStateFailed)
	transaction := decision.AuthorityTransaction()
	record := RuntimeStateRecord{
		SchemaVersion: RuntimeStateSchemaVersionCurrent,
		StateRevision: 1,
		Definition: RuntimeDefinitionContext{
			Revision: facts.DefinitionRevision, GoalID: "goal-1", GoalDefinition: "goal",
			DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"file/write"},
		},
		Facts: facts, Decision: decision,
		CandidateAttempts: []RuntimeCandidateAttemptRecord{{
			AuthorityContractID: StableAuthorityContractID(facts.WorkspaceID, facts.RunID, transaction),
			Transaction:         transaction,
			Attempt:             0,
			StablePrefixHash:    "stable-prefix",
			ManifestHash:        "manifest-1",
			LogicalRequestID:    "request-0",
			State:               RuntimeCandidateAttemptStatePending,
		}},
	}
	if err := record.Validate(); err == nil || !strings.Contains(err.Error(), "terminal runtime state cannot retain a pending candidate attempt") {
		t.Fatalf("terminal runtime state with pending candidate attempt err=%v", err)
	}
}

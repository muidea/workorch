package workorchruntimecontract

import (
	"testing"
	"time"
)

func runtimeContractNonTerminalRecoveryRequest(kind RuntimeNonTerminalRecoveryKind, observedAt string) RuntimeNonTerminalRecoveryRequest {
	request := RuntimeNonTerminalRecoveryRequest{
		WorkspaceID: "ws-v2", RunID: "run-v2", Kind: kind,
		FailedLeaseGeneration: 2, ExpectedRuntimeRevision: 7, OwnerObservedRuntimeRevision: 8,
		IssuedAtStateRevision: 8, ObservedAt: observedAt,
	}
	switch kind {
	case RuntimeNonTerminalRecoveryCASConflict:
		request.FailureSignature = RuntimeNonTerminalRecoveryFailureCASConflict
	case RuntimeNonTerminalRecoveryOwnerTransient:
		request.FailureSignature = RuntimeNonTerminalRecoveryFailureOwnerTransient
		request.OwnerFeedbackHash = StableRuntimeFeedbackHash(RuntimeFeedback{})
	}
	request.RequestID = StableRuntimeNonTerminalRecoveryRequestID(request)
	return request
}

func TestRuntimeNonTerminalRecoveryRequestIDExcludesRedeliveryTimeButBindsAdmissionFence(t *testing.T) {
	first := runtimeContractNonTerminalRecoveryRequest(RuntimeNonTerminalRecoveryCASConflict, "2026-09-04T12:00:00Z")
	if err := first.Validate(); err != nil {
		t.Fatal(err)
	}
	redelivered := first
	redelivered.ObservedAt = "2026-09-04T12:01:00Z"
	if got := StableRuntimeNonTerminalRecoveryRequestID(redelivered); got != first.RequestID {
		t.Fatalf("redelivery changed stable request ID: got %q want %q", got, first.RequestID)
	}
	if !SameRuntimeNonTerminalRecoveryRequestIdentity(first, redelivered) {
		t.Fatal("redelivery time split one typed persistence outcome into two requests")
	}
	reissued := first
	reissued.IssuedAtStateRevision++
	if StableRuntimeNonTerminalRecoveryRequestID(reissued) == first.RequestID {
		t.Fatal("fresh-load admission revision is missing from request identity")
	}
}

func TestRuntimeNonTerminalRecoveryRecordBindsSharedCASBudgetAndSuccessor(t *testing.T) {
	policy := DefaultRuntimeRecoveryPolicy()
	request := runtimeContractNonTerminalRecoveryRequest(RuntimeNonTerminalRecoveryOwnerTransient, "2026-09-04T12:00:00Z")
	dueAt, err := RuntimeNonTerminalRecoveryDueAt(request.ObservedAt, 1)
	if err != nil {
		t.Fatal(err)
	}
	lease := RuntimeDriveLease{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID,
		AggregateRevision: 9, RecoveryGeneration: 3,
		Reason: RuntimeDriveLeaseReasonOwnerTransient, FailureSignature: request.FailureSignature,
		RecoveryRequestID: request.RequestID, DueAt: dueAt, State: RuntimeDriveLeaseScheduled,
	}
	record := RuntimeNonTerminalRecoveryRecord{
		Request: request, Disposition: RuntimeNonTerminalRecoveryDispositionBudgetConsumed,
		Attempt: 1, RecordedAtStateRevision: 9, SuccessorLease: lease,
	}
	ledger := RuntimeRecoveryLedger{CASReloadAttempts: 1, NonTerminalRecovery: record}
	if err := ledger.Validate(policy); err != nil {
		t.Fatalf("valid owner-transient shared-CAS record: %v", err)
	}
	if ledger.OperationRecoveryAttempts != 0 {
		t.Fatal("owner transient consumed operation recovery authority")
	}
	tamperedFence := record
	tamperedFence.RecordedAtStateRevision++
	tamperedFence.SuccessorLease.AggregateRevision++
	if err := tamperedFence.Validate(policy); err == nil {
		t.Fatal("recorded revision escaped the issued-at CAS fence")
	}

	claimed := lease
	claimed.State = RuntimeDriveLeaseRunning
	claimed.AggregateRevision++
	if err := ValidateRuntimeNonTerminalRecoveryLeaseBinding(ledger, claimed); err != nil {
		t.Fatalf("claimed successor lost request lineage: %v", err)
	}
	recovered := claimed
	recovered.State = RuntimeDriveLeaseScheduled
	recovered.AggregateRevision++
	recovered.RecoveryGeneration++
	recovered.DueAt = time.Date(2026, 9, 4, 12, 2, 0, 0, time.UTC).Format(time.RFC3339Nano)
	if err := ValidateRuntimeNonTerminalRecoveryLeaseBinding(ledger, recovered); err != nil {
		t.Fatalf("restart successor lost request lineage: %v", err)
	}

	exhausted := record
	exhausted.Disposition = RuntimeNonTerminalRecoveryDispositionBudgetExhausted
	if err := exhausted.Validate(policy); err == nil {
		t.Fatal("exhaustion below the shared CAS limit was accepted")
	}
}

func TestRuntimeDriveLeaseRecoveryRequestLegalityIsNarrow(t *testing.T) {
	base := RuntimeDriveLease{
		WorkspaceID: "ws-v2", RunID: "run-v2", AggregateRevision: 2, RecoveryGeneration: 1,
		FailureSignature: RuntimeNonTerminalRecoveryFailureCASConflict, RecoveryRequestID: "request-1",
		DueAt: "2026-09-04T12:00:00Z", State: RuntimeDriveLeaseScheduled,
	}
	cas := base
	cas.Reason = RuntimeDriveLeaseReasonCASReload
	if err := cas.Validate(); err != nil {
		t.Fatalf("CAS request-bound lease: %v", err)
	}
	provider := base
	provider.Reason = RuntimeDriveLeaseReasonProviderRetry
	if err := provider.Validate(); err == nil {
		t.Fatal("provider retry smuggled a nonterminal recovery request")
	}
	owner := base
	owner.Reason = RuntimeDriveLeaseReasonOwnerTransient
	owner.RecoveryRequestID = ""
	if err := owner.Validate(); err == nil {
		t.Fatal("owner-transient lease omitted its recovery request")
	}
}

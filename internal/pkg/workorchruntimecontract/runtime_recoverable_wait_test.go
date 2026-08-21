package workorchruntimecontract

import (
	"testing"
	"time"
)

func TestRuntimeRecoverableWaitIdentityValidationAndProjection(t *testing.T) {
	ledger, err := NewRuntimeExecutionBudgetLedger(DefaultRuntimeExecutionBudgetPolicy())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, time.August, 21, 8, 0, 0, 0, time.UTC)
	record := RuntimeRecoverableWaitRecord{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", AuthorityTransactionID: "authority-1",
		StateRevisionOpened: 2, ReasonCode: RuntimeRecoverableWaitReasonProviderCeilingUnavailable,
		ProbeKind: RuntimeRecoverableWaitProbeProviderCeiling, ProbeOwner: RuntimeModuleLLM,
		ProbeRequestHash: "provider-ceiling-request-1", RecoveryConditionHash: "provider-ceiling-condition-1",
		OpenedAt: base.Format(time.RFC3339Nano), NextEligibleAt: base.Add(time.Second).Format(time.RFC3339Nano), Deadline: base.Add(time.Minute).Format(time.RFC3339Nano),
		MaxAttempts: 1, BudgetDimension: RuntimeBudgetDimensionRecoveryProbe,
		BudgetPolicyRevision: ledger.Policy.PolicyRevision, Status: RuntimeRecoverableWaitWaiting,
	}
	record.WaitID = StableRuntimeRecoverableWaitID(record)
	ledger, allocations, _, err := ReserveRuntimeBudgetAllocations(ledger, []RuntimeBudgetAllocationSpec{{
		Dimension: record.BudgetDimension, LogicalConsumptionID: record.WaitID, ScopeRef: record.ProbeRequestHash, ReservedAmount: 1,
	}})
	if err != nil || len(allocations) != 1 {
		t.Fatalf("allocations=%+v err=%v", allocations, err)
	}
	record.BudgetAllocationID = allocations[0].AllocationID
	if err := record.Validate(ledger); err != nil {
		t.Fatal(err)
	}
	view, err := RuntimeRecoverableWaitProjection(record, 2)
	if err != nil || view.WaitID != record.WaitID || view.Status != RuntimeRecoverableWaitWaiting || view.RecoveryConditionSummary == "" {
		t.Fatalf("view=%+v err=%v", view, err)
	}

	drifted := record
	drifted.ProbeKind = RuntimeRecoverableWaitProbeCWIReconciliation
	if err := drifted.Validate(ledger); err == nil {
		t.Fatal("unsupported owner/probe combination was accepted")
	}
	drifted = record
	drifted.ProbeRequestHash = "other-request"
	if err := drifted.Validate(ledger); err == nil {
		t.Fatal("drifted wait identity was accepted")
	}
}

func TestReleaseRuntimeBudgetAllocationRequiresUnconsumedReservation(t *testing.T) {
	ledger, err := NewRuntimeExecutionBudgetLedger(DefaultRuntimeExecutionBudgetPolicy())
	if err != nil {
		t.Fatal(err)
	}
	ledger, allocations, _, err := ReserveRuntimeBudgetAllocations(ledger, []RuntimeBudgetAllocationSpec{{
		Dimension: RuntimeBudgetDimensionRecoveryProbe, LogicalConsumptionID: "wait-1", ScopeRef: "probe-1", ReservedAmount: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	released, changed, err := ReleaseRuntimeBudgetAllocation(ledger, allocations[0].AllocationID, "wait-expired/wait-1")
	if err != nil || !changed {
		t.Fatalf("release changed=%v err=%v", changed, err)
	}
	allocation, _ := RuntimeBudgetAllocationForID(released.Allocations, allocations[0].AllocationID)
	if allocation.State != RuntimeBudgetAllocationReleased {
		t.Fatalf("released allocation=%+v", allocation)
	}
	duplicate, changed, err := ReleaseRuntimeBudgetAllocation(released, allocations[0].AllocationID, "wait-expired/wait-1")
	if err != nil || changed || len(duplicate.Allocations) != len(released.Allocations) {
		t.Fatalf("duplicate release changed=%v err=%v", changed, err)
	}
}

func TestRuntimeRecoverableWaitRequiresExactBudgetReconciliationByTerminalShape(t *testing.T) {
	ledger, err := NewRuntimeExecutionBudgetLedger(DefaultRuntimeExecutionBudgetPolicy())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, time.August, 21, 9, 0, 0, 0, time.UTC)
	wait := RuntimeRecoverableWaitRecord{
		WorkspaceID: "ws-1", RunID: "run-1", DefinitionRevision: "definition-1", AuthorityTransactionID: "authority-1",
		StateRevisionOpened: 2, ReasonCode: RuntimeRecoverableWaitReasonProviderCeilingUnavailable,
		ProbeKind: RuntimeRecoverableWaitProbeProviderCeiling, ProbeOwner: RuntimeModuleLLM,
		ProbeRequestHash: "provider-ceiling-request-1", RecoveryConditionHash: "provider-ceiling-condition-1",
		OpenedAt: base.Format(time.RFC3339Nano), NextEligibleAt: base.Add(time.Second).Format(time.RFC3339Nano), Deadline: base.Add(time.Minute).Format(time.RFC3339Nano),
		MaxAttempts: 1, BudgetDimension: RuntimeBudgetDimensionRecoveryProbe,
		BudgetPolicyRevision: ledger.Policy.PolicyRevision, Status: RuntimeRecoverableWaitWaiting,
	}
	wait.WaitID = StableRuntimeRecoverableWaitID(wait)
	ledger, allocations, _, err := ReserveRuntimeBudgetAllocations(ledger, []RuntimeBudgetAllocationSpec{{
		Dimension: wait.BudgetDimension, LogicalConsumptionID: wait.WaitID, ScopeRef: wait.ProbeRequestHash, ReservedAmount: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	wait.BudgetAllocationID = allocations[0].AllocationID
	forgedExpired := wait
	forgedExpired.Status = RuntimeRecoverableWaitExpired
	if err := forgedExpired.Validate(ledger); err == nil {
		t.Fatal("expired wait retained a reserved allocation")
	}

	unknownHash := StableRuntimeRecoverableWaitProbeUnknownHash(wait.WaitID, wait.ProbeRequestHash)
	ledger, _, _, err = JoinRuntimeBudgetConsumption(ledger, RuntimeBudgetConsumptionJoin{
		AllocationID: wait.BudgetAllocationID, Owner: wait.ProbeOwner, PhysicalConsumptionID: wait.ProbeRequestHash,
		UnknownAmount: 1, OwnerResultRef: "recoverable-wait-unknown/" + wait.WaitID, OwnerResultHash: unknownHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := wait.Validate(ledger); err == nil {
		t.Fatal("active unknown wait without reconciliation was accepted")
	}
	reconciliation, err := NewRuntimeBudgetReconciliationRecord(ledger, wait.BudgetAllocationID, string(wait.ProbeKind), wait.ProbeOwner, wait.ProbeRequestHash, wait.RecoveryConditionHash)
	if err != nil {
		t.Fatal(err)
	}
	ledger, _, err = OpenRuntimeBudgetReconciliation(ledger, reconciliation)
	if err != nil {
		t.Fatal(err)
	}
	if err := wait.Validate(ledger); err != nil {
		t.Fatalf("active unknown wait with exact reconciliation: %v", err)
	}
	exhausted := wait
	exhausted.Status = RuntimeRecoverableWaitExhausted
	if err := exhausted.Validate(ledger); err == nil {
		t.Fatal("exhausted wait retained a pending reconciliation")
	}
	ledger, _, err = FailRuntimeBudgetReconciliation(ledger, reconciliation.ReconciliationID, unknownHash)
	if err != nil {
		t.Fatal(err)
	}
	if err := exhausted.Validate(ledger); err != nil {
		t.Fatalf("exhausted wait with failed reconciliation: %v", err)
	}
}

package workorchruntimecontract

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/llmaudit"
)

func TestRuntimeExecutionBudgetReserveObserveAndProject(t *testing.T) {
	ledger, err := NewRuntimeExecutionBudgetLedger(DefaultRuntimeExecutionBudgetPolicy())
	if err != nil {
		t.Fatal(err)
	}
	specs, err := RuntimeProviderBudgetAllocationSpecs(ledger, RuntimeProviderDocumentOperations, 0, 1, "logical-1", "physical-1")
	if err != nil {
		t.Fatal(err)
	}
	reserved, allocations, duplicate, err := ReserveRuntimeBudgetAllocations(ledger, specs)
	if err != nil || duplicate || len(allocations) != 4 {
		t.Fatalf("reserve allocations=%d duplicate=%t err=%v", len(allocations), duplicate, err)
	}
	replayed, _, duplicate, err := ReserveRuntimeBudgetAllocations(reserved, specs)
	if err != nil || !duplicate || len(replayed.Allocations) != len(reserved.Allocations) {
		t.Fatalf("duplicate reserve duplicate=%t err=%v", duplicate, err)
	}
	observation, err := NewRuntimeProviderBudgetObservation("logical-1", "physical-1", RuntimeProviderConsumptionConfirmed, &llmaudit.ProviderUsage{InputTokens: 17, OutputTokens: 9, TotalTokens: 26})
	if err != nil {
		t.Fatal(err)
	}
	confirmed, changed, err := ApplyRuntimeProviderBudgetObservation(reserved, observation)
	if err != nil || !changed {
		t.Fatalf("confirm changed=%t err=%v", changed, err)
	}
	duplicateConfirmed, changed, err := ApplyRuntimeProviderBudgetObservation(confirmed, observation)
	if err != nil || changed || len(duplicateConfirmed.Consumptions) != len(confirmed.Consumptions) {
		t.Fatalf("duplicate observation changed=%t err=%v", changed, err)
	}
	view, err := BuildRuntimeExecutionBudgetView("ws-1", "run-1", 7, confirmed, "", "")
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeBudgetDimensionView(t, view, RuntimeBudgetDimensionProviderCall, 1, 0, 0)
	assertRuntimeBudgetDimensionView(t, view, RuntimeBudgetDimensionProviderInputToken, 17, 0, 0)
	assertRuntimeBudgetDimensionView(t, view, RuntimeBudgetDimensionProviderOutputToken, 9, 0, 0)
}

func TestRuntimeExecutionBudgetUnknownIsNotZeroAndExactReconciliationCanConfirm(t *testing.T) {
	ledger, err := NewRuntimeExecutionBudgetLedger(DefaultRuntimeExecutionBudgetPolicy())
	if err != nil {
		t.Fatal(err)
	}
	specs, _ := RuntimeProviderBudgetAllocationSpecs(ledger, RuntimeProviderDocumentOperations, 0, 1, "logical-unknown", "physical-unknown")
	ledger, _, _, err = ReserveRuntimeBudgetAllocations(ledger, specs)
	if err != nil {
		t.Fatal(err)
	}
	unknown, _ := NewRuntimeProviderBudgetObservation("logical-unknown", "physical-unknown", RuntimeProviderConsumptionUnknown, nil)
	ledger, changed, err := ApplyRuntimeProviderBudgetObservation(ledger, unknown)
	if err != nil || !changed {
		t.Fatalf("unknown observation changed=%t err=%v", changed, err)
	}
	view, err := BuildRuntimeExecutionBudgetView("ws-1", "run-1", 2, ledger, "wait-1", "")
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeBudgetDimensionView(t, view, RuntimeBudgetDimensionProviderCall, 0, 0, 1)
	confirmed, _ := NewRuntimeProviderBudgetObservation("logical-unknown", "physical-unknown", RuntimeProviderConsumptionConfirmed, &llmaudit.ProviderUsage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5})
	ledger, changed, err = ApplyRuntimeProviderBudgetObservation(ledger, confirmed)
	if err != nil || !changed {
		t.Fatalf("exact reconciliation changed=%t err=%v", changed, err)
	}
	view, err = BuildRuntimeExecutionBudgetView("ws-1", "run-1", 3, ledger, "", "")
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeBudgetDimensionView(t, view, RuntimeBudgetDimensionProviderCall, 1, 0, 0)
}

func TestRuntimeExecutionBudgetExhaustionIsTypedAndDoesNotMutateLedger(t *testing.T) {
	policy := DefaultRuntimeExecutionBudgetPolicy()
	for idx := range policy.Dimensions {
		if policy.Dimensions[idx].Dimension == RuntimeBudgetDimensionProviderCall {
			policy.Dimensions[idx].Limit = 1
		}
	}
	policy.PolicyHash = StableRuntimeExecutionBudgetPolicyHash(policy)
	ledger, err := NewRuntimeExecutionBudgetLedger(policy)
	if err != nil {
		t.Fatal(err)
	}
	ledger, _, _, err = ReserveRuntimeBudgetAllocations(ledger, []RuntimeBudgetAllocationSpec{{Dimension: RuntimeBudgetDimensionProviderCall, LogicalConsumptionID: "logical-1", ScopeRef: "physical-1", ReservedAmount: 1}})
	if err != nil {
		t.Fatal(err)
	}
	before := ledger.Clone()
	_, _, _, err = ReserveRuntimeBudgetAllocations(ledger, []RuntimeBudgetAllocationSpec{{Dimension: RuntimeBudgetDimensionProviderCall, LogicalConsumptionID: "logical-2", ScopeRef: "physical-2", ReservedAmount: 1}})
	var exhausted *RuntimeBudgetExhaustedError
	if !errors.As(err, &exhausted) || exhausted.Dimension != RuntimeBudgetDimensionProviderCall || len(ledger.Allocations) != len(before.Allocations) {
		t.Fatalf("exhaustion=%+v err=%v ledger mutated=%t", exhausted, err, len(ledger.Allocations) != len(before.Allocations))
	}
}

func TestRuntimeExecutionBudgetViewDerivesRepairAndRecoveryFromSourceLedgers(t *testing.T) {
	convergence := DefaultRuntimeConvergencePolicy()
	convergence.MaxRepairAttemptsPerFocus = 2
	recovery := DefaultRuntimeRecoveryPolicy()
	recovery.MaxOperationRecoveryAttempts = 4
	policy := RuntimeExecutionBudgetPolicyForRuntimePolicies(convergence, recovery)
	if err := ValidateRuntimeExecutionBudgetPolicySources(policy, convergence, recovery); err != nil {
		t.Fatal(err)
	}
	ledger, err := NewRuntimeExecutionBudgetLedger(policy)
	if err != nil {
		t.Fatal(err)
	}
	view, err := BuildRuntimeExecutionBudgetView("ws-1", "run-1", 9, ledger, "", "", RuntimeExecutionBudgetSourceUsage{
		OperationRecoveryAttempts: 3,
		SameFocusRepairAttempts:   2,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeBudgetDimensionView(t, view, RuntimeBudgetDimensionOperationRecovery, 3, 0, 0)
	assertRuntimeBudgetDimensionView(t, view, RuntimeBudgetDimensionSameFocusRepair, 2, 0, 0)
	for _, item := range view.Dimensions {
		switch item.Dimension {
		case RuntimeBudgetDimensionOperationRecovery:
			if item.Limit != 4 || item.Remaining != 1 || item.Exhausted {
				t.Fatalf("operation recovery view = %+v", item)
			}
		case RuntimeBudgetDimensionSameFocusRepair:
			if item.Limit != 2 || item.Remaining != 0 || !item.Exhausted {
				t.Fatalf("same-focus repair view = %+v", item)
			}
		}
	}
}

func TestRuntimeExecutionBudgetRejectsSecondRepairOrRecoveryLedger(t *testing.T) {
	policy := DefaultRuntimeExecutionBudgetPolicy()
	ledger, err := NewRuntimeExecutionBudgetLedger(policy)
	if err != nil {
		t.Fatal(err)
	}
	for _, dimension := range []RuntimeBudgetDimension{RuntimeBudgetDimensionOperationRecovery, RuntimeBudgetDimensionSameFocusRepair} {
		_, _, _, err := ReserveRuntimeBudgetAllocations(ledger, []RuntimeBudgetAllocationSpec{{
			Dimension: dimension, LogicalConsumptionID: "duplicate-source", ScopeRef: "duplicate-source", ReservedAmount: 1,
		}})
		if !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("source-owned dimension %q reserve err = %v", dimension, err)
		}
	}
	drifted := policy.Clone()
	for idx := range drifted.Dimensions {
		if drifted.Dimensions[idx].Dimension == RuntimeBudgetDimensionSameFocusRepair {
			drifted.Dimensions[idx].Limit++
		}
	}
	drifted.PolicyHash = StableRuntimeExecutionBudgetPolicyHash(drifted)
	if err := ValidateRuntimeExecutionBudgetPolicySources(drifted, DefaultRuntimeConvergencePolicy(), DefaultRuntimeRecoveryPolicy()); err == nil {
		t.Fatal("execution budget accepted drift from the authoritative repair policy")
	}
}

func TestRuntimeExecutionBudgetReconciliationIsPersistedAndIdempotent(t *testing.T) {
	ledger, err := NewRuntimeExecutionBudgetLedger(DefaultRuntimeExecutionBudgetPolicy())
	if err != nil {
		t.Fatal(err)
	}
	spec := RuntimeBudgetAllocationSpec{Dimension: RuntimeBudgetDimensionRecoveryProbe, LogicalConsumptionID: "wait-1", ScopeRef: "probe-1", ReservedAmount: 1}
	ledger, allocations, _, err := ReserveRuntimeBudgetAllocations(ledger, []RuntimeBudgetAllocationSpec{spec})
	if err != nil {
		t.Fatal(err)
	}
	unknownJoin := RuntimeBudgetConsumptionJoin{
		AllocationID: allocations[0].AllocationID, Owner: RuntimeModuleLLM, PhysicalConsumptionID: "probe-1", UnknownAmount: 1,
		OwnerResultRef: "probe/unknown", OwnerResultHash: "probe-unknown-hash",
	}
	ledger, _, _, err = JoinRuntimeBudgetConsumption(ledger, unknownJoin)
	if err != nil {
		t.Fatal(err)
	}
	reconciliation, err := NewRuntimeBudgetReconciliationRecord(ledger, allocations[0].AllocationID, "provider_ceiling", RuntimeModuleLLM, "probe-1", "condition-1")
	if err != nil {
		t.Fatal(err)
	}
	ledger, changed, err := OpenRuntimeBudgetReconciliation(ledger, reconciliation)
	if err != nil || !changed {
		t.Fatalf("open reconciliation changed=%v err=%v", changed, err)
	}
	duplicate, changed, err := OpenRuntimeBudgetReconciliation(ledger, reconciliation)
	if err != nil || changed || len(duplicate.Reconciliations) != 1 {
		t.Fatalf("duplicate reconciliation changed=%v values=%+v err=%v", changed, duplicate.Reconciliations, err)
	}
	confirmedJoin := RuntimeBudgetConsumptionJoin{
		AllocationID: allocations[0].AllocationID, Owner: RuntimeModuleLLM, PhysicalConsumptionID: "probe-1", ConfirmedAmount: 1,
		OwnerResultRef: "probe/result", OwnerResultHash: "probe-result-hash",
	}
	ledger, fact, changed, err := ConfirmRuntimeBudgetReconciliation(ledger, reconciliation.ReconciliationID, confirmedJoin)
	if err != nil || !changed || fact.ConfirmedAmount != 1 || ledger.Reconciliations[0].Status != RuntimeBudgetReconciliationConfirmed {
		t.Fatalf("confirm reconciliation changed=%v fact=%+v record=%+v err=%v", changed, fact, ledger.Reconciliations, err)
	}
	duplicate, _, changed, err = ConfirmRuntimeBudgetReconciliation(ledger, reconciliation.ReconciliationID, confirmedJoin)
	if err != nil || changed || duplicate.Reconciliations[0].Status != RuntimeBudgetReconciliationConfirmed {
		t.Fatalf("duplicate confirmation changed=%v record=%+v err=%v", changed, duplicate.Reconciliations, err)
	}
	if _, _, _, err := ReserveRuntimeBudgetAllocations(ledger, []RuntimeBudgetAllocationSpec{spec}); err == nil {
		t.Fatal("confirmed allocation was reopened by duplicate reserve")
	}
	forged := ledger.Clone()
	forged.Allocations[0].OwnerFactRef = "owner-result-other"
	forged.Allocations[0].AllocationHash = StableRuntimeBudgetAllocationHash(forged.Allocations[0])
	if err := forged.Validate(); err == nil {
		t.Fatal("allocation owner fact drift from its consumption was accepted")
	}
}

func TestRuntimeExecutionBudgetReconciliationCanReleaseExactUnknownConsumption(t *testing.T) {
	ledger, allocation, reconciliation := runtimeUnknownBudgetReconciliationFixture(t, "release")
	released, changed, err := ReleaseRuntimeBudgetReconciliation(ledger, reconciliation.ReconciliationID, "probe/released", "probe-released-hash")
	if err != nil || !changed {
		t.Fatalf("release reconciliation changed=%v err=%v", changed, err)
	}
	value, found := RuntimeBudgetAllocationForID(released.Allocations, allocation.AllocationID)
	if !found || value.State != RuntimeBudgetAllocationReleased || value.OwnerFactRef != "probe/released" || len(released.Consumptions) != 0 {
		t.Fatalf("released allocation=%+v found=%v consumptions=%+v", value, found, released.Consumptions)
	}
	if len(released.Reconciliations) != 1 || released.Reconciliations[0].Status != RuntimeBudgetReconciliationReleased || released.Reconciliations[0].OwnerResultHash != "probe-released-hash" {
		t.Fatalf("released reconciliation=%+v", released.Reconciliations)
	}
	duplicate, changed, err := ReleaseRuntimeBudgetReconciliation(released, reconciliation.ReconciliationID, "probe/released", "probe-released-hash")
	if err != nil || changed || len(duplicate.Consumptions) != 0 {
		t.Fatalf("duplicate release changed=%v err=%v ledger=%+v", changed, err, duplicate)
	}
	if _, _, err := ReleaseRuntimeBudgetReconciliation(released, reconciliation.ReconciliationID, "probe/forged", "probe-released-hash"); err == nil {
		t.Fatal("conflicting reconciliation release was accepted")
	}
	view, err := BuildRuntimeExecutionBudgetView("ws-1", "run-1", 1, released, "", "")
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeBudgetDimensionView(t, view, RuntimeBudgetDimensionRecoveryProbe, 0, 0, 0)
}

func TestRuntimeExecutionBudgetFailedReconciliationSealsExhaustedUnknown(t *testing.T) {
	ledger, allocation, reconciliation := runtimeUnknownBudgetReconciliationFixture(t, "failure")
	exhausted, changed, err := FailRuntimeBudgetReconciliation(ledger, reconciliation.ReconciliationID, "probe-failed-hash")
	if err != nil || !changed {
		t.Fatalf("fail reconciliation changed=%v err=%v", changed, err)
	}
	value, found := RuntimeBudgetAllocationForID(exhausted.Allocations, allocation.AllocationID)
	if !found || value.State != RuntimeBudgetAllocationExhausted || len(exhausted.Consumptions) != 1 || exhausted.Consumptions[0].UnknownAmount != 1 {
		t.Fatalf("exhausted allocation=%+v found=%v consumptions=%+v", value, found, exhausted.Consumptions)
	}
	if len(exhausted.Reconciliations) != 1 || exhausted.Reconciliations[0].Status != RuntimeBudgetReconciliationFailed || exhausted.Reconciliations[0].OwnerResultHash != "probe-failed-hash" {
		t.Fatalf("failed reconciliation=%+v", exhausted.Reconciliations)
	}
	duplicate, changed, err := FailRuntimeBudgetReconciliation(exhausted, reconciliation.ReconciliationID, "probe-failed-hash")
	if err != nil || changed || len(duplicate.Consumptions) != 1 {
		t.Fatalf("duplicate failure changed=%v err=%v ledger=%+v", changed, err, duplicate)
	}
	view, err := BuildRuntimeExecutionBudgetView("ws-1", "run-1", 1, exhausted, "", "")
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeBudgetDimensionView(t, view, RuntimeBudgetDimensionRecoveryProbe, 0, 0, 1)
	if _, _, _, err := ReserveRuntimeBudgetAllocations(exhausted, []RuntimeBudgetAllocationSpec{{
		Dimension: RuntimeBudgetDimensionRecoveryProbe, LogicalConsumptionID: allocation.LogicalConsumptionID, ScopeRef: allocation.ScopeRef, ReservedAmount: 1,
	}}); err == nil {
		t.Fatal("exhausted allocation was reopened")
	}
}

func TestRuntimeExecutionBudgetReconciliationPersistsStillUnknownResult(t *testing.T) {
	ledger, allocation, reconciliation := runtimeUnknownBudgetReconciliationFixture(t, "still-unknown")
	stillUnknown, changed, err := MarkRuntimeBudgetReconciliationStillUnknown(ledger, reconciliation.ReconciliationID, "probe-still-unknown-hash")
	if err != nil || !changed {
		t.Fatalf("mark still unknown changed=%v err=%v", changed, err)
	}
	if len(stillUnknown.Reconciliations) != 1 || stillUnknown.Reconciliations[0].Status != RuntimeBudgetReconciliationStillUnknown || stillUnknown.Reconciliations[0].OwnerResultHash != "probe-still-unknown-hash" {
		t.Fatalf("still-unknown reconciliation=%+v", stillUnknown.Reconciliations)
	}
	duplicate, changed, err := MarkRuntimeBudgetReconciliationStillUnknown(stillUnknown, reconciliation.ReconciliationID, "probe-still-unknown-hash")
	if err != nil || changed || duplicate.Reconciliations[0].Status != RuntimeBudgetReconciliationStillUnknown {
		t.Fatalf("duplicate still-unknown changed=%v err=%v ledger=%+v", changed, err, duplicate)
	}
	if _, _, err := MarkRuntimeBudgetReconciliationStillUnknown(stillUnknown, reconciliation.ReconciliationID, "probe-forged-hash"); err == nil {
		t.Fatal("conflicting still-unknown reconciliation was accepted")
	}
	confirmed, _, changed, err := ConfirmRuntimeBudgetReconciliation(stillUnknown, reconciliation.ReconciliationID, RuntimeBudgetConsumptionJoin{
		AllocationID: allocation.AllocationID, Owner: RuntimeModuleLLM, PhysicalConsumptionID: "probe-still-unknown", ConfirmedAmount: 1,
		OwnerResultRef: "probe/confirmed/still-unknown", OwnerResultHash: "probe-confirmed-hash",
	})
	if err != nil || !changed || confirmed.Reconciliations[0].Status != RuntimeBudgetReconciliationConfirmed {
		t.Fatalf("confirm after still-unknown changed=%v err=%v ledger=%+v", changed, err, confirmed)
	}
}

func runtimeUnknownBudgetReconciliationFixture(t *testing.T, suffix string) (RuntimeExecutionBudgetLedger, RuntimeBudgetAllocation, RuntimeBudgetReconciliationRecord) {
	t.Helper()
	ledger, err := NewRuntimeExecutionBudgetLedger(DefaultRuntimeExecutionBudgetPolicy())
	if err != nil {
		t.Fatal(err)
	}
	logicalID := "wait-" + suffix
	physicalID := "probe-" + suffix
	ledger, allocations, _, err := ReserveRuntimeBudgetAllocations(ledger, []RuntimeBudgetAllocationSpec{{
		Dimension: RuntimeBudgetDimensionRecoveryProbe, LogicalConsumptionID: logicalID, ScopeRef: physicalID, ReservedAmount: 1,
	}})
	if err != nil || len(allocations) != 1 {
		t.Fatalf("reserve allocations=%+v err=%v", allocations, err)
	}
	allocation := allocations[0]
	ledger, _, _, err = JoinRuntimeBudgetConsumption(ledger, RuntimeBudgetConsumptionJoin{
		AllocationID: allocation.AllocationID, Owner: RuntimeModuleLLM, PhysicalConsumptionID: physicalID, UnknownAmount: 1,
		OwnerResultRef: "probe/unknown/" + suffix, OwnerResultHash: "probe-unknown-hash-" + suffix,
	})
	if err != nil {
		t.Fatal(err)
	}
	reconciliation, err := NewRuntimeBudgetReconciliationRecord(ledger, allocation.AllocationID, "provider_ceiling", RuntimeModuleLLM, physicalID, "condition-"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	ledger, changed, err := OpenRuntimeBudgetReconciliation(ledger, reconciliation)
	if err != nil || !changed {
		t.Fatalf("open reconciliation changed=%v err=%v", changed, err)
	}
	return ledger, allocation, reconciliation
}

func assertRuntimeBudgetDimensionView(t *testing.T, view RuntimeExecutionBudgetView, dimension RuntimeBudgetDimension, confirmed, reserved, unknown int64) {
	t.Helper()
	for _, item := range view.Dimensions {
		if item.Dimension == dimension {
			if item.Confirmed != confirmed || item.Reserved != reserved || item.Unknown != unknown {
				t.Fatalf("budget dimension %q = %+v", dimension, item)
			}
			return
		}
	}
	t.Fatalf("budget dimension %q is missing", dimension)
}

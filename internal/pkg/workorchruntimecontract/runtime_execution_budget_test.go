package workorchruntimecontract

import (
	"errors"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func providerBudgetTestCeiling() *ProviderCeiling {
	return &ProviderCeiling{
		Selection:           ProviderInferenceSelection{Target: ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "fixed-model"}},
		ContextWindowTokens: 500000, ProviderMaxOutputTokens: 128000, EffectiveMaxOutputTokens: 128000,
		MaxInputTokens: 279000, MaxBytes: 1116000, CapabilityRevision: "fixed-cap", ConfigurationRevision: "fixed-config",
	}
}

func TestProviderBudgetUsesCapabilityBoundsWithoutChangingRunTotal(t *testing.T) {
	policy := DefaultRuntimeExecutionBudgetPolicy()
	ledger, err := NewRuntimeExecutionBudgetLedger(policy)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []RuntimeProviderDocumentKind{RuntimeProviderDocumentOperations, RuntimeProviderDocumentSemanticPlan} {
		for _, sequence := range []int{1, 2} {
			specs, err := RuntimeProviderBudgetAllocationSpecs(ledger, providerBudgetTestCeiling(), 128000, kind, 1, sequence, "logical", "physical")
			if err != nil {
				t.Fatal(err)
			}
			for _, spec := range specs {
				switch spec.Dimension {
				case RuntimeBudgetDimensionProviderInputToken:
					if spec.ReservedAmount != 500000 {
						t.Fatalf("input reservation=%d", spec.ReservedAmount)
					}
				case RuntimeBudgetDimensionProviderOutputToken:
					if spec.ReservedAmount != 128000 {
						t.Fatalf("output reservation=%d", spec.ReservedAmount)
					}
				}
			}
			if _, _, _, err := ReserveRuntimeBudgetAllocations(ledger, specs); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, dimension := range []RuntimeBudgetDimension{RuntimeBudgetDimensionProviderInputToken, RuntimeBudgetDimensionProviderOutputToken} {
		p, _ := RuntimeBudgetDimensionPolicyFor(policy, dimension)
		if p.Limit != 4096*131072 {
			t.Fatal("changed total token budget")
		}
	}
}

func TestProviderBudgetRejectsMissingCapacityAndPreservesSealedLimits(t *testing.T) {
	ledger, _ := NewRuntimeExecutionBudgetLedger(DefaultRuntimeExecutionBudgetPolicy())
	for _, c := range []*ProviderCeiling{nil, {}, {ContextWindowTokens: 500000}} {
		if _, err := RuntimeProviderBudgetAllocationSpecs(ledger, c, 128000, RuntimeProviderDocumentOperations, 0, 1, "logical", "physical"); err == nil {
			t.Fatal("missing capacity accepted")
		}
	}
	for _, output := range []int{0, -1, 8192, 128001} {
		if _, err := RuntimeProviderBudgetAllocationSpecs(ledger, providerBudgetTestCeiling(), output, RuntimeProviderDocumentOperations, 0, 1, "logical", "physical"); err == nil {
			t.Fatal("invalid output accepted")
		}
	}
	for _, dimension := range []RuntimeBudgetDimension{RuntimeBudgetDimensionProviderInputToken, RuntimeBudgetDimensionProviderOutputToken} {
		for _, limitedBalance := range []bool{false, true} {
			policy := DefaultRuntimeExecutionBudgetPolicy()
			for i := range policy.Dimensions {
				p := &policy.Dimensions[i]
				if p.Dimension != dimension {
					continue
				}
				p.MaxIncrement = 100000
				if limitedBalance {
					p.Limit = 100000
				}
			}
			policy.PolicyHash = StableRuntimeExecutionBudgetPolicyHash(policy)
			before, err := NewRuntimeExecutionBudgetLedger(policy)
			if err != nil {
				t.Fatal(err)
			}
			specs, err := RuntimeProviderBudgetAllocationSpecs(before, providerBudgetTestCeiling(), 128000, RuntimeProviderDocumentOperations, 0, 1, "logical", "physical")
			if err != nil {
				t.Fatal(err)
			}
			_, _, _, err = ReserveRuntimeBudgetAllocations(before, specs)
			var exhausted *RuntimeBudgetExhaustedError
			if !errors.As(err, &exhausted) || exhausted.Dimension != dimension || exhausted.MaxIncrement != 100000 {
				t.Fatalf("wrong pre-call refusal: %v", err)
			}
			if len(before.Allocations) != 0 || before.Policy.PolicyHash != policy.PolicyHash {
				t.Fatal("rejection changed sealed budget")
			}
		}
	}
}

func TestRuntimeProviderBudgetAllocationSpecsReserveJoinAndProject(t *testing.T) {
	ledger, err := NewRuntimeExecutionBudgetLedger(DefaultRuntimeExecutionBudgetPolicy())
	if err != nil {
		t.Fatal(err)
	}
	specs, err := RuntimeProviderBudgetAllocationSpecs(ledger, providerBudgetTestCeiling(), 128000, RuntimeProviderDocumentOperations, 0, 1, "logical-1", "physical-1")
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
	confirmed := reserved
	joins := make([]RuntimeBudgetConsumptionJoin, 0, len(allocations))
	for _, allocation := range allocations {
		amount := int64(1)
		switch allocation.Dimension {
		case RuntimeBudgetDimensionProviderInputToken:
			amount = 17
		case RuntimeBudgetDimensionProviderOutputToken:
			amount = 9
		}
		join := RuntimeBudgetConsumptionJoin{
			AllocationID: allocation.AllocationID, Owner: RuntimeModuleLLM, PhysicalConsumptionID: "physical-1",
			ConfirmedAmount: amount, OwnerResultRef: "llm/provider-request-audit/physical-1", OwnerResultHash: "audit-hash-1",
		}
		var changed bool
		confirmed, _, changed, err = JoinRuntimeBudgetConsumption(confirmed, join)
		if err != nil || !changed {
			t.Fatalf("join dimension=%s changed=%t err=%v", allocation.Dimension, changed, err)
		}
		joins = append(joins, join)
	}
	duplicateConfirmed := confirmed
	for _, join := range joins {
		var changed bool
		duplicateConfirmed, _, changed, err = JoinRuntimeBudgetConsumption(duplicateConfirmed, join)
		if err != nil || changed {
			t.Fatalf("duplicate join allocation=%s changed=%t err=%v", join.AllocationID, changed, err)
		}
	}
	if len(duplicateConfirmed.Consumptions) != len(confirmed.Consumptions) {
		t.Fatalf("duplicate join changed consumption count: got=%d want=%d", len(duplicateConfirmed.Consumptions), len(confirmed.Consumptions))
	}
	view, err := BuildRuntimeExecutionBudgetView("ws-1", "run-1", 7, confirmed, "", "")
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeBudgetDimensionView(t, view, RuntimeBudgetDimensionProviderCall, 1, 0, 0)
	assertRuntimeBudgetDimensionView(t, view, RuntimeBudgetDimensionProviderInputToken, 17, 0, 0)
	assertRuntimeBudgetDimensionView(t, view, RuntimeBudgetDimensionProviderOutputToken, 9, 0, 0)
}

func TestRuntimeProviderBudgetAllocationSpecsDoNotRepeatLogicalAttemptDimensionsDuringRecovery(t *testing.T) {
	ledger, err := NewRuntimeExecutionBudgetLedger(DefaultRuntimeExecutionBudgetPolicy())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		kind     RuntimeProviderDocumentKind
		attempt  int
		sequence int
		included []RuntimeBudgetDimension
		excluded []RuntimeBudgetDimension
	}{
		{
			name: "operations correction first call", kind: RuntimeProviderDocumentOperations, attempt: 1, sequence: 1,
			included: []RuntimeBudgetDimension{RuntimeBudgetDimensionCandidateCorrection},
			excluded: []RuntimeBudgetDimension{RuntimeBudgetDimensionWholeDocumentRecovery},
		},
		{
			name: "operations correction recovery", kind: RuntimeProviderDocumentOperations, attempt: 1, sequence: 2,
			included: []RuntimeBudgetDimension{RuntimeBudgetDimensionWholeDocumentRecovery},
			excluded: []RuntimeBudgetDimension{RuntimeBudgetDimensionCandidateCorrection},
		},
		{
			name: "semantic initial recovery", kind: RuntimeProviderDocumentSemanticPlan, attempt: 0, sequence: 2,
			included: []RuntimeBudgetDimension{RuntimeBudgetDimensionWholeDocumentRecovery},
			excluded: []RuntimeBudgetDimension{RuntimeBudgetDimensionSemanticGeneration, RuntimeBudgetDimensionSemanticCorrection},
		},
		{
			name: "semantic correction first call", kind: RuntimeProviderDocumentSemanticPlan, attempt: 1, sequence: 1,
			included: []RuntimeBudgetDimension{RuntimeBudgetDimensionSemanticGeneration, RuntimeBudgetDimensionSemanticCorrection},
			excluded: []RuntimeBudgetDimension{RuntimeBudgetDimensionWholeDocumentRecovery},
		},
		{
			name: "semantic correction recovery", kind: RuntimeProviderDocumentSemanticPlan, attempt: 1, sequence: 2,
			included: []RuntimeBudgetDimension{RuntimeBudgetDimensionWholeDocumentRecovery},
			excluded: []RuntimeBudgetDimension{RuntimeBudgetDimensionSemanticGeneration, RuntimeBudgetDimensionSemanticCorrection},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			specs, err := RuntimeProviderBudgetAllocationSpecs(ledger, providerBudgetTestCeiling(), 128000, testCase.kind, testCase.attempt, testCase.sequence, "logical-1", "physical-1")
			if err != nil {
				t.Fatal(err)
			}
			found := make(map[RuntimeBudgetDimension]bool, len(specs))
			for _, spec := range specs {
				found[spec.Dimension] = true
			}
			for _, dimension := range testCase.included {
				if !found[dimension] {
					t.Fatalf("budget specs = %+v, want dimension %q", specs, dimension)
				}
			}
			for _, dimension := range testCase.excluded {
				if found[dimension] {
					t.Fatalf("budget specs = %+v, do not want repeated dimension %q", specs, dimension)
				}
			}
		})
	}
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

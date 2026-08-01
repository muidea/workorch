package runtime

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRepairLedgerTransitionsAreOrchestratorOwned(t *testing.T) {
	policy := runtimecontract.DefaultRuntimeConvergencePolicy()
	policy.MaxRepairAttemptsPerFocus = 2
	sig := runtimecontract.StableAssessmentFailureSignature("focus-1", runtimecontract.AssessmentDispositionRepairableGap, &runtimecontract.RuntimeAssessmentGap{CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}})
	ledger, fresh, err := beginRepairCycle(runtimecontract.RuntimeRepairLedger{}, "focus-1", "slice-1", runtimecontract.StableRepairCycleID("run-1", "focus-1", sig, "assessment-1"), sig, policy)
	if err != nil || !fresh || ledger.RepairAttempts != 1 {
		t.Fatalf("first cycle = %+v fresh=%v err=%v", ledger, fresh, err)
	}
	ledger, fresh, err = beginRepairCycle(ledger, "focus-1", "slice-1", ledger.LastCycleID, sig, policy)
	if err != nil || fresh || ledger.RepairAttempts != 1 {
		t.Fatalf("duplicate cycle = %+v fresh=%v err=%v", ledger, fresh, err)
	}
	ledger = noteRepairProgress(runtimecontract.RuntimeRepairLedger{FocusID: "focus-1", RepairAttempts: 1, NoProgressCount: 2, ActiveGap: &runtimecontract.RuntimeAssessmentGap{Reason: "gap"}}, "evidence-2")
	if ledger.RepairAttempts != 1 || ledger.NoProgressCount != 0 || ledger.ActiveGap != nil || ledger.LastProgressRevision != "evidence-2" {
		t.Fatalf("same-focus progress = %+v", ledger)
	}
	ledger = resetRepairForFocus("focus-2", "slice-2", "evidence-3")
	if ledger.FocusID != "focus-2" || ledger.RepairAttempts != 0 || ledger.LastProgressRevision != "evidence-3" {
		t.Fatalf("focus advance reset = %+v", ledger)
	}
}

func TestStateChangeAndRecoveryLedgerTransitionsAreOrchestratorOwned(t *testing.T) {
	ledger := runtimecontract.NewRuntimeStateChangeLedger(runtimecontract.RuntimeRunStateChangeRequirement{Required: true, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}})
	ledger = applyAcceptedStateChangeEvidence(ledger, runtimecontract.EvidenceFact{ID: "unbound", Kind: runtimecontract.EvidenceKindStateChange}, []common.CoverageWorkItemID{"cwi-1"})
	if ledger.Satisfied {
		t.Fatalf("unbound evidence satisfied ledger: %+v", ledger)
	}
	ledger = applyAcceptedStateChangeEvidence(ledger, runtimecontract.EvidenceFact{ID: "state-1", Kind: runtimecontract.EvidenceKindStateChange, Revision: "evidence-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}, nil)
	if !ledger.Satisfied || len(ledger.EvidenceRefs) != 1 {
		t.Fatalf("bound state change did not satisfy: %+v", ledger)
	}
	policy := runtimecontract.RuntimeRecoveryPolicy{MaxProviderTransportAttempts: 1}
	recovery, consumed, err := consumeRecoveryBudget(runtimecontract.RuntimeRecoveryLedger{}, runtimecontract.RuntimeRecoveryBudgetProviderTransport, policy)
	if err != nil || !consumed || recovery.ProviderTransportAttempts != 1 {
		t.Fatalf("first recovery consume = %+v consumed=%v err=%v", recovery, consumed, err)
	}
	_, consumed, err = consumeRecoveryBudget(recovery, runtimecontract.RuntimeRecoveryBudgetProviderTransport, policy)
	if err != nil || consumed {
		t.Fatalf("recovery budget exhaustion consumed=%v err=%v", consumed, err)
	}
}

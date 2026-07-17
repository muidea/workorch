package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeRepairLedgerBeginCycleIsIdempotentAndBudgeted(t *testing.T) {
	policy := DefaultRuntimeConvergencePolicy()
	policy.MaxRepairAttemptsPerFocus = 2
	ledger := RuntimeRepairLedger{}

	sig := StableAssessmentFailureSignature("focus-1", AssessmentDispositionRepairableGap, &RuntimeAssessmentGap{
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
		MissingEvidenceKinds: []string{EvidenceKindStateChange},
		Reason:               "gap",
	})
	cycle1 := StableRepairCycleID("run-1", "focus-1", sig, "assessment-1")
	var err error
	var isNew bool
	ledger, isNew, err = ledger.BeginRepairCycle("focus-1", "slice-1", cycle1, sig, policy)
	if err != nil || !isNew || ledger.RepairAttempts != 1 {
		t.Fatalf("first cycle = attempts=%d new=%v err=%v", ledger.RepairAttempts, isNew, err)
	}
	// duplicate cycle id does not increment
	ledger, isNew, err = ledger.BeginRepairCycle("focus-1", "slice-1", cycle1, sig, policy)
	if err != nil || isNew || ledger.RepairAttempts != 1 {
		t.Fatalf("duplicate cycle = attempts=%d new=%v err=%v", ledger.RepairAttempts, isNew, err)
	}
	cycle2 := StableRepairCycleID("run-1", "focus-1", sig, "assessment-2")
	ledger, isNew, err = ledger.BeginRepairCycle("focus-1", "slice-1", cycle2, sig, policy)
	if err != nil || !isNew || ledger.RepairAttempts != 2 {
		t.Fatalf("second cycle = attempts=%d new=%v err=%v", ledger.RepairAttempts, isNew, err)
	}
	cycle3 := StableRepairCycleID("run-1", "focus-1", sig, "assessment-3")
	ledger, isNew, err = ledger.BeginRepairCycle("focus-1", "slice-1", cycle3, sig, policy)
	if err != nil || isNew || !ledger.Exhausted {
		t.Fatalf("budget exhaust = attempts=%d new=%v exhausted=%v err=%v", ledger.RepairAttempts, isNew, ledger.Exhausted, err)
	}
}

func TestStableAssessmentFailureSignatureIgnoresMessageNoise(t *testing.T) {
	gap := &RuntimeAssessmentGap{
		CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
		MissingEvidenceKinds: []string{EvidenceKindValidation, EvidenceKindStateChange},
		TargetRefs:           []string{"b.go", "a.go"},
		Reason:               "ignored-for-signature-stability-check-only-fields-matter",
		FailureClass:         FailureClassExecutionFailure,
		FailureCode:          "exit_nonzero",
	}
	left := StableAssessmentFailureSignature("focus-1", AssessmentDispositionRepairableGap, gap)
	gap.Reason = "different reason text must not change signature"
	right := StableAssessmentFailureSignature("focus-1", AssessmentDispositionRepairableGap, gap)
	if left == "" || left != right {
		t.Fatalf("signature unstable: %q vs %q", left, right)
	}
}

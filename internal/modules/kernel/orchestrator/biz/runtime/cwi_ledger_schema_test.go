package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeCWITransitionsFailClosedWithoutCurrentLedger(t *testing.T) {
	facts := controllerFacts()
	facts.CWILedgerSchemaVersion = ""

	if _, err := settleRuntimeCWIsForCurrentFocus(facts); err == nil {
		t.Fatal("legacy CWI facts were accepted by settlement")
	}
	if _, err := advanceRuntimeCWIFocus(facts); err == nil {
		t.Fatal("legacy CWI facts were accepted by focus advance")
	}
	if _, _, err := reopenRuntimeCWIs(facts, []runtimeCWIReopenTrigger{{CoverageWorkItemID: "cwi-v2", TriggerRef: "receipt-1", Reason: "test"}}); err == nil {
		t.Fatal("legacy CWI facts were accepted by reopen")
	}
}

func TestRuntimeAggregateRejectsMissingCurrentCWILedger(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.CWILedgerSchemaVersion = ""
	if err := aggregate.Validate(); err == nil || !strings.Contains(err.Error(), "current CWI ledger schema") {
		t.Fatalf("legacy aggregate validation error = %v", err)
	}
}

func TestSemanticCWIFocusProjectionRebindsRepairAuthorityAcrossRetirementAndReseal(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	facts := aggregate.Facts
	oldFocus := facts.Focus.Normalized()
	if len(oldFocus.CoverageEvidenceBindings) != 1 || facts.Coverage[0].CurrentBindingRef == nil {
		t.Fatalf("test fixture has no current CWI binding: %+v", facts)
	}

	// The repair state is valid authority for the current sealed focus. Both
	// semantic retirement and re-seal change that focus projection inside SM-2,
	// so neither transition may carry it into the new projection.
	facts.Repair = runtimecontract.RuntimeRepairLedger{
		FocusID:               oldFocus.ID,
		SliceID:               oldFocus.SliceID,
		RepairAttempts:        3,
		ValidationOnlyRetries: 1,
		NoProgressCount:       2,
		SignatureCounts:       []runtimecontract.RuntimeFailureSignatureCount{{Signature: "old-focus-gap", Count: 2}},
		LastCycleID:           "old-focus-cycle",
		ActiveGap: &runtimecontract.RuntimeAssessmentGap{
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
			MissingEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			Reason:               "old focus requires a repair",
		},
		Exhausted: true,
	}
	facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{
		ID:                         "repair/old-focus",
		FocusID:                    oldFocus.ID,
		Trigger:                    runtimecontract.RuntimeRepairTriggerAssessmentGap,
		RequiredState:              runtimecontract.ConvergenceStateNeedStateChange,
		RequiredEffectScopes:       []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		RequiredEvidenceKinds:      []string{runtimecontract.EvidenceKindStateChange},
		AllowedOperationKinds:      []string{runtimecontract.RuntimeBlockKindPatchApply},
		CoverageWorkItemIDs:        []common.CoverageWorkItemID{"cwi-1"},
		TargetRefs:                 []string{"main.go"},
		SupersedesEvidenceRevision: facts.EvidenceRevision,
	}

	previous := facts.Coverage[0].CurrentBindingRef.Clone()
	facts.Coverage[0].BindingRetirementLineage = append(facts.Coverage[0].BindingRetirementLineage, runtimecontract.RuntimeCWIBindingRetirementLineage{
		RetiredBindingRef: previous,
		CausationRef:      "test/no-effect-binding-retirement",
		Reason:            "test binding was never executed",
	})
	facts.Coverage[0].CurrentBindingRef = nil
	facts.Coverage[0].CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStatePlanning
	retired, err := replaceRuntimeCWIFocusProjection(facts, []common.CoverageWorkItemID{"cwi-1"})
	if err != nil {
		t.Fatalf("retire current semantic binding projection: %v", err)
	}
	retired.ReconciliationContract = runtimecontract.RuntimeCWIReconciliationContract{}
	if err := runtimecontract.ValidateRuntimeCWILedger(retired); err != nil {
		t.Fatalf("validate explicit no-effect retirement: %v", err)
	}
	assertRuntimeRepairAuthorityRebound(t, retired, "semantic-pending/cwi-1")

	newFocus := oldFocus
	newFocus.ID = "focus-1-resealed"
	newBinding := oldFocus.CoverageEvidenceBindings[0].Clone()
	newBinding.ID = "binding/test/runtime/focus-1-resealed/cwi-1"
	newBinding.FocusID = newFocus.ID
	newBinding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(newBinding)
	newFocus.CoverageEvidenceBindings = []runtimecontract.CoverageEvidenceBinding{newBinding}

	resealed, err := sealRuntimeCWIBindings(retired, "semantic-manifest-resealed", []runtimecontract.CoverageFocus{newFocus}, nil)
	if err != nil {
		t.Fatalf("re-seal current semantic binding: %v", err)
	}
	reconciliation, err := canonicalRuntimeCWIReconciliationContract(resealed)
	if err != nil {
		t.Fatalf("compile re-sealed canonical reconciliation contract: %v", err)
	}
	resealed.ReconciliationContract = reconciliation
	if err := runtimecontract.ValidateRuntimeCWILedger(resealed); err != nil {
		t.Fatalf("validate re-sealed CWI projection: %v", err)
	}
	assertRuntimeRepairAuthorityRebound(t, resealed, newFocus.ID)

	current := resealed.Coverage[0]
	if current.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound || current.CurrentBindingRef == nil ||
		current.CurrentBindingRef.FocusID != newFocus.ID || len(current.BindingLineage) != 2 {
		t.Fatalf("re-sealed canonical CWI row = %+v", current)
	}
}

func assertRuntimeRepairAuthorityRebound(t *testing.T, facts runtimecontract.RuntimeFacts, focusID string) {
	t.Helper()
	ledger := facts.Repair
	if facts.Focus.ID != focusID || ledger.FocusID != focusID || ledger.SliceID != facts.Focus.SliceID ||
		ledger.RepairAttempts != 0 || ledger.ValidationOnlyRetries != 0 || ledger.NoProgressCount != 0 ||
		len(ledger.SignatureCounts) != 0 || ledger.LastCycleID != "" || ledger.ActiveGap != nil || ledger.Exhausted ||
		facts.RepairDirective != nil {
		t.Fatalf("focus projection retained predecessor repair authority: facts=%+v", facts)
	}
}

func TestEligibleFocusProjectionSelectsLaterEligibleCWIAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repository := &controllerRepository{}
	controller, err := NewRuntimeController(repository)
	if err != nil {
		t.Fatal(err)
	}
	facts := controllerFacts()
	blocked := facts.Focus.Normalized()
	blocked.ID = "focus-blocked"
	blocked.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-v2"}
	later := blocked
	later.ID = "focus-later"
	later.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-later"}
	gate := blocked
	gate.ID = "focus-gate"
	gate.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-gate"}
	facts.Focus = blocked
	facts.RemainingFocuses = []runtimecontract.CoverageFocus{later, gate}
	facts.Coverage[0].PrerequisiteCoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-gate"}
	facts.Coverage = append(facts.Coverage,
		runtimecontract.RuntimeCoverageItemFact{ID: "cwi-later", SliceID: "slice-v2", SourceScope: "dodItem", SourceID: "dod-later", TargetText: "later", State: runtimecontract.RuntimeCoverageItemStateOpen},
		runtimecontract.RuntimeCoverageItemFact{ID: "cwi-gate", SliceID: "slice-v2", SourceScope: "dodItem", SourceID: "dod-gate", TargetText: "gate", State: runtimecontract.RuntimeCoverageItemStateOpen},
	)
	facts.DefinitionSeed.CoverageItems[0].PrerequisiteCoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-gate"}
	facts.DefinitionSeed.CoverageItems = append(facts.DefinitionSeed.CoverageItems,
		runtimecontract.RuntimeCoverageItem{ID: "cwi-later", SliceID: "slice-v2", SourceScope: "dodItem", SourceID: "dod-later", TargetText: "later", Order: 2},
		runtimecontract.RuntimeCoverageItem{ID: "cwi-gate", SliceID: "slice-v2", SourceScope: "dodItem", SourceID: "dod-gate", TargetText: "gate", Order: 3})
	facts.Slices[0].CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-v2", "cwi-later", "cwi-gate"}
	facts.DefinitionSeed.Slices = facts.Slices
	facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	definition := runtimecontract.RuntimeDefinitionContext{
		Revision: facts.DefinitionRevision, GoalID: "goal-neutral", GoalDefinition: "complete the neutral goal",
		DoDID: "dod-neutral", DoDDefinition: "all neutral coverage is satisfied", CapabilityContract: []string{"neutral-capability"},
	}
	initialized, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventActivationObserved, WorkspaceID: facts.WorkspaceID, RunID: facts.RunID,
		ActivationFacts: &facts, ActivationDefinition: &definition,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{Kind: RuntimeInternalEventEligibleFocusProjected, WorkspaceID: facts.WorkspaceID, RunID: facts.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if result.Aggregate.Facts.Focus.CoverageWorkItemIDs[0] != "cwi-later" {
		t.Fatalf("projected focus = %+v", result.Aggregate.Facts.Focus)
	}
	if initialized.Aggregate.StateRevision >= result.Aggregate.StateRevision {
		t.Fatalf("projection did not commit a new revision: before=%d after=%d", initialized.Aggregate.StateRevision, result.Aggregate.StateRevision)
	}
	duplicate, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{Kind: RuntimeInternalEventEligibleFocusProjected, WorkspaceID: facts.WorkspaceID, RunID: facts.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Aggregate.StateRevision != result.Aggregate.StateRevision {
		t.Fatalf("duplicate projection revision = %d, want %d", duplicate.Aggregate.StateRevision, result.Aggregate.StateRevision)
	}
}

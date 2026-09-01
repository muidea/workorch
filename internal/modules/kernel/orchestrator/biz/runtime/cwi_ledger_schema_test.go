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
		t.Fatal("CWI facts without the current ledger were accepted by settlement")
	}
	if _, err := advanceRuntimeCWIFocus(facts); err == nil {
		t.Fatal("CWI facts without the current ledger were accepted by focus advance")
	}
	if _, _, err := reopenRuntimeCWIs(facts, []runtimeCWIReopenTrigger{{CoverageWorkItemID: "cwi-v2", TriggerRef: "receipt-1", Reason: "test"}}); err == nil {
		t.Fatal("CWI facts without the current ledger were accepted by reopen")
	}
}

func TestRuntimeAggregateRejectsMissingCurrentCWILedger(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.Facts.CWILedgerSchemaVersion = ""
	if err := aggregate.Validate(); err == nil || !strings.Contains(err.Error(), "canonical CWI ledger contract") {
		t.Fatalf("missing-ledger aggregate validation error = %v", err)
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

func TestValidationFailureRepairSemanticRebindingClearsExecutionAuthorityThroughFocusReducer(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	facts := aggregate.Facts
	predecessor := facts.Focus.Normalized()
	declaration := predecessor
	declaration.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	declaration.RequireAssessment = true
	declaration.TargetRole = "definition-validation-target"
	declaration.TargetSelectorRef = "profile/workspace-deliverable"
	declaration.TargetSelectorOwnerRef = "test-profile"
	declaration.TargetSelectorSubjectRef = "dodItem"
	declaration.TargetSelectorCandidateRefs = []string{"new-validation-target.go"}
	declaration.ProspectiveTargetAllOfConstraintRefs = []string{"workspace-relative-path", "new-target-requires-owner-selector"}
	declaration.RequireTargetGrounding = true
	declaration.RequireExecutableBehaviorEvidence = true

	// Model the retired acquisition generation that caused the deployed
	// regression: its all-of declaration fields were copied into the pending
	// cursor even after target/selector fields were cleared.
	predecessor.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	predecessor.RequireAssessment = false
	predecessor.TargetRefs = []string{"old-validation-target.go"}
	predecessor.ProspectiveTargetRefs = []string{"old-repair-target.go"}
	predecessor.TargetRole = "old-target-role"
	predecessor.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	predecessor.TargetSelectorRef = "old-operation-selector"
	predecessor.TargetSelectorOwnerRef = "old-owner"
	predecessor.TargetSelectorSubjectRef = "old-subject"
	predecessor.TargetSelectorCandidateRefs = []string{"old-repair-target.go"}
	predecessor.ProspectiveTargetAllOfConstraintRefs = []string{"workspace-relative-path", "new-target-requires-owner-selector"}
	predecessor.RequireTargetGrounding = true
	predecessor.RequireExecutableBehaviorEvidence = true
	predecessor.AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	predecessor.AcceptedOperationVariants = []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.RuntimeBlockKindManagedValidationSession, VariantID: "old-validation"}}
	predecessor.ValidationRecipeID = "old-validation-recipe"
	predecessor.RequiredCheckRefs = []string{"old-validation-check"}
	convergence := runtimeValidationFailureRepairConvergenceForTest(1)
	facts.Focus = predecessor
	facts.DefinitionFocuses = runtimecontract.CompileRuntimeDefinitionFocuses([]runtimecontract.CoverageFocus{declaration})
	facts.DefinitionFocuses[0].ValidationFailureRepairConvergence = &convergence
	facts.DefinitionFocuses = runtimecontract.NormalizeRuntimeDefinitionFocuses(facts.DefinitionFocuses)

	if len(facts.Coverage) != 1 || facts.Coverage[0].CurrentBindingRef == nil {
		t.Fatalf("test fixture has no current binding: %+v", facts.Coverage)
	}
	previous := facts.Coverage[0].CurrentBindingRef.Clone()
	facts.Coverage[0].ReopenLineage = append(facts.Coverage[0].ReopenLineage, runtimecontract.RuntimeCWIReopenLineage{
		PreviousBindingRef: previous,
		TriggerRef:         "validation/rejected",
		Reason:             runtimeCWIReopenReasonValidationFailureRepairRequired,
	})
	facts.Coverage[0].CurrentBindingRef = nil
	facts.Coverage[0].CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateReopened
	facts.Coverage[0].State = runtimecontract.RuntimeCoverageItemStateOpen

	projected, err := replaceRuntimeCWIFocusProjectionForSemanticRebinding(facts, []common.CoverageWorkItemID{"cwi-1"})
	if err != nil {
		t.Fatalf("project validation-failure semantic rebinding: %v", err)
	}
	got := projected.Focus
	expected := projected.DefinitionFocuses[0].RuntimeFocusSeed()
	expected.ID = "semantic-pending/cwi-1"
	expected.CoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-1"}
	expected = expected.Normalized()
	if !runtimecontract.SameRuntimeCoverageFocus(got, expected) {
		t.Fatalf("semantic rebinding projection differs from Definition seed:\n got=%+v\nwant=%+v", got, expected)
	}
	if len(got.RequiredEvidenceKinds) != 1 || got.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindValidation || !got.RequireAssessment ||
		len(got.ProspectiveTargetAllOfConstraintRefs) != 0 || got.RequireTargetGrounding ||
		len(projected.DefinitionFocuses) != 1 ||
		projected.DefinitionFocuses[0].ValidationFailureRepairConvergence == nil ||
		projected.DefinitionFocuses[0].ValidationFailureRepairConvergence.ID != convergence.ID {
		t.Fatalf("semantic rebinding did not restore the minimal terminal cursor: focus=%+v definitions=%+v", got, projected.DefinitionFocuses)
	}
	definition := projected.DefinitionFocuses[0]
	if !definition.RequireTargetGrounding || !definition.RequireExecutableBehaviorEvidence ||
		!runtimecontract.SameRuntimeProspectiveTargetAllOfConstraintRefs(
			definition.ProspectiveTargetAllOfConstraintRefs,
			[]string{"new-target-requires-owner-selector", "workspace-relative-path"},
		) {
		t.Fatalf("semantic rebinding changed immutable Definition all-of declaration: %+v", definition)
	}
	if err := validateRuntimeCWISemanticRebindingFocusProjections(projected); err != nil {
		t.Fatalf("Definition-seeded semantic rebinding projection failed owner invariant: %v", err)
	}
	leaked := projected
	leaked.Focus = runtimeCWIPendingFocus(predecessor, "cwi-1")
	if err := validateRuntimeCWISemanticRebindingFocusProjections(leaked); err == nil ||
		!strings.Contains(err.Error(), "differs from immutable Definition seed") {
		t.Fatalf("semantic rebinding owner invariant accepted predecessor authority: %v", err)
	}

	missingDefinition := facts
	missingDefinition.DefinitionFocuses = nil
	if _, err := replaceRuntimeCWIFocusProjectionForSemanticRebinding(missingDefinition, []common.CoverageWorkItemID{"cwi-1"}); err == nil ||
		!strings.Contains(err.Error(), "has no immutable Definition focus") {
		t.Fatalf("semantic rebinding without Definition focus error = %v", err)
	}
	duplicateDefinition := facts
	duplicateDefinition.DefinitionFocuses = append(
		runtimecontract.CloneRuntimeDefinitionFocuses(facts.DefinitionFocuses),
		facts.DefinitionFocuses[0].Clone(),
	)
	if _, err := replaceRuntimeCWIFocusProjectionForSemanticRebinding(duplicateDefinition, []common.CoverageWorkItemID{"cwi-1"}); err == nil ||
		!strings.Contains(err.Error(), "multiple Definition focus declarations") {
		t.Fatalf("semantic rebinding with duplicate Definition focus error = %v", err)
	}
}

func TestSemanticCWISealRejectsReactivationOfReopenedBinding(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	facts := aggregate.Facts
	oldFocus := facts.Focus.Normalized()
	previous := facts.Coverage[0].CurrentBindingRef.Clone()
	facts.Coverage[0].ReopenLineage = append(facts.Coverage[0].ReopenLineage, runtimecontract.RuntimeCWIReopenLineage{
		PreviousBindingRef: previous,
		TriggerRef:         "assessment/owner-fact-open",
		Reason:             runtimeCWIReopenReasonOwnerFactPostcondition,
	})
	facts.Coverage[0].CurrentBindingRef = nil
	facts.Coverage[0].CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateReopened
	facts.Coverage[0].State = runtimecontract.RuntimeCoverageItemStateOpen

	pending, err := replaceRuntimeCWIFocusProjection(facts, []common.CoverageWorkItemID{"cwi-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sealRuntimeCWIBindings(pending, previous.SemanticManifestHash, []runtimecontract.CoverageFocus{oldFocus}, nil); err == nil ||
		!strings.Contains(err.Error(), "cannot reactivate historical binding") {
		t.Fatalf("reopened binding reactivation error = %v", err)
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
	facts.DefinitionFocuses = runtimeTestDefinitionFocuses(facts)
	definition := runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test",
		Revision: facts.DefinitionRevision, GoalID: "goal-neutral", GoalDefinition: "complete the neutral goal",
		DoDID: "dod-neutral", DoDDefinition: "all neutral coverage is satisfied", CapabilityContract: []string{"neutral-capability"},
	}
	initialized, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{
		Kind: RuntimeInternalEventActivationObserved, Scope: RuntimeInternalEventScope{WorkspaceID: facts.WorkspaceID, RunID: facts.RunID}, Activation: &RuntimeInternalActivationPayload{ActivationFacts: &facts, ActivationDefinition: &definition},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{Kind: RuntimeInternalEventEligibleFocusProjected, Scope: RuntimeInternalEventScope{WorkspaceID: facts.WorkspaceID, RunID: facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Aggregate.Facts.Focus.CoverageWorkItemIDs[0] != "cwi-later" {
		t.Fatalf("projected focus = %+v", result.Aggregate.Facts.Focus)
	}
	if initialized.Aggregate.StateRevision >= result.Aggregate.StateRevision {
		t.Fatalf("projection did not commit a new revision: before=%d after=%d", initialized.Aggregate.StateRevision, result.Aggregate.StateRevision)
	}
	duplicate, err := controller.ApplyInternalEvent(ctx, RuntimeInternalEvent{Kind: RuntimeInternalEventEligibleFocusProjected, Scope: RuntimeInternalEventScope{WorkspaceID: facts.WorkspaceID, RunID: facts.RunID}, Authority: &RuntimeInternalAuthorityPayload{}})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Aggregate.StateRevision != result.Aggregate.StateRevision {
		t.Fatalf("duplicate projection revision = %d, want %d", duplicate.Aggregate.StateRevision, result.Aggregate.StateRevision)
	}
}

func TestCurrentRuntimeCWIOrderFromSemanticLineageDropsOnlySettledPredecessor(t *testing.T) {
	settledRef := runtimecontract.RuntimeCWIBindingRef{
		BindingID: "binding-settled", BindingHash: "hash-settled", FocusID: "focus-settled", SemanticManifestHash: "manifest-order",
	}
	facts := runtimecontract.RuntimeFacts{
		DefinitionSeed: runtimecontract.RuntimeDefinitionSeed{CoverageItems: []runtimecontract.RuntimeCoverageItem{
			{ID: "cwi-settled", Order: 1}, {ID: "cwi-next", Order: 2}, {ID: "cwi-blocked", Order: 3},
		}},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{
			{ID: "cwi-settled", State: runtimecontract.RuntimeCoverageItemStateSatisfied, CWIConvergenceState: runtimecontract.RuntimeCWIConvergenceStateSettled, CurrentBindingRef: &settledRef},
			{ID: "cwi-next", State: runtimecontract.RuntimeCoverageItemStateOpen, CWIConvergenceState: runtimecontract.RuntimeCWIConvergenceStatePlanning},
			{ID: "cwi-blocked", State: runtimecontract.RuntimeCoverageItemStateOpen, CWIConvergenceState: runtimecontract.RuntimeCWIConvergenceStatePlanning, PrerequisiteCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-next"}},
		},
	}

	manifest := runtimecontract.RuntimeSemanticPlanManifest{Hash: "manifest-order"}
	order, changed, err := currentRuntimeCWIOrderFromSemanticLineage(facts, manifest, []common.CoverageWorkItemID{"cwi-settled", "cwi-next"})
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 1 || order[0] != "cwi-next" || len(changed) != 0 {
		t.Fatalf("current semantic order = %+v, want only cwi-next", order)
	}

	if _, _, err := currentRuntimeCWIOrderFromSemanticLineage(facts, manifest, []common.CoverageWorkItemID{"cwi-settled", "cwi-blocked", "cwi-next"}); err == nil || !strings.Contains(err.Error(), "non-settled ineligible") {
		t.Fatalf("blocked historical order error = %v", err)
	}
	_, changed, err = currentRuntimeCWIOrderFromSemanticLineage(facts, manifest, []common.CoverageWorkItemID{"cwi-settled"})
	if err != nil || len(changed) != 1 || changed[0] != "cwi-next" {
		t.Fatalf("new eligible closure change = %+v, error = %v", changed, err)
	}
}

func TestCurrentRuntimeCWIOrderFromSemanticLineageScopesValidationRepairGeneration(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		DefinitionSeed: runtimecontract.RuntimeDefinitionSeed{CoverageItems: []runtimecontract.RuntimeCoverageItem{
			{ID: "cwi-001", Order: 1}, {ID: "cwi-002", Order: 2}, {ID: "cwi-003", Order: 3},
		}},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{
			{ID: "cwi-001", State: runtimecontract.RuntimeCoverageItemStateOpen, CWIConvergenceState: runtimecontract.RuntimeCWIConvergenceStateReopened},
			{ID: "cwi-002", State: runtimecontract.RuntimeCoverageItemStateOpen, CWIConvergenceState: runtimecontract.RuntimeCWIConvergenceStatePlanning},
			{ID: "cwi-003", State: runtimecontract.RuntimeCoverageItemStateOpen, CWIConvergenceState: runtimecontract.RuntimeCWIConvergenceStatePlanning},
		},
	}
	repairManifest := runtimecontract.RuntimeSemanticPlanManifest{
		Hash:          "manifest-repair-cwi-001",
		CoverageItems: []runtimecontract.RuntimeCoverageItem{{ID: "cwi-001"}},
		ProofObligations: []runtimecontract.RuntimeSemanticProofObligation{{
			CoverageWorkItemID: "cwi-001", ValidationFailureRepair: &runtimecontract.RuntimeSemanticValidationFailureRepair{FailureSignature: "validation-failure-001"},
		}},
	}
	order, changed, err := currentRuntimeCWIOrderFromSemanticLineage(facts, repairManifest, []common.CoverageWorkItemID{"cwi-001"})
	if err != nil || len(order) != 1 || order[0] != "cwi-001" || len(changed) != 0 {
		t.Fatalf("repair generation closure = order=%v changed=%v err=%v", order, changed, err)
	}

	// The same single-CWI manifest shape without a repair obligation is a
	// normal Definition generation. It must see concurrently eligible rows as
	// a real closure change instead of silently narrowing global authority.
	normalManifest := runtimecontract.RuntimeSemanticPlanManifest{Hash: "manifest-normal-cwi-001", CoverageItems: []runtimecontract.RuntimeCoverageItem{{ID: "cwi-001"}}}
	_, changed, err = currentRuntimeCWIOrderFromSemanticLineage(facts, normalManifest, []common.CoverageWorkItemID{"cwi-001"})
	if err != nil || !sameCoverageIDs(changed, []common.CoverageWorkItemID{"cwi-002", "cwi-003"}) {
		t.Fatalf("normal generation closure change = %v err=%v", changed, err)
	}

	if _, _, err := currentRuntimeCWIOrderFromSemanticLineage(facts, repairManifest, []common.CoverageWorkItemID{"cwi-001", "cwi-001"}); err == nil || !strings.Contains(err.Error(), "duplicates coverage") {
		t.Fatalf("duplicate repair semantic order error = %v", err)
	}
}

func TestCurrentRuntimeCWIOrderFromSemanticLineageRetainsCurrentGenerationBoundQueueMember(t *testing.T) {
	oldBinding := runtimecontract.CoverageEvidenceBinding{
		ID: "binding-current", CoverageWorkItemID: "cwi-current", FocusID: "focus-current", DefinitionRevision: "definition-1",
		Proofs:     []runtimecontract.CoverageEvidenceProof{{EvidenceKind: runtimecontract.EvidenceKindValidation, AllowedOperationKinds: []string{runtimecontract.StepKindMCPToolCall}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}, ValidationRecipeID: "recipe/current", RequiredCheckRefs: []string{"recipe/current"}}},
		TargetRefs: []string{"target-current"}, DeclarationRef: "dod/current",
	}.Normalized()
	oldBinding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(oldBinding)
	reboundBinding := runtimecontract.CoverageEvidenceBinding{
		ID: "binding-rebound", CoverageWorkItemID: "cwi-rebound", FocusID: "focus-rebound", DefinitionRevision: "definition-1",
		Proofs:     []runtimecontract.CoverageEvidenceProof{{EvidenceKind: runtimecontract.EvidenceKindValidation, AllowedOperationKinds: []string{runtimecontract.StepKindMCPToolCall}, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone}, ValidationRecipeID: "recipe/rebound", RequiredCheckRefs: []string{"recipe/rebound"}}},
		TargetRefs: []string{"target-rebound"}, DeclarationRef: "dod/rebound",
	}.Normalized()
	reboundBinding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(reboundBinding)
	oldRef := runtimecontract.RuntimeCWIBindingRefForCoverageBinding(oldBinding, "manifest-previous")
	reboundRef := runtimecontract.RuntimeCWIBindingRefForCoverageBinding(reboundBinding, "manifest-current")
	facts := runtimecontract.RuntimeFacts{
		DefinitionSeed:   runtimecontract.RuntimeDefinitionSeed{CoverageItems: []runtimecontract.RuntimeCoverageItem{{ID: "cwi-current", Order: 1}, {ID: "cwi-rebound", Order: 2}}},
		Focus:            runtimecontract.CoverageFocus{ID: oldBinding.FocusID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-current"}, CoverageEvidenceBindings: []runtimecontract.CoverageEvidenceBinding{oldBinding}},
		RemainingFocuses: []runtimecontract.CoverageFocus{{ID: reboundBinding.FocusID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-rebound"}, CoverageEvidenceBindings: []runtimecontract.CoverageEvidenceBinding{reboundBinding}}},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{
			{ID: "cwi-current", State: runtimecontract.RuntimeCoverageItemStateOpen, CWIConvergenceState: runtimecontract.RuntimeCWIConvergenceStateBound, CurrentBindingRef: &oldRef},
			{ID: "cwi-rebound", State: runtimecontract.RuntimeCoverageItemStateOpen, CWIConvergenceState: runtimecontract.RuntimeCWIConvergenceStateBound, CurrentBindingRef: &reboundRef},
		},
	}

	currentManifest := runtimecontract.RuntimeSemanticPlanManifest{Hash: "manifest-current"}
	order, changed, err := currentRuntimeCWIOrderFromSemanticLineage(facts, currentManifest, []common.CoverageWorkItemID{"cwi-rebound"})
	if err != nil || len(order) != 1 || order[0] != "cwi-rebound" || len(changed) != 0 {
		t.Fatalf("current-generation bound lineage = %+v, changed=%+v, error=%v", order, changed, err)
	}
	if pending := runtimeCWIPlanningMembersOfSemanticOrder(facts, order); len(pending) != 0 {
		t.Fatalf("already-bound semantic order leaked into planning projection: %+v", pending)
	}
	if _, _, err := currentRuntimeCWIOrderFromSemanticLineage(facts, runtimecontract.RuntimeSemanticPlanManifest{Hash: "manifest-stale"}, []common.CoverageWorkItemID{"cwi-rebound"}); err == nil || !strings.Contains(err.Error(), "non-settled ineligible") {
		t.Fatalf("stale bound semantic lineage error = %v", err)
	}
	drifted := facts
	drifted.RemainingFocuses = append([]runtimecontract.CoverageFocus(nil), facts.RemainingFocuses...)
	driftedFocus := drifted.RemainingFocuses[0]
	driftedFocus.CoverageEvidenceBindings = append([]runtimecontract.CoverageEvidenceBinding(nil), driftedFocus.CoverageEvidenceBindings...)
	driftedFocus.CoverageEvidenceBindings[0].TargetRefs = []string{"target-drift"}
	drifted.RemainingFocuses[0] = driftedFocus
	if _, _, err := currentRuntimeCWIOrderFromSemanticLineage(drifted, currentManifest, []common.CoverageWorkItemID{"cwi-rebound"}); err == nil || !strings.Contains(err.Error(), "non-settled ineligible") {
		t.Fatalf("drifted bound semantic lineage error = %v", err)
	}
	completed := facts
	completed.RemainingFocuses = nil
	completed.CompletedFocuses = []runtimecontract.CoverageFocus{facts.RemainingFocuses[0]}
	if _, _, err := currentRuntimeCWIOrderFromSemanticLineage(completed, currentManifest, []common.CoverageWorkItemID{"cwi-rebound"}); err == nil || !strings.Contains(err.Error(), "non-settled ineligible") {
		t.Fatalf("completed bound semantic lineage error = %v", err)
	}
	blocked := facts
	blocked.Coverage = append([]runtimecontract.RuntimeCoverageItemFact(nil), facts.Coverage...)
	blocked.Coverage[1].PrerequisiteCoverageWorkItemIDs = []common.CoverageWorkItemID{"cwi-current"}
	if _, _, err := currentRuntimeCWIOrderFromSemanticLineage(blocked, currentManifest, []common.CoverageWorkItemID{"cwi-rebound"}); err == nil || !strings.Contains(err.Error(), "non-settled ineligible") {
		t.Fatalf("bound lineage with reopened prerequisite error = %v", err)
	}
}

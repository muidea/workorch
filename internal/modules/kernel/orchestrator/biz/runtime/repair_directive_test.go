package runtime

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRepairDirectiveSurvivesPrerequisiteReacquisition(t *testing.T) {
	// Mirrors an owner-grounded implementation failure: SM-2 selects a
	// state-change repair and a later prerequisite interlude must not re-open
	// validation.
	facts := validationFacts()
	facts.Focus.TargetRefs = []string{"lsp_manager.go"}
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure,
		Code:  "validation_target_missing", Signature: "missing-test-target",
		Message: "implementation target is not executable", TargetRefs: []string{"lsp_manager.go"},
	}
	facts.Resources.FailureTarget = &runtimecontract.FailureTarget{Signature: "missing-test-target", TargetRefs: []string{"lsp_manager.go"}}

	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedStateChange || facts.RepairDirective == nil {
		t.Fatalf("initial repair = decision=%+v directive=%+v", decision, facts.RepairDirective)
	}

	facts = PreflightRuntimeFacts(facts, []runtimecontract.RuntimeRequirement{{
		Kind: runtimecontract.PrerequisiteKindTargetSnapshot, SubjectRef: "lsp_manager.go", Reason: "repair requires a current target snapshot",
	}})
	facts, decision, err = ReduceRuntimeStateWithDirective(facts, &decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedPrerequisite {
		t.Fatalf("prerequisite decision = %+v", decision)
	}
	if facts.RepairDirective == nil || facts.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("prerequisite acquisition lost state-change repair directive: %+v", facts.RepairDirective)
	}

	prerequisiteID := facts.Prerequisites.Pending[0].ID
	facts, err = CommitPrerequisiteResult(facts, PrerequisiteResult{
		PrerequisiteID: prerequisiteID,
		TargetSnapshot: &runtimecontract.TargetSnapshot{
			TargetRef: "lsp_manager.go", Content: "package main\n", ContentHash: "snapshot-2", ContentState: runtimecontract.SnapshotContentStateComplete,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	facts, decision, err = ReduceRuntimeStateWithDirective(facts, &decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("post-prerequisite repair action = %s, want produce_state_change", decision.State)
	}
	if facts.RepairDirective == nil || facts.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("post-prerequisite directive = %+v", facts.RepairDirective)
	}
}

func TestRepairDirectiveRequiresExactCreditedEvidence(t *testing.T) {
	facts := validationFacts()
	facts = runtimeTestFactsWithCurrentFocusBindings(t, facts, runtimecontract.RuntimeDefinitionContext{
		StartupInputHash: "startup-input-test",
		Revision:         facts.DefinitionRevision,
	})
	directive := runtimecontract.RuntimeRepairDirective{
		ID: "repair-validation-credit", FocusID: facts.Focus.ID,
		RequiredState:         runtimecontract.ConvergenceStateNeedValidation,
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		CoverageWorkItemIDs:   append([]common.CoverageWorkItemID(nil), facts.Focus.CoverageWorkItemIDs...),
	}
	fact := runtimecontract.EvidenceFact{
		ID: "accepted-diagnostic-validation", Kind: runtimecontract.EvidenceKindValidation,
		Revision:    "runtime-evidence-00000000000000000001",
		BindingRefs: []string{facts.Focus.CoverageEvidenceBindings[0].ID},
	}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{fact}
	if repairDirectiveEvidenceSatisfied(facts, directive) {
		t.Fatal("empty-credit accepted diagnostic closed the repair directive")
	}
	facts.Evidence.Accepted[0].CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), facts.Focus.CoverageWorkItemIDs...)
	if !repairDirectiveEvidenceSatisfied(facts, directive) {
		t.Fatal("exact binding-derived CWI evidence did not close the repair directive")
	}
}

func TestRepairDirectiveFlipsToValidationAfterAcceptedStateChangeEvenWhenAssessmentIsRequired(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireAssessment = true
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	facts = runtimeTestFactsWithCurrentFocusBindings(t, facts, runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: facts.DefinitionRevision})
	facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{
		ID: "repair-cwi-1-produce_state_change-0", FocusID: facts.Focus.ID,
		Trigger:                    runtimecontract.RuntimeRepairTriggerCandidateInfeasible,
		RequiredState:              runtimecontract.ConvergenceStateNeedStateChange,
		RequiredEffectScopes:       []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		RequiredEvidenceKinds:      []string{runtimecontract.EvidenceKindStateChange},
		AllowedOperationKinds:      []string{runtimecontract.RuntimeBlockKindPatchApply},
		CoverageWorkItemIDs:        facts.Focus.CoverageWorkItemIDs,
		SupersedesEvidenceRevision: "runtime-evidence-00000000000000000002",
	}
	// A rejected state-change does not complete or flip the repair continuation.
	facts.Evidence.Rejected = []runtimecontract.EvidenceFact{{
		ID: "rejected-patch", Kind: runtimecontract.EvidenceKindStateChange,
		Revision: "runtime-evidence-00000000000000000003", FailureClass: runtimecontract.FailureClassEffectMismatch,
		FailureMessage: "hunk context mismatch", CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
		FocusID: facts.Focus.ID, BindingRefs: []string{facts.Focus.CoverageEvidenceBindings[0].ID}, TargetRefs: append([]string(nil), facts.Focus.TargetRefs...),
	}}
	facts.EvidenceRevision = "runtime-evidence-00000000000000000003"
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("rejected repair action = %s", decision.State)
	}

	facts.Evidence.Accepted = append(facts.Evidence.Accepted, runtimecontract.EvidenceFact{
		ID: "accepted-repair", Kind: runtimecontract.EvidenceKindStateChange,
		Revision: "runtime-evidence-00000000000000000004", CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
		FocusID: facts.Focus.ID, BindingRefs: []string{facts.Focus.CoverageEvidenceBindings[0].ID}, TargetRefs: append([]string(nil), facts.Focus.TargetRefs...),
	})
	facts.EvidenceRevision = "runtime-evidence-00000000000000000004"
	facts, decision, err = ReduceRuntimeStateWithDirective(facts, &decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedValidation || facts.RepairDirective == nil || facts.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("accepted repair did not advance to validation: decision=%+v directive=%+v", decision, facts.RepairDirective)
	}
}

func TestRepairValidationDirectiveWithoutRecipeProducesTypedTerminal(t *testing.T) {
	facts := validationFacts()
	facts.Capabilities.ValidationKinds = nil
	facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{
		ID: "repair-cwi-1-produce_validation-0", FocusID: facts.Focus.ID,
		Trigger:                    runtimecontract.RuntimeRepairTriggerAssessmentGap,
		RequiredState:              runtimecontract.ConvergenceStateNeedValidation,
		RequiredEffectScopes:       []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
		RequiredEvidenceKinds:      []string{runtimecontract.EvidenceKindValidation},
		CoverageWorkItemIDs:        facts.Focus.CoverageWorkItemIDs,
		SupersedesEvidenceRevision: facts.EvidenceRevision,
	}

	updated, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateFailed || decision.Terminal == nil || decision.Terminal.Code != "validation_recipe_exhausted" {
		t.Fatalf("missing validation recipe must be a typed SM-2 terminal: decision=%+v", decision)
	}
	if updated.RepairDirective != nil {
		t.Fatalf("terminal repair must not retain executable directive: %+v", updated.RepairDirective)
	}
}

func TestRepairDirectiveDoesNotFlipForAcceptedStateChangeOutsideGroundedTarget(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	facts.Focus.TargetRefs = []string{"implementation.go"}
	facts = runtimeTestFactsWithCurrentFocusBindings(t, facts, runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: facts.DefinitionRevision})
	facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{
		ID: "repair-cwi-1-produce_state_change-0", FocusID: facts.Focus.ID,
		Trigger:               runtimecontract.RuntimeRepairTriggerRejectedValidation,
		RequiredState:         runtimecontract.ConvergenceStateNeedStateChange,
		RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindPatchApply}, CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
		TargetRefs: []string{"implementation.go"}, SupersedesEvidenceRevision: "runtime-evidence-00000000000000000002",
	}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "accepted-validation-entry-edit", Kind: runtimecontract.EvidenceKindStateChange,
		Revision: "runtime-evidence-00000000000000000003", TargetRefs: []string{"entry_test.go"}, CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
		FocusID: facts.Focus.ID, BindingRefs: []string{facts.Focus.CoverageEvidenceBindings[0].ID},
	}}
	facts.EvidenceRevision = "runtime-evidence-00000000000000000003"
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedStateChange || facts.RepairDirective == nil || facts.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("outside-target state change advanced repair: decision=%+v directive=%+v", decision, facts.RepairDirective)
	}

	facts.Evidence.Accepted = append(facts.Evidence.Accepted, runtimecontract.EvidenceFact{
		ID: "accepted-implementation-edit", Kind: runtimecontract.EvidenceKindStateChange,
		Revision: "runtime-evidence-00000000000000000004", TargetRefs: []string{"implementation.go"}, CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
		FocusID: facts.Focus.ID, BindingRefs: []string{facts.Focus.CoverageEvidenceBindings[0].ID},
	})
	facts.EvidenceRevision = "runtime-evidence-00000000000000000004"
	facts, decision, err = ReduceRuntimeStateWithDirective(facts, &decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedValidation || facts.RepairDirective == nil || facts.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("grounded state change did not advance repair: decision=%+v directive=%+v", decision, facts.RepairDirective)
	}
}

func TestBuildRepairDirectiveDoesNotPromoteOwnerScopeToConcreteRepairTarget(t *testing.T) {
	facts := validationFacts()
	facts.Focus.TargetRefs = nil
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-scope", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "implementation", SelectorRef: "file/workspace_deliverable",
		Disposition: runtimecontract.RuntimeTargetSelectionScope,
		TargetRefs:  []string{"implementation.go", "support.go"}, CandidateTargetRefs: []string{"implementation.go", "support.go"},
		CandidateCount: 2, ObservedRevision: "workspace-tree-v1",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	facts.Resources.TargetSelection = &selection
	bindCurrentFocusTargetSelection(&facts)
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassEffectMismatch, Code: "operation_effect_mismatch",
		Signature: "effect-mismatch", Message: "durable effect not observed",
	}

	decision := runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:         []string{runtimecontract.RuntimeBlockKindPatchApply},
			RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			CoverageWorkItemIDs:   facts.Focus.CoverageWorkItemIDs, TargetRefs: selection.TargetRefs,
		},
	}
	directive := buildRepairDirective(facts, decision)
	if len(directive.TargetRefs) != 0 {
		t.Fatalf("owner scope became a concrete repair target: %+v", directive)
	}
	if directive.CurrentFailure == nil || directive.CurrentFailure.Signature != "effect-mismatch" {
		t.Fatalf("effect mismatch lineage was not retained: %+v", directive)
	}
}

func TestFailureWithoutConcreteTargetDoesNotFallbackToHistoricalSelection(t *testing.T) {
	facts := validationFacts()
	facts.Focus.TargetRefs = nil
	facts.Focus.TargetRole = "implementation"
	facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	facts.Focus.TargetSelectorRef = "file/profile_declared_candidates"
	facts.Focus.TargetSelectorOwnerRef = "profile-bootstrap"
	facts.Focus.TargetSelectorSubjectRef = "manager"
	facts.Focus.TargetSelectorCandidateRefs = []string{"internal/lsp/manager.go"}
	facts.Focus.RequireTargetGrounding = true
	// No concrete target survived the rejected validation receipt. The old
	// selection is a predecessor Focus fact, not a valid repair fallback.
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Code: "validation_failed",
		Signature: "validation-failed", Message: "validation did not identify a grounded repair target",
	}
	historicalSelection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "selection-old-go-mod", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "module_definition", SelectorRef: "file/profile_declared_candidates",
		Disposition: runtimecontract.RuntimeTargetSelectionSelected, TargetRefs: []string{"go.mod"},
		CandidateTargetRefs: []string{"go.mod"}, CandidateCount: 1, ObservedRevision: "tree-old",
	}
	historicalSelection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(historicalSelection)
	facts.Resources.TargetSelection = &historicalSelection
	facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{
		TargetRef: "go.mod", ContentHash: "go-mod-old", ContentState: runtimecontract.SnapshotContentStateComplete,
	}}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{{
		ID: historicalSelection.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindTargetSelection,
		Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: "focus-module", OwnerModuleID: runtimecontract.RuntimeModuleFile,
	}}

	updated, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || updated.Resources.TargetSelection != nil ||
		len(updated.Resources.TargetSnapshots) != 0 || updated.Resources.FailureTarget != nil ||
		len(updated.Failure.TargetRefs) != 0 || updated.RepairDirective != nil || len(GroundedRepairTargets(updated)) != 0 {
		t.Fatalf("historical selection leaked into nil-target repair authority: facts=%+v decision=%+v", updated, decision)
	}
	if len(updated.Prerequisites.Retired) != 1 || updated.Prerequisites.Retired[0].ID != historicalSelection.PrerequisiteID {
		t.Fatalf("historical selection was not retired: %+v", updated.Prerequisites)
	}
	for _, prerequisite := range updated.Prerequisites.Pending {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSelection && prerequisite.FocusID == facts.Focus.ID &&
			len(prerequisite.TargetSelectorCandidateRefs) == 1 && prerequisite.TargetSelectorCandidateRefs[0] == "internal/lsp/manager.go" {
			return
		}
	}
	t.Fatalf("current focus selection was not reacquired: %+v", updated.Prerequisites.Pending)
}

func TestRepairDirectiveStateChangeEvidenceCanAdvanceBeforeRequiredAssessment(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireAssessment = true
	facts.Focus.TargetRefs = []string{"implementation.go"}
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{
		ID: "repair-focus-1-produce_state_change-0", FocusID: facts.Focus.ID,
		Trigger: runtimecontract.RuntimeRepairTriggerAssessmentGap, RequiredState: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		TargetRefs: []string{"implementation.go"}, SupersedesEvidenceRevision: facts.EvidenceRevision,
	}
	facts.Evidence.Accepted = append(facts.Evidence.Accepted, runtimecontract.EvidenceFact{
		ID: "changed-target", Kind: runtimecontract.EvidenceKindStateChange, Revision: "runtime-evidence-00000000000000000001", TargetRefs: []string{"implementation.go"},
		CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	})
	facts = repairDirectiveFactsWithCanonicalCWI(t, facts)
	updated := syncRepairDirective(facts, runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedAssessment, Focus: facts.Focus})
	if updated.RepairDirective == nil || updated.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedValidation ||
		updated.RepairDirective.SupersedesEvidenceRevision != "runtime-evidence-00000000000000000001" {
		t.Fatalf("accepted repair state-change must retain directive and require fresh validation: %+v", updated.RepairDirective)
	}
	if !acceptedStateChangeAfterRepairDirective(facts, facts.RepairDirective) {
		t.Fatal("accepted repair state change must advance to missing validation before assessment")
	}
}

func TestNormalStateChangeDoesNotCreateRepairDirective(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedStateChange || facts.RepairDirective != nil {
		t.Fatalf("normal state change = decision=%+v directive=%+v", decision, facts.RepairDirective)
	}
}

func TestAssessmentGapRepairClosureBindsGroundedTargetsAndFollowupValidation(t *testing.T) {
	facts := validationFacts()
	facts.Focus.TargetRefs = []string{"implementation.go", "implementation_test.go"}
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	facts.Repair = runtimecontract.RuntimeRepairLedger{
		FocusID: facts.Focus.ID,
		ActiveGap: &runtimecontract.RuntimeAssessmentGap{
			CoverageWorkItemIDs:  facts.Focus.CoverageWorkItemIDs,
			MissingEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			TargetRefs:           []string{"implementation.go", "implementation_test.go"},
			FailureClass:         runtimecontract.FailureClassCoverageAssessmentIncomplete,
			FailureCode:          "validation_evidence_insufficient",
			FailureSignature:     "assessment-gap-v1",
			Reason:               "direct validation remains missing",
		},
	}
	base := runtimecontract.RuntimeDecision{Focus: facts.Focus}
	initial := runtimecontract.RuntimeDecision{
		State: runtimecontract.ConvergenceStateNeedStateChange,
		RequiredOutput: runtimecontract.RequiredOutputContract{
			AcceptedKinds:         []string{runtimecontract.RuntimeBlockKindPatchApply},
			AcceptedEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
		},
	}
	directive := buildRepairDirective(facts, initial)
	if directive.Closure == nil || directive.Closure.ID == "" || !directive.Closure.RequireValidationAfterStateChange ||
		!sameRuntimeTargetRefs(directive.Closure.GroundedTargetRefs, []string{"implementation.go", "implementation_test.go"}) ||
		!sameRuntimeTargetRefs(directive.Closure.RequiredEvidenceKinds, []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}) {
		t.Fatalf("assessment gap did not become a grounded repair closure: %+v", directive)
	}

	decision, err := repairStateChangeDecision(base, facts, directive)
	if err != nil {
		t.Fatal(err)
	}
	if decision.RequiredOutput.RepairClosureID != directive.Closure.ID ||
		!sameRuntimeTargetRefs(decision.RequiredOutput.TargetRefs, directive.Closure.GroundedTargetRefs) {
		t.Fatalf("state-change decision did not seal repair closure: %+v", decision)
	}

	facts.RepairDirective = &directive
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "changed", Kind: runtimecontract.EvidenceKindStateChange,
		Revision: "runtime-evidence-00000000000000000002", TargetRefs: []string{"implementation.go"},
		CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	}}
	facts = repairDirectiveFactsWithCanonicalCWI(t, facts)
	next := syncRepairDirective(facts, decision)
	if next.RepairDirective == nil || next.RepairDirective.RequiredState != runtimecontract.ConvergenceStateNeedValidation ||
		next.RepairDirective.Closure == nil || next.RepairDirective.Closure.ID != directive.Closure.ID {
		t.Fatalf("accepted repair state change did not preserve closure into validation: %+v", next.RepairDirective)
	}
}

func repairDirectiveFactsWithCanonicalCWI(t *testing.T, facts runtimecontract.RuntimeFacts) runtimecontract.RuntimeFacts {
	t.Helper()
	aggregate := RuntimeAggregate{Facts: facts}
	runtimeTestSealCurrentFocusBindings(t, &aggregate)
	runtimeTestSynchronizeCanonicalCWILedger(t, &aggregate)
	return aggregate.Facts
}

func TestRejectedValidationWithNoFeasibleRecipeTerminals(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "accepted-state-change", Kind: runtimecontract.EvidenceKindStateChange,
		Revision: "runtime-evidence-00000000000000000002", CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	}}
	facts.Evidence.Rejected = []runtimecontract.EvidenceFact{{
		ID: "rejected-validation", Kind: runtimecontract.EvidenceKindValidation,
		Revision: "runtime-evidence-00000000000000000004", FailureClass: runtimecontract.FailureClassExecutionFailure,
		FailureCode: "shell_exit_nonzero", FailureMessage: "validation exited non-zero",
		CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	}}
	facts.EvidenceRevision = "runtime-evidence-00000000000000000004"
	facts.Feasibility = &runtimecontract.RuntimeFeasibilityFact{
		FocusID: facts.Focus.ID, State: runtimecontract.ConvergenceStateNeedValidation,
		RequiredOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		Reason:                 "previous validation action has no feasible operation",
	}
	facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{
		ID: "repair-cwi-1-produce_validation-1", FocusID: facts.Focus.ID,
		Trigger:                    runtimecontract.RuntimeRepairTriggerMissingEvidence,
		RequiredState:              runtimecontract.ConvergenceStateNeedValidation,
		RequiredEffectScopes:       []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
		RequiredEvidenceKinds:      []string{runtimecontract.EvidenceKindValidation},
		AllowedOperationKinds:      []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		CoverageWorkItemIDs:        facts.Focus.CoverageWorkItemIDs,
		SupersedesEvidenceRevision: "runtime-evidence-00000000000000000002",
	}

	facts, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateFailed || decision.Terminal == nil || decision.Terminal.Code != "capability_unavailable" {
		t.Fatalf("decision = %+v, want typed no-validation-recipe terminal", decision)
	}
	if facts.RepairDirective != nil && facts.RepairDirective.RequiredState == runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("validation failure fabricated state-change repair directive: %+v", facts.RepairDirective)
	}
}

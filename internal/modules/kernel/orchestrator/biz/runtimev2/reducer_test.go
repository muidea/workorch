package runtimev2

import (
	"fmt"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestReduceRuntimeStateLiveFailureFixtures(t *testing.T) {
	tests := []struct {
		name  string
		facts runtimecontract.RuntimeFacts
	}{
		{
			name: "lsp validation failure remains validation without owner repair target",
			facts: runtimecontract.RuntimeFacts{
				Activation:         runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
				WorkspaceID:        "ws-5cfc9874cebda0b8",
				RunID:              "run-006",
				DefinitionRevision: "definition-lsp",
				EvidenceRevision:   "evidence-lsp-build-failed",
				Focus: runtimecontract.CoverageFocus{
					ID:                    "cwi-036",
					CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-036"},
					RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
					TargetRefs:            []string{"server.go"},
				},
				Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-036", State: runtimecontract.RuntimeCoverageItemStateOpen}},
				Evidence: runtimecontract.EvidenceLedger{
					Accepted: []runtimecontract.EvidenceFact{{ID: "e-state", Kind: runtimecontract.EvidenceKindStateChange}},
					Rejected: []runtimecontract.EvidenceFact{{ID: "e-vet", Kind: runtimecontract.EvidenceKindValidation, FailureClass: runtimecontract.FailureClassBuildFailure, FailureMessage: "server.go:29:6: main redeclared in this block"}},
				},
				Capabilities: runtimecontract.RuntimeCapabilities{
					StateChangeKinds: []string{"patch/apply"},
					ValidationKinds:  []string{"shell/command", "validation/managed_session"},
				},
			},
		},
		{
			name: "gateway incomplete coverage requires state change",
			facts: runtimecontract.RuntimeFacts{
				Activation:         runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
				WorkspaceID:        "ws-609ce9149734ebb7",
				RunID:              "run-007-replan-001",
				DefinitionRevision: "definition-gateway",
				EvidenceRevision:   "evidence-coverage-assessment-incomplete",
				Focus: runtimecontract.CoverageFocus{
					ID:                    "cwi-085",
					CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-085"},
					RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
					TargetRefs:            []string{"gateway"},
				},
				Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-085", State: runtimecontract.RuntimeCoverageItemStateOpen}},
				Evidence: runtimecontract.EvidenceLedger{
					Accepted: []runtimecontract.EvidenceFact{{ID: "e-validation", Kind: runtimecontract.EvidenceKindValidation}},
				},
				Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassCoverageAssessmentIncomplete, Signature: "coverage_assessment_incomplete:cwi-085"},
				Capabilities: runtimecontract.RuntimeCapabilities{
					StateChangeKinds: []string{"patch/apply"},
					ValidationKinds:  []string{"shell/command"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReduceRuntimeState(tt.facts, nil)
			if err != nil {
				t.Fatalf("ReduceRuntimeState() error = %v", err)
			}
			wantAction := runtimecontract.ConvergenceStateNeedStateChange
			if tt.name == "lsp validation failure remains validation without owner repair target" {
				wantAction = runtimecontract.ConvergenceStateNeedValidation
			}
			if got.State != wantAction {
				t.Fatalf("decision = %+v, want %s", got, wantAction)
			}
			if got.AuthoritySnapshot.RequiredOutputHash == "" {
				t.Fatalf("required output is not atomically sealed: %+v", got)
			}
		})
	}
}

func TestReduceRuntimeStateIsDeterministic(t *testing.T) {
	facts := validationFacts()
	left, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	right, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if left.AuthoritySnapshot != right.AuthoritySnapshot {
		t.Fatalf("snapshot changed for identical facts: left=%+v right=%+v", left.AuthoritySnapshot, right.AuthoritySnapshot)
	}
}

func TestReduceRuntimeStateUsesSealedRepairBudgetForRepeatedStableFailure(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.RuntimeBlockKindPatchApply}
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Signature: "stable-patch-failure", Message: "patch input remains invalid", TargetRefs: []string{"main.go"},
	}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{ID: "accepted-unrelated", Kind: runtimecontract.EvidenceKindValidation, Revision: "evidence-v2-00000000000000000001"}}
	for idx := 0; idx < 2; idx++ {
		facts.Evidence.Rejected = append(facts.Evidence.Rejected, runtimecontract.EvidenceFact{
			ID: common.EvidenceRecordID("rejected-" + string(rune('a'+idx))), Kind: runtimecontract.EvidenceKindStateChange,
			FailureClass: runtimecontract.FailureClassExecutionFailure, FailureSignature: "stable-patch-failure",
			Revision:            fmt.Sprintf("evidence-v2-%020d", idx+2),
			CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
		})
	}
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("two repeated failures action = %s, want bounded retry", decision.State)
	}
	facts.Evidence.Rejected = append(facts.Evidence.Rejected, runtimecontract.EvidenceFact{
		ID: "rejected-c", Kind: runtimecontract.EvidenceKindStateChange,
		FailureClass: runtimecontract.FailureClassExecutionFailure, FailureSignature: "stable-patch-failure",
		Revision:            "evidence-v2-00000000000000000004",
		CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	})
	// Rejected evidence count is diagnostic only; the sealed repair ledger is
	// the single budget authority.
	decision, err = ReduceRuntimeState(facts, &decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("rejected count bypassed repair ledger: %+v", decision)
	}
	facts.ConvergencePolicy = runtimecontract.RuntimeConvergencePolicy{MaxRepairAttemptsPerFocus: 3}
	facts.Repair = runtimecontract.RuntimeRepairLedger{FocusID: facts.Focus.ID, RepairAttempts: 3}
	decision, err = ReduceRuntimeState(facts, &decision.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateFailed || decision.Terminal == nil || decision.Terminal.Code != "no_progress_exhausted" {
		t.Fatalf("sealed repair budget decision = %+v", decision)
	}
}

func TestReduceRuntimeStateProgression(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*runtimecontract.RuntimeFacts)
		state  runtimecontract.ConvergenceState
		action runtimecontract.ConvergenceState
	}{
		{name: "missing prerequisite", mutate: func(f *runtimecontract.RuntimeFacts) {
			f.Prerequisites.Pending = []runtimecontract.RuntimePrerequisite{{ID: "snapshot", Kind: "target_snapshot"}}
		}, state: runtimecontract.ConvergenceStateNeedPrerequisite, action: runtimecontract.ConvergenceStateNeedPrerequisite},
		{name: "validation", state: runtimecontract.ConvergenceStateNeedValidation, action: runtimecontract.ConvergenceStateNeedValidation},
		{name: "complete", mutate: func(f *runtimecontract.RuntimeFacts) {
			f.Evidence.Accepted = []runtimecontract.EvidenceFact{{ID: "e-validation", Kind: runtimecontract.EvidenceKindValidation}}
		}, state: runtimecontract.ConvergenceStateComplete, action: runtimecontract.ConvergenceStateComplete},
		{name: "governance", mutate: func(f *runtimecontract.RuntimeFacts) {
			f.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassGovernanceDependency, GovernanceDependency: &runtimecontract.ExternalDependency{
				Kind: "external-approval", SubjectRef: "subject-1", Reason: "external approval required",
			}}
			f.Capabilities.GovernanceKinds = []string{"decision/resolution"}
		}, state: runtimecontract.ConvergenceStateNeedGovernance, action: runtimecontract.ConvergenceStateNeedGovernance},
		{name: "runtime defect fails", mutate: func(f *runtimecontract.RuntimeFacts) {
			f.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassRuntimeContractMisalignment}
		}, state: runtimecontract.ConvergenceStateFailed, action: runtimecontract.ConvergenceStateFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := validationFacts()
			if tt.mutate != nil {
				tt.mutate(&facts)
			}
			got, err := ReduceRuntimeState(facts, nil)
			if err != nil {
				t.Fatalf("ReduceRuntimeState() error = %v", err)
			}
			if got.State != tt.state || got.State != tt.action {
				t.Fatalf("decision = %s/%s, want %s/%s", got.State, got.State, tt.state, tt.action)
			}
		})
	}
}

func TestReduceRuntimeStateMissingConcreteCapabilityUsesFormalDecision(t *testing.T) {
	tests := []struct {
		name       string
		governance bool
		action     runtimecontract.ConvergenceState
		code       string
	}{
		{name: "bounded terminal without governance", action: runtimecontract.ConvergenceStateFailed, code: "capability_unavailable"},
		{name: "bounded terminal with unused governance capability", governance: true, action: runtimecontract.ConvergenceStateFailed, code: "capability_unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := validationFacts()
			facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
			facts.Capabilities.StateChangeKinds = nil
			if tt.governance {
				facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindDecisionRequest}
			}
			decision, err := ReduceRuntimeState(facts, nil)
			if err != nil {
				t.Fatalf("missing capability escaped reducer as error: %v", err)
			}
			if decision.State != tt.action {
				t.Fatalf("decision = %+v, want action %s", decision, tt.action)
			}
			if tt.code != "" && (decision.Terminal == nil || decision.Terminal.Code != tt.code) {
				t.Fatalf("terminal = %+v, want code %s", decision.Terminal, tt.code)
			}
		})
	}
}

func TestReduceRuntimeStateCreationScopeRequiresAdmissibleDurableFileOperation(t *testing.T) {
	newCreationFacts := func() runtimecontract.RuntimeFacts {
		facts := validationFacts()
		facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
		facts.Focus.TargetRole = "deliverable"
		facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
		facts.Focus.TargetSelectorRef = "file/workspace_deliverable"
		facts.Focus.ProspectiveTargetRef = "initial-deliverable.txt"
		facts.Focus.RequireTargetGrounding = true
		selection := runtimecontract.RuntimeTargetSelection{
			PrerequisiteID: "selection-1", OwnerModuleID: runtimecontract.RuntimeModuleFile,
			TargetRole: facts.Focus.TargetRole, SelectorRef: facts.Focus.TargetSelectorRef,
			Disposition: runtimecontract.RuntimeTargetSelectionCreate, ProspectiveTargetRef: facts.Focus.ProspectiveTargetRef, CandidateCount: 0, ObservedRevision: "workspace-empty-v1",
		}
		selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
		facts.Resources.TargetSelection = &selection
		bindCurrentFocusTargetSelection(&facts)
		return facts
	}

	t.Run("fails before provider when only non-file durable capability is sealed", func(t *testing.T) {
		facts := newCreationFacts()
		facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindGitWrite}
		facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindGitWrite}
		decision, err := ReduceRuntimeState(facts, nil)
		if err != nil {
			t.Fatal(err)
		}
		if decision.State != runtimecontract.ConvergenceStateFailed || decision.Terminal == nil || decision.Terminal.Code != "creation_scope_file_capability_unavailable" {
			t.Fatalf("creation scope without File admission decision = %+v", decision)
		}
	})

	t.Run("permits only owner-declared create operation", func(t *testing.T) {
		facts := newCreationFacts()
		facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindPatchApply}
		facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindPatchApply}
		inventory := runtimecontract.RuntimeOperationAdmissibility{
			PrerequisiteID: "admissibility-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
			OperationKinds: []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindPatchApply}, SelectorRefs: []string{"owner-new-target-selector"},
			ObservedRevision: "workspace-empty-v1",
		}
		inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
		facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
		decision, err := ReduceRuntimeState(facts, nil)
		if err != nil {
			t.Fatal(err)
		}
		if decision.State != runtimecontract.ConvergenceStateNeedStateChange || len(decision.RequiredOutput.AcceptedKinds) != 1 || decision.RequiredOutput.AcceptedKinds[0] != runtimecontract.StepKindFileWrite {
			t.Fatalf("creation scope File admission decision = %+v", decision)
		}
	})

	t.Run("create precedes ordinary validation until durable target evidence exists", func(t *testing.T) {
		facts := newCreationFacts()
		facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
		facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
		facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindFileWrite}
		facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
		inventory := runtimecontract.RuntimeOperationAdmissibility{
			PrerequisiteID: "admissibility-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
			OperationKinds: []string{runtimecontract.StepKindFileWrite}, SelectorRefs: []string{"owner-new-target-selector"},
			ObservedRevision: "workspace-empty-v1",
		}
		inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
		facts.Resources.OperationAdmissibilities = []runtimecontract.RuntimeOperationAdmissibility{inventory}
		decision, err := ReduceRuntimeState(facts, nil)
		if err != nil {
			t.Fatal(err)
		}
		if decision.State != runtimecontract.ConvergenceStateNeedStateChange || len(decision.RequiredOutput.AcceptedKinds) != 1 || decision.RequiredOutput.AcceptedKinds[0] != runtimecontract.StepKindFileWrite || len(decision.RequiredOutput.TargetRefs) != 1 || decision.RequiredOutput.TargetRefs[0] != "initial-deliverable.txt" {
			t.Fatalf("initial creation decision = %+v", decision)
		}

		facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
			ID: "created", Kind: runtimecontract.EvidenceKindStateChange, TargetRefs: []string{"initial-deliverable.txt"},
			CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs, Revision: "evidence-v2-00000000000000000001",
		}}
		facts.Resources.TargetSelection = &runtimecontract.RuntimeTargetSelection{
			PrerequisiteID: "selection-1", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: facts.Focus.TargetRole,
			SelectorRef: facts.Focus.TargetSelectorRef, Disposition: runtimecontract.RuntimeTargetSelectionSelected,
			TargetRefs: []string{"initial-deliverable.txt"}, CandidateTargetRefs: []string{"initial-deliverable.txt"}, CandidateCount: 1, ObservedRevision: "workspace-created-v1",
		}
		facts.Resources.TargetSelection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(*facts.Resources.TargetSelection)
		bindCurrentFocusTargetSelection(&facts)
		decision, err = ReduceRuntimeState(facts, nil)
		if err != nil {
			t.Fatal(err)
		}
		if decision.State != runtimecontract.ConvergenceStateNeedValidation || len(decision.RequiredOutput.TargetRefs) != 1 || decision.RequiredOutput.TargetRefs[0] != "initial-deliverable.txt" {
			t.Fatalf("post-create validation decision = %+v", decision)
		}
	})

	t.Run("rejects create observation that does not match the sealed declaration", func(t *testing.T) {
		facts := newCreationFacts()
		facts.Resources.TargetSelection.ProspectiveTargetRef = "unsealed-target.txt"
		facts.Resources.TargetSelection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(*facts.Resources.TargetSelection)
		decision, err := ReduceRuntimeState(facts, nil)
		if err != nil {
			t.Fatal(err)
		}
		if decision.State != runtimecontract.ConvergenceStateFailed || decision.Terminal == nil || decision.Terminal.Code != "target_grounding_unresolved" {
			t.Fatalf("unsealed creation selection decision = %+v", decision)
		}
	})
}

func TestAdvanceSatisfiedCoverageFocusDropsPriorFocusTargetFacts(t *testing.T) {
	facts := validationFacts()
	facts.Focus = runtimecontract.CoverageFocus{ID: "bootstrap-module", SliceID: "bootstrap", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-module"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}}
	facts.RemainingFocuses = []runtimecontract.CoverageFocus{{
		ID: "bootstrap-source", SliceID: "bootstrap", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-source"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		TargetRole: "implementation", TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}, TargetSelectorRef: "file/profile_declared_candidates",
		TargetSelectorOwnerRef: "software-dev", TargetSelectorSubjectRef: "go-lsp/source", TargetSelectorCandidateRefs: []string{"internal/lsp/manager.go"}, ProspectiveTargetRef: "internal/lsp/manager.go", RequireTargetGrounding: true,
	}}
	facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-module", State: runtimecontract.RuntimeCoverageItemStateSatisfied}, {ID: "cwi-source", State: runtimecontract.RuntimeCoverageItemStateOpen}}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{ID: "module-created", Kind: runtimecontract.EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-module"}}}
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "selection-module", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: "module_definition", SelectorRef: "file/profile_declared_candidates",
		Disposition: runtimecontract.RuntimeTargetSelectionSelected, TargetRefs: []string{"go.mod"}, CandidateTargetRefs: []string{"go.mod"}, CandidateCount: 1, ObservedRevision: "tree-module",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	facts.Resources.TargetSelection = &selection
	facts.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{TargetRef: "go.mod", ContentHash: "hash", ContentState: runtimecontract.SnapshotContentStateComplete}}
	facts.Resources.FailureTarget = &runtimecontract.FailureTarget{Signature: "old-focus", TargetRefs: []string{"go.mod"}}
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: selection.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindTargetSelection,
		Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: facts.Focus.ID,
		OwnerModuleID: runtimecontract.RuntimeModuleFile,
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
		ContentHash: selection.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: "tree-module",
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}

	advanced := advanceSatisfiedCoverageFocus(facts)
	if advanced.Focus.ID != "bootstrap-source" || advanced.Resources.TargetSelection != nil || len(advanced.Resources.TargetSnapshots) != 0 || advanced.Resources.FailureTarget != nil {
		t.Fatalf("advanced focus retained predecessor target facts: %+v", advanced)
	}
	if len(advanced.Prerequisites.Satisfied) != 0 || len(advanced.Resources.PrerequisiteObservations) != 0 ||
		len(advanced.Prerequisites.Retired) != 1 || advanced.Prerequisites.Retired[0].ID != prerequisite.ID ||
		len(advanced.Resources.RetiredPrerequisiteObservations) != 1 || advanced.Resources.RetiredPrerequisiteObservations[0].ObservationID != observation.ObservationID {
		t.Fatalf("advanced focus did not retire predecessor target selection authority: %+v", advanced)
	}
	if _, err := CommitPrerequisiteResult(advanced, PrerequisiteResult{PrerequisiteID: prerequisite.ID, TargetSelection: &selection, Observation: &observation}); err == nil {
		t.Fatal("delayed predecessor target-selection result was accepted after focus advance")
	}
	refreshed, prerequisiteIDs, observationIDs, _, err := invalidatePrerequisiteObservationsForChanges(advanced, []runtimecontract.RuntimeResourceChange{{
		Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		SubjectRef: "workspace_deliverable", AfterRevision: "tree-source",
	}})
	if err != nil || len(prerequisiteIDs) != 0 || len(observationIDs) != 0 || len(refreshed.Prerequisites.Pending) != 0 {
		t.Fatalf("workspace change reactivated retired target selection: facts=%+v prerequisiteIDs=%v observationIDs=%v err=%v", refreshed, prerequisiteIDs, observationIDs, err)
	}
	updated, decision, err := ReduceRuntimeStateWithDirective(advanced, nil)
	if err != nil || decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(updated.Prerequisites.Pending) == 0 {
		t.Fatalf("next bootstrap focus did not acquire a fresh target selection: facts=%+v decision=%+v err=%v", updated, decision, err)
	}
	found := false
	for _, prerequisite := range updated.Prerequisites.Pending {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSelection && prerequisite.ProspectiveTargetRef == "internal/lsp/manager.go" &&
			prerequisite.Scope == runtimecontract.RuntimePrerequisiteScopeFocus && prerequisite.FocusID == "bootstrap-source" {
			found = true
		}
	}
	if !found {
		t.Fatalf("next bootstrap focus selection prerequisite = %+v", updated.Prerequisites.Pending)
	}
}

func TestRuntimeFactsRejectsHistoricalFocusScopedPrerequisiteAsCurrentAuthority(t *testing.T) {
	facts := validationFacts()
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "selection-history", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/profile_declared_candidates",
		Disposition: runtimecontract.RuntimeTargetSelectionSelected, TargetRefs: []string{"go.mod"},
		CandidateTargetRefs: []string{"go.mod"}, CandidateCount: 1, ObservedRevision: "tree-history",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: selection.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindTargetSelection,
		Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: "focus-history",
		OwnerModuleID: runtimecontract.RuntimeModuleFile,
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
		ContentHash: selection.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: "tree-history",
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	facts.Resources.TargetSelection = &selection
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{prerequisite}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}

	if _, err := ReduceRuntimeState(facts, nil); err == nil || !strings.Contains(err.Error(), "current-focus prerequisite") {
		t.Fatalf("historical focus-scoped prerequisite was accepted as current authority: %v", err)
	}
}

func TestReduceRuntimeStateUnsupportedTargetSelectorFailsWithoutProviderAuthority(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Focus.RequireTargetGrounding = true
	facts.Focus.TargetRole = "deliverable"
	facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	facts.Focus.TargetSelectorRef = "profile/semantic_owner_selector"
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "selection-unsupported", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: facts.Focus.TargetRole,
		SelectorRef: facts.Focus.TargetSelectorRef, Disposition: runtimecontract.RuntimeTargetSelectionUnsupported, ObservedRevision: "unsupported-selector:profile/semantic_owner_selector",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	facts.Resources.TargetSelection = &selection
	facts.Prerequisites = runtimecontract.PrerequisiteState{}
	bindCurrentFocusTargetSelection(&facts)
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateFailed || decision.Terminal == nil || decision.Terminal.Code != "target_grounding_unresolved" {
		t.Fatalf("unsupported selector decision = %+v", decision)
	}
}

func TestReduceRuntimeStateAttemptOnlyIncrementsForSameFactsRevision(t *testing.T) {
	facts := validationFacts()
	first, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := ReduceRuntimeState(facts, &first.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if retry.AuthoritySnapshot.Attempt != 1 {
		t.Fatalf("same facts attempt = %d, want 1", retry.AuthoritySnapshot.Attempt)
	}
	facts.EvidenceRevision = "evidence-2"
	next, err := ReduceRuntimeState(facts, &retry.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if next.AuthoritySnapshot.Attempt != 0 {
		t.Fatalf("advanced evidence attempt = %d, want reset to 0", next.AuthoritySnapshot.Attempt)
	}
}

func TestReduceRuntimeStatePrerequisiteProgressResetsAttempt(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Capabilities.StateChangeKinds = []string{"file/write"}
	facts.Prerequisites.Pending = []runtimecontract.RuntimePrerequisite{{ID: "manifest", Kind: runtimecontract.PrerequisiteKindManifest}}
	first, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := ReduceRuntimeState(facts, &first.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if retry.AuthoritySnapshot.Attempt != 1 {
		t.Fatalf("same prerequisite attempt = %d", retry.AuthoritySnapshot.Attempt)
	}
	facts.Prerequisites.Pending = nil
	next, err := ReduceRuntimeState(facts, &retry.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if next.State != runtimecontract.ConvergenceStateNeedStateChange || next.AuthoritySnapshot.Attempt != 0 {
		t.Fatalf("progressed decision = %+v", next)
	}
}

func TestReduceRuntimeStateFiltersCapabilitiesByFocusContract(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Focus.EvidenceRole = "supplemental_evidence"
	facts.Focus.AcceptedStepKinds = []string{"git/write"}
	facts.Focus.AcceptedOperationVariants = []runtimecontract.RuntimeOperationVariantRef{{Kind: "git/write", VariantID: "commit"}}
	facts.Focus.RejectedStepKinds = []string{"patch/apply"}
	facts.Capabilities.StateChangeKinds = []string{"patch/apply", "git/write", "file/edit"}
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.RequiredOutput.AcceptedKinds) != 1 || decision.RequiredOutput.AcceptedKinds[0] != "git/write" ||
		len(decision.RequiredOutput.AcceptedOperationVariants) != 1 || decision.RequiredOutput.AcceptedOperationVariants[0] != (runtimecontract.RuntimeOperationVariantRef{Kind: "git/write", VariantID: "commit"}) ||
		decision.Focus.EvidenceRole != "supplemental_evidence" {
		t.Fatalf("focus-specific required output = %+v focus=%+v", decision.RequiredOutput, decision.Focus)
	}
}

func TestReduceRuntimeStateDoesNotApplyGitSequenceToFileOnlyFocus(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileEdit}
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindFileEdit, runtimecontract.StepKindGitWrite}
	facts.Resources.CapabilityStates = []runtimecontract.RuntimeCapabilityStateSnapshot{{
		RequirementID: "git-state", OwnerModuleID: runtimecontract.RuntimeModuleGit,
		CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable", ObservedRevision: "repo-v1", ContentHash: "content-v1", SnapshotID: "snapshot-v1",
		AcquisitionKey: "acquisition-v1", QueryReceiptRef: "receipt-v1",
		Disposition:               runtimecontract.RuntimeCapabilityStateSequenceRequired,
		RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{{RequirementID: "repo-init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init"}},
	}}
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedStateChange || len(decision.RequiredOutput.AcceptedKinds) != 1 ||
		decision.RequiredOutput.AcceptedKinds[0] != runtimecontract.StepKindFileEdit || len(decision.RequiredOutput.RequiredOperationSequence) != 0 {
		t.Fatalf("File-only focus leaked Git sequence into required output: %+v", decision.RequiredOutput)
	}

	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindGitWrite}
	decision, err = ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.RequiredOutput.RequiredOperationSequence) != 1 || decision.RequiredOutput.RequiredOperationSequence[0].RequirementID != "repo-init" {
		t.Fatalf("Git focus did not retain its required sequence: %+v", decision.RequiredOutput)
	}
}

func TestReduceRuntimeStateKeepsStateChangeAfterRejectedRepair(t *testing.T) {
	// Validation rejected, then state-change repair also rejected: stay on produce_state_change.
	// Re-opening produce_validation would anti-replay the same failed validation shape.
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-1", RunID: "run-2", DefinitionRevision: "def-1", EvidenceRevision: "ev-6",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-001", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
			TargetRefs:            []string{"cmd/proxy/main.go"},
		},
		Evidence: runtimecontract.EvidenceLedger{
			Accepted: []runtimecontract.EvidenceFact{{
				ID: "acc-state", Kind: runtimecontract.EvidenceKindStateChange, Revision: "evidence-v2-00000000000000000001",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			}},
			Rejected: []runtimecontract.EvidenceFact{
				{
					ID: "rej-val", Kind: runtimecontract.EvidenceKindValidation, Revision: "evidence-v2-00000000000000000002",
					FailureClass: runtimecontract.FailureClassExecutionFailure, FailureMessage: "exit status 2",
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
				},
				{
					ID: "rej-patch", Kind: runtimecontract.EvidenceKindStateChange, Revision: "evidence-v2-00000000000000000003",
					FailureClass: runtimecontract.FailureClassExecutionFailure, FailureCode: "conflict",
					FailureSignature: "sig-repair", FailureMessage: "conflict",
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
				},
			},
		},
		Failure: &runtimecontract.FailureFact{
			Class: runtimecontract.FailureClassExecutionFailure, Code: "conflict",
			Signature: "sig-repair", Message: "conflict", TargetRefs: []string{"cmd/proxy/main.go"},
		},
		Resources: runtimecontract.RuntimeResourceFacts{FailureTarget: &runtimecontract.FailureTarget{Signature: "sig-repair", TargetRefs: []string{"cmd/proxy/main.go"}}},
		Coverage:  []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-001", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities: runtimecontract.RuntimeCapabilities{
			StateChangeKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
			ValidationKinds:  []string{runtimecontract.StepKindShellCommand},
		},
	}
	got, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("action = %s, want produce_state_change after rejected repair", got.State)
	}
}

func TestReduceRuntimeStateKeepsValidationGapAsValidationWithoutOwnerTarget(t *testing.T) {
	facts := validationFacts()
	facts.Activation.ExecutionContract.ValidationPolicies = []runtimecontract.RuntimeValidationPolicy{{
		ID: "validation", Scope: "workspace_profile", AppliesToEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		MaxValidationOnlyRetriesPerFocus: 2,
	}}
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.RuntimeBlockKindPatchApply}
	facts.Capabilities.ValidationKinds = []string{runtimecontract.StepKindFileRead}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "accepted-state", Kind: runtimecontract.EvidenceKindStateChange, Revision: "evidence-v2-00000000000000000001", CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	}}
	facts.Evidence.Rejected = []runtimecontract.EvidenceFact{{
		ID: "rejected-validation", Kind: runtimecontract.EvidenceKindValidation, Revision: "evidence-v2-00000000000000000002",
		FailureClass: runtimecontract.FailureClassExecutionFailure, FailureMessage: "validation failed", TargetRefs: []string{"validation-entry"}, CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	}}
	facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionFailure, Signature: "validation-failed", Message: "validation failed", TargetRefs: []string{"validation-entry"}}
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("ungrounded validation failure decision = %+v", decision)
	}
	if len(decision.RequiredOutput.TargetRefs) != 0 {
		t.Fatalf("validation command entry became repair target: %+v", decision.RequiredOutput)
	}
}

func TestReduceRuntimeStateUsesOwnerGroundedFailureForStateChangeRepair(t *testing.T) {
	base := validationFacts()
	base.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	base.Capabilities.StateChangeKinds = []string{runtimecontract.RuntimeBlockKindPatchApply}
	base.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	base.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "accepted-state", Kind: runtimecontract.EvidenceKindStateChange, Revision: "evidence-v2-00000000000000000001", CoverageWorkItemIDs: base.Focus.CoverageWorkItemIDs,
	}}
	base.Evidence.Rejected = []runtimecontract.EvidenceFact{{
		ID: "rejected-validation", Kind: runtimecontract.EvidenceKindValidation, Revision: "evidence-v2-00000000000000000002",
		FailureClass: runtimecontract.FailureClassExecutionFailure, FailureMessage: "validation failed", CoverageWorkItemIDs: base.Focus.CoverageWorkItemIDs,
	}}

	ungrounded := base
	ungrounded.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Signature: "build-failed", Message: "build failed", TargetRefs: []string{"implementation.go"},
	}
	decision, err := ReduceRuntimeState(ungrounded, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("ungrounded implementation failure decision = %+v, want validation", decision)
	}

	grounded := base
	grounded.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Signature: "build-failed", Message: "build failed", TargetRefs: []string{"implementation.go"},
	}
	grounded.Resources.FailureTarget = &runtimecontract.FailureTarget{Signature: "build-failed", TargetRefs: []string{"implementation.go"}}
	decision, err = ReduceRuntimeState(grounded, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("owner-grounded implementation failure decision = %+v, want state-change repair", decision)
	}
	if got := decision.RequiredOutput.TargetRefs; len(got) != 1 || got[0] != "implementation.go" {
		t.Fatalf("state-change repair targets = %v, want owner target", got)
	}
}

func TestReduceRuntimeStateAllowsValidationAfterAcceptedRepair(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-1", RunID: "run-2", DefinitionRevision: "def-1", EvidenceRevision: "ev-4",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-001", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
		},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-001", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Evidence: runtimecontract.EvidenceLedger{
			Accepted: []runtimecontract.EvidenceFact{
				{
					ID: "acc-old", Kind: runtimecontract.EvidenceKindStateChange, Revision: "evidence-v2-00000000000000000001",
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
				},
				{
					ID: "acc-repair", Kind: runtimecontract.EvidenceKindStateChange, Revision: "evidence-v2-00000000000000000003",
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
				},
			},
			Rejected: []runtimecontract.EvidenceFact{{
				ID: "rej-val", Kind: runtimecontract.EvidenceKindValidation, Revision: "evidence-v2-00000000000000000002",
				FailureClass: runtimecontract.FailureClassExecutionFailure, FailureMessage: "exit status 2",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			}},
		},
		Capabilities: runtimecontract.RuntimeCapabilities{
			StateChangeKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
			ValidationKinds:  []string{runtimecontract.StepKindShellCommand},
		},
	}
	got, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("action = %s, want produce_validation after accepted repair", got.State)
	}
}

func TestReduceRuntimeStateRejectedValidationWithoutOwnerTargetRemainsValidation(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		WorkspaceID: "ws-1", RunID: "run-2", DefinitionRevision: "def-1", EvidenceRevision: "ev-2",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-001", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
		},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-001", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Evidence: runtimecontract.EvidenceLedger{
			Accepted: []runtimecontract.EvidenceFact{{
				ID: "acc-state", Kind: runtimecontract.EvidenceKindStateChange, Revision: "evidence-v2-00000000000000000001",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			}},
			Rejected: []runtimecontract.EvidenceFact{{
				ID: "rej-val", Kind: runtimecontract.EvidenceKindValidation, Revision: "evidence-v2-00000000000000000002",
				FailureClass: runtimecontract.FailureClassExecutionFailure, FailureMessage: "exit status 2",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			}},
		},
		Capabilities: runtimecontract.RuntimeCapabilities{
			StateChangeKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
			ValidationKinds:  []string{runtimecontract.StepKindShellCommand},
		},
	}
	got, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("action = %s, want produce_validation for ungrounded validation failure", got.State)
	}
}

func TestReduceRuntimeStatePrerequisiteRefreshExhaustionOverridesPendingReacquisition(t *testing.T) {
	facts := validationFacts()
	facts.Prerequisites.Pending = []runtimecontract.RuntimePrerequisite{{
		ID: "manifest:blocks/file", Kind: runtimecontract.PrerequisiteKindManifest,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, Reason: "fresh manifest required",
	}}
	facts.FreshnessReacquireCoverage = []common.CoverageWorkItemID{"cwi-1"}
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassPrerequisiteRefreshExhausted,
		Code:  "prerequisite_refresh_exhausted", Signature: "refresh-budget:receipt-1",
		Message: "prerequisite refresh budget exhausted after owner resource change",
	}

	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateFailed || decision.Terminal == nil || decision.Terminal.Code != "prerequisite_refresh_exhausted" {
		t.Fatalf("decision = %+v, want prerequisite refresh terminal", decision)
	}
}

func TestReduceRuntimeStateDeterminedTerminalFailureOverridesFreshnessReacquisition(t *testing.T) {
	for _, tt := range []struct {
		name         string
		failureClass runtimecontract.FailureClass
		terminalCode string
	}{
		{name: "candidate retry exhausted", failureClass: runtimecontract.FailureClassCandidateExhausted, terminalCode: "candidate_retry_exhausted"},
		{name: "runtime contract defect", failureClass: runtimecontract.FailureClassRuntimeContractMisalignment, terminalCode: "runtime_contract_defect"},
		{name: "governance rejected", failureClass: runtimecontract.FailureClassGovernanceRejected, terminalCode: "governance_rejected"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			facts := validationFacts()
			facts.Prerequisites.Pending = []runtimecontract.RuntimePrerequisite{{
				ID: "manifest:blocks/file", Kind: runtimecontract.PrerequisiteKindManifest,
				OwnerModuleID: runtimecontract.RuntimeModuleFile, Reason: "fresh manifest required",
			}}
			facts.FreshnessReacquireCoverage = []common.CoverageWorkItemID{"cwi-1"}
			facts.Failure = &runtimecontract.FailureFact{
				Class: tt.failureClass, Code: tt.terminalCode, Signature: tt.terminalCode,
				Message: "determined failure must close the current authority",
			}

			decision, err := ReduceRuntimeState(facts, nil)
			if err != nil {
				t.Fatal(err)
			}
			if decision.State != runtimecontract.ConvergenceStateFailed || decision.Terminal == nil || decision.Terminal.Code != tt.terminalCode {
				t.Fatalf("decision = %+v, want terminal %s", decision, tt.terminalCode)
			}
		})
	}
}

func validationFacts() runtimecontract.RuntimeFacts {
	return runtimecontract.RuntimeFacts{
		Activation: runtimecontract.RuntimeActivationContext{
			ProviderTarget:   runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
			EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{runtimeV2ManagedValidationEvidencePolicy()},
		},
		WorkspaceID:        "ws-1",
		RunID:              "run-1",
		DefinitionRevision: "definition-1",
		EvidenceRevision:   "evidence-1",
		Focus: runtimecontract.CoverageFocus{
			ID:                    "cwi-1",
			CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
		},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Capabilities: runtimecontract.RuntimeCapabilities{
			StateChangeKinds: []string{"patch/apply"},
			ValidationKinds:  []string{"validation/managed_session"},
		},
	}
}

func TestReduceRuntimeStateRequiresRunLevelStateChangeBeforeComplete(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireAssessment = false
	facts.RemainingFocuses = nil
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "val-1", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Revision: facts.EvidenceRevision,
	}}
	facts.StateChange = runtimecontract.NewRuntimeStateChangeLedger(runtimecontract.RuntimeRunStateChangeRequirement{
		Required: true, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
	})
	facts = facts.Normalized()
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("decision = %+v, want produce_state_change for unsatisfied run ledger", decision)
	}

	// satisfy ledger then complete
	facts.StateChange = applyAcceptedStateChangeEvidence(facts.StateChange, runtimecontract.EvidenceFact{
		ID: "state-1", Kind: runtimecontract.EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Revision: "evidence-v2-00000000000000000002",
	}, []common.CoverageWorkItemID{"cwi-1"})
	decision, err = ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateComplete {
		t.Fatalf("decision = %+v, want complete after ledger satisfied", decision)
	}
}

func TestReduceRuntimeStateRepairableAssessmentDoesNotTerminal(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireAssessment = true
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{
		{ID: "state-1", Kind: runtimecontract.EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: facts.EvidenceRevision},
		{ID: "val-1", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: facts.EvidenceRevision},
	}
	facts.Assessments = []runtimecontract.RuntimeAssessmentFact{{
		ID: "assessment-repairable", FocusID: facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Disposition: runtimecontract.AssessmentDispositionRepairableGap,
		Summary:     "implementation incomplete",
		Gap: &runtimecontract.RuntimeAssessmentGap{
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
			// This is an LLM observation. The state-change decision below must
			// instead come from persisted state-change policy and valid validation.
			MissingEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			Reason:               "tools not implemented",
		},
	}}
	facts = facts.Normalized()
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("decision = %+v, want produce_state_change from persisted repair facts", decision)
	}
	if decision.State == runtimecontract.ConvergenceStateFailed {
		t.Fatalf("repairable assessment must not terminal: %+v", decision)
	}
}

func TestReduceRuntimeStateTerminalAssessmentWithoutPersistedGovernanceRejectionRepairs(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireAssessment = true
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "val-1", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Revision: facts.EvidenceRevision,
	}}
	facts.Assessments = []runtimecontract.RuntimeAssessmentFact{{
		ID: "assessment-terminal", FocusID: facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Disposition: runtimecontract.AssessmentDispositionTerminalRejection,
		Summary:     "irrecoverable acceptance failure",
		Gap: &runtimecontract.RuntimeAssessmentGap{
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
			MissingEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			Reason:               "irrecoverable acceptance failure",
		},
	}}
	facts = facts.Normalized()
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedValidation || decision.State == runtimecontract.ConvergenceStateFailed {
		t.Fatalf("decision = %+v, want validation repair because assessment text cannot choose terminal or state change", decision)
	}
}

func TestReduceRuntimeStateSameFocusRepairThenReassess(t *testing.T) {
	// After repairable assessment -> state-change -> validation, missing evidence is empty and
	// the prior repairable assessment has been invalidated, so reducer must request assessment again.
	facts := validationFacts()
	facts.Focus.RequireAssessment = true
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	facts.Capabilities.StateChangeKinds = []string{"patch/apply"}
	facts.Capabilities.ValidationKinds = []string{"validation/managed_session"}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{
		{ID: "state-1", Kind: runtimecontract.EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "evidence-v2-00000000000000000001"},
		{ID: "val-1", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "evidence-v2-00000000000000000002"},
	}
	// No assessment present (invalidated after repair evidence) => produce_assessment.
	facts = facts.Normalized()
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("decision = %+v, want produce_assessment after repair evidence", decision)
	}

	// Satisfied assessment then completes.
	facts.Assessments = []runtimecontract.RuntimeAssessmentFact{{
		ID: "assessment-1", FocusID: facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Disposition: runtimecontract.AssessmentDispositionSatisfied,
		Summary:     "coverage satisfied", EvidenceRefs: []string{"val-1"},
	}}
	facts.StateChange = runtimecontract.NewRuntimeStateChangeLedger(runtimecontract.RuntimeRunStateChangeRequirement{Required: false})
	facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateSatisfied, EvidenceRefs: []string{"val-1"}}}
	facts = facts.Normalized()
	decision, err = ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateComplete {
		t.Fatalf("decision = %+v, want complete after satisfied assessment", decision)
	}
}

func TestInvalidateNonSatisfiedAssessmentsForFocus(t *testing.T) {
	values := []runtimecontract.RuntimeAssessmentFact{
		{ID: "assessment-gap", FocusID: "focus-1", Disposition: runtimecontract.AssessmentDispositionRepairableGap, Summary: "gap"},
		{ID: "assessment-ok", FocusID: "focus-1", Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "ok"},
		{ID: "assessment-other", FocusID: "focus-2", Disposition: runtimecontract.AssessmentDispositionRepairableGap, Summary: "other"},
	}
	got := invalidateNonSatisfiedAssessmentsForFocus(values, "focus-1")
	if len(got) != 2 {
		t.Fatalf("got = %+v, want satisfied focus-1 and focus-2 retained", got)
	}
	if got[0].Disposition != runtimecontract.AssessmentDispositionSatisfied || got[1].FocusID != "focus-2" {
		t.Fatalf("got = %+v", got)
	}
}

func TestReduceRuntimeStateRepairBudgetExhausted(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireAssessment = true
	facts.ConvergencePolicy = runtimecontract.RuntimeConvergencePolicy{MaxRepairAttemptsPerFocus: 2, MaxValidationOnlyRetriesPerFocus: 1}
	facts.Repair = runtimecontract.RuntimeRepairLedger{FocusID: facts.Focus.ID, RepairAttempts: 2, Exhausted: false}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "val-1", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Revision: facts.EvidenceRevision,
	}}
	facts.Assessments = []runtimecontract.RuntimeAssessmentFact{{
		ID: "assessment-exhausted", FocusID: facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Disposition: runtimecontract.AssessmentDispositionRepairableGap,
		Summary:     "still broken",
		Gap: &runtimecontract.RuntimeAssessmentGap{
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
			MissingEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			Reason:               "still broken",
		},
	}}
	facts = facts.Normalized()
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateFailed {
		t.Fatalf("decision = %+v, want budget exhausted terminal", decision)
	}
	if decision.Terminal == nil || decision.Terminal.Code != "no_progress_exhausted" {
		t.Fatalf("terminal = %+v, want no_progress_exhausted", decision.Terminal)
	}
}

func TestReduceRuntimeStateValidationOnlyCapTerminalsWithoutStateChange(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireAssessment = true
	facts.Capabilities.StateChangeKinds = []string{"patch/apply"}
	facts.Capabilities.ValidationKinds = []string{"validation/managed_session"}
	facts.ConvergencePolicy = runtimecontract.RuntimeConvergencePolicy{MaxRepairAttemptsPerFocus: 10, MaxValidationOnlyRetriesPerFocus: 1}
	facts.Repair = runtimecontract.RuntimeRepairLedger{FocusID: facts.Focus.ID, RepairAttempts: 1, ValidationOnlyRetries: 1}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "val-1", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Revision: facts.EvidenceRevision,
	}}
	facts.Assessments = []runtimecontract.RuntimeAssessmentFact{{
		ID: "assessment-validation-cap", FocusID: facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Disposition: runtimecontract.AssessmentDispositionRepairableGap,
		Summary:     "validation still failing",
		Gap: &runtimecontract.RuntimeAssessmentGap{
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
			MissingEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			Reason:               "validation still failing",
		},
	}}
	facts = facts.Normalized()
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateFailed || decision.Terminal == nil || decision.Terminal.Code != "no_progress_exhausted" {
		t.Fatalf("decision = %+v, want validation-only budget terminal", decision)
	}
}

func TestReduceRuntimeStateGovernanceRequiredOpensGovernance(t *testing.T) {
	facts := controllerFacts()
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassGovernanceRequired, Code: "approval_required",
		Signature: "governance-approval-web", Message: "web operation requires approval before side effects",
		TargetRefs: []string{"https://example.com"},
		GovernanceDependency: &runtimecontract.ExternalDependency{
			Kind: string(runtimecontract.GovernanceApproval), SubjectRef: "https://example.com", Reason: "web operation requires approval before side effects",
		},
	}
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindApprovalRequest}
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedGovernance {
		t.Fatalf("action = %s, want request_governance (not execution repair)", decision.State)
	}
}

func TestReduceRuntimeStateGenericGovernanceFailureDoesNotOpenTicket(t *testing.T) {
	facts := controllerFacts()
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassGovernanceRequired, Code: "approval_required",
		Signature: "unbound-governance", Message: "approval required",
		TargetRefs: []string{".env"},
	}
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindApprovalRequest}
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateFailed || decision.Terminal == nil || decision.Terminal.Code != "governance_dependency_missing" {
		t.Fatalf("generic governance decision = %+v, want dependency-missing terminal", decision)
	}
}

func TestReduceRuntimeStateProviderAuthenticationOpensConfigurationGovernance(t *testing.T) {
	facts := controllerFacts()
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassProviderAuthentication, Code: "provider_authentication",
		Message: "provider rejected the configured credential",
	}
	facts.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-1"}}
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindDecisionRequest}
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedGovernance {
		t.Fatalf("provider authentication decision = %+v", decision)
	}
}

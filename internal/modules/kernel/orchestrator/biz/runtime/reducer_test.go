package runtime

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRequiredStateChangeOperationSequenceSealsCompleteProspectiveAllOfTargets(t *testing.T) {
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: "binding/prospective/all-of", CoverageWorkItemID: "cwi-prospective", FocusID: "focus-prospective",
		DefinitionRevision: "definition-prospective", DeclarationRef: "profile/prospective",
		Proofs: []runtimecontract.CoverageEvidenceProof{{
			EvidenceKind: runtimecontract.EvidenceKindStateChange, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectWorkspaceDurable},
			AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite},
		}},
		TargetSelectorRef: "workspace-relative-new-file-target", TargetSelectorOwnerRef: runtimecontract.RuntimeModuleFile,
		TargetSelectorSubjectRef: "workspace_deliverable", ProspectiveTargetRefs: []string{"workorch_test.go", "go.mod"},
	}.Normalized()
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	facts := runtimecontract.RuntimeFacts{Focus: runtimecontract.CoverageFocus{
		ID: binding.FocusID, CoverageWorkItemIDs: []common.CoverageWorkItemID{binding.CoverageWorkItemID},
		CoverageEvidenceBindings: []runtimecontract.CoverageEvidenceBinding{binding},
	}}

	got := requiredStateChangeOperationSequence(facts, []string{runtimecontract.StepKindFileWrite})
	requirementPrefix := "prospective-" + binding.Hash[:12] + "-target-"
	want := []runtimecontract.RuntimeRequiredOperationVariant{
		{RequirementID: requirementPrefix + "01", Kind: runtimecontract.StepKindFileWrite, TargetRef: "go.mod", Reason: "complete the sealed prospective all-of target transaction"},
		{RequirementID: requirementPrefix + "02", Kind: runtimecontract.StepKindFileWrite, TargetRef: "workorch_test.go", DependsOnRequirementIDs: []string{requirementPrefix + "01"}, Reason: "complete the sealed prospective all-of target transaction"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prospective all-of operation sequence = %+v, want %+v", got, want)
	}
	if repeated := requiredStateChangeOperationSequence(facts, []string{runtimecontract.StepKindFileWrite}); !reflect.DeepEqual(repeated, want) {
		t.Fatalf("prospective all-of sequence changed across deterministic re-drive: %+v", repeated)
	}
	if ignored := requiredStateChangeOperationSequence(facts, []string{runtimecontract.StepKindGitWrite}); len(ignored) != 0 {
		t.Fatalf("prospective sequence escaped accepted owner kind: %+v", ignored)
	}
}

func TestReduceRuntimeStateLiveFailureFixtures(t *testing.T) {
	tests := []struct {
		name  string
		facts runtimecontract.RuntimeFacts
	}{
		{
			name: "lsp validation failure remains validation without owner repair target",
			facts: runtimecontract.RuntimeFacts{
				Activation:         runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
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
					Accepted: []runtimecontract.EvidenceFact{{ID: "e-state", Kind: runtimecontract.EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-036"}}},
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
				Activation:         runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
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
			got, err := reduceRuntimeStateFixture(t, tt.facts, nil)
			if err != nil {
				t.Fatalf("reduceRuntimeStateFixture(t, ) error = %v", err)
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
	left, err := reduceRuntimeStateFixture(t, facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	right, err := reduceRuntimeStateFixture(t, facts, nil)
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
	facts.ConvergencePolicy.MaxRepairAttemptsPerFocus = 3
	facts = runtimeTestFactsWithCurrentFocusBindings(t, facts, runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: facts.DefinitionRevision})
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Signature: "stable-patch-failure", Message: "patch input remains invalid", TargetRefs: append([]string(nil), facts.Focus.TargetRefs...),
	}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{ID: "accepted-unrelated", Kind: runtimecontract.EvidenceKindValidation, Revision: "runtime-evidence-00000000000000000001"}}
	for idx := 0; idx < 2; idx++ {
		facts.Evidence.Rejected = append(facts.Evidence.Rejected, runtimecontract.EvidenceFact{
			ID: common.EvidenceRecordID("rejected-" + string(rune('a'+idx))), Kind: runtimecontract.EvidenceKindStateChange,
			FailureClass: runtimecontract.FailureClassExecutionFailure, FailureSignature: "stable-patch-failure",
			Revision:            fmt.Sprintf("runtime-evidence-%020d", idx+2),
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
		Revision:            "runtime-evidence-00000000000000000004",
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
		{name: "assessment after accepted evidence", mutate: func(f *runtimecontract.RuntimeFacts) {
			f.Evidence.Accepted = []runtimecontract.EvidenceFact{{ID: "e-validation", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}}}
		}, state: runtimecontract.ConvergenceStateNeedAssessment, action: runtimecontract.ConvergenceStateNeedAssessment},
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
			got, err := reduceRuntimeStateFixture(t, facts, nil)
			if err != nil {
				t.Fatalf("reduceRuntimeStateFixture(t, ) error = %v", err)
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
			decision, err := reduceRuntimeStateFixture(t, facts, nil)
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
		facts.Focus.TargetSelectorOwnerRef = runtimecontract.RuntimeModuleFile
		facts.Focus.TargetSelectorSubjectRef = facts.Focus.TargetRole
		facts.Focus.ProspectiveTargetRefs = []string{"initial-deliverable.txt"}
		facts.Focus.RequireTargetGrounding = true
		selection := runtimecontract.RuntimeTargetSelection{
			PrerequisiteID: "selection-1", OwnerModuleID: runtimecontract.RuntimeModuleFile,
			TargetRole: facts.Focus.TargetRole, SelectorRef: facts.Focus.TargetSelectorRef,
			SelectorOwnerRef: facts.Focus.TargetSelectorOwnerRef, SelectorSubjectRef: facts.Focus.TargetSelectorSubjectRef,
			Disposition: runtimecontract.RuntimeTargetSelectionCreate, ProspectiveTargetRef: facts.Focus.ProspectiveTargetRefs[0], CandidateCount: 0, ObservedRevision: "workspace-empty-v1",
		}
		selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
		facts.Resources.TargetSelection = &selection
		inventory := runtimecontract.RuntimeOperationAdmissibility{
			PrerequisiteID: "admissibility-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
			OperationKinds: []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindPatchApply},
			SelectorRefs:   []string{facts.Focus.TargetSelectorRef}, ConstraintRefs: []string{"new-target-requires-owner-selector", "workspace-relative-path"},
			ObservedRevision: "workspace-empty-v1",
			OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{
				{Kind: runtimecontract.StepKindFileWrite, AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget, runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget}, SelectorRefs: []string{facts.Focus.TargetSelectorRef}, ConstraintRefs: []string{"new-target-requires-owner-selector", "workspace-relative-path"}},
				{Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "file_write", AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget, runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget}, SelectorRefs: []string{facts.Focus.TargetSelectorRef}, ConstraintRefs: []string{"new-target-requires-owner-selector", "workspace-relative-path"}},
				{Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "text_replace", AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget, runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope}},
				{Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "unified_diff", AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget, runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope, runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget}, SelectorRefs: []string{facts.Focus.TargetSelectorRef}, ConstraintRefs: []string{"new-target-requires-owner-selector", "workspace-relative-path"}},
			},
		}
		inventory.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(inventory)
		facts.Resources.OperationAdmissibilities = append(facts.Resources.OperationAdmissibilities, inventory)
		bindCurrentFocusTargetSelection(&facts)
		return facts
	}

	t.Run("fails before provider when only non-file durable capability is sealed", func(t *testing.T) {
		facts := runtimeTestFactsWithCurrentFocusBindings(t, newCreationFacts(), runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: "definition-1"})
		facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindGitWrite}
		facts.Focus.AcceptedOperationVariants = nil
		facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindGitWrite}
		facts = runtimeTestFactsWithCanonicalCWIProjection(t, facts)
		decision, err := ReduceRuntimeState(facts, nil)
		if err != nil {
			t.Fatal(err)
		}
		if decision.State != runtimecontract.ConvergenceStateFailed || decision.Terminal == nil || decision.Terminal.Code != "creation_scope_capability_unavailable" {
			t.Fatalf("creation scope without File admission decision = %+v", decision)
		}
	})

	t.Run("permits only owner-declared create operation", func(t *testing.T) {
		facts := newCreationFacts()
		facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindPatchApply}
		facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindPatchApply}
		decision, err := reduceRuntimeStateFixture(t, facts, nil)
		if err != nil {
			t.Fatal(err)
		}
		if decision.State != runtimecontract.ConvergenceStateNeedStateChange || len(decision.RequiredOutput.AcceptedKinds) != 1 || decision.RequiredOutput.AcceptedKinds[0] != runtimecontract.StepKindFileWrite {
			t.Fatalf("creation scope File admission decision = %+v", decision)
		}
	})

	t.Run("create precedes ordinary validation until durable target evidence exists", func(t *testing.T) {
		facts := newCreationFacts()
		facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
		facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindManagedValidationSession}
		facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindFileWrite}
		facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
		facts = runtimeTestFactsWithCurrentFocusBindings(t, facts, runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: facts.DefinitionRevision})
		decision, err := ReduceRuntimeState(facts, nil)
		if err != nil {
			t.Fatal(err)
		}
		if decision.State != runtimecontract.ConvergenceStateNeedStateChange || len(decision.RequiredOutput.AcceptedKinds) != 1 || decision.RequiredOutput.AcceptedKinds[0] != runtimecontract.StepKindFileWrite || len(decision.RequiredOutput.TargetRefs) != 1 || decision.RequiredOutput.TargetRefs[0] != "initial-deliverable.txt" {
			t.Fatalf("initial creation decision = %+v", decision)
		}

		facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
			ID: "created", Kind: runtimecontract.EvidenceKindStateChange, TargetRefs: []string{"initial-deliverable.txt"},
			CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs, Revision: "runtime-evidence-00000000000000000001",
			FocusID: facts.Focus.ID, BindingRefs: []string{facts.Focus.CoverageEvidenceBindings[0].ID},
		}}
		facts.EvidenceRevision = "runtime-evidence-00000000000000000001"
		facts.Resources.TargetSelection = &runtimecontract.RuntimeTargetSelection{
			PrerequisiteID: "selection-1", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: facts.Focus.TargetRole,
			SelectorRef: facts.Focus.TargetSelectorRef, SelectorOwnerRef: facts.Focus.TargetSelectorOwnerRef, SelectorSubjectRef: facts.Focus.TargetSelectorSubjectRef,
			Disposition: runtimecontract.RuntimeTargetSelectionSelected,
			TargetRefs:  []string{"initial-deliverable.txt"}, CandidateTargetRefs: []string{"initial-deliverable.txt"}, CandidateCount: 1, ObservedRevision: "workspace-created-v1",
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

	t.Run("mismatched create observation cannot rewrite the sealed declaration", func(t *testing.T) {
		facts := runtimeTestFactsWithCurrentFocusBindings(t, newCreationFacts(), runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: "definition-1"})
		facts.Resources.TargetSelection.ProspectiveTargetRef = "unsealed-target.txt"
		facts.Resources.TargetSelection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(*facts.Resources.TargetSelection)
		bindCurrentFocusTargetSelection(&facts)
		decision, err := ReduceRuntimeState(facts, nil)
		if err != nil {
			t.Fatal(err)
		}
		if decision.State != runtimecontract.ConvergenceStateNeedStateChange || len(decision.RequiredOutput.TargetRefs) != 1 ||
			decision.RequiredOutput.TargetRefs[0] != "initial-deliverable.txt" || runtimeStringsContain(decision.RequiredOutput.TargetRefs, "unsealed-target.txt") {
			t.Fatalf("mismatched owner observation rewrote sealed prospective authority: %+v", decision)
		}
	})
}

func TestSemanticOwnerGroundedTargetScopeRequiresMatchingSealedCatalog(t *testing.T) {
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "mcp-catalog", OwnerModuleID: runtimecontract.RuntimeModuleMCP,
		OperationKinds: []string{runtimecontract.StepKindMCPToolCall}, SelectorRefs: []string{"mcp-server"},
		ValidationRecipeRefs: []string{"mcp/tool_call/mcp-server/inspect"}, ObservedRevision: "mcp-revision",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	facts := runtimecontract.RuntimeFacts{
		Focus: runtimecontract.CoverageFocus{
			TargetSelectorOwnerRef: "software-dev", TargetSelectorRef: "mcp-server",
			CoverageEvidenceBindings: []runtimecontract.CoverageEvidenceBinding{{
				ID:                     "semantic-binding-mcp",
				TargetSelectorOwnerRef: "software-dev", TargetSelectorRef: "mcp-server",
				PrerequisiteRefs: []string{catalog.SnapshotID, "semantic-feasibility/owner-fact"},
				Proofs: []runtimecontract.CoverageEvidenceProof{{
					EvidenceKind: runtimecontract.EvidenceKindValidation, RequiredEffectScopes: []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone},
					AllowedOperationKinds: []string{runtimecontract.StepKindMCPToolCall}, ValidationRecipeID: "mcp/tool_call/mcp-server/inspect",
				}},
			}},
		},
		Resources: runtimecontract.RuntimeResourceFacts{OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{catalog}},
	}
	if !hasSemanticOwnerGroundedTargetScope(facts) {
		t.Fatal("matching SM-2-sealed MCP catalog was not recognized as owner-grounded scope")
	}
	targetAuthority := runtimeTargetAuthorityPlanForFacts(facts)
	if !targetAuthority.grounded() || targetAuthority.hasProspectiveTargets() || len(targetAuthority.targets()) != 0 {
		t.Fatalf("semantic existing-owner scope was not preserved by unified target authority: %+v", targetAuthority)
	}
	facts.Focus.CoverageEvidenceBindings[0].PrerequisiteRefs[0] = "operation-admissibility-forged"
	if hasSemanticOwnerGroundedTargetScope(facts) {
		t.Fatal("forged MCP catalog snapshot was accepted as owner-grounded scope")
	}
	facts.Focus.CoverageEvidenceBindings[0].PrerequisiteRefs[0] = catalog.SnapshotID
	facts.Focus.CoverageEvidenceBindings[0].Proofs[0].AllowedOperationKinds = []string{runtimecontract.StepKindShellCommand}
	if hasSemanticOwnerGroundedTargetScope(facts) {
		t.Fatal("MCP catalog accepted semantic scope from a Shell-owned proof operation")
	}
}

func TestSemanticFileProspectiveTargetScopeRequiresMatchingSealedOwnerFacts(t *testing.T) {
	newFacts := func() runtimecontract.RuntimeFacts {
		facts := validationFacts()
		facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
		facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileWrite}
		// The declaration owner is Profile lineage, not the File capability
		// owner. Capability ownership must be derived from the sealed proof kind.
		facts.Focus.TargetSelectorOwnerRef = "software-dev"
		facts.Focus.TargetSelectorRef = "workspace-relative-new-file-target"
		facts.Focus.TargetSelectorSubjectRef = "workspace_deliverable"
		facts.Focus.ProspectiveTargetRefs = []string{"runtime-validation-source.txt"}
		facts.Focus.RequireTargetGrounding = true
		facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindFileWrite}
		catalog := runtimecontract.RuntimeOperationAdmissibility{
			PrerequisiteID: "test-semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
			OperationKinds: []string{runtimecontract.StepKindFileWrite}, SelectorRefs: []string{facts.Focus.TargetSelectorRef},
			ConstraintRefs: []string{"new-target-requires-owner-selector", "workspace-relative-path"}, ObservedRevision: "workspace-empty-v1",
			OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
				Kind: runtimecontract.StepKindFileWrite,
				AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{
					runtimecontract.RuntimeOperationTargetPolicyExactTarget,
					runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
					runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget,
				},
				SelectorRefs: []string{facts.Focus.TargetSelectorRef}, ConstraintRefs: []string{"new-target-requires-owner-selector", "workspace-relative-path"},
			}},
		}
		catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
		facts.Resources.OperationAdmissibilities = append(facts.Resources.OperationAdmissibilities, catalog)
		facts = runtimeTestFactsWithCurrentFocusBindings(t, facts, runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: facts.DefinitionRevision})
		return runtimeTestFactsWithCanonicalCWIProjection(t, facts)
	}

	t.Run("matching prospective binding enters state change", func(t *testing.T) {
		facts := newFacts()
		if !hasSemanticOwnerGroundedTargetScope(facts) || !hasOwnerGroundedProspectiveCreationScope(facts) {
			t.Fatal("matching SM-2-sealed File prospective scope was not recognized")
		}
		targetAuthority := runtimeTargetAuthorityPlanForFacts(facts)
		if !targetAuthority.hasProspectiveTargets() || len(targetAuthority.existingTargets()) != 0 ||
			!sameRuntimeTargetRefs(targetAuthority.prospectiveTargets(), facts.Focus.ProspectiveTargetRefs) {
			t.Fatalf("semantic target authority conflated Profile declaration owner with File capability owner: %+v", targetAuthority)
		}
		for _, requirement := range runtimeTargetGroundingRequirements(facts) {
			if requirement.Kind == runtimecontract.PrerequisiteKindTargetSnapshot {
				t.Fatalf("binding-sealed prospective target requested a pre-create snapshot: %+v", requirement)
			}
		}
		decision, err := ReduceRuntimeState(facts, nil)
		if err != nil {
			t.Fatal(err)
		}
		if decision.State != runtimecontract.ConvergenceStateNeedStateChange || !runtimeStringsContain(decision.RequiredOutput.AcceptedKinds, runtimecontract.StepKindFileWrite) {
			t.Fatalf("semantic File prospective decision = %+v", decision)
		}
	})
	fileCatalogIndex := func(facts *runtimecontract.RuntimeFacts) int {
		for index := range facts.Resources.OperationAdmissibilities {
			if facts.Resources.OperationAdmissibilities[index].OwnerModuleID == runtimecontract.RuntimeModuleFile {
				return index
			}
		}
		return -1
	}

	for _, test := range []struct {
		name   string
		mutate func(*runtimecontract.RuntimeFacts)
	}{
		{name: "forged catalog snapshot", mutate: func(facts *runtimecontract.RuntimeFacts) {
			binding := facts.Focus.CoverageEvidenceBindings[0]
			binding.PrerequisiteRefs[0] = "operation-admissibility-forged"
			binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
			facts.Focus.CoverageEvidenceBindings[0] = binding
		}},
		{name: "missing owner selector", mutate: func(facts *runtimecontract.RuntimeFacts) {
			index := fileCatalogIndex(facts)
			if index < 0 {
				return
			}
			catalog := facts.Resources.OperationAdmissibilities[index]
			catalog.SelectorRefs = nil
			catalog.OperationVariants[0].AllowedTargetPolicies = []runtimecontract.RuntimeOperationTargetPolicy{
				runtimecontract.RuntimeOperationTargetPolicyExactTarget,
				runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
			}
			catalog.OperationVariants[0].SelectorRefs = nil
			catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
			facts.Resources.OperationAdmissibilities[index] = catalog
			binding := facts.Focus.CoverageEvidenceBindings[0]
			binding.PrerequisiteRefs[0] = catalog.SnapshotID
			binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
			facts.Focus.CoverageEvidenceBindings[0] = binding
		}},
		{name: "missing prospective constraint", mutate: func(facts *runtimecontract.RuntimeFacts) {
			index := fileCatalogIndex(facts)
			if index < 0 {
				return
			}
			catalog := facts.Resources.OperationAdmissibilities[index]
			catalog.ConstraintRefs = []string{"workspace-relative-path"}
			catalog.OperationVariants[0].AllowedTargetPolicies = []runtimecontract.RuntimeOperationTargetPolicy{
				runtimecontract.RuntimeOperationTargetPolicyExactTarget,
				runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope,
			}
			catalog.OperationVariants[0].ConstraintRefs = []string{"workspace-relative-path"}
			catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
			facts.Resources.OperationAdmissibilities[index] = catalog
			binding := facts.Focus.CoverageEvidenceBindings[0]
			binding.PrerequisiteRefs[0] = catalog.SnapshotID
			binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
			facts.Focus.CoverageEvidenceBindings[0] = binding
		}},
		{name: "prospective target mismatch", mutate: func(facts *runtimecontract.RuntimeFacts) {
			binding := facts.Focus.CoverageEvidenceBindings[0]
			binding.ProspectiveTargetRefs = []string{"different-target.txt"}
			binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
			facts.Focus.CoverageEvidenceBindings[0] = binding
		}},
		{name: "proof operation belongs to another capability owner", mutate: func(facts *runtimecontract.RuntimeFacts) {
			binding := facts.Focus.CoverageEvidenceBindings[0]
			binding.Proofs[0].AllowedOperationKinds = []string{runtimecontract.StepKindShellCommand}
			binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
			facts.Focus.CoverageEvidenceBindings[0] = binding
		}},
		{name: "missing semantic feasibility lineage", mutate: func(facts *runtimecontract.RuntimeFacts) {
			binding := facts.Focus.CoverageEvidenceBindings[0]
			binding.PrerequisiteRefs = binding.PrerequisiteRefs[:1]
			binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
			facts.Focus.CoverageEvidenceBindings[0] = binding
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			facts := newFacts()
			test.mutate(&facts)
			if test.name == "proof operation belongs to another capability owner" {
				if hasSemanticOwnerGroundedTargetScope(facts) {
					t.Fatal("File catalog accepted prospective authority from a proof operation owned by Shell")
				}
				return
			}
			if test.name == "prospective target mismatch" {
				if err := runtimecontract.ValidateCoverageEvidenceBindingsForFocus(facts.Focus, facts.DefinitionRevision); err == nil || !strings.Contains(err.Error(), "target surface") {
					t.Fatalf("mismatched binding escaped the sealed target-surface contract: %v", err)
				}
				return
			}
			facts = runtimeTestFactsWithCanonicalCWIProjection(t, facts)
			if hasSemanticOwnerGroundedTargetScope(facts) || hasOwnerGroundedProspectiveCreationScope(facts) {
				t.Fatal("unsealed or mismatched File prospective scope was accepted")
			}
			decision, err := ReduceRuntimeState(facts, nil)
			if err != nil {
				t.Fatal(err)
			}
			if decision.State != runtimecontract.ConvergenceStateFailed || decision.Terminal == nil || decision.Terminal.Code != "target_grounding_unresolved" {
				t.Fatalf("mismatched File prospective decision = %+v", decision)
			}
		})
	}
}

func TestAdvanceSatisfiedCoverageFocusDropsPriorFocusTargetFacts(t *testing.T) {
	facts := validationFacts()
	facts.Focus = runtimecontract.CoverageFocus{ID: "bootstrap-module", SliceID: "bootstrap", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-module"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}}
	facts.RemainingFocuses = []runtimecontract.CoverageFocus{{
		ID: "bootstrap-source", SliceID: "bootstrap", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-source"}, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		TargetRole: "implementation", TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}, TargetSelectorRef: "file/profile_declared_candidates",
		TargetSelectorOwnerRef: "software-dev", TargetSelectorSubjectRef: "go-lsp/source", TargetSelectorCandidateRefs: []string{"internal/lsp/manager.go"}, ProspectiveTargetRefs: []string{"internal/lsp/manager.go"}, RequireTargetGrounding: true,
	}}
	facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{
		{ID: "cwi-module", SliceID: "bootstrap", SourceScope: "dodItem", SourceID: "dod-module", TargetText: "create module", State: runtimecontract.RuntimeCoverageItemStateOpen},
		{ID: "cwi-source", SliceID: "bootstrap", SourceScope: "dodItem", SourceID: "dod-source", TargetText: "create source", State: runtimecontract.RuntimeCoverageItemStateOpen},
	}
	facts.Slices = []runtimecontract.RuntimeAcceptanceSlice{{ID: "bootstrap", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-module", "cwi-source"}}}
	facts.ConvergencePolicy.MaxAcceptanceSliceWorkItems = 2
	facts.DefinitionSeed = runtimecontract.RuntimeDefinitionSeed{
		SchemaVersion: runtimecontract.RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: facts.DefinitionRevision, PolicyHash: "bootstrap-policy",
		CoverageItems: []runtimecontract.RuntimeCoverageItem{
			{ID: "cwi-module", SliceID: "bootstrap", SourceScope: "dodItem", SourceID: "dod-module", TargetText: "create module", Order: 0},
			{ID: "cwi-source", SliceID: "bootstrap", SourceScope: "dodItem", SourceID: "dod-source", TargetText: "create source", Order: 1},
		},
		Slices: facts.Slices,
	}
	facts.DefinitionSeed.Hash = runtimecontract.StableRuntimeDefinitionSeedHash(facts.DefinitionSeed)
	fileCatalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "test-semantic-file", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"go.mod"},
		SelectorRefs: []string{"file/profile_declared_candidates"}, ConstraintRefs: []string{"new-target-requires-owner-selector", "workspace-relative-path"},
		OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
			Kind:                  runtimecontract.StepKindFileWrite,
			AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget, runtimecontract.RuntimeOperationTargetPolicyProspectiveOwnerTarget},
			SelectorRefs:          []string{"file/profile_declared_candidates"}, ConstraintRefs: []string{"new-target-requires-owner-selector", "workspace-relative-path"},
		}},
		ObservedRevision: "tree-module",
	}
	fileCatalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(fileCatalog)
	facts.Resources.OperationAdmissibilities = append(facts.Resources.OperationAdmissibilities, fileCatalog)
	facts = runtimeTestFactsWithCurrentFocusBindings(t, facts, runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: facts.DefinitionRevision})
	for index := range facts.Coverage {
		if facts.Coverage[index].ID == "cwi-module" {
			facts.Coverage[index].State = runtimecontract.RuntimeCoverageItemStateSatisfied
		}
	}
	binding, found := coverageBindingForRuntimeCWI(facts.Focus, "cwi-module")
	if !found {
		t.Fatal("sealed module focus has no exact CWI binding")
	}
	facts.EvidenceRevision = "runtime-evidence-00000000000000000001"
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "module-created", Kind: runtimecontract.EvidenceKindStateChange, FocusID: facts.Focus.ID,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-module"}, BindingRefs: []string{binding.ID},
		TargetRefs: append([]string(nil), binding.TargetRefs...), Revision: facts.EvidenceRevision,
		DispatchID: "dispatch/module-created", OperationID: "operation/module-created",
	}}
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
	facts = runtimeTestFactsWithCanonicalCWIProjection(t, facts)

	advanced, err := advanceSatisfiedCoverageFocus(facts)
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil || decision.State != runtimecontract.ConvergenceStateNeedPrerequisite || len(updated.Prerequisites.Pending) != 2 {
		t.Fatalf("next bootstrap focus did not acquire a fresh target selection: facts=%+v decision=%+v err=%v", updated, decision, err)
	}
	selectionFound := false
	for _, prerequisite := range updated.Prerequisites.Pending {
		if prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSelection && sameRuntimeTargetRefs(prerequisite.TargetSelectorCandidateRefs, []string{"internal/lsp/manager.go"}) &&
			prerequisite.Scope == runtimecontract.RuntimePrerequisiteScopeFocus && prerequisite.FocusID == "bootstrap-source" {
			selectionFound = true
		}
	}
	if !selectionFound {
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
	facts = runtimeTestFactsWithCurrentFocusBindings(t, facts, runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: facts.DefinitionRevision})
	facts.Focus.RequireTargetGrounding = true
	facts.Focus.TargetRole = "deliverable"
	facts.Focus.TargetDiscoverySources = []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}
	facts.Focus.TargetSelectorRef = "profile/semantic_owner_selector"
	facts.Focus.TargetSelectorOwnerRef = "profile-owner"
	facts.Focus.TargetSelectorSubjectRef = facts.Focus.TargetRole
	runtimeTestSealCurrentFocusBindingTargetSurface(&facts, runtimeTestBindingTargetSurface{
		SelectorRef: facts.Focus.TargetSelectorRef, SelectorOwnerRef: facts.Focus.TargetSelectorOwnerRef, SelectorSubjectRef: facts.Focus.TargetSelectorSubjectRef,
	})
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "selection-unsupported", OwnerModuleID: runtimecontract.RuntimeModuleFile, TargetRole: facts.Focus.TargetRole,
		SelectorRef: facts.Focus.TargetSelectorRef, SelectorOwnerRef: facts.Focus.TargetSelectorOwnerRef, SelectorSubjectRef: facts.Focus.TargetSelectorSubjectRef,
		Disposition: runtimecontract.RuntimeTargetSelectionUnsupported, ObservedRevision: "unsupported-selector:profile/semantic_owner_selector",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	facts.Resources.TargetSelection = &selection
	bindCurrentFocusTargetSelection(&facts)
	facts = runtimeTestFactsWithCanonicalCWIProjection(t, facts)
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
	first, err := reduceRuntimeStateFixture(t, facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := reduceRuntimeStateFixture(t, facts, &first.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if retry.AuthoritySnapshot.Attempt != 1 {
		t.Fatalf("same facts attempt = %d, want 1", retry.AuthoritySnapshot.Attempt)
	}
	facts.EvidenceRevision = "evidence-2"
	next, err := reduceRuntimeStateFixture(t, facts, &retry.AuthoritySnapshot)
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
	first, err := reduceRuntimeStateFixture(t, facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := reduceRuntimeStateFixture(t, facts, &first.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if retry.AuthoritySnapshot.Attempt != 1 {
		t.Fatalf("same prerequisite attempt = %d", retry.AuthoritySnapshot.Attempt)
	}
	facts.Prerequisites.Pending = nil
	next, err := reduceRuntimeStateFixture(t, facts, &retry.AuthoritySnapshot)
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
	facts.Capabilities.StateChangeKinds = []string{"patch/apply", "git/write", "file/edit"}
	facts = runtimeTestFactsWithCurrentFocusBindings(t, facts, runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: facts.DefinitionRevision})
	facts.Focus.EvidenceRole = "supplemental_evidence"
	facts.Focus.AcceptedStepKinds = []string{"git/write"}
	facts.Focus.AcceptedOperationVariants = []runtimecontract.RuntimeOperationVariantRef{{Kind: "git/write", VariantID: "commit"}}
	facts.Focus.RejectedStepKinds = []string{"patch/apply"}
	facts = runtimeTestFactsWithCanonicalCWIProjection(t, facts)
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

func TestReduceRuntimeStateFiltersAcceptedVariantsByCurrentActionKinds(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Focus.AcceptedOperationVariants = []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "file_write"}}
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.RuntimeBlockKindPatchApply}
	facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}

	stateChange, err := reduceRuntimeStateFixture(t, facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(stateChange.RequiredOutput.AcceptedOperationVariants) != 1 || stateChange.RequiredOutput.AcceptedOperationVariants[0].Kind != runtimecontract.RuntimeBlockKindPatchApply {
		t.Fatalf("state-change variants = %+v", stateChange.RequiredOutput.AcceptedOperationVariants)
	}

	facts.Evidence.Accepted = append(facts.Evidence.Accepted, runtimecontract.EvidenceFact{
		ID: "state-change", Kind: runtimecontract.EvidenceKindStateChange, Revision: "runtime-evidence-00000000000000000001",
		CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	})
	facts.EvidenceRevision = "runtime-evidence-00000000000000000001"
	validation, err := reduceRuntimeStateFixture(t, facts, &stateChange.AuthoritySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if validation.State != runtimecontract.ConvergenceStateNeedValidation || len(validation.RequiredOutput.AcceptedOperationVariants) != 0 {
		t.Fatalf("validation required output leaked state-change variants: %+v", validation.RequiredOutput)
	}
}

func TestReduceRuntimeStateDoesNotApplyGitSequenceToFileOnlyFocus(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
	facts.Capabilities.StateChangeKinds = []string{runtimecontract.StepKindFileEdit, runtimecontract.StepKindGitWrite}
	facts.Resources.CapabilityStates = []runtimecontract.RuntimeCapabilityStateSnapshot{{
		RequirementID: "git-state", OwnerModuleID: runtimecontract.RuntimeModuleGit,
		CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable", ObservedRevision: "repo-v1", ContentHash: "content-v1", SnapshotID: "snapshot-v1",
		AcquisitionKey: "acquisition-v1", QueryReceiptRef: "receipt-v1",
		Disposition:               runtimecontract.RuntimeCapabilityStateSequenceRequired,
		RequiredOperationSequence: []runtimecontract.RuntimeRequiredOperationVariant{{RequirementID: "repo-init", Kind: runtimecontract.StepKindGitWrite, VariantID: "init"}},
	}}
	facts = runtimeTestFactsWithCurrentFocusBindings(t, facts, runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: facts.DefinitionRevision})
	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindFileEdit}
	facts.Focus.AcceptedOperationVariants = nil
	facts = runtimeTestFactsWithCanonicalCWIProjection(t, facts)
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedStateChange || len(decision.RequiredOutput.AcceptedKinds) != 1 ||
		decision.RequiredOutput.AcceptedKinds[0] != runtimecontract.StepKindFileEdit || len(decision.RequiredOutput.RequiredOperationSequence) != 0 {
		t.Fatalf("File-only focus leaked Git sequence into required output: %+v", decision.RequiredOutput)
	}

	facts.Focus.AcceptedStepKinds = []string{runtimecontract.StepKindGitWrite}
	facts = runtimeTestFactsWithCanonicalCWIProjection(t, facts)
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
		Activation:  runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
		WorkspaceID: "ws-1", RunID: "run-2", DefinitionRevision: "def-1", EvidenceRevision: "ev-6",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-001", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
			TargetRefs:            []string{"cmd/proxy/main.go"},
		},
		Evidence: runtimecontract.EvidenceLedger{
			Accepted: []runtimecontract.EvidenceFact{{
				ID: "acc-state", Kind: runtimecontract.EvidenceKindStateChange, Revision: "runtime-evidence-00000000000000000001",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			}},
			Rejected: []runtimecontract.EvidenceFact{
				{
					ID: "rej-val", Kind: runtimecontract.EvidenceKindValidation, Revision: "runtime-evidence-00000000000000000002",
					FailureClass: runtimecontract.FailureClassExecutionFailure, FailureMessage: "exit status 2",
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
				},
				{
					ID: "rej-patch", Kind: runtimecontract.EvidenceKindStateChange, Revision: "runtime-evidence-00000000000000000003",
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
	got, err := reduceRuntimeStateFixture(t, facts, nil)
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
	facts.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "accepted-state", Kind: runtimecontract.EvidenceKindStateChange, Revision: "runtime-evidence-00000000000000000001", CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	}}
	facts.Evidence.Rejected = []runtimecontract.EvidenceFact{{
		ID: "rejected-validation", Kind: runtimecontract.EvidenceKindValidation, Revision: "runtime-evidence-00000000000000000002",
		FailureClass: runtimecontract.FailureClassExecutionFailure, FailureMessage: "validation failed", TargetRefs: []string{"validation-entry"}, CoverageWorkItemIDs: facts.Focus.CoverageWorkItemIDs,
	}}
	facts.Failure = &runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionFailure, Signature: "validation-failed", Message: "validation failed", TargetRefs: []string{"validation-entry"}}
	decision, err := reduceRuntimeStateFixture(t, facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("ungrounded validation failure decision = %+v", decision)
	}
	if !reflect.DeepEqual(decision.RequiredOutput.TargetRefs, decision.Focus.TargetRefs) ||
		contains(decision.RequiredOutput.TargetRefs, "validation-entry") {
		t.Fatalf("validation command entry replaced the semantic validation target: output=%+v focus=%+v", decision.RequiredOutput, decision.Focus)
	}
}

func TestReduceRuntimeStateUsesOwnerGroundedFailureForStateChangeRepair(t *testing.T) {
	base := validationFacts()
	base.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	base.Focus.AcceptedStepKinds = []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.RuntimeBlockKindManagedValidationSession}
	base.Capabilities.StateChangeKinds = []string{runtimecontract.RuntimeBlockKindPatchApply}
	base.Capabilities.ValidationKinds = []string{runtimecontract.RuntimeBlockKindManagedValidationSession}
	base.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "accepted-state", Kind: runtimecontract.EvidenceKindStateChange, Revision: "runtime-evidence-00000000000000000001", CoverageWorkItemIDs: base.Focus.CoverageWorkItemIDs,
	}}
	base.Evidence.Rejected = []runtimecontract.EvidenceFact{{
		ID: "rejected-validation", Kind: runtimecontract.EvidenceKindValidation, Revision: "runtime-evidence-00000000000000000002",
		FailureClass: runtimecontract.FailureClassExecutionFailure, FailureMessage: "validation failed", CoverageWorkItemIDs: base.Focus.CoverageWorkItemIDs,
	}}

	ungrounded := base
	ungrounded.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Signature: "build-failed", Message: "build failed", TargetRefs: []string{"implementation.go"},
	}
	decision, err := reduceRuntimeStateFixture(t, ungrounded, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("ungrounded implementation failure decision = %+v, want validation", decision)
	}

	grounded := base
	targetObservation := &runtimecontract.RuntimeFailureTargetObservation{
		OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable",
		TargetRole: "implementation", ObservedRevision: "file-revision-1", TargetRefs: []string{"implementation.go"},
	}
	grounded.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassExecutionFailure, Signature: "build-failed", Message: "build failed", TargetRefs: []string{"implementation.go"},
		TargetObservation: targetObservation,
	}
	grounded.Resources.FailureTarget = &runtimecontract.FailureTarget{Signature: "build-failed", TargetRefs: []string{"implementation.go"}, TargetObservation: targetObservation}
	decision, err = reduceRuntimeStateFixture(t, grounded, nil)
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

func TestOwnerGroundedFailureRepairKindsRequiresCompleteOwnerAuthority(t *testing.T) {
	const prerequisiteID = "operation-admissibility:file-repair"
	const shellPrerequisiteID = "operation-admissibility:shell-repair"
	base := func() runtimecontract.RuntimeFacts {
		return runtimecontract.RuntimeFacts{
			Focus: runtimecontract.CoverageFocus{AcceptedStepKinds: []string{runtimecontract.StepKindMCPToolCall}},
			Failure: &runtimecontract.FailureFact{
				Class: runtimecontract.FailureClassExecutionFailure, Signature: "validation-failed", TargetRefs: []string{"implementation.go"},
			},
			Capabilities: runtimecontract.RuntimeCapabilities{StateChangeKinds: []string{runtimecontract.RuntimeBlockKindPatchApply, runtimecontract.StepKindShellCommand}},
			Prerequisites: runtimecontract.PrerequisiteState{Satisfied: []runtimecontract.RuntimePrerequisite{{
				ID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
				OwnerModuleID: runtimecontract.RuntimeModuleFile, RequiredCapabilities: []string{runtimecontract.RuntimeBlockKindPatchApply},
			}, {
				ID: shellPrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
				OwnerModuleID: runtimecontract.RuntimeModuleShell, RequiredCapabilities: []string{runtimecontract.StepKindShellCommand},
			}}},
			Resources: runtimecontract.RuntimeResourceFacts{
				FailureTarget: &runtimecontract.FailureTarget{
					Signature: "validation-failed", TargetRefs: []string{"implementation.go"},
					TargetObservation: &runtimecontract.RuntimeFailureTargetObservation{
						OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "workspace_deliverable",
						TargetRole: "implementation", ObservedRevision: "file-revision-1", TargetRefs: []string{"implementation.go"},
					},
				},
				OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{{
					PrerequisiteID: prerequisiteID, SnapshotID: "file-repair-catalog", OwnerModuleID: runtimecontract.RuntimeModuleFile,
					OperationKinds: []string{runtimecontract.RuntimeBlockKindPatchApply}, TargetRefs: []string{"implementation.go"}, ObservedRevision: "file-revision-1",
					OperationVariants: []runtimecontract.RuntimeOperationAdmissibilityVariant{{
						Kind: runtimecontract.RuntimeBlockKindPatchApply, VariantID: "file_write",
						AllowedTargetPolicies: []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExactTarget},
					}},
				}, {
					PrerequisiteID: shellPrerequisiteID, SnapshotID: "shell-repair-catalog", OwnerModuleID: runtimecontract.RuntimeModuleShell,
					OperationKinds: []string{runtimecontract.StepKindShellCommand}, TargetRefs: []string{"implementation.go"}, ObservedRevision: "shell-revision-1",
				}},
				PrerequisiteObservations: []runtimecontract.RuntimePrerequisiteObservation{{
					PrerequisiteID: prerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
					OwnerModuleID: runtimecontract.RuntimeModuleFile, ObservationID: "file-repair-observation", ContentHash: "file-repair-catalog",
				}, {
					PrerequisiteID: shellPrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
					OwnerModuleID: runtimecontract.RuntimeModuleShell, ObservationID: "shell-repair-observation", ContentHash: "shell-repair-catalog",
				}},
			},
		}
	}

	facts := base()
	if got := ownerGroundedFailureRepairKinds(facts); len(got) != 1 || got[0] != runtimecontract.RuntimeBlockKindPatchApply {
		t.Fatalf("complete owner repair authority kinds = %v", got)
	}
	decision, err := stateChangeDecision(runtimecontract.RuntimeDecision{}, facts)
	if err != nil {
		t.Fatal(err)
	}
	if got := decision.RequiredOutput.AcceptedKinds; len(got) != 1 || got[0] != runtimecontract.RuntimeBlockKindPatchApply {
		t.Fatalf("owner-grounded decision leaked cross-owner repair kinds = %v", got)
	}
	receiptGrounded := base()
	receiptGrounded.Resources.FailureTarget.TargetObservation = &runtimecontract.RuntimeFailureTargetObservation{
		OwnerModuleID: runtimecontract.RuntimeModuleMCP, SubjectRef: "mcp-observed-target",
		TargetRole: "implementation", ObservedRevision: "mcp-receipt-1", TargetRefs: []string{"implementation.go"},
		CatalogDependency: &runtimecontract.RuntimeFreshnessDependency{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleMCP,
			SubjectRef: "mcp_server_catalog", Revision: "mcp-catalog-1",
		},
	}
	receiptGrounded.Resources.TargetSnapshots = []runtimecontract.TargetSnapshot{{
		TargetRef: "implementation.go", ContentHash: "file-content-1", ContentState: runtimecontract.SnapshotContentStateComplete,
	}}
	receiptGrounded.Prerequisites.Satisfied = append(receiptGrounded.Prerequisites.Satisfied, runtimecontract.RuntimePrerequisite{
		ID: "target-snapshot:implementation.go", Kind: runtimecontract.PrerequisiteKindTargetSnapshot,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "implementation.go",
	})
	receiptGrounded.Resources.PrerequisiteObservations = append(receiptGrounded.Resources.PrerequisiteObservations, runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: "target-snapshot:implementation.go", Kind: runtimecontract.PrerequisiteKindTargetSnapshot,
		OwnerModuleID: runtimecontract.RuntimeModuleFile,
	})
	if got := ownerGroundedFailureRepairKinds(receiptGrounded); len(got) != 1 || got[0] != runtimecontract.RuntimeBlockKindPatchApply {
		t.Fatalf("receipt-evidence target did not resolve to File repair authority: %v", got)
	}
	receiptGrounded.Resources.TargetSnapshots = nil
	if got := ownerGroundedFailureRepairKinds(receiptGrounded); len(got) != 0 {
		t.Fatalf("receipt-evidence target without File snapshot granted repair authority: %v", got)
	}
	for _, test := range []struct {
		name   string
		mutate func(*runtimecontract.RuntimeFacts)
	}{
		{name: "missing satisfied prerequisite", mutate: func(facts *runtimecontract.RuntimeFacts) { facts.Prerequisites.Satisfied = nil }},
		{name: "missing observation", mutate: func(facts *runtimecontract.RuntimeFacts) { facts.Resources.PrerequisiteObservations = nil }},
		{name: "target outside catalog", mutate: func(facts *runtimecontract.RuntimeFacts) {
			facts.Resources.OperationAdmissibilities[0].TargetRefs = []string{"validation-entry.go"}
		}},
		{name: "missing exact target policy", mutate: func(facts *runtimecontract.RuntimeFacts) {
			facts.Resources.OperationAdmissibilities[0].OperationVariants[0].AllowedTargetPolicies = []runtimecontract.RuntimeOperationTargetPolicy{runtimecontract.RuntimeOperationTargetPolicyExistingOwnerScope}
		}},
		{name: "focus rejects repair kind", mutate: func(facts *runtimecontract.RuntimeFacts) {
			facts.Focus.RejectedStepKinds = []string{runtimecontract.RuntimeBlockKindPatchApply}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			facts := base()
			test.mutate(&facts)
			if got := ownerGroundedFailureRepairKinds(facts); len(got) != 0 {
				t.Fatalf("incomplete owner repair authority kinds = %v", got)
			}
		})
	}
}

func TestReduceRuntimeStateAllowsValidationAfterAcceptedRepair(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
		WorkspaceID: "ws-1", RunID: "run-2", DefinitionRevision: "def-1", EvidenceRevision: "ev-4",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-001", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
		},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-001", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Evidence: runtimecontract.EvidenceLedger{
			Accepted: []runtimecontract.EvidenceFact{
				{
					ID: "acc-old", Kind: runtimecontract.EvidenceKindStateChange, Revision: "runtime-evidence-00000000000000000001",
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
				},
				{
					ID: "acc-repair", Kind: runtimecontract.EvidenceKindStateChange, Revision: "runtime-evidence-00000000000000000003",
					CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
				},
			},
			Rejected: []runtimecontract.EvidenceFact{{
				ID: "rej-val", Kind: runtimecontract.EvidenceKindValidation, Revision: "runtime-evidence-00000000000000000002",
				FailureClass: runtimecontract.FailureClassExecutionFailure, FailureMessage: "exit status 2",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			}},
		},
		Capabilities: runtimecontract.RuntimeCapabilities{
			StateChangeKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
			ValidationKinds:  []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		},
	}
	got, err := reduceRuntimeStateFixture(t, facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("action = %s, want produce_validation after accepted repair", got.State)
	}
}

func TestReduceRuntimeStateRejectedValidationWithoutOwnerTargetRemainsValidation(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		Activation:  runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
		WorkspaceID: "ws-1", RunID: "run-2", DefinitionRevision: "def-1", EvidenceRevision: "ev-2",
		Focus: runtimecontract.CoverageFocus{
			ID: "focus-001", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation},
		},
		Coverage: []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-001", State: runtimecontract.RuntimeCoverageItemStateOpen}},
		Evidence: runtimecontract.EvidenceLedger{
			Accepted: []runtimecontract.EvidenceFact{{
				ID: "acc-state", Kind: runtimecontract.EvidenceKindStateChange, Revision: "runtime-evidence-00000000000000000001",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			}},
			Rejected: []runtimecontract.EvidenceFact{{
				ID: "rej-val", Kind: runtimecontract.EvidenceKindValidation, Revision: "runtime-evidence-00000000000000000002",
				FailureClass: runtimecontract.FailureClassExecutionFailure, FailureMessage: "exit status 2",
				CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-001"},
			}},
		},
		Capabilities: runtimecontract.RuntimeCapabilities{
			StateChangeKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
			ValidationKinds:  []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
		},
	}
	got, err := reduceRuntimeStateFixture(t, facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != runtimecontract.ConvergenceStateNeedValidation {
		t.Fatalf("action = %s, want produce_validation for ungrounded validation failure", got.State)
	}
}

func TestReduceRuntimeStatePrerequisiteRefreshExhaustionOverridesPendingPrerequisite(t *testing.T) {
	facts := validationFacts()
	facts.Prerequisites.Pending = []runtimecontract.RuntimePrerequisite{{
		ID: "manifest:blocks/file", Kind: runtimecontract.PrerequisiteKindManifest,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, Reason: "fresh manifest required",
	}}
	facts.Failure = &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClassPrerequisiteRefreshExhausted,
		Code:  "prerequisite_refresh_exhausted", Signature: "refresh-budget:receipt-1",
		Message: "prerequisite refresh budget exhausted after owner resource change",
	}

	decision, err := reduceRuntimeStateFixture(t, facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateFailed || decision.Terminal == nil || decision.Terminal.Code != "prerequisite_refresh_exhausted" {
		t.Fatalf("decision = %+v, want prerequisite refresh terminal", decision)
	}
}

func TestReduceRuntimeStateDeterminedTerminalFailureOverridesPendingPrerequisite(t *testing.T) {
	for _, tt := range []struct {
		name         string
		failureClass runtimecontract.FailureClass
		failureCode  string
		terminalCode string
	}{
		{name: "candidate retry exhausted", failureClass: runtimecontract.FailureClassCandidateExhausted, failureCode: "candidate_retry_exhausted", terminalCode: "candidate_retry_exhausted"},
		{name: "candidate authority exhausted", failureClass: runtimecontract.FailureClassCandidateExhausted, failureCode: "candidate_authority_exhausted", terminalCode: "candidate_authority_exhausted"},
		{name: "candidate exhaustion code is undeclared", failureClass: runtimecontract.FailureClassCandidateExhausted, failureCode: "candidate_other_exhaustion", terminalCode: "runtime_contract_defect"},
		{name: "runtime contract defect", failureClass: runtimecontract.FailureClassRuntimeContractMisalignment, terminalCode: "runtime_contract_defect"},
		{name: "governance rejected", failureClass: runtimecontract.FailureClassGovernanceRejected, terminalCode: "governance_rejected"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			facts := validationFacts()
			facts.Prerequisites.Pending = []runtimecontract.RuntimePrerequisite{{
				ID: "manifest:blocks/file", Kind: runtimecontract.PrerequisiteKindManifest,
				OwnerModuleID: runtimecontract.RuntimeModuleFile, Reason: "fresh manifest required",
			}}
			failureCode := tt.failureCode
			if failureCode == "" {
				failureCode = tt.terminalCode
			}
			facts.Failure = &runtimecontract.FailureFact{
				Class: tt.failureClass, Code: failureCode, Signature: failureCode,
				Message: "determined failure must close the current authority",
			}

			decision, err := reduceRuntimeStateFixture(t, facts, nil)
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
	facts := runtimecontract.RuntimeFacts{
		Activation: runtimecontract.RuntimeActivationContext{
			ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
			EvidencePolicies:  []runtimecontract.RuntimeEvidencePolicy{runtimeManagedValidationEvidencePolicy()},
			ExecutionContract: runtimecontract.RuntimeExecutionContract{StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired},
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
	facts.ConvergencePolicy = runtimecontract.DefaultRuntimeConvergencePolicy()
	facts.RecoveryPolicy = runtimecontract.DefaultRuntimeRecoveryPolicy()
	facts.ExecutionBudgetPolicy = runtimecontract.DefaultRuntimeExecutionBudgetPolicy()
	facts = runtimeTestFactsWithManagedValidationOwnerPreflight(facts)
	// Keep the shared reducer seed pre-semantic: individual fixtures frequently
	// switch this focus between state-change and validation. The canonical test
	// lifecycle helper below reselects the managed recipe/check and accepted kind
	// only when the resulting exact Definition still requires Shell validation.
	facts.Focus.AcceptedStepKinds = nil
	facts.Focus.ValidationRecipeID = ""
	facts.Focus.RequiredCheckRefs = nil
	return facts
}

func TestReduceRuntimeStateRequiresRunLevelStateChangeBeforeComplete(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
	facts.RemainingFocuses = nil
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "val-1", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Revision: "runtime-evidence-00000000000000000001",
	}}
	facts.StateChange = runtimecontract.NewRuntimeStateChangeLedger(runtimecontract.RuntimeRunStateChangeRequirement{
		Required: true, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
	})
	facts.EvidenceRevision = "runtime-evidence-00000000000000000001"
	facts = runtimeTestFactsWithCurrentFocusBindings(t, facts.Normalized(), runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: facts.DefinitionRevision})
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedStateChange {
		t.Fatalf("decision = %+v, want produce_state_change for unsatisfied run ledger", decision)
	}

	// satisfy ledger then complete
	stateEvidence := runtimecontract.EvidenceFact{
		ID: "state-1", Kind: runtimecontract.EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Revision: "runtime-evidence-00000000000000000002", FocusID: facts.Focus.ID,
		BindingRefs: []string{facts.Focus.CoverageEvidenceBindings[0].ID}, TargetRefs: append([]string(nil), facts.Focus.TargetRefs...),
	}
	facts.Evidence.Accepted = append(facts.Evidence.Accepted, stateEvidence)
	facts.StateChange = applyAcceptedStateChangeEvidence(facts.StateChange, stateEvidence, facts.Focus.CoverageWorkItemIDs)
	facts.Assessments = []runtimecontract.RuntimeAssessmentFact{{
		ID: "assessment-run-state-change", Revision: "runtime-evidence-00000000000000000003", FocusID: facts.Focus.ID,
		CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), facts.Focus.CoverageWorkItemIDs...),
		Disposition:         runtimecontract.AssessmentDispositionSatisfied, Summary: "coverage satisfied", EvidenceRefs: []string{"val-1", "state-1"},
	}}
	facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateSatisfied, EvidenceRefs: []string{"val-1", "state-1"}}}
	facts.EvidenceRevision = "runtime-evidence-00000000000000000003"
	facts = runtimeTestFactsWithCanonicalCWIProjection(t, facts.Normalized())
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
		ID: "assessment-repairable", Revision: "runtime-evidence-00000000000000000001", FocusID: facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
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
	decision, err := reduceRuntimeStateFixture(t, facts, nil)
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
		ID: "assessment-terminal", Revision: "runtime-evidence-00000000000000000001", FocusID: facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Disposition: runtimecontract.AssessmentDispositionTerminalRejection,
		Summary:     "irrecoverable acceptance failure",
		Gap: &runtimecontract.RuntimeAssessmentGap{
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
			MissingEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			Reason:               "irrecoverable acceptance failure",
		},
	}}
	facts = facts.Normalized()
	decision, err := reduceRuntimeStateFixture(t, facts, nil)
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
		{ID: "state-1", Kind: runtimecontract.EvidenceKindStateChange, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "runtime-evidence-00000000000000000001"},
		{ID: "val-1", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, Revision: "runtime-evidence-00000000000000000002"},
	}
	// No assessment present (invalidated after repair evidence) => produce_assessment.
	facts.EvidenceRevision = "runtime-evidence-00000000000000000002"
	facts = runtimeTestFactsWithCurrentFocusBindings(t, facts.Normalized(), runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: facts.DefinitionRevision})
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedAssessment {
		t.Fatalf("decision = %+v, want produce_assessment after repair evidence", decision)
	}

	// Satisfied assessment then completes.
	facts.Assessments = []runtimecontract.RuntimeAssessmentFact{{
		ID: "assessment-1", Revision: "runtime-evidence-00000000000000000003", FocusID: facts.Focus.ID, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Disposition: runtimecontract.AssessmentDispositionSatisfied,
		Summary:     "coverage satisfied", EvidenceRefs: []string{"val-1"},
	}}
	facts.StateChange = runtimecontract.NewRuntimeStateChangeLedger(runtimecontract.RuntimeRunStateChangeRequirement{Required: false})
	facts.Coverage = []runtimecontract.RuntimeCoverageItemFact{{ID: "cwi-1", State: runtimecontract.RuntimeCoverageItemStateSatisfied, EvidenceRefs: []string{"val-1"}}}
	facts.EvidenceRevision = "runtime-evidence-00000000000000000003"
	facts = runtimeTestFactsWithCanonicalCWIProjection(t, facts.Normalized())
	decision, err = ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateComplete {
		t.Fatalf("decision = %+v, want complete after satisfied assessment", decision)
	}
}

func TestAssessmentForCurrentFocusRetainsAuditAndUsesRevisionFreshness(t *testing.T) {
	facts := validationFacts()
	facts.Assessments = []runtimecontract.RuntimeAssessmentFact{
		{ID: "assessment-gap", Revision: "runtime-evidence-00000000000000000001", FocusID: facts.Focus.ID, Disposition: runtimecontract.AssessmentDispositionRepairableGap, Summary: "gap"},
		{ID: "assessment-ok", Revision: "runtime-evidence-00000000000000000002", FocusID: facts.Focus.ID, Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "ok"},
		{ID: "assessment-other", Revision: "runtime-evidence-00000000000000000003", FocusID: "focus-2", Disposition: runtimecontract.AssessmentDispositionRepairableGap, Summary: "other"},
	}
	got, found := assessmentForCurrentFocus(facts)
	if !found || got.ID != "assessment-ok" || len(facts.Assessments) != 3 {
		t.Fatalf("latest assessment = %+v found=%t audit=%+v", got, found, facts.Assessments)
	}

	facts = runtimeTestFactsWithCurrentFocusBindings(t, facts, runtimecontract.RuntimeDefinitionContext{
		StartupInputHash: "startup-input-test",
		Revision:         facts.DefinitionRevision,
	})
	bindingRef := facts.Focus.CoverageEvidenceBindings[0].ID
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "validation-new", Kind: runtimecontract.EvidenceKindValidation,
		Revision:    "runtime-evidence-00000000000000000004",
		BindingRefs: []string{bindingRef},
	}}
	if current, ok := assessmentForCurrentFocus(facts); !ok || current.ID != "assessment-ok" {
		t.Fatalf("empty-credit diagnostic evidence invalidated assessment: current=%+v found=%t", current, ok)
	}
	facts.Evidence.Accepted[0].CoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), facts.Focus.CoverageWorkItemIDs...)
	if stale, ok := assessmentForCurrentFocus(facts); ok {
		t.Fatalf("assessment older than accepted evidence remained current: %+v", stale)
	}
	facts.Assessments = append(facts.Assessments, runtimecontract.RuntimeAssessmentFact{
		ID: "assessment-fresh", Revision: "runtime-evidence-00000000000000000005", FocusID: facts.Focus.ID,
		Disposition: runtimecontract.AssessmentDispositionSatisfied, Summary: "fresh",
	})
	if fresh, ok := assessmentForCurrentFocus(facts); !ok || fresh.ID != "assessment-fresh" {
		t.Fatalf("fresh assessment = %+v found=%t", fresh, ok)
	}
}

func TestReduceRuntimeStateRepairBudgetExhausted(t *testing.T) {
	facts := validationFacts()
	facts.Focus.RequireAssessment = true
	facts.ConvergencePolicy = runtimecontract.RuntimeConvergencePolicy{MaxRepairAttemptsPerFocus: 2, MaxValidationOnlyRetriesPerFocus: 1}
	facts = runtimeTestFactsWithCurrentFocusBindings(t, facts, runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: facts.DefinitionRevision})
	bindingRef := facts.Focus.CoverageEvidenceBindings[0].ID
	facts.Repair = runtimecontract.RuntimeRepairLedger{FocusID: facts.Focus.ID, RepairAttempts: 2, Exhausted: false}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "val-1", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Revision: "runtime-evidence-00000000000000000001", FocusID: facts.Focus.ID, BindingRefs: []string{bindingRef},
	}}
	facts.Assessments = []runtimecontract.RuntimeAssessmentFact{{
		ID: "assessment-exhausted", Revision: "runtime-evidence-00000000000000000002", FocusID: facts.Focus.ID, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), facts.Focus.CoverageWorkItemIDs...),
		Disposition: runtimecontract.AssessmentDispositionRepairableGap,
		Summary:     "still broken",
		Gap: &runtimecontract.RuntimeAssessmentGap{
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
			MissingEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			Reason:               "still broken",
		},
	}}
	facts.EvidenceRevision = "runtime-evidence-00000000000000000002"
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
	facts = runtimeTestFactsWithCurrentFocusBindings(t, facts, runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: facts.DefinitionRevision})
	bindingRef := facts.Focus.CoverageEvidenceBindings[0].ID
	facts.Repair = runtimecontract.RuntimeRepairLedger{FocusID: facts.Focus.ID, RepairAttempts: 1, ValidationOnlyRetries: 1}
	facts.Evidence.Accepted = []runtimecontract.EvidenceFact{{
		ID: "val-1", Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		Revision: "runtime-evidence-00000000000000000001", FocusID: facts.Focus.ID, BindingRefs: []string{bindingRef},
	}}
	facts.Assessments = []runtimecontract.RuntimeAssessmentFact{{
		ID: "assessment-validation-cap", Revision: "runtime-evidence-00000000000000000002", FocusID: facts.Focus.ID, CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), facts.Focus.CoverageWorkItemIDs...),
		Disposition: runtimecontract.AssessmentDispositionRepairableGap,
		Summary:     "validation still failing",
		Gap: &runtimecontract.RuntimeAssessmentGap{
			CoverageWorkItemIDs:  []common.CoverageWorkItemID{"cwi-1"},
			MissingEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			Reason:               "validation still failing",
		},
	}}
	facts.EvidenceRevision = "runtime-evidence-00000000000000000002"
	facts = facts.Normalized()
	decision, err := ReduceRuntimeState(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateFailed || decision.Terminal == nil || decision.Terminal.Code != "no_progress_exhausted" {
		t.Fatalf("decision = %+v, want validation-only budget terminal", decision)
	}
}

func TestReduceRuntimeStateValidationOnlyCapCoversSameFocusRepairEntries(t *testing.T) {
	exhaustedRepairFacts := func() runtimecontract.RuntimeFacts {
		facts := validationFacts()
		facts.ConvergencePolicy.MaxValidationOnlyRetriesPerFocus = 1
		facts.Repair = runtimecontract.RuntimeRepairLedger{
			FocusID: facts.Focus.ID, RepairAttempts: 1, ValidationOnlyRetries: 1,
			ActiveGap: &runtimecontract.RuntimeAssessmentGap{
				CoverageWorkItemIDs:  append([]common.CoverageWorkItemID(nil), facts.Focus.CoverageWorkItemIDs...),
				MissingEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
			},
		}
		return facts
	}

	tests := []struct {
		name  string
		facts func() runtimecontract.RuntimeFacts
	}{
		{
			name:  "ordinary missing validation after rejected execution",
			facts: exhaustedRepairFacts,
		},
		{
			name: "persisted validation repair directive",
			facts: func() runtimecontract.RuntimeFacts {
				facts := exhaustedRepairFacts()
				facts.RepairDirective = &runtimecontract.RuntimeRepairDirective{
					ID: "repair-validation-budget", FocusID: facts.Focus.ID,
					Trigger: runtimecontract.RuntimeRepairTriggerExecutionRejected, RequiredState: runtimecontract.ConvergenceStateNeedValidation,
					RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectNone, runtimecontract.RuntimeEffectEphemeralSession},
					RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
					CoverageWorkItemIDs: append([]common.CoverageWorkItemID(nil), facts.Focus.CoverageWorkItemIDs...),
				}
				return facts
			},
		},
		{
			name: "state-change no-op validation observation",
			facts: func() runtimecontract.RuntimeFacts {
				facts := exhaustedRepairFacts()
				facts.Focus.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange, runtimecontract.EvidenceKindValidation}
				facts.Repair.ActiveGap.MissingEvidenceKinds = []string{runtimecontract.EvidenceKindStateChange}
				facts.Repair.NoProgressOperationShapes = []runtimecontract.RuntimeRejectedOperationShape{{
					Operation:    patchApplyTestOperation("no-op-patch", "main.go"),
					FailureClass: runtimecontract.FailureClassEffectMismatch,
					FailureCode:  runtimeStateChangeNoOpWithoutProvenanceCode,
				}}
				return facts
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := test.facts()
			facts := runtimeTestFactsWithCurrentFocusBindings(t, raw, runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: "definition-1"})
			facts.Repair = raw.Repair
			facts.Repair.FocusID = facts.Focus.ID
			if raw.RepairDirective != nil {
				directive := raw.RepairDirective.Normalized()
				directive.FocusID = facts.Focus.ID
				facts.RepairDirective = &directive
			}
			decision, err := ReduceRuntimeState(facts, nil)
			if err != nil {
				t.Fatal(err)
			}
			if decision.State != runtimecontract.ConvergenceStateFailed || decision.Terminal == nil || decision.Terminal.Code != "no_progress_exhausted" {
				t.Fatalf("decision = %+v, want validation-only budget terminal", decision)
			}
		})
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
			RequestedAction: "web/fetch", AuthorizationContext: workorcheventpayload.New(workorcheventpayload.StringField("targetRef", "https://example.com")),
		},
	}
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindApprovalRequest}
	decision, err := reduceRuntimeStateFixture(t, facts, nil)
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
	decision, err := reduceRuntimeStateFixture(t, facts, nil)
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
	facts.Resources.ProviderCeiling = &runtimecontract.ProviderCeiling{Selection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "model-1"}}}
	facts.Capabilities.GovernanceKinds = []string{runtimecontract.RuntimeBlockKindDecisionRequest}
	decision, err := reduceRuntimeStateFixture(t, facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedGovernance {
		t.Fatalf("provider authentication decision = %+v", decision)
	}
}

package grounding

import (
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestTargetRefsContainedAndCatalogScopePolicy(t *testing.T) {
	if !TargetRefsContained([]string{"a", "a"}, []string{"a", "b"}) {
		t.Fatal("duplicate required target should remain contained")
	}
	if TargetRefsContained([]string{"a", "c"}, []string{"a", "b"}) {
		t.Fatal("unknown target was contained")
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "p1", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite},
		SelectorRefs:   []string{"workspace-scope"}, ConstraintRefs: []string{"scope-bound"},
	}
	focus := runtimecontract.CoverageFocus{
		AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite},
	}
	if !CatalogCoversFocusTargets(catalog, focus, []string{"main.go"}) {
		t.Fatal("existing owner scope should cover target")
	}
}

func TestValidationFailureTargetsRequirePriorAuthorityOrFreshOwnerObservation(t *testing.T) {
	if !SameTargetRefs([]string{"b.go", "a.go", "a.go"}, []string{"a.go", "b.go"}) {
		t.Fatal("canonical target-set comparison is order or duplicate sensitive")
	}
	facts := runtimecontract.RuntimeFacts{Focus: runtimecontract.CoverageFocus{
		ID: "focus-1", TargetRole: "implementation", RequireTargetGrounding: true,
	}}
	failure := runtimecontract.FailureFact{TargetRefs: []string{"main.go"}}
	if ValidationFailureTargetsAreGrounded(facts, failure) {
		t.Fatal("diagnostic target without prior authority was grounded")
	}
	facts.Focus.TargetRefs = []string{"main.go"}
	if !ValidationFailureTargetsAreGrounded(facts, failure) {
		t.Fatal("current focus target was not grounded")
	}
	facts.Focus.TargetRefs = nil
	failure.ValidationDiagnostic = &runtimecontract.ValidationFailureDiagnostic{Class: runtimecontract.ValidationDiagnosticBuildFailed}
	failure.TargetObservation = &runtimecontract.RuntimeFailureTargetObservation{
		OwnerModuleID: runtimecontract.RuntimeModuleShell, SubjectRef: "shell_runtime_policy", TargetRole: facts.Focus.TargetRole,
		ObservedRevision: "shell-policy-v1", TargetRefs: []string{"main.go"}, DiagnosticClass: runtimecontract.ValidationDiagnosticBuildFailed,
		EvidenceRefs: []string{"stdout-1"}, ContentHash: strings.Repeat("a", 64),
	}
	if ValidationFailureTargetsAreGrounded(facts, failure) {
		t.Fatal("owner observation without current freshness was grounded")
	}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{{
		PrerequisiteID: "shell-policy", Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: runtimecontract.RuntimeModuleShell, ObservationID: "observation-1", ContentHash: "content-1",
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: runtimecontract.RuntimeModuleShell,
			SubjectRef: "shell-policy", Revision: "shell-policy-v1",
		}},
	}}
	if !ValidationFailureTargetsAreGrounded(facts, failure) {
		t.Fatal("fresh structured Shell owner observation was not grounded")
	}
}

func TestCompareCatalogFreshnessRequiresSnapshotAndObservation(t *testing.T) {
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "p1", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite},
		TargetRefs:     []string{"main.go"}, ObservedRevision: "tree-v2",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	facts := runtimecontract.RuntimeFacts{Resources: runtimecontract.RuntimeResourceFacts{
		OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{catalog},
	}}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		ObservationID: "obs-1", PrerequisiteID: catalog.PrerequisiteID,
		OwnerModuleID: catalog.OwnerModuleID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		ContentHash: catalog.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceOwnerCatalog, OwnerModuleID: catalog.OwnerModuleID,
			SubjectRef: catalog.PrerequisiteID, Revision: catalog.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	if err := observation.Validate(); err != nil {
		t.Fatalf("observation: %v", err)
	}
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	if gaps := CompareCatalogFreshness([]runtimecontract.RuntimeOperationAdmissibility{catalog}, facts); len(gaps) != 0 {
		t.Fatalf("fresh catalog gaps = %+v", gaps)
	}
	observation.ContentHash = "stale"
	facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	if gaps := CompareCatalogFreshness([]runtimecontract.RuntimeOperationAdmissibility{catalog}, facts); len(gaps) != 1 || gaps[0].CurrentObservationID != "" {
		t.Fatalf("stale observation gaps = %+v", gaps)
	}
}

func TestCurrentFocusTargetSelectionRequiresSatisfiedFocusPrerequisite(t *testing.T) {
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-1", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		Disposition: runtimecontract.RuntimeTargetSelectionSelected, TargetRefs: []string{"main.go"},
	}
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "workspace-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus:     runtimecontract.CoverageFocus{ID: "focus-1"},
		Resources: runtimecontract.RuntimeResourceFacts{TargetSelection: &selection},
	}
	if current := CurrentFocusTargetSelection(facts); current != nil {
		t.Fatalf("raw target resource became authority: %+v", current)
	}
	facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{{
		ID: selection.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindTargetSelection,
		Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: facts.Focus.ID,
	}}
	if current := CurrentFocusTargetSelection(facts); current == nil || !TargetSelectionHasConcreteTarget(*current) {
		t.Fatalf("satisfied current selection missing: %+v", current)
	}
	facts.Prerequisites.Satisfied[0].FocusID = "old-focus"
	if current := CurrentFocusTargetSelection(facts); current != nil {
		t.Fatalf("stale focus selection became authority: %+v", current)
	}
}

func TestRepairTargetsUsesOneOwnerGroundedPrecedence(t *testing.T) {
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "workspace-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: runtimecontract.CoverageFocus{ID: "focus-1", TargetRefs: []string{"focus.go"}},
		Resources: runtimecontract.RuntimeResourceFacts{FailureTarget: &runtimecontract.FailureTarget{
			Signature: "failure-v1", TargetRefs: []string{"failure.go"},
		}},
		RepairDirective: &runtimecontract.RuntimeRepairDirective{FocusID: "focus-1", TargetRefs: []string{"repair.go"}},
	}
	if targets := RepairTargets(facts); len(targets) != 1 || targets[0] != "failure.go" {
		t.Fatalf("failure target precedence = %v", targets)
	}
	facts.Resources.FailureTarget = nil
	if targets := RepairTargets(facts); len(targets) != 1 || targets[0] != "focus.go" {
		t.Fatalf("focus target precedence = %v", targets)
	}
	facts.Focus.TargetRefs = nil
	if targets := RepairTargets(facts); len(targets) != 1 || targets[0] != "repair.go" {
		t.Fatalf("repair directive fallback = %v", targets)
	}
}

func TestOwnerGroundedCreationAndWorkspaceScopeRemainBounded(t *testing.T) {
	focus := runtimecontract.CoverageFocus{
		ID: "focus-1", TargetRole: "deliverable", TargetSelectorRef: "file/workspace_deliverable",
		TargetSelectorOwnerRef: "profile-1", TargetSelectorSubjectRef: "workspace-root",
		TargetSelectorCandidateRefs: []string{"candidate-a"}, ProspectiveTargetRef: "new.go",
	}
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "target-selection-1", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: focus.TargetRole, SelectorRef: focus.TargetSelectorRef,
		SelectorOwnerRef: focus.TargetSelectorOwnerRef, SelectorSubjectRef: focus.TargetSelectorSubjectRef,
		SelectorCandidateRefs: focus.TargetSelectorCandidateRefs,
		Disposition:           runtimecontract.RuntimeTargetSelectionCreate, ProspectiveTargetRef: focus.ProspectiveTargetRef,
	}
	facts := runtimecontract.RuntimeFacts{
		WorkspaceID: "workspace-1", RunID: "run-1", DefinitionRevision: "definition-1", EvidenceRevision: "evidence-1",
		Focus: focus, Resources: runtimecontract.RuntimeResourceFacts{TargetSelection: &selection},
		Prerequisites: runtimecontract.PrerequisiteState{Satisfied: []runtimecontract.RuntimePrerequisite{{
			ID: selection.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindTargetSelection,
			Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: focus.ID,
		}}},
	}
	if !HasOwnerGroundedCreationScope(facts) {
		t.Fatal("matching File create observation was not grounded")
	}
	if targets := RepairTargets(facts); len(targets) != 1 || targets[0] != "new.go" {
		t.Fatalf("creation repair targets = %v", targets)
	}
	selection.Disposition = runtimecontract.RuntimeTargetSelectionScope
	selection.ProspectiveTargetRef = ""
	selection.TargetRefs = []string{"go.mod", "go.sum"}
	selection.CandidateTargetRefs = []string{"go.mod", "go.sum"}
	selection.CandidateCount = 2
	facts.Resources.TargetSelection = &selection
	if !HasOwnerGroundedTargetScope(facts) || !OwnerGroundedTargetScopeMaySelectNewFileTarget(facts) {
		t.Fatalf("bounded workspace scope was not recognized: %+v", facts.Resources.TargetSelection)
	}
	if !TargetRefsAllWithin([]string{"go.sum"}, WorkspaceScopeTargets(&selection)) ||
		TargetRefsAllWithin([]string{"go.sum", "outside.go"}, WorkspaceScopeTargets(&selection)) {
		t.Fatal("workspace scope membership was not exact")
	}
	facts.Failure = &runtimecontract.FailureFact{TargetRefs: []string{"go.mod"}}
	if OwnerGroundedTargetScopeMaySelectNewFileTarget(facts) {
		t.Fatal("workspace scope bypassed a higher-precedence failure target")
	}
}

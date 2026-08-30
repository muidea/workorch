package runtime

import (
	"fmt"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPresemanticDefinitionTargetSelectionRequirementUsesImmutableDeclaration(t *testing.T) {
	facts := validationFacts()
	facts.Focus = runtimecontract.CoverageFocus{
		ID: "focus-presemantic", SliceID: "slice-presemantic",
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
	}
	facts.DefinitionFocuses = []runtimecontract.RuntimeDefinitionFocus{{
		ID: "focus-presemantic", SliceID: "slice-presemantic",
		CoverageWorkItemIDs:   []common.CoverageWorkItemID{"cwi-1"},
		RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		TargetRole:            "deliverable",
		TargetDiscoverySources: []string{
			runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest,
		},
		TargetSelectorRef:           "profile/declaration-selector",
		TargetSelectorOwnerRef:      "profile/test",
		TargetSelectorSubjectRef:    "deliverable",
		TargetSelectorCandidateRefs: []string{"allowed.go"},
		ProspectiveTargetAllOfConstraintRefs: []string{
			"workspace-relative-path",
			"new-target-requires-owner-selector",
		},
		RequireTargetGrounding: true,
	}}
	facts.Prerequisites = runtimecontract.PrerequisiteState{}
	facts.Resources.Manifest = nil
	facts.Resources.TargetSelection = nil

	updated, decision, err := ReduceRuntimeStateWithDirective(facts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.State != runtimecontract.ConvergenceStateNeedPrerequisite {
		t.Fatalf("presemantic target declaration decision=%+v, want owner prerequisite", decision)
	}
	var selection *runtimecontract.RuntimePrerequisite
	for index := range updated.Prerequisites.Pending {
		value := &updated.Prerequisites.Pending[index]
		if value.Kind == runtimecontract.PrerequisiteKindTargetSelection {
			selection = value
			break
		}
	}
	if selection == nil || selection.Scope != runtimecontract.RuntimePrerequisiteScopeFocus || selection.FocusID != facts.Focus.ID ||
		selection.OwnerModuleID != runtimecontract.RuntimeModuleFile || selection.SubjectRef != "deliverable" ||
		selection.TargetSelectorRef != "profile/declaration-selector" || selection.TargetSelectorOwnerRef != "profile/test" ||
		selection.TargetSelectorSubjectRef != "deliverable" ||
		!runtimecontract.SameRuntimeTargetSelectorCandidateRefs(selection.TargetSelectorCandidateRefs, []string{"allowed.go"}) ||
		!runtimecontract.SameRuntimeProspectiveTargetAllOfConstraintRefs(selection.ProspectiveTargetAllOfConstraintRefs, []string{"new-target-requires-owner-selector", "workspace-relative-path"}) {
		t.Fatalf("presemantic target-selection prerequisite=%+v", selection)
	}
	if updated.Focus.TargetSelectorRef != "" || updated.Focus.RequireTargetGrounding {
		t.Fatalf("immutable target declaration leaked into presemantic focus cursor: %+v", updated.Focus)
	}
}

func TestPresemanticTargetSelectionOwnerFactFencesSemanticRequest(t *testing.T) {
	boundedCandidates := []string{"allowed.go"}
	transient := presemanticTargetSelectionGateAggregate(t, "", boundedCandidates)
	if !SemanticPlanningPrerequisitesPending(transient.Facts.Prerequisites.Pending) {
		t.Fatal("binding-free target selection was not recognized as a semantic prerequisite")
	}
	if err := candidateRequestSessionActionAllowed(transient, runtimecontract.RuntimeProviderDocumentSemanticPlan); err == nil {
		t.Fatal("semantic request opened before the typed target-selection owner fact")
	}

	for _, disposition := range []runtimecontract.RuntimeTargetSelectionDisposition{
		runtimecontract.RuntimeTargetSelectionAbsent,
		runtimecontract.RuntimeTargetSelectionAmbiguous,
		runtimecontract.RuntimeTargetSelectionUnsupported,
	} {
		t.Run(string(disposition), func(t *testing.T) {
			aggregate := presemanticTargetSelectionGateAggregate(t, disposition, boundedCandidates)
			if !CurrentFocusTargetSelectionConfirmsUnresolved(aggregate.Facts) {
				t.Fatalf("exact current-focus owner selection %q was not recognized", disposition)
			}
			if err := candidateRequestSessionActionAllowed(aggregate, runtimecontract.RuntimeProviderDocumentSemanticPlan); err == nil {
				t.Fatalf("stable target-grounding terminal %q reopened semantic provider", disposition)
			}
		})
	}

	boundedEmptyScope := presemanticTargetSelectionGateAggregate(t, runtimecontract.RuntimeTargetSelectionScope, boundedCandidates)
	if CurrentFocusTargetSelectionConfirmsUnresolved(boundedEmptyScope.Facts) {
		t.Fatal("empty current owner scope hid a bounded prospective declaration candidate")
	}
	if err := candidateRequestSessionActionAllowed(boundedEmptyScope, runtimecontract.RuntimeProviderDocumentSemanticPlan); err != nil {
		t.Fatalf("bounded prospective declaration did not continue semantic planning: %v", err)
	}

	allOfEmptyScope := presemanticTargetSelectionGateAggregate(
		t,
		runtimecontract.RuntimeTargetSelectionScope,
		nil,
		"workspace-relative-path",
		"new-target-requires-owner-selector",
	)
	if CurrentFocusTargetSelectionConfirmsUnresolved(allOfEmptyScope.Facts) {
		t.Fatal("empty owner scope hid an owner-accepted all-of prospective declaration")
	}
	if err := candidateRequestSessionActionAllowed(allOfEmptyScope, runtimecontract.RuntimeProviderDocumentSemanticPlan); err != nil {
		t.Fatalf("owner-accepted all-of declaration did not continue semantic planning: %v", err)
	}

	unboundedEmptyScope := presemanticTargetSelectionGateAggregate(t, runtimecontract.RuntimeTargetSelectionScope, nil)
	if !CurrentFocusTargetSelectionConfirmsUnresolved(unboundedEmptyScope.Facts) {
		t.Fatal("empty owner scope without a declaration candidate was not recognized as unresolved")
	}
	if err := candidateRequestSessionActionAllowed(unboundedEmptyScope, runtimecontract.RuntimeProviderDocumentSemanticPlan); err == nil {
		t.Fatal("unbounded empty owner scope reopened semantic provider")
	}

	selected := presemanticTargetSelectionGateAggregate(t, runtimecontract.RuntimeTargetSelectionSelected, boundedCandidates)
	if CurrentFocusTargetSelectionConfirmsUnresolved(selected.Facts) {
		t.Fatal("selected owner target was classified as unresolved")
	}
	if err := candidateRequestSessionActionAllowed(selected, runtimecontract.RuntimeProviderDocumentSemanticPlan); err != nil {
		t.Fatalf("selected target did not continue semantic planning: %v", err)
	}
}

func presemanticTargetSelectionGateAggregate(t *testing.T, disposition runtimecontract.RuntimeTargetSelectionDisposition, selectorCandidates []string, prospectiveAllOf ...string) RuntimeAggregate {
	t.Helper()
	const (
		focusID      = "focus-presemantic-gate"
		prerequisite = "target-selection-presemantic-gate"
		observed     = "workspace-tree-presemantic-gate"
	)
	facts := runtimecontract.RuntimeFacts{
		Focus: runtimecontract.CoverageFocus{
			ID: focusID, SliceID: "slice-presemantic-gate", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-gate"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
		},
		DefinitionFocuses: []runtimecontract.RuntimeDefinitionFocus{{
			ID: focusID, SliceID: "slice-presemantic-gate", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-gate"},
			RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRole: "deliverable",
			TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
			TargetSelectorRef:      "profile/declaration-selector", TargetSelectorOwnerRef: "profile/test", TargetSelectorSubjectRef: "deliverable",
			TargetSelectorCandidateRefs: append([]string(nil), selectorCandidates...), ProspectiveTargetAllOfConstraintRefs: append([]string(nil), prospectiveAllOf...), RequireTargetGrounding: true,
		}},
	}
	selectionPrerequisite := runtimecontract.RuntimePrerequisite{
		ID: prerequisite, Kind: runtimecontract.PrerequisiteKindTargetSelection,
		Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: focusID,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "deliverable",
		TargetSelectorRef: "profile/declaration-selector", TargetSelectorOwnerRef: "profile/test", TargetSelectorSubjectRef: "deliverable",
		TargetSelectorCandidateRefs: append([]string(nil), selectorCandidates...), ProspectiveTargetAllOfConstraintRefs: append([]string(nil), prospectiveAllOf...),
	}
	aggregate := RuntimeAggregate{
		Facts:            facts,
		Decision:         runtimecontract.RuntimeDecision{State: runtimecontract.ConvergenceStateNeedPrerequisite},
		SemanticPlanning: runtimecontract.RuntimeSemanticPlanningLedger{Phase: runtimecontract.RuntimeSemanticPlanningRequired},
	}
	if disposition == "" {
		aggregate.Facts.Prerequisites.Pending = []runtimecontract.RuntimePrerequisite{selectionPrerequisite}
		return aggregate
	}

	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: prerequisite, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "profile/declaration-selector", SelectorOwnerRef: "profile/test", SelectorSubjectRef: "deliverable",
		SelectorCandidateRefs: append([]string(nil), selectorCandidates...), ProspectiveTargetAllOfConstraintRefs: append([]string(nil), prospectiveAllOf...), Disposition: disposition, ObservedRevision: observed,
	}
	switch disposition {
	case runtimecontract.RuntimeTargetSelectionSelected:
		selection.TargetRefs = []string{"allowed.go"}
		selection.CandidateTargetRefs = []string{"allowed.go"}
		selection.CandidateCount = 1
	case runtimecontract.RuntimeTargetSelectionAmbiguous:
		selection.CandidateTargetRefs = []string{"first.go", "second.go"}
		selection.CandidateCount = 2
	case runtimecontract.RuntimeTargetSelectionScope:
		selection.CandidateCount = 0
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	if err := selection.Validate(); err != nil {
		t.Fatalf("selection fixture %q: %v", disposition, err)
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite, Kind: runtimecontract.PrerequisiteKindTargetSelection,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, ContentHash: selection.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace", Revision: observed,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	aggregate.Facts.Prerequisites.Satisfied = []runtimecontract.RuntimePrerequisite{selectionPrerequisite}
	aggregate.Facts.Resources.TargetSelection = &selection
	aggregate.Facts.Resources.PrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}
	if disposition != runtimecontract.RuntimeTargetSelectionSelected {
		aggregate.Decision = runtimecontract.RuntimeDecision{
			State: runtimecontract.ConvergenceStateFailed,
			Terminal: &runtimecontract.RuntimeTerminalDecision{
				Code: "target_grounding_unresolved", Summary: fmt.Sprintf("owner selection %s", disposition),
			},
		}
	}
	return aggregate
}

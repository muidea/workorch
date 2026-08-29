package orchestrator

import (
	"testing"

	runtime "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/runtime"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeTargetGroundingTerminalRequiresExactCurrentOwnerSelection(t *testing.T) {
	boundedCandidates := []string{"allowed.go"}
	transient := runtimeTargetSelectionTerminalAggregate(t, "", boundedCandidates)
	if !runtimeSemanticPrerequisitesPending(transient.Facts.Prerequisites.Pending) {
		t.Fatal("unobserved target selection was not kept on the typed prerequisite path")
	}
	if runtimeTerminalDeliveryReady(transient) {
		t.Fatal("binding-free target-grounding terminal escaped before owner selection")
	}

	for _, disposition := range []runtimecontract.RuntimeTargetSelectionDisposition{
		runtimecontract.RuntimeTargetSelectionAbsent,
		runtimecontract.RuntimeTargetSelectionAmbiguous,
		runtimecontract.RuntimeTargetSelectionUnsupported,
	} {
		t.Run(string(disposition), func(t *testing.T) {
			aggregate := runtimeTargetSelectionTerminalAggregate(t, disposition, boundedCandidates)
			if !runtimeTerminalDeliveryReady(aggregate) {
				t.Fatalf("owner-confirmed unresolved disposition %q remained transient", disposition)
			}
		})
	}

	boundedEmptyScope := runtimeTargetSelectionTerminalAggregate(t, runtimecontract.RuntimeTargetSelectionScope, boundedCandidates)
	if runtimeTerminalDeliveryReady(boundedEmptyScope) {
		t.Fatal("bounded prospective declaration candidate was hidden by an empty current owner scope")
	}
	unboundedEmptyScope := runtimeTargetSelectionTerminalAggregate(t, runtimecontract.RuntimeTargetSelectionScope, nil)
	if !runtimeTerminalDeliveryReady(unboundedEmptyScope) {
		t.Fatal("empty owner scope without a bounded declaration candidate remained transient")
	}

	selected := runtimeTargetSelectionTerminalAggregate(t, runtimecontract.RuntimeTargetSelectionSelected, boundedCandidates)
	if runtimeTerminalDeliveryReady(selected) {
		t.Fatal("selected target incorrectly stabilized a target-grounding terminal")
	}
}

func runtimeTargetSelectionTerminalAggregate(t *testing.T, disposition runtimecontract.RuntimeTargetSelectionDisposition, selectorCandidates []string) runtime.RuntimeAggregate {
	t.Helper()
	const (
		focusID      = "focus-target-terminal"
		prerequisite = "target-selection-terminal"
		observed     = "workspace-tree-terminal"
	)
	selectionPrerequisite := runtimecontract.RuntimePrerequisite{
		ID: prerequisite, Kind: runtimecontract.PrerequisiteKindTargetSelection,
		Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: focusID,
		OwnerModuleID: runtimecontract.RuntimeModuleFile, SubjectRef: "deliverable",
		TargetSelectorRef: "profile/declaration-selector", TargetSelectorOwnerRef: "profile/test", TargetSelectorSubjectRef: "deliverable",
		TargetSelectorCandidateRefs: append([]string(nil), selectorCandidates...),
	}
	aggregate := runtime.RuntimeAggregate{
		Facts: runtimecontract.RuntimeFacts{
			Focus: runtimecontract.CoverageFocus{
				ID: focusID, SliceID: "slice-target-terminal", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-terminal"},
				RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
			},
			DefinitionFocuses: []runtimecontract.RuntimeDefinitionFocus{{
				ID: focusID, SliceID: "slice-target-terminal", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-terminal"},
				RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, TargetRole: "deliverable",
				TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
				TargetSelectorRef:      "profile/declaration-selector", TargetSelectorOwnerRef: "profile/test", TargetSelectorSubjectRef: "deliverable",
				TargetSelectorCandidateRefs: append([]string(nil), selectorCandidates...), RequireTargetGrounding: true,
			}},
		},
		Decision: runtimecontract.RuntimeDecision{
			State: runtimecontract.ConvergenceStateFailed,
			Terminal: &runtimecontract.RuntimeTerminalDecision{
				Code: "target_grounding_unresolved", Summary: "target grounding is unresolved",
			},
		},
		SemanticPlanning: runtimecontract.RuntimeSemanticPlanningLedger{Phase: runtimecontract.RuntimeSemanticPlanningRequired},
	}
	if disposition == "" {
		aggregate.Facts.Prerequisites.Pending = []runtimecontract.RuntimePrerequisite{selectionPrerequisite}
		return aggregate
	}

	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: prerequisite, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "profile/declaration-selector", SelectorOwnerRef: "profile/test", SelectorSubjectRef: "deliverable",
		SelectorCandidateRefs: append([]string(nil), selectorCandidates...), Disposition: disposition, ObservedRevision: observed,
	}
	switch disposition {
	case runtimecontract.RuntimeTargetSelectionSelected:
		selection.TargetRefs = []string{"allowed.go"}
		selection.CandidateTargetRefs = []string{"allowed.go"}
		selection.CandidateCount = 1
	case runtimecontract.RuntimeTargetSelectionAmbiguous:
		selection.CandidateTargetRefs = []string{"first.go", "second.go"}
		selection.CandidateCount = 2
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
	return aggregate
}

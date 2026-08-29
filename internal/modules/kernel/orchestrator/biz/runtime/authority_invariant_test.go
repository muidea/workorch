package runtime

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestInspectRuntimeAuthorityInvariantsReportsHistoricalSelectionAsFirstBadRevision(t *testing.T) {
	valid := stateChangeAggregate(t)
	valid.StateRevision = 11
	invalid := valid
	invalid.StateRevision = 12
	selection := runtimecontract.RuntimeTargetSelection{
		PrerequisiteID: "selection-old", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		TargetRole: "deliverable", SelectorRef: "file/profile_declared_candidates",
		Disposition: runtimecontract.RuntimeTargetSelectionSelected, TargetRefs: []string{"go.mod"},
		CandidateTargetRefs: []string{"go.mod"}, CandidateCount: 1, ObservedRevision: "tree-old",
	}
	selection.SnapshotID = runtimecontract.StableRuntimeTargetSelectionSnapshotID(selection)
	invalid.Facts.Resources.TargetSelection = &selection
	invalid.Facts.Prerequisites.Satisfied = append(invalid.Facts.Prerequisites.Satisfied, runtimecontract.RuntimePrerequisite{
		ID: selection.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindTargetSelection,
		Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: "focus-old", OwnerModuleID: runtimecontract.RuntimeModuleFile,
	})

	report := InspectRuntimeAuthorityInvariants(invalid)
	if report.FocusID != invalid.Facts.Focus.ID || report.TargetSelectionPrerequisiteID != "selection-old" ||
		len(report.Violations) != 2 || report.Violations[0].Code != RuntimeAuthorityInvariantActiveFocusScope ||
		report.Violations[1].Code != RuntimeAuthorityInvariantSelectionBinding {
		t.Fatalf("authority invariant report = %+v", report)
	}
	first := FirstRuntimeAuthorityInvariantViolation([]RuntimeAggregate{invalid, valid})
	if first == nil || first.StateRevision != 12 || first.Code != RuntimeAuthorityInvariantActiveFocusScope || first.PrerequisiteID != "selection-old" {
		t.Fatalf("first authority invariant violation = %+v", first)
	}
}

func TestInspectRuntimeAuthorityInvariantsSeparatesRetiredObservationFromActiveFreshness(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	aggregate.StateRevision = 15
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: "selection-retired", Kind: runtimecontract.PrerequisiteKindTargetSelection,
		Scope: runtimecontract.RuntimePrerequisiteScopeFocus, FocusID: "focus-old", OwnerModuleID: runtimecontract.RuntimeModuleFile,
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: prerequisite.ID, Kind: prerequisite.Kind, OwnerModuleID: prerequisite.OwnerModuleID,
		ContentHash: "selection-hash", Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: "workspace_deliverable", Revision: "tree-old",
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	aggregate.Facts.Prerequisites.Retired = []runtimecontract.RuntimePrerequisite{prerequisite}
	aggregate.Facts.Resources.RetiredPrerequisiteObservations = []runtimecontract.RuntimePrerequisiteObservation{observation}

	report := InspectRuntimeAuthorityInvariants(aggregate)
	if len(report.Violations) != 0 || len(report.RetiredPrerequisiteIDs) != 1 || len(report.RetiredObservationIDs) != 1 ||
		len(report.ActivePrerequisiteObservationIDs) != len(aggregate.Facts.Resources.PrerequisiteObservations) {
		t.Fatalf("retired authority report = %+v", report)
	}
}

func TestInspectRuntimeAuthorityInvariantsReportsMissingSealedProviderCeilingBeforeCandidate(t *testing.T) {
	aggregate := stateChangeAggregate(t)
	requirement := runtimecontract.RuntimeRequirement{
		Kind: runtimecontract.PrerequisiteKindProviderCeiling, OwnerModuleID: runtimecontract.RuntimeModuleLLM,
		MinimumBytes: 96 * 1024, Reason: "provider ceiling required",
	}
	requirement = normalizeRuntimeRequirements([]runtimecontract.RuntimeRequirement{requirement})[0]
	prerequisite := runtimecontract.RuntimePrerequisite{
		ID: runtimePrerequisiteID(requirement), Kind: requirement.Kind, OwnerModuleID: requirement.OwnerModuleID,
		MinimumBytes: requirement.MinimumBytes, Reason: requirement.Reason,
	}
	aggregate.Facts.Prerequisites.Satisfied = append(aggregate.Facts.Prerequisites.Satisfied, prerequisite)
	aggregate.Facts.Resources.ProviderCeiling = nil

	report := InspectRuntimeAuthorityInvariants(aggregate)
	if len(report.Violations) != 1 || report.Violations[0].Code != RuntimeAuthorityInvariantProviderCeiling ||
		report.Violations[0].PrerequisiteID != prerequisite.ID {
		t.Fatalf("provider-ceiling authority invariant report = %+v", report)
	}
}

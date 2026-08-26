package runtime

import (
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// SM-3 must retain the exact SM-2 feasibility catalog identity; otherwise a
// restart could silently substitute another valid catalog for the operation.
func TestSealedExecutionRetainsFeasibilityOwnerCatalogSnapshot(t *testing.T) {
	query := "owner catalog binding"
	operation := runtimecontract.RuntimeOperation{
		ID: "web-search", Kind: runtimecontract.StepKindWebSearch,
		Parameters: []runtimecontract.OperationParameter{{Name: "query", String: &query}},
	}
	candidate := runtimecontract.RuntimeCandidate{Validation: &runtimecontract.ValidationCandidate{Operations: []runtimecontract.ValidationOperation{{Operation: operation}}}}
	records := sealOperationExecutionRecords("ws-1", "run-1", "dispatch-1", candidate, runtimecontract.RuntimeActivationContext{}, []runtimecontract.RuntimeCandidateFeasibilityRecord{{
		OperationID: operation.ID, AdmissibilitySnapshotID: "catalog-snapshot-2", AcquisitionBinding: "acquisition-2",
		Observation: &runtimecontract.RuntimeOwnerFeasibilityObservation{Outcome: runtimecontract.RuntimeOwnerFeasibilityFeasible},
	}})
	if len(records) != 1 || records[0].AdmissibilitySnapshotID != "catalog-snapshot-2" || records[0].AcquisitionBinding != "acquisition-2" {
		t.Fatalf("sealed execution lost feasibility catalog identity: %+v", records)
	}
}

func TestRuntimeOwnerCatalogSelectionUsesSealedFeasibilitySnapshot(t *testing.T) {
	query := "owner catalog binding"
	operation := runtimecontract.RuntimeOperation{
		ID: "web-search", Kind: runtimecontract.StepKindWebSearch,
		Parameters: []runtimecontract.OperationParameter{{Name: "query", String: &query}},
	}
	first, firstObservation := runtimeWebCatalogFixture(t, "catalog-1", "web-revision-1")
	second, secondObservation := runtimeWebCatalogFixture(t, "catalog-2", "web-revision-2")
	selected := runtimeOwnerCatalogForOperation(
		[]runtimecontract.RuntimeOperationAdmissibility{first, second},
		[]runtimecontract.RuntimePrerequisiteObservation{firstObservation, secondObservation},
		operation,
		second.SnapshotID,
		second.AcquisitionBinding,
	)
	if selected == nil || selected.SnapshotID != second.SnapshotID || selected.ObservedRevision != second.ObservedRevision {
		t.Fatalf("execution substituted owner catalog: selected=%+v want=%+v", selected, second)
	}
	if selected := runtimeOwnerCatalogForOperation(
		[]runtimecontract.RuntimeOperationAdmissibility{first, second},
		[]runtimecontract.RuntimePrerequisiteObservation{firstObservation, secondObservation},
		operation,
		"missing-feasibility-snapshot",
		second.AcquisitionBinding,
	); selected != nil {
		t.Fatalf("missing feasibility catalog did not fail closed: %+v", selected)
	}
	if selected := runtimeOwnerCatalogForOperation(
		[]runtimecontract.RuntimeOperationAdmissibility{first, second},
		[]runtimecontract.RuntimePrerequisiteObservation{firstObservation, secondObservation},
		operation,
		second.SnapshotID,
		"tampered-acquisition-binding",
	); selected != nil {
		t.Fatalf("mismatched feasibility acquisition binding did not fail closed: %+v", selected)
	}
	if selected := runtimeOwnerCatalogForOperation(
		[]runtimecontract.RuntimeOperationAdmissibility{first, second},
		[]runtimecontract.RuntimePrerequisiteObservation{firstObservation, secondObservation},
		operation,
		"",
		"",
	); selected != nil {
		t.Fatalf("missing sealed feasibility snapshot selected current catalog: %+v", selected)
	}
	invalidObservation := secondObservation
	invalidObservation.ObservationID = ""
	if selected := runtimeOwnerCatalogForOperation(
		[]runtimecontract.RuntimeOperationAdmissibility{second},
		[]runtimecontract.RuntimePrerequisiteObservation{invalidObservation},
		operation,
		second.SnapshotID,
		second.AcquisitionBinding,
	); selected != nil {
		t.Fatalf("invalid persisted catalog observation authorized execution: %+v", selected)
	}
}

func runtimeWebCatalogFixture(t *testing.T, prerequisiteID, revision string) (runtimecontract.RuntimeOperationAdmissibility, runtimecontract.RuntimePrerequisiteObservation) {
	t.Helper()
	catalog, err := runtimecontract.NewRuntimeOperationAdmissibility(
		prerequisiteID, runtimecontract.RuntimeModuleWeb, []string{runtimecontract.StepKindWebSearch}, nil, []string{"provider"},
		[]string{"owner-execution-policy"}, revision,
	)
	if err != nil {
		t.Fatal(err)
	}
	catalog.OperationPolicies = []runtimecontract.RuntimeOperationPolicyFact{{
		ID: "web-search-policy", OwnerModuleID: runtimecontract.RuntimeModuleWeb, OperationKind: runtimecontract.StepKindWebSearch,
		SubjectRef: "provider", EffectScope: runtimecontract.RuntimeEffectNone,
		IdempotencyClass: runtimecontract.RuntimeOperationIdempotencyAtMostOnce,
	}}
	catalog.AcquisitionBinding = "capability-state/" + prerequisiteID
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: catalog.OwnerModuleID, ContentHash: catalog.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceProviderConfig, OwnerModuleID: runtimecontract.RuntimeModuleWeb,
			SubjectRef: "configured_web_providers", Revision: catalog.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	return catalog, observation
}

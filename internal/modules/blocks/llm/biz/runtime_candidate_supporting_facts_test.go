package llm

import (
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeCandidateSupportingFactsAcceptImplementationWorkCatalogObservation(t *testing.T) {
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "work-file-catalog", OwnerModuleID: runtimecontract.RuntimeModuleFile,
		OperationKinds: []string{runtimecontract.StepKindFileWrite}, TargetRefs: []string{"target.txt"}, ObservedRevision: "tree-1",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	observation := runtimecontract.RuntimePrerequisiteObservation{
		PrerequisiteID: catalog.PrerequisiteID, Kind: runtimecontract.PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: catalog.OwnerModuleID, ContentHash: catalog.SnapshotID,
		Dependencies: []runtimecontract.RuntimeFreshnessDependency{{
			Domain: runtimecontract.RuntimeResourceWorkspaceTree, OwnerModuleID: catalog.OwnerModuleID,
			SubjectRef: "workspace_deliverable", Revision: catalog.ObservedRevision,
		}},
	}
	observation.ObservationID = runtimecontract.StableRuntimePrerequisiteObservationID(observation)
	manifest := runtimecontract.RuntimeCandidateContractManifest{Operations: []runtimecontract.RuntimeCandidateOperationManifest{{
		Kind: runtimecontract.StepKindFileWrite, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		PrerequisiteSnapshotRefs: []string{observation.ObservationID}, AdmissibilitySnapshotRefs: []string{catalog.SnapshotID},
	}}}
	resources := runtimecontract.RuntimeResourceFacts{
		OperationAdmissibilities: []runtimecontract.RuntimeOperationAdmissibility{catalog},
		PrerequisiteObservations: []runtimecontract.RuntimePrerequisiteObservation{observation},
	}
	if err := validateRuntimeCandidateManifestSupportingFacts(manifest, resources); err != nil {
		t.Fatalf("implementation-work supporting facts rejected: %v", err)
	}

	missing := resources
	missing.PrerequisiteObservations = nil
	if err := validateRuntimeCandidateManifestSupportingFacts(manifest, missing); err == nil || !strings.Contains(err.Error(), "is missing") {
		t.Fatalf("missing implementation-work observation error = %v", err)
	}

	drifted := resources
	drifted.PrerequisiteObservations = append([]runtimecontract.RuntimePrerequisiteObservation(nil), resources.PrerequisiteObservations...)
	drifted.PrerequisiteObservations[0].Dependencies[0].Revision = "tree-2"
	if err := validateRuntimeCandidateManifestSupportingFacts(manifest, drifted); err == nil || !strings.Contains(err.Error(), "is invalid") {
		t.Fatalf("drifted implementation-work observation error = %v", err)
	}
}

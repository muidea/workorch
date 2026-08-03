package workorchruntimecontract

import "testing"

func TestRuntimeOperationAdmissibilityAcquisitionRejectsScopeAndObservationDrift(t *testing.T) {
	request := RuntimeOperationAdmissibilityRequest{
		WorkspaceID: "ws-1", RunID: "run-1", AuthoritySnapshotID: "authority-1",
		PrerequisiteID: "admissibility-1", OwnerModuleID: RuntimeModuleFile,
		WorkspaceDeliverableDirectory: t.TempDir(), RequiredKinds: []string{StepKindFileWrite}, MaxItems: 4,
	}
	inventory, err := NewRuntimeOperationAdmissibility(request.PrerequisiteID, request.OwnerModuleID, request.RequiredKinds, nil, nil, nil, "tree-1")
	if err != nil {
		t.Fatal(err)
	}
	observation := RuntimePrerequisiteObservation{
		PrerequisiteID: request.PrerequisiteID, Kind: PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: request.OwnerModuleID, ContentHash: inventory.SnapshotID,
		Dependencies: []RuntimeFreshnessDependency{{Domain: RuntimeResourceOwnerCatalog, OwnerModuleID: request.OwnerModuleID, SubjectRef: "operation_admissibility", Revision: "tree-1"}},
	}
	observation.ObservationID = StableRuntimePrerequisiteObservationID(observation)
	acquisition := RuntimeOperationAdmissibilityAcquisition{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, AuthoritySnapshotID: request.AuthoritySnapshotID,
		Admissibility: inventory, Observation: observation,
	}
	if err := acquisition.ValidateForRequest(request); err != nil {
		t.Fatalf("valid scoped acquisition: %v", err)
	}
	wrongScope := acquisition
	wrongScope.RunID = "run-2"
	if err := wrongScope.ValidateForRequest(request); err == nil {
		t.Fatal("cross-run admissibility acquisition unexpectedly validated")
	}
	wrongObservation := acquisition
	wrongObservation.Observation.PrerequisiteID = "admissibility-2"
	if err := wrongObservation.ValidateForRequest(request); err == nil {
		t.Fatal("mismatched admissibility observation unexpectedly validated")
	}
}

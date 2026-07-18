package workorchruntimecontract

import "testing"

func TestRuntimeOperationAdmissibilityRequiresCanonicalSnapshotIdentity(t *testing.T) {
	inventory := RuntimeOperationAdmissibility{
		PrerequisiteID: "admissibility-file", OwnerModuleID: RuntimeModuleFile,
		OperationKinds: []string{StepKindFileEdit}, TargetRefs: []string{"target.txt"}, ObservedRevision: "revision-1",
	}
	inventory.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	if err := inventory.Validate(); err != nil {
		t.Fatalf("canonical inventory rejected: %v", err)
	}
	inventory.SnapshotID = "operation-admissibility-forged"
	if err := inventory.Validate(); err == nil {
		t.Fatal("forged inventory snapshot identity was accepted")
	}
}

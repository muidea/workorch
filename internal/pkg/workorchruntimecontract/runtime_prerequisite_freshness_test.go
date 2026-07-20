package workorchruntimecontract

import "testing"

func TestRuntimePrerequisiteInvalidationRetainsReceiptResourceChanges(t *testing.T) {
	change := RuntimeResourceChange{
		Domain: RuntimeResourceWorkspaceTarget, OwnerModuleID: RuntimeModuleFile,
		SubjectRef: "main.go", BeforeRevision: "v1", AfterRevision: "v2",
	}
	record := RuntimePrerequisiteInvalidationRecord{
		Signature: "resource-change-1", PrerequisiteIDs: []string{"target:main.go"}, ObservationIDs: []string{"observation-1"},
		ReceiptHash: "receipt-1", ResourceChanges: []RuntimeResourceChange{change},
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("resource-change invalidation validation failed: %v", err)
	}
	normalized := record.Normalized()
	if len(normalized.ResourceChanges) != 1 || normalized.ResourceChanges[0].AfterRevision != "v2" {
		t.Fatalf("resource changes not retained: %#v", normalized.ResourceChanges)
	}
	stale := RuntimePrerequisiteStale{
		PrerequisiteID: "target:main.go", ObservationID: "observation-1", Domain: RuntimeResourceWorkspaceTarget,
		OwnerModuleID: RuntimeModuleFile, SubjectRef: "main.go", ExpectedRevision: "v1", ObservedRevision: "v2", Reason: "changed",
	}
	record.Stale = &stale
	if err := record.Validate(); err == nil {
		t.Fatal("stale invalidation with resource changes was accepted")
	}
}

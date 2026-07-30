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

func TestCapabilityAcquisitionBindingParticipatesInStableIdentities(t *testing.T) {
	inventory := RuntimeOperationAdmissibility{
		PrerequisiteID: "admissibility-git", OwnerModuleID: RuntimeModuleGit,
		OperationKinds: []string{StepKindGitWrite}, ObservedRevision: "repo-v1", AcquisitionBinding: "capability-state/ws/run/git/0",
	}
	inventory.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	if err := inventory.Validate(); err != nil {
		t.Fatalf("bound inventory rejected: %v", err)
	}
	changedInventory := inventory
	changedInventory.AcquisitionBinding = "capability-state/ws/run/git/1"
	if StableRuntimeOperationAdmissibilitySnapshotID(changedInventory) == inventory.SnapshotID {
		t.Fatal("admissibility snapshot identity ignored acquisition binding")
	}

	snapshot := RuntimeCapabilityStateSnapshot{
		RequirementID: "capability-state:git", AcquisitionKey: "capability-state/ws/run/git/0", AcquisitionBinding: "capability-state/ws/run/git/0",
		OwnerModuleID: RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable",
		ObservedRevision: "repo-v1", ContentHash: "hash-v1", Disposition: RuntimeCapabilityStateReady,
		QueryReceiptRef: "capability-state-receipt/capability-state/ws/run/git/0",
	}
	snapshot.SnapshotID = StableRuntimeCapabilityStateSnapshotID(snapshot)
	changedSnapshot := snapshot
	changedSnapshot.AcquisitionBinding = "capability-state/ws/run/git/1"
	if StableRuntimeCapabilityStateSnapshotID(changedSnapshot) == snapshot.SnapshotID ||
		StableRuntimeCapabilityStateHash([]RuntimeCapabilityStateSnapshot{changedSnapshot}) == StableRuntimeCapabilityStateHash([]RuntimeCapabilityStateSnapshot{snapshot}) {
		t.Fatal("capability-state identity/hash ignored acquisition binding")
	}
}

func TestShellValidationRecipesParticipateInAdmissibilitySnapshotIdentity(t *testing.T) {
	inventory := RuntimeOperationAdmissibility{
		PrerequisiteID: "admissibility-shell", OwnerModuleID: RuntimeModuleShell,
		OperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ObservedRevision: "shell-policy-v1",
		ValidationRecipes: []RuntimeValidationRecipe{{ID: "recipe-a", Command: "sh", Args: []string{"-c", "true"}}},
	}
	inventory.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	if err := inventory.Validate(); err != nil {
		t.Fatalf("canonical shell recipe inventory rejected: %v", err)
	}
	changed := inventory
	changed.ValidationRecipes[0].Args = []string{"-c", "false"}
	if StableRuntimeOperationAdmissibilitySnapshotID(changed) == inventory.SnapshotID {
		t.Fatal("admissibility snapshot identity ignored owner-confirmed recipe contents")
	}
	changed = inventory
	changed.ValidationRecipes = nil
	if StableRuntimeOperationAdmissibilitySnapshotID(changed) == inventory.SnapshotID {
		t.Fatal("admissibility snapshot identity ignored recipe removal")
	}
}

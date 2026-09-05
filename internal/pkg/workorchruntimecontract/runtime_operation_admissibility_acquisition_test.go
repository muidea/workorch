package workorchruntimecontract

import "testing"

func TestRuntimeWorkspaceManifestBindingAcceptsOnlyCanonicalOwner(t *testing.T) {
	const syntheticOwner = "blocks/synthetic-manifest-owner"
	observation := RuntimePrerequisiteObservation{
		PrerequisiteID: "manifest-synthetic", Kind: PrerequisiteKindManifest, OwnerModuleID: syntheticOwner,
		ContentHash: "tree-synthetic", Dependencies: []RuntimeFreshnessDependency{{
			Domain: RuntimeResourceWorkspaceTree, OwnerModuleID: syntheticOwner, SubjectRef: "workspace_deliverable", Revision: "tree-synthetic",
		}},
	}
	observation.ObservationID = StableRuntimePrerequisiteObservationID(observation)
	binding := RuntimeWorkspaceManifestBinding{
		Manifest: WorkspaceManifest{Revision: "tree-synthetic", TargetRefs: []string{"artifact.txt"}}, Observation: observation,
	}
	if err := binding.Validate(); err != nil {
		t.Fatalf("synthetic non-File manifest owner: %v", err)
	}

	for _, ownerModuleID := range []string{"", "   ", " " + syntheticOwner} {
		invalid := binding
		invalid.Observation.OwnerModuleID = ownerModuleID
		invalid.Observation.ObservationID = StableRuntimePrerequisiteObservationID(invalid.Observation)
		if err := invalid.Validate(); err == nil {
			t.Fatalf("workspace manifest binding accepted non-canonical owner %q", ownerModuleID)
		}
	}
}

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

func TestShellValidationAdmissibilityRequiresAndPreservesFileManifestBinding(t *testing.T) {
	manifestObservation := RuntimePrerequisiteObservation{
		PrerequisiteID: "manifest-1", Kind: PrerequisiteKindManifest, OwnerModuleID: RuntimeModuleFile,
		ContentHash: "tree-1", Dependencies: []RuntimeFreshnessDependency{{
			Domain: RuntimeResourceWorkspaceTree, OwnerModuleID: RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree-1",
		}},
	}
	manifestObservation.ObservationID = StableRuntimePrerequisiteObservationID(manifestObservation)
	binding := RuntimeWorkspaceManifestBinding{
		Manifest: WorkspaceManifest{Revision: "tree-1", TargetRefs: []string{"main.go", "main_test.go"}}, Observation: manifestObservation,
	}
	declared := RuntimeValidationRecipe{ID: "go-tests", Command: "go", Args: []string{"test", "./..."}}
	request := RuntimeOperationAdmissibilityRequest{
		WorkspaceID: "ws-1", RunID: "run-1", AuthoritySnapshotID: "authority-1",
		PrerequisiteID: "admissibility-shell", OwnerModuleID: RuntimeModuleShell,
		WorkspaceDeliverableDirectory: t.TempDir(), RequiredKinds: []string{RuntimeBlockKindManagedValidationSession},
		ValidationRecipes: []RuntimeValidationRecipe{declared}, WorkspaceManifestBinding: binding, MaxItems: 4,
	}
	withoutBinding := request
	withoutBinding.WorkspaceManifestBinding = RuntimeWorkspaceManifestBinding{}
	if err := ValidateRuntimeOperationAdmissibilityRequest(withoutBinding, RuntimeModuleShell); err == nil {
		t.Fatal("Shell validation recipe request accepted without File manifest binding")
	}
	if err := ValidateRuntimeOperationAdmissibilityRequest(request, RuntimeModuleShell); err != nil {
		t.Fatalf("valid Shell validation recipe request: %v", err)
	}
	inventory := RuntimeOperationAdmissibility{
		PrerequisiteID: request.PrerequisiteID, OwnerModuleID: RuntimeModuleShell,
		OperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationRecipes: []RuntimeValidationRecipe{declared},
		ValidationSurfaces: []RuntimeValidationSurface{{
			ID: declared.ID, OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
			Readiness: RuntimeValidationSurfaceReady, InputSchemaHash: "schema/go-tests", InputBindingRef: "input/go-tests/tree-1",
			ProofApplicability: RuntimeValidationProofAuxiliary, EffectScope: RuntimeEffectEphemeralSession,
			IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
		}},
		ObservedRevision: "shell-policy-1",
	}
	inventory.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	observation := RuntimePrerequisiteObservation{
		PrerequisiteID: request.PrerequisiteID, Kind: PrerequisiteKindOperationAdmissibility, OwnerModuleID: RuntimeModuleShell,
		ContentHash: inventory.SnapshotID, Dependencies: []RuntimeFreshnessDependency{
			{Domain: RuntimeResourceOwnerCatalog, OwnerModuleID: RuntimeModuleShell, SubjectRef: "shell_runtime_policy", Revision: "shell-policy-1"},
			{Domain: RuntimeResourceWorkspaceTree, OwnerModuleID: RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree-1"},
		},
	}
	observation.ObservationID = StableRuntimePrerequisiteObservationID(observation)
	acquisition := RuntimeOperationAdmissibilityAcquisition{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, AuthoritySnapshotID: request.AuthoritySnapshotID,
		Admissibility: inventory, Observation: observation,
	}
	if err := acquisition.ValidateForRequest(request); err != nil {
		t.Fatalf("valid Shell acquisition: %v", err)
	}
	missingTree := acquisition
	missingTree.Observation.Dependencies = missingTree.Observation.Dependencies[:1]
	missingTree.Observation.ObservationID = StableRuntimePrerequisiteObservationID(missingTree.Observation)
	if err := missingTree.ValidateForRequest(request); err == nil {
		t.Fatal("Shell acquisition omitted File workspace-tree dependency")
	}
	undeclared := acquisition
	undeclared.Admissibility.ValidationRecipes = []RuntimeValidationRecipe{{ID: "other", Command: "go", Args: []string{"test", "./..."}}}
	undeclared.Admissibility.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(undeclared.Admissibility)
	undeclared.Observation.ContentHash = undeclared.Admissibility.SnapshotID
	undeclared.Observation.ObservationID = StableRuntimePrerequisiteObservationID(undeclared.Observation)
	if err := undeclared.ValidateForRequest(request); err == nil {
		t.Fatal("Shell acquisition published an undeclared recipe")
	}
	unconfirmedRef := acquisition
	unconfirmedRef.Admissibility.ValidationRecipeRefs = append(unconfirmedRef.Admissibility.ValidationRecipeRefs, "recipe/not-confirmed")
	unconfirmedRef.Admissibility.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(unconfirmedRef.Admissibility)
	unconfirmedRef.Observation.ContentHash = unconfirmedRef.Admissibility.SnapshotID
	unconfirmedRef.Observation.ObservationID = StableRuntimePrerequisiteObservationID(unconfirmedRef.Observation)
	if err := unconfirmedRef.ValidateForRequest(request); err == nil {
		t.Fatal("Shell acquisition published an unconfirmed recipe ref")
	}
}

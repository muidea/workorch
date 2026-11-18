package workorchruntimecontract

import (
	"errors"
	"testing"
)

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

func TestShellValidationAdmissibilityAcceptsOnlyOwnerResolutionOfAdapterDeclaration(t *testing.T) {
	manifestObservation := RuntimePrerequisiteObservation{
		PrerequisiteID: "manifest-adapter", Kind: PrerequisiteKindManifest, OwnerModuleID: RuntimeModuleFile,
		ContentHash: "tree-adapter", Dependencies: []RuntimeFreshnessDependency{{
			Domain: RuntimeResourceWorkspaceTree, OwnerModuleID: RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree-adapter",
		}},
	}
	manifestObservation.ObservationID = StableRuntimePrerequisiteObservationID(manifestObservation)
	binding := RuntimeWorkspaceManifestBinding{
		Manifest:    WorkspaceManifest{Revision: "tree-adapter", TargetRefs: []string{"go.mod", "main.go", "main_test.go"}},
		Observation: manifestObservation,
	}
	declaration := RuntimeValidationRecipe{ID: "project-native", AdapterRef: "shell/project-native-validation", TimeoutMillis: 60_000}
	confirmed := RuntimeValidationRecipe{ID: declaration.ID, AdapterRef: declaration.AdapterRef, Command: "go", Args: []string{"test", "-v", "./..."}, TimeoutMillis: declaration.TimeoutMillis}
	request := RuntimeOperationAdmissibilityRequest{
		WorkspaceID: "ws-adapter", RunID: "run-adapter", AuthoritySnapshotID: "authority-adapter",
		PrerequisiteID: "admissibility-adapter", OwnerModuleID: RuntimeModuleShell,
		WorkspaceDeliverableDirectory: t.TempDir(), RequiredKinds: []string{RuntimeBlockKindManagedValidationSession},
		ValidationRecipes: []RuntimeValidationRecipe{declaration}, WorkspaceManifestBinding: binding, MaxItems: 4,
	}
	inventory := RuntimeOperationAdmissibility{
		PrerequisiteID: request.PrerequisiteID, OwnerModuleID: RuntimeModuleShell,
		OperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationRecipes: []RuntimeValidationRecipe{confirmed},
		ValidationSurfaces: []RuntimeValidationSurface{{
			ID: confirmed.ID, OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
			Readiness: RuntimeValidationSurfaceReady, InputSchemaHash: "schema/project-native", InputBindingRef: "input/project-native/tree-adapter",
			ProofApplicability: RuntimeValidationProofAuxiliary, EffectScope: RuntimeEffectEphemeralSession,
			IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
		}},
		ObservedRevision: "shell-policy-adapter",
	}
	inventory.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(inventory)
	observation := RuntimePrerequisiteObservation{
		PrerequisiteID: request.PrerequisiteID, Kind: PrerequisiteKindOperationAdmissibility, OwnerModuleID: RuntimeModuleShell,
		ContentHash: inventory.SnapshotID, Dependencies: []RuntimeFreshnessDependency{
			{Domain: RuntimeResourceOwnerCatalog, OwnerModuleID: RuntimeModuleShell, SubjectRef: "shell_runtime_policy", Revision: inventory.ObservedRevision},
			{Domain: RuntimeResourceWorkspaceTree, OwnerModuleID: RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: binding.Manifest.Revision},
		},
	}
	observation.ObservationID = StableRuntimePrerequisiteObservationID(observation)
	acquisition := RuntimeOperationAdmissibilityAcquisition{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, AuthoritySnapshotID: request.AuthoritySnapshotID,
		Admissibility: inventory, Observation: observation,
	}
	if err := acquisition.ValidateForRequest(request); err != nil {
		t.Fatalf("owner-resolved adapter recipe rejected: %v", err)
	}

	drifted := acquisition
	drifted.Admissibility.ValidationRecipes[0].AdapterRef = "shell/foreign-adapter/v1"
	drifted.Admissibility.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(drifted.Admissibility)
	drifted.Observation.ContentHash = drifted.Admissibility.SnapshotID
	drifted.Observation.ObservationID = StableRuntimePrerequisiteObservationID(drifted.Observation)
	if err := drifted.ValidateForRequest(request); err == nil {
		t.Fatal("Shell acquisition accepted a recipe resolved by a different adapter")
	}
}

func TestRuntimeOperationAdmissibilityRejectsOverBudgetAndMissingRequiredTargets(t *testing.T) {
	manifestObservation := RuntimePrerequisiteObservation{
		PrerequisiteID: "manifest-scoped", Kind: PrerequisiteKindManifest, OwnerModuleID: RuntimeModuleFile,
		ContentHash: "tree-scoped", Dependencies: []RuntimeFreshnessDependency{{
			Domain: RuntimeResourceWorkspaceTree, OwnerModuleID: RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: "tree-scoped",
		}},
	}
	manifestObservation.ObservationID = StableRuntimePrerequisiteObservationID(manifestObservation)
	binding := RuntimeWorkspaceManifestBinding{
		Manifest:    WorkspaceManifest{Revision: "tree-scoped", TargetRefs: []string{"extra1.go", "extra2.go", "go.mod", "z_last.go"}},
		Observation: manifestObservation,
	}

	makeInventoryAndObs := func(targets []string) (RuntimeOperationAdmissibility, RuntimePrerequisiteObservation) {
		inv := RuntimeOperationAdmissibility{
			PrerequisiteID:   "admissibility-scoped",
			OwnerModuleID:    RuntimeModuleFile,
			OperationKinds:   []string{StepKindFileRead},
			TargetRefs:       targets,
			SelectorRefs:     []string{"file/workspace_relative_path", "file/existing_target"},
			ConstraintRefs:   []string{"file/workspace_relative_path", "file/regular_existing_target"},
			ObservedRevision: binding.Manifest.Revision,
		}
		inv.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(inv)
		obs := RuntimePrerequisiteObservation{
			PrerequisiteID: "admissibility-scoped",
			Kind:           PrerequisiteKindOperationAdmissibility,
			OwnerModuleID:  RuntimeModuleFile,
			ContentHash:    inv.SnapshotID,
			Dependencies: []RuntimeFreshnessDependency{{
				Domain: RuntimeResourceWorkspaceTree, OwnerModuleID: RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: binding.Manifest.Revision,
			}},
		}
		obs.ObservationID = StableRuntimePrerequisiteObservationID(obs)
		return inv, obs
	}

	overBudgetReq := RuntimeOperationAdmissibilityRequest{
		WorkspaceID: "ws-scoped", RunID: "run-scoped", AuthoritySnapshotID: "authority-scoped",
		PrerequisiteID: "admissibility-scoped", OwnerModuleID: RuntimeModuleFile,
		WorkspaceDeliverableDirectory: t.TempDir(), RequiredKinds: []string{StepKindFileRead},
		RequiredTargetRefs: []string{"extra1.go", "extra2.go", "go.mod"}, WorkspaceManifestBinding: binding, MaxItems: 2,
	}
	err := ValidateRuntimeOperationAdmissibilityRequest(overBudgetReq, RuntimeModuleFile)
	var budgetErr *RuntimeOwnerInventoryBudgetExceededError
	if !errors.As(err, &budgetErr) || budgetErr.ItemCount != 3 || budgetErr.MaxItems != 2 {
		t.Fatalf("over-budget request err = %v, want RuntimeOwnerInventoryBudgetExceededError(3/2)", err)
	}
	truncatedInv, truncatedObs := makeInventoryAndObs([]string{"extra1.go", "extra2.go"})
	if err := (RuntimeOperationAdmissibilityAcquisition{
		WorkspaceID: overBudgetReq.WorkspaceID, RunID: overBudgetReq.RunID, AuthoritySnapshotID: overBudgetReq.AuthoritySnapshotID,
		Admissibility: truncatedInv, Observation: truncatedObs,
	}).ValidateForRequest(overBudgetReq); !errors.As(err, &budgetErr) {
		t.Fatalf("ValidateForRequest over-budget err = %v, want RuntimeOwnerInventoryBudgetExceededError", err)
	}

	scopedReq := overBudgetReq
	scopedReq.RequiredTargetRefs = []string{"go.mod", "z_last.go"}
	scopedReq.MaxItems = 3

	missingRequiredInv, missingRequiredObs := makeInventoryAndObs([]string{"extra1.go", "extra2.go", "go.mod"})
	if err := (RuntimeOperationAdmissibilityAcquisition{
		WorkspaceID: scopedReq.WorkspaceID, RunID: scopedReq.RunID, AuthoritySnapshotID: scopedReq.AuthoritySnapshotID,
		Admissibility: missingRequiredInv, Observation: missingRequiredObs,
	}).ValidateForRequest(scopedReq); err == nil {
		t.Fatal("ValidateForRequest accepted inventory omitting required target z_last.go")
	}

	validInv, validObs := makeInventoryAndObs([]string{"extra1.go", "go.mod", "z_last.go"})
	if err := (RuntimeOperationAdmissibilityAcquisition{
		WorkspaceID: scopedReq.WorkspaceID, RunID: scopedReq.RunID, AuthoritySnapshotID: scopedReq.AuthoritySnapshotID,
		Admissibility: validInv, Observation: validObs,
	}).ValidateForRequest(scopedReq); err != nil {
		t.Fatalf("ValidateForRequest rejected valid scoped inventory (selectors/constraints must not consume target budget): %v", err)
	}
}

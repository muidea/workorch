package workorchruntimecontract

import "testing"

func TestRuntimeOwnerFactRequirementRequiresMatchingOwnerOperation(t *testing.T) {
	requirement := RuntimeOwnerFactRequirement{
		OwnerModuleID: RuntimeModuleShell,
		FactKind:      RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: RuntimeBlockKindManagedValidationSession,
	}
	if err := requirement.Validate(); err != nil {
		t.Fatalf("valid owner-fact requirement: %v", err)
	}
	invalid := requirement
	invalid.OwnerModuleID = RuntimeModuleFile
	if err := invalid.Validate(); err == nil {
		t.Fatal("owner-fact requirement accepted an operation owned by another component")
	}
	invalid.OwnerModuleID = RuntimeModuleFile
	invalid.OperationKind = StepKindFileWrite
	if err := invalid.Validate(); err == nil {
		t.Fatal("selectable validation owner fact accepted a non-validation operation")
	}
}

func TestRuntimeFactsRejectMalformedOwnerFactOnCurrentFocus(t *testing.T) {
	facts := testRuntimeFacts()
	facts.Focus.RequiredOwnerFacts = []RuntimeOwnerFactRequirement{{
		OwnerModuleID: RuntimeModuleFile, FactKind: RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: RuntimeBlockKindManagedValidationSession,
	}}
	if err := facts.Validate(); err == nil {
		t.Fatal("malformed owner fact on current focus was accepted")
	}
}

func TestRuntimeMissingOwnerFactRequirementsRequiresReadyDirectPublishedSurface(t *testing.T) {
	requirement := RuntimeOwnerFactRequirement{
		OwnerModuleID: RuntimeModuleShell,
		FactKind:      RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: RuntimeBlockKindManagedValidationSession,
	}
	direct := RuntimeValidationSurface{
		ID: "recipe/direct", OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
		Readiness: RuntimeValidationSurfaceReady, InputSchemaHash: "schema-direct", InputBindingRef: "input-direct",
		CheckRefs: []string{"check/direct"}, ProofApplicability: RuntimeValidationProofDirect,
		EffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
	}
	catalog := RuntimeOperationAdmissibility{
		PrerequisiteID: "shell-catalog", OwnerModuleID: RuntimeModuleShell,
		OperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationRecipeRefs: []string{direct.ID},
		ValidationSurfaces: []RuntimeValidationSurface{direct}, ObservedRevision: "shell-v1",
	}
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	if err := catalog.Validate(); err != nil {
		t.Fatalf("valid direct owner catalog: %v", err)
	}
	if missing := RuntimeMissingOwnerFactRequirements([]RuntimeOwnerFactRequirement{requirement}, []RuntimeOperationAdmissibility{catalog}); len(missing) != 0 {
		t.Fatalf("ready direct published surface remained missing: %+v", missing)
	}

	auxiliary := direct
	auxiliary.ID = "recipe/auxiliary"
	auxiliary.ProofApplicability = RuntimeValidationProofAuxiliary
	auxiliaryCatalog := catalog
	auxiliaryCatalog.ValidationRecipeRefs = nil
	auxiliaryCatalog.ValidationSurfaces = []RuntimeValidationSurface{auxiliary}
	auxiliaryCatalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(auxiliaryCatalog)
	if missing := RuntimeMissingOwnerFactRequirements([]RuntimeOwnerFactRequirement{requirement}, []RuntimeOperationAdmissibility{auxiliaryCatalog}); len(missing) != 1 {
		t.Fatalf("ready auxiliary surface satisfied direct owner fact: %+v", missing)
	}

	requiresInput := direct
	requiresInput.ID = "recipe/requires-input"
	requiresInput.Readiness = RuntimeValidationSurfacePrerequisiteRequired
	requiresInput.ReadinessReason = "sealed input required"
	requiresInput.InputBindingRef = ""
	requiresInput.PrerequisiteRef = "input/required"
	requiresInputCatalog := catalog
	requiresInputCatalog.ValidationRecipeRefs = nil
	requiresInputCatalog.ValidationSurfaces = []RuntimeValidationSurface{requiresInput}
	requiresInputCatalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(requiresInputCatalog)
	if missing := RuntimeMissingOwnerFactRequirements([]RuntimeOwnerFactRequirement{requirement}, []RuntimeOperationAdmissibility{requiresInputCatalog}); len(missing) != 1 {
		t.Fatalf("required-input surface satisfied ready owner fact: %+v", missing)
	}

	unpublished := catalog
	unpublished.ValidationRecipeRefs = nil
	unpublished.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(unpublished)
	if missing := RuntimeMissingOwnerFactRequirements([]RuntimeOwnerFactRequirement{requirement}, []RuntimeOperationAdmissibility{unpublished}); len(missing) != 1 {
		t.Fatalf("unpublished direct surface satisfied owner fact: %+v", missing)
	}
}

func TestResolveRuntimeOwnerFactAcquisitionUsesOnlySealedMissingFactClosure(t *testing.T) {
	requirement := RuntimeOwnerFactRequirement{
		OwnerModuleID: RuntimeModuleShell, FactKind: RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: RuntimeBlockKindManagedValidationSession,
	}
	convergence := RuntimeOwnerFactConvergence{
		Requirement: requirement, MissingTrigger: RuntimeOwnerFactMissingTrigger,
		AcquisitionOwnerModuleID: RuntimeModuleFile, AcquisitionOperationKind: StepKindFileWrite,
		TargetSelectorRef: "file/workspace_deliverable", TargetSelectorOwnerRef: "profile/test", TargetSelectorSubjectRef: "validation_preparation",
		ReobserveOwnerModuleID: RuntimeModuleShell, ReobserveFactKind: RuntimeOwnerFactSelectableValidationSurface,
		MaxAcquisitionAttempts: 1,
	}
	selected, found := ResolveRuntimeOwnerFactAcquisition([]RuntimeOwnerFactRequirement{requirement}, []RuntimeOwnerFactConvergence{convergence}, nil)
	if !found || selected.AcquisitionOwnerModuleID != RuntimeModuleFile || selected.AcquisitionOperationKind != StepKindFileWrite || selected.TargetSelectorRef != convergence.TargetSelectorRef {
		t.Fatalf("missing owner fact acquisition = %+v found=%t", selected, found)
	}

	preexisting := convergence
	preexisting = RuntimeOwnerFactConvergence{Requirement: requirement, RequirePreexisting: true}
	if _, found := ResolveRuntimeOwnerFactAcquisition([]RuntimeOwnerFactRequirement{requirement}, []RuntimeOwnerFactConvergence{preexisting}, nil); found {
		t.Fatal("missing preexisting-only owner fact granted acquisition authority")
	}

	readyCatalog := RuntimeOperationAdmissibility{
		PrerequisiteID: "shell-ready", OwnerModuleID: RuntimeModuleShell,
		OperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationRecipeRefs: []string{"recipe/ready"},
		ValidationSurfaces: []RuntimeValidationSurface{{
			ID: "recipe/ready", OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
			Readiness: RuntimeValidationSurfaceReady, InputSchemaHash: "schema/ready", InputBindingRef: "input/ready",
			CheckRefs: []string{"check/ready"}, ProofApplicability: RuntimeValidationProofDirect,
			EffectScope: RuntimeEffectNone, IdempotencyClass: RuntimeOperationIdempotencySafeToReplay,
		}},
		ObservedRevision: "shell-ready-v1",
	}
	readyCatalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(readyCatalog)
	if _, found := ResolveRuntimeOwnerFactAcquisition([]RuntimeOwnerFactRequirement{requirement}, []RuntimeOwnerFactConvergence{convergence}, []RuntimeOperationAdmissibility{readyCatalog}); found {
		t.Fatal("already satisfied owner fact retained acquisition authority")
	}
}

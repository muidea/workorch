package workorchruntimecontract

import "testing"

const runtimeProspectiveSatisfactionContractRefForTest = "shell/managed-validation/workspace-manifest-v1"

func TestRuntimeOwnerFactConvergenceProspectiveSatisfactionContractPolicyGate(t *testing.T) {
	requirement := RuntimeOwnerFactRequirement{
		OwnerModuleID: RuntimeModuleShell, FactKind: RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: RuntimeBlockKindManagedValidationSession,
	}
	value := RuntimeOwnerFactConvergence{
		Requirement: requirement, MissingTrigger: RuntimeOwnerFactMissingTrigger,
		AcquisitionOwnerModuleID: RuntimeModuleFile, AcquisitionOperationKind: StepKindFileWrite,
		AcquisitionTargetPolicy: RuntimeOperationTargetPolicyProspectiveOwnerTarget, AcquisitionTargetSelectorRef: "file/workspace-relative-create",
		TargetSelectorRef: "file/workspace-deliverable", TargetSelectorOwnerRef: "profile/test", TargetSelectorSubjectRef: "validation",
		ReobserveOwnerModuleID: RuntimeModuleShell, ReobserveFactKind: RuntimeOwnerFactSelectableValidationSurface,
		MaxAcquisitionAttempts: 1,
	}
	if err := value.Validate(); err == nil {
		t.Fatal("prospective owner-fact acquisition omitted its satisfaction contract")
	}
	value.ProspectiveSatisfactionContractRef = runtimeProspectiveSatisfactionContractRefForTest
	if err := value.Validate(); err != nil {
		t.Fatalf("prospective owner-fact satisfaction contract rejected: %v", err)
	}
	preexisting := RuntimeOwnerFactConvergence{
		Requirement: requirement, RequirePreexisting: true,
		ProspectiveSatisfactionContractRef: runtimeProspectiveSatisfactionContractRefForTest,
	}
	if err := preexisting.Validate(); err == nil {
		t.Fatal("preexisting owner-fact convergence carried a prospective satisfaction contract")
	}
	existingScope := value
	existingScope.AcquisitionTargetPolicy = RuntimeOperationTargetPolicyExistingOwnerScope
	if err := existingScope.Validate(); err == nil {
		t.Fatal("existing-scope owner-fact acquisition carried a prospective satisfaction contract")
	}
}

func TestRuntimeValidationSurfaceProspectiveSatisfactionContractReadinessGate(t *testing.T) {
	value := runtimeProspectiveSatisfactionSurfaceForTest(t)
	if err := value.Validate(); err != nil {
		t.Fatalf("prerequisite-required prospective satisfaction surface rejected: %v", err)
	}
	ready := value
	ready.Readiness = RuntimeValidationSurfaceReady
	ready.ReadinessReason = ""
	ready.PrerequisiteRef = ""
	ready.InputBindingRef = "input/ready"
	if err := ready.Validate(); err == nil {
		t.Fatal("ready validation surface carried a prospective satisfaction contract")
	}
	unavailable := value
	unavailable.Readiness = RuntimeValidationSurfaceUnavailable
	unavailable.PrerequisiteRef = ""
	unavailable.ReadinessReason = "unavailable"
	if err := unavailable.Validate(); err == nil {
		t.Fatal("unavailable validation surface carried a prospective satisfaction contract")
	}
}

func TestRuntimeProspectiveSatisfactionOwnerAndFinalValidationStages(t *testing.T) {
	manifest, _, proposal := semanticMissingOwnerFactAcquisitionFixture(t)
	fileCatalog, found := runtimeSemanticOwnerCatalogFor(manifest, RuntimeModuleFile)
	if !found {
		t.Fatal("semantic fixture has no File owner catalog")
	}
	delta, err := NewRuntimeProspectiveWorkspaceManifestDelta(
		manifest, proposal, runtimeProspectiveSatisfactionContractRefForTest, fileCatalog,
		WorkspaceManifest{Revision: fileCatalog.ObservedRevision, TargetRefs: append([]string(nil), fileCatalog.TargetRefs...)},
	)
	if err != nil {
		t.Fatalf("prospective workspace manifest delta: %v", err)
	}
	if hash := StableRuntimeProspectiveWorkspaceManifestDeltaHash(delta); hash == "" {
		t.Fatal("prospective workspace manifest delta has no stable hash")
	}
	changed := delta.Clone()
	changed.ResultingTargetRefs = append(changed.ResultingTargetRefs, "other.go")
	if StableRuntimeProspectiveWorkspaceManifestDeltaHash(changed) == StableRuntimeProspectiveWorkspaceManifestDeltaHash(delta) {
		t.Fatal("prospective workspace manifest delta hash ignored resulting targets")
	}

	requiredCatalog, found := runtimeSemanticOwnerFactDependencyCatalogFor(manifest, RuntimeModuleShell)
	if !found {
		t.Fatal("semantic fixture has no Shell dependency catalog")
	}
	observation, recipes := runtimeProspectiveSatisfactionOwnerFactsForTest(t, requiredCatalog, delta.BaseWorkspaceManifestRevision)
	request := RuntimeOwnerFactProspectiveSatisfactionRequest{
		WorkspaceID: manifest.WorkspaceID, RunID: manifest.RunID, FeasibilityBatchID: "feasibility-batch-1",
		Manifest: manifest, Proposal: proposal,
		Requirement:                        manifest.ProofObligations[0].OwnerFactAcquisition.Requirement,
		ProspectiveSatisfactionContractRef: runtimeProspectiveSatisfactionContractRefForTest,
		OwnerCatalog:                       requiredCatalog, CatalogObservation: &observation, ValidationRecipes: recipes,
		ProspectiveWorkspaceManifestDelta: delta,
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("prospective satisfaction request rejected: %v", err)
	}
	staleLineage := request.Clone()
	staleObservation := *staleLineage.CatalogObservation
	staleObservation.Dependencies = append([]RuntimeFreshnessDependency(nil), staleObservation.Dependencies...)
	for idx := range staleObservation.Dependencies {
		if staleObservation.Dependencies[idx].Domain == RuntimeResourceWorkspaceTree {
			staleObservation.Dependencies[idx].Revision = "workspace-tree-stale"
		}
	}
	staleObservation.ObservationID = StableRuntimePrerequisiteObservationID(staleObservation)
	staleLineage.CatalogObservation = &staleObservation
	if err := staleLineage.Validate(); err == nil {
		t.Fatal("prospective satisfaction request accepted a Shell catalog from another File manifest revision")
	}
	response, err := NewRuntimeOwnerFactProspectiveSatisfactionObservation(
		request, RuntimeOwnerFactProspectiveWouldSatisfy, []string{recipes[0].ID}, "",
	)
	if err != nil {
		t.Fatalf("prospective would-satisfy observation: %v", err)
	}
	if err := response.ValidateForRequest(request); err != nil {
		t.Fatalf("prospective satisfaction response rejected: %v", err)
	}
	if hash := StableRuntimeOwnerFactProspectiveSatisfactionObservationHash(response); hash == "" {
		t.Fatal("prospective satisfaction observation has no stable hash")
	}
	staleResponse, err := NewRuntimeOwnerFactProspectiveSatisfactionStaleObservation(request, "shell-policy-v2", "Shell policy changed")
	if err != nil {
		t.Fatalf("prospective typed stale observation: %v", err)
	}
	if staleResponse.Stale == nil || staleResponse.Satisfaction != nil || staleResponse.Stale.ExpectedRevision != requiredCatalog.ObservedRevision {
		t.Fatalf("prospective typed stale observation = %+v", staleResponse)
	}
	if StableRuntimeOwnerFactProspectiveSatisfactionObservationHash(staleResponse) == StableRuntimeOwnerFactProspectiveSatisfactionObservationHash(response) {
		t.Fatal("prospective observation hash did not distinguish satisfaction from stale")
	}

	fact := semanticFeasibilityForTest(t, manifest, proposal, RuntimeSemanticProposalTargetRefs(proposal), "", nil)
	fact.ProspectiveWorkspaceManifestDelta = ptrRuntimeProspectiveWorkspaceManifestDelta(delta)
	fact.OwnerFactProspectiveSatisfactionObservation = nil
	if err := fact.ValidateOwnerForProposal(manifest, proposal); err != nil {
		t.Fatalf("File owner-stage feasibility without satisfaction rejected: %v", err)
	}
	if err := fact.ValidateForProposal(manifest, proposal); err == nil {
		t.Fatal("final prospective feasibility accepted missing required-owner satisfaction")
	}
	fact.OwnerFactProspectiveSatisfactionObservation = &response
	if err := fact.ValidateForProposal(manifest, proposal); err != nil {
		t.Fatalf("final exact would-satisfy feasibility rejected: %v", err)
	}
	setHash := StableRuntimeSemanticFeasibilitySetHash([]RuntimeSemanticProofFeasibility{fact})
	changedFact := fact.Clone()
	changedFact.OwnerFactProspectiveSatisfactionObservation.Satisfaction.ProspectiveValidationRecipeRefs = []string{"recipe/other"}
	if StableRuntimeSemanticFeasibilitySetHash([]RuntimeSemanticProofFeasibility{changedFact}) == setHash {
		t.Fatal("semantic feasibility set hash ignored prospective satisfaction")
	}
	staleFact := fact.Clone()
	staleFact.OwnerFactProspectiveSatisfactionObservation = &staleResponse
	if err := staleFact.ValidateCompositeForProposal(manifest, proposal); err != nil {
		t.Fatalf("persistable prospective typed stale rejected: %v", err)
	}
	if err := staleFact.ValidateForProposal(manifest, proposal); err == nil {
		t.Fatal("seal-ready feasibility accepted prospective typed stale")
	}
	gap := RuntimeSemanticFeasibilityGap{
		RefreshGeneration:  1,
		ManifestHash:       manifest.Hash,
		DocumentHash:       "semantic-document-hash",
		FeasibilityBatchID: "feasibility-batch-1",
		Feasibilities:      []RuntimeSemanticProofFeasibility{staleFact},
	}
	gap.FeasibilitySetHash = StableRuntimeSemanticFeasibilitySetHash(gap.Feasibilities)
	if err := gap.Validate(); err != nil {
		t.Fatalf("prospective nested stale feasibility gap rejected: %v", err)
	}

	wouldNot, err := NewRuntimeOwnerFactProspectiveSatisfactionObservation(
		request, RuntimeOwnerFactProspectiveWouldNotSatisfy, nil, "prospective manifest has no applicable validation input",
	)
	if err != nil {
		t.Fatalf("prospective would-not-satisfy observation: %v", err)
	}
	fact.OwnerFactProspectiveSatisfactionObservation = &wouldNot
	if err := fact.ValidateOwnerForProposal(manifest, proposal); err == nil {
		t.Fatal("File owner-stage accepted another owner's factual observation")
	}
	if err := fact.ValidateCompositeForProposal(manifest, proposal); err != nil {
		t.Fatalf("persistable prospective would_not_satisfy rejected: %v", err)
	}
	if err := fact.ValidateForProposal(manifest, proposal); err == nil {
		t.Fatal("final prospective feasibility accepted would_not_satisfy")
	}
}

func runtimeProspectiveSatisfactionOwnerFactsForTest(t *testing.T, catalog RuntimeOperationAdmissibility, workspaceTreeRevision string) (RuntimePrerequisiteObservation, []RuntimeValidationRecipe) {
	t.Helper()
	catalog = catalog.Normalized()
	if len(catalog.ValidationSurfaces) == 0 {
		t.Fatal("prospective required-owner catalog has no validation surface")
	}
	recipe := RuntimeValidationRecipe{ID: catalog.ValidationSurfaces[0].ID, Command: "go", Args: []string{"test", "./..."}, TimeoutMillis: 30000}
	observation := RuntimePrerequisiteObservation{
		PrerequisiteID: catalog.PrerequisiteID, Kind: PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: RuntimeModuleShell, ContentHash: catalog.SnapshotID,
		Dependencies: []RuntimeFreshnessDependency{
			{Domain: RuntimeResourceOwnerCatalog, OwnerModuleID: RuntimeModuleShell, SubjectRef: "managed-validation", Revision: catalog.ObservedRevision},
			{Domain: RuntimeResourceWorkspaceTree, OwnerModuleID: RuntimeModuleFile, SubjectRef: "workspace_deliverable", Revision: workspaceTreeRevision},
		},
	}
	observation.ObservationID = StableRuntimePrerequisiteObservationID(observation)
	if err := observation.Validate(); err != nil {
		t.Fatalf("prospective required-owner observation: %v", err)
	}
	return observation, []RuntimeValidationRecipe{recipe}
}

func runtimeProspectiveSatisfactionSurfaceForTest(t *testing.T) RuntimeValidationSurface {
	value := bindRuntimeValidationSurfaceForTest(RuntimeValidationSurface{
		ID: "recipe/prospective", OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
		Readiness: RuntimeValidationSurfacePrerequisiteRequired, ReadinessReason: "applicable workspace input required",
		InputSchemaHash: "schema/prospective", PrerequisiteRef: "workspace/validation-input",
		ProspectiveSatisfactionContractRef: runtimeProspectiveSatisfactionContractRefForTest,
		EffectScope:                        RuntimeEffectEphemeralSession, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
	}, runtimeValidationProofRequirementForTest(t, "definition-prospective", RuntimeCoverageItem{
		ID: "cwi-prospective", SourceScope: "dodItem", SourceID: "dod-prospective", TargetText: "prospective validation",
	}, "definition/dodItem/dod-prospective", "recipe/prospective", []string{"check/prospective"}))
	constraint := runtimeProspectiveTargetConstraintForTest(value, runtimeProspectiveSatisfactionContractRefForTest, "shell-policy-test", ".sh")
	value.ProspectiveTargetSatisfactionConstraint = &constraint
	return prepareRuntimeValidationSurfaceForTest(t, value, constraint)
}

func runtimeProspectiveTargetConstraintForTest(surface RuntimeValidationSurface, contractRef, catalogRevision, suffix string) RuntimeProspectiveTargetSatisfactionConstraint {
	return RuntimeProspectiveTargetSatisfactionConstraint{
		SchemaVersion:                      RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent,
		OwnerModuleID:                      surface.OwnerModuleID,
		ProspectiveSatisfactionContractRef: contractRef,
		ValidationSurfaceID:                surface.ID,
		ValidationRecipeID:                 surface.ID,
		InputSchemaHash:                    surface.InputSchemaHash,
		CatalogObservedRevision:            catalogRevision,
		AllOf: []RuntimeProspectiveTargetClause{{ID: "required-input", AllOf: []RuntimeProspectiveTargetPredicate{{
			Kind: RuntimeProspectiveTargetPredicatePathSuffix, Value: suffix,
		}}}},
	}.Normalized()
}

func ptrRuntimeProspectiveWorkspaceManifestDelta(value RuntimeProspectiveWorkspaceManifestDelta) *RuntimeProspectiveWorkspaceManifestDelta {
	return &value
}

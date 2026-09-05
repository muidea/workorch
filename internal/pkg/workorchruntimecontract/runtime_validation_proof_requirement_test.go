package workorchruntimecontract

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func runtimeValidationProofRequirementForTest(
	t testing.TB,
	definitionRevision string,
	item RuntimeCoverageItem,
	declarationRef string,
	recipeID string,
	declaredCheckRefs []string,
) RuntimeValidationProofRequirement {
	t.Helper()
	if item.ID == "" {
		item = RuntimeCoverageItem{
			ID: "cwi-validation-test", SliceID: "slice-validation-test",
			SourceScope: "dodItem", SourceID: "dod-validation-test", TargetText: "validation test requirement",
		}
	}
	if item.SourceScope == "" {
		item.SourceScope = "dodItem"
	}
	if item.SourceID == "" {
		item.SourceID = string(item.ID)
	}
	if item.TargetText == "" {
		item.TargetText = "prove " + string(item.ID)
	}
	if declarationRef == "" {
		declarationRef = CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID)
	}
	requirements, err := CompileRuntimeValidationProofRequirements(
		definitionRevision,
		[]RuntimeCoverageItem{item},
		[]RuntimeDefinitionFocus{{
			ID: "definition-focus-" + string(item.ID), SliceID: item.SliceID,
			AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
			CoverageWorkItemIDs:    []common.CoverageWorkItemID{item.ID},
			RequiredEvidenceKinds:  []string{EvidenceKindValidation},
			AcceptedStepKinds:      []string{RuntimeBlockKindManagedValidationSession},
			RequireClaimEvidence:   true,
			EvidenceDeclarationRef: declarationRef, ValidationRecipeID: recipeID,
			RequiredCheckRefs: append([]string(nil), declaredCheckRefs...),
		}},
	)
	if err != nil || len(requirements) != 1 {
		t.Fatalf("compile validation proof requirement: requirements=%+v err=%v", requirements, err)
	}
	return requirements[0]
}

func runtimeValidationCheckAttestationsForTest(requiredCheckRefs []string) []RuntimeValidationCheckAttestationBinding {
	ret := make([]RuntimeValidationCheckAttestationBinding, 0, len(requiredCheckRefs))
	for index, checkRef := range normalizeRuntimeValidationProofCheckRefs(requiredCheckRefs) {
		ret = append(ret, RuntimeValidationCheckAttestationBinding{
			RequiredCheckRef:       checkRef,
			ObservationProtocolRef: "test/owner-native-validation/v1",
			NativeCheckRef:         fmt.Sprintf("native-check-%d", index+1),
		})
	}
	return NormalizeRuntimeValidationCheckAttestationBindings(ret)
}

func bindRuntimeValidationSurfaceForTest(surface RuntimeValidationSurface, requirement RuntimeValidationProofRequirement) RuntimeValidationSurface {
	binding, _ := RuntimeValidationProofRequirementBindingForRecipe(requirement, surface.ID, runtimeValidationCheckAttestationsForTest(requirement.RequiredCheckRefs)...)
	surface.CheckRefs = append([]string(nil), requirement.RequiredCheckRefs...)
	surface.ProofApplicability = RuntimeValidationProofDirect
	surface.ProofRequirementBindings = []RuntimeValidationProofRequirementBinding{
		binding,
	}
	return surface
}

func prepareRuntimeValidationSurfaceForTest(t testing.TB, surface RuntimeValidationSurface, constraint RuntimeProspectiveTargetSatisfactionConstraint) RuntimeValidationSurface {
	t.Helper()
	if len(surface.ProofRequirementBindings) != 1 {
		t.Fatalf("proof preparation fixture requires one exact source binding: %+v", surface)
	}
	binding := surface.ProofRequirementBindings[0].Clone()
	surface.ProofRequirementBindings = nil
	surface.CheckRefs = nil
	surface.ProofRequirementPreparations = []RuntimeValidationProofPreparation{{
		RequirementHash: binding.RequirementHash, DefinitionRevision: binding.DefinitionRevision,
		CoverageWorkItemID: binding.CoverageWorkItemID, DeclarationRef: binding.DeclarationRef,
		ValidationRecipeID:                      binding.ValidationRecipeID,
		CheckAttestations:                       CloneRuntimeValidationCheckAttestationBindings(binding.CheckAttestations),
		ProspectiveTargetSatisfactionConstraint: constraint,
	}}
	if err := surface.Validate(); err != nil {
		t.Fatalf("proof preparation fixture: %v", err)
	}
	return surface
}

func runtimeSemanticValidationRequirementForTest(t *testing.T, manifest RuntimeSemanticPlanManifest, coverageID common.CoverageWorkItemID) RuntimeValidationProofRequirement {
	t.Helper()
	for _, obligation := range manifest.Normalized().ProofObligations {
		if obligation.CoverageWorkItemID == coverageID && obligation.ValidationProofRequirement != nil {
			return obligation.ValidationProofRequirement.Clone()
		}
	}
	t.Fatalf("semantic manifest has no validation proof requirement for %q", coverageID)
	return RuntimeValidationProofRequirement{}
}

func TestCompileRuntimeValidationProofRequirementsKeepsDistinctExactCWIs(t *testing.T) {
	items := []RuntimeCoverageItem{
		{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1", TargetText: "first behavior"},
		{ID: "cwi-2", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-2", TargetText: "second behavior"},
	}
	focuses := []RuntimeDefinitionFocus{
		{ID: "focus-1", AssessmentScope: "dodItem", AssessmentID: "dod-1", AssessmentText: "first behavior", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindValidation}, AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession}, EvidenceDeclarationRef: "definition/dodItem/dod-1", ValidationRecipeID: "recipe/shared", RequiredCheckRefs: []string{"profile/check"}},
		{ID: "focus-2", AssessmentScope: "dodItem", AssessmentID: "dod-2", AssessmentText: "second behavior", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-2"}, RequiredEvidenceKinds: []string{EvidenceKindValidation}, AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession}, EvidenceDeclarationRef: "definition/dodItem/dod-2", ValidationRecipeID: "recipe/shared", RequiredCheckRefs: []string{"profile/check"}},
	}
	requirements, err := CompileRuntimeValidationProofRequirements("definition-1", items, focuses)
	if err != nil {
		t.Fatal(err)
	}
	if len(requirements) != 2 || requirements[0].DeclarationRef == requirements[1].DeclarationRef ||
		requirements[0].Hash == requirements[1].Hash ||
		StableRuntimeCWIValidationCheckRef("definition-1", items[0]) == StableRuntimeCWIValidationCheckRef("definition-1", items[1]) {
		t.Fatalf("per-CWI requirements were not distinct: %+v", requirements)
	}
	for _, requirement := range requirements {
		if strings.Contains(requirement.DeclarationRef, "*") || len(requirement.RequiredCheckRefs) != 2 {
			t.Fatalf("requirement retained wildcard or lost CWI check: %+v", requirement)
		}
	}

	firstSurface := bindRuntimeValidationSurfaceForTest(RuntimeValidationSurface{
		ID: "recipe/shared", OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
		Readiness: RuntimeValidationSurfaceReady, InputSchemaHash: "schema/shared", InputBindingRef: "input/shared",
		EffectScope: RuntimeEffectEphemeralSession, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
	}, requirements[0])
	if _, _, found := RuntimeValidationSurfaceForProofRequirement([]RuntimeValidationSurface{firstSurface}, requirements[1]); found {
		t.Fatal("one CWI applicability binding satisfied a different CWI")
	}
	notReady := firstSurface.Clone()
	notReady.Readiness = RuntimeValidationSurfacePrerequisiteRequired
	notReady.ReadinessReason = "owner input missing"
	notReady.InputBindingRef = ""
	notReady.PrerequisiteRef = "owner/input"
	if err := notReady.Validate(); err == nil || !strings.Contains(err.Error(), "ready proof bindings while not ready") {
		t.Fatalf("non-ready surface retained a ready proof binding: %v", err)
	}
}

func TestRecipeNeutralCWIRequirementBindsEveryCompatibleReadyOwnerRecipe(t *testing.T) {
	item := RuntimeCoverageItem{
		ID: "cwi-neutral", SliceID: "slice-neutral", SourceScope: "dodItem", SourceID: "dod-neutral", TargetText: "neutral behavior",
	}
	requirement := runtimeValidationProofRequirementForTest(
		t, "definition-neutral", item, "definition/dodItem/dod-neutral", "", nil,
	)
	if requirement.ValidationRecipeID != "" || len(requirement.RequiredCheckRefs) != 1 ||
		requirement.RequiredCheckRefs[0] != StableRuntimeCWIValidationCheckRef("definition-neutral", item) {
		t.Fatalf("recipe-neutral requirement = %+v", requirement)
	}
	var surfaces []RuntimeValidationSurface
	for _, recipeID := range []string{"recipe/go", "recipe/python"} {
		binding, err := RuntimeValidationProofRequirementBindingForRecipe(requirement, recipeID, runtimeValidationCheckAttestationsForTest(requirement.RequiredCheckRefs)...)
		if err != nil {
			t.Fatalf("bind neutral requirement to %q: %v", recipeID, err)
		}
		surfaces = append(surfaces, RuntimeValidationSurface{
			ID: recipeID, OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
			Readiness: RuntimeValidationSurfaceReady, InputSchemaHash: "schema/" + recipeID, InputBindingRef: "input/" + recipeID,
			CheckRefs: append([]string(nil), requirement.RequiredCheckRefs...), ProofApplicability: RuntimeValidationProofDirect,
			ProofRequirementBindings: []RuntimeValidationProofRequirementBinding{binding},
			EffectScope:              RuntimeEffectEphemeralSession, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
		})
	}
	if got := RuntimeSelectableValidationRecipeRefsForProofRequirement(surfaces, requirement); !sameStrings(got, []string{"recipe/go", "recipe/python"}) {
		t.Fatalf("recipe-neutral selectable surfaces = %v", got)
	}

	constrained := requirement.Clone()
	constrained.ValidationRecipeID = "recipe/go"
	constrained.Hash = StableRuntimeValidationProofRequirementHash(constrained)
	if _, err := RuntimeValidationProofRequirementBindingForRecipe(constrained, "recipe/python"); err == nil {
		t.Fatal("profile-constrained requirement widened to a different owner recipe")
	}
}

func TestRuntimeValidationProofAuthorityRejectsDuplicateNormalizationLaundering(t *testing.T) {
	item := RuntimeCoverageItem{ID: "cwi-duplicate", SourceScope: "dodItem", SourceID: "dod-duplicate", TargetText: "duplicate checks"}
	requirement := runtimeValidationProofRequirementForTest(t, "definition-duplicate", item, "definition/dodItem/dod-duplicate", "recipe/duplicate", nil)
	duplicateCheck := requirement.Clone()
	duplicateCheck.RequiredCheckRefs = append(duplicateCheck.RequiredCheckRefs, duplicateCheck.RequiredCheckRefs[0])
	duplicateCheck.Hash = StableRuntimeValidationProofRequirementHash(duplicateCheck)
	if err := duplicateCheck.Validate(); err == nil {
		t.Fatal("duplicate requirement check was normalized away")
	}
	if err := ValidateRuntimeValidationProofRequirements([]RuntimeValidationProofRequirement{requirement, requirement}); err == nil {
		t.Fatal("duplicate requirements were normalized away")
	}
	binding := RuntimeValidationProofRequirementBindingFor(requirement)
	if err := ValidateRuntimeValidationProofRequirementBindings([]RuntimeValidationProofRequirementBinding{binding, binding}); err == nil {
		t.Fatal("duplicate applicability bindings were normalized away")
	}
}

func TestRuntimeValidationSurfaceRejectsNativeCheckReuseAcrossReadyAndPreparationRecords(t *testing.T) {
	const recipeID = "recipe/native-collision"
	first := runtimeValidationProofRequirementForTest(t, "definition-native-collision", RuntimeCoverageItem{
		ID: "cwi-native-ready", SourceScope: "dodItem", SourceID: "dod-native-ready", TargetText: "ready behavior",
	}, "", recipeID, nil)
	second := runtimeValidationProofRequirementForTest(t, "definition-native-collision", RuntimeCoverageItem{
		ID: "cwi-native-missing", SourceScope: "dodItem", SourceID: "dod-native-missing", TargetText: "missing behavior",
	}, "", recipeID, nil)
	sharedAttestation := func(requirement RuntimeValidationProofRequirement) []RuntimeValidationCheckAttestationBinding {
		return []RuntimeValidationCheckAttestationBinding{{
			RequiredCheckRef: requirement.RequiredCheckRefs[0], ObservationProtocolRef: "test/native-collision/v1", NativeCheckRef: "NativeCheckShared",
		}}
	}
	ready, err := RuntimeValidationProofRequirementBindingForRecipe(first, recipeID, sharedAttestation(first)...)
	if err != nil {
		t.Fatal(err)
	}
	preparationFor := func(requirement RuntimeValidationProofRequirement, targetRef string) RuntimeValidationProofPreparation {
		return RuntimeValidationProofPreparation{
			RequirementHash: requirement.Hash, DefinitionRevision: requirement.DefinitionRevision,
			CoverageWorkItemID: requirement.CoverageWorkItemID, DeclarationRef: requirement.DeclarationRef,
			ValidationRecipeID: recipeID, TargetRef: targetRef, CheckAttestations: sharedAttestation(requirement),
			ProspectiveTargetSatisfactionConstraint: RuntimeProspectiveTargetSatisfactionConstraint{
				SchemaVersion: RuntimeProspectiveTargetSatisfactionConstraintSchemaCurrent,
				OwnerModuleID: RuntimeModuleShell, ProspectiveSatisfactionContractRef: "test/native-preparation/v1",
				ValidationSurfaceID: recipeID, ValidationRecipeID: recipeID, InputSchemaHash: "schema/native-collision",
				CatalogObservedRevision: "shell-native-collision-v1",
				AllOf: []RuntimeProspectiveTargetClause{{ID: "native-target/" + targetRef, AllOf: []RuntimeProspectiveTargetPredicate{{
					Kind: RuntimeProspectiveTargetPredicatePathExact, Value: targetRef,
				}}}},
			},
		}
	}
	base := RuntimeValidationSurface{
		ID: recipeID, OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
		Readiness: RuntimeValidationSurfaceReady, InputSchemaHash: "schema/native-collision", InputBindingRef: "input/native-collision",
		ProofApplicability: RuntimeValidationProofDirect, EffectScope: RuntimeEffectEphemeralSession,
		IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
	}
	readyAndPreparation := base
	readyAndPreparation.CheckRefs = append([]string(nil), first.RequiredCheckRefs...)
	readyAndPreparation.ProofRequirementBindings = []RuntimeValidationProofRequirementBinding{ready}
	readyAndPreparation.ProofRequirementPreparations = []RuntimeValidationProofPreparation{preparationFor(second, "checks/missing_test.go")}
	if err := readyAndPreparation.Validate(); err == nil || !strings.Contains(err.Error(), "shared by coverage") {
		t.Fatalf("binding/preparation native collision error = %v", err)
	}

	third := runtimeValidationProofRequirementForTest(t, "definition-native-collision", RuntimeCoverageItem{
		ID: "cwi-native-third", SourceScope: "dodItem", SourceID: "dod-native-third", TargetText: "third behavior",
	}, "", recipeID, nil)
	preparationCollision := base
	preparationCollision.ProofRequirementPreparations = []RuntimeValidationProofPreparation{
		preparationFor(second, "checks/missing_test.go"), preparationFor(third, "checks/third_test.go"),
	}
	if err := preparationCollision.Validate(); err == nil || !strings.Contains(err.Error(), "shared by coverage") {
		t.Fatalf("preparation/preparation native collision error = %v", err)
	}
	catalog := RuntimeOperationAdmissibility{
		PrerequisiteID: "catalog/native-collision", OwnerModuleID: RuntimeModuleShell,
		OperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationSurfaces: []RuntimeValidationSurface{preparationCollision},
		ObservedRevision: "shell-native-collision-v1",
	}
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	payload, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded RuntimeOperationAdmissibility
	if err := json.Unmarshal(payload, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Validate(); err == nil || !strings.Contains(err.Error(), "shared by coverage") {
		t.Fatalf("opaque catalog reload laundered native collision: %v", err)
	}
}

func TestStableRuntimeCWICheckRefIsDeterministicAndSourceSpecific(t *testing.T) {
	item := RuntimeCoverageItem{ID: "cwi-stable", SourceScope: "dodItem", SourceID: "dod-stable", TargetText: "observable behavior"}
	baseline := StableRuntimeCWIValidationCheckRef("definition-stable", item)
	if baseline == "" || !strings.HasPrefix(baseline, "workorch-cwi-check-") || baseline != StableRuntimeCWIValidationCheckRef(" definition-stable ", item) {
		t.Fatalf("stable CWI check identity = %q", baseline)
	}
	changes := []RuntimeCoverageItem{
		{ID: "cwi-other", SourceScope: item.SourceScope, SourceID: item.SourceID, TargetText: item.TargetText},
		{ID: item.ID, SourceScope: "goal", SourceID: item.SourceID, TargetText: item.TargetText},
		{ID: item.ID, SourceScope: item.SourceScope, SourceID: "dod-other", TargetText: item.TargetText},
		{ID: item.ID, SourceScope: item.SourceScope, SourceID: item.SourceID, TargetText: "different behavior"},
	}
	for _, changed := range changes {
		if StableRuntimeCWIValidationCheckRef("definition-stable", changed) == baseline {
			t.Fatalf("CWI check identity ignored source change: baseline=%+v changed=%+v", item, changed)
		}
	}
	if StableRuntimeCWIValidationCheckRef("definition-other", item) == baseline {
		t.Fatal("CWI check identity ignored definition revision")
	}
}

func TestCompileRuntimeValidationProofRequirementsRejectsRawAndSourceDrift(t *testing.T) {
	item := RuntimeCoverageItem{ID: "cwi-source", SliceID: "slice-source", SourceScope: "dodItem", SourceID: "dod-source", TargetText: "source behavior"}
	focus := RuntimeDefinitionFocus{
		ID: "focus-source", SliceID: item.SliceID,
		AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{item.ID}, RequiredEvidenceKinds: []string{EvidenceKindValidation},
		AcceptedStepKinds:      []string{RuntimeBlockKindManagedValidationSession},
		EvidenceDeclarationRef: "definition/dodItem/dod-source", ValidationRecipeID: "recipe/source", RequiredCheckRefs: []string{"profile/source-check"},
	}
	if requirements, err := CompileRuntimeValidationProofRequirements("definition-source", []RuntimeCoverageItem{item}, []RuntimeDefinitionFocus{focus}); err != nil || len(requirements) != 1 {
		t.Fatalf("valid exact source did not compile: requirements=%+v err=%v", requirements, err)
	}
	wildcard := focus
	wildcard.EvidenceDeclarationRef = "profile/dodItem/*"
	if _, err := CompileRuntimeValidationProofRequirements("definition-source", []RuntimeCoverageItem{item}, []RuntimeDefinitionFocus{wildcard}); err == nil || !strings.Contains(err.Error(), "canonical Definition declaration") {
		t.Fatalf("wildcard semantic authority error = %v", err)
	}

	duplicateCheck := focus
	duplicateCheck.RequiredCheckRefs = append(duplicateCheck.RequiredCheckRefs, duplicateCheck.RequiredCheckRefs[0])
	if _, err := CompileRuntimeValidationProofRequirements("definition-source", []RuntimeCoverageItem{item}, []RuntimeDefinitionFocus{duplicateCheck}); err == nil {
		t.Fatal("raw duplicate Profile checks were normalized away")
	}
	duplicateCoverage := focus
	duplicateCoverage.CoverageWorkItemIDs = append(duplicateCoverage.CoverageWorkItemIDs, item.ID)
	if _, err := CompileRuntimeValidationProofRequirements("definition-source", []RuntimeCoverageItem{item}, []RuntimeDefinitionFocus{duplicateCoverage}); err == nil {
		t.Fatal("raw duplicate focus coverage identity was normalized away")
	}
	for name, mutate := range map[string]func(*RuntimeDefinitionFocus){
		"scope": func(value *RuntimeDefinitionFocus) { value.AssessmentScope = "goal" },
		"id":    func(value *RuntimeDefinitionFocus) { value.AssessmentID = "dod-other" },
		"text":  func(value *RuntimeDefinitionFocus) { value.AssessmentText = "different behavior" },
	} {
		t.Run(name, func(t *testing.T) {
			drifted := focus
			mutate(&drifted)
			if _, err := CompileRuntimeValidationProofRequirements("definition-source", []RuntimeCoverageItem{item}, []RuntimeDefinitionFocus{drifted}); err == nil || !strings.Contains(err.Error(), "source does not exactly match") {
				t.Fatalf("source drift error = %v", err)
			}
		})
	}
	if _, err := CompileRuntimeValidationProofRequirements("definition-source", []RuntimeCoverageItem{item, item}, []RuntimeDefinitionFocus{focus}); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate coverage item error = %v", err)
	}
}

func TestShellAdmissibilityAcquisitionRejectsDuplicateProofAuthorityBeforeNormalization(t *testing.T) {
	item := RuntimeCoverageItem{ID: "cwi-acquisition", SourceScope: "dodItem", SourceID: "dod-acquisition", TargetText: "acquisition behavior"}
	requirement := runtimeValidationProofRequirementForTest(t, "definition-acquisition", item, "definition/dodItem/dod-acquisition", "recipe/acquisition", nil)
	recipe := RuntimeValidationRecipe{ID: requirement.ValidationRecipeID, Command: "go", Args: []string{"test", "-v", "./..."}}
	surface := bindRuntimeValidationSurfaceForTest(RuntimeValidationSurface{
		ID: recipe.ID, OwnerModuleID: RuntimeModuleShell, OperationKind: RuntimeBlockKindManagedValidationSession,
		Readiness: RuntimeValidationSurfaceReady, InputSchemaHash: "schema/acquisition", InputBindingRef: "input/acquisition",
		EffectScope: RuntimeEffectEphemeralSession, IdempotencyClass: RuntimeOperationIdempotencyAtMostOnce,
	}, requirement)
	request := RuntimeOperationAdmissibilityRequest{
		WorkspaceID: "ws-acquisition", RunID: "run-acquisition", AuthoritySnapshotID: "authority-acquisition",
		PrerequisiteID: "admissibility-acquisition", OwnerModuleID: RuntimeModuleShell,
		RequiredKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationRecipes: []RuntimeValidationRecipe{recipe},
		ValidationProofRequirements: []RuntimeValidationProofRequirement{requirement}, MaxItems: 4,
	}
	catalog := RuntimeOperationAdmissibility{
		PrerequisiteID: request.PrerequisiteID, OwnerModuleID: RuntimeModuleShell,
		OperationKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationRecipeRefs: []string{recipe.ID},
		ValidationRecipes: []RuntimeValidationRecipe{recipe}, ValidationSurfaces: []RuntimeValidationSurface{surface},
		ObservedRevision: "shell-policy-acquisition-v1",
	}
	catalog.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	observation := RuntimePrerequisiteObservation{
		PrerequisiteID: catalog.PrerequisiteID, Kind: PrerequisiteKindOperationAdmissibility,
		OwnerModuleID: RuntimeModuleShell, ContentHash: catalog.SnapshotID,
		Dependencies: []RuntimeFreshnessDependency{{
			Domain: RuntimeResourceOwnerCatalog, OwnerModuleID: RuntimeModuleShell,
			SubjectRef: "shell_runtime_policy", Revision: catalog.ObservedRevision,
		}},
	}
	observation.ObservationID = StableRuntimePrerequisiteObservationID(observation)
	valid := RuntimeOperationAdmissibilityAcquisition{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, AuthoritySnapshotID: request.AuthoritySnapshotID,
		Admissibility: catalog, Observation: observation,
	}
	if err := valid.ValidateForRequest(request); err != nil {
		t.Fatalf("valid exact acquisition: %v", err)
	}

	duplicateRequest := request
	duplicateRequest.ValidationProofRequirements = append(duplicateRequest.ValidationProofRequirements, requirement)
	if err := valid.ValidateForRequest(duplicateRequest); err == nil {
		t.Fatal("duplicate requested proof requirement was normalized away")
	}
	duplicateSurface := valid
	duplicateSurface.Admissibility = valid.Admissibility.Normalized()
	duplicateSurface.Admissibility.ValidationSurfaces = append(duplicateSurface.Admissibility.ValidationSurfaces, surface)
	duplicateSurface.Admissibility.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(duplicateSurface.Admissibility)
	duplicateSurface.Observation.ContentHash = duplicateSurface.Admissibility.SnapshotID
	duplicateSurface.Observation.ObservationID = StableRuntimePrerequisiteObservationID(duplicateSurface.Observation)
	if err := duplicateSurface.ValidateForRequest(request); err == nil || !strings.Contains(err.Error(), "duplicates validation surface") {
		t.Fatalf("duplicate surface acquisition error = %v", err)
	}
	duplicateBinding := valid
	duplicateBinding.Admissibility = valid.Admissibility.Normalized()
	duplicateBinding.Admissibility.ValidationSurfaces[0].ProofRequirementBindings = append(
		duplicateBinding.Admissibility.ValidationSurfaces[0].ProofRequirementBindings,
		duplicateBinding.Admissibility.ValidationSurfaces[0].ProofRequirementBindings[0].Clone(),
	)
	duplicateBinding.Admissibility.SnapshotID = StableRuntimeOperationAdmissibilitySnapshotID(duplicateBinding.Admissibility)
	duplicateBinding.Observation.ContentHash = duplicateBinding.Admissibility.SnapshotID
	duplicateBinding.Observation.ObservationID = StableRuntimePrerequisiteObservationID(duplicateBinding.Observation)
	if err := duplicateBinding.ValidateForRequest(request); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate binding acquisition error = %v", err)
	}
	if err := ValidateRuntimeOperationAdmissibilities([]RuntimeOperationAdmissibility{catalog, catalog}); err == nil || !strings.Contains(err.Error(), "duplicates prerequisite") {
		t.Fatalf("duplicate RuntimeFacts catalog error = %v", err)
	}
}

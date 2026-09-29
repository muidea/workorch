package shell

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeAdapterPersistsZeroWorkGoTestObservation(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available")
	}
	workspace := newGoValidationWorkspace(t, `package fixture

import "testing"

func TestExisting(t *testing.T) {}
`)
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspace,
		ValidationRecipeID:            "go-test-zero-work",
		ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true,
			Kind:           runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
		Command:    "go",
		Args:       []string{"test", "-v", "-run", "TestMissing", "./..."},
		TargetRefs: []string{"fixture_test.go"},
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
			ID: "go-test-zero-work", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "go",
			ArgsContain: []string{"test"}, ZeroWorkMarkers: []string{"no tests to run", "[no tests to run]"},
			WorkEvidenceMarkers: []string{"=== RUN", "--- PASS:"}, QualityOnZeroWork: string(common.EvidenceQualityInconclusive),
			EvidenceReason: "configured zero-work marker matched", EvidenceTags: []string{"validation", "zero_work"},
		}},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	if !result.ZeroWork || result.EvidenceQuality != string(common.EvidenceQualityInconclusive) ||
		result.Evidence.Quality != common.EvidenceQualityInconclusive || !result.Evidence.ZeroWork ||
		result.Evidence.EvidenceReason != "configured zero-work marker matched" {
		t.Fatalf("zero-work quality was not persisted: %+v", result)
	}
	if result.Evidence.Validation == nil || len(result.Evidence.Validation.PassedChecks) != 0 || result.Evidence.Validation.OutputPreview == "" {
		t.Fatalf("zero-work validation observation = %+v", result.Evidence.Validation)
	}
}

func TestRuntimeAdapterMarksUnconfiguredGoTestZeroWorkAsInconclusive(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available")
	}
	workspace := newGoValidationWorkspace(t, `package fixture

import "testing"

func TestExisting(t *testing.T) {}
`)
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspace,
		ValidationRecipeID:            "go-test-unconfigured-zero-work",
		ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true,
			Kind:           runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
		Command:    "go",
		Args:       []string{"test", "-v", "-run", "TestMissing", "./..."},
		TargetRefs: []string{"fixture_test.go"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	if !result.ZeroWork || result.Evidence.Quality != common.EvidenceQualityInconclusive ||
		result.Evidence.EvidenceReason != "validation command reported no executed checks" {
		t.Fatalf("unconfigured zero-work evidence = %+v", result.Evidence)
	}
	if result.Evidence.Validation == nil || len(result.Evidence.Validation.ExecutedChecks) != 0 ||
		len(result.Evidence.Validation.PassedChecks) != 0 || len(result.Evidence.Validation.SkippedChecks) != 0 {
		t.Fatalf("unconfigured zero-work observation = %+v", result.Evidence.Validation)
	}
}

func TestRuntimeAdapterAttestsExecutedNativeCheckOnlyAfterActualRun(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available")
	}
	workspace := newGoValidationWorkspace(t, `package fixture
import "testing"
func TestWorkOrchCWI_EXECUTED(t *testing.T) { if 2+2 != 4 { t.Fatal("failed") } }
`)
	request := RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspace,
		ValidationRecipeID:            "executed-check",
		ManagedValidation:             runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession},
		Command:                       "go", Args: []string{"test", "-v", "./..."}, TargetRefs: []string{"fixture_test.go"},
		ValidationCheckAttestations: []runtimecontract.RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "check", NativeCheckRef: "TestWorkOrchCWI_EXECUTED", ObservationProtocolRef: shellGoTestObservationProtocolRef}},
	}
	if err := instrumentManagedValidationNativeChecks(&request); err != nil {
		t.Fatal(err)
	}
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), request)
	if err != nil || result.Evidence.Validation == nil || result.Evidence.Validation.ProofClass != runtimecontract.RuntimeValidationProofExecuted ||
		!runtimecontract.RuntimeValidationCheckPassed(*result.Evidence.Validation, "TestWorkOrchCWI_EXECUTED") {
		t.Fatalf("real executed validation did not attest level 3 modality: err=%v observation=%+v evidence=%+v", err, result.Evidence.Validation, result.Evidence)
	}
}

func TestRuntimeAdapterObservesTopLevelPassedAndSkippedGoChecks(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available")
	}
	workspace := newGoValidationWorkspace(t, `package fixture

import "testing"

func TestBehavior(t *testing.T) {
	t.Run("subcase", func(t *testing.T) {})
}

func TestSkipped(t *testing.T) {
	t.Skip("not applicable")
}
`)
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspace,
		ValidationRecipeID:            "go-test-check-observation",
		ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true,
			Kind:           runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
		Command:    "go",
		Args:       []string{"test", "-v", "./..."},
		TargetRefs: []string{"fixture_test.go"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	observation := result.Evidence.Validation
	if result.Evidence.Quality != common.EvidenceQualityValid || observation == nil {
		t.Fatalf("validation evidence = %+v", result.Evidence)
	}
	if !equalValidationChecks(observation.ExecutedChecks, []string{"TestBehavior", "TestSkipped"}) ||
		!equalValidationChecks(observation.PassedChecks, []string{"TestBehavior"}) ||
		!equalValidationChecks(observation.SkippedChecks, []string{"TestSkipped"}) || len(observation.FailedChecks) != 0 {
		t.Fatalf("top-level validation observation = %+v", observation)
	}
}

func TestDefaultManagedValidationRecipeDoesNotManufactureCWISpecificProof(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available")
	}
	workspace := newGoValidationWorkspace(t, "package fixture\nimport \"testing\"\nfunc TestDefaultRecipe(t *testing.T) {}\n")
	recipe := runtimecontract.RuntimeValidationRecipe{
		ID: "software-dev-validation-evidence", Command: "go", Args: []string{"test", "-v", "./..."}, TimeoutMillis: 60_000,
	}
	requirement := shellValidationProofRequirementForTest(t, "", "cwi-default-validation")
	binding := shellTestWorkspaceManifestBinding("tree-default-validation", []string{"fixture.go", "fixture_test.go", "go.mod"})
	surfaces := shellValidationSurfaces(
		[]runtimecontract.RuntimeValidationRecipe{recipe},
		[]runtimecontract.RuntimeValidationRecipe{recipe},
		[]runtimecontract.RuntimeValidationRecipe{recipe},
		[]runtimecontract.RuntimeValidationProofRequirement{requirement},
		binding,
		"shell-policy-test",
		workspace,
	)
	if len(surfaces) != 1 || len(surfaces[0].ProofRequirementBindings) != 0 || len(surfaces[0].ProofRequirementPreparations) != 1 {
		t.Fatalf("default validation surface = %+v", surfaces)
	}
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspace,
		ValidationRecipeID:            recipe.ID,
		ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
		Command: recipe.Command, Args: recipe.Args, TimeoutMillis: recipe.TimeoutMillis,
		TargetRefs: []string{"fixture_test.go"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	observation := result.ValidationObservation
	if observation == nil || observation.RecipeID != recipe.ID ||
		runtimecontract.RuntimeValidationCheckPassed(*observation, requirement.RequiredCheckRefs[0]) {
		t.Fatalf("generic recipe manufactured CWI-specific proof: surface=%+v observation=%+v", surfaces[0], observation)
	}
}

func TestDefaultManagedValidationRecipeCreditsExactOwnerNativeAttestation(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available")
	}
	recipe := runtimecontract.RuntimeValidationRecipe{
		ID: "software-dev-validation-evidence", Command: "go", Args: []string{"test", "-v", "./..."}, TimeoutMillis: 60_000,
		AdapterRef: modulecommon.ValidationRecipeAdapterProjectNative,
	}
	requirement := shellValidationProofRequirementForTest(t, "", "cwi-asserted-validation")
	attestations, supported := shellValidationCheckAttestations(recipe, requirement)
	if !supported || len(attestations) != 1 {
		t.Fatalf("native attestations = %+v supported=%v", attestations, supported)
	}
	nativeCheck := attestations[0].NativeCheckRef
	workspace := newGoValidationWorkspace(t, fmt.Sprintf(`package fixture

import "testing"

func %s(t *testing.T) {
	got, want := deliveredValue(), 4
	if got != want {
		t.Fatalf("detailed CWI assertion: got %%d, want %%d", got, want)
	}
}
`, nativeCheck))
	writeNativeStructureSource(t, workspace, "fixture.go", "package fixture\nfunc deliveredValue() int { return 4 }\n")
	manifestBinding := shellTestWorkspaceManifestBinding("tree-asserted-validation", []string{"fixture.go", "fixture_test.go", "go.mod"})
	surfaces := shellValidationSurfaces(
		[]runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe},
		[]runtimecontract.RuntimeValidationProofRequirement{requirement}, manifestBinding, "shell-policy-test", workspace,
	)
	if len(surfaces) != 1 || len(surfaces[0].ProofRequirementBindings) != 1 || len(surfaces[0].ProofRequirementPreparations) != 0 {
		t.Fatalf("ready exact validation surface = %+v", surfaces)
	}
	ownerBinding := surfaces[0].ProofRequirementBindings[0]
	request := RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspace,
		ValidationRecipeID:            recipe.ID,
		ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
		Command: recipe.Command, Args: recipe.Args, TimeoutMillis: recipe.TimeoutMillis,
		TargetRefs:                  []string{"fixture_test.go"},
		ValidationCheckAttestations: ownerBinding.CheckAttestations,
		ValidationScopeTargetRefs:   manifestBinding.Manifest.TargetRefs,
	}
	if err := instrumentManagedValidationNativeChecks(&request); err != nil {
		t.Fatalf("instrument exact native checks: %v", err)
	}
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), request)
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	if result.ValidationObservation == nil || result.ValidationObservation.ObservationProtocolRef != shellGoBehaviorObservationProtocolRef ||
		result.ValidationObservation.ProofClass != runtimecontract.RuntimeValidationProofBehavioral ||
		!runtimecontract.RuntimeValidationCheckPassed(*result.ValidationObservation, nativeCheck) ||
		runtimecontract.RuntimeValidationCheckPassed(*result.ValidationObservation, requirement.RequiredCheckRefs[0]) {
		t.Fatalf("real assertion did not produce exact owner-native observation: %+v", result.ValidationObservation)
	}
	binding := runtimecontract.CoverageEvidenceBinding{
		ID: "binding/shell/cwi-asserted-validation", CoverageWorkItemID: requirement.CoverageWorkItemID,
		FocusID: "focus/cwi-asserted-validation", DefinitionRevision: requirement.DefinitionRevision,
		Proofs: []runtimecontract.CoverageEvidenceProof{{
			EvidenceKind:          runtimecontract.EvidenceKindValidation,
			RequiredEffectScopes:  []runtimecontract.RuntimeEffectScope{runtimecontract.RuntimeEffectEphemeralSession},
			AllowedOperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
			ValidationRecipeID:    recipe.ID, ValidationProofRequirementHash: requirement.Hash,
			RequiredCheckRefs:           append([]string(nil), requirement.RequiredCheckRefs...),
			ValidationCheckAttestations: runtimecontract.CloneRuntimeValidationCheckAttestationBindings(ownerBinding.CheckAttestations),
		}},
		TargetRefs: []string{"fixture_test.go"}, DeclarationRef: requirement.DeclarationRef,
	}.Normalized()
	binding.Hash = runtimecontract.StableCoverageEvidenceBindingHash(binding)
	operation := runtimecontract.RuntimeOperation{
		ID: "validate-cwi-asserted-validation", Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		TargetRefs: []string{"fixture_test.go"},
	}
	receipt := runtimecontract.RuntimeOperationReceipt{
		ExecutionKey: "execution/shell/cwi-asserted-validation", Fence: 1, OperationID: operation.ID,
		Outcome: runtimecontract.RuntimeOperationOutcomeAccepted, ObservedEffectScope: runtimecontract.RuntimeEffectEphemeralSession,
		TargetRefs: append([]string(nil), operation.TargetRefs...), Evidence: result.Evidence,
	}
	if err := runtimecontract.MatchCoverageEvidenceBindingReceipt(binding, runtimecontract.EvidenceKindValidation, operation, receipt); err != nil {
		t.Fatalf("exact owner-native attestation did not receive CWI proof credit: %v", err)
	}
}

func TestShellValidationSurfaceKeepsPerCWIMixedReadiness(t *testing.T) {
	recipe := runtimecontract.RuntimeValidationRecipe{
		ID: "software-dev-validation-evidence", Command: "go", Args: []string{"test", "-v", "./..."}, TimeoutMillis: 60_000,
	}
	readyRequirement := shellValidationProofRequirementForTest(t, "", "cwi-ready")
	missingRequirement := shellValidationProofRequirementForTest(t, "", "cwi-missing")
	readyAttestations, supported := shellValidationCheckAttestations(recipe, readyRequirement)
	if !supported || len(readyAttestations) != 1 {
		t.Fatalf("ready native mapping = %+v supported=%t", readyAttestations, supported)
	}
	workspace := newGoValidationWorkspace(t, fmt.Sprintf(`package fixture

import "testing"

func %s(t *testing.T) {
	if 2+2 != 4 {
		t.Fatal("ready CWI assertion failed")
	}
}
`, readyAttestations[0].NativeCheckRef))
	manifestBinding := shellTestWorkspaceManifestBinding("tree-mixed-cwi-readiness", []string{"fixture.go", "fixture_test.go", "go.mod"})
	surfaces := shellValidationSurfaces(
		[]runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe},
		[]runtimecontract.RuntimeValidationProofRequirement{readyRequirement, missingRequirement}, manifestBinding, "shell-policy-mixed", workspace,
	)
	if len(surfaces) != 1 {
		t.Fatalf("mixed readiness surfaces = %+v", surfaces)
	}
	surface := surfaces[0]
	if surface.Readiness != runtimecontract.RuntimeValidationSurfaceReady || len(surface.ProofRequirementBindings) != 1 ||
		len(surface.ProofRequirementPreparations) != 1 || surface.ProofRequirementBindings[0].RequirementHash != readyRequirement.Hash ||
		surface.ProofRequirementPreparations[0].RequirementHash != missingRequirement.Hash {
		t.Fatalf("mixed per-CWI readiness collapsed into recipe-wide state: %+v", surface)
	}
	if _, _, found := runtimecontract.RuntimeValidationSurfaceForProofRequirement(surfaces, readyRequirement); !found {
		t.Fatal("ready CWI lost its exact selectable binding")
	}
	if _, _, found := runtimecontract.RuntimeValidationSurfaceForProofRequirement(surfaces, missingRequirement); found {
		t.Fatal("missing CWI borrowed the ready CWI binding")
	}
	catalog := runtimecontract.RuntimeOperationAdmissibility{
		PrerequisiteID: "shell-mixed-readiness", OwnerModuleID: runtimecontract.RuntimeModuleShell,
		OperationKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, ValidationSurfaces: surfaces,
		ValidationRecipeRefs: []string{recipe.ID}, ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{recipe},
		ObservedRevision: "shell-policy-mixed",
	}
	catalog.SnapshotID = runtimecontract.StableRuntimeOperationAdmissibilitySnapshotID(catalog)
	preparation, err := runtimecontract.ResolveRuntimeValidationProofPreparation(catalog, runtimecontract.RuntimeOwnerFactRequirement{
		OwnerModuleID: runtimecontract.RuntimeModuleShell, FactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
		OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession, ValidationProofRequirementHash: missingRequirement.Hash,
	}, modulecommon.ProspectiveSatisfactionContractRef)
	if err != nil || preparation.CoverageWorkItemID != missingRequirement.CoverageWorkItemID {
		t.Fatalf("missing CWI preparation = %+v err=%v", preparation, err)
	}
}

func TestRawWorkspaceMarkerCannotCreditManagedValidation(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	recipe := runtimecontract.RuntimeValidationRecipe{ID: "portable-cwi-validation", Command: "sh", TimeoutMillis: 10_000}
	requirement := shellValidationProofRequirementForTest(t, "", "cwi-portable-assertion")
	exactCheck := requirement.RequiredCheckRefs[0]
	recipe.Args = []string{"-c", fmt.Sprintf("printf 'WORKORCH_CHECK_PASSED=%s\\n'", exactCheck)}
	workspace := t.TempDir()
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspace,
		ValidationRecipeID:            recipe.ID,
		ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
		Command: recipe.Command, Args: recipe.Args, TimeoutMillis: recipe.TimeoutMillis,
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	if result.ValidationObservation == nil || result.ValidationObservation.ObservationProtocolRef != "" ||
		len(result.ValidationObservation.PassedChecks) != 0 || runtimecontract.RuntimeValidationCheckPassed(*result.ValidationObservation, exactCheck) {
		t.Fatalf("raw workspace marker entered proof authority: %+v", result.ValidationObservation)
	}
}

func TestRuntimeAdapterMarksAllSkippedGoChecksAsSkippedEvidence(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not available")
	}
	workspace := newGoValidationWorkspace(t, `package fixture

import "testing"

func TestSkippedOne(t *testing.T) { t.Skip("not applicable") }
func TestSkippedTwo(t *testing.T) { t.Skip("not applicable") }
`)
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspace,
		ValidationRecipeID:            "go-test-all-skipped",
		ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true,
			Kind:           runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
		Command:    "go",
		Args:       []string{"test", "-v", "./..."},
		TargetRefs: []string{"fixture_test.go"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	observation := result.Evidence.Validation
	if result.Evidence.Quality != common.EvidenceQualitySkipped || observation == nil ||
		result.Evidence.EvidenceReason != "all observed validation checks were skipped" {
		t.Fatalf("all-skipped evidence = %+v", result.Evidence)
	}
	if !equalValidationChecks(observation.ExecutedChecks, []string{"TestSkippedOne", "TestSkippedTwo"}) ||
		!equalValidationChecks(observation.SkippedChecks, []string{"TestSkippedOne", "TestSkippedTwo"}) ||
		len(observation.PassedChecks) != 0 || len(observation.FailedChecks) != 0 {
		t.Fatalf("all-skipped observation = %+v", observation)
	}
}

func TestRuntimeAdapterIgnoresRawManagedValidationMarkers(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
	workspace := t.TempDir()
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), RuntimeCommandRequest{
		WorkspaceDeliverableDirectory: workspace,
		ValidationRecipeID:            "file-presence-check",
		ManagedValidation: runtimecontract.RuntimeManagedValidation{
			RuntimeManaged: true,
			Kind:           runtimecontract.RuntimeBlockKindManagedValidationSession,
		},
		Command:    "sh",
		Args:       []string{"-c", "printf 'WORKORCH_CHECK_PASSED=file/present\\n'"},
		TargetRefs: []string{"fixture.go"},
	})
	if err != nil {
		t.Fatalf("ExecuteOperation: %v output=%+v", err, result)
	}
	observation := result.ValidationObservation
	if observation == nil {
		t.Fatal("managed validation observation is nil")
	}
	if observation.ObservationProtocolRef != "" || len(observation.ExecutedChecks) != 0 || len(observation.PassedChecks) != 0 ||
		len(observation.FailedChecks) != 0 || len(observation.SkippedChecks) != 0 {
		t.Fatalf("raw marker entered managed validation authority = %+v", observation)
	}
}

func TestGoTestJSONOutputEventCannotForgeNativePass(t *testing.T) {
	observation := goTestJSONValidationObservation(
		`{"Action":"output","Test":"TestWorkOrchCWI_FAKE","Output":"{\"Action\":\"pass\",\"Test\":\"TestWorkOrchCWI_FAKE\"}"}`,
		"",
	)
	if len(observation.ExecutedChecks) != 0 || len(observation.PassedChecks) != 0 || len(observation.FailedChecks) != 0 || len(observation.SkippedChecks) != 0 {
		t.Fatalf("go test output event forged native pass: %+v", observation)
	}
}

func shellValidationProofRequirementForTest(t *testing.T, recipeID string, coverageID common.CoverageWorkItemID) runtimecontract.RuntimeValidationProofRequirement {
	t.Helper()
	item := runtimecontract.RuntimeCoverageItem{
		ID: coverageID, SourceScope: "dodItem", SourceID: string(coverageID), TargetText: "prove " + string(coverageID),
	}
	requirements, err := runtimecontract.CompileRuntimeValidationProofRequirements(
		"definition-shell-test",
		[]runtimecontract.RuntimeCoverageItem{item},
		[]runtimecontract.RuntimeDefinitionFocus{{
			ID: "definition-focus-" + string(coverageID), AssessmentScope: item.SourceScope, AssessmentID: item.SourceID, AssessmentText: item.TargetText,
			CoverageWorkItemIDs:    []common.CoverageWorkItemID{coverageID},
			RequiredEvidenceKinds:  []string{runtimecontract.EvidenceKindValidation},
			AcceptedStepKinds:      []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
			EvidenceDeclarationRef: "definition/dodItem/" + item.SourceID, ValidationRecipeID: recipeID,
		}},
	)
	if err != nil || len(requirements) != 1 {
		t.Fatalf("compile validation proof requirement: requirements=%+v err=%v", requirements, err)
	}
	return requirements[0]
}

func newGoValidationWorkspace(t *testing.T, testSource string) string {
	t.Helper()
	workspace := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":          "module example.com/workorch-validation-fixture\n\ngo 1.26\n",
		"fixture.go":      "package fixture\n",
		"fixture_test.go": testSource,
	} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return workspace
}

func equalValidationChecks(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for idx := range got {
		if got[idx] != want[idx] {
			return false
		}
	}
	return true
}

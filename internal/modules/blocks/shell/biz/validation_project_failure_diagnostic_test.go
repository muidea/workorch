package shell

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestProjectFailureDiagnosticAdapterIsExplicitAndPreservesOldMappings(t *testing.T) {
	recipe, requirement, _ := conjunctiveSourceRequirement(t)
	recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectConjunction
	old, ok := shellValidationCheckAttestations(recipe, requirement)
	if !ok {
		t.Fatal("old mapping missing")
	}
	recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectFailureDiagnostic
	checks, ok := shellValidationCheckAttestations(recipe, requirement)
	if !ok || len(checks) != len(old) {
		t.Fatal("v7 mapping missing")
	}
	for index, check := range checks {
		if check.ObservationProtocolRef != shellProjectFailureDiagnosticProtocolRef || check.ExecutionStagesHash != old[index].ExecutionStagesHash || check.NativeCheckRef != old[index].NativeCheckRef || check.RequiredCheckRef != old[index].RequiredCheckRef {
			t.Fatal("v7 changed required all-of proof")
		}
	}
	request := RuntimeCommandRequest{ValidationRecipeID: recipe.ID, ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{recipe}, ValidationCheckAttestations: checks}
	if selected, err := shellProjectRequestSelected(request); err != nil || !selected {
		t.Fatal("v7 exact recipe rejected", err)
	}
	request.ValidationCheckAttestations = old
	if _, err := shellProjectRequestSelected(request); err == nil {
		t.Fatal("new recipe implicitly upgraded old protocol")
	}
	request.ValidationRecipes[0].AdapterRef = modulecommon.ValidationRecipeAdapterProjectConjunction
	request.ValidationCheckAttestations = checks
	if _, err := shellProjectRequestSelected(request); err == nil {
		t.Fatal("old recipe accepted new diagnostic protocol")
	}
}

func TestProjectFailureDiagnosticRealOwnerNeverCreditsFailedProject(t *testing.T) {
	for _, scenario := range []string{"exact", "neighbor", "skip", "changed-failure", "legacy"} {
		t.Run(scenario, func(t *testing.T) {
			root, internal := t.TempDir(), t.TempDir()
			recipe := isolatedInputRecipe()
			recipe.InputReviewProtocolRef = runtimecontract.RuntimeValidationInputReviewProtocolV2
			recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectFailureDiagnostic
			if scenario == "legacy" {
				recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectConjunction
			}
			requirement := shellValidationProofRequirementForTest(t, recipe.ID, "diagnostic-cwi")
			checks, ok := shellValidationCheckAttestations(recipe, requirement)
			if !ok {
				t.Fatal("exact mapping missing")
			}
			target := shellValidationNativeCheckTargetRef(requirement, checks...)
			imports := `"testing"; delivery "fixture/delivery"`
			body := `if delivery.Value()!=4{t.Fatal("WORKORCH_ASSERTION_V1 code=value_mismatch expected=4 actual=3")}`
			if scenario == "changed-failure" {
				imports += `; "os"; "strings"`
				body = `if delivery.Value()!=4{if strings.Contains(os.Getenv("GOFLAGS"),"-cover"){t.Fatal("different assertion")};t.Fatal("WORKORCH_ASSERTION_V1 code=value_mismatch expected=4 actual=3")}`
			}
			files := map[string]string{"go.mod": "module fixture\ngo 1.26\n", "delivery/value.go": "package delivery\nfunc Value() int{return 3}\n", target: fmt.Sprintf("package checks\nimport(%s)\nfunc %s(t *testing.T){%s}\n", imports, checks[0].NativeCheckRef, body)}
			if scenario == "neighbor" {
				files["neighbor/old_test.go"] = "package neighbor\nimport \"testing\"\nfunc TestOld(t *testing.T){t.Fatal(\"original failure\")}\n"
			}
			if scenario == "skip" {
				files["neighbor/old_test.go"] = "package neighbor\nimport \"testing\"\nfunc TestOld(t *testing.T){t.Skip(\"unfinished obligation\")}\n"
			}
			var refs []string
			for ref, body := range files {
				writeNativeStructureSource(t, root, ref, body)
				refs = append(refs, ref)
			}
			request := RuntimeCommandRequest{WorkspaceID: "ws", RunID: "run", StepID: "dispatch/op", WorkspaceDeliverableDirectory: root, WorkspaceInternalDirectory: internal, ValidationRecipeID: recipe.ID, ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{recipe}, Command: recipe.Command, Args: recipe.Args, TimeoutMillis: 60000, TargetRefs: []string{target}, ValidationScopeTargetRefs: refs, ValidationCheckAttestations: checks, ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}}
			if err := instrumentManagedValidationNativeChecks(&request); err != nil {
				t.Fatal(err)
			}
			scope := shellProjectRawScope{WorkspaceID: "ws", RunID: "run", ExecutionKey: "diagnostic/exact"}
			result, err := NewRuntimeAdapter().executeShellProjectScope(context.Background(), request, scope)
			if err == nil || result.Conjunction == nil || result.Conjunction.Complete || result.Result.Evidence.Quality == common.EvidenceQualityValid || result.Result.ExitCode == 0 || result.Conjunction.Validate() != nil {
				t.Fatalf("failed project was credited: err=%v result=%+v", err, result)
			}
			if !result.Conjunction.Stages[1].KnownFailure || result.Conjunction.Stages[1].Identity.ID != "project-test" {
				t.Fatal("original project failure erased")
			}
			wantDiagnostic := scenario == "exact"
			if (result.Result.ValidationFailureDiagnostic != nil) != wantDiagnostic {
				t.Fatalf("diagnostic attribution mismatch: %s stages=%v diagnostic=%+v", scenario, result.Conjunction.Stages, result.Result.ValidationFailureDiagnostic)
			}
			if wantDiagnostic {
				if len(result.Conjunction.Stages) != 3 || result.Conjunction.Stages[2].Identity.ID != shellProjectFailureDiagnosticStageID || !reflect.DeepEqual(result.Result.ValidationFailureDiagnostic.TargetRefs, []string{"delivery/value.go"}) || result.Result.ValidationFailureDiagnostic.OwnerCode != modulecommon.ValidationDiagnosticCodeTestFailed {
					t.Fatalf("missing exact instrumented diagnostic: %+v", result.Result.ValidationFailureDiagnostic)
				}
			} else if scenario != "changed-failure" && len(result.Conjunction.Stages) != 2 {
				t.Fatal("ineligible project failure launched diagnostic")
			}
			store, err := openShellProjectRawStore(internal)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			for _, refs := range result.Raw {
				for stream, ref := range map[string]string{"stdout": refs.Stdout, "stderr": refs.Stderr} {
					readScope := scope
					readScope.StageID, readScope.Stream = refs.StageID, stream
					raw, err := store.Read(context.Background(), readScope, ref)
					if err != nil || !shellProjectRawFactValid(raw) {
						t.Fatal("raw diagnostic missing", err)
					}
				}
			}
		})
	}
}

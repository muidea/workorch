package shell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestVersionedValidationInputDoesNotBorrowLegacyNativeIdentity(t *testing.T) {
	requirement := runtimecontract.RuntimeValidationProofRequirement{Hash: "requirement", RequiredCheckRefs: []string{"workorch-cwi-check-1234"}}
	recipe := runtimecontract.RuntimeValidationRecipe{ID: "validation", Command: "go", Args: []string{"test", "./..."}, AdapterRef: modulecommon.ValidationRecipeAdapterProjectEntrypoint}
	legacy, ok := shellValidationCheckAttestations(recipe, requirement)
	if !ok {
		t.Fatal("legacy mapping missing")
	}
	recipe.AdapterRef = modulecommon.ValidationRecipeAdapterVersionedEntrypoint
	current, ok := shellValidationCheckAttestations(recipe, requirement)
	if !ok || current[0].RequiredCheckRef != legacy[0].RequiredCheckRef || current[0].NativeCheckRef == legacy[0].NativeCheckRef {
		t.Fatal("input version changed requirement or reused old proof identity")
	}
	unchangedRecipe := recipe
	unchangedRecipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectEntrypoint
	if replay, ok := shellValidationCheckAttestations(unchangedRecipe, requirement); !ok || !reflect.DeepEqual(replay, legacy) {
		t.Fatal("new adapter upgraded a legacy activation mapping")
	}
	multiple := requirement.Clone()
	multiple.RequiredCheckRefs = append(multiple.RequiredCheckRefs, "workorch-cwi-check-5678")
	if _, ok := shellValidationCheckAttestations(recipe, multiple); ok {
		t.Fatal("single-input adapter advertised a partial multi-check acquisition")
	}
	root := newGoValidationWorkspace(t, "package fixture\nimport \"testing\"\nfunc "+legacy[0].NativeCheckRef+"(t *testing.T){ t.Fatal(\"old backend-only check\") }")
	refs := []string{"go.mod", "fixture_test.go"}
	inspection := inspectShellNativeValidationChecks(root, refs, current)
	if inspection.Code != "native_check_missing" || !shellNativeCheckInspectionPreparable(inspection, requirement, refs, current) {
		t.Fatalf("old input blocked independent acquisition: %+v", inspection)
	}
	if got := inspectShellNativeValidationChecks(root, refs, legacy); got.Code != "" {
		t.Fatalf("old adapter changed: %+v", got)
	}
	target := shellValidationNativeCheckTargetRef(requirement, current...)
	if !strings.HasPrefix(target, "workorch_validation/v3/") || target == shellValidationNativeCheckTargetRef(requirement) {
		t.Fatal("input target not versioned")
	}
	writeNativeStructureSource(t, root, "wrong_test.go", "package fixture\nimport \"testing\"\nfunc "+current[0].NativeCheckRef+"(t *testing.T){ t.Fatal(\"wrong location\") }")
	if got := inspectShellNativeValidationChecks(root, append(refs, "wrong_test.go"), current); got.Code != "native_check_versioned_out_of_scope" {
		t.Fatalf("wrong versioned input location admitted: %+v", got)
	}
}

func TestVersionedValidationInputExecutionCalibration(t *testing.T) {
	const native = "TestWorkOrchCWI_CALIBRATION_InputV3"
	for _, scenario := range []string{"delivery", "wrong-response", "dependency-only", "runner-override"} {
		t.Run(scenario, func(t *testing.T) {
			old := "package fixture\nimport \"testing\"\nfunc TestWorkOrchCWI_CALIBRATION(t *testing.T){ t.Fatal(\"historical input must not be selected\") }"
			root := newGoValidationWorkspace(t, old)
			delivery := strings.ReplaceAll(processBehaviorDelivery, "RUST_PATH", "src/rust/symbol")
			if scenario == "wrong-response" {
				delivery = strings.ReplaceAll(delivery, "src/rust/symbol", "wrong/symbol")
			}
			writeNativeStructureSource(t, root, "cmd/delivery/main.go", delivery)
			target := shellVersionedNativeCheckTarget(native)
			body := `bin:=filepath.Join(t.TempDir(),"delivery"); build:=exec.Command("go","build","-buildvcs=false","-o",bin,"./cmd/delivery"); build.Dir="../.."
if out,e:=build.CombinedOutput(); e!=nil { t.Fatalf("build: %v %s",e,out) }
for _, language:=range []string{"go","rust","python","typescript","shell"} {
cmd:=exec.Command(bin); cmd.Stdin=bytes.NewBufferString("{\"Language\":\""+language+"\"}")
out,e:=cmd.Output(); if e!=nil { t.Fatal(e) }; var got struct{Path string; Line int}
if json.Unmarshal(out,&got)!=nil || got.Path!="src/"+language+"/symbol" || got.Line!=3 { t.Fatalf("invalid response for %s",language) }
}`
			if scenario == "dependency-only" {
				body = `if out,e:=exec.Command("go","version").Output(); e!=nil || len(out)==0 { t.Fatal("dependency unavailable") }`
			}
			input := fmt.Sprintf("package checks\nimport (\"bytes\";\"encoding/json\";\"os/exec\";\"path/filepath\";\"testing\")\nfunc %s(t *testing.T){ _=bytes.NewBufferString; _=json.Unmarshal; _=filepath.Join; %s }", native, body)
			if scenario == "runner-override" {
				input += "\nfunc init(){}"
			}
			writeNativeStructureSource(t, root, target, input)
			checks := []runtimecontract.RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "check", NativeCheckRef: native, ObservationProtocolRef: shellGoEntrypointObservationProtocolRef}}
			request := RuntimeCommandRequest{WorkspaceDeliverableDirectory: root, ValidationRecipeID: "versioned", Command: "go", Args: []string{"test", "./..."}, TimeoutMillis: 60000, TargetRefs: []string{target}, ValidationScopeTargetRefs: []string{"go.mod", "fixture_test.go", "cmd/delivery/main.go", target}, ValidationCheckAttestations: checks, ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}}
			if err := instrumentManagedValidationNativeChecks(&request); err != nil {
				t.Fatal(err)
			}
			output, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), request)
			passed := err == nil && output.Evidence.Quality == common.EvidenceQualityValid && output.ValidationObservation != nil && runtimecontract.RuntimeValidationAttestationPassed(*output.ValidationObservation, checks[0])
			if passed != (scenario == "delivery") {
				t.Fatalf("calibration %s passed=%t err=%v code=%s", scenario, passed, err, output.FailureCode)
			}
			if scenario == "delivery" && !reflect.DeepEqual(output.ValidationObservation.ExecutedChecks, []string{native}) {
				t.Fatalf("old native check participated: %+v", output.ValidationObservation)
			}
			for ref, want := range map[string]string{"fixture_test.go": old, target: input, "cmd/delivery/main.go": delivery} {
				got, e := os.ReadFile(filepath.Join(root, ref))
				if e != nil || string(got) != want {
					t.Fatalf("validation rewrote %s", ref)
				}
			}
		})
	}
}

func TestVersionedInputPublishesExactReviewSubjectNotConsistencyVerdict(t *testing.T) {
	recipe := runtimecontract.RuntimeValidationRecipe{ID: "native", Command: "go", Args: []string{"test", "./..."}, AdapterRef: modulecommon.ValidationRecipeAdapterVersionedEntrypoint}
	requirement := shellValidationProofRequirementForTest(t, recipe.ID, "cwi-versioned")
	checks, ok := shellValidationCheckAttestations(recipe, requirement)
	if !ok {
		t.Fatal("missing versioned mapping")
	}
	root := t.TempDir()
	target := shellValidationNativeCheckTargetRef(requirement, checks...)
	writeNativeStructureSource(t, root, target, fmt.Sprintf("package checks\nimport \"testing\"\nfunc %s(t *testing.T){ if false { t.Fatal(\"structurally valid but not semantic proof\") } }", checks[0].NativeCheckRef))
	binding := shellTestWorkspaceManifestBinding("same-tree", []string{target})
	for range 2 {
		surfaces := shellValidationSurfaces([]runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationProofRequirement{requirement}, binding, "catalog-versioned", root)
		if len(surfaces) != 1 || surfaces[0].Validate() != nil || len(surfaces[0].ProofRequirementBindings) != 1 || len(surfaces[0].ProofRequirementPreparations) != 0 || len(surfaces[0].ProofRequirementUnavailabilities) != 0 {
			t.Fatalf("owner review subject unavailable: %+v", surfaces)
		}
		proof := surfaces[0].ProofRequirementBindings[0]
		if proof.InputReview == nil || proof.InputReview.ValidateAgainstBinding(proof) != nil || len(proof.InputReview.Files) != 1 || proof.InputReview.Files[0].TargetRef != target {
			t.Fatal("missing exact independently reviewable source")
		}
	}
}

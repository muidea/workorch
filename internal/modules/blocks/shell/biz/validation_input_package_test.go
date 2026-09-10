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

func isolatedInputRecipe() runtimecontract.RuntimeValidationRecipe {
	return runtimecontract.RuntimeValidationRecipe{ID: "isolated", Command: "go", Args: []string{"test", "-v", "./..."}, AdapterRef: modulecommon.ValidationRecipeAdapterIsolatedEntrypoint}
}

func TestIsolatedInputReviewScalesAcrossTwentyFiveCWIs(t *testing.T) {
	root, recipe := t.TempDir(), isolatedInputRecipe()
	writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
	refs := []string{"go.mod"}
	var requirements []runtimecontract.RuntimeValidationProofRequirement
	for index := range 25 {
		requirement := shellValidationProofRequirementForTest(t, recipe.ID, common.CoverageWorkItemID(fmt.Sprintf("cwi-%03d", index)))
		checks, ok := shellValidationCheckAttestations(recipe, requirement)
		if !ok {
			t.Fatal("missing exact mapping")
		}
		target := shellValidationNativeCheckTargetRef(requirement, checks...)
		writeNativeStructureSource(t, root, target, fmt.Sprintf("package checks\nimport \"testing\"\nfunc %s(t *testing.T){ if false { t.Fatal(\"not semantic proof\") } }", checks[0].NativeCheckRef))
		refs = append(refs, target)
		requirements = append(requirements, requirement)
	}
	acquire := func(refs []string) []runtimecontract.RuntimeValidationProofRequirementBinding {
		t.Helper()
		binding := shellTestWorkspaceManifestBinding("current-tree", refs)
		surfaces := shellValidationSurfaces([]runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, requirements, binding, "current-policy", root)
		if len(surfaces) != 1 || surfaces[0].Validate() != nil || len(surfaces[0].ProofRequirementBindings) != 25 || len(surfaces[0].InputTargetRefs) != len(refs)-1 {
			t.Fatalf("full-size review/protection did not remain independent: %+v", surfaces)
		}
		return surfaces[0].ProofRequirementBindings
	}
	proofs := acquire(refs)
	for _, proof := range proofs {
		if proof.InputReview == nil || len(proof.InputReview.Files) != 2 || proof.InputReview.ValidateAgainstBinding(proof) != nil || proof.InputReview.ScopeContractRef != shellIsolatedReviewScopeContractRef || proof.InputReview.ScopeRequirements != shellIsolatedReviewScopeRequirements {
			t.Fatal("review omitted exact input or exceeded per-CWI scope")
		}
	}
	first := proofs[0]
	target := shellVersionedNativeCheckTarget(first.CheckAttestations[0].NativeCheckRef)
	helper := filepath.ToSlash(filepath.Join(filepath.Dir(target), "helper.go"))
	writeNativeStructureSource(t, root, helper, "package checks\nconst expected = 1\n")
	updated := acquire(append(refs, helper))
	for _, proof := range updated {
		if proof.RequirementHash == first.RequirementHash {
			if len(proof.InputReview.Files) != 3 || proof.InputReview.Hash == first.InputReview.Hash {
				t.Fatal("same-package helper was not fully reviewed")
			}
			continue
		}
		for _, old := range proofs {
			if old.RequirementHash == proof.RequirementHash && old.InputReview.Hash != proof.InputReview.Hash {
				t.Fatal("another CWI input changed an independent review subject")
			}
		}
	}
	// The reviewed preimage, not a later read, fences a changed helper.
	changedProof := updated[0]
	for _, proof := range updated {
		if proof.RequirementHash == first.RequirementHash {
			changedProof = proof
		}
	}
	writeNativeStructureSource(t, root, helper, "package checks\nconst expected = 2\n")
	inventory := runtimecontract.RuntimeOperationAdmissibility{ValidationSurfaces: []runtimecontract.RuntimeValidationSurface{{ProofRequirementBindings: []runtimecontract.RuntimeValidationProofRequirementBinding{changedProof}}}}
	observation := NewRuntimeAdapter().runtimeAdmissibilityObservation(root, "review", inventory, append(refs, helper))
	for _, dependency := range observation.Dependencies {
		if dependency.SubjectRef == helper && dependency.Revision == runtimecontract.RuntimeValidationInputContentHash("package checks\nconst expected = 1\n") {
			return
		}
	}
	t.Fatal("freshness lost reviewed helper preimage")
}

func TestIsolatedInputScopeFailsClosedWithoutChangingLegacy(t *testing.T) {
	recipe := isolatedInputRecipe()
	requirement := shellValidationProofRequirementForTest(t, recipe.ID, "cwi-scope")
	checks, _ := shellValidationCheckAttestations(recipe, requirement)
	target := shellValidationNativeCheckTargetRef(requirement, checks...)
	pkg := filepath.ToSlash(filepath.Dir(target))
	base := fmt.Sprintf("package checks\nimport \"testing\"\nfunc %s(t *testing.T){ if false { t.Fatal(\"assert\") } }", checks[0].NativeCheckRef)
	binding, _ := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(requirement, recipe.ID, checks...)
	for _, scenario := range []string{"bytes", "files", "symlink", "nested-module", "ancestor-module", "nested-package", "helper-init", "helper-syntax", "cross-harness"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			writeNativeStructureSource(t, root, target, base)
			ref, content := pkg+"/helper.go", "package checks\n"
			refs, want := []string{target}, "validation_input_review_capacity_exceeded"
			switch scenario {
			case "bytes":
				content += strings.Repeat(" ", runtimecontract.RuntimeValidationInputReviewMaxBytes)
			case "files":
				for i := range runtimecontract.RuntimeValidationInputReviewMaxFiles {
					file := fmt.Sprintf("%s/fixture-%d.txt", pkg, i)
					writeNativeStructureSource(t, root, file, "fixture")
					refs = append(refs, file)
				}
			case "symlink":
				ref, want = pkg+"/link.txt", "validation_input_review_source_unavailable"
				if err := os.Symlink(filepath.Join(root, target), filepath.Join(root, ref)); err != nil {
					t.Fatal(err)
				}
			case "nested-module":
				ref, content, want = pkg+"/go.mod", "module nested\n", "validation_input_review_scope_unsupported"
			case "ancestor-module":
				ref, content, want = "workorch_validation/go.mod", "module ancestor\n", "validation_input_review_scope_unsupported"
			case "nested-package":
				ref, want = pkg+"/helper/helper.go", "validation_input_review_scope_unsupported"
			case "helper-init":
				content += "func init(){}\n"
				want = "native_check_versioned_contract_invalid"
			case "helper-syntax":
				content += "func broken(\n"
				want = "native_check_source_invalid"
			case "cross-harness":
				content += "import _ \"fixture/workorch_validation/v3\"\n"
				want = "native_check_versioned_contract_invalid"
			}
			if scenario != "symlink" {
				writeNativeStructureSource(t, root, ref, content)
			}
			refs = append(refs, ref)
			if scenario == "helper-init" || scenario == "helper-syntax" || scenario == "cross-harness" {
				if got := inspectShellNativeValidationChecks(root, refs, checks); got.Code != want {
					t.Fatalf("unsupported check accepted: %+v", got)
				}
			} else if _, code := shellVersionedInputReview(root, refs, binding); code != want {
				t.Fatalf("incomplete review accepted: %s != %s", code, want)
			}
		})
	}
	missingRoot := t.TempDir()
	prep, ready, ok := shellValidationProofPreparation(missingRoot, runtimecontract.WorkspaceManifest{Revision: "empty"}, "catalog", false, recipe, requirement, checks)
	if !ok || ready || prep.Validate() != nil || prep.ContentContractRef != shellIsolatedInputContentContractRef || prep.ContentRequirements == "" || prep.TargetRef != target {
		t.Fatalf("create-only input contract unavailable: %+v", prep)
	}
	legacy := recipe
	legacy.AdapterRef = modulecommon.ValidationRecipeAdapterVersionedEntrypoint
	oldChecks, _ := shellValidationCheckAttestations(legacy, requirement)
	oldPrep, _, _ := shellValidationProofPreparation(missingRoot, runtimecontract.WorkspaceManifest{Revision: "empty"}, "catalog", false, legacy, requirement, oldChecks)
	if oldPrep.ContentContractRef != "" || oldChecks[0].ObservationProtocolRef != shellGoEntrypointObservationProtocolRef || !strings.HasSuffix(oldChecks[0].NativeCheckRef, "_InputV3") {
		t.Fatal("legacy sealed contract silently upgraded")
	}
}

func TestIsolatedInputSourceMembershipCannotOmitOnDiskInputs(t *testing.T) {
	for _, scenario := range []string{"helper", "fixture", "root-module", "removed-input", "directory-symlink", "unlisted-ancestor-module"} {
		t.Run(scenario, func(t *testing.T) {
			root, recipe := t.TempDir(), isolatedInputRecipe()
			requirement := shellValidationProofRequirementForTest(t, recipe.ID, "cwi-membership")
			checks, _ := shellValidationCheckAttestations(recipe, requirement)
			target := shellValidationNativeCheckTargetRef(requirement, checks...)
			writeNativeStructureSource(t, root, "go.mod", "module fixture\ngo 1.26\n")
			writeNativeStructureSource(t, root, target, fmt.Sprintf("package checks\nimport \"testing\"\nfunc %s(t *testing.T){ if false {t.Fatal(\"assert\")} }", checks[0].NativeCheckRef))
			refs, want := []string{"go.mod", target}, "validation_input_review_source_set_changed"
			binding, _ := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(requirement, recipe.ID, checks...)
			if _, code := shellVersionedInputReview(root, refs, binding); code != "" {
				t.Fatal(code)
			}
			pkg := filepath.ToSlash(filepath.Dir(target))
			switch scenario {
			case "helper":
				writeNativeStructureSource(t, root, pkg+"/unlisted.go", "package checks\nconst value=1\n")
			case "fixture":
				writeNativeStructureSource(t, root, pkg+"/fixtures/expected.txt", "expected")
			case "root-module":
				refs = refs[1:]
			case "removed-input":
				if err := os.Remove(filepath.Join(root, target)); err != nil {
					t.Fatal(err)
				}
			case "directory-symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(root, pkg, "fixtures")); err != nil {
					t.Fatal(err)
				}
				want = "validation_input_review_source_unavailable"
			case "unlisted-ancestor-module":
				writeNativeStructureSource(t, root, "workorch_validation/go.mod", "module ancestor\n")
				want = "validation_input_review_scope_unsupported"
			}
			if _, code := shellVersionedInputReview(root, refs, binding); code != want {
				t.Fatalf("review accepted mismatched manifest: %s != %s", code, want)
			}
			if inspection := inspectShellNativeValidationChecks(root, refs, checks); inspection.Code != want {
				t.Fatalf("execution readiness accepted mismatched manifest: %+v", inspection)
			}
		})
	}
}

func TestIsolatedInputExecutesOnlyReviewedPackageAndActualDelivery(t *testing.T) {
	for _, scenario := range []string{"delivery", "wrong-response", "dependency-only", "skip", "helper-override", "unlisted-helper", "supplement"} {
		t.Run(scenario, func(t *testing.T) {
			root, recipe := t.TempDir(), isolatedInputRecipe()
			requirement := shellValidationProofRequirementForTest(t, recipe.ID, "cwi-delivery")
			checks, _ := shellValidationCheckAttestations(recipe, requirement)
			target := shellValidationNativeCheckTargetRef(requirement, checks...)
			delivery := "package main\nimport \"fmt\"\nfunc main(){fmt.Print(4)}\n"
			if scenario == "wrong-response" {
				delivery = strings.Replace(delivery, "Print(4)", "Print(2)", 1)
			}
			body := `bin:=filepath.Join(t.TempDir(),"delivery"); build:=exec.Command("go","build","-buildvcs=false","-o",bin,"./cmd/delivery"); build.Dir="../../.."
if out,e:=build.CombinedOutput(); e!=nil { t.Fatalf("build: %v %s",e,out) }
out,e:=exec.Command(bin).Output(); if e!=nil || string(out)!="4" { t.Fatal("wrong response") }`
			if scenario == "dependency-only" {
				body = `if out,e:=exec.Command("go","version").Output(); e!=nil || len(out)==0 { t.Fatal("dependency missing") }`
			}
			if scenario == "skip" {
				body += `; t.Run("required",func(t *testing.T){t.Skip("dependency unavailable")})`
			}
			input := fmt.Sprintf("package checks\nimport (\"testing\";\"os/exec\";\"path/filepath\")\nfunc %s(t *testing.T){ _=filepath.Join; %s }\n", checks[0].NativeCheckRef, body)
			files := map[string]string{
				"go.mod": "module fixture\ngo 1.26\n", target: input, "cmd/delivery/main.go": delivery,
				"workorch_validation/v3/broken_test.go":        "package old\nfunc init(){panic(\"unrelated old input\")}\nfunc broken(\n",
				"workorch_validation/v4/another/input_test.go": "package another\nfunc init(){panic(\"another CWI\")}\n",
			}
			if scenario == "helper-override" {
				files[filepath.ToSlash(filepath.Join(filepath.Dir(target), "helper.go"))] = "package checks\nfunc init(){}\n"
			}
			if scenario == "supplement" {
				files[shellSupplementTargetRef(checks[0].NativeCheckRef)] = fmt.Sprintf("package checks\nimport \"testing\"\nfunc %s(t *testing.T){ if 2+2!=4 {t.Fatal(\"supplement\")} }", shellSupplementCheckRef(checks[0].NativeCheckRef))
			}
			var refs []string
			for ref, content := range files {
				writeNativeStructureSource(t, root, ref, content)
				refs = append(refs, ref)
			}
			if scenario == "unlisted-helper" {
				// Arrives after the manifest/review scope was bound; a Go loader
				// would otherwise execute it despite its absent review bytes.
				writeNativeStructureSource(t, root, filepath.ToSlash(filepath.Join(filepath.Dir(target), "unlisted.go")), "package checks\nfunc init(){panic(\"must not execute\")}\n")
			}
			checks = shellValidationSupplementAttestations(checks, refs)
			request := RuntimeCommandRequest{WorkspaceDeliverableDirectory: root, ValidationRecipeID: recipe.ID, Command: recipe.Command, Args: recipe.Args, TimeoutMillis: 60000, TargetRefs: []string{target}, ValidationScopeTargetRefs: refs, ValidationCheckAttestations: checks, ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}}
			if err := instrumentManagedValidationNativeChecks(&request); err != nil {
				t.Fatal(err)
			}
			if request.Args[len(request.Args)-1] != "./"+shellIsolatedNativePackage(checks[0].NativeCheckRef) {
				t.Fatal("execution did not use exact reviewed package")
			}
			output, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), request)
			passed := err == nil && output.Evidence.Quality == common.EvidenceQualityValid && output.ValidationObservation != nil && runtimecontract.RuntimeValidationAttestationPassed(*output.ValidationObservation, checks[0])
			if passed != (scenario == "delivery" || scenario == "supplement") {
				t.Fatalf("scenario=%s passed=%t err=%v code=%s observation=%+v", scenario, passed, err, output.FailureCode, output.ValidationObservation)
			}
			if (scenario == "helper-override" || scenario == "unlisted-helper") && (err == nil || output.FailureCode != "validation_behavior_contract_rejected" || output.ValidationObservation != nil) {
				t.Fatalf("unreviewable helper reached process execution: err=%v result=%+v", err, output)
			}
			if scenario == "delivery" && !reflect.DeepEqual(output.ValidationObservation.ExecutedChecks, []string{checks[0].NativeCheckRef}) {
				t.Fatal("unrelated CWI received execution credit")
			}
			for ref, want := range files {
				got, err := os.ReadFile(filepath.Join(root, ref))
				if err != nil || string(got) != want {
					t.Fatalf("managed execution changed input %s", ref)
				}
			}
		})
	}
}

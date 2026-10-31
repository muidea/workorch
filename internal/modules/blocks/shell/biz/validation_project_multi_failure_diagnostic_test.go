package shell

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	modulecommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestProjectMultiFailureSourcePublicationIsExactAndProtectsOriginalTests(t *testing.T) {
	recipe, requirement, _ := conjunctiveSourceRequirement(t)
	recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic
	recipe.InputReviewProtocolRef = runtimecontract.RuntimeValidationInputReviewProtocol
	checks, ok := shellValidationCheckAttestations(recipe, requirement)
	if !ok {
		t.Fatal("v8 mapping missing")
	}
	root := t.TempDir()
	target := shellValidationNativeCheckTargetRef(requirement, checks...)
	files := map[string]string{"go.mod": "module fixture\ngo 1.26\n", target: conjunctiveSourceInput(checks, requirement.SourceObligationChecks[1].CheckRef, `if delivery.Compute(4)!=11 {t.Fatal("B")}`), "delivery/value.go": "package delivery\nfunc Compute(n int) int {return n*2+3}\n", "delivery/production_test.go": "package delivery\nimport \"testing\"\nfunc TestProduction(t *testing.T){if Compute(4)!=11{t.Fatal(\"wrong\")}}\n"}
	var refs []string
	for ref, body := range files {
		writeNativeStructureSource(t, root, ref, body)
		refs = append(refs, ref)
	}
	slices.Sort(refs)
	manifest := shellTestWorkspaceManifestBinding("source-publication", refs)
	surfaces := shellValidationSurfaces([]runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationProofRequirement{requirement}, manifest, "policy", root)
	if len(surfaces) != 1 || len(surfaces[0].ProofRequirementBindings) != 1 {
		t.Fatalf("actual owner publication missing: %+v", surfaces)
	}
	binding := surfaces[0].ProofRequirementBindings[0]
	if binding.Validate() != nil || binding.InputReview.DiagnosticSourceContractRef != runtimecontract.RuntimeValidationDiagnosticSourceReviewV1 {
		t.Fatal("source publication not version bound")
	}
	if !slices.Contains(surfaces[0].InputTargetRefs, "delivery/production_test.go") || slices.Contains(surfaces[0].InputTargetRefs, "delivery/value.go") {
		t.Fatal("original test protection or implementation role lost")
	}
	for _, file := range binding.InputReview.Files {
		if file.TargetRef == "delivery/value.go" {
			t.Fatal("mutable implementation became immutable input")
		}
	}
	found := false
	for _, source := range binding.InputReview.DiagnosticSources {
		if source.NativeCaseRef == "fixture/delivery/TestProduction" {
			found = source.InputTargetRef == "delivery/production_test.go"
		}
	}
	if !found {
		t.Fatal("production native source absent")
	}
	writeNativeStructureSource(t, root, "delivery/value.go", strings.Replace(files["delivery/value.go"], "n*2+3", "n*2+4", 1))
	current, code := shellInputReview(root, refs, binding)
	if code != "" || current.Hash != binding.InputReview.Hash {
		t.Fatal("legitimate implementation repair changed immutable test subject", code)
	}
	for _, protocol := range []string{"", "unsupported-review-contract"} {
		unsupported := recipe
		unsupported.InputReviewProtocolRef = protocol
		if shellProjectRecipeDeclared(unsupported) {
			t.Fatal("project adapter accepted unreviewed sources")
		}
	}
}

func TestProjectMultiFailureDiagnosticActualSourcesAndBehavior(t *testing.T) {
	for _, scenario := range []string{"exact", "disjoint", "unobserved", "partial-unobserved", "skip", "drift", "missing-source", "changed-assertion", "legacy"} {
		t.Run(scenario, func(t *testing.T) {
			root, internal := t.TempDir(), t.TempDir()
			recipe := isolatedInputRecipe()
			recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectMultiFailureDiagnostic
			recipe.InputReviewProtocolRef = runtimecontract.RuntimeValidationInputReviewProtocol
			requirement := shellValidationProofRequirementForTest(t, recipe.ID, "diagnostic-cwi")
			checks, ok := shellValidationCheckAttestations(recipe, requirement)
			if !ok {
				t.Fatal("v8 mapping missing")
			}
			target := shellValidationNativeCheckTargetRef(requirement, checks...)
			files := map[string]string{"go.mod": "module fixture\ngo 1.26\n", "delivery/value.go": "package delivery\nfunc Value() int{return 3}\n", target: fmt.Sprintf("package checks\nimport(\"testing\"; delivery \"fixture/delivery\")\nfunc %s(t *testing.T){if delivery.Value()!=4{t.Fatal(\"WORKORCH_ASSERTION_V1 code=value_mismatch expected=4 actual=3\")}}\n", checks[0].NativeCheckRef), "delivery/value_test.go": "package delivery\nimport \"testing\"\nfunc TestProduction(t *testing.T){if Value()!=4{t.Fatal(\"WORKORCH_ASSERTION_V1 code=value_mismatch expected=4 actual=3\")}}\n"}
			if scenario == "disjoint" {
				files["delivery/other.go"] = "package delivery\nfunc Other() int{return 2}\n"
				files["delivery/value_test.go"] = "package delivery\nimport \"testing\"\nfunc TestProduction(t *testing.T){if Other()!=4{t.Fatal(\"WORKORCH_ASSERTION_V1 code=other_mismatch expected=4 actual=2\")}}\n"
			}
			if scenario == "unobserved" {
				files["delivery/value_test.go"] = "package delivery\nimport \"testing\"\nfunc TestProduction(t *testing.T){t.Fatal(\"WORKORCH_ASSERTION_V1 code=value_mismatch expected=4 actual=3\")}\n"
			}
			if scenario == "skip" {
				files["delivery/skip_test.go"] = "package delivery\nimport \"testing\"\nfunc TestSkipped(t *testing.T){t.Skip(\"unfinished\")}\n"
			}
			if scenario == "changed-assertion" {
				files["delivery/value_test.go"] = "package delivery\nimport(\"testing\";\"os\";\"strings\")\nfunc TestProduction(t *testing.T){if Value()!=4{if strings.Contains(os.Getenv(\"GOFLAGS\"),\"-cover\"){t.Fatal(\"changed assertion\")};t.Fatal(\"original assertion\")}}\n"
			}
			var refs []string
			for ref, body := range files {
				writeNativeStructureSource(t, root, ref, body)
				refs = append(refs, ref)
			}
			slices.Sort(refs)
			binding, err := runtimecontract.RuntimeValidationProofRequirementBindingForRecipe(requirement, recipe.ID, checks...)
			if err != nil {
				t.Fatal(err)
			}
			binding.InputReview, _ = shellInputReview(root, refs, binding)
			if binding.InputReview == nil {
				t.Fatal("complete project input not observed")
			}
			if scenario == "partial-unobserved" {
				last := binding.InputReview.DiagnosticSources[len(binding.InputReview.DiagnosticSources)-1]
				body := files[last.InputTargetRef]
				body = strings.ReplaceAll(body, "if delivery.Value()!=4", "if true")
				body = strings.ReplaceAll(body, "if Value()!=4", "if true")
				body = strings.ReplaceAll(body, `delivery "fixture/delivery"`, `_ "fixture/delivery"`)
				writeNativeStructureSource(t, root, last.InputTargetRef, body)
				binding.InputReview, _ = shellInputReview(root, refs, binding)
				if binding.InputReview == nil {
					t.Fatal("partial attribution fixture not independently observed")
				}
			}
			// This is a direct owner producer fixture, not a Core binding/credit test.
			if scenario == "drift" {
				writeNativeStructureSource(t, root, "delivery/value_test.go", files["delivery/value_test.go"]+"// changed after review\n")
			}
			if scenario == "missing-source" {
				binding.InputReview.DiagnosticSources = binding.InputReview.DiagnosticSources[:1]
				binding.InputReview.Hash = runtimecontract.StableRuntimeValidationInputReviewSubjectHash(*binding.InputReview, binding)
			}
			if scenario == "legacy" {
				recipe.AdapterRef = modulecommon.ValidationRecipeAdapterProjectFailureDiagnostic
				checks, _ = shellValidationCheckAttestations(recipe, requirement)
			}
			request := RuntimeCommandRequest{WorkspaceID: "ws", RunID: "run", StepID: "dispatch/op", WorkspaceDeliverableDirectory: root, WorkspaceInternalDirectory: internal, ValidationRecipeID: recipe.ID, ValidationRecipes: []runtimecontract.RuntimeValidationRecipe{recipe}, Command: recipe.Command, Args: recipe.Args, TimeoutMillis: 60000, TargetRefs: []string{target}, ValidationScopeTargetRefs: refs, ValidationCheckAttestations: checks, validationProofBinding: binding, ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}}
			if err := instrumentManagedValidationNativeChecks(&request); err != nil {
				t.Fatal(err)
			}
			result, err := NewRuntimeAdapter().executeShellProjectScope(context.Background(), request, shellProjectRawScope{WorkspaceID: "ws", RunID: "run", ExecutionKey: "multi-failure/" + scenario})
			if err == nil || result.Conjunction == nil || result.Conjunction.Complete || result.Result.Evidence.Quality == common.EvidenceQualityValid || result.Conjunction.Validate() != nil {
				t.Fatalf("failed project credited or lost: err=%v conjunction=%+v", err, result.Conjunction)
			}
			wantDiagnostic := scenario == "exact" || scenario == "disjoint"
			if (result.Result.ValidationFailureDiagnostic != nil) != wantDiagnostic {
				t.Fatalf("%s wrong attribution: diagnostic=%+v stages=%+v", scenario, result.Result.ValidationFailureDiagnostic, result.Conjunction.Stages)
			}
			if scenario == "partial-unobserved" {
				attributed := 0
				for _, stage := range result.Conjunction.Stages {
					if stage.DiagnosticSourceRef != "" {
						attributed++
					}
				}
				if attributed != 1 || !reflect.DeepEqual(result.Conjunction.BehaviorTargetRefs, []string{"delivery/value.go"}) || result.Result.Evidence.Validation == nil {
					t.Fatal("later unknown erased real earlier source attribution")
				}
				if err := runtimecontract.ValidateRuntimeValidationDiagnosticFailureObservation(result.Result.Evidence.Validation, binding, refs, nil); err != nil {
					t.Fatal("partial historical source facts cannot be preserved", err)
				}
			}
			if wantDiagnostic {
				wantTargets := []string{"delivery/value.go"}
				if scenario == "disjoint" {
					wantTargets = []string{"delivery/other.go", "delivery/value.go"}
				}
				if result.Result.Evidence.Validation == nil || result.Result.Evidence.Validation.ObservationProtocolRef != shellProjectMultiFailureDiagnosticProtocolRef || result.Result.Evidence.Validation.ExecutionConjunction == nil || !reflect.DeepEqual(result.Result.Evidence.Validation.ExecutionConjunction, result.Conjunction) {
					t.Fatal("receipt projection dropped source-specific conjunction or downgraded protocol")
				}
				if len(result.Conjunction.Stages) != 4 || !reflect.DeepEqual(result.Result.ValidationFailureDiagnostic.TargetRefs, wantTargets) || !reflect.DeepEqual(result.Conjunction.BehaviorTargetRefs, wantTargets) {
					t.Fatal("missing independent source attribution")
				}
				for _, stage := range result.Conjunction.Stages[2:] {
					wantSourceTargets := []string{"delivery/value.go"}
					if scenario == "disjoint" {
						for _, source := range binding.InputReview.DiagnosticSources {
							if source.SourceRef == stage.DiagnosticSourceRef && source.NativeCaseRef == "fixture/delivery/TestProduction" {
								wantSourceTargets = []string{"delivery/other.go"}
							}
						}
					}
					if stage.DiagnosticSourceRef == "" || !reflect.DeepEqual(stage.BehaviorTargetRefs, wantSourceTargets) || stage.Stdout.Storage == nil || stage.Stdout.Storage.Reference == "" {
						t.Fatal("source-specific real behavior/raw fact missing")
					}
				}
			}
			if result.Conjunction.Stages[1].Identity.ID != "project-test" || !result.Conjunction.Stages[1].KnownFailure {
				t.Fatal("original failure erased")
			}
		})
	}
}

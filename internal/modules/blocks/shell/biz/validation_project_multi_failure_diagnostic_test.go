package shell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

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
	for _, scenario := range []string{"native-source-channel", "interleaved-output", "large-output", "dynamic-observation", "isolated-overlap", "cancel-overlap", "diagnostic-capacity", "exact", "disjoint", "unobserved", "partial-unobserved", "missing-assertion", "over-capacity", "skip", "drift", "missing-source", "changed-assertion", "additive-input-test", "legacy"} {
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
			if scenario == "native-source-channel" {
				for _, ref := range []string{target, "delivery/value_test.go"} {
					body := strings.Replace(files[ref], `"testing"`, `"testing";"os";"os/exec"`, 1)
					if ref == "delivery/value_test.go" {
						body = strings.Replace(body, `import "testing";"os";"os/exec"`, `import("testing";"os";"os/exec")`, 1)
					}
					body = strings.Replace(body, "(t *testing.T){", "(t *testing.T){if managedNativeSource(t){return};", 1)
					files[ref] = body + managedNativeSourceTestHelper
				}
			}
			if scenario == "dynamic-observation" {
				files["delivery/value_test.go"] = `package delivery
import("testing";"time")
func TestProduction(t *testing.T){
 for i:=0;i<80;i++ { t.Logf("ordinary observation elapsed=%d",time.Now().UnixNano()) }
 if Value()!=4 { t.Fatal("stable expected=4 actual=3") }
}
`
			}
			barrier := ""
			if scenario == "isolated-overlap" || scenario == "cancel-overlap" {
				barrier = t.TempDir()
				files["delivery/value_test.go"] = fmt.Sprintf(`package delivery
import("testing";"os";"path/filepath";"time")
func exercise(t *testing.T,name,other string){
 f,err:=os.OpenFile("source.lock",os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600)
 if err!=nil {t.Fatal("diagnostic workspace was shared")};f.Close();defer os.Remove("source.lock")
 if testing.CoverMode()!="" {
  root:=%q
  if err:=os.WriteFile(filepath.Join(root,name),[]byte("ready"),0600);err!=nil {t.Fatal(err)}
  until:=time.Now().Add(5*time.Second)
  for {if _,err:=os.Stat(filepath.Join(root,other));err==nil {break};if time.Now().After(until){t.Fatal("independent diagnostics did not overlap")};time.Sleep(time.Millisecond)}
 }
 if %t && testing.CoverMode()!="" {for {time.Sleep(time.Millisecond)}}
 if Value()!=4 {t.Fatal("stable original failure")}
}
func TestProduction(t *testing.T){exercise(t,"first","second")}
func TestOther(t *testing.T){exercise(t,"second","first")}
`, barrier, scenario == "cancel-overlap")
			}

			if scenario == "diagnostic-capacity" {
				files["delivery/value_test.go"] = `package delivery
import "testing"
func TestProduction(t *testing.T){
 if testing.CoverMode()!="" {for n:=0;n<80;n++ {t.Errorf("independent failure %d",n)}}
 if Value()!=4 {t.Fatal("stable original failure")}
}
`
			}
			if scenario == "missing-assertion" {
				files["delivery/value_test.go"] = "package delivery\nimport \"testing\"\nfunc TestProduction(t *testing.T){if Value()!=4{t.Fail()}}\n"
			}
			if scenario == "over-capacity" {
				for i := 0; i < 6; i++ {
					files[fmt.Sprintf("delivery/extra%d_test.go", i)] = fmt.Sprintf("package delivery\nimport \"testing\"\nfunc TestExtra%d(t *testing.T){if Value()!=4{t.Fatal(\"extra failure\")}}\n", i)
				}
			}
			if scenario == "interleaved-output" {
				files["delivery/value_test.go"] = strings.Replace(files["delivery/value_test.go"], `import "testing"`, `import("testing";"fmt")`, 1)
				files["delivery/value_test.go"] = strings.Replace(files["delivery/value_test.go"], "if Value()!=4", `t.Log("ordinary source log");fmt.Println("unrelated process output");if Value()!=4`, 1)
			}
			if scenario == "large-output" {
				files["delivery/value_test.go"] = strings.Replace(files["delivery/value_test.go"], `import "testing"`, `import("testing";"strings")`, 1)
				files["delivery/value_test.go"] = strings.Replace(files["delivery/value_test.go"], "if Value()!=4", `for range 600 {t.Log(strings.Repeat("x",4096))};if Value()!=4`, 1)
			}
			if scenario == "additive-input-test" {
				files[target] += "func TestAdditive(t *testing.T){if delivery.Value()!=4{t.Fatal(\"WORKORCH_ASSERTION_V1 code=value_mismatch expected=4 actual=3\")}}\n"
				if inspection := inspectShellNativeValidationChecks(root, nil, checks); inspection.Code == "" {
					t.Fatal("absent original must not be ready")
				}
			}
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
			var reviewCode string
			binding.InputReview, reviewCode = shellInputReview(root, refs, binding)
			if binding.InputReview == nil {
				t.Fatal("complete project input not observed", reviewCode)
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
			executionContext := context.Background()
			var joined chan bool
			if scenario == "cancel-overlap" {
				child, cancel := context.WithCancel(executionContext)
				defer cancel()
				executionContext = child
				joined = make(chan bool, 1)
				go func() {
					timer := time.NewTimer(15 * time.Second)
					defer timer.Stop()
					ticker := time.NewTicker(time.Millisecond)
					defer ticker.Stop()
					for {
						select {
						case <-timer.C:
							cancel()
							joined <- false
							return
						case <-ticker.C:
							_, a := os.Stat(filepath.Join(barrier, "first"))
							_, b := os.Stat(filepath.Join(barrier, "second"))
							if a == nil && b == nil {
								cancel()
								joined <- true
								return
							}
						}
					}
				}()
			}
			result, err := NewRuntimeAdapter().executeShellProjectScope(executionContext, request, shellProjectRawScope{WorkspaceID: "ws", RunID: "run", ExecutionKey: "multi-failure/" + scenario})
			if joined != nil {
				if result.Conjunction == nil {
					t.Fatal("cancelled diagnostic stage history missing")
				}
				if !<-joined {
					t.Fatal("cancellation did not observe both independently running diagnostics")
				}
				isolated := 0
				for _, stage := range result.Conjunction.Stages {
					if stage.DiagnosticIsolationRef != "" {
						isolated++
					}
				}
				if isolated < 2 || result.Result.ValidationFailureDiagnostic != nil {
					t.Fatal("cancelled independent jobs lost joined history or published attribution without complete output storage")
				}
				for _, resource := range result.Result.ValidationResources {
					if resource.Status != "released" {
						t.Fatal("cancelled workspace/process lifetime was not drained")
					}
				}
			}
			if err == nil || result.Conjunction == nil || result.Conjunction.Complete || result.Result.Evidence.Quality == common.EvidenceQualityValid || result.Conjunction.Validate() != nil {
				t.Fatalf("failed project credited or lost: err=%v conjunction=%+v", err, result.Conjunction)
			}
			wantDiagnostic := scenario == "native-source-channel" || scenario == "dynamic-observation" || scenario == "isolated-overlap" || scenario == "interleaved-output" || scenario == "large-output" || scenario == "exact" || scenario == "disjoint" || scenario == "additive-input-test"
			diagnostic := result.Result.ValidationFailureDiagnostic
			if wantDiagnostic {
				if diagnostic == nil || diagnostic.Unavailability != "" {
					t.Fatalf("%s lost complete attribution: %+v", scenario, diagnostic)
				}
			} else if scenario != "legacy" && scenario != "cancel-overlap" {
				if diagnostic == nil || diagnostic.Unavailability == "" || len(diagnostic.TargetRefs) != 0 || diagnostic.Class != runtimecontract.ValidationDiagnosticOutputInconclusive || result.Result.Evidence.Validation == nil {
					t.Fatalf("%s lost explicit diagnostic rejection: %+v", scenario, diagnostic)
				}
				expected := map[string]runtimecontract.RuntimeValidationDiagnosticUnavailability{
					"unobserved": runtimecontract.ValidationDiagnosticBehaviorUnobserved, "partial-unobserved": runtimecontract.ValidationDiagnosticBehaviorUnobserved,
					"missing-assertion": runtimecontract.ValidationDiagnosticAssertionsUnavailable, "over-capacity": runtimecontract.ValidationDiagnosticCapacityExceeded,
					"skip": runtimecontract.ValidationDiagnosticSourcesUnavailable, "drift": runtimecontract.ValidationDiagnosticInputChanged, "missing-source": runtimecontract.ValidationDiagnosticInputChanged,
					"changed-assertion":   runtimecontract.ValidationDiagnosticReproductionUnavailable,
					"diagnostic-capacity": runtimecontract.ValidationDiagnosticCapacityExceeded,
					"cancel-overlap":      runtimecontract.ValidationDiagnosticExecutionUnavailable,
				}[scenario]
				if scenario == "diagnostic-capacity" && diagnostic.OwnerCode != "validation_failure_assertion_count_exceeded" {
					t.Fatalf("independent capacity lost exact owner cause: %+v", diagnostic)
				}
				if diagnostic.Unavailability != expected {
					t.Fatalf("%s misclassified cause: got=%s want=%s", scenario, diagnostic.Unavailability, expected)
				}
			} else if diagnostic != nil {
				t.Fatalf("legacy fixture unexpectedly attributed: %+v", diagnostic)
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
				if err := runtimecontract.ValidateRuntimeValidationDiagnosticFailureObservation(result.Result.Evidence.Validation, binding, refs, nil, nil); err != nil {
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
				wantStages := 4
				if scenario == "additive-input-test" || scenario == "isolated-overlap" {
					wantStages = 5
				}
				if len(result.Conjunction.Stages) != wantStages || !reflect.DeepEqual(result.Result.ValidationFailureDiagnostic.TargetRefs, wantTargets) || !reflect.DeepEqual(result.Conjunction.BehaviorTargetRefs, wantTargets) {
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
					if stage.DiagnosticSourceRef == "" || stage.DiagnosticIsolationRef == "" || stage.DiagnosticOriginStageID != "project-test" || !reflect.DeepEqual(stage.BehaviorTargetRefs, wantSourceTargets) || stage.Stdout.Storage == nil || stage.Stdout.Storage.Reference == "" {
						t.Fatal("source-specific real behavior/raw fact missing")
					}
				}
			}
			if scenario == "large-output" {
				stage := result.Conjunction.Stages[1]
				if !stage.Stdout.Complete || stage.Stdout.SavedBytes < 2000000 || stage.Stdout.Storage == nil {
					t.Fatal("late failure raw incomplete")
				}
				store, err := openShellProjectRawStore(internal)
				if err != nil {
					t.Fatal(err)
				}
				defer store.Close()
				raw, err := store.Read(context.Background(), shellProjectRawScope{WorkspaceID: "ws", RunID: "run", ExecutionKey: "multi-failure/" + scenario, StageID: stage.Identity.ID, Stream: "stdout"}, stage.Stdout.Storage.Reference)
				if err != nil || !strings.Contains(string(raw.Raw[65536:]), "value_mismatch expected=4 actual=3") {
					t.Fatal("late assertion lost", err)
				}
			}
			if result.Conjunction.Stages[1].Identity.ID != "project-test" || !result.Conjunction.Stages[1].KnownFailure {
				t.Fatal("original failure erased")
			}
		})
	}
}

// The diagnostic shares the original operation context; cancellation must not
// be reported as a timeout or change the known original execution failure.
func TestProjectMultiFailureDiagnosticContextDisposition(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		var ctx context.Context
		if timeout {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer cancel()
		} else {
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(context.Background())
			cancel()
		}
		baseline := RuntimeCommandResult{ValidationObservation: &runtimecontract.RuntimeValidationObservation{FailedChecks: []string{"native"}}}
		stages, _, got := runShellProjectMultiFailureDiagnostic(ctx, t.TempDir(), nil, RuntimeCommandRequest{}, shellProjectStageFact{ID: "project-test", ExitCode: 1}, 2, baseline)
		want := runtimecontract.ValidationDiagnosticExecutionUnavailable
		if timeout {
			want = runtimecontract.ValidationDiagnosticDeadlineExceeded
		}
		if got == nil || got.ValidationFailureDiagnostic == nil || got.ValidationFailureDiagnostic.Unavailability != want || got.ValidationFailureDiagnostic.ExitCode != 1 || len(stages) != 0 || !reflect.DeepEqual(got.ValidationObservation.FailedChecks, baseline.ValidationObservation.FailedChecks) {
			t.Fatalf("timeout=%t lost exact disposition: %+v", timeout, got)
		}
	}
}

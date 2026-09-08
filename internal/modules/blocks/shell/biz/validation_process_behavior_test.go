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

// This is calibration of the owner observation protocol, not evidence that an
// arbitrary workspace implements MCP or that five real language servers pass.
const processBehaviorDelivery = `package main
import ("encoding/json"; "os")
func main() {
 var req struct { Language string }; if json.NewDecoder(os.Stdin).Decode(&req)!=nil { os.Exit(2) }
 path := "src/"+req.Language+"/symbol";
 if req.Language == "rust" { path = "RUST_PATH" }
 _ = json.NewEncoder(os.Stdout).Encode(map[string]any{"path":path,"line":3})
}
`

func TestEntrypointBehaviorObservesBuiltGoProcessWithoutRewritingInputs(t *testing.T) {
	const native = "TestWorkOrchCWI_PROCESS"
	for _, tc := range []struct {
		name, body, implementation string
		pass, observed             bool
	}{
		{"five protocol inputs", `for _, lang := range languages { verify(t, bin, lang) }`, "src/rust/symbol", true, true},
		{"one language wrong", `for _, lang := range languages { verify(t, bin, lang) }`, "wrong/symbol", false, true},
		{"build only", `if _,e:=os.Stat(bin); e!=nil { t.Fatal(e) }`, "src/rust/symbol", false, false},
		{"external process only", `if out,e:=exec.Command("go","version").Output(); e!=nil || len(out)==0 { t.Fatal("external command") }`, "src/rust/symbol", false, false},
		{"original check fails", `for _, lang := range languages { verify(t, bin, lang) }`, "src/rust/symbol", false, true},
		{"required dependency skipped", `t.Run("rust",func(t *testing.T){ t.Skip("backend unavailable") }); verify(t,bin,"go")`, "src/rust/symbol", false, true},
		{"instrumentation disabled", `os.Setenv("GOCOVERDIR",t.TempDir()); verify(t,bin,"go")`, "src/rust/symbol", false, false},
		{"source changed inside sandbox", `verify(t,bin,"go"); if e:=os.WriteFile("../cmd/delivery/main.go",[]byte("package main\nfunc main(){}"),0600); e!=nil { t.Fatal(e) }`, "src/rust/symbol", false, false},
		{"wrong native check unselected", `if len(languages)!=5 { t.Fatal("fixture") }`, "src/rust/symbol", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := fmt.Sprintf("package fixture\nimport \"testing\"\nfunc %s(t *testing.T){ if false { t.Fatal(\"original assertion\") } }", native)
			if tc.name == "original check fails" {
				original = strings.Replace(original, "if false", "if true", 1)
			}
			workspace := newGoValidationWorkspace(t, original)
			implementation := strings.ReplaceAll(processBehaviorDelivery, "RUST_PATH", tc.implementation)
			writeNativeStructureSource(t, workspace, "cmd/delivery/main.go", implementation)
			target := shellSupplementTargetRef(native)
			input := fmt.Sprintf(`package checks
import ("bytes"; "encoding/json"; "os"; "os/exec"; "path/filepath"; "testing")
const line = 3
var languages = []string{"go","rust","python","typescript","shell"}
type response struct { Path string; Line int }
func verify(t *testing.T, bin, language string) {
 t.Helper(); cmd:=exec.Command(bin); cmd.Stdin=bytes.NewBufferString("{\"Language\":\""+language+"\"}")
 out,e:=cmd.Output(); if e!=nil { t.Fatal(e) }; var got response
 if json.Unmarshal(out,&got)!=nil || got.Path!="src/"+language+"/symbol" || got.Line!=line { t.Fatalf("invalid protocol response for %%s",language) }
}
func %s(t *testing.T) {
 bin:=filepath.Join(t.TempDir(),"delivery"); build:=exec.Command("go","build","-buildvcs=false","-o",bin,"./cmd/delivery"); build.Dir=".."
 if out,e:=build.CombinedOutput(); e!=nil { t.Fatalf("build: %%v %%s",e,out) }; _=os.Stat
 %s
}
`, shellSupplementCheckRef(native), tc.body)
			writeNativeStructureSource(t, workspace, target, input)
			refs := []string{"go.mod", "fixture_test.go", "cmd/delivery/main.go", target}
			base := runtimecontract.RuntimeValidationCheckAttestationBinding{RequiredCheckRef: "check", NativeCheckRef: native, ObservationProtocolRef: shellGoEntrypointObservationProtocolRef}
			checks := shellValidationSupplementAttestations([]runtimecontract.RuntimeValidationCheckAttestationBinding{base}, refs)
			if inspection := inspectShellNativeValidationChecks(workspace, refs, checks); inspection.Code != "" {
				t.Fatalf("unchanged helper-based input not ready: %+v", inspection)
			}
			request := RuntimeCommandRequest{WorkspaceDeliverableDirectory: workspace, ValidationRecipeID: "entrypoint", Command: "go", Args: []string{"test", "./..."}, TimeoutMillis: 60000,
				TargetRefs: []string{"fixture_test.go"}, ValidationScopeTargetRefs: refs, ValidationCheckAttestations: checks,
				ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}}
			if err := instrumentManagedValidationNativeChecks(&request); err != nil {
				t.Fatal(err)
			}
			result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), request)
			passed := err == nil && result.Evidence.Quality == common.EvidenceQualityValid && result.ValidationObservation != nil && runtimecontract.RuntimeValidationAttestationPassed(*result.ValidationObservation, checks[0])
			if passed != tc.pass || (len(result.workspaceProcessBehaviorTargetRefs) != 0) != tc.observed {
				t.Fatalf("pass=%t targets=%v want=%t/%t code=%s observationError=%v err=%v stderr=%s", passed, result.workspaceProcessBehaviorTargetRefs, tc.pass, tc.observed, result.FailureCode, result.workspaceProcessBehaviorError, err, result.Stderr)
			}
			if result.ValidationObservation == nil || result.ValidationObservation.ObservationProtocolRef != shellGoEntrypointObservationProtocolRef {
				t.Fatal("lost sealed v4 identity")
			}
			if !tc.pass && result.ValidationFailureDiagnostic != nil && result.ValidationFailureDiagnostic.InputAddition != (runtimecontract.RuntimeValidationInputAddition{}) {
				t.Fatal("ineffective supplement authorized a repeated input addition")
			}
			if tc.name == "one language wrong" && (result.ValidationFailureDiagnostic == nil || !reflect.DeepEqual(result.ValidationFailureDiagnostic.TargetRefs, []string{"cmd/delivery/main.go"})) {
				t.Fatalf("failed behavior did not identify the implementation independently: %+v", result.ValidationFailureDiagnostic)
			}
			for ref, want := range map[string]string{"fixture_test.go": original, target: input, "cmd/delivery/main.go": implementation} {
				body, readErr := os.ReadFile(filepath.Join(workspace, ref))
				if readErr != nil || string(body) != want {
					t.Fatalf("validation rewrote %s", ref)
				}
			}
			for _, resource := range result.ValidationResources {
				if resource.Kind == "scratch_dir" {
					if _, err := os.Stat(resource.Path); !os.IsNotExist(err) {
						t.Fatal("process coverage escaped session cleanup")
					}
				}
			}
		})
	}
}

func TestEntrypointProcessObservationRejectsUnboundArtifacts(t *testing.T) {
	for _, tc := range []struct{ name, filename, body string }{
		{"stdout marker", "WORKORCH_CHECK_PASSED", "passed"},
		{"incomplete counter", "covcounters.fake", "incomplete"},
		{"incomplete metadata", "covmeta.fake", "incomplete"},
		{"oversized", "covmeta.large", strings.Repeat("x", maxNativeCheckSourceBytes+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeNativeStructureSource(t, dir, tc.filename, tc.body)
			if targets, err := shellProcessBehaviorTargets(context.Background(), "go", t.TempDir(), dir, nil, nil); err == nil || len(targets) != 0 {
				t.Fatal("incomplete/unbound artifacts became execution evidence")
			}
		})
	}
	t.Run("file bound", func(t *testing.T) {
		dir := t.TempDir()
		for i := 0; i <= maxShellProcessCoverageFiles; i++ {
			writeNativeStructureSource(t, dir, fmt.Sprintf("covmeta.%d", i), "bounded")
		}
		if targets, err := shellProcessBehaviorTargets(context.Background(), "go", t.TempDir(), dir, nil, nil); err == nil || len(targets) != 0 {
			t.Fatal("unbounded observation supplied execution evidence")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		dir, outside := t.TempDir(), t.TempDir()
		writeNativeStructureSource(t, outside, "cover", "data")
		if err := os.Symlink(filepath.Join(outside, "cover"), filepath.Join(dir, "covmeta.link")); err != nil {
			t.Fatal(err)
		}
		if targets, err := shellProcessBehaviorTargets(context.Background(), "go", t.TempDir(), dir, nil, nil); err == nil || len(targets) != 0 {
			t.Fatal("symlink became owner execution evidence")
		}
	})
}

func TestEntrypointContractReobservesHistoricalInputOnlyForNewAdapter(t *testing.T) {
	recipe := runtimecontract.RuntimeValidationRecipe{ID: "native", Command: "go", Args: []string{"test", "./..."}, AdapterRef: modulecommon.ValidationRecipeAdapterProjectNative}
	requirement := shellValidationProofRequirementForTest(t, recipe.ID, "cwi-existing-input")
	old, _ := shellValidationCheckAttestations(recipe, requirement)
	root := t.TempDir()
	writeNativeStructureSource(t, root, "original_test.go", fmt.Sprintf("package checks\nimport \"testing\"\nfunc %s(t *testing.T) { if false { t.Fatal(\"original\") } }", old[0].NativeCheckRef))
	target := shellSupplementTargetRef(old[0].NativeCheckRef)
	writeNativeStructureSource(t, root, target, fmt.Sprintf("package checks\nimport \"testing\"\nconst expected=1\nfunc verify(t *testing.T) { if expected!=1 { t.Fatal(\"bad\") } }\nfunc %s(t *testing.T){ verify(t) }", shellSupplementCheckRef(old[0].NativeCheckRef)))
	binding := shellTestWorkspaceManifestBinding("same-tree", []string{"go.mod", "original_test.go", target})
	for _, adapter := range []string{modulecommon.ValidationRecipeAdapterProjectNative, modulecommon.ValidationRecipeAdapterProjectEntrypoint, modulecommon.ValidationRecipeAdapterProjectNative} {
		recipe.AdapterRef = adapter
		surfaces := shellValidationSurfaces([]runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationRecipe{recipe}, []runtimecontract.RuntimeValidationProofRequirement{requirement}, binding, "current-catalog", root)
		if len(surfaces) != 1 || surfaces[0].Validate() != nil || len(surfaces[0].ProofRequirementPreparations) != 0 {
			t.Fatal("input reuse invented a preparation or invalid surface")
		}
		if adapter == modulecommon.ValidationRecipeAdapterProjectNative {
			if len(surfaces[0].ProofRequirementUnavailabilities) != 1 || len(surfaces[0].ProofRequirementBindings) != 0 {
				t.Fatal("old activation silently acquired new input semantics")
			}
		} else if len(surfaces[0].ProofRequirementUnavailabilities) != 0 || len(surfaces[0].ProofRequirementBindings) != 1 || surfaces[0].ProofRequirementBindings[0].CheckAttestations[0].ObservationProtocolRef != shellGoEntrypointObservationProtocolRef {
			t.Fatal("fresh explicit adapter did not bind the unchanged conjunctive inputs")
		}
	}
}

func TestEntrypointInputContractRejectsRunnerAndInitializerEffects(t *testing.T) {
	const native = "TestWorkOrchCWI_SHAPE"
	for _, extra := range []string{"func init() {}", "func TestMain(m *testing.M) {}", "var value = func() int { return 1 }()", "func TestReplacement(t *testing.T) {}", "type R struct{}; func (R) helper() {}"} {
		t.Run(extra, func(t *testing.T) {
			root := t.TempDir()
			writeNativeStructureSource(t, root, "original_test.go", "package fixture\nimport \"testing\"\nfunc "+native+"(t *testing.T) { t.Fatal(\"original\") }")
			target := shellSupplementTargetRef(native)
			writeNativeStructureSource(t, root, target, "package checks\nimport \"testing\"\nfunc "+shellSupplementCheckRef(native)+"(t *testing.T) { t.Fatal(\"supplement\") }\n"+extra)
			checks := shellValidationSupplementAttestations([]runtimecontract.RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "check", NativeCheckRef: native, ObservationProtocolRef: shellGoEntrypointObservationProtocolRef}}, []string{target})
			if got := inspectShellNativeValidationChecks(root, []string{"original_test.go", target}, checks); got.Code == "" {
				t.Fatal("unsafe input supplied readiness")
			}
		})
	}
}

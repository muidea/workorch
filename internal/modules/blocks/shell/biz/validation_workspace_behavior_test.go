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

func TestWorkspaceBehaviorProofRequiresDeliveryExecutionAndProtectsAssertionInputs(t *testing.T) {
	const check = "TestWorkOrchCWI_BEHAVIOR"
	for _, test := range []struct {
		name, body, helpers    string
		wantPass, wantObserved bool
	}{
		{"delivered function", `if compute(4) != 11 { t.Fatal("wrong result") }`, "", true, true},
		{"helper and subtest", `t.Run("input", func(child *testing.T) { verify(child) })`, `func verify(t *testing.T) { if compute(-3) != 4 { t.Fatal("wrong result") } }`, true, true},
		{"source text only", `data, err := os.ReadFile("fixture.go"); if err != nil || !strings.Contains(string(data), "compute") { t.Fatal("missing name") }`, "", false, false},
		{"dependency only", `if strings.TrimSpace(" a ") != "a" { t.Fatal("external behavior") }`, "", false, false},
		{"shadow implementation in test", `if fakeCompute(4) != 11 { t.Fatal("wrong result") }`, `func fakeCompute(v int) int { return v+7 }`, false, false},
		{"unrelated check is not selected", `if 1 != 1 { t.Fatal("placeholder") }`, `func TestUnrelated(t *testing.T) { if compute(4) != 11 { t.Fatal("wrong result") } }`, false, false},
		{"failed assertion", `if compute(4) != 12 { t.Fatal("wrong result") }`, "", false, true},
		{"failed dependency assertion", `if strings.TrimSpace(" a ") != "b" { t.Fatal("dependency assertion failed") }`, "", false, false},
		{"required subtest skipped", `t.Run("required", func(t *testing.T) { t.Skip("missing dependency"); if compute(4) != 11 { t.Fatal("wrong result") } })`, "", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace := newGoValidationWorkspace(t, fmt.Sprintf("package fixture\nimport (\"testing\"; \"os\"; \"strings\")\nvar _ = os.ReadFile; var _ = strings.Contains\nfunc %s(t *testing.T) { %s }\n%s\n", check, test.body, test.helpers))
			writeNativeStructureSource(t, workspace, "fixture.go", "package fixture\nfunc compute(v int) int { return v+7 }\n")
			request := RuntimeCommandRequest{WorkspaceDeliverableDirectory: workspace, ValidationRecipeID: "behavior",
				Command: "go", Args: []string{"test", "./..."}, TimeoutMillis: 60000, TargetRefs: []string{"fixture_test.go"},
				ValidationScopeTargetRefs:   []string{"go.mod", "fixture.go", "fixture_test.go"},
				ManagedValidation:           runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession},
				ValidationCheckAttestations: []runtimecontract.RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "check", NativeCheckRef: check, ObservationProtocolRef: shellGoBehaviorObservationProtocolRef}},
			}
			if err := instrumentManagedValidationNativeChecks(&request); err != nil {
				t.Fatal(err)
			}
			result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), request)
			if got := err == nil && result.Evidence.Quality == common.EvidenceQualityValid; got != test.wantPass {
				t.Fatalf("pass=%t want=%t code=%s reason=%s err=%v", got, test.wantPass, result.FailureCode, result.EvidenceReason, err)
			}
			if result.ValidationObservation == nil || result.ValidationObservation.ObservationProtocolRef != shellGoBehaviorObservationProtocolRef {
				t.Fatal("lost exact v2 observation")
			}
			if test.wantPass && result.Evidence.Validation.ProofClass != runtimecontract.RuntimeValidationProofBehavioral {
				t.Fatal("successful owner-observed workspace behavior lost behavioral attestation")
			}
			if !test.wantPass && result.Evidence.Validation.ProofClass != "" {
				t.Fatal("incomplete or failed workspace behavior claimed behavioral grade")
			}
			if (len(result.workspaceBehaviorTargetRefs) != 0) != test.wantObserved {
				t.Fatalf("observed implementation=%v", result.workspaceBehaviorTargetRefs)
			}
			if !test.wantObserved && len(result.ValidationObservation.IncompleteChecks) == 0 && len(result.ValidationObservation.CheckFailures) == 0 {
				t.Fatal("weak behavior could satisfy a check-only consumer")
			}
			if test.name == "failed assertion" {
				if result.ValidationFailureDiagnostic == nil || !reflect.DeepEqual(result.ValidationFailureDiagnostic.TargetRefs, []string{"fixture.go"}) {
					t.Fatalf("assertion input became implementation repair authority: %+v", result.ValidationFailureDiagnostic)
				}
				if len(result.ValidationObservation.CheckFailures) == 0 || result.ValidationObservation.CheckFailures[0].TargetRef != "fixture_test.go" {
					t.Fatal("assertion location was not retained independently")
				}
			}
			if test.name == "failed dependency assertion" {
				diagnostic := result.ValidationFailureDiagnostic
				if diagnostic == nil || diagnostic.Class != runtimecontract.ValidationDiagnosticTestFailed || diagnostic.OwnerCode != "validation_test_failure_unattributed" ||
					diagnostic.RecommendedEvidenceTargetType != runtimecontract.ValidationEvidenceTargetValidationOutput || len(diagnostic.TargetRefs) != 0 || !reflect.DeepEqual(diagnostic.InputAddition, runtimecontract.RuntimeValidationInputAddition{}) || diagnostic.InputRepair != nil {
					t.Fatalf("unattributed failure authorized a repair: %+v", diagnostic)
				}
				if result.ExitCode == 0 || len(result.ValidationObservation.CheckFailures) == 0 {
					t.Fatal("unattributed failure lost its original execution/check facts")
				}
			}
			for _, resource := range result.ValidationResources {
				if resource.Kind == "scratch_dir" {
					if _, err := os.Stat(resource.Path); !os.IsNotExist(err) {
						t.Fatal("behavior artifact escaped managed cleanup")
					}
				}
			}
		})
	}
}

func TestWorkspaceBehaviorObservationRejectsForeignHarnessAndIncompleteCoverage(t *testing.T) {
	workspace := newGoValidationWorkspace(t, "package fixture")
	writeNativeStructureSource(t, workspace, "fixture.go", "package fixture\nfunc compute() int { return 1 }")
	writeNativeStructureSource(t, workspace, "workorch_validation/helper.go", "package checks\nfunc fake() int { return 1 }")
	refs := []string{"fixture.go", "fixture_test.go", "workorch_validation/helper.go"}
	for name, content := range map[string]string{
		"foreign module":   "mode: count\nexternal.invalid/fixture.go:1.1,2.1 1 1\n",
		"test source":      "mode: count\nexample.com/workorch-validation-fixture/fixture_test.go:1.1,2.1 1 1\n",
		"harness source":   "mode: count\nexample.com/workorch-validation-fixture/workorch_validation/helper.go:1.1,2.1 1 1\n",
		"zero execution":   "mode: count\nexample.com/workorch-validation-fixture/fixture.go:1.1,2.1 1 0\n",
		"bad header":       "mode: set\n",
		"truncated record": "mode: count\npartial",
	} {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "coverage")
			if err := os.WriteFile(file, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if targets, _ := shellWorkspaceBehaviorTargets(workspace, file, refs); len(targets) != 0 {
				t.Fatalf("non-delivery supplied targets %v", targets)
			}
		})
	}
}

func TestWorkspaceBehaviorInstrumentationPreservesLegacyAndRejectsOverrides(t *testing.T) {
	legacy := RuntimeCommandRequest{Command: "go", Args: []string{"test", "./..."}, ValidationCheckAttestations: []runtimecontract.RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "old", NativeCheckRef: "TestWorkOrchCWI_OLD", ObservationProtocolRef: shellGoTestObservationProtocolRef}}}
	if err := instrumentManagedValidationNativeChecks(&legacy); err != nil {
		t.Fatal(err)
	}
	args, path, err := prepareShellBehaviorObservation(legacy, legacy.Args)
	if err != nil || path != "" || !reflect.DeepEqual(args, legacy.Args) {
		t.Fatal("v1 execution was silently upgraded")
	}
	for _, flag := range []string{"-coverprofile=caller", "-coverpkg=other", "-covermode=set", "-exec=wrapper", "-overlay=input", "-args", "-count=0", "-json=false", "-list=.", "-test.run=Other", "-test.coverprofile=caller", "-test.count=0", "-fuzz=Other"} {
		if err := validateShellBehaviorRecipeArgs([]string{"test", flag, "./..."}); err == nil {
			t.Fatalf("accepted instrumentation override %s", flag)
		}
	}
	env := strings.Join(shellBehaviorEnvironment([]string{"GOFLAGS=-overlay=old", "GOWORK=/old", "PATH=/bin"}), "\n")
	if strings.Contains(env, "old") || !strings.Contains(env, "PATH=/bin") || !strings.Contains(env, "GOWORK=off") {
		t.Fatalf("behavior execution inherited caller instrumentation: %s", env)
	}
}

func TestWorkspaceBehaviorProtocolIsOwnedByProjectNativeAdapter(t *testing.T) {
	requirement := runtimecontract.RuntimeValidationProofRequirement{RequiredCheckRefs: []string{"workorch-cwi-check-a", "workorch-cwi-check-b"}}
	for _, test := range []struct{ adapter, protocol string }{
		{"", shellGoTestObservationProtocolRef},
		{modulecommon.ValidationRecipeAdapterProjectNative, shellGoBehaviorObservationProtocolRef},
		{modulecommon.ValidationRecipeAdapterProjectEntrypoint, shellGoEntrypointObservationProtocolRef},
	} {
		recipe := runtimecontract.RuntimeValidationRecipe{Command: "go", Args: []string{"test", "./..."}, AdapterRef: test.adapter}
		attestations, ok := shellValidationCheckAttestations(recipe, requirement)
		if !ok || len(attestations) != len(requirement.RequiredCheckRefs) {
			t.Fatalf("adapter %q lost the complete all-of check set: %+v", test.adapter, attestations)
		}
		for _, attestation := range attestations {
			if attestation.ObservationProtocolRef != test.protocol {
				t.Fatalf("adapter %q changed protocol: %+v", test.adapter, attestation)
			}
		}
	}
}

func TestWorkspaceBehaviorInstrumentationSealsCompleteBoundedCheckSet(t *testing.T) {
	request := RuntimeCommandRequest{Command: "go", Args: []string{"test", "./..."},
		ManagedValidation:          runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession},
		ValidationScratchDirectory: t.TempDir(),
		ValidationCheckAttestations: []runtimecontract.RuntimeValidationCheckAttestationBinding{
			{RequiredCheckRef: "a", NativeCheckRef: "TestWorkOrchCWI_A", ObservationProtocolRef: shellGoBehaviorObservationProtocolRef},
			{RequiredCheckRef: "b", NativeCheckRef: "TestWorkOrchCWI_B", ObservationProtocolRef: shellGoBehaviorObservationProtocolRef},
		},
	}
	if err := instrumentManagedValidationNativeChecks(&request); err != nil {
		t.Fatal(err)
	}
	if _, path, err := prepareShellBehaviorObservation(request, request.Args); err != nil || path == "" {
		t.Fatalf("complete check set failed preparation: %s %v", path, err)
	}
	for _, extra := range [][]string{{"-run", "Other"}, {"-run", "^(TestWorkOrchCWI_A|TestWorkOrchCWI_B)$"}, {"-test.run=Other"}} {
		args := append(append([]string(nil), request.Args...), extra...)
		if _, _, err := prepareShellBehaviorObservation(request, args); err == nil {
			t.Fatalf("sealed native selection was overridden by %v", extra)
		}
	}
	request.Args = []string{"test", "./..."}
	request.ValidationCheckAttestations[1].ObservationProtocolRef = shellGoTestObservationProtocolRef
	if err := instrumentManagedValidationNativeChecks(&request); err == nil {
		t.Fatal("mixed protocols were accepted")
	}
	request.ValidationCheckAttestations = nil
	for index := 0; index <= maxShellBehaviorNativeChecks; index++ {
		request.ValidationCheckAttestations = append(request.ValidationCheckAttestations, runtimecontract.RuntimeValidationCheckAttestationBinding{
			RequiredCheckRef: fmt.Sprint(index), NativeCheckRef: fmt.Sprintf("TestWorkOrchCWI_%d", index), ObservationProtocolRef: shellGoBehaviorObservationProtocolRef,
		})
	}
	if err := instrumentManagedValidationNativeChecks(&request); err == nil {
		t.Fatal("native-check observation bound was bypassed")
	}
}

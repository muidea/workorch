package shell

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestValidationInputCompileDiagnosticAttestsOnlyExactCaseDelta(t *testing.T) {
	const native = "TestWorkOrchCWI_INPUT"
	const declarations = "const workorchInit = 1\nconst workorchList = 2\nconst workorchDone = 3\n"
	input := "package checks\nimport \"testing\"\n" + declarations + "func " + shellSupplementCheckRef(native) + "(t *testing.T) {\n values := []int{workOrchInit, workOrchList, workOrchDone}\n if len(values)!=3 { t.Fatal(\"original criterion\") }\n}\n"
	workspace := newGoValidationWorkspace(t, fmt.Sprintf("package fixture\nimport \"testing\"\nfunc %s(t *testing.T){ if false { t.Fatal(\"original\") } }", native))
	target := shellSupplementTargetRef(native)
	writeNativeStructureSource(t, workspace, target, input)
	checks := []runtimecontract.RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "required", NativeCheckRef: native, SupplementalNativeCheckRef: shellSupplementCheckRef(native), ObservationProtocolRef: shellGoEntrypointObservationProtocolRef}}
	refs := []string{"go.mod", "fixture_test.go", target}
	if inspection := inspectShellNativeValidationChecks(workspace, refs, checks); inspection.Code != "" {
		t.Fatalf("historical input must be structurally ready: %+v", inspection)
	}
	request := RuntimeCommandRequest{WorkspaceDeliverableDirectory: workspace, ValidationRecipeID: "input", Command: "go", Args: []string{"test", "./..."}, TimeoutMillis: 60000,
		TargetRefs: []string{"fixture_test.go"}, ValidationScopeTargetRefs: refs, ValidationCheckAttestations: checks,
		ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}}
	if err := instrumentManagedValidationNativeChecks(&request); err != nil {
		t.Fatal(err)
	}
	result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), request)
	diagnostic := result.ValidationFailureDiagnostic
	if err == nil || diagnostic == nil || diagnostic.InputRepair == nil {
		t.Fatalf("compiler input failure not independently repairable: diagnostic=%+v err=%v output=%s %s", diagnostic, err, result.Stdout, result.Stderr)
	}
	if diagnostic.OwnerCode != shellInputSymbolRepairCode || diagnostic.RecommendedEvidenceTargetType != runtimecontract.ValidationEvidenceTargetValidationInput || len(diagnostic.InputLocations) != 3 || len(diagnostic.InputRepair.Corrections) != 3 || !reflect.DeepEqual(diagnostic.TargetRefs, []string{target}) {
		t.Fatalf("exact input failure facts incomplete: %+v", diagnostic)
	}
	wantAfter := strings.ReplaceAll(input, "workOrch", "workorch")
	beforeHash, afterHash := sha256.Sum256([]byte(input)), sha256.Sum256([]byte(wantAfter))
	if diagnostic.InputRepair.BeforeHash != hex.EncodeToString(beforeHash[:]) || diagnostic.InputRepair.AfterHash != hex.EncodeToString(afterHash[:]) || diagnostic.InputRepair.Validate() != nil {
		t.Fatalf("input delta changed more than compiler-located symbols: %+v", diagnostic.InputRepair)
	}
	after, err := os.ReadFile(filepath.Join(workspace, target))
	if err != nil || string(after) != input {
		t.Fatal("Shell changed durable verification input")
	}
}

func TestValidationInputRepairRefusesAmbiguityAndNonSymbolChanges(t *testing.T) {
	const native = "TestWorkOrchCWI_INPUT"
	target := shellSupplementTargetRef(native)
	for _, scenario := range []string{"exact", "ambiguous declaration", "local shadow", "wrong symbol", "wrong location", "test stdout", "out of scope", "old protocol", "truncated", "input changed", "function declaration", "assertion failure", "partial diagnostics", "overflow"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			input := "package checks\nconst workorchValue = 1\nfunc helper() int { return workOrchValue }\n"
			request := RuntimeCommandRequest{ValidationScopeTargetRefs: []string{target}, ValidationCheckAttestations: []runtimecontract.RuntimeValidationCheckAttestationBinding{{RequiredCheckRef: "required", NativeCheckRef: native, SupplementalNativeCheckRef: shellSupplementCheckRef(native), ObservationProtocolRef: shellGoEntrypointObservationProtocolRef}}, ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}}
			record := shellGoTestJSONDiagnosticRecord{Action: "build-output", Output: target + ":3:28: undefined: workOrchValue\n"}
			output := RuntimeCommandResult{WorkingDirectory: root, Command: "go", Args: []string{"test", "-json", "./..."}, ExitCode: 1, validationSourcesUnchanged: true}
			switch scenario {
			case "ambiguous declaration":
				input += "const WORKORCHVALUE = 2\n"
			case "local shadow":
				input += "func other() { workorchValue := 2; _ = workorchValue }\n"
			case "wrong symbol":
				record.Output = strings.ReplaceAll(record.Output, "workOrchValue", "missingValue")
			case "wrong location":
				record.Output = strings.ReplaceAll(record.Output, ":3:28:", ":3:27:")
			case "test stdout":
				record.Action = "output"
			case "out of scope":
				request.ValidationScopeTargetRefs = []string{"other.go"}
			case "old protocol":
				request.ValidationCheckAttestations[0].ObservationProtocolRef = shellGoSupplementObservationProtocolRef
			case "truncated":
				output.OutputTruncated = true
			case "input changed":
				output.validationSourcesUnchanged = false
			case "function declaration":
				input = strings.ReplaceAll(input, "const workorchValue = 1", "func workorchValue() int { return 1 }")
			case "assertion failure":
				record.Output = target + ":3:28: expected result did not match\n"
			case "partial diagnostics":
				record.Output += "subject.go:1:1: undefined: missing\n"
			case "overflow":
				for i := 4; i < 13; i++ {
					record.Output += fmt.Sprintf("%s:%d:1: undefined: workOrchValue\n", target, i)
				}
			}
			writeNativeStructureSource(t, root, target, input)
			output.goTestJSONFacts = &shellGoTestJSONFacts{DiagnosticRecords: []shellGoTestJSONDiagnosticRecord{record}}
			_, repair := shellValidationInputCompileFact(output, request, "go")
			if (repair != nil) != (scenario == "exact") {
				t.Fatalf("unsafe/absent repair: %+v", repair)
			}
		})
	}
}

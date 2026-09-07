package shell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestValidationInputSupplementRequiresOriginalAndDeliveryBehavior(t *testing.T) {
	const native = "TestWorkOrchCWI_INPUT"
	for _, test := range []struct {
		name, body, extra                                   string
		wrongImplementation, originalFails, valid, notReady bool
	}{
		{name: "delivery entry", body: `if delivery.DeliveredValue(4) != 11 { t.Fatal("delivery result") }`, valid: true},
		{name: "incorrect delivery", body: `if delivery.DeliveredValue(4) != 11 { t.Fatal("delivery result") }`, wrongImplementation: true},
		{name: "original check fails", body: `if delivery.DeliveredValue(4) != 11 { t.Fatal("delivery result") }`, originalFails: true},
		{name: "input compile failure", body: `if delivery.UndeclaredEntry(4) != 11 { t.Fatal("delivery result") }`},
		{name: "source strings", body: `b,e:=os.ReadFile("../fixture.go"); if e!=nil || !strings.Contains(string(b),"DeliveredValue") { t.Fatal("source") }`},
		{name: "dependency bypass", body: `if strings.TrimSpace(" x ")!="x" { t.Fatal("dependency") }`},
		{name: "test replacement", body: `fake:=func(v int) int { return v+7 }; if fake(4)!=11 { t.Fatal("fake") }`},
		{name: "required check skipped", body: `t.Skip("not available"); if delivery.DeliveredValue(4)!=11 { t.Fatal("delivery") }`},
		{name: "required subcheck skipped", body: `t.Run("required",func(t *testing.T){ t.Skip("not available"); if delivery.DeliveredValue(4)!=11 { t.Fatal("delivery") } })`},
		{name: "deleted assertions", body: `_ = delivery.DeliveredValue(4)`, notReady: true},
		{name: "test main override", body: `if delivery.DeliveredValue(4)!=11 { t.Fatal("delivery") }`, extra: `func TestMain(m *testing.M) { os.Exit(0) }`, notReady: true},
		{name: "initialization override", body: `if delivery.DeliveredValue(4)!=11 { t.Fatal("delivery") }`, extra: `func init() { os.Exit(0) }`, notReady: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := fmt.Sprintf("package fixture\nimport (\"testing\";\"os\";\"strings\")\nfunc %s(t *testing.T){ b,e:=os.ReadFile(\"fixture.go\"); if e!=nil || !strings.Contains(string(b),\"DeliveredValue\") { t.Fatal(\"source\") }; %s }\n", native, map[bool]string{true: `t.Fatal("original criterion remains required")`}[test.originalFails])
			workspace := newGoValidationWorkspace(t, original)
			delta := 7
			if test.wrongImplementation {
				delta = 6
			}
			writeNativeStructureSource(t, workspace, "fixture.go", fmt.Sprintf("package fixture\nfunc DeliveredValue(v int) int { return v+%d }\n", delta))
			base := runtimecontract.RuntimeValidationCheckAttestationBinding{RequiredCheckRef: "check", NativeCheckRef: native, ObservationProtocolRef: shellGoBehaviorObservationProtocolRef}
			refs := []string{"go.mod", "fixture.go", "fixture_test.go"}
			request := RuntimeCommandRequest{WorkspaceDeliverableDirectory: workspace, ValidationRecipeID: "behavior", Command: "go", Args: []string{"test", "./..."}, TimeoutMillis: 60000, TargetRefs: []string{"fixture_test.go"}, ValidationScopeTargetRefs: refs,
				ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}, ValidationCheckAttestations: []runtimecontract.RuntimeValidationCheckAttestationBinding{base}}
			execute := func(req RuntimeCommandRequest) (RuntimeCommandResult, error) {
				t.Helper()
				if err := instrumentManagedValidationNativeChecks(&req); err != nil {
					t.Fatal(err)
				}
				return NewRuntimeAdapter().ExecuteOperation(context.Background(), req)
			}
			if test.valid {
				before, err := execute(request)
				if err == nil || before.ValidationFailureDiagnostic == nil || before.ValidationFailureDiagnostic.InputAddition.Validate() != nil {
					t.Fatalf("missing typed input contract: %+v %v", before.ValidationFailureDiagnostic, err)
				}
				addition := before.ValidationFailureDiagnostic.InputAddition
				if addition.TargetRef != shellSupplementTargetRef(native) || addition.NativeCheckRef != shellSupplementCheckRef(native) || addition.OriginalNativeCheckRef != native {
					t.Fatal("wrong input contract")
				}
			}
			target := shellSupplementTargetRef(native)
			content := fmt.Sprintf("package checks\nimport (\"testing\"; \"os\"; \"strings\"; delivery \"example.com/workorch-validation-fixture\")\nfunc %s(t *testing.T) { _=os.ReadFile; _=strings.Contains; _=delivery.DeliveredValue; %s }\n%s\n", shellSupplementCheckRef(native), test.body, test.extra)
			writeNativeStructureSource(t, workspace, target, content)
			refs = append(refs, target)
			checks := shellValidationSupplementAttestations([]runtimecontract.RuntimeValidationCheckAttestationBinding{base}, refs)
			if checks[0].NativeCheckRef != native || checks[0].SupplementalNativeCheckRef != shellSupplementCheckRef(native) || checks[0].ObservationProtocolRef != shellGoSupplementObservationProtocolRef {
				t.Fatal("supplement replaced original")
			}
			inspection := inspectShellNativeValidationChecks(workspace, refs, checks)
			if (inspection.Code != "") != test.notReady {
				t.Fatalf("readiness=%+v", inspection)
			}
			if test.notReady {
				return
			}
			request.ValidationScopeTargetRefs, request.ValidationCheckAttestations = refs, checks
			result, err := execute(request)
			passed := err == nil && result.Evidence.Quality == common.EvidenceQualityValid && runtimecontract.RuntimeValidationAttestationPassed(*result.ValidationObservation, checks[0])
			if passed != test.valid {
				t.Fatalf("passed=%t want=%t code=%s err=%v", passed, test.valid, result.FailureCode, err)
			}
			if result.ValidationObservation == nil || result.ValidationObservation.ObservationProtocolRef != shellGoSupplementObservationProtocolRef {
				t.Fatal("lost v3 protocol")
			}
			if result.FailureCode == "validation_workspace_behavior_unobserved" {
				t.Fatal("ineffective supplement could open another input-addition cycle")
			}
			if test.name == "input compile failure" && (result.ValidationFailureDiagnostic == nil || len(result.ValidationFailureDiagnostic.TargetRefs) != 0) {
				t.Fatalf("invalid input granted implementation-repair targets: %+v", result.ValidationFailureDiagnostic)
			}
			bytes, err := os.ReadFile(filepath.Join(workspace, "fixture_test.go"))
			if err != nil || string(bytes) != original {
				t.Fatal("original input changed")
			}
			if test.valid {
				request.ValidationCheckAttestations = []runtimecontract.RuntimeValidationCheckAttestationBinding{base}
				old, err := execute(request)
				if err == nil || old.FailureCode != "validation_workspace_behavior_unobserved" {
					t.Fatal("old sealed v2 input silently acquired supplement")
				}
			}
		})
	}
}

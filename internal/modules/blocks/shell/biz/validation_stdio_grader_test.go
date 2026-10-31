package shell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// A fixed reference grader for a small newline/JSON-RPC tools/list envelope
// slice. This calibrates the grader, not arbitrary generated tests, real MCP
// conformance, five language backends, a deployed provider or release DCE.
const stdioCalibrationDelivery = `package main
import ("encoding/json"; "fmt"; "os")
func main() {
 var req struct { ID json.RawMessage; Method string }
 if json.NewDecoder(os.Stdin).Decode(&req)!=nil || req.Method!="tools/list" { os.Exit(2) }
 tools:=[]map[string]string{{"name":"inspect"},{"name":"resolve"}}
 reply:=map[string]any{"jsonrpc":"2.0","id":req.ID,"result":map[string]any{"tools":tools}}
 _=fmt.Print
 RESPONSE
}
`

const stdioCalibrationGrader = `package checks
import ("bytes"; "context"; "encoding/json"; "os/exec"; "path/filepath"; "testing"; "time")
func %s(t *testing.T) {
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second); defer cancel()
 bin:=filepath.Join(t.TempDir(),"delivery")
 build:=exec.CommandContext(ctx,"go","build","-buildvcs=false","-o",bin,"./cmd/delivery"); build.Dir=".."
 if _,err:=build.CombinedOutput(); err!=nil { t.Fatal("build failed") }
 cmd:=exec.CommandContext(ctx,bin); cmd.Stdin=bytes.NewBufferString("{\"jsonrpc\":\"2.0\",\"id\":73,\"method\":\"tools/list\"}\n")
 out,err:=cmd.Output(); if err!=nil { t.Fatal("entrypoint failed") }
 body:=bytes.TrimSpace(out)
 if len(out)==0 || out[len(out)-1]!='\n' || bytes.Contains(body,[]byte{'\n'}) { t.Fatal("WORKORCH_ASSERTION_V1 code=predicate_mismatch expected=true observed=false") }
 var reply struct { JSONRPC string; ID int; Result json.RawMessage }
 if len(body)==0 || body[0]!='{' || json.Unmarshal(body,&reply)!=nil { t.Fatal("WORKORCH_ASSERTION_V1 code=shape_mismatch expected=object observed=string") }
 if reply.JSONRPC!="2.0" || reply.ID!=73 { t.Fatal("WORKORCH_ASSERTION_V1 code=value_mismatch expected=equal observed=different") }
 result:=bytes.TrimSpace(reply.Result)
 if len(result)>0 && result[0]=='[' { t.Fatal("WORKORCH_ASSERTION_V1 code=shape_mismatch expected=object observed=array") }
 var value struct { Tools []struct { Name string } }
 if len(result)==0 || result[0]!='{' || json.Unmarshal(result,&value)!=nil { t.Fatal("WORKORCH_ASSERTION_V1 code=presence_mismatch expected=present observed=absent") }
 if len(value.Tools)!=2 || value.Tools[0].Name!="inspect" || value.Tools[1].Name!="resolve" { t.Fatal("WORKORCH_ASSERTION_V1 code=value_mismatch expected=equal observed=different") }
 // A diagnostic report cannot change a passing check's native outcome.
 t.Log("WORKORCH_ASSERTION_V1 code=shape_mismatch expected=object observed=array")
}
`

func TestStdioEnvelopeReferenceGraderCalibration(t *testing.T) {
	const native = "TestWorkOrchCWI_ENVELOPE"
	const correct = `_ = json.NewEncoder(os.Stdout).Encode(reply)`
	const bareArray = `reply["result"]=tools; _ = json.NewEncoder(os.Stdout).Encode(reply)`
	var firstFailure *runtimecontract.RuntimeValidationObservation
	for _, tc := range []struct {
		name, response, code string
		pass                 bool
	}{
		{"correct entrypoint", correct, "", true},
		{"bare result array", bareArray, "shape_mismatch", false},
		{"comment only repair", bareArray + "\n// repaired tools response\n", "shape_mismatch", false},
		{"wrong framing", `b,_:=json.Marshal(reply); fmt.Printf("Content-Length: %d\r\n\r\n%s",len(b),b)`, "predicate_mismatch", false},
		{"wrong response id", `reply["id"]=74; ` + correct, "value_mismatch", false},
		{"missing tool", `reply["result"]=map[string]any{"tools":tools[:1]}; ` + correct, "value_mismatch", false},
		{"marker without response", `_ = reply; fmt.Println("WORKORCH_CHECK_PASSED=TestWorkOrchCWI_ENVELOPE")`, "shape_mismatch", false},
		{"actual implementation repair", correct, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := fmt.Sprintf("package fixture\nimport \"testing\"\nfunc %s(t *testing.T){ if false { t.Fatal(\"baseline\") } }", native)
			workspace := newGoValidationWorkspace(t, original)
			implementation := strings.Replace(stdioCalibrationDelivery, "RESPONSE", tc.response, 1)
			writeNativeStructureSource(t, workspace, "cmd/delivery/main.go", implementation)
			target := shellSupplementTargetRef(native)
			grader := fmt.Sprintf(stdioCalibrationGrader, shellSupplementCheckRef(native))
			writeNativeStructureSource(t, workspace, target, grader)
			refs := []string{"go.mod", "fixture_test.go", "cmd/delivery/main.go", target}
			checks := shellValidationSupplementAttestations([]runtimecontract.RuntimeValidationCheckAttestationBinding{{
				RequiredCheckRef: "entrypoint/envelope", NativeCheckRef: native, ObservationProtocolRef: shellGoEntrypointObservationProtocolRef,
			}}, refs)
			if inspection := inspectShellNativeValidationChecks(workspace, refs, checks); inspection.Code != "" {
				t.Fatalf("reference grader is not ready: %+v", inspection)
			}
			request := RuntimeCommandRequest{WorkspaceDeliverableDirectory: workspace, ValidationRecipeID: "entrypoint/envelope", Command: "go", Args: []string{"test", "./..."}, TimeoutMillis: 60000,
				TargetRefs: []string{"fixture_test.go"}, ValidationScopeTargetRefs: refs, ValidationCheckAttestations: checks,
				ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}}
			if err := instrumentManagedValidationNativeChecks(&request); err != nil {
				t.Fatal(err)
			}
			result, err := NewRuntimeAdapter().ExecuteOperation(context.Background(), request)
			observation := result.ValidationObservation
			passed := err == nil && result.Evidence.Quality == common.EvidenceQualityValid && observation != nil && runtimecontract.RuntimeValidationAttestationPassed(*observation, checks[0])
			if passed != tc.pass || observation == nil || !reflect.DeepEqual(result.workspaceProcessBehaviorTargetRefs, []string{"cmd/delivery/main.go"}) {
				t.Fatalf("reference verdict pass=%t want=%t observation=%+v code=%s err=%v stderr=%s", passed, tc.pass, observation, result.FailureCode, err, result.Stderr)
			}
			if tc.pass {
				if len(observation.CheckFailures) != 0 || observation.FailureDetailProtocolRef != "" {
					t.Fatal("passing test's self-reported marker became failed-check evidence")
				}
			} else {
				if observation.ValidateFailureDetails() != nil || observation.FailureDetailProtocolRef != runtimecontract.RuntimeValidationFailureDetailProtocol || len(observation.CheckFailures) != 1 {
					t.Fatalf("failed grader lost typed diagnostic: %+v", observation)
				}
				detail := observation.CheckFailures[0]
				if detail.ReportedAssertion.Code != tc.code || detail.TargetRef != target || detail.CheckRef != checks[0].SupplementalNativeCheckRef || detail.ComparisonHash == "" {
					t.Fatalf("wrong check-scoped feedback: %+v", detail)
				}
				if tc.name == "bare result array" {
					firstFailure = observation
				}
				if tc.name == "comment only repair" {
					fact := runtimecontract.EvidenceFact{Kind: runtimecontract.EvidenceKindValidation, CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}, Validation: firstFailure}
					before := runtimecontract.RuntimeValidationFailureFingerprint(fact)
					fact.Validation = observation
					if before == "" || before != runtimecontract.RuntimeValidationFailureFingerprint(fact) {
						t.Fatal("comment-only effect or new sandbox disguised unchanged failure")
					}
				}
			}
			// Calibration cannot change an immutable criterion, existing test or
			// product file in the actual workspace while running isolated copies.
			for ref, want := range map[string]string{"fixture_test.go": original, target: grader, "cmd/delivery/main.go": implementation} {
				body, err := os.ReadFile(filepath.Join(workspace, ref))
				if err != nil || string(body) != want {
					t.Fatalf("reference validation rewrote %s", ref)
				}
			}
		})
	}
}

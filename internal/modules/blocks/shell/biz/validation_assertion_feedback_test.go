package shell

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestShellAssertionComparisonNormalizesOnlyOwnedTemporaryNoise(t *testing.T) {
	a := "built /tmp/workorch-shell-validation-123/tmp/TestBridge987654321/001/bridge: expected output, got none"
	b := "built /tmp/workorch-shell-validation-456/tmp/TestBridge123456789/001/bridge: expected output, got none"
	want := shellAssertionComparisonHash(a, "/tmp/workorch-shell-validation-123")
	if want != shellAssertionComparisonHash(b, "/tmp/workorch-shell-validation-456") {
		t.Fatal("same assertion changed identity with sandbox and testing temporary paths")
	}
	for _, changed := range []string{
		strings.Replace(b, "got none", "got a different result", 1),
		strings.Replace(b, "/bridge:", "/another-binary:", 1),
		strings.Replace(b, "/001/", "/002/", 1),
		strings.Replace(b, "/tmp/workorch-shell-validation-456/", "/tmp/another-owner/", 1),
		strings.Replace(b, "built /tmp", "built /outside/tmp", 1),
		strings.Replace(b, "-456/", "-456-neighbor/", 1),
	} {
		if want == shellAssertionComparisonHash(changed, "/tmp/workorch-shell-validation-456") {
			t.Fatalf("non-noise change disappeared: %s", changed)
		}
	}
	if shellAssertionComparisonHash(a, "") == shellAssertionComparisonHash(b, "") {
		t.Fatal("unobserved scratch paths were guessed")
	}
	for _, scratch := range []string{"/", filepath.Clean(os.TempDir()), "relative/scratch"} {
		if shellAssertionComparisonHash(a, scratch) != shellAssertionComparisonHash(a, "") {
			t.Fatal("broad or unresolved scratch path normalized unrelated text")
		}
	}
	if shellAssertionComparisonHash(`binary="`+a+`", again=[`+a+`]`, "/tmp/workorch-shell-validation-123") !=
		shellAssertionComparisonHash(`binary="`+b+`", again=[`+b+`]`, "/tmp/workorch-shell-validation-456") {
		t.Fatal("multiple quoted owner paths did not normalize consistently")
	}
}

func TestShellAssertionComparisonUsesRealManagedTemporaryPaths(t *testing.T) {
	workspace := newGoValidationWorkspace(t, `package fixture
import ("testing"; "path/filepath")
func TestAssertion(t *testing.T) { t.Fatalf("built %s: expected response, got none", filepath.Join(t.TempDir(),"delivery")) }
`)
	var first runtimecontract.RuntimeValidationCheckFailure
	var firstFingerprint string
	for attempt := 0; attempt < 2; attempt++ {
		request := RuntimeCommandRequest{WorkspaceDeliverableDirectory: workspace, ValidationRecipeID: "recipe", Command: "go", Args: []string{"test", "-json", "./..."}, TimeoutMillis: 60000,
			TargetRefs: []string{"fixture_test.go"}, ValidationScopeTargetRefs: []string{"go.mod", "fixture_test.go"},
			ManagedValidation: runtimecontract.RuntimeManagedValidation{RuntimeManaged: true, Kind: runtimecontract.RuntimeBlockKindManagedValidationSession}}
		result, _ := NewRuntimeAdapter().ExecuteOperation(context.Background(), request)
		o := result.ValidationObservation
		if o == nil || o.ValidateFailureDetails() != nil || len(o.CheckFailures) != 1 || len(o.FailedChecks) != 1 {
			t.Fatalf("missing native failure: %+v stderr=%s", o, result.Stderr)
		}
		detail := o.CheckFailures[0]
		fingerprint := runtimecontract.RuntimeValidationFailureFingerprint(runtimecontract.EvidenceFact{Kind: runtimecontract.EvidenceKindValidation,
			CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi"}, Validation: o})
		if attempt == 0 {
			first, firstFingerprint = detail, fingerprint
		} else if first.DiagnosticHash == detail.DiagnosticHash || first.ComparisonHash != detail.ComparisonHash || firstFingerprint == "" || firstFingerprint != fingerprint {
			t.Fatal("real managed scratch noise changed comparison or erased exact raw identity")
		}
		if detail.ReportedAssertion != (runtimecontract.RuntimeValidationReportedAssertion{}) {
			t.Fatal("ordinary assertion text was guessed as a typed shape")
		}
	}
}

func TestShellAssertionFeedbackRequiresFailedScopedNativeOutput(t *testing.T) {
	report := "WORKORCH_ASSERTION_V1 code=shape_mismatch expected=object observed=array"
	events := []shellGoTestJSONEvent{
		{Action: "output", Package: "example/checks", Test: "TestA/passing", Output: "a_test.go:1: " + report},
		{Action: "pass", Package: "example/checks", Test: "TestA/passing"},
		{Action: "output", Package: "example/checks", Test: "TestA/failing", Output: "a_test.go:9: " + report},
		{Action: "fail", Package: "example/checks", Test: "TestA/failing"},
		{Action: "output", Package: "example/passing", Test: "TestA/failing", Output: "a_test.go:77: " + report},
		{Action: "pass", Package: "example/passing", Test: "TestA/failing"},
		{Action: "output", Package: "example/checks", Test: "TestA/skipped", Output: "a_test.go:78: " + report},
		{Action: "skip", Package: "example/checks", Test: "TestA/skipped"},
		{Action: "output", Package: "example/checks", Test: "TestA", Output: "/outside/a_test.go:10: " + report},
		{Action: "fail", Package: "example/checks", Test: "TestA"},
		{Action: "output", Package: "example/checks", Test: "TestB", Output: "a_test.go:19: " + report},
		{Action: "pass", Package: "example/checks", Test: "TestB"},
	}
	var transcript strings.Builder
	for _, event := range events {
		body, _ := json.Marshal(event)
		fmt.Fprintln(&transcript, string(body))
	}
	output := RuntimeCommandResult{Stdout: transcript.String(), ExitCode: 1}
	attachShellValidationObservation(&output, "go", []string{"test", "-json"}, "recipe", true)
	attachShellValidationCheckFailures(&output, RuntimeCommandRequest{ValidationScopeTargetRefs: []string{"checks/a_test.go", "passing/a_test.go"}})
	o := output.ValidationObservation
	if o == nil || o.FailureDetailProtocolRef != runtimecontract.RuntimeValidationFailureDetailProtocolV2 || len(o.CheckFailures) != 1 || o.ValidateFailureDetails() != nil {
		t.Fatalf("invalid typed feedback: %+v", o)
	}
	detail := o.CheckFailures[0]
	digest := sha256.Sum256([]byte(report))
	if detail.CaseRef != "TestA/failing" || detail.ReportedAssertion.Code != "shape_mismatch" || detail.ReportedAssertion.ObservedShape != "array" ||
		detail.DiagnosticHash != hex.EncodeToString(digest[:]) || detail.ComparisonHash == "" {
		t.Fatalf("wrong report or weakened raw digest: %+v", detail)
	}
	if runtimecontract.RuntimeValidationCheckPassed(*o, "TestA") {
		t.Fatal("diagnostic report manufactured a pass")
	}
}

func TestShellReportedAssertionRejectsRawValuesAndAmbiguousGrammar(t *testing.T) {
	for _, input := range []string{
		"raw expected private-token got private-value",
		"WORKORCH_ASSERTION_V1 code=shape_mismatch expected=private-token observed=array",
		"WORKORCH_ASSERTION_V1 code=shape_mismatch expected=object observed=https://private.example",
		"WORKORCH_ASSERTION_V1 code=shape_mismatch expected=object observed=object",
		"WORKORCH_ASSERTION_V1 code=shape_mismatch expected=object observed=array extra=private",
		"WORKORCH_ASSERTION_V1 code=unknown expected=object observed=array",
		"WORKORCH_ASSERTION_V1 code=presence_mismatch expected=object observed=array",
		"WORKORCH_ASSERTION_V1 code=shape_mismatch expected=object expected=array",
	} {
		if shellReportedAssertion(input) != (runtimecontract.RuntimeValidationReportedAssertion{}) {
			t.Fatalf("unsafe/ambiguous assertion report accepted: %q", input)
		}
	}
}

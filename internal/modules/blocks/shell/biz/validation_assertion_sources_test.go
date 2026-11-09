package shell

import (
	"fmt"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestNativeObservationsUseExactSourceNotMessageVocabulary(t *testing.T) {
	source := `package checks
import "testing"
func TestA(t *testing.T) {
 t.Logf("changing elapsed_ms=%d", 123)
 t.Error("changing elapsed_ms=456")
 t.Log("ordinary"); t.Error("same line")
 helper(t)
}
func helper(t *testing.T) { t.Helper(); t.Log("helper observation") }
`
	subject := &runtimecontract.RuntimeValidationInputReviewSubject{Files: []runtimecontract.RuntimeValidationInputReviewFile{{TargetRef: "checks/a_test.go", Content: source, ContentHash: runtimecontract.RuntimeValidationInputContentHash(source)}}}
	makeOutput := func() *RuntimeCommandResult {
		return &RuntimeCommandResult{WorkingDirectory: "", ValidationObservation: &runtimecontract.RuntimeValidationObservation{FailedChecks: []string{"TestA"}}, goTestJSONFacts: &shellGoTestJSONFacts{CaseOutcomes: map[string]string{"checks\x00TestA": "fail"}, DiagnosticRecords: []shellGoTestJSONDiagnosticRecord{{Package: "checks", Test: "TestA", Action: "output", Output: "    a_test.go:4: changing elapsed_ms=123\n    a_test.go:5: changing elapsed_ms=456\n    a_test.go:6: ordinary\n    a_test.go:6: same line\n    a_test.go:7: helper observation\n"}}}}
	}
	request := RuntimeCommandRequest{ValidationScopeTargetRefs: []string{"checks/a_test.go"}, validationProofBinding: runtimecontract.RuntimeValidationProofRequirementBinding{InputReview: subject}}
	out := makeOutput()
	attachShellValidationCheckFailures(out, request)
	if out.ValidationObservation.FailureDetailsTruncated || len(out.ValidationObservation.CheckFailures) != 4 {
		t.Fatalf("only unambiguous direct observation may be excluded: %+v", out.ValidationObservation)
	}
	for _, detail := range out.ValidationObservation.CheckFailures {
		if detail.Line == 4 {
			t.Fatal("ordinary log became assertion")
		}
		if detail.Line == 5 && detail.AssertionText != "changing elapsed_ms=456" {
			t.Fatal("real error was rewritten")
		}
	}
	subject.Files[0].ContentHash = strings.Repeat("0", 64)
	out = makeOutput()
	attachShellValidationCheckFailures(out, request)
	if len(out.ValidationObservation.CheckFailures) != 5 {
		t.Fatal("unverified source classified output")
	}
}

func TestNativeObservationCompilerRejectsShadowedAndAmbiguousReceivers(t *testing.T) {
	source := `package checks
import "testing"
func TestA(t *testing.T) {
 t.Log("observation")
 { t:=custom{}; t.Log("possible failure") }
 t.Log("observation"); t.Fatal("failure")
}
type custom struct{}
func(custom) Log(s string){}
`
	s := &runtimecontract.RuntimeValidationInputReviewSubject{Files: []runtimecontract.RuntimeValidationInputReviewFile{{TargetRef: "a_test.go", Content: source, ContentHash: runtimecontract.RuntimeValidationInputContentHash(source)}}}
	lines := shellNativeObservationLines(s)["a_test.go"]
	if !lines[4] || lines[5] || lines[6] {
		t.Fatalf("receiver/line ambiguity hidden: %v", lines)
	}
}

func TestNativeFailureReproductionKeepsValuesAndSourceButNotHelperMultiplicity(t *testing.T) {
	detail := runtimecontract.RuntimeValidationCheckFailure{CheckRef: "TestA", CaseRef: "TestA/child", PackageRef: "pkg", TargetRef: "a_test.go", Line: 8, AssertionText: "expected=13 actual=15", ComparisonHash: strings.Repeat("a", 64)}
	second := detail
	second.Line = 9
	if !shellSameFailureAssertions([]runtimecontract.RuntimeValidationCheckFailure{detail, second}, []runtimecontract.RuntimeValidationCheckFailure{second}) {
		t.Fatal("helper attribution multiplicity became another failure")
	}
	for _, change := range []func(*runtimecontract.RuntimeValidationCheckFailure){func(v *runtimecontract.RuntimeValidationCheckFailure) { v.ComparisonHash = strings.Repeat("b", 64) }, func(v *runtimecontract.RuntimeValidationCheckFailure) { v.CaseRef = "TestA/other" }, func(v *runtimecontract.RuntimeValidationCheckFailure) { v.TargetRef = "b_test.go" }, func(v *runtimecontract.RuntimeValidationCheckFailure) { v.PackageRef = "other" }} {
		changed := detail
		change(&changed)
		if shellSameFailureAssertions([]runtimecontract.RuntimeValidationCheckFailure{detail}, []runtimecontract.RuntimeValidationCheckFailure{changed}) {
			t.Fatal("real failure or source changed")
		}
	}
	if shellSameFailureAssertions([]runtimecontract.RuntimeValidationCheckFailure{detail}, nil) {
		t.Fatal("missing assertion accepted")
	}
}

func TestNativeFailureCapacityPreserves39AndRejectsRealOverflow(t *testing.T) {
	for _, count := range []int{31, 32, 33, 39, runtimecontract.RuntimeValidationFailureDetailLimit, runtimecontract.RuntimeValidationFailureDetailLimit + 1} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			output := RuntimeCommandResult{ValidationObservation: &runtimecontract.RuntimeValidationObservation{FailedChecks: []string{"TestA"}}, goTestJSONFacts: &shellGoTestJSONFacts{}}
			for n := range count {
				output.goTestJSONFacts.DiagnosticRecords = append(output.goTestJSONFacts.DiagnosticRecords, shellGoTestJSONDiagnosticRecord{Test: "TestA", Action: "output", Output: fmt.Sprintf("a_test.go:%d: failure %d\n", n+1, n)})
			}
			attachShellValidationCheckFailures(&output, RuntimeCommandRequest{ValidationScopeTargetRefs: []string{"a_test.go"}})
			if count <= runtimecontract.RuntimeValidationFailureDetailLimit {
				if output.ValidationObservation.FailureDetailsTruncated || len(output.ValidationObservation.CheckFailures) != count || output.validationFailureDetailIssue != nil {
					t.Fatal("complete bounded failures lost")
				}
			} else if !output.ValidationObservation.FailureDetailsTruncated || output.validationFailureDetailIssue == nil || !output.validationFailureDetailIssue.capacity || output.validationFailureDetailIssue.code != "validation_failure_assertion_count_exceeded" {
				t.Fatal("overflow lost precise capacity classification")
			}
		})
	}
}

func TestInapplicableLargeObservationIsExcludedBeforeAssertionCapacity(t *testing.T) {
	source := "package checks\nimport \"testing\"\nfunc TestA(t *testing.T){\n t.Log(\"observation\")\n t.Error(\"real failure\")\n}\n"
	subject := &runtimecontract.RuntimeValidationInputReviewSubject{Files: []runtimecontract.RuntimeValidationInputReviewFile{{TargetRef: "a_test.go", Content: source, ContentHash: runtimecontract.RuntimeValidationInputContentHash(source)}}}
	output := RuntimeCommandResult{ValidationObservation: &runtimecontract.RuntimeValidationObservation{FailedChecks: []string{"TestA"}}, goTestJSONFacts: &shellGoTestJSONFacts{DiagnosticRecords: []shellGoTestJSONDiagnosticRecord{{Action: "output", Test: "TestA", Output: "    outside_test.go:1: " + strings.Repeat("outside_scope", 4000) + "\n    a_test.go:4: " + strings.Repeat("observation", 4000) + "\n        nested_test.go:999: nested log\n    a_test.go:5: real failure\n"}}}}
	attachShellValidationCheckFailures(&output, RuntimeCommandRequest{ValidationScopeTargetRefs: []string{"a_test.go"}, validationProofBinding: runtimecontract.RuntimeValidationProofRequirementBinding{InputReview: subject}})
	if output.ValidationObservation.FailureDetailsTruncated || output.validationFailureDetailIssue != nil || len(output.ValidationObservation.CheckFailures) != 1 || output.ValidationObservation.CheckFailures[0].AssertionText != "real failure" {
		t.Fatal("inapplicable log consumed assertion capacity", output.ValidationObservation)
	}
}

func TestNativeObservationRetainsEffectfulArgumentsAndUserFormatting(t *testing.T) {
	source := `package checks
import("testing";"time")
type message struct { t *testing.T }
func(m message) String() string {m.t.Helper();m.t.Error("real failure");return "text"}
func helper(t *testing.T) int {t.Helper();t.Error("real failure");return 0}
func TestA(t *testing.T) {
 t.Log(helper(t))
 t.Log(message{t})
 value:=message{t}; t.Log(value)
 t.Logf("time=%d",time.Now().UnixNano())
 t.Logf("literal=%d",3)
 t.Log(func() string {t.Helper();t.Error("real failure");return "x"}())
}
`
	subject := &runtimecontract.RuntimeValidationInputReviewSubject{Files: []runtimecontract.RuntimeValidationInputReviewFile{{TargetRef: "a_test.go", Content: source, ContentHash: runtimecontract.RuntimeValidationInputContentHash(source)}}}
	lines := shellNativeObservationLines(subject)["a_test.go"]
	if lines[7] || lines[8] || lines[9] || lines[12] || !lines[10] || !lines[11] {
		t.Fatalf("unsafe log argument lost or safe native primitive not classified: %v", lines)
	}
}

func TestNativeAssertionBodyAndAggregateCapacityHaveExactCauses(t *testing.T) {
	for _, scenario := range []string{"body", "aggregate"} {
		t.Run(scenario, func(t *testing.T) {
			output := RuntimeCommandResult{ValidationObservation: &runtimecontract.RuntimeValidationObservation{FailedChecks: []string{"TestA"}}, goTestJSONFacts: &shellGoTestJSONFacts{}}
			count, size := 1, runtimecontract.RuntimeValidationAssertionMaxBytes+1
			want := "validation_failure_assertion_body_exceeded"
			if scenario == "aggregate" {
				count = 9
				size = runtimecontract.RuntimeValidationAssertionMaxBytes
				want = "validation_failure_assertions_bytes_exceeded"
			}
			for n := 0; n < count; n++ {
				output.goTestJSONFacts.DiagnosticRecords = append(output.goTestJSONFacts.DiagnosticRecords, shellGoTestJSONDiagnosticRecord{Test: "TestA", Action: "output", Output: fmt.Sprintf("a_test.go:%d: %s\n", n+1, strings.Repeat("x", size))})
			}
			attachShellValidationCheckFailures(&output, RuntimeCommandRequest{ValidationScopeTargetRefs: []string{"a_test.go"}})
			if !output.ValidationObservation.FailureDetailsTruncated || output.validationFailureDetailIssue == nil || !output.validationFailureDetailIssue.capacity || output.validationFailureDetailIssue.code != want {
				t.Fatalf("wrong capacity dimension: %+v", output.validationFailureDetailIssue)
			}
		})
	}
}

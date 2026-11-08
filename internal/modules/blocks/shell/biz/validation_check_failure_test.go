package shell

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestShellValidationFailureDetailsJoinExactFailedCheckAfterNoisyOutput(t *testing.T) {
	events := []shellGoTestJSONEvent{
		{Action: "output", Package: "example/checks", Test: "TestB", Output: strings.Repeat("noise", 1000)},
		{Action: "output", Package: "example/checks", Test: "TestB", Output: "b_test.go:10: unrelated assertion"},
		{Action: "pass", Test: "TestB"},
		{Action: "run", Test: "TestA"},
		{Action: "output", Package: "example/checks", Test: "TestA/case", Output: "a_test.go:47: expected private-token, got private-value\n"},
		{Action: "output", Package: "example/checks", Test: "TestA/case", Output: "/outside/secret_test.go:1: do not export\n"},
		{Action: "fail", Test: "TestA"},
	}
	var stdout strings.Builder
	for _, event := range events {
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		stdout.Write(encoded)
		stdout.WriteByte('\n')
	}
	output := RuntimeCommandResult{Stdout: stdout.String(), ExitCode: 1}
	attachShellValidationObservation(&output, "go", []string{"test", "-json"}, "recipe", true)
	attachShellValidationCheckFailures(&output, RuntimeCommandRequest{ValidationScopeTargetRefs: []string{"checks/a_test.go", "checks/b_test.go"}})
	observation := output.ValidationObservation
	if observation == nil || len(observation.CheckFailures) != 1 {
		t.Fatalf("lost exact failed check behind noise: %+v", observation)
	}
	detail := observation.CheckFailures[0]
	if detail.CheckRef != "TestA" || detail.CaseRef != "TestA/case" || detail.TargetRef != "checks/a_test.go" || detail.Line != 47 || len(detail.DiagnosticHash) != 64 {
		t.Fatalf("wrong check/location association: %+v", detail)
	}
	encoded, _ := json.Marshal(observation.CheckFailures)
	if detail.AssertionText != "expected private-token, got private-value" {
		t.Fatal("exact scoped assertion lost")
	}
	for _, private := range []string{"/outside", "unrelated assertion"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("raw text leaked: %s", encoded)
		}
	}
	if err := observation.ValidateFailureDetails(); err != nil {
		t.Fatal(err)
	}
}

func TestShellValidationFailureDetailsAreBoundedAndDoNotInventLocations(t *testing.T) {
	output := RuntimeCommandResult{ValidationObservation: &runtimecontract.RuntimeValidationObservation{FailedChecks: []string{"TestA"}},
		goTestJSONFacts: &shellGoTestJSONFacts{}}
	for i := 0; i <= runtimecontract.RuntimeValidationFailureDetailLimit; i++ {
		output.goTestJSONFacts.DiagnosticRecords = append(output.goTestJSONFacts.DiagnosticRecords, shellGoTestJSONDiagnosticRecord{
			Action: "output", Test: "TestA", Output: fmt.Sprintf("a_test.go:%d: assertion %d\n", i+1, i)})
	}
	request := RuntimeCommandRequest{ValidationScopeTargetRefs: []string{"a_test.go"}}
	attachShellValidationCheckFailures(&output, request)
	if len(output.ValidationObservation.CheckFailures) != runtimecontract.RuntimeValidationFailureDetailLimit || !output.ValidationObservation.FailureDetailsTruncated {
		t.Fatal("diagnostic bound was not explicit")
	}
	attachShellValidationCheckFailures(&output, request)
	if len(output.ValidationObservation.CheckFailures) != runtimecontract.RuntimeValidationFailureDetailLimit || output.ValidationObservation.ValidateFailureDetails() != nil {
		t.Fatal("repeated fact assembly duplicated diagnostics")
	}
	output.ValidationObservation = &runtimecontract.RuntimeValidationObservation{FailedChecks: []string{"TestA"}}
	attachShellValidationCheckFailures(&output, RuntimeCommandRequest{})
	if len(output.ValidationObservation.CheckFailures) != 0 {
		t.Fatal("unknown scope manufactured source location")
	}
}

func TestShellValidationFailureDetailsPreserveMultilineNativeBody(t *testing.T) {
	output := RuntimeCommandResult{ValidationObservation: &runtimecontract.RuntimeValidationObservation{FailedChecks: []string{"TestA"}}, goTestJSONFacts: &shellGoTestJSONFacts{CaseOutcomes: map[string]string{"pkg\x00TestA": "fail"}, DiagnosticRecords: []shellGoTestJSONDiagnosticRecord{
		{Action: "output", Package: "pkg", Test: "TestA", Output: "    a_test.go:7: expected:\n"},
		{Action: "output", Package: "pkg", Test: "TestA", Output: "        original exact value\r\n        got another value\n"},
		{Action: "output", Package: "pkg", Test: "TestA", Output: "--- FAIL: TestA (0.00s)\n"},
	}}}
	attachShellValidationCheckFailures(&output, RuntimeCommandRequest{ValidationScopeTargetRefs: []string{"a_test.go"}})
	o := output.ValidationObservation
	if o.FailureDetailsTruncated || len(o.CheckFailures) != 1 || o.CheckFailures[0].AssertionText != "expected:\n        original exact value\r\n        got another value" {
		t.Fatalf("multiline native body lost: %+v", o)
	}
	if o.ValidateFailureDetails() != nil {
		t.Fatal("multiline identity invalid")
	}
	if runtimecontract.ProjectRuntimeValidationExecutionFeedback(o).FailureDetailsIncomplete {
		t.Fatal("complete native body marked missing")
	}
}

func TestShellValidationFailureDetailsExplicitGaps(t *testing.T) {
	for name, body := range map[string]string{"oversize": strings.Repeat("a", runtimecontract.RuntimeValidationAssertionMaxBytes+1), "utf8": string([]byte{0xff}), "unframed": "first line\nno native continuation frame"} {
		t.Run(name, func(t *testing.T) {
			output := RuntimeCommandResult{ValidationObservation: &runtimecontract.RuntimeValidationObservation{FailedChecks: []string{"TestA"}}, goTestJSONFacts: &shellGoTestJSONFacts{DiagnosticRecords: []shellGoTestJSONDiagnosticRecord{{Action: "output", Test: "TestA", Output: "a_test.go:1: " + body}}}}
			attachShellValidationCheckFailures(&output, RuntimeCommandRequest{ValidationScopeTargetRefs: []string{"a_test.go"}})
			if !output.ValidationObservation.FailureDetailsTruncated || !runtimecontract.ProjectRuntimeValidationExecutionFeedback(output.ValidationObservation).FailureDetailsIncomplete {
				t.Fatal("partial native body appeared complete")
			}
		})
	}
}

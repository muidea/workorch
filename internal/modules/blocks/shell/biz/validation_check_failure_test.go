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
		{Action: "output", Package: "pkg", Test: "TestA", Output: "        original exact value\r\n\n        got another value\n"},
		{Action: "output", Package: "pkg", Test: "TestA", Output: "--- FAIL: TestA (0.00s)\n"},
	}}}
	attachShellValidationCheckFailures(&output, RuntimeCommandRequest{ValidationScopeTargetRefs: []string{"a_test.go"}})
	o := output.ValidationObservation
	if o.FailureDetailsTruncated || len(o.CheckFailures) != 1 || o.CheckFailures[0].AssertionText != "expected:\n        original exact value\r\n\n        got another value" {
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

func TestShellValidationAssertionFramesSeparateOuterLogsAndNestedSourceText(t *testing.T) {
	// Native decoration defines the body boundary; neither application log
	// vocabulary nor a source-looking nested transcript can choose attribution.
	body := "    a_test.go:7: nested command output:\n        b_test.go:19: inner failure\n        tool: command failed\nouter process output\n    a_test.go:9: actual failure\n--- FAIL: TestA (0.00s)\n"
	for _, split := range []int{0, 23, len(body) - 2} {
		out := RuntimeCommandResult{ValidationObservation: &runtimecontract.RuntimeValidationObservation{FailedChecks: []string{"TestA"}}, goTestJSONFacts: &shellGoTestJSONFacts{CaseOutcomes: map[string]string{"pkg\x00TestA": "fail"}}}
		for _, text := range []string{body[:split], body[split:]} {
			out.goTestJSONFacts.DiagnosticRecords = append(out.goTestJSONFacts.DiagnosticRecords, shellGoTestJSONDiagnosticRecord{Action: "output", Package: "pkg", Test: "TestA", Output: text})
		}
		attachShellValidationCheckFailures(&out, RuntimeCommandRequest{ValidationScopeTargetRefs: []string{"a_test.go", "b_test.go"}})
		o := out.ValidationObservation
		if o.FailureDetailsTruncated || len(o.CheckFailures) != 2 || o.CheckFailures[0].TargetRef != "a_test.go" || o.CheckFailures[0].Line != 7 || o.CheckFailures[0].AssertionText != "nested command output:\n        b_test.go:19: inner failure\n        tool: command failed" || o.CheckFailures[1].Line != 9 || o.CheckFailures[1].AssertionText != "actual failure" {
			t.Fatalf("split=%d frame identity/body lost: %+v", split, o)
		}
	}
}

func TestShellValidationAssertionEqualIndentUnknownContinuationRemainsIncomplete(t *testing.T) {
	out := RuntimeCommandResult{ValidationObservation: &runtimecontract.RuntimeValidationObservation{FailedChecks: []string{"TestA"}}, goTestJSONFacts: &shellGoTestJSONFacts{DiagnosticRecords: []shellGoTestJSONDiagnosticRecord{{Action: "output", Test: "TestA", Output: "    a_test.go:3: assertion\n    unknown continuation\n"}}}}
	attachShellValidationCheckFailures(&out, RuntimeCommandRequest{ValidationScopeTargetRefs: []string{"a_test.go"}})
	if !out.ValidationObservation.FailureDetailsTruncated {
		t.Fatal("ambiguous same-frame text was silently dropped")
	}
}

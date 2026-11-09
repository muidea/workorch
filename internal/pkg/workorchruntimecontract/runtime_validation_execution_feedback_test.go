package workorchruntimecontract

import (
	"fmt"
	"strings"
	"testing"
)

func TestRuntimeValidationExecutionFeedbackBoundsPreserveValidIncompleteFacts(t *testing.T) {
	var checks []string
	for i := 0; i <= RuntimeValidationFailureDetailLimit; i++ {
		checks = append(checks, fmt.Sprintf("check/%03d", i))
	}
	observation := RuntimeValidationObservation{ExecutedChecks: checks, FailedChecks: checks,
		CheckFailures: []RuntimeValidationCheckFailure{{CheckRef: checks[len(checks)-1], DiagnosticHash: strings.Repeat("a", 64)}}}
	if err := observation.ValidateFailureDetails(); err != nil {
		t.Fatal(err)
	}
	f := ProjectRuntimeValidationExecutionFeedback(&observation)
	if !f.ChecksTruncated || !f.FailureDetailsIncomplete || len(f.FailedChecks) != RuntimeValidationFailureDetailLimit || len(f.CheckFailures) != 0 {
		t.Fatalf("bounded projection lost truncation or retained orphan detail: %+v", f)
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("valid bounded incomplete feedback rejected: %v", err)
	}
	if len(observation.CheckFailures) != 1 || len(observation.FailedChecks) != RuntimeValidationFailureDetailLimit+1 {
		t.Fatal("projection changed immutable observation")
	}
}

func TestRuntimeValidationExecutionFeedbackReportsSuccessSkipAndMissingObservation(t *testing.T) {
	if ProjectRuntimeValidationExecutionFeedback(nil) != nil {
		t.Fatal("missing observation must remain explicit")
	}
	f := ProjectRuntimeValidationExecutionFeedback(&RuntimeValidationObservation{
		ExecutedChecks: []string{"check/pass"}, PassedChecks: []string{"check/pass"}, SkippedChecks: []string{"check/skip"}, IncompleteChecks: []string{"check/incomplete"},
	})
	if err := f.Validate(); err != nil || len(f.PassedChecks) != 1 || len(f.SkippedChecks) != 1 || len(f.IncompleteChecks) != 1 {
		t.Fatalf("check dispositions lost: %+v err=%v", f, err)
	}
}

func TestValidationFailureFeedbackExcludesUnrelatedPassingSuiteWithoutLosingFailures(t *testing.T) {
	body := "expected original, got actual"
	digest := RuntimeValidationInputContentHash(body)
	o := RuntimeValidationObservation{FailedChecks: []string{"failed"}, ExecutedChecks: []string{"failed"}, FailureDetailProtocolRef: RuntimeValidationFailureDetailProtocol,
		CheckFailures: []RuntimeValidationCheckFailure{{CheckRef: "failed", TargetRef: "input.go", Line: 9, DiagnosticHash: digest, ComparisonHash: digest, AssertionText: body}}}
	for i := 0; i < 150; i++ {
		c := fmt.Sprintf("unrelated-pass-%03d", i)
		o.ExecutedChecks = append(o.ExecutedChecks, c)
		o.PassedChecks = append(o.PassedChecks, c)
	}
	f := ProjectRuntimeValidationExecutionFeedback(&o)
	if f.Validate() != nil || f.ChecksTruncated || f.FailureDetailsIncomplete || len(f.ExecutedChecks) != 1 || len(f.PassedChecks) != 0 || len(f.CheckFailures) != 1 || len(o.PassedChecks) != 150 {
		t.Fatalf("unrelated pass inventory blocked exact failure input: %+v", f)
	}
	o.PassedChecks = append(o.PassedChecks, "failed")
	f = ProjectRuntimeValidationExecutionFeedback(&o)
	if len(f.PassedChecks) != 1 || f.PassedChecks[0] != "failed" {
		t.Fatal("same-check conflict was hidden")
	}
}

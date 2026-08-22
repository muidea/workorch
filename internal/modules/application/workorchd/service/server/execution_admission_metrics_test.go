package server

import (
	"strings"
	"testing"
)

func TestExecutionAdmissionMetricsContainOnlyMonotonicCounters(t *testing.T) {
	metrics := newExecutionAdmissionMetrics()
	metrics.recordCommand(executionAdmissionKindWorkspaceRun, "accepted")
	metrics.recordCommand(executionAdmissionKindWorkspaceRun, "accepted")
	metrics.recordFailure(executionAdmissionKindWorkspaceRun, executionAdmissionReasonSubmissionFailed)

	body := metrics.render()
	for _, want := range []string{
		`workorch_execution_admission_commands_total{kind="workspace_run",result="accepted"} 2`,
		`workorch_execution_admission_failures_total{kind="workspace_run",reason="submission_failed"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{"attempt", "dedupe", "recoverability", "recommended"} {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Fatalf("metrics retained execution state %q:\n%s", forbidden, body)
		}
	}
}

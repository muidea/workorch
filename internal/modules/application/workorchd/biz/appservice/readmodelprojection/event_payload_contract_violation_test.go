package readmodelprojection

import "testing"

func TestProjectEventPayloadKeepsContractViolationSummaryFields(t *testing.T) {
	projected := projectEventPayload(map[string]any{
		"workspaceId":          "ws-contract-event",
		"runId":                "run-contract-event",
		"stepId":               "step-contract-event",
		"sourceStepId":         "source-step",
		"retryStepId":          "retry-step",
		"ticketId":             "ticket-step",
		"violatedRule":         "state_changing_required_missing",
		"recoverability":       "retry_same_contract",
		"correctionProfileId":  "state_changing_required_output",
		"correctionMode":       "forced_narrow",
		"targetScope":          "failureTargets=target.txt",
		"attempt":              1,
		"maxAttempts":          2,
		"signatureRepeatCount": 1,
		"failureSignature":     "rule=state_changing_required_missing",
		"rejectedOutputRef":    "run/step/rejected-output",
		"summary":              "contract violation retry_requested: state_changing_required_missing",
		"contractViolation": map[string]any{
			"actualOutputSummary": map[string]any{"large": "object"},
		},
		"context": map[string]any{"large": "object"},
	})

	for _, key := range []string{
		"workspaceId",
		"runId",
		"stepId",
		"sourceStepId",
		"retryStepId",
		"ticketId",
		"violatedRule",
		"recoverability",
		"attempt",
		"maxAttempts",
		"signatureRepeatCount",
		"correctionProfileId",
		"correctionMode",
		"targetScope",
		"failureSignature",
		"rejectedOutputRef",
		"summary",
	} {
		if _, ok := projected[key]; !ok {
			t.Fatalf("projected payload = %+v, want key %s", projected, key)
		}
	}
	if _, ok := projected["contractViolation"]; ok {
		t.Fatalf("projected payload = %+v, want heavy contractViolation object omitted", projected)
	}
	if _, ok := projected["context"]; ok {
		t.Fatalf("projected payload = %+v, want heavy context object omitted", projected)
	}
}

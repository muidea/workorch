package workorchexecutioncontract

import (
	"net/http"
	"testing"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPresentAdmissionClassifiesAllCanonicalOutcomes(t *testing.T) {
	tests := []struct {
		outcome  runtimecontract.RuntimeExecutionIntentOutcome
		reason   runtimecontract.RuntimeExecutionIntentReason
		accepted bool
		status   int
		display  string
	}{
		{runtimecontract.RuntimeExecutionIntentReconciliationRequested, runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted, true, http.StatusAccepted, AdmissionDisplayAccepted},
		{runtimecontract.RuntimeExecutionIntentReplacementPending, runtimecontract.RuntimeExecutionIntentReasonDuplicateRestart, true, http.StatusAccepted, AdmissionDisplayAccepted},
		{runtimecontract.RuntimeExecutionIntentRestartRequired, runtimecontract.RuntimeExecutionIntentReasonRunNotResumable, false, http.StatusConflict, AdmissionDisplayRejected},
		{runtimecontract.RuntimeExecutionIntentFreshActivationRequired, runtimecontract.RuntimeExecutionIntentReasonRuntimeStateIncompatible, false, http.StatusConflict, AdmissionDisplayRejected},
		{runtimecontract.RuntimeExecutionIntentDefinitionChanged, runtimecontract.RuntimeExecutionIntentReasonDefinitionChanged, false, http.StatusConflict, AdmissionDisplayRejected},
		{runtimecontract.RuntimeExecutionIntentGovernanceRequired, runtimecontract.RuntimeExecutionIntentReasonGovernancePending, false, http.StatusConflict, AdmissionDisplayRejected},
		{runtimecontract.RuntimeExecutionIntentDataResetRequired, runtimecontract.RuntimeExecutionIntentReasonDevelopmentDataIncompatible, false, http.StatusConflict, AdmissionDisplayRejected},
	}
	for _, test := range tests {
		t.Run(string(test.outcome), func(t *testing.T) {
			got, err := PresentAdmission(runtimecontract.RuntimeExecutionIntentAdmission{
				WorkspaceID: "ws-1", RequestID: "request-1", Kind: runtimecontract.RuntimeExecutionIntentResume,
				Outcome: test.outcome, Reason: test.reason,
			})
			if err != nil {
				t.Fatalf("present admission: %v", err)
			}
			if got.Accepted != test.accepted || got.HTTPStatusCode != test.status || got.Display != test.display {
				t.Fatalf("presentation = %+v, want accepted=%t status=%d display=%q", got, test.accepted, test.status, test.display)
			}
		})
	}
}

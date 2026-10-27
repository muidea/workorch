package workorchexecutioncontract

import (
	"net/http"
	"strings"
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
		{runtimecontract.RuntimeExecutionIntentFreshActivationRequired, runtimecontract.RuntimeExecutionIntentReasonRuntimeBootstrapMissing, false, http.StatusConflict, AdmissionDisplayRejected},
		{runtimecontract.RuntimeExecutionIntentDefinitionChanged, runtimecontract.RuntimeExecutionIntentReasonDefinitionChanged, false, http.StatusConflict, AdmissionDisplayRejected},
		{runtimecontract.RuntimeExecutionIntentGovernanceRequired, runtimecontract.RuntimeExecutionIntentReasonGovernancePending, false, http.StatusConflict, AdmissionDisplayRejected},
		{runtimecontract.RuntimeExecutionIntentGovernanceRequired, runtimecontract.RuntimeExecutionIntentReasonDefinitionClarificationPending, false, http.StatusConflict, AdmissionDisplayRejected},
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

func TestUnresolvedAdmissionRoundTripPreservesBothRequestIdentities(t *testing.T) {
	a := runtimecontract.RuntimeExecutionIntentAdmission{WorkspaceID: "ws", RequestID: "current", Kind: runtimecontract.RuntimeExecutionIntentRestart,
		Outcome: runtimecontract.RuntimeExecutionIntentFreshUnresolved, Reason: runtimecontract.RuntimeExecutionIntentReasonFreshResultUnknown,
		PendingRequestID: "original", PendingStartupInputHash: "startup", PendingPayloadHash: strings.Repeat("a", 64)}
	r, status, err := NewWorkspaceExecutionAdmissionResponse("workorch", "memory", "", "execution-activation", "", "", a)
	if err != nil || status != http.StatusConflict || r.Accepted || r.State != AdmissionDisplayPending || r.RunID != "" {
		t.Fatalf("unresolved was misclassified: %+v %d %v", r, status, err)
	}
	got, err := r.Admission()
	if err != nil || got != a {
		t.Fatalf("lost original identity: %+v %v", got, err)
	}
	r.State = AdmissionDisplayRejected
	if _, err := r.Admission(); err == nil {
		t.Fatal("pending was relabeled rejected")
	}
}

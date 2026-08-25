package backend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	executioncontract "github.com/muidea/workorch/internal/pkg/workorchexecutioncontract"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestLocalAndRemoteRunBackendsReturnTheSameOwnerAdmission(t *testing.T) {
	tests := []struct {
		outcome runtimecontract.RuntimeExecutionIntentOutcome
		reason  runtimecontract.RuntimeExecutionIntentReason
	}{
		{runtimecontract.RuntimeExecutionIntentReconciliationRequested, runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted},
		{runtimecontract.RuntimeExecutionIntentReplacementPending, runtimecontract.RuntimeExecutionIntentReasonDuplicateRestart},
		{runtimecontract.RuntimeExecutionIntentRestartRequired, runtimecontract.RuntimeExecutionIntentReasonRunNotResumable},
		{runtimecontract.RuntimeExecutionIntentFreshActivationRequired, runtimecontract.RuntimeExecutionIntentReasonRuntimeBootstrapMissing},
		{runtimecontract.RuntimeExecutionIntentDefinitionChanged, runtimecontract.RuntimeExecutionIntentReasonDefinitionChanged},
		{runtimecontract.RuntimeExecutionIntentGovernanceRequired, runtimecontract.RuntimeExecutionIntentReasonGovernancePending},
		{runtimecontract.RuntimeExecutionIntentGovernanceRequired, runtimecontract.RuntimeExecutionIntentReasonDefinitionClarificationPending},
	}
	for _, test := range tests {
		t.Run(string(test.outcome), func(t *testing.T) {
			admission := runtimecontract.RuntimeExecutionIntentAdmission{
				WorkspaceID: "ws-consistency", RunID: "run-consistency", RequestID: "request-consistency",
				Kind: runtimecontract.RuntimeExecutionIntentResume, Outcome: test.outcome, Reason: test.reason,
			}
			var remoteCalls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				remoteCalls.Add(1)
				response, statusCode, err := executioncontract.NewWorkspaceExecutionAdmissionResponse("workorch", "memory", "", "execution-activation", "test", "test", admission)
				if err != nil {
					t.Fatalf("build HTTP admission: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(statusCode)
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			hub := magicevent.NewHub(16)
			defer hub.Terminate(context.Background())
			var localCalls atomic.Int64
			observer := magicevent.NewSimpleObserver(workorchdevents.CommandDestination, hub)
			observer.Subscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, func(_ magicevent.Event, result magicevent.Result) {
				localCalls.Add(1)
				result.Set(workorchdevents.WorkspaceRunRequestedResult(admission), nil)
			})
			defer observer.Unsubscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested)

			request := port.WorkspaceRunCommand{
				WorkspaceID: admission.WorkspaceID, Intent: port.RunCommandIntentResume,
				RequestID: admission.RequestID, Reason: "test", Source: "test",
			}
			local, localErr := NewEventHub(hub).Run().Command(context.Background(), request)
			remote, remoteErr := NewRemote(server.URL).Run().Command(context.Background(), request)
			if localErr != nil || remoteErr != nil {
				t.Fatalf("local err=%v remote err=%v", localErr, remoteErr)
			}
			if local == nil || remote == nil || !reflect.DeepEqual(*local, *remote) || !reflect.DeepEqual(runtimecontract.RuntimeExecutionIntentAdmission(*local), admission) {
				t.Fatalf("local=%+v remote=%+v want=%+v", local, remote, admission)
			}
			if localCalls.Load() != 1 || remoteCalls.Load() != 1 {
				t.Fatalf("local calls=%d remote calls=%d, want one owner command and one HTTP request", localCalls.Load(), remoteCalls.Load())
			}
		})
	}
}

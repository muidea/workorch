package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	executioncontract "github.com/muidea/workorch/internal/pkg/workorchexecutioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type recordingWorkspaceExecutionSubmitter struct {
	commands []workorchdevents.WorkspaceRunRequestedCommand
}

type fixedWorkspaceExecutionAdmission struct {
	outcome runtimecontract.RuntimeExecutionIntentOutcome
	reason  runtimecontract.RuntimeExecutionIntentReason
	calls   []workorchdevents.WorkspaceRunRequestedCommand
}

func (a *fixedWorkspaceExecutionAdmission) AdmitWorkspaceExecution(_ context.Context, _ string, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	a.calls = append(a.calls, command)
	return workorchdevents.WorkspaceRunRequestedResult{
		WorkspaceID: command.WorkspaceID,
		RunID:       "run-typed-http",
		RequestID:   command.RequestID,
		Kind:        runtimecontract.RuntimeExecutionIntentKind(command.Intent),
		Outcome:     a.outcome,
		Reason:      a.reason,
	}, nil
}

func TestWorkspaceExecutionHTTPClassifiesAllOwnerOutcomesWithoutLosingIdentity(t *testing.T) {
	tests := []struct {
		outcome runtimecontract.RuntimeExecutionIntentOutcome
		reason  runtimecontract.RuntimeExecutionIntentReason
		status  int
		state   string
	}{
		{runtimecontract.RuntimeExecutionIntentReconciliationRequested, runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted, http.StatusAccepted, executioncontract.AdmissionDisplayAccepted},
		{runtimecontract.RuntimeExecutionIntentReplacementPending, runtimecontract.RuntimeExecutionIntentReasonDuplicateRestart, http.StatusAccepted, executioncontract.AdmissionDisplayAccepted},
		{runtimecontract.RuntimeExecutionIntentRestartRequired, runtimecontract.RuntimeExecutionIntentReasonRunNotResumable, http.StatusConflict, executioncontract.AdmissionDisplayRejected},
		{runtimecontract.RuntimeExecutionIntentFreshActivationRequired, runtimecontract.RuntimeExecutionIntentReasonRuntimeBootstrapMissing, http.StatusConflict, executioncontract.AdmissionDisplayRejected},
		{runtimecontract.RuntimeExecutionIntentDefinitionChanged, runtimecontract.RuntimeExecutionIntentReasonDefinitionChanged, http.StatusConflict, executioncontract.AdmissionDisplayRejected},
		{runtimecontract.RuntimeExecutionIntentGovernanceRequired, runtimecontract.RuntimeExecutionIntentReasonGovernancePending, http.StatusConflict, executioncontract.AdmissionDisplayRejected},
		{runtimecontract.RuntimeExecutionIntentGovernanceRequired, runtimecontract.RuntimeExecutionIntentReasonDefinitionClarificationPending, http.StatusConflict, executioncontract.AdmissionDisplayRejected},
	}
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	admissionPort := &fixedWorkspaceExecutionAdmission{}
	state.workspaceExecutionAdmission = admissionPort
	handler := newMuxWithState(state)
	for index, test := range tests {
		t.Run(string(test.outcome), func(t *testing.T) {
			admissionPort.outcome = test.outcome
			admissionPort.reason = test.reason
			requestID := "request-outcome-" + string(rune('a'+index))
			req := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/ws-typed-http/execution-activations", strings.NewReader(`{"intent":"resume","requestId":"`+requestID+`","reason":"operator_request","source":"test"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != test.status {
				t.Fatalf("status = %d, want %d, body=%s", rec.Code, test.status, rec.Body.String())
			}
			var response executioncontract.WorkspaceExecutionAdmissionResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response.State != test.state || response.Outcome != test.outcome || response.Reason != test.reason || response.WorkspaceID != "ws-typed-http" || response.RequestID != requestID || response.Intent != runtimecontract.RuntimeExecutionIntentResume {
				t.Fatalf("response = %+v, want exact owner admission", response)
			}
		})
	}
	if len(admissionPort.calls) != len(tests) {
		t.Fatalf("owner commands = %d, want exactly one per request", len(admissionPort.calls))
	}
}

func TestWorkspaceExecutionHTTPRetiresCallerRunPriority(t *testing.T) {
	state := newTestServerState(t, normalizeConfig(Config{Backend: "memory"}))
	admissionPort := &fixedWorkspaceExecutionAdmission{
		outcome: runtimecontract.RuntimeExecutionIntentReconciliationRequested,
		reason:  runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
	}
	state.workspaceExecutionAdmission = admissionPort
	handler := newMuxWithState(state)

	bodyPriority := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/ws-priority-retired/execution-activations", strings.NewReader(`{"intent":"resume","requestId":"request-body-priority","priority":9}`))
	bodyPriority.Header.Set("Content-Type", "application/json")
	bodyPriorityRecorder := httptest.NewRecorder()
	handler.ServeHTTP(bodyPriorityRecorder, bodyPriority)
	if bodyPriorityRecorder.Code != http.StatusBadRequest || len(admissionPort.calls) != 0 {
		t.Fatalf("body priority status=%d owner calls=%d, want 400 and zero owner commands", bodyPriorityRecorder.Code, len(admissionPort.calls))
	}

	queryAndHeader := httptest.NewRequest(http.MethodPost, "/api/v1/workorch/workspaces/ws-priority-retired/execution-activations?priority=7", strings.NewReader(`{"intent":"resume","requestId":"request-query-header-priority"}`))
	queryAndHeader.Header.Set("Content-Type", "application/json")
	queryAndHeader.Header.Set("X-Workorch-Run-Priority", "8")
	queryAndHeaderRecorder := httptest.NewRecorder()
	handler.ServeHTTP(queryAndHeaderRecorder, queryAndHeader)
	if queryAndHeaderRecorder.Code != http.StatusAccepted || len(admissionPort.calls) != 1 {
		t.Fatalf("query/header priority status=%d owner calls=%d, want normal typed admission with no priority surface", queryAndHeaderRecorder.Code, len(admissionPort.calls))
	}
}

func (s *recordingWorkspaceExecutionSubmitter) SubmitWorkspaceRun(_ context.Context, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	s.commands = append(s.commands, command)
	return workorchdevents.WorkspaceRunRequestedResult{WorkspaceID: command.WorkspaceID, RequestID: command.RequestID, Kind: "resume", Outcome: "reconciliation_requested", Reason: "current_run_continuation_accepted"}, nil
}

func TestWorkorchDExecutionAdmissionConsumesTypedWorkspaceRunCommand(t *testing.T) {
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	submitter := &recordingWorkspaceExecutionSubmitter{}
	registerTestWorkspaceExecutionAdmission(t, rawHub, submitter)
	command := workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: "ws-typed-admission",
		Intent:      "resume",
		Source:      "test",
		RequestID:   "request-typed-admission",
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, "test", workorchdevents.CommandDestination, magicevent.NewHeader(), context.Background(), command)
	response := rawHub.Send(event)
	if response == nil {
		t.Fatal("execution admission response is nil")
	}
	if err := response.Error(); err != nil {
		t.Fatalf("execution admission response error = %v", err)
	}
	data, err := response.Get()
	if err != nil {
		t.Fatalf("execution admission result: %v", err)
	}
	result, ok := data.(workorchdevents.WorkspaceRunRequestedResult)
	if !ok || result.WorkspaceID != command.WorkspaceID || result.RequestID != command.RequestID {
		t.Fatalf("execution admission result = %#v, want accepted typed result", data)
	}

	if len(submitter.commands) != 1 || submitter.commands[0].WorkspaceID != command.WorkspaceID || submitter.commands[0].RequestID != command.RequestID {
		t.Fatalf("workspace intent ingress = %+v, want one immutable admission request", submitter.commands)
	}
}

func TestWorkorchDExecutionAdmissionRejectsInvalidTypedWorkspaceRunCommand(t *testing.T) {
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	state := newTestServerStateWithCommandHubAndBackground(normalizeConfig(Config{Backend: "memory"}), rawHub, &recordingBackgroundRoutine{})
	submitter := directWorkspaceExecutionSubmitter{state: state}
	registerTestWorkspaceExecutionAdmission(t, rawHub, submitter)
	event := magicevent.NewEventWithContext(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, "test", workorchdevents.CommandDestination, magicevent.NewHeader(), context.Background(), workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: common.WorkspaceID("ws-invalid-typed-admission"),
	})
	response := rawHub.Send(event)
	if response == nil || response.Error() == nil {
		t.Fatalf("execution admission response = %v, want invalid command error", response)
	}
}

// registerTestWorkspaceExecutionAdmission models the application Biz subscription
// that owns this command in production. It deliberately lives in the server
// test fixture so the HTTP adapter itself remains free of EventHub observers.
func registerTestWorkspaceExecutionAdmission(t *testing.T, hub magicevent.Hub, submitter testWorkspaceRunSubmitter) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(workorchdevents.CommandDestination, hub)
	observer.Subscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, func(event magicevent.Event, result magicevent.Result) {
		command, ok := event.Data().(workorchdevents.WorkspaceRunRequestedCommand)
		if !ok {
			if result != nil {
				result.Set(nil, cd.NewError(cd.IllegalParam, common.ErrInvalidArgument.Error()))
			}
			return
		}
		outcome, err := submitter.SubmitWorkspaceRun(event.Context(), command)
		if err != nil {
			if result != nil {
				result.Set(nil, cd.NewError(cd.Unexpected, err.Error()))
			}
			return
		}
		if result != nil {
			result.Set(outcome, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested) })
}

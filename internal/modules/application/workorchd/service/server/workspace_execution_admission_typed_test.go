package server

import (
	"context"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

type recordingWorkspaceExecutionSubmitter struct {
	commands []workorchdevents.WorkspaceRunRequestedCommand
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
		Priority:    7,
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

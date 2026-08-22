package server

import (
	"context"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

type recordingWorkspaceRunScheduler struct {
	commands []workorchdevents.WorkspaceRunRequestedCommand
}

func (s *recordingWorkspaceRunScheduler) SubmitWorkspaceRun(_ context.Context, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	s.commands = append(s.commands, command)
	return workorchdevents.WorkspaceRunRequestedResult{WorkspaceID: command.WorkspaceID, RequestID: command.RequestID, Kind: "resume", Outcome: "reconciliation_requested", Reason: "current_run_continuation_accepted"}, nil
}

func TestWorkorchDSchedulerConsumesTypedWorkspaceRunCommand(t *testing.T) {
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	scheduler := &recordingWorkspaceRunScheduler{}
	registerTestWorkspaceRunScheduler(t, rawHub, scheduler)
	command := workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: "ws-typed-scheduler",
		Intent:      "resume",
		Priority:    7,
		Source:      "test",
		RequestID:   "request-typed-scheduler",
	}
	event := magicevent.NewEventWithContext(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, "test", workorchdevents.CommandDestination, magicevent.NewHeader(), context.Background(), command)
	response := rawHub.Send(event)
	if response == nil {
		t.Fatal("scheduler response is nil")
	}
	if err := response.Error(); err != nil {
		t.Fatalf("scheduler response error = %v", err)
	}
	data, err := response.Get()
	if err != nil {
		t.Fatalf("scheduler result: %v", err)
	}
	result, ok := data.(workorchdevents.WorkspaceRunRequestedResult)
	if !ok || result.WorkspaceID != command.WorkspaceID || result.RequestID != command.RequestID {
		t.Fatalf("scheduler result = %#v, want accepted typed result", data)
	}

	if len(scheduler.commands) != 1 || scheduler.commands[0].WorkspaceID != command.WorkspaceID || scheduler.commands[0].RequestID != command.RequestID {
		t.Fatalf("workspace intent ingress = %+v, want one immutable scheduler request", scheduler.commands)
	}
}

func TestWorkorchDSchedulerRejectsInvalidTypedWorkspaceRunCommand(t *testing.T) {
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	state := newTestServerStateWithCommandHubAndBackground(normalizeConfig(Config{Backend: "memory"}), rawHub, &recordingBackgroundRoutine{})
	scheduler := directWorkspaceRunScheduler{state: state}
	registerTestWorkspaceRunScheduler(t, rawHub, scheduler)
	event := magicevent.NewEventWithContext(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, "test", workorchdevents.CommandDestination, magicevent.NewHeader(), context.Background(), workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: common.WorkspaceID("ws-invalid-typed-scheduler"),
	})
	response := rawHub.Send(event)
	if response == nil || response.Error() == nil {
		t.Fatalf("scheduler response = %v, want invalid command error", response)
	}
}

// registerTestWorkspaceRunScheduler models the application Biz subscription
// that owns this command in production. It deliberately lives in the server
// test fixture so the HTTP adapter itself remains free of EventHub observers.
func registerTestWorkspaceRunScheduler(t *testing.T, hub magicevent.Hub, scheduler testWorkspaceRunSubmitter) {
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
		outcome, err := scheduler.SubmitWorkspaceRun(event.Context(), command)
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

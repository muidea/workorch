package backend

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	port "github.com/muidea/workorch/internal/pkg/workorchport"
)

func TestEventHubRunBackendSendsTypedExecutionAdmissionCommand(t *testing.T) {
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	var captured workorchdevents.WorkspaceRunRequestedCommand
	var capturedLane string
	observer := magicevent.NewSimpleObserver(workorchdevents.CommandDestination, rawHub)
	observer.Subscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, func(ev magicevent.Event, result magicevent.Result) {
		captured, _ = ev.Data().(workorchdevents.WorkspaceRunRequestedCommand)
		capturedLane = ev.LaneKey()
		result.Set(workorchdevents.WorkspaceRunRequestedResult{WorkspaceID: "ws-typed-backend", RequestID: "request-typed-backend", Kind: "resume", Outcome: "reconciliation_requested", Reason: "current_run_continuation_accepted"}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested) })

	backend := NewEventHub(rawHub)
	_, err := backend.Run().Command(context.Background(), port.WorkspaceRunCommand{
		WorkspaceID: "ws-typed-backend",
		Intent:      port.RunCommandIntentResume,
		Reason:      "typed execution admission contract",
		RequestID:   "request-typed-backend",
		Source:      "test",
		Priority:    9,
	})
	if err != nil {
		t.Fatalf("run command: type=%T value=%#v text=%v", err, err, err)
	}
	if captured.WorkspaceID != "ws-typed-backend" || captured.Intent != "resume" || captured.Priority != 9 {
		t.Fatalf("captured command = %+v", captured)
	}
	if want := "state/workorchd.execution_admission/workspace/ws-typed-backend"; capturedLane != want {
		t.Fatalf("captured execution admission lane = %q, want %q", capturedLane, want)
	}
}

package backend

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
)

func TestEventHubRunBackendSendsTypedSchedulerCommand(t *testing.T) {
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	var captured workorchdevents.WorkspaceRunRequestedCommand
	observer := magicevent.NewSimpleObserver(workorchdevents.CommandDestination, rawHub)
	observer.Subscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, func(ev magicevent.Event, result magicevent.Result) {
		captured, _ = ev.Data().(workorchdevents.WorkspaceRunRequestedCommand)
		result.Set(workorchdevents.WorkspaceRunRequestedResult{Accepted: true}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested) })

	backend := NewEventHub(rawHub)
	_, err := backend.Run().Command(context.Background(), WorkspaceRunCommand{
		WorkspaceID: "ws-typed-backend",
		Intent:      RunCommandIntentResume,
		Reason:      "typed contract",
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
}

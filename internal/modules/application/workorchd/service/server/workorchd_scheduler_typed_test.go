package server

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestWorkorchDSchedulerConsumesTypedWorkspaceRunCommand(t *testing.T) {
	rawHub := magicevent.NewHub(16)
	hub := events.NewMagicHubWithHubOwner(rawHub, "workorchd/scheduler/test")
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	state := newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), hub, &recordingBackgroundRoutine{})
	command := workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: "ws-typed-scheduler",
		RunID:       "run-typed-scheduler",
		Intent:      "resume",
		Action:      "resume",
		Priority:    7,
		Source:      "test",
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
	if !ok || !result.Accepted {
		t.Fatalf("scheduler result = %#v, want accepted typed result", data)
	}

	state.runContinuations.runMu.Lock()
	pending, exists := state.runContinuations.runPending["ws-typed-scheduler/run-typed-scheduler/resume"]
	state.runContinuations.runMu.Unlock()
	if !exists || pending.Priority != 7 || pending.Action != "resume" {
		t.Fatalf("pending continuation = %+v exists=%v", pending, exists)
	}
}

func TestWorkorchDSchedulerRejectsInvalidTypedWorkspaceRunCommand(t *testing.T) {
	rawHub := magicevent.NewHub(16)
	hub := events.NewMagicHubWithHubOwner(rawHub, "workorchd/scheduler/test")
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	_ = newServerStateWithBackground(normalizeConfig(Config{Backend: "memory"}), hub, &recordingBackgroundRoutine{})
	event := magicevent.NewEventWithContext(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, "test", workorchdevents.CommandDestination, magicevent.NewHeader(), context.Background(), workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: common.WorkspaceID("ws-invalid-typed-scheduler"),
	})
	response := rawHub.Send(event)
	if response == nil || response.Error() == nil {
		t.Fatalf("scheduler response = %v, want invalid command error", response)
	}
}

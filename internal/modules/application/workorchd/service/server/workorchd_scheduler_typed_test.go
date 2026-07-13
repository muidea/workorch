package server

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkorchDSchedulerConsumesTypedWorkspaceRunCommand(t *testing.T) {
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	var captured orchestratorevents.ResumeRunCommand
	owner := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, rawHub)
	owner.Subscribe(orchestratorevents.TopicOrchestratorCommandResumeRun, func(event magicevent.Event, result magicevent.Result) {
		captured, _ = event.Data().(orchestratorevents.ResumeRunCommand)
		result.Set(orchestratorevents.OrchestratorCommandResult{}, nil)
	})
	t.Cleanup(func() { owner.Unsubscribe(orchestratorevents.TopicOrchestratorCommandResumeRun) })
	_ = newServerStateWithCommandHubAndBackground(normalizeConfig(Config{Backend: "memory"}), rawHub, &recordingBackgroundRoutine{})
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

	if captured.WorkspaceID != command.WorkspaceID || captured.RunID != command.RunID {
		t.Fatalf("orchestrator resume command = %+v, want direct typed Runtime V2 owner command", captured)
	}
}

func TestWorkorchDSchedulerRejectsInvalidTypedWorkspaceRunCommand(t *testing.T) {
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	_ = newServerStateWithCommandHubAndBackground(normalizeConfig(Config{Backend: "memory"}), rawHub, &recordingBackgroundRoutine{})
	event := magicevent.NewEventWithContext(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, "test", workorchdevents.CommandDestination, magicevent.NewHeader(), context.Background(), workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: common.WorkspaceID("ws-invalid-typed-scheduler"),
	})
	response := rawHub.Send(event)
	if response == nil || response.Error() == nil {
		t.Fatalf("scheduler response = %v, want invalid command error", response)
	}
}

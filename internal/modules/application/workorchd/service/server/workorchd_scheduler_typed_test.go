package server

import (
	"context"
	"testing"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

type recordedRuntimeDispatch struct {
	workspaceID string
	runID       string
	action      string
	causationID string
}

type recordingWorkspaceRunScheduler struct{ dispatches []recordedRuntimeDispatch }

func (s *recordingWorkspaceRunScheduler) SubmitWorkspaceRun(context.Context, workorchdevents.WorkspaceRunRequestedCommand) error {
	return nil
}

func (s *recordingWorkspaceRunScheduler) DispatchRuntimeRun(_ context.Context, workspaceID, runID, action, causationID string) error {
	s.dispatches = append(s.dispatches, recordedRuntimeDispatch{workspaceID: workspaceID, runID: runID, action: action, causationID: causationID})
	return nil
}

func TestWorkorchDSchedulerConsumesTypedWorkspaceRunCommand(t *testing.T) {
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	var captured runevents.ContinueRunCommand
	owner := magicevent.NewSimpleObserver(runevents.CommandDestination, rawHub)
	owner.Subscribe(runevents.TopicRunCommandContinue, func(event magicevent.Event, result magicevent.Result) {
		captured, _ = event.Data().(runevents.ContinueRunCommand)
		result.Set(runevents.RunLifecycleCommandResult{}, nil)
	})
	t.Cleanup(func() { owner.Unsubscribe(runevents.TopicRunCommandContinue) })
	state := newTestServerStateWithCommandHubAndBackground(normalizeConfig(Config{Backend: "memory"}), rawHub, &recordingBackgroundRoutine{})
	scheduler := directWorkspaceRunScheduler{state: state}
	state.workspaceRunScheduler = scheduler
	registerTestWorkspaceRunScheduler(t, rawHub, scheduler)
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

	if captured.WorkspaceID != command.WorkspaceID || captured.RunID != command.RunID || captured.CausationID == "" {
		t.Fatalf("orchestrator resume command = %+v, want direct typed Runtime V2 owner command", captured)
	}
}

func TestWorkorchDSchedulerRejectsInvalidTypedWorkspaceRunCommand(t *testing.T) {
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	state := newTestServerStateWithCommandHubAndBackground(normalizeConfig(Config{Backend: "memory"}), rawHub, &recordingBackgroundRoutine{})
	scheduler := directWorkspaceRunScheduler{state: state}
	state.workspaceRunScheduler = scheduler
	registerTestWorkspaceRunScheduler(t, rawHub, scheduler)
	event := magicevent.NewEventWithContext(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested, "test", workorchdevents.CommandDestination, magicevent.NewHeader(), context.Background(), workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: common.WorkspaceID("ws-invalid-typed-scheduler"),
	})
	response := rawHub.Send(event)
	if response == nil || response.Error() == nil {
		t.Fatalf("scheduler response = %v, want invalid command error", response)
	}
}

func TestWorkspaceExecutionActivationDispatchesRuntimeV2ResumeForActiveRun(t *testing.T) {
	rawHub := magicevent.NewHub(16)
	t.Cleanup(func() { rawHub.Terminate(context.Background()) })
	state := newTestServerStateWithCommandHubAndBackground(normalizeConfig(Config{Backend: "memory"}), rawHub, &recordingBackgroundRoutine{})
	scheduler := &recordingWorkspaceRunScheduler{}
	state.workspaceRunScheduler = scheduler
	state.resumeWorkspaceExecution = func(ctx context.Context, _, _, workspaceID string, _ appservice.ActivateWorkspaceExecutionCommand) (appservice.WorkspaceExecutionActivation, error) {
		if got := common.RuntimeWorkspaceIDFromContext(ctx); got != common.WorkspaceID(workspaceID) {
			t.Fatalf("activation workspace scope = %q, want %q", got, workspaceID)
		}
		return appservice.WorkspaceExecutionActivation{
			Run: appservice.Run{ID: "run-active", WorkspaceID: workspaceID, State: string(common.RunStateActive)},
		}, nil
	}

	err := state.executeWorkspaceRuntimeV2Activation(context.Background(), workspaceRunRequest{
		WorkspaceID: "ws-active",
		Intent:      "resume",
		RequestID:   "request-active",
	})
	if err != nil {
		t.Fatalf("executeWorkspaceRuntimeV2Activation: %v", err)
	}
	if len(scheduler.dispatches) != 1 {
		t.Fatalf("runtime dispatches = %+v, want one resume", scheduler.dispatches)
	}
	got := scheduler.dispatches[0]
	if got.workspaceID != "ws-active" || got.runID != "run-active" || got.action != "resume" || got.causationID != "workspace-run/resume/request-active" {
		t.Fatalf("runtime dispatch = %+v, want ws-active/run-active/resume", got)
	}
}

// registerTestWorkspaceRunScheduler models the application Biz subscription
// that owns this command in production. It deliberately lives in the server
// test fixture so the HTTP adapter itself remains free of EventHub observers.
func registerTestWorkspaceRunScheduler(t *testing.T, hub magicevent.Hub, scheduler WorkspaceRunScheduler) {
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
		if err := scheduler.SubmitWorkspaceRun(event.Context(), command); err != nil {
			if result != nil {
				result.Set(nil, cd.NewError(cd.Unexpected, err.Error()))
			}
			return
		}
		if result != nil {
			result.Set(workorchdevents.WorkspaceRunRequestedResult{Accepted: true}, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(workorchdevents.TopicWorkorchDCommandWorkspaceRunRequested) })
}

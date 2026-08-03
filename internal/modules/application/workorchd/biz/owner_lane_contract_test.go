package biz

import (
	"context"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestSubmitWorkspaceRunBindsWorkspaceOwnerLane(t *testing.T) {
	hub := magicevent.NewHub(8)
	background := magictask.NewBackgroundRoutine(8)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	t.Cleanup(func() {
		hub.Terminate(ctx)
		background.Shutdown(ctx)
	})

	workspaceID := common.WorkspaceID("ws-lane-owner")
	requestID := "resume-lane-owner"
	observer := magicevent.NewSimpleObserver(workspaceevents.CommandDestination, hub)
	observer.Subscribe(workspaceevents.TopicWorkspaceCommandExecutionIntent, func(event magicevent.Event, result magicevent.Result) {
		if got, want := event.LaneKey(), "state/workspace/"+string(workspaceID); got != want {
			t.Errorf("execution intent lane = %q, want %q", got, want)
		}
		command, ok := event.Data().(workspaceevents.WorkspaceExecutionIntentCommand)
		if !ok {
			t.Errorf("execution intent payload type = %T", event.Data())
			return
		}
		result.Set(workspaceevents.WorkspaceExecutionIntentResult{
			WorkspaceID: command.WorkspaceID,
			RunID:       "run-lane-owner",
			RequestID:   command.RequestID,
			Kind:        command.Kind,
			Outcome:     runtimecontract.RuntimeExecutionIntentReconciliationRequested,
			Reason:      runtimecontract.RuntimeExecutionIntentReasonCurrentRunContinuationAccepted,
		}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(workspaceevents.TopicWorkspaceCommandExecutionIntent) })

	b := &Biz{
		Base:    basebiz.New(ModuleID, hub, background),
		session: &appservice.RuntimeSession{},
	}
	got, err := b.SubmitWorkspaceRun(ctx, workorchdevents.WorkspaceRunRequestedCommand{
		WorkspaceID: workspaceID,
		RequestID:   requestID,
		Intent:      "resume",
	})
	if err != nil {
		t.Fatalf("SubmitWorkspaceRun: %v", err)
	}
	if got.WorkspaceID != workspaceID || got.RequestID != requestID || got.Kind != string(workspaceevents.WorkspaceExecutionIntentResume) {
		t.Fatalf("execution intent result = %+v", got)
	}
}

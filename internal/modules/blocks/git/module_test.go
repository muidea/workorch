package git

import (
	"context"
	"testing"

	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/blocks/pkg/runtimeevent"
	"github.com/muidea/workorch/internal/pkg/events"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestGitModuleQueuesRuntimeExecution(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMagicHub()
	defer hub.Terminate(ctx)
	background := magictask.NewBackgroundRoutine(2)
	defer background.Shutdown(ctx)

	module := New()
	if err := module.Configure(Dependencies{EventHub: hub, Background: background}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer module.Stop(ctx)

	err := hub.Publish(ctx, events.Envelope{
		Topic:       runtimeevent.RuntimeBlockExecuteTopic(StepKindRead),
		WorkspaceID: "ws-git-queue",
		RunID:       "run-git-queue",
		StepID:      "run-git-queue/step-1",
		Payload: map[string]any{
			events.PayloadKeyRuntimeRequestID: "git-queue-request",
			events.PayloadKeyRuntimeRequest: runtimemodel.StepExecutionRequest{
				WorkspaceID:                   "ws-git-queue",
				RunID:                         "run-git-queue",
				StepID:                        "run-git-queue/step-1",
				Kind:                          StepKindRead,
				WorkspaceDeliverableDirectory: t.TempDir(),
				Input:                         map[string]any{"operation": "status"},
			},
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("runtime command err = %v, want accepted response", err)
	}
	if _, ok := response.Payload[events.PayloadKeyRuntimeAccepted].(runtimemodel.RuntimeStepExecutionAccepted); !ok {
		t.Fatalf("accepted payload missing: %#v", response.Payload)
	}
}

package file

import (
	"context"
	"testing"

	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/blocks/pkg/runtimeevent"
	"github.com/muidea/workorch/internal/pkg/events"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFileModuleQueuesRuntimeExecution(t *testing.T) {
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
		Topic:       runtimeevent.RuntimeBlockExecuteTopic(StepKindList),
		WorkspaceID: "ws-file-queue",
		RunID:       "run-file-queue",
		StepID:      "run-file-queue/step-1",
		Payload: map[string]any{
			events.PayloadKeyRuntimeRequestID: "file-queue-request",
			events.PayloadKeyRuntimeRequest: runtimemodel.StepExecutionRequest{
				WorkspaceID:                   "ws-file-queue",
				RunID:                         "run-file-queue",
				StepID:                        "run-file-queue/step-1",
				Kind:                          StepKindList,
				WorkspaceDeliverableDirectory: t.TempDir(),
				Input:                         map[string]any{"path": "."},
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

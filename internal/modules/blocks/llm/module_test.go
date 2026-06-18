package llm

import (
	"context"
	"testing"

	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/blocks/pkg/runtimeevent"
	"github.com/muidea/workorch/internal/pkg/events"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestLLMModuleQueuesRuntimeExecution(t *testing.T) {
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
		Topic:       runtimeevent.RuntimeBlockExecuteTopic(StepKindWorkspaceExecution),
		WorkspaceID: "ws-llm-queue",
		RunID:       "run-llm-queue",
		StepID:      "run-llm-queue/step-1",
		Payload: map[string]any{
			events.PayloadKeyRuntimeRequestID: "llm-queue-request",
			events.PayloadKeyRuntimeRequest: runtimemodel.StepExecutionRequest{
				WorkspaceID: "ws-llm-queue",
				RunID:       "run-llm-queue",
				StepID:      "run-llm-queue/step-1",
				Kind:        StepKindWorkspaceExecution,
				Input:       map[string]any{"workspaceId": "ws-llm-queue"},
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

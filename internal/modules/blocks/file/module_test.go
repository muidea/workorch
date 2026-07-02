package file

import (
	"context"
	"errors"
	"testing"

	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/runtimeevent"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestFileModuleIdentityConfigureStartStopContract(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMagicHub()
	defer hub.Terminate(ctx)
	background := magictask.NewBackgroundRoutine(1)
	defer background.Shutdown(ctx)

	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}
	if err := module.Configure(Dependencies{EventHub: hub, Background: background}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.runtimeAdapter == nil || module.eventHub == nil || module.background == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	firstExecutor := module.executor
	if firstExecutor == nil {
		t.Fatal("executor is nil after Start")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if module.executor != firstExecutor {
		t.Fatal("second Start replaced executor")
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.executor != nil ||
		module.runtimeAdapter != nil ||
		module.eventHub != nil ||
		module.background != nil {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestFileModuleStartRequiresRuntimeDependencies(t *testing.T) {
	if err := New().Configure(); err != nil {
		t.Fatalf("Configure without dependencies: %v", err)
	}
	if err := New().Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start unconfigured err = %v, want invalid argument", err)
	}
	module := New()
	if err := module.Configure(Dependencies{}); err != nil {
		t.Fatalf("Configure with empty dependencies: %v", err)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start without EventHub/Background err = %v, want invalid argument", err)
	}
}

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

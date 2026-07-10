package mcp

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/runtimeevent"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMCPModuleIdentityConfigureStartStopContract(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	defer rawHub.Terminate(ctx)
	background := magictask.NewBackgroundRoutine(1)
	defer background.Shutdown(ctx)

	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if err := module.Configure(Dependencies{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Configure without EventHub err = %v, want invalid argument", err)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}
	if err := module.Configure(Dependencies{EventHub: rawHub, Background: background}); err != nil {
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

func TestMCPModuleStartRequiresBackgroundRoutine(t *testing.T) {
	module := New()
	if err := module.Configure(Dependencies{EventHub: magicevent.NewHub(16)}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start without Background err = %v, want invalid argument", err)
	}
}

func TestMCPModuleQueuesRuntimeExecution(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	defer rawHub.Terminate(ctx)
	background := magictask.NewBackgroundRoutine(2)
	defer background.Shutdown(ctx)

	module := New()
	if err := module.Configure(Dependencies{EventHub: rawHub, Background: background}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer module.Stop(ctx)

	result := rawHub.Send(magicevent.NewEventWithContext(
		runtimeevent.RuntimeBlockExecuteTopic(StepKindToolCall), ModuleID, runtimeevent.CommandDestination, magicevent.NewHeader(), ctx,
		runtimeevent.ExecuteCommand{RequestID: "mcp-queue-request", Request: runtimemodel.StepExecutionRequest{
			WorkspaceID: "ws-mcp-queue",
			RunID:       "run-mcp-queue",
			StepID:      "run-mcp-queue/step-1",
			Kind:        StepKindToolCall,
			Input:       map[string]any{"serverId": "missing", "name": "missing.tool"},
		}},
	))
	if err := result.Error(); err != nil {
		t.Fatalf("runtime command err = %v", err)
	}
	data, err := result.Get()
	if _, ok := data.(runtimeevent.ExecuteCommandResult); err != nil || !ok {
		t.Fatalf("accepted result = %T, err=%v", data, err)
	}
}

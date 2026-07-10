package llm

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

func TestLLMModuleIdentityConfigureStartStopContract(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	defer rawHub.Terminate(ctx)
	background := magictask.NewBackgroundRoutine(1)
	defer background.Shutdown(ctx)

	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
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
	firstSubscriptionCount := len(module.topics)
	if firstExecutor == nil || module.observer == nil || firstSubscriptionCount == 0 {
		t.Fatalf("executor=%v observer=%v subscriptions=%d, want runtime executor and provider command subscriptions", firstExecutor != nil, module.observer != nil, firstSubscriptionCount)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if module.executor != firstExecutor || len(module.topics) != firstSubscriptionCount {
		t.Fatalf("second Start changed executor/subscriptions: executor changed=%v subscriptions=%d want %d", module.executor != firstExecutor, len(module.topics), firstSubscriptionCount)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.executor != nil ||
		module.runtimeAdapter != nil ||
		module.eventHub != nil ||
		module.rawEventHub != nil ||
		module.background != nil ||
		module.observer != nil ||
		len(module.topics) != 0 {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestLLMModuleStartRequiresRuntimeDependencies(t *testing.T) {
	module := New()
	if err := module.Configure(Dependencies{}); err != nil {
		t.Fatalf("Configure with empty dependencies: %v", err)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start without EventHub/Background err = %v, want invalid argument", err)
	}
}

func TestLLMModuleQueuesRuntimeExecution(t *testing.T) {
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
		runtimeevent.RuntimeBlockExecuteTopic(StepKindWorkspaceExecution), ModuleID, runtimeevent.CommandDestination, magicevent.NewHeader(), ctx,
		runtimeevent.ExecuteCommand{RequestID: "llm-queue-request", Request: runtimemodel.StepExecutionRequest{
			WorkspaceID: "ws-llm-queue",
			RunID:       "run-llm-queue",
			StepID:      "run-llm-queue/step-1",
			Kind:        StepKindWorkspaceExecution,
			Input:       map[string]any{"workspaceId": "ws-llm-queue"},
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

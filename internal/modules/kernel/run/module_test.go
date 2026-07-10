package run

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func newRunModuleTestHub() (*events.MagicHub, magicevent.Hub) {
	rawHub := magicevent.NewHub(16)
	return events.NewMagicHubWithHub(rawHub), rawHub
}

func sendRunTestCommand[Command any](ctx context.Context, rawHub magicevent.Hub, topic string, command Command) (runevents.RunCommandResult, error) {
	ev := magicevent.NewEventWithContext(topic, "run/module_test", runevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	result := rawHub.Send(ev)
	if result == nil {
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
	}
	if err := result.Error(); err != nil {
		if err.Cause != nil {
			return runevents.RunCommandResult{}, err.Cause
		}
		return runevents.RunCommandResult{}, err
	}
	data, err := result.Get()
	if err != nil {
		return runevents.RunCommandResult{}, err
	}
	response, ok := data.(runevents.RunCommandResult)
	if !ok {
		return runevents.RunCommandResult{}, common.ErrInvalidArgument
	}
	return response, nil
}

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if module.servicePtr != nil {
		t.Fatal("new module service pointer is non-nil before Configure")
	}
	_, rawHub := newRunModuleTestHub()
	if err := module.Configure(Dependencies{
		Runs:        memory.NewRunRepository(),
		Steps:       memory.NewStepRepository(),
		Checkpoints: memory.NewRuntimeCheckpointRepository(),
		EventHub:    rawHub,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.servicePtr == nil {
		t.Fatal("service pointer is nil after Configure")
	}
	if module.eventHub == nil {
		t.Fatal("event hub is nil after Configure")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	firstSubscriptionCount := len(module.topics)
	if firstSubscriptionCount == 0 {
		t.Fatal("Start did not subscribe run commands")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if len(module.topics) != firstSubscriptionCount {
		t.Fatalf("second Start subscription count = %d, want %d", len(module.topics), firstSubscriptionCount)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.servicePtr != nil ||
		module.bizPtr != nil ||
		module.eventHub != nil ||
		module.runs != nil ||
		module.steps != nil ||
		module.checkpoints != nil ||
		module.observer != nil ||
		len(module.topics) != 0 {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModuleConfigureRequiresRunRepositoryAndEventHub(t *testing.T) {
	tests := []struct {
		name string
		deps Dependencies
	}{
		{name: "missing runs", deps: Dependencies{EventHub: magicevent.NewHub(16)}},
		{name: "missing event hub", deps: Dependencies{Runs: memory.NewRunRepository()}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := New().Configure(tc.deps); !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("Configure err = %v, want invalid argument", err)
			}
		})
	}
}

func TestRunCommandGetHonorsWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	_, rawHub := newRunModuleTestHub()
	runs := memory.NewRunRepository()
	runID := common.RunID("run-scoped-get")
	if err := runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-run-owner-a",
		State:       common.RunStatePending,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-run-owner-b",
		State:       common.RunStateActive,
	}); err != nil {
		t.Fatalf("create duplicate scoped run: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Runs: runs, EventHub: rawHub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	_, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandGet, runevents.GetRunCommand{
		WorkspaceID: "ws-run-owner-c",
		RunID:       runID,
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace get err = %v, want not found", err)
	}

	response, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandGet, runevents.GetRunCommand{
		WorkspaceID: "ws-run-owner-b",
		RunID:       runID,
	})
	if err != nil {
		t.Fatalf("duplicate scoped get err = %v, want response", err)
	}
	item := response.Run
	if item == nil || item.WorkspaceID != "ws-run-owner-b" || item.State != common.RunStateActive {
		t.Fatalf("duplicate scoped response run = %+v, want workspace b running run", response.Run)
	}

	response, err = sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandGet, runevents.GetRunCommand{
		WorkspaceID: "ws-run-owner-a",
		RunID:       runID,
	})
	if err != nil {
		t.Fatalf("scoped get err = %v, want response", err)
	}
	item = response.Run
	if item == nil || item.WorkspaceID != "ws-run-owner-a" || item.ID != runID {
		t.Fatalf("response run = %+v, want scoped run", response.Run)
	}
}

func TestRunCommandListSupportsCompactStateFilter(t *testing.T) {
	ctx := context.Background()
	_, rawHub := newRunModuleTestHub()
	runs := memory.NewRunRepository()
	if err := runs.Create(ctx, &runmodel.Run{
		ID:               "run-compact-running",
		WorkspaceID:      "ws-run-compact",
		State:            common.RunStateActive,
		ExecutionContext: map[string]any{"large": "context"},
		Priority:         3,
	}); err != nil {
		t.Fatalf("create running run: %v", err)
	}
	if err := runs.Create(ctx, &runmodel.Run{
		ID:               "run-compact-completed",
		WorkspaceID:      "ws-run-compact",
		State:            common.RunStateCompleted,
		ExecutionContext: map[string]any{"large": "context"},
	}); err != nil {
		t.Fatalf("create completed run: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Runs: runs, EventHub: rawHub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	response, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandList, runevents.ListRunsCommand{
		States:  []common.RunState{common.RunStateActive},
		Compact: true,
	})
	if err != nil {
		t.Fatalf("list err = %v, want response", err)
	}
	items := response.Runs
	if len(items) != 1 || items[0].ID != "run-compact-running" {
		t.Fatalf("items = %+v, want only running run", items)
	}
	if items[0].ExecutionContext != nil || items[0].Priority != 3 || items[0].WorkspaceID != "ws-run-compact" {
		t.Fatalf("compact item = %+v, want metadata without execution context", items[0])
	}
}

func TestRunCommandDeleteHonorsWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	_, rawHub := newRunModuleTestHub()
	runs := memory.NewRunRepository()
	runID := common.RunID("run-scoped-delete")
	if err := runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-run-delete-a",
		State:       common.RunStatePending,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Runs: runs, EventHub: rawHub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	_, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandDelete, runevents.DeleteRunCommand{
		WorkspaceID: "ws-run-delete-b",
		RunID:       runID,
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace delete err = %v, want not found", err)
	}
	if _, err := runs.Get(ctx, runID); err != nil {
		t.Fatalf("run deleted on wrong workspace: %v", err)
	}

	_, err = sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandDelete, runevents.DeleteRunCommand{
		WorkspaceID: "ws-run-delete-a",
		RunID:       runID,
	})
	if err != nil {
		t.Fatalf("scoped delete err = %v, want response", err)
	}
	if _, err := runs.Get(ctx, runID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("run still exists after scoped delete err = %v", err)
	}
}

func TestRunWriteCommandsRequireWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	_, rawHub := newRunModuleTestHub()
	runs := memory.NewRunRepository()
	runID := common.RunID("run-write-scope")
	if err := runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-write-scope-a",
		State:       common.RunStatePending,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Runs: runs, EventHub: rawHub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	_, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandUpdate, runevents.UpdateRunCommand{
		RunID: runID,
		State: common.RunStateActive,
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unscoped update err = %v, want invalid argument", err)
	}

	_, err = sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandDelete, runevents.DeleteRunCommand{
		RunID: runID,
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unscoped delete err = %v, want invalid argument", err)
	}
}

func TestRunCreateCommandRequiresWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	_, rawHub := newRunModuleTestHub()
	runs := memory.NewRunRepository()
	module := New()
	if err := module.Configure(Dependencies{Runs: runs, EventHub: rawHub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	_, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandCreate, runevents.CreateRunCommand{
		RunID: "run-create-scope",
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("create missing workspace err = %v, want invalid argument", err)
	}
}

func TestRunStepReadCommandsHonorWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	_, rawHub := newRunModuleTestHub()
	runs := memory.NewRunRepository()
	steps := memory.NewStepRepository()
	runID := common.RunID("run-step-read-scope")
	stepID := common.StepID("step-read-scope")
	wsCtx := common.ContextWithRuntimeWorkspaceID(ctx, "ws-step-read-a")
	if err := runs.Create(wsCtx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-step-read-a",
		State:       common.RunStateActive,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := steps.Create(wsCtx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "llm/workspace_execution",
		State: common.StepStateExecuting,
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Runs: runs, Steps: steps, EventHub: rawHub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	_, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandGetStep, runevents.GetStepCommand{
		WorkspaceID: "ws-step-read-b",
		StepID:      stepID,
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace get step err = %v, want not found", err)
	}

	_, err = sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandListStepsByRun, runevents.ListStepsByRunCommand{
		WorkspaceID: "ws-step-read-b",
		RunID:       runID,
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace list steps err = %v, want not found", err)
	}

	response, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandGetStep, runevents.GetStepCommand{
		WorkspaceID: "ws-step-read-a",
		StepID:      stepID,
	})
	if err != nil {
		t.Fatalf("scoped get step err = %v, want response", err)
	}
	item := response.Step
	if item == nil || item.ID != stepID || item.RunID != runID {
		t.Fatalf("response step = %+v, want scoped step", response.Step)
	}
}

func TestRunCheckpointReadCommandsHonorWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	_, rawHub := newRunModuleTestHub()
	runs := memory.NewRunRepository()
	checkpoints := memory.NewRuntimeCheckpointRepository()
	runID := common.RunID("run-checkpoint-read-scope")
	checkpointID := common.CheckpointID("checkpoint-read-scope")
	if err := runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-checkpoint-read-a",
		State:       common.RunStateActive,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := checkpoints.Create(ctx, &runmodel.RuntimeCheckpoint{
		ID:          checkpointID,
		WorkspaceID: "ws-checkpoint-read-a",
		RunID:       runID,
	}); err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Runs: runs, Checkpoints: checkpoints, EventHub: rawHub}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(ctx); err != nil {
			t.Fatalf("Teardown: %v", err)
		}
	})

	_, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandGetRuntimeCheckpoint, runevents.GetRuntimeCheckpointCommand{
		WorkspaceID:  "ws-checkpoint-read-b",
		CheckpointID: checkpointID,
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace get checkpoint err = %v, want not found", err)
	}

	_, err = sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandListCheckpointsByRun, runevents.ListRuntimeCheckpointsByRunCommand{
		WorkspaceID: "ws-checkpoint-read-b",
		RunID:       runID,
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace list checkpoints err = %v, want not found", err)
	}

	response, err := sendRunTestCommand(ctx, rawHub, runevents.TopicRunCommandGetRuntimeCheckpoint, runevents.GetRuntimeCheckpointCommand{
		WorkspaceID:  "ws-checkpoint-read-a",
		CheckpointID: checkpointID,
	})
	if err != nil {
		t.Fatalf("scoped get checkpoint err = %v, want response", err)
	}
	item := response.Checkpoint
	if item == nil || item.ID != checkpointID || item.RunID != runID || item.WorkspaceID != "ws-checkpoint-read-a" {
		t.Fatalf("response checkpoint = %+v, want scoped checkpoint", response.Checkpoint)
	}
}

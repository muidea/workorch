package run

import (
	"context"
	"errors"
	"testing"

	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestRunCommandGetHonorsWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	runs := memory.NewRunRepository()
	runID := common.RunID("run-scoped-get")
	if err := runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-run-owner-a",
		State:       common.RunStateCreated,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Runs: runs, EventHub: hub}); err != nil {
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

	err := hub.Publish(ctx, events.Envelope{
		Topic:       runevents.TopicRunCommandGet,
		WorkspaceID: "ws-run-owner-b",
		RunID:       string(runID),
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: "ws-run-owner-b",
			events.PayloadKeyRunID:       string(runID),
		},
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace get err = %v, want not found", err)
	}

	err = hub.Publish(ctx, events.Envelope{
		Topic:       runevents.TopicRunCommandGet,
		WorkspaceID: "ws-run-owner-a",
		RunID:       string(runID),
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: "ws-run-owner-a",
			events.PayloadKeyRunID:       string(runID),
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("scoped get err = %v, want response", err)
	}
	item, ok := response.Payload[events.PayloadKeyRun].(*runmodel.Run)
	if !ok || item == nil || item.WorkspaceID != "ws-run-owner-a" || item.ID != runID {
		t.Fatalf("response run = %+v, want scoped run", response.Payload[events.PayloadKeyRun])
	}
}

func TestRunCommandDeleteHonorsWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	runs := memory.NewRunRepository()
	runID := common.RunID("run-scoped-delete")
	if err := runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-run-delete-a",
		State:       common.RunStateCreated,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Runs: runs, EventHub: hub}); err != nil {
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

	err := hub.Publish(ctx, events.Envelope{
		Topic:       runevents.TopicRunCommandDelete,
		WorkspaceID: "ws-run-delete-b",
		RunID:       string(runID),
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: "ws-run-delete-b",
			events.PayloadKeyRunID:       string(runID),
		},
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace delete err = %v, want not found", err)
	}
	if _, err := runs.Get(ctx, runID); err != nil {
		t.Fatalf("run deleted on wrong workspace: %v", err)
	}

	err = hub.Publish(ctx, events.Envelope{
		Topic:       runevents.TopicRunCommandDelete,
		WorkspaceID: "ws-run-delete-a",
		RunID:       string(runID),
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: "ws-run-delete-a",
			events.PayloadKeyRunID:       string(runID),
		},
	})
	if _, ok := events.ResponseEnvelope(err); !ok {
		t.Fatalf("scoped delete err = %v, want response", err)
	}
	if _, err := runs.Get(ctx, runID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("run still exists after scoped delete err = %v", err)
	}
}

func TestRunWriteCommandsRequireWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	runs := memory.NewRunRepository()
	runID := common.RunID("run-write-scope")
	if err := runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-write-scope-a",
		State:       common.RunStateCreated,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Runs: runs, EventHub: hub}); err != nil {
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

	err := hub.Publish(ctx, events.Envelope{
		Topic: runevents.TopicRunCommandUpdate,
		RunID: string(runID),
		Payload: map[string]any{
			events.PayloadKeyCommand: runcontracts.UpdateRunCommand{
				RunID:       runID,
				WorkspaceID: "ws-write-scope-a",
				State:       common.RunStateRunning,
			},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unscoped update err = %v, want invalid argument", err)
	}

	err = hub.Publish(ctx, events.Envelope{
		Topic:       runevents.TopicRunCommandUpdate,
		WorkspaceID: "ws-write-scope-b",
		RunID:       string(runID),
		Payload: map[string]any{
			events.PayloadKeyCommand: runcontracts.UpdateRunCommand{
				RunID:       runID,
				WorkspaceID: "ws-write-scope-a",
				State:       common.RunStateRunning,
			},
		},
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("wrong scope update err = %v, want conflict", err)
	}

	err = hub.Publish(ctx, events.Envelope{
		Topic: runevents.TopicRunCommandDelete,
		RunID: string(runID),
		Payload: map[string]any{
			events.PayloadKeyCommand: runcontracts.DeleteRunCommand{RunID: runID},
		},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("unscoped delete err = %v, want invalid argument", err)
	}
}

func TestRunCreateCommandRejectsWorkspaceScopeMismatch(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	runs := memory.NewRunRepository()
	module := New()
	if err := module.Configure(Dependencies{Runs: runs, EventHub: hub}); err != nil {
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

	err := hub.Publish(ctx, events.Envelope{
		Topic:       runevents.TopicRunCommandCreate,
		WorkspaceID: "ws-create-scope-envelope",
		RunID:       "run-create-scope",
		Payload: map[string]any{
			events.PayloadKeyCommand: runcontracts.CreateRunCommand{
				RunID:       "run-create-scope",
				WorkspaceID: "ws-create-scope-command",
			},
		},
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("create scope mismatch err = %v, want conflict", err)
	}
}

func TestRunStepReadCommandsHonorWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	runs := memory.NewRunRepository()
	steps := memory.NewStepRepository()
	runID := common.RunID("run-step-read-scope")
	stepID := common.StepID("step-read-scope")
	wsCtx := common.ContextWithRuntimeWorkspaceID(ctx, "ws-step-read-a")
	if err := runs.Create(wsCtx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-step-read-a",
		State:       common.RunStateRunning,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := steps.Create(wsCtx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  "llm/workspace_execution",
		State: common.StepStateDispatched,
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}

	module := New()
	if err := module.Configure(Dependencies{Runs: runs, Steps: steps, EventHub: hub}); err != nil {
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

	err := hub.Publish(ctx, events.Envelope{
		Topic:       runevents.TopicRunCommandGetStep,
		WorkspaceID: "ws-step-read-b",
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: "ws-step-read-b",
			events.PayloadKeyStepID:      string(stepID),
		},
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace get step err = %v, want not found", err)
	}

	err = hub.Publish(ctx, events.Envelope{
		Topic:       runevents.TopicRunCommandListStepsByRun,
		WorkspaceID: "ws-step-read-b",
		RunID:       string(runID),
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: "ws-step-read-b",
			events.PayloadKeyRunID:       string(runID),
		},
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace list steps err = %v, want not found", err)
	}

	err = hub.Publish(ctx, events.Envelope{
		Topic:       runevents.TopicRunCommandGetStep,
		WorkspaceID: "ws-step-read-a",
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: "ws-step-read-a",
			events.PayloadKeyStepID:      string(stepID),
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("scoped get step err = %v, want response", err)
	}
	item, ok := response.Payload[events.PayloadKeyStep].(*runmodel.Step)
	if !ok || item == nil || item.ID != stepID || item.RunID != runID {
		t.Fatalf("response step = %+v, want scoped step", response.Payload[events.PayloadKeyStep])
	}
}

func TestRunCheckpointReadCommandsHonorWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	runs := memory.NewRunRepository()
	checkpoints := memory.NewRuntimeCheckpointRepository()
	runID := common.RunID("run-checkpoint-read-scope")
	checkpointID := common.CheckpointID("checkpoint-read-scope")
	if err := runs.Create(ctx, &runmodel.Run{
		ID:          runID,
		WorkspaceID: "ws-checkpoint-read-a",
		State:       common.RunStateRunning,
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
	if err := module.Configure(Dependencies{Runs: runs, Checkpoints: checkpoints, EventHub: hub}); err != nil {
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

	err := hub.Publish(ctx, events.Envelope{
		Topic:       runevents.TopicRunCommandGetRuntimeCheckpoint,
		WorkspaceID: "ws-checkpoint-read-b",
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID:  "ws-checkpoint-read-b",
			events.PayloadKeyCheckpointID: string(checkpointID),
		},
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace get checkpoint err = %v, want not found", err)
	}

	err = hub.Publish(ctx, events.Envelope{
		Topic:       runevents.TopicRunCommandListCheckpointsByRun,
		WorkspaceID: "ws-checkpoint-read-b",
		RunID:       string(runID),
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID: "ws-checkpoint-read-b",
			events.PayloadKeyRunID:       string(runID),
		},
	})
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("wrong workspace list checkpoints err = %v, want not found", err)
	}

	err = hub.Publish(ctx, events.Envelope{
		Topic:       runevents.TopicRunCommandGetRuntimeCheckpoint,
		WorkspaceID: "ws-checkpoint-read-a",
		Payload: map[string]any{
			events.PayloadKeyWorkspaceID:  "ws-checkpoint-read-a",
			events.PayloadKeyCheckpointID: string(checkpointID),
		},
	})
	response, ok := events.ResponseEnvelope(err)
	if !ok {
		t.Fatalf("scoped get checkpoint err = %v, want response", err)
	}
	item, ok := response.Payload[events.PayloadKeyCheckpoint].(*runmodel.RuntimeCheckpoint)
	if !ok || item == nil || item.ID != checkpointID || item.RunID != runID || item.WorkspaceID != "ws-checkpoint-read-a" {
		t.Fatalf("response checkpoint = %+v, want scoped checkpoint", response.Payload[events.PayloadKeyCheckpoint])
	}
}

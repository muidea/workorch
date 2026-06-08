package run

import (
	"context"
	"errors"
	"testing"

	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

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

package workspace

import (
	"context"
	"errors"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	workspaceservice "github.com/muidea/workorch/internal/modules/kernel/workspace/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestGoalDoDManager_CreateWorkspace(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	manager := NewGoalDoDManager(repo, nil)

	cmd := workspaceservice.CreateWorkspaceCommand{
		WorkspaceID: common.WorkspaceID("ws-1"),
		Title:       "phase1 workspace",
	}
	if err := manager.CreateWorkspace(ctx, cmd); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	ws, err := repo.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.Title != cmd.Title {
		t.Fatalf("title = %q, want %q", ws.Title, cmd.Title)
	}
	if ws.State != common.WorkspaceStateActive {
		t.Fatalf("state = %q, want active", ws.State)
	}
}

func TestGoalDoDManager_CreateWorkspace_ValidateArguments(t *testing.T) {
	ctx := context.Background()
	manager := NewGoalDoDManager(memory.NewWorkspaceRepository(), nil)

	tests := []workspaceservice.CreateWorkspaceCommand{
		{WorkspaceID: "", Title: "ok"},
		{WorkspaceID: common.WorkspaceID("ws-1"), Title: ""},
	}
	for _, tt := range tests {
		if err := manager.CreateWorkspace(ctx, tt); !errors.Is(err, common.ErrInvalidArgument) {
			t.Fatalf("cmd %#v err = %v, want invalid argument", tt, err)
		}
	}
}

func TestGoalDoDManager_CreateWorkspace_RejectsDuplicateWorkspaceID(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	manager := NewGoalDoDManager(repo, nil)

	cmd := workspaceservice.CreateWorkspaceCommand{
		WorkspaceID: common.WorkspaceID("ws-duplicate"),
		Title:       "phase1 workspace",
	}
	if err := manager.CreateWorkspace(ctx, cmd); err != nil {
		t.Fatalf("first create workspace: %v", err)
	}

	second := workspaceservice.CreateWorkspaceCommand{
		WorkspaceID: cmd.WorkspaceID,
		Title:       "phase1 workspace updated",
	}
	if err := manager.CreateWorkspace(ctx, second); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("second create err = %v, want conflict", err)
	}

	got, err := repo.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		t.Fatalf("get workspace after duplicate create: %v", err)
	}
	if got.Title != cmd.Title {
		t.Fatalf("workspace title = %q, want original %q", got.Title, cmd.Title)
	}
}

func TestGoalDoDManager_CreateWorkspace_PublishesWorkspaceCreatedEvent(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	hub := events.NewMemoryHub()
	manager := NewGoalDoDManager(repo, hub)

	cmd := workspaceservice.CreateWorkspaceCommand{
		WorkspaceID: common.WorkspaceID("ws-event"),
		Title:       "phase1 workspace",
	}
	if err := manager.CreateWorkspace(ctx, cmd); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	published := hub.Published()
	if len(published) != 1 {
		t.Fatalf("len(published) = %d, want 1", len(published))
	}
	if published[0].Topic != events.TopicWorkspaceCreated {
		t.Fatalf("topic = %s, want %s", published[0].Topic, events.TopicWorkspaceCreated)
	}
	if published[0].Version != events.Phase1EventVersion {
		t.Fatalf("version = %s, want %s", published[0].Version, events.Phase1EventVersion)
	}
	if published[0].Payload[events.PayloadKeyTitle] != cmd.Title {
		t.Fatalf("title payload = %v, want %s", published[0].Payload[events.PayloadKeyTitle], cmd.Title)
	}
	if published[0].Payload[events.PayloadKeyState] != string(common.WorkspaceStateActive) {
		t.Fatalf("state payload = %v, want %s", published[0].Payload[events.PayloadKeyState], common.WorkspaceStateActive)
	}
}

func TestGoalDoDManager_EnsureWorkspaceCreatesMissingWorkspace(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	manager := NewGoalDoDManager(repo, nil)

	cmd := workspaceservice.EnsureWorkspaceCommand{
		WorkspaceID: common.WorkspaceID("ws-ensure-create"),
		Title:       "phase1 ensure workspace",
	}
	if err := manager.EnsureWorkspace(ctx, cmd); err != nil {
		t.Fatalf("ensure workspace: %v", err)
	}

	ws, err := repo.Get(ctx, cmd.WorkspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.Title != cmd.Title {
		t.Fatalf("title = %q, want %q", ws.Title, cmd.Title)
	}
	if ws.State != common.WorkspaceStateActive {
		t.Fatalf("state = %q, want active", ws.State)
	}
	if ws.ActiveRunID != "" {
		t.Fatalf("activeRunId = %q, want empty", ws.ActiveRunID)
	}
}

func TestGoalDoDManager_EnsureWorkspaceResetsWorkspaceAndPublishesStateChanged(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	hub := events.NewMemoryHub()
	manager := NewGoalDoDManager(repo, hub)

	workspaceID := common.WorkspaceID("ws-ensure-reset")
	if err := repo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "stale title",
		State:       common.WorkspaceStateArchived,
		ActiveRunID: common.RunID("run-stale"),
	}); err != nil {
		t.Fatalf("create stale workspace: %v", err)
	}

	if err := manager.EnsureWorkspace(ctx, workspaceservice.EnsureWorkspaceCommand{
		WorkspaceID: workspaceID,
		Title:       "phase1 reset workspace",
	}); err != nil {
		t.Fatalf("ensure workspace reset: %v", err)
	}

	ws, err := repo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get reset workspace: %v", err)
	}
	if ws.Title != "phase1 reset workspace" {
		t.Fatalf("title = %q, want phase1 reset workspace", ws.Title)
	}
	if ws.State != common.WorkspaceStateActive {
		t.Fatalf("state = %q, want active", ws.State)
	}
	if ws.ActiveRunID != "" {
		t.Fatalf("activeRunId = %q, want empty", ws.ActiveRunID)
	}

	published := hub.Published()
	if len(published) != 1 {
		t.Fatalf("len(published) = %d, want 1", len(published))
	}
	if published[0].Topic != events.TopicWorkspaceStateChanged {
		t.Fatalf("topic = %s, want %s", published[0].Topic, events.TopicWorkspaceStateChanged)
	}
	if published[0].Payload[events.PayloadKeyTitle] != "phase1 reset workspace" {
		t.Fatalf("title payload = %v, want phase1 reset workspace", published[0].Payload[events.PayloadKeyTitle])
	}
	if published[0].Payload[events.PayloadKeyState] != string(common.WorkspaceStateActive) {
		t.Fatalf("state payload = %v, want %s", published[0].Payload[events.PayloadKeyState], common.WorkspaceStateActive)
	}
	if published[0].Payload[events.PayloadKeyActiveRunID] != "" {
		t.Fatalf("activeRunId payload = %v, want empty", published[0].Payload[events.PayloadKeyActiveRunID])
	}
}

package workspace

import (
	"context"
	"errors"
	"testing"

	workspaceservice "github.com/muidea/workorch/internal/modules/kernel/workspace/service"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestGoalDoDManager_CreateWorkspace(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWorkspaceRepository()
	manager := NewGoalDoDManager(repo)

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
	manager := NewGoalDoDManager(memory.NewWorkspaceRepository())

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

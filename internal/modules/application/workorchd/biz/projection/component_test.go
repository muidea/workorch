package projection

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceviewcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

type loaderFixture struct {
	workspaces []appmodelcore.Workspace
	snapshots  map[string]readmodelcore.WorkspaceSnapshotState
}

func (l loaderFixture) ListWorkspaces(context.Context) ([]appmodelcore.Workspace, error) {
	return append([]appmodelcore.Workspace(nil), l.workspaces...), nil
}

func (l loaderFixture) ReadWorkspace(_ context.Context, workspaceID string) (appmodelcore.Workspace, error) {
	for _, workspace := range l.workspaces {
		if workspace.ID == workspaceID {
			return workspace, nil
		}
	}
	return appmodelcore.Workspace{}, common.ErrNotFound
}

func (l loaderFixture) ReadWorkspaceSnapshot(_ context.Context, workspaceID string) (readmodelcore.WorkspaceSnapshotState, error) {
	if snapshot, ok := l.snapshots[workspaceID]; ok {
		return snapshot, nil
	}
	return readmodelcore.WorkspaceSnapshotState{}, common.ErrNotFound
}

func TestComponentOwnsProjectionStartAndCloseLifecycle(t *testing.T) {
	background := task.NewBackgroundRoutine(8)
	t.Cleanup(func() { background.Shutdown(context.Background()) })
	component := New(loaderFixture{
		workspaces: []appmodelcore.Workspace{{ID: "ws-projection-component", Title: "Projection", State: "active"}},
		snapshots:  map[string]readmodelcore.WorkspaceSnapshotState{},
	}, background, workspaceviewcore.EventPorts{})
	if err := component.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		items, status, err := component.ListWorkspaces()
		if err == nil && status.State == workspaceviewcore.StateReady && len(items) == 1 && items[0].ID == "ws-projection-component" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("projection did not become ready: items=%v status=%+v err=%v", items, status, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := component.CloseContext(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
	if status := component.Status(); status.State != workspaceviewcore.StateStopped {
		t.Fatalf("status after close = %+v", status)
	}
	if err := component.Start(context.Background()); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("restart after close = %v, want ErrConflict", err)
	}
}

func TestComponentFailsClosedWhenBackgroundRoutineIsUnavailable(t *testing.T) {
	background := task.NewBackgroundRoutine(1)
	if !background.Shutdown(context.Background()) {
		t.Fatal("background shutdown failed")
	}
	component := New(loaderFixture{}, background, workspaceviewcore.EventPorts{})
	if err := component.Start(context.Background()); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("start error = %v, want ErrConflict", err)
	}
	if status := component.Status(); status.State != workspaceviewcore.StateFailed {
		t.Fatalf("status = %+v, want failed", status)
	}
	if err := component.CloseContext(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
}

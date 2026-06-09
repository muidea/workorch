package workspaceviewcore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestStoreReplaceAllListSnapshotAndDelete(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{
		{ID: "ws-b", Title: "B", State: "active"},
		{ID: "ws-a", Title: "A", State: "active"},
	}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {Workspace: readmodelcore.Workspace{ID: "ws-a", Title: "A+", State: "running"}},
	}, nil)

	items, status, err := store.List()
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if status.State != StateReady || status.GlobalRevision != 1 || status.WorkspaceTotal != 2 {
		t.Fatalf("status = %+v, want ready revision 1 total 2", status)
	}
	if len(items) != 2 || items[0].ID != "ws-a" || items[0].Title != "A+" || items[1].ID != "ws-b" {
		t.Fatalf("items = %+v, want sorted snapshot-backed workspaces", items)
	}

	snapshot, snapshotStatus, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if snapshotStatus.GlobalRevision != status.GlobalRevision || snapshot.Workspace.State != "running" {
		t.Fatalf("snapshot/status = %+v/%+v, want running snapshot at same revision", snapshot, snapshotStatus)
	}

	store.DeleteWorkspace("ws-a")
	items, status, err = store.List()
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if status.GlobalRevision != 2 || status.WorkspaceTotal != 1 {
		t.Fatalf("status after delete = %+v, want revision 2 total 1", status)
	}
	if len(items) != 1 || items[0].ID != "ws-b" {
		t.Fatalf("items after delete = %+v, want only ws-b", items)
	}
}

func TestStoreDegradedAndDetailNotReady(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, nil, errors.New("snapshot unavailable"))

	items, status, err := store.List()
	if err != nil {
		t.Fatalf("list degraded workspaces: %v", err)
	}
	if status.State != StateDegraded || status.Error == "" || len(items) != 1 {
		t.Fatalf("status/items = %+v/%+v, want degraded with one overview", status, items)
	}
	if !status.Degraded {
		t.Fatalf("status = %+v, want degraded flag", status)
	}
	if _, detailStatus, err := store.GetSnapshot("ws-a"); err == nil {
		t.Fatal("get missing detail succeeded, want error")
	} else if detailStatus.State != StateDegraded {
		t.Fatalf("detail status = %+v, want degraded", detailStatus)
	}
}

func TestStoreMarksStaleWithSourceEventRevision(t *testing.T) {
	store := NewStore()
	store.ReplaceAll([]appmodelcore.Workspace{{ID: "ws-a", State: "active"}}, map[string]readmodelcore.WorkspaceSnapshot{
		"ws-a": {Workspace: readmodelcore.Workspace{ID: "ws-a", State: "active"}},
	}, nil)

	store.MarkStale("ws-a", 42)
	_, status, err := store.List()
	if err != nil {
		t.Fatalf("list stale workspaces: %v", err)
	}
	if status.State != StateStale || status.SourceEventRevision != 42 {
		t.Fatalf("status = %+v, want stale source revision 42", status)
	}
	_, detailStatus, err := store.GetSnapshot("ws-a")
	if err != nil {
		t.Fatalf("get stale snapshot: %v", err)
	}
	if detailStatus.State != StateStale || detailStatus.SourceEventRevision != 42 {
		t.Fatalf("detail status = %+v, want stale source revision 42", detailStatus)
	}
	workspaceStatus := store.StatusForWorkspace("ws-a")
	if workspaceStatus.State != StateStale || workspaceStatus.SourceEventRevision != 42 {
		t.Fatalf("workspace status = %+v, want stale source revision 42", workspaceStatus)
	}

	store.UpsertSnapshot(readmodelcore.WorkspaceSnapshot{Workspace: readmodelcore.Workspace{ID: "ws-a", State: "running"}})
	_, status, err = store.List()
	if err != nil {
		t.Fatalf("list rebuilt workspaces: %v", err)
	}
	if status.State != StateReady || status.SourceEventRevision != 42 {
		t.Fatalf("rebuilt status = %+v, want ready retaining source revision 42", status)
	}
}

func TestManagerPublishesProjectionChangedAndIgnoresOwnEvent(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	scheduled := map[string]int{}
	scheduledFuncs := map[string][]func(context.Context){}
	manager := NewManager(ManagerOptions{
		Loader: fakeProjectionLoader{
			workspaces: []appmodelcore.Workspace{{ID: "ws-a", State: "active"}},
			snapshots: map[string]readmodelcore.WorkspaceSnapshot{
				"ws-a": {Workspace: readmodelcore.Workspace{ID: "ws-a", State: "running"}},
			},
		},
		Hub: hub,
		Schedule: func(name string, fn func(context.Context)) bool {
			scheduled[name]++
			scheduledFuncs[name] = append(scheduledFuncs[name], fn)
			return true
		},
	})
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	if scheduled["workspace_projection_rebuild_all"] != 1 {
		t.Fatalf("startup full rebuild scheduled %d times, want 1", scheduled["workspace_projection_rebuild_all"])
	}
	scheduledFuncs["workspace_projection_rebuild_all"][0](ctx)
	if err := manager.RebuildWorkspace(ctx, "ws-a"); err != nil {
		t.Fatalf("rebuild workspace: %v", err)
	}
	if scheduled["workspace_projection_rebuild"] != 0 {
		t.Fatalf("scheduled own projection event rebuild count = %d, want 0", scheduled["workspace_projection_rebuild"])
	}
	var found events.Envelope
	for _, item := range hub.Published() {
		if item.Topic == workorchdevents.TopicWorkspaceProjectionChanged && item.WorkspaceID == "ws-a" {
			found = item
		}
	}
	if found.Topic == "" {
		t.Fatalf("published events = %+v, want workspace projection changed for ws-a", hub.Published())
	}
	if found.Payload["changeKind"] != "detail" || found.Payload["projectionState"] != StateReady {
		t.Fatalf("projection payload = %+v, want detail ready", found.Payload)
	}

	if err := hub.Publish(ctx, events.Envelope{
		ID:          "run-a/state",
		Version:     events.WorkorchEventVersion,
		Topic:       runevents.TopicRunStateChanged,
		WorkspaceID: "ws-a",
		RunID:       "run-a",
		SubjectType: "run",
		SubjectID:   "run-a",
		Payload:     events.RunPayload("", "", common.RunStateCompleted, "", "", ""),
	}); err != nil {
		t.Fatalf("publish run state event: %v", err)
	}
	if scheduled["workspace_projection_rebuild"] != 1 {
		t.Fatalf("scheduled rebuild count after run event = %d, want 1", scheduled["workspace_projection_rebuild"])
	}
	status := manager.Status()
	if status.State != StateStale || status.SourceEventRevision == 0 {
		t.Fatalf("manager status after run event = %+v, want stale with source revision", status)
	}
}

func TestManagerStartRequiresScheduler(t *testing.T) {
	manager := NewManager(ManagerOptions{
		Loader: fakeProjectionLoader{},
	})
	if err := manager.Start(context.Background()); err == nil {
		t.Fatal("Start returned nil error without scheduler")
	}
}

func TestManagerReschedulesWorkspaceRebuildWhenEventArrivesInFlight(t *testing.T) {
	ctx := context.Background()
	loader := &countingProjectionLoader{}
	var scheduled []func(context.Context)
	manager := NewManager(ManagerOptions{
		Loader: loader,
		Schedule: func(name string, fn func(context.Context)) bool {
			if name != "workspace_projection_rebuild" {
				t.Fatalf("scheduled task = %q, want workspace_projection_rebuild", name)
			}
			scheduled = append(scheduled, fn)
			return true
		},
	})

	manager.ScheduleRebuildWorkspace("ws-a")
	manager.ScheduleRebuildWorkspace("ws-a")
	if len(scheduled) != 1 {
		t.Fatalf("scheduled rebuilds = %d, want one in-flight rebuild before pending rerun", len(scheduled))
	}

	scheduled[0](ctx)
	if len(scheduled) != 2 {
		t.Fatalf("scheduled rebuilds after first run = %d, want pending rerun", len(scheduled))
	}
	scheduled[1](ctx)

	snapshot, _, err := manager.ReadWorkspaceSnapshot("ws-a")
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if snapshot.Workspace.Title != "snapshot-2" || loader.calls != 2 {
		t.Fatalf("snapshot title/calls = %q/%d, want second rebuild to win", snapshot.Workspace.Title, loader.calls)
	}
}

func TestManagerSchedulesFullRebuildAfterEventDuringFullRebuild(t *testing.T) {
	ctx := context.Background()
	loader := &blockingProjectionLoader{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	var (
		scheduledMu sync.Mutex
		scheduled   []func(context.Context)
	)
	manager := NewManager(ManagerOptions{
		Loader: loader,
		Schedule: func(name string, fn func(context.Context)) bool {
			if name != "workspace_projection_rebuild_all" {
				t.Fatalf("scheduled task = %q, want workspace_projection_rebuild_all", name)
			}
			scheduledMu.Lock()
			scheduled = append(scheduled, fn)
			scheduledMu.Unlock()
			return true
		},
	})

	done := make(chan error, 1)
	go func() {
		done <- manager.RebuildAll(ctx)
	}()
	<-loader.started

	if err := manager.handleEvent(ctx, events.Envelope{
		Topic:       runevents.TopicRunStateChanged,
		WorkspaceID: "ws-a",
		SubjectType: "run",
		SubjectID:   "run-a",
		Revision:    7,
	}); err != nil {
		t.Fatalf("handle workspace event: %v", err)
	}
	if err := manager.handleEvent(ctx, events.Envelope{
		Topic:       runevents.TopicRunStateChanged,
		SubjectType: "workspace",
		SubjectID:   "ws-a",
		Revision:    8,
	}); err != nil {
		t.Fatalf("handle global workspace event: %v", err)
	}
	close(loader.release)
	if err := <-done; err != nil {
		t.Fatalf("full rebuild: %v", err)
	}

	scheduledMu.Lock()
	if len(scheduled) != 1 {
		t.Fatalf("scheduled rebuilds = %d, want one follow-up full rebuild", len(scheduled))
	}
	followup := scheduled[0]
	scheduledMu.Unlock()
	followup(ctx)
	if loader.calls != 2 {
		t.Fatalf("loader calls = %d, want initial and follow-up full rebuild", loader.calls)
	}
}

type fakeProjectionLoader struct {
	workspaces []appmodelcore.Workspace
	snapshots  map[string]readmodelcore.WorkspaceSnapshot
}

func (f fakeProjectionLoader) ListWorkspaces(context.Context) ([]appmodelcore.Workspace, error) {
	return append([]appmodelcore.Workspace(nil), f.workspaces...), nil
}

func (f fakeProjectionLoader) ReadWorkspace(_ context.Context, workspaceID string) (appmodelcore.Workspace, error) {
	for _, item := range f.workspaces {
		if item.ID == workspaceID {
			return item, nil
		}
	}
	return appmodelcore.Workspace{}, errors.New("workspace not found")
}

func (f fakeProjectionLoader) ReadWorkspaceSnapshot(_ context.Context, workspaceID string) (readmodelcore.WorkspaceSnapshot, error) {
	if item, ok := f.snapshots[workspaceID]; ok {
		return item, nil
	}
	return readmodelcore.WorkspaceSnapshot{}, errors.New("snapshot not found")
}

type countingProjectionLoader struct {
	calls int
}

func (f *countingProjectionLoader) ListWorkspaces(context.Context) ([]appmodelcore.Workspace, error) {
	return []appmodelcore.Workspace{{ID: "ws-a", Title: "workspace"}}, nil
}

func (f *countingProjectionLoader) ReadWorkspace(context.Context, string) (appmodelcore.Workspace, error) {
	return appmodelcore.Workspace{ID: "ws-a", Title: "workspace"}, nil
}

func (f *countingProjectionLoader) ReadWorkspaceSnapshot(context.Context, string) (readmodelcore.WorkspaceSnapshot, error) {
	f.calls++
	return readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-a", Title: fmt.Sprintf("snapshot-%d", f.calls)},
	}, nil
}

type blockingProjectionLoader struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func (f *blockingProjectionLoader) ListWorkspaces(context.Context) ([]appmodelcore.Workspace, error) {
	f.mu.Lock()
	f.calls++
	call := f.calls
	f.mu.Unlock()
	if call == 1 {
		close(f.started)
		<-f.release
	}
	return []appmodelcore.Workspace{{ID: "ws-a", Title: fmt.Sprintf("workspace-%d", call)}}, nil
}

func (f *blockingProjectionLoader) ReadWorkspace(context.Context, string) (appmodelcore.Workspace, error) {
	return appmodelcore.Workspace{ID: "ws-a", Title: "workspace"}, nil
}

func (f *blockingProjectionLoader) ReadWorkspaceSnapshot(context.Context, string) (readmodelcore.WorkspaceSnapshot, error) {
	f.mu.Lock()
	call := f.calls
	f.mu.Unlock()
	return readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-a", Title: fmt.Sprintf("snapshot-%d", call)},
	}, nil
}

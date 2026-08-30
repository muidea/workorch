package workorchrepo

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func fileEventStreamDeletionScope(workspaceID common.WorkspaceID, mode deletioncontract.Mode) deletioncontract.Scope {
	scope := deletioncontract.Scope{
		WorkspaceID: workspaceID, DeletionID: "deletion-" + string(workspaceID), DeletionGeneration: 1,
		Mode: mode, QuiescenceID: "quiescence-" + string(workspaceID),
	}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	return scope
}

func fileEventStreamFence(scope deletioncontract.Scope) *eventstreammodel.WorkspaceDeletion {
	return &eventstreammodel.WorkspaceDeletion{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		Mode: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
		FenceID: eventstreamevents.StableEventStreamDeletionFenceID(scope), Phase: eventstreammodel.WorkspaceDeletionPhaseFenced,
		Revision: 1, SealAckHash: eventstreamevents.StableEventStreamDeletionSealAckHash(scope),
		RequestedAt: "2026-08-25T00:00:00Z", UpdatedAt: "2026-08-25T00:00:00Z",
	}
}

func TestFileEventStreamRetainReopenHidesPhysicalRecordsAndRejectsOrdinaryMutation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repos := NewWorkspaceScopedFileRepositorySet(root)
	workspaceID := common.WorkspaceID("ws-eventstream-file-retain")
	visibleWorkspaceID := common.WorkspaceID("ws-eventstream-file-visible")
	runID := common.RunID("run-eventstream-file-retain")
	position, err := repos.EventRecords.ReserveEventRecordStreamPosition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	record := &eventstreammodel.EventRecord{ID: "event-file-retain", Topic: "/test", OccurredAt: "2026-08-25T00:00:00Z", WorkspaceID: workspaceID, RunID: runID, Revision: 1, StreamPosition: position}
	if err := repos.EventRecords.Append(ctx, record); err != nil {
		t.Fatal(err)
	}
	fence := fileEventStreamFence(fileEventStreamDeletionScope(workspaceID, deletioncontract.ModeRetain))
	if err := repos.EventStreamDeletions.Create(ctx, fence); err != nil {
		t.Fatal(err)
	}
	visiblePosition, err := repos.EventRecords.ReserveEventRecordStreamPosition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	visibleRecord := &eventstreammodel.EventRecord{ID: record.ID, Topic: "/test", OccurredAt: "2026-08-25T00:00:01Z", WorkspaceID: visibleWorkspaceID, RunID: runID, Revision: 1, StreamPosition: visiblePosition}
	if err := repos.EventRecords.Append(ctx, visibleRecord); err != nil {
		t.Fatal(err)
	}
	reopened := NewWorkspaceScopedFileRepositorySet(root)
	scoped := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	if _, err := reopened.EventRecords.Get(scoped, record.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("reopened get after retain = %v, want not found", err)
	}
	if items, err := reopened.EventRecords.List(scoped, workorchquerycontract.EventRecordQuery{WorkspaceID: workspaceID}); err != nil || len(items) != 0 {
		t.Fatalf("reopened list after retain = %+v err=%v", items, err)
	}
	if item, err := reopened.EventRecords.Get(ctx, record.ID); err != nil || item.WorkspaceID != visibleWorkspaceID {
		t.Fatalf("global get with hidden duplicate = %+v err=%v, want visible workspace", item, err)
	}
	if items, err := reopened.EventRecords.List(ctx, workorchquerycontract.EventRecordQuery{RunID: runID}); err != nil || len(items) != 1 || items[0].WorkspaceID != visibleWorkspaceID {
		t.Fatalf("global run list with hidden duplicate = %+v err=%v, want visible workspace only", items, err)
	}
	concrete := reopened.EventRecords.(*workspaceScopedEventRecordRepository)
	physicalPath := concrete.store.runPath(concrete.store.runRoot(workspaceID, runID), workspaceScopedEventRecordFile)
	physical, err := concrete.store.eventRecordRepo(physicalPath).List(scoped, workorchquerycontract.EventRecordQuery{WorkspaceID: workspaceID})
	if err != nil || len(physical) != 1 || physical[0].ID != record.ID {
		t.Fatalf("retained physical records = %+v err=%v", physical, err)
	}
	if err := reopened.EventRecords.Append(ctx, &eventstreammodel.EventRecord{ID: "event-file-after-retain", Topic: "/test", OccurredAt: "2026-08-25T00:00:02Z", WorkspaceID: workspaceID, RunID: runID, Revision: 2, StreamPosition: position + 1}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("append after retain = %v, want not found", err)
	}
	if err := reopened.EventRecords.Delete(scoped, record.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("ordinary delete after retain = %v, want not found", err)
	}
	if _, ok := reopened.EventRecords.(interface {
		PurgeWorkspace(context.Context, common.WorkspaceID) (int, error)
	}); ok {
		t.Fatal("workspace-scoped EventRecord repository exposes forbidden generic workspace purge")
	}
	if _, err := reopened.EventStreamPurger.PurgeWorkspaceEventRecords(ctx, *fence); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("exact purge for retain = %v, want invalid argument", err)
	}
}

func TestFileEventStreamExactPurgeReopenIsFixedPointAndPositionMonotonic(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repos := NewWorkspaceScopedFileRepositorySet(root)
	workspaceID := common.WorkspaceID("ws-eventstream-file-purge")
	runID := common.RunID("run-eventstream-file-purge")
	for idx, id := range []common.EventRecordID{"event-file-purge-a", "event-file-purge-b", "event-file-keep"} {
		position, err := repos.EventRecords.ReserveEventRecordStreamPosition(ctx)
		if err != nil {
			t.Fatal(err)
		}
		itemWorkspace, itemRun := workspaceID, runID
		if idx == 2 {
			itemWorkspace, itemRun = "ws-eventstream-file-keep", "run-eventstream-file-keep"
		}
		if err := repos.EventRecords.Append(ctx, &eventstreammodel.EventRecord{ID: id, Topic: "/test", OccurredAt: fmt.Sprintf("2026-08-25T00:00:0%dZ", idx), WorkspaceID: itemWorkspace, RunID: itemRun, Revision: int64(idx + 1), StreamPosition: position}); err != nil {
			t.Fatal(err)
		}
	}
	fence := fileEventStreamFence(fileEventStreamDeletionScope(workspaceID, deletioncontract.ModePurge))
	if err := repos.EventStreamDeletions.Create(ctx, fence); err != nil {
		t.Fatal(err)
	}
	if _, ok := repos.EventRecords.(interface {
		PurgeWorkspace(context.Context, common.WorkspaceID) (int, error)
	}); ok {
		t.Fatal("workspace-scoped EventRecord repository exposes forbidden generic workspace purge")
	}
	deleted, err := repos.EventStreamPurger.PurgeWorkspaceEventRecords(ctx, *fence)
	if err != nil || deleted != 2 {
		t.Fatalf("exact purge deleted=%d err=%v, want 2", deleted, err)
	}
	deleted, err = repos.EventStreamPurger.PurgeWorkspaceEventRecords(ctx, *fence)
	if err != nil || deleted != 0 {
		t.Fatalf("fixed-point purge deleted=%d err=%v, want 0", deleted, err)
	}
	purged := eventstreammodel.CloneWorkspaceDeletion(fence)
	purged.Phase = eventstreammodel.WorkspaceDeletionPhasePurged
	purged.Revision = 2
	purged.PurgeID = eventstreamevents.StableEventStreamDeletionPurgeID(fence.Scope())
	purged.PurgeAckHash = eventstreamevents.StableEventStreamDeletionPurgeAckHash(fence.Scope())
	purged.PurgeProofHash = eventstreamevents.StableEventStreamDeletionPurgeProofHash(fence.Scope())
	purged.PurgedAt = "2026-08-25T00:01:00Z"
	purged.UpdatedAt = purged.PurgedAt
	if err := repos.EventStreamDeletions.CompareAndSwap(ctx, purged, 1); err != nil {
		t.Fatal(err)
	}
	reopened := NewWorkspaceScopedFileRepositorySet(root)
	current, err := reopened.EventStreamDeletions.Get(ctx, workspaceID)
	if err != nil || current.Phase != eventstreammodel.WorkspaceDeletionPhasePurged || current.Revision != 2 {
		t.Fatalf("reopened tombstone = %+v err=%v", current, err)
	}
	if items, err := reopened.EventRecords.List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: workspaceID}); err != nil || len(items) != 0 {
		t.Fatalf("reopened purged records = %+v err=%v", items, err)
	}
	keep, err := reopened.EventRecords.List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: "ws-eventstream-file-keep"})
	if err != nil || len(keep) != 1 || keep[0].ID != "event-file-keep" {
		t.Fatalf("retained other workspace records = %+v err=%v", keep, err)
	}
	nextPosition, err := reopened.EventRecords.ReserveEventRecordStreamPosition(ctx)
	if err != nil || nextPosition != 4 {
		t.Fatalf("next global position = %d err=%v, want 4", nextPosition, err)
	}
	if err := reopened.EventRecords.Append(ctx, &eventstreammodel.EventRecord{ID: "event-global-after-file-purge", Topic: "/test", OccurredAt: "2026-08-25T00:02:00Z", StreamPosition: nextPosition}); err != nil {
		t.Fatalf("global append after workspace purge: %v", err)
	}
}

func TestFileEventStreamIndependentRepositorySetsSealVsAppend(t *testing.T) {
	base := t.TempDir()
	for iteration := 0; iteration < 50; iteration++ {
		ctx := context.Background()
		root := filepath.Join(base, fmt.Sprintf("iteration-%03d", iteration))
		first := NewWorkspaceScopedFileRepositorySet(root)
		second := NewWorkspaceScopedFileRepositorySet(root)
		workspaceID := common.WorkspaceID("ws-eventstream-file-race")
		fence := fileEventStreamFence(fileEventStreamDeletionScope(workspaceID, deletioncontract.ModeRetain))
		start := make(chan struct{})
		var wait sync.WaitGroup
		wait.Add(2)
		var appendErr, fenceErr error
		go func() {
			defer wait.Done()
			<-start
			appendErr = first.EventRecords.Append(ctx, &eventstreammodel.EventRecord{ID: "event-file-race", Topic: "/test", OccurredAt: "2026-08-25T00:03:00Z", WorkspaceID: workspaceID, RunID: "run-eventstream-file-race", Revision: 1, StreamPosition: 1})
		}()
		go func() {
			defer wait.Done()
			<-start
			fenceErr = second.EventStreamDeletions.Create(ctx, fence)
		}()
		close(start)
		wait.Wait()
		if fenceErr != nil {
			t.Fatalf("iteration %d fence error: %v", iteration, fenceErr)
		}
		if appendErr != nil && !errors.Is(appendErr, common.ErrNotFound) {
			t.Fatalf("iteration %d append error: %v", iteration, appendErr)
		}
		reopened := NewWorkspaceScopedFileRepositorySet(root)
		if err := reopened.EventRecords.Append(ctx, &eventstreammodel.EventRecord{ID: "event-file-after-race", Topic: "/test", OccurredAt: "2026-08-25T00:03:01Z", WorkspaceID: workspaceID, RunID: "run-eventstream-file-race", Revision: 2, StreamPosition: 2}); !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("iteration %d post-fence append = %v, want not found", iteration, err)
		}
		if items, err := reopened.EventRecords.List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: workspaceID}); err != nil || len(items) != 0 {
			t.Fatalf("iteration %d public records=%+v err=%v", iteration, items, err)
		}
	}
}

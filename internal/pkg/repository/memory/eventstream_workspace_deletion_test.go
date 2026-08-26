package memory

import (
	"context"
	"errors"
	"sync"
	"testing"

	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

func memoryEventStreamDeletionScope(workspaceID common.WorkspaceID, mode deletioncontract.Mode) deletioncontract.Scope {
	scope := deletioncontract.Scope{
		WorkspaceID: workspaceID, DeletionID: "deletion-" + string(workspaceID), DeletionGeneration: 1,
		Mode: mode, QuiescenceID: "quiescence-" + string(workspaceID),
	}
	scope.RequestHash = deletioncontract.StableRequestHash(scope)
	return scope
}

func memoryEventStreamFence(scope deletioncontract.Scope) *eventstreammodel.WorkspaceDeletion {
	return &eventstreammodel.WorkspaceDeletion{
		WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
		Mode: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
		FenceID: eventstreamevents.StableEventStreamDeletionFenceID(scope), Phase: eventstreammodel.WorkspaceDeletionPhaseFenced,
		Revision: 1, SealAckHash: eventstreamevents.StableEventStreamDeletionSealAckHash(scope),
		RequestedAt: "2026-08-25T00:00:00Z", UpdatedAt: "2026-08-25T00:00:00Z",
	}
}

func TestMemoryEventStreamRetainFenceHidesWithoutPhysicalDeletionAndRejectsMutation(t *testing.T) {
	ctx := context.Background()
	owner := NewEventStreamOwnerRepositorySet()
	workspaceID := common.WorkspaceID("ws-eventstream-memory-retain")
	visibleWorkspaceID := common.WorkspaceID("ws-eventstream-memory-visible")
	sharedRunID := common.RunID("run-eventstream-memory-shared")
	position, err := owner.EventRecords.ReserveEventRecordStreamPosition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	record := &eventstreammodel.EventRecord{ID: "event-retain", Topic: "/test", WorkspaceID: workspaceID, RunID: sharedRunID, Revision: 1, StreamPosition: position}
	if err := owner.EventRecords.Append(ctx, record); err != nil {
		t.Fatal(err)
	}
	fence := memoryEventStreamFence(memoryEventStreamDeletionScope(workspaceID, deletioncontract.ModeRetain))
	if err := owner.WorkspaceDeletions.Create(ctx, fence); err != nil {
		t.Fatal(err)
	}
	scoped := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	if _, err := owner.EventRecords.Get(scoped, record.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("get after retain fence = %v, want not found", err)
	}
	if items, err := owner.EventRecords.List(scoped, workorchquerycontract.EventRecordQuery{WorkspaceID: workspaceID}); err != nil || len(items) != 0 {
		t.Fatalf("list after retain fence = %+v err=%v", items, err)
	}
	if len(owner.EventRecords.items) != 1 {
		t.Fatalf("physical records after retain = %d, want 1", len(owner.EventRecords.items))
	}
	if err := owner.EventRecords.Append(ctx, &eventstreammodel.EventRecord{ID: "event-after-retain", Topic: "/test", WorkspaceID: workspaceID, Revision: 2, StreamPosition: position + 1}); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("append after retain fence = %v, want not found", err)
	}
	if err := owner.EventRecords.Delete(scoped, record.ID); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("delete after retain fence = %v, want not found", err)
	}
	if _, ok := any(owner.EventRecords).(interface {
		PurgeWorkspace(context.Context, common.WorkspaceID) (int, error)
	}); ok {
		t.Fatal("memory EventRecord repository exposes forbidden generic workspace purge")
	}
	if _, err := owner.EventRecords.PurgeWorkspaceEventRecords(ctx, *fence); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("exact purge for retain = %v, want invalid argument", err)
	}
	visiblePosition, err := owner.EventRecords.ReserveEventRecordStreamPosition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	visibleRecord := &eventstreammodel.EventRecord{ID: record.ID, Topic: "/test", WorkspaceID: visibleWorkspaceID, RunID: sharedRunID, Revision: 1, StreamPosition: visiblePosition}
	if err := owner.EventRecords.Append(ctx, visibleRecord); err != nil {
		t.Fatal(err)
	}
	if item, err := owner.EventRecords.Get(ctx, record.ID); err != nil || item.WorkspaceID != visibleWorkspaceID {
		t.Fatalf("global get with hidden duplicate = %+v err=%v, want visible workspace", item, err)
	}
	if items, err := owner.EventRecords.List(ctx, workorchquerycontract.EventRecordQuery{RunID: sharedRunID}); err != nil || len(items) != 1 || items[0].WorkspaceID != visibleWorkspaceID {
		t.Fatalf("global run list with hidden duplicate = %+v err=%v, want visible workspace only", items, err)
	}
	globalPosition, err := owner.EventRecords.ReserveEventRecordStreamPosition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.EventRecords.Append(ctx, &eventstreammodel.EventRecord{ID: "event-global", Topic: "/test", StreamPosition: globalPosition}); err != nil {
		t.Fatalf("global append after workspace retain fence: %v", err)
	}
}

func TestMemoryEventStreamExactPurgeIsFixedPointAndPreservesPositionFloor(t *testing.T) {
	ctx := context.Background()
	owner := NewEventStreamOwnerRepositorySet()
	workspaceID := common.WorkspaceID("ws-eventstream-memory-purge")
	for idx, id := range []common.EventRecordID{"event-purge-a", "event-purge-b", "event-keep"} {
		position, err := owner.EventRecords.ReserveEventRecordStreamPosition(ctx)
		if err != nil {
			t.Fatal(err)
		}
		itemWorkspace := workspaceID
		if idx == 2 {
			itemWorkspace = "ws-eventstream-memory-keep"
		}
		if err := owner.EventRecords.Append(ctx, &eventstreammodel.EventRecord{ID: id, Topic: "/test", WorkspaceID: itemWorkspace, Revision: int64(idx + 1), StreamPosition: position}); err != nil {
			t.Fatal(err)
		}
	}
	fence := memoryEventStreamFence(memoryEventStreamDeletionScope(workspaceID, deletioncontract.ModePurge))
	if err := owner.WorkspaceDeletions.Create(ctx, fence); err != nil {
		t.Fatal(err)
	}
	changed := *fence
	changed.QuiescenceID = "quiescence-changed"
	changed.RequestHash = deletioncontract.StableRequestHash(changed.Scope())
	if _, err := owner.EventRecords.PurgeWorkspaceEventRecords(ctx, changed); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("changed exact purge = %v, want conflict", err)
	}
	deleted, err := owner.EventRecords.PurgeWorkspaceEventRecords(ctx, *fence)
	if err != nil || deleted != 2 {
		t.Fatalf("exact purge deleted=%d err=%v, want 2", deleted, err)
	}
	deleted, err = owner.EventRecords.PurgeWorkspaceEventRecords(ctx, *fence)
	if err != nil || deleted != 0 {
		t.Fatalf("fixed-point exact purge deleted=%d err=%v, want 0", deleted, err)
	}
	if len(owner.EventRecords.items) != 1 {
		t.Fatalf("physical records after purge = %d, want one retained workspace", len(owner.EventRecords.items))
	}
	purged := eventstreammodel.CloneWorkspaceDeletion(fence)
	purged.Phase = eventstreammodel.WorkspaceDeletionPhasePurged
	purged.Revision = 2
	purged.PurgeID = eventstreamevents.StableEventStreamDeletionPurgeID(fence.Scope())
	purged.PurgeAckHash = eventstreamevents.StableEventStreamDeletionPurgeAckHash(fence.Scope())
	purged.PurgeProofHash = eventstreamevents.StableEventStreamDeletionPurgeProofHash(fence.Scope())
	purged.PurgedAt = "2026-08-25T00:01:00Z"
	purged.UpdatedAt = purged.PurgedAt
	if err := owner.WorkspaceDeletions.CompareAndSwap(ctx, purged, 1); err != nil {
		t.Fatal(err)
	}
	nextPosition, err := owner.EventRecords.ReserveEventRecordStreamPosition(ctx)
	if err != nil || nextPosition != 4 {
		t.Fatalf("next global position = %d err=%v, want 4", nextPosition, err)
	}
}

func TestMemoryEventStreamSealVsAppendUsesOneSharedGuard(t *testing.T) {
	for iteration := 0; iteration < 100; iteration++ {
		ctx := context.Background()
		owner := NewEventStreamOwnerRepositorySet()
		workspaceID := common.WorkspaceID("ws-eventstream-memory-race")
		fence := memoryEventStreamFence(memoryEventStreamDeletionScope(workspaceID, deletioncontract.ModeRetain))
		start := make(chan struct{})
		var wait sync.WaitGroup
		wait.Add(2)
		var appendErr, fenceErr error
		go func() {
			defer wait.Done()
			<-start
			appendErr = owner.EventRecords.Append(ctx, &eventstreammodel.EventRecord{ID: "event-race", Topic: "/test", WorkspaceID: workspaceID, Revision: 1, StreamPosition: 1})
		}()
		go func() {
			defer wait.Done()
			<-start
			fenceErr = owner.WorkspaceDeletions.Create(ctx, fence)
		}()
		close(start)
		wait.Wait()
		if fenceErr != nil {
			t.Fatalf("iteration %d fence error: %v", iteration, fenceErr)
		}
		if appendErr != nil && !errors.Is(appendErr, common.ErrNotFound) {
			t.Fatalf("iteration %d append error: %v", iteration, appendErr)
		}
		if err := owner.EventRecords.Append(ctx, &eventstreammodel.EventRecord{ID: "event-after-race", Topic: "/test", WorkspaceID: workspaceID, Revision: 2, StreamPosition: 2}); !errors.Is(err, common.ErrNotFound) {
			t.Fatalf("iteration %d post-fence append = %v, want not found", iteration, err)
		}
		if items, err := owner.EventRecords.List(ctx, workorchquerycontract.EventRecordQuery{WorkspaceID: workspaceID}); err != nil || len(items) != 0 {
			t.Fatalf("iteration %d public records=%+v err=%v", iteration, items, err)
		}
	}
}

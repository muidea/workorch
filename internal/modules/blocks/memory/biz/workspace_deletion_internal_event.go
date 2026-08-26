package memory

import (
	"context"
	"errors"
	"sync"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

const workspaceDeletionCommandSource = "blocks/workspace"

type workspaceDeletionInternalEventKind string

const (
	workspaceDeletionInternalEventFenceObserved  workspaceDeletionInternalEventKind = "fence_observed"
	workspaceDeletionInternalEventPurgeRequested workspaceDeletionInternalEventKind = "purge_requested"
	workspaceDeletionInternalEventPurgeCompleted workspaceDeletionInternalEventKind = "purge_completed"
)

type workspaceDeletionInternalEvent struct {
	Kind  workspaceDeletionInternalEventKind
	Scope deletioncontract.Scope
}

func (m *Biz) lockWorkspaceMutation(workspaceID common.WorkspaceID) func() {
	m.workspaceLocksMu.Lock()
	if m.workspaceLocks == nil {
		m.workspaceLocks = map[common.WorkspaceID]*sync.Mutex{}
	}
	lock := m.workspaceLocks[workspaceID]
	if lock == nil {
		lock = &sync.Mutex{}
		m.workspaceLocks[workspaceID] = lock
	}
	m.workspaceLocksMu.Unlock()
	lock.Lock()
	return lock.Unlock
}

func (m *Biz) rejectFencedWorkspace(ctx context.Context, workspaceID common.WorkspaceID) error {
	if m == nil || m.workspaceDeletions == nil || workspaceID == "" {
		return common.ErrInvalidArgument
	}
	_, err := m.workspaceDeletions.Get(ctx, workspaceID)
	if errors.Is(err, common.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return common.ErrNotFound
}

func (m *Biz) executeWorkspaceDeletionCommand(ctx context.Context, ev magicevent.Event, result magicevent.Result) error {
	if ev == nil || ev.Source() != workspaceDeletionCommandSource || m == nil || m.workspaceDeletions == nil || m.service == nil {
		setMemoryMagicResult(result, struct{}{}, common.ErrInvalidArgument)
		return common.ErrInvalidArgument
	}
	switch ev.ID() {
	case memoryevents.TopicMemoryCommandSealWorkspaceDeletion:
		cmd, ok := ev.Data().(memoryevents.SealWorkspaceMemoryDeletionCommand)
		if !ok || cmd.Validate() != nil {
			setMemoryMagicResult(result, memoryevents.SealWorkspaceMemoryDeletionResult{}, common.ErrInvalidArgument)
			return common.ErrInvalidArgument
		}
		response, err := m.sealWorkspaceDeletion(ctx, cmd.Scope())
		setMemoryMagicResult(result, response, err)
		return err
	case memoryevents.TopicMemoryCommandPurgeWorkspace:
		cmd, ok := ev.Data().(memoryevents.PurgeWorkspaceMemoryCommand)
		if !ok || cmd.Validate() != nil {
			setMemoryMagicResult(result, memoryevents.PurgeWorkspaceMemoryResult{}, common.ErrInvalidArgument)
			return common.ErrInvalidArgument
		}
		response, err := m.purgeWorkspace(ctx, cmd.Scope())
		setMemoryMagicResult(result, response, err)
		return err
	default:
		setMemoryMagicResult(result, struct{}{}, common.ErrInvalidArgument)
		return common.ErrInvalidArgument
	}
}

func (m *Biz) sealWorkspaceDeletion(ctx context.Context, scope deletioncontract.Scope) (memoryevents.SealWorkspaceMemoryDeletionResult, error) {
	if err := scope.Validate(); err != nil {
		return memoryevents.SealWorkspaceMemoryDeletionResult{}, err
	}
	current, err := m.workspaceDeletions.Get(ctx, scope.WorkspaceID)
	if errors.Is(err, common.ErrNotFound) {
		current, err = m.applyWorkspaceDeletionInternalEvent(ctx, nil, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventFenceObserved, Scope: scope})
	}
	if err != nil {
		return memoryevents.SealWorkspaceMemoryDeletionResult{}, err
	}
	if !sameMemoryDeletionScope(current, scope) {
		return memoryevents.SealWorkspaceMemoryDeletionResult{}, common.ErrConflict
	}
	return memoryevents.SealWorkspaceMemoryDeletionResult{
		WorkspaceID: current.WorkspaceID, DeletionID: current.DeletionID, DeletionGeneration: current.DeletionGeneration,
		FenceID: current.FenceID, Revision: 1, AckHash: current.SealAckHash,
	}, nil
}

func (m *Biz) purgeWorkspace(ctx context.Context, scope deletioncontract.Scope) (memoryevents.PurgeWorkspaceMemoryResult, error) {
	if err := scope.Validate(); err != nil || scope.Mode != deletioncontract.ModePurge {
		if err != nil {
			return memoryevents.PurgeWorkspaceMemoryResult{}, err
		}
		return memoryevents.PurgeWorkspaceMemoryResult{}, common.ErrInvalidArgument
	}
	current, err := m.workspaceDeletions.Get(ctx, scope.WorkspaceID)
	if err != nil {
		return memoryevents.PurgeWorkspaceMemoryResult{}, err
	}
	if !sameMemoryDeletionScope(current, scope) {
		return memoryevents.PurgeWorkspaceMemoryResult{}, common.ErrConflict
	}
	if current.Phase != memorymodel.WorkspaceDeletionPhasePurged {
		current, err = m.applyWorkspaceDeletionInternalEvent(ctx, current, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventPurgeRequested, Scope: scope})
		if err != nil {
			return memoryevents.PurgeWorkspaceMemoryResult{}, err
		}
	}
	return memoryevents.PurgeWorkspaceMemoryResult{
		WorkspaceID: current.WorkspaceID, DeletionID: current.DeletionID, DeletionGeneration: current.DeletionGeneration,
		FenceID: current.FenceID, PurgeID: current.PurgeID, Revision: current.Revision, AckHash: current.PurgeAckHash, PurgeProofHash: current.PurgeProofHash,
	}, nil
}

func (m *Biz) applyWorkspaceDeletionInternalEvent(ctx context.Context, current *memorymodel.WorkspaceDeletion, event workspaceDeletionInternalEvent) (*memorymodel.WorkspaceDeletion, error) {
	if m == nil || m.workspaceDeletions == nil {
		return nil, common.ErrInvalidArgument
	}
	if event.Kind == workspaceDeletionInternalEventPurgeCompleted {
		return nil, common.ErrConflict
	}
	if event.Kind == workspaceDeletionInternalEventPurgeRequested {
		if current == nil || current.Validate() != nil || current.Phase != memorymodel.WorkspaceDeletionPhaseFenced ||
			event.Scope.Validate() != nil || event.Scope.Mode != deletioncontract.ModePurge || !sameMemoryDeletionScope(current, event.Scope) {
			return nil, common.ErrConflict
		}
		if err := m.workspaceDeletions.WithWorkspacePurgeFence(ctx, event.Scope, func(guarded context.Context) error {
			return m.purgeWorkspaceData(guarded, event.Scope.WorkspaceID)
		}); err != nil {
			return nil, err
		}
		event = workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventPurgeCompleted, Scope: event.Scope}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	next, err := reduceWorkspaceDeletionInternalEvent(current, event, now)
	if err != nil {
		return nil, err
	}
	if current == nil {
		err = m.workspaceDeletions.Create(ctx, next)
	} else {
		err = m.workspaceDeletions.CompareAndSwap(ctx, next, current.Revision)
	}
	if err != nil {
		return nil, err
	}
	return next, nil
}
func reduceWorkspaceDeletionInternalEvent(current *memorymodel.WorkspaceDeletion, event workspaceDeletionInternalEvent, now string) (*memorymodel.WorkspaceDeletion, error) {
	if event.Scope.Validate() != nil || now == "" {
		return nil, common.ErrInvalidArgument
	}
	switch event.Kind {
	case workspaceDeletionInternalEventFenceObserved:
		if current != nil {
			return nil, common.ErrConflict
		}
		next := &memorymodel.WorkspaceDeletion{
			WorkspaceID: event.Scope.WorkspaceID, DeletionID: event.Scope.DeletionID, DeletionGeneration: event.Scope.DeletionGeneration,
			Mode: event.Scope.Mode, QuiescenceID: event.Scope.QuiescenceID, RequestHash: event.Scope.RequestHash,
			FenceID: memoryevents.StableMemoryDeletionFenceID(event.Scope), Phase: memorymodel.WorkspaceDeletionPhaseFenced, Revision: 1,
			SealAckHash: memoryevents.StableMemoryDeletionSealAckHash(event.Scope), RequestedAt: now, UpdatedAt: now,
		}
		return next, nil
	case workspaceDeletionInternalEventPurgeCompleted:
		if current == nil || current.Phase != memorymodel.WorkspaceDeletionPhaseFenced || !sameMemoryDeletionScope(current, event.Scope) {
			return nil, common.ErrConflict
		}
		next := memorymodel.CloneWorkspaceDeletion(current)
		next.Phase = memorymodel.WorkspaceDeletionPhasePurged
		next.Revision = current.Revision + 1
		next.PurgeID = memoryevents.StableMemoryDeletionPurgeID(event.Scope)
		next.PurgeProofHash = memoryevents.StableMemoryDeletionPurgeProofHash(event.Scope)
		next.PurgeAckHash = memoryevents.StableMemoryDeletionPurgeAckHash(event.Scope)
		next.PurgedAt = now
		next.UpdatedAt = now
		return next, nil
	default:
		return nil, common.ErrInvalidArgument
	}
}

func (m *Biz) purgeWorkspaceData(ctx context.Context, workspaceID common.WorkspaceID) error {
	for pass := 0; pass < 1000; pass++ {
		items, err := m.service.items.ListByWorkspaceID(ctx, workspaceID)
		if err != nil {
			return err
		}
		traces, err := m.service.traces.List(ctx, workorchquerycontract.MemoryRecallTraceQuery{WorkspaceID: workspaceID})
		if err != nil {
			return err
		}
		if len(items) == 0 && len(traces) == 0 {
			return nil
		}
		for _, item := range items {
			if item == nil || item.WorkspaceID != workspaceID {
				return common.ErrConflict
			}
			if err := m.service.items.Delete(ctx, item.ID); err != nil && !errors.Is(err, common.ErrNotFound) {
				return err
			}
		}
		for _, trace := range traces {
			if trace == nil || trace.WorkspaceID != workspaceID {
				return common.ErrConflict
			}
			if err := m.service.traces.Delete(ctx, trace.ID); err != nil && !errors.Is(err, common.ErrNotFound) {
				return err
			}
		}
	}
	return common.ErrConflict
}

func sameMemoryDeletionScope(item *memorymodel.WorkspaceDeletion, scope deletioncontract.Scope) bool {
	return item != nil && item.Scope() == scope.Normalize()
}

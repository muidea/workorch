package result

import (
	"context"
	"errors"
	"sync"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
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

func (m *Biz) executeWorkspaceDeletionCommand(ctx context.Context, ev magicevent.Event) (any, error) {
	if ev == nil || ev.Source() != workspaceDeletionCommandSource || m == nil || m.workspaceDeletions == nil || m.service == nil {
		return nil, common.ErrInvalidArgument
	}
	switch ev.ID() {
	case resultevents.TopicResultCommandSealWorkspaceDeletion:
		cmd, ok := ev.Data().(resultevents.SealWorkspaceResultDeletionCommand)
		if !ok || cmd.Validate() != nil {
			return resultevents.SealWorkspaceResultDeletionResult{}, common.ErrInvalidArgument
		}
		return m.sealWorkspaceDeletion(ctx, cmd.Scope())
	case resultevents.TopicResultCommandPurgeWorkspace:
		cmd, ok := ev.Data().(resultevents.PurgeWorkspaceResultsCommand)
		if !ok || cmd.Validate() != nil {
			return resultevents.PurgeWorkspaceResultsResult{}, common.ErrInvalidArgument
		}
		return m.purgeWorkspace(ctx, cmd.Scope())
	default:
		return nil, common.ErrInvalidArgument
	}
}

func (m *Biz) sealWorkspaceDeletion(ctx context.Context, scope deletioncontract.Scope) (resultevents.SealWorkspaceResultDeletionResult, error) {
	if err := scope.Validate(); err != nil {
		return resultevents.SealWorkspaceResultDeletionResult{}, err
	}
	current, err := m.workspaceDeletions.Get(ctx, scope.WorkspaceID)
	if errors.Is(err, common.ErrNotFound) {
		current, err = m.applyWorkspaceDeletionInternalEvent(ctx, nil, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventFenceObserved, Scope: scope})
	}
	if err != nil {
		return resultevents.SealWorkspaceResultDeletionResult{}, err
	}
	if !sameResultDeletionScope(current, scope) {
		return resultevents.SealWorkspaceResultDeletionResult{}, common.ErrConflict
	}
	return resultevents.SealWorkspaceResultDeletionResult{
		WorkspaceID: current.WorkspaceID, DeletionID: current.DeletionID, DeletionGeneration: current.DeletionGeneration,
		FenceID: current.FenceID, Revision: 1, AckHash: current.SealAckHash,
	}, nil
}

func (m *Biz) purgeWorkspace(ctx context.Context, scope deletioncontract.Scope) (resultevents.PurgeWorkspaceResultsResult, error) {
	if err := scope.Validate(); err != nil || scope.Mode != deletioncontract.ModePurge {
		if err != nil {
			return resultevents.PurgeWorkspaceResultsResult{}, err
		}
		return resultevents.PurgeWorkspaceResultsResult{}, common.ErrInvalidArgument
	}
	current, err := m.workspaceDeletions.Get(ctx, scope.WorkspaceID)
	if err != nil {
		return resultevents.PurgeWorkspaceResultsResult{}, err
	}
	if !sameResultDeletionScope(current, scope) {
		return resultevents.PurgeWorkspaceResultsResult{}, common.ErrConflict
	}
	if current.Phase != resultmodel.WorkspaceDeletionPhasePurged {
		current, err = m.applyWorkspaceDeletionInternalEvent(ctx, current, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventPurgeRequested, Scope: scope})
		if err != nil {
			return resultevents.PurgeWorkspaceResultsResult{}, err
		}
	}
	return resultevents.PurgeWorkspaceResultsResult{
		WorkspaceID: current.WorkspaceID, DeletionID: current.DeletionID, DeletionGeneration: current.DeletionGeneration,
		FenceID: current.FenceID, PurgeID: current.PurgeID, Revision: current.Revision, AckHash: current.PurgeAckHash, PurgeProofHash: current.PurgeProofHash,
	}, nil
}

func (m *Biz) applyWorkspaceDeletionInternalEvent(ctx context.Context, current *resultmodel.WorkspaceDeletion, event workspaceDeletionInternalEvent) (*resultmodel.WorkspaceDeletion, error) {
	if m == nil || m.workspaceDeletions == nil {
		return nil, common.ErrInvalidArgument
	}
	if event.Kind == workspaceDeletionInternalEventPurgeCompleted {
		return nil, common.ErrConflict
	}
	if event.Kind == workspaceDeletionInternalEventPurgeRequested {
		if current == nil || current.Validate() != nil || current.Phase != resultmodel.WorkspaceDeletionPhaseFenced ||
			event.Scope.Validate() != nil || event.Scope.Mode != deletioncontract.ModePurge || !sameResultDeletionScope(current, event.Scope) {
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
func reduceWorkspaceDeletionInternalEvent(current *resultmodel.WorkspaceDeletion, event workspaceDeletionInternalEvent, now string) (*resultmodel.WorkspaceDeletion, error) {
	if event.Scope.Validate() != nil || now == "" {
		return nil, common.ErrInvalidArgument
	}
	switch event.Kind {
	case workspaceDeletionInternalEventFenceObserved:
		if current != nil {
			return nil, common.ErrConflict
		}
		next := &resultmodel.WorkspaceDeletion{
			WorkspaceID: event.Scope.WorkspaceID, DeletionID: event.Scope.DeletionID, DeletionGeneration: event.Scope.DeletionGeneration,
			Mode: event.Scope.Mode, QuiescenceID: event.Scope.QuiescenceID, RequestHash: event.Scope.RequestHash,
			FenceID: resultevents.StableResultDeletionFenceID(event.Scope), Phase: resultmodel.WorkspaceDeletionPhaseFenced, Revision: 1,
			SealAckHash: resultevents.StableResultDeletionSealAckHash(event.Scope), RequestedAt: now, UpdatedAt: now,
		}
		return next, nil
	case workspaceDeletionInternalEventPurgeCompleted:
		if current == nil || current.Phase != resultmodel.WorkspaceDeletionPhaseFenced || !sameResultDeletionScope(current, event.Scope) {
			return nil, common.ErrConflict
		}
		next := resultmodel.CloneWorkspaceDeletion(current)
		next.Phase = resultmodel.WorkspaceDeletionPhasePurged
		next.Revision = current.Revision + 1
		next.PurgeID = resultevents.StableResultDeletionPurgeID(event.Scope)
		next.PurgeProofHash = resultevents.StableResultDeletionPurgeProofHash(event.Scope)
		next.PurgeAckHash = resultevents.StableResultDeletionPurgeAckHash(event.Scope)
		next.PurgedAt = now
		next.UpdatedAt = now
		return next, nil
	default:
		return nil, common.ErrInvalidArgument
	}
}

func (m *Biz) purgeWorkspaceData(ctx context.Context, workspaceID common.WorkspaceID) error {
	for pass := 0; pass < 1000; pass++ {
		results, err := m.service.results.ListByWorkspace(ctx, workspaceID)
		if err != nil {
			return err
		}
		artifacts, err := m.service.artifacts.ListByWorkspace(ctx, workspaceID)
		if err != nil {
			return err
		}
		if len(results) == 0 && len(artifacts) == 0 {
			return nil
		}
		for _, item := range results {
			if item == nil || item.WorkspaceID != workspaceID {
				return common.ErrConflict
			}
			if err := m.service.results.Delete(ctx, item.ID); err != nil && !errors.Is(err, common.ErrNotFound) {
				return err
			}
		}
		for _, item := range artifacts {
			if item == nil || item.WorkspaceID != workspaceID {
				return common.ErrConflict
			}
			if err := m.service.artifacts.Delete(ctx, item.ID); err != nil && !errors.Is(err, common.ErrNotFound) {
				return err
			}
		}
	}
	return common.ErrConflict
}

func sameResultDeletionScope(item *resultmodel.WorkspaceDeletion, scope deletioncontract.Scope) bool {
	return item != nil && item.Scope() == scope.Normalize()
}

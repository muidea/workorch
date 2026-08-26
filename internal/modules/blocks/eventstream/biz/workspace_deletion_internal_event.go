package eventstream

import (
	"context"
	"errors"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
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

func (s *Biz) workspaceDeletionHidden(ctx context.Context, workspaceID common.WorkspaceID) (bool, error) {
	if s == nil || s.workspaceDeletions == nil || workspaceID == "" {
		return false, common.ErrInvalidArgument
	}
	_, err := s.workspaceDeletions.Get(ctx, workspaceID)
	if errors.Is(err, common.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Biz) executeWorkspaceDeletionCommand(ctx context.Context, ev magicevent.Event) (any, error) {
	if s == nil || s.workspaceDeletions == nil || s.workspacePurger == nil || ev == nil || ev.Source() != workspaceDeletionCommandSource {
		return nil, common.ErrInvalidArgument
	}
	switch ev.ID() {
	case eventstreamevents.TopicEventStreamCommandSealWorkspaceDeletion:
		cmd, ok := ev.Data().(eventstreamevents.SealWorkspaceEventStreamDeletionCommand)
		if !ok || cmd.Validate() != nil {
			return eventstreamevents.SealWorkspaceEventStreamDeletionResult{}, common.ErrInvalidArgument
		}
		return s.sealWorkspaceDeletion(ctx, cmd.Scope())
	case eventstreamevents.TopicEventStreamCommandPurgeWorkspace:
		cmd, ok := ev.Data().(eventstreamevents.PurgeWorkspaceCommand)
		if !ok || cmd.Validate() != nil {
			return eventstreamevents.PurgeWorkspaceResult{}, common.ErrInvalidArgument
		}
		return s.purgeWorkspace(ctx, cmd.Scope())
	default:
		return nil, common.ErrInvalidArgument
	}
}

func (s *Biz) sealWorkspaceDeletion(ctx context.Context, scope deletioncontract.Scope) (eventstreamevents.SealWorkspaceEventStreamDeletionResult, error) {
	if err := scope.Validate(); err != nil {
		return eventstreamevents.SealWorkspaceEventStreamDeletionResult{}, err
	}
	s.appendMu.Lock()
	defer s.appendMu.Unlock()
	current, err := s.workspaceDeletions.Get(ctx, scope.WorkspaceID)
	if errors.Is(err, common.ErrNotFound) {
		current, err = s.dispatchWorkspaceDeletionInternalEvent(ctx, nil, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventFenceObserved, Scope: scope})
		if errors.Is(err, common.ErrConflict) {
			current, err = s.workspaceDeletions.Get(ctx, scope.WorkspaceID)
		}
	}
	if err != nil {
		return eventstreamevents.SealWorkspaceEventStreamDeletionResult{}, err
	}
	if !sameEventStreamDeletionScope(current, scope) {
		return eventstreamevents.SealWorkspaceEventStreamDeletionResult{}, common.ErrConflict
	}
	delete(s.revisions, scope.WorkspaceID)
	return eventstreamevents.SealWorkspaceEventStreamDeletionResult{
		WorkspaceID: current.WorkspaceID, DeletionID: current.DeletionID, DeletionGeneration: current.DeletionGeneration,
		FenceID: current.FenceID, Revision: 1, AckHash: current.SealAckHash,
	}, nil
}

func (s *Biz) purgeWorkspace(ctx context.Context, scope deletioncontract.Scope) (eventstreamevents.PurgeWorkspaceResult, error) {
	if err := scope.Validate(); err != nil || scope.Mode != deletioncontract.ModePurge {
		if err != nil {
			return eventstreamevents.PurgeWorkspaceResult{}, err
		}
		return eventstreamevents.PurgeWorkspaceResult{}, common.ErrInvalidArgument
	}
	s.appendMu.Lock()
	defer s.appendMu.Unlock()
	current, err := s.workspaceDeletions.Get(ctx, scope.WorkspaceID)
	if err != nil {
		return eventstreamevents.PurgeWorkspaceResult{}, err
	}
	if !sameEventStreamDeletionScope(current, scope) {
		return eventstreamevents.PurgeWorkspaceResult{}, common.ErrConflict
	}
	if current.Phase != eventstreammodel.WorkspaceDeletionPhasePurged {
		current, err = s.dispatchWorkspaceDeletionInternalEvent(ctx, current, workspaceDeletionInternalEvent{
			Kind: workspaceDeletionInternalEventPurgeRequested, Scope: scope,
		})
		if err != nil {
			return eventstreamevents.PurgeWorkspaceResult{}, err
		}
	}
	delete(s.revisions, scope.WorkspaceID)
	return eventstreamevents.PurgeWorkspaceResult{
		WorkspaceID: current.WorkspaceID, DeletionID: current.DeletionID, DeletionGeneration: current.DeletionGeneration,
		FenceID: current.FenceID, PurgeID: current.PurgeID, Revision: current.Revision,
		AckHash: current.PurgeAckHash, PurgeProofHash: current.PurgeProofHash,
	}, nil
}

func (s *Biz) dispatchWorkspaceDeletionInternalEvent(ctx context.Context, current *eventstreammodel.WorkspaceDeletion, event workspaceDeletionInternalEvent) (*eventstreammodel.WorkspaceDeletion, error) {
	if s == nil || s.workspaceDeletions == nil || s.workspacePurger == nil {
		return nil, common.ErrInvalidArgument
	}
	if event.Kind == workspaceDeletionInternalEventPurgeCompleted {
		return nil, common.ErrConflict
	}
	purgeRequested := event.Kind == workspaceDeletionInternalEventPurgeRequested
	if event.Kind == workspaceDeletionInternalEventPurgeRequested {
		scope := event.Scope.Normalize()
		if current == nil || current.Validate() != nil || current.Phase != eventstreammodel.WorkspaceDeletionPhaseFenced ||
			scope.Validate() != nil || scope.Mode != deletioncontract.ModePurge || !sameEventStreamDeletionScope(current, scope) {
			return nil, common.ErrConflict
		}
		persisted, err := s.workspaceDeletions.Get(ctx, scope.WorkspaceID)
		if err != nil {
			return nil, err
		}
		if persisted == nil || persisted.Validate() != nil {
			return nil, common.ErrConflict
		}
		if *persisted != *current {
			if sameEventStreamDeletionScope(persisted, scope) && persisted.Phase == eventstreammodel.WorkspaceDeletionPhasePurged {
				return persisted, nil
			}
			return nil, common.ErrConflict
		}
		if _, err := s.workspacePurger.PurgeWorkspaceEventRecords(ctx, *eventstreammodel.CloneWorkspaceDeletion(current)); err != nil {
			return s.resolveWorkspaceDeletionPurgeReplay(ctx, scope, err)
		}
		event = workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventPurgeCompleted, Scope: scope}
	}
	next, err := reduceWorkspaceDeletionInternalEvent(current, event, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		if purgeRequested {
			return s.resolveWorkspaceDeletionPurgeReplay(ctx, event.Scope, err)
		}
		return nil, err
	}
	if current == nil {
		err = s.workspaceDeletions.Create(ctx, next)
	} else {
		err = s.workspaceDeletions.CompareAndSwap(ctx, next, current.Revision)
	}
	if err != nil {
		if purgeRequested {
			return s.resolveWorkspaceDeletionPurgeReplay(ctx, event.Scope, err)
		}
		return nil, err
	}
	return next, nil
}

func (s *Biz) resolveWorkspaceDeletionPurgeReplay(ctx context.Context, scope deletioncontract.Scope, cause error) (*eventstreammodel.WorkspaceDeletion, error) {
	if !errors.Is(cause, common.ErrConflict) {
		return nil, cause
	}
	latest, err := s.workspaceDeletions.Get(ctx, scope.WorkspaceID)
	if err != nil || !sameEventStreamDeletionScope(latest, scope) || latest.Phase != eventstreammodel.WorkspaceDeletionPhasePurged {
		return nil, cause
	}
	return latest, nil
}

func reduceWorkspaceDeletionInternalEvent(current *eventstreammodel.WorkspaceDeletion, event workspaceDeletionInternalEvent, now string) (*eventstreammodel.WorkspaceDeletion, error) {
	scope := event.Scope.Normalize()
	if err := scope.Validate(); err != nil || now == "" {
		return nil, common.ErrInvalidArgument
	}
	switch event.Kind {
	case workspaceDeletionInternalEventFenceObserved:
		if current != nil {
			return nil, common.ErrConflict
		}
		next := &eventstreammodel.WorkspaceDeletion{
			WorkspaceID: scope.WorkspaceID, DeletionID: scope.DeletionID, DeletionGeneration: scope.DeletionGeneration,
			Mode: scope.Mode, QuiescenceID: scope.QuiescenceID, RequestHash: scope.RequestHash,
			FenceID: eventstreamevents.StableEventStreamDeletionFenceID(scope), Phase: eventstreammodel.WorkspaceDeletionPhaseFenced,
			Revision: 1, SealAckHash: eventstreamevents.StableEventStreamDeletionSealAckHash(scope), RequestedAt: now, UpdatedAt: now,
		}
		if err := next.Validate(); err != nil {
			return nil, err
		}
		return next, nil
	case workspaceDeletionInternalEventPurgeCompleted:
		if current == nil || current.Validate() != nil || current.Phase != eventstreammodel.WorkspaceDeletionPhaseFenced ||
			scope.Mode != deletioncontract.ModePurge || !sameEventStreamDeletionScope(current, scope) {
			return nil, common.ErrConflict
		}
		next := eventstreammodel.CloneWorkspaceDeletion(current)
		next.Phase = eventstreammodel.WorkspaceDeletionPhasePurged
		next.Revision = 2
		next.PurgeID = eventstreamevents.StableEventStreamDeletionPurgeID(scope)
		next.PurgeProofHash = eventstreamevents.StableEventStreamDeletionPurgeProofHash(scope)
		next.PurgeAckHash = eventstreamevents.StableEventStreamDeletionPurgeAckHash(scope)
		next.PurgedAt = now
		next.UpdatedAt = now
		if err := next.Validate(); err != nil {
			return nil, err
		}
		return next, nil
	default:
		return nil, common.ErrInvalidArgument
	}
}

func sameEventStreamDeletionScope(item *eventstreammodel.WorkspaceDeletion, scope deletioncontract.Scope) bool {
	return item != nil && item.Scope() == scope.Normalize()
}

package approval

import (
	"context"
	"errors"
	"sync"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
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

func (m *Biz) lockWorkspaceMutation(id common.WorkspaceID) func() {
	m.workspaceLocksMu.Lock()
	if m.workspaceLocks == nil {
		m.workspaceLocks = map[common.WorkspaceID]*sync.Mutex{}
	}
	l := m.workspaceLocks[id]
	if l == nil {
		l = &sync.Mutex{}
		m.workspaceLocks[id] = l
	}
	m.workspaceLocksMu.Unlock()
	l.Lock()
	return l.Unlock
}

func (m *Biz) lockWorkspaceLifecycleMutation(id common.WorkspaceID) func() {
	unlockWorkspace := m.lockWorkspaceMutation(id)
	m.lifecycleMu.Lock()
	return func() {
		m.lifecycleMu.Unlock()
		unlockWorkspace()
	}
}

func (m *Biz) rejectFencedWorkspace(ctx context.Context, id common.WorkspaceID) error {
	_, err := m.workspaceDeletions.Get(ctx, id)
	if errors.Is(err, common.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return common.ErrNotFound
}

// withWorkspaceActiveDelivery linearizes an outbound owner delivery with the
// durable Approval deletion fence. Callers must already hold the owner-local
// workspace mutation lock so the in-process seal/reconcile order and the
// repository-backed fence order have the same linearization point.
func (m *Biz) withWorkspaceActiveDelivery(ctx context.Context, id common.WorkspaceID, deliver func(context.Context) error) error {
	if m == nil || m.workspaceDeletions == nil || id == "" || deliver == nil {
		return common.ErrInvalidArgument
	}
	return m.workspaceDeletions.WithWorkspaceActiveFence(ctx, id, deliver)
}
func (m *Biz) executeSealWorkspaceDeletion(ctx context.Context, ev magicevent.Event) (approvalevents.SealWorkspaceApprovalDeletionResult, error) {
	cmd, ok := ev.Data().(approvalevents.SealWorkspaceApprovalDeletionCommand)
	if !ok || ev.Source() != workspaceDeletionCommandSource || cmd.Validate() != nil {
		return approvalevents.SealWorkspaceApprovalDeletionResult{}, common.ErrInvalidArgument
	}
	s := cmd.Scope()
	current, err := m.workspaceDeletions.Get(ctx, s.WorkspaceID)
	if errors.Is(err, common.ErrNotFound) {
		current, err = m.applyWorkspaceDeletionInternalEvent(ctx, nil, workspaceDeletionInternalEvent{workspaceDeletionInternalEventFenceObserved, s})
		if errors.Is(err, common.ErrConflict) {
			current, err = m.workspaceDeletions.Get(ctx, s.WorkspaceID)
		}
	}
	if err != nil {
		return approvalevents.SealWorkspaceApprovalDeletionResult{}, err
	}
	if current.Scope() != s {
		return approvalevents.SealWorkspaceApprovalDeletionResult{}, common.ErrConflict
	}
	return approvalevents.SealWorkspaceApprovalDeletionResult{WorkspaceID: current.WorkspaceID, DeletionID: current.DeletionID, DeletionGeneration: current.DeletionGeneration, FenceID: current.FenceID, Revision: 1, AckHash: current.SealAckHash}, nil
}
func (m *Biz) executePurgeWorkspaceCommand(ctx context.Context, ev magicevent.Event) (approvalevents.PurgeWorkspaceApprovalsResult, error) {
	cmd, ok := ev.Data().(approvalevents.PurgeWorkspaceApprovalsCommand)
	if !ok || ev.Source() != workspaceDeletionCommandSource || cmd.Validate() != nil {
		return approvalevents.PurgeWorkspaceApprovalsResult{}, common.ErrInvalidArgument
	}
	s := cmd.Scope()
	current, err := m.workspaceDeletions.Get(ctx, s.WorkspaceID)
	if err != nil {
		return approvalevents.PurgeWorkspaceApprovalsResult{}, err
	}
	if current.Scope() != s {
		return approvalevents.PurgeWorkspaceApprovalsResult{}, common.ErrConflict
	}
	if current.Phase != approvalmodel.WorkspaceDeletionPhasePurged {
		current, err = m.dispatchWorkspaceDeletionInternalEvent(ctx, current, workspaceDeletionInternalEvent{
			Kind: workspaceDeletionInternalEventPurgeRequested, Scope: s,
		})
		if errors.Is(err, common.ErrConflict) {
			current, err = m.workspaceDeletions.Get(ctx, s.WorkspaceID)
		}
		if err != nil {
			return approvalevents.PurgeWorkspaceApprovalsResult{}, err
		}
		if current.Scope() != s || current.Phase != approvalmodel.WorkspaceDeletionPhasePurged {
			return approvalevents.PurgeWorkspaceApprovalsResult{}, common.ErrConflict
		}
	}
	return approvalevents.PurgeWorkspaceApprovalsResult{WorkspaceID: current.WorkspaceID, DeletionID: current.DeletionID, DeletionGeneration: current.DeletionGeneration, FenceID: current.FenceID, PurgeID: current.PurgeID, Revision: current.Revision, AckHash: current.PurgeAckHash, PurgeProofHash: current.PurgeProofHash}, nil
}
func (m *Biz) applyWorkspaceDeletionInternalEvent(ctx context.Context, current *approvalmodel.WorkspaceDeletion, event workspaceDeletionInternalEvent) (*approvalmodel.WorkspaceDeletion, error) {
	return m.dispatchWorkspaceDeletionInternalEvent(ctx, current, event)
}

// dispatchWorkspaceDeletionInternalEvent is the sole Approval-owner path for
// the deletion aggregate and its privileged physical purge. Public command
// adapters may translate an exact Workspace intent into purge_requested, but
// cannot invoke the repository fence or purge helper directly. purge_completed
// is produced only after the fixed-point purge has succeeded.
func (m *Biz) dispatchWorkspaceDeletionInternalEvent(ctx context.Context, current *approvalmodel.WorkspaceDeletion, event workspaceDeletionInternalEvent) (*approvalmodel.WorkspaceDeletion, error) {
	if m == nil || m.workspaceDeletions == nil {
		return nil, common.ErrInvalidArgument
	}
	if event.Kind == workspaceDeletionInternalEventPurgeCompleted {
		return nil, common.ErrConflict
	}
	if event.Kind == workspaceDeletionInternalEventPurgeRequested {
		scope := event.Scope.Normalize()
		if current == nil || current.Validate() != nil || event.Scope != scope || scope.Validate() != nil ||
			scope.Mode != deletioncontract.ModePurge || current.Phase != approvalmodel.WorkspaceDeletionPhaseFenced || current.Scope() != scope {
			return nil, common.ErrConflict
		}
		if err := m.workspaceDeletions.WithWorkspacePurgeFence(ctx, scope, func(guarded context.Context) error {
			return m.purgeWorkspaceData(guarded, scope.WorkspaceID)
		}); err != nil {
			return nil, err
		}
		event = workspaceDeletionInternalEvent{
			Kind: workspaceDeletionInternalEventPurgeCompleted, Scope: scope,
		}
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
func reduceWorkspaceDeletionInternalEvent(current *approvalmodel.WorkspaceDeletion, event workspaceDeletionInternalEvent, now string) (*approvalmodel.WorkspaceDeletion, error) {
	if event.Scope.Validate() != nil || now == "" {
		return nil, common.ErrInvalidArgument
	}
	switch event.Kind {
	case workspaceDeletionInternalEventFenceObserved:
		if current != nil {
			return nil, common.ErrConflict
		}
		next := &approvalmodel.WorkspaceDeletion{WorkspaceID: event.Scope.WorkspaceID, DeletionID: event.Scope.DeletionID, DeletionGeneration: event.Scope.DeletionGeneration, Mode: event.Scope.Mode, QuiescenceID: event.Scope.QuiescenceID, RequestHash: event.Scope.RequestHash, FenceID: approvalevents.StableApprovalDeletionFenceID(event.Scope), Phase: approvalmodel.WorkspaceDeletionPhaseFenced, Revision: 1, SealAckHash: approvalevents.StableApprovalDeletionSealAckHash(event.Scope), RequestedAt: now, UpdatedAt: now}
		return next, nil
	case workspaceDeletionInternalEventPurgeCompleted:
		if current == nil || current.Phase != approvalmodel.WorkspaceDeletionPhaseFenced || current.Scope() != event.Scope.Normalize() {
			return nil, common.ErrConflict
		}
		next := approvalmodel.CloneWorkspaceDeletion(current)
		next.Phase = approvalmodel.WorkspaceDeletionPhasePurged
		next.Revision = 2
		next.PurgeID = approvalevents.StableApprovalDeletionPurgeID(event.Scope)
		next.PurgeProofHash = approvalevents.StableApprovalDeletionPurgeProofHash(event.Scope)
		next.PurgeAckHash = approvalevents.StableApprovalDeletionPurgeAckHash(event.Scope)
		next.PurgedAt = now
		next.UpdatedAt = now
		return next, nil
	}
	return nil, common.ErrInvalidArgument
}
func (m *Biz) purgeWorkspaceData(ctx context.Context, id common.WorkspaceID) error {
	for pass := 0; pass < 1000; pass++ {
		items, err := m.approvals.ListByWorkspaceID(ctx, id)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		for _, item := range items {
			if item == nil || item.WorkspaceID != id {
				return common.ErrConflict
			}
			if err := m.approvals.DeleteScoped(ctx, item.WorkspaceID, item.RunID, item.ID); err != nil && !errors.Is(err, common.ErrNotFound) {
				return err
			}
		}
	}
	return common.ErrConflict
}

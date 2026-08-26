package orchestrator

import (
	"context"
	"time"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	deletioncontract "github.com/muidea/workorch/internal/pkg/workorchdeletioncontract"
)

type workspaceDeletionInternalEventKind string

const (
	workspaceDeletionInternalEventQuiescingStarted workspaceDeletionInternalEventKind = "quiescing_started"
	workspaceDeletionInternalEventQuiesced         workspaceDeletionInternalEventKind = "quiesced"
	workspaceDeletionInternalEventFenceObserved    workspaceDeletionInternalEventKind = "fence_observed"
	workspaceDeletionInternalEventPurgeRequested   workspaceDeletionInternalEventKind = "purge_requested"
	workspaceDeletionInternalEventPurgeCompleted   workspaceDeletionInternalEventKind = "purge_completed"
)

type workspaceDeletionInternalEvent struct {
	Kind         workspaceDeletionInternalEventKind
	Intent       deletioncontract.WorkspaceRuntimeQuiescenceIntent
	GateID       string
	Token        *deletioncontract.WorkspaceRuntimeQuiescenceToken
	Scope        deletioncontract.Scope
	PurgeReceipt *repository.OrchestratorWorkspaceEvidencePurgeReceipt
}

type workspaceDeletionInternalEventResult struct {
	Deletion     *orchestratormodel.WorkspaceDeletion
	PurgeReceipt *repository.OrchestratorWorkspaceEvidencePurgeReceipt
}

func (m *Biz) applyWorkspaceDeletionInternalEvent(ctx context.Context, current *orchestratormodel.WorkspaceDeletion, event workspaceDeletionInternalEvent) (*orchestratormodel.WorkspaceDeletion, error) {
	result, err := m.dispatchWorkspaceDeletionInternalEvent(ctx, current, event)
	return result.Deletion, err
}

// dispatchWorkspaceDeletionInternalEvent is the only owner-private transition
// dispatcher for both the durable deletion aggregate and its physical evidence
// purge journal. A command adapter cannot invoke the purge sink directly.
func (m *Biz) dispatchWorkspaceDeletionInternalEvent(ctx context.Context, current *orchestratormodel.WorkspaceDeletion, event workspaceDeletionInternalEvent) (workspaceDeletionInternalEventResult, error) {
	if m == nil || m.workspaceDeletions == nil {
		return workspaceDeletionInternalEventResult{}, common.ErrInvalidArgument
	}
	// Purge completion is an owner-private consequence of the physical purge
	// sink below. Reject a directly constructed completion so an in-package
	// caller cannot forge a valid-looking receipt and advance the tombstone
	// without executing the owner effect.
	if event.Kind == workspaceDeletionInternalEventPurgeCompleted {
		return workspaceDeletionInternalEventResult{}, common.ErrConflict
	}
	var purgeReceipt *repository.OrchestratorWorkspaceEvidencePurgeReceipt
	if event.Kind == workspaceDeletionInternalEventPurgeRequested {
		if m.workspacePurger == nil || current == nil || current.Validate() != nil ||
			current.Phase != orchestratormodel.WorkspaceDeletionPhaseFenced || event.Scope.Validate() != nil ||
			current.Scope() != event.Scope || current.Mode != deletioncontract.ModePurge ||
			event.Intent != (deletioncontract.WorkspaceRuntimeQuiescenceIntent{}) || event.GateID != "" || event.Token != nil || event.PurgeReceipt != nil {
			return workspaceDeletionInternalEventResult{}, common.ErrConflict
		}
		receipt, err := m.workspacePurger.PurgeWorkspaceEvidence(ctx, event.Scope)
		if err != nil {
			return workspaceDeletionInternalEventResult{}, err
		}
		if err := receipt.ValidateFor(event.Scope); err != nil {
			return workspaceDeletionInternalEventResult{}, err
		}
		event = workspaceDeletionInternalEvent{
			Kind: workspaceDeletionInternalEventPurgeCompleted, Scope: event.Scope, PurgeReceipt: &receipt,
		}
		cloned := receipt.Clone()
		purgeReceipt = &cloned
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	next, err := reduceWorkspaceDeletionInternalEvent(current, event, now)
	if err != nil {
		return workspaceDeletionInternalEventResult{}, err
	}
	if current == nil {
		err = m.workspaceDeletions.Create(ctx, next)
	} else {
		err = m.workspaceDeletions.CompareAndSwap(ctx, next, current.Revision)
	}
	if err != nil {
		return workspaceDeletionInternalEventResult{}, err
	}
	return workspaceDeletionInternalEventResult{Deletion: next, PurgeReceipt: purgeReceipt}, nil
}

func reduceWorkspaceDeletionInternalEvent(current *orchestratormodel.WorkspaceDeletion, event workspaceDeletionInternalEvent, now string) (*orchestratormodel.WorkspaceDeletion, error) {
	if now == "" {
		return nil, common.ErrInvalidArgument
	}
	switch event.Kind {
	case workspaceDeletionInternalEventQuiescingStarted:
		intent := event.Intent.Normalized()
		if current != nil || intent.Validate() != nil || event.GateID != orchestratorevents.StableWorkspaceRuntimeQuiescingGateID(intent) || event.Token != nil || event.Scope != (deletioncontract.Scope{}) || event.PurgeReceipt != nil {
			return nil, common.ErrConflict
		}
		next := &orchestratormodel.WorkspaceDeletion{
			WorkspaceID: intent.WorkspaceID, DeletionID: intent.DeletionID, DeletionGeneration: intent.DeletionGeneration,
			Mode: intent.DataDispositionIntent, WorkspaceIntentHash: intent.WorkspaceIntentHash,
			RunDrainingRevision: intent.RunDrainingRevision, WorkspaceRunGeneration: intent.WorkspaceRunGeneration,
			QuiescingGateID: event.GateID, Phase: orchestratormodel.WorkspaceDeletionPhaseQuiescing, Revision: 1,
			RequestedAt: now, UpdatedAt: now,
		}
		return next, next.Validate()
	case workspaceDeletionInternalEventQuiesced:
		intent := event.Intent.Normalized()
		if current == nil || current.Phase != orchestratormodel.WorkspaceDeletionPhaseQuiescing || current.Revision != 1 || current.QuiescenceIntent() != intent ||
			event.GateID != current.QuiescingGateID || event.Token == nil || event.Token.ValidateFor(intent) != nil || event.Scope != (deletioncontract.Scope{}) || event.PurgeReceipt != nil {
			return nil, common.ErrConflict
		}
		next := orchestratormodel.CloneWorkspaceDeletion(current)
		token := deletioncontract.CloneWorkspaceRuntimeQuiescenceToken(*event.Token)
		next.Phase = orchestratormodel.WorkspaceDeletionPhaseQuiesced
		next.Revision = 2
		next.QuiescenceToken = &token
		next.QuiescenceID = token.QuiescenceID
		next.RequestHash = deletioncontract.StableRequestHash(next.Scope())
		next.UpdatedAt = now
		return next, next.Validate()
	case workspaceDeletionInternalEventFenceObserved:
		if event.Scope.Validate() != nil || current == nil || current.Phase != orchestratormodel.WorkspaceDeletionPhaseQuiesced || current.Scope() != event.Scope || event.Intent != (deletioncontract.WorkspaceRuntimeQuiescenceIntent{}) || event.GateID != "" || event.Token != nil || event.PurgeReceipt != nil {
			return nil, common.ErrConflict
		}
		next := orchestratormodel.CloneWorkspaceDeletion(current)
		next.FenceID = orchestratorevents.StableOrchestratorDeletionFenceID(event.Scope)
		next.Phase = orchestratormodel.WorkspaceDeletionPhaseFenced
		next.Revision = 3
		next.SealAckHash = orchestratorevents.StableOrchestratorDeletionSealAckHash(event.Scope)
		next.UpdatedAt = now
		return next, next.Validate()
	case workspaceDeletionInternalEventPurgeCompleted:
		if event.Scope.Validate() != nil || current == nil || current.Phase != orchestratormodel.WorkspaceDeletionPhaseFenced || current.Scope() != event.Scope || current.Mode != deletioncontract.ModePurge || event.Intent != (deletioncontract.WorkspaceRuntimeQuiescenceIntent{}) || event.GateID != "" || event.Token != nil || event.PurgeReceipt == nil || event.PurgeReceipt.ValidateFor(event.Scope) != nil {
			return nil, common.ErrConflict
		}
		next := orchestratormodel.CloneWorkspaceDeletion(current)
		next.Phase = orchestratormodel.WorkspaceDeletionPhasePurged
		next.Revision = 4
		next.PurgeID = orchestratorevents.StableOrchestratorDeletionPurgeID(event.Scope)
		next.PurgeProofHash = event.PurgeReceipt.ProofHash
		next.PurgeAckHash = orchestratorevents.StableOrchestratorDeletionPurgeAckHash(event.Scope, event.PurgeReceipt.ProofHash)
		next.PurgedAt = now
		next.UpdatedAt = now
		return next, next.Validate()
	default:
		return nil, common.ErrInvalidArgument
	}
}

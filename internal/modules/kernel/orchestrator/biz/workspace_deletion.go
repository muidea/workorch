package orchestrator

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (m *Biz) executeWorkspaceDeletionCommand(ctx context.Context, event magicevent.Event) (any, error) {
	if event == nil || event.Source() != workspaceRetirementOrchestratorCommandSource || m == nil || m.workspaceDeletions == nil {
		return nil, common.ErrInvalidArgument
	}
	switch event.ID() {
	case orchestratorevents.TopicOrchestratorCommandSealWorkspaceDeletion:
		command, ok := event.Data().(orchestratorevents.SealWorkspaceOrchestratorDeletionCommand)
		if !ok || command.Validate() != nil {
			return orchestratorevents.SealWorkspaceOrchestratorDeletionResult{}, common.ErrInvalidArgument
		}
		return m.sealWorkspaceDeletion(ctx, command)
	case orchestratorevents.TopicOrchestratorCommandPurgeWorkspaceEvidence:
		command, ok := event.Data().(orchestratorevents.PurgeWorkspaceOrchestratorEvidenceCommand)
		if !ok || command.Validate() != nil {
			return orchestratorevents.PurgeWorkspaceOrchestratorEvidenceResult{}, common.ErrInvalidArgument
		}
		return m.purgeWorkspaceOrchestratorEvidence(ctx, command)
	default:
		return nil, common.ErrInvalidArgument
	}
}

func (m *Biz) sealWorkspaceDeletion(ctx context.Context, command orchestratorevents.SealWorkspaceOrchestratorDeletionCommand) (orchestratorevents.SealWorkspaceOrchestratorDeletionResult, error) {
	scope := command.Scope()
	current, err := m.workspaceDeletions.Get(ctx, scope.WorkspaceID)
	if err != nil {
		return orchestratorevents.SealWorkspaceOrchestratorDeletionResult{}, err
	}
	if current.Scope() != scope {
		return orchestratorevents.SealWorkspaceOrchestratorDeletionResult{}, common.ErrConflict
	}
	if err := m.validateWorkspaceDeletionRetiredFact(ctx, current); err != nil {
		return orchestratorevents.SealWorkspaceOrchestratorDeletionResult{}, err
	}
	if current.Phase == orchestratormodel.WorkspaceDeletionPhaseQuiesced {
		current, err = m.applyWorkspaceDeletionInternalEvent(ctx, current, workspaceDeletionInternalEvent{Kind: workspaceDeletionInternalEventFenceObserved, Scope: scope})
		if err != nil {
			return orchestratorevents.SealWorkspaceOrchestratorDeletionResult{}, err
		}
	}
	if current.Phase != orchestratormodel.WorkspaceDeletionPhaseFenced && current.Phase != orchestratormodel.WorkspaceDeletionPhasePurged {
		return orchestratorevents.SealWorkspaceOrchestratorDeletionResult{}, common.ErrConflict
	}
	result := orchestratorevents.SealWorkspaceOrchestratorDeletionResult{
		WorkspaceID: current.WorkspaceID, DeletionID: current.DeletionID, DeletionGeneration: current.DeletionGeneration,
		FenceID: current.FenceID, Revision: 1, AckHash: current.SealAckHash,
	}
	if err := result.ValidateFor(command); err != nil {
		return orchestratorevents.SealWorkspaceOrchestratorDeletionResult{}, err
	}
	return result, nil
}

func (m *Biz) purgeWorkspaceOrchestratorEvidence(ctx context.Context, command orchestratorevents.PurgeWorkspaceOrchestratorEvidenceCommand) (orchestratorevents.PurgeWorkspaceOrchestratorEvidenceResult, error) {
	scope := command.Scope()
	current, err := m.workspaceDeletions.Get(ctx, scope.WorkspaceID)
	if err != nil {
		return orchestratorevents.PurgeWorkspaceOrchestratorEvidenceResult{}, err
	}
	if current.Scope() != scope {
		return orchestratorevents.PurgeWorkspaceOrchestratorEvidenceResult{}, common.ErrConflict
	}
	if err := m.validateWorkspaceDeletionRetiredFact(ctx, current); err != nil {
		return orchestratorevents.PurgeWorkspaceOrchestratorEvidenceResult{}, err
	}
	if current.Phase != orchestratormodel.WorkspaceDeletionPhaseFenced && current.Phase != orchestratormodel.WorkspaceDeletionPhasePurged {
		return orchestratorevents.PurgeWorkspaceOrchestratorEvidenceResult{}, common.ErrConflict
	}
	if current.Phase != orchestratormodel.WorkspaceDeletionPhasePurged {
		transition, transitionErr := m.dispatchWorkspaceDeletionInternalEvent(ctx, current, workspaceDeletionInternalEvent{
			Kind: workspaceDeletionInternalEventPurgeRequested, Scope: scope,
		})
		err = transitionErr
		if err != nil {
			return orchestratorevents.PurgeWorkspaceOrchestratorEvidenceResult{}, err
		}
		if transition.Deletion == nil || transition.PurgeReceipt == nil || transition.PurgeReceipt.ProofHash != transition.Deletion.PurgeProofHash {
			return orchestratorevents.PurgeWorkspaceOrchestratorEvidenceResult{}, common.ErrConflict
		}
		current = transition.Deletion
	}
	result := orchestratorevents.PurgeWorkspaceOrchestratorEvidenceResult{
		WorkspaceID: current.WorkspaceID, DeletionID: current.DeletionID, DeletionGeneration: current.DeletionGeneration,
		FenceID: current.FenceID, PurgeID: current.PurgeID, Revision: 2,
		AckHash: current.PurgeAckHash, PurgeProofHash: current.PurgeProofHash,
	}
	if err := result.ValidateFor(command); err != nil {
		return orchestratorevents.PurgeWorkspaceOrchestratorEvidenceResult{}, err
	}
	return result, nil
}

func (m *Biz) validateWorkspaceDeletionRetiredFact(ctx context.Context, current *orchestratormodel.WorkspaceDeletion) error {
	if m == nil || current == nil || current.Validate() != nil || current.QuiescenceToken == nil || m.EventHub() == nil {
		return common.ErrInvalidArgument
	}
	retirement, err := queryRunWorkspaceRetirementFact(ctx, m.EventHub(), current.WorkspaceID, current.DeletionID)
	if err != nil {
		return err
	}
	fence := retirement.Fence.Normalized()
	token := current.QuiescenceToken.Normalized()
	if !runWorkspaceRetirementFenceIsRetired(fence) || fence.WorkspaceID != current.WorkspaceID || fence.DeletionID != current.DeletionID ||
		fence.DeletionGeneration != current.DeletionGeneration || fence.DataDispositionIntent != current.Mode ||
		fence.WorkspaceIntentHash != current.WorkspaceIntentHash || fence.FenceRevision != current.RunDrainingRevision+1 ||
		fence.WorkspaceRunGeneration != current.WorkspaceRunGeneration || fence.QuiescenceID != token.QuiescenceID ||
		fence.RuntimeInventoryHash != token.RuntimeInventoryHash {
		return common.ErrConflict
	}
	return nil
}

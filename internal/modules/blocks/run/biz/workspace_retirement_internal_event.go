package run

import (
	"context"

	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type workspaceRetirementInternalEventKind string

const (
	workspaceRetirementInternalEventPrepare  workspaceRetirementInternalEventKind = "prepare"
	workspaceRetirementInternalEventFinalize workspaceRetirementInternalEventKind = "finalize"
	workspaceRetirementInternalEventPurge    workspaceRetirementInternalEventKind = "purge"
)

type workspaceRetirementInternalEvent struct {
	Kind     workspaceRetirementInternalEventKind
	Prepare  *runevents.PrepareWorkspaceRetirementCommand
	Finalize *runevents.FinalizeWorkspaceRetirementCommand
	Purge    *runevents.PurgeWorkspaceRunsCommand
}

type workspaceRetirementInternalResult struct {
	Fence        *runmodel.WorkspaceRetirementFence
	PurgeReceipt *repository.RunWorkspaceResourcePurgeReceipt
}

// submitWorkspaceRetirementInternalEvent is the single Run-owner dispatcher
// for the durable draining -> retired resource state and its subordinate
// canonical three-surface purge transaction. Cross-owner commands are first
// validated and translated to this private event by workspace_retirement.go.
func (m *Biz) submitWorkspaceRetirementInternalEvent(ctx context.Context, event workspaceRetirementInternalEvent) (workspaceRetirementInternalResult, error) {
	if m == nil || m.runs == nil {
		return workspaceRetirementInternalResult{}, common.ErrInvalidArgument
	}
	switch event.Kind {
	case workspaceRetirementInternalEventPrepare:
		if event.Prepare == nil || event.Finalize != nil || event.Purge != nil {
			return workspaceRetirementInternalResult{}, common.ErrInvalidArgument
		}
		command := event.Prepare.Normalized()
		if err := command.Validate(); err != nil {
			return workspaceRetirementInternalResult{}, err
		}
		fence, err := m.runs.PrepareWorkspaceRetirement(ctx, runmodel.WorkspaceRetirementIntent{
			WorkspaceID: command.WorkspaceID, DeletionID: command.DeletionID, DeletionGeneration: command.DeletionGeneration,
			DataDispositionIntent: command.DataDispositionIntent, WorkspaceIntentHash: command.WorkspaceIntentHash,
		})
		return workspaceRetirementInternalResult{Fence: fence}, err
	case workspaceRetirementInternalEventFinalize:
		if event.Finalize == nil || event.Prepare != nil || event.Purge != nil {
			return workspaceRetirementInternalResult{}, common.ErrInvalidArgument
		}
		command := event.Finalize.Normalized()
		if err := command.Validate(); err != nil {
			return workspaceRetirementInternalResult{}, err
		}
		fence, err := m.runs.FinalizeWorkspaceRetirement(ctx, command.QuiescenceToken)
		return workspaceRetirementInternalResult{Fence: fence}, err
	case workspaceRetirementInternalEventPurge:
		if event.Purge == nil || event.Prepare != nil || event.Finalize != nil {
			return workspaceRetirementInternalResult{}, common.ErrInvalidArgument
		}
		command := event.Purge.Normalized()
		if err := command.Validate(); err != nil {
			return workspaceRetirementInternalResult{}, err
		}
		fence, err := m.runs.GetWorkspaceRetirement(ctx, command.WorkspaceID)
		if err != nil {
			return workspaceRetirementInternalResult{}, err
		}
		expected := runWorkspaceRetirementFenceToModel(command.RetirementFence())
		if !fence.IsRetired() || !runmodel.SameWorkspaceRetirementFence(*fence, expected) {
			return workspaceRetirementInternalResult{}, common.ErrConflict
		}
		if m.workspacePurger == nil {
			return workspaceRetirementInternalResult{}, common.ErrConflict
		}
		receipt, err := m.workspacePurger.PurgeWorkspaceResources(ctx, expected)
		if err != nil || receipt.ValidateFor(expected) != nil {
			if err != nil {
				return workspaceRetirementInternalResult{}, err
			}
			return workspaceRetirementInternalResult{}, common.ErrConflict
		}
		if receipt.ProofHash != runmodel.StableWorkspaceRetirementPurgeProofHash(expected, receipt.Observations) {
			return workspaceRetirementInternalResult{}, common.ErrConflict
		}
		return workspaceRetirementInternalResult{Fence: fence, PurgeReceipt: &receipt}, nil
	default:
		return workspaceRetirementInternalResult{}, common.ErrInvalidArgument
	}
}

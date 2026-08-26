package memory

import (
	"context"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type runWorkspaceResourcePurger struct {
	runs        *RunRepository
	steps       *StepRepository
	checkpoints *RuntimeCheckpointRepository
}

func NewRunOwnerRepositorySet() repository.RunOwnerRepositories {
	runs := NewRunRepository()
	steps := NewRunGuardedStepRepository(runs)
	checkpoints := NewRunGuardedRuntimeCheckpointRepository(runs)
	purger := &runWorkspaceResourcePurger{runs: runs, steps: steps, checkpoints: checkpoints}
	owner, err := repository.NewRunOwnerRepositories(runs, steps, checkpoints, purger)
	if err != nil {
		panic(err)
	}
	return owner
}

func (p *runWorkspaceResourcePurger) RunOwnerRepositoryBinding() repository.RunOwnerRepositoryBinding {
	if p == nil || p.runs == nil {
		return repository.RunOwnerRepositoryBinding{}
	}
	return p.runs.ownerBinding
}

func (p *runWorkspaceResourcePurger) PurgeWorkspaceResources(ctx context.Context, expected runmodel.WorkspaceRetirementFence) (repository.RunWorkspaceResourcePurgeReceipt, error) {
	expected = expected.Normalized()
	if p == nil || p.runs == nil || p.steps == nil || p.checkpoints == nil || expected.Validate() != nil || !expected.IsRetired() || expected.DataDispositionIntent != "purge" {
		return repository.RunWorkspaceResourcePurgeReceipt{}, common.ErrInvalidArgument
	}
	if err := validateRuntimeWorkspaceScope(ctx, expected.WorkspaceID); err != nil {
		return repository.RunWorkspaceResourcePurgeReceipt{}, err
	}
	p.runs.mu.Lock()
	defer p.runs.mu.Unlock()
	current, ok := p.runs.workspaceRetirements[expected.WorkspaceID]
	if !ok || !current.IsRetired() || !runmodel.SameWorkspaceRetirementFence(current, expected) {
		return repository.RunWorkspaceResourcePurgeReceipt{}, common.ErrConflict
	}
	if existing, ok := p.runs.workspacePurgeReceipts[expected.WorkspaceID]; ok {
		if err := existing.ValidateFor(expected); err != nil {
			return repository.RunWorkspaceResourcePurgeReceipt{}, common.ErrConflict
		}
		return existing.Clone(), nil
	}

	// Ordinary mutations take the Run lock before their child repository lock;
	// use the same order so retirement and all three physical surfaces are one
	// linearized owner transaction.
	p.steps.mu.Lock()
	defer p.steps.mu.Unlock()
	p.checkpoints.mu.Lock()
	defer p.checkpoints.mu.Unlock()

	observedRuns := uint64(0)
	observedSteps := uint64(0)
	observedCheckpoints := uint64(0)
	for key := range p.steps.items {
		if workspaceFromScopedKey(key) == expected.WorkspaceID {
			observedSteps++
			delete(p.steps.items, key)
		}
	}
	for key := range p.checkpoints.items {
		if workspaceFromScopedKey(key) == expected.WorkspaceID {
			observedCheckpoints++
			delete(p.checkpoints.items, key)
		}
	}
	for key := range p.runs.items {
		if workspaceFromScopedKey(key) == expected.WorkspaceID {
			observedRuns++
			delete(p.runs.items, key)
		}
	}
	for key := range p.steps.items {
		if workspaceFromScopedKey(key) == expected.WorkspaceID {
			return repository.RunWorkspaceResourcePurgeReceipt{}, common.ErrConflict
		}
	}
	for key := range p.checkpoints.items {
		if workspaceFromScopedKey(key) == expected.WorkspaceID {
			return repository.RunWorkspaceResourcePurgeReceipt{}, common.ErrConflict
		}
	}
	for key := range p.runs.items {
		if workspaceFromScopedKey(key) == expected.WorkspaceID {
			return repository.RunWorkspaceResourcePurgeReceipt{}, common.ErrConflict
		}
	}
	receipt, err := repository.NewRunWorkspaceResourcePurgeReceipt(expected, []runmodel.WorkspaceRetirementPurgeObservation{
		{Surface: runmodel.WorkspaceRetirementPurgeSurfaceRuns, ObservedBefore: observedRuns, DeletedCount: observedRuns},
		{Surface: runmodel.WorkspaceRetirementPurgeSurfaceSteps, ObservedBefore: observedSteps, DeletedCount: observedSteps},
		{Surface: runmodel.WorkspaceRetirementPurgeSurfaceCheckpoints, ObservedBefore: observedCheckpoints, DeletedCount: observedCheckpoints},
	})
	if err != nil {
		return repository.RunWorkspaceResourcePurgeReceipt{}, err
	}
	// The receipt belongs to the same Run-owner transaction as the physical
	// deletion. Keeping it beside the permanent retired fence makes a
	// response-loss retry an exact replay rather than a newly invented empty
	// proof.
	p.runs.workspacePurgeReceipts[expected.WorkspaceID] = receipt.Clone()
	return receipt, nil
}

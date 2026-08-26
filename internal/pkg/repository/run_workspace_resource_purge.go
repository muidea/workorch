package repository

import (
	"context"

	run "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// RunOwnerRepositoryBinding is a process-local construction identity. Backend
// factories mint one token per Run/Step/Checkpoint owner set, and the
// canonical bundle rejects repositories assembled from different sets before
// any deletion command can run.
type RunOwnerRepositoryBinding struct {
	token *runOwnerRepositoryBindingToken
}

// Keep the token non-zero-sized: Go permits distinct zero-sized allocations
// to share an address, which would make independently constructed owner sets
// compare equal and defeat the mixed-set guard.
type runOwnerRepositoryBindingToken struct{ marker byte }

func NewRunOwnerRepositoryBinding() RunOwnerRepositoryBinding {
	return RunOwnerRepositoryBinding{token: &runOwnerRepositoryBindingToken{}}
}

func (b RunOwnerRepositoryBinding) Valid() bool { return b.token != nil }

func (b RunOwnerRepositoryBinding) Same(other RunOwnerRepositoryBinding) bool {
	return b.token != nil && b.token == other.token
}

// RunOwnerRepositoryBound exposes only an opaque construction identity, not a
// repository mutation capability.
type RunOwnerRepositoryBound interface {
	RunOwnerRepositoryBinding() RunOwnerRepositoryBinding
}

type RunWorkspaceResourcePurgeReceipt struct {
	Fence        run.WorkspaceRetirementFence              `json:"fence"`
	Observations []run.WorkspaceRetirementPurgeObservation `json:"observations"`
	ProofHash    string                                    `json:"proofHash"`
}

func NewRunWorkspaceResourcePurgeReceipt(fence run.WorkspaceRetirementFence, observations []run.WorkspaceRetirementPurgeObservation) (RunWorkspaceResourcePurgeReceipt, error) {
	fence = fence.Normalized()
	if err := fence.Validate(); err != nil || !fence.IsRetired() || fence.DataDispositionIntent != "purge" {
		return RunWorkspaceResourcePurgeReceipt{}, common.ErrInvalidArgument
	}
	ret := RunWorkspaceResourcePurgeReceipt{
		Fence: fence, Observations: append([]run.WorkspaceRetirementPurgeObservation(nil), observations...),
		ProofHash: run.StableWorkspaceRetirementPurgeProofHash(fence, observations),
	}
	if err := ret.ValidateFor(fence); err != nil {
		return RunWorkspaceResourcePurgeReceipt{}, err
	}
	return ret, nil
}

func (r RunWorkspaceResourcePurgeReceipt) Clone() RunWorkspaceResourcePurgeReceipt {
	r.Observations = append([]run.WorkspaceRetirementPurgeObservation(nil), r.Observations...)
	return r
}

func (r RunWorkspaceResourcePurgeReceipt) ValidateFor(fence run.WorkspaceRetirementFence) error {
	fence = fence.Normalized()
	if err := fence.Validate(); err != nil || !fence.IsRetired() || fence.DataDispositionIntent != "purge" {
		return common.ErrInvalidArgument
	}
	if !run.SameWorkspaceRetirementFence(r.Fence, fence) {
		return common.ErrConflict
	}
	if err := run.ValidateEmptyWorkspaceRetirementPurgeObservations(r.Observations); err != nil {
		return err
	}
	if r.ProofHash != run.StableWorkspaceRetirementPurgeProofHash(fence, r.Observations) {
		return common.ErrConflict
	}
	return nil
}

// RunWorkspaceResourcePurger is the sole Run-owner physical purge lane. It
// validates the exact retired fence and empties all three Run-owned surfaces
// under one backend owner lock before returning a typed empty receipt.
type RunWorkspaceResourcePurger interface {
	RunOwnerRepositoryBound
	PurgeWorkspaceResources(context.Context, run.WorkspaceRetirementFence) (RunWorkspaceResourcePurgeReceipt, error)
}

// RunOwnerRepositories is the only production construction surface for the
// Run/Step/Checkpoint repository set. Focused tests may inject individual
// repositories, but Workspace purge is unavailable without this exact bundle.
type RunOwnerRepositories struct {
	runs        RunRepository
	steps       StepRepository
	checkpoints RuntimeCheckpointRepository
	purger      RunWorkspaceResourcePurger
	binding     RunOwnerRepositoryBinding
}

func NewRunOwnerRepositories(runs RunRepository, steps StepRepository, checkpoints RuntimeCheckpointRepository, purger RunWorkspaceResourcePurger) (RunOwnerRepositories, error) {
	if runs == nil || steps == nil || checkpoints == nil || purger == nil {
		return RunOwnerRepositories{}, common.ErrInvalidArgument
	}
	runBound, runOK := runs.(RunOwnerRepositoryBound)
	stepBound, stepOK := steps.(RunOwnerRepositoryBound)
	checkpointBound, checkpointOK := checkpoints.(RunOwnerRepositoryBound)
	if !runOK || !stepOK || !checkpointOK {
		return RunOwnerRepositories{}, common.ErrConflict
	}
	binding := runBound.RunOwnerRepositoryBinding()
	if !binding.Valid() || !binding.Same(stepBound.RunOwnerRepositoryBinding()) ||
		!binding.Same(checkpointBound.RunOwnerRepositoryBinding()) || !binding.Same(purger.RunOwnerRepositoryBinding()) {
		return RunOwnerRepositories{}, common.ErrConflict
	}
	return RunOwnerRepositories{runs: runs, steps: steps, checkpoints: checkpoints, purger: purger, binding: binding}, nil
}

func (r RunOwnerRepositories) Validate() error {
	if r.runs == nil || r.steps == nil || r.checkpoints == nil || r.purger == nil || !r.binding.Valid() {
		return common.ErrInvalidArgument
	}
	for _, bound := range []RunOwnerRepositoryBound{
		r.runs.(RunOwnerRepositoryBound), r.steps.(RunOwnerRepositoryBound), r.checkpoints.(RunOwnerRepositoryBound), r.purger,
	} {
		if !r.binding.Same(bound.RunOwnerRepositoryBinding()) {
			return common.ErrConflict
		}
	}
	return nil
}

func (r RunOwnerRepositories) Present() bool {
	return r.runs != nil || r.steps != nil || r.checkpoints != nil || r.purger != nil || r.binding.Valid()
}

func (r RunOwnerRepositories) Runs() RunRepository { return r.runs }

func (r RunOwnerRepositories) Steps() StepRepository { return r.steps }

func (r RunOwnerRepositories) Checkpoints() RuntimeCheckpointRepository { return r.checkpoints }

func (r RunOwnerRepositories) WorkspacePurger() RunWorkspaceResourcePurger { return r.purger }

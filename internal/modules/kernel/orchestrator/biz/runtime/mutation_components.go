package runtime

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

// runtimeMutationKernel is the only runtime-local object that owns the
// repository port and the transition coordinator.  The coordinator remains
// the sole CAS boundary; the kernel only supplies the narrow load/commit
// plumbing needed by the private state-machine components.
//
// It is intentionally embedded by RuntimeController and the private
// components.  Embedding keeps the existing package-private reducer code
// source-compatible while making ownership explicit: no component receives a
// second repository or coordinator instance.
type runtimeMutationKernel struct {
	repository  RuntimeStateRepository
	transitions *RuntimeTransitionCoordinator
}

func newRuntimeMutationKernel(repository RuntimeStateRepository, transitions *RuntimeTransitionCoordinator) (*runtimeMutationKernel, error) {
	if repository == nil || transitions == nil {
		return nil, common.ErrInvalidArgument
	}
	return &runtimeMutationKernel{repository: repository, transitions: transitions}, nil
}

func (k *runtimeMutationKernel) load(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID) (RuntimeAggregate, error) {
	if k == nil || k.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	return k.repository.Load(ctx, workspaceID, runID)
}

func (k *runtimeMutationKernel) commit(ctx context.Context, aggregate RuntimeAggregate, expectedRevision uint64) error {
	if k == nil || k.transitions == nil {
		return common.ErrInvalidArgument
	}
	return k.transitions.Commit(ctx, aggregate, expectedRevision)
}

// The following components are deliberately private and stay in this Go
// package.  They share the kernel but never hold one another, an aggregate, a
// lifecycle handle, or an EventHub.  A handler is therefore owned by exactly
// one state-machine component while persistence remains centralized.
type authorityComponent struct {
	*runtimeMutationKernel
}

type planningTransactionComponent struct {
	*runtimeMutationKernel
}

type candidateDispatchComponent struct {
	*runtimeMutationKernel
}

type driveDeliveryComponent struct {
	*runtimeMutationKernel
}

func newAuthorityComponent(kernel *runtimeMutationKernel) *authorityComponent {
	if kernel == nil {
		return nil
	}
	return &authorityComponent{runtimeMutationKernel: kernel}
}

func newPlanningTransactionComponent(kernel *runtimeMutationKernel) *planningTransactionComponent {
	if kernel == nil {
		return nil
	}
	return &planningTransactionComponent{runtimeMutationKernel: kernel}
}

func newCandidateDispatchComponent(kernel *runtimeMutationKernel) *candidateDispatchComponent {
	if kernel == nil {
		return nil
	}
	return &candidateDispatchComponent{runtimeMutationKernel: kernel}
}

func newDriveDeliveryComponent(kernel *runtimeMutationKernel) *driveDeliveryComponent {
	if kernel == nil {
		return nil
	}
	return &driveDeliveryComponent{runtimeMutationKernel: kernel}
}

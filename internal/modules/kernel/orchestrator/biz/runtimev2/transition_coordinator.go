package runtimev2

import (
	"context"
	"errors"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeTransitionCoordinator is the sole aggregate CAS boundary for the
// Orchestrator owner. SM-2/SM-3/SM-4 use-cases may construct a complete next
// aggregate, but they cannot persist partial state independently.
type RuntimeTransitionCoordinator struct {
	repository RuntimeStateRepository
}

func NewRuntimeTransitionCoordinator(repository RuntimeStateRepository) (*RuntimeTransitionCoordinator, error) {
	if repository == nil {
		return nil, common.ErrInvalidArgument
	}
	return &RuntimeTransitionCoordinator{repository: repository}, nil
}

// Commit validates the complete cross-SM invariant and advances exactly one
// aggregate revision before delegating opaque CAS persistence to the Run owner.
func (c *RuntimeTransitionCoordinator) Commit(ctx context.Context, aggregate RuntimeAggregate, expectedRevision uint64) error {
	if c == nil || c.repository == nil {
		return common.ErrInvalidArgument
	}
	if aggregate.StateRevision != expectedRevision+1 {
		return fmt.Errorf("%w: runtime transition revision %d does not follow expected revision %d", common.ErrConflict, aggregate.StateRevision, expectedRevision)
	}
	if err := aggregate.Validate(); err != nil {
		return fmt.Errorf("runtime transition invariant: %w", err)
	}
	if err := c.repository.Save(ctx, aggregate, expectedRevision); err != nil {
		var typed *RuntimeCASConflictError
		if errors.As(err, &typed) {
			return typed
		}
		if errors.Is(err, common.ErrConflict) {
			observedRevision := uint64(0)
			if current, loadErr := c.repository.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID); loadErr == nil {
				observedRevision = current.StateRevision
			}
			return &RuntimeCASConflictError{
				WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID,
				ExpectedRevision: expectedRevision, ObservedRevision: observedRevision, Cause: err,
			}
		}
		return err
	}
	return nil
}

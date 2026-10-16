package runtime

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
	repository       RuntimeStateRepository
	ownerTransaction *ownerStateTransaction
}

// validatedRuntimeStateRepository is an optional persistence fast path for the
// production adapter. Commit is the only caller: it has already validated the
// complete aggregate immediately before invoking SaveValidated. Direct
// repository callers continue through Save and retain the repository-boundary
// validation.
type validatedRuntimeStateRepository interface {
	SaveValidated(context.Context, RuntimeAggregate, uint64) error
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
	if expectedRevision == ^uint64(0) || aggregate.StateRevision != expectedRevision+1 {
		return fmt.Errorf("%w: runtime transition revision %d does not follow expected revision %d", common.ErrConflict, aggregate.StateRevision, expectedRevision)
	}
	validate := aggregate.Validate
	if transaction := c.ownerTransaction; transaction != nil && transaction.loaded {
		validate = func() error { return transaction.validateAfterImage(aggregate) }
	}
	if err := validate(); err != nil {
		return fmt.Errorf("runtime transition invariant: %w", err)
	}
	if expectedRevision == 0 {
		if len(aggregate.Facts.CWISuspensions) != 0 {
			return fmt.Errorf("%w: initial aggregate cannot carry CWI suspension", common.ErrConflict)
		}
		for _, row := range aggregate.Facts.Coverage {
			if row.HistoricalBehavior != nil || len(row.HistoricalAcceptanceProofs) != 0 {
				return fmt.Errorf("%w: initial aggregate cannot carry historical behavior completion", common.ErrConflict)
			}
		}
		if aggregate.TerminalMaterialSeal != nil {
			return fmt.Errorf("%w: initial aggregate cannot carry a terminal material seal", common.ErrConflict)
		}
	} else {
		previous, err := c.loadTransitionPreimage(ctx, aggregate, expectedRevision)
		if err != nil {
			return err
		}
		if previous.Facts.WorkspaceID != aggregate.Facts.WorkspaceID || previous.Facts.RunID != aggregate.Facts.RunID {
			return fmt.Errorf("%w: runtime transition preimage scope mismatch", common.ErrConflict)
		}
		if previous.StateRevision != expectedRevision {
			return &RuntimeCASConflictError{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, ExpectedRevision: expectedRevision, ObservedRevision: previous.StateRevision}
		}
		if err := validateHistoricalBehaviorTransition(previous, aggregate); err != nil {
			return err
		}
		if err := validateTerminalMaterialSealTransition(previous, aggregate); err != nil {
			return err
		}
		if err := validateEvidenceApplicabilityObservationTransition(previous, aggregate); err != nil {
			return err
		}
		if err := validateAssessmentApplicabilitySealTransition(previous, aggregate); err != nil {
			return err
		}
		if err := validateRuntimeCWISuspensionTransition(previous, aggregate); err != nil {
			return err
		}
	}
	var err error
	if repository, ok := c.repository.(validatedRuntimeStateRepository); ok {
		err = repository.SaveValidated(ctx, aggregate, expectedRevision)
	} else {
		err = c.repository.Save(ctx, aggregate, expectedRevision)
	}
	if err != nil {
		var typed *RuntimeCASConflictError
		if errors.As(err, &typed) {
			return typed
		}
		return err
	}
	return nil
}

func (c *RuntimeTransitionCoordinator) loadTransitionPreimage(ctx context.Context, aggregate RuntimeAggregate, expectedRevision uint64) (RuntimeAggregate, error) {
	if repository, ok := c.repository.(transitionPreimageRepository); ok {
		request := RuntimeTransitionPreimageRequest{call: &runtimeTransitionPreimageCall{
			workspaceID: aggregate.Facts.WorkspaceID, runID: aggregate.Facts.RunID, revision: expectedRevision,
		}}
		if transaction := c.ownerTransaction; transaction != nil && transaction.loaded {
			request.call.compiler.Store(&runtimeOwnerStateImageCompiler{validate: transaction.validate})
		}
		defer func() {
			request.call.state.Store(4)
			request.call.compiler.Store(nil)
		}()
		value, err := repository.LoadTransitionPreimage(ctx, request)
		if err != nil {
			return RuntimeAggregate{}, err
		}
		return request.consume(value)
	}
	previous, err := c.repository.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	// Preserve the ordinary repository path and its original rejection order.
	if previous.Facts.WorkspaceID == aggregate.Facts.WorkspaceID && previous.Facts.RunID == aggregate.Facts.RunID && previous.StateRevision == expectedRevision {
		if err := previous.Validate(); err != nil {
			return RuntimeAggregate{}, fmt.Errorf("runtime transition preimage invariant: %w", err)
		}
	}
	return previous, nil
}

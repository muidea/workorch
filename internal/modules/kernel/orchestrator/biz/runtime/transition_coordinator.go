package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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
	region := startRuntimeTraceRegion(ctx, "workorch.runtime.commit")
	defer region.End()
	if c == nil || c.repository == nil {
		return common.ErrInvalidArgument
	}
	if expectedRevision == ^uint64(0) || aggregate.StateRevision != expectedRevision+1 {
		return fmt.Errorf("%w: runtime transition revision %d does not follow expected revision %d", common.ErrConflict, aggregate.StateRevision, expectedRevision)
	}
	var (
		previous RuntimeAggregate
		diff     runtimecontract.RuntimeStateTransitionDiff
		hasDiff  bool
	)
	validate := aggregate.Validate
	if expectedRevision > 0 {
		if transaction := c.ownerTransaction; transaction != nil && transaction.loaded && transaction.original != nil {
			previous = RuntimeAggregate(*transaction.original)
			diff = runtimecontract.ComputeRuntimeStateTransitionDiff(*transaction.original, ownerStateRecord(aggregate))
			hasDiff = true
			validate = func() error { return transaction.validateAfterImageWithDiff(aggregate, diff) }
		}
	}
	if err := validate(); err != nil {
		return fmt.Errorf("runtime transition invariant: %w", err)
	}
	if expectedRevision == 0 {
		if len(aggregate.Facts.Assessments) != 0 {
			return fmt.Errorf("%w: initial aggregate cannot carry committed assessments", common.ErrConflict)
		}
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
		if !hasDiff {
			var err error
			previous, err = c.loadTransitionPreimage(ctx, aggregate, expectedRevision)
			if err != nil {
				return err
			}
			diff = runtimecontract.ComputeRuntimeStateTransitionDiff(ownerStateRecord(previous), ownerStateRecord(aggregate))
		}
		if previous.Facts.WorkspaceID != aggregate.Facts.WorkspaceID || previous.Facts.RunID != aggregate.Facts.RunID {
			return fmt.Errorf("%w: runtime transition preimage scope mismatch", common.ErrConflict)
		}
		if previous.StateRevision != expectedRevision {
			return &RuntimeCASConflictError{WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, ExpectedRevision: expectedRevision, ObservedRevision: previous.StateRevision}
		}
		if c.ownerTransaction == nil {
			if err := runtimecontract.ValidateRuntimeCommittedHistoryTransitionWithDiff(ownerStateRecord(previous), ownerStateRecord(aggregate), diff); err != nil {
				return err
			}
		}
		if diff.PrerequisiteInvalidations {
			if err := validatePrerequisiteInvalidationChronologyTransition(previous, aggregate); err != nil {
				return err
			}
		}
		if diff.FactsCoverage || diff.ActiveDispatch || diff.CompletedDispatches {
			if err := validateHistoricalBehaviorTransition(previous, aggregate); err != nil {
				return err
			}
		}
		if diff.TerminalMaterialSeal {
			if err := validateTerminalMaterialSealTransition(previous, aggregate); err != nil {
				return err
			}
		}
		if diff.FactsEvidenceApplicabilityObservations {
			if err := validateEvidenceApplicabilityObservationTransition(previous, aggregate); err != nil {
				return err
			}
		}
		if diff.FactsAssessments || diff.CompletedDispatches {
			if err := validateCommittedAssessmentResultTransition(previous, aggregate); err != nil {
				return err
			}
		}
		if diff.ActiveDispatch || diff.CompletedDispatches {
			if err := validateAssessmentApplicabilitySealTransition(previous, aggregate); err != nil {
				return err
			}
		}
		if diff.FactsCWISuspensions {
			if err := validateRuntimeCWISuspensionTransition(previous, aggregate); err != nil {
				return err
			}
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
	region := startRuntimeTraceRegion(ctx, "workorch.runtime.commit_reread")
	defer region.End()
	previous, err := c.repository.Load(ctx, aggregate.Facts.WorkspaceID, aggregate.Facts.RunID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	// Preserve the ordinary repository path and its original rejection order.
	if previous.Facts.WorkspaceID == aggregate.Facts.WorkspaceID && previous.Facts.RunID == aggregate.Facts.RunID && previous.StateRevision == expectedRevision {
		validate := previous.Validate
		if t := c.ownerTransaction; t != nil && t.loaded {
			validate = func() error { return t.admit(nil, previous) }
		}
		if err := validate(); err != nil {
			return RuntimeAggregate{}, fmt.Errorf("runtime transition preimage invariant: %w", err)
		}
	}
	return previous, nil
}

package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// recordValidationFailureObservation is intentionally an identity fence, not
// a state transition. The receipt/evidence transaction is the only producer
// of the Shell fact; this method makes the following File handoff explicit and
// rejects stale/forged observations before any File query is used.
func (c *RuntimeController) recordValidationFailureObservation(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, observation runtimecontract.RuntimeValidationFailureObservation) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || observation.Validate() != nil || observation.WorkspaceID != workspaceID || observation.RunID != runID {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	if aggregate.Facts.Failure == nil || aggregate.Facts.Failure.ValidationFailureObservation == nil || !runtimecontract.SameRuntimeValidationFailureObservation(*aggregate.Facts.Failure.ValidationFailureObservation, observation) || !sealedValidationFailureObservationMatches(aggregate, observation) {
		return RuntimeAggregate{}, fmt.Errorf("%w: validation failure observation does not match persisted receipt", common.ErrConflict)
	}
	return aggregate, nil
}

// recordRepairSurfaceConfirmation is the sole SM-2 transition from a File
// fact to repair authority. File cannot select the action, and Shell cannot
// promote its diagnostic candidates without this exact confirmation.
func (c *RuntimeController) recordRepairSurfaceConfirmation(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, confirmation runtimecontract.RuntimeRepairSurfaceConfirmation) (RuntimeAggregate, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || confirmation.Validate() != nil || confirmation.Observation.WorkspaceID != workspaceID || confirmation.Observation.RunID != runID {
		return RuntimeAggregate{}, common.ErrInvalidArgument
	}
	confirmation = confirmation.Normalized()
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	// Persisted typed confirmation records are checked before terminal/failure
	// guards.  This makes exact replay a true no-op even after restart or after
	// the first confirmation has retired the mutable target pointers.
	if repairSurfaceConfirmationExists(aggregate, confirmation) {
		return aggregate, nil
	}
	if repairSurfaceConfirmationIdentityConflict(aggregate, confirmation) {
		return RuntimeAggregate{}, fmt.Errorf("%w: repair surface confirmation conflicts with persisted disposition", common.ErrConflict)
	}
	if aggregate.Decision.State == runtimecontract.ConvergenceStateComplete || aggregate.Decision.State == runtimecontract.ConvergenceStateFailed {
		return aggregate, nil
	}
	failure := aggregate.Facts.Failure
	if failure == nil || failure.ValidationFailureObservation == nil || !runtimecontract.SameRuntimeValidationFailureObservation(*failure.ValidationFailureObservation, confirmation.Observation) || !sealedValidationFailureObservationMatches(aggregate, confirmation.Observation) {
		return RuntimeAggregate{}, fmt.Errorf("%w: repair surface confirmation does not match persisted validation failure", common.ErrConflict)
	}
	if confirmation.Disposition == runtimecontract.RuntimeRepairSurfaceConfirmed && aggregate.Facts.Resources.FailureTarget != nil {
		if aggregate.Facts.Resources.FailureTarget.Signature == failure.Signature && sameRuntimeTargetRefs(aggregate.Facts.Resources.FailureTarget.TargetRefs, confirmation.ConfirmedTargetRefs) {
			return aggregate, nil
		}
		return RuntimeAggregate{}, fmt.Errorf("%w: repair surface already resolved for another target", common.ErrConflict)
	}
	expected := aggregate.StateRevision
	if confirmation.Disposition == runtimecontract.RuntimeRepairSurfaceRejected {
		updatedFailure := failure.Clone()
		// A rejected owner surface is a typed infeasibility fact, not evidence
		// that the repair budget was consumed.  Keep the terminal code stable for
		// callers while preserving the original diagnostic nested in the failure.
		updatedFailure.Class = runtimecontract.FailureClassCandidateInfeasible
		updatedFailure.Code = "repair_surface_unavailable"
		updatedFailure.Message = firstNonEmpty(strings.TrimSpace(confirmation.Reason), "File owner could not confirm a repair surface")
		updatedFailure.TargetRefs = nil
		updatedFailure.TargetObservation = nil
		aggregate.Facts.Failure = &updatedFailure
	} else if confirmation.Disposition == runtimecontract.RuntimeRepairSurfaceStale {
		updatedFailure := failure.Clone()
		// Stale owner scope is a freshness fact. Retire every current target
		// pointer and let the normal SM-2 prerequisite compiler reacquire the
		// sealed focus scope; do not turn a stale observation into a terminal.
		updatedFailure.TargetRefs = nil
		updatedFailure.TargetObservation = nil
		aggregate.Facts.Failure = &updatedFailure
		aggregate.Facts.Resources.FailureTarget = nil
		aggregate.Facts.Resources.TargetSelection = nil
		aggregate.Facts.Resources.TargetSnapshots = nil
		aggregate.Facts = PreflightRuntimeFacts(aggregate.Facts, runtimeTargetGroundingRequirements(aggregate.Facts))
	} else {
		updatedFailure := failure.Clone()
		updatedFailure.TargetRefs = append([]string(nil), confirmation.ConfirmedTargetRefs...)
		updatedFailure.TargetObservation = &runtimecontract.RuntimeFailureTargetObservation{OwnerModuleID: confirmation.OwnerModuleID, SubjectRef: confirmation.SubjectRef, TargetRole: confirmation.Observation.TargetRole, ObservedRevision: confirmation.ObservedRevision, TargetRefs: confirmation.ConfirmedTargetRefs}
		aggregate.Facts.Failure = &updatedFailure
		aggregate.Facts.Resources.FailureTarget = &runtimecontract.FailureTarget{Signature: updatedFailure.Signature, TargetRefs: append([]string(nil), confirmation.ConfirmedTargetRefs...), TargetObservation: updatedFailure.TargetObservation}
		// A newly confirmed File surface replaces stale focus snapshots before
		// issuing the state-change authority.
		aggregate.Facts = PreflightRuntimeFacts(aggregate.Facts, runtimeTargetGroundingRequirements(aggregate.Facts))
	}
	facts, decision, err := ReduceRuntimeStateWithDirective(aggregate.Facts, &aggregate.Decision.AuthoritySnapshot, aggregate.CoverageProofLineage)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.Facts, aggregate.Decision = facts, decision
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, err
	}
	record, err := runtimecontract.NewRuntimeRepairSurfaceConfirmationRecord(confirmation)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	aggregate.RepairSurfaceConfirmations = append(aggregate.RepairSurfaceConfirmations, record)
	if err := c.transitions.Commit(ctx, aggregate, expected); err != nil {
		return RuntimeAggregate{}, err
	}
	return aggregate, nil
}

func repairSurfaceConfirmationExists(aggregate RuntimeAggregate, confirmation runtimecontract.RuntimeRepairSurfaceConfirmation) bool {
	signature := runtimecontract.StableRuntimeRepairSurfaceConfirmationHash(confirmation)
	for _, record := range aggregate.RepairSurfaceConfirmations {
		if record.Signature == signature || runtimecontract.SameRuntimeRepairSurfaceConfirmation(record.Confirmation, confirmation) {
			return true
		}
	}
	return false
}

func repairSurfaceConfirmationIdentityConflict(aggregate RuntimeAggregate, confirmation runtimecontract.RuntimeRepairSurfaceConfirmation) bool {
	identity := runtimecontract.StableRuntimeRepairSurfaceConfirmationIdentityHash(confirmation)
	for _, record := range aggregate.RepairSurfaceConfirmations {
		if record.IdentitySignature == identity || runtimecontract.SameRuntimeRepairSurfaceConfirmationIdentity(record.Confirmation, confirmation) {
			return !runtimecontract.SameRuntimeRepairSurfaceConfirmation(record.Confirmation, confirmation)
		}
	}
	return false
}

// sealedValidationFailureObservationMatches binds the diagnostic candidates
// back to the immutable validation operation that produced them.  This is the
// Orchestrator-side causation check; File subsequently verifies the current
// resource revision. Neither check interprets diagnostic text.
func sealedValidationFailureObservationMatches(aggregate RuntimeAggregate, observation runtimecontract.RuntimeValidationFailureObservation) bool {
	observation = observation.Normalized()
	if observation.TargetRole != strings.TrimSpace(aggregate.Facts.Focus.TargetRole) {
		return false
	}
	for _, dispatch := range aggregate.CompletedDispatches {
		if dispatch.ID != observation.DispatchID || dispatch.Transaction.AuthoritySnapshotID != observation.AuthoritySnapshotID || dispatch.Candidate.Validation == nil {
			continue
		}
		for _, operation := range dispatch.Candidate.Validation.Operations {
			value := operation.Operation.Normalized()
			if value.ID == observation.OperationID && sameRuntimeTargetRefs(value.TargetRefs, observation.ValidationScopeTargetRefs) {
				return true
			}
		}
	}
	return false
}

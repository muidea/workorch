package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// NewRuntimeNonTerminalRecoveryRequest converts exactly one typed Run-owner
// persistence outcome into the owner-private SM-4 request. Plain ErrConflict
// and arbitrary Temporary errors never enter this path.
func NewRuntimeNonTerminalRecoveryRequest(
	workspaceID common.WorkspaceID,
	runID common.RunID,
	failedLeaseGeneration uint64,
	issuedAtStateRevision uint64,
	observedAt string,
	cause error,
) (runtimecontract.RuntimeNonTerminalRecoveryRequest, error) {
	request := runtimecontract.RuntimeNonTerminalRecoveryRequest{
		WorkspaceID: workspaceID, RunID: runID,
		FailedLeaseGeneration: failedLeaseGeneration,
		IssuedAtStateRevision: issuedAtStateRevision,
		ObservedAt:            strings.TrimSpace(observedAt),
	}
	var cas *RuntimeCASConflictError
	var ownerTransient *RuntimeOwnerTransientError
	hasCAS := errors.As(cause, &cas) && cas != nil
	hasOwnerTransient := errors.As(cause, &ownerTransient) && ownerTransient != nil
	if hasCAS == hasOwnerTransient {
		return runtimecontract.RuntimeNonTerminalRecoveryRequest{}, fmt.Errorf("%w: nonterminal recovery requires exactly one typed persistence outcome", common.ErrInvalidArgument)
	}
	if hasCAS {
		if cas.WorkspaceID != workspaceID || cas.RunID != runID {
			return runtimecontract.RuntimeNonTerminalRecoveryRequest{}, fmt.Errorf("%w: CAS recovery outcome scope mismatch", common.ErrInvalidArgument)
		}
		request.Kind = runtimecontract.RuntimeNonTerminalRecoveryCASConflict
		request.ExpectedRuntimeRevision = cas.ExpectedRevision
		request.OwnerObservedRuntimeRevision = cas.ObservedRevision
		request.FailureSignature = runtimecontract.RuntimeNonTerminalRecoveryFailureCASConflict
	} else {
		if ownerTransient.WorkspaceID != workspaceID || ownerTransient.RunID != runID {
			return runtimecontract.RuntimeNonTerminalRecoveryRequest{}, fmt.Errorf("%w: owner-transient recovery outcome scope mismatch", common.ErrInvalidArgument)
		}
		feedback := ownerTransient.Feedback.Normalized()
		if feedback.Validate() != nil || feedback.Domain != runtimecontract.RuntimeFeedbackDomainPersistence ||
			feedback.StableCategory != runtimecontract.RuntimeFeedbackCategoryOwnerTransient ||
			feedback.WorkspaceID != workspaceID || feedback.RunID != runID {
			return runtimecontract.RuntimeNonTerminalRecoveryRequest{}, fmt.Errorf("%w: owner-transient recovery requires exact Run persistence feedback", common.ErrInvalidArgument)
		}
		request.Kind = runtimecontract.RuntimeNonTerminalRecoveryOwnerTransient
		request.ExpectedRuntimeRevision = ownerTransient.ExpectedRevision
		request.OwnerObservedRuntimeRevision = ownerTransient.ObservedRevision
		request.FailureSignature = runtimecontract.RuntimeNonTerminalRecoveryFailureOwnerTransient
		request.OwnerFeedbackHash = runtimecontract.StableRuntimeFeedbackHash(feedback)
	}
	request.RequestID = runtimecontract.StableRuntimeNonTerminalRecoveryRequestID(request)
	if err := request.Validate(); err != nil {
		return runtimecontract.RuntimeNonTerminalRecoveryRequest{}, fmt.Errorf("%w: %v", common.ErrInvalidArgument, err)
	}
	return request.Normalized(), nil
}

func validateRuntimeNonTerminalRecoveryCause(request runtimecontract.RuntimeNonTerminalRecoveryRequest, cause error) error {
	expected, err := NewRuntimeNonTerminalRecoveryRequest(
		request.WorkspaceID,
		request.RunID,
		request.FailedLeaseGeneration,
		request.IssuedAtStateRevision,
		request.ObservedAt,
		cause,
	)
	if err != nil {
		return err
	}
	if !runtimecontract.SameRuntimeNonTerminalRecoveryRequestIdentity(request, expected) {
		return fmt.Errorf("%w: nonterminal recovery request does not match typed persistence outcome", common.ErrInvalidArgument)
	}
	return nil
}

// scheduleNonTerminalRecovery atomically records the shared CAS-reload budget
// disposition and its only scheduled successor lease in one aggregate CAS.
func (c *driveDeliveryComponent) scheduleNonTerminalRecovery(
	ctx context.Context,
	request runtimecontract.RuntimeNonTerminalRecoveryRequest,
	cause error,
) (RuntimeAggregate, runtimecontract.RuntimeNonTerminalRecoveryRecord, runtimecontract.RuntimeDriveLease, bool, bool, error) {
	if c == nil || c.repository == nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeNonTerminalRecoveryRecord{}, runtimecontract.RuntimeDriveLease{}, false, false, common.ErrInvalidArgument
	}
	request = request.Normalized()
	if err := request.Validate(); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeNonTerminalRecoveryRecord{}, runtimecontract.RuntimeDriveLease{}, false, false, common.ErrInvalidArgument
	}
	if err := validateRuntimeNonTerminalRecoveryCause(request, cause); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeNonTerminalRecoveryRecord{}, runtimecontract.RuntimeDriveLease{}, false, false, err
	}
	aggregate, err := c.repository.Load(ctx, request.WorkspaceID, request.RunID)
	if err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeNonTerminalRecoveryRecord{}, runtimecontract.RuntimeDriveLease{}, false, false, err
	}

	currentRecord := aggregate.Facts.Recovery.Normalized().NonTerminalRecovery
	if runtimecontract.RuntimeNonTerminalRecoveryRecordPresent(currentRecord) && currentRecord.Request.RequestID == request.RequestID {
		if !runtimecontract.SameRuntimeNonTerminalRecoveryRequestIdentity(currentRecord.Request, request) {
			return RuntimeAggregate{}, runtimecontract.RuntimeNonTerminalRecoveryRecord{}, runtimecontract.RuntimeDriveLease{}, false, false,
				fmt.Errorf("%w: nonterminal recovery request identity conflicts", common.ErrConflict)
		}
		var currentLease runtimecontract.RuntimeDriveLease
		if aggregate.DriveLease != nil && aggregate.DriveLease.RecoveryRequestID == request.RequestID {
			currentLease = aggregate.DriveLease.Clone()
		}
		return aggregate, currentRecord, currentLease, false, false, nil
	}
	if aggregate.StateRevision != request.IssuedAtStateRevision {
		return RuntimeAggregate{}, runtimecontract.RuntimeNonTerminalRecoveryRecord{}, runtimecontract.RuntimeDriveLease{}, false, false,
			fmt.Errorf("%w: nonterminal recovery request was not issued from the current aggregate revision", common.ErrConflict)
	}
	if aggregate.Termination != nil || isTerminalConvergenceState(aggregate.Decision.State) {
		return RuntimeAggregate{}, runtimecontract.RuntimeNonTerminalRecoveryRecord{}, runtimecontract.RuntimeDriveLease{}, false, false,
			fmt.Errorf("%w: terminal runtime cannot admit nonterminal recovery", common.ErrConflict)
	}
	if request.FailedLeaseGeneration == ^uint64(0) {
		return RuntimeAggregate{}, runtimecontract.RuntimeNonTerminalRecoveryRecord{}, runtimecontract.RuntimeDriveLease{}, false, false,
			fmt.Errorf("%w: runtime drive lease generation exhausted", common.ErrConflict)
	}
	if request.FailedLeaseGeneration == 0 {
		if aggregate.DriveLease != nil {
			return RuntimeAggregate{}, runtimecontract.RuntimeNonTerminalRecoveryRecord{}, runtimecontract.RuntimeDriveLease{}, false, false,
				fmt.Errorf("%w: initial nonterminal recovery cannot replace persisted lease authority", common.ErrConflict)
		}
	} else if aggregate.DriveLease == nil || aggregate.DriveLease.State != runtimecontract.RuntimeDriveLeaseRunning ||
		aggregate.DriveLease.RecoveryGeneration != request.FailedLeaseGeneration {
		return RuntimeAggregate{}, runtimecontract.RuntimeNonTerminalRecoveryRecord{}, runtimecontract.RuntimeDriveLease{}, false, false,
			fmt.Errorf("%w: nonterminal recovery request has no exact failed running authority", common.ErrConflict)
	}

	policy := aggregate.Facts.RecoveryPolicy.Normalized()
	attempt := aggregate.Facts.Recovery.CASReloadAttempts
	disposition := runtimecontract.RuntimeNonTerminalRecoveryDispositionBudgetExhausted
	budgetConsumed := false
	if attempt < policy.MaxCASReloadAttempts {
		attempt++
		aggregate.Facts.Recovery.CASReloadAttempts = attempt
		disposition = runtimecontract.RuntimeNonTerminalRecoveryDispositionBudgetConsumed
		budgetConsumed = true
	}
	if attempt < 1 {
		return RuntimeAggregate{}, runtimecontract.RuntimeNonTerminalRecoveryRecord{}, runtimecontract.RuntimeDriveLease{}, false, false,
			fmt.Errorf("%w: nonterminal recovery has no valid CAS reload budget", common.ErrConflict)
	}
	reason, _ := runtimecontract.RuntimeNonTerminalRecoveryLeaseReason(request.Kind)
	dueAt, err := runtimecontract.RuntimeNonTerminalRecoveryDueAt(request.ObservedAt, attempt, disposition)
	if err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeNonTerminalRecoveryRecord{}, runtimecontract.RuntimeDriveLease{}, false, false, err
	}
	expectedRevision := aggregate.StateRevision
	aggregate.StateRevision++
	lease := runtimecontract.RuntimeDriveLease{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID,
		AggregateRevision: aggregate.StateRevision, RecoveryGeneration: request.FailedLeaseGeneration + 1,
		Reason: reason, FailureSignature: request.FailureSignature, RecoveryRequestID: request.RequestID,
		DueAt: dueAt, State: runtimecontract.RuntimeDriveLeaseScheduled,
	}
	record := runtimecontract.RuntimeNonTerminalRecoveryRecord{
		Request: request, Disposition: disposition, Attempt: attempt,
		RecordedAtStateRevision: aggregate.StateRevision, SuccessorLease: lease,
	}.Normalized()
	aggregate.Facts.Recovery.NonTerminalRecovery = record
	aggregate.DriveLease = &lease
	if err := c.validateAggregate(aggregate); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeNonTerminalRecoveryRecord{}, runtimecontract.RuntimeDriveLease{}, false, false,
			fmt.Errorf("schedule runtime nonterminal recovery: %w", err)
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeNonTerminalRecoveryRecord{}, runtimecontract.RuntimeDriveLease{}, false, false, err
	}
	return aggregate, record, lease.Clone(), true, budgetConsumed, nil
}

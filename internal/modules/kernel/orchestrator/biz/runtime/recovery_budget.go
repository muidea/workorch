package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (c *driveDeliveryComponent) consumeRecoveryBudget(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, kind runtimecontract.RuntimeRecoveryBudgetKind, recoveryOperationID, observedAt string, terminalRequests ...runtimecontract.RuntimeTerminalDeliveryRecoveryRequest) (RuntimeAggregate, bool, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" {
		return RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	// CAS reload and Run-owner transient recovery use the dedicated atomic
	// nonterminal transaction. Letting this generic budget event increment the
	// shared counter without its request record and successor lease would restore
	// the retired two-CAS recovery path.
	if kind == runtimecontract.RuntimeRecoveryBudgetCASReload {
		return RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	recoveryOperationID = strings.TrimSpace(recoveryOperationID)
	observedAt = strings.TrimSpace(observedAt)
	if len(terminalRequests) > 1 || (kind == runtimecontract.RuntimeRecoveryBudgetOperationRecovery) != (recoveryOperationID != "") ||
		(kind == runtimecontract.RuntimeRecoveryBudgetOperationRecovery) != (observedAt != "") {
		return RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	var terminalRequest runtimecontract.RuntimeTerminalDeliveryRecoveryRequest
	if len(terminalRequests) == 1 {
		terminalRequest = terminalRequests[0].Normalized()
	}
	if kind == runtimecontract.RuntimeRecoveryBudgetTerminalDelivery {
		if terminalRequest.Validate() != nil {
			return RuntimeAggregate{}, false, common.ErrInvalidArgument
		}
	} else if terminalRequest.RequestID != "" || terminalRequest.TerminalDeliveryID != "" || terminalRequest.FailedLeaseGeneration != 0 {
		return RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	var recoveryNow time.Time
	if observedAt != "" {
		var parseErr error
		recoveryNow, parseErr = time.Parse(time.RFC3339Nano, observedAt)
		if parseErr != nil {
			return RuntimeAggregate{}, false, common.ErrInvalidArgument
		}
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, false, err
	}
	if kind == runtimecontract.RuntimeRecoveryBudgetTerminalDelivery && TerminalDeliveryFullyAcknowledged(aggregate) {
		// The last owner acknowledgement can commit after the caller's initial
		// failure observation. Budget consumption shares the same fresh-load
		// linearization rule as terminal-delivery lease scheduling.
		return aggregate, false, nil
	}
	if kind == runtimecontract.RuntimeRecoveryBudgetTerminalDelivery {
		outcome, terminal, outcomeErr := DeriveTerminalOutcome(aggregate.Decision, aggregate.Termination)
		if outcomeErr != nil || !terminal {
			return RuntimeAggregate{}, false, fmt.Errorf("%w: terminal delivery recovery requires sealed terminal authority", common.ErrConflict)
		}
		expectedDeliveryID := runtimecontract.StableTerminalDeliveryID(workspaceID, runID, aggregate.Decision.AuthoritySnapshot.ID, outcome)
		if terminalRequest.TerminalDeliveryID != expectedDeliveryID {
			return RuntimeAggregate{}, false, fmt.Errorf("%w: terminal delivery recovery request does not match current terminal", common.ErrConflict)
		}
		currentIntervention := aggregate.Facts.Recovery.Normalized().TerminalDeliveryIntervention
		if currentIntervention.RequestID != "" {
			if currentIntervention != terminalRequest {
				return RuntimeAggregate{}, false, fmt.Errorf("%w: a different terminal delivery intervention is already sealed", common.ErrConflict)
			}
			return aggregate, false, nil
		}
		currentRequest := aggregate.Facts.Recovery.Normalized().TerminalDeliveryRequest
		if currentRequest.RequestID == terminalRequest.RequestID {
			if currentRequest != terminalRequest {
				return RuntimeAggregate{}, false, fmt.Errorf("%w: terminal delivery recovery request identity conflicts", common.ErrConflict)
			}
			return aggregate, false, nil
		}
		if currentRequest.RequestID != "" && terminalRequest.FailedLeaseGeneration <= currentRequest.FailedLeaseGeneration {
			return RuntimeAggregate{}, false, fmt.Errorf("%w: terminal delivery recovery request generation is stale", common.ErrConflict)
		}
		if terminalRequest.FailedLeaseGeneration == 0 {
			recovery := aggregate.Facts.Recovery.Normalized()
			if aggregate.DriveLease != nil ||
				recovery.TerminalDeliveryRequest != (runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{}) ||
				recovery.TerminalDeliveryIntervention != (runtimecontract.RuntimeTerminalDeliveryRecoveryRequest{}) ||
				runtimecontract.RuntimeDriveLeasePresent(recovery.RetiredTerminalDeliveryLease) ||
				recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID != "" {
				return RuntimeAggregate{}, false, fmt.Errorf("%w: initial terminal delivery recovery cannot replace persisted lease authority", common.ErrConflict)
			}
		} else {
			runningMatches := aggregate.DriveLease != nil && aggregate.DriveLease.State == runtimecontract.RuntimeDriveLeaseRunning &&
				aggregate.DriveLease.RecoveryGeneration == terminalRequest.FailedLeaseGeneration
			retired := aggregate.Facts.Recovery.Normalized().RetiredTerminalDeliveryLease
			retiredMatches := aggregate.DriveLease == nil && aggregate.Termination != nil && retired.Validate() == nil &&
				retired.State == runtimecontract.RuntimeDriveLeaseRunning && retired.RecoveryGeneration == terminalRequest.FailedLeaseGeneration &&
				aggregate.Facts.Recovery.Normalized().RetiredTerminalDeliveryLeaseRecoveryRequestID == ""
			if !runningMatches && !retiredMatches {
				return RuntimeAggregate{}, false, fmt.Errorf("%w: terminal delivery recovery request has no exact failed running authority", common.ErrConflict)
			}
		}
	}
	expectedRevision := aggregate.StateRevision
	authorizationExpired := false
	var recoveryExecution runtimecontract.RuntimeOperationExecutionRecord
	if kind == runtimecontract.RuntimeRecoveryBudgetOperationRecovery {
		if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.State != DispatchStateSealed {
			return RuntimeAggregate{}, false, fmt.Errorf("%w: operation recovery requires an active sealed dispatch", common.ErrConflict)
		}
		var found bool
		recoveryExecution, found = operationExecutionRecord(aggregate.ActiveDispatch.OperationExecutions, recoveryOperationID)
		if !found || recoveryExecution.State != runtimecontract.OperationExecutionClaimed || !runtimeOperationHasReplayAuthority(recoveryExecution) {
			return RuntimeAggregate{}, false, fmt.Errorf("%w: operation recovery requires an exact replayable claimed execution", common.ErrConflict)
		}
		if _, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, recoveryExecution.ExecutionKey, recoveryExecution.Fence); found {
			return RuntimeAggregate{}, false, fmt.Errorf("%w: operation recovery execution already has a receipt", common.ErrConflict)
		}
		if _, issued := runtimecontract.RuntimeOperationRecoveryAuthorizationForExecution(aggregate.OperationRecoveryAuthorizations, aggregate.ActiveDispatch.ID, recoveryExecution); issued {
			// A duplicate budget event is a pure read. The issued authorization must
			// first be claimed by its own pre-call SM-4 event.
			return aggregate, false, nil
		}
		if current, found := runtimecontract.RuntimeCurrentOperationRecoveryAuthorizationForExecution(aggregate.OperationRecoveryAuthorizations, aggregate.ActiveDispatch.ID, recoveryExecution); found {
			switch current.State {
			case runtimecontract.RuntimeOperationRecoveryAuthorizationInFlight:
				deadline, deadlineErr := time.Parse(time.RFC3339Nano, current.LeaseDeadline)
				if deadlineErr != nil {
					return RuntimeAggregate{}, false, common.ErrInvalidArgument
				}
				if !recoveryNow.After(deadline) {
					return aggregate, false, &runtimeOperationRecoveryInFlightError{OperationID: recoveryOperationID, ClaimID: current.ClaimID, LeaseDeadline: current.LeaseDeadline}
				}
				for idx := range aggregate.OperationRecoveryAuthorizations {
					if aggregate.OperationRecoveryAuthorizations[idx].AuthorizationID == current.AuthorizationID {
						aggregate.OperationRecoveryAuthorizations[idx].State = runtimecontract.RuntimeOperationRecoveryAuthorizationUncertain
						authorizationExpired = true
						break
					}
				}
			case runtimecontract.RuntimeOperationRecoveryAuthorizationSettled:
				return RuntimeAggregate{}, false, fmt.Errorf("%w: settled operation recovery has no receipt", common.ErrConflict)
			case runtimecontract.RuntimeOperationRecoveryAuthorizationRetired:
				return RuntimeAggregate{}, false, fmt.Errorf("%w: retired operation recovery cannot be reissued", common.ErrConflict)
			}
		}
		if len(aggregate.OperationRecoveryAuthorizations) != 0 {
			latest := aggregate.OperationRecoveryAuthorizations[len(aggregate.OperationRecoveryAuthorizations)-1].Normalized()
			if (latest.State == runtimecontract.RuntimeOperationRecoveryAuthorizationIssued || latest.State == runtimecontract.RuntimeOperationRecoveryAuthorizationInFlight) &&
				(latest.DispatchID != aggregate.ActiveDispatch.ID || latest.OperationID != recoveryExecution.OperationID || latest.ExecutionKey != recoveryExecution.ExecutionKey || latest.Fence != recoveryExecution.Fence) {
				return RuntimeAggregate{}, false, fmt.Errorf("%w: another operation recovery authorization is actionable", common.ErrConflict)
			}
		}
	}
	ledger, consumed, err := consumeRecoveryBudget(aggregate.Facts.Recovery, kind, aggregate.Facts.RecoveryPolicy)
	if err != nil {
		return RuntimeAggregate{}, false, err
	}
	if !consumed {
		if kind == runtimecontract.RuntimeRecoveryBudgetTerminalDelivery {
			currentIntervention := aggregate.Facts.Recovery.Normalized().TerminalDeliveryIntervention
			if currentIntervention.RequestID != "" {
				if currentIntervention != terminalRequest {
					return RuntimeAggregate{}, false, fmt.Errorf("%w: a different terminal delivery intervention is already sealed", common.ErrConflict)
				}
				return aggregate, false, nil
			}
			// Exhaustion is itself a durable SM-4 disposition. Retire only the exact
			// failed running generation; a marker-backed administrative handoff has
			// already retired that generation in the termination CAS.
			if aggregate.DriveLease != nil {
				if aggregate.DriveLease.State != runtimecontract.RuntimeDriveLeaseRunning ||
					aggregate.DriveLease.RecoveryGeneration != terminalRequest.FailedLeaseGeneration {
					return RuntimeAggregate{}, false, fmt.Errorf("%w: terminal delivery intervention cannot retire a different lease", common.ErrConflict)
				}
				aggregate.DriveLease = nil
			}
			aggregate.Facts.Recovery.TerminalDeliveryIntervention = terminalRequest
			aggregate.StateRevision++
			if err := c.validateAggregate(aggregate); err != nil {
				return RuntimeAggregate{}, false, fmt.Errorf("seal terminal delivery intervention: %w", err)
			}
			if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
				return RuntimeAggregate{}, false, err
			}
			return aggregate, false, nil
		}
		if authorizationExpired {
			aggregate.StateRevision++
			if err := c.validateAggregate(aggregate); err != nil {
				return RuntimeAggregate{}, false, fmt.Errorf("expire runtime operation recovery claim: %w", err)
			}
			if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
				return RuntimeAggregate{}, false, err
			}
		}
		return aggregate, false, nil
	}
	aggregate.Facts.Recovery = ledger
	if kind == runtimecontract.RuntimeRecoveryBudgetTerminalDelivery {
		aggregate.Facts.Recovery.TerminalDeliveryRequest = terminalRequest
	}
	if kind == runtimecontract.RuntimeRecoveryBudgetOperationRecovery {
		authorization := runtimecontract.RuntimeOperationRecoveryAuthorization{
			WorkspaceID: workspaceID, RunID: runID, DispatchID: aggregate.ActiveDispatch.ID,
			OperationID: recoveryExecution.OperationID, ExecutionKey: recoveryExecution.ExecutionKey, Fence: recoveryExecution.Fence,
			RecoveryAttempt: ledger.OperationRecoveryAttempts, IssuedAtStateRevision: expectedRevision + 1,
			State: runtimecontract.RuntimeOperationRecoveryAuthorizationIssued,
		}
		authorization.AuthorizationID = runtimecontract.StableRuntimeOperationRecoveryAuthorizationID(authorization)
		aggregate.OperationRecoveryAuthorizations = append(aggregate.OperationRecoveryAuthorizations, authorization)
	}
	aggregate.StateRevision++
	if err := c.validateAggregate(aggregate); err != nil {
		return RuntimeAggregate{}, false, fmt.Errorf("runtime recovery budget: %w", err)
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, false, err
	}
	return aggregate, true, nil
}

func (c *driveDeliveryComponent) completeOperationRecoveryCall(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, operationID, authorizationID, claimID string) (RuntimeAggregate, bool, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(operationID) == "" || strings.TrimSpace(authorizationID) == "" || strings.TrimSpace(claimID) == "" {
		return RuntimeAggregate{}, false, common.ErrInvalidArgument
	}
	operationID = strings.TrimSpace(operationID)
	authorizationID = strings.TrimSpace(authorizationID)
	claimID = strings.TrimSpace(claimID)
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, false, err
	}
	idx := -1
	for current := range aggregate.OperationRecoveryAuthorizations {
		authorization := aggregate.OperationRecoveryAuthorizations[current].Normalized()
		if authorization.AuthorizationID != authorizationID {
			continue
		}
		if authorization.OperationID != operationID || authorization.WorkspaceID != workspaceID || authorization.RunID != runID || authorization.ClaimID != claimID {
			return RuntimeAggregate{}, false, fmt.Errorf("%w: operation recovery call identity conflicts", common.ErrConflict)
		}
		if authorization.State == runtimecontract.RuntimeOperationRecoveryAuthorizationSettled || authorization.State == runtimecontract.RuntimeOperationRecoveryAuthorizationUncertain {
			return aggregate, false, nil
		}
		if authorization.State != runtimecontract.RuntimeOperationRecoveryAuthorizationInFlight {
			return RuntimeAggregate{}, false, fmt.Errorf("%w: operation recovery call is not in flight", common.ErrConflict)
		}
		idx = current
		break
	}
	if idx < 0 {
		return RuntimeAggregate{}, false, fmt.Errorf("%w: operation recovery call claim is absent", common.ErrConflict)
	}
	authorization := aggregate.OperationRecoveryAuthorizations[idx].Normalized()
	if _, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, authorization.ExecutionKey, authorization.Fence); found {
		authorization.State = runtimecontract.RuntimeOperationRecoveryAuthorizationSettled
	} else {
		authorization.State = runtimecontract.RuntimeOperationRecoveryAuthorizationUncertain
	}
	expectedRevision := aggregate.StateRevision
	aggregate.OperationRecoveryAuthorizations[idx] = authorization
	aggregate.StateRevision++
	if err := c.validateAggregate(aggregate); err != nil {
		return RuntimeAggregate{}, false, fmt.Errorf("complete runtime operation recovery call: %w", err)
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, false, err
	}
	return aggregate, true, nil
}

func (c *driveDeliveryComponent) claimOperationRecoveryAuthorization(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, operationID, authorizationID, observedAt string) (RuntimeAggregate, string, bool, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(operationID) == "" || strings.TrimSpace(authorizationID) == "" || strings.TrimSpace(observedAt) == "" {
		return RuntimeAggregate{}, "", false, common.ErrInvalidArgument
	}
	operationID = strings.TrimSpace(operationID)
	authorizationID = strings.TrimSpace(authorizationID)
	claimedAt, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(observedAt))
	if parseErr != nil {
		return RuntimeAggregate{}, "", false, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, "", false, err
	}
	idx := -1
	for current := range aggregate.OperationRecoveryAuthorizations {
		authorization := aggregate.OperationRecoveryAuthorizations[current].Normalized()
		if authorization.AuthorizationID != authorizationID {
			continue
		}
		if authorization.OperationID != operationID || authorization.WorkspaceID != workspaceID || authorization.RunID != runID {
			return RuntimeAggregate{}, "", false, fmt.Errorf("%w: operation recovery authorization identity conflicts", common.ErrConflict)
		}
		if authorization.State != runtimecontract.RuntimeOperationRecoveryAuthorizationIssued {
			return aggregate, "", false, nil
		}
		idx = current
		break
	}
	if idx < 0 || aggregate.ActiveDispatch == nil {
		return RuntimeAggregate{}, "", false, fmt.Errorf("%w: operation recovery authorization is absent", common.ErrConflict)
	}
	execution, found := operationExecutionRecord(aggregate.ActiveDispatch.OperationExecutions, operationID)
	authorization := aggregate.OperationRecoveryAuthorizations[idx].Normalized()
	if !found || execution.State != runtimecontract.OperationExecutionClaimed || aggregate.ActiveDispatch.ID != authorization.DispatchID ||
		execution.ExecutionKey != authorization.ExecutionKey || execution.Fence != authorization.Fence {
		return RuntimeAggregate{}, "", false, fmt.Errorf("%w: operation recovery authorization no longer matches claimed execution", common.ErrConflict)
	}
	if _, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence); found {
		return RuntimeAggregate{}, "", false, fmt.Errorf("%w: operation recovery claim already has a receipt", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	authorization.State = runtimecontract.RuntimeOperationRecoveryAuthorizationInFlight
	authorization.ClaimGeneration = 1
	authorization.ClaimedAt = claimedAt.UTC().Format(time.RFC3339Nano)
	authorization.LeaseDeadline = claimedAt.UTC().Add(runtimecontract.RuntimeOperationRecoveryClaimLeaseDuration).Format(time.RFC3339Nano)
	authorization.ClaimID = runtimecontract.StableRuntimeOperationRecoveryClaimID(authorization)
	aggregate.OperationRecoveryAuthorizations[idx] = authorization
	aggregate.StateRevision++
	if err := c.validateAggregate(aggregate); err != nil {
		return RuntimeAggregate{}, "", false, fmt.Errorf("claim runtime operation recovery authorization: %w", err)
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, "", false, err
	}
	return aggregate, authorization.ClaimID, true, nil
}

package runtime

import (
	"fmt"
	"time"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/governancecheck"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (x *activeDispatchExecution) prepareOperation(operation runtimecontract.RuntimeOperation) (preparedActiveDispatchOperation, error) {
	x.recoveryAuthorization = nil
	execution, found := operationExecutionRecord(x.aggregate.ActiveDispatch.OperationExecutions, operation.ID)
	if !found {
		return preparedActiveDispatchOperation{}, fmt.Errorf("%w: operation %q execution ledger is missing", common.ErrConflict, operation.ID)
	}
	switch execution.State {
	case runtimecontract.OperationExecutionEvidenceCommitted:
		x.completed[operation.ID] = struct{}{}
		return preparedActiveDispatchOperation{flow: activeDispatchExecutionSkip}, nil
	case runtimecontract.OperationExecutionOwnerCommitted:
		return x.prepareOwnerCommittedOperation(operation, execution)
	case runtimecontract.OperationExecutionClaimed:
		return x.prepareClaimedOperation(operation, execution)
	case runtimecontract.OperationExecutionOutcomeUnknown:
		return preparedActiveDispatchOperation{}, operationOutcomeUnknown(operation.ID, "blind replay is forbidden")
	case runtimecontract.OperationExecutionSealed:
		return x.claimSealedOperation(operation)
	default:
		// Aggregate validation excludes this shape. Preserve the former execution
		// coordinator fall-through so this structural split does not introduce a
		// second state classifier at the side-effect boundary.
		return preparedActiveDispatchOperation{execution: execution, flow: activeDispatchExecutionReady}, nil
	}
}

func (x *activeDispatchExecution) prepareOwnerCommittedOperation(operation runtimecontract.RuntimeOperation, execution runtimecontract.RuntimeOperationExecutionRecord) (preparedActiveDispatchOperation, error) {
	receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(x.aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
	if !found {
		return preparedActiveDispatchOperation{}, fmt.Errorf("operation %q owner_committed without persisted receipt", operation.ID)
	}
	return x.projectPersistedOperationReceipt(operation, receipt)
}

func (x *activeDispatchExecution) prepareClaimedOperation(operation runtimecontract.RuntimeOperation, execution runtimecontract.RuntimeOperationExecutionRecord) (preparedActiveDispatchOperation, error) {
	if receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(x.aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence); found {
		return x.projectPersistedOperationReceipt(operation, receipt)
	}
	governanceGrant := governancecheck.GrantForExecution(x.aggregate.OperationGovernanceBindings, execution)
	if governanceGrant != nil {
		if execution.ClaimedAt == runtimecontract.RuntimeOperationClaimMarkerGovernanceGranted {
			started, startedGrant, err := x.component.markGovernanceResumeExecutionStarted(x.ctx, x.workspaceID, x.runID, operation.ID)
			if err != nil {
				return preparedActiveDispatchOperation{}, err
			}
			x.aggregate = started
			x.dispatch = *started.ActiveDispatch
			execution, executionFound := operationExecutionRecord(started.ActiveDispatch.OperationExecutions, operation.ID)
			if !executionFound {
				return preparedActiveDispatchOperation{}, fmt.Errorf("operation %q governance resume execution is missing", operation.ID)
			}
			if startedGrant == nil {
				return preparedActiveDispatchOperation{}, fmt.Errorf("operation %q governance resume grant is missing", operation.ID)
			}
			return preparedActiveDispatchOperation{execution: execution, governanceGrant: startedGrant, flow: activeDispatchExecutionReady}, nil
		}
		if err := x.authorizeClaimedOperationReplay(operation, execution); err != nil {
			return preparedActiveDispatchOperation{}, err
		}
		return preparedActiveDispatchOperation{execution: execution, governanceGrant: governanceGrant, flow: activeDispatchExecutionReady}, nil
	}
	if err := x.authorizeClaimedOperationReplay(operation, execution); err != nil {
		return preparedActiveDispatchOperation{}, err
	}
	x.dispatch = *x.aggregate.ActiveDispatch
	execution, found := operationExecutionRecord(x.dispatch.OperationExecutions, operation.ID)
	if !found || execution.State != runtimecontract.OperationExecutionClaimed {
		return preparedActiveDispatchOperation{}, fmt.Errorf("operation %q recovery lost claimed execution", operation.ID)
	}
	return preparedActiveDispatchOperation{execution: execution, flow: activeDispatchExecutionReady}, nil
}

func (x *activeDispatchExecution) authorizeClaimedOperationReplay(operation runtimecontract.RuntimeOperation, execution runtimecontract.RuntimeOperationExecutionRecord) error {
	if !runtimeOperationHasReplayAuthority(execution) {
		return &runtimeOperationOutcomeUnknownTransitionRequiredError{OperationID: operation.ID}
	}
	// A one-shot call token is accepted only for the latest exact authorization
	// that this process just changed from issued to in-flight. Persisted
	// in-flight state by itself never authorizes an effect after restart.
	if x.recoveryCallToken != "" {
		authorization, authorized := runtimecontract.RuntimeInFlightOperationRecoveryAuthorizationForExecution(
			x.aggregate.OperationRecoveryAuthorizations,
			x.recoveryCallToken,
			x.recoveryNow,
			x.dispatch.ID,
			execution,
		)
		if !authorized {
			return fmt.Errorf("%w: operation recovery call token does not match the latest in-flight authorization", common.ErrConflict)
		}
		value := authorization.Clone()
		x.recoveryAuthorization = &value
		return nil
	}
	// The exact persisted issued authorization is a request for an SM-4
	// pre-call claim CAS, not replay authority. This closes the call-after-CAS
	// crash window: a fresh process can claim an unclaimed issue, but can never
	// reuse a grant whose physical call may already have occurred.
	if authorization, authorized := runtimecontract.RuntimeOperationRecoveryAuthorizationForExecution(x.aggregate.OperationRecoveryAuthorizations, x.dispatch.ID, execution); authorized {
		return &runtimeOperationRecoveryAuthorizationClaimRequiredError{
			OperationID: operation.ID, AuthorizationID: authorization.AuthorizationID,
		}
	}
	if authorization, found := runtimecontract.RuntimeCurrentOperationRecoveryAuthorizationForExecution(x.aggregate.OperationRecoveryAuthorizations, x.dispatch.ID, execution); found {
		switch authorization.State {
		case runtimecontract.RuntimeOperationRecoveryAuthorizationInFlight:
			now, nowErr := time.Parse(time.RFC3339Nano, x.recoveryNow)
			deadline, deadlineErr := time.Parse(time.RFC3339Nano, authorization.LeaseDeadline)
			if nowErr != nil || deadlineErr != nil {
				return common.ErrInvalidArgument
			}
			if !now.After(deadline) {
				return &runtimeOperationRecoveryInFlightError{OperationID: operation.ID, ClaimID: authorization.ClaimID, LeaseDeadline: authorization.LeaseDeadline}
			}
		case runtimecontract.RuntimeOperationRecoveryAuthorizationUncertain:
			// A completed call without a receipt admits one newly budgeted replay.
		case runtimecontract.RuntimeOperationRecoveryAuthorizationSettled:
			return fmt.Errorf("%w: settled recovery authorization has no persisted receipt", common.ErrConflict)
		case runtimecontract.RuntimeOperationRecoveryAuthorizationRetired:
			return fmt.Errorf("%w: retired recovery authorization cannot execute", common.ErrConflict)
		}
	}
	if !runtimeOperationRecoveryBudgetAvailable(x.aggregate) {
		return &runtimeOperationOutcomeUnknownTransitionRequiredError{OperationID: operation.ID}
	}
	return &runtimeOperationRecoveryBudgetRequiredError{OperationID: operation.ID}
}

func runtimeOperationHasReplayAuthority(execution runtimecontract.RuntimeOperationExecutionRecord) bool {
	execution = execution.Normalized()
	return execution.IdempotencyClass == runtimecontract.RuntimeOperationIdempotencySafeToReplay ||
		execution.IdempotencyClass == runtimecontract.RuntimeOperationIdempotencyOwnerManaged || execution.ReplayProof != nil
}

func (x *activeDispatchExecution) claimSealedOperation(operation runtimecontract.RuntimeOperation) (preparedActiveDispatchOperation, error) {
	claimed, err := x.component.claimOperationExecution(x.ctx, x.workspaceID, x.runID, operation.ID)
	if err != nil {
		return preparedActiveDispatchOperation{}, err
	}
	x.aggregate = claimed
	x.dispatch = *x.aggregate.ActiveDispatch
	execution, found := operationExecutionRecord(x.dispatch.OperationExecutions, operation.ID)
	if !found || execution.State != runtimecontract.OperationExecutionClaimed {
		return preparedActiveDispatchOperation{}, fmt.Errorf("operation %q claim was not persisted", operation.ID)
	}
	return preparedActiveDispatchOperation{execution: execution, flow: activeDispatchExecutionReady}, nil
}

func (x *activeDispatchExecution) projectPersistedOperationReceipt(operation runtimecontract.RuntimeOperation, receipt runtimecontract.RuntimeOperationReceipt) (preparedActiveDispatchOperation, error) {
	if receipt.PrerequisiteStale != nil {
		value := receipt.Clone()
		return preparedActiveDispatchOperation{handoff: &RuntimeDispatchExecutionHandoff{
			DispatchID: x.dispatch.ID, OperationID: operation.ID, PrerequisiteStaleReceipt: &value,
		}, flow: activeDispatchExecutionStop}, nil
	}
	if receipt.Outcome == runtimecontract.RuntimeOperationOutcomeGovernanceRequired {
		if receipt.Governance == nil {
			return preparedActiveDispatchOperation{}, fmt.Errorf("persisted governance receipt has no requirement")
		}
		cloned := receipt.Clone()
		result := RuntimeOperationExecutionResult{OperationID: operation.ID, Receipt: cloned, Governance: cloned.Governance}
		return preparedActiveDispatchOperation{handoff: &RuntimeDispatchExecutionHandoff{
			DispatchID: x.dispatch.ID, OperationID: operation.ID, GovernanceRequired: &result,
		}, flow: activeDispatchExecutionStop}, nil
	}
	evidence, err := operationEvidenceFromPersistedReceipt(x.aggregate, x.dispatch, operation, receipt)
	if err != nil {
		return preparedActiveDispatchOperation{}, err
	}
	return preparedActiveDispatchOperation{handoff: &RuntimeDispatchExecutionHandoff{
		DispatchID: x.dispatch.ID, OperationID: operation.ID, Evidence: &evidence,
	}, flow: activeDispatchExecutionStop}, nil
}

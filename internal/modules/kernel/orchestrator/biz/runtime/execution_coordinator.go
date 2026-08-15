package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runtimeOperationRecoveryBudgetRequiredError is a read-only handoff from
// SM-3 execution to the SM-4 recovery-budget event. It deliberately carries no
// aggregate or mutation callback: the next event must reload the aggregate and
// own its independent CAS.
type runtimeOperationRecoveryBudgetRequiredError struct {
	OperationID string
}

func (e *runtimeOperationRecoveryBudgetRequiredError) Error() string {
	if e == nil || e.OperationID == "" {
		return "runtime operation recovery budget is required"
	}
	return fmt.Sprintf("runtime operation %q requires recovery budget", e.OperationID)
}

// RuntimeOperationRecoveryBudgetRequired classifies the owner-local handoff
// without exposing a mutation entry or the concrete error type to Biz.
func RuntimeOperationRecoveryBudgetRequired(err error) (string, bool) {
	var required *runtimeOperationRecoveryBudgetRequiredError
	if !errors.As(err, &required) || required == nil || required.OperationID == "" {
		return "", false
	}
	return required.OperationID, true
}

func runtimeOperationRecoveryBudgetAvailable(aggregate RuntimeAggregate) bool {
	policy := aggregate.Facts.RecoveryPolicy.Normalized()
	return aggregate.Facts.Recovery.OperationRecoveryAttempts < policy.MaxOperationRecoveryAttempts
}

func (c *candidateDispatchComponent) executeActiveDispatch(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, executor RuntimeOperationExecutor, recoveryOperationID string, recoveryBudgetAttempt int) (RuntimeAggregate, error) {
	execution, err := prepareActiveDispatchExecution(ctx, c, workspaceID, runID, executor, recoveryOperationID, recoveryBudgetAttempt)
	if err != nil {
		return RuntimeAggregate{}, err
	}
	return execution.run()
}

func (c *candidateDispatchComponent) recoverOperationReceiptAfterExecuteError(
	ctx context.Context,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	dispatch SealedDispatch,
	operation runtimecontract.RuntimeOperation,
	execution runtimecontract.RuntimeOperationExecutionRecord,
) (RuntimeAggregate, bool, error) {
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, false, err
	}
	receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
	if !found {
		return aggregate, false, nil
	}
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.ID != dispatch.ID {
		return RuntimeAggregate{}, false, fmt.Errorf("%w: recovered owner receipt has no matching active dispatch", common.ErrConflict)
	}
	persistedExecution, found := operationExecutionRecord(aggregate.ActiveDispatch.OperationExecutions, operation.ID)
	if !found || persistedExecution.State != runtimecontract.OperationExecutionOwnerCommitted ||
		persistedExecution.ExecutionKey != receipt.ExecutionKey || persistedExecution.Fence != receipt.Fence {
		return RuntimeAggregate{}, false, fmt.Errorf("%w: recovered owner receipt does not match owner-committed execution", common.ErrConflict)
	}
	if receipt.Outcome == runtimecontract.RuntimeOperationOutcomeGovernanceRequired {
		if receipt.Governance == nil {
			return RuntimeAggregate{}, false, fmt.Errorf("recovered governance receipt has no requirement")
		}
		recovered, recoverErr := c.recordOperationGovernanceRequired(ctx, workspaceID, runID, operation.ID, RuntimeOperationExecutionResult{
			OperationID: operation.ID,
			Receipt:     receipt,
			Governance:  receipt.Governance,
		})
		return recovered, true, recoverErr
	}
	commit, commitErr := c.commitPersistedOperationReceiptEvidence(ctx, aggregate, *aggregate.ActiveDispatch, operation, receipt)
	if commitErr != nil {
		return RuntimeAggregate{}, false, commitErr
	}
	return commit.Aggregate, true, nil
}

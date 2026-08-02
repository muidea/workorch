package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (c *RuntimeController) QueryOperationReceipt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID, operationID, executionKey string, fence uint64, grant *runtimecontract.RuntimeOperationGovernanceGrant) (runtimecontract.RuntimeOperationReceipt, bool, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(dispatchID) == "" || strings.TrimSpace(operationID) == "" || strings.TrimSpace(executionKey) == "" || fence == 0 {
		return runtimecontract.RuntimeOperationReceipt{}, false, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return runtimecontract.RuntimeOperationReceipt{}, false, err
	}
	if grant != nil {
		value := grant.Normalized()
		if err := value.Validate(); err != nil {
			return runtimecontract.RuntimeOperationReceipt{}, false, err
		}
		execution := runtimecontract.RuntimeOperationExecutionRecord{ExecutionKey: executionKey, Fence: fence}
		persisted := governanceGrantForExecution(aggregate, execution)
		if persisted == nil || !runtimecontract.SameRuntimeOperationGovernanceGrant(*persisted, value) {
			return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: governance grant does not match persisted binding", common.ErrConflict)
		}
	}
	receipt, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, executionKey, fence)
	if !ok {
		return runtimecontract.RuntimeOperationReceipt{}, false, nil
	}
	if receipt.OperationID != strings.TrimSpace(operationID) {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: receipt operation does not match query", common.ErrConflict)
	}
	if !aggregateContainsDispatch(aggregate, dispatchID) {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: receipt dispatch is not present in runtime aggregate", common.ErrConflict)
	}
	return receipt, true, nil
}

func (c *RuntimeController) CommitOperationReceipt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, fence uint64, receipt runtimecontract.RuntimeOperationReceipt) (RuntimeAggregate, runtimecontract.RuntimeOperationReceipt, error) {
	if c == nil || c.repository == nil || workspaceID == "" || runID == "" || strings.TrimSpace(dispatchID) == "" || fence == 0 {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, common.ErrInvalidArgument
	}
	receipt = receipt.Normalized()
	receipt.Fence = fence
	if err := receipt.Validate(); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, err
	}
	aggregate, err := c.repository.Load(ctx, workspaceID, runID)
	if err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, err
	}
	if err := rejectRuntimeProgressAfterTermination(aggregate); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, err
	}
	if existing, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, receipt.ExecutionKey, receipt.Fence); ok {
		if !runtimecontract.SameRuntimeOperationReceipt(existing, receipt) {
			return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("%w: execution key already has a different terminal receipt", common.ErrConflict)
		}
		return aggregate, existing, nil
	}
	if err := validateOperationReceiptFeedback(aggregate, workspaceID, runID, dispatchID, receipt); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, err
	}
	if aggregate.ActiveDispatch == nil || aggregate.ActiveDispatch.ID != strings.TrimSpace(dispatchID) {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("%w: receipt does not match active dispatch", common.ErrConflict)
	}
	expectedRevision := aggregate.StateRevision
	found := false
	for idx := range aggregate.ActiveDispatch.OperationExecutions {
		execution := &aggregate.ActiveDispatch.OperationExecutions[idx]
		if execution.OperationID != receipt.OperationID || execution.ExecutionKey != receipt.ExecutionKey {
			continue
		}
		found = true
		if execution.Fence != fence || execution.State != runtimecontract.OperationExecutionClaimed {
			return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("%w: stale operation receipt fence/state", common.ErrConflict)
		}
		execution.State = runtimecontract.OperationExecutionOwnerCommitted
		execution.OwnerReceiptRef = "runtime-receipt/" + receipt.ExecutionKey
		execution.OwnerReceiptHash = runtimecontract.StableRuntimeOperationReceiptHash(receipt)
		break
	}
	if !found {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("%w: receipt execution is not sealed", common.ErrConflict)
	}
	aggregate.OperationReceipts = append(aggregate.OperationReceipts, receipt.Clone())
	aggregate.StateRevision++
	if err := aggregate.Validate(); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, err
	}
	if err := c.transitions.Commit(ctx, aggregate, expectedRevision); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, err
	}
	return aggregate, receipt, nil
}

func validateOperationReceiptFeedback(aggregate RuntimeAggregate, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, receipt runtimecontract.RuntimeOperationReceipt) error {
	if receipt.Failure == nil {
		if receipt.Feedback != nil {
			return fmt.Errorf("operation receipt without failure cannot carry diagnostic feedback")
		}
		return nil
	}
	if receipt.Feedback == nil {
		return fmt.Errorf("rejected operation receipt requires owner diagnostic feedback")
	}
	feedback := receipt.Feedback.Normalized()
	if err := feedback.Validate(); err != nil {
		return err
	}
	if feedback.Domain != runtimecontract.RuntimeFeedbackDomainOperation || feedback.StableCategory != runtimecontract.RuntimeFeedbackCategory(receipt.Outcome) ||
		feedback.WorkspaceID != workspaceID || feedback.RunID != runID || feedback.DispatchID != strings.TrimSpace(dispatchID) || feedback.OriginalDiagnostic == nil ||
		feedback.OriginalDiagnostic.OperationID != receipt.OperationID || feedback.OriginalDiagnostic.OriginalClass != string(receipt.Failure.Class) ||
		feedback.OriginalDiagnostic.OriginalCode != receipt.Failure.Code || feedback.OriginalDiagnostic.OriginalMessage != receipt.Failure.Message || feedback.OriginalDiagnostic.Signature != receipt.Failure.Signature {
		return fmt.Errorf("operation receipt feedback does not match receipt identity or failure")
	}
	dispatch := aggregate.ActiveDispatch
	if dispatch == nil || dispatch.ID != strings.TrimSpace(dispatchID) {
		return fmt.Errorf("%w: receipt feedback dispatch is not active", common.ErrConflict)
	}
	operation, found := candidateOperationByID(dispatch.Candidate, receipt.OperationID)
	if !found {
		return fmt.Errorf("%w: receipt feedback operation is not sealed", common.ErrConflict)
	}
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !found || feedback.OriginalDiagnostic.OwnerModuleID != descriptor.OwnerModuleID || feedback.AuthoritySnapshotID != dispatch.Transaction.AuthoritySnapshotID {
		return fmt.Errorf("operation receipt feedback does not match sealed owner or authority")
	}
	return nil
}

func aggregateContainsDispatch(aggregate RuntimeAggregate, dispatchID string) bool {
	dispatchID = strings.TrimSpace(dispatchID)
	if aggregate.ActiveDispatch != nil && aggregate.ActiveDispatch.ID == dispatchID {
		return true
	}
	for _, dispatch := range aggregate.CompletedDispatches {
		if dispatch.ID == dispatchID {
			return true
		}
	}
	return false
}

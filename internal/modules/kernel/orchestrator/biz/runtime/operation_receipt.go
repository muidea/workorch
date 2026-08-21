package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/governancecheck"
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
	dispatch, active, found := operationReceiptDispatch(aggregate, dispatchID)
	if !found {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: receipt query dispatch is not present in runtime aggregate", common.ErrConflict)
	}
	if _, found := candidateOperationByID(dispatch.Candidate, operationID); !found {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: receipt query operation is not sealed in dispatch", common.ErrConflict)
	}
	execution, found := operationExecutionRecord(dispatch.OperationExecutions, operationID)
	if !found || execution.ExecutionKey != strings.TrimSpace(executionKey) || execution.Fence != fence {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: receipt query does not match sealed execution key/fence", common.ErrConflict)
	}
	persistedGrant := governancecheck.GrantForExecution(aggregate.OperationGovernanceBindings, execution)
	if (persistedGrant == nil) != (grant == nil) {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: governance grant presence does not match persisted binding", common.ErrConflict)
	}
	if grant != nil {
		value := grant.Normalized()
		if err := value.Validate(); err != nil {
			return runtimecontract.RuntimeOperationReceipt{}, false, err
		}
		if !governancecheck.SameGrant(*persistedGrant, value) {
			return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: governance grant does not match persisted binding", common.ErrConflict)
		}
	}
	receipt, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, executionKey, fence)
	if !ok {
		// found=false is itself an execution authorization: only the exact active
		// claimed record may proceed to an owner side effect. Sealed, stale,
		// completed, invalidated, and fabricated identities fail closed.
		if !active || execution.State != runtimecontract.OperationExecutionClaimed {
			return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: receipt query execution is not active and claimed", common.ErrConflict)
		}
		return runtimecontract.RuntimeOperationReceipt{}, false, nil
	}
	if receipt.OperationID != strings.TrimSpace(operationID) {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: receipt operation does not match query", common.ErrConflict)
	}
	if execution.State != runtimecontract.OperationExecutionOwnerCommitted && execution.State != runtimecontract.OperationExecutionEvidenceCommitted {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: receipt query execution state does not admit replay", common.ErrConflict)
	}
	if execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) {
		return runtimecontract.RuntimeOperationReceipt{}, false, fmt.Errorf("%w: receipt query hash does not match execution ledger", common.ErrConflict)
	}
	return receipt, true, nil
}

func (c *candidateDispatchComponent) commitOperationReceipt(ctx context.Context, workspaceID common.WorkspaceID, runID common.RunID, dispatchID string, fence uint64, receipt runtimecontract.RuntimeOperationReceipt) (RuntimeAggregate, runtimecontract.RuntimeOperationReceipt, error) {
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
	// A receipt is an owner fact, but accepting it into the Runtime aggregate is
	// still an SM-3 admission. Do not persist a new receipt once the focus has
	// lost its canonical binding: evidence cannot legally attribute it and a
	// later retry must not turn that stale fact into a second execution path.
	// Existing matching receipts remain idempotent above.
	if err := validateCurrentCWIBindingFenceForEvidenceCommit(aggregate.Facts); err != nil {
		return RuntimeAggregate{}, runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("%w: operation receipt binding fence: %v", common.ErrConflict, err)
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
	if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected {
		if receipt.Feedback != nil {
			return fmt.Errorf("non-rejected operation receipt cannot carry diagnostic feedback")
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
		feedback.OriginalDiagnostic.OperationID != receipt.OperationID {
		return fmt.Errorf("operation receipt feedback does not match receipt identity")
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

func failureFactFromOperationReceiptFeedback(receipt runtimecontract.RuntimeOperationReceipt) *runtimecontract.FailureFact {
	if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || receipt.Feedback == nil || receipt.Feedback.OriginalDiagnostic == nil {
		return nil
	}
	diagnostic := receipt.Feedback.OriginalDiagnostic.CloneOriginal()
	failure := &runtimecontract.FailureFact{
		Class: runtimecontract.FailureClass(diagnostic.OriginalClass), Code: diagnostic.OriginalCode,
		Signature: diagnostic.Signature, Message: diagnostic.OriginalMessage,
		OriginalDiagnostic: diagnostic.CloneOriginal(), TargetRefs: append([]string(nil), diagnostic.TargetRefs...),
	}
	if diagnostic.TargetObservation != nil {
		value := diagnostic.TargetObservation.Clone()
		failure.TargetObservation = &value
	}
	if diagnostic.ValidationDiagnostic != nil {
		value := diagnostic.ValidationDiagnostic.Clone()
		failure.ValidationDiagnostic = &value
	}
	if diagnostic.ValidationFailureObservation != nil {
		value := diagnostic.ValidationFailureObservation.Clone()
		failure.ValidationFailureObservation = &value
	}
	if diagnostic.GovernanceDependency != nil {
		value := diagnostic.GovernanceDependency.Clone()
		failure.GovernanceDependency = &value
	}
	return failure
}

func operationReceiptDispatch(aggregate RuntimeAggregate, dispatchID string) (SealedDispatch, bool, bool) {
	dispatchID = strings.TrimSpace(dispatchID)
	if aggregate.ActiveDispatch != nil && aggregate.ActiveDispatch.ID == dispatchID {
		return *aggregate.ActiveDispatch, true, true
	}
	for _, dispatch := range aggregate.CompletedDispatches {
		if dispatch.ID == dispatchID {
			return dispatch, false, true
		}
	}
	return SealedDispatch{}, false, false
}

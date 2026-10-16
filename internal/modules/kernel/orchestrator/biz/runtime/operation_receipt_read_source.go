package runtime

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// queryOperationReceiptWithReadSource 只反证原封存来源，不授权新的 execute 或 Runtime transition。
func (c *RuntimeController) queryOperationReceiptWithReadSource(ctx context.Context, source runtimecontract.RuntimeOperationExecutionContext, owner string) (runtimecontract.RuntimeOperationReceipt, error) {
	if source.WorkspaceID == "" || string(source.WorkspaceID) != strings.TrimSpace(string(source.WorkspaceID)) ||
		source.RunID == "" || string(source.RunID) != strings.TrimSpace(string(source.RunID)) ||
		source.DispatchID == "" || source.DispatchID != strings.TrimSpace(source.DispatchID) ||
		source.OperationID == "" || source.OperationID != strings.TrimSpace(source.OperationID) {
		return runtimecontract.RuntimeOperationReceipt{}, common.ErrInvalidArgument
	}
	if err := runtimecontract.ValidateRuntimeOperationExecutionContextScope(source); err != nil {
		return runtimecontract.RuntimeOperationReceipt{}, err
	}
	if c == nil || c.repository == nil || !exactOperationReceiptOwnerSource(owner) {
		return runtimecontract.RuntimeOperationReceipt{}, common.ErrInvalidArgument
	}
	aggregate, err := c.repository.Load(ctx, source.WorkspaceID, source.RunID)
	if err != nil {
		return runtimecontract.RuntimeOperationReceipt{}, err
	}
	receipt, found, err := queryOperationReceiptFromAggregate(aggregate, source.DispatchID, source.OperationID, source.Execution.ExecutionKey, source.Execution.Fence, owner, source.Execution.GovernanceGrant)
	if err != nil {
		return runtimecontract.RuntimeOperationReceipt{}, err
	}
	if !found {
		return runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("%w: read dependency source requires a persisted receipt", common.ErrConflict)
	}
	dispatch, _, found := operationReceiptDispatch(aggregate, source.DispatchID)
	if !found || source.Transaction != dispatch.Transaction || source.Transaction.DefinitionRevision != aggregate.Facts.DefinitionRevision {
		return runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("%w: read dependency source transaction mismatch", common.ErrConflict)
	}
	operation, found := candidateOperationByID(dispatch.Candidate, source.OperationID)
	if !found || !reflect.DeepEqual(operation.TargetRefs, source.TargetRefs) || !reflect.DeepEqual(operation.CoverageWorkItemIDs, source.CoverageWorkItemIDs) {
		return runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("%w: read dependency source operation scope mismatch", common.ErrConflict)
	}
	if source.HistoricalEffectProofContractRef != runtimeHistoricalEffectProofContractForDispatch(dispatch, operation) {
		return runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("%w: read source proof purpose mismatch", common.ErrConflict)
	}
	if err := runtimecontract.ValidateRuntimeOperationExecutionAuthority(operation, source.Execution, nil); err != nil {
		return runtimecontract.RuntimeOperationReceipt{}, err
	}
	execution, found := operationExecutionRecord(dispatch.OperationExecutions, source.OperationID)
	if !found || execution.ExecutionKey != source.Execution.ExecutionKey || execution.Fence != source.Execution.Fence ||
		execution.OperationHash != source.Execution.OperationHash || execution.AuthorityHash != source.Execution.AuthorityHash ||
		execution.AdmissibilitySnapshotID != source.Execution.AdmissibilitySnapshotID || execution.AcquisitionBinding != source.Execution.AcquisitionBinding ||
		!reflect.DeepEqual(execution.ReplayProof, source.Execution.ReplayProof) || execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) {
		return runtimecontract.RuntimeOperationReceipt{}, fmt.Errorf("%w: read dependency source sealed execution mismatch", common.ErrConflict)
	}
	return receipt.Clone(), nil
}

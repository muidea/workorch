package runtime

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/governancecheck"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// RuntimeOperationReadSource 是原持久回执的 detached 读取来源，不是新的 execution authorization。
type RuntimeOperationReadSource struct {
	Context              runtimecontract.RuntimeOperationExecutionContext
	Operation            runtimecontract.RuntimeOperation
	SourceReceiptHash    string
	OriginalDependencies runtimecontract.RuntimeOwnerRevisionSet
	SourceContentAddress *runtimecontract.RuntimeEvidenceContentAddress
	ApplicabilitySource  *runtimecontract.RuntimeEvidenceApplicabilityDependencies
}

// CompileRuntimeOperationReadSource consumes the Run-owner committed aggregate
// and projects one original sealed source. It is not a raw snapshot admission:
// external snapshots enter AdmitRuntimeOwnerStateRecord before this owner-local
// read. Only the selected source's scope, receipt and execution joins belong
// here; historical business decisions are not readmission prerequisites.
func CompileRuntimeOperationReadSource(aggregate RuntimeAggregate, dispatchID, operationID, receiptHash string) (RuntimeOperationReadSource, error) {
	if dispatchID == "" || dispatchID != strings.TrimSpace(dispatchID) || operationID == "" || operationID != strings.TrimSpace(operationID) || receiptHash == "" || receiptHash != strings.TrimSpace(receiptHash) {
		return RuntimeOperationReadSource{}, common.ErrInvalidArgument
	}
	dispatch, _, found := operationReceiptDispatch(aggregate, dispatchID)
	if !found {
		return RuntimeOperationReadSource{}, fmt.Errorf("%w: read source dispatch is not persisted", common.ErrConflict)
	}
	if aggregate.StateRevision == 0 || dispatch.WorkspaceID != aggregate.Facts.WorkspaceID || dispatch.RunID != aggregate.Facts.RunID ||
		dispatch.Transaction.DefinitionRevision != aggregate.Facts.DefinitionRevision {
		return RuntimeOperationReadSource{}, fmt.Errorf("%w: read source dispatch scope does not match committed owner", common.ErrConflict)
	}
	operation, found := candidateOperationByID(dispatch.Candidate, operationID)
	if !found {
		return RuntimeOperationReadSource{}, fmt.Errorf("%w: read source operation is not sealed", common.ErrConflict)
	}
	descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
	if !found || !descriptor.RequiresOwnerReceipt || descriptor.OwnerModuleID == "" {
		return RuntimeOperationReadSource{}, fmt.Errorf("%w: read source operation has no receipt owner", common.ErrConflict)
	}
	execution, found := operationExecutionRecord(dispatch.OperationExecutions, operationID)
	if !found || execution.OwnerReceiptHash != receiptHash {
		return RuntimeOperationReadSource{}, fmt.Errorf("%w: read source receipt hash does not match sealed execution", common.ErrConflict)
	}
	grant := governancecheck.GrantForExecution(aggregate.OperationGovernanceBindings, execution)
	receipt, found, err := queryOperationReceiptFromAggregate(aggregate, dispatchID, operationID, execution.ExecutionKey, execution.Fence, descriptor.OwnerModuleID, grant)
	if err != nil {
		return RuntimeOperationReadSource{}, err
	}
	if !found || runtimecontract.StableRuntimeOperationReceiptHash(receipt) != receiptHash || receipt.Evidence.OwnerRevisionSet == nil {
		return RuntimeOperationReadSource{}, fmt.Errorf("%w: read source requires original receipt dependencies", common.ErrConflict)
	}
	if err := runtimecontract.ValidateRuntimeEvidenceDependencyPair(receipt.Evidence.OwnerRevisionSet, receipt.Evidence.ApplicabilityDependencies, dispatch.Transaction.DefinitionRevision); err != nil {
		return RuntimeOperationReadSource{}, err
	}
	evidenceKind := ""
	switch {
	case dispatch.Candidate.StateChange != nil:
		evidenceKind = runtimecontract.EvidenceKindStateChange
	case dispatch.Candidate.Validation != nil:
		evidenceKind = runtimecontract.EvidenceKindValidation
	}
	source := runtimecontract.CloneRuntimeOperationExecutionContext(runtimecontract.RuntimeOperationExecutionContext{
		WorkspaceID: aggregate.Facts.WorkspaceID, RunID: aggregate.Facts.RunID, DispatchID: dispatch.ID,
		Transaction: dispatch.Transaction, OperationID: operation.ID,
		TargetRefs: operation.TargetRefs, CoverageWorkItemIDs: operation.CoverageWorkItemIDs,
		EvidenceKind: evidenceKind, EvidencePolicies: aggregate.Facts.Activation.EvidencePolicies,
		HistoricalEffectProofContractRef: runtimeHistoricalEffectProofContractForDispatch(dispatch, operation),
		Execution: runtimecontract.RuntimeOperationExecutionEnvelope{
			ExecutionKey: execution.ExecutionKey, Fence: execution.Fence, OperationHash: execution.OperationHash,
			ExpectedEffectScope: execution.ExpectedEffectScope, IdempotencyClass: execution.IdempotencyClass,
			AuthorityHash: execution.AuthorityHash, AdmissibilitySnapshotID: execution.AdmissibilitySnapshotID,
			AcquisitionBinding: execution.AcquisitionBinding, ReplayProof: execution.ReplayProof, GovernanceGrant: grant,
		},
	})
	if err := runtimecontract.ValidateRuntimeOperationExecutionContextScope(source); err != nil {
		return RuntimeOperationReadSource{}, err
	}
	policies := runtimecontract.RuntimeDispatchSealedOperationPolicies(runtimecontract.RuntimeDispatchRecord{
		OperationAdmissibilities: dispatch.OperationAdmissibilities, OperationPolicies: dispatch.OperationPolicies,
	})
	if err := runtimecontract.ValidateRuntimeOperationExecutionAuthority(operation, source.Execution, policies); err != nil {
		return RuntimeOperationReadSource{}, err
	}
	var address *runtimecontract.RuntimeEvidenceContentAddress
	if receipt.Evidence.ContentAddress != nil {
		value := *receipt.Evidence.ContentAddress
		address = &value
	}
	return RuntimeOperationReadSource{
		Context: source, Operation: operation.Clone(), SourceReceiptHash: receiptHash,
		OriginalDependencies: *runtimecontract.CloneRuntimeOwnerRevisionSet(receipt.Evidence.OwnerRevisionSet),
		SourceContentAddress: address,
		ApplicabilitySource:  runtimecontract.CloneRuntimeEvidenceApplicabilityDependencies(receipt.Evidence.ApplicabilityDependencies),
	}, nil
}

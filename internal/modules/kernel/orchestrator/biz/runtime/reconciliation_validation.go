package runtime

import (
	"fmt"
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ReconciliationValidationResultSource projects an already committed result.
// The owner receipt and its exact binding supply the source; this read never
// recompiles the historical validation method or judges its business outcome.
// a is the committed owner aggregate, already admitted by the ordinary read.
func ReconciliationValidationResultSource(a RuntimeAggregate, binding runtimecontract.RuntimeCWIReconciliationBindingRef) (*runtimecontract.RuntimeOperationExecutionContext, string, error) {
	for i := len(a.CompletedDispatches) - 1; i >= 0; i-- {
		d := a.CompletedDispatches[i]
		if d.Candidate.Validation == nil {
			continue
		}
		b, ok := runtimecontract.CoverageEvidenceBindingForRef(d.CoverageBindings, binding.BindingID)
		if !ok || b.CoverageWorkItemID != binding.CoverageWorkItemID || b.DefinitionRevision != a.Facts.DefinitionRevision || runtimecontract.StableCoverageEvidenceBindingHash(b) != binding.BindingHash {
			continue
		}
		operations, err := orderedCandidateOperations(d.Candidate)
		if err != nil {
			return nil, "", err
		}
		for _, op := range operations {
			if op.Kind != runtimecontract.RuntimeBlockKindManagedValidationSession || len(op.BindingRefs) != 1 || op.BindingRefs[0] != binding.BindingID || !slices.Contains(d.CompletedOperationIDs, op.ID) {
				continue
			}
			execution, ok := operationExecutionRecord(d.OperationExecutions, op.ID)
			if !ok || execution.State != runtimecontract.OperationExecutionEvidenceCommitted {
				continue
			}
			receipt, ok := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(a.OperationReceipts, execution.ExecutionKey, execution.Fence)
			if !ok {
				return nil, "", fmt.Errorf("%w: committed validation receipt missing", common.ErrConflict)
			}
			if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
				return nil, "", nil
			}
			source, err := compileRuntimeOperationReadSource(a, d.ID, op.ID, execution.OwnerReceiptHash)
			if err != nil {
				return nil, "", err
			}
			return &source.Context, source.SourceReceiptHash, nil
		}
	}
	return nil, "", nil
}

// Full historical method expansion is retained for offline execution-use audits
// of original records, not the production reconciliation query path.
func reconciliationValidationContext(aggregate RuntimeAggregate, binding runtimecontract.RuntimeCWIReconciliationBindingRef, requireAccepted bool, receiptHash string) ([]string, []runtimecontract.RuntimeValidationRecipe, string) {
	for i := len(aggregate.CompletedDispatches) - 1; i >= 0; i-- {
		dispatch := aggregate.CompletedDispatches[i]
		if dispatch.State != DispatchStateCommitted || dispatch.WorkspaceID != aggregate.Facts.WorkspaceID || dispatch.RunID != aggregate.Facts.RunID || dispatch.DispatchAuthorityHash == "" {
			continue
		}
		// Membership is only a necessary applicability filter. Unrelated seals
		// cannot supply this source; every matching seal still requires its full
		// original authority check before any context can be returned.
		sealed, found := runtimecontract.CoverageEvidenceBindingForRef(dispatch.CoverageBindings, binding.BindingID)
		if !found || sealed.CoverageWorkItemID != binding.CoverageWorkItemID || sealed.DefinitionRevision != aggregate.Facts.DefinitionRevision || runtimecontract.StableCoverageEvidenceBindingHash(sealed) != binding.BindingHash {
			continue
		}
		if dispatch.DispatchAuthorityHash != runtimecontract.StableRuntimeDispatchAuthorityHash((dispatch)) {
			continue
		}
		operations, err := orderedCandidateOperations(dispatch.Candidate)
		if err != nil {
			continue
		}
		for _, operation := range operations {
			if runtimecontract.CanonicalRuntimeBlockKind(operation.Kind) != runtimecontract.RuntimeBlockKindManagedValidationSession || len(operation.BindingRefs) != 1 || operation.BindingRefs[0] != binding.BindingID || !slices.Contains(dispatch.CompletedOperationIDs, operation.ID) {
				continue
			}
			execution, found := operationExecutionRecord(dispatch.OperationExecutions, operation.ID)
			if !found {
				continue
			}
			sourceHash := ""
			if requireAccepted {
				receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(aggregate.OperationReceipts, execution.ExecutionKey, execution.Fence)
				if !found || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted {
					continue
				}
				sourceHash = runtimecontract.StableRuntimeOperationReceiptHash(receipt)
				if execution.OwnerReceiptHash != sourceHash || (receiptHash != "" && sourceHash != receiptHash) {
					continue
				}
			}
			catalog := runtimeOwnerCatalogForOperation(dispatch.OperationAdmissibilities, dispatch.OperationAdmissibilityObservations, operation, execution.AdmissibilitySnapshotID, execution.AcquisitionBinding)
			proof, err := runtimeValidationProofBindingForOperation(dispatch, operation, catalog)
			if err != nil || len(binding.ValidationRecipeIDs) != 1 || binding.ValidationRecipeIDs[0] != proof.ValidationRecipeID || !runtimecontract.SameRuntimeValidationCheckAttestationBindings(binding.ValidationCheckAttestations, proof.CheckAttestations) || !sameRuntimeStrings(binding.RequiredCheckRefs, proof.RequiredCheckRefs) {
				continue
			}
			scope, err := runtimeValidationScopeFromOwnerCatalog(dispatch, operation, catalog)
			if err != nil || len(scope) == 0 {
				continue
			}
			for _, recipe := range dispatch.ValidationRecipes {
				if recipe.ID == proof.ValidationRecipeID {
					return scope, runtimecontract.NormalizeRuntimeValidationRecipes([]runtimecontract.RuntimeValidationRecipe{recipe}), sourceHash
				}
			}
		}
	}
	return nil, nil, ""
}

package runtime

import (
	"slices"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ReconciliationValidationContext is a read-only projection of an exact SM-3
// seal. It cannot acquire a new catalog, expand targets or grant CWI credit.
// Missing/legacy authority stays unavailable instead of being reconstructed.
func ReconciliationValidationContext(aggregate RuntimeAggregate, binding runtimecontract.RuntimeCWIReconciliationBindingRef) ([]string, []runtimecontract.RuntimeValidationRecipe) {
	scope, recipes, _ := reconciliationValidationContext(aggregate, binding, false, "")
	return scope, recipes
}

// ReconciliationValidationExecutionSource requires a real accepted receipt
// for fresh grouped observations; it never promotes a completed-but-rejected
// operation into a reusable success.
func ReconciliationValidationExecutionSource(aggregate RuntimeAggregate, binding runtimecontract.RuntimeCWIReconciliationBindingRef) ([]string, []runtimecontract.RuntimeValidationRecipe, string) {
	return reconciliationValidationContext(aggregate, binding, true, "")
}

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

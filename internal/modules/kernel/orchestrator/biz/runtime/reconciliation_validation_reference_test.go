package runtime

import (
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"slices"
)

// Frozen complete source lookup before applicability-first scanning. Keep independent
// from the production lookup so rejected sources cannot certify one another.
func referenceReconciliationValidationContext(aggregate RuntimeAggregate, binding runtimecontract.RuntimeCWIReconciliationBindingRef, requireAccepted bool, receiptHash string) ([]string, []runtimecontract.RuntimeValidationRecipe, string) {
	for i := len(aggregate.CompletedDispatches) - 1; i >= 0; i-- {
		dispatch := aggregate.CompletedDispatches[i]
		if dispatch.State != DispatchStateCommitted || dispatch.WorkspaceID != aggregate.Facts.WorkspaceID || dispatch.RunID != aggregate.Facts.RunID ||
			dispatch.DispatchAuthorityHash == "" || dispatch.DispatchAuthorityHash != runtimecontract.StableRuntimeDispatchAuthorityHash(sealedDispatchToRuntimeRecord(dispatch)) {
			continue
		}
		sealed, found := runtimecontract.CoverageEvidenceBindingForRef(dispatch.CoverageBindings, binding.BindingID)
		if !found || sealed.CoverageWorkItemID != binding.CoverageWorkItemID || sealed.DefinitionRevision != aggregate.Facts.DefinitionRevision || runtimecontract.StableCoverageEvidenceBindingHash(sealed) != binding.BindingHash {
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
				if !found || receipt.Outcome != runtimecontract.RuntimeOperationOutcomeAccepted || execution.OwnerReceiptHash != runtimecontract.StableRuntimeOperationReceiptHash(receipt) {
					continue
				}
				sourceHash = runtimecontract.StableRuntimeOperationReceiptHash(receipt)
				if receiptHash != "" && sourceHash != receiptHash {
					continue
				}
			}
			catalog := historySourceReferenceRuntimeOwnerCatalogForOperation(dispatch.OperationAdmissibilities, dispatch.OperationAdmissibilityObservations, operation, execution.AdmissibilitySnapshotID, execution.AcquisitionBinding)
			proof, err := historySourceReferenceRuntimeValidationProofBindingForOperation(dispatch, operation, catalog)
			if err != nil || len(binding.ValidationRecipeIDs) != 1 || binding.ValidationRecipeIDs[0] != proof.ValidationRecipeID || !runtimecontract.SameRuntimeValidationCheckAttestationBindings(binding.ValidationCheckAttestations, proof.CheckAttestations) || !sameRuntimeStrings(binding.RequiredCheckRefs, proof.RequiredCheckRefs) {
				continue
			}
			scope, err := historySourceReferenceRuntimeValidationScopeTargetRefs(dispatch, operation)
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

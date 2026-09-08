package runtime

import (
	"slices"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// ReconciliationValidationContext is a read-only projection of an exact SM-3
// seal. It cannot acquire a new catalog, expand targets or grant CWI credit.
// Missing/legacy authority stays unavailable instead of being reconstructed.
func ReconciliationValidationContext(aggregate RuntimeAggregate, binding runtimecontract.RuntimeCWIReconciliationBindingRef) ([]string, []runtimecontract.RuntimeValidationRecipe) {
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
			catalog := runtimeOwnerCatalogForOperation(dispatch.OperationAdmissibilities, dispatch.OperationAdmissibilityObservations, operation, execution.AdmissibilitySnapshotID, execution.AcquisitionBinding)
			proof, err := runtimeValidationProofBindingForOperation(dispatch, operation, catalog)
			if err != nil || len(binding.ValidationRecipeIDs) != 1 || binding.ValidationRecipeIDs[0] != proof.ValidationRecipeID || !runtimecontract.SameRuntimeValidationCheckAttestationBindings(binding.ValidationCheckAttestations, proof.CheckAttestations) || !sameRuntimeStrings(binding.RequiredCheckRefs, proof.RequiredCheckRefs) {
				continue
			}
			scope, err := runtimeValidationScopeTargetRefs(dispatch, operation)
			if err != nil || len(scope) == 0 {
				continue
			}
			for _, recipe := range dispatch.ValidationRecipes {
				if recipe.ID == proof.ValidationRecipeID {
					return scope, runtimecontract.NormalizeRuntimeValidationRecipes([]runtimecontract.RuntimeValidationRecipe{recipe})
				}
			}
		}
	}
	return nil, nil
}

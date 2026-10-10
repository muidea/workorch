package runtime

import (
	"reflect"
	"slices"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// runtimeVerificationPolicySatisfied is an additional SM-2 closure guard, not
// an alternative to the exact proof/lineage gates. Source identity is derived
// from the sealed operation descriptor and its committed owner receipt, never
// from evidence IDs, provider claims, tags or owner-authored arbitrary labels.
func runtimeVerificationPolicySatisfied(aggregate RuntimeAggregate, binding runtimecontract.CoverageEvidenceBinding, coverageID common.CoverageWorkItemID, refs []string) bool {
	if binding.VerificationPolicy == nil {
		return true
	}
	policy := binding.VerificationPolicy.Normalized()
	if policy.Validate() != nil {
		return false
	}
	evidence := acceptedEvidenceFactsForRefs(aggregate.Facts, coverageID, refs)
	owners := make(map[string]struct{})
	maxLevel := -1
	for _, fact := range evidence {
		if weakRuntimeEvidence(fact) || !runtimeEvidenceAppliesToCoverageWorkItemWithLineage(aggregate.Facts, fact, coverageID, aggregate.CoverageProofLineage) {
			continue
		}
		operation, found := runtimeOperationForEvidence(aggregate, fact)
		if !found || !runtimeOperationAllowedForFocus(aggregate.Facts.Focus, operation) {
			continue
		}
		descriptor, found := runtimecontract.RuntimeOperationDescriptorForKind(operation.Kind)
		if !found || !descriptor.RequiresOwnerReceipt || descriptor.OwnerModuleID == "" || fact.DispatchID == "" || fact.OperationID != operation.ID {
			continue
		}
		// Only an exact completed execution bearing the same evidence fact hash
		// and a persisted owner receipt counts as an independent source.
		for _, dispatch := range runtimeVerificationDispatches(aggregate) {
			if dispatch.ID != fact.DispatchID || dispatch.State == DispatchStateInvalidated {
				continue
			}
			for _, execution := range dispatch.OperationExecutions {
				if execution.OperationID != fact.OperationID || execution.EvidenceFactID != fact.ID ||
					execution.EvidenceFactHash != runtimecontract.StableRuntimeEvidenceFactHash(fact) || execution.OwnerReceiptHash == "" ||
					execution.State != runtimecontract.OperationExecutionEvidenceCommitted {
					continue
				}
				verifiedReceipt := false
				for _, receipt := range aggregate.OperationReceipts {
					if receipt.ExecutionKey == execution.ExecutionKey && receipt.Fence == execution.Fence &&
						receipt.OperationID == operation.ID && receipt.Outcome == runtimecontract.RuntimeOperationOutcomeAccepted &&
						reflect.DeepEqual(receipt.Evidence.OwnerRevisionSet, fact.OwnerRevisionSet) &&
						((receipt.Evidence.Validation == nil && fact.Validation == nil) ||
							(receipt.Evidence.Validation != nil && fact.Validation != nil &&
								receipt.Evidence.Validation.ObservationProtocolRef == fact.Validation.ObservationProtocolRef &&
								receipt.Evidence.Validation.RecipeID == fact.Validation.RecipeID &&
								receipt.Evidence.Validation.ProofClass == fact.Validation.ProofClass &&
								sameRuntimeValidationChecksForVerification(*receipt.Evidence.Validation, *fact.Validation))) &&
						runtimecontract.StableRuntimeOperationReceiptHash(receipt) == execution.OwnerReceiptHash {
						verifiedReceipt = true
						break
					}
				}
				if !verifiedReceipt {
					continue
				}
				if policy.RequireBoundContext && (strings.TrimSpace(fact.AuthoritySnapshotID) == "" || len(fact.TargetRefs) == 0) {
					continue
				}
				owners[descriptor.OwnerModuleID] = struct{}{}
				level := 1 // owner-backed inspection
				if fact.Kind == runtimecontract.EvidenceKindValidation && fact.Validation != nil &&
					fact.Validation.ValidateFailureDetails() == nil &&
					runtimeVerificationObservationBoundToSealedRecipe(dispatch, operation, binding, *fact.Validation) {
					switch fact.Validation.ProofClass {
					case runtimecontract.RuntimeValidationProofStatic:
						level = 2
					case runtimecontract.RuntimeValidationProofExecuted:
						level = 3
					case runtimecontract.RuntimeValidationProofBehavioral:
						level = 4
					}
				}
				if level > maxLevel {
					maxLevel = level
				}
			}
		}
	}
	if maxLevel < 0 && policy.MinimumEvidenceLevel == 0 && policy.MinIndependentSources == 0 && !policy.RequireBoundContext {
		return len(evidence) != 0
	}
	// Independent sources remain a separate scope-bound provenance gate;
	// additional owners cannot increase the evidence modality grade. Level 5
	// requires independently proven environment/external acceptance, for which
	// there is currently no trusted owner protocol.
	return maxLevel >= policy.MinimumEvidenceLevel && len(owners) >= policy.MinIndependentSources &&
		(policy.MinIndependentSources == 0 || maxLevel >= 1)
}

func runtimeVerificationObservationBoundToSealedRecipe(dispatch SealedDispatch, operation runtimecontract.RuntimeOperation, binding runtimecontract.CoverageEvidenceBinding, observation runtimecontract.RuntimeValidationObservation) bool {
	if observation.ProofClass == "" || observation.ValidateFailureDetails() != nil ||
		operation.Kind != runtimecontract.RuntimeBlockKindManagedValidationSession ||
		len(dispatch.ValidationRecipes) == 0 {
		return false
	}
	proof, found := runtimecontract.CoverageEvidenceProofForKind(binding, runtimecontract.EvidenceKindValidation)
	if !found || !runtimecontract.RuntimeValidationObservationSatisfiesCoverageProof(proof, observation) ||
		runtimecontract.ValidateRuntimeCandidateManagedValidationRecipeSnapshot(dispatch.Candidate, dispatch.ValidationRecipes) != nil {
		return false
	}
	for _, recipe := range dispatch.ValidationRecipes {
		if recipe.ID == observation.RecipeID && recipe.Validate() == nil &&
			runtimeVerificationOperationRecipeID(operation) == observation.RecipeID {
			return true
		}
	}
	return false
}

func runtimeVerificationOperationRecipeID(operation runtimecontract.RuntimeOperation) string {
	for _, parameter := range operation.Parameters {
		if parameter.Name == "recipeId" && parameter.String != nil {
			return strings.TrimSpace(*parameter.String)
		}
	}
	return ""
}

func sameRuntimeValidationChecksForVerification(left, right runtimecontract.RuntimeValidationObservation) bool {
	left, right = left.Normalized(), right.Normalized()
	return slices.Equal(left.ExecutedChecks, right.ExecutedChecks) &&
		slices.Equal(left.PassedChecks, right.PassedChecks) &&
		slices.Equal(left.FailedChecks, right.FailedChecks) &&
		slices.Equal(left.SkippedChecks, right.SkippedChecks) &&
		left.OutputTruncated == right.OutputTruncated
}

func runtimeVerificationDispatches(aggregate RuntimeAggregate) []SealedDispatch {
	ret := aggregate.CompletedDispatches
	if aggregate.ActiveDispatch != nil {
		ret = append(append([]SealedDispatch(nil), ret...), *aggregate.ActiveDispatch)
	}
	return ret
}

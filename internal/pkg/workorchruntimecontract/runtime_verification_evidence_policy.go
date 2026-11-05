package workorchruntimecontract

import (
	"reflect"
	"slices"
	"strings"
)

// RuntimeVerificationEvidencePolicySatisfied evaluates source independence and
// evidence modality for the caller's exact eligible proof set. The owning
// assessment compiler selects this set once; this rule does not select again.
func RuntimeVerificationEvidencePolicySatisfied(binding CoverageEvidenceBinding, evidence []EvidenceFact, completed []RuntimeDispatchRecord, receipts []RuntimeOperationReceipt) bool {
	if binding.VerificationPolicy == nil {
		return true
	}
	policy := binding.VerificationPolicy.Normalized()
	if policy.Validate() != nil {
		return false
	}
	owners := make(map[string]struct{})
	maxLevel := -1
	for _, fact := range evidence {
		operation, found := assessmentOperationForEvidence(completed, fact)
		if !found {
			continue
		}
		descriptor, found := RuntimeOperationDescriptorForKind(operation.Kind)
		if !found || !descriptor.RequiresOwnerReceipt || descriptor.OwnerModuleID == "" || fact.DispatchID == "" || fact.OperationID != operation.ID {
			continue
		}
		// Only an exact completed execution bearing the same evidence fact hash
		// and a persisted owner receipt counts as an independent source.
		for _, dispatch := range completed {
			if dispatch.ID != fact.DispatchID || dispatch.State == RuntimeDispatchStateInvalidated {
				continue
			}
			for _, execution := range dispatch.OperationExecutions {
				if execution.OperationID != fact.OperationID || execution.EvidenceFactID != fact.ID ||
					execution.EvidenceFactHash != StableRuntimeEvidenceFactHash(fact) || execution.OwnerReceiptHash == "" ||
					execution.State != OperationExecutionEvidenceCommitted {
					continue
				}
				verifiedReceipt := false
				for _, receipt := range receipts {
					if receipt.ExecutionKey == execution.ExecutionKey && receipt.Fence == execution.Fence &&
						receipt.OperationID == operation.ID && receipt.Outcome == RuntimeOperationOutcomeAccepted &&
						reflect.DeepEqual(receipt.Evidence.OwnerRevisionSet, fact.OwnerRevisionSet) &&
						reflect.DeepEqual(receipt.Evidence.ApplicabilityDependencies, fact.ApplicabilityDependencies) &&
						((receipt.Evidence.Validation == nil && fact.Validation == nil) ||
							(receipt.Evidence.Validation != nil && fact.Validation != nil &&
								receipt.Evidence.Validation.ObservationProtocolRef == fact.Validation.ObservationProtocolRef &&
								receipt.Evidence.Validation.RecipeID == fact.Validation.RecipeID &&
								receipt.Evidence.Validation.ProofClass == fact.Validation.ProofClass &&
								sameRuntimeValidationChecksForVerification(*receipt.Evidence.Validation, *fact.Validation))) &&
						StableRuntimeOperationReceiptHash(receipt) == execution.OwnerReceiptHash {
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
				if fact.Kind == EvidenceKindValidation && fact.Validation != nil &&
					fact.Validation.ValidateFailureDetails() == nil &&
					runtimeVerificationObservationBoundToSealedRecipe(dispatch, operation, binding, *fact.Validation) {
					switch fact.Validation.ProofClass {
					case RuntimeValidationProofStatic:
						level = 2
					case RuntimeValidationProofExecuted:
						level = 3
					case RuntimeValidationProofBehavioral:
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

func runtimeVerificationObservationBoundToSealedRecipe(dispatch RuntimeDispatchRecord, operation RuntimeOperation, binding CoverageEvidenceBinding, observation RuntimeValidationObservation) bool {
	if observation.ProofClass == "" || observation.ValidateFailureDetails() != nil ||
		operation.Kind != RuntimeBlockKindManagedValidationSession ||
		len(dispatch.ValidationRecipes) == 0 {
		return false
	}
	proof, found := CoverageEvidenceProofForKind(binding, EvidenceKindValidation)
	if !found || !RuntimeValidationObservationSatisfiesCoverageProof(proof, observation) ||
		ValidateRuntimeCandidateManagedValidationRecipeSnapshot(dispatch.Candidate, dispatch.ValidationRecipes) != nil {
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

func runtimeVerificationOperationRecipeID(operation RuntimeOperation) string {
	for _, parameter := range operation.Parameters {
		if parameter.Name == "recipeId" && parameter.String != nil {
			return strings.TrimSpace(*parameter.String)
		}
	}
	return ""
}

func sameRuntimeValidationChecksForVerification(left, right RuntimeValidationObservation) bool {
	left, right = left.Normalized(), right.Normalized()
	return slices.Equal(left.ExecutedChecks, right.ExecutedChecks) &&
		slices.Equal(left.PassedChecks, right.PassedChecks) &&
		slices.Equal(left.FailedChecks, right.FailedChecks) &&
		slices.Equal(left.SkippedChecks, right.SkippedChecks) &&
		left.OutputTruncated == right.OutputTruncated
}

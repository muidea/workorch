package workorchruntimecontract

// Input semantics is independent of the future execution result. These values
// identify the source of uncertainty, never an action, waiver or success bit.
const (
	RuntimeValidationInputSemantics              = "input_semantics"
	RuntimeValidationSemanticInformationMissing  = "semantic_information_missing"
	RuntimeValidationExecutionObservationPending = "execution_observation_pending"
	RuntimeValidationAssessmentPending           = "assessment_pending"
	RuntimeValidationTerminalDeliveryPending     = "terminal_delivery_pending"
)

func validateRuntimeInputReviewUncertainty(path, conclusion, kind string, subject RuntimeValidationInputReviewSubject) error {
	if subject.EvaluationScope == "" {
		if kind != "" {
			return runtimeInputViolation(path+".uncertaintyKind", "undeclared_uncertainty_scope", "omit uncertaintyKind for a subject without a declared evaluation scope", "uncertainty cannot extend a frozen owner subject")
		}
		return nil
	}
	if conclusion != RuntimeValidationInputInconclusive {
		if kind != "" {
			return runtimeInputViolation(path+".uncertaintyKind", "unexpected_uncertainty", "omit uncertaintyKind for a conclusive input review", "conclusive review cannot declare missing information")
		}
		return nil
	}
	if kind != RuntimeValidationSemanticInformationMissing {
		return runtimeInputViolation(path+".uncertaintyKind", "out_of_stage_or_missing_uncertainty", "semantic_information_missing for a concrete unresolved input-meaning question; execution timing/results, assessment and delivery belong to their later owners and are not prerequisites of input consistency", "input semantics cannot require a future owner result")
	}
	return nil
}

func writeRuntimeInputReviewUncertaintyHash(h interface{ Write([]byte) (int, error) }, kind string) {
	if kind != "" {
		writeHashString(h, "input-review-uncertainty")
		writeHashString(h, kind)
	}
}

package workorchruntimecontract

// Only the exact formal preimage may supply an unchanged generation. Cold
// inputs normalize independently; no historical compiler survives an event.
func normalizeRuntimeSemanticGenerationForValidation(raw RuntimeSemanticPlanningGeneration, session *runtimeSemanticValidationSession) RuntimeSemanticPlanningGeneration {
	if session.committedGeneration(raw) {
		return raw
	}
	return raw.Normalized()
}

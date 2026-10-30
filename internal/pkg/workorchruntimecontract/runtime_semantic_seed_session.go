package workorchruntimecontract

// No caller-supplied validation flag or hash hit: compare the complete input
// against a detached raw seed whose full gate succeeded in this exact call.
func validateRuntimeSemanticSeedInSession(seed RuntimeDefinitionSeed, session *runtimeSemanticValidationSession) error {
	for _, prior := range session.seeds {
		if runtimeRawEqual(prior, seed) {
			return nil
		}
	}
	if err := seed.Validate(); err != nil {
		return err
	}
	session.seeds = append(session.seeds, cloneRuntimeDefinitionSeedRaw(seed))
	return nil
}

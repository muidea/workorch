package workorchruntimecontract

import "fmt"

// ValidateRuntimeStateReloadSourcePair is the pure source gate for
// runtime-owner-state-joint-preimage/v1. It always validates the entire original
// DTO first, then the aggregate's ordered header and convergence-source gates.
// It returns no session, product or caller-supplied validation credential.
// Every invocation starts empty; the existing private consumers only reuse
// successful pure subresults after comparing detached, complete raw subtrees.
// Conversion differences therefore execute their full original gates again.
// The owner supplies its actual intervening and remaining invariant bodies;
// neither continuation receives any context, successful preimage or credential.
func ValidateRuntimeStateReloadSourcePair(original RuntimeStateRecord, converted RuntimeLineageAssertionSources, output RequiredOutputContract, revision uint64, feedbackVersion uint32, beforeSemantic, afterSemantic func() error) error {
	semantic := newRuntimeSemanticValidationSession()
	historical := &runtimeHistoricalPreimageSession{semantic: semantic}
	if err := original.validateInSourceSession(semantic, historical); err != nil {
		return fmt.Errorf("load Runtime state: %w", err)
	}
	if err := ValidateRuntimeDefinitionLineageAssertions(converted.Facts.DefinitionFocuses, converted.Facts.Activation.ExecutionContract.ProfileContracts); err != nil {
		return err
	}
	if err := ValidateRuntimeResultFeedbackVersion(feedbackVersion); err != nil {
		return err
	}
	return validateRuntimeConvergenceSourcesAndSemanticLedgerInSession(converted, output, revision, beforeSemantic, afterSemantic, semantic, historical)
}

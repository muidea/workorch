package workorchruntimecontract

import "fmt"

// ValidateRuntimeConvergenceSourceLedgers is one complete, ordered pure source
// gate. The private session cannot escape this call or be supplied by a caller.
// It retains the aggregate's historical error labels and fail-fast order;
// every independent call starts again from the complete raw sources.
func ValidateRuntimeConvergenceSourceLedgers(s RuntimeLineageAssertionSources, output RequiredOutputContract, revision uint64) error {
	semantic := newRuntimeSemanticValidationSession()
	historical := &runtimeHistoricalPreimageSession{semantic: semantic}
	return validateRuntimeConvergenceSourceLedgersInSession(s, output, revision, semantic, historical)
}

// ValidateRuntimeConvergenceSourcesAndSemanticLedger runs the aggregate's
// ordered source and semantic-ledger gates with one private pure context.
// The owner supplies the intervening and remaining invariant bodies, not a
// success flag. No context or validated value is passed to either continuation.
// These synchronous pure continuations are not owner I/O ports or EventHub data.
func ValidateRuntimeConvergenceSourcesAndSemanticLedger(s RuntimeLineageAssertionSources, output RequiredOutputContract, revision uint64, beforeSemantic, afterSemantic func() error) error {
	semantic := newRuntimeSemanticValidationSession()
	historical := &runtimeHistoricalPreimageSession{semantic: semantic}
	return validateRuntimeConvergenceSourcesAndSemanticLedgerInSession(s, output, revision, beforeSemantic, afterSemantic, semantic, historical)
}

func validateRuntimeConvergenceSourcesAndSemanticLedgerInSession(s RuntimeLineageAssertionSources, output RequiredOutputContract, revision uint64, beforeSemantic, afterSemantic func() error, semantic *runtimeSemanticValidationSession, historical *runtimeHistoricalPreimageSession) error {
	if err := validateRuntimeConvergenceSourceLedgersInSession(s, output, revision, semantic, historical); err != nil {
		return err
	}
	if beforeSemantic == nil || afterSemantic == nil {
		return fmt.Errorf("runtime aggregate requires complete invariant continuations")
	}
	if err := beforeSemantic(); err != nil {
		return err
	}
	if err := validateRuntimeSemanticPlanningLedgerInSession(s.Semantic, semantic); err != nil {
		return fmt.Errorf("runtime aggregate semantic planning ledger: %w", err)
	}
	return afterSemantic()
}

func validateRuntimeConvergenceSourceLedgersInSession(s RuntimeLineageAssertionSources, output RequiredOutputContract, revision uint64, semantic *runtimeSemanticValidationSession, historical *runtimeHistoricalPreimageSession) error {
	if err := validateRuntimeHistoricalBehaviorCompletionsInSession(s, historical); err != nil {
		return fmt.Errorf("runtime aggregate historical behavior: %w", err)
	}
	if err := validateRuntimeImplementationWorkStateInSession(s.Definition, s.Facts, s.Work, s.Semantic, s.Active, s.Completed, s.Receipts, s.Governance, s.Budget, semantic); err != nil {
		return fmt.Errorf("runtime aggregate implementation work: %w", err)
	}
	if output.ImplementationWorkBindingHash != "" {
		matches := 0
		for _, round := range s.Work.Rounds {
			if round.WorkingBinding != nil && round.WorkingBinding.Hash == output.ImplementationWorkBindingHash && round.CompletedReceiptSetHash == "" && round.FailedReceiptHash == "" && round.GovernanceReceiptHash == "" && len(s.Facts.Focus.CoverageWorkItemIDs) == 1 && s.Facts.Focus.CoverageWorkItemIDs[0] == round.CoverageWorkItemID && s.Facts.Failure == nil && s.Facts.RepairDirective == nil {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("runtime aggregate working output has no unique original live semantic work commitment")
		}
	}
	if err := validateRuntimeDispatchHistoryShapeInIndex(s.Facts.WorkspaceID, s.Facts.RunID, s.Active, s.Completed, runtimeOwnerLineageIndexFor(semantic)); err != nil {
		return fmt.Errorf("runtime aggregate dispatch history: %w", err)
	}
	if err := ValidateRuntimeEvidenceLedgerSequence(s.EvidenceSequence, s.Facts, s.Active, s.Completed, s.Resolutions); err != nil {
		return fmt.Errorf("runtime aggregate evidence ledger: %w", err)
	}
	if err := ValidateRuntimeEvidenceApplicabilityObservations(s.Facts, s.Receipts, revision, s.Reconciliation); err != nil {
		return fmt.Errorf("runtime aggregate evidence applicability: %w", err)
	}
	if err := ValidateRuntimeCWISuspensionSources(s.Facts, s.Completed, revision); err != nil {
		return err
	}
	if err := ValidateRuntimeApplicabilityCandidateInvalidations(s.Facts.EvidenceApplicabilityObservations, s.Attempts, s.Sessions); err != nil {
		return err
	}
	if err := ValidateRuntimeAssessmentAuthoritySources(s.Facts, s.Active, s.Completed); err != nil {
		return fmt.Errorf("runtime aggregate assessment authority: %w", err)
	}
	if err := ValidateRuntimeAssessmentApplicabilityGenerations(s.Reconciliation, s.Active, s.Completed); err != nil {
		return fmt.Errorf("runtime aggregate assessment applicability generation: %w", err)
	}
	if err := validateRuntimeAssessmentAdjudicationsInSession(s.Facts, s.Completed, s.ProofLineage, s, historical); err != nil {
		return fmt.Errorf("runtime aggregate assessment adjudication: %w", err)
	}
	return nil
}

package workorchruntimecontract

import "fmt"

// Frozen original aggregate source gate: consumers independently validate
// complete sources and preserve every interstitial gate and first-error order.
func referenceConvergenceSourceLedgers(s RuntimeLineageAssertionSources, output RequiredOutputContract, revision uint64) error {
	if err := ValidateRuntimeHistoricalBehaviorCompletions(s); err != nil {
		return fmt.Errorf("runtime aggregate historical behavior: %w", err)
	}
	if err := ValidateRuntimeImplementationWorkState(s.Definition, s.Facts, s.Work, s.Semantic, s.Active, s.Completed, s.Receipts, s.Governance, s.Budget); err != nil {
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
	if err := ValidateRuntimeDispatchHistoryShape(s.Facts.WorkspaceID, s.Facts.RunID, s.Active, s.Completed); err != nil {
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
	if err := ValidateRuntimeAssessmentAdjudications(s.Facts, s.Completed, s.ProofLineage, s); err != nil {
		return fmt.Errorf("runtime aggregate assessment adjudication: %w", err)
	}
	return nil
}

package runtime

import (
	"fmt"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Independent semantic-ledger context between the original invariant bodies.
// Production instead keeps one lexical source/semantic context; no test oracle
// can obtain that context or borrow its already-validated subtrees.
func validateRuntimeAggregateSM2Authority(a RuntimeAggregate, active *runtimecontract.RuntimeDispatchRecord, completed []runtimecontract.RuntimeDispatchRecord) error {
	if err := validateRuntimeAggregateSM2BeforeSemantic(a, active, completed); err != nil {
		return err
	}
	if err := a.SemanticPlanning.Validate(); err != nil {
		return fmt.Errorf("runtime aggregate semantic planning ledger: %w", err)
	}
	return validateRuntimeAggregateSM2AfterSemantic(a)
}

// Frozen original aggregate source gate: consumers independently validate
// complete sources and preserve every interstitial gate and first-error order.
func referenceAggregateConvergenceSources(s runtimecontract.RuntimeLineageAssertionSources, output runtimecontract.RequiredOutputContract, revision uint64) error {
	if err := runtimecontract.ValidateRuntimeHistoricalBehaviorCompletions(s); err != nil {
		return fmt.Errorf("runtime aggregate historical behavior: %w", err)
	}
	if err := runtimecontract.ValidateRuntimeImplementationWorkState(s.Definition, s.Facts, s.Work, s.Semantic, s.Active, s.Completed, s.Receipts, s.Governance, s.Budget); err != nil {
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
	if err := runtimecontract.ValidateRuntimeDispatchHistoryShape(s.Facts.WorkspaceID, s.Facts.RunID, s.Active, s.Completed); err != nil {
		return fmt.Errorf("runtime aggregate dispatch history: %w", err)
	}
	if err := runtimecontract.ValidateRuntimeEvidenceLedgerSequence(s.EvidenceSequence, s.Facts, s.Active, s.Completed, s.Resolutions); err != nil {
		return fmt.Errorf("runtime aggregate evidence ledger: %w", err)
	}
	if err := runtimecontract.ValidateRuntimeEvidenceApplicabilityObservations(s.Facts, s.Receipts, revision, s.Reconciliation); err != nil {
		return fmt.Errorf("runtime aggregate evidence applicability: %w", err)
	}
	if err := runtimecontract.ValidateRuntimeCWISuspensionSources(s.Facts, s.Completed, revision); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeApplicabilityCandidateInvalidations(s.Facts.EvidenceApplicabilityObservations, s.Attempts, s.Sessions); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeAssessmentAuthoritySources(s.Facts, s.Active, s.Completed); err != nil {
		return fmt.Errorf("runtime aggregate assessment authority: %w", err)
	}
	if err := runtimecontract.ValidateRuntimeAssessmentApplicabilityGenerations(s.Reconciliation, s.Active, s.Completed); err != nil {
		return fmt.Errorf("runtime aggregate assessment applicability generation: %w", err)
	}
	if err := runtimecontract.ValidateRuntimeAssessmentAdjudications(s.Facts, s.Completed, s.ProofLineage, s); err != nil {
		return fmt.Errorf("runtime aggregate assessment adjudication: %w", err)
	}
	return nil
}
func referenceAggregateSourceSessionValidate(a RuntimeAggregate) error {
	if err := runtimecontract.ValidateRuntimeDefinitionLineageAssertions(a.Facts.DefinitionFocuses, a.Facts.Activation.ExecutionContract.ProfileContracts); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeResultFeedbackVersion(a.ResultFeedbackVersion); err != nil {
		return err
	}
	// 同次完整门禁只生成一次 detached dispatch；各阶段仍执行原完整校验。
	var active *runtimecontract.RuntimeDispatchRecord
	if a.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*a.ActiveDispatch)
		active = &value
	}
	completed := sealedDispatchesToRuntimeRecords(a.CompletedDispatches)
	if err := referenceAggregateConvergenceSources(runtimeLineageAssertionSourcesWithDispatches(a, active, completed), a.Decision.RequiredOutput, a.StateRevision); err != nil {
		return err
	}
	if err := validateRuntimeAggregateSM2Authority(a, active, completed); err != nil {
		return err
	}
	if err := validateRuntimeAggregateSM3ActiveDispatch(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateActivationFence(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateSM3CandidateTransaction(a, active, completed); err != nil {
		return err
	}
	if err := validateRuntimeAggregateExecutionBudget(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateRecoverableWaits(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateOperationRecoveryAuthorizations(a, active, completed); err != nil {
		return err
	}
	if err := validateRuntimeAggregateGovernanceLedgers(a, active, completed); err != nil {
		return err
	}
	if err := validateRuntimeAggregateSM4DeliveryAcknowledgements(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateSM2RepairLedgers(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateSM4DriveLease(a); err != nil {
		return err
	}
	return validateRuntimeAggregateCrossMachineFences(a, active, completed)
}

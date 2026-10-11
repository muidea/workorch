package runtime

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/governancecheck"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// 保留切片前完整 aggregate 调用次序及每阶段独立 dispatch 投影，不随生产复用路径变化。
func referenceAggregateDispatchReuseValidate(a RuntimeAggregate) error {
	if err := runtimecontract.ValidateRuntimeDefinitionLineageAssertions(a.Facts.DefinitionFocuses, a.Facts.Activation.ExecutionContract.ProfileContracts); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeResultFeedbackVersion(a.ResultFeedbackVersion); err != nil {
		return err
	}
	var active *runtimecontract.RuntimeDispatchRecord
	if a.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*a.ActiveDispatch)
		active = &value
	}
	completed := sealedDispatchesToRuntimeRecords(a.CompletedDispatches)
	if err := runtimecontract.ValidateRuntimeImplementationWorkState(a.Definition, a.Facts, a.ImplementationWork, a.SemanticPlanning, active, completed, a.OperationReceipts, a.OperationGovernanceBindings, a.ExecutionBudget); err != nil {
		return fmt.Errorf("runtime aggregate implementation work: %w", err)
	}
	if a.Decision.RequiredOutput.ImplementationWorkBindingHash != "" {
		matches := 0
		for _, round := range a.ImplementationWork.Rounds {
			if round.WorkingBinding != nil && round.WorkingBinding.Hash == a.Decision.RequiredOutput.ImplementationWorkBindingHash && round.CompletedReceiptSetHash == "" && round.FailedReceiptHash == "" && round.GovernanceReceiptHash == "" && len(a.Facts.Focus.CoverageWorkItemIDs) == 1 && a.Facts.Focus.CoverageWorkItemIDs[0] == round.CoverageWorkItemID && a.Facts.Failure == nil && a.Facts.RepairDirective == nil {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("runtime aggregate working output has no unique original live semantic work commitment")
		}
	}
	if err := runtimecontract.ValidateRuntimeDispatchHistoryShape(a.Facts.WorkspaceID, a.Facts.RunID, active, completed); err != nil {
		return fmt.Errorf("runtime aggregate dispatch history: %w", err)
	}
	if err := runtimecontract.ValidateRuntimeEvidenceLedgerSequence(a.EvidenceSequence, a.Facts, active, completed, a.GovernanceResolutions); err != nil {
		return fmt.Errorf("runtime aggregate evidence ledger: %w", err)
	}
	if err := runtimecontract.ValidateRuntimeAssessmentAuthoritySources(a.Facts, active, completed); err != nil {
		return fmt.Errorf("runtime aggregate assessment authority: %w", err)
	}
	if err := runtimecontract.ValidateRuntimeAssessmentAdjudications(a.Facts, completed, a.CoverageProofLineage, runtimeLineageAssertionSources(a)); err != nil {
		return fmt.Errorf("runtime aggregate assessment adjudication: %w", err)
	}
	if err := referenceDispatchReuseValidateRuntimeAggregateSM2Authority(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateSM3ActiveDispatch(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateActivationFence(a); err != nil {
		return err
	}
	if err := referenceDispatchReuseValidateRuntimeAggregateSM3CandidateTransaction(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateExecutionBudget(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateRecoverableWaits(a); err != nil {
		return err
	}
	if err := referenceDispatchReuseValidateRuntimeAggregateOperationRecoveryAuthorizations(a); err != nil {
		return err
	}
	if err := referenceDispatchReuseValidateRuntimeAggregateGovernanceLedgers(a); err != nil {
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
	return referenceDispatchReuseValidateRuntimeAggregateCrossMachineFences(a)
}

func referenceDispatchReuseValidateRuntimeAggregateOperationRecoveryAuthorizations(a RuntimeAggregate) error {
	var active *runtimecontract.RuntimeDispatchRecord
	if a.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*a.ActiveDispatch)
		active = &value
	}
	if err := runtimecontract.ValidateRuntimeOperationRecoveryAuthorizations(
		a.Facts.WorkspaceID,
		a.Facts.RunID,
		active,
		sealedDispatchesToRuntimeRecords(a.CompletedDispatches),
		a.OperationReceipts,
		a.OperationRecoveryAuthorizations,
		a.Facts.Recovery,
		a.StateRevision,
	); err != nil {
		return fmt.Errorf("runtime aggregate operation recovery authorization: %w", err)
	}
	return nil
}

func referenceDispatchReuseValidateRuntimeAggregateSM2Authority(a RuntimeAggregate) error {
	if a.Facts.WorkspaceID == "" || a.Facts.RunID == "" {
		return fmt.Errorf("runtime aggregate requires workspace and run scope")
	}
	if err := runtimecontract.ValidateRuntimeStateCWILedger(a.Facts, a.Reconciliation); err != nil {
		return err
	}
	if err := validateRuntimeCWISemanticRebindingFocusProjections(a.Facts); err != nil {
		return err
	}
	var active *runtimecontract.RuntimeDispatchRecord
	if a.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*a.ActiveDispatch)
		active = &value
	}
	completed := sealedDispatchesToRuntimeRecords(a.CompletedDispatches)
	if err := runtimecontract.ValidateRuntimeCWIBindingRetirementsAgainstExecution(a.Facts, active, completed, a.OperationReceipts); err != nil {
		return err
	}
	if err := validateRuntimeProspectiveCoverageCollectiveCredit(a); err != nil {
		return err
	}
	if err := a.Facts.Validate(); err != nil {
		return err
	}
	if err := validateRuntimeAggregateValidationProofApplicability(a); err != nil {
		return err
	}
	if err := a.SemanticPlanning.Validate(); err != nil {
		return fmt.Errorf("runtime aggregate semantic planning ledger: %w", err)
	}
	if err := runtimecontract.ValidateRuntimeSemanticProofBatchLimits(a.SemanticPlanning, a.Facts.Activation.ExecutionContract.ExecutionLimits); err != nil {
		return err
	}
	if err := validateRuntimeAggregateSemanticValidationProofRequirements(a); err != nil {
		return err
	}
	for idx, history := range a.SemanticPlanning.History {
		if history.Snapshot.Manifest == nil {
			// The typed ledger validator already rejects this shape; retain an
			// aggregate-level guard so a future history representation cannot
			// cross workspace/run ownership silently.
			return fmt.Errorf("runtime aggregate semantic generation %d has no manifest", idx)
		}
		if history.Snapshot.Manifest.WorkspaceID != a.Facts.WorkspaceID || history.Snapshot.Manifest.RunID != a.Facts.RunID {
			return fmt.Errorf("runtime aggregate semantic generation %d scope mismatch", idx)
		}
	}
	if err := validateRuntimeSemanticBindingPreimages(a); err != nil {
		return err
	}
	if err := validateRuntimeSemanticBindingPurposeInvariant(a); err != nil {
		return err
	}
	if a.SemanticPlanning.DefinitionSeed.Hash != a.Facts.DefinitionSeed.Hash {
		return fmt.Errorf("runtime aggregate semantic planning seed does not match facts")
	}
	if err := a.Reconciliation.Validate(); err != nil {
		return fmt.Errorf("runtime aggregate reconciliation ledger: %w", err)
	}
	if err := runtimecontract.ValidateRuntimeCoverageProofLineage(a.CoverageProofLineage); err != nil {
		return fmt.Errorf("runtime aggregate coverage proof lineage: %w", err)
	}
	if err := a.Definition.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(a.Definition.Revision) != strings.TrimSpace(a.Facts.DefinitionRevision) {
		return fmt.Errorf("runtime aggregate definition revision mismatch")
	}
	if !runtimecontract.SameRuntimeSourceObligationInventory(a.Definition.SourceObligations, a.Facts.DefinitionSeed.SourceObligations) {
		return fmt.Errorf("runtime aggregate Definition and seed source obligations mismatch")
	}
	if a.Facts.DefinitionSeed.SourceObligations != nil && a.Facts.DefinitionSeed.SourceObligations.StartupInputHash != a.Definition.StartupInputHash {
		return fmt.Errorf("runtime aggregate source obligations startup input mismatch")
	}
	if err := a.Decision.Validate(); err != nil {
		return err
	}
	if terminal := a.Decision.Terminal; terminal != nil && (terminal.OriginalDiagnostic != nil ||
		(terminal.Code == "owner_fact_convergence_exhausted" && missingOwnerFactIsUnavailable(a.Facts))) {
		if terminal.Code != "owner_fact_convergence_exhausted" || !reflect.DeepEqual(terminal.OriginalDiagnostic, missingOwnerFactDiagnostic(a.Facts)) {
			return fmt.Errorf("runtime terminal owner cause does not match exact-current missing owner fact")
		}
	}
	if a.Decision.AuthoritySnapshot.WorkspaceID != a.Facts.WorkspaceID || a.Decision.AuthoritySnapshot.RunID != a.Facts.RunID {
		return fmt.Errorf("runtime aggregate decision scope mismatch")
	}
	return nil
}

func referenceDispatchReuseValidateRuntimeAggregateSM3CandidateTransaction(a RuntimeAggregate) error {
	if err := runtimecontract.ValidateRuntimeCandidateAttemptRecordsForScope(a.Facts.WorkspaceID, a.Facts.RunID, a.CandidateAttempts); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeCandidateRequestSessions(a.CandidateRequestSessions); err != nil {
		return err
	}
	if err := validateRuntimeDefinitionClarificationAggregate(a); err != nil {
		return err
	}
	if err := validateRuntimeCandidateAttemptSessionBindings(a.CandidateAttempts, a.CandidateRequestSessions); err != nil {
		return err
	}
	if err := validateRuntimeCandidateFeasibilityBindings(a); err != nil {
		return err
	}
	if (isTerminalConvergenceState(a.Decision.State) || a.Decision.Terminal != nil) && hasPendingCandidateAttempt(a.CandidateAttempts) {
		return fmt.Errorf("terminal runtime aggregate cannot retain a pending candidate attempt")
	}
	if hasUnreducedCurrentCandidateRetryFailure(a.Decision, a.CandidateAttempts) {
		return fmt.Errorf("current runtime authority retains a failed candidate attempt 1 without reducer transition")
	}
	if err := referenceDispatchReuseValidateAcceptedCandidateDispatchBindings(a); err != nil {
		return err
	}
	var active *runtimecontract.RuntimeDispatchRecord
	if a.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*a.ActiveDispatch)
		active = &value
	}
	completed := sealedDispatchesToRuntimeRecords(a.CompletedDispatches)
	if err := runtimecontract.ValidateRuntimeDispatchAdmissionLineage(
		active,
		completed,
		a.CandidateAttempts,
		a.OperationGovernanceBindings,
	); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeAcceptedCandidateManifestLineage(active, completed, a.CandidateAttempts, a.CandidateRequestSessions); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeDurableDispatchLimits(a.Facts.Activation.ExecutionContract.ExecutionLimits, active, completed, a.CandidateAttempts); err != nil {
		return err
	}
	return runtimecontract.ValidateRuntimeAcceptedAssessmentPurposePreimagesWithValidatedSemanticLedger(a.SemanticPlanning, active, completed, a.CandidateAttempts, a.CandidateRequestSessions)
}

func referenceDispatchReuseValidateAcceptedCandidateDispatchBindings(a RuntimeAggregate) error {
	for _, attempt := range a.CandidateAttempts {
		if attempt.State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
			continue
		}
		matched := a.ActiveDispatch != nil && runtimecontract.RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, sealedDispatchToRuntimeRecord(*a.ActiveDispatch))
		if !matched {
			for _, dispatch := range a.CompletedDispatches {
				if runtimecontract.RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, sealedDispatchToRuntimeRecord(dispatch)) {
					matched = true
					break
				}
			}
		}
		if !matched {
			return fmt.Errorf("accepted candidate attempt has no exact sealed dispatch target-authorization projection")
		}
	}
	return nil
}

func referenceDispatchReuseValidateRuntimeAggregateGovernanceLedgers(a RuntimeAggregate) error {
	if err := validateRuntimeInputReviewAggregate(a); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeGovernanceResolutionRecords(a.GovernanceResolutions); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeGovernanceRequestRecords(a.GovernanceRequests); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeOperationGovernanceBindings(a.OperationGovernanceBindings); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeOperationReceipts(a.OperationReceipts); err != nil {
		return err
	}
	for _, receipt := range a.OperationReceipts {
		if receipt.Outcome != runtimecontract.RuntimeOperationOutcomeRejected || receipt.Feedback == nil || receipt.Feedback.OriginalDiagnostic == nil {
			continue
		}
		dispatch, err := operationReceiptFeedbackDispatch(a, receipt)
		if err != nil {
			return fmt.Errorf("runtime aggregate operation receipt diagnostic execution projection: %w", err)
		}
		if err := validateOperationReceiptFeedbackAgainstDispatch(a, dispatch, a.Facts.WorkspaceID, a.Facts.RunID, receipt); err != nil {
			return fmt.Errorf("runtime aggregate operation receipt diagnostic: %w", err)
		}
		execution, found := operationExecutionRecord(dispatch.OperationExecutions, receipt.OperationID)
		if !found {
			return fmt.Errorf("runtime aggregate rejected operation receipt has no execution projection")
		}
		if execution.State == runtimecontract.OperationExecutionEvidenceCommitted {
			operation, found := candidateOperationByID(dispatch.Candidate, receipt.OperationID)
			if !found {
				return fmt.Errorf("runtime aggregate rejected operation receipt has no sealed operation")
			}
			if err := validateRuntimeReceiptEvidenceRejectedLineage(a, dispatch, operation, execution, receipt, runtimecontract.StableRuntimeOperationReceiptHash(receipt)); err != nil {
				return fmt.Errorf("runtime aggregate rejected operation receipt evidence projection: %w", err)
			}
		}
	}
	var active *runtimecontract.RuntimeDispatchRecord
	if a.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*a.ActiveDispatch)
		active = &value
	}
	if err := runtimecontract.ValidateRuntimeOperationReceiptExecutionLineage(active, sealedDispatchesToRuntimeRecords(a.CompletedDispatches), a.OperationReceipts); err != nil {
		return err
	}
	return validateRuntimeAggregateEvidenceCommitLedger(a)
}

func referenceDispatchReuseValidateRuntimeAggregateCrossMachineFences(a RuntimeAggregate) error {
	if err := referenceDispatchReuseValidateRuntimeGovernanceRequestDispatchBindings(a); err != nil {
		return err
	}
	if err := referenceDispatchReuseValidateRuntimeOperationGovernanceBindingFences(a); err != nil {
		return err
	}
	return validateRuntimeGovernanceResolutionBindings(a)
}

func referenceDispatchReuseValidateRuntimeGovernanceRequestDispatchBindings(a RuntimeAggregate) error {
	var active *runtimecontract.RuntimeDispatchRecord
	if a.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*a.ActiveDispatch)
		active = &value
	}
	return runtimecontract.ValidateRuntimeGovernanceRequestDispatchLineage(a.GovernanceRequests, active, sealedDispatchesToRuntimeRecords(a.CompletedDispatches))
}

func referenceDispatchReuseValidateRuntimeOperationGovernanceBindingFences(a RuntimeAggregate) error {
	var active *runtimecontract.RuntimeDispatchRecord
	if a.ActiveDispatch != nil {
		value := sealedDispatchToRuntimeRecord(*a.ActiveDispatch)
		active = &value
	}
	if err := runtimecontract.ValidateRuntimeDispatchExecutionKeyLineage(
		a.Facts.WorkspaceID,
		a.Facts.RunID,
		active,
		sealedDispatchesToRuntimeRecords(a.CompletedDispatches),
		a.OperationGovernanceBindings,
	); err != nil {
		return err
	}
	for _, rawBinding := range a.OperationGovernanceBindings {
		binding := rawBinding.Normalized()
		var source *SealedDispatch
		for idx := range a.CompletedDispatches {
			if a.CompletedDispatches[idx].ID == binding.OperationDispatchID {
				value := a.CompletedDispatches[idx]
				source = &value
				break
			}
		}
		if source == nil || source.State != DispatchStateCommitted {
			return fmt.Errorf("operation governance binding source dispatch %q is not completed", binding.OperationDispatchID)
		}
		execution, found := operationExecutionRecord(source.OperationExecutions, binding.OperationID)
		if !found || !governancecheck.BindingMatchesExecution(binding, execution) {
			return fmt.Errorf("operation governance binding source execution does not match sealed receipt")
		}
		receipt, found := runtimecontract.RuntimeOperationReceiptForExecutionKeyFence(a.OperationReceipts, binding.ExecutionKey, binding.Fence)
		if !found || !governancecheck.BindingMatchesReceipt(binding, receipt) {
			return fmt.Errorf("operation governance binding has no matching governance receipt")
		}
		if binding.State == runtimecontract.RuntimeOperationGovernanceBindingSuspended {
			continue
		}
		bound := false
		for _, request := range a.GovernanceRequests {
			if governancecheck.BindingMatchesRequest(binding, request) {
				bound = true
				break
			}
		}
		if !bound {
			return fmt.Errorf("operation governance binding has no matching ticket request")
		}
	}
	governedExecutions := make([]runtimecontract.RuntimeOperationExecutionRecord, 0)
	if a.ActiveDispatch != nil {
		governedExecutions = append(governedExecutions, a.ActiveDispatch.OperationExecutions...)
	}
	for _, dispatch := range a.CompletedDispatches {
		governedExecutions = append(governedExecutions, dispatch.OperationExecutions...)
	}
	if err := runtimecontract.ValidateRuntimeOperationGovernanceExecutionBindings(a.OperationGovernanceBindings, governedExecutions); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeOperationGovernanceGrantLineage(a.OperationGovernanceBindings, a.GovernanceResolutions, a.Facts.Evidence.Accepted, a.Facts.Evidence.Rejected); err != nil {
		return err
	}
	return nil
}

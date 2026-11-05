package runtime

import (
	"fmt"
	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/governancecheck"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"reflect"
	"strings"
)

// Frozen pre-closure independent aggregate checks. Test oracle only.
func (a RuntimeAggregate) validateAfterSemanticSources(active *runtimecontract.RuntimeDispatchRecord, completed []runtimecontract.RuntimeDispatchRecord) error {
	return a.validateAfterSemanticSourcesInGate(active, completed, nil)
}

func (a RuntimeAggregate) validateAfterSemanticSourcesInGate(active *runtimecontract.RuntimeDispatchRecord, completed []runtimecontract.RuntimeDispatchRecord, gate *runtimecontract.RuntimeOwnerInvariantSourceGate) error {
	if err := validateRuntimeAggregateSM2AfterSemanticInGate(a, gate); err != nil {
		return err
	}
	if err := validateRuntimeAggregateSM3ActiveDispatch(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateActivationFence(a); err != nil {
		return err
	}
	if err := validateRuntimeAggregateSM3CandidateTransactionInGate(a, active, completed, gate); err != nil {
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

func validateRuntimeAggregateOperationRecoveryAuthorizations(a RuntimeAggregate, active *runtimecontract.RuntimeDispatchRecord, completed []runtimecontract.RuntimeDispatchRecord) error {
	if err := runtimecontract.ValidateRuntimeOperationRecoveryAuthorizations(
		a.Facts.WorkspaceID,
		a.Facts.RunID,
		active,
		completed,
		a.OperationReceipts,
		a.OperationRecoveryAuthorizations,
		a.Facts.Recovery,
		a.StateRevision,
	); err != nil {
		return fmt.Errorf("runtime aggregate operation recovery authorization: %w", err)
	}
	return nil
}

func validateRuntimeAggregateExecutionBudget(a RuntimeAggregate) error {
	if err := a.ExecutionBudget.Validate(); err != nil {
		return fmt.Errorf("runtime aggregate execution budget: %w", err)
	}
	if !runtimecontract.SameRuntimeExecutionBudgetPolicy(a.ExecutionBudget.Policy, a.Facts.ExecutionBudgetPolicy) {
		return fmt.Errorf("runtime aggregate execution budget policy does not match activation facts")
	}
	return nil
}

func validateRuntimeAggregateSM2BeforeSemantic(a RuntimeAggregate, active *runtimecontract.RuntimeDispatchRecord, completed []runtimecontract.RuntimeDispatchRecord) error {
	if a.Facts.WorkspaceID == "" || a.Facts.RunID == "" {
		return fmt.Errorf("runtime aggregate requires workspace and run scope")
	}
	if err := runtimecontract.ValidateRuntimeStateCWILedger(a.Facts, a.Reconciliation); err != nil {
		return err
	}
	if err := validateRuntimeCWISemanticRebindingFocusProjections(a.Facts); err != nil {
		return err
	}
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
	return nil
}

func validateRuntimeAggregateSM2AfterSemantic(a RuntimeAggregate) error {
	return validateRuntimeAggregateSM2AfterSemanticInGate(a, nil)
}

func validateRuntimeAggregateSM2AfterSemanticInGate(a RuntimeAggregate, gate *runtimecontract.RuntimeOwnerInvariantSourceGate) error {
	if err := runtimecontract.ValidateRuntimeSemanticProofBatchLimits(a.SemanticPlanning, a.Facts.Activation.ExecutionContract.ExecutionLimits); err != nil {
		return err
	}
	if err := gate.ValidateDefinitionSources(a.SemanticPlanning, a.Facts.DefinitionSeed, a.Facts.DefinitionFocuses); err != nil {
		return err
	}
	for idx, history := range a.SemanticPlanning.AllHistory() {
		if v := history.OwnerInputRevision; v != nil {
			if v.WorkspaceID != a.Facts.WorkspaceID || v.RunID != a.Facts.RunID {
				return fmt.Errorf("runtime input observation scope mismatch")
			}
			continue
		}
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
	if err := validateRuntimeSemanticBindingPreimagesInGate(a, gate); err != nil {
		return err
	}
	if err := validateRuntimeSemanticBindingPurposeInvariantInGate(a, gate); err != nil {
		return err
	}
	if a.SemanticPlanning.DefinitionSeed.Hash != a.Facts.DefinitionSeed.Hash {
		return fmt.Errorf("runtime aggregate semantic planning seed does not match facts")
	}
	if err := a.Reconciliation.Validate(); err != nil {
		return fmt.Errorf("runtime aggregate reconciliation ledger: %w", err)
	}
	if err := validateReconciliationExecutionSources(a); err != nil {
		return err
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
	if err := validateRuntimeApplicabilityPrerequisiteProjection(a); err != nil {
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

func validateRuntimeAggregateSemanticValidationProofRequirements(a RuntimeAggregate) error {
	return runtimecontract.ValidateRuntimeSemanticLedgerDefinitionSources(a.SemanticPlanning, a.Facts.DefinitionSeed, a.Facts.DefinitionFocuses)
}

func validateRuntimeSemanticBindingPurposeInvariant(a RuntimeAggregate) error {
	return validateRuntimeSemanticBindingPurposeInvariantInGate(a, nil)
}

func validateRuntimeAggregateSM3ActiveDispatch(a RuntimeAggregate) error {
	if a.ActiveDispatch == nil {
		return nil
	}
	if err := a.ActiveDispatch.ValidateWithActivation(a.Decision, runtimecontract.RuntimeActivationForFacts(a.Facts)); err != nil {
		return fmt.Errorf("runtime aggregate active dispatch: %w", err)
	}
	return nil
}

func validateRuntimeAggregateActivationFence(a RuntimeAggregate) error {
	if a.Termination == nil {
		return nil
	}
	if err := a.Termination.Validate(); err != nil {
		return fmt.Errorf("runtime aggregate termination: %w", err)
	}
	if a.Termination.Revision > a.StateRevision {
		return fmt.Errorf("runtime aggregate termination revision exceeds aggregate revision")
	}
	return nil
}

func validateRuntimeAggregateSM3CandidateTransaction(a RuntimeAggregate, active *runtimecontract.RuntimeDispatchRecord, completed []runtimecontract.RuntimeDispatchRecord) error {
	return validateRuntimeAggregateSM3CandidateTransactionInGate(a, active, completed, nil)
}

func validateRuntimeAggregateSM3CandidateTransactionInGate(a RuntimeAggregate, active *runtimecontract.RuntimeDispatchRecord, completed []runtimecontract.RuntimeDispatchRecord, gate *runtimecontract.RuntimeOwnerInvariantSourceGate) error {
	if err := gate.ValidateCandidateAttempts(a.Facts.WorkspaceID, a.Facts.RunID, a.CandidateAttempts); err != nil {
		return err
	}
	if err := gate.ValidateCandidateRequests(a.CandidateRequestSessions); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeInputRevisionObservationSources(a.SemanticPlanning, a.Facts, a.PrerequisiteInvalidations); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeInputRevisionRequestSources(a.SemanticPlanning, a.CandidateRequestSessions); err != nil {
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
	if err := validateAcceptedCandidateDispatchBindings(a.CandidateAttempts, active, completed); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeDispatchAdmissionLineage(
		active,
		completed,
		a.CandidateAttempts,
		a.OperationGovernanceBindings,
	); err != nil {
		return err
	}
	if err := gate.ValidateCandidateManifestSources(active, completed, a.CandidateAttempts, a.CandidateRequestSessions); err != nil {
		return err
	}
	if err := runtimecontract.ValidateRuntimeDurableDispatchLimits(a.Facts.Activation.ExecutionContract.ExecutionLimits, active, completed, a.CandidateAttempts); err != nil {
		return err
	}
	if gate != nil {
		return gate.ValidateAssessmentSources(a.SemanticPlanning, active, completed, a.CandidateAttempts, a.CandidateRequestSessions)
	}
	return runtimecontract.ValidateRuntimeAcceptedAssessmentPurposePreimagesWithValidatedSemanticLedger(a.SemanticPlanning, active, completed, a.CandidateAttempts, a.CandidateRequestSessions)
}

func validateRuntimeDefinitionClarificationAggregate(a RuntimeAggregate) error {
	if a.DefinitionClarification == nil {
		return nil
	}
	if err := a.DefinitionClarification.Validate(); err != nil {
		return fmt.Errorf("runtime Definition clarification: %w", err)
	}
	clarification := a.DefinitionClarification.Clone()
	definition := a.Definition.Normalized()
	if clarification.Observation.WorkspaceID != a.Facts.WorkspaceID || clarification.Observation.RunID != a.Facts.RunID ||
		clarification.Observation.DefinitionRevision != definition.Revision || clarification.Observation.StartupInputHash != definition.StartupInputHash ||
		a.Decision.State != runtimecontract.ConvergenceStateNeedGovernance ||
		!sameRuntimeStrings(a.Decision.RequiredOutput.Normalized().AcceptedKinds, []string{runtimecontract.RuntimeDefinitionClarificationRequiredOutputKind}) ||
		a.ActiveDispatch != nil {
		return fmt.Errorf("runtime Definition clarification aggregate binding mismatch")
	}
	if clarification.Dispatch != nil && clarification.Dispatch.AuthoritySnapshotID != a.Decision.AuthoritySnapshot.ID {
		return fmt.Errorf("runtime Definition clarification dispatch authority mismatch")
	}
	for _, session := range a.CandidateRequestSessions {
		if session.LogicalRequestID == clarification.Observation.LogicalRequestID && session.State == runtimecontract.RuntimeCandidateRequestSessionDocumentComplete &&
			session.Transaction.AuthoritySnapshotID == clarification.Observation.AuthoritySnapshotID {
			return nil
		}
	}
	return fmt.Errorf("runtime Definition clarification has no complete semantic request session")
}

func validateAcceptedCandidateDispatchBindings(attempts []runtimecontract.RuntimeCandidateAttemptRecord, active *runtimecontract.RuntimeDispatchRecord, completed []runtimecontract.RuntimeDispatchRecord) error {
	for _, attempt := range attempts {
		if attempt.State != runtimecontract.RuntimeCandidateAttemptStateAccepted {
			continue
		}
		matched := active != nil && runtimecontract.RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, *active)
		if !matched {
			for _, dispatch := range completed {
				if runtimecontract.RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, dispatch) {
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

func validateRuntimeAggregateGovernanceLedgers(a RuntimeAggregate, active *runtimecontract.RuntimeDispatchRecord, completed []runtimecontract.RuntimeDispatchRecord) error {
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
	if err := runtimecontract.ValidateRuntimeOperationReceiptExecutionLineage(active, completed, a.OperationReceipts); err != nil {
		return err
	}
	return validateRuntimeAggregateReceiptJoins(a)
}

func validateRuntimeAggregateSM4DeliveryAcknowledgements(a RuntimeAggregate) error {
	if err := runtimecontract.ValidateRuntimeTerminalMaterialSeal(a.TerminalMaterialSeal, terminalMaterialSource(a)); err != nil {
		return err
	}
	for _, acknowledgement := range a.DeliveryAcknowledgements {
		if err := acknowledgement.Validate(); err != nil {
			return err
		}
		if acknowledgement.WorkspaceID != a.Facts.WorkspaceID || acknowledgement.RunID != a.Facts.RunID {
			return fmt.Errorf("delivery acknowledgement scope mismatch")
		}
	}
	return nil
}

func validateRuntimeAggregateSM2RepairLedgers(a RuntimeAggregate) error {
	if err := runtimecontract.ValidateRuntimeRepairSurfaceConfirmationRecords(a.RepairSurfaceConfirmations); err != nil {
		return fmt.Errorf("runtime aggregate repair surface confirmations: %w", err)
	}
	return validateRuntimeAggregateRepairJoins(a)
}

func validateRuntimeAggregateSM4DriveLease(a RuntimeAggregate) error {
	recovery := a.Facts.Recovery.Normalized()
	if err := runtimecontract.ValidateRuntimeNonTerminalRecoveryLinearization(recovery, a.StateRevision, a.DriveLease); err != nil {
		return fmt.Errorf("runtime aggregate recovery: %w", err)
	}
	if recovery.TerminalDeliveryIntervention.Validate() == nil && a.DriveLease != nil {
		return fmt.Errorf("runtime aggregate terminal delivery intervention cannot retain automatic drive authority")
	}
	retired := recovery.RetiredTerminalDeliveryLease
	retiredPresent := runtimecontract.RuntimeDriveLeasePresent(retired)
	if retiredPresent {
		requestBoundNonterminal := runtimecontract.RuntimeDriveLeaseBindsNonTerminalRecovery(retired) &&
			runtimecontract.ValidateRuntimeNonTerminalRecoveryLeaseBinding(recovery, retired) == nil
		if a.Termination == nil || retired.Validate() != nil || retired.State != runtimecontract.RuntimeDriveLeaseRunning ||
			retired.WorkspaceID != a.Facts.WorkspaceID || retired.RunID != a.Facts.RunID || retired.AggregateRevision >= a.Termination.Revision ||
			(retired.RecoveryRequestID != "" && !requestBoundNonterminal) ||
			(retired.Reason == runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry && retired.FailureSignature == "terminal-delivery") {
			return fmt.Errorf("runtime aggregate retired terminal-delivery lease does not match administrative termination")
		}
	} else if recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID != "" {
		return fmt.Errorf("runtime aggregate retired terminal-delivery handoff request has no exact lease")
	}
	if a.DriveLease == nil {
		return nil
	}
	if err := a.DriveLease.Validate(); err != nil {
		return fmt.Errorf("runtime aggregate drive lease: %w", err)
	}
	if a.DriveLease.WorkspaceID != a.Facts.WorkspaceID || a.DriveLease.RunID != a.Facts.RunID {
		return fmt.Errorf("runtime aggregate drive lease scope mismatch")
	}
	canonicalTerminalDelivery := a.DriveLease.Reason == runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry &&
		a.DriveLease.FailureSignature == "terminal-delivery"
	if canonicalTerminalDelivery {
		authorization := recovery.TerminalDeliveryRequest
		outcome, terminal, outcomeErr := DeriveTerminalOutcome(a.Decision, a.Termination)
		if outcomeErr != nil || !terminal {
			return fmt.Errorf("runtime aggregate terminal-delivery lease has no sealed terminal")
		}
		deliveryID := runtimecontract.StableTerminalDeliveryID(a.Facts.WorkspaceID, a.Facts.RunID, a.Decision.AuthoritySnapshot.ID, outcome)
		creatorGeneration := a.DriveLease.RecoveryGeneration - 1
		creatorRequestID := runtimecontract.StableRuntimeTerminalDeliveryRecoveryRequestID(deliveryID, creatorGeneration)
		phaseMatches := authorization.Validate() == nil && authorization.TerminalDeliveryID == deliveryID &&
			((authorization.FailedLeaseGeneration == creatorGeneration && authorization.RequestID == creatorRequestID) ||
				(a.DriveLease.State == runtimecontract.RuntimeDriveLeaseRunning && authorization.FailedLeaseGeneration == a.DriveLease.RecoveryGeneration &&
					authorization.RequestID == runtimecontract.StableRuntimeTerminalDeliveryRecoveryRequestID(deliveryID, a.DriveLease.RecoveryGeneration)))
		if a.DriveLease.RecoveryRequestID != creatorRequestID || !phaseMatches {
			return fmt.Errorf("runtime aggregate terminal-delivery lease does not match its persisted recovery request")
		}
	} else if runtimecontract.RuntimeDriveLeaseBindsNonTerminalRecovery(*a.DriveLease) {
		if err := runtimecontract.ValidateRuntimeNonTerminalRecoveryLeaseBinding(recovery, *a.DriveLease); err != nil {
			return fmt.Errorf("runtime aggregate drive lease: %w", err)
		}
		// The worker holding this exact running request may seal ordinary
		// convergence before its exit settlement releases the lease or performs
		// first terminal delivery. A scheduled lease has not entered that worker
		// cut, and administrative termination owns a separate retired-lease lane.
		if a.Termination != nil || (isTerminalConvergenceState(a.Decision.State) && a.DriveLease.State != runtimecontract.RuntimeDriveLeaseRunning) {
			return fmt.Errorf("runtime aggregate terminal state cannot retain nonterminal recovery authority outside an exact running convergence handoff")
		}
	} else if a.DriveLease.RecoveryRequestID != "" {
		return fmt.Errorf("runtime aggregate non-terminal lease carries terminal recovery request")
	}
	if retiredPresent && a.DriveLease.RecoveryGeneration == retired.RecoveryGeneration+1 &&
		recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID != a.DriveLease.RecoveryRequestID {
		return fmt.Errorf("runtime aggregate retired terminal-delivery successor is not bound to its handoff request")
	}
	if a.Termination != nil && (a.DriveLease.Reason != runtimecontract.RuntimeDriveLeaseReasonDeliveryRetry ||
		a.DriveLease.FailureSignature != "terminal-delivery") {
		return fmt.Errorf("administratively terminated runtime aggregate can retain only the canonical terminal-delivery retry lease")
	}
	return nil
}

func validateRuntimeAggregateCrossMachineFences(a RuntimeAggregate, active *runtimecontract.RuntimeDispatchRecord, completed []runtimecontract.RuntimeDispatchRecord) error {
	if err := validateRuntimeGovernanceRequestDispatchBindings(a, active, completed); err != nil {
		return err
	}
	if err := validateRuntimeOperationGovernanceBindingFences(a, active, completed); err != nil {
		return err
	}
	return validateRuntimeGovernanceResolutionBindings(a)
}

func validateRuntimeGovernanceRequestDispatchBindings(a RuntimeAggregate, active *runtimecontract.RuntimeDispatchRecord, completed []runtimecontract.RuntimeDispatchRecord) error {
	return runtimecontract.ValidateRuntimeGovernanceRequestDispatchLineage(a.GovernanceRequests, active, completed)
}

func validateRuntimeOperationGovernanceBindingFences(a RuntimeAggregate, active *runtimecontract.RuntimeDispatchRecord, completed []runtimecontract.RuntimeDispatchRecord) error {
	if err := runtimecontract.ValidateRuntimeDispatchExecutionKeyLineage(
		a.Facts.WorkspaceID,
		a.Facts.RunID,
		active,
		completed,
		a.OperationGovernanceBindings,
	); err != nil {
		return err
	}
	for _, rawBinding := range a.OperationGovernanceBindings {
		binding := rawBinding.Normalized()
		var source *runtimecontract.RuntimeDispatchRecord
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

func validateRuntimeGovernanceResolutionBindings(a RuntimeAggregate) error {
	for _, resolution := range a.GovernanceResolutions {
		resolution = resolution.Normalized()
		if resolution.WorkspaceID != a.Facts.WorkspaceID || resolution.RunID != a.Facts.RunID {
			return fmt.Errorf("runtime aggregate governance resolution scope mismatch")
		}
		matched := false
		bound := false
		for _, request := range a.GovernanceRequests {
			if governancecheck.ResolutionRecordMatchesRequest(resolution, request) {
				bound = true
				break
			}
		}
		for _, dispatch := range a.CompletedDispatches {
			if dispatch.State == DispatchStateCommitted && governancecheck.ResolutionRecordMatchesDispatch(resolution, governancecheck.DispatchFacts{
				DispatchID: dispatch.ID, AuthoritySnapshotID: dispatch.Transaction.AuthoritySnapshotID,
				CandidateGovernance: dispatch.Candidate.Governance != nil,
			}) {
				matched = true
				break
			}
		}
		if !matched || !bound {
			return fmt.Errorf("runtime aggregate governance resolution has no completed governed dispatch")
		}
	}
	return nil
}

func hasPendingCandidateAttempt(values []runtimecontract.RuntimeCandidateAttemptRecord) bool {
	for _, value := range values {
		if value.State == runtimecontract.RuntimeCandidateAttemptStatePending {
			return true
		}
	}
	return false
}

func hasUnreducedCurrentCandidateRetryFailure(decision runtimecontract.RuntimeDecision, values []runtimecontract.RuntimeCandidateAttemptRecord) bool {
	current := decision.AuthorityTransaction()
	for _, value := range values {
		if value.Attempt == 1 && value.State == runtimecontract.RuntimeCandidateAttemptStateFailed && runtimecontract.SameAuthorityContract(value.Transaction, current) {
			return true
		}
	}
	return false
}

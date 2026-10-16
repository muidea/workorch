package workorchruntimecontract

import (
	"fmt"
	"strings"
)

// Original full record gate and ordering before cross-consumer call-local reuse.
func referenceRuntimeStateConsumerSession(r RuntimeStateRecord) error {
	if err := ValidateRuntimeResultFeedbackVersion(r.ResultFeedbackVersion); err != nil {
		return err
	}
	if r.Facts.WorkspaceID == "" || r.Facts.RunID == "" {
		return fmt.Errorf("runtime state requires workspace and run scope")
	}
	if r.Decision.RequiredOutput.ImplementationWorkBindingHash != "" {
		matches := 0
		for _, round := range r.ImplementationWork.Rounds {
			if round.WorkingBinding != nil && round.WorkingBinding.Hash == r.Decision.RequiredOutput.ImplementationWorkBindingHash && round.CompletedReceiptSetHash == "" && round.FailedReceiptHash == "" && round.GovernanceReceiptHash == "" && len(r.Facts.Focus.CoverageWorkItemIDs) == 1 && r.Facts.Focus.CoverageWorkItemIDs[0] == round.CoverageWorkItemID && r.Facts.Failure == nil && r.Facts.RepairDirective == nil {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("runtime state working output has no unique live original semantic work commitment")
		}
	}
	if err := ValidateRuntimeImplementationWorkState(r.Definition, r.Facts, r.ImplementationWork, r.SemanticPlanning, r.ActiveDispatch, r.CompletedDispatches, r.OperationReceipts, r.OperationGovernanceBindings, r.ExecutionBudget); err != nil {
		return fmt.Errorf("runtime state implementation work: %w", err)
	}
	if err := ValidateRuntimeDispatchHistoryShape(r.Facts.WorkspaceID, r.Facts.RunID, r.ActiveDispatch, r.CompletedDispatches); err != nil {
		return fmt.Errorf("runtime state dispatch history: %w", err)
	}
	if err := ValidateRuntimeEvidenceLedgerSequence(r.EvidenceSequence, r.Facts, r.ActiveDispatch, r.CompletedDispatches, r.GovernanceResolutions); err != nil {
		return fmt.Errorf("runtime state evidence ledger: %w", err)
	}
	if err := ValidateRuntimeEvidenceApplicabilityObservations(r.Facts, r.OperationReceipts, r.StateRevision, r.Reconciliation); err != nil {
		return fmt.Errorf("runtime state evidence applicability: %w", err)
	}
	if err := ValidateRuntimeCWISuspensionSources(r.Facts, r.CompletedDispatches, r.StateRevision); err != nil {
		return err
	}
	if err := ValidateRuntimeApplicabilityCandidateInvalidations(r.Facts.EvidenceApplicabilityObservations, r.CandidateAttempts, r.CandidateRequestSessions); err != nil {
		return err
	}
	if err := ValidateRuntimeAssessmentAuthoritySources(r.Facts, r.ActiveDispatch, r.CompletedDispatches); err != nil {
		return fmt.Errorf("runtime state assessment authority: %w", err)
	}
	if err := ValidateRuntimeAssessmentApplicabilityGenerations(r.Reconciliation, r.ActiveDispatch, r.CompletedDispatches); err != nil {
		return fmt.Errorf("runtime state assessment applicability generation: %w", err)
	}
	if err := ValidateRuntimeAssessmentAdjudications(r.Facts, r.CompletedDispatches, r.CoverageProofLineage, RuntimeLineageAssertionSources{Definition: r.Definition, Facts: r.Facts, Work: r.ImplementationWork, Semantic: r.SemanticPlanning, Active: r.ActiveDispatch, Completed: r.CompletedDispatches, Receipts: r.OperationReceipts, Governance: r.OperationGovernanceBindings, Budget: r.ExecutionBudget, EvidenceSequence: r.EvidenceSequence, Resolutions: r.GovernanceResolutions, Attempts: r.CandidateAttempts, Sessions: r.CandidateRequestSessions, ProcessedEventIDs: r.ProcessedEventIDs, ProofLineage: r.CoverageProofLineage, Confirmations: r.RepairSurfaceConfirmations}); err != nil {
		return fmt.Errorf("runtime state assessment adjudication: %w", err)
	}
	if err := ValidateRuntimeDefinitionFocuses(r.Facts.DefinitionSeed, r.Facts.DefinitionFocuses); err != nil {
		return fmt.Errorf("runtime state definition focuses: %w", err)
	}
	if err := ValidateRuntimeStateCWILedger(r.Facts, r.Reconciliation); err != nil {
		return err
	}
	if err := ValidateRuntimeCWIBindingRetirementsAgainstExecution(r.Facts, r.ActiveDispatch, r.CompletedDispatches, r.OperationReceipts); err != nil {
		return err
	}
	if err := r.Facts.Validate(); err != nil {
		return err
	}
	if err := r.SemanticPlanning.Validate(); err != nil {
		return fmt.Errorf("runtime semantic planning ledger: %w", err)
	}
	if err := ValidateRuntimeSemanticProofBatchLimits(r.SemanticPlanning, r.Facts.Activation.ExecutionContract.ExecutionLimits); err != nil {
		return err
	}
	if r.SemanticPlanning.DefinitionSeed.Hash != r.Facts.DefinitionSeed.Hash {
		return fmt.Errorf("runtime semantic planning seed does not match facts")
	}
	if err := r.Reconciliation.Validate(); err != nil {
		return fmt.Errorf("runtime reconciliation ledger: %w", err)
	}
	if err := ValidateRuntimeCoverageProofLineage(r.CoverageProofLineage); err != nil {
		return fmt.Errorf("runtime coverage proof lineage: %w", err)
	}
	if err := r.Definition.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.Definition.Revision) != strings.TrimSpace(r.Facts.DefinitionRevision) {
		return fmt.Errorf("runtime state definition revision mismatch")
	}
	if !SameRuntimeSourceObligationInventory(r.Definition.SourceObligations, r.Facts.DefinitionSeed.SourceObligations) {
		return fmt.Errorf("runtime state Definition and seed source obligations mismatch")
	}
	if r.Facts.DefinitionSeed.SourceObligations != nil && r.Facts.DefinitionSeed.SourceObligations.StartupInputHash != r.Definition.StartupInputHash {
		return fmt.Errorf("runtime state source obligations startup input mismatch")
	}
	if err := r.Decision.Validate(); err != nil {
		return err
	}
	if r.Decision.AuthoritySnapshot.WorkspaceID != r.Facts.WorkspaceID || r.Decision.AuthoritySnapshot.RunID != r.Facts.RunID {
		return fmt.Errorf("runtime state decision scope mismatch")
	}
	if r.ActiveDispatch != nil {
		if err := r.ActiveDispatch.ValidateWithActivation(r.Decision, RuntimeActivationForFacts(r.Facts)); err != nil {
			return fmt.Errorf("runtime state active dispatch: %w", err)
		}
	}
	if err := ValidateRuntimeCandidateAttemptRecordsForScope(r.Facts.WorkspaceID, r.Facts.RunID, r.CandidateAttempts); err != nil {
		return err
	}
	if err := ValidateRuntimeCandidateRequestSessions(r.CandidateRequestSessions); err != nil {
		return err
	}
	if err := r.ExecutionBudget.Validate(); err != nil {
		return fmt.Errorf("runtime execution budget: %w", err)
	}
	if !SameRuntimeExecutionBudgetPolicy(r.ExecutionBudget.Policy, r.Facts.ExecutionBudgetPolicy) {
		return fmt.Errorf("runtime execution budget policy does not match activation facts")
	}
	if r.DefinitionClarification != nil {
		if err := r.DefinitionClarification.Validate(); err != nil {
			return fmt.Errorf("runtime Definition clarification: %w", err)
		}
		clarification := r.DefinitionClarification.Clone()
		if clarification.Observation.WorkspaceID != r.Facts.WorkspaceID || clarification.Observation.RunID != r.Facts.RunID ||
			clarification.Observation.DefinitionRevision != r.Definition.Normalized().Revision || clarification.Observation.StartupInputHash != r.Definition.Normalized().StartupInputHash ||
			r.Decision.State != ConvergenceStateNeedGovernance || !sameStrings(r.Decision.RequiredOutput.Normalized().AcceptedKinds, []string{RuntimeDefinitionClarificationRequiredOutputKind}) || r.ActiveDispatch != nil {
			return fmt.Errorf("runtime Definition clarification aggregate binding mismatch")
		}
		if clarification.Dispatch != nil && clarification.Dispatch.AuthoritySnapshotID != r.Decision.AuthoritySnapshot.ID {
			return fmt.Errorf("runtime Definition clarification dispatch authority mismatch")
		}
		matchedSession := false
		for _, session := range r.CandidateRequestSessions {
			if session.LogicalRequestID == clarification.Observation.LogicalRequestID && session.State == RuntimeCandidateRequestSessionDocumentComplete &&
				session.Transaction.AuthoritySnapshotID == clarification.Observation.AuthoritySnapshotID {
				matchedSession = true
				break
			}
		}
		if !matchedSession {
			return fmt.Errorf("runtime Definition clarification has no complete semantic request session")
		}
	}
	if isTerminalRuntimeDecision(r.Decision) && hasPendingRuntimeCandidateAttempt(r.CandidateAttempts) {
		return fmt.Errorf("terminal runtime state cannot retain a pending candidate attempt")
	}
	if hasUnreducedCurrentCandidateRetryFailure(r.Decision, r.CandidateAttempts) {
		return fmt.Errorf("current runtime authority retains a failed candidate attempt 1 without reducer transition")
	}
	for _, attempt := range r.CandidateAttempts {
		// A pending proposal is the exact candidate that can be resumed into the
		// feasibility gate. Validate it again at aggregate load so corrupted
		// persistence cannot cause owner probes for a candidate that was never
		// schema-valid under the still-current authority contract.
		if attempt.State != RuntimeCandidateAttemptStatePending || attempt.Proposal == nil {
			continue
		}
		if !SameAuthorityContract(attempt.Transaction, r.Decision.AuthorityTransaction()) {
			return fmt.Errorf("pending runtime candidate proposal does not match current authority contract")
		}
		if err := attempt.Proposal.ValidateWithActivation(r.Decision, RuntimeActivationForFacts(r.Facts)); err != nil {
			return fmt.Errorf("pending runtime candidate proposal is invalid: %w", err)
		}
	}
	if err := ValidateRuntimeGovernanceResolutionRecords(r.GovernanceResolutions); err != nil {
		return err
	}
	if err := ValidateRuntimeGovernanceRequestRecords(r.GovernanceRequests); err != nil {
		return err
	}
	if err := ValidateRuntimeGovernanceRequestDispatchLineage(r.GovernanceRequests, r.ActiveDispatch, r.CompletedDispatches); err != nil {
		return err
	}
	if err := ValidateRuntimeOperationGovernanceBindings(r.OperationGovernanceBindings); err != nil {
		return err
	}
	if err := ValidateRuntimeDispatchExecutionKeyLineage(r.Facts.WorkspaceID, r.Facts.RunID, r.ActiveDispatch, r.CompletedDispatches, r.OperationGovernanceBindings); err != nil {
		return err
	}
	if err := ValidateRuntimeDispatchAdmissionLineage(r.ActiveDispatch, r.CompletedDispatches, r.CandidateAttempts, r.OperationGovernanceBindings); err != nil {
		return err
	}
	if err := ValidateRuntimeAcceptedCandidateManifestLineage(r.ActiveDispatch, r.CompletedDispatches, r.CandidateAttempts, r.CandidateRequestSessions); err != nil {
		return err
	}
	if err := ValidateRuntimeDurableDispatchLimits(r.Facts.Activation.ExecutionContract.ExecutionLimits, r.ActiveDispatch, r.CompletedDispatches, r.CandidateAttempts); err != nil {
		return err
	}
	if err := ValidateRuntimeAcceptedAssessmentPurposePreimagesWithValidatedSemanticLedger(r.SemanticPlanning, r.ActiveDispatch, r.CompletedDispatches, r.CandidateAttempts, r.CandidateRequestSessions); err != nil {
		return err
	}
	if err := ValidateRuntimeOperationReceipts(r.OperationReceipts); err != nil {
		return err
	}
	if err := ValidateRuntimeOperationReceiptExecutionLineage(r.ActiveDispatch, r.CompletedDispatches, r.OperationReceipts); err != nil {
		return err
	}
	if err := ValidateRuntimeHistoricalBehaviorCompletions(RuntimeLineageAssertionSources{
		Definition: r.Definition, Facts: r.Facts, Semantic: r.SemanticPlanning, Completed: r.CompletedDispatches,
		Receipts: r.OperationReceipts, ProcessedEventIDs: r.ProcessedEventIDs, Reconciliation: r.Reconciliation,
		Work: r.ImplementationWork, Active: r.ActiveDispatch, Governance: r.OperationGovernanceBindings,
		Budget: r.ExecutionBudget, EvidenceSequence: r.EvidenceSequence, Resolutions: r.GovernanceResolutions,
		Attempts: r.CandidateAttempts, Sessions: r.CandidateRequestSessions,
		ProofLineage: r.CoverageProofLineage, Confirmations: r.RepairSurfaceConfirmations,
	}); err != nil {
		return err
	}
	for _, invalidation := range r.PrerequisiteInvalidations {
		if err := invalidation.Validate(); err != nil {
			return fmt.Errorf("runtime prerequisite invalidation: %w", err)
		}
	}
	if err := ValidateRuntimeRepairSurfaceConfirmationRecords(r.RepairSurfaceConfirmations); err != nil {
		return fmt.Errorf("runtime repair surface confirmations: %w", err)
	}
	for _, confirmation := range r.RepairSurfaceConfirmations {
		if confirmation.Confirmation.Observation.WorkspaceID != r.Facts.WorkspaceID || confirmation.Confirmation.Observation.RunID != r.Facts.RunID {
			return fmt.Errorf("runtime repair surface confirmation scope mismatch")
		}
	}
	recovery := r.Facts.Recovery.Normalized()
	if err := ValidateRuntimeNonTerminalRecoveryLinearization(recovery, r.StateRevision, r.DriveLease); err != nil {
		return fmt.Errorf("runtime state recovery: %w", err)
	}
	if r.DriveLease != nil {
		if err := r.DriveLease.Validate(); err != nil {
			return fmt.Errorf("runtime state drive lease: %w", err)
		}
		if r.DriveLease.WorkspaceID != r.Facts.WorkspaceID || r.DriveLease.RunID != r.Facts.RunID {
			return fmt.Errorf("runtime state drive lease scope mismatch")
		}
		canonicalTerminalDelivery := r.DriveLease.Reason == RuntimeDriveLeaseReasonDeliveryRetry && r.DriveLease.FailureSignature == "terminal-delivery"
		if canonicalTerminalDelivery {
			authorization := r.Facts.Recovery.Normalized().TerminalDeliveryRequest
			outcome, terminal, outcomeErr := DeriveRuntimeTerminalOutcome(r.Decision, r.Termination)
			if outcomeErr != nil || !terminal {
				return fmt.Errorf("runtime state terminal-delivery lease has no sealed terminal")
			}
			deliveryID := StableTerminalDeliveryID(r.Facts.WorkspaceID, r.Facts.RunID, r.Decision.AuthoritySnapshot.ID, outcome)
			creatorGeneration := r.DriveLease.RecoveryGeneration - 1
			creatorRequestID := StableRuntimeTerminalDeliveryRecoveryRequestID(deliveryID, creatorGeneration)
			phaseMatches := authorization.Validate() == nil && authorization.TerminalDeliveryID == deliveryID &&
				((authorization.FailedLeaseGeneration == creatorGeneration && authorization.RequestID == creatorRequestID) ||
					(r.DriveLease.State == RuntimeDriveLeaseRunning && authorization.FailedLeaseGeneration == r.DriveLease.RecoveryGeneration &&
						authorization.RequestID == StableRuntimeTerminalDeliveryRecoveryRequestID(deliveryID, r.DriveLease.RecoveryGeneration)))
			if r.DriveLease.RecoveryRequestID != creatorRequestID || !phaseMatches {
				return fmt.Errorf("runtime state terminal-delivery lease does not match persisted recovery request")
			}
		} else if RuntimeDriveLeaseBindsNonTerminalRecovery(*r.DriveLease) {
			if err := ValidateRuntimeNonTerminalRecoveryLeaseBinding(r.Facts.Recovery, *r.DriveLease); err != nil {
				return fmt.Errorf("runtime state drive lease: %w", err)
			}
			// A newly sealed convergence terminal may retain only the exact
			// request-bound running worker until SM-4 exit settlement. Scheduled
			// authority and the administrative termination lane remain disjoint.
			if r.Termination != nil || (isTerminalRuntimeDecision(r.Decision) && r.DriveLease.State != RuntimeDriveLeaseRunning) {
				return fmt.Errorf("runtime state terminal cannot retain nonterminal recovery authority outside an exact running convergence handoff")
			}
		} else if r.DriveLease.RecoveryRequestID != "" {
			return fmt.Errorf("runtime state non-terminal lease carries terminal recovery request")
		}
	}
	if recovery.TerminalDeliveryIntervention.Validate() == nil && r.DriveLease != nil {
		return fmt.Errorf("runtime state terminal delivery intervention cannot retain automatic drive authority")
	}
	retiredLease := recovery.RetiredTerminalDeliveryLease
	retiredLeasePresent := RuntimeDriveLeasePresent(retiredLease)
	if retiredLeasePresent {
		requestBoundNonterminal := RuntimeDriveLeaseBindsNonTerminalRecovery(retiredLease) &&
			ValidateRuntimeNonTerminalRecoveryLeaseBinding(recovery, retiredLease) == nil
		if r.Termination == nil || retiredLease.Validate() != nil || retiredLease.State != RuntimeDriveLeaseRunning ||
			retiredLease.WorkspaceID != r.Facts.WorkspaceID || retiredLease.RunID != r.Facts.RunID ||
			retiredLease.AggregateRevision >= r.Termination.Revision || (retiredLease.RecoveryRequestID != "" && !requestBoundNonterminal) ||
			(retiredLease.Reason == RuntimeDriveLeaseReasonDeliveryRetry && retiredLease.FailureSignature == "terminal-delivery") {
			return fmt.Errorf("runtime state retired terminal-delivery lease does not match administrative termination")
		}
		if recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID != "" {
			outcome, terminal, err := DeriveRuntimeTerminalOutcome(r.Decision, r.Termination)
			if err != nil || !terminal {
				return fmt.Errorf("runtime state retired terminal-delivery lease has no terminal authority")
			}
			deliveryID := StableTerminalDeliveryID(r.Facts.WorkspaceID, r.Facts.RunID, r.Decision.AuthoritySnapshot.ID, outcome)
			expectedRequestID := StableRuntimeTerminalDeliveryRecoveryRequestID(deliveryID, retiredLease.RecoveryGeneration)
			if recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID != expectedRequestID {
				return fmt.Errorf("runtime state retired terminal-delivery lease handoff request mismatch")
			}
		}
		if r.DriveLease != nil && r.DriveLease.Reason == RuntimeDriveLeaseReasonDeliveryRetry && r.DriveLease.FailureSignature == "terminal-delivery" &&
			r.DriveLease.RecoveryGeneration == retiredLease.RecoveryGeneration+1 &&
			recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID != r.DriveLease.RecoveryRequestID {
			return fmt.Errorf("runtime state retired terminal-delivery successor is not bound to its handoff request")
		}
	} else if recovery.RetiredTerminalDeliveryLeaseRecoveryRequestID != "" {
		return fmt.Errorf("runtime state retired terminal-delivery handoff request has no exact lease")
	}
	for _, rawBinding := range r.OperationGovernanceBindings {
		binding := rawBinding.Normalized()
		var source *RuntimeDispatchRecord
		for idx := range r.CompletedDispatches {
			if r.CompletedDispatches[idx].ID == binding.OperationDispatchID {
				value := r.CompletedDispatches[idx]
				source = &value
				break
			}
		}
		if source == nil || source.State != RuntimeDispatchStateCommitted {
			return fmt.Errorf("operation governance binding source dispatch %q is not completed", binding.OperationDispatchID)
		}
		var execution *RuntimeOperationExecutionRecord
		for idx := range source.OperationExecutions {
			if source.OperationExecutions[idx].OperationID == binding.OperationID {
				value := source.OperationExecutions[idx].Normalized()
				execution = &value
				break
			}
		}
		if execution == nil || execution.ExecutionKey != binding.ExecutionKey || execution.Fence != binding.Fence ||
			execution.ExpectedEffectScope != binding.ExpectedEffectScope ||
			execution.IdempotencyClass != binding.IdempotencyClass || execution.AuthorityHash != binding.ExecutionAuthorityHash ||
			execution.OwnerReceiptHash != binding.GovernanceReceiptHash || execution.State != OperationExecutionOwnerCommitted {
			return fmt.Errorf("operation governance binding source execution does not match sealed receipt")
		}
		receipt, found := RuntimeOperationReceiptForExecutionKeyFence(r.OperationReceipts, binding.ExecutionKey, binding.Fence)
		if !found || receipt.Outcome != RuntimeOperationOutcomeGovernanceRequired || StableRuntimeOperationReceiptHash(receipt) != binding.GovernanceReceiptHash {
			return fmt.Errorf("operation governance binding has no matching governance receipt")
		}
		if binding.State == RuntimeOperationGovernanceBindingSuspended {
			continue
		}
		bound := false
		for _, request := range r.GovernanceRequests {
			if request.TicketID == binding.TicketID && request.ApprovalID == binding.ApprovalID && request.DispatchID == binding.GovernanceDispatchID &&
				request.AuthoritySnapshotID == binding.GovernanceSnapshotID && request.DependencyKind == binding.DependencyKind && request.SubjectRef == binding.SubjectRef &&
				request.RequestedAction == binding.RequestedAction && request.RiskLevel == binding.RiskLevel && request.AuthorizationHash == binding.AuthorizationHash {
				bound = true
				break
			}
		}
		if !bound {
			return fmt.Errorf("operation governance binding has no matching owner request")
		}
	}
	governedExecutions := make([]RuntimeOperationExecutionRecord, 0)
	if r.ActiveDispatch != nil {
		governedExecutions = append(governedExecutions, r.ActiveDispatch.OperationExecutions...)
	}
	for _, dispatch := range r.CompletedDispatches {
		governedExecutions = append(governedExecutions, dispatch.OperationExecutions...)
	}
	if err := ValidateRuntimeOperationGovernanceExecutionBindings(r.OperationGovernanceBindings, governedExecutions); err != nil {
		return err
	}
	if err := ValidateRuntimeOperationGovernanceGrantLineage(r.OperationGovernanceBindings, r.GovernanceResolutions, r.Facts.Evidence.Accepted, r.Facts.Evidence.Rejected); err != nil {
		return err
	}
	for _, attempt := range r.CandidateAttempts {
		if attempt.State != RuntimeCandidateAttemptStateAccepted {
			continue
		}
		matched := r.ActiveDispatch != nil && RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, *r.ActiveDispatch)
		if !matched {
			for _, dispatch := range r.CompletedDispatches {
				if RuntimeAcceptedCandidateAttemptMatchesDispatch(attempt, dispatch) {
					matched = true
					break
				}
			}
		}
		if !matched {
			return fmt.Errorf("accepted runtime candidate attempt has no exact sealed dispatch target/validation-recipe authority projection")
		}
	}
	for _, resolution := range r.GovernanceResolutions {
		resolution = resolution.Normalized()
		if resolution.WorkspaceID != r.Facts.WorkspaceID || resolution.RunID != r.Facts.RunID {
			return fmt.Errorf("runtime governance resolution scope mismatch")
		}
		matched := false
		bound := false
		for _, request := range r.GovernanceRequests {
			if request.TicketID == resolution.TicketID && request.ApprovalID == resolution.ApprovalID && request.DispatchID == resolution.DispatchID && request.AuthoritySnapshotID == resolution.AuthoritySnapshotID &&
				strings.TrimSpace(request.DependencyKind) == resolution.DependencyKind && strings.TrimSpace(request.SubjectRef) == resolution.SubjectRef {
				bound = true
				break
			}
		}
		for _, dispatch := range r.CompletedDispatches {
			if dispatch.ID == resolution.DispatchID && dispatch.Transaction.AuthoritySnapshotID == resolution.AuthoritySnapshotID &&
				dispatch.State == RuntimeDispatchStateCommitted && dispatch.Candidate.Governance != nil {
				matched = true
				break
			}
		}
		if !matched || !bound {
			return fmt.Errorf("runtime governance resolution has no completed governed dispatch")
		}
	}
	if r.Termination != nil {
		if err := r.Termination.Validate(); err != nil {
			return fmt.Errorf("runtime state termination: %w", err)
		}
		if r.Termination.Revision > r.StateRevision {
			return fmt.Errorf("runtime state termination revision exceeds state revision")
		}
	}
	if err := ValidateRuntimeTerminalMaterialSeal(r.TerminalMaterialSeal, r); err != nil {
		return err
	}
	if err := ValidateRuntimeOperationRecoveryAuthorizations(r.Facts.WorkspaceID, r.Facts.RunID, r.ActiveDispatch, r.CompletedDispatches, r.OperationReceipts, r.OperationRecoveryAuthorizations, r.Facts.Recovery, r.StateRevision); err != nil {
		return err
	}
	if r.ActiveDispatch != nil {
		if err := ValidateRuntimeOperationExecutionRecords(r.ActiveDispatch.OperationExecutions); err != nil {
			return fmt.Errorf("runtime state active dispatch executions: %w", err)
		}
	}
	for idx, dispatch := range r.CompletedDispatches {
		if err := ValidateRuntimeOperationExecutionRecords(dispatch.OperationExecutions); err != nil {
			return fmt.Errorf("runtime state completed dispatch[%d] executions: %w", idx, err)
		}
	}
	return nil
}

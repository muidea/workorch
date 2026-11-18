package runtime

import (
	"fmt"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// validateOwnerAggregateJoins follows the one canonical DTO admission. These
// invariants exist only in the Runtime aggregate: it must not re-admit the DTO's
// source/semantic/history/budget/receipt or dispatch ledgers.
func (a RuntimeAggregate) validateOwnerAggregateJoins(gate *rc.RuntimeOwnerInvariantSourceGate) error {
	return a.validateOwnerAggregateTransition(nil, gate)
}

// Each unchanged rule consumes its formal preimage result. Dependencies below
// are typed inputs of that rule, not a runtime registry or a retained PASS.
func (a RuntimeAggregate) validateOwnerAggregateTransition(previous *RuntimeAggregate, gate *rc.RuntimeOwnerInvariantSourceGate) error {
	changedActiveDispatch := previous == nil || !rc.RuntimeRawEqual(a.ActiveDispatch, previous.ActiveDispatch)
	changedCandidateAttempts := previous == nil || !rc.RuntimeRawEqual(a.CandidateAttempts, previous.CandidateAttempts)
	changedCandidateRequestSessions := previous == nil || !rc.RuntimeRawEqual(a.CandidateRequestSessions, previous.CandidateRequestSessions)
	changedCompletedDispatches := previous == nil || !rc.RuntimeRawEqual(a.CompletedDispatches, previous.CompletedDispatches)
	changedCoverageProofLineage := previous == nil || !rc.RuntimeRawEqual(a.CoverageProofLineage, previous.CoverageProofLineage)
	changedDecision := previous == nil || !rc.RuntimeRawEqual(a.Decision, previous.Decision)
	changedDefinition := previous == nil || !rc.RuntimeRawEqual(a.Definition, previous.Definition)
	changedExecutionBudget := previous == nil || !rc.RuntimeRawEqual(a.ExecutionBudget, previous.ExecutionBudget)
	changedGovernanceResolutions := previous == nil || !rc.RuntimeRawEqual(a.GovernanceResolutions, previous.GovernanceResolutions)
	changedOperationReceipts := previous == nil || !rc.RuntimeRawEqual(a.OperationReceipts, previous.OperationReceipts)
	changedProcessedEventIDs := previous == nil || !rc.RuntimeRawEqual(a.ProcessedEventIDs, previous.ProcessedEventIDs)
	changedReconciliation := previous == nil || !rc.RuntimeRawEqual(a.Reconciliation, previous.Reconciliation)
	changedRecoverableWaits := previous == nil || !rc.RuntimeRawEqual(a.RecoverableWaits, previous.RecoverableWaits)
	changedRepairSurfaceConfirmations := previous == nil || !rc.RuntimeRawEqual(a.RepairSurfaceConfirmations, previous.RepairSurfaceConfirmations)
	changedSemanticPlanning := previous == nil || !rc.RuntimeRawEqual(a.SemanticPlanning, previous.SemanticPlanning)
	changedTermination := previous == nil || !rc.RuntimeRawEqual(a.Termination, previous.Termination)
	changedFactsScope := previous == nil
	changedDefinitionRevision := previous == nil
	changedDefinitionSeed := previous == nil
	changedDefinitionFocuses := previous == nil
	changedFocusQueues := previous == nil
	changedCoverageFacts := previous == nil
	changedAdmissibilityFacts := previous == nil
	changedAcceptedEvidence := previous == nil
	changedRejectedEvidence := previous == nil
	changedProfileContracts := previous == nil
	changedEvidencePolicies := previous == nil
	changedCWISuspensions := previous == nil
	changedReconciliationContract := previous == nil
	changedRepairDirective := previous == nil
	changedPrerequisites := previous == nil
	changedFailure := previous == nil
	changedResourcesOther := previous == nil
	changedActivationOther := previous == nil
	changedFactsOther := previous == nil
	if previous != nil {
		changedFactsScope = a.Facts.WorkspaceID != previous.Facts.WorkspaceID || a.Facts.RunID != previous.Facts.RunID
		changedDefinitionRevision = a.Facts.DefinitionRevision != previous.Facts.DefinitionRevision
		changedDefinitionSeed = !rc.RuntimeRawEqual(a.Facts.DefinitionSeed, previous.Facts.DefinitionSeed)
		changedDefinitionFocuses = !rc.RuntimeRawEqual(a.Facts.DefinitionFocuses, previous.Facts.DefinitionFocuses)
		changedFocusQueues = !rc.RuntimeRawEqual(a.Facts.Focus, previous.Facts.Focus) ||
			!rc.RuntimeRawEqual(a.Facts.RemainingFocuses, previous.Facts.RemainingFocuses) ||
			!rc.RuntimeRawEqual(a.Facts.CompletedFocuses, previous.Facts.CompletedFocuses)
		changedCoverageFacts = !rc.RuntimeRawEqual(a.Facts.Coverage, previous.Facts.Coverage)
		changedAdmissibilityFacts = !rc.RuntimeRawEqual(a.Facts.Resources.OperationAdmissibilities, previous.Facts.Resources.OperationAdmissibilities)
		changedAcceptedEvidence = !rc.RuntimeRawEqual(a.Facts.Evidence.Accepted, previous.Facts.Evidence.Accepted)
		changedRejectedEvidence = !rc.RuntimeRawEqual(a.Facts.Evidence.Rejected, previous.Facts.Evidence.Rejected)
		changedProfileContracts = !rc.RuntimeRawEqual(a.Facts.Activation.ExecutionContract.ProfileContracts, previous.Facts.Activation.ExecutionContract.ProfileContracts)
		changedEvidencePolicies = !rc.RuntimeRawEqual(a.Facts.Activation.EvidencePolicies, previous.Facts.Activation.EvidencePolicies)
		changedCWISuspensions = !rc.RuntimeRawEqual(a.Facts.CWISuspensions, previous.Facts.CWISuspensions)
		changedReconciliationContract = !rc.RuntimeRawEqual(a.Facts.ReconciliationContract, previous.Facts.ReconciliationContract)
		changedRepairDirective = !rc.RuntimeRawEqual(a.Facts.RepairDirective, previous.Facts.RepairDirective)
		changedPrerequisites = !rc.RuntimeRawEqual(a.Facts.Prerequisites, previous.Facts.Prerequisites)
		changedFailure = !rc.RuntimeRawEqual(a.Facts.Failure, previous.Facts.Failure)
		if changedAdmissibilityFacts {
			changedResourcesOther = true
		} else {
			prevRes := previous.Facts.Resources
			prevRes.OperationAdmissibilities = a.Facts.Resources.OperationAdmissibilities
			changedResourcesOther = !rc.RuntimeRawEqual(a.Facts.Resources, prevRes)
		}
		if changedProfileContracts || changedEvidencePolicies {
			changedActivationOther = true
		} else {
			prevAct := previous.Facts.Activation
			prevAct.ExecutionContract.ProfileContracts = a.Facts.Activation.ExecutionContract.ProfileContracts
			prevAct.EvidencePolicies = a.Facts.Activation.EvidencePolicies
			changedActivationOther = !rc.RuntimeRawEqual(a.Facts.Activation, prevAct)
		}
		if changedFactsScope || changedDefinitionRevision || changedDefinitionSeed || changedDefinitionFocuses ||
			changedFocusQueues || changedCoverageFacts || changedAcceptedEvidence || changedRejectedEvidence ||
			changedCWISuspensions || changedReconciliationContract || changedRepairDirective ||
			changedPrerequisites || changedFailure || changedResourcesOther || changedActivationOther {
			changedFactsOther = true
		} else {
			changedFactsOther = a.Facts.CWILedgerSchemaVersion != previous.Facts.CWILedgerSchemaVersion ||
				a.Facts.EvidenceRevision != previous.Facts.EvidenceRevision ||
				!rc.RuntimeRawEqual(a.Facts.Capabilities, previous.Facts.Capabilities) ||
				!rc.RuntimeRawEqual(a.Facts.ConvergencePolicy, previous.Facts.ConvergencePolicy) ||
				!rc.RuntimeRawEqual(a.Facts.RecoveryPolicy, previous.Facts.RecoveryPolicy) ||
				!rc.RuntimeRawEqual(a.Facts.ExecutionBudgetPolicy, previous.Facts.ExecutionBudgetPolicy) ||
				!rc.RuntimeRawEqual(a.Facts.Assessments, previous.Facts.Assessments) ||
				!rc.RuntimeRawEqual(a.Facts.EvidenceApplicabilityObservations, previous.Facts.EvidenceApplicabilityObservations) ||
				!rc.RuntimeRawEqual(a.Facts.StateChange, previous.Facts.StateChange) ||
				!rc.RuntimeRawEqual(a.Facts.Recovery, previous.Facts.Recovery) ||
				!rc.RuntimeRawEqual(a.Facts.Repair, previous.Facts.Repair) ||
				!rc.RuntimeRawEqual(a.Facts.Feasibility, previous.Facts.Feasibility) ||
				!rc.RuntimeRawEqual(a.Facts.Slices, previous.Facts.Slices)
		}
	}
	changedFacts := changedFactsOther

	if changedDefinitionFocuses || changedProfileContracts {
		if err := rc.ValidateRuntimeDefinitionLineageAssertions(a.Facts.DefinitionFocuses, a.Facts.Activation.ExecutionContract.ProfileContracts); err != nil {
			return err
		}
	}
	if changedFocusQueues || changedCoverageFacts || changedDefinitionRevision || changedDefinitionSeed || changedDefinitionFocuses {
		if err := validateRuntimeCWISemanticRebindingFocusProjections(a.Facts); err != nil {
			return err
		}
	}
	if changedFocusQueues || changedCoverageFacts || changedAcceptedEvidence || changedActiveDispatch || changedCompletedDispatches || changedOperationReceipts || changedProcessedEventIDs || changedCoverageProofLineage {
		if err := validateRuntimeProspectiveCoverageCollectiveCredit(a); err != nil {
			return err
		}
	}
	if changedDefinitionRevision || changedDefinitionSeed || changedDefinitionFocuses || changedAdmissibilityFacts ||
		changedEvidencePolicies || changedSemanticPlanning {
		if err := validateRuntimeAggregateValidationProofApplicabilityTransition(previous, a); err != nil {
			return err
		}
	}
	if changedSemanticPlanning || changedDefinitionSeed || changedDefinitionFocuses {
		if err := gate.ValidateDefinitionSources(a.SemanticPlanning, a.Facts.DefinitionSeed, a.Facts.DefinitionFocuses); err != nil {
			return err
		}
	}
	changedBindingFacts := previous == nil || changedFactsScope || changedDefinitionRevision || changedDefinitionSeed || changedDefinitionFocuses || changedFocusQueues || changedCoverageFacts ||
		changedCWISuspensions || changedReconciliationContract
	if (changedBindingFacts || changedSemanticPlanning || changedCandidateRequestSessions || changedCandidateAttempts || changedActiveDispatch || changedCompletedDispatches) && !runtimeSemanticBindingProjectionsUnchanged(previous, a) {
		if err := validateRuntimeSemanticBindingPreimagesTransitionInGate(previous, a, changedBindingFacts, changedSemanticPlanning, gate); err != nil {
			return err
		}
	}
	// Purpose reads coverage credit, focus bindings and the exact repair
	// transaction. An unrelated failure/resource/budget observation is not a new
	// purpose decision and must not expand every historical manifest again.
	changedPurposeFacts := changedCoverageFacts || changedFocusQueues || changedRepairDirective
	if changedPurposeFacts || changedSemanticPlanning || changedCandidateRequestSessions || changedCandidateAttempts || changedActiveDispatch || changedCompletedDispatches || changedRepairSurfaceConfirmations || changedOperationReceipts {
		if err := validateRuntimeSemanticBindingPurposeInvariantTransitionInGate(previous, a, gate); err != nil {
			return err
		}
	}
	if previous == nil {
		if err := validateReconciliationExecutionSources(a); err != nil {
			return err
		}
	} else if changedReconciliation {
		if err := validateReconciliationExecutionTransition(*previous, a); err != nil {
			return err
		}
	}

	if len(a.Decision.Prerequisites) > 0 && (changedAcceptedEvidence || changedDecision || changedReconciliation) {
		if err := validateRuntimeApplicabilityPrerequisiteProjection(a); err != nil {
			return err
		}
	}
	if terminal := a.Decision.Terminal; terminal != nil && (terminal.OriginalDiagnostic != nil || (terminal.Code == "owner_fact_convergence_exhausted" && missingOwnerFactIsUnavailable(a.Facts))) {
		if terminal.Code != "owner_fact_convergence_exhausted" || !rc.RuntimeRawEqual(terminal.OriginalDiagnostic, missingOwnerFactDiagnostic(a.Facts)) {
			return fmt.Errorf("runtime terminal owner cause does not match exact-current missing owner fact")
		}
	}
	if changedCandidateAttempts || changedCandidateRequestSessions {
		if err := validateRuntimeCandidateAttemptSessionBindings(a.CandidateAttempts, a.CandidateRequestSessions); err != nil {
			return err
		}
	}
	changedFeasibilityBindingFacts := changedFactsScope || changedAdmissibilityFacts || changedPrerequisites
	if changedFeasibilityBindingFacts || changedSemanticPlanning || changedCandidateAttempts || changedCandidateRequestSessions {
		if err := validateRuntimeCandidateFeasibilityBindingsTransition(previous, a); err != nil {
			return err
		}
	}
	if (len(a.RecoverableWaits) > 0 || (previous != nil && len(previous.RecoverableWaits) > 0)) &&
		(changedFacts || changedDecision || changedRecoverableWaits || changedExecutionBudget || changedTermination) {
		if err := validateRuntimeAggregateRecoverableWaits(a); err != nil {
			return err
		}
	}
	changedDecisionTerminal := previous == nil || !rc.RuntimeRawEqual(a.Decision.Terminal, previous.Decision.Terminal)
	changedInputReviewFacts := changedFactsScope || changedCoverageFacts || changedRejectedEvidence || changedDefinitionSeed || changedDefinitionFocuses || changedFocusQueues || changedAdmissibilityFacts ||
		changedFailure
	if changedInputReviewFacts || changedSemanticPlanning || changedGovernanceResolutions || changedDecisionTerminal ||
		changedDefinition || changedActiveDispatch || changedCompletedDispatches || changedOperationReceipts {
		if err := validateRuntimeInputReviewAggregate(a); err != nil {
			return err
		}
	}
	changedReceiptJoinFacts := changedFactsScope || changedAcceptedEvidence || changedRejectedEvidence
	activeHasCommittedExec := func(d *rc.RuntimeDispatchRecord) bool {
		if d == nil {
			return false
		}
		for i := range d.OperationExecutions {
			switch d.OperationExecutions[i].Normalized().State {
			case rc.OperationExecutionOwnerCommitted, rc.OperationExecutionEvidenceCommitted:
				return true
			}
		}
		return false
	}
	if changedReceiptJoinFacts || changedCompletedDispatches || changedOperationReceipts || changedProcessedEventIDs ||
		(changedActiveDispatch && (activeHasCommittedExec(a.ActiveDispatch) || (previous != nil && activeHasCommittedExec(previous.ActiveDispatch)))) {
		if err := validateRuntimeAggregateReceiptJoins(a); err != nil {
			return err
		}
	}
	changedRepairJoinFacts := changedFactsScope || changedFocusQueues || changedCoverageFacts || changedDefinitionFocuses || changedAcceptedEvidence || changedRejectedEvidence ||
		changedFailure || changedRepairDirective || changedResourcesOther
	if changedRepairJoinFacts || changedDefinition || changedActiveDispatch || changedCompletedDispatches || changedOperationReceipts || changedRepairSurfaceConfirmations || changedSemanticPlanning || changedCoverageProofLineage || changedGovernanceResolutions || changedCandidateRequestSessions || changedCandidateAttempts {
		if err := validateRuntimeAggregateRepairJoinsTransition(previous, a); err != nil {
			return err
		}
	}
	return nil
}

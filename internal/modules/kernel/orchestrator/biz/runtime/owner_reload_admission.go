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
	diff := rc.FullRuntimeStateTransitionDiff()
	if previous != nil {
		diff = rc.ComputeRuntimeStateTransitionDiff(ownerStateRecord(*previous), ownerStateRecord(a))
	}
	return a.validateOwnerAggregateTransitionWithDiff(previous, diff, gate)
}

func (a RuntimeAggregate) validateOwnerAggregateTransitionWithDiff(previous *RuntimeAggregate, diff rc.RuntimeStateTransitionDiff, gate *rc.RuntimeOwnerInvariantSourceGate) error {
	changedActiveDispatch := diff.ActiveDispatch
	changedCandidateAttempts := diff.CandidateAttempts
	changedCandidateRequestSessions := diff.CandidateRequestSessions
	changedCompletedDispatches := diff.CompletedDispatches
	changedCoverageProofLineage := diff.CoverageProofLineage
	changedDecision := diff.Decision
	changedDefinition := diff.Definition
	changedExecutionBudget := diff.ExecutionBudget
	changedGovernanceResolutions := diff.GovernanceResolutions
	changedOperationReceipts := diff.OperationReceipts
	changedProcessedEventIDs := diff.ProcessedEventIDs
	changedReconciliation := diff.Reconciliation
	changedRecoverableWaits := diff.RecoverableWaits
	changedRepairSurfaceConfirmations := diff.RepairSurfaceConfirmations
	changedSemanticPlanning := diff.SemanticPlanning
	changedTermination := diff.Termination
	changedFactsScope := diff.FactsScope
	changedDefinitionRevision := diff.FactsDefinitionRevision
	changedDefinitionSeed := diff.FactsDefinitionSeed
	changedDefinitionFocuses := diff.FactsDefinitionFocuses
	changedFocusQueues := diff.FactsFocusQueues
	changedCoverageFacts := diff.FactsCoverage
	changedAdmissibilityFacts := diff.FactsResourcesOperationAdmissibilities
	changedAcceptedEvidence := diff.FactsEvidenceAccepted
	changedRejectedEvidence := diff.FactsEvidenceRejected
	changedProfileContracts := diff.FactsActivationProfileContracts
	changedEvidencePolicies := diff.FactsActivationEvidencePolicies
	changedCWISuspensions := diff.FactsCWISuspensions
	changedReconciliationContract := diff.FactsReconciliationContract
	changedRepairDirective := diff.FactsRepairDirective
	changedPrerequisites := diff.FactsPrerequisites
	changedFailure := diff.FactsFailure
	changedResourcesOther := diff.FactsResources()
	changedFacts := diff.FactsAny()

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
	changedDecisionTerminal := diff.DecisionTerminal
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

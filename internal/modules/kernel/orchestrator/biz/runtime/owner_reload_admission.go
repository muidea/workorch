package runtime

import (
	"fmt"
	"reflect"

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
	changedActiveDispatch := previous == nil || !reflect.DeepEqual(a.ActiveDispatch, previous.ActiveDispatch)
	changedCandidateAttempts := previous == nil || !reflect.DeepEqual(a.CandidateAttempts, previous.CandidateAttempts)
	changedCandidateRequestSessions := previous == nil || !reflect.DeepEqual(a.CandidateRequestSessions, previous.CandidateRequestSessions)
	changedCompletedDispatches := previous == nil || !reflect.DeepEqual(a.CompletedDispatches, previous.CompletedDispatches)
	changedCoverageProofLineage := previous == nil || !reflect.DeepEqual(a.CoverageProofLineage, previous.CoverageProofLineage)
	changedDecision := previous == nil || !reflect.DeepEqual(a.Decision, previous.Decision)
	changedDefinition := previous == nil || !reflect.DeepEqual(a.Definition, previous.Definition)
	changedExecutionBudget := previous == nil || !reflect.DeepEqual(a.ExecutionBudget, previous.ExecutionBudget)
	changedFacts := previous == nil || !reflect.DeepEqual(a.Facts, previous.Facts)
	changedGovernanceResolutions := previous == nil || !reflect.DeepEqual(a.GovernanceResolutions, previous.GovernanceResolutions)
	changedOperationReceipts := previous == nil || !reflect.DeepEqual(a.OperationReceipts, previous.OperationReceipts)
	changedProcessedEventIDs := previous == nil || !reflect.DeepEqual(a.ProcessedEventIDs, previous.ProcessedEventIDs)
	changedReconciliation := previous == nil || !reflect.DeepEqual(a.Reconciliation, previous.Reconciliation)
	changedRecoverableWaits := previous == nil || !reflect.DeepEqual(a.RecoverableWaits, previous.RecoverableWaits)
	changedRepairSurfaceConfirmations := previous == nil || !reflect.DeepEqual(a.RepairSurfaceConfirmations, previous.RepairSurfaceConfirmations)
	changedSemanticPlanning := previous == nil || !reflect.DeepEqual(a.SemanticPlanning, previous.SemanticPlanning)
	changedTermination := previous == nil || !reflect.DeepEqual(a.Termination, previous.Termination)

	if changedFacts {
		if err := rc.ValidateRuntimeDefinitionLineageAssertions(a.Facts.DefinitionFocuses, a.Facts.Activation.ExecutionContract.ProfileContracts); err != nil {
			return err
		}
	}
	if changedFacts {
		if err := validateRuntimeCWISemanticRebindingFocusProjections(a.Facts); err != nil {
			return err
		}
	}
	if changedFacts || changedActiveDispatch || changedCompletedDispatches || changedOperationReceipts || changedProcessedEventIDs || changedCoverageProofLineage {
		if err := validateRuntimeProspectiveCoverageCollectiveCredit(a); err != nil {
			return err
		}
	}
	if changedFacts || changedSemanticPlanning {
		if err := validateRuntimeAggregateValidationProofApplicabilityTransition(previous, a); err != nil {
			return err
		}
	}
	if changedFacts || changedSemanticPlanning {
		if err := gate.ValidateDefinitionSources(a.SemanticPlanning, a.Facts.DefinitionSeed, a.Facts.DefinitionFocuses); err != nil {
			return err
		}
	}
	if (changedFacts || changedSemanticPlanning || changedCandidateRequestSessions || changedCandidateAttempts || changedActiveDispatch || changedCompletedDispatches) && !runtimeSemanticBindingProjectionsUnchanged(previous, a) {
		if err := validateRuntimeSemanticBindingPreimagesInGate(a, gate); err != nil {
			return err
		}
	}
	// Purpose reads coverage credit, focus bindings and the exact repair
	// transaction. An unrelated failure/resource/budget observation is not a new
	// purpose decision and must not expand every historical manifest again.
	changedPurposeFacts := previous == nil || !reflect.DeepEqual(a.Facts.Coverage, previous.Facts.Coverage) ||
		!reflect.DeepEqual(a.Facts.Focus, previous.Facts.Focus) || !reflect.DeepEqual(a.Facts.RemainingFocuses, previous.Facts.RemainingFocuses) ||
		!reflect.DeepEqual(a.Facts.CompletedFocuses, previous.Facts.CompletedFocuses) || !reflect.DeepEqual(a.Facts.RepairDirective, previous.Facts.RepairDirective)
	if changedPurposeFacts || changedSemanticPlanning || changedCandidateRequestSessions || changedCandidateAttempts || changedActiveDispatch || changedCompletedDispatches || changedRepairSurfaceConfirmations || changedOperationReceipts {
		if err := validateRuntimeSemanticBindingPurposeInvariantInGate(a, gate); err != nil {
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

	if changedFacts || changedDecision || changedReconciliation {
		if err := validateRuntimeApplicabilityPrerequisiteProjection(a); err != nil {
			return err
		}
	}
	if terminal := a.Decision.Terminal; terminal != nil && (terminal.OriginalDiagnostic != nil || (terminal.Code == "owner_fact_convergence_exhausted" && missingOwnerFactIsUnavailable(a.Facts))) {
		if terminal.Code != "owner_fact_convergence_exhausted" || !reflect.DeepEqual(terminal.OriginalDiagnostic, missingOwnerFactDiagnostic(a.Facts)) {
			return fmt.Errorf("runtime terminal owner cause does not match exact-current missing owner fact")
		}
	}
	if changedCandidateAttempts || changedCandidateRequestSessions {
		if err := validateRuntimeCandidateAttemptSessionBindings(a.CandidateAttempts, a.CandidateRequestSessions); err != nil {
			return err
		}
	}
	if changedFacts || changedCandidateAttempts || changedCandidateRequestSessions {
		if err := validateRuntimeCandidateFeasibilityBindings(a); err != nil {
			return err
		}
	}
	if changedFacts || changedDecision || changedRecoverableWaits || changedExecutionBudget || changedTermination {
		if err := validateRuntimeAggregateRecoverableWaits(a); err != nil {
			return err
		}
	}
	if changedFacts || changedSemanticPlanning || changedGovernanceResolutions || changedDecision {
		if err := validateRuntimeInputReviewAggregate(a); err != nil {
			return err
		}
	}
	if err := validateRuntimeReceiptJoinTransition(previous, a); err != nil {
		return err
	}
	if changedFacts || changedDefinition || changedActiveDispatch || changedCompletedDispatches || changedOperationReceipts || changedRepairSurfaceConfirmations || changedSemanticPlanning || changedCoverageProofLineage || changedGovernanceResolutions || changedCandidateRequestSessions || changedCandidateAttempts {
		if err := validateRuntimeAggregateRepairJoins(a); err != nil {
			return err
		}
	}
	return nil
}

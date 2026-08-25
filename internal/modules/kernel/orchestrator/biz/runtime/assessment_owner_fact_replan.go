package runtime

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const (
	runtimeCWIReopenReasonOwnerFactPostcondition      = "assessment_owner_fact_postcondition_unmet"
	runtimeCWIReopenReasonOwnerFactAcquisitionSettled = "assessment_owner_fact_acquisition_settled"
	runtimeOwnerFactAcquisitionAttemptSignaturePrefix = "owner_fact_acquisition_attempt/"
)

// currentRuntimeSemanticOwnerFactAcquisition identifies the intermediate
// obligation sealed for the current binding. The immutable Definition proof
// remains in DefinitionFocuses; this value only describes the current semantic
// generation and therefore cannot grant terminal coverage.
func currentRuntimeSemanticOwnerFactAcquisition(aggregate RuntimeAggregate) (runtimecontract.RuntimeSemanticOwnerFactAcquisition, bool, error) {
	ledger := aggregate.SemanticPlanning.Normalized()
	focus := aggregate.Facts.Focus.Normalized()
	if ledger.Phase != runtimecontract.RuntimeSemanticPlanningSealed || ledger.Manifest == nil || len(focus.CoverageWorkItemIDs) != 1 {
		return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, nil
	}
	coverageID := focus.CoverageWorkItemIDs[0]
	index := runtimeCWICoverageIndex(aggregate.Facts.Coverage, coverageID)
	if index < 0 || aggregate.Facts.Coverage[index].CurrentBindingRef == nil {
		return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, nil
	}
	if aggregate.Facts.Coverage[index].CurrentBindingRef.SemanticManifestHash != ledger.Manifest.Hash {
		return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, fmt.Errorf("%w: current CWI binding does not belong to the sealed semantic manifest", common.ErrConflict)
	}
	for _, obligation := range ledger.Manifest.ProofObligations {
		if obligation.CoverageWorkItemID != coverageID || obligation.OwnerFactAcquisition == nil {
			continue
		}
		evidenceObligation, err := runtimecontract.ResolveRuntimeSemanticEvidenceObligation(obligation)
		if err != nil {
			return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, err
		}
		if evidenceObligation.Purpose != runtimecontract.RuntimeSemanticEvidenceObligationOwnerFactAcquisition {
			return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, fmt.Errorf("%w: semantic owner-fact acquisition has no acquisition evidence obligation", common.ErrConflict)
		}
		return obligation.OwnerFactAcquisition.Clone(), true, nil
	}
	return runtimecontract.RuntimeSemanticOwnerFactAcquisition{}, false, nil
}

// retainRuntimeOwnerFactAcquisitionCoverageOpen prevents current-generation
// state-change evidence from being misclassified as terminal Definition proof.
// Evidence and assessment lineage remain attached to the row; only terminal
// credit is withheld until the successor generation proves the original kinds.
func retainRuntimeOwnerFactAcquisitionCoverageOpen(coverage []runtimecontract.RuntimeCoverageItemFact, focus runtimecontract.CoverageFocus) ([]runtimecontract.RuntimeCoverageItemFact, error) {
	focus = focus.Normalized()
	if len(focus.CoverageWorkItemIDs) != 1 {
		return nil, fmt.Errorf("%w: owner-fact acquisition settlement requires one current coverage item", common.ErrConflict)
	}
	coverageID := focus.CoverageWorkItemIDs[0]
	ret := append([]runtimecontract.RuntimeCoverageItemFact(nil), coverage...)
	found := false
	for index := range ret {
		if ret[index].ID != coverageID {
			continue
		}
		found = true
		if ret[index].State != runtimecontract.RuntimeCoverageItemStateSatisfied || ret[index].CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound {
			return nil, fmt.Errorf("%w: owner-fact acquisition did not establish its current bound proof", common.ErrConflict)
		}
		ret[index].State = runtimecontract.RuntimeCoverageItemStateOpen
	}
	if !found {
		return nil, fmt.Errorf("%w: owner-fact acquisition references unknown coverage %q", common.ErrConflict, coverageID)
	}
	return runtimecontract.NormalizeRuntimeCoverageFacts(ret), nil
}

// settleRuntimeOwnerFactAcquisition is an SM-2-local transition invoked only
// by the owner-private AssessmentObserved route. It records the one bounded
// acquisition attempt, retires that semantic generation, and reopens the same
// CWI for its still-immutable terminal Definition proof.
func settleRuntimeOwnerFactAcquisition(aggregate *RuntimeAggregate, assessmentID string, acquisition runtimecontract.RuntimeSemanticOwnerFactAcquisition) (bool, error) {
	if aggregate == nil || strings.TrimSpace(assessmentID) == "" {
		return false, common.ErrInvalidArgument
	}
	acquisition = acquisition.Normalized()
	if err := acquisition.Validate(); err != nil {
		return false, err
	}
	if aggregate.ActiveDispatch != nil {
		return false, fmt.Errorf("%w: owner-fact acquisition settlement cannot retain an active dispatch", common.ErrConflict)
	}
	if !canonicalAssessmentProofCompleteWithoutOwnerFacts(*aggregate) ||
		len(runtimecontract.RuntimeMissingOwnerFactRequirements(aggregate.Facts.Focus.RequiredOwnerFacts, aggregate.Facts.Resources.OperationAdmissibilities)) != 0 {
		return false, fmt.Errorf("%w: owner-fact acquisition settlement requires proof and the re-observed owner fact", common.ErrConflict)
	}
	if runtimeOwnerFactAcquisitionAttemptRecorded(aggregate.Facts, acquisition.Requirement, acquisition.AcquisitionOperationKind) {
		return false, fmt.Errorf("%w: owner-fact acquisition attempt was already consumed", common.ErrConflict)
	}
	carriedRepair, recorded := recordAcceptedOwnerFactAcquisitionShapes(*aggregate, aggregate.Facts.Repair, acquisition)
	if !recorded {
		return false, fmt.Errorf("%w: owner-fact acquisition settlement has no accepted acquisition operation", common.ErrConflict)
	}
	return reopenRuntimeCWIForOwnerFactAssessment(aggregate, assessmentID, runtimeCWIReopenReasonOwnerFactAcquisitionSettled, carriedRepair)
}

func recordAcceptedOwnerFactAcquisitionShapes(aggregate RuntimeAggregate, ledger runtimecontract.RuntimeRepairLedger, acquisition runtimecontract.RuntimeSemanticOwnerFactAcquisition) (runtimecontract.RuntimeRepairLedger, bool) {
	recorded := false
	signature := runtimeOwnerFactAcquisitionFailureSignature(acquisition.Requirement)
	for _, evidence := range aggregate.Facts.Evidence.Accepted {
		if evidence.Kind != runtimecontract.EvidenceKindStateChange ||
			!runtimeEvidenceAppliesToCurrentCoverageWithLineage(aggregate.Facts, evidence, aggregate.CoverageProofLineage) {
			continue
		}
		operation, found := runtimeOperationForEvidence(aggregate, evidence)
		if !found || runtimecontract.CanonicalRuntimeBlockKind(operation.Kind) != acquisition.AcquisitionOperationKind {
			continue
		}
		ledger = recordNoProgressOperationShape(ledger, runtimecontract.RuntimeRejectedOperationShape{
			Operation:        operation,
			EvidenceID:       evidence.ID,
			FailureClass:     runtimecontract.FailureClassCoverageAssessmentIncomplete,
			FailureCode:      runtimeCWIReopenReasonOwnerFactAcquisitionSettled,
			FailureSignature: signature,
			FailureMessage:   "accepted operation settled the bounded owner-fact acquisition but is not terminal Definition proof",
		})
		recorded = true
	}
	return ledger, recorded
}

// replanRuntimeCWIForUnmetOwnerFacts is an SM-2-local transition after an
// assessment dispatch has been committed. The capability owner has already
// published its current catalog; this reducer only records that the sealed
// proof did not establish the profile-declared postcondition and reopens the
// same CWI for a new semantic generation. No capability or Run state is
// mutated here.
func replanRuntimeCWIForUnmetOwnerFacts(aggregate *RuntimeAggregate, assessmentID string) (bool, error) {
	if aggregate == nil || strings.TrimSpace(assessmentID) == "" {
		return false, common.ErrInvalidArgument
	}
	if aggregate.ActiveDispatch != nil {
		return false, fmt.Errorf("%w: owner-fact CWI replan cannot retain an active dispatch", common.ErrConflict)
	}
	facts := aggregate.Facts.Normalized()
	if err := requireCurrentRuntimeCWILedger(facts); err != nil {
		return false, err
	}
	if !canonicalAssessmentProofCompleteWithoutOwnerFacts(*aggregate) {
		return false, nil
	}
	if len(runtimecontract.RuntimeMissingOwnerFactRequirements(facts.Focus.RequiredOwnerFacts, facts.Resources.OperationAdmissibilities)) == 0 {
		return false, nil
	}
	acquisition, acquisitionReady := runtimecontract.ResolveRuntimeOwnerFactAcquisition(facts.Focus.RequiredOwnerFacts, facts.Focus.OwnerFactConvergences, facts.Resources.OperationAdmissibilities)
	if !acquisitionReady || runtimeOwnerFactReplanBudgetExhausted(facts, acquisition) {
		return false, nil
	}
	// Account the operation that produced the just-committed assessment before
	// deciding whether another semantic generation is authorized. The sealed
	// max is an acquisition-attempt bound, not a count of provider replans.
	carriedRepair := recordAcceptedNoProgressShapes(*aggregate, facts.Repair, runtimeOwnerFactAcquisitionFailureSignature(acquisition.Requirement))
	factsWithCurrentAttempt := facts
	factsWithCurrentAttempt.Repair = carriedRepair
	if runtimeOwnerFactReplanBudgetExhausted(factsWithCurrentAttempt, acquisition) {
		aggregate.Facts.Repair = carriedRepair
		return false, nil
	}
	return reopenRuntimeCWIForOwnerFactAssessment(aggregate, assessmentID, runtimeCWIReopenReasonOwnerFactPostcondition, carriedRepair)
}

func reopenRuntimeCWIForOwnerFactAssessment(aggregate *RuntimeAggregate, assessmentID, reason string, carriedRepair runtimecontract.RuntimeRepairLedger) (bool, error) {
	facts := aggregate.Facts.Normalized()
	if len(facts.Focus.CoverageWorkItemIDs) != 1 {
		return false, fmt.Errorf("%w: owner-fact CWI replan requires one current coverage item", common.ErrConflict)
	}
	coverageID := facts.Focus.CoverageWorkItemIDs[0]
	index := runtimeCWICoverageIndex(facts.Coverage, coverageID)
	if index < 0 {
		return false, fmt.Errorf("%w: owner-fact CWI replan references unknown coverage %q", common.ErrConflict, coverageID)
	}
	item := &facts.Coverage[index]
	if item.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound || item.CurrentBindingRef == nil ||
		item.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
		return false, fmt.Errorf("%w: owner-fact CWI replan requires a current unsatisfied binding", common.ErrConflict)
	}
	lineage := runtimecontract.RuntimeCWIReopenLineage{
		PreviousBindingRef: item.CurrentBindingRef.Clone(),
		TriggerRef:         strings.TrimSpace(assessmentID),
		Reason:             strings.TrimSpace(reason),
	}.Normalized()
	if err := lineage.Validate(); err != nil {
		return false, err
	}
	if len(item.ReopenLineage) >= runtimecontract.RuntimeCWIReopenLineageLimit {
		return false, fmt.Errorf("runtime CWI reopen lineage exceeds bounded limit %d", runtimecontract.RuntimeCWIReopenLineageLimit)
	}
	// The owner-fact postcondition proves that an accepted state-change shape did
	// not close this CWI. Preserve only those exact shapes across the successor
	// semantic focus; attempts, gap text and directives remain focus-local and
	// are reset by the canonical projection transition.
	item.ReopenLineage = append(item.ReopenLineage, lineage)
	item.CurrentBindingRef = nil
	item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateReopened
	item.State = runtimecontract.RuntimeCoverageItemStateOpen

	updated, err := replaceRuntimeCWIFocusProjection(facts, []common.CoverageWorkItemID{coverageID})
	if err != nil {
		return false, err
	}
	updated.Repair.FocusID = updated.Focus.ID
	updated.Repair.SliceID = updated.Focus.SliceID
	updated.Repair.NoProgressOperationShapes = append([]runtimecontract.RuntimeRejectedOperationShape(nil), carriedRepair.NoProgressOperationShapes...)
	updated.Repair = updated.Repair.Normalized()
	contract, err := canonicalRuntimeCWIReconciliationContract(updated)
	if err != nil {
		return false, err
	}
	updated.ReconciliationContract = contract
	aggregate.Facts = updated
	if _, err := supersedeActiveReconciliationForCurrentContract(aggregate); err != nil {
		return false, err
	}
	if err := retireSemanticPlanningGenerationForCWIReopen(aggregate, []common.CoverageWorkItemID{coverageID}); err != nil {
		return false, err
	}
	return true, nil
}

func runtimeOwnerFactReplanBudgetExhausted(facts runtimecontract.RuntimeFacts, acquisition runtimecontract.RuntimeOwnerFactConvergence) bool {
	facts = facts.Normalized()
	acquisition = acquisition.Normalized()
	if acquisition.Validate() != nil || acquisition.RequirePreexisting || acquisition.MaxAcquisitionAttempts != 1 || len(facts.Focus.CoverageWorkItemIDs) != 1 {
		return true
	}
	return runtimeOwnerFactAcquisitionAttemptRecorded(facts, acquisition.Requirement, acquisition.AcquisitionOperationKind)
}

func runtimeOwnerFactAcquisitionAttemptRecorded(facts runtimecontract.RuntimeFacts, requirement runtimecontract.RuntimeOwnerFactRequirement, operationKind string) bool {
	facts = facts.Normalized()
	signature := runtimeOwnerFactAcquisitionFailureSignature(requirement)
	operationKind = runtimecontract.CanonicalRuntimeBlockKind(operationKind)
	for _, shape := range facts.Repair.NoProgressOperationShapes {
		if runtimecontract.CanonicalRuntimeBlockKind(shape.Operation.Kind) == operationKind && strings.TrimSpace(shape.FailureSignature) == signature {
			return true
		}
	}
	return false
}

func runtimeOwnerFactAcquisitionFailureSignature(requirement runtimecontract.RuntimeOwnerFactRequirement) string {
	return runtimeOwnerFactAcquisitionAttemptSignaturePrefix + runtimecontract.RuntimeOwnerFactRequirementRef(requirement)
}

package runtime

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeCWIReopenReasonOwnerFactPostcondition = "assessment_owner_fact_postcondition_unmet"

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
		Reason:             runtimeCWIReopenReasonOwnerFactPostcondition,
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
	signature := runtimeOwnerFactAcquisitionFailureSignature(acquisition.Requirement)
	for _, shape := range facts.Repair.NoProgressOperationShapes {
		operation := shape.Operation.Normalized()
		failureSignature := strings.TrimSpace(shape.FailureSignature)
		if runtimecontract.CanonicalRuntimeBlockKind(operation.Kind) == acquisition.AcquisitionOperationKind &&
			(failureSignature == signature || failureSignature == runtimeCWIReopenReasonOwnerFactPostcondition) {
			return true
		}
	}
	return false
}

func runtimeOwnerFactAcquisitionFailureSignature(requirement runtimecontract.RuntimeOwnerFactRequirement) string {
	return runtimeCWIReopenReasonOwnerFactPostcondition + "/" + runtimecontract.RuntimeOwnerFactRequirementRef(requirement)
}

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
	if runtimeOwnerFactReplanBudgetExhausted(facts) {
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
	item.ReopenLineage = append(item.ReopenLineage, lineage)
	item.CurrentBindingRef = nil
	item.CWIConvergenceState = runtimecontract.RuntimeCWIConvergenceStateReopened
	item.State = runtimecontract.RuntimeCoverageItemStateOpen

	updated, err := replaceRuntimeCWIFocusProjection(facts, []common.CoverageWorkItemID{coverageID})
	if err != nil {
		return false, err
	}
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

func runtimeOwnerFactReplanBudgetExhausted(facts runtimecontract.RuntimeFacts) bool {
	facts = facts.Normalized()
	policy := facts.ConvergencePolicy.Normalized()
	if len(facts.Focus.CoverageWorkItemIDs) != 1 {
		return true
	}
	coverageID := facts.Focus.CoverageWorkItemIDs[0]
	for _, item := range facts.Coverage {
		if item.ID != coverageID {
			continue
		}
		count := 0
		for _, lineage := range item.ReopenLineage {
			if lineage.Normalized().Reason == runtimeCWIReopenReasonOwnerFactPostcondition {
				count++
			}
		}
		return count >= policy.MaxRepairAttemptsPerFocus
	}
	return true
}

package runtime

import (
	"fmt"
	"reflect"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const runtimeCWIRevalidationSourceUnavailable = "cwi_revalidation_source_unavailable"

// suspendRuntimeSuccessorForStalePredecessor is an SM-2 safe-window projection.
// The private receipt/observation CAS is its only producer; external callers
// cannot select a predecessor or supply a frozen focus. Unsupported sources
// become an explicit failed outcome, never another effect/candidate request.
func suspendRuntimeSuccessorForStalePredecessor(a *RuntimeAggregate) (bool, error) {
	if a == nil {
		return false, common.ErrInvalidArgument
	}
	if a.ActiveDispatch != nil || runtimeApplicabilityHasUnsettledProvider(*a) || a.Termination != nil || a.TerminalMaterialSeal != nil || isTerminalConvergenceState(a.Decision.State) {
		return false, nil
	}
	for _, s := range a.Facts.CWISuspensions {
		if s.ResumeBinding == nil {
			return false, nil
		}
	}
	if len(a.Facts.Focus.CoverageWorkItemIDs) != 1 {
		return false, nil
	}
	currentID := a.Facts.Focus.CoverageWorkItemIDs[0]
	rows := runtimeCoverageByID(a.Facts.Coverage)
	current := rows[currentID]
	var blocked []runtimecontract.RuntimeCoverageItemFact
	for _, id := range current.PrerequisiteCoverageWorkItemIDs {
		row, found := rows[id]
		if !found {
			return false, fmt.Errorf("%w: suspension prerequisite missing", common.ErrConflict)
		}
		if !runtimecontract.RuntimeCWIPrerequisiteReadyForConsumer(a.Facts, row, currentID) {
			blocked = append(blocked, row)
		}
	}
	if len(blocked) == 0 {
		return false, nil
	}
	// A generic unresolved prerequisite is not permission to replace focus.
	// Only an original stale owner observation of a settled predecessor admits
	// this narrow lane; other reopen causes retain their existing contracts.
	for _, row := range blocked {
		if row.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateReopened || len(row.ReopenLineage) == 0 {
			return false, nil
		}
		cause := row.ReopenLineage[len(row.ReopenLineage)-1]
		observed := false
		for _, o := range a.Facts.EvidenceApplicabilityObservations {
			observed = observed || o.Request.RequestID == cause.TriggerRef
		}
		if !observed {
			return false, nil
		}
	}
	unavailable := func(reason string) (bool, error) {
		failure := runtimecontract.FailureFact{Class: runtimecontract.FailureClassCandidateInfeasible, Code: runtimeCWIRevalidationSourceUnavailable, Message: reason}
		if a.Facts.Failure != nil {
			failure = a.Facts.Failure.Clone()
			failure.Class, failure.Code, failure.Message = runtimecontract.FailureClassCandidateInfeasible, runtimeCWIRevalidationSourceUnavailable, reason
		}
		if err := replaceRuntimeFailureAuthority(&a.Facts, failure, runtimeFailureAuthorityRetire, nil); err != nil {
			return false, err
		}
		a.Facts.Resources.TargetSelection, a.Facts.Resources.TargetSnapshots = nil, nil
		return true, nil
	}
	if len(blocked) != 1 {
		return unavailable("stale predecessor revalidation requires one exact source; multiple sources cannot be guessed")
	}
	predecessor := blocked[0]
	if !runtimecontract.RuntimeCWISuspensionOrdinaryPredecessorSupported(a.Facts, predecessor.ID) {
		return unavailable(fmt.Sprintf("predecessor %q has no declared validation-only current acceptance source; historical state change cannot be replayed or promoted to current proof", predecessor.ID))
	}
	if current.CurrentBindingRef == nil || current.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateBound {
		return unavailable("stale predecessor revalidation cannot freeze a successor without its original current binding")
	}
	if len(a.Facts.CWISuspensions) >= runtimecontract.RuntimeCWISuspensionLimit {
		return unavailable("bounded CWI suspension history exhausted")
	}
	if a.Facts.Repair.Exhausted {
		return unavailable("CWI suspension cannot reset exhausted repair budget")
	}
	eligible := false
	for _, id := range runtimecontract.RuntimeCWIsEligibleForPlanning(a.Facts) {
		eligible = eligible || id == predecessor.ID
	}
	if !eligible {
		return unavailable("exact stale predecessor is not currently eligible; nested suspension is unavailable")
	}
	var effectID common.EvidenceRecordID
	for _, fact := range a.Facts.Evidence.Accepted {
		if fact.Kind != runtimecontract.EvidenceKindStateChange || !runtimecontract.EvidenceAppliesToCoverageWorkItem(a.Facts.Focus, fact, currentID) {
			continue
		}
		for _, dispatch := range a.CompletedDispatches {
			if dispatch.ID != fact.DispatchID {
				continue
			}
			for _, execution := range dispatch.OperationExecutions {
				if execution.OperationID == fact.OperationID && execution.State == runtimecontract.OperationExecutionEvidenceCommitted && execution.EvidenceFactID == fact.ID && execution.EvidenceFactHash == runtimecontract.StableRuntimeEvidenceFactHash(fact) && runtimecontract.RuntimeEffectIsDurable(execution.ExpectedEffectScope) {
					effectID = fact.ID
				}
			}
		}
	}
	if effectID == "" {
		return unavailable("CWI suspension requires an exact completed durable successor effect")
	}
	for _, dispatch := range a.CompletedDispatches {
		for _, execution := range dispatch.OperationExecutions {
			if execution.State == runtimecontract.OperationExecutionOutcomeUnknown {
				return unavailable("CWI suspension cannot cross unknown execution outcome")
			}
		}
	}
	remaining := make([]runtimecontract.CoverageFocus, 0, len(a.Facts.RemainingFocuses))
	var next runtimecontract.CoverageFocus
	for _, focus := range a.Facts.RemainingFocuses {
		if len(focus.CoverageWorkItemIDs) == 1 && focus.CoverageWorkItemIDs[0] == predecessor.ID {
			if next.ID != "" {
				return false, fmt.Errorf("%w: duplicate suspension predecessor cursor", common.ErrConflict)
			}
			next = focus
		} else {
			remaining = append(remaining, focus)
		}
	}
	if next.ID == "" {
		return false, fmt.Errorf("%w: missing suspension predecessor cursor", common.ErrConflict)
	}
	pause := (runtimecontract.RuntimeCWISuspension{ProtocolRef: runtimecontract.RuntimeCWISuspensionV1, SuspendedAtStateRevision: a.StateRevision + 1,
		FrozenFocus: a.Facts.Focus, FrozenBinding: *current.CurrentBindingRef, EffectEvidenceID: effectID,
		PredecessorID: predecessor.ID, ReopenSource: predecessor.ReopenLineage[len(predecessor.ReopenLineage)-1],
		Repair: a.Facts.Repair, RepairDirective: a.Facts.RepairDirective, Failure: a.Facts.Failure}).CloneRaw()
	pause.FrozenPrerequisites, pause.FrozenObservations = runtimeSuspensionFocusPrerequisites(a.Facts)
	facts := a.Facts.Normalized()
	facts.CWISuspensions = append(facts.CWISuspensions, pause)
	clearRuntimeSuspendedFocusAuthority(&facts)
	remaining = append([]runtimecontract.CoverageFocus{pause.FrozenFocus}, remaining...)
	projected, err := installRuntimeCWIFocusProjection(facts, next, remaining, facts.CompletedFocuses)
	if err != nil {
		return false, err
	}
	a.Facts = projected
	return true, nil
}

func clearRuntimeSuspendedFocusAuthority(facts *runtimecontract.RuntimeFacts) {
	retireFocusScopedPrerequisites(facts, facts.Focus.ID)
	facts.Failure, facts.Feasibility, facts.RepairDirective = nil, nil, nil
	facts.Resources.TargetSelection, facts.Resources.TargetSnapshots, facts.Resources.FailureTarget = nil, nil, nil
}

// resumeRuntimeSuspendedSuccessor runs only after ordinary current settlement
// and fresh applicability sampling. It restores the original successor, not
// its owner catalogs/current qualification, and preserves its original repair
// debt. The completed predecessor remains in the canonical completed queue.
func resumeRuntimeSuspendedSuccessor(facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeFacts, bool, error) {
	facts = facts.Normalized()
	for index, pause := range facts.CWISuspensions {
		if pause.ResumeBinding != nil {
			continue
		}
		rows := runtimeCoverageByID(facts.Coverage)
		predecessor := rows[pause.PredecessorID]
		if predecessor.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateSettled || predecessor.State != runtimecontract.RuntimeCoverageItemStateSatisfied || predecessor.CurrentBindingRef == nil || predecessor.LastAssessmentID == "" {
			return facts, false, nil
		}
		if len(facts.Focus.CoverageWorkItemIDs) != 1 || facts.Focus.CoverageWorkItemIDs[0] != pause.PredecessorID {
			return runtimecontract.RuntimeFacts{}, false, fmt.Errorf("%w: suspension resume cursor drift", common.ErrConflict)
		}
		if len(predecessor.ReopenLineage) == 0 || !reflect.DeepEqual(predecessor.ReopenLineage[len(predecessor.ReopenLineage)-1], pause.ReopenSource) || runtimecontract.SameRuntimeCWIBindingRef(*predecessor.CurrentBindingRef, pause.ReopenSource.PreviousBindingRef) {
			return runtimecontract.RuntimeFacts{}, false, fmt.Errorf("%w: suspension resume source drift", common.ErrConflict)
		}
		current := rows[pause.FrozenFocus.CoverageWorkItemIDs[0]]
		if current.CurrentBindingRef == nil || !runtimecontract.SameRuntimeCWIBindingRef(*current.CurrentBindingRef, pause.FrozenBinding) {
			return runtimecontract.RuntimeFacts{}, false, fmt.Errorf("%w: suspension frozen binding drift", common.ErrConflict)
		}
		for _, id := range current.PrerequisiteCoverageWorkItemIDs {
			if !runtimecontract.RuntimeCWIPrerequisiteReadyForConsumer(facts, rows[id], current.ID) {
				return facts, false, nil
			}
		}
		remaining := make([]runtimecontract.CoverageFocus, 0, len(facts.RemainingFocuses))
		found := 0
		for _, focus := range facts.RemainingFocuses {
			if focus.ID == pause.FrozenFocus.ID {
				if !reflect.DeepEqual(focus, pause.FrozenFocus) {
					return runtimecontract.RuntimeFacts{}, false, common.ErrConflict
				}
				found++
			} else {
				remaining = append(remaining, focus)
			}
		}
		if found != 1 {
			return runtimecontract.RuntimeFacts{}, false, fmt.Errorf("%w: suspension frozen queue missing/duplicate", common.ErrConflict)
		}
		completed := append(append([]runtimecontract.CoverageFocus(nil), facts.CompletedFocuses...), facts.Focus)
		clearRuntimeSuspendedFocusAuthority(&facts)
		// Remove only these exact archived identities from the active retirement
		// index. Their original bodies remain in the immutable pause; no old
		// observation is restored as current, so ordinary preflight reacquires.
		archived := make(map[string]bool)
		for _, prerequisite := range pause.FrozenPrerequisites {
			archived[prerequisite.ID] = true
		}
		retired := facts.Prerequisites.Retired[:0]
		for _, prerequisite := range facts.Prerequisites.Retired {
			if !archived[prerequisite.ID] {
				retired = append(retired, prerequisite)
			}
		}
		facts.Prerequisites.Retired = retired
		observations := facts.Resources.RetiredPrerequisiteObservations[:0]
		for _, observation := range facts.Resources.RetiredPrerequisiteObservations {
			if !archived[observation.PrerequisiteID] {
				observations = append(observations, observation)
			}
		}
		facts.Resources.RetiredPrerequisiteObservations = observations
		ref := predecessor.CurrentBindingRef.Clone()
		facts.CWISuspensions[index].ResumeBinding = &ref
		facts.CWISuspensions[index].ResumeAssessmentID = predecessor.LastAssessmentID
		facts.CWISuspensions[index].ResumeEvidenceRevision = facts.EvidenceRevision
		facts.CWISuspensions[index].ResumeObservationCount = len(facts.EvidenceApplicabilityObservations)
		projected, err := installRuntimeCWIFocusProjection(facts, pause.FrozenFocus, remaining, completed)
		if err != nil {
			return runtimecontract.RuntimeFacts{}, false, err
		}
		projected.Repair, projected.RepairDirective, projected.Failure = pause.Repair, pause.RepairDirective, pause.Failure
		return projected.Normalized(), true, nil
	}
	return facts, false, nil
}

func validateRuntimeCWISuspensionTransition(previous, next RuntimeAggregate) error {
	before, after := previous.Facts.CWISuspensions, next.Facts.CWISuspensions
	if len(after) < len(before) || len(after) > len(before)+1 {
		return fmt.Errorf("%w: CWI suspension history is append-only", common.ErrConflict)
	}
	for i, original := range before {
		if reflect.DeepEqual(original, after[i]) {
			continue
		}
		if original.ResumeBinding != nil || after[i].ResumeBinding == nil {
			return fmt.Errorf("%w: CWI suspension source cannot be rewritten", common.ErrConflict)
		}
		copy := after[i].CloneRaw()
		copy.ResumeBinding, copy.ResumeAssessmentID, copy.ResumeEvidenceRevision, copy.ResumeObservationCount = nil, "", "", 0
		if !reflect.DeepEqual(copy, original) {
			return fmt.Errorf("%w: CWI suspension resume rewrote original source", common.ErrConflict)
		}
		if previous.ActiveDispatch != nil || runtimeApplicabilityHasUnsettledProvider(previous) || previous.Termination != nil || previous.TerminalMaterialSeal != nil || isTerminalConvergenceState(previous.Decision.State) {
			return fmt.Errorf("%w: CWI resume crossed in-flight responsibility", common.ErrConflict)
		}
		if !reflect.DeepEqual(next.Facts.Focus, original.FrozenFocus) || !reflect.DeepEqual(next.Facts.Repair, original.Repair) || !reflect.DeepEqual(next.Facts.RepairDirective, original.RepairDirective) || !reflect.DeepEqual(next.Facts.Failure, original.Failure) {
			return fmt.Errorf("%w: CWI resume lost frozen focus/repair budget", common.ErrConflict)
		}
	}
	if len(after) != len(before) {
		if previous.ActiveDispatch != nil || runtimeApplicabilityHasUnsettledProvider(previous) || previous.Termination != nil || previous.TerminalMaterialSeal != nil || isTerminalConvergenceState(previous.Decision.State) {
			return fmt.Errorf("%w: CWI suspension crossed original unsafe window", common.ErrConflict)
		}
		added := after[len(before)]
		if added.ResumeBinding != nil || added.ResumeAssessmentID != "" || added.ResumeEvidenceRevision != "" || added.ResumeObservationCount != 0 {
			return fmt.Errorf("%w: CWI suspension cannot append a pre-resumed source", common.ErrConflict)
		}
		prerequisites, observations := runtimeSuspensionFocusPrerequisites(previous.Facts)
		if !reflect.DeepEqual(added.FrozenPrerequisites, prerequisites) || !reflect.DeepEqual(added.FrozenObservations, observations) {
			return fmt.Errorf("%w: suspension prerequisite archive differs from original owner preimage", common.ErrConflict)
		}
		if added.SuspendedAtStateRevision != next.StateRevision || !reflect.DeepEqual(added.FrozenFocus, previous.Facts.Focus) || !reflect.DeepEqual(added.Repair, previous.Facts.Repair) || !reflect.DeepEqual(added.RepairDirective, previous.Facts.RepairDirective) || !reflect.DeepEqual(added.Failure, previous.Facts.Failure) {
			return fmt.Errorf("%w: CWI suspension has no exact original focus/repair preimage", common.ErrConflict)
		}
		if !reflect.DeepEqual(previous.CompletedDispatches, next.CompletedDispatches) || !reflect.DeepEqual(previous.OperationReceipts, next.OperationReceipts) || !reflect.DeepEqual(previous.ExecutionBudget, next.ExecutionBudget) {
			return fmt.Errorf("%w: CWI suspension cannot change effects/receipts/budget", common.ErrConflict)
		}
	}
	return nil
}

func runtimeSuspensionFocusPrerequisites(facts runtimecontract.RuntimeFacts) ([]runtimecontract.RuntimePrerequisite, []runtimecontract.RuntimePrerequisiteObservation) {
	// Normalized constructs detached owner facts; original body integrity is
	// checked by the enclosing aggregate before any pause transaction.
	facts = facts.Normalized()
	var prerequisites []runtimecontract.RuntimePrerequisite
	ids := make(map[string]bool)
	for _, prerequisite := range append(facts.Prerequisites.Pending, facts.Prerequisites.Satisfied...) {
		if prerequisite.Scope == runtimecontract.RuntimePrerequisiteScopeFocus && prerequisite.FocusID == facts.Focus.ID {
			prerequisites = append(prerequisites, prerequisite)
			ids[prerequisite.ID] = true
		}
	}
	var observations []runtimecontract.RuntimePrerequisiteObservation
	for _, observation := range facts.Resources.PrerequisiteObservations {
		if ids[observation.PrerequisiteID] {
			observations = append(observations, observation)
		}
	}
	return prerequisites, observations
}

package runtime

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// recordHistoricalBehaviorCompletion is part of the original receipt/evidence
// transaction. Only the complete committed dispatch can create this SM-2 fact;
// no external command or second CAS can turn a partial effect into completion.
func recordHistoricalBehaviorCompletion(aggregate *RuntimeAggregate, dispatch SealedDispatch) error {
	if dispatch.State != DispatchStateCommitted || dispatch.Transaction.State != runtimecontract.ConvergenceStateNeedStateChange {
		return nil
	}
	for _, binding := range dispatch.CoverageBindings {
		if binding.HistoricalAcceptance == nil {
			continue
		}
		completion, err := runtimecontract.CompileRuntimeHistoricalBehaviorCompletion(runtimeLineageAssertionSources(*aggregate), binding.CoverageWorkItemID, dispatch.ID)
		if err != nil {
			return err
		}
		for index := range aggregate.Facts.Coverage {
			row := &aggregate.Facts.Coverage[index]
			if row.ID != binding.CoverageWorkItemID {
				continue
			}
			if row.HistoricalBehavior != nil && !reflect.DeepEqual(row.HistoricalBehavior, completion) {
				return fmt.Errorf("%w: historical behavior already completed under another dispatch", common.ErrConflict)
			}
			row.HistoricalBehavior = completion
		}
	}
	return nil
}

// handoffHistoricalBehaviorFocus retires behavior execution, preserving the
// bound/open predecessor in the same canonical queue. Its completion unlocks
// only the confirmed consumer; other already-eligible work uses ordinary
// prerequisite admission and never borrows historical terminal credit.
// CompletedFocuses means no more behavior execution, not satisfied coverage.
func handoffHistoricalBehaviorFocus(aggregate *RuntimeAggregate) (bool, error) {
	if len(aggregate.Facts.Focus.CoverageWorkItemIDs) != 1 || aggregate.ActiveDispatch != nil {
		return false, nil
	}
	rows := runtimeCoverageByID(aggregate.Facts.Coverage)
	row := rows[aggregate.Facts.Focus.CoverageWorkItemIDs[0]]
	if row.HistoricalBehavior == nil {
		return false, nil
	}
	consumer := row.HistoricalBehavior.Source.AcceptanceCoverageWorkItemID
	eligible := runtimecontract.RuntimeCWIsEligibleForPlanning(aggregate.Facts)
	ready := func(id common.CoverageWorkItemID) bool {
		if slices.Contains(eligible, id) {
			return true
		}
		return aggregate.SemanticPlanning.Manifest != nil &&
			runtimeCWIBoundBySemanticLineage(aggregate.Facts, rows[id], aggregate.SemanticPlanning.Manifest.Hash, rows)
	}
	nextID := consumer
	if !ready(consumer) {
		// In particular, two independent historical sources may share one
		// ordinary consumer. The first real receipt must persist while the
		// second prerequisite executes under its own original admission.
		for _, focus := range aggregate.Facts.RemainingFocuses {
			if len(focus.CoverageWorkItemIDs) == 1 && ready(focus.CoverageWorkItemIDs[0]) {
				nextID = focus.CoverageWorkItemIDs[0]
				break
			}
		}
	}
	var next runtimecontract.CoverageFocus
	remaining := make([]runtimecontract.CoverageFocus, 0, len(aggregate.Facts.RemainingFocuses))
	matches := 0
	for _, focus := range aggregate.Facts.RemainingFocuses {
		if len(focus.CoverageWorkItemIDs) == 1 && focus.CoverageWorkItemIDs[0] == nextID {
			next, matches = focus.Normalized(), matches+1
		} else {
			remaining = append(remaining, focus.Normalized())
		}
	}
	if matches != 1 {
		return false, fmt.Errorf("%w: historical behavior has no unique original acceptance cursor", common.ErrConflict)
	}
	facts := aggregate.Facts
	history := facts.Focus.Normalized()
	retireFocusScopedPrerequisites(&facts, history.ID)
	facts.Failure, facts.Feasibility, facts.RepairDirective = nil, nil, nil
	facts.Resources.TargetSelection, facts.Resources.TargetSnapshots, facts.Resources.FailureTarget = nil, nil, nil
	completed := append([]runtimecontract.CoverageFocus(nil), facts.CompletedFocuses...)
	completed = append(completed, history)
	projected, err := installRuntimeCWIFocusProjection(facts, next, remaining, completed)
	if err != nil {
		return false, err
	}
	aggregate.Facts = projected
	if ready(nextID) {
		if err := prepareSemanticPlanningForAdvancedFocus(aggregate); err != nil {
			return false, err
		}
	}
	return true, nil
}

func validateHistoricalBehaviorTransition(previous, next RuntimeAggregate) error {
	old := runtimeCoverageByID(previous.Facts.Coverage)
	for _, row := range next.Facts.Coverage {
		before, found := old[row.ID]
		if !found {
			return fmt.Errorf("%w: historical behavior transition changed canonical coverage inventory", common.ErrConflict)
		}
		if len(row.HistoricalAcceptanceProofs) < len(before.HistoricalAcceptanceProofs) ||
			(len(before.HistoricalAcceptanceProofs) != 0 && !reflect.DeepEqual(row.HistoricalAcceptanceProofs[:len(before.HistoricalAcceptanceProofs)], before.HistoricalAcceptanceProofs)) {
			return fmt.Errorf("%w: historical acceptance proof history is append-only", common.ErrConflict)
		}
		if len(row.HistoricalAcceptanceProofs) > len(before.HistoricalAcceptanceProofs) {
			if len(row.HistoricalAcceptanceProofs) != len(before.HistoricalAcceptanceProofs)+1 || previous.ActiveDispatch == nil || previous.ActiveDispatch.Candidate.Assessment == nil || next.ActiveDispatch != nil ||
				row.HistoricalAcceptanceProofs[len(row.HistoricalAcceptanceProofs)-1].AssessmentDispatchID != previous.ActiveDispatch.ID {
				return fmt.Errorf("%w: historical acceptance must share original ordinary assessment CAS", common.ErrConflict)
			}
		}
		if before.HistoricalBehavior != nil {
			if !reflect.DeepEqual(before.HistoricalBehavior, row.HistoricalBehavior) {
				return fmt.Errorf("%w: historical behavior completion is immutable", common.ErrConflict)
			}
			continue
		}
		if row.HistoricalBehavior == nil {
			continue
		}
		if previous.ActiveDispatch == nil || previous.ActiveDispatch.ID != row.HistoricalBehavior.DispatchID || next.ActiveDispatch != nil {
			return fmt.Errorf("%w: historical completion must share original complete receipt CAS", common.ErrConflict)
		}
	}
	return nil
}

// settleHistoricalAcceptanceConsumers executes no owner operation. In the
// same original assessment CAS, it compiles the independent all-of source and
// only then grants canonical terminal credit to each confirmed predecessor.
func settleHistoricalAcceptanceConsumers(aggregate *RuntimeAggregate, assessmentID string) error {
	for index := range aggregate.Facts.Coverage {
		row := &aggregate.Facts.Coverage[index]
		if row.HistoricalBehavior == nil || row.State == runtimecontract.RuntimeCoverageItemStateSatisfied {
			continue
		}
		consumer := runtimeCoverageByID(aggregate.Facts.Coverage)[row.HistoricalBehavior.Source.AcceptanceCoverageWorkItemID]
		if consumer.LastAssessmentID != assessmentID || consumer.State != runtimecontract.RuntimeCoverageItemStateSatisfied || consumer.CWIConvergenceState != runtimecontract.RuntimeCWIConvergenceStateSettled {
			continue
		}
		proof, err := runtimecontract.CompileRuntimeHistoricalAcceptanceProof(runtimeLineageAssertionSources(*aggregate), row.ID, assessmentID)
		if err != nil {
			return err
		}
		if len(row.HistoricalAcceptanceProofs) >= runtimecontract.RuntimeHistoricalAcceptanceProofLimit {
			return fmt.Errorf("%w: historical acceptance proof history exceeds bound", common.ErrConflict)
		}
		row.HistoricalAcceptanceProofs = append(row.HistoricalAcceptanceProofs, proof)
		row.State, row.CWIConvergenceState = runtimecontract.RuntimeCoverageItemStateSatisfied, runtimecontract.RuntimeCWIConvergenceStateSettled
		row.LastAssessmentID = assessmentID
		row.EvidenceRefs = normalizeStringSet(append(append([]string(nil), proof.BehaviorEvidenceRefs...), proof.AcceptanceEvidenceRefs...))
	}
	return nil
}

// revokeHistoricalAcceptanceCredit retains original effects and all previous
// settlements. Only the ordinary consumer is reopened/rebound; create-only
// history stays outside the execution queue and never regains old credit.
func revokeHistoricalAcceptanceCredit(facts *runtimecontract.RuntimeFacts) {
	rows := runtimeCoverageByID(facts.Coverage)
	for index := range facts.Coverage {
		row := &facts.Coverage[index]
		if row.HistoricalBehavior == nil || row.State != runtimecontract.RuntimeCoverageItemStateSatisfied || len(row.HistoricalAcceptanceProofs) == 0 {
			continue
		}
		proof := row.HistoricalAcceptanceProofs[len(row.HistoricalAcceptanceProofs)-1]
		consumer := rows[proof.Source.AcceptanceCoverageWorkItemID]
		if consumer.State == runtimecontract.RuntimeCoverageItemStateSatisfied && consumer.CWIConvergenceState == runtimecontract.RuntimeCWIConvergenceStateSettled && consumer.CurrentBindingRef != nil &&
			runtimecontract.SameRuntimeCWIBindingRef(*consumer.CurrentBindingRef, proof.AcceptanceBindingRef) && consumer.LastAssessmentID == proof.AssessmentDispatchID {
			continue
		}
		row.State, row.CWIConvergenceState = runtimecontract.RuntimeCoverageItemStateOpen, runtimecontract.RuntimeCWIConvergenceStateBound
		row.LastAssessmentID, row.EvidenceRefs = "", nil
	}
}

package runtime

import (
	"github.com/muidea/workorch/internal/pkg/common"
	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// The current execution focus is an existing SM-2 choice, not a new scheduler.
// When its complete member set is ready, a new ordinary generation uses the
// ready members of its declared acceptance slice (the existing request group).
// Slice priority/identity never chooses the focus or changes execution rank.
// All original per-CWI obligations remain in the manifest; unrelated
// ready branches remain in the canonical ledger/queue for a later generation.
// A blocked or partially ready focus cannot certify a smaller complete task:
// leave the existing full ready-set projection to the ordinary owner path.
func runtimeSemanticPlanningFrontier(facts rc.RuntimeFacts) []common.CoverageWorkItemID {
	eligible := rc.RuntimeCWIsEligibleForPlanning(facts)
	ready := make(map[common.CoverageWorkItemID]bool, len(eligible))
	for _, id := range eligible {
		ready[id] = true
	}
	members := facts.Focus.CoverageWorkItemIDs
	if len(members) == 0 {
		return eligible
	}
	seen := make(map[common.CoverageWorkItemID]bool, len(members))
	for _, id := range members {
		if !ready[id] || seen[id] {
			return eligible
		}
		seen[id] = true
	}
	for _, slice := range facts.Slices {
		if slice.ID != facts.Focus.SliceID {
			continue
		}
		group := make(map[common.CoverageWorkItemID]bool, len(slice.CoverageWorkItemIDs))
		for _, id := range slice.CoverageWorkItemIDs {
			group[id] = true
		}
		for _, id := range members {
			if !group[id] {
				return eligible
			}
		}
		var frontier []common.CoverageWorkItemID
		for _, id := range eligible {
			if group[id] {
				frontier = append(frontier, id)
			}
		}
		return frontier
	}
	return append([]common.CoverageWorkItemID(nil), members...)
}

// A sealed ordinary frontier remains scoped while any member is planning or
// bound. Once it is completely consumed, current eligibility may advance to a
// different branch. This never changes an already issued manifest or dispatch.
func runtimeOrdinarySemanticEligibleClosure(facts rc.RuntimeFacts, manifest rc.RuntimeSemanticPlanManifest, eligible []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	members := make(map[common.CoverageWorkItemID]bool, len(manifest.CoverageItems))
	for _, item := range manifest.CoverageItems {
		members[item.ID] = true
	}
	var scoped []common.CoverageWorkItemID
	for _, id := range eligible {
		if members[id] {
			scoped = append(scoped, id)
		}
	}
	if len(scoped) != 0 {
		return scoped
	}
	for _, row := range facts.Coverage {
		if members[row.ID] && row.CWIConvergenceState == rc.RuntimeCWIConvergenceStateBound && row.HistoricalBehavior == nil {
			return nil
		}
	}
	return runtimeSemanticPlanningFrontier(facts)
}

func runtimeSemanticCompletedFrontierRefs(a RuntimeAggregate) []string {
	l := a.SemanticPlanning
	if l.Phase != rc.RuntimeSemanticPlanningSealed || l.Manifest == nil || len(l.Manifest.CoverageItems) == 0 {
		return nil
	}
	byID := runtimeCoverageByID(a.Facts.Coverage)
	var refs []string
	for _, item := range l.Manifest.CoverageItems {
		row, found := byID[item.ID]
		if !found || row.CurrentBindingRef == nil || row.CurrentBindingRef.SemanticManifestHash != l.Manifest.Hash {
			return nil
		}
		settled := row.CWIConvergenceState == rc.RuntimeCWIConvergenceStateSettled && row.State == rc.RuntimeCoverageItemStateSatisfied
		completedEffect := false
		if row.HistoricalBehavior != nil {
			for _, focus := range a.Facts.CompletedFocuses {
				if focus.ID == row.FocusID && len(focus.CoverageWorkItemIDs) == 1 && focus.CoverageWorkItemIDs[0] == row.ID {
					completedEffect = true
				}
			}
		}
		if !settled && !completedEffect {
			return nil
		}
		refs = append(refs, row.CurrentBindingRef.BindingID)
	}
	return refs
}

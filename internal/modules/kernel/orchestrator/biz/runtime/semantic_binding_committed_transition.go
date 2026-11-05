package runtime

import (
	"reflect"

	rc "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Binding consumers read the formal projection while its exact inputs remain
// the same. Request progress, receipts and unrelated facts are not new semantic
// binding tasks. New/changed projections go through the complete source gate.
// The commit boundary separately preserves original accepted semantic sources.
func runtimeSemanticBindingProjectionsUnchanged(previous *RuntimeAggregate, current RuntimeAggregate) bool {
	if previous == nil {
		return false
	}
	before, after := previous.Facts, current.Facts
	if before.WorkspaceID != after.WorkspaceID || before.RunID != after.RunID || before.DefinitionRevision != after.DefinitionRevision ||
		!reflect.DeepEqual(before.DefinitionSeed, after.DefinitionSeed) || !reflect.DeepEqual(before.DefinitionFocuses, after.DefinitionFocuses) ||
		!reflect.DeepEqual(before.Focus, after.Focus) || !reflect.DeepEqual(before.RemainingFocuses, after.RemainingFocuses) || !reflect.DeepEqual(before.CompletedFocuses, after.CompletedFocuses) ||
		!reflect.DeepEqual(before.CWISuspensions, after.CWISuspensions) || !reflect.DeepEqual(before.ReconciliationContract, after.ReconciliationContract) {
		return false
	}
	if len(before.Coverage) != len(after.Coverage) {
		return false
	}
	for i, row := range after.Coverage {
		prior := before.Coverage[i]
		if prior.ID != row.ID || !reflect.DeepEqual(prior.CurrentBindingRef, row.CurrentBindingRef) || !reflect.DeepEqual(prior.BindingLineage, row.BindingLineage) {
			return false
		}
	}
	// New candidate/dispatch projections may consume the same owner-committed
	// binding. They cannot introduce another binding by retaining only its hash.
	known := map[string][]rc.CoverageEvidenceBinding{}
	for _, f := range append(append([]rc.CoverageFocus{before.Focus}, before.RemainingFocuses...), before.CompletedFocuses...) {
		if old, found := known[f.ID]; found && !reflect.DeepEqual(old, f.CoverageEvidenceBindings) {
			return false
		}
		known[f.ID] = f.CoverageEvidenceBindings
	}
	matches := func(id string, bindings []rc.CoverageEvidenceBinding) bool {
		original, found := known[id]
		return found && reflect.DeepEqual(original, bindings)
	}
	for _, s := range current.CandidateRequestSessions {
		if s.CandidateManifest != nil && runtimeSemanticBindingProjectionState(s.CandidateManifest.State) && !matches(s.Transaction.FocusID, s.CandidateManifest.Bindings) {
			found := false
			for _, old := range previous.CandidateRequestSessions {
				if reflect.DeepEqual(old, s) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	for _, a := range current.CandidateAttempts {
		if a.State == rc.RuntimeCandidateAttemptStateAccepted && runtimeSemanticBindingProjectionState(a.Transaction.State) && !matches(a.Transaction.FocusID, a.AdmissionCoverageBindings) {
			found := false
			for _, old := range previous.CandidateAttempts {
				if reflect.DeepEqual(old, a) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	dispatchMatches := func(d rc.RuntimeDispatchRecord) bool {
		if !runtimeSemanticBindingProjectionState(d.Transaction.State) || matches(d.Transaction.FocusID, d.CoverageBindings) {
			return true
		}
		if previous.ActiveDispatch != nil && previous.ActiveDispatch.ID == d.ID && reflect.DeepEqual(previous.ActiveDispatch.CoverageBindings, d.CoverageBindings) && reflect.DeepEqual(previous.ActiveDispatch.Transaction, d.Transaction) {
			return true
		}
		for _, old := range previous.CompletedDispatches {
			if old.ID == d.ID && reflect.DeepEqual(old.Transaction, d.Transaction) && reflect.DeepEqual(old.CoverageBindings, d.CoverageBindings) {
				return true
			}
		}
		return false
	}
	if current.ActiveDispatch != nil && !dispatchMatches(*current.ActiveDispatch) {
		return false
	}
	for _, d := range current.CompletedDispatches {
		if !dispatchMatches(d) {
			return false
		}
	}
	return true
}

package runtime

import (
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
		!rc.RuntimeRawEqual(before.DefinitionSeed, after.DefinitionSeed) || !rc.RuntimeRawEqual(before.DefinitionFocuses, after.DefinitionFocuses) ||
		!rc.RuntimeRawEqual(before.CWISuspensions, after.CWISuspensions) || !rc.RuntimeRawEqual(before.ReconciliationContract, after.ReconciliationContract) {
		return false
	}
	if len(before.Coverage) != len(after.Coverage) {
		return false
	}
	for i, row := range after.Coverage {
		prior := before.Coverage[i]
		if prior.ID != row.ID || !rc.RuntimeRawEqual(prior.CurrentBindingRef, row.CurrentBindingRef) || !rc.RuntimeRawEqual(prior.BindingLineage, row.BindingLineage) {
			return false
		}
	}
	// New candidate/dispatch projections or pure focus-queue advancements may
	// consume the same owner-committed binding. They cannot introduce another
	// binding without entering the complete source gate.
	known := make(map[string][]rc.CoverageEvidenceBinding, 1+len(before.RemainingFocuses)+len(before.CompletedFocuses))
	recordBeforeFocus := func(f rc.CoverageFocus) bool {
		if old, found := known[f.ID]; found && !rc.RuntimeRawEqual(old, f.CoverageEvidenceBindings) {
			return false
		}
		known[f.ID] = f.CoverageEvidenceBindings
		return true
	}
	if !recordBeforeFocus(before.Focus) {
		return false
	}
	for _, f := range before.RemainingFocuses {
		if !recordBeforeFocus(f) {
			return false
		}
	}
	for _, f := range before.CompletedFocuses {
		if !recordBeforeFocus(f) {
			return false
		}
	}
	matches := func(id string, bindings []rc.CoverageEvidenceBinding) bool {
		original, found := known[id]
		return found && rc.RuntimeRawEqual(original, bindings)
	}
	checkAfterFocus := func(f rc.CoverageFocus) bool {
		if len(f.CoverageEvidenceBindings) == 0 {
			return true
		}
		return matches(f.ID, f.CoverageEvidenceBindings)
	}
	if !checkAfterFocus(after.Focus) {
		return false
	}
	for _, f := range after.RemainingFocuses {
		if !checkAfterFocus(f) {
			return false
		}
	}
	for _, f := range after.CompletedFocuses {
		if !checkAfterFocus(f) {
			return false
		}
	}
	for _, s := range current.CandidateRequestSessions {
		if s.CandidateManifest != nil && runtimeSemanticBindingProjectionState(s.CandidateManifest.State) && !matches(s.Transaction.FocusID, s.CandidateManifest.Bindings) {
			found := false
			for _, old := range previous.CandidateRequestSessions {
				if old.LogicalRequestID == s.LogicalRequestID && rc.RuntimeRawEqual(old, s) {
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
				if old.LogicalRequestID == a.LogicalRequestID && old.Attempt == a.Attempt && rc.RuntimeRawEqual(old, a) {
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
		if previous.ActiveDispatch != nil && previous.ActiveDispatch.ID == d.ID && rc.RuntimeRawEqual(previous.ActiveDispatch.CoverageBindings, d.CoverageBindings) && rc.RuntimeRawEqual(previous.ActiveDispatch.Transaction, d.Transaction) {
			return true
		}
		for _, old := range previous.CompletedDispatches {
			if old.ID == d.ID && rc.RuntimeRawEqual(old.Transaction, d.Transaction) && rc.RuntimeRawEqual(old.CoverageBindings, d.CoverageBindings) {
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

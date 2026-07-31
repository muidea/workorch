package workorchruntimecontract

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// MissingRequiredEvidenceKinds returns the required evidence kinds for which
// the current focus has no accepted proof or diagnostic observation. This
// helper only classifies persisted evidence facts; it does not grant CWI
// coverage or select a Runtime action. The coverage ledger separately requires
// every CWI to be closed by its own exact binding-matched receipt before focus
// advance or terminal completion is possible.
func MissingRequiredEvidenceKinds(facts RuntimeFacts) []string {
	facts = facts.Normalized()
	missing := make([]string, 0, len(facts.Focus.RequiredEvidenceKinds))
	for _, kind := range facts.Focus.RequiredEvidenceKinds {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			continue
		}
		if !currentFocusHasEvidenceKind(facts, kind) {
			missing = append(missing, kind)
		}
	}
	return missing
}

func currentFocusHasEvidenceKind(facts RuntimeFacts, evidenceKind string) bool {
	if len(facts.Focus.CoverageWorkItemIDs) == 0 {
		return false
	}
	for _, evidence := range facts.Evidence.Accepted {
		if strings.TrimSpace(evidence.Kind) != evidenceKind {
			continue
		}
		// A sealed non-credit validation/current-state observation may advance
		// the focus to assessment, where SM-2 retains its original diagnostic
		// but still requires exact binding-derived CWI proof. It must never be
		// mistaken for CWI credit below.
		if len(evidence.CoverageWorkItemIDs) == 0 && EvidenceAppliesToCurrentFocus(facts, evidence) {
			return true
		}
		for _, coverageID := range facts.Focus.CoverageWorkItemIDs {
			if EvidenceAppliesToCoverageWorkItem(facts.Focus, evidence, coverageID) {
				return true
			}
		}
	}
	return false
}

// EvidenceAppliesToCurrentFocus reports whether an evidence fact is scoped to
// the active focus for diagnostic/repair lineage. A non-credit fact without a
// CWI belongs to the final focus only; this supports the bounded
// current-state-validation/provenance path without granting coverage.
//
// This broad scope predicate is also used for rejected-operation and repair
// diagnostics, which have no coverage credit. Coverage-sensitive callers must
// use EvidenceAppliesToCoverageWorkItem so an accepted fact additionally has
// the sealed binding proof required by C12.
func EvidenceAppliesToCurrentFocus(facts RuntimeFacts, evidence EvidenceFact) bool {
	facts = facts.Normalized()
	if len(evidence.CoverageWorkItemIDs) == 0 {
		return len(facts.RemainingFocuses) == 0
	}
	for _, coverageID := range facts.Focus.CoverageWorkItemIDs {
		if coverageWorkItemIDContained(evidence.CoverageWorkItemIDs, coverageID) {
			return true
		}
	}
	return false
}

// EvidenceAppliesToCoverageWorkItem is the pure attribution predicate shared
// by repair/context helpers. In a current-schema focus, a matching CWI ID is
// necessary but not sufficient: the evidence must also name a binding from
// that focus whose proof contains the observed evidence kind. The CWI ID is
// reducer-derived during receipt commit, and the binding ref originates only
// from the sealed operation authorization.
//
// Isolated contract fixtures that do not carry any bindings retain the narrow
// CWI-ID-only predicate. Production activation rejects such a focus before a
// Runtime V2 drive starts, so this is not a current-schema compatibility path.
func EvidenceAppliesToCoverageWorkItem(focus CoverageFocus, evidence EvidenceFact, coverageID common.CoverageWorkItemID) bool {
	focus = focus.Normalized()
	coverageID = common.CoverageWorkItemID(strings.TrimSpace(string(coverageID)))
	if coverageID == "" || !coverageWorkItemIDContained(evidence.CoverageWorkItemIDs, coverageID) {
		return false
	}
	if len(focus.CoverageEvidenceBindings) == 0 {
		return true
	}
	// This branch exists solely for direct in-process test fixtures that model
	// a reducer fact without a persisted operation. Runtime-produced operation
	// evidence always has a dispatch or operation identity, and therefore must
	// name an exact sealed binding below. It is not a current-schema aggregate
	// compatibility path: activation prevents a binding-less focus and SM-2
	// never persists CWI credit from an unbound receipt.
	if strings.TrimSpace(evidence.DispatchID) == "" && strings.TrimSpace(evidence.OperationID) == "" {
		return true
	}
	for _, bindingRef := range evidence.BindingRefs {
		binding, found := CoverageEvidenceBindingForRef(focus.CoverageEvidenceBindings, bindingRef)
		if !found || binding.CoverageWorkItemID != coverageID {
			continue
		}
		if _, proofFound := CoverageEvidenceProofForKind(binding, evidence.Kind); proofFound {
			return true
		}
	}
	return false
}

func coverageWorkItemIDContained(values []common.CoverageWorkItemID, expected common.CoverageWorkItemID) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func containsStringValue(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

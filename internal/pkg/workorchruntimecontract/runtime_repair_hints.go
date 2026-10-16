package workorchruntimecontract

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// MissingRequiredEvidenceKinds returns the required evidence kinds for which
// the current focus has no accepted binding-derived CWI proof. Diagnostic
// observations without reducer-derived CWI credit never satisfy this gate.
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
		if strings.TrimSpace(evidence.Kind) != evidenceKind || !RuntimeEvidenceHasProofApplicabilityAtRevision(evidence, facts.EvidenceApplicabilityObservations, facts.EvidenceRevision) {
			continue
		}
		for _, coverageID := range facts.Focus.CoverageWorkItemIDs {
			if EvidenceAppliesToCoverageWorkItem(facts.Focus, evidence, coverageID) {
				return true
			}
		}
	}
	return false
}

// EvidenceBelongsToCurrentFocusForDiagnosticLineage reports whether an
// evidence fact is scoped to the active focus for diagnostic/repair lineage.
// Ordinary receipt facts carry their reducer-derived owning FocusID, so this
// attribution never guesses from the mutable final/current-focus position.
// Diagnostic attribution never grants coverage, satisfies a required kind,
// or invalidates an assessment.
//
// This broad scope predicate is also used for rejected-operation and repair
// diagnostics, which have no coverage credit. Coverage-sensitive callers must
// use EvidenceAppliesToCoverageWorkItem so an accepted fact additionally has
// the sealed binding proof required by C12.
func EvidenceBelongsToCurrentFocusForDiagnosticLineage(facts RuntimeFacts, evidence EvidenceFact) bool {
	facts = facts.Normalized()
	return evidence.FocusID != "" && evidence.FocusID == facts.Focus.ID
}

// EvidenceAppliesToCoverageWorkItem is the pure attribution predicate shared
// by repair/context helpers. In a current-schema focus, a matching CWI ID is
// necessary but not sufficient: the evidence must also name a binding from
// that focus whose proof contains the observed evidence kind. The CWI ID is
// reducer-derived during receipt commit, and the binding ref originates only
// from the sealed operation authorization.
func EvidenceAppliesToCoverageWorkItem(focus CoverageFocus, evidence EvidenceFact, coverageID common.CoverageWorkItemID) bool {
	focus = focus.Normalized()
	coverageID = common.CoverageWorkItemID(strings.TrimSpace(string(coverageID)))
	if coverageID == "" || !coverageWorkItemIDContained(evidence.CoverageWorkItemIDs, coverageID) {
		return false
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

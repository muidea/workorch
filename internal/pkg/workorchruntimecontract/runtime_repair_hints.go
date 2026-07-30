package workorchruntimecontract

import (
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// MissingRequiredEvidenceKinds returns the required evidence kinds not yet
// attributed to the current focus by the Runtime V2 authority rule.
func MissingRequiredEvidenceKinds(facts RuntimeFacts) []string {
	facts = facts.Normalized()
	accepted := make(map[string]struct{}, len(facts.Evidence.Accepted))
	for _, evidence := range facts.Evidence.Accepted {
		if EvidenceAppliesToCurrentFocus(facts, evidence) {
			accepted[strings.TrimSpace(evidence.Kind)] = struct{}{}
		}
	}
	missing := make([]string, 0, len(facts.Focus.RequiredEvidenceKinds))
	for _, kind := range facts.Focus.RequiredEvidenceKinds {
		kind = strings.TrimSpace(kind)
		if kind == "" {
			continue
		}
		if _, ok := accepted[kind]; !ok {
			missing = append(missing, kind)
		}
	}
	return missing
}

// EvidenceAppliesToCurrentFocus is the authority rule for attributing evidence to the
// active coverage focus. Evidence without CoverageWorkItemIDs only applies when the
// current focus is the last remaining focus (no RemainingFocuses).
func EvidenceAppliesToCurrentFocus(facts RuntimeFacts, evidence EvidenceFact) bool {
	facts = facts.Normalized()
	if len(evidence.CoverageWorkItemIDs) == 0 {
		return len(facts.RemainingFocuses) == 0
	}
	current := make(map[common.CoverageWorkItemID]struct{}, len(facts.Focus.CoverageWorkItemIDs))
	for _, id := range facts.Focus.CoverageWorkItemIDs {
		current[id] = struct{}{}
	}
	for _, id := range evidence.CoverageWorkItemIDs {
		if _, ok := current[id]; ok {
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

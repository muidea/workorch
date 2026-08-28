package grounding

import (
	"sort"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// NormalizeTargetRefs canonicalizes a detached target set for exact owner
// fact comparison. It does not infer, discover, or authorize a target.
func NormalizeTargetRefs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	sort.Strings(ret)
	return ret
}

// SameTargetRefs compares two target sets after canonical normalization.
func SameTargetRefs(left, right []string) bool {
	left = NormalizeTargetRefs(left)
	right = NormalizeTargetRefs(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

// CurrentFocusTargetSelection returns only an owner selection committed as a
// satisfied Focus-scoped prerequisite for the current Focus. A raw resource
// pointer is audit lineage, not target authority.
func CurrentFocusTargetSelection(facts runtimecontract.RuntimeFacts) *runtimecontract.RuntimeTargetSelection {
	facts = facts.Normalized()
	if facts.Resources.TargetSelection == nil {
		return nil
	}
	selection := facts.Resources.TargetSelection.Normalized()
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		if prerequisite.ID == selection.PrerequisiteID &&
			prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSelection &&
			prerequisite.Scope == runtimecontract.RuntimePrerequisiteScopeFocus &&
			prerequisite.FocusID == facts.Focus.ID {
			return &selection
		}
	}
	return nil
}

// HasOwnerGroundedCreationScope recognizes only a persisted owner create
// observation matching the sealed Definition selector and the SM-2
// binding-sealed prospective target.
func HasOwnerGroundedCreationScope(facts runtimecontract.RuntimeFacts) bool {
	selection := CurrentFocusTargetSelection(facts)
	if selection == nil {
		return false
	}
	value := selection.Normalized()
	focus := facts.Focus.Normalized()
	return value.Disposition == runtimecontract.RuntimeTargetSelectionCreate &&
		value.OwnerModuleID != "" &&
		value.TargetRole == focus.TargetRole && value.SelectorRef == focus.TargetSelectorRef &&
		value.SelectorOwnerRef == focus.TargetSelectorOwnerRef && value.SelectorSubjectRef == focus.TargetSelectorSubjectRef &&
		runtimecontract.SameRuntimeTargetSelectorCandidateRefs(value.SelectorCandidateRefs, focus.TargetSelectorCandidateRefs) &&
		value.ProspectiveTargetRef != "" && len(focus.ProspectiveTargetRefs) == 1 && value.ProspectiveTargetRef == focus.ProspectiveTargetRefs[0]
}

// HasOwnerGroundedTargetScope recognizes a bounded, non-empty existing owner
// inventory. An empty observation proves only that no existing target was
// found; creation requires a separate SM-2 binding-sealed prospective target.
func HasOwnerGroundedTargetScope(facts runtimecontract.RuntimeFacts) bool {
	selection := CurrentFocusTargetSelection(facts)
	if selection == nil || selection.Disposition != runtimecontract.RuntimeTargetSelectionScope ||
		selection.OwnerModuleID == "" {
		return false
	}
	return len(selection.TargetRefs) != 0
}

// TargetSelectionHasConcreteTarget distinguishes an exact selected target from
// an owner scope. Only the former is a concrete repair surface.
func TargetSelectionHasConcreteTarget(selection runtimecontract.RuntimeTargetSelection) bool {
	selection = selection.Normalized()
	return selection.Disposition == runtimecontract.RuntimeTargetSelectionSelected && len(selection.TargetRefs) != 0
}

// OwnerGroundedTargetScopeMaySelectNewTarget preserves an owner's bounded
// existing-target scope adjudication only while no higher-precedence target
// fact has been sealed for the current focus.
func OwnerGroundedTargetScopeMaySelectNewTarget(facts runtimecontract.RuntimeFacts) bool {
	facts = facts.Normalized()
	if !HasOwnerGroundedTargetScope(facts) || len(facts.Focus.TargetRefs) != 0 {
		return false
	}
	if facts.Resources.FailureTarget != nil && len(facts.Resources.FailureTarget.TargetRefs) != 0 {
		return false
	}
	if facts.Failure != nil && len(facts.Failure.TargetRefs) != 0 {
		return false
	}
	return facts.RepairDirective == nil || len(facts.RepairDirective.TargetRefs) == 0
}

// RepairTargets projects the single persisted target-precedence set shared by
// SM-2 prerequisite acquisition and SM-3 admission. It never discovers or
// invents a target.
func RepairTargets(facts runtimecontract.RuntimeFacts) []string {
	if facts.Resources.FailureTarget != nil && len(facts.Resources.FailureTarget.TargetRefs) > 0 {
		return append([]string(nil), facts.Resources.FailureTarget.TargetRefs...)
	}
	if len(facts.Focus.TargetRefs) > 0 {
		return append([]string(nil), facts.Focus.TargetRefs...)
	}
	if selection := CurrentFocusTargetSelection(facts); selection != nil && TargetSelectionHasConcreteTarget(*selection) {
		return append([]string(nil), selection.TargetRefs...)
	}
	if selection := CurrentFocusTargetSelection(facts); HasOwnerGroundedCreationScope(facts) && selection != nil {
		return []string{selection.ProspectiveTargetRef}
	}
	if facts.RepairDirective != nil && len(facts.RepairDirective.TargetRefs) > 0 {
		return append([]string(nil), facts.RepairDirective.TargetRefs...)
	}
	return nil
}

// WorkspaceScopeTargets returns the canonical bounded inventory observed by
// the target owner. It does not authorize targets outside that inventory.
func WorkspaceScopeTargets(selection *runtimecontract.RuntimeTargetSelection) []string {
	if selection == nil {
		return nil
	}
	value := selection.Normalized()
	return append([]string(nil), value.CandidateTargetRefs...)
}

// TargetRefsIntersect reports exact normalized membership intersection.
func TargetRefsIntersect(left, right []string) bool {
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		if value = strings.TrimSpace(value); value != "" {
			seen[value] = struct{}{}
		}
	}
	for _, value := range right {
		if value = strings.TrimSpace(value); value != "" {
			if _, found := seen[value]; found {
				return true
			}
		}
	}
	return false
}

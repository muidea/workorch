package grounding

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// ValidationFailureTargetsAreGrounded checks whether a validation diagnostic
// names an already authoritative repair target. Receipt-evidence target
// discovery is promoted only by the Orchestrator's complete receipt/dispatch/
// evidence lineage gate; this fact-only helper must not recreate that authority
// from a raw owner field.
func ValidationFailureTargetsAreGrounded(facts runtimecontract.RuntimeFacts, failure runtimecontract.FailureFact) bool {
	targets := failure.TargetRefs
	if len(targets) == 0 {
		return false
	}
	grounded := append([]string(nil), facts.Focus.TargetRefs...)
	if facts.Resources.FailureTarget != nil {
		grounded = append(grounded, facts.Resources.FailureTarget.TargetRefs...)
	}
	if selection := CurrentFocusTargetSelection(facts); selection != nil &&
		selection.Disposition == runtimecontract.RuntimeTargetSelectionSelected && len(selection.TargetRefs) > 0 {
		grounded = append(grounded, selection.TargetRefs...)
	}
	if facts.RepairDirective != nil {
		grounded = append(grounded, facts.RepairDirective.TargetRefs...)
	}
	return TargetRefsIntersect(grounded, targets)
}

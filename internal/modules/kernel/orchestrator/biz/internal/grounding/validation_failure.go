package grounding

import runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"

// ValidationFailureTargetsAreGrounded checks whether a validation diagnostic
// names an already authoritative repair target or carries the exact fresh
// owner observation permitted for target discovery. It only compares facts;
// it does not promote a target or create repair authority.
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
	if TargetRefsIntersect(grounded, targets) {
		return true
	}
	observation := failure.TargetObservation
	if observation == nil {
		return false
	}
	value := observation.Normalized()
	if err := value.Validate(); err != nil || !failureTargetObservationOwnerAllowed(value, failure) ||
		value.TargetRole != facts.Focus.TargetRole || !SameTargetRefs(value.TargetRefs, targets) ||
		!failureTargetObservationHasCurrentFreshness(facts, value) {
		return false
	}
	return facts.Focus.RequireTargetGrounding
}

func failureTargetObservationOwnerAllowed(value runtimecontract.RuntimeFailureTargetObservation, failure runtimecontract.FailureFact) bool {
	switch value.OwnerModuleID {
	case runtimecontract.RuntimeModuleMCP:
		return true
	case runtimecontract.RuntimeModuleShell:
		return value.SubjectRef == "shell_runtime_policy" && value.DiagnosticClass != "" && value.ContentHash != "" && len(value.EvidenceRefs) > 0 &&
			failure.ValidationDiagnostic != nil && value.DiagnosticClass == failure.ValidationDiagnostic.Normalized().Class
	default:
		return false
	}
}

func failureTargetObservationHasCurrentFreshness(facts runtimecontract.RuntimeFacts, target runtimecontract.RuntimeFailureTargetObservation) bool {
	target = target.Normalized()
	for _, observation := range facts.Resources.PrerequisiteObservations {
		for _, dependency := range observation.Normalized().Dependencies {
			if dependency.Domain == runtimecontract.RuntimeResourceOwnerCatalog && dependency.OwnerModuleID == target.OwnerModuleID && dependency.Revision == target.ObservedRevision {
				return true
			}
		}
	}
	return false
}

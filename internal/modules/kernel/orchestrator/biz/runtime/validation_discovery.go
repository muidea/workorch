package runtime

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// MissingRuntimeValidationDiscoverySources compares the profile-selected
// discovery vocabulary with fresh, committed owner observations. It is a pure
// SM-2 authority check; a missing source is prerequisite debt, never evidence
// that a capability is unavailable.
func MissingRuntimeValidationDiscoverySources(facts runtimecontract.RuntimeFacts) []string {
	required := runtimecontract.RequiredRuntimeValidationDiscoverySources(
		facts.Activation.ExecutionContract.ValidationPolicies,
		runtimecontract.EvidenceKindValidation,
	)
	if len(required) == 0 {
		return nil
	}
	facts = facts.Normalized()
	missing := make([]string, 0, len(required))
	for _, source := range required {
		if runtimeValidationDiscoverySourceSatisfied(facts, source) {
			continue
		}
		missing = append(missing, source)
	}
	return missing
}

func runtimeValidationDiscoverySourceSatisfied(facts runtimecontract.RuntimeFacts, source string) bool {
	hasObservation := func(prerequisiteID string, kind runtimecontract.PrerequisiteKind) bool {
		observation, found := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, prerequisiteID)
		return found && observation.Kind == kind && observation.ContentHash != ""
	}
	switch strings.TrimSpace(source) {
	case runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest:
		if facts.Resources.Manifest == nil || strings.TrimSpace(facts.Resources.Manifest.Revision) == "" {
			return false
		}
		for _, prerequisite := range facts.Prerequisites.Satisfied {
			if prerequisite.Kind == runtimecontract.PrerequisiteKindManifest && hasObservation(prerequisite.ID, prerequisite.Kind) {
				return true
			}
		}
		return false
	case runtimecontract.RuntimeValidationDiscoverySourceProjectFiles:
		for _, prerequisite := range facts.Prerequisites.Satisfied {
			if prerequisite.Kind != runtimecontract.PrerequisiteKindManifest || !hasObservation(prerequisite.ID, prerequisite.Kind) {
				continue
			}
			observation, _ := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, prerequisite.ID)
			for _, dependency := range observation.Dependencies {
				if dependency.Domain == runtimecontract.RuntimeResourceWorkspaceTree {
					return true
				}
			}
		}
		return false
	case runtimecontract.RuntimeValidationDiscoverySourceDeclaredCommands:
		for _, inventory := range facts.Resources.OperationAdmissibilities {
			if len(inventory.OperationKinds) > 0 && hasObservation(inventory.PrerequisiteID, runtimecontract.PrerequisiteKindOperationAdmissibility) {
				return true
			}
		}
		return false
	case runtimecontract.RuntimeValidationDiscoverySourceAvailableCapabilities:
		if len(facts.Capabilities.StateChangeKinds)+len(facts.Capabilities.ValidationKinds)+len(facts.Capabilities.GovernanceKinds) == 0 {
			return false
		}
		for _, prerequisite := range facts.Prerequisites.Satisfied {
			if prerequisite.Kind == runtimecontract.PrerequisiteKindCapability && hasObservation(prerequisite.ID, prerequisite.Kind) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// reconcileRuntimeValidationDiscoveryPrerequisites turns each missing current
// owner observation back into the same typed acquisition. It never invents an
// owner action: manifest, capability and admissibility requirements are
// recovered only from the active graph or a current owner catalog.
func reconcileRuntimeValidationDiscoveryPrerequisites(facts runtimecontract.RuntimeFacts) (runtimecontract.RuntimeFacts, error) {
	missing := MissingRuntimeValidationDiscoverySources(facts)
	if len(missing) == 0 {
		return facts.Normalized(), nil
	}
	requirements, unresolved := runtimeValidationDiscoveryRequirements(facts, missing)
	if len(unresolved) != 0 {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("runtime validation discovery sources have no typed prerequisite contract: %s", strings.Join(unresolved, ", "))
	}
	facts = preflightRuntimeFactsPreservingPending(facts, requirements)

	// Preflight normally treats an already materialized resource as satisfied.
	// Discovery additionally requires its matching owner observation; if that
	// observation is absent/stale, move the exact satisfied prerequisite back to
	// pending without fabricating a resource revision or owner response.
	for _, requirement := range requirements {
		if runtimeValidationDiscoveryRequirementFresh(facts, requirement) {
			continue
		}
		prerequisiteID := runtimePrerequisiteID(requirement)
		alreadyPending := false
		for _, prerequisite := range facts.Prerequisites.Pending {
			if prerequisite.ID == prerequisiteID {
				alreadyPending = true
				break
			}
		}
		if alreadyPending {
			continue
		}
		var current runtimecontract.RuntimePrerequisite
		found := false
		satisfied := make([]runtimecontract.RuntimePrerequisite, 0, len(facts.Prerequisites.Satisfied))
		for _, prerequisite := range facts.Prerequisites.Satisfied {
			if prerequisite.ID == prerequisiteID {
				current, found = prerequisite, true
				continue
			}
			satisfied = append(satisfied, prerequisite)
		}
		facts.Prerequisites.Satisfied = satisfied
		if !found {
			current = runtimePrerequisiteFromRequirement(requirement)
		}
		facts.Prerequisites.Pending = append(facts.Prerequisites.Pending, current)
		observations := make([]runtimecontract.RuntimePrerequisiteObservation, 0, len(facts.Resources.PrerequisiteObservations))
		for _, observation := range facts.Resources.PrerequisiteObservations {
			if observation.PrerequisiteID != prerequisiteID {
				observations = append(observations, observation)
			}
		}
		facts.Resources.PrerequisiteObservations = observations
	}
	facts.Feasibility = nil
	return facts.Normalized(), nil
}

func runtimeValidationDiscoveryRequirements(facts runtimecontract.RuntimeFacts, missing []string) ([]runtimecontract.RuntimeRequirement, []string) {
	facts = facts.Normalized()
	active := runtimeActivePrerequisiteRequirements(facts)
	values := make([]runtimecontract.RuntimeRequirement, 0, len(active)+1)
	unresolved := make([]string, 0)
	for _, source := range missing {
		switch source {
		case runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest, runtimecontract.RuntimeValidationDiscoverySourceProjectFiles:
			manifestRequirements := make([]runtimecontract.RuntimeRequirement, 0, 1)
			for _, requirement := range active {
				if requirement.Kind == runtimecontract.PrerequisiteKindManifest {
					manifestRequirements = append(manifestRequirements, requirement)
				}
			}
			if _, ok := runtimeUniqueRequirementOwner(manifestRequirements); !ok {
				unresolved = append(unresolved, source)
				continue
			}
			values = append(values, manifestRequirements...)
		case runtimecontract.RuntimeValidationDiscoverySourceDeclaredCommands:
			before := len(values)
			for _, requirement := range active {
				if requirement.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility {
					values = append(values, requirement)
				}
			}
			if len(values) == before {
				for _, inventory := range facts.Resources.OperationAdmissibilities {
					values = append(values, runtimecontract.RuntimeRequirement{
						Kind: runtimecontract.PrerequisiteKindOperationAdmissibility, OwnerModuleID: inventory.OwnerModuleID,
						RequiredCapabilities: append([]string(nil), inventory.OperationKinds...), Reason: "validation discovery requires fresh declared commands",
					})
				}
			}
			if len(values) == before {
				unresolved = append(unresolved, source)
			}
		case runtimecontract.RuntimeValidationDiscoverySourceAvailableCapabilities:
			before := len(values)
			for _, requirement := range active {
				if requirement.Kind == runtimecontract.PrerequisiteKindCapability {
					values = append(values, requirement)
				}
			}
			if len(values) == before {
				unresolved = append(unresolved, source)
			}
		default:
			unresolved = append(unresolved, source)
		}
	}
	return normalizeRuntimeRequirements(values), normalizeRequirementStrings(unresolved)
}

func runtimeValidationDiscoveryRequirementFresh(facts runtimecontract.RuntimeFacts, requirement runtimecontract.RuntimeRequirement) bool {
	id := runtimePrerequisiteID(requirement)
	observation, found := runtimecontract.RuntimePrerequisiteObservationForID(facts.Resources.PrerequisiteObservations, id)
	if !found || observation.Kind != requirement.Kind || observation.OwnerModuleID != requirement.OwnerModuleID || observation.ContentHash == "" {
		return false
	}
	if requirement.Kind != runtimecontract.PrerequisiteKindManifest {
		return true
	}
	for _, dependency := range observation.Dependencies {
		if dependency.Domain == runtimecontract.RuntimeResourceWorkspaceTree {
			return true
		}
	}
	return false
}

func runtimePrerequisiteFromRequirement(requirement runtimecontract.RuntimeRequirement) runtimecontract.RuntimePrerequisite {
	requirement = normalizeRuntimeRequirements([]runtimecontract.RuntimeRequirement{requirement})[0]
	return runtimecontract.RuntimePrerequisite{
		ID: runtimePrerequisiteID(requirement), Kind: requirement.Kind, Scope: requirement.Scope, FocusID: requirement.FocusID,
		OwnerModuleID: requirement.OwnerModuleID, SubjectRef: requirement.SubjectRef,
		TargetSelectorRef: requirement.TargetSelectorRef, TargetSelectorOwnerRef: requirement.TargetSelectorOwnerRef,
		TargetSelectorSubjectRef: requirement.TargetSelectorSubjectRef, TargetSelectorCandidateRefs: requirement.TargetSelectorCandidateRefs,
		ProspectiveTargetAllOfConstraintRefs: requirement.ProspectiveTargetAllOfConstraintRefs,
		ProspectiveTargetRef:                 requirement.ProspectiveTargetRef, CapabilityClass: requirement.CapabilityClass,
		RequiredCapabilities: requirement.RequiredCapabilities, CapabilityStateKind: requirement.CapabilityStateKind,
		MinimumBytes: requirement.MinimumBytes, Reason: requirement.Reason,
	}
}

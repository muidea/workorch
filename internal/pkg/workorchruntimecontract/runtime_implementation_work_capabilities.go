package workorchruntimecontract

import (
	"fmt"
	"slices"
	"sort"
)

// CompileRuntimeImplementationWorkCapabilityRequirements projects the inventory
// needed by admitted exact-CWI policies. Inventory is not permission to dispatch:
// variants, targets, budgets and proof remain bound to their original CWI policy.
// Unmatched Profile declarations cannot enlist additional capability owners.
func CompileRuntimeImplementationWorkCapabilityRequirements(focuses []RuntimeDefinitionFocus, profiles []RuntimeProfileContract) ([]RuntimeRequirement, error) {
	if err := ValidateRuntimeDefinitionImplementationWorkPolicies(focuses, profiles); err != nil {
		return nil, err
	}
	byOwner := map[string][]string{}
	for _, focus := range focuses {
		if focus.ImplementationWorkPolicy == nil {
			continue
		}
		policy := focus.ImplementationWorkPolicy.Normalized()
		for _, operation := range policy.AllowedOperations {
			byOwner[policy.OwnerModuleID] = append(byOwner[policy.OwnerModuleID], operation.Kind)
		}
	}
	owners := make([]string, 0, len(byOwner))
	for owner := range byOwner {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	var result []RuntimeRequirement
	for _, owner := range owners {
		kinds := byOwner[owner]
		sort.Strings(kinds)
		kinds = slices.Compact(kinds)
		result = append(result, RuntimeRequirement{
			Kind: PrerequisiteKindCapability, Scope: RuntimePrerequisiteScopeGlobal,
			OwnerModuleID: owner, CapabilityClass: RuntimeCapabilityClassStateChange,
			RequiredCapabilities: kinds, Reason: "admitted implementation work capability inventory required",
		})
	}
	return result, nil
}

// ValidateRuntimeImplementationWorkCapabilityRequirements rejects incomplete
// bootstrap projections without repairing them on load. Owner inventory and
// parameter admissibility are separate prerequisites; neither seals work scope.
func ValidateRuntimeImplementationWorkCapabilityRequirements(focuses []RuntimeDefinitionFocus, profiles []RuntimeProfileContract, capabilityContract []string, requirements []RuntimeRequirement) error {
	expected, err := CompileRuntimeImplementationWorkCapabilityRequirements(focuses, profiles)
	if err != nil {
		return err
	}
	for _, inventory := range expected {
		for _, kind := range inventory.RequiredCapabilities {
			if !slices.Contains(capabilityContract, kind) {
				return fmt.Errorf("implementation work operation %q missing from Definition capability contract", kind)
			}
			capability, admissibility := false, false
			for _, requirement := range requirements {
				if requirement.OwnerModuleID != inventory.OwnerModuleID ||
					(requirement.Scope != "" && requirement.Scope != RuntimePrerequisiteScopeGlobal) || requirement.FocusID != "" ||
					!slices.Contains(requirement.RequiredCapabilities, kind) {
					continue
				}
				if requirement.Kind == PrerequisiteKindCapability && requirement.CapabilityClass == RuntimeCapabilityClassStateChange {
					capability = true
				}
				if requirement.Kind == PrerequisiteKindOperationAdmissibility {
					admissibility = true
				}
			}
			if !capability || !admissibility {
				return fmt.Errorf("implementation work operation %q requires owner %q global capability and admissibility prerequisites", kind, inventory.OwnerModuleID)
			}
		}
	}
	return nil
}

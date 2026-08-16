package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"
)

// RuntimeOwnerFactKind identifies a declarative, current capability-owner fact
// which must hold before SM-2 may settle one coverage item. It is not a
// prerequisite or an action selector: owners publish their ordinary typed
// catalogs, while the Orchestrator alone decides coverage from those facts.
type RuntimeOwnerFactKind string

const (
	// RuntimeOwnerFactSelectableValidationSurface requires the named owner and
	// operation kind to publish at least one currently selectable validation
	// surface. Selectability remains owner-authored readiness plus direct proof
	// applicability; a callable, auxiliary, or required-input surface is not
	// sufficient.
	RuntimeOwnerFactSelectableValidationSurface RuntimeOwnerFactKind = "selectable_validation_surface"
)

// RuntimeOwnerFactRequirement is a profile-declared postcondition over an
// existing typed owner catalog. It carries no owner-specific parameters and
// cannot create, refresh, or mutate that catalog.
type RuntimeOwnerFactRequirement struct {
	OwnerModuleID string
	FactKind      RuntimeOwnerFactKind
	OperationKind string
}

func (r RuntimeOwnerFactRequirement) Normalized() RuntimeOwnerFactRequirement {
	r.OwnerModuleID = strings.TrimSpace(r.OwnerModuleID)
	r.FactKind = RuntimeOwnerFactKind(strings.TrimSpace(string(r.FactKind)))
	r.OperationKind = CanonicalRuntimeBlockKind(r.OperationKind)
	return r
}

func (r RuntimeOwnerFactRequirement) Validate() error {
	r = r.Normalized()
	if r.OwnerModuleID == "" || r.FactKind == "" || r.OperationKind == "" {
		return fmt.Errorf("runtime owner-fact requirement requires owner, fact kind and operation kind")
	}
	descriptor, found := RuntimeOperationDescriptorForKind(r.OperationKind)
	if !found || descriptor.OwnerModuleID != r.OwnerModuleID {
		return fmt.Errorf("runtime owner-fact requirement operation %q does not belong to owner %q", r.OperationKind, r.OwnerModuleID)
	}
	switch r.FactKind {
	case RuntimeOwnerFactSelectableValidationSurface:
		if !runtimeDescriptorAllowsState(descriptor, ConvergenceStateNeedValidation) {
			return fmt.Errorf("runtime owner-fact requirement operation %q is not a validation operation", r.OperationKind)
		}
		return nil
	default:
		return fmt.Errorf("runtime owner-fact requirement has unsupported fact kind %q", r.FactKind)
	}
}

func NormalizeRuntimeOwnerFactRequirements(values []RuntimeOwnerFactRequirement) []RuntimeOwnerFactRequirement {
	byKey := make(map[string]RuntimeOwnerFactRequirement, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		if value.OwnerModuleID == "" || value.FactKind == "" || value.OperationKind == "" {
			continue
		}
		key := value.OwnerModuleID + "|" + string(value.FactKind) + "|" + value.OperationKind
		byKey[key] = value
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimeOwnerFactRequirement, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byKey[key])
	}
	return nilIfEmpty(ret)
}

func ValidateRuntimeOwnerFactRequirements(values []RuntimeOwnerFactRequirement) error {
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		if err := value.Validate(); err != nil {
			return err
		}
		key := value.OwnerModuleID + "|" + string(value.FactKind) + "|" + value.OperationKind
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("runtime owner-fact requirement %q is duplicated", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// RuntimeMissingOwnerFactRequirements returns the exact declarative
// postconditions absent from the current owner catalogs. It is a pure value
// projection; callers retain authority over every state transition.
func RuntimeMissingOwnerFactRequirements(requirements []RuntimeOwnerFactRequirement, catalogs []RuntimeOperationAdmissibility) []RuntimeOwnerFactRequirement {
	requirements = NormalizeRuntimeOwnerFactRequirements(requirements)
	catalogs = NormalizeRuntimeOperationAdmissibilities(catalogs)
	missing := make([]RuntimeOwnerFactRequirement, 0)
	for _, requirement := range requirements {
		if !runtimeOwnerFactRequirementSatisfied(requirement, catalogs) {
			missing = append(missing, requirement)
		}
	}
	return nilIfEmpty(missing)
}

func runtimeOwnerFactRequirementSatisfied(requirement RuntimeOwnerFactRequirement, catalogs []RuntimeOperationAdmissibility) bool {
	requirement = requirement.Normalized()
	for _, catalog := range catalogs {
		catalog = catalog.Normalized()
		if catalog.OwnerModuleID != requirement.OwnerModuleID || !runtimeOperationKindsContain(catalog.OperationKinds, requirement.OperationKind) {
			continue
		}
		switch requirement.FactKind {
		case RuntimeOwnerFactSelectableValidationSurface:
			selectable := make(map[string]struct{}, len(catalog.ValidationRecipeRefs))
			for _, ref := range catalog.ValidationRecipeRefs {
				selectable[ref] = struct{}{}
			}
			for _, surface := range catalog.ValidationSurfaces {
				surface = surface.Normalized()
				if surface.OperationKind != requirement.OperationKind || surface.Readiness != RuntimeValidationSurfaceReady ||
					surface.ProofApplicability != RuntimeValidationProofDirect {
					continue
				}
				if _, published := selectable[surface.ID]; published {
					return true
				}
			}
		}
	}
	return false
}

func sameRuntimeOwnerFactRequirements(left, right []RuntimeOwnerFactRequirement) bool {
	left = NormalizeRuntimeOwnerFactRequirements(left)
	right = NormalizeRuntimeOwnerFactRequirements(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

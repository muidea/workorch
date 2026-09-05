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
	RuntimeOwnerFactMissingTrigger                                   = "owner_fact_missing"
)

// RuntimeOwnerFactRequirement is a profile-declared postcondition over an
// existing typed owner catalog. It carries no owner-specific parameters and
// cannot create, refresh, or mutate that catalog.
type RuntimeOwnerFactRequirement struct {
	OwnerModuleID                  string
	FactKind                       RuntimeOwnerFactKind
	OperationKind                  string
	ValidationProofRequirementHash string
}

// RuntimeOwnerFactConvergence is a profile-owned, static closure declaration
// for one required owner fact. It either requires that fact to pre-exist or
// authorizes one exact capability-owner acquisition followed by re-observation
// of the original fact owner. It is immutable policy input, not a retry state
// or an instruction that capability owners may use to transition Runtime.
type RuntimeOwnerFactConvergence struct {
	Requirement        RuntimeOwnerFactRequirement
	RequirePreexisting bool
	// ProspectiveSatisfactionContractRef binds a bounded acquisition to the
	// required-fact owner's declaration of the exact postcondition that the
	// proposed owner delta must satisfy. It is static Definition/profile
	// authority, not a capability-owner prediction or a Runtime transition.
	ProspectiveSatisfactionContractRef string
	MissingTrigger                     string
	AcquisitionOwnerModuleID           string
	AcquisitionOperationKind           string
	// AcquisitionOperationVariantID and AcquisitionTarget* identify the
	// concrete operation-admissibility fact that authorizes the one allowed
	// acquisition. TargetSelectorRef below remains the Definition-owned target
	// selection scope. The two selector namespaces are intentionally distinct
	// and must never be compared or normalized as aliases.
	AcquisitionOperationVariantID string
	AcquisitionTargetPolicy       RuntimeOperationTargetPolicy
	AcquisitionTargetSelectorRef  string
	TargetSelectorRef             string
	TargetSelectorOwnerRef        string
	TargetSelectorSubjectRef      string
	ReobserveOwnerModuleID        string
	ReobserveFactKind             RuntimeOwnerFactKind
	MaxAcquisitionAttempts        int
	DependsOnRequirementRefs      []string
}

func (c RuntimeOwnerFactConvergence) Normalized() RuntimeOwnerFactConvergence {
	c.Requirement = c.Requirement.Normalized()
	c.ProspectiveSatisfactionContractRef = strings.TrimSpace(c.ProspectiveSatisfactionContractRef)
	c.MissingTrigger = strings.TrimSpace(c.MissingTrigger)
	c.AcquisitionOwnerModuleID = strings.TrimSpace(c.AcquisitionOwnerModuleID)
	c.AcquisitionOperationKind = CanonicalRuntimeBlockKind(c.AcquisitionOperationKind)
	c.AcquisitionOperationVariantID = strings.TrimSpace(c.AcquisitionOperationVariantID)
	c.AcquisitionTargetPolicy = RuntimeOperationTargetPolicy(strings.TrimSpace(string(c.AcquisitionTargetPolicy)))
	c.AcquisitionTargetSelectorRef = strings.TrimSpace(c.AcquisitionTargetSelectorRef)
	c.TargetSelectorRef = strings.TrimSpace(c.TargetSelectorRef)
	c.TargetSelectorOwnerRef = strings.TrimSpace(c.TargetSelectorOwnerRef)
	c.TargetSelectorSubjectRef = strings.TrimSpace(c.TargetSelectorSubjectRef)
	c.ReobserveOwnerModuleID = strings.TrimSpace(c.ReobserveOwnerModuleID)
	c.ReobserveFactKind = RuntimeOwnerFactKind(strings.TrimSpace(string(c.ReobserveFactKind)))
	c.DependsOnRequirementRefs = normalizeRuntimeOwnerFactRefs(c.DependsOnRequirementRefs)
	return c
}

func (c RuntimeOwnerFactConvergence) Validate() error {
	c = c.Normalized()
	if err := c.Requirement.Validate(); err != nil {
		return err
	}
	if c.RequirePreexisting {
		if c.ProspectiveSatisfactionContractRef != "" || c.MissingTrigger != "" || c.AcquisitionOwnerModuleID != "" || c.AcquisitionOperationKind != "" ||
			c.AcquisitionOperationVariantID != "" || c.AcquisitionTargetPolicy != "" || c.AcquisitionTargetSelectorRef != "" ||
			c.TargetSelectorRef != "" || c.TargetSelectorOwnerRef != "" || c.TargetSelectorSubjectRef != "" ||
			c.ReobserveOwnerModuleID != "" || c.ReobserveFactKind != "" || c.MaxAcquisitionAttempts != 0 || len(c.DependsOnRequirementRefs) != 0 {
			return fmt.Errorf("preexisting owner-fact convergence cannot declare acquisition or re-observation")
		}
		return nil
	}
	if c.MissingTrigger != RuntimeOwnerFactMissingTrigger || c.AcquisitionOwnerModuleID == "" || c.AcquisitionOperationKind == "" ||
		c.AcquisitionTargetPolicy == "" || c.AcquisitionTargetSelectorRef == "" ||
		c.TargetSelectorRef == "" || c.TargetSelectorOwnerRef == "" || c.TargetSelectorSubjectRef == "" ||
		c.ReobserveOwnerModuleID != c.Requirement.OwnerModuleID || c.ReobserveFactKind != c.Requirement.FactKind || c.MaxAcquisitionAttempts != 1 {
		return fmt.Errorf("owner-fact acquisition requires exact missing trigger, owner operation, selector, re-observation and one attempt")
	}
	descriptor, found := RuntimeOperationDescriptorForKind(c.AcquisitionOperationKind)
	if !found || descriptor.OwnerModuleID != c.AcquisitionOwnerModuleID {
		return fmt.Errorf("owner-fact acquisition operation %q does not belong to owner %q", c.AcquisitionOperationKind, c.AcquisitionOwnerModuleID)
	}
	if contract, found := RuntimeOperationContractForKind(c.AcquisitionOperationKind); found {
		if len(contract.Variants) == 0 && c.AcquisitionOperationVariantID != "" {
			return fmt.Errorf("owner-fact acquisition operation %q does not declare a static variant", c.AcquisitionOperationKind)
		}
		if len(contract.Variants) != 0 {
			matched := false
			for _, variant := range contract.Variants {
				if variant.ID == c.AcquisitionOperationVariantID {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("owner-fact acquisition operation %q requires one exact static variant", c.AcquisitionOperationKind)
			}
		}
	}
	switch c.AcquisitionTargetPolicy {
	case RuntimeOperationTargetPolicyExistingOwnerScope:
		if c.ProspectiveSatisfactionContractRef != "" {
			return fmt.Errorf("existing-scope owner-fact acquisition cannot declare a prospective satisfaction contract")
		}
	case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		if c.ProspectiveSatisfactionContractRef == "" {
			return fmt.Errorf("prospective owner-fact acquisition requires a prospective satisfaction contract")
		}
	default:
		return fmt.Errorf("owner-fact acquisition requires a selector-backed target policy, got %q", c.AcquisitionTargetPolicy)
	}
	return nil
}

func NormalizeRuntimeOwnerFactConvergences(values []RuntimeOwnerFactConvergence) []RuntimeOwnerFactConvergence {
	ret := append([]RuntimeOwnerFactConvergence(nil), values...)
	for idx := range ret {
		ret[idx] = ret[idx].Normalized()
	}
	sort.Slice(ret, func(i, j int) bool {
		return RuntimeOwnerFactRequirementRef(ret[i].Requirement) < RuntimeOwnerFactRequirementRef(ret[j].Requirement)
	})
	return nilIfEmpty(ret)
}

func CloneRuntimeOwnerFactConvergences(values []RuntimeOwnerFactConvergence) []RuntimeOwnerFactConvergence {
	ret := append([]RuntimeOwnerFactConvergence(nil), values...)
	for idx := range ret {
		ret[idx].DependsOnRequirementRefs = append([]string(nil), values[idx].DependsOnRequirementRefs...)
	}
	return ret
}

func ValidateRuntimeOwnerFactConvergences(requirements []RuntimeOwnerFactRequirement, values []RuntimeOwnerFactConvergence) error {
	requirements = NormalizeRuntimeOwnerFactRequirements(requirements)
	values = NormalizeRuntimeOwnerFactConvergences(values)
	declared := make(map[string]struct{}, len(requirements))
	for _, requirement := range requirements {
		declared[RuntimeOwnerFactRequirementRef(requirement)] = struct{}{}
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := value.Validate(); err != nil {
			return err
		}
		key := RuntimeOwnerFactRequirementRef(value.Requirement)
		if _, ok := declared[key]; !ok {
			return fmt.Errorf("owner-fact convergence %q has no matching required fact", key)
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("owner-fact convergence %q is duplicated", key)
		}
		seen[key] = struct{}{}
	}
	edges := make(map[string][]string, len(values))
	for _, value := range values {
		key := RuntimeOwnerFactRequirementRef(value.Requirement)
		for _, dependency := range value.DependsOnRequirementRefs {
			if dependency == key {
				return fmt.Errorf("owner-fact convergence %q cannot depend on itself", key)
			}
			if _, ok := declared[dependency]; !ok {
				return fmt.Errorf("owner-fact convergence %q depends on unknown requirement %q", key, dependency)
			}
			edges[key] = append(edges[key], dependency)
		}
	}
	if cycle := runtimeOwnerFactConvergenceCycle(edges); len(cycle) > 0 {
		return fmt.Errorf("owner-fact convergence contains dependency cycle %q", strings.Join(cycle, " -> "))
	}
	for key := range declared {
		if _, ok := seen[key]; !ok {
			return fmt.Errorf("required owner fact %q has no static convergence declaration", key)
		}
	}
	return nil
}

func SameRuntimeOwnerFactConvergences(left, right []RuntimeOwnerFactConvergence) bool {
	left = NormalizeRuntimeOwnerFactConvergences(left)
	right = NormalizeRuntimeOwnerFactConvergences(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx].Requirement != right[idx].Requirement || left[idx].RequirePreexisting != right[idx].RequirePreexisting ||
			left[idx].ProspectiveSatisfactionContractRef != right[idx].ProspectiveSatisfactionContractRef ||
			left[idx].MissingTrigger != right[idx].MissingTrigger || left[idx].AcquisitionOwnerModuleID != right[idx].AcquisitionOwnerModuleID ||
			left[idx].AcquisitionOperationKind != right[idx].AcquisitionOperationKind || left[idx].AcquisitionOperationVariantID != right[idx].AcquisitionOperationVariantID ||
			left[idx].AcquisitionTargetPolicy != right[idx].AcquisitionTargetPolicy || left[idx].AcquisitionTargetSelectorRef != right[idx].AcquisitionTargetSelectorRef ||
			left[idx].TargetSelectorRef != right[idx].TargetSelectorRef ||
			left[idx].TargetSelectorOwnerRef != right[idx].TargetSelectorOwnerRef || left[idx].TargetSelectorSubjectRef != right[idx].TargetSelectorSubjectRef ||
			left[idx].ReobserveOwnerModuleID != right[idx].ReobserveOwnerModuleID || left[idx].ReobserveFactKind != right[idx].ReobserveFactKind ||
			left[idx].MaxAcquisitionAttempts != right[idx].MaxAcquisitionAttempts || !sameStrings(left[idx].DependsOnRequirementRefs, right[idx].DependsOnRequirementRefs) {
			return false
		}
	}
	return true
}

// ResolveRuntimeOwnerFactAcquisition returns the deterministic first ready
// acquisition in an all-of owner-fact closure. It never observes or mutates an
// owner: catalogs are immutable facts supplied by their owners, while callers
// in SM-2 decide whether to persist or execute the returned declaration.
// Independent ready requirements are ordered by their stable requirement ref;
// they are all still required and therefore do not form one-of semantics.
func ResolveRuntimeOwnerFactAcquisition(requirements []RuntimeOwnerFactRequirement, values []RuntimeOwnerFactConvergence, catalogs []RuntimeOperationAdmissibility) (RuntimeOwnerFactConvergence, bool) {
	requirements = NormalizeRuntimeOwnerFactRequirements(requirements)
	values = NormalizeRuntimeOwnerFactConvergences(values)
	if len(requirements) == 0 || ValidateRuntimeOwnerFactConvergences(requirements, values) != nil {
		return RuntimeOwnerFactConvergence{}, false
	}
	missing := RuntimeMissingOwnerFactRequirements(requirements, catalogs)
	if len(missing) == 0 {
		return RuntimeOwnerFactConvergence{}, false
	}
	missingRefs := make(map[string]struct{}, len(missing))
	for _, requirement := range missing {
		missingRefs[RuntimeOwnerFactRequirementRef(requirement)] = struct{}{}
	}
	byRequirement := make(map[string]RuntimeOwnerFactConvergence, len(values))
	for _, value := range values {
		byRequirement[RuntimeOwnerFactRequirementRef(value.Requirement)] = value
	}
	ready := make([]RuntimeOwnerFactConvergence, 0, len(missing))
	for _, requirement := range missing {
		value, found := byRequirement[RuntimeOwnerFactRequirementRef(requirement)]
		if !found || value.RequirePreexisting || value.MaxAcquisitionAttempts != 1 {
			// A missing preexisting-only postcondition is definition drift, not
			// authority to execute some other acquisition.
			return RuntimeOwnerFactConvergence{}, false
		}
		blocked := false
		for _, dependency := range value.DependsOnRequirementRefs {
			if _, stillMissing := missingRefs[dependency]; stillMissing {
				blocked = true
				break
			}
		}
		if !blocked {
			ready = append(ready, value)
		}
	}
	if len(ready) == 0 {
		return RuntimeOwnerFactConvergence{}, false
	}
	sort.Slice(ready, func(i, j int) bool {
		return RuntimeOwnerFactRequirementRef(ready[i].Requirement) < RuntimeOwnerFactRequirementRef(ready[j].Requirement)
	})
	return ready[0], true
}
func RuntimeOwnerFactRequirementRef(value RuntimeOwnerFactRequirement) string {
	value = value.Normalized()
	return value.OwnerModuleID + "|" + string(value.FactKind) + "|" + value.OperationKind + "|" + value.ValidationProofRequirementHash
}

func runtimeOwnerFactConvergenceCycle(edges map[string][]string) []string {
	state := make(map[string]uint8, len(edges))
	stack := make([]string, 0, len(edges))
	var visit func(string) []string
	visit = func(ref string) []string {
		switch state[ref] {
		case 1:
			for idx := range stack {
				if stack[idx] == ref {
					return append([]string(nil), stack[idx:]...)
				}
			}
			return []string{ref}
		case 2:
			return nil
		}
		state[ref] = 1
		stack = append(stack, ref)
		for _, dependency := range edges[ref] {
			if cycle := visit(dependency); len(cycle) > 0 {
				return cycle
			}
		}
		stack = stack[:len(stack)-1]
		state[ref] = 2
		return nil
	}
	refs := make([]string, 0, len(edges))
	for ref := range edges {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		if cycle := visit(ref); len(cycle) > 0 {
			return cycle
		}
	}
	return nil
}

func normalizeRuntimeOwnerFactRefs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	sort.Strings(ret)
	return nilIfEmpty(ret)
}

func (r RuntimeOwnerFactRequirement) Normalized() RuntimeOwnerFactRequirement {
	r.OwnerModuleID = strings.TrimSpace(r.OwnerModuleID)
	r.FactKind = RuntimeOwnerFactKind(strings.TrimSpace(string(r.FactKind)))
	r.OperationKind = CanonicalRuntimeBlockKind(r.OperationKind)
	r.ValidationProofRequirementHash = strings.TrimSpace(r.ValidationProofRequirementHash)
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
		if r.ValidationProofRequirementHash != "" && !strings.HasPrefix(r.ValidationProofRequirementHash, RuntimeValidationProofRequirementHashPrefix) {
			return fmt.Errorf("runtime owner-fact requirement has invalid validation proof requirement identity")
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
		key := RuntimeOwnerFactRequirementRef(value)
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
		key := RuntimeOwnerFactRequirementRef(value)
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
				if _, published := selectable[surface.ID]; !published {
					continue
				}
				if requirement.ValidationProofRequirementHash == "" {
					return true
				}
				for _, binding := range surface.ProofRequirementBindings {
					if binding.RequirementHash == requirement.ValidationProofRequirementHash {
						return true
					}
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

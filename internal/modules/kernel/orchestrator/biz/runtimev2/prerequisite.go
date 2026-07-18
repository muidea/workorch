package runtimev2

import (
	"fmt"
	"sort"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type RuntimeRequirement = runtimecontract.RuntimeRequirement

type PrerequisiteResult struct {
	PrerequisiteID         string
	Manifest               *runtimecontract.WorkspaceManifest
	TargetSnapshot         *runtimecontract.TargetSnapshot
	FailureTarget          *runtimecontract.FailureTarget
	Capabilities           *runtimecontract.RuntimeCapabilities
	ValidationScratch      *runtimecontract.ValidationScratch
	ProviderCeiling        *runtimecontract.ProviderCeiling
	CapabilityState        *runtimecontract.RuntimeCapabilityStateSnapshot
	OperationAdmissibility *runtimecontract.RuntimeOperationAdmissibility
}

func PreflightRuntimeFacts(facts runtimecontract.RuntimeFacts, requirements []RuntimeRequirement) runtimecontract.RuntimeFacts {
	facts = facts.Normalized()
	pending := make([]runtimecontract.RuntimePrerequisite, 0, len(requirements))
	for _, requirement := range normalizeRuntimeRequirements(requirements) {
		if runtimeRequirementSatisfied(facts, requirement) {
			continue
		}
		pending = append(pending, runtimecontract.RuntimePrerequisite{
			ID:                   runtimePrerequisiteID(requirement),
			Kind:                 requirement.Kind,
			OwnerModuleID:        requirement.OwnerModuleID,
			SubjectRef:           requirement.SubjectRef,
			CapabilityClass:      requirement.CapabilityClass,
			RequiredCapabilities: requirement.RequiredCapabilities,
			CapabilityStateKind:  requirement.CapabilityStateKind,
			MinimumBytes:         requirement.MinimumBytes,
			Reason:               requirement.Reason,
		})
	}
	facts.Prerequisites.Pending = pending
	return facts.Normalized()
}

func CommitPrerequisiteResult(facts runtimecontract.RuntimeFacts, result PrerequisiteResult) (runtimecontract.RuntimeFacts, error) {
	facts = facts.Normalized()
	result.PrerequisiteID = strings.TrimSpace(result.PrerequisiteID)
	if result.PrerequisiteID == "" {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("prerequisite result requires an id")
	}
	var completed *runtimecontract.RuntimePrerequisite
	for idx := range facts.Prerequisites.Pending {
		if facts.Prerequisites.Pending[idx].ID == result.PrerequisiteID {
			item := facts.Prerequisites.Pending[idx]
			completed = &item
			break
		}
	}
	if completed == nil {
		return runtimecontract.RuntimeFacts{}, fmt.Errorf("unknown or already committed prerequisite %q", result.PrerequisiteID)
	}
	if err := applyPrerequisiteResult(&facts, *completed, result); err != nil {
		return runtimecontract.RuntimeFacts{}, err
	}
	next := make([]runtimecontract.RuntimePrerequisite, 0, len(facts.Prerequisites.Pending)-1)
	for _, item := range facts.Prerequisites.Pending {
		if item.ID != result.PrerequisiteID {
			next = append(next, item)
		}
	}
	facts.Prerequisites.Pending = next
	facts.Prerequisites.Satisfied = append(facts.Prerequisites.Satisfied, *completed)
	return facts.Normalized(), nil
}

func applyPrerequisiteResult(facts *runtimecontract.RuntimeFacts, prerequisite runtimecontract.RuntimePrerequisite, result PrerequisiteResult) error {
	switch prerequisite.Kind {
	case runtimecontract.PrerequisiteKindManifest:
		if result.Manifest == nil || strings.TrimSpace(result.Manifest.Revision) == "" {
			return fmt.Errorf("manifest prerequisite returned no manifest")
		}
		facts.Resources.Manifest = result.Manifest
	case runtimecontract.PrerequisiteKindTargetSnapshot:
		if result.TargetSnapshot == nil || result.TargetSnapshot.TargetRef != prerequisite.SubjectRef || !result.TargetSnapshot.Complete() {
			return fmt.Errorf("target snapshot prerequisite returned no complete snapshot for %q", prerequisite.SubjectRef)
		}
		facts.Resources.TargetSnapshots = append(facts.Resources.TargetSnapshots, *result.TargetSnapshot)
	case runtimecontract.PrerequisiteKindFailureTarget:
		if result.FailureTarget == nil || len(result.FailureTarget.TargetRefs) == 0 {
			return fmt.Errorf("failure target prerequisite returned no targets")
		}
		facts.Resources.FailureTarget = result.FailureTarget
	case runtimecontract.PrerequisiteKindCapability:
		if result.Capabilities == nil || !capabilitiesSatisfy(*result.Capabilities, prerequisite.CapabilityClass, prerequisite.RequiredCapabilities) {
			return fmt.Errorf("capability prerequisite returned no required capabilities")
		}
		// Owner merges are additive and must not replace another owner's sealed kinds.
		facts.Capabilities.StateChangeKinds = mergeCapabilityKinds(facts.Capabilities.StateChangeKinds, result.Capabilities.StateChangeKinds)
		facts.Capabilities.ValidationKinds = mergeCapabilityKinds(facts.Capabilities.ValidationKinds, result.Capabilities.ValidationKinds)
		facts.Capabilities.GovernanceKinds = mergeCapabilityKinds(facts.Capabilities.GovernanceKinds, result.Capabilities.GovernanceKinds)
	case runtimecontract.PrerequisiteKindCapabilityState:
		if result.CapabilityState == nil || result.CapabilityState.RequirementID != prerequisite.ID || result.CapabilityState.OwnerModuleID != prerequisite.OwnerModuleID || result.CapabilityState.CapabilityStateKind != prerequisite.CapabilityStateKind || result.CapabilityState.SubjectRef != prerequisite.SubjectRef {
			return fmt.Errorf("capability-state prerequisite returned a mismatched snapshot")
		}
		facts.Resources.CapabilityStates = append(facts.Resources.CapabilityStates, *result.CapabilityState)
	case runtimecontract.PrerequisiteKindValidationScratch:
		if result.ValidationScratch == nil || strings.TrimSpace(result.ValidationScratch.Reference) == "" || strings.TrimSpace(result.ValidationScratch.Revision) == "" {
			return fmt.Errorf("validation scratch prerequisite returned no scratch reference")
		}
		facts.Resources.ValidationScratch = result.ValidationScratch
	case runtimecontract.PrerequisiteKindProviderCeiling:
		if result.ProviderCeiling == nil {
			return fmt.Errorf("provider ceiling prerequisite returned no ceiling")
		}
		if err := result.ProviderCeiling.Validate(); err != nil {
			return fmt.Errorf("provider ceiling prerequisite returned invalid ceiling: %w", err)
		}
		if result.ProviderCeiling.MaxBytes < prerequisite.MinimumBytes {
			return fmt.Errorf("provider ceiling prerequisite returned an insufficient ceiling")
		}
		if target := facts.Activation.ProviderTarget.Normalized(); target.ProviderID != "" || target.ModelID != "" {
			if !result.ProviderCeiling.Target.Same(target) {
				return fmt.Errorf("provider ceiling target does not match activation target")
			}
		}
		facts.Resources.ProviderCeiling = result.ProviderCeiling
	case runtimecontract.PrerequisiteKindOperationAdmissibility:
		if result.OperationAdmissibility == nil {
			return fmt.Errorf("operation admissibility prerequisite returned no inventory")
		}
		inventory := result.OperationAdmissibility.Normalized()
		if inventory.PrerequisiteID != prerequisite.ID || inventory.OwnerModuleID != prerequisite.OwnerModuleID {
			return fmt.Errorf("operation admissibility prerequisite returned a mismatched inventory")
		}
		if err := inventory.Validate(); err != nil {
			return fmt.Errorf("operation admissibility prerequisite returned invalid inventory: %w", err)
		}
		maxItems := facts.ConvergencePolicy.MaxCandidateContextItems
		if maxItems < 1 || len(inventory.TargetRefs) > maxItems || len(inventory.SelectorRefs) > maxItems {
			return fmt.Errorf("operation admissibility inventory exceeds sealed candidate context bound")
		}
		required := make([]string, 0, len(prerequisite.RequiredCapabilities))
		for _, kind := range prerequisite.RequiredCapabilities {
			if canonical := runtimecontract.CanonicalRuntimeBlockKind(kind); canonical != "" {
				required = append(required, canonical)
			}
		}
		available := map[string]struct{}{}
		for _, kind := range inventory.OperationKinds {
			available[kind] = struct{}{}
		}
		for _, kind := range required {
			if _, ok := available[kind]; !ok {
				return fmt.Errorf("operation admissibility inventory does not include required kind %q", kind)
			}
		}
		facts.Resources.OperationAdmissibilities = append(facts.Resources.OperationAdmissibilities, inventory)
	default:
		return fmt.Errorf("unsupported prerequisite kind %q", prerequisite.Kind)
	}
	return nil
}

func runtimeRequirementSatisfied(facts runtimecontract.RuntimeFacts, requirement RuntimeRequirement) bool {
	switch requirement.Kind {
	case runtimecontract.PrerequisiteKindManifest:
		return facts.Resources.Manifest != nil && facts.Resources.Manifest.Revision != ""
	case runtimecontract.PrerequisiteKindTargetSnapshot:
		for _, snapshot := range facts.Resources.TargetSnapshots {
			if snapshot.TargetRef == requirement.SubjectRef && snapshot.Complete() {
				return true
			}
		}
		return false
	case runtimecontract.PrerequisiteKindFailureTarget:
		return facts.Resources.FailureTarget != nil && len(facts.Resources.FailureTarget.TargetRefs) > 0
	case runtimecontract.PrerequisiteKindCapability:
		return capabilitiesSatisfy(facts.Capabilities, requirement.CapabilityClass, requirement.RequiredCapabilities)
	case runtimecontract.PrerequisiteKindCapabilityState:
		for _, snapshot := range facts.Resources.CapabilityStates {
			if snapshot.OwnerModuleID == requirement.OwnerModuleID && snapshot.CapabilityStateKind == requirement.CapabilityStateKind && snapshot.SubjectRef == requirement.SubjectRef {
				return true
			}
		}
		return false
	case runtimecontract.PrerequisiteKindValidationScratch:
		return facts.Resources.ValidationScratch != nil && facts.Resources.ValidationScratch.Reference != "" && facts.Resources.ValidationScratch.Revision != ""
	case runtimecontract.PrerequisiteKindProviderCeiling:
		return facts.Resources.ProviderCeiling != nil && facts.Resources.ProviderCeiling.MaxBytes >= requirement.MinimumBytes
	case runtimecontract.PrerequisiteKindOperationAdmissibility:
		for _, inventory := range facts.Resources.OperationAdmissibilities {
			if inventory.PrerequisiteID == runtimePrerequisiteID(requirement) && inventory.OwnerModuleID == requirement.OwnerModuleID {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func mergeCapabilityKinds(existing []string, incoming []string) []string {
	ret := make([]string, 0, len(existing)+len(incoming))
	seen := map[string]struct{}{}
	for _, values := range [][]string{existing, incoming} {
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
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func capabilitiesSatisfy(capabilities runtimecontract.RuntimeCapabilities, class runtimecontract.RuntimeCapabilityClass, required []string) bool {
	var values []string
	switch class {
	case runtimecontract.RuntimeCapabilityClassStateChange:
		values = capabilities.StateChangeKinds
	case runtimecontract.RuntimeCapabilityClassValidation:
		values = capabilities.ValidationKinds
	case runtimecontract.RuntimeCapabilityClassGovernance:
		values = capabilities.GovernanceKinds
	default:
		return false
	}
	available := make(map[string]struct{}, len(values))
	for _, value := range values {
		available[value] = struct{}{}
	}
	for _, value := range required {
		if _, ok := available[value]; !ok {
			return false
		}
	}
	return len(required) > 0
}

func normalizeRuntimeRequirements(values []RuntimeRequirement) []RuntimeRequirement {
	byID := make(map[string]RuntimeRequirement, len(values))
	for _, value := range values {
		value.SubjectRef = strings.TrimSpace(value.SubjectRef)
		value.OwnerModuleID = strings.TrimSpace(value.OwnerModuleID)
		value.CapabilityClass = runtimecontract.RuntimeCapabilityClass(strings.TrimSpace(string(value.CapabilityClass)))
		value.RequiredCapabilities = normalizeRequirementStrings(value.RequiredCapabilities)
		value.CapabilityStateKind = strings.TrimSpace(value.CapabilityStateKind)
		value.Reason = strings.TrimSpace(value.Reason)
		if value.Kind == "" {
			continue
		}
		byID[runtimePrerequisiteID(value)] = value
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	ret := make([]RuntimeRequirement, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, byID[id])
	}
	return ret
}

func runtimePrerequisiteID(requirement RuntimeRequirement) string {
	parts := []string{string(requirement.Kind), requirement.OwnerModuleID, requirement.SubjectRef, string(requirement.CapabilityClass), strings.Join(requirement.RequiredCapabilities, ","), requirement.CapabilityStateKind, fmt.Sprintf("%d", requirement.MinimumBytes)}
	return strings.Join(parts, ":")
}

func normalizeRequirementStrings(values []string) []string {
	seen := map[string]struct{}{}
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
	return ret
}

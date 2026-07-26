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
	TargetSelection        *runtimecontract.RuntimeTargetSelection
	FailureTarget          *runtimecontract.FailureTarget
	Capabilities           *runtimecontract.RuntimeCapabilities
	ValidationScratch      *runtimecontract.ValidationScratch
	ProviderCeiling        *runtimecontract.ProviderCeiling
	CapabilityState        *runtimecontract.RuntimeCapabilityStateSnapshot
	OperationAdmissibility *runtimecontract.RuntimeOperationAdmissibility
	Observation            *runtimecontract.RuntimePrerequisiteObservation
}

func PreflightRuntimeFacts(facts runtimecontract.RuntimeFacts, requirements []RuntimeRequirement) runtimecontract.RuntimeFacts {
	facts = facts.Normalized()
	normalizedRequirements := normalizeRuntimeRequirements(requirements)
	pending := make([]runtimecontract.RuntimePrerequisite, 0, len(normalizedRequirements))
	reacquiring := make(map[string]struct{})
	for _, requirement := range normalizedRequirements {
		if runtimeRequirementSatisfied(facts, requirement) {
			continue
		}
		prerequisiteID := runtimePrerequisiteID(requirement)
		reacquiring[prerequisiteID] = struct{}{}
		pending = append(pending, runtimecontract.RuntimePrerequisite{
			ID:                          prerequisiteID,
			Kind:                        requirement.Kind,
			Scope:                       requirement.Scope,
			FocusID:                     requirement.FocusID,
			OwnerModuleID:               requirement.OwnerModuleID,
			SubjectRef:                  requirement.SubjectRef,
			TargetSelectorRef:           requirement.TargetSelectorRef,
			TargetSelectorOwnerRef:      requirement.TargetSelectorOwnerRef,
			TargetSelectorSubjectRef:    requirement.TargetSelectorSubjectRef,
			TargetSelectorCandidateRefs: requirement.TargetSelectorCandidateRefs,
			ProspectiveTargetRef:        requirement.ProspectiveTargetRef,
			CapabilityClass:             requirement.CapabilityClass,
			RequiredCapabilities:        requirement.RequiredCapabilities,
			CapabilityStateKind:         requirement.CapabilityStateKind,
			MinimumBytes:                requirement.MinimumBytes,
			Reason:                      requirement.Reason,
		})
	}
	if len(reacquiring) != 0 {
		// A resource may have been invalidated or cleared while its old
		// prerequisite identity is still persisted (for example, authority
		// reconciliation after a delayed Focus result). A prerequisite cannot
		// assert both "satisfied" and "must be acquired" in the same SM-2
		// authority. Remove the stale satisfaction and its active observation
		// before scheduling the replacement; retired lineage is reserved for a
		// completed Focus and must not be abused for current-Focus reacquisition.
		satisfied := make([]runtimecontract.RuntimePrerequisite, 0, len(facts.Prerequisites.Satisfied))
		for _, prerequisite := range facts.Prerequisites.Satisfied {
			if _, stale := reacquiring[prerequisite.ID]; stale {
				continue
			}
			satisfied = append(satisfied, prerequisite)
		}
		facts.Prerequisites.Satisfied = satisfied
		observations := make([]runtimecontract.RuntimePrerequisiteObservation, 0, len(facts.Resources.PrerequisiteObservations))
		for _, observation := range facts.Resources.PrerequisiteObservations {
			if _, stale := reacquiring[observation.PrerequisiteID]; stale {
				continue
			}
			observations = append(observations, observation)
		}
		facts.Resources.PrerequisiteObservations = observations
		// A FailureTarget is local current-failure authority rather than audit
		// history. If the same prerequisite identity must be reacquired because
		// the typed signature/target set changed, retaining its old resource
		// pointer would leave two contradictory repair surfaces in SM-2.
		for _, prerequisite := range pending {
			if prerequisite.Kind == runtimecontract.PrerequisiteKindFailureTarget {
				facts.Resources.FailureTarget = nil
				break
			}
		}
	}
	facts.Prerequisites.Pending = pending
	return facts.Normalized()
}

// runtimeTargetGroundingRequirements translates a sealed focus declaration
// into owner acquisitions. It never chooses a target from workspace content:
// explicit refs require File snapshots, while discovery source IDs only assert
// the owner facts that the profile declared as a bounded basis.
func runtimeTargetGroundingRequirements(facts runtimecontract.RuntimeFacts) []RuntimeRequirement {
	facts = facts.Normalized()
	focus := facts.Focus
	if !focus.RequireTargetGrounding {
		return nil
	}
	requirements := make([]RuntimeRequirement, 0, len(focus.TargetRefs)+len(focus.TargetDiscoverySources)+1)
	// Use the same retarget precedence as SM-3 candidate admission. A newly
	// committed owner failure observation must acquire snapshots for its repair
	// targets, never silently reuse an older explicit Focus target.
	groundedTargets := GroundedRepairTargets(facts)
	for _, target := range groundedTargets {
		if ownerGroundedCreationTarget(facts, target) {
			// An empty-workspace create target is a sealed absent path.  It is
			// concrete operation authority, but cannot and must not be read as a
			// File snapshot before the durable create has occurred.
			continue
		}
		requirements = append(requirements, focusScopedRuntimeRequirement(facts, RuntimeRequirement{
			Kind: runtimecontract.PrerequisiteKindTargetSnapshot, OwnerModuleID: runtimecontract.RuntimeModuleFile,
			SubjectRef: target, Reason: "grounded repair target requires owner snapshot",
		}))
	}
	// A failure target is already an owner-selected repair surface. Otherwise a
	// File owner may only make the neutral cardinality observation below; it
	// must not infer an implementation role from paths, suffixes, or content.
	if len(groundedTargets) == 0 {
		selection := currentFocusTargetSelection(facts)
		if selection == nil {
			for _, source := range focus.TargetDiscoverySources {
				switch source {
				case runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest, runtimecontract.RuntimeValidationDiscoverySourceProjectFiles:
					requirements = append(requirements, focusScopedRuntimeRequirement(facts, RuntimeRequirement{
						Kind: runtimecontract.PrerequisiteKindTargetSelection, OwnerModuleID: runtimecontract.RuntimeModuleFile,
						SubjectRef: focus.TargetRole, TargetSelectorRef: focus.TargetSelectorRef, TargetSelectorOwnerRef: focus.TargetSelectorOwnerRef, TargetSelectorSubjectRef: focus.TargetSelectorSubjectRef, TargetSelectorCandidateRefs: focus.TargetSelectorCandidateRefs, ProspectiveTargetRef: focus.ProspectiveTargetRef, Reason: "focus target grounding requires owner target selection",
					}))
					// One File-owner observation covers both file-backed source IDs.
					goto selectionRequirementDone
				}
			}
		} else if targetSelectionHasConcreteTarget(*selection) {
			for _, target := range selection.TargetRefs {
				requirements = append(requirements, focusScopedRuntimeRequirement(facts, RuntimeRequirement{
					Kind: runtimecontract.PrerequisiteKindTargetSnapshot, OwnerModuleID: runtimecontract.RuntimeModuleFile,
					SubjectRef: target, Reason: "selected focus target requires owner snapshot",
				}))
			}
		}
	}
selectionRequirementDone:
	for _, source := range focus.TargetDiscoverySources {
		switch source {
		case runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest, runtimecontract.RuntimeValidationDiscoverySourceProjectFiles:
			requirements = append(requirements, RuntimeRequirement{
				Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				Reason: "focus target grounding requires workspace owner discovery",
			})
		case runtimecontract.RuntimeValidationDiscoverySourceDeclaredCommands:
			requirements = append(requirements, RuntimeRequirement{
				Kind: runtimecontract.PrerequisiteKindValidationScratch, OwnerModuleID: runtimecontract.RuntimeModuleOrchestrator,
				Reason: "focus target grounding requires declared command discovery",
			})
		case runtimecontract.RuntimeValidationDiscoverySourceAvailableCapabilities:
			// Capability and admissibility requirements are activation-sealed for
			// every executable operation. The core must not invent an owner here.
		}
	}
	return normalizeRuntimeRequirements(requirements)
}

func ownerGroundedCreationTarget(facts runtimecontract.RuntimeFacts, target string) bool {
	selection := currentFocusTargetSelection(facts)
	return hasOwnerGroundedCreationScope(facts) && selection != nil &&
		strings.TrimSpace(selection.ProspectiveTargetRef) == strings.TrimSpace(target)
}

// currentFocusTargetSelection returns only a File-owner selection committed as
// a satisfied Focus-scoped prerequisite for the current Focus.  A raw resource
// pointer is not authority by itself: it can be historical audit lineage or a
// malformed aggregate and must never skip a current acquisition.
func currentFocusTargetSelection(facts runtimecontract.RuntimeFacts) *runtimecontract.RuntimeTargetSelection {
	facts = facts.Normalized()
	if facts.Resources.TargetSelection == nil {
		return nil
	}
	selection := facts.Resources.TargetSelection.Normalized()
	for _, prerequisite := range facts.Prerequisites.Satisfied {
		if prerequisite.ID == selection.PrerequisiteID && prerequisite.Kind == runtimecontract.PrerequisiteKindTargetSelection &&
			prerequisite.Scope == runtimecontract.RuntimePrerequisiteScopeFocus && prerequisite.FocusID == facts.Focus.ID {
			return &selection
		}
	}
	return nil
}

func focusScopedRuntimeRequirement(facts runtimecontract.RuntimeFacts, requirement RuntimeRequirement) RuntimeRequirement {
	requirement.Scope = runtimecontract.RuntimePrerequisiteScopeFocus
	requirement.FocusID = facts.Focus.ID
	return requirement
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
	if result.Observation != nil {
		observation := result.Observation.Normalized()
		if observation.PrerequisiteID != completed.ID || observation.Kind != completed.Kind ||
			(completed.OwnerModuleID != "" && observation.OwnerModuleID != completed.OwnerModuleID) {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("prerequisite observation does not match completed prerequisite %q", completed.ID)
		}
		if err := observation.Validate(); err != nil {
			return runtimecontract.RuntimeFacts{}, fmt.Errorf("invalid prerequisite observation: %w", err)
		}
		facts.Resources.PrerequisiteObservations = append(facts.Resources.PrerequisiteObservations, observation)
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
	case runtimecontract.PrerequisiteKindTargetSelection:
		if result.TargetSelection == nil {
			return fmt.Errorf("target selection prerequisite returned no selection")
		}
		selection := result.TargetSelection.Normalized()
		if selection.PrerequisiteID != prerequisite.ID || selection.OwnerModuleID != prerequisite.OwnerModuleID || selection.TargetRole != prerequisite.SubjectRef || selection.SelectorRef != prerequisite.TargetSelectorRef || selection.SelectorOwnerRef != prerequisite.TargetSelectorOwnerRef || selection.SelectorSubjectRef != prerequisite.TargetSelectorSubjectRef || !runtimecontract.SameRuntimeTargetSelectorCandidateRefs(selection.SelectorCandidateRefs, prerequisite.TargetSelectorCandidateRefs) || selection.ProspectiveTargetRef != prerequisite.ProspectiveTargetRef {
			return fmt.Errorf("target selection prerequisite returned a mismatched selection")
		}
		if err := selection.Validate(); err != nil {
			return fmt.Errorf("target selection prerequisite returned invalid selection: %w", err)
		}
		facts.Resources.TargetSelection = &selection
	case runtimecontract.PrerequisiteKindFailureTarget:
		if facts.Failure == nil || strings.TrimSpace(facts.Failure.Signature) == "" || len(facts.Failure.TargetRefs) == 0 {
			return fmt.Errorf("failure target prerequisite has no typed failure identity")
		}
		if result.FailureTarget == nil || strings.TrimSpace(result.FailureTarget.Signature) == "" || len(result.FailureTarget.TargetRefs) == 0 {
			return fmt.Errorf("failure target prerequisite returned no typed target identity")
		}
		failure := facts.Failure.Normalized()
		if strings.TrimSpace(result.FailureTarget.Signature) != failure.Signature || !sameRuntimeTargetRefs(result.FailureTarget.TargetRefs, failure.TargetRefs) {
			return fmt.Errorf("failure target prerequisite does not match current typed failure")
		}
		target := *result.FailureTarget
		target.Signature = strings.TrimSpace(target.Signature)
		target.TargetRefs = normalizeRuntimeTargetRefs(target.TargetRefs)
		facts.Resources.FailureTarget = &target
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
		if inventory.AcquisitionBinding != "" && !runtimeCapabilityStateBindingExists(facts.Resources.CapabilityStates, inventory.OwnerModuleID, inventory.AcquisitionBinding) {
			return fmt.Errorf("operation admissibility inventory references an unknown owner capability-state acquisition binding")
		}
		// Owner inventories establish admissibility facts for SM-2 and must
		// remain complete enough to cover a target produced by an accepted
		// earlier operation.  They are not the per-request candidate-context
		// projection controlled by MaxCandidateContextItems.
		maxItems := runtimecontract.RuntimeOwnerInventoryItemsMaxCount
		if maxItems < 1 || len(inventory.TargetRefs) > maxItems || len(inventory.SelectorRefs) > maxItems {
			return fmt.Errorf("operation admissibility inventory exceeds owner inventory bound")
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

func runtimeCapabilityStateBindingExists(values []runtimecontract.RuntimeCapabilityStateSnapshot, owner, binding string) bool {
	for _, snapshot := range values {
		if snapshot.OwnerModuleID != owner {
			continue
		}
		if snapshot.AcquisitionBinding == binding || (snapshot.AcquisitionBinding == "" && snapshot.AcquisitionKey == binding) {
			return true
		}
	}
	return false
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
	case runtimecontract.PrerequisiteKindTargetSelection:
		selection := facts.Resources.TargetSelection
		if selection == nil || selection.PrerequisiteID != runtimePrerequisiteID(requirement) {
			return false
		}
		if requirement.Scope != runtimecontract.RuntimePrerequisiteScopeFocus {
			return true
		}
		return currentFocusTargetSelection(facts) != nil
	case runtimecontract.PrerequisiteKindFailureTarget:
		if facts.Failure == nil || facts.Resources.FailureTarget == nil || strings.TrimSpace(facts.Failure.Signature) == "" || len(facts.Failure.TargetRefs) == 0 {
			return false
		}
		target := *facts.Resources.FailureTarget
		target.Signature = strings.TrimSpace(target.Signature)
		failure := facts.Failure.Normalized()
		return target.Signature == failure.Signature && sameRuntimeTargetRefs(target.TargetRefs, failure.TargetRefs)
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
		value.Scope = runtimecontract.RuntimePrerequisiteScope(strings.TrimSpace(string(value.Scope)))
		if value.Scope == "" {
			value.Scope = runtimecontract.RuntimePrerequisiteScopeGlobal
		}
		value.FocusID = strings.TrimSpace(value.FocusID)
		if value.Scope != runtimecontract.RuntimePrerequisiteScopeFocus {
			value.Scope = runtimecontract.RuntimePrerequisiteScopeGlobal
			value.FocusID = ""
		}
		value.TargetSelectorRef = strings.TrimSpace(value.TargetSelectorRef)
		value.TargetSelectorOwnerRef = strings.TrimSpace(value.TargetSelectorOwnerRef)
		value.TargetSelectorSubjectRef = strings.TrimSpace(value.TargetSelectorSubjectRef)
		value.TargetSelectorCandidateRefs = normalizeRequirementStrings(value.TargetSelectorCandidateRefs)
		value.ProspectiveTargetRef = strings.TrimSpace(value.ProspectiveTargetRef)
		value.OwnerModuleID = strings.TrimSpace(value.OwnerModuleID)
		if value.Kind == runtimecontract.PrerequisiteKindValidationScratch && value.OwnerModuleID == "" {
			// Scratch is an Orchestrator-local prerequisite. Recording its owner
			// in the normalized requirement makes its observation/invalidation
			// binding explicit rather than treating a local resource as ownerless.
			value.OwnerModuleID = runtimecontract.RuntimeModuleOrchestrator
		}
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
	parts := []string{string(requirement.Kind), string(requirement.Scope), requirement.FocusID, requirement.OwnerModuleID, requirement.SubjectRef, requirement.TargetSelectorRef, requirement.TargetSelectorOwnerRef, requirement.TargetSelectorSubjectRef, strings.Join(requirement.TargetSelectorCandidateRefs, ","), requirement.ProspectiveTargetRef, string(requirement.CapabilityClass), strings.Join(requirement.RequiredCapabilities, ","), requirement.CapabilityStateKind, fmt.Sprintf("%d", requirement.MinimumBytes)}
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

package runtime

import (
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz/internal/grounding"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

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

func PreflightRuntimeFacts(facts runtimecontract.RuntimeFacts, requirements []runtimecontract.RuntimeRequirement) runtimecontract.RuntimeFacts {
	facts = facts.Normalized()
	normalizedRequirements := normalizeRuntimeRequirements(requirements)
	normalizedRequirements = runtimeRequirementsWithShellValidationManifest(facts, normalizedRequirements)
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

// runtimeRequirementsWithShellValidationManifest seals the resource
// dependency implied by a declared managed-validation recipe.  The rule is
// owner-neutral orchestration: File still owns the manifest and Shell still
// decides recipe applicability.  SM-2 only ensures the immutable File fact is
// acquired before asking Shell for a workspace-sensitive catalog.
func runtimeRequirementsWithShellValidationManifest(facts runtimecontract.RuntimeFacts, requirements []runtimecontract.RuntimeRequirement) []runtimecontract.RuntimeRequirement {
	if len(runtimecontract.RuntimeValidationRecipeDeclarationsFromEvidencePolicies(facts.Activation.EvidencePolicies)) == 0 {
		return requirements
	}
	requiresManifest := false
	for _, requirement := range requirements {
		if requirement.Kind == runtimecontract.PrerequisiteKindOperationAdmissibility &&
			requirement.OwnerModuleID == runtimecontract.RuntimeModuleShell &&
			runtimeRequirementKindsContain(requirement.RequiredCapabilities, runtimecontract.RuntimeBlockKindManagedValidationSession) {
			requiresManifest = true
			break
		}
	}
	if !requiresManifest {
		return requirements
	}
	return normalizeRuntimeRequirements(append(append([]runtimecontract.RuntimeRequirement(nil), requirements...), runtimecontract.RuntimeRequirement{
		Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile,
		Reason: "Shell managed-validation admissibility requires File workspace manifest",
	}))
}

func runtimeRequirementKindsContain(values []string, wanted string) bool {
	wanted = runtimecontract.CanonicalRuntimeBlockKind(wanted)
	for _, value := range values {
		if runtimecontract.CanonicalRuntimeBlockKind(value) == wanted {
			return true
		}
	}
	return false
}

// runtimeTargetGroundingRequirements translates a sealed focus declaration
// into owner acquisitions. It never chooses a target from workspace content:
// explicit refs require File snapshots, while discovery source IDs only assert
// the owner facts that the profile declared as a bounded basis.
func runtimeTargetGroundingRequirements(facts runtimecontract.RuntimeFacts) []runtimecontract.RuntimeRequirement {
	facts = facts.Normalized()
	focus := facts.Focus
	if !focus.RequireTargetGrounding {
		return nil
	}
	requirements := make([]runtimecontract.RuntimeRequirement, 0, len(focus.TargetRefs)+len(focus.TargetDiscoverySources)+1)
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
		requirements = append(requirements, focusScopedRuntimeRequirement(facts, runtimecontract.RuntimeRequirement{
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
			// A File-owned prospective selector is itself a sealed target
			// acquisition requirement.  Semantic planning may provide the
			// selector/prospective declaration without a discovery-source hint;
			// that must not let SM-2 skip the File observation and reach the
			// operations provider with an ungrounded target.  File remains the
			// sole owner of existence/path-boundary facts; this branch only
			// schedules its typed prerequisite.
			if focus.TargetSelectorOwnerRef == runtimecontract.RuntimeModuleFile &&
				focus.TargetSelectorRef != "" && focus.ProspectiveTargetRef != "" {
				requirements = append(requirements, focusScopedRuntimeRequirement(facts, runtimecontract.RuntimeRequirement{
					Kind: runtimecontract.PrerequisiteKindTargetSelection, OwnerModuleID: runtimecontract.RuntimeModuleFile,
					SubjectRef: focus.TargetRole, TargetSelectorRef: focus.TargetSelectorRef,
					TargetSelectorOwnerRef: focus.TargetSelectorOwnerRef, TargetSelectorSubjectRef: focus.TargetSelectorSubjectRef,
					TargetSelectorCandidateRefs: focus.TargetSelectorCandidateRefs, ProspectiveTargetRef: focus.ProspectiveTargetRef,
					Reason: "prospective File target requires owner target selection",
				}))
				goto selectionRequirementDone
			}
			for _, source := range focus.TargetDiscoverySources {
				switch source {
				case runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest, runtimecontract.RuntimeValidationDiscoverySourceProjectFiles:
					requirements = append(requirements, focusScopedRuntimeRequirement(facts, runtimecontract.RuntimeRequirement{
						Kind: runtimecontract.PrerequisiteKindTargetSelection, OwnerModuleID: runtimecontract.RuntimeModuleFile,
						SubjectRef: focus.TargetRole, TargetSelectorRef: focus.TargetSelectorRef, TargetSelectorOwnerRef: focus.TargetSelectorOwnerRef, TargetSelectorSubjectRef: focus.TargetSelectorSubjectRef, TargetSelectorCandidateRefs: focus.TargetSelectorCandidateRefs, ProspectiveTargetRef: focus.ProspectiveTargetRef, Reason: "focus target grounding requires owner target selection",
					}))
					// One File-owner observation covers both file-backed source IDs.
					goto selectionRequirementDone
				}
			}
		} else if targetSelectionHasConcreteTarget(*selection) {
			for _, target := range selection.TargetRefs {
				requirements = append(requirements, focusScopedRuntimeRequirement(facts, runtimecontract.RuntimeRequirement{
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
			requirements = append(requirements, runtimecontract.RuntimeRequirement{
				Kind: runtimecontract.PrerequisiteKindManifest, OwnerModuleID: runtimecontract.RuntimeModuleFile,
				Reason: "focus target grounding requires workspace owner discovery",
			})
		case runtimecontract.RuntimeValidationDiscoverySourceDeclaredCommands:
			requirements = append(requirements, runtimecontract.RuntimeRequirement{
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
	return grounding.CurrentFocusTargetSelection(facts)
}

func focusScopedRuntimeRequirement(facts runtimecontract.RuntimeFacts, requirement runtimecontract.RuntimeRequirement) runtimecontract.RuntimeRequirement {
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
		return applyManifestPrerequisiteResult(facts, result)
	case runtimecontract.PrerequisiteKindTargetSnapshot:
		return applyTargetSnapshotPrerequisiteResult(facts, prerequisite, result)
	case runtimecontract.PrerequisiteKindTargetSelection:
		return applyTargetSelectionPrerequisiteResult(facts, prerequisite, result)
	case runtimecontract.PrerequisiteKindFailureTarget:
		return applyFailureTargetPrerequisiteResult(facts, result)
	case runtimecontract.PrerequisiteKindCapability:
		return applyCapabilityPrerequisiteResult(facts, prerequisite, result)
	case runtimecontract.PrerequisiteKindCapabilityState:
		return applyCapabilityStatePrerequisiteResult(facts, prerequisite, result)
	case runtimecontract.PrerequisiteKindValidationScratch:
		return applyValidationScratchPrerequisiteResult(facts, result)
	case runtimecontract.PrerequisiteKindProviderCeiling:
		return applyProviderCeilingPrerequisiteResult(facts, prerequisite, result)
	case runtimecontract.PrerequisiteKindOperationAdmissibility:
		return applyOperationAdmissibilityPrerequisiteResult(facts, prerequisite, result)
	default:
		return fmt.Errorf("unsupported prerequisite kind %q", prerequisite.Kind)
	}
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

func runtimeRequirementSatisfied(facts runtimecontract.RuntimeFacts, requirement runtimecontract.RuntimeRequirement) bool {
	switch requirement.Kind {
	case runtimecontract.PrerequisiteKindManifest:
		return facts.Resources.Manifest != nil && facts.Resources.Manifest.Revision != ""
	case runtimecontract.PrerequisiteKindTargetSnapshot:
		return runtimeTargetSnapshotRequirementSatisfied(facts, requirement)
	case runtimecontract.PrerequisiteKindTargetSelection:
		return runtimeTargetSelectionRequirementSatisfied(facts, requirement)
	case runtimecontract.PrerequisiteKindFailureTarget:
		return runtimeFailureTargetRequirementSatisfied(facts)
	case runtimecontract.PrerequisiteKindCapability:
		return capabilitiesSatisfy(facts.Capabilities, requirement.CapabilityClass, requirement.RequiredCapabilities)
	case runtimecontract.PrerequisiteKindCapabilityState:
		return runtimeCapabilityStateRequirementSatisfied(facts, requirement)
	case runtimecontract.PrerequisiteKindValidationScratch:
		return facts.Resources.ValidationScratch != nil && facts.Resources.ValidationScratch.Reference != "" && facts.Resources.ValidationScratch.Revision != ""
	case runtimecontract.PrerequisiteKindProviderCeiling:
		return facts.Resources.ProviderCeiling != nil && facts.Resources.ProviderCeiling.MaxBytes >= requirement.MinimumBytes
	case runtimecontract.PrerequisiteKindOperationAdmissibility:
		return runtimeOperationAdmissibilityRequirementSatisfied(facts, requirement)
	default:
		return false
	}
}

func runtimeTargetSnapshotRequirementSatisfied(facts runtimecontract.RuntimeFacts, requirement runtimecontract.RuntimeRequirement) bool {
	for _, snapshot := range facts.Resources.TargetSnapshots {
		if snapshot.TargetRef == requirement.SubjectRef && snapshot.Complete() {
			return true
		}
	}
	return false
}

func runtimeTargetSelectionRequirementSatisfied(facts runtimecontract.RuntimeFacts, requirement runtimecontract.RuntimeRequirement) bool {
	selection := facts.Resources.TargetSelection
	if selection == nil || selection.PrerequisiteID != runtimePrerequisiteID(requirement) {
		return false
	}
	if requirement.Scope != runtimecontract.RuntimePrerequisiteScopeFocus {
		return true
	}
	return currentFocusTargetSelection(facts) != nil
}

func runtimeFailureTargetRequirementSatisfied(facts runtimecontract.RuntimeFacts) bool {
	if facts.Failure == nil || facts.Resources.FailureTarget == nil || strings.TrimSpace(facts.Failure.Signature) == "" || len(facts.Failure.TargetRefs) == 0 {
		return false
	}
	target := *facts.Resources.FailureTarget
	target.Signature = strings.TrimSpace(target.Signature)
	failure := facts.Failure.Normalized()
	return target.Signature == failure.Signature && sameRuntimeTargetRefs(target.TargetRefs, failure.TargetRefs)
}

func runtimeCapabilityStateRequirementSatisfied(facts runtimecontract.RuntimeFacts, requirement runtimecontract.RuntimeRequirement) bool {
	for _, snapshot := range facts.Resources.CapabilityStates {
		if snapshot.OwnerModuleID == requirement.OwnerModuleID && snapshot.CapabilityStateKind == requirement.CapabilityStateKind && snapshot.SubjectRef == requirement.SubjectRef {
			return true
		}
	}
	return false
}

func runtimeOperationAdmissibilityRequirementSatisfied(facts runtimecontract.RuntimeFacts, requirement runtimecontract.RuntimeRequirement) bool {
	prerequisiteID := runtimePrerequisiteID(requirement)
	for _, inventory := range facts.Resources.OperationAdmissibilities {
		if inventory.PrerequisiteID == prerequisiteID && inventory.OwnerModuleID == requirement.OwnerModuleID {
			return true
		}
	}
	return false
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

func normalizeRuntimeRequirements(values []runtimecontract.RuntimeRequirement) []runtimecontract.RuntimeRequirement {
	byID := make(map[string]runtimecontract.RuntimeRequirement, len(values))
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
	ret := make([]runtimecontract.RuntimeRequirement, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, byID[id])
	}
	return ret
}

func runtimePrerequisiteID(requirement runtimecontract.RuntimeRequirement) string {
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

package runtime

import (
	"fmt"
	"strings"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func applyManifestPrerequisiteResult(facts *runtimecontract.RuntimeFacts, result PrerequisiteResult) error {
	if result.Manifest == nil || strings.TrimSpace(result.Manifest.Revision) == "" {
		return fmt.Errorf("manifest prerequisite returned no manifest")
	}
	facts.Resources.Manifest = result.Manifest
	return nil
}

func applyTargetSnapshotPrerequisiteResult(facts *runtimecontract.RuntimeFacts, prerequisite runtimecontract.RuntimePrerequisite, result PrerequisiteResult) error {
	if result.TargetSnapshot == nil || result.TargetSnapshot.TargetRef != prerequisite.SubjectRef || !result.TargetSnapshot.Complete() {
		return fmt.Errorf("target snapshot prerequisite returned no complete snapshot for %q", prerequisite.SubjectRef)
	}
	facts.Resources.TargetSnapshots = append(facts.Resources.TargetSnapshots, *result.TargetSnapshot)
	return nil
}

func applyTargetSelectionPrerequisiteResult(facts *runtimecontract.RuntimeFacts, prerequisite runtimecontract.RuntimePrerequisite, result PrerequisiteResult) error {
	if result.TargetSelection == nil {
		return fmt.Errorf("target selection prerequisite returned no selection")
	}
	selection := result.TargetSelection.Normalized()
	if selection.PrerequisiteID != prerequisite.ID || selection.OwnerModuleID != prerequisite.OwnerModuleID || selection.TargetRole != prerequisite.SubjectRef || selection.SelectorRef != prerequisite.TargetSelectorRef || selection.SelectorOwnerRef != prerequisite.TargetSelectorOwnerRef || selection.SelectorSubjectRef != prerequisite.TargetSelectorSubjectRef || !runtimecontract.SameRuntimeTargetSelectorCandidateRefs(selection.SelectorCandidateRefs, prerequisite.TargetSelectorCandidateRefs) || !runtimecontract.SameRuntimeProspectiveTargetAllOfConstraintRefs(selection.ProspectiveTargetAllOfConstraintRefs, prerequisite.ProspectiveTargetAllOfConstraintRefs) || selection.ProspectiveTargetRef != prerequisite.ProspectiveTargetRef {
		return fmt.Errorf("target selection prerequisite returned a mismatched selection")
	}
	if err := selection.Validate(); err != nil {
		return fmt.Errorf("target selection prerequisite returned invalid selection: %w", err)
	}
	facts.Resources.TargetSelection = &selection
	return nil
}

func applyCapabilityPrerequisiteResult(facts *runtimecontract.RuntimeFacts, prerequisite runtimecontract.RuntimePrerequisite, result PrerequisiteResult) error {
	if result.Capabilities == nil || !capabilitiesSatisfy(*result.Capabilities, prerequisite.CapabilityClass, prerequisite.RequiredCapabilities) {
		return fmt.Errorf("capability prerequisite returned no required capabilities")
	}
	// Owner merges are additive and must not replace another owner's sealed kinds.
	facts.Capabilities.StateChangeKinds = mergeCapabilityKinds(facts.Capabilities.StateChangeKinds, result.Capabilities.StateChangeKinds)
	facts.Capabilities.ValidationKinds = mergeCapabilityKinds(facts.Capabilities.ValidationKinds, result.Capabilities.ValidationKinds)
	facts.Capabilities.GovernanceKinds = mergeCapabilityKinds(facts.Capabilities.GovernanceKinds, result.Capabilities.GovernanceKinds)
	mergedPrerequisiteFacts, err := runtimecontract.MergeRuntimeCapabilityPrerequisiteFacts(facts.Capabilities.PrerequisiteFacts, result.Capabilities.PrerequisiteFacts)
	if err != nil {
		return fmt.Errorf("capability prerequisite returned conflicting resource facts: %w", err)
	}
	facts.Capabilities.PrerequisiteFacts = mergedPrerequisiteFacts
	return nil
}

func applyCapabilityStatePrerequisiteResult(facts *runtimecontract.RuntimeFacts, prerequisite runtimecontract.RuntimePrerequisite, result PrerequisiteResult) error {
	if result.CapabilityState == nil || result.CapabilityState.RequirementID != prerequisite.ID || result.CapabilityState.OwnerModuleID != prerequisite.OwnerModuleID || result.CapabilityState.CapabilityStateKind != prerequisite.CapabilityStateKind || result.CapabilityState.SubjectRef != prerequisite.SubjectRef {
		return fmt.Errorf("capability-state prerequisite returned a mismatched snapshot")
	}
	facts.Resources.CapabilityStates = append(facts.Resources.CapabilityStates, *result.CapabilityState)
	return nil
}

func applyValidationScratchPrerequisiteResult(facts *runtimecontract.RuntimeFacts, result PrerequisiteResult) error {
	if result.ValidationScratch == nil || strings.TrimSpace(result.ValidationScratch.Reference) == "" || strings.TrimSpace(result.ValidationScratch.Revision) == "" {
		return fmt.Errorf("validation scratch prerequisite returned no scratch reference")
	}
	facts.Resources.ValidationScratch = result.ValidationScratch
	return nil
}

func applyProviderCeilingPrerequisiteResult(facts *runtimecontract.RuntimeFacts, prerequisite runtimecontract.RuntimePrerequisite, result PrerequisiteResult) error {
	if result.ProviderCeiling == nil {
		return fmt.Errorf("provider ceiling prerequisite returned no ceiling")
	}
	if err := result.ProviderCeiling.Validate(); err != nil {
		return fmt.Errorf("provider ceiling prerequisite returned invalid ceiling: %w", err)
	}
	if result.ProviderCeiling.MaxBytes < prerequisite.MinimumBytes {
		return fmt.Errorf("provider ceiling prerequisite returned an insufficient ceiling")
	}
	if target := facts.Activation.ProviderSelection.Target.Normalized(); target.ProviderID != "" || target.ModelID != "" {
		if !result.ProviderCeiling.Selection.Target.Same(target) {
			return fmt.Errorf("provider ceiling target does not match activation target")
		}
	}
	facts.Resources.ProviderCeiling = result.ProviderCeiling
	return nil
}

func applyOperationAdmissibilityPrerequisiteResult(facts *runtimecontract.RuntimeFacts, prerequisite runtimecontract.RuntimePrerequisite, result PrerequisiteResult) error {
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
	// Owner inventories establish complete SM-2 facts; they are not bounded by
	// the provider-facing candidate context projection.
	maxItems := runtimecontract.DefaultRuntimeOwnerInventoryBudget().MaxItems
	if maxItems < 1 || len(inventory.TargetRefs) > maxItems || len(inventory.SelectorRefs) > maxItems {
		return fmt.Errorf("operation admissibility inventory exceeds owner inventory bound")
	}
	if missing := missingRequiredAdmissibilityKind(prerequisite.RequiredCapabilities, inventory.OperationKinds); missing != "" {
		return fmt.Errorf("operation admissibility inventory does not include required kind %q", missing)
	}
	facts.Resources.OperationAdmissibilities = append(facts.Resources.OperationAdmissibilities, inventory)
	return nil
}

func missingRequiredAdmissibilityKind(requiredCapabilities, availableKinds []string) string {
	available := make(map[string]struct{}, len(availableKinds))
	for _, kind := range availableKinds {
		available[kind] = struct{}{}
	}
	for _, kind := range requiredCapabilities {
		canonical := runtimecontract.CanonicalRuntimeBlockKind(kind)
		if canonical == "" {
			continue
		}
		if _, ok := available[canonical]; !ok {
			return canonical
		}
	}
	return ""
}

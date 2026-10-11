package workorchruntimecontract

import "fmt"

// 保留 e1b1f9fc 的独立旧校验路径，不调用新增的私有校验产物 helper。
func referenceRuntimeOperationAdmissibilityValidate(a RuntimeOperationAdmissibility) error {
	if err := ValidateRuntimeValidationRecipeProjection(a.ValidationRecipes); err != nil {
		return fmt.Errorf("runtime operation admissibility validation recipes: %w", err)
	}
	// Preserve duplicate variant facts until validation. Silently canonicalizing
	// them would make two conflicting owner declarations replay as one fact.
	seenVariants := make(map[string]struct{}, len(a.OperationVariants))
	for _, raw := range a.OperationVariants {
		value := raw.Normalized()
		if value.Kind == "" {
			return fmt.Errorf("runtime operation admissibility has a variant fact without operation kind")
		}
		key := value.Kind + "|" + value.VariantID
		if _, duplicate := seenVariants[key]; duplicate {
			return fmt.Errorf("runtime operation admissibility has duplicate variant fact %q/%q", value.Kind, value.VariantID)
		}
		seenVariants[key] = struct{}{}
	}
	seenSurfaces := make(map[string]struct{}, len(a.ValidationSurfaces))
	reviewInputVersions := make(map[string]string)
	for _, raw := range a.ValidationSurfaces {
		if err := raw.Validate(); err != nil {
			return err
		}
		value := raw.Normalized()
		if value.ID == "" {
			return fmt.Errorf("runtime operation admissibility has a validation surface without identity")
		}
		if _, duplicate := seenSurfaces[value.ID]; duplicate {
			return fmt.Errorf("runtime operation admissibility duplicates validation surface %q", value.ID)
		}
		seenSurfaces[value.ID] = struct{}{}
		recipe, _ := RuntimeValidationRecipeForID(a.ValidationRecipes, value.ID)
		for _, binding := range raw.ProofRequirementBindings {
			if recipe.InputReviewProtocolRef != "" && (binding.InputReview == nil || binding.InputReview.ProtocolRef != recipe.InputReviewProtocolRef) {
				return fmt.Errorf("runtime owner catalog downgraded the sealed input review protocol")
			}
			if binding.InputReview != nil && binding.InputReview.ProtocolRef == RuntimeValidationInputReviewProtocolV2 && recipe.InputReviewProtocolRef != RuntimeValidationInputReviewProtocolV2 {
				return fmt.Errorf("runtime owner catalog introduced an undeclared v2 input review protocol")
			}
			if binding.InputReview == nil {
				continue
			}
			for _, file := range binding.InputReview.Files {
				if prior, found := reviewInputVersions[file.TargetRef]; found && prior != file.ContentHash {
					return fmt.Errorf("runtime owner catalog has conflicting review input versions for %q", file.TargetRef)
				}
				reviewInputVersions[file.TargetRef] = file.ContentHash
			}
		}
	}
	if err := ValidateRuntimeOperationPolicyFacts(a.OperationPolicies); err != nil {
		return err
	}
	a = a.Normalized()
	if err := referenceRuntimeTargetParameterBindingsValidate(a.TargetParameterBindings, a.OwnerModuleID, a.OperationKinds, a.TargetRefs); err != nil {
		return err
	}
	if a.PrerequisiteID == "" || a.SnapshotID == "" || a.OwnerModuleID == "" || len(a.OperationKinds) == 0 || a.ObservedRevision == "" {
		return fmt.Errorf("runtime operation admissibility requires identity, owner, kinds and revision")
	}
	for _, kind := range a.OperationKinds {
		descriptor, ok := RuntimeOperationDescriptorForKind(kind)
		if !ok || descriptor.OwnerModuleID != a.OwnerModuleID {
			return fmt.Errorf("runtime operation admissibility kind %q does not belong to owner %q", kind, a.OwnerModuleID)
		}
	}
	for _, variant := range a.OperationVariants {
		if err := variant.Validate(a.OwnerModuleID, a.OperationKinds, a.SelectorRefs, a.ConstraintRefs); err != nil {
			return err
		}
	}
	if len(a.ValidationRecipes) > 0 {
		if a.OwnerModuleID != RuntimeModuleShell || !runtimeOperationKindsContain(a.OperationKinds, RuntimeBlockKindManagedValidationSession) {
			return fmt.Errorf("runtime operation admissibility validation recipes require Shell managed-validation inventory")
		}
	}
	if len(a.ValidationSurfaces) > 0 {
		for _, surface := range a.ValidationSurfaces {
			if surface.OwnerModuleID != a.OwnerModuleID || !runtimeOperationKindsContain(a.OperationKinds, surface.OperationKind) {
				return fmt.Errorf("runtime validation surface %q is outside owner operation inventory", surface.ID)
			}
			if surface.ProspectiveTargetSatisfactionConstraint != nil {
				constraint := surface.ProspectiveTargetSatisfactionConstraint.Normalized()
				if constraint.CatalogObservedRevision != a.ObservedRevision {
					return fmt.Errorf("runtime validation surface %q prospective target constraint catalog revision mismatch", surface.ID)
				}
			}
			for _, preparation := range surface.ProofRequirementPreparations {
				constraint := preparation.ProspectiveTargetSatisfactionConstraint.Normalized()
				if constraint.CatalogObservedRevision != a.ObservedRevision {
					return fmt.Errorf("runtime validation surface %q proof preparation catalog revision mismatch", surface.ID)
				}
			}
			for _, unavailable := range surface.ProofRequirementUnavailabilities {
				if unavailable.OriginalDiagnostic == nil || unavailable.OriginalDiagnostic.ObservedRevision != a.ObservedRevision {
					return fmt.Errorf("runtime validation surface %q proof unavailability catalog revision mismatch", surface.ID)
				}
			}
		}
		if selectable := runtimeSelectableValidationRecipeRefsCanonical(a.ValidationSurfaces); !sameStrings(selectable, a.ValidationRecipeRefs) {
			return fmt.Errorf("runtime operation admissibility validation recipe refs do not match selectable validation surfaces")
		}
	}
	for _, policy := range a.OperationPolicies {
		if policy.OwnerModuleID != a.OwnerModuleID || !runtimeOperationKindsContain(a.OperationKinds, policy.OperationKind) {
			return fmt.Errorf("runtime operation policy fact %q is outside owner operation inventory", policy.ID)
		}
	}
	if expected := stableRuntimeOperationAdmissibilitySnapshotIDCanonical(a); a.SnapshotID != expected {
		return fmt.Errorf("runtime operation admissibility snapshot identity mismatch")
	}
	return nil
}

package workorchruntimecontract

import (
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// 保留修改前完整 surface 校验及拒绝顺序，供独立旧路径对照。
func referenceRuntimeValidationSurfaceValidate(s RuntimeValidationSurface) error {
	raw := s
	if err := validateRuntimeValidationInputTargets(raw); err != nil {
		return err
	}
	if duplicate := duplicateRuntimeValidationProofCheckRef(raw.CheckRefs); duplicate != "" {
		return fmt.Errorf("runtime validation surface %q duplicates check %q", strings.TrimSpace(raw.ID), duplicate)
	}
	if err := ValidateRuntimeValidationProofRequirementBindings(raw.ProofRequirementBindings); err != nil {
		return fmt.Errorf("runtime validation surface %q: %w", strings.TrimSpace(raw.ID), err)
	}
	if err := ValidateRuntimeValidationProofPreparations(raw.ProofRequirementPreparations); err != nil {
		return fmt.Errorf("runtime validation surface %q: %w", strings.TrimSpace(raw.ID), err)
	}
	seenRequirementHashes := make(map[string]struct{}, len(raw.ProofRequirementBindings))
	seenRequirementCoverage := make(map[common.CoverageWorkItemID]struct{}, len(raw.ProofRequirementBindings))
	for _, binding := range raw.ProofRequirementBindings {
		binding = binding.Normalized()
		if _, duplicate := seenRequirementHashes[binding.RequirementHash]; duplicate {
			return fmt.Errorf("runtime validation surface %q duplicates proof requirement %q", strings.TrimSpace(raw.ID), binding.RequirementHash)
		}
		if _, duplicate := seenRequirementCoverage[binding.CoverageWorkItemID]; duplicate {
			return fmt.Errorf("runtime validation surface %q duplicates proof requirement coverage %q", strings.TrimSpace(raw.ID), binding.CoverageWorkItemID)
		}
		seenRequirementHashes[binding.RequirementHash] = struct{}{}
		seenRequirementCoverage[binding.CoverageWorkItemID] = struct{}{}
	}
	for _, preparation := range raw.ProofRequirementPreparations {
		preparation = preparation.Normalized()
		if _, duplicate := seenRequirementHashes[preparation.RequirementHash]; duplicate {
			return fmt.Errorf("runtime validation surface %q publishes proof requirement %q as both ready and prerequisite", strings.TrimSpace(raw.ID), preparation.RequirementHash)
		}
		if _, duplicate := seenRequirementCoverage[preparation.CoverageWorkItemID]; duplicate {
			return fmt.Errorf("runtime validation surface %q publishes proof requirement coverage %q more than once", strings.TrimSpace(raw.ID), preparation.CoverageWorkItemID)
		}
		seenRequirementHashes[preparation.RequirementHash] = struct{}{}
		seenRequirementCoverage[preparation.CoverageWorkItemID] = struct{}{}
	}
	for _, unavailable := range raw.ProofRequirementUnavailabilities {
		if err := unavailable.Validate(); err != nil {
			return fmt.Errorf("runtime validation surface %q: %w", raw.ID, err)
		}
		unavailable = unavailable.Normalized()
		requirement := unavailable.Requirement.Normalized()
		if _, duplicate := seenRequirementHashes[requirement.Hash]; duplicate {
			return fmt.Errorf("runtime validation surface %q duplicates unavailable requirement %q", raw.ID, requirement.Hash)
		}
		if _, duplicate := seenRequirementCoverage[requirement.CoverageWorkItemID]; duplicate {
			return fmt.Errorf("runtime validation surface %q duplicates unavailable coverage %q", raw.ID, requirement.CoverageWorkItemID)
		}
		seenRequirementHashes[requirement.Hash] = struct{}{}
		seenRequirementCoverage[requirement.CoverageWorkItemID] = struct{}{}
		if unavailable.ValidationRecipeID != strings.TrimSpace(raw.ID) || unavailable.OriginalDiagnostic.OwnerModuleID != strings.TrimSpace(raw.OwnerModuleID) {
			return fmt.Errorf("runtime validation surface %q unavailable proof owner or recipe mismatch", raw.ID)
		}
	}
	s = s.Normalized()
	if s.ID == "" || s.OwnerModuleID == "" || s.OperationKind == "" || s.InputSchemaHash == "" {
		return fmt.Errorf("runtime validation surface requires identity, owner, operation and input schema")
	}
	descriptor, found := RuntimeOperationDescriptorForKind(s.OperationKind)
	if !found || descriptor.OwnerModuleID != s.OwnerModuleID {
		return fmt.Errorf("runtime validation surface operation %q does not belong to owner %q", s.OperationKind, s.OwnerModuleID)
	}
	if !ValidRuntimeEffectScope(RuntimeEffectScope(strings.TrimSpace(string(raw.EffectScope)))) ||
		strings.TrimSpace(string(raw.IdempotencyClass)) == "" ||
		NormalizeRuntimeOperationIdempotencyClass(raw.IdempotencyClass) != RuntimeOperationIdempotencyClass(strings.TrimSpace(string(raw.IdempotencyClass))) {
		return fmt.Errorf("runtime validation surface %q has invalid effect or idempotency", s.ID)
	}
	switch s.ProofApplicability {
	case RuntimeValidationProofDirect, RuntimeValidationProofAuxiliary, RuntimeValidationProofNotDeclared:
	default:
		return fmt.Errorf("runtime validation surface %q has invalid proof applicability %q", s.ID, s.ProofApplicability)
	}
	if s.OwnerModuleID == RuntimeModuleShell && s.Readiness == RuntimeValidationSurfaceReady && s.ProofApplicability == RuntimeValidationProofDirect &&
		len(s.ProofRequirementBindings) == 0 && len(s.ProofRequirementPreparations) == 0 && len(s.ProofRequirementUnavailabilities) == 0 {
		return fmt.Errorf("direct Shell validation surface %q requires exact CWI proof records", s.ID)
	}
	if (len(s.ProofRequirementBindings) != 0 || len(s.ProofRequirementPreparations) != 0 || len(s.ProofRequirementUnavailabilities) != 0) &&
		(s.OwnerModuleID != RuntimeModuleShell || s.OperationKind != RuntimeBlockKindManagedValidationSession) {
		return fmt.Errorf("runtime validation surface %q exact CWI proof records require Shell managed-validation ownership", s.ID)
	}
	boundChecks := make([]string, 0)
	boundNativeChecks := make(map[string]common.CoverageWorkItemID)
	for _, binding := range s.ProofRequirementBindings {
		if err := binding.Validate(); err != nil {
			return err
		}
		if binding.ValidationRecipeID != s.ID {
			return fmt.Errorf("runtime validation surface %q proof requirement recipe mismatch", s.ID)
		}
		boundChecks = append(boundChecks, binding.RequiredCheckRefs...)
		for _, attestation := range binding.CheckAttestations {
			attestation = attestation.Normalized()
			for _, ref := range attestation.NativeCheckRefs() {
				nativeIdentity := attestation.ObservationProtocolRef + "\x00" + ref
				if priorCoverage, duplicate := boundNativeChecks[nativeIdentity]; duplicate && priorCoverage != binding.CoverageWorkItemID {
					return fmt.Errorf("runtime validation surface %q native check %q is shared by coverage %q and %q", s.ID, ref, priorCoverage, binding.CoverageWorkItemID)
				}
				boundNativeChecks[nativeIdentity] = binding.CoverageWorkItemID
			}
		}
	}
	if len(s.ProofRequirementBindings) != 0 && s.ProofApplicability != RuntimeValidationProofDirect {
		return fmt.Errorf("runtime validation surface %q has exact proof bindings without direct applicability", s.ID)
	}
	if len(s.ProofRequirementBindings) != 0 && s.Readiness != RuntimeValidationSurfaceReady {
		return fmt.Errorf("runtime validation surface %q publishes ready proof bindings while not ready", s.ID)
	}
	if len(s.ProofRequirementPreparations) != 0 && s.ProofApplicability != RuntimeValidationProofDirect {
		return fmt.Errorf("runtime validation surface %q has exact proof preparations without direct applicability", s.ID)
	}
	if len(s.ProofRequirementUnavailabilities) != 0 && s.ProofApplicability != RuntimeValidationProofDirect {
		return fmt.Errorf("runtime validation surface %q has exact unavailability without direct applicability", s.ID)
	}
	if len(s.ProofRequirementBindings) != 0 && !sameStrings(normalizeStrings(boundChecks), s.CheckRefs) {
		return fmt.Errorf("runtime validation surface %q checks do not match exact proof requirement bindings", s.ID)
	}
	for _, preparation := range s.ProofRequirementPreparations {
		if err := preparation.Validate(); err != nil {
			return err
		}
		for _, attestation := range preparation.CheckAttestations {
			attestation = attestation.Normalized()
			for _, ref := range attestation.NativeCheckRefs() {
				nativeIdentity := attestation.ObservationProtocolRef + "\x00" + ref
				if priorCoverage, duplicate := boundNativeChecks[nativeIdentity]; duplicate && priorCoverage != preparation.CoverageWorkItemID {
					return fmt.Errorf("runtime validation surface %q native check %q is shared by coverage %q and %q", s.ID, ref, priorCoverage, preparation.CoverageWorkItemID)
				}
				boundNativeChecks[nativeIdentity] = preparation.CoverageWorkItemID
			}
		}
		constraint := preparation.ProspectiveTargetSatisfactionConstraint.Normalized()
		if preparation.ValidationRecipeID != s.ID || constraint.OwnerModuleID != s.OwnerModuleID ||
			constraint.ValidationSurfaceID != s.ID || constraint.ValidationRecipeID != s.ID ||
			constraint.InputSchemaHash != s.InputSchemaHash {
			return fmt.Errorf("runtime validation surface %q proof preparation identity mismatch", s.ID)
		}
	}
	if len(s.ProofRequirementPreparations) != 0 && s.Readiness == RuntimeValidationSurfaceUnavailable {
		return fmt.Errorf("unavailable runtime validation surface %q cannot publish proof preparations", s.ID)
	}
	switch s.Readiness {
	case RuntimeValidationSurfaceReady:
		if s.InputBindingRef == "" || s.PrerequisiteRef != "" || s.ProspectiveSatisfactionContractRef != "" || s.ProspectiveTargetSatisfactionConstraint != nil || s.ReadinessReason != "" {
			return fmt.Errorf("ready runtime validation surface %q requires only an input binding", s.ID)
		}
	case RuntimeValidationSurfacePrerequisiteRequired:
		if s.InputBindingRef != "" || s.PrerequisiteRef == "" || s.ReadinessReason == "" {
			return fmt.Errorf("prerequisite runtime validation surface %q requires only prerequisite lineage", s.ID)
		}
		if (s.ProspectiveSatisfactionContractRef == "") != (s.ProspectiveTargetSatisfactionConstraint == nil) {
			return fmt.Errorf("prerequisite runtime validation surface %q requires prospective contract and typed all-of constraint together", s.ID)
		}
		if s.ProspectiveTargetSatisfactionConstraint != nil {
			constraint := s.ProspectiveTargetSatisfactionConstraint.Normalized()
			if err := constraint.Validate(); err != nil {
				return err
			}
			if constraint.OwnerModuleID != s.OwnerModuleID || constraint.ProspectiveSatisfactionContractRef != s.ProspectiveSatisfactionContractRef ||
				constraint.ValidationSurfaceID != s.ID || constraint.ValidationRecipeID != s.ID || constraint.InputSchemaHash != s.InputSchemaHash {
				return fmt.Errorf("runtime validation surface %q prospective target constraint identity mismatch", s.ID)
			}
		}
	case RuntimeValidationSurfaceUnavailable:
		if s.InputBindingRef != "" || s.PrerequisiteRef != "" || s.ProspectiveSatisfactionContractRef != "" || s.ProspectiveTargetSatisfactionConstraint != nil || s.ReadinessReason == "" {
			return fmt.Errorf("unavailable runtime validation surface %q requires only a factual reason", s.ID)
		}
	default:
		return fmt.Errorf("runtime validation surface %q has unknown readiness %q", s.ID, s.Readiness)
	}
	return nil
}

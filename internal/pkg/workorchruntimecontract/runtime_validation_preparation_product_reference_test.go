package workorchruntimecontract

import (
	"fmt"
	"github.com/muidea/workorch/internal/pkg/common"
	"slices"
	"strings"
	"unicode/utf8"
)

// Complete frozen pre-change methods, reconstructed from this turn's initial
// tracked patch; test-only oracle, never a production compatibility path.
func referencePreparationBeforeProduct(p RuntimeValidationProofPreparation) error {
	rawAttestations := p.CheckAttestations
	// 纯校验只读原诊断；其 Validate 自行构造比较副本，不先 raw-clone 随后丢弃。
	originalDiagnostic := p.OriginalDiagnostic
	p.OriginalDiagnostic = nil
	p = p.Normalized()
	p.OriginalDiagnostic = originalDiagnostic
	if p.RequirementHash == "" || p.DefinitionRevision == "" || p.CoverageWorkItemID == "" || p.DeclarationRef == "" ||
		p.ValidationRecipeID == "" || (p.TargetRef != "" && !runtimeProspectiveTargetRefValid(p.TargetRef)) || len(p.CheckAttestations) == 0 {
		return fmt.Errorf("runtime validation proof preparation requires exact requirement, CWI, recipe and native attestations")
	}
	if strings.Contains(p.DeclarationRef, "*") {
		return fmt.Errorf("runtime validation proof preparation declaration %q is not CWI-specific", p.DeclarationRef)
	}
	if (p.ContentContractRef == "") != (p.ContentRequirements == "") || len(p.ContentContractRef) > 512 || len(p.ContentRequirements) > 4096 || !utf8.ValidString(p.ContentContractRef+p.ContentRequirements) ||
		strings.IndexFunc(p.ContentContractRef+p.ContentRequirements, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
		return fmt.Errorf("runtime validation proof preparation requires bounded owner content contract and requirements together")
	}
	required := make([]string, 0, len(p.CheckAttestations))
	for _, attestation := range p.CheckAttestations {
		required = append(required, attestation.RequiredCheckRef)
	}
	if err := ValidateRuntimeValidationCheckAttestationBindings(rawAttestations, required); err != nil {
		return err
	}
	if err := ValidateRuntimeSourceObligationChecks(p.DefinitionRevision, p.CoverageWorkItemID, p.SourceObligationChecks); err != nil {
		return err
	}
	for _, source := range p.SourceObligationChecks {
		if !slices.Contains(required, source.CheckRef) {
			return fmt.Errorf("preparation source obligation has no exact native check")
		}
	}
	constraint := p.ProspectiveTargetSatisfactionConstraint
	if err := constraint.Validate(); err != nil {
		return fmt.Errorf("runtime validation proof preparation target constraint: %w", err)
	}
	if constraint.ValidationRecipeID != p.ValidationRecipeID {
		return fmt.Errorf("runtime validation proof preparation recipe does not match target constraint")
	}
	if p.OriginalDiagnostic != nil {
		if err := p.OriginalDiagnostic.Validate(); err != nil {
			return fmt.Errorf("runtime validation proof preparation diagnostic: %w", err)
		}
		if p.OriginalDiagnostic.OwnerModuleID != constraint.OwnerModuleID || p.OriginalDiagnostic.ObservedRevision != constraint.CatalogObservedRevision {
			return fmt.Errorf("runtime validation proof preparation diagnostic must match its owner and catalog revision")
		}
	}
	if p.TargetRef != "" {
		exactTargetDeclared := false
		for _, clause := range constraint.AllOf {
			for _, predicate := range clause.AllOf {
				if predicate.Kind == RuntimeProspectiveTargetPredicatePathExact && predicate.Value == p.TargetRef {
					exactTargetDeclared = true
				}
			}
		}
		if !exactTargetDeclared {
			return fmt.Errorf("runtime validation proof preparation target %q is absent from exact target constraint", p.TargetRef)
		}
	}
	return nil
}

func referencePreparationCollectionBeforeProduct(values []RuntimeValidationProofPreparation) error {
	seenHash := make(map[string]struct{}, len(values))
	seenCoverage := make(map[common.CoverageWorkItemID]struct{}, len(values))
	for _, raw := range values {
		if err := referencePreparationBeforeProduct(raw); err != nil {
			return err
		}
		requirementHash := strings.TrimSpace(raw.RequirementHash)
		coverageID := common.CoverageWorkItemID(strings.TrimSpace(string(raw.CoverageWorkItemID)))
		if _, duplicate := seenHash[requirementHash]; duplicate {
			return fmt.Errorf("runtime validation proof preparation %q is duplicated", requirementHash)
		}
		if _, duplicate := seenCoverage[coverageID]; duplicate {
			return fmt.Errorf("runtime validation proof preparation coverage %q is duplicated", coverageID)
		}
		seenHash[requirementHash] = struct{}{}
		seenCoverage[coverageID] = struct{}{}
	}
	return nil
}

func referenceSurfaceBeforePreparationProduct(s RuntimeValidationSurface) (RuntimeValidationSurface, error) {
	raw := s
	if s.GenerationContext != nil {
		if len(s.ProofRequirementPreparations) == 0 && len(s.ProofRequirementBindings) == 0 {
			return RuntimeValidationSurface{}, fmt.Errorf("generation context requires an exact validation preparation or binding")
		}
		if err := s.GenerationContext.Validate(s.GenerationContext.ObservedRevision); err != nil {
			return RuntimeValidationSurface{}, err
		}
	}
	if err := validateRuntimeValidationInputTargets(raw); err != nil {
		return RuntimeValidationSurface{}, err
	}
	if duplicate := duplicateRuntimeValidationProofCheckRef(raw.CheckRefs); duplicate != "" {
		return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q duplicates check %q", strings.TrimSpace(raw.ID), duplicate)
	}
	if err := ValidateRuntimeValidationProofRequirementBindings(raw.ProofRequirementBindings); err != nil {
		return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q: %w", strings.TrimSpace(raw.ID), err)
	}
	if err := referencePreparationCollectionBeforeProduct(raw.ProofRequirementPreparations); err != nil {
		return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q: %w", strings.TrimSpace(raw.ID), err)
	}
	seenRequirementHashes := make(map[string]struct{}, len(raw.ProofRequirementBindings))
	seenRequirementCoverage := make(map[common.CoverageWorkItemID]struct{}, len(raw.ProofRequirementBindings))
	for _, binding := range raw.ProofRequirementBindings {
		binding.RequirementHash = strings.TrimSpace(binding.RequirementHash)
		binding.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(binding.CoverageWorkItemID)))
		if _, duplicate := seenRequirementHashes[binding.RequirementHash]; duplicate {
			return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q duplicates proof requirement %q", strings.TrimSpace(raw.ID), binding.RequirementHash)
		}
		if _, duplicate := seenRequirementCoverage[binding.CoverageWorkItemID]; duplicate {
			return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q duplicates proof requirement coverage %q", strings.TrimSpace(raw.ID), binding.CoverageWorkItemID)
		}
		seenRequirementHashes[binding.RequirementHash] = struct{}{}
		seenRequirementCoverage[binding.CoverageWorkItemID] = struct{}{}
	}
	for _, preparation := range raw.ProofRequirementPreparations {
		// 完整 proof 校验已执行；duplicate 查询只读取两个规范化身份字段。
		preparation.RequirementHash = strings.TrimSpace(preparation.RequirementHash)
		preparation.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(preparation.CoverageWorkItemID)))
		if _, duplicate := seenRequirementHashes[preparation.RequirementHash]; duplicate {
			return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q publishes proof requirement %q as both ready and prerequisite", strings.TrimSpace(raw.ID), preparation.RequirementHash)
		}
		if _, duplicate := seenRequirementCoverage[preparation.CoverageWorkItemID]; duplicate {
			return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q publishes proof requirement coverage %q more than once", strings.TrimSpace(raw.ID), preparation.CoverageWorkItemID)
		}
		seenRequirementHashes[preparation.RequirementHash] = struct{}{}
		seenRequirementCoverage[preparation.CoverageWorkItemID] = struct{}{}
	}
	for _, unavailable := range raw.ProofRequirementUnavailabilities {
		if err := unavailable.Validate(); err != nil {
			return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q: %w", raw.ID, err)
		}
		unavailable = unavailable.Normalized()
		requirement := unavailable.Requirement.Normalized()
		if _, duplicate := seenRequirementHashes[requirement.Hash]; duplicate {
			return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q duplicates unavailable requirement %q", raw.ID, requirement.Hash)
		}
		if _, duplicate := seenRequirementCoverage[requirement.CoverageWorkItemID]; duplicate {
			return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q duplicates unavailable coverage %q", raw.ID, requirement.CoverageWorkItemID)
		}
		seenRequirementHashes[requirement.Hash] = struct{}{}
		seenRequirementCoverage[requirement.CoverageWorkItemID] = struct{}{}
		if unavailable.ValidationRecipeID != strings.TrimSpace(raw.ID) || unavailable.OriginalDiagnostic.OwnerModuleID != strings.TrimSpace(raw.OwnerModuleID) {
			return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q unavailable proof owner or recipe mismatch", raw.ID)
		}
	}
	s = s.Normalized()
	if s.ID == "" || s.OwnerModuleID == "" || s.OperationKind == "" || s.InputSchemaHash == "" {
		return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface requires identity, owner, operation and input schema")
	}
	descriptor, found := RuntimeOperationDescriptorForKind(s.OperationKind)
	if !found || descriptor.OwnerModuleID != s.OwnerModuleID {
		return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface operation %q does not belong to owner %q", s.OperationKind, s.OwnerModuleID)
	}
	if !ValidRuntimeEffectScope(RuntimeEffectScope(strings.TrimSpace(string(raw.EffectScope)))) ||
		strings.TrimSpace(string(raw.IdempotencyClass)) == "" ||
		NormalizeRuntimeOperationIdempotencyClass(raw.IdempotencyClass) != RuntimeOperationIdempotencyClass(strings.TrimSpace(string(raw.IdempotencyClass))) {
		return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q has invalid effect or idempotency", s.ID)
	}
	switch s.ProofApplicability {
	case RuntimeValidationProofDirect, RuntimeValidationProofAuxiliary, RuntimeValidationProofNotDeclared:
	default:
		return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q has invalid proof applicability %q", s.ID, s.ProofApplicability)
	}
	if s.OwnerModuleID == RuntimeModuleShell && s.Readiness == RuntimeValidationSurfaceReady && s.ProofApplicability == RuntimeValidationProofDirect &&
		len(s.ProofRequirementBindings) == 0 && len(s.ProofRequirementPreparations) == 0 && len(s.ProofRequirementUnavailabilities) == 0 {
		return RuntimeValidationSurface{}, fmt.Errorf("direct Shell validation surface %q requires exact CWI proof records", s.ID)
	}
	if (len(s.ProofRequirementBindings) != 0 || len(s.ProofRequirementPreparations) != 0 || len(s.ProofRequirementUnavailabilities) != 0) &&
		(s.OwnerModuleID != RuntimeModuleShell || s.OperationKind != RuntimeBlockKindManagedValidationSession) {
		return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q exact CWI proof records require Shell managed-validation ownership", s.ID)
	}
	boundChecks := make([]string, 0)
	boundNativeChecks := make(map[string]common.CoverageWorkItemID)
	for _, binding := range s.ProofRequirementBindings {
		// 原始逐项完整校验已在上方执行；这里只反证 surface 间身份约束。
		if binding.ValidationRecipeID != s.ID {
			return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q proof requirement recipe mismatch", s.ID)
		}
		boundChecks = append(boundChecks, binding.RequiredCheckRefs...)
		for _, attestation := range binding.CheckAttestations {
			attestation = attestation.Normalized()
			for _, ref := range attestation.NativeCheckRefs() {
				nativeIdentity := attestation.ObservationProtocolRef + "\x00" + ref
				if priorCoverage, duplicate := boundNativeChecks[nativeIdentity]; duplicate && priorCoverage != binding.CoverageWorkItemID {
					return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q native check %q is shared by coverage %q and %q", s.ID, ref, priorCoverage, binding.CoverageWorkItemID)
				}
				boundNativeChecks[nativeIdentity] = binding.CoverageWorkItemID
			}
		}
	}
	if len(s.ProofRequirementBindings) != 0 && s.ProofApplicability != RuntimeValidationProofDirect {
		return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q has exact proof bindings without direct applicability", s.ID)
	}
	if len(s.ProofRequirementBindings) != 0 && s.Readiness != RuntimeValidationSurfaceReady {
		return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q publishes ready proof bindings while not ready", s.ID)
	}
	if len(s.ProofRequirementPreparations) != 0 && s.ProofApplicability != RuntimeValidationProofDirect {
		return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q has exact proof preparations without direct applicability", s.ID)
	}
	if len(s.ProofRequirementUnavailabilities) != 0 && s.ProofApplicability != RuntimeValidationProofDirect {
		return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q has exact unavailability without direct applicability", s.ID)
	}
	if len(s.ProofRequirementBindings) != 0 && !sameStrings(normalizeStrings(boundChecks), s.CheckRefs) {
		return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q checks do not match exact proof requirement bindings", s.ID)
	}
	for _, preparation := range s.ProofRequirementPreparations {
		// 本次已完整校验原 preparation；不重验未改变的规范化 proof 正文。
		for _, attestation := range preparation.CheckAttestations {
			attestation = attestation.Normalized()
			for _, ref := range attestation.NativeCheckRefs() {
				nativeIdentity := attestation.ObservationProtocolRef + "\x00" + ref
				if priorCoverage, duplicate := boundNativeChecks[nativeIdentity]; duplicate && priorCoverage != preparation.CoverageWorkItemID {
					return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q native check %q is shared by coverage %q and %q", s.ID, ref, priorCoverage, preparation.CoverageWorkItemID)
				}
				boundNativeChecks[nativeIdentity] = preparation.CoverageWorkItemID
			}
		}
		constraint := preparation.ProspectiveTargetSatisfactionConstraint.Normalized()
		if preparation.ValidationRecipeID != s.ID || constraint.OwnerModuleID != s.OwnerModuleID ||
			constraint.ValidationSurfaceID != s.ID || constraint.ValidationRecipeID != s.ID ||
			constraint.InputSchemaHash != s.InputSchemaHash {
			return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q proof preparation identity mismatch", s.ID)
		}
	}
	if len(s.ProofRequirementPreparations) != 0 && s.Readiness == RuntimeValidationSurfaceUnavailable {
		return RuntimeValidationSurface{}, fmt.Errorf("unavailable runtime validation surface %q cannot publish proof preparations", s.ID)
	}
	switch s.Readiness {
	case RuntimeValidationSurfaceReady:
		if s.InputBindingRef == "" || s.PrerequisiteRef != "" || s.ProspectiveSatisfactionContractRef != "" || s.ProspectiveTargetSatisfactionConstraint != nil || s.ReadinessReason != "" {
			return RuntimeValidationSurface{}, fmt.Errorf("ready runtime validation surface %q requires only an input binding", s.ID)
		}
	case RuntimeValidationSurfacePrerequisiteRequired:
		if s.InputBindingRef != "" || s.PrerequisiteRef == "" || s.ReadinessReason == "" {
			return RuntimeValidationSurface{}, fmt.Errorf("prerequisite runtime validation surface %q requires only prerequisite lineage", s.ID)
		}
		if (s.ProspectiveSatisfactionContractRef == "") != (s.ProspectiveTargetSatisfactionConstraint == nil) {
			return RuntimeValidationSurface{}, fmt.Errorf("prerequisite runtime validation surface %q requires prospective contract and typed all-of constraint together", s.ID)
		}
		if s.ProspectiveTargetSatisfactionConstraint != nil {
			constraint := s.ProspectiveTargetSatisfactionConstraint.Normalized()
			if err := constraint.Validate(); err != nil {
				return RuntimeValidationSurface{}, err
			}
			if constraint.OwnerModuleID != s.OwnerModuleID || constraint.ProspectiveSatisfactionContractRef != s.ProspectiveSatisfactionContractRef ||
				constraint.ValidationSurfaceID != s.ID || constraint.ValidationRecipeID != s.ID || constraint.InputSchemaHash != s.InputSchemaHash {
				return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q prospective target constraint identity mismatch", s.ID)
			}
		}
	case RuntimeValidationSurfaceUnavailable:
		if s.InputBindingRef != "" || s.PrerequisiteRef != "" || s.ProspectiveSatisfactionContractRef != "" || s.ProspectiveTargetSatisfactionConstraint != nil || s.ReadinessReason == "" {
			return RuntimeValidationSurface{}, fmt.Errorf("unavailable runtime validation surface %q requires only a factual reason", s.ID)
		}
	default:
		return RuntimeValidationSurface{}, fmt.Errorf("runtime validation surface %q has unknown readiness %q", s.ID, s.Readiness)
	}
	return s, nil
}

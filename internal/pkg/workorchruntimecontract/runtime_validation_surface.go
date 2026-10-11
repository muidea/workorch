package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeValidationSurfaceReadiness separates an owner's callable operation
// catalog from the validation recipes that are executable with the facts
// currently sealed into the catalog. It is a factual owner observation and
// never selects a Runtime action or state transition.
type RuntimeValidationSurfaceReadiness string

const (
	RuntimeValidationSurfaceReady                RuntimeValidationSurfaceReadiness = "ready"
	RuntimeValidationSurfacePrerequisiteRequired RuntimeValidationSurfaceReadiness = "prerequisite_required"
	RuntimeValidationSurfaceUnavailable          RuntimeValidationSurfaceReadiness = "unavailable"
)

// RuntimeValidationProofApplicability is the capability owner's declarative
// statement about whether a technically ready surface may be used as direct
// evidence for a CWI. It is independent from callability and input readiness:
// an auxiliary surface may execute successfully without being allowed to
// close direct validation coverage.
type RuntimeValidationProofApplicability string

const (
	RuntimeValidationProofDirect      RuntimeValidationProofApplicability = "direct"
	RuntimeValidationProofAuxiliary   RuntimeValidationProofApplicability = "auxiliary"
	RuntimeValidationProofNotDeclared RuntimeValidationProofApplicability = "not_declared"
)

// RuntimeValidationSurface is the owner-neutral, immutable description of one
// validation proof surface. ValidationRecipeRefs remains the provider-visible
// ready-and-direct set; this value preserves why another callable surface was
// not exposed without leaking owner implementation or choosing a retry.
type RuntimeValidationSurface struct {
	// Input targets are capability-owner observations, not writable delivery
	// targets. The resource owner is explicit; Core does not infer it from a
	// filename, language or validation adapter. Empty fields preserve archives.
	InputTargetOwnerModuleID string   `json:",omitempty"`
	InputTargetRefs          []string `json:",omitempty"`
	ID                       string
	OwnerModuleID            string
	OperationKind            string
	Readiness                RuntimeValidationSurfaceReadiness
	ReadinessReason          string
	InputSchemaHash          string
	InputBindingRef          string
	PrerequisiteRef          string
	// ProspectiveSatisfactionContractRef is present only on a prerequisite-
	// required surface whose owner can judge whether a typed prospective
	// workspace delta would make that surface selectable.
	ProspectiveSatisfactionContractRef string
	// ProspectiveTargetSatisfactionConstraint is the exact all-of-only,
	// owner-authored target fact for that contract. It cannot choose a target
	// or mutate Runtime state.
	ProspectiveTargetSatisfactionConstraint *RuntimeProspectiveTargetSatisfactionConstraint
	CheckRefs                               []string
	ProofApplicability                      RuntimeValidationProofApplicability
	// ProofRequirementBindings are the exact per-CWI declarations this owner
	// says the surface can observe. CheckRefs is only the surface-wide union for
	// audit; semantic feasibility must resolve through this binding and may not
	// use that union directly.
	ProofRequirementBindings []RuntimeValidationProofRequirementBinding
	// ProofRequirementPreparations are exact per-CWI requirements whose owner-
	// native check is not observable yet. A recipe can therefore remain
	// technically ready for CWI-A while CWI-B independently requires a bounded
	// preparation; surface-wide Readiness never collapses those two facts.
	ProofRequirementPreparations []RuntimeValidationProofPreparation
	// Unavailability is per requirement, independent from recipe readiness. It
	// must never be projected as a prospective preparation or executable proof.
	ProofRequirementUnavailabilities []RuntimeValidationProofUnavailability `json:",omitempty"`
	EffectScope                      RuntimeEffectScope
	IdempotencyClass                 RuntimeOperationIdempotencyClass
}

func (s RuntimeValidationSurface) Normalized() RuntimeValidationSurface {
	s.InputTargetOwnerModuleID = strings.TrimSpace(s.InputTargetOwnerModuleID)
	s.InputTargetRefs = normalizeStrings(s.InputTargetRefs)
	s.ID = strings.TrimSpace(s.ID)
	s.OwnerModuleID = strings.TrimSpace(s.OwnerModuleID)
	s.OperationKind = CanonicalRuntimeBlockKind(s.OperationKind)
	s.Readiness = RuntimeValidationSurfaceReadiness(strings.TrimSpace(string(s.Readiness)))
	s.ReadinessReason = strings.TrimSpace(s.ReadinessReason)
	s.InputSchemaHash = strings.TrimSpace(s.InputSchemaHash)
	s.InputBindingRef = strings.TrimSpace(s.InputBindingRef)
	s.PrerequisiteRef = strings.TrimSpace(s.PrerequisiteRef)
	s.ProspectiveSatisfactionContractRef = strings.TrimSpace(s.ProspectiveSatisfactionContractRef)
	if s.ProspectiveTargetSatisfactionConstraint != nil {
		value := s.ProspectiveTargetSatisfactionConstraint.Normalized()
		s.ProspectiveTargetSatisfactionConstraint = &value
	}
	s.CheckRefs = normalizeStrings(s.CheckRefs)
	s.ProofRequirementBindings = NormalizeRuntimeValidationProofRequirementBindings(s.ProofRequirementBindings)
	s.ProofRequirementPreparations = normalizeRuntimeValidationProofPreparations(s.ProofRequirementPreparations)
	s.ProofRequirementUnavailabilities = normalizeRuntimeValidationProofUnavailabilities(s.ProofRequirementUnavailabilities)
	s.ProofApplicability = RuntimeValidationProofApplicability(strings.TrimSpace(string(s.ProofApplicability)))
	s.EffectScope = NormalizeRuntimeEffectScope(s.EffectScope)
	s.IdempotencyClass = NormalizeRuntimeOperationIdempotencyClass(s.IdempotencyClass)
	return s
}

func (s RuntimeValidationSurface) Clone() RuntimeValidationSurface {
	// Normalized allocates all nested proof records and optional constraint
	// values. Re-cloning them here multiplied immutable semantic-history work.
	return s.Normalized()
}

func (s RuntimeValidationSurface) Validate() error {
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
		binding.RequirementHash = strings.TrimSpace(binding.RequirementHash)
		binding.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(binding.CoverageWorkItemID)))
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
		// 完整 proof 校验已执行；duplicate 查询只读取两个规范化身份字段。
		preparation.RequirementHash = strings.TrimSpace(preparation.RequirementHash)
		preparation.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(preparation.CoverageWorkItemID)))
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
		// 原始逐项完整校验已在上方执行；这里只反证 surface 间身份约束。
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
		// 本次已完整校验原 preparation；不重验未改变的规范化 proof 正文。
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

// ValidateRuntimeValidationProofPreparations rejects duplicate or aliased raw
// per-CWI records before normalization can sort them into an apparently valid
// catalog.
func ValidateRuntimeValidationProofPreparations(values []RuntimeValidationProofPreparation) error {
	seenHash := make(map[string]struct{}, len(values))
	seenCoverage := make(map[common.CoverageWorkItemID]struct{}, len(values))
	for _, raw := range values {
		if err := raw.Validate(); err != nil {
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

// RuntimeValidationProofRequirementBinding is the capability-owner statement
// that one executable surface can observe one exact Definition requirement.
// ValidationRecipeID is the owner's concrete selected recipe; it may therefore
// be more specific than a recipe-neutral requirement. The full requirement
// remains Definition-owned; this value only repeats the immutable identity
// needed to reject a stale or cross-CWI owner fact.
type RuntimeValidationProofRequirementBinding struct {
	RequirementHash    string
	DefinitionRevision string
	CoverageWorkItemID common.CoverageWorkItemID
	DeclarationRef     string
	ValidationRecipeID string
	RequiredCheckRefs  []string
	// CheckAttestations is the capability-owner mapping from each immutable
	// Definition check to one owner-native observation. Provider/workspace
	// stdout is never itself an attestation protocol.
	CheckAttestations []RuntimeValidationCheckAttestationBinding
	// InputReview requires a complete LLM consistency report before SM-2 may
	// seal a proof. Owner readiness alone cannot discharge that requirement.
	InputReview *RuntimeValidationInputReviewSubject `json:",omitempty"`
}

// RuntimeValidationCheckAttestationBinding binds one Definition-owned check
// identity to the exact native check and observation protocol that the
// capability owner can attest in its receipt. Both native values are opaque to
// Runtime core; their meaning remains inside the capability owner.
type RuntimeValidationCheckAttestationBinding struct {
	RequiredCheckRef       string
	ObservationProtocolRef string
	NativeCheckRef         string
	// Optional conjunctive input: never replaces the original native check.
	// Empty preserves the identity and behavior of already sealed bindings.
	SupplementalNativeCheckRef string `json:",omitempty"`
	// 非空时所有准确 native checks 还须通过 owner 声明的有序执行合取。
	ExecutionStagesHash string `json:",omitempty"`
}

func (b RuntimeValidationCheckAttestationBinding) Normalized() RuntimeValidationCheckAttestationBinding {
	b.RequiredCheckRef = strings.TrimSpace(b.RequiredCheckRef)
	b.ObservationProtocolRef = strings.TrimSpace(b.ObservationProtocolRef)
	b.NativeCheckRef = strings.TrimSpace(b.NativeCheckRef)
	b.SupplementalNativeCheckRef = strings.TrimSpace(b.SupplementalNativeCheckRef)
	return b
}

func (b RuntimeValidationCheckAttestationBinding) Validate() error {
	b = b.Normalized()
	if b.RequiredCheckRef == "" || b.ObservationProtocolRef == "" || b.NativeCheckRef == "" {
		return fmt.Errorf("runtime validation check attestation requires Definition check, observation protocol and native check")
	}
	if b.ExecutionStagesHash != "" && !validationExecutionDigest(b.ExecutionStagesHash) {
		return fmt.Errorf("runtime validation check execution stages identity invalid")
	}
	if b.SupplementalNativeCheckRef != "" && (b.SupplementalNativeCheckRef == b.NativeCheckRef || len(b.SupplementalNativeCheckRef) > 512 || strings.IndexFunc(b.SupplementalNativeCheckRef, func(r rune) bool { return r < 32 || r == 127 }) >= 0) {
		return fmt.Errorf("supplemental native check must be distinct and bounded")
	}
	return nil
}

// NativeCheckRefs is the complete all-of native input, not a set of alternatives.
func (b RuntimeValidationCheckAttestationBinding) NativeCheckRefs() []string {
	b = b.Normalized()
	refs := []string{b.NativeCheckRef}
	if b.SupplementalNativeCheckRef != "" {
		refs = append(refs, b.SupplementalNativeCheckRef)
	}
	return refs
}

// RuntimeValidationAttestationPassed checks all native inputs under the exact
// owner protocol. Receipt quality, freshness and CWI credit remain separate gates.
func RuntimeValidationAttestationPassed(observation RuntimeValidationObservation, binding RuntimeValidationCheckAttestationBinding) bool {
	binding = binding.Normalized()
	observation = observation.Normalized()
	if binding.Validate() != nil || observation.ObservationProtocolRef != binding.ObservationProtocolRef {
		return false
	}
	if binding.ExecutionStagesHash != "" {
		conjunction := observation.ExecutionConjunction
		if conjunction == nil || conjunction.Validate() != nil || !conjunction.Complete || conjunction.ProtocolRef != binding.ObservationProtocolRef || conjunction.RequiredStagesHash != binding.ExecutionStagesHash {
			return false
		}
	}
	for _, ref := range binding.NativeCheckRefs() {
		if !RuntimeValidationCheckPassed(observation, ref) {
			return false
		}
	}
	return true
}

func NormalizeRuntimeValidationCheckAttestationBindings(values []RuntimeValidationCheckAttestationBinding) []RuntimeValidationCheckAttestationBinding {
	ret := make([]RuntimeValidationCheckAttestationBinding, 0, len(values))
	for _, raw := range values {
		value := raw.Normalized()
		if value.RequiredCheckRef != "" {
			ret = append(ret, value)
		}
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].RequiredCheckRef != ret[j].RequiredCheckRef {
			return ret[i].RequiredCheckRef < ret[j].RequiredCheckRef
		}
		if ret[i].ObservationProtocolRef != ret[j].ObservationProtocolRef {
			return ret[i].ObservationProtocolRef < ret[j].ObservationProtocolRef
		}
		return ret[i].NativeCheckRef < ret[j].NativeCheckRef
	})
	return nilIfEmpty(ret)
}

func CloneRuntimeValidationCheckAttestationBindings(values []RuntimeValidationCheckAttestationBinding) []RuntimeValidationCheckAttestationBinding {
	return NormalizeRuntimeValidationCheckAttestationBindings(values)
}

func SameRuntimeValidationCheckAttestationBindings(left, right []RuntimeValidationCheckAttestationBinding) bool {
	left = NormalizeRuntimeValidationCheckAttestationBindings(left)
	right = NormalizeRuntimeValidationCheckAttestationBindings(right)
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

func ValidateRuntimeValidationCheckAttestationBindings(values []RuntimeValidationCheckAttestationBinding, requiredCheckRefs []string) error {
	seenRequired := make(map[string]struct{}, len(values))
	seenNative := make(map[string]struct{}, len(values))
	executionStagesHash := ""
	for index, raw := range values {
		if err := raw.Validate(); err != nil {
			return err
		}
		value := raw.Normalized()
		if index == 0 {
			executionStagesHash = value.ExecutionStagesHash
		} else if value.ExecutionStagesHash != executionStagesHash {
			return fmt.Errorf("runtime validation check attestations mix execution conjunction contracts")
		}
		if _, duplicate := seenRequired[value.RequiredCheckRef]; duplicate {
			return fmt.Errorf("runtime validation check attestation duplicates Definition check %q", value.RequiredCheckRef)
		}
		for _, ref := range value.NativeCheckRefs() {
			nativeIdentity := value.ObservationProtocolRef + "\x00" + ref
			if _, duplicate := seenNative[nativeIdentity]; duplicate {
				return fmt.Errorf("runtime validation check attestation duplicates native check %q", ref)
			}
			seenNative[nativeIdentity] = struct{}{}
		}
		seenRequired[value.RequiredCheckRef] = struct{}{}
	}
	if !sameStrings(normalizeStrings(requiredCheckRefs), func() []string {
		refs := make([]string, 0, len(values))
		for _, value := range values {
			refs = append(refs, strings.TrimSpace(value.RequiredCheckRef))
		}
		return normalizeStrings(refs)
	}()) {
		return fmt.Errorf("runtime validation check attestations do not exactly cover required checks")
	}
	return nil
}

func (b RuntimeValidationProofRequirementBinding) Normalized() RuntimeValidationProofRequirementBinding {
	b.RequirementHash = strings.TrimSpace(b.RequirementHash)
	b.DefinitionRevision = strings.TrimSpace(b.DefinitionRevision)
	b.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(b.CoverageWorkItemID)))
	b.DeclarationRef = strings.TrimSpace(b.DeclarationRef)
	b.ValidationRecipeID = strings.TrimSpace(b.ValidationRecipeID)
	b.RequiredCheckRefs = normalizeRuntimeValidationProofCheckRefs(b.RequiredCheckRefs)
	b.CheckAttestations = NormalizeRuntimeValidationCheckAttestationBindings(b.CheckAttestations)
	if b.InputReview != nil {
		value := b.InputReview.Clone()
		b.InputReview = &value
	}
	return b
}

func (b RuntimeValidationProofRequirementBinding) Clone() RuntimeValidationProofRequirementBinding {
	// Normalized detaches both nested slices.
	return b.Normalized()
}

func (b RuntimeValidationProofRequirementBinding) Validate() error {
	if b.InputReview != nil {
		if err := b.InputReview.ValidateAgainstBinding(b); err != nil {
			return err
		}
	}
	if duplicate := duplicateRuntimeValidationProofCheckRef(b.RequiredCheckRefs); duplicate != "" {
		return fmt.Errorf("runtime validation proof requirement binding duplicates check %q", duplicate)
	}
	b = b.Normalized()
	if b.RequirementHash == "" || b.DefinitionRevision == "" || b.CoverageWorkItemID == "" || b.DeclarationRef == "" || b.ValidationRecipeID == "" || len(b.RequiredCheckRefs) == 0 {
		return fmt.Errorf("runtime validation proof requirement binding requires requirement, definition, CWI, declaration, recipe and checks")
	}
	if strings.Contains(b.DeclarationRef, "*") {
		return fmt.Errorf("runtime validation proof requirement binding declaration %q is not CWI-specific", b.DeclarationRef)
	}
	if err := ValidateRuntimeValidationCheckAttestationBindings(b.CheckAttestations, b.RequiredCheckRefs); err != nil {
		return err
	}
	return nil
}

func RuntimeValidationProofRequirementBindingFor(requirement RuntimeValidationProofRequirement) RuntimeValidationProofRequirementBinding {
	binding, _ := RuntimeValidationProofRequirementBindingForRecipe(requirement, requirement.ValidationRecipeID)
	return binding
}

// RuntimeValidationProofRequirementBindingForRecipe binds one exact
// Definition requirement to one concrete capability-owner recipe. A
// Profile-constrained requirement cannot be widened to another recipe.
func RuntimeValidationProofRequirementBindingForRecipe(requirement RuntimeValidationProofRequirement, recipeID string, checkAttestations ...RuntimeValidationCheckAttestationBinding) (RuntimeValidationProofRequirementBinding, error) {
	requirement = requirement.Normalized()
	recipeID = strings.TrimSpace(recipeID)
	if err := requirement.Validate(); err != nil {
		return RuntimeValidationProofRequirementBinding{}, err
	}
	if recipeID == "" || (requirement.ValidationRecipeID != "" && requirement.ValidationRecipeID != recipeID) {
		return RuntimeValidationProofRequirementBinding{}, fmt.Errorf("runtime validation proof requirement %q is not compatible with recipe %q", requirement.Hash, recipeID)
	}
	binding := RuntimeValidationProofRequirementBinding{
		RequirementHash: requirement.Hash, DefinitionRevision: requirement.DefinitionRevision,
		CoverageWorkItemID: requirement.CoverageWorkItemID, DeclarationRef: requirement.DeclarationRef,
		ValidationRecipeID: recipeID, RequiredCheckRefs: append([]string(nil), requirement.RequiredCheckRefs...),
		CheckAttestations: CloneRuntimeValidationCheckAttestationBindings(checkAttestations),
	}.Normalized()
	if err := binding.Validate(); err != nil {
		return RuntimeValidationProofRequirementBinding{}, err
	}
	return binding, nil
}

func NormalizeRuntimeValidationProofRequirementBindings(values []RuntimeValidationProofRequirementBinding) []RuntimeValidationProofRequirementBinding {
	ret := make([]RuntimeValidationProofRequirementBinding, 0, len(values))
	for _, raw := range values {
		value := raw.Clone()
		if value.RequirementHash != "" {
			ret = append(ret, value)
		}
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].CoverageWorkItemID != ret[j].CoverageWorkItemID {
			return ret[i].CoverageWorkItemID < ret[j].CoverageWorkItemID
		}
		return ret[i].RequirementHash < ret[j].RequirementHash
	})
	return nilIfEmpty(ret)
}

func CloneRuntimeValidationProofRequirementBindings(values []RuntimeValidationProofRequirementBinding) []RuntimeValidationProofRequirementBinding {
	return NormalizeRuntimeValidationProofRequirementBindings(values)
}

func ValidateRuntimeValidationProofRequirementBindings(values []RuntimeValidationProofRequirementBinding) error {
	seenHashes := make(map[string]struct{}, len(values))
	seenCoverage := make(map[common.CoverageWorkItemID]struct{}, len(values))
	for _, raw := range values {
		if err := raw.Validate(); err != nil {
			return err
		}
		requirementHash := strings.TrimSpace(raw.RequirementHash)
		coverageID := common.CoverageWorkItemID(strings.TrimSpace(string(raw.CoverageWorkItemID)))
		if _, duplicate := seenHashes[requirementHash]; duplicate {
			return fmt.Errorf("runtime validation proof requirement binding %q is duplicated", requirementHash)
		}
		if _, duplicate := seenCoverage[coverageID]; duplicate {
			return fmt.Errorf("runtime validation proof requirement binding coverage %q is duplicated", coverageID)
		}
		seenHashes[requirementHash] = struct{}{}
		seenCoverage[coverageID] = struct{}{}
	}
	return nil
}

func NormalizeRuntimeValidationSurfaces(values []RuntimeValidationSurface) []RuntimeValidationSurface {
	ret := make([]RuntimeValidationSurface, 0, len(values))
	for _, raw := range values {
		value := raw.Clone()
		if value.ID != "" {
			ret = append(ret, value)
		}
	}
	sort.SliceStable(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	return nilIfEmpty(ret)
}

// RuntimeReadyValidationRecipeRefs reports technical readiness for owner audit
// and diagnostics. Provider selection must use
// RuntimeSelectableValidationRecipeRefs instead.
func RuntimeReadyValidationRecipeRefs(values []RuntimeValidationSurface) []string {
	ret := make([]string, 0, len(values))
	for _, value := range NormalizeRuntimeValidationSurfaces(values) {
		if value.Readiness == RuntimeValidationSurfaceReady {
			ret = append(ret, value.ID)
		}
	}
	return normalizeStrings(ret)
}

// RuntimeSelectableValidationRecipeRefs returns the exact provider-selectable
// proof surface. A successful/ready auxiliary, durable, or replay-unknown probe
// remains audit evidence but cannot be promoted into a semantic recipe or
// receive direct CWI credit. At-most-once validation remains selectable, but an
// ambiguous claimed execution must converge as outcome_unknown, never replay.
func RuntimeSelectableValidationRecipeRefs(values []RuntimeValidationSurface) []string {
	return runtimeSelectableValidationRecipeRefsCanonical(NormalizeRuntimeValidationSurfaces(values))
}

func runtimeSelectableValidationRecipeRefsCanonical(values []RuntimeValidationSurface) []string {
	ret := make([]string, 0, len(values))
	for _, value := range values {
		if value.Readiness == RuntimeValidationSurfaceReady && value.ProofApplicability == RuntimeValidationProofDirect &&
			RuntimeEffectIsNonDurable(value.EffectScope) && runtimeValidationSurfaceHasExplicitReplayPolicy(value.IdempotencyClass) &&
			(value.OwnerModuleID != RuntimeModuleShell || len(value.ProofRequirementBindings) != 0) {
			ret = append(ret, value.ID)
		}
	}
	return normalizeStrings(ret)
}

func runtimeValidationSurfaceHasExplicitReplayPolicy(value RuntimeOperationIdempotencyClass) bool {
	switch NormalizeRuntimeOperationIdempotencyClass(value) {
	case RuntimeOperationIdempotencySafeToReplay, RuntimeOperationIdempotencyAtMostOnce, RuntimeOperationIdempotencyOwnerManaged:
		return true
	default:
		return false
	}
}

func RuntimeValidationSurfaceForID(values []RuntimeValidationSurface, id string) (RuntimeValidationSurface, bool) {
	id = strings.TrimSpace(id)
	for _, value := range NormalizeRuntimeValidationSurfaces(values) {
		if value.ID == id {
			return value.Clone(), true
		}
	}
	return RuntimeValidationSurface{}, false
}

// RuntimeValidationSurfaceForProofRequirement resolves an exact CWI binding;
// it deliberately does not fall back to the surface-wide check union.
func RuntimeValidationSurfaceForProofRequirement(values []RuntimeValidationSurface, requirement RuntimeValidationProofRequirement) (RuntimeValidationSurface, RuntimeValidationProofRequirementBinding, bool) {
	surface, binding, found := RuntimeValidationSurfaceDeclaringProofRequirement(values, requirement)
	if !found || surface.Readiness != RuntimeValidationSurfaceReady || surface.ProofApplicability != RuntimeValidationProofDirect {
		return RuntimeValidationSurface{}, RuntimeValidationProofRequirementBinding{}, false
	}
	return surface, binding, true
}

// RuntimeValidationSurfaceDeclaringProofRequirement checks exact owner
// applicability without conflating it with mutable readiness. It is suitable
// for acquisition/recovery validation of ready, prerequisite-required and
// unavailable surfaces alike.
func RuntimeValidationSurfaceDeclaringProofRequirement(values []RuntimeValidationSurface, requirement RuntimeValidationProofRequirement) (RuntimeValidationSurface, RuntimeValidationProofRequirementBinding, bool) {
	requirement = requirement.Normalized()
	if requirement.Validate() != nil {
		return RuntimeValidationSurface{}, RuntimeValidationProofRequirementBinding{}, false
	}
	for _, surface := range NormalizeRuntimeValidationSurfaces(values) {
		if surface.OwnerModuleID != RuntimeModuleShell || surface.OperationKind != RuntimeBlockKindManagedValidationSession ||
			surface.ProofApplicability != RuntimeValidationProofDirect ||
			(requirement.ValidationRecipeID != "" && surface.ID != requirement.ValidationRecipeID) {
			continue
		}
		for _, binding := range surface.ProofRequirementBindings {
			binding = binding.Normalized()
			if binding.RequirementHash == requirement.Hash && binding.DefinitionRevision == requirement.DefinitionRevision &&
				binding.CoverageWorkItemID == requirement.CoverageWorkItemID && binding.DeclarationRef == requirement.DeclarationRef &&
				binding.ValidationRecipeID == surface.ID &&
				(requirement.ValidationRecipeID == "" || binding.ValidationRecipeID == requirement.ValidationRecipeID) &&
				sameStrings(binding.RequiredCheckRefs, requirement.RequiredCheckRefs) {
				return surface.Clone(), binding.Clone(), true
			}
		}
	}
	return RuntimeValidationSurface{}, RuntimeValidationProofRequirementBinding{}, false
}

func RuntimeSelectableValidationRecipeRefsForProofRequirement(values []RuntimeValidationSurface, requirement RuntimeValidationProofRequirement) []string {
	ret := make([]string, 0)
	for _, surface := range NormalizeRuntimeValidationSurfaces(values) {
		if matched, _, found := RuntimeValidationSurfaceForProofRequirement([]RuntimeValidationSurface{surface}, requirement); found &&
			RuntimeEffectIsNonDurable(matched.EffectScope) && runtimeValidationSurfaceHasExplicitReplayPolicy(matched.IdempotencyClass) {
			ret = append(ret, matched.ID)
		}
	}
	return normalizeStrings(ret)
}

// RuntimeSelectableValidationRecipeRefsForProofRequirementInCatalogs returns
// the concrete Shell-owned recipes that are ready and exactly bound to one
// Definition requirement. Other validation owners (for example MCP) retain
// their own typed proof contract and cannot declare this Shell binding.
func RuntimeSelectableValidationRecipeRefsForProofRequirementInCatalogs(values []RuntimeOperationAdmissibility, requirement RuntimeValidationProofRequirement) []string {
	ret := make([]string, 0)
	for _, catalog := range NormalizeRuntimeOperationAdmissibilities(values) {
		if catalog.OwnerModuleID != RuntimeModuleShell {
			continue
		}
		ret = append(ret, RuntimeSelectableValidationRecipeRefsForProofRequirement(catalog.ValidationSurfaces, requirement)...)
	}
	return normalizeStrings(ret)
}

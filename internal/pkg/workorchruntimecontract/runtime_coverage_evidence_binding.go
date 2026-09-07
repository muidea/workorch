package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// CoverageEvidenceProof is the evidence-kind-specific portion of one
// CoverageEvidenceBinding. Keeping effect, operation and validation facts
// together prevents a state-change receipt from borrowing a validation
// recipe/check (or a validation receipt from borrowing a durable effect) from
// a different proof in the same CWI binding.
//
// This is a pure immutable DTO. It does not identify a CWI by itself and it
// cannot grant coverage credit.
type CoverageEvidenceProof struct {
	EvidenceKind                   string
	RequiredEffectScopes           []RuntimeEffectScope
	AllowedOperationKinds          []string
	AllowedOperationVariants       []RuntimeOperationVariantRef
	ValidationRecipeID             string
	ValidationProofRequirementHash string
	RequiredCheckRefs              []string
	ValidationCheckAttestations    []RuntimeValidationCheckAttestationBinding
	MetricConstraints              []string
	ArtifactRefs                   []string
}

func (p CoverageEvidenceProof) Normalized() CoverageEvidenceProof {
	p.EvidenceKind = strings.TrimSpace(p.EvidenceKind)
	p.RequiredEffectScopes = NormalizeRuntimeEffectScopes(p.RequiredEffectScopes)
	p.AllowedOperationKinds = canonicalizeOperationKinds(p.AllowedOperationKinds)
	p.AllowedOperationVariants = NormalizeRuntimeOperationVariantRefs(p.AllowedOperationVariants)
	p.ValidationRecipeID = strings.TrimSpace(p.ValidationRecipeID)
	p.ValidationProofRequirementHash = strings.TrimSpace(p.ValidationProofRequirementHash)
	p.RequiredCheckRefs = normalizeStrings(p.RequiredCheckRefs)
	p.ValidationCheckAttestations = NormalizeRuntimeValidationCheckAttestationBindings(p.ValidationCheckAttestations)
	p.MetricConstraints = normalizeStrings(p.MetricConstraints)
	p.ArtifactRefs = normalizeStrings(p.ArtifactRefs)
	return p
}

func (p CoverageEvidenceProof) Clone() CoverageEvidenceProof {
	p = p.Normalized()
	p.RequiredEffectScopes = append([]RuntimeEffectScope(nil), p.RequiredEffectScopes...)
	p.AllowedOperationKinds = append([]string(nil), p.AllowedOperationKinds...)
	p.AllowedOperationVariants = append([]RuntimeOperationVariantRef(nil), p.AllowedOperationVariants...)
	p.RequiredCheckRefs = append([]string(nil), p.RequiredCheckRefs...)
	p.ValidationCheckAttestations = CloneRuntimeValidationCheckAttestationBindings(p.ValidationCheckAttestations)
	p.MetricConstraints = append([]string(nil), p.MetricConstraints...)
	p.ArtifactRefs = append([]string(nil), p.ArtifactRefs...)
	return p
}

func (p CoverageEvidenceProof) Validate() error {
	if duplicate := duplicateRuntimeValidationProofCheckRef(p.RequiredCheckRefs); duplicate != "" {
		return fmt.Errorf("coverage evidence proof duplicates check %q", duplicate)
	}
	p = p.Normalized()
	if p.EvidenceKind != EvidenceKindStateChange && p.EvidenceKind != EvidenceKindValidation {
		return fmt.Errorf("coverage evidence proof has unsupported evidence kind %q", p.EvidenceKind)
	}
	if len(p.RequiredEffectScopes) == 0 || len(p.AllowedOperationKinds) == 0 {
		return fmt.Errorf("coverage evidence proof %q requires effect and operation proof shape", p.EvidenceKind)
	}
	for _, scope := range p.RequiredEffectScopes {
		switch p.EvidenceKind {
		case EvidenceKindStateChange:
			if !RuntimeEffectIsDurable(scope) {
				return fmt.Errorf("state-change proof requires a durable effect, got %q", scope)
			}
		case EvidenceKindValidation:
			if !RuntimeEffectIsNonDurable(scope) {
				return fmt.Errorf("validation proof requires a non-durable effect, got %q", scope)
			}
		}
	}
	for _, kind := range p.AllowedOperationKinds {
		if !RuntimeOperationKindSupportsCoverageEvidence(kind, p.EvidenceKind) {
			return fmt.Errorf("coverage evidence proof %q cannot use operation kind %q", p.EvidenceKind, kind)
		}
	}
	for _, variant := range p.AllowedOperationVariants {
		if !runtimeOperationKindsContain(p.AllowedOperationKinds, variant.Kind) {
			return fmt.Errorf("coverage evidence proof %q variant %q is outside allowed operation kinds", p.EvidenceKind, variant.VariantID)
		}
	}
	if p.EvidenceKind != EvidenceKindValidation &&
		(p.ValidationRecipeID != "" || p.ValidationProofRequirementHash != "" || len(p.RequiredCheckRefs) != 0 || len(p.ValidationCheckAttestations) != 0) {
		return fmt.Errorf("coverage evidence proof %q cannot carry validation recipe/check facts", p.EvidenceKind)
	}
	if p.EvidenceKind == EvidenceKindValidation && p.ValidationRecipeID == "" {
		return fmt.Errorf("validation proof requires a validation recipe")
	}
	if p.EvidenceKind == EvidenceKindValidation {
		managed := runtimeOperationKindsContain(p.AllowedOperationKinds, RuntimeBlockKindManagedValidationSession)
		if managed && (p.ValidationProofRequirementHash == "" || len(p.RequiredCheckRefs) == 0 || len(p.ValidationCheckAttestations) == 0) {
			return fmt.Errorf("managed validation proof requires an exact CWI proof requirement and native check attestations")
		}
		if managed {
			if err := ValidateRuntimeValidationCheckAttestationBindings(p.ValidationCheckAttestations, p.RequiredCheckRefs); err != nil {
				return err
			}
		}
		if !managed && (p.ValidationProofRequirementHash != "" || len(p.ValidationCheckAttestations) != 0) {
			return fmt.Errorf("non-managed validation proof cannot carry a Shell CWI proof requirement")
		}
	}
	return nil
}

// RuntimeOperationKindSupportsCoverageEvidence reports whether a concrete
// operation kind can provide the owner receipt facts required by a C12
// coverage proof. It is deliberately narrower than a descriptor's
// AllowedStates: an operation can be a valid non-credit diagnostic in
// need_validation without being able to attest a sealed recipe and its exact
// checks.
//
// State-change proof remains descriptor-driven because its owner receipt
// attests the concrete durable effect and target at execution time. Validation
// proof is limited to operation kinds whose owner emits an explicit typed
// RuntimeValidationObservation for the sealed proof identity. Adding another
// validation proof-capable operation requires extending this pure contract
// together with that capability owner's typed receipt implementation and
// regression tests; callers must not promote a successful read or generic
// exit status to proof.
func RuntimeOperationKindSupportsCoverageEvidence(kind, evidenceKind string) bool {
	kind = CanonicalRuntimeBlockKind(kind)
	evidenceKind = strings.TrimSpace(evidenceKind)
	descriptor, found := RuntimeOperationDescriptorForKind(kind)
	if !found || !descriptor.RequiresOwnerReceipt {
		return false
	}
	switch evidenceKind {
	case EvidenceKindStateChange:
		return runtimeDescriptorAllowsState(descriptor, ConvergenceStateNeedStateChange)
	case EvidenceKindValidation:
		return (kind == RuntimeBlockKindManagedValidationSession || kind == StepKindMCPToolCall) &&
			runtimeDescriptorAllowsState(descriptor, ConvergenceStateNeedValidation)
	default:
		return false
	}
}

// RuntimeValidationObservationSatisfiesCoverageProof matches one owner
// observation against the immutable validation proof. Shell managed
// validation resolves Definition checks only through the exact owner-native
// attestation mapping; raw output and semantic check names cannot attest
// themselves. Other typed validation owners retain their direct check
// vocabulary.
func RuntimeValidationObservationSatisfiesCoverageProof(proof CoverageEvidenceProof, observation RuntimeValidationObservation) bool {
	proof = proof.Normalized()
	observation = observation.Normalized()
	if proof.Validate() != nil || proof.EvidenceKind != EvidenceKindValidation ||
		observation.ValidateFailureDetails() != nil || observation.OutputTruncated ||
		observation.RecipeID != proof.ValidationRecipeID || len(RuntimeValidationConflictingChecks(observation)) != 0 {
		return false
	}
	if proof.ValidationProofRequirementHash != "" {
		for _, attestation := range proof.ValidationCheckAttestations {
			if observation.ObservationProtocolRef != attestation.ObservationProtocolRef ||
				!RuntimeValidationCheckPassed(observation, attestation.NativeCheckRef) {
				return false
			}
		}
		return len(proof.ValidationCheckAttestations) != 0
	}
	for _, required := range proof.RequiredCheckRefs {
		if !RuntimeValidationCheckPassed(observation, required) {
			return false
		}
	}
	return len(proof.RequiredCheckRefs) != 0
}

// RuntimeMCPToolValidationRecipeID is the exact, owner-attested validation
// proof identity for one MCP tool invocation. It is intentionally derived
// only from the sealed MCP server/tool pair: summary text, input values,
// provider output and candidate claims cannot alter the recipe identity.
//
// An empty value is invalid for a validation proof and is returned only so
// callers can reject incomplete owner facts without manufacturing an identity.
func RuntimeMCPToolValidationRecipeID(serverID, tool string) string {
	serverID = strings.TrimSpace(serverID)
	tool = strings.TrimSpace(tool)
	if serverID == "" || tool == "" {
		return ""
	}
	return "mcp/tool_call/" + serverID + "/" + tool
}

func NormalizeCoverageEvidenceProofs(values []CoverageEvidenceProof) []CoverageEvidenceProof {
	ret := make([]CoverageEvidenceProof, 0, len(values))
	for _, raw := range values {
		value := raw.Clone()
		if value.EvidenceKind == "" {
			continue
		}
		ret = append(ret, value)
	}
	sort.SliceStable(ret, func(i, j int) bool { return ret[i].EvidenceKind < ret[j].EvidenceKind })
	return nilIfEmpty(ret)
}

// CoverageEvidenceBinding is the immutable, SM-2-sealed proof contract for
// exactly one coverage work item. It is a pure value: only the SM-2 reducer may
// seal semantic proposal/owner-fact agreement or turn a receipt match into
// coverage credit.
type CoverageEvidenceBinding struct {
	ID                       string
	CoverageWorkItemID       common.CoverageWorkItemID
	FocusID                  string
	DefinitionRevision       string
	Proofs                   []CoverageEvidenceProof
	TargetRefs               []string
	TargetSelectorRef        string
	TargetSelectorOwnerRef   string
	TargetSelectorSubjectRef string
	// TargetSelectorCandidateRefs is the bounded declaration required by a
	// selector that cannot be independently replayed from the workspace alone.
	// It is sealed with the binding so reconciliation can ask the owner to
	// verify the same target surface after a process restart.
	TargetSelectorCandidateRefs []string
	// ProspectiveTargetAllOfConstraintRefs is the exact mandatory Definition
	// declaration accepted for this semantic generation.
	ProspectiveTargetAllOfConstraintRefs []string
	// ProspectiveTargetRefs is the complete bounded target set accepted by owner
	// facts and sealed by SM-2. It is generation authority and is never a
	// pre-seal Definition/Profile declaration.
	ProspectiveTargetRefs []string
	DeclarationRef        string
	PrerequisiteRefs      []string
	Hash                  string
}

func (b CoverageEvidenceBinding) Normalized() CoverageEvidenceBinding {
	b.ID = strings.TrimSpace(b.ID)
	b.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(b.CoverageWorkItemID)))
	b.FocusID = strings.TrimSpace(b.FocusID)
	b.DefinitionRevision = strings.TrimSpace(b.DefinitionRevision)
	b.Proofs = NormalizeCoverageEvidenceProofs(b.Proofs)
	b.TargetRefs = normalizeStrings(b.TargetRefs)
	b.TargetSelectorRef = strings.TrimSpace(b.TargetSelectorRef)
	b.TargetSelectorOwnerRef = strings.TrimSpace(b.TargetSelectorOwnerRef)
	b.TargetSelectorSubjectRef = strings.TrimSpace(b.TargetSelectorSubjectRef)
	b.TargetSelectorCandidateRefs = normalizeStrings(b.TargetSelectorCandidateRefs)
	b.ProspectiveTargetAllOfConstraintRefs = NormalizeRuntimeProspectiveTargetAllOfConstraintRefs(b.ProspectiveTargetAllOfConstraintRefs)
	b.ProspectiveTargetRefs = normalizeStrings(b.ProspectiveTargetRefs)
	b.DeclarationRef = strings.TrimSpace(b.DeclarationRef)
	b.PrerequisiteRefs = normalizeStrings(b.PrerequisiteRefs)
	b.Hash = strings.TrimSpace(b.Hash)
	return b
}

func (b CoverageEvidenceBinding) Clone() CoverageEvidenceBinding {
	b = b.Normalized()
	b.Proofs = append([]CoverageEvidenceProof(nil), b.Proofs...)
	for index := range b.Proofs {
		b.Proofs[index] = b.Proofs[index].Clone()
	}
	b.TargetRefs = append([]string(nil), b.TargetRefs...)
	b.TargetSelectorCandidateRefs = append([]string(nil), b.TargetSelectorCandidateRefs...)
	b.ProspectiveTargetAllOfConstraintRefs = append([]string(nil), b.ProspectiveTargetAllOfConstraintRefs...)
	b.ProspectiveTargetRefs = append([]string(nil), b.ProspectiveTargetRefs...)
	b.PrerequisiteRefs = append([]string(nil), b.PrerequisiteRefs...)
	return b
}

func (b CoverageEvidenceBinding) Validate() error {
	seenProofKinds := make(map[string]struct{}, len(b.Proofs))
	for _, proof := range b.Proofs {
		kind := strings.TrimSpace(proof.EvidenceKind)
		if kind == "" {
			return fmt.Errorf("coverage evidence binding %q has a proof without evidence kind", strings.TrimSpace(b.ID))
		}
		if _, duplicate := seenProofKinds[kind]; duplicate {
			return fmt.Errorf("coverage evidence binding %q duplicates evidence proof %q", strings.TrimSpace(b.ID), kind)
		}
		seenProofKinds[kind] = struct{}{}
		if err := proof.Validate(); err != nil {
			return fmt.Errorf("coverage evidence binding %q: %w", strings.TrimSpace(b.ID), err)
		}
	}
	b = b.Normalized()
	if b.ID == "" || b.CoverageWorkItemID == "" || b.FocusID == "" || b.DefinitionRevision == "" || b.DeclarationRef == "" {
		return fmt.Errorf("coverage evidence binding requires identity, coverage item, focus, definition and declaration")
	}
	if len(b.Proofs) == 0 {
		return fmt.Errorf("coverage evidence binding %q requires at least one evidence proof", b.ID)
	}
	if len(b.TargetRefs) == 0 && b.TargetSelectorRef == "" && len(b.ProspectiveTargetRefs) == 0 {
		return fmt.Errorf("coverage evidence binding %q requires an exact target, selector or prospective target", b.ID)
	}
	if b.TargetSelectorRef != "" && (b.TargetSelectorOwnerRef == "" || b.TargetSelectorSubjectRef == "") {
		return fmt.Errorf("coverage evidence binding %q selector requires owner and subject", b.ID)
	}
	if len(b.TargetSelectorCandidateRefs) != 0 && b.TargetSelectorRef == "" {
		return fmt.Errorf("coverage evidence binding %q selector candidates require a selector", b.ID)
	}
	if len(b.ProspectiveTargetAllOfConstraintRefs) != 0 {
		if b.TargetSelectorRef == "" || b.TargetSelectorOwnerRef == "" || b.TargetSelectorSubjectRef == "" {
			return fmt.Errorf("coverage evidence binding %q prospective target all-of declaration requires a complete selector", b.ID)
		}
		if err := ValidateRuntimeProspectiveTargetAllOfConstraintRefs(b.ProspectiveTargetAllOfConstraintRefs); err != nil {
			return fmt.Errorf("coverage evidence binding %q: %w", b.ID, err)
		}
	}
	if len(b.ProspectiveTargetRefs) != 0 {
		if len(b.TargetRefs) != 0 || b.TargetSelectorRef == "" || b.TargetSelectorOwnerRef == "" || b.TargetSelectorSubjectRef == "" {
			return fmt.Errorf("coverage evidence binding %q prospective targets require only a complete selector target surface", b.ID)
		}
		if len(b.ProspectiveTargetRefs) > RuntimeProspectiveTargetRefsMaxCount {
			return fmt.Errorf("coverage evidence binding %q prospective target set exceeds %d", b.ID, RuntimeProspectiveTargetRefsMaxCount)
		}
		for _, targetRef := range b.ProspectiveTargetRefs {
			if len(b.TargetSelectorCandidateRefs) != 0 && !containsString(b.TargetSelectorCandidateRefs, targetRef) {
				return fmt.Errorf("coverage evidence binding %q prospective target %q is outside selector candidates", b.ID, targetRef)
			}
		}
	}
	if b.Hash != "" && b.Hash != StableCoverageEvidenceBindingHash(b) {
		return fmt.Errorf("coverage evidence binding %q hash does not match contents", b.ID)
	}
	return nil
}

// StableCoverageEvidenceBindingHash is the immutable value identity used in a
// sealed manifest/dispatch. It intentionally excludes Hash itself.
func StableCoverageEvidenceBindingHash(binding CoverageEvidenceBinding) string {
	binding = binding.Normalized()
	binding.Hash = ""
	h := sha256.New()
	writeHashString(h, binding.ID)
	writeHashString(h, string(binding.CoverageWorkItemID))
	writeHashString(h, binding.FocusID)
	writeHashString(h, binding.DefinitionRevision)
	for _, proof := range binding.Proofs {
		writeHashString(h, proof.EvidenceKind)
		writeHashStrings(h, RuntimeEffectScopesAsStrings(proof.RequiredEffectScopes))
		writeHashStrings(h, proof.AllowedOperationKinds)
		for _, variant := range proof.AllowedOperationVariants {
			writeHashString(h, variant.Kind)
			writeHashString(h, variant.VariantID)
		}
		writeHashString(h, proof.ValidationRecipeID)
		writeHashString(h, proof.ValidationProofRequirementHash)
		writeHashStrings(h, proof.RequiredCheckRefs)
		for _, attestation := range proof.ValidationCheckAttestations {
			writeHashString(h, attestation.RequiredCheckRef)
			writeHashString(h, attestation.ObservationProtocolRef)
			writeHashString(h, attestation.NativeCheckRef)
		}
		writeHashStrings(h, proof.MetricConstraints)
		writeHashStrings(h, proof.ArtifactRefs)
	}
	writeHashStrings(h, binding.TargetRefs)
	writeHashString(h, binding.TargetSelectorRef)
	writeHashString(h, binding.TargetSelectorOwnerRef)
	writeHashString(h, binding.TargetSelectorSubjectRef)
	writeHashStrings(h, binding.TargetSelectorCandidateRefs)
	writeHashStrings(h, binding.ProspectiveTargetAllOfConstraintRefs)
	writeHashStrings(h, binding.ProspectiveTargetRefs)
	writeHashString(h, binding.DeclarationRef)
	writeHashStrings(h, binding.PrerequisiteRefs)
	return hex.EncodeToString(h.Sum(nil))
}

func NormalizeCoverageEvidenceBindings(values []CoverageEvidenceBinding) []CoverageEvidenceBinding {
	ret := make([]CoverageEvidenceBinding, 0, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.ID == "" {
			continue
		}
		value.Hash = StableCoverageEvidenceBindingHash(value)
		ret = append(ret, value)
	}
	// Preserve duplicate identities until an authority validator can reject
	// them. A normalizer must never choose which sealed CWI binding wins.
	sort.SliceStable(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	return nilIfEmpty(ret)
}

// CloneCoverageEvidenceBindings detaches the complete immutable binding
// projection selected by one SM-3 admission/dispatch seal.
func CloneCoverageEvidenceBindings(values []CoverageEvidenceBinding) []CoverageEvidenceBinding {
	return NormalizeCoverageEvidenceBindings(values)
}

// ValidateCoverageEvidenceBindingProjection requires the persisted projection
// to already be canonical. Recovery must not normalize duplicate, reordered or
// hash-drifted binding authority into a different valid value.
func ValidateCoverageEvidenceBindingProjection(values []CoverageEvidenceBinding) error {
	normalized := NormalizeCoverageEvidenceBindings(values)
	if !reflect.DeepEqual(values, normalized) {
		return fmt.Errorf("coverage evidence binding projection is not canonical")
	}
	seen := make(map[string]struct{}, len(values))
	for _, binding := range values {
		if _, duplicate := seen[binding.ID]; duplicate {
			return fmt.Errorf("coverage evidence binding projection duplicates %q", binding.ID)
		}
		seen[binding.ID] = struct{}{}
		if err := binding.Validate(); err != nil {
			return fmt.Errorf("coverage evidence binding projection %q: %w", binding.ID, err)
		}
	}
	return nil
}

// StableRuntimeCoverageEvidenceBindingSnapshotHash commits the complete
// canonical binding bodies. Individual binding hashes are included as audit
// identities, while every body field is already committed by that hash.
func StableRuntimeCoverageEvidenceBindingSnapshotHash(values []CoverageEvidenceBinding) string {
	bindings := NormalizeCoverageEvidenceBindings(values)
	h := sha256.New()
	writeHashString(h, "runtime-coverage-evidence-binding-snapshot/v1")
	writeHashString(h, fmt.Sprintf("%d", len(bindings)))
	for _, binding := range bindings {
		writeHashString(h, binding.ID)
		writeHashString(h, binding.Hash)
	}
	return "runtime-coverage-binding-snapshot-" + hex.EncodeToString(h.Sum(nil))
}

func RuntimeCoverageEvidenceBindingProjectionsEqual(left, right []CoverageEvidenceBinding) bool {
	return reflect.DeepEqual(NormalizeCoverageEvidenceBindings(left), NormalizeCoverageEvidenceBindings(right))
}

// ValidateCoverageEvidenceBindingsForFocus verifies the static declaration
// only. It never receives a receipt and cannot grant credit.
func ValidateCoverageEvidenceBindingsForFocus(focus CoverageFocus, definitionRevision string) error {
	seenRawIDs := make(map[string]struct{}, len(focus.CoverageEvidenceBindings))
	for _, binding := range focus.CoverageEvidenceBindings {
		if err := binding.Validate(); err != nil {
			return err
		}
		id := strings.TrimSpace(binding.ID)
		if _, duplicate := seenRawIDs[id]; duplicate {
			return fmt.Errorf("coverage binding projection duplicates %q", id)
		}
		seenRawIDs[id] = struct{}{}
	}
	focus = focus.Normalized()
	definitionRevision = strings.TrimSpace(definitionRevision)
	// A presentation slice may contain several CWI, but validation, executable
	// behavior, metric, and artifact proof are semantic obligations.  Their
	// bindings must therefore be compiled into one-CWI focuses; copying a
	// focus-level proof declaration would otherwise permit one receipt to fan
	// out across unrelated claims.  This is a pure static invariant and does
	// not interpret receipts or transition coverage state.
	if err := ValidateRuntimeFocusEvidenceDeclaration(focus); err != nil {
		return err
	}
	bindings := NormalizeCoverageEvidenceBindings(focus.CoverageEvidenceBindings)
	if len(bindings) != len(focus.CoverageWorkItemIDs) {
		return fmt.Errorf("coverage binding unavailable for focus %q: requires exactly one binding per coverage item", focus.ID)
	}
	expected := make(map[common.CoverageWorkItemID]struct{}, len(focus.CoverageWorkItemIDs))
	for _, id := range focus.CoverageWorkItemIDs {
		expected[id] = struct{}{}
	}
	seen := make(map[common.CoverageWorkItemID]struct{}, len(bindings))
	for _, binding := range bindings {
		if err := binding.Validate(); err != nil {
			return err
		}
		if binding.FocusID != focus.ID || binding.DefinitionRevision != definitionRevision {
			return fmt.Errorf("coverage binding %q does not match focus/definition identity", binding.ID)
		}
		if !sameStrings(binding.EvidenceKinds(), focus.RequiredEvidenceKinds) {
			return fmt.Errorf("coverage binding %q proof kinds do not match focus requirements", binding.ID)
		}
		if !sameStrings(binding.TargetRefs, focus.TargetRefs) ||
			binding.TargetSelectorRef != focus.TargetSelectorRef ||
			binding.TargetSelectorOwnerRef != focus.TargetSelectorOwnerRef ||
			binding.TargetSelectorSubjectRef != focus.TargetSelectorSubjectRef ||
			!sameStrings(binding.TargetSelectorCandidateRefs, focus.TargetSelectorCandidateRefs) ||
			!SameRuntimeProspectiveTargetAllOfConstraintRefs(binding.ProspectiveTargetAllOfConstraintRefs, focus.ProspectiveTargetAllOfConstraintRefs) ||
			!sameStrings(binding.ProspectiveTargetRefs, focus.ProspectiveTargetRefs) {
			return fmt.Errorf("coverage binding %q target surface does not match focus authority", binding.ID)
		}
		if _, ok := expected[binding.CoverageWorkItemID]; !ok {
			return fmt.Errorf("coverage binding %q references coverage item outside focus", binding.ID)
		}
		if _, duplicate := seen[binding.CoverageWorkItemID]; duplicate {
			return fmt.Errorf("coverage binding %q duplicates coverage item %s", binding.ID, binding.CoverageWorkItemID)
		}
		seen[binding.CoverageWorkItemID] = struct{}{}
	}
	return nil
}

// ValidateRuntimeFocusEvidenceDeclaration is a pure activation/bootstrap
// invariant. It rejects a semantic proof copied from a wildcard quality rule
// before any owner receipt, Run, or provider request can exist.
func ValidateRuntimeFocusEvidenceDeclaration(focus CoverageFocus) error {
	focus = focus.Normalized()
	if err := ValidateRuntimeOwnerFactRequirements(focus.RequiredOwnerFacts); err != nil {
		return fmt.Errorf("coverage focus %q owner facts: %w", focus.ID, err)
	}
	if err := ValidateRuntimeOwnerFactConvergences(focus.RequiredOwnerFacts, focus.OwnerFactConvergences); err != nil {
		return fmt.Errorf("coverage focus %q owner-fact convergence: %w", focus.ID, err)
	}
	if !RuntimeFocusRequiresIndependentEvidenceBinding(focus) {
		return nil
	}
	if len(focus.CoverageWorkItemIDs) != 1 {
		return fmt.Errorf("coverage binding unavailable for focus %q: semantic proof must be bound to exactly one CWI, got %d", focus.ID, len(focus.CoverageWorkItemIDs))
	}
	if strings.HasSuffix(focus.EvidenceDeclarationRef, "/*") {
		return fmt.Errorf("coverage binding unavailable for focus %q: semantic proof requires a CWI-specific declaration; wildcard declaration %q is a quality gate and cannot close coverage", focus.ID, focus.EvidenceDeclarationRef)
	}
	return nil
}

// RuntimeFocusRequiresIndependentEvidenceBinding identifies focus declarations
// for which receipt evidence has CWI semantics.  It is intentionally a pure
// contract predicate: activation uses it to compile focus shape and bootstrap
// validation uses it to reject an invalid persisted shape.  It never observes
// an owner receipt or grants coverage credit.
func RuntimeFocusRequiresIndependentEvidenceBinding(focus CoverageFocus) bool {
	focus = focus.Normalized()
	return containsEvidenceKind(focus.RequiredEvidenceKinds, EvidenceKindValidation) ||
		focus.RequireExecutableBehaviorEvidence || len(focus.RequiredOwnerFacts) != 0 || len(focus.MetricConstraints) != 0 || len(focus.ArtifactRefs) != 0
}

func containsEvidenceKind(values []string, wanted string) bool {
	wanted = strings.TrimSpace(wanted)
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}

func CoverageEvidenceBindingForRef(values []CoverageEvidenceBinding, ref string) (CoverageEvidenceBinding, bool) {
	ref = strings.TrimSpace(ref)
	for _, value := range NormalizeCoverageEvidenceBindings(values) {
		if value.ID == ref {
			return value.Clone(), true
		}
	}
	return CoverageEvidenceBinding{}, false
}

// EvidenceKinds returns the exactly sealed proof kind set. It is a read-only
// pure helper for manifest/evidence validation and never interprets coverage.
func (b CoverageEvidenceBinding) EvidenceKinds() []string {
	b = b.Normalized()
	ret := make([]string, 0, len(b.Proofs))
	for _, proof := range b.Proofs {
		ret = append(ret, proof.EvidenceKind)
	}
	return normalizeStrings(ret)
}

// CoverageEvidenceProofForKind returns one exact proof shape from a sealed
// binding. There is intentionally no broad kind/effect union fallback.
func CoverageEvidenceProofForKind(binding CoverageEvidenceBinding, evidenceKind string) (CoverageEvidenceProof, bool) {
	evidenceKind = strings.TrimSpace(evidenceKind)
	for _, proof := range binding.Normalized().Proofs {
		if proof.EvidenceKind == evidenceKind {
			return proof.Clone(), true
		}
	}
	return CoverageEvidenceProof{}, false
}

func CoverageEvidenceBindingRefsForOperation(values []CoverageEvidenceBinding, kind string) []string {
	return CoverageEvidenceBindingRefsForOperationEvidenceKind(values, kind, "")
}

// CoverageEvidenceBindingRefsForOperationEvidenceKind returns only binding
// refs whose exact proof can produce the requested evidence kind. An empty
// evidenceKind is reserved for diagnostic/read-only callers; current manifest
// construction always passes its sealed convergence state.
func CoverageEvidenceBindingRefsForOperationEvidenceKind(values []CoverageEvidenceBinding, kind, evidenceKind string) []string {
	kind = CanonicalRuntimeBlockKind(kind)
	evidenceKind = strings.TrimSpace(evidenceKind)
	ret := make([]string, 0, len(values))
	for _, binding := range NormalizeCoverageEvidenceBindings(values) {
		for _, proof := range binding.Proofs {
			if evidenceKind != "" && proof.EvidenceKind != evidenceKind {
				continue
			}
			if runtimeOperationKindsContain(proof.AllowedOperationKinds, kind) {
				ret = append(ret, binding.ID)
				break
			}
		}
	}
	return normalizeStrings(ret)
}

// RuntimeOperationProofIdentity is the immutable, non-executable portion of a
// sealed operation needed to replay one coverage-proof decision. It exists so
// audit projections can reuse the exact production matcher without exposing
// operation parameters or implementing target/variant semantics again.
//
// This value never grants execution authority and never changes Runtime state.
// The executable RuntimeOperation remains the SM-3 authority.
type RuntimeOperationProofIdentity struct {
	Kind       string
	VariantID  string
	TargetRefs []string
}

func (i RuntimeOperationProofIdentity) Normalized() RuntimeOperationProofIdentity {
	i.Kind = CanonicalRuntimeBlockKind(i.Kind)
	i.VariantID = strings.TrimSpace(i.VariantID)
	i.TargetRefs = normalizeStrings(i.TargetRefs)
	return i
}

// RuntimeOperationProofIdentityForOperation derives audit-safe proof identity
// from one executable sealed operation. Variant selection stays centralized in
// the operation contract; callers must not reimplement discriminator rules.
func RuntimeOperationProofIdentityForOperation(operation RuntimeOperation) (RuntimeOperationProofIdentity, error) {
	operation = operation.Normalized()
	identity := RuntimeOperationProofIdentity{
		Kind:       operation.Kind,
		TargetRefs: append([]string(nil), operation.TargetRefs...),
	}.Normalized()
	if identity.Kind == "" {
		return RuntimeOperationProofIdentity{}, fmt.Errorf("runtime operation proof identity requires operation kind")
	}
	contract, found := RuntimeOperationContractForKind(identity.Kind)
	if !found || len(contract.Variants) == 0 {
		return identity, nil
	}
	variant, err := MatchRuntimeOperationVariant(operation, contract)
	if err != nil {
		return RuntimeOperationProofIdentity{}, err
	}
	identity.VariantID = strings.TrimSpace(variant.ID)
	return identity, nil
}

// MatchCoverageEvidenceBindingReceipt is a pure exact-proof matcher. It
// compares only a sealed binding, observed evidence kind, sealed operation and
// owner receipt fact. It deliberately does not examine Goal/DoD text,
// candidate CWI IDs, assessment claims, summaries or provider output.
//
// One CWI may require both a durable change and validation. Recipe/check proof
// applies only to its validation receipt; requiring it on a state-change
// receipt would make that declarative conjunction impossible to satisfy.
func MatchCoverageEvidenceBindingReceipt(binding CoverageEvidenceBinding, evidenceKind string, operation RuntimeOperation, receipt RuntimeOperationReceipt) error {
	binding = binding.Normalized()
	operation = operation.Normalized()
	evidenceKind = strings.TrimSpace(evidenceKind)
	identity := RuntimeOperationProofIdentity{Kind: operation.Kind, TargetRefs: append([]string(nil), operation.TargetRefs...)}.Normalized()
	// Preserve the original receipt matcher boundary: variant discrimination is
	// required only when the sealed proof restricts variants. Audit projection
	// may still derive the canonical variant for every fully sealed operation.
	if proof, found := CoverageEvidenceProofForKind(binding, evidenceKind); found && len(proof.AllowedOperationVariants) != 0 {
		contract, known := RuntimeOperationContractForKind(operation.Kind)
		if !known {
			return fmt.Errorf("runtime operation proof identity has unknown kind %q", operation.Kind)
		}
		variant, err := MatchRuntimeOperationVariant(operation, contract)
		if err != nil {
			return err
		}
		identity.VariantID = strings.TrimSpace(variant.ID)
	}
	return MatchCoverageEvidenceBindingReceiptIdentity(binding, evidenceKind, identity, receipt)
}

// MatchCoverageEvidenceBindingReceiptIdentity is the single pure proof
// evaluator shared by the SM-2 receipt-credit path and read-only audit tools.
// An audit caller supplies only identity projected from an already sealed SM-3
// operation; it cannot execute that operation or trigger a state transition.
func MatchCoverageEvidenceBindingReceiptIdentity(binding CoverageEvidenceBinding, evidenceKind string, operation RuntimeOperationProofIdentity, receipt RuntimeOperationReceipt) error {
	binding = binding.Normalized()
	operation = operation.Normalized()
	receipt = receipt.Normalized()
	evidenceKind = strings.TrimSpace(evidenceKind)
	if err := binding.Validate(); err != nil {
		return err
	}
	proof, found := CoverageEvidenceProofForKind(binding, evidenceKind)
	if !found {
		return fmt.Errorf("binding %q does not require evidence kind %q", binding.ID, evidenceKind)
	}
	if !runtimeOperationKindsContain(proof.AllowedOperationKinds, operation.Kind) {
		return fmt.Errorf("binding %q does not allow operation kind %q", binding.ID, operation.Kind)
	}
	// C12 credit is only derivable from an accepted owner fact.  An execution
	// result may still be recorded for diagnostics when it is rejected/no-op or
	// has weak validation quality, but that result is not proof for a CWI.
	// Quality is intentionally evaluated here, beside effect/target/recipe,
	// rather than later in assessment-only code, so a weak receipt never enters
	// EvidenceFact.CoverageWorkItemIDs as provisional credit.
	if receipt.Outcome != RuntimeOperationOutcomeAccepted {
		return fmt.Errorf("binding %q requires an accepted owner receipt, got %q", binding.ID, receipt.Outcome)
	}
	if receipt.Evidence.ZeroWork {
		return fmt.Errorf("binding %q cannot use zero-work owner evidence", binding.ID)
	}
	switch common.EvidenceQuality(strings.TrimSpace(string(receipt.Evidence.Quality))) {
	case common.EvidenceQualityInvalid, common.EvidenceQualityInconclusive, common.EvidenceQualitySkipped:
		return fmt.Errorf("binding %q cannot use weak owner evidence quality %q", binding.ID, receipt.Evidence.Quality)
	}
	if evidenceKind == EvidenceKindStateChange && !receipt.Changed {
		return fmt.Errorf("binding %q state-change proof requires a changed owner receipt", binding.ID)
	}
	if len(proof.AllowedOperationVariants) != 0 {
		matched := false
		for _, allowed := range proof.AllowedOperationVariants {
			if allowed.Kind == operation.Kind && allowed.VariantID == operation.VariantID {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("binding %q does not allow operation variant %q", binding.ID, operation.VariantID)
		}
	}
	if !runtimeEffectScopeIncluded(proof.RequiredEffectScopes, receipt.ObservedEffectScope) {
		return fmt.Errorf("binding %q proof %q requires effect %v, receipt observed %q", binding.ID, evidenceKind, proof.RequiredEffectScopes, receipt.ObservedEffectScope)
	}
	targetAuthorization, err := RuntimeOperationTargetAuthorizationFromCoverageEvidenceBinding(binding)
	if err != nil {
		return fmt.Errorf("binding %q target authorization: %w", binding.ID, err)
	}
	// A binding with a finite target surface proves a concrete owner target;
	// selector-only existing scope is the sole shape whose membership remains
	// an owner-feasibility fact. prospective_owner_target is always singleton
	// per operation, while dispatch-level proof separately requires every member.
	requireOperationTargets := targetAuthorization.Policy != RuntimeOperationTargetPolicyExistingOwnerScope || len(targetAuthorization.OwnerScopeTargetRefs) != 0
	if err := ValidateRuntimeOperationTargetsAgainstAuthorization(targetAuthorization, operation.TargetRefs, requireOperationTargets); err != nil {
		return fmt.Errorf("binding %q sealed operation targets are unauthorized: %w", binding.ID, err)
	}
	if !sameStrings(operation.TargetRefs, receipt.TargetRefs) {
		return fmt.Errorf("binding %q receipt targets do not match sealed operation", binding.ID)
	}
	if evidenceKind == EvidenceKindValidation {
		if receipt.Evidence.Validation == nil {
			return fmt.Errorf("binding %q requires validation observation", binding.ID)
		}
		if !RuntimeValidationObservationSatisfiesCoverageProof(proof, *receipt.Evidence.Validation) {
			return fmt.Errorf("binding %q validation observation does not satisfy sealed proof", binding.ID)
		}
	}
	for _, metric := range proof.MetricConstraints {
		if !containsString(receipt.Evidence.EvidenceTags, metric) {
			return fmt.Errorf("binding %q requires metric constraint %q", binding.ID, metric)
		}
	}
	for _, artifact := range proof.ArtifactRefs {
		if !containsString(receipt.Evidence.EvidenceRefs, artifact) {
			return fmt.Errorf("binding %q requires artifact ref %q", binding.ID, artifact)
		}
	}
	return nil
}

func runtimeEffectScopeIncluded(values []RuntimeEffectScope, actual RuntimeEffectScope) bool {
	actual = NormalizeRuntimeEffectScope(actual)
	for _, value := range NormalizeRuntimeEffectScopes(values) {
		if value == actual {
			return true
		}
	}
	return false
}

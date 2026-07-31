package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	EvidenceKind             string
	RequiredEffectScopes     []RuntimeEffectScope
	AllowedOperationKinds    []string
	AllowedOperationVariants []RuntimeOperationVariantRef
	ValidationRecipeID       string
	RequiredCheckRefs        []string
	MetricConstraints        []string
	ArtifactRefs             []string
}

func (p CoverageEvidenceProof) Normalized() CoverageEvidenceProof {
	p.EvidenceKind = strings.TrimSpace(p.EvidenceKind)
	p.RequiredEffectScopes = NormalizeRuntimeEffectScopes(p.RequiredEffectScopes)
	p.AllowedOperationKinds = canonicalizeOperationKinds(p.AllowedOperationKinds)
	p.AllowedOperationVariants = NormalizeRuntimeOperationVariantRefs(p.AllowedOperationVariants)
	p.ValidationRecipeID = strings.TrimSpace(p.ValidationRecipeID)
	p.RequiredCheckRefs = normalizeStrings(p.RequiredCheckRefs)
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
	p.MetricConstraints = append([]string(nil), p.MetricConstraints...)
	p.ArtifactRefs = append([]string(nil), p.ArtifactRefs...)
	return p
}

func (p CoverageEvidenceProof) Validate() error {
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
		(p.ValidationRecipeID != "" || len(p.RequiredCheckRefs) != 0) {
		return fmt.Errorf("coverage evidence proof %q cannot carry validation recipe/check facts", p.EvidenceKind)
	}
	if p.EvidenceKind == EvidenceKindValidation && p.ValidationRecipeID == "" {
		return fmt.Errorf("validation proof requires a validation recipe")
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
	byKind := make(map[string]CoverageEvidenceProof, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.EvidenceKind == "" {
			continue
		}
		byKind[value.EvidenceKind] = value
	}
	kinds := make([]string, 0, len(byKind))
	for kind := range byKind {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	ret := make([]CoverageEvidenceProof, 0, len(kinds))
	for _, kind := range kinds {
		ret = append(ret, byKind[kind].Clone())
	}
	return nilIfEmpty(ret)
}

// CoverageEvidenceBinding is the immutable, profile/template-compiled proof
// contract for exactly one coverage work item. It is a pure value: only the
// SM-2 reducer may turn a receipt match into coverage credit.
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
	ProspectiveTargetRef     string
	DeclarationRef           string
	PrerequisiteRefs         []string
	Hash                     string
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
	b.ProspectiveTargetRef = strings.TrimSpace(b.ProspectiveTargetRef)
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
	b.PrerequisiteRefs = append([]string(nil), b.PrerequisiteRefs...)
	return b
}

func (b CoverageEvidenceBinding) Validate() error {
	b = b.Normalized()
	if b.ID == "" || b.CoverageWorkItemID == "" || b.FocusID == "" || b.DefinitionRevision == "" || b.DeclarationRef == "" {
		return fmt.Errorf("coverage evidence binding requires identity, coverage item, focus, definition and declaration")
	}
	if len(b.Proofs) == 0 {
		return fmt.Errorf("coverage evidence binding %q requires at least one evidence proof", b.ID)
	}
	for _, proof := range b.Proofs {
		if err := proof.Validate(); err != nil {
			return fmt.Errorf("coverage evidence binding %q: %w", b.ID, err)
		}
	}
	if len(b.TargetRefs) == 0 && b.TargetSelectorRef == "" && b.ProspectiveTargetRef == "" {
		return fmt.Errorf("coverage evidence binding %q requires an exact target, selector or prospective target", b.ID)
	}
	if b.TargetSelectorRef != "" && (b.TargetSelectorOwnerRef == "" || b.TargetSelectorSubjectRef == "") {
		return fmt.Errorf("coverage evidence binding %q selector requires owner and subject", b.ID)
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
		writeHashStrings(h, proof.RequiredCheckRefs)
		writeHashStrings(h, proof.MetricConstraints)
		writeHashStrings(h, proof.ArtifactRefs)
	}
	writeHashStrings(h, binding.TargetRefs)
	writeHashString(h, binding.TargetSelectorRef)
	writeHashString(h, binding.TargetSelectorOwnerRef)
	writeHashString(h, binding.TargetSelectorSubjectRef)
	writeHashString(h, binding.ProspectiveTargetRef)
	writeHashString(h, binding.DeclarationRef)
	writeHashStrings(h, binding.PrerequisiteRefs)
	return hex.EncodeToString(h.Sum(nil))
}

func NormalizeCoverageEvidenceBindings(values []CoverageEvidenceBinding) []CoverageEvidenceBinding {
	byID := make(map[string]CoverageEvidenceBinding, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.ID == "" {
			continue
		}
		value.Hash = StableCoverageEvidenceBindingHash(value)
		byID[value.ID] = value
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	ret := make([]CoverageEvidenceBinding, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, byID[id].Clone())
	}
	return nilIfEmpty(ret)
}

// ValidateCoverageEvidenceBindingsForFocus verifies the static declaration
// only. It never receives a receipt and cannot grant credit.
func ValidateCoverageEvidenceBindingsForFocus(focus CoverageFocus, definitionRevision string) error {
	focus = focus.Normalized()
	definitionRevision = strings.TrimSpace(definitionRevision)
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
		contract, found := RuntimeOperationContractForKind(operation.Kind)
		if !found {
			return fmt.Errorf("binding %q operation kind %q has no variant contract", binding.ID, operation.Kind)
		}
		variant, err := MatchRuntimeOperationVariant(operation, contract)
		if err != nil {
			return fmt.Errorf("binding %q operation variant: %w", binding.ID, err)
		}
		matched := false
		for _, allowed := range proof.AllowedOperationVariants {
			if allowed.Kind == operation.Kind && allowed.VariantID == variant.ID {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("binding %q does not allow operation variant %q", binding.ID, variant.ID)
		}
	}
	if !runtimeEffectScopeIncluded(proof.RequiredEffectScopes, receipt.ObservedEffectScope) {
		return fmt.Errorf("binding %q proof %q requires effect %v, receipt observed %q", binding.ID, evidenceKind, proof.RequiredEffectScopes, receipt.ObservedEffectScope)
	}
	if !sameStrings(operation.TargetRefs, receipt.TargetRefs) {
		return fmt.Errorf("binding %q receipt targets do not match sealed operation", binding.ID)
	}
	if len(binding.TargetRefs) != 0 && !sameStrings(binding.TargetRefs, receipt.TargetRefs) {
		return fmt.Errorf("binding %q receipt targets do not match exact binding target", binding.ID)
	}
	if len(binding.TargetRefs) == 0 && binding.TargetSelectorRef == "" && binding.ProspectiveTargetRef == "" {
		return fmt.Errorf("binding %q has no target authority", binding.ID)
	}
	if binding.ProspectiveTargetRef != "" && !sameStrings([]string{binding.ProspectiveTargetRef}, receipt.TargetRefs) {
		return fmt.Errorf("binding %q receipt targets do not match prospective target", binding.ID)
	}
	if evidenceKind == EvidenceKindValidation {
		if receipt.Evidence.Validation == nil {
			return fmt.Errorf("binding %q requires validation observation", binding.ID)
		}
		observation := receipt.Evidence.Validation.Normalized()
		if observation.RecipeID != proof.ValidationRecipeID {
			return fmt.Errorf("binding %q requires recipe %q", binding.ID, proof.ValidationRecipeID)
		}
		for _, required := range proof.RequiredCheckRefs {
			if !containsString(observation.PassedChecks, required) {
				return fmt.Errorf("binding %q requires passed check %q", binding.ID, required)
			}
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

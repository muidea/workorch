package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// CoverageEvidenceProofIdentity is the immutable semantic identity of one
// evidence-kind proof inside a sealed CoverageEvidenceBinding. It deliberately
// excludes binding, batch, manifest, request and prerequisite-observation
// identities: those are transaction/audit fences rather than the obligation
// that an owner receipt proved.
//
// This is a pure contract value. It neither grants coverage credit nor decides
// a Runtime transition; those decisions remain Orchestrator SM-2-owned.
type CoverageEvidenceProofIdentity struct {
	DefinitionRevision             string
	CoverageWorkItemID             common.CoverageWorkItemID
	EvidenceKind                   string
	OwnerModuleID                  string
	AllowedOperationKinds          []string
	AllowedOperationVariants       []RuntimeOperationVariantRef
	TargetPolicy                   CoverageEvidenceProofTargetPolicy
	TargetRefs                     []string
	TargetSelectorRef              string
	TargetSelectorOwnerRef         string
	TargetSelectorSubjectRef       string
	ProspectiveTargetRefs          []string
	RequiredEffectScopes           []RuntimeEffectScope
	DeclarationRef                 string
	ValidationRecipeID             string
	ValidationProofRequirementHash string
	RequiredCheckRefs              []string
	ValidationCheckAttestations    []RuntimeValidationCheckAttestationBinding
	MetricConstraints              []string
	ArtifactRefs                   []string
	VerificationPolicy             *RuntimeVerificationPolicy `json:",omitempty"`
	Hash                           string
}

// CoverageEvidenceProofTargetPolicy is derived only from the sealed binding
// shape. It is not provider input and it cannot be inferred from a target name.
type CoverageEvidenceProofTargetPolicy string

const (
	CoverageEvidenceProofTargetPolicyExactTarget            CoverageEvidenceProofTargetPolicy = "exact_target"
	CoverageEvidenceProofTargetPolicyExistingOwnerScope     CoverageEvidenceProofTargetPolicy = "existing_owner_scope"
	CoverageEvidenceProofTargetPolicyProspectiveOwnerTarget CoverageEvidenceProofTargetPolicy = "prospective_owner_target"
)

func (i CoverageEvidenceProofIdentity) Normalized() CoverageEvidenceProofIdentity {
	i.DefinitionRevision = strings.TrimSpace(i.DefinitionRevision)
	i.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(i.CoverageWorkItemID)))
	i.EvidenceKind = strings.TrimSpace(i.EvidenceKind)
	i.OwnerModuleID = strings.TrimSpace(i.OwnerModuleID)
	i.AllowedOperationKinds = canonicalizeOperationKinds(i.AllowedOperationKinds)
	i.AllowedOperationVariants = NormalizeRuntimeOperationVariantRefs(i.AllowedOperationVariants)
	i.TargetPolicy = CoverageEvidenceProofTargetPolicy(strings.TrimSpace(string(i.TargetPolicy)))
	i.TargetRefs = normalizeStrings(i.TargetRefs)
	i.TargetSelectorRef = strings.TrimSpace(i.TargetSelectorRef)
	i.TargetSelectorOwnerRef = strings.TrimSpace(i.TargetSelectorOwnerRef)
	i.TargetSelectorSubjectRef = strings.TrimSpace(i.TargetSelectorSubjectRef)
	i.ProspectiveTargetRefs = normalizeStrings(i.ProspectiveTargetRefs)
	i.RequiredEffectScopes = NormalizeRuntimeEffectScopes(i.RequiredEffectScopes)
	i.DeclarationRef = strings.TrimSpace(i.DeclarationRef)
	i.ValidationRecipeID = strings.TrimSpace(i.ValidationRecipeID)
	i.ValidationProofRequirementHash = strings.TrimSpace(i.ValidationProofRequirementHash)
	i.RequiredCheckRefs = normalizeStrings(i.RequiredCheckRefs)
	i.ValidationCheckAttestations = NormalizeRuntimeValidationCheckAttestationBindings(i.ValidationCheckAttestations)
	i.MetricConstraints = normalizeStrings(i.MetricConstraints)
	i.ArtifactRefs = normalizeStrings(i.ArtifactRefs)
	if i.VerificationPolicy != nil {
		value := i.VerificationPolicy.Normalized()
		i.VerificationPolicy = &value
	}
	i.Hash = strings.TrimSpace(i.Hash)
	return i
}

func (i CoverageEvidenceProofIdentity) Clone() CoverageEvidenceProofIdentity {
	i = i.Normalized()
	i.AllowedOperationKinds = append([]string(nil), i.AllowedOperationKinds...)
	i.AllowedOperationVariants = append([]RuntimeOperationVariantRef(nil), i.AllowedOperationVariants...)
	i.TargetRefs = append([]string(nil), i.TargetRefs...)
	i.ProspectiveTargetRefs = append([]string(nil), i.ProspectiveTargetRefs...)
	i.RequiredEffectScopes = append([]RuntimeEffectScope(nil), i.RequiredEffectScopes...)
	i.RequiredCheckRefs = append([]string(nil), i.RequiredCheckRefs...)
	i.ValidationCheckAttestations = CloneRuntimeValidationCheckAttestationBindings(i.ValidationCheckAttestations)
	i.MetricConstraints = append([]string(nil), i.MetricConstraints...)
	i.ArtifactRefs = append([]string(nil), i.ArtifactRefs...)
	i.VerificationPolicy = cloneRuntimeVerificationPolicy(i.VerificationPolicy)
	return i
}

func (i CoverageEvidenceProofIdentity) Validate() error {
	if duplicate := duplicateRuntimeValidationProofCheckRef(i.RequiredCheckRefs); duplicate != "" {
		return fmt.Errorf("coverage evidence proof identity duplicates check %q", duplicate)
	}
	i = i.Normalized()
	if i.DefinitionRevision == "" || i.CoverageWorkItemID == "" || i.OwnerModuleID == "" || i.DeclarationRef == "" {
		return fmt.Errorf("coverage evidence proof identity requires definition, coverage item, owner and declaration")
	}
	if i.EvidenceKind != EvidenceKindStateChange && i.EvidenceKind != EvidenceKindValidation {
		return fmt.Errorf("coverage evidence proof identity has unsupported evidence kind %q", i.EvidenceKind)
	}
	if len(i.AllowedOperationKinds) == 0 || len(i.RequiredEffectScopes) == 0 {
		return fmt.Errorf("coverage evidence proof identity requires operation and effect shape")
	}
	for _, kind := range i.AllowedOperationKinds {
		descriptor, found := RuntimeOperationDescriptorForKind(kind)
		if !found || descriptor.OwnerModuleID != i.OwnerModuleID {
			return fmt.Errorf("coverage evidence proof identity operation %q is not owned by %q", kind, i.OwnerModuleID)
		}
	}
	for _, variant := range i.AllowedOperationVariants {
		if !runtimeOperationKindsContain(i.AllowedOperationKinds, variant.Kind) {
			return fmt.Errorf("coverage evidence proof identity variant %q is outside its operation kinds", variant.VariantID)
		}
	}
	switch i.TargetPolicy {
	case CoverageEvidenceProofTargetPolicyExactTarget:
		if len(i.TargetRefs) == 0 || i.TargetSelectorRef != "" || len(i.ProspectiveTargetRefs) != 0 {
			return fmt.Errorf("exact-target proof identity requires only exact target refs")
		}
	case CoverageEvidenceProofTargetPolicyExistingOwnerScope:
		if (len(i.TargetRefs) == 0 && i.TargetSelectorRef == "") || len(i.ProspectiveTargetRefs) != 0 {
			return fmt.Errorf("existing-owner-scope proof identity requires targets or selector without prospective target")
		}
		if i.TargetSelectorRef != "" && (i.TargetSelectorOwnerRef == "" || i.TargetSelectorSubjectRef == "") {
			return fmt.Errorf("existing-owner-scope proof identity selector requires owner and subject")
		}
	case CoverageEvidenceProofTargetPolicyProspectiveOwnerTarget:
		if len(i.TargetRefs) != 0 || i.TargetSelectorRef == "" || i.TargetSelectorOwnerRef == "" || i.TargetSelectorSubjectRef == "" || len(i.ProspectiveTargetRefs) == 0 {
			return fmt.Errorf("prospective-owner-target proof identity requires selector owner, subject and prospective target")
		}
	default:
		return fmt.Errorf("coverage evidence proof identity has unsupported target policy %q", i.TargetPolicy)
	}
	if i.EvidenceKind == EvidenceKindStateChange && (i.ValidationRecipeID != "" || i.ValidationProofRequirementHash != "" || len(i.RequiredCheckRefs) != 0 || len(i.ValidationCheckAttestations) != 0) {
		return fmt.Errorf("state-change proof identity cannot carry validation recipe/checks")
	}
	if i.EvidenceKind == EvidenceKindValidation && i.ValidationRecipeID == "" {
		return fmt.Errorf("validation proof identity requires validation recipe")
	}
	if i.EvidenceKind == EvidenceKindValidation {
		managed := runtimeOperationKindsContain(i.AllowedOperationKinds, RuntimeBlockKindManagedValidationSession)
		if managed && (i.ValidationProofRequirementHash == "" || len(i.RequiredCheckRefs) == 0 || len(i.ValidationCheckAttestations) == 0) {
			return fmt.Errorf("managed validation proof identity requires an exact CWI proof requirement and native check attestations")
		}
		if managed {
			if err := ValidateRuntimeValidationCheckAttestationBindings(i.ValidationCheckAttestations, i.RequiredCheckRefs); err != nil {
				return err
			}
		}
		if !managed && (i.ValidationProofRequirementHash != "" || len(i.ValidationCheckAttestations) != 0) {
			return fmt.Errorf("non-managed validation proof identity cannot carry a Shell CWI proof requirement")
		}
	}
	if i.Hash == "" || i.Hash != StableCoverageEvidenceProofIdentityHash(i) {
		return fmt.Errorf("coverage evidence proof identity hash does not match contents")
	}
	return nil
}

// DeriveCoverageEvidenceProofIdentity derives the owner-neutral proof identity
// from one exact sealed binding proof. A proof that spans operation owners is
// rejected rather than resolved through a global owner lookup.
func DeriveCoverageEvidenceProofIdentity(binding CoverageEvidenceBinding, evidenceKind string) (CoverageEvidenceProofIdentity, error) {
	binding = binding.Normalized()
	if err := binding.Validate(); err != nil {
		return CoverageEvidenceProofIdentity{}, err
	}
	evidenceKind = strings.TrimSpace(evidenceKind)
	proof, found := CoverageEvidenceProofForKind(binding, evidenceKind)
	if !found {
		return CoverageEvidenceProofIdentity{}, fmt.Errorf("coverage binding %q has no proof for evidence kind %q", binding.ID, evidenceKind)
	}
	owner := ""
	for _, kind := range proof.AllowedOperationKinds {
		descriptor, found := RuntimeOperationDescriptorForKind(kind)
		if !found || strings.TrimSpace(descriptor.OwnerModuleID) == "" {
			return CoverageEvidenceProofIdentity{}, fmt.Errorf("coverage binding %q proof operation %q has no owner descriptor", binding.ID, kind)
		}
		if owner != "" && owner != descriptor.OwnerModuleID {
			return CoverageEvidenceProofIdentity{}, fmt.Errorf("coverage binding %q proof spans multiple operation owners", binding.ID)
		}
		owner = descriptor.OwnerModuleID
	}
	policy, err := coverageEvidenceProofTargetPolicyForBinding(binding)
	if err != nil {
		return CoverageEvidenceProofIdentity{}, err
	}
	identity := CoverageEvidenceProofIdentity{
		DefinitionRevision:             binding.DefinitionRevision,
		CoverageWorkItemID:             binding.CoverageWorkItemID,
		EvidenceKind:                   proof.EvidenceKind,
		OwnerModuleID:                  owner,
		AllowedOperationKinds:          append([]string(nil), proof.AllowedOperationKinds...),
		AllowedOperationVariants:       append([]RuntimeOperationVariantRef(nil), proof.AllowedOperationVariants...),
		TargetPolicy:                   policy,
		TargetRefs:                     append([]string(nil), binding.TargetRefs...),
		TargetSelectorRef:              binding.TargetSelectorRef,
		TargetSelectorOwnerRef:         binding.TargetSelectorOwnerRef,
		TargetSelectorSubjectRef:       binding.TargetSelectorSubjectRef,
		ProspectiveTargetRefs:          binding.ProspectiveTargetRefs,
		RequiredEffectScopes:           append([]RuntimeEffectScope(nil), proof.RequiredEffectScopes...),
		DeclarationRef:                 binding.DeclarationRef,
		ValidationRecipeID:             proof.ValidationRecipeID,
		ValidationProofRequirementHash: proof.ValidationProofRequirementHash,
		RequiredCheckRefs:              append([]string(nil), proof.RequiredCheckRefs...),
		ValidationCheckAttestations:    CloneRuntimeValidationCheckAttestationBindings(proof.ValidationCheckAttestations),
		MetricConstraints:              append([]string(nil), proof.MetricConstraints...),
		ArtifactRefs:                   append([]string(nil), proof.ArtifactRefs...),
		VerificationPolicy:             cloneRuntimeVerificationPolicy(binding.VerificationPolicy),
	}.Normalized()
	identity.Hash = StableCoverageEvidenceProofIdentityHash(identity)
	if err := identity.Validate(); err != nil {
		return CoverageEvidenceProofIdentity{}, err
	}
	return identity, nil
}

func coverageEvidenceProofTargetPolicyForBinding(binding CoverageEvidenceBinding) (CoverageEvidenceProofTargetPolicy, error) {
	binding = binding.Normalized()
	if len(binding.ProspectiveTargetRefs) != 0 {
		if len(binding.TargetRefs) != 0 || binding.TargetSelectorRef == "" {
			return "", fmt.Errorf("coverage binding %q has ambiguous prospective target policy", binding.ID)
		}
		return CoverageEvidenceProofTargetPolicyProspectiveOwnerTarget, nil
	}
	if binding.TargetSelectorRef != "" {
		return CoverageEvidenceProofTargetPolicyExistingOwnerScope, nil
	}
	if len(binding.TargetRefs) != 0 {
		return CoverageEvidenceProofTargetPolicyExactTarget, nil
	}
	return "", fmt.Errorf("coverage binding %q has no target policy", binding.ID)
}

// StableCoverageEvidenceProofIdentityHash returns the stable semantic identity
// of a proof. Binding ref/hash, semantic batch/manifest/request and observed
// resource revisions are intentionally absent.
func StableCoverageEvidenceProofIdentityHash(identity CoverageEvidenceProofIdentity) string {
	identity = identity.Normalized()
	identity.Hash = ""
	h := sha256.New()
	writeHashString(h, identity.DefinitionRevision)
	writeHashString(h, string(identity.CoverageWorkItemID))
	writeHashString(h, identity.EvidenceKind)
	writeHashString(h, identity.OwnerModuleID)
	writeHashStrings(h, identity.AllowedOperationKinds)
	for _, variant := range identity.AllowedOperationVariants {
		writeHashString(h, variant.Kind)
		writeHashString(h, variant.VariantID)
	}
	writeHashString(h, string(identity.TargetPolicy))
	writeHashStrings(h, identity.TargetRefs)
	writeHashString(h, identity.TargetSelectorRef)
	writeHashString(h, identity.TargetSelectorOwnerRef)
	writeHashString(h, identity.TargetSelectorSubjectRef)
	writeHashStrings(h, identity.ProspectiveTargetRefs)
	writeHashStrings(h, RuntimeEffectScopesAsStrings(identity.RequiredEffectScopes))
	writeHashString(h, identity.DeclarationRef)
	writeHashString(h, identity.ValidationRecipeID)
	writeHashString(h, identity.ValidationProofRequirementHash)
	writeHashStrings(h, identity.RequiredCheckRefs)
	for _, attestation := range identity.ValidationCheckAttestations {
		writeHashString(h, attestation.RequiredCheckRef)
		writeHashString(h, attestation.ObservationProtocolRef)
		writeHashString(h, attestation.NativeCheckRef)
		writeRuntimeSupplementalNativeCheckHash(h, attestation)
		writeRuntimeValidationExecutionRequirementHash(h, attestation)
	}
	writeHashStrings(h, identity.MetricConstraints)
	writeHashStrings(h, identity.ArtifactRefs)
	if identity.VerificationPolicy != nil {
		writeHashString(h, "verification-policy")
		writeHashString(h, StableRuntimeVerificationPolicyHash(*identity.VerificationPolicy))
	}
	return "coverage-evidence-proof-identity-" + hex.EncodeToString(h.Sum(nil))
}

// SameCoverageEvidenceProofIdentity compares every semantic proof obligation,
// excluding only the derivative Hash field.
func SameCoverageEvidenceProofIdentity(left, right CoverageEvidenceProofIdentity) bool {
	return FirstCoverageEvidenceProofIdentityDifference(left, right) == ""
}

// FirstCoverageEvidenceProofIdentityDifference returns the first stable field
// category that changed. The category is audit data for SM-2 lineage gaps; it
// is not a repair instruction or provider diagnostic.
func FirstCoverageEvidenceProofIdentityDifference(left, right CoverageEvidenceProofIdentity) string {
	left = left.Normalized()
	right = right.Normalized()
	switch {
	case left.DefinitionRevision != right.DefinitionRevision:
		return "definition_revision"
	case left.CoverageWorkItemID != right.CoverageWorkItemID:
		return "coverage_work_item"
	case left.EvidenceKind != right.EvidenceKind:
		return "evidence_kind"
	case left.OwnerModuleID != right.OwnerModuleID:
		return "owner_module"
	case !sameStrings(left.AllowedOperationKinds, right.AllowedOperationKinds):
		return "allowed_operation_kinds"
	case !SameRuntimeOperationVariantRefs(left.AllowedOperationVariants, right.AllowedOperationVariants):
		return "allowed_operation_variants"
	case left.TargetPolicy != right.TargetPolicy:
		return "target_policy"
	case !sameStrings(left.TargetRefs, right.TargetRefs) || left.TargetSelectorRef != right.TargetSelectorRef ||
		left.TargetSelectorOwnerRef != right.TargetSelectorOwnerRef || left.TargetSelectorSubjectRef != right.TargetSelectorSubjectRef ||
		!sameStrings(left.ProspectiveTargetRefs, right.ProspectiveTargetRefs):
		return "target"
	case !sameStrings(RuntimeEffectScopesAsStrings(left.RequiredEffectScopes), RuntimeEffectScopesAsStrings(right.RequiredEffectScopes)):
		return "effect"
	case left.DeclarationRef != right.DeclarationRef:
		return "declaration"
	case left.ValidationRecipeID != right.ValidationRecipeID:
		return "validation_recipe"
	case left.ValidationProofRequirementHash != right.ValidationProofRequirementHash:
		return "validation_proof_requirement"
	case !sameStrings(left.RequiredCheckRefs, right.RequiredCheckRefs):
		return "required_checks"
	case !SameRuntimeValidationCheckAttestationBindings(left.ValidationCheckAttestations, right.ValidationCheckAttestations):
		return "validation_check_attestations"
	case !sameStrings(left.MetricConstraints, right.MetricConstraints):
		return "metrics"
	case !sameStrings(left.ArtifactRefs, right.ArtifactRefs):
		return "artifacts"
	case !SameRuntimeVerificationPolicy(left.VerificationPolicy, right.VerificationPolicy):
		return "verification_policy"
	default:
		return ""
	}
}

// RuntimeCoverageProofLineageLimit bounds the aggregate-local source and
// resolution records. Overflow is a contract error; records are never pruned.
const RuntimeCoverageProofLineageLimit = 512

type RuntimeCoverageProofLineageKind string

const (
	RuntimeCoverageProofLineageKindSource     RuntimeCoverageProofLineageKind = "source"
	RuntimeCoverageProofLineageKindResolution RuntimeCoverageProofLineageKind = "resolution"
)

type RuntimeCoverageProofLineageDisposition string

const (
	RuntimeCoverageProofLineageDispositionEquivalent   RuntimeCoverageProofLineageDisposition = "equivalent"
	RuntimeCoverageProofLineageDispositionIncompatible RuntimeCoverageProofLineageDisposition = "incompatible"
)

const RuntimeCoverageProofLineageIncompatibleGapCode = "binding_lineage_incompatible"

// RuntimeCoverageProofLineageGap is a bounded audit fact for an incompatible
// re-sealed proof. It deliberately does not contain a retry, operation, state
// or terminal decision.
type RuntimeCoverageProofLineageGap struct {
	Code                        string
	CoverageWorkItemID          common.CoverageWorkItemID
	EvidenceKind                string
	SourceProofIdentityHash     string
	CurrentProofIdentityHash    string
	FirstDifferingFieldCategory string
}

func (g RuntimeCoverageProofLineageGap) Normalized() RuntimeCoverageProofLineageGap {
	g.Code = strings.TrimSpace(g.Code)
	g.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(g.CoverageWorkItemID)))
	g.EvidenceKind = strings.TrimSpace(g.EvidenceKind)
	g.SourceProofIdentityHash = strings.TrimSpace(g.SourceProofIdentityHash)
	g.CurrentProofIdentityHash = strings.TrimSpace(g.CurrentProofIdentityHash)
	g.FirstDifferingFieldCategory = strings.TrimSpace(g.FirstDifferingFieldCategory)
	return g
}

func (g RuntimeCoverageProofLineageGap) Clone() RuntimeCoverageProofLineageGap {
	return g.Normalized()
}

func (g RuntimeCoverageProofLineageGap) Validate() error {
	g = g.Normalized()
	if g.Code != RuntimeCoverageProofLineageIncompatibleGapCode || g.CoverageWorkItemID == "" ||
		(g.EvidenceKind != EvidenceKindStateChange && g.EvidenceKind != EvidenceKindValidation) ||
		g.SourceProofIdentityHash == "" || g.CurrentProofIdentityHash == "" || g.FirstDifferingFieldCategory == "" {
		return fmt.Errorf("coverage proof lineage incompatible gap requires exact proof identity facts")
	}
	if g.SourceProofIdentityHash == g.CurrentProofIdentityHash {
		return fmt.Errorf("coverage proof lineage incompatible gap cannot use equal identities")
	}
	return nil
}

// RuntimeCoverageProofLineage is one aggregate-local SM-2 source or current
// resolution row. Source rows record only accepted exact receipt evidence;
// resolution rows are idempotent by source row plus current binding ref/hash.
// It is not an EventHub contract, state machine or global index.
type RuntimeCoverageProofLineage struct {
	ID          string
	Kind        RuntimeCoverageProofLineageKind
	WorkspaceID common.WorkspaceID
	RunID       common.RunID

	CoverageWorkItemID common.CoverageWorkItemID
	EvidenceKind       string

	// Source-only immutable audit fields.
	SourceEvidenceID       common.EvidenceRecordID
	SourceDispatchID       string
	SourceOperationID      string
	SourceExecutionKey     string
	SourceFence            uint64
	SourceReceiptHash      string
	SourceBindingRef       string
	SourceBindingHash      string
	SourceManifestHash     string
	SourceLogicalRequestID string
	SourceProofIdentity    CoverageEvidenceProofIdentity

	// Resolution-only fields.
	SourceLineageID      string
	CurrentBindingRef    string
	CurrentBindingHash   string
	CurrentProofIdentity CoverageEvidenceProofIdentity
	Disposition          RuntimeCoverageProofLineageDisposition
	Gap                  *RuntimeCoverageProofLineageGap
}

func (l RuntimeCoverageProofLineage) Normalized() RuntimeCoverageProofLineage {
	l.ID = strings.TrimSpace(l.ID)
	l.Kind = RuntimeCoverageProofLineageKind(strings.TrimSpace(string(l.Kind)))
	l.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(l.WorkspaceID)))
	l.RunID = common.RunID(strings.TrimSpace(string(l.RunID)))
	l.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(l.CoverageWorkItemID)))
	l.EvidenceKind = strings.TrimSpace(l.EvidenceKind)
	l.SourceEvidenceID = common.EvidenceRecordID(strings.TrimSpace(string(l.SourceEvidenceID)))
	l.SourceDispatchID = strings.TrimSpace(l.SourceDispatchID)
	l.SourceOperationID = strings.TrimSpace(l.SourceOperationID)
	l.SourceExecutionKey = strings.TrimSpace(l.SourceExecutionKey)
	l.SourceReceiptHash = strings.TrimSpace(l.SourceReceiptHash)
	l.SourceBindingRef = strings.TrimSpace(l.SourceBindingRef)
	l.SourceBindingHash = strings.TrimSpace(l.SourceBindingHash)
	l.SourceManifestHash = strings.TrimSpace(l.SourceManifestHash)
	l.SourceLogicalRequestID = strings.TrimSpace(l.SourceLogicalRequestID)
	l.SourceProofIdentity = l.SourceProofIdentity.Clone()
	l.SourceLineageID = strings.TrimSpace(l.SourceLineageID)
	l.CurrentBindingRef = strings.TrimSpace(l.CurrentBindingRef)
	l.CurrentBindingHash = strings.TrimSpace(l.CurrentBindingHash)
	l.CurrentProofIdentity = l.CurrentProofIdentity.Clone()
	l.Disposition = RuntimeCoverageProofLineageDisposition(strings.TrimSpace(string(l.Disposition)))
	if l.Gap != nil {
		gap := l.Gap.Clone()
		l.Gap = &gap
	}
	return l
}

func (l RuntimeCoverageProofLineage) Clone() RuntimeCoverageProofLineage {
	return l.Normalized()
}

func (l RuntimeCoverageProofLineage) Validate() error {
	// Validate nested proof identities before Normalized/Clone can sort and
	// deduplicate their slices. Persisted lineage is opaque owner authority;
	// recovery must reject duplicate/aliased checks instead of repairing them.
	if l.SourceProofIdentity.Hash != "" {
		if err := l.SourceProofIdentity.Validate(); err != nil {
			return fmt.Errorf("coverage proof lineage source proof identity: %w", err)
		}
	}
	if l.CurrentProofIdentity.Hash != "" {
		if err := l.CurrentProofIdentity.Validate(); err != nil {
			return fmt.Errorf("coverage proof lineage current proof identity: %w", err)
		}
	}
	l = l.Normalized()
	if l.ID == "" || l.WorkspaceID == "" || l.RunID == "" || l.CoverageWorkItemID == "" ||
		(l.EvidenceKind != EvidenceKindStateChange && l.EvidenceKind != EvidenceKindValidation) {
		return fmt.Errorf("coverage proof lineage requires scope, identity, coverage item and evidence kind")
	}
	switch l.Kind {
	case RuntimeCoverageProofLineageKindSource:
		if l.SourceLineageID != "" || l.CurrentBindingRef != "" || l.CurrentBindingHash != "" ||
			l.CurrentProofIdentity.Hash != "" || l.Disposition != "" || l.Gap != nil {
			return fmt.Errorf("coverage proof lineage source row contains resolution data")
		}
		if l.SourceEvidenceID == "" || l.SourceDispatchID == "" || l.SourceOperationID == "" ||
			l.SourceExecutionKey == "" || l.SourceFence == 0 || l.SourceReceiptHash == "" ||
			l.SourceBindingRef == "" || l.SourceBindingHash == "" || l.SourceManifestHash == "" || l.SourceLogicalRequestID == "" {
			return fmt.Errorf("coverage proof lineage source row requires receipt, evidence, binding and semantic audit identity")
		}
		if err := l.SourceProofIdentity.Validate(); err != nil {
			return fmt.Errorf("coverage proof lineage source proof identity: %w", err)
		}
		if l.SourceProofIdentity.CoverageWorkItemID != l.CoverageWorkItemID || l.SourceProofIdentity.EvidenceKind != l.EvidenceKind {
			return fmt.Errorf("coverage proof lineage source identity does not match row coverage proof")
		}
		if l.ID != StableRuntimeCoverageProofLineageSourceID(l.SourceReceiptHash, l.SourceEvidenceID, l.SourceBindingHash, l.SourceProofIdentity.Hash) {
			return fmt.Errorf("coverage proof lineage source id does not match immutable receipt identity")
		}
	case RuntimeCoverageProofLineageKindResolution:
		if l.SourceEvidenceID != "" || l.SourceDispatchID != "" || l.SourceOperationID != "" ||
			l.SourceExecutionKey != "" || l.SourceFence != 0 || l.SourceReceiptHash != "" ||
			l.SourceBindingRef != "" || l.SourceBindingHash != "" || l.SourceManifestHash != "" ||
			l.SourceLogicalRequestID != "" || l.SourceProofIdentity.Hash != "" {
			return fmt.Errorf("coverage proof lineage resolution row contains source data")
		}
		if l.SourceLineageID == "" || l.CurrentBindingRef == "" || l.CurrentBindingHash == "" {
			return fmt.Errorf("coverage proof lineage resolution row requires source and current binding identity")
		}
		if err := l.CurrentProofIdentity.Validate(); err != nil {
			return fmt.Errorf("coverage proof lineage current proof identity: %w", err)
		}
		if l.CurrentProofIdentity.CoverageWorkItemID != l.CoverageWorkItemID || l.CurrentProofIdentity.EvidenceKind != l.EvidenceKind {
			return fmt.Errorf("coverage proof lineage resolution identity does not match row coverage proof")
		}
		if l.ID != StableRuntimeCoverageProofLineageResolutionID(l.SourceLineageID, l.CurrentBindingRef, l.CurrentBindingHash) {
			return fmt.Errorf("coverage proof lineage resolution id does not match source/current binding identity")
		}
		switch l.Disposition {
		case RuntimeCoverageProofLineageDispositionEquivalent:
			if l.Gap != nil {
				return fmt.Errorf("equivalent coverage proof lineage resolution cannot carry a gap")
			}
		case RuntimeCoverageProofLineageDispositionIncompatible:
			if l.Gap == nil {
				return fmt.Errorf("incompatible coverage proof lineage resolution requires a gap")
			}
			if err := l.Gap.Validate(); err != nil {
				return err
			}
			if l.Gap.CoverageWorkItemID != l.CoverageWorkItemID || l.Gap.EvidenceKind != l.EvidenceKind ||
				l.Gap.CurrentProofIdentityHash != l.CurrentProofIdentity.Hash {
				return fmt.Errorf("coverage proof lineage gap does not match current resolution")
			}
		default:
			return fmt.Errorf("coverage proof lineage resolution has unsupported disposition %q", l.Disposition)
		}
	default:
		return fmt.Errorf("coverage proof lineage has unsupported row kind %q", l.Kind)
	}
	return nil
}

func StableRuntimeCoverageProofLineageSourceID(receiptHash string, evidenceID common.EvidenceRecordID, bindingHash, proofIdentityHash string) string {
	h := sha256.New()
	writeHashString(h, strings.TrimSpace(receiptHash))
	writeHashString(h, strings.TrimSpace(string(evidenceID)))
	writeHashString(h, strings.TrimSpace(bindingHash))
	writeHashString(h, strings.TrimSpace(proofIdentityHash))
	return "coverage-proof-lineage-source-" + hex.EncodeToString(h.Sum(nil))
}

func StableRuntimeCoverageProofLineageResolutionID(sourceLineageID, currentBindingRef, currentBindingHash string) string {
	h := sha256.New()
	writeHashString(h, strings.TrimSpace(sourceLineageID))
	writeHashString(h, strings.TrimSpace(currentBindingRef))
	writeHashString(h, strings.TrimSpace(currentBindingHash))
	return "coverage-proof-lineage-resolution-" + hex.EncodeToString(h.Sum(nil))
}

func NormalizeRuntimeCoverageProofLineage(values []RuntimeCoverageProofLineage) []RuntimeCoverageProofLineage {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeCoverageProofLineage, len(values))
	for idx := range values {
		ret[idx] = values[idx].Clone()
	}
	sort.SliceStable(ret, func(i, j int) bool {
		leftRank := runtimeCoverageProofLineageKindRank(ret[i].Kind)
		rightRank := runtimeCoverageProofLineageKindRank(ret[j].Kind)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return ret[i].ID < ret[j].ID
	})
	return ret
}

func runtimeCoverageProofLineageKindRank(kind RuntimeCoverageProofLineageKind) int {
	if kind == RuntimeCoverageProofLineageKindSource {
		return 0
	}
	return 1
}

// ValidateRuntimeCoverageProofLineage validates the aggregate-local ledger's
// immutable row identities and source/current relationships. It intentionally
// does not decide whether an equivalent row grants current CWI credit.
func ValidateRuntimeCoverageProofLineage(values []RuntimeCoverageProofLineage) error {
	if len(values) > RuntimeCoverageProofLineageLimit {
		return fmt.Errorf("coverage proof lineage exceeds bounded limit %d", RuntimeCoverageProofLineageLimit)
	}
	// Validate the opaque rows before sorting. In particular this preserves
	// nested proof-check duplicates that their canonical representation cannot
	// carry.
	for _, row := range values {
		if err := row.Validate(); err != nil {
			return err
		}
	}
	normalized := NormalizeRuntimeCoverageProofLineage(values)
	if len(normalized) != len(values) {
		return fmt.Errorf("coverage proof lineage normalization changed record count")
	}
	sources := make(map[string]RuntimeCoverageProofLineage, len(normalized))
	seenIDs := make(map[string]struct{}, len(normalized))
	for _, row := range normalized {
		if err := row.Validate(); err != nil {
			return err
		}
		if _, duplicate := seenIDs[row.ID]; duplicate {
			return fmt.Errorf("coverage proof lineage duplicates row %q", row.ID)
		}
		seenIDs[row.ID] = struct{}{}
		if row.Kind == RuntimeCoverageProofLineageKindSource {
			sources[row.ID] = row
		}
	}
	seenResolutionPairs := make(map[string]struct{}, len(normalized))
	for _, row := range normalized {
		if row.Kind != RuntimeCoverageProofLineageKindResolution {
			continue
		}
		source, found := sources[row.SourceLineageID]
		if !found {
			return fmt.Errorf("coverage proof lineage resolution references missing source %q", row.SourceLineageID)
		}
		if source.WorkspaceID != row.WorkspaceID || source.RunID != row.RunID ||
			source.CoverageWorkItemID != row.CoverageWorkItemID || source.EvidenceKind != row.EvidenceKind {
			return fmt.Errorf("coverage proof lineage resolution scope does not match source")
		}
		pair := row.SourceLineageID + "\x00" + row.CurrentBindingRef + "\x00" + row.CurrentBindingHash
		if _, duplicate := seenResolutionPairs[pair]; duplicate {
			return fmt.Errorf("coverage proof lineage duplicates source/current resolution")
		}
		seenResolutionPairs[pair] = struct{}{}
		switch row.Disposition {
		case RuntimeCoverageProofLineageDispositionEquivalent:
			if !SameCoverageEvidenceProofIdentity(source.SourceProofIdentity, row.CurrentProofIdentity) {
				return fmt.Errorf("equivalent coverage proof lineage resolution changes proof identity")
			}
		case RuntimeCoverageProofLineageDispositionIncompatible:
			if SameCoverageEvidenceProofIdentity(source.SourceProofIdentity, row.CurrentProofIdentity) || row.Gap == nil ||
				row.Gap.SourceProofIdentityHash != source.SourceProofIdentity.Hash ||
				row.Gap.FirstDifferingFieldCategory != FirstCoverageEvidenceProofIdentityDifference(source.SourceProofIdentity, row.CurrentProofIdentity) {
				return fmt.Errorf("incompatible coverage proof lineage resolution does not describe source/current difference")
			}
		}
	}
	return nil
}

package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

// RuntimeCWIReconciliationContractSchemaCurrent identifies the profile-compiled,
// immutable CWI reconciliation declaration.  It is intentionally independent
// from an aggregate schema: callers must explicitly seal it into a new
// RuntimeBootstrap rather than infer it for an older Run.
const RuntimeCWIReconciliationContractSchemaCurrent = "workorch.runtime_cwi_reconciliation"

// RuntimeCWIReconciliationScope describes the only two reconciliation
// boundaries.  It is a factual request scope, never a lifecycle target.
type RuntimeCWIReconciliationScope string

const (
	RuntimeCWIReconciliationScopeCurrentRun    RuntimeCWIReconciliationScope = "current_run"
	RuntimeCWIReconciliationScopeWorkspaceFull RuntimeCWIReconciliationScope = "workspace_full"
)

// RuntimeCWIReconciliationObservationStatus is an owner-authored observation
// category.  In particular, satisfied does not grant new CWI credit: only the
// SM-2 reducer may retain an already recorded credit after consuming it.
type RuntimeCWIReconciliationObservationStatus string

const (
	RuntimeCWIReconciliationObservationSatisfied    RuntimeCWIReconciliationObservationStatus = "satisfied"
	RuntimeCWIReconciliationObservationUnsatisfied  RuntimeCWIReconciliationObservationStatus = "unsatisfied"
	RuntimeCWIReconciliationObservationUnverifiable RuntimeCWIReconciliationObservationStatus = "unverifiable"
	RuntimeCWIReconciliationObservationStale        RuntimeCWIReconciliationObservationStatus = "stale"
)

// RuntimeCWIReconciliationDisposition is an Orchestrator-private SM-2 result
// vocabulary.  It intentionally records no target state and is not an
// EventHub command for another owner.
type RuntimeCWIReconciliationDisposition string

const (
	RuntimeCWIReconciliationDispositionRetain       RuntimeCWIReconciliationDisposition = "retain"
	RuntimeCWIReconciliationDispositionReopen       RuntimeCWIReconciliationDisposition = "reopen"
	RuntimeCWIReconciliationDispositionPrerequisite RuntimeCWIReconciliationDisposition = "prerequisite"
	RuntimeCWIReconciliationDispositionTerminal     RuntimeCWIReconciliationDisposition = "terminal"
)

// RuntimeCWIReconciliationBindingRef is the bounded, profile-compiled
// reconciliation identity for exactly one sealed CoverageEvidenceBinding.  It
// exposes owner identities and proof kinds only so a capability owner can
// validate its own observation; it does not expose Goal/DoD text or a desired
// coverage outcome.
type RuntimeCWIReconciliationBindingRef struct {
	CoverageWorkItemID    common.CoverageWorkItemID
	BindingID             string
	BindingHash           string
	OwnerModuleIDs        []string
	RequiredEvidenceKinds []string
	// The following fields are the sealed, owner-verifiable proof surface. They
	// are copied from CoverageEvidenceBinding at activation; an owner may use
	// them to inspect its own resource, but it must not infer semantics from
	// Goal/DoD text or from a candidate.
	AllowedOperationKinds                []string
	TargetRefs                           []string
	TargetSelectorRef                    string
	TargetSelectorOwnerRef               string
	TargetSelectorSubjectRef             string
	TargetSelectorCandidateRefs          []string
	ProspectiveTargetAllOfConstraintRefs []string
	ProspectiveTargetRefs                []string
	ValidationRecipeIDs                  []string
	RequiredCheckRefs                    []string
	ValidationCheckAttestations          []RuntimeValidationCheckAttestationBinding
	MetricConstraints                    []string
	ArtifactRefs                         []string
}

func (r RuntimeCWIReconciliationBindingRef) Normalized() RuntimeCWIReconciliationBindingRef {
	r.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(r.CoverageWorkItemID)))
	r.BindingID = strings.TrimSpace(r.BindingID)
	r.BindingHash = strings.TrimSpace(r.BindingHash)
	r.OwnerModuleIDs = normalizeStrings(r.OwnerModuleIDs)
	r.RequiredEvidenceKinds = normalizeStrings(r.RequiredEvidenceKinds)
	r.AllowedOperationKinds = canonicalizeOperationKinds(r.AllowedOperationKinds)
	r.TargetRefs = normalizeStrings(r.TargetRefs)
	r.TargetSelectorRef = strings.TrimSpace(r.TargetSelectorRef)
	r.TargetSelectorOwnerRef = strings.TrimSpace(r.TargetSelectorOwnerRef)
	r.TargetSelectorSubjectRef = strings.TrimSpace(r.TargetSelectorSubjectRef)
	r.TargetSelectorCandidateRefs = normalizeStrings(r.TargetSelectorCandidateRefs)
	r.ProspectiveTargetAllOfConstraintRefs = NormalizeRuntimeProspectiveTargetAllOfConstraintRefs(r.ProspectiveTargetAllOfConstraintRefs)
	r.ProspectiveTargetRefs = normalizeStrings(r.ProspectiveTargetRefs)
	r.ValidationRecipeIDs = normalizeStrings(r.ValidationRecipeIDs)
	r.RequiredCheckRefs = normalizeStrings(r.RequiredCheckRefs)
	r.ValidationCheckAttestations = NormalizeRuntimeValidationCheckAttestationBindings(r.ValidationCheckAttestations)
	r.MetricConstraints = normalizeStrings(r.MetricConstraints)
	r.ArtifactRefs = normalizeStrings(r.ArtifactRefs)
	return r
}

func (r RuntimeCWIReconciliationBindingRef) Clone() RuntimeCWIReconciliationBindingRef {
	r = r.Normalized()
	r.OwnerModuleIDs = append([]string(nil), r.OwnerModuleIDs...)
	r.RequiredEvidenceKinds = append([]string(nil), r.RequiredEvidenceKinds...)
	r.AllowedOperationKinds = append([]string(nil), r.AllowedOperationKinds...)
	r.TargetRefs = append([]string(nil), r.TargetRefs...)
	r.TargetSelectorCandidateRefs = append([]string(nil), r.TargetSelectorCandidateRefs...)
	r.ProspectiveTargetAllOfConstraintRefs = append([]string(nil), r.ProspectiveTargetAllOfConstraintRefs...)
	r.ProspectiveTargetRefs = append([]string(nil), r.ProspectiveTargetRefs...)
	r.ValidationRecipeIDs = append([]string(nil), r.ValidationRecipeIDs...)
	r.RequiredCheckRefs = append([]string(nil), r.RequiredCheckRefs...)
	r.ValidationCheckAttestations = CloneRuntimeValidationCheckAttestationBindings(r.ValidationCheckAttestations)
	r.MetricConstraints = append([]string(nil), r.MetricConstraints...)
	r.ArtifactRefs = append([]string(nil), r.ArtifactRefs...)
	return r
}

func (r RuntimeCWIReconciliationBindingRef) Validate() error {
	if duplicate := duplicateRuntimeValidationProofCheckRef(r.RequiredCheckRefs); duplicate != "" {
		return fmt.Errorf("reconciliation binding ref %q duplicates check %q", strings.TrimSpace(r.BindingID), duplicate)
	}
	if len(r.ValidationCheckAttestations) != 0 {
		if err := ValidateRuntimeValidationCheckAttestationBindings(r.ValidationCheckAttestations, r.RequiredCheckRefs); err != nil {
			return fmt.Errorf("reconciliation binding ref %q: %w", strings.TrimSpace(r.BindingID), err)
		}
	}
	r = r.Normalized()
	if r.CoverageWorkItemID == "" || r.BindingID == "" || r.BindingHash == "" {
		return fmt.Errorf("reconciliation binding ref requires coverage item and binding identity")
	}
	if len(r.OwnerModuleIDs) == 0 || len(r.RequiredEvidenceKinds) == 0 {
		return fmt.Errorf("reconciliation binding ref %q requires owners and evidence kinds", r.BindingID)
	}
	for _, kind := range r.RequiredEvidenceKinds {
		if kind != EvidenceKindStateChange && kind != EvidenceKindValidation {
			return fmt.Errorf("reconciliation binding ref %q has unsupported evidence kind %q", r.BindingID, kind)
		}
	}
	if len(r.TargetSelectorCandidateRefs) != 0 && r.TargetSelectorRef == "" {
		return fmt.Errorf("reconciliation binding ref %q selector candidates require a selector", r.BindingID)
	}
	if len(r.ProspectiveTargetAllOfConstraintRefs) != 0 && (r.TargetSelectorRef == "" || r.TargetSelectorOwnerRef == "" || r.TargetSelectorSubjectRef == "") {
		return fmt.Errorf("reconciliation binding ref %q prospective target all-of declaration requires a complete selector", r.BindingID)
	}
	if err := ValidateRuntimeProspectiveTargetAllOfConstraintRefs(r.ProspectiveTargetAllOfConstraintRefs); err != nil {
		return fmt.Errorf("reconciliation binding ref %q: %w", r.BindingID, err)
	}
	managed := runtimeOperationKindsContain(r.AllowedOperationKinds, RuntimeBlockKindManagedValidationSession)
	if managed && (len(r.RequiredCheckRefs) == 0 || len(r.ValidationCheckAttestations) == 0) {
		return fmt.Errorf("reconciliation binding ref %q managed validation requires native check attestations", r.BindingID)
	}
	if !managed && len(r.ValidationCheckAttestations) != 0 {
		return fmt.Errorf("reconciliation binding ref %q non-managed proof cannot carry Shell native check attestations", r.BindingID)
	}
	return nil
}

// RuntimeCWIReconciliationContract is sealed during activation from the exact
// per-CWI proof bindings.  pkg only validates its immutable identity; it does
// not dispatch observations, retain coverage, or select a repair.
type RuntimeCWIReconciliationContract struct {
	SchemaVersion      string
	DefinitionRevision string
	BindingSetHash     string
	Bindings           []RuntimeCWIReconciliationBindingRef
}

func (c RuntimeCWIReconciliationContract) Normalized() RuntimeCWIReconciliationContract {
	c.SchemaVersion = strings.TrimSpace(c.SchemaVersion)
	c.DefinitionRevision = strings.TrimSpace(c.DefinitionRevision)
	c.BindingSetHash = strings.TrimSpace(c.BindingSetHash)
	c.Bindings = cloneRuntimeCWIReconciliationBindingRefs(c.Bindings)
	sort.Slice(c.Bindings, func(i, j int) bool {
		if c.Bindings[i].CoverageWorkItemID != c.Bindings[j].CoverageWorkItemID {
			return c.Bindings[i].CoverageWorkItemID < c.Bindings[j].CoverageWorkItemID
		}
		return c.Bindings[i].BindingID < c.Bindings[j].BindingID
	})
	return c
}

func (c RuntimeCWIReconciliationContract) Clone() RuntimeCWIReconciliationContract {
	return c.Normalized()
}

func (c RuntimeCWIReconciliationContract) Validate() error {
	c = c.Normalized()
	if c.SchemaVersion != RuntimeCWIReconciliationContractSchemaCurrent || c.DefinitionRevision == "" || len(c.Bindings) == 0 {
		return fmt.Errorf("reconciliation contract requires schema, definition revision and bindings")
	}
	seenCoverage := make(map[common.CoverageWorkItemID]struct{}, len(c.Bindings))
	seenBinding := make(map[string]struct{}, len(c.Bindings))
	for _, binding := range c.Bindings {
		if err := binding.Validate(); err != nil {
			return err
		}
		if _, duplicate := seenCoverage[binding.CoverageWorkItemID]; duplicate {
			return fmt.Errorf("reconciliation contract duplicates coverage item %q", binding.CoverageWorkItemID)
		}
		if _, duplicate := seenBinding[binding.BindingID]; duplicate {
			return fmt.Errorf("reconciliation contract duplicates binding %q", binding.BindingID)
		}
		seenCoverage[binding.CoverageWorkItemID] = struct{}{}
		seenBinding[binding.BindingID] = struct{}{}
	}
	if c.BindingSetHash != StableRuntimeCWIReconciliationContractHash(c) {
		return fmt.Errorf("reconciliation contract binding set hash does not match contents")
	}
	return nil
}

// CompileRuntimeCWIReconciliationContract is the pure activation-time
// conversion from sealed focus bindings to their reconciliation declaration.
// It does not inspect a workspace or derive a target from definition text.
func CompileRuntimeCWIReconciliationContract(definitionRevision string, focuses []CoverageFocus) (RuntimeCWIReconciliationContract, error) {
	definitionRevision = strings.TrimSpace(definitionRevision)
	if definitionRevision == "" {
		return RuntimeCWIReconciliationContract{}, fmt.Errorf("reconciliation contract requires definition revision")
	}
	ret := RuntimeCWIReconciliationContract{
		SchemaVersion:      RuntimeCWIReconciliationContractSchemaCurrent,
		DefinitionRevision: definitionRevision,
	}
	seenCoverage := map[common.CoverageWorkItemID]struct{}{}
	seenBinding := map[string]struct{}{}
	for _, focus := range focuses {
		focus = focus.Normalized()
		if err := ValidateCoverageEvidenceBindingsForFocus(focus, definitionRevision); err != nil {
			return RuntimeCWIReconciliationContract{}, err
		}
		for _, binding := range focus.CoverageEvidenceBindings {
			binding = binding.Normalized()
			if _, duplicate := seenCoverage[binding.CoverageWorkItemID]; duplicate {
				return RuntimeCWIReconciliationContract{}, fmt.Errorf("reconciliation contract duplicates coverage item %q", binding.CoverageWorkItemID)
			}
			if _, duplicate := seenBinding[binding.ID]; duplicate {
				return RuntimeCWIReconciliationContract{}, fmt.Errorf("reconciliation contract duplicates binding %q", binding.ID)
			}
			owners := make([]string, 0, len(binding.Proofs))
			operationKinds := make([]string, 0, len(binding.Proofs))
			recipeIDs := make([]string, 0, len(binding.Proofs))
			checkRefs := make([]string, 0, len(binding.Proofs))
			checkAttestations := make([]RuntimeValidationCheckAttestationBinding, 0, len(binding.Proofs))
			metricConstraints := make([]string, 0, len(binding.Proofs))
			artifactRefs := make([]string, 0, len(binding.Proofs))
			for _, proof := range binding.Proofs {
				operationKinds = append(operationKinds, proof.AllowedOperationKinds...)
				if proof.ValidationRecipeID != "" {
					recipeIDs = append(recipeIDs, proof.ValidationRecipeID)
				}
				checkRefs = append(checkRefs, proof.RequiredCheckRefs...)
				checkAttestations = append(checkAttestations, proof.ValidationCheckAttestations...)
				metricConstraints = append(metricConstraints, proof.MetricConstraints...)
				artifactRefs = append(artifactRefs, proof.ArtifactRefs...)
				for _, kind := range proof.AllowedOperationKinds {
					descriptor, found := RuntimeOperationDescriptorForKind(kind)
					if !found || strings.TrimSpace(descriptor.OwnerModuleID) == "" {
						return RuntimeCWIReconciliationContract{}, fmt.Errorf("reconciliation binding %q has unknown operation owner for %q", binding.ID, kind)
					}
					owners = append(owners, descriptor.OwnerModuleID)
				}
			}
			ref := RuntimeCWIReconciliationBindingRef{
				CoverageWorkItemID:                   binding.CoverageWorkItemID,
				BindingID:                            binding.ID,
				BindingHash:                          binding.Hash,
				OwnerModuleIDs:                       owners,
				RequiredEvidenceKinds:                binding.EvidenceKinds(),
				AllowedOperationKinds:                operationKinds,
				TargetRefs:                           binding.TargetRefs,
				TargetSelectorRef:                    binding.TargetSelectorRef,
				TargetSelectorOwnerRef:               binding.TargetSelectorOwnerRef,
				TargetSelectorSubjectRef:             binding.TargetSelectorSubjectRef,
				TargetSelectorCandidateRefs:          binding.TargetSelectorCandidateRefs,
				ProspectiveTargetAllOfConstraintRefs: binding.ProspectiveTargetAllOfConstraintRefs,
				ProspectiveTargetRefs:                binding.ProspectiveTargetRefs,
				ValidationRecipeIDs:                  recipeIDs,
				RequiredCheckRefs:                    checkRefs,
				ValidationCheckAttestations:          checkAttestations,
				MetricConstraints:                    metricConstraints,
				ArtifactRefs:                         artifactRefs,
			}.Normalized()
			if err := ref.Validate(); err != nil {
				return RuntimeCWIReconciliationContract{}, err
			}
			ret.Bindings = append(ret.Bindings, ref)
			seenCoverage[ref.CoverageWorkItemID] = struct{}{}
			seenBinding[ref.BindingID] = struct{}{}
		}
	}
	ret = ret.Normalized()
	ret.BindingSetHash = StableRuntimeCWIReconciliationContractHash(ret)
	if err := ret.Validate(); err != nil {
		return RuntimeCWIReconciliationContract{}, err
	}
	return ret, nil
}

// StableRuntimeCWIReconciliationContractHash hashes only normalized
// contractual values.  It excludes timestamps, request IDs, leases, provider
// data, projection revisions and original diagnostics.
func StableRuntimeCWIReconciliationContractHash(contract RuntimeCWIReconciliationContract) string {
	contract = contract.Normalized()
	contract.BindingSetHash = ""
	h := sha256.New()
	writeHashString(h, contract.SchemaVersion)
	writeHashString(h, contract.DefinitionRevision)
	for _, binding := range contract.Bindings {
		writeHashString(h, string(binding.CoverageWorkItemID))
		writeHashString(h, binding.BindingID)
		writeHashString(h, binding.BindingHash)
		writeHashStrings(h, binding.OwnerModuleIDs)
		writeHashStrings(h, binding.RequiredEvidenceKinds)
		writeHashStrings(h, binding.AllowedOperationKinds)
		writeHashStrings(h, binding.TargetRefs)
		writeHashString(h, binding.TargetSelectorRef)
		writeHashString(h, binding.TargetSelectorOwnerRef)
		writeHashString(h, binding.TargetSelectorSubjectRef)
		writeHashStrings(h, binding.TargetSelectorCandidateRefs)
		writeHashStrings(h, binding.ProspectiveTargetAllOfConstraintRefs)
		writeHashStrings(h, binding.ProspectiveTargetRefs)
		writeHashStrings(h, binding.ValidationRecipeIDs)
		writeHashStrings(h, binding.RequiredCheckRefs)
		for _, attestation := range binding.ValidationCheckAttestations {
			writeHashString(h, attestation.RequiredCheckRef)
			writeHashString(h, attestation.ObservationProtocolRef)
			writeHashString(h, attestation.NativeCheckRef)
			writeRuntimeSupplementalNativeCheckHash(h, attestation)
		}
		writeHashStrings(h, binding.MetricConstraints)
		writeHashStrings(h, binding.ArtifactRefs)
	}
	return "runtime-cwi-reconciliation-" + hex.EncodeToString(h.Sum(nil))
}

// SameRuntimeCWIReconciliationContract compares the normalized immutable
// declaration.  It is a pure bootstrap equality helper and never repairs a
// missing contract from an old Run.
func SameRuntimeCWIReconciliationContract(left, right RuntimeCWIReconciliationContract) bool {
	left = left.Normalized()
	right = right.Normalized()
	return left.SchemaVersion == right.SchemaVersion &&
		left.DefinitionRevision == right.DefinitionRevision &&
		left.BindingSetHash == right.BindingSetHash &&
		sameRuntimeCWIReconciliationBindingRefs(left.Bindings, right.Bindings)
}

// RuntimeCWIReconciliationRequest is the run-scoped, immutable request that
// SM-4 may schedule.  It has no requested coverage result and cannot select a
// lifecycle transition.
type RuntimeCWIReconciliationRequest struct {
	RequestID                 string
	WorkspaceID               common.WorkspaceID
	RunID                     common.RunID
	Scope                     RuntimeCWIReconciliationScope
	DefinitionRevision        string
	BindingSetHash            string
	Bindings                  []RuntimeCWIReconciliationBindingRef
	WorkspaceBaselineRevision string
	WorkspaceBaselineRef      string
	Generation                uint64
}

// RuntimeCWIOwnerReconciliationRequest is the owner-neutral value from which
// Orchestrator constructs one concrete receiver-owned query command. It is not
// itself an EventHub command.
type RuntimeCWIOwnerReconciliationRequest struct {
	WorkspaceID                   common.WorkspaceID
	RunID                         common.RunID
	RequestID                     string
	Generation                    uint64
	DefinitionRevision            string
	BindingSetHash                string
	OwnerModuleID                 string
	Binding                       RuntimeCWIReconciliationBindingRef
	WorkspaceDeliverableDirectory string
	WorkspaceInternalDirectory    string
	WorkspaceBaselineRevision     string
	WorkspaceBaselineRef          string
	ValidationRecipes             []RuntimeValidationRecipe
	// ValidationScopeTargetRefs is projected from the exact completed dispatch
	// for this binding, never from a later workspace inventory. Empty means no
	// replayable validation scope; owners must not infer one from TargetRefs.
	ValidationScopeTargetRefs []string
}

func ValidateRuntimeCWIOwnerReconciliationRequest(c RuntimeCWIOwnerReconciliationRequest, ownerModuleID string) error {
	if c.WorkspaceID == "" || c.RunID == "" || strings.TrimSpace(c.RequestID) == "" || c.Generation == 0 ||
		strings.TrimSpace(c.DefinitionRevision) == "" || strings.TrimSpace(c.BindingSetHash) == "" ||
		strings.TrimSpace(c.OwnerModuleID) == "" || strings.TrimSpace(ownerModuleID) != c.OwnerModuleID ||
		strings.TrimSpace(c.WorkspaceDeliverableDirectory) == "" || strings.TrimSpace(c.WorkspaceBaselineRevision) == "" ||
		strings.TrimSpace(c.WorkspaceBaselineRef) == "" {
		return common.ErrInvalidArgument
	}
	if err := c.Binding.Validate(); err != nil {
		return err
	}
	for _, value := range c.Binding.OwnerModuleIDs {
		if value == c.OwnerModuleID {
			return nil
		}
	}
	return fmt.Errorf("reconciliation binding %q does not authorize owner %q", c.Binding.BindingID, c.OwnerModuleID)
}

func (r RuntimeCWIReconciliationRequest) Normalized() RuntimeCWIReconciliationRequest {
	r.RequestID = strings.TrimSpace(r.RequestID)
	r.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(r.WorkspaceID)))
	r.RunID = common.RunID(strings.TrimSpace(string(r.RunID)))
	r.Scope = RuntimeCWIReconciliationScope(strings.TrimSpace(string(r.Scope)))
	r.DefinitionRevision = strings.TrimSpace(r.DefinitionRevision)
	r.BindingSetHash = strings.TrimSpace(r.BindingSetHash)
	r.Bindings = cloneRuntimeCWIReconciliationBindingRefs(r.Bindings)
	r.WorkspaceBaselineRevision = strings.TrimSpace(r.WorkspaceBaselineRevision)
	r.WorkspaceBaselineRef = strings.TrimSpace(r.WorkspaceBaselineRef)
	return r
}

func (r RuntimeCWIReconciliationRequest) Clone() RuntimeCWIReconciliationRequest {
	return r.Normalized()
}

func (r RuntimeCWIReconciliationRequest) Validate() error {
	r = r.Normalized()
	if r.RequestID == "" || r.WorkspaceID == "" || r.RunID == "" || r.DefinitionRevision == "" || r.BindingSetHash == "" || r.Generation == 0 {
		return fmt.Errorf("reconciliation request requires request, scope, definition, binding and generation identity")
	}
	if r.Scope != RuntimeCWIReconciliationScopeCurrentRun && r.Scope != RuntimeCWIReconciliationScopeWorkspaceFull {
		return fmt.Errorf("reconciliation request has unsupported scope %q", r.Scope)
	}
	if r.WorkspaceBaselineRevision == "" || r.WorkspaceBaselineRef == "" {
		return fmt.Errorf("reconciliation request requires workspace baseline revision and ref")
	}
	if len(r.Bindings) == 0 {
		return fmt.Errorf("reconciliation request requires binding refs")
	}
	return RuntimeCWIReconciliationContract{
		SchemaVersion:      RuntimeCWIReconciliationContractSchemaCurrent,
		DefinitionRevision: r.DefinitionRevision,
		BindingSetHash:     r.BindingSetHash,
		Bindings:           r.Bindings,
	}.Validate()
}

func (r RuntimeCWIReconciliationRequest) ValidateAgainstContract(contract RuntimeCWIReconciliationContract) error {
	r = r.Normalized()
	contract = contract.Normalized()
	if err := r.Validate(); err != nil {
		return err
	}
	if err := contract.Validate(); err != nil {
		return err
	}
	if r.DefinitionRevision != contract.DefinitionRevision || r.BindingSetHash != contract.BindingSetHash || !sameRuntimeCWIReconciliationBindingRefs(r.Bindings, contract.Bindings) {
		return fmt.Errorf("reconciliation request does not match sealed contract")
	}
	return nil
}

// RuntimeCWIReconciliationObservation is an immutable current-workspace fact
// from one declared owner.  Its structured refs are opaque capability facts;
// callers must not fill them from Goal/DoD prose, summaries or LLM output.
type RuntimeCWIReconciliationObservation struct {
	RequestID          string
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	Generation         uint64
	CoverageWorkItemID common.CoverageWorkItemID
	BindingID          string
	BindingHash        string
	OwnerModuleID      string
	ObservedRevision   string
	Status             RuntimeCWIReconciliationObservationStatus
	TargetRefs         []string
	CheckRefs          []string
	MetricRefs         []string
	ArtifactRefs       []string
	OriginalDiagnostic *RuntimeOriginalDiagnostic
}

// StableRuntimeCWIReconciliationObservationRef identifies one owner fact in
// a batch without persisting diagnostic text. It is safe for result lineage,
// duplicate delivery and projection keys.
func StableRuntimeCWIReconciliationObservationRef(observation RuntimeCWIReconciliationObservation) string {
	observation = observation.Normalized()
	h := sha256.New()
	writeHashString(h, observation.RequestID)
	writeHashString(h, string(observation.WorkspaceID))
	writeHashString(h, string(observation.RunID))
	writeHashString(h, fmt.Sprintf("%d", observation.Generation))
	writeHashString(h, string(observation.CoverageWorkItemID))
	writeHashString(h, observation.BindingID)
	writeHashString(h, observation.BindingHash)
	writeHashString(h, observation.OwnerModuleID)
	writeHashString(h, observation.ObservedRevision)
	writeHashString(h, string(observation.Status))
	return "runtime-cwi-observation-" + hex.EncodeToString(h.Sum(nil))
}

// StableRuntimeCWIReconciliationObservationSetRef joins the complete owner
// fact set for one binding into a deterministic lineage identity.
func StableRuntimeCWIReconciliationObservationSetRef(observations []RuntimeCWIReconciliationObservation) string {
	observations = cloneRuntimeCWIReconciliationObservations(observations)
	h := sha256.New()
	for _, observation := range observations {
		writeHashString(h, StableRuntimeCWIReconciliationObservationRef(observation))
	}
	return "runtime-cwi-observation-set-" + hex.EncodeToString(h.Sum(nil))
}

func (o RuntimeCWIReconciliationObservation) Normalized() RuntimeCWIReconciliationObservation {
	o.RequestID = strings.TrimSpace(o.RequestID)
	o.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(o.WorkspaceID)))
	o.RunID = common.RunID(strings.TrimSpace(string(o.RunID)))
	o.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(o.CoverageWorkItemID)))
	o.BindingID = strings.TrimSpace(o.BindingID)
	o.BindingHash = strings.TrimSpace(o.BindingHash)
	o.OwnerModuleID = strings.TrimSpace(o.OwnerModuleID)
	o.ObservedRevision = strings.TrimSpace(o.ObservedRevision)
	o.Status = RuntimeCWIReconciliationObservationStatus(strings.TrimSpace(string(o.Status)))
	o.TargetRefs = normalizeStrings(o.TargetRefs)
	o.CheckRefs = normalizeStrings(o.CheckRefs)
	o.MetricRefs = normalizeStrings(o.MetricRefs)
	o.ArtifactRefs = normalizeStrings(o.ArtifactRefs)
	o.OriginalDiagnostic = o.OriginalDiagnostic.CloneOriginal()
	return o
}

func (o RuntimeCWIReconciliationObservation) Clone() RuntimeCWIReconciliationObservation {
	return o.Normalized()
}

func (o RuntimeCWIReconciliationObservation) Validate() error {
	o = o.Normalized()
	if o.RequestID == "" || o.WorkspaceID == "" || o.RunID == "" || o.Generation == 0 || o.CoverageWorkItemID == "" ||
		o.BindingID == "" || o.BindingHash == "" || o.OwnerModuleID == "" || o.ObservedRevision == "" {
		return fmt.Errorf("reconciliation observation requires complete request, binding, owner and revision identity")
	}
	switch o.Status {
	case RuntimeCWIReconciliationObservationSatisfied, RuntimeCWIReconciliationObservationUnsatisfied,
		RuntimeCWIReconciliationObservationUnverifiable, RuntimeCWIReconciliationObservationStale:
	default:
		return fmt.Errorf("reconciliation observation has unsupported status %q", o.Status)
	}
	if o.OriginalDiagnostic != nil {
		if err := o.OriginalDiagnostic.Validate(); err != nil {
			return fmt.Errorf("reconciliation observation diagnostic: %w", err)
		}
		if o.OriginalDiagnostic.Normalized().OwnerModuleID != o.OwnerModuleID {
			return fmt.Errorf("reconciliation observation diagnostic owner mismatch")
		}
	}
	return nil
}

func (o RuntimeCWIReconciliationObservation) ValidateForRequest(request RuntimeCWIReconciliationRequest) error {
	o = o.Normalized()
	request = request.Normalized()
	if err := o.Validate(); err != nil {
		return err
	}
	if o.RequestID != request.RequestID || o.WorkspaceID != request.WorkspaceID || o.RunID != request.RunID || o.Generation != request.Generation {
		return fmt.Errorf("reconciliation observation request scope mismatch")
	}
	for _, binding := range request.Bindings {
		if binding.CoverageWorkItemID != o.CoverageWorkItemID || binding.BindingID != o.BindingID || binding.BindingHash != o.BindingHash {
			continue
		}
		if !containsString(binding.OwnerModuleIDs, o.OwnerModuleID) {
			return fmt.Errorf("reconciliation observation owner %q is outside binding %q", o.OwnerModuleID, o.BindingID)
		}
		return nil
	}
	return fmt.Errorf("reconciliation observation references unknown binding")
}

// RuntimeCWIReconciliationBatch is the complete response set for one
// persisted request generation.  It carries only factual observations and
// cannot add credit or select an SM-2 action. Stable observation refs are
// derived from each canonical observation and are not stored a second time.
type RuntimeCWIReconciliationBatch struct {
	RequestID      string
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	Generation     uint64
	BindingSetHash string
	Observations   []RuntimeCWIReconciliationObservation
	Hash           string
}

func (b RuntimeCWIReconciliationBatch) Normalized() RuntimeCWIReconciliationBatch {
	b.RequestID = strings.TrimSpace(b.RequestID)
	b.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(b.WorkspaceID)))
	b.RunID = common.RunID(strings.TrimSpace(string(b.RunID)))
	b.BindingSetHash = strings.TrimSpace(b.BindingSetHash)
	b.Hash = strings.TrimSpace(b.Hash)
	b.Observations = cloneRuntimeCWIReconciliationObservations(b.Observations)
	return b
}

func (b RuntimeCWIReconciliationBatch) Clone() RuntimeCWIReconciliationBatch { return b.Normalized() }

func (b RuntimeCWIReconciliationBatch) ValidateForRequest(request RuntimeCWIReconciliationRequest) error {
	b = b.Normalized()
	request = request.Normalized()
	if err := request.Validate(); err != nil {
		return err
	}
	if b.RequestID != request.RequestID || b.WorkspaceID != request.WorkspaceID || b.RunID != request.RunID ||
		b.Generation != request.Generation || b.BindingSetHash != request.BindingSetHash {
		return fmt.Errorf("reconciliation batch request identity mismatch")
	}
	if len(b.Observations) == 0 {
		return fmt.Errorf("reconciliation batch requires observations")
	}
	seen := make(map[string]struct{}, len(b.Observations))
	for _, observation := range b.Observations {
		if err := observation.ValidateForRequest(request); err != nil {
			return err
		}
		key := observation.BindingID + "\x00" + observation.OwnerModuleID
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("reconciliation batch duplicates observation for binding %q owner %q", observation.BindingID, observation.OwnerModuleID)
		}
		seen[key] = struct{}{}
	}
	for _, binding := range request.Bindings {
		for _, owner := range binding.OwnerModuleIDs {
			if _, found := seen[binding.BindingID+"\x00"+owner]; !found {
				return fmt.Errorf("reconciliation batch is incomplete for binding %q owner %q", binding.BindingID, owner)
			}
		}
	}
	if b.Hash != StableRuntimeCWIReconciliationBatchHash(b) {
		return fmt.Errorf("reconciliation batch hash does not match contents")
	}
	return nil
}

// StableRuntimeCWIReconciliationBatchHash intentionally excludes raw
// diagnostic text and all clock/lease/provider data.  The original diagnostic
// remains preserved in the observation for audit but cannot perturb stable
// request identity or recovery behavior.
func StableRuntimeCWIReconciliationBatchHash(batch RuntimeCWIReconciliationBatch) string {
	batch = batch.Normalized()
	batch.Hash = ""
	h := sha256.New()
	writeHashString(h, batch.RequestID)
	writeHashString(h, string(batch.WorkspaceID))
	writeHashString(h, string(batch.RunID))
	writeHashString(h, fmt.Sprintf("%d", batch.Generation))
	writeHashString(h, batch.BindingSetHash)
	for _, observation := range batch.Observations {
		writeHashString(h, observation.RequestID)
		writeHashString(h, string(observation.WorkspaceID))
		writeHashString(h, string(observation.RunID))
		writeHashString(h, fmt.Sprintf("%d", observation.Generation))
		writeHashString(h, string(observation.CoverageWorkItemID))
		writeHashString(h, observation.BindingID)
		writeHashString(h, observation.BindingHash)
		writeHashString(h, observation.OwnerModuleID)
		writeHashString(h, observation.ObservedRevision)
		writeHashString(h, string(observation.Status))
		writeHashStrings(h, observation.TargetRefs)
		writeHashStrings(h, observation.CheckRefs)
		writeHashStrings(h, observation.MetricRefs)
		writeHashStrings(h, observation.ArtifactRefs)
	}
	return "runtime-cwi-reconciliation-batch-" + hex.EncodeToString(h.Sum(nil))
}

// RuntimeCWIReconciliationResult is the bounded internal input prepared for
// the SM-2 reducer after a batch is validated.  Its disposition does not cross
// owner boundaries and is not a command to mutate coverage directly.
type RuntimeCWIReconciliationResult struct {
	RequestID      string
	WorkspaceID    common.WorkspaceID
	RunID          common.RunID
	Generation     uint64
	BatchHash      string
	CoverageItemID common.CoverageWorkItemID
	BindingID      string
	Disposition    RuntimeCWIReconciliationDisposition
	ObservationRef string
}

func (r RuntimeCWIReconciliationResult) Normalized() RuntimeCWIReconciliationResult {
	r.RequestID = strings.TrimSpace(r.RequestID)
	r.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(r.WorkspaceID)))
	r.RunID = common.RunID(strings.TrimSpace(string(r.RunID)))
	r.BatchHash = strings.TrimSpace(r.BatchHash)
	r.CoverageItemID = common.CoverageWorkItemID(strings.TrimSpace(string(r.CoverageItemID)))
	r.BindingID = strings.TrimSpace(r.BindingID)
	r.Disposition = RuntimeCWIReconciliationDisposition(strings.TrimSpace(string(r.Disposition)))
	r.ObservationRef = strings.TrimSpace(r.ObservationRef)
	return r
}

func (r RuntimeCWIReconciliationResult) Validate() error {
	r = r.Normalized()
	if r.RequestID == "" || r.WorkspaceID == "" || r.RunID == "" || r.Generation == 0 || r.BatchHash == "" ||
		r.CoverageItemID == "" || r.BindingID == "" || r.ObservationRef == "" {
		return fmt.Errorf("reconciliation result requires complete request, batch, coverage and observation identity")
	}
	switch r.Disposition {
	case RuntimeCWIReconciliationDispositionRetain, RuntimeCWIReconciliationDispositionReopen,
		RuntimeCWIReconciliationDispositionPrerequisite, RuntimeCWIReconciliationDispositionTerminal:
		return nil
	default:
		return fmt.Errorf("reconciliation result has unsupported disposition %q", r.Disposition)
	}
}

// RuntimeCWIReconciliationReopenedLineage records why an already accepted
// coverage credit was reopened by a current-workspace observation.  The old
// evidence remains authoritative history; this value is only the explicit
// SM-2 lineage connecting that history to the new observation.
type RuntimeCWIReconciliationReopenedLineage struct {
	RequestID            string
	WorkspaceID          common.WorkspaceID
	RunID                common.RunID
	Generation           uint64
	BatchHash            string
	CoverageWorkItemID   common.CoverageWorkItemID
	BindingID            string
	ObservationRef       string
	PreviousEvidenceRefs []string
	Reason               string
}

func (l RuntimeCWIReconciliationReopenedLineage) Normalized() RuntimeCWIReconciliationReopenedLineage {
	l.RequestID = strings.TrimSpace(l.RequestID)
	l.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(l.WorkspaceID)))
	l.RunID = common.RunID(strings.TrimSpace(string(l.RunID)))
	l.BatchHash = strings.TrimSpace(l.BatchHash)
	l.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(l.CoverageWorkItemID)))
	l.BindingID = strings.TrimSpace(l.BindingID)
	l.ObservationRef = strings.TrimSpace(l.ObservationRef)
	l.PreviousEvidenceRefs = normalizeStrings(l.PreviousEvidenceRefs)
	l.Reason = strings.TrimSpace(l.Reason)
	return l
}

func (l RuntimeCWIReconciliationReopenedLineage) Clone() RuntimeCWIReconciliationReopenedLineage {
	return l.Normalized()
}

func (l RuntimeCWIReconciliationReopenedLineage) Validate() error {
	l = l.Normalized()
	if l.RequestID == "" || l.WorkspaceID == "" || l.RunID == "" || l.Generation == 0 ||
		l.BatchHash == "" || l.CoverageWorkItemID == "" || l.BindingID == "" ||
		l.ObservationRef == "" || l.Reason == "" {
		return fmt.Errorf("reconciliation reopened lineage requires complete identity and reason")
	}
	return nil
}

// RuntimeCWIReconciliationLedger is the Orchestrator-owned persisted SM-2
// ledger.  It deliberately stores the request/batch facts and their derived
// per-CWI dispositions together, so duplicate delivery and process restart
// never need to infer reconciliation state from coverage alone.
type RuntimeCWIReconciliationLedger struct {
	Request     *RuntimeCWIReconciliationRequest
	RequestHash string
	Batch       *RuntimeCWIReconciliationBatch
	Results     []RuntimeCWIReconciliationResult
	Reopened    []RuntimeCWIReconciliationReopenedLineage
	// History keeps every superseded request/batch generation.  A new resume
	// must not overwrite the prior owner observations: SM-2 needs the exact
	// lineage to explain which old credit was retained or reopened, and a
	// duplicate request must remain idempotent after a process restart.
	History []RuntimeCWIReconciliationGeneration
}

// RuntimeCWIReconciliationHistoryLimit keeps superseded SM-4 request
// generations bounded. The active request is intentionally not part of this
// count, so a full history remains auditable while one current generation can
// still be scheduled by the Orchestrator.
const RuntimeCWIReconciliationHistoryLimit = 16

// RuntimeCWIReconciliationGeneration is an immutable historical snapshot of
// one reconciliation request and its owner response.  It is pure contract
// data; only the Orchestrator reducer decides when a generation is superseded.
type RuntimeCWIReconciliationGeneration struct {
	// Superseded is explicit because a historical request may carry a binding
	// set that no longer equals the current SM-2 contract. History is audit
	// only and must never be selected as an active reconciliation generation.
	Superseded  bool
	Request     RuntimeCWIReconciliationRequest
	RequestHash string
	Batch       *RuntimeCWIReconciliationBatch
	Results     []RuntimeCWIReconciliationResult
	Reopened    []RuntimeCWIReconciliationReopenedLineage
}

func (g RuntimeCWIReconciliationGeneration) Normalized() RuntimeCWIReconciliationGeneration {
	g.Request = g.Request.Clone()
	g.RequestHash = strings.TrimSpace(g.RequestHash)
	if g.Batch != nil {
		batch := g.Batch.Clone()
		g.Batch = &batch
	}
	if len(g.Results) > 0 {
		values := make([]RuntimeCWIReconciliationResult, 0, len(g.Results))
		for _, value := range g.Results {
			values = append(values, value.Normalized())
		}
		sort.SliceStable(values, func(i, j int) bool {
			left := string(values[i].CoverageItemID) + "\x00" + values[i].BindingID
			right := string(values[j].CoverageItemID) + "\x00" + values[j].BindingID
			return left < right
		})
		g.Results = values
	}
	if len(g.Reopened) > 0 {
		values := make([]RuntimeCWIReconciliationReopenedLineage, 0, len(g.Reopened))
		for _, value := range g.Reopened {
			values = append(values, value.Normalized())
		}
		sort.SliceStable(values, func(i, j int) bool {
			left := string(values[i].CoverageWorkItemID) + "\x00" + values[i].ObservationRef
			right := string(values[j].CoverageWorkItemID) + "\x00" + values[j].ObservationRef
			return left < right
		})
		g.Reopened = values
	}
	return g
}

func (g RuntimeCWIReconciliationGeneration) Clone() RuntimeCWIReconciliationGeneration {
	return g.Normalized()
}

func (g RuntimeCWIReconciliationGeneration) Validate() error {
	g = g.Normalized()
	if !g.Superseded {
		return fmt.Errorf("reconciliation history generation is not marked superseded")
	}
	if err := g.Request.Validate(); err != nil {
		return fmt.Errorf("reconciliation history request: %w", err)
	}
	if g.RequestHash == "" || g.RequestHash != StableRuntimeCWIReconciliationRequestHash(g.Request) {
		return fmt.Errorf("reconciliation history request hash does not match request")
	}
	if g.Batch == nil {
		if len(g.Results) != 0 || len(g.Reopened) != 0 {
			return fmt.Errorf("reconciliation history has derived data without batch")
		}
		return nil
	}
	if err := g.Batch.ValidateForRequest(g.Request); err != nil {
		return fmt.Errorf("reconciliation history batch: %w", err)
	}
	if len(g.Results) != len(g.Request.Bindings) {
		return fmt.Errorf("reconciliation history requires one result per binding")
	}
	for _, result := range g.Results {
		if err := result.Validate(); err != nil {
			return fmt.Errorf("reconciliation history result: %w", err)
		}
		if result.RequestID != g.Request.RequestID || result.WorkspaceID != g.Request.WorkspaceID || result.RunID != g.Request.RunID || result.Generation != g.Request.Generation || result.BatchHash != g.Batch.Hash {
			return fmt.Errorf("reconciliation history result identity mismatch")
		}
	}
	for _, lineage := range g.Reopened {
		if err := lineage.Validate(); err != nil {
			return fmt.Errorf("reconciliation history lineage: %w", err)
		}
		if lineage.RequestID != g.Request.RequestID || lineage.WorkspaceID != g.Request.WorkspaceID || lineage.RunID != g.Request.RunID || lineage.Generation != g.Request.Generation || lineage.BatchHash != g.Batch.Hash {
			return fmt.Errorf("reconciliation history lineage identity mismatch")
		}
	}
	return nil
}

func (l RuntimeCWIReconciliationLedger) Normalized() RuntimeCWIReconciliationLedger {
	l.RequestHash = strings.TrimSpace(l.RequestHash)
	if l.Request != nil {
		value := l.Request.Clone()
		l.Request = &value
	}
	if l.Batch != nil {
		value := l.Batch.Clone()
		l.Batch = &value
	}
	if len(l.Results) > 0 {
		values := make([]RuntimeCWIReconciliationResult, 0, len(l.Results))
		for _, value := range l.Results {
			values = append(values, value.Normalized())
		}
		// The request's binding order is semantic; result order is not. Stable
		// sorting makes persistence hashes and duplicate comparisons deterministic.
		sort.SliceStable(values, func(i, j int) bool {
			left := string(values[i].CoverageItemID) + "\x00" + values[i].BindingID
			right := string(values[j].CoverageItemID) + "\x00" + values[j].BindingID
			return left < right
		})
		l.Results = values
	}
	if len(l.Reopened) > 0 {
		values := make([]RuntimeCWIReconciliationReopenedLineage, 0, len(l.Reopened))
		for _, value := range l.Reopened {
			values = append(values, value.Normalized())
		}
		sort.SliceStable(values, func(i, j int) bool {
			left := string(values[i].CoverageWorkItemID) + "\x00" + values[i].ObservationRef
			right := string(values[j].CoverageWorkItemID) + "\x00" + values[j].ObservationRef
			return left < right
		})
		l.Reopened = values
	}
	if len(l.History) > 0 {
		values := make([]RuntimeCWIReconciliationGeneration, 0, len(l.History))
		for _, value := range l.History {
			values = append(values, value.Normalized())
		}
		sort.SliceStable(values, func(i, j int) bool {
			return values[i].Request.Generation < values[j].Request.Generation
		})
		l.History = values
	}
	return l
}

func (l RuntimeCWIReconciliationLedger) Clone() RuntimeCWIReconciliationLedger {
	return l.Normalized()
}

func (l RuntimeCWIReconciliationLedger) Validate() error {
	l = l.Normalized()
	if len(l.History) > RuntimeCWIReconciliationHistoryLimit {
		return fmt.Errorf("reconciliation history exceeds bounded limit %d", RuntimeCWIReconciliationHistoryLimit)
	}
	seenHistoryGeneration := make(map[uint64]struct{}, len(l.History))
	seenHistoryRequest := make(map[string]struct{}, len(l.History))
	var historyWorkspaceID common.WorkspaceID
	var historyRunID common.RunID
	for _, history := range l.History {
		history = history.Normalized()
		if err := history.Validate(); err != nil {
			return err
		}
		if historyWorkspaceID == "" {
			historyWorkspaceID, historyRunID = history.Request.WorkspaceID, history.Request.RunID
		} else if history.Request.WorkspaceID != historyWorkspaceID || history.Request.RunID != historyRunID {
			return fmt.Errorf("reconciliation history mixes runtime scopes")
		}
		if _, duplicate := seenHistoryGeneration[history.Request.Generation]; duplicate {
			return fmt.Errorf("reconciliation history duplicates generation %d", history.Request.Generation)
		}
		if _, duplicate := seenHistoryRequest[history.Request.RequestID]; duplicate {
			return fmt.Errorf("reconciliation history duplicates request %q", history.Request.RequestID)
		}
		seenHistoryGeneration[history.Request.Generation] = struct{}{}
		seenHistoryRequest[history.Request.RequestID] = struct{}{}
	}
	if l.Request == nil {
		if l.RequestHash != "" || l.Batch != nil || len(l.Results) != 0 || len(l.Reopened) != 0 {
			return fmt.Errorf("reconciliation ledger has data without request")
		}
		return nil
	}
	request := l.Request.Clone()
	if err := request.Validate(); err != nil {
		return fmt.Errorf("reconciliation ledger request: %w", err)
	}
	if l.RequestHash == "" || l.RequestHash != StableRuntimeCWIReconciliationRequestHash(request) {
		return fmt.Errorf("reconciliation ledger request hash does not match request")
	}
	if historyWorkspaceID != "" && (historyWorkspaceID != request.WorkspaceID || historyRunID != request.RunID) {
		return fmt.Errorf("reconciliation history request scope mismatch")
	}
	if l.Batch != nil {
		batch := l.Batch.Clone()
		if err := batch.ValidateForRequest(request); err != nil {
			return fmt.Errorf("reconciliation ledger batch: %w", err)
		}
		if len(l.Results) != len(request.Bindings) {
			return fmt.Errorf("reconciliation ledger requires one result per binding")
		}
		seen := make(map[string]struct{}, len(l.Results))
		validRefs := make(map[string]struct{}, len(batch.Observations)+len(request.Bindings))
		byBinding := make(map[string][]RuntimeCWIReconciliationObservation)
		for _, observation := range batch.Observations {
			validRefs[StableRuntimeCWIReconciliationObservationRef(observation)] = struct{}{}
			key := string(observation.CoverageWorkItemID) + "\x00" + observation.BindingID
			byBinding[key] = append(byBinding[key], observation)
		}
		for _, observations := range byBinding {
			validRefs[StableRuntimeCWIReconciliationObservationSetRef(observations)] = struct{}{}
		}
		for _, result := range l.Results {
			result = result.Normalized()
			if err := result.Validate(); err != nil {
				return fmt.Errorf("reconciliation ledger result: %w", err)
			}
			if result.RequestID != request.RequestID || result.WorkspaceID != request.WorkspaceID || result.RunID != request.RunID || result.Generation != request.Generation || result.BatchHash != batch.Hash {
				return fmt.Errorf("reconciliation ledger result identity mismatch")
			}
			key := string(result.CoverageItemID) + "\x00" + result.BindingID
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("reconciliation ledger duplicates result %q", key)
			}
			seen[key] = struct{}{}
			if _, found := validRefs[result.ObservationRef]; !found {
				return fmt.Errorf("reconciliation ledger result references unknown observation %q", result.ObservationRef)
			}
		}
	} else if len(l.Results) != 0 || len(l.Reopened) != 0 {
		return fmt.Errorf("reconciliation ledger has derived data without batch")
	}
	for _, history := range l.History {
		history = history.Normalized()
		if history.Request.WorkspaceID != request.WorkspaceID || history.Request.RunID != request.RunID {
			return fmt.Errorf("reconciliation history request scope mismatch")
		}
		if history.Request.Generation >= request.Generation {
			return fmt.Errorf("reconciliation history generation is not older than current request")
		}
	}
	if _, duplicate := seenHistoryGeneration[request.Generation]; duplicate {
		return fmt.Errorf("reconciliation history duplicates current generation %d", request.Generation)
	}
	if _, duplicate := seenHistoryRequest[request.RequestID]; duplicate {
		return fmt.Errorf("reconciliation history duplicates current request %q", request.RequestID)
	}
	for _, lineage := range l.Reopened {
		if err := lineage.Validate(); err != nil {
			return fmt.Errorf("reconciliation ledger reopened lineage: %w", err)
		}
		if lineage.RequestID != request.RequestID || lineage.WorkspaceID != request.WorkspaceID || lineage.RunID != request.RunID || lineage.Generation != request.Generation {
			return fmt.Errorf("reconciliation ledger reopened lineage scope mismatch")
		}
		if l.Batch == nil || lineage.BatchHash != l.Batch.Hash {
			return fmt.Errorf("reconciliation ledger reopened lineage batch mismatch")
		}
	}
	return nil
}

// StableRuntimeCWIReconciliationRequestHash is the idempotency identity for a
// sealed request.  It excludes only the request's mutable transport identity;
// all scope, baseline, generation and binding facts remain part of the hash.
func StableRuntimeCWIReconciliationRequestHash(request RuntimeCWIReconciliationRequest) string {
	request = request.Normalized()
	h := sha256.New()
	writeHashString(h, request.RequestID)
	writeHashString(h, string(request.WorkspaceID))
	writeHashString(h, string(request.RunID))
	writeHashString(h, string(request.Scope))
	writeHashString(h, request.DefinitionRevision)
	writeHashString(h, request.BindingSetHash)
	writeHashString(h, request.WorkspaceBaselineRevision)
	writeHashString(h, request.WorkspaceBaselineRef)
	writeHashString(h, fmt.Sprintf("%d", request.Generation))
	for _, binding := range request.Bindings {
		writeHashString(h, string(binding.CoverageWorkItemID))
		writeHashString(h, binding.BindingID)
		writeHashString(h, binding.BindingHash)
		writeHashStrings(h, binding.OwnerModuleIDs)
		writeHashStrings(h, binding.RequiredEvidenceKinds)
	}
	return "runtime-cwi-reconciliation-request-" + hex.EncodeToString(h.Sum(nil))
}

func cloneRuntimeCWIReconciliationBindingRefs(values []RuntimeCWIReconciliationBindingRef) []RuntimeCWIReconciliationBindingRef {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeCWIReconciliationBindingRef, len(values))
	for index := range values {
		ret[index] = values[index].Clone()
	}
	return ret
}

func cloneRuntimeCWIReconciliationObservations(values []RuntimeCWIReconciliationObservation) []RuntimeCWIReconciliationObservation {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeCWIReconciliationObservation, len(values))
	for index := range values {
		ret[index] = values[index].Clone()
	}
	sort.SliceStable(ret, func(i, j int) bool {
		left := ret[i].BindingID + "\x00" + ret[i].OwnerModuleID
		right := ret[j].BindingID + "\x00" + ret[j].OwnerModuleID
		return left < right
	})
	return ret
}

func sameRuntimeCWIReconciliationBindingRefs(left, right []RuntimeCWIReconciliationBindingRef) bool {
	left = cloneRuntimeCWIReconciliationBindingRefs(left)
	right = cloneRuntimeCWIReconciliationBindingRefs(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].CoverageWorkItemID != right[index].CoverageWorkItemID || left[index].BindingID != right[index].BindingID ||
			left[index].BindingHash != right[index].BindingHash || !sameStrings(left[index].OwnerModuleIDs, right[index].OwnerModuleIDs) ||
			!sameStrings(left[index].RequiredEvidenceKinds, right[index].RequiredEvidenceKinds) ||
			!sameStrings(left[index].AllowedOperationKinds, right[index].AllowedOperationKinds) ||
			!sameStrings(left[index].TargetRefs, right[index].TargetRefs) ||
			left[index].TargetSelectorRef != right[index].TargetSelectorRef ||
			left[index].TargetSelectorOwnerRef != right[index].TargetSelectorOwnerRef ||
			left[index].TargetSelectorSubjectRef != right[index].TargetSelectorSubjectRef ||
			!sameStrings(left[index].TargetSelectorCandidateRefs, right[index].TargetSelectorCandidateRefs) ||
			!SameRuntimeProspectiveTargetAllOfConstraintRefs(left[index].ProspectiveTargetAllOfConstraintRefs, right[index].ProspectiveTargetAllOfConstraintRefs) ||
			!sameStrings(left[index].ProspectiveTargetRefs, right[index].ProspectiveTargetRefs) ||
			!sameStrings(left[index].ValidationRecipeIDs, right[index].ValidationRecipeIDs) ||
			!sameStrings(left[index].RequiredCheckRefs, right[index].RequiredCheckRefs) ||
			!SameRuntimeValidationCheckAttestationBindings(left[index].ValidationCheckAttestations, right[index].ValidationCheckAttestations) ||
			!sameStrings(left[index].MetricConstraints, right[index].MetricConstraints) ||
			!sameStrings(left[index].ArtifactRefs, right[index].ArtifactRefs) {
			return false
		}
	}
	return true
}

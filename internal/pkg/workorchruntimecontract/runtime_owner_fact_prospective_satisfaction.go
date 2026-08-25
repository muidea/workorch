package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	RuntimeProspectiveWorkspaceManifestDeltaSchemaCurrent        = "workorch.runtime.prospective-workspace-manifest-delta.v1"
	RuntimeOwnerFactProspectiveSatisfactionSchemaCurrent         = "workorch.runtime.owner-fact-prospective-satisfaction.v1"
	RuntimeOwnerFactProspectiveSatisfactionRequestHashPrefix     = "runtime-owner-fact-prospective-satisfaction-request-"
	RuntimeOwnerFactProspectiveSatisfactionObservationHashPrefix = "runtime-owner-fact-prospective-satisfaction-observation-"
	RuntimeProspectiveWorkspaceManifestDeltaHashPrefix           = "runtime-prospective-workspace-manifest-delta-"
	RuntimeOwnerFactProspectiveSatisfactionHashPrefix            = "runtime-owner-fact-prospective-satisfaction-"
)

// RuntimeProspectiveWorkspaceManifestDelta is the File owner's immutable,
// side-effect-free statement of the exact bounded manifest that would result
// from one prospective creation proposal. It is not a workspace mutation or
// authority for another owner to execute the proposal.
type RuntimeProspectiveWorkspaceManifestDelta struct {
	SchemaVersion                      string
	WorkspaceID                        common.WorkspaceID
	RunID                              common.RunID
	ManifestHash                       string
	DefinitionSeedHash                 string
	CoverageWorkItemID                 common.CoverageWorkItemID
	ProposalHash                       string
	OwnerModuleID                      string
	CatalogSnapshotID                  string
	ProspectiveSatisfactionContractRef string
	BaseWorkspaceManifestRevision      string
	ProspectiveTargetRef               string
	ResultingTargetRefs                []string
}

func (d RuntimeProspectiveWorkspaceManifestDelta) Normalized() RuntimeProspectiveWorkspaceManifestDelta {
	d.SchemaVersion = strings.TrimSpace(d.SchemaVersion)
	d.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(d.WorkspaceID)))
	d.RunID = common.RunID(strings.TrimSpace(string(d.RunID)))
	d.ManifestHash = strings.TrimSpace(d.ManifestHash)
	d.DefinitionSeedHash = strings.TrimSpace(d.DefinitionSeedHash)
	d.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(d.CoverageWorkItemID)))
	d.ProposalHash = strings.TrimSpace(d.ProposalHash)
	d.OwnerModuleID = strings.TrimSpace(d.OwnerModuleID)
	d.CatalogSnapshotID = strings.TrimSpace(d.CatalogSnapshotID)
	d.ProspectiveSatisfactionContractRef = strings.TrimSpace(d.ProspectiveSatisfactionContractRef)
	d.BaseWorkspaceManifestRevision = strings.TrimSpace(d.BaseWorkspaceManifestRevision)
	d.ProspectiveTargetRef = strings.TrimSpace(d.ProspectiveTargetRef)
	d.ResultingTargetRefs = normalizeStrings(d.ResultingTargetRefs)
	return d
}

func (d RuntimeProspectiveWorkspaceManifestDelta) Clone() RuntimeProspectiveWorkspaceManifestDelta {
	d = d.Normalized()
	d.ResultingTargetRefs = append([]string(nil), d.ResultingTargetRefs...)
	return d
}

func (d RuntimeProspectiveWorkspaceManifestDelta) Validate() error {
	d = d.Normalized()
	if d.SchemaVersion != RuntimeProspectiveWorkspaceManifestDeltaSchemaCurrent || d.WorkspaceID == "" || d.RunID == "" ||
		d.ManifestHash == "" || d.DefinitionSeedHash == "" || d.CoverageWorkItemID == "" || d.ProposalHash == "" ||
		d.OwnerModuleID != RuntimeModuleFile || d.CatalogSnapshotID == "" || d.ProspectiveSatisfactionContractRef == "" ||
		d.BaseWorkspaceManifestRevision == "" || d.ProspectiveTargetRef == "" || len(d.ResultingTargetRefs) == 0 {
		return fmt.Errorf("runtime prospective workspace manifest delta requires File-owned scope, proposal, contract, catalog, revision and targets")
	}
	if !containsString(d.ResultingTargetRefs, d.ProspectiveTargetRef) {
		return fmt.Errorf("runtime prospective workspace manifest delta does not contain its prospective target")
	}
	return nil
}

// NewRuntimeProspectiveWorkspaceManifestDelta derives a bounded hypothetical
// File manifest from an already observed current manifest. It performs no I/O.
func NewRuntimeProspectiveWorkspaceManifestDelta(manifest RuntimeSemanticPlanManifest, proposal RuntimeSemanticProofProposal, contractRef string, ownerCatalog RuntimeOperationAdmissibility, current WorkspaceManifest) (RuntimeProspectiveWorkspaceManifestDelta, error) {
	manifest = manifest.Normalized()
	proposal = proposal.Normalized()
	ownerCatalog = ownerCatalog.Normalized()
	current.Revision = strings.TrimSpace(current.Revision)
	current.TargetRefs = normalizeStrings(current.TargetRefs)
	contractRef = strings.TrimSpace(contractRef)
	if err := manifest.Validate(); err != nil {
		return RuntimeProspectiveWorkspaceManifestDelta{}, err
	}
	if err := proposal.Validate(); err != nil {
		return RuntimeProspectiveWorkspaceManifestDelta{}, err
	}
	if err := ownerCatalog.Validate(); err != nil {
		return RuntimeProspectiveWorkspaceManifestDelta{}, err
	}
	acquisition, found := runtimeSemanticOwnerFactAcquisitionForCoverage(manifest, proposal.CoverageWorkItemID)
	if !found || acquisition.AcquisitionTargetPolicy != RuntimeOperationTargetPolicyProspectiveOwnerTarget {
		return RuntimeProspectiveWorkspaceManifestDelta{}, fmt.Errorf("runtime prospective workspace manifest delta requires a sealed prospective acquisition")
	}
	if err := validateRuntimeSemanticOwnerFactAcquisition(RuntimeSemanticCWIProofSet{
		CoverageWorkItemID: proposal.CoverageWorkItemID,
		Proposals:          []RuntimeSemanticProofProposal{proposal},
	}, &acquisition, 0); err != nil {
		return RuntimeProspectiveWorkspaceManifestDelta{}, err
	}
	if proposal.OwnerModuleID != RuntimeModuleFile || ownerCatalog.OwnerModuleID != RuntimeModuleFile ||
		proposal.Target.Policy != RuntimeOperationTargetPolicyProspectiveOwnerTarget ||
		ownerCatalog.SnapshotID == "" || ownerCatalog.ObservedRevision != current.Revision ||
		proposal.Target.ProspectiveTargetRef == "" || containsString(current.TargetRefs, proposal.Target.ProspectiveTargetRef) ||
		contractRef == "" || contractRef != acquisition.ProspectiveSatisfactionContractRef {
		return RuntimeProspectiveWorkspaceManifestDelta{}, fmt.Errorf("runtime prospective workspace manifest delta input does not match a fresh File prospective proposal")
	}
	value := RuntimeProspectiveWorkspaceManifestDelta{
		SchemaVersion: RuntimeProspectiveWorkspaceManifestDeltaSchemaCurrent,
		WorkspaceID:   manifest.WorkspaceID, RunID: manifest.RunID, ManifestHash: manifest.Hash,
		DefinitionSeedHash: manifest.DefinitionSeedHash, CoverageWorkItemID: proposal.CoverageWorkItemID,
		ProposalHash: StableRuntimeSemanticProofProposalHash(proposal), OwnerModuleID: RuntimeModuleFile,
		CatalogSnapshotID: ownerCatalog.SnapshotID, ProspectiveSatisfactionContractRef: contractRef,
		BaseWorkspaceManifestRevision: current.Revision, ProspectiveTargetRef: proposal.Target.ProspectiveTargetRef,
		ResultingTargetRefs: append(append([]string(nil), current.TargetRefs...), proposal.Target.ProspectiveTargetRef),
	}.Normalized()
	return value, value.Validate()
}

func StableRuntimeProspectiveWorkspaceManifestDeltaHash(value RuntimeProspectiveWorkspaceManifestDelta) string {
	value = value.Normalized()
	h := sha256.New()
	writeHashString(h, RuntimeProspectiveWorkspaceManifestDeltaSchemaCurrent)
	writeHashString(h, string(value.WorkspaceID))
	writeHashString(h, string(value.RunID))
	writeHashString(h, value.ManifestHash)
	writeHashString(h, value.DefinitionSeedHash)
	writeHashString(h, string(value.CoverageWorkItemID))
	writeHashString(h, value.ProposalHash)
	writeHashString(h, value.OwnerModuleID)
	writeHashString(h, value.CatalogSnapshotID)
	writeHashString(h, value.ProspectiveSatisfactionContractRef)
	writeHashString(h, value.BaseWorkspaceManifestRevision)
	writeHashString(h, value.ProspectiveTargetRef)
	writeHashStrings(h, value.ResultingTargetRefs)
	return RuntimeProspectiveWorkspaceManifestDeltaHashPrefix + hex.EncodeToString(h.Sum(nil))[:20]
}

type RuntimeOwnerFactProspectiveSatisfactionOutcome string

const (
	RuntimeOwnerFactProspectiveWouldSatisfy    RuntimeOwnerFactProspectiveSatisfactionOutcome = "would_satisfy"
	RuntimeOwnerFactProspectiveWouldNotSatisfy RuntimeOwnerFactProspectiveSatisfactionOutcome = "would_not_satisfy"
)

// RuntimeOwnerFactProspectiveSatisfaction is the required-fact owner's exact
// answer for one File-authored delta and one activation-sealed contract.
type RuntimeOwnerFactProspectiveSatisfaction struct {
	SchemaVersion                         string
	WorkspaceID                           common.WorkspaceID
	RunID                                 common.RunID
	ManifestHash                          string
	DefinitionSeedHash                    string
	CoverageWorkItemID                    common.CoverageWorkItemID
	ProposalHash                          string
	Requirement                           RuntimeOwnerFactRequirement
	ProspectiveSatisfactionContractRef    string
	OwnerModuleID                         string
	CatalogSnapshotID                     string
	CatalogObservationID                  string
	ObservedRevision                      string
	ProspectiveWorkspaceManifestDeltaHash string
	Outcome                               RuntimeOwnerFactProspectiveSatisfactionOutcome
	ProspectiveValidationRecipeRefs       []string
	Reason                                string
}

func (s RuntimeOwnerFactProspectiveSatisfaction) Normalized() RuntimeOwnerFactProspectiveSatisfaction {
	s.SchemaVersion = strings.TrimSpace(s.SchemaVersion)
	s.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(s.WorkspaceID)))
	s.RunID = common.RunID(strings.TrimSpace(string(s.RunID)))
	s.ManifestHash = strings.TrimSpace(s.ManifestHash)
	s.DefinitionSeedHash = strings.TrimSpace(s.DefinitionSeedHash)
	s.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(s.CoverageWorkItemID)))
	s.ProposalHash = strings.TrimSpace(s.ProposalHash)
	s.Requirement = s.Requirement.Normalized()
	s.ProspectiveSatisfactionContractRef = strings.TrimSpace(s.ProspectiveSatisfactionContractRef)
	s.OwnerModuleID = strings.TrimSpace(s.OwnerModuleID)
	s.CatalogSnapshotID = strings.TrimSpace(s.CatalogSnapshotID)
	s.CatalogObservationID = strings.TrimSpace(s.CatalogObservationID)
	s.ObservedRevision = strings.TrimSpace(s.ObservedRevision)
	s.ProspectiveWorkspaceManifestDeltaHash = strings.TrimSpace(s.ProspectiveWorkspaceManifestDeltaHash)
	s.Outcome = RuntimeOwnerFactProspectiveSatisfactionOutcome(strings.TrimSpace(string(s.Outcome)))
	s.ProspectiveValidationRecipeRefs = normalizeStrings(s.ProspectiveValidationRecipeRefs)
	s.Reason = strings.TrimSpace(s.Reason)
	return s
}

func (s RuntimeOwnerFactProspectiveSatisfaction) Clone() RuntimeOwnerFactProspectiveSatisfaction {
	s = s.Normalized()
	s.ProspectiveValidationRecipeRefs = append([]string(nil), s.ProspectiveValidationRecipeRefs...)
	return s
}

func (s RuntimeOwnerFactProspectiveSatisfaction) Validate() error {
	s = s.Normalized()
	if s.SchemaVersion != RuntimeOwnerFactProspectiveSatisfactionSchemaCurrent || s.WorkspaceID == "" || s.RunID == "" ||
		s.ManifestHash == "" || s.DefinitionSeedHash == "" || s.CoverageWorkItemID == "" || s.ProposalHash == "" ||
		s.ProspectiveSatisfactionContractRef == "" || s.OwnerModuleID == "" || s.CatalogSnapshotID == "" ||
		s.CatalogObservationID == "" || s.ObservedRevision == "" || s.ProspectiveWorkspaceManifestDeltaHash == "" {
		return fmt.Errorf("runtime owner-fact prospective satisfaction requires scope, proposal, contract, owner, catalog and delta identity")
	}
	if err := s.Requirement.Validate(); err != nil {
		return err
	}
	if s.OwnerModuleID != RuntimeModuleShell || s.Requirement.OwnerModuleID != RuntimeModuleShell || s.OwnerModuleID != s.Requirement.OwnerModuleID {
		return fmt.Errorf("runtime owner-fact prospective satisfaction must be authored by the required Shell owner")
	}
	switch s.Outcome {
	case RuntimeOwnerFactProspectiveWouldSatisfy:
		if len(s.ProspectiveValidationRecipeRefs) == 0 || s.Reason != "" {
			return fmt.Errorf("would-satisfy owner fact requires exact validation recipes and no rejection reason")
		}
	case RuntimeOwnerFactProspectiveWouldNotSatisfy:
		if len(s.ProspectiveValidationRecipeRefs) != 0 || s.Reason == "" {
			return fmt.Errorf("would-not-satisfy owner fact requires only a factual reason")
		}
	default:
		return fmt.Errorf("runtime owner-fact prospective satisfaction has invalid outcome %q", s.Outcome)
	}
	return nil
}

func StableRuntimeOwnerFactProspectiveSatisfactionHash(value RuntimeOwnerFactProspectiveSatisfaction) string {
	value = value.Normalized()
	h := sha256.New()
	writeHashString(h, RuntimeOwnerFactProspectiveSatisfactionSchemaCurrent)
	writeHashString(h, string(value.WorkspaceID))
	writeHashString(h, string(value.RunID))
	writeHashString(h, value.ManifestHash)
	writeHashString(h, value.DefinitionSeedHash)
	writeHashString(h, string(value.CoverageWorkItemID))
	writeHashString(h, value.ProposalHash)
	writeHashString(h, RuntimeOwnerFactRequirementRef(value.Requirement))
	writeHashString(h, value.ProspectiveSatisfactionContractRef)
	writeHashString(h, value.OwnerModuleID)
	writeHashString(h, value.CatalogSnapshotID)
	writeHashString(h, value.CatalogObservationID)
	writeHashString(h, value.ObservedRevision)
	writeHashString(h, value.ProspectiveWorkspaceManifestDeltaHash)
	writeHashString(h, string(value.Outcome))
	writeHashStrings(h, value.ProspectiveValidationRecipeRefs)
	writeHashString(h, value.Reason)
	return RuntimeOwnerFactProspectiveSatisfactionHashPrefix + hex.EncodeToString(h.Sum(nil))[:20]
}

// RuntimeOwnerFactProspectiveSatisfactionRequest is the concrete typed value
// that Orchestrator may route to the required-fact owner. It carries only
// sealed scope plus File/owner facts and cannot authorize an operation.
type RuntimeOwnerFactProspectiveSatisfactionRequest struct {
	WorkspaceID                        common.WorkspaceID
	RunID                              common.RunID
	FeasibilityBatchID                 string
	Manifest                           RuntimeSemanticPlanManifest
	Proposal                           RuntimeSemanticProofProposal
	Requirement                        RuntimeOwnerFactRequirement
	ProspectiveSatisfactionContractRef string
	OwnerCatalog                       RuntimeOperationAdmissibility
	CatalogObservation                 *RuntimePrerequisiteObservation
	ValidationRecipes                  []RuntimeValidationRecipe
	ProspectiveWorkspaceManifestDelta  RuntimeProspectiveWorkspaceManifestDelta
}

func (r RuntimeOwnerFactProspectiveSatisfactionRequest) Normalized() RuntimeOwnerFactProspectiveSatisfactionRequest {
	r.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(r.WorkspaceID)))
	r.RunID = common.RunID(strings.TrimSpace(string(r.RunID)))
	r.FeasibilityBatchID = strings.TrimSpace(r.FeasibilityBatchID)
	r.Manifest = r.Manifest.Normalized()
	r.Proposal = r.Proposal.Normalized()
	r.Requirement = r.Requirement.Normalized()
	r.ProspectiveSatisfactionContractRef = strings.TrimSpace(r.ProspectiveSatisfactionContractRef)
	r.OwnerCatalog = r.OwnerCatalog.Normalized()
	if r.CatalogObservation != nil {
		value := r.CatalogObservation.Normalized()
		r.CatalogObservation = &value
	}
	r.ValidationRecipes = NormalizeRuntimeValidationRecipes(r.ValidationRecipes)
	r.ProspectiveWorkspaceManifestDelta = r.ProspectiveWorkspaceManifestDelta.Normalized()
	return r
}

func (r RuntimeOwnerFactProspectiveSatisfactionRequest) Clone() RuntimeOwnerFactProspectiveSatisfactionRequest {
	r = r.Normalized()
	r.Manifest = r.Manifest.Clone()
	r.Proposal = r.Proposal.Clone()
	r.ValidationRecipes = cloneRuntimeValidationRecipesForProspectiveSatisfaction(r.ValidationRecipes)
	r.ProspectiveWorkspaceManifestDelta = r.ProspectiveWorkspaceManifestDelta.Clone()
	if r.CatalogObservation != nil {
		value := r.CatalogObservation.Normalized()
		value.Dependencies = append([]RuntimeFreshnessDependency(nil), value.Dependencies...)
		r.CatalogObservation = &value
	}
	return r
}

func (r RuntimeOwnerFactProspectiveSatisfactionRequest) Validate() error {
	rawRecipes := append([]RuntimeValidationRecipe(nil), r.ValidationRecipes...)
	r = r.Normalized()
	if r.WorkspaceID == "" || r.RunID == "" || r.FeasibilityBatchID == "" ||
		r.ProspectiveSatisfactionContractRef == "" || r.CatalogObservation == nil || len(r.ValidationRecipes) == 0 ||
		len(rawRecipes) != len(r.ValidationRecipes) {
		return fmt.Errorf("runtime owner-fact prospective satisfaction request requires scope, contract, catalog observation and validation recipes")
	}
	if err := r.Manifest.Validate(); err != nil {
		return err
	}
	if err := r.Proposal.Validate(); err != nil {
		return err
	}
	if err := r.Requirement.Validate(); err != nil {
		return err
	}
	if r.Manifest.WorkspaceID != r.WorkspaceID || r.Manifest.RunID != r.RunID {
		return fmt.Errorf("runtime owner-fact prospective satisfaction request scope does not match manifest")
	}
	acquisition, found := runtimeSemanticOwnerFactAcquisitionForCoverage(r.Manifest, r.Proposal.CoverageWorkItemID)
	if !found || acquisition.AcquisitionTargetPolicy != RuntimeOperationTargetPolicyProspectiveOwnerTarget ||
		acquisition.Requirement != r.Requirement || acquisition.ProspectiveSatisfactionContractRef != r.ProspectiveSatisfactionContractRef {
		return fmt.Errorf("runtime owner-fact prospective satisfaction request does not match sealed acquisition")
	}
	if err := validateRuntimeSemanticOwnerFactAcquisition(RuntimeSemanticCWIProofSet{
		CoverageWorkItemID: r.Proposal.CoverageWorkItemID, Proposals: []RuntimeSemanticProofProposal{r.Proposal},
	}, &acquisition, 0); err != nil {
		return err
	}
	if err := r.OwnerCatalog.Validate(); err != nil {
		return err
	}
	if r.OwnerCatalog.OwnerModuleID != RuntimeModuleShell || r.Requirement.OwnerModuleID != RuntimeModuleShell ||
		r.OwnerCatalog.OwnerModuleID != r.Requirement.OwnerModuleID || r.OwnerCatalog.SnapshotID == "" {
		return fmt.Errorf("runtime owner-fact prospective satisfaction request catalog does not match required Shell owner")
	}
	sealedDependency, found := runtimeSemanticOwnerFactDependencyCatalogFor(r.Manifest, r.Requirement.OwnerModuleID)
	if !found || sealedDependency.SnapshotID != r.OwnerCatalog.SnapshotID || sealedDependency.PrerequisiteID != r.OwnerCatalog.PrerequisiteID {
		return fmt.Errorf("runtime owner-fact prospective satisfaction request catalog is outside the sealed dependency catalog")
	}
	observation := r.CatalogObservation.Normalized()
	if err := observation.Validate(); err != nil {
		return err
	}
	if observation.Kind != PrerequisiteKindOperationAdmissibility || observation.PrerequisiteID != r.OwnerCatalog.PrerequisiteID ||
		observation.OwnerModuleID != r.OwnerCatalog.OwnerModuleID || observation.ContentHash != r.OwnerCatalog.SnapshotID {
		return fmt.Errorf("runtime owner-fact prospective satisfaction catalog observation does not match catalog")
	}
	if err := r.ProspectiveWorkspaceManifestDelta.Validate(); err != nil {
		return err
	}
	if err := validateRuntimeProspectiveWorkspaceManifestDeltaForProposal(r.ProspectiveWorkspaceManifestDelta, r.Manifest, r.Proposal, acquisition); err != nil {
		return err
	}
	if !runtimeObservationDependsOnWorkspaceTreeRevision(observation, r.ProspectiveWorkspaceManifestDelta.BaseWorkspaceManifestRevision) {
		return fmt.Errorf("runtime owner-fact prospective satisfaction catalog observation is not bound to the File manifest delta base revision")
	}
	if !runtimeObservationDependsOnOwnerCatalogRevision(observation, r.OwnerCatalog.OwnerModuleID, r.OwnerCatalog.ObservedRevision) {
		return fmt.Errorf("runtime owner-fact prospective satisfaction catalog observation is not bound to the required-owner policy revision")
	}
	contractRecipes := runtimeProspectiveSatisfactionContractRecipeRefs(r.OwnerCatalog, r.Requirement, r.ProspectiveSatisfactionContractRef)
	if len(contractRecipes) == 0 || !runtimeValidationRecipesContainAll(r.ValidationRecipes, contractRecipes) {
		return fmt.Errorf("%w: runtime owner-fact prospective satisfaction contract is missing an activation-sealed validation recipe", common.ErrConflict)
	}
	return nil
}

func StableRuntimeOwnerFactProspectiveSatisfactionRequestHash(value RuntimeOwnerFactProspectiveSatisfactionRequest) string {
	value = value.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-owner-fact-prospective-satisfaction-request")
	writeHashString(h, string(value.WorkspaceID))
	writeHashString(h, string(value.RunID))
	writeHashString(h, value.FeasibilityBatchID)
	writeHashString(h, value.Manifest.Hash)
	writeHashString(h, StableRuntimeSemanticProofProposalHash(value.Proposal))
	writeHashString(h, RuntimeOwnerFactRequirementRef(value.Requirement))
	writeHashString(h, value.ProspectiveSatisfactionContractRef)
	writeHashString(h, value.OwnerCatalog.SnapshotID)
	if value.CatalogObservation == nil {
		writeHashString(h, "no-catalog-observation")
	} else {
		writeHashString(h, value.CatalogObservation.ObservationID)
	}
	for _, recipe := range value.ValidationRecipes {
		writeHashString(h, recipe.ID)
		writeHashString(h, recipe.Command)
		writeHashStrings(h, recipe.Args)
		writeHashString(h, fmt.Sprintf("%d", recipe.TimeoutMillis))
	}
	writeHashString(h, StableRuntimeProspectiveWorkspaceManifestDeltaHash(value.ProspectiveWorkspaceManifestDelta))
	return RuntimeOwnerFactProspectiveSatisfactionRequestHashPrefix + hex.EncodeToString(h.Sum(nil))[:20]
}

type RuntimeOwnerFactProspectiveSatisfactionObservation struct {
	FeasibilityBatchID string
	RequestHash        string
	Satisfaction       *RuntimeOwnerFactProspectiveSatisfaction
	Stale              *RuntimePrerequisiteStale
}

func (o RuntimeOwnerFactProspectiveSatisfactionObservation) Normalized() RuntimeOwnerFactProspectiveSatisfactionObservation {
	o.FeasibilityBatchID = strings.TrimSpace(o.FeasibilityBatchID)
	o.RequestHash = strings.TrimSpace(o.RequestHash)
	if o.Satisfaction != nil {
		value := o.Satisfaction.Normalized()
		o.Satisfaction = &value
	}
	if o.Stale != nil {
		value := o.Stale.Normalized()
		o.Stale = &value
	}
	return o
}

func (o RuntimeOwnerFactProspectiveSatisfactionObservation) Clone() RuntimeOwnerFactProspectiveSatisfactionObservation {
	o = o.Normalized()
	if o.Satisfaction != nil {
		value := o.Satisfaction.Clone()
		o.Satisfaction = &value
	}
	if o.Stale != nil {
		value := o.Stale.Normalized()
		o.Stale = &value
	}
	return o
}

func (o RuntimeOwnerFactProspectiveSatisfactionObservation) Validate() error {
	o = o.Normalized()
	if o.FeasibilityBatchID == "" || o.RequestHash == "" {
		return fmt.Errorf("runtime owner-fact prospective satisfaction observation requires batch and request identity")
	}
	if (o.Satisfaction == nil) == (o.Stale == nil) {
		return fmt.Errorf("runtime owner-fact prospective satisfaction observation requires exactly one satisfaction or stale result")
	}
	if o.Satisfaction != nil {
		return o.Satisfaction.Validate()
	}
	return o.Stale.Validate()
}

func StableRuntimeOwnerFactProspectiveSatisfactionObservationHash(value RuntimeOwnerFactProspectiveSatisfactionObservation) string {
	value = value.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-owner-fact-prospective-satisfaction-observation")
	writeHashString(h, value.FeasibilityBatchID)
	writeHashString(h, value.RequestHash)
	if value.Satisfaction == nil {
		writeHashString(h, "no-satisfaction")
	} else {
		writeHashString(h, StableRuntimeOwnerFactProspectiveSatisfactionHash(*value.Satisfaction))
	}
	if value.Stale == nil {
		writeHashString(h, "no-stale")
	} else {
		writeHashString(h, StableRuntimePrerequisiteStaleSignature(*value.Stale))
		writeHashString(h, value.Stale.Reason)
	}
	return RuntimeOwnerFactProspectiveSatisfactionObservationHashPrefix + hex.EncodeToString(h.Sum(nil))[:20]
}

func (o RuntimeOwnerFactProspectiveSatisfactionObservation) ValidateForRequest(request RuntimeOwnerFactProspectiveSatisfactionRequest) error {
	o = o.Normalized()
	request = request.Normalized()
	if err := o.Validate(); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if o.FeasibilityBatchID != request.FeasibilityBatchID || o.RequestHash != StableRuntimeOwnerFactProspectiveSatisfactionRequestHash(request) {
		return fmt.Errorf("runtime owner-fact prospective satisfaction observation request identity mismatch")
	}
	if o.Satisfaction != nil {
		return validateRuntimeOwnerFactProspectiveSatisfactionForRequest(*o.Satisfaction, request)
	}
	return validateRuntimeOwnerFactProspectiveSatisfactionStaleForRequest(*o.Stale, request)
}

func NewRuntimeOwnerFactProspectiveSatisfactionObservation(request RuntimeOwnerFactProspectiveSatisfactionRequest, outcome RuntimeOwnerFactProspectiveSatisfactionOutcome, recipeRefs []string, reason string) (RuntimeOwnerFactProspectiveSatisfactionObservation, error) {
	request = request.Normalized()
	if err := request.Validate(); err != nil {
		return RuntimeOwnerFactProspectiveSatisfactionObservation{}, err
	}
	value := RuntimeOwnerFactProspectiveSatisfaction{
		SchemaVersion: RuntimeOwnerFactProspectiveSatisfactionSchemaCurrent,
		WorkspaceID:   request.WorkspaceID, RunID: request.RunID, ManifestHash: request.Manifest.Hash,
		DefinitionSeedHash: request.Manifest.DefinitionSeedHash, CoverageWorkItemID: request.Proposal.CoverageWorkItemID,
		ProposalHash: StableRuntimeSemanticProofProposalHash(request.Proposal), Requirement: request.Requirement,
		ProspectiveSatisfactionContractRef: request.ProspectiveSatisfactionContractRef,
		OwnerModuleID:                      request.OwnerCatalog.OwnerModuleID, CatalogSnapshotID: request.OwnerCatalog.SnapshotID,
		CatalogObservationID: request.CatalogObservation.ObservationID, ObservedRevision: request.OwnerCatalog.ObservedRevision,
		ProspectiveWorkspaceManifestDeltaHash: StableRuntimeProspectiveWorkspaceManifestDeltaHash(request.ProspectiveWorkspaceManifestDelta),
		Outcome:                               outcome, ProspectiveValidationRecipeRefs: append([]string(nil), recipeRefs...), Reason: reason,
	}.Normalized()
	observation := RuntimeOwnerFactProspectiveSatisfactionObservation{
		FeasibilityBatchID: request.FeasibilityBatchID,
		RequestHash:        StableRuntimeOwnerFactProspectiveSatisfactionRequestHash(request),
		Satisfaction:       &value,
	}.Normalized()
	return observation, observation.ValidateForRequest(request)
}

// NewRuntimeOwnerFactProspectiveSatisfactionStaleObservation returns the
// required owner's typed read-only stale fact. It never chooses a refresh;
// only an Orchestrator-private bounded SM-2 event may consume it.
func NewRuntimeOwnerFactProspectiveSatisfactionStaleObservation(request RuntimeOwnerFactProspectiveSatisfactionRequest, observedRevision, reason string) (RuntimeOwnerFactProspectiveSatisfactionObservation, error) {
	request = request.Normalized()
	observedRevision = strings.TrimSpace(observedRevision)
	reason = strings.TrimSpace(reason)
	if err := request.Validate(); err != nil {
		return RuntimeOwnerFactProspectiveSatisfactionObservation{}, err
	}
	dependency, found := runtimeOwnerCatalogDependencyForProspectiveRequest(request)
	if !found || observedRevision == "" || observedRevision == dependency.Revision || reason == "" {
		return RuntimeOwnerFactProspectiveSatisfactionObservation{}, fmt.Errorf("runtime owner-fact prospective satisfaction stale result requires an exact changed owner revision")
	}
	stale := RuntimePrerequisiteStale{
		PrerequisiteID:   request.CatalogObservation.PrerequisiteID,
		ObservationID:    request.CatalogObservation.ObservationID,
		Domain:           dependency.Domain,
		OwnerModuleID:    dependency.OwnerModuleID,
		SubjectRef:       dependency.SubjectRef,
		ExpectedRevision: dependency.Revision,
		ObservedRevision: observedRevision,
		Reason:           reason,
	}.Normalized()
	observation := RuntimeOwnerFactProspectiveSatisfactionObservation{
		FeasibilityBatchID: request.FeasibilityBatchID,
		RequestHash:        StableRuntimeOwnerFactProspectiveSatisfactionRequestHash(request),
		Stale:              &stale,
	}.Normalized()
	return observation, observation.ValidateForRequest(request)
}

func validateRuntimeOwnerFactProspectiveSatisfactionForRequest(value RuntimeOwnerFactProspectiveSatisfaction, request RuntimeOwnerFactProspectiveSatisfactionRequest) error {
	value = value.Normalized()
	request = request.Normalized()
	if err := value.Validate(); err != nil {
		return err
	}
	if value.WorkspaceID != request.WorkspaceID || value.RunID != request.RunID || value.ManifestHash != request.Manifest.Hash ||
		value.DefinitionSeedHash != request.Manifest.DefinitionSeedHash || value.CoverageWorkItemID != request.Proposal.CoverageWorkItemID ||
		value.ProposalHash != StableRuntimeSemanticProofProposalHash(request.Proposal) || value.Requirement != request.Requirement ||
		value.ProspectiveSatisfactionContractRef != request.ProspectiveSatisfactionContractRef || value.OwnerModuleID != request.OwnerCatalog.OwnerModuleID ||
		value.CatalogSnapshotID != request.OwnerCatalog.SnapshotID || value.CatalogObservationID != request.CatalogObservation.ObservationID ||
		value.ObservedRevision != request.OwnerCatalog.ObservedRevision ||
		value.ProspectiveWorkspaceManifestDeltaHash != StableRuntimeProspectiveWorkspaceManifestDeltaHash(request.ProspectiveWorkspaceManifestDelta) {
		return fmt.Errorf("runtime owner-fact prospective satisfaction does not match request")
	}
	if value.Outcome == RuntimeOwnerFactProspectiveWouldSatisfy {
		allowed := runtimeProspectiveSatisfactionContractRecipeRefs(request.OwnerCatalog, request.Requirement, request.ProspectiveSatisfactionContractRef)
		if !sameStrings(value.ProspectiveValidationRecipeRefs, runtimeStringIntersection(value.ProspectiveValidationRecipeRefs, allowed)) ||
			!runtimeValidationRecipesContainAll(request.ValidationRecipes, value.ProspectiveValidationRecipeRefs) {
			return fmt.Errorf("runtime owner-fact prospective satisfaction recipes are outside sealed contract")
		}
	}
	return nil
}

func validateRuntimeOwnerFactProspectiveSatisfactionStaleForRequest(value RuntimePrerequisiteStale, request RuntimeOwnerFactProspectiveSatisfactionRequest) error {
	value = value.Normalized()
	request = request.Normalized()
	if err := value.Validate(); err != nil {
		return err
	}
	dependency, found := runtimeOwnerCatalogDependencyForProspectiveRequest(request)
	if !found || value.PrerequisiteID != request.CatalogObservation.PrerequisiteID ||
		value.ObservationID != request.CatalogObservation.ObservationID || value.Domain != dependency.Domain ||
		value.OwnerModuleID != dependency.OwnerModuleID || value.SubjectRef != dependency.SubjectRef ||
		value.ExpectedRevision != dependency.Revision || value.ObservedRevision == value.ExpectedRevision {
		return fmt.Errorf("runtime owner-fact prospective satisfaction stale result does not match request")
	}
	return nil
}

func validateRuntimeOwnerFactProspectiveSatisfactionObservationForProposal(value RuntimeOwnerFactProspectiveSatisfactionObservation, manifest RuntimeSemanticPlanManifest, proposal RuntimeSemanticProofProposal, acquisition RuntimeSemanticOwnerFactAcquisition, delta RuntimeProspectiveWorkspaceManifestDelta) error {
	value = value.Normalized()
	manifest = manifest.Normalized()
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Satisfaction != nil {
		return validateRuntimeOwnerFactProspectiveSatisfactionForProposal(*value.Satisfaction, manifest, proposal, acquisition, delta)
	}
	catalog, found := runtimeSemanticOwnerFactDependencyCatalogFor(manifest, acquisition.Requirement.OwnerModuleID)
	if !found || value.Stale.PrerequisiteID != catalog.PrerequisiteID || value.Stale.OwnerModuleID != catalog.OwnerModuleID ||
		value.Stale.ExpectedRevision != catalog.ObservedRevision {
		return fmt.Errorf("runtime owner-fact prospective satisfaction stale result does not match sealed dependency catalog")
	}
	return nil
}

func validateRuntimeOwnerFactProspectiveSatisfactionForProposal(value RuntimeOwnerFactProspectiveSatisfaction, manifest RuntimeSemanticPlanManifest, proposal RuntimeSemanticProofProposal, acquisition RuntimeSemanticOwnerFactAcquisition, delta RuntimeProspectiveWorkspaceManifestDelta) error {
	value = value.Normalized()
	manifest = manifest.Normalized()
	proposal = proposal.Normalized()
	acquisition = acquisition.Normalized()
	delta = delta.Normalized()
	if err := value.Validate(); err != nil {
		return err
	}
	if value.WorkspaceID != manifest.WorkspaceID || value.RunID != manifest.RunID || value.ManifestHash != manifest.Hash ||
		value.DefinitionSeedHash != manifest.DefinitionSeedHash || value.CoverageWorkItemID != proposal.CoverageWorkItemID ||
		value.ProposalHash != StableRuntimeSemanticProofProposalHash(proposal) || value.Requirement != acquisition.Requirement ||
		value.ProspectiveSatisfactionContractRef != acquisition.ProspectiveSatisfactionContractRef ||
		value.OwnerModuleID != acquisition.Requirement.OwnerModuleID ||
		value.ProspectiveWorkspaceManifestDeltaHash != StableRuntimeProspectiveWorkspaceManifestDeltaHash(delta) {
		return fmt.Errorf("runtime owner-fact prospective satisfaction does not match sealed proposal and File delta")
	}
	dependencyCatalog, found := runtimeSemanticOwnerFactDependencyCatalogFor(manifest, acquisition.Requirement.OwnerModuleID)
	if !found || value.CatalogSnapshotID != dependencyCatalog.SnapshotID || value.ObservedRevision != dependencyCatalog.ObservedRevision {
		return fmt.Errorf("runtime owner-fact prospective satisfaction does not match sealed dependency catalog")
	}
	return nil
}

func validateRuntimeProspectiveWorkspaceManifestDeltaForProposal(delta RuntimeProspectiveWorkspaceManifestDelta, manifest RuntimeSemanticPlanManifest, proposal RuntimeSemanticProofProposal, acquisition RuntimeSemanticOwnerFactAcquisition) error {
	delta = delta.Normalized()
	manifest = manifest.Normalized()
	proposal = proposal.Normalized()
	acquisition = acquisition.Normalized()
	if delta.WorkspaceID != manifest.WorkspaceID || delta.RunID != manifest.RunID || delta.ManifestHash != manifest.Hash ||
		delta.DefinitionSeedHash != manifest.DefinitionSeedHash || delta.CoverageWorkItemID != proposal.CoverageWorkItemID ||
		delta.ProposalHash != StableRuntimeSemanticProofProposalHash(proposal) || delta.OwnerModuleID != proposal.OwnerModuleID ||
		delta.CatalogSnapshotID != acquisition.OwnerCatalogSnapshotID ||
		delta.ProspectiveSatisfactionContractRef != acquisition.ProspectiveSatisfactionContractRef ||
		delta.BaseWorkspaceManifestRevision != acquisition.OwnerCatalogObservedRevision ||
		delta.ProspectiveTargetRef != proposal.Target.ProspectiveTargetRef ||
		delta.ProspectiveTargetRef != acquisition.ProspectiveTargetRef {
		return fmt.Errorf("runtime prospective workspace manifest delta does not match sealed prospective proposal")
	}
	return nil
}

func runtimeSemanticOwnerFactAcquisitionForCoverage(manifest RuntimeSemanticPlanManifest, coverageID common.CoverageWorkItemID) (RuntimeSemanticOwnerFactAcquisition, bool) {
	coverageID = common.CoverageWorkItemID(strings.TrimSpace(string(coverageID)))
	for _, obligation := range manifest.Normalized().ProofObligations {
		if obligation.CoverageWorkItemID == coverageID && obligation.OwnerFactAcquisition != nil {
			return obligation.OwnerFactAcquisition.Clone(), true
		}
	}
	return RuntimeSemanticOwnerFactAcquisition{}, false
}

func runtimeSemanticOwnerCatalogFor(manifest RuntimeSemanticPlanManifest, ownerModuleID string) (RuntimeOperationAdmissibility, bool) {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	for _, catalog := range manifest.Normalized().OwnerCatalogs {
		if catalog.OwnerModuleID == ownerModuleID {
			return catalog.Normalized(), true
		}
	}
	return RuntimeOperationAdmissibility{}, false
}

func runtimeSemanticOwnerFactDependencyCatalogFor(manifest RuntimeSemanticPlanManifest, ownerModuleID string) (RuntimeOperationAdmissibility, bool) {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	for _, catalog := range manifest.Normalized().OwnerFactDependencyCatalogs {
		if catalog.OwnerModuleID == ownerModuleID {
			return catalog.Normalized(), true
		}
	}
	return RuntimeOperationAdmissibility{}, false
}

func runtimeProspectiveSatisfactionContractRecipeRefs(catalog RuntimeOperationAdmissibility, requirement RuntimeOwnerFactRequirement, contractRef string) []string {
	catalog = catalog.Normalized()
	requirement = requirement.Normalized()
	contractRef = strings.TrimSpace(contractRef)
	ret := make([]string, 0)
	for _, surface := range catalog.ValidationSurfaces {
		surface = surface.Normalized()
		if surface.OwnerModuleID == requirement.OwnerModuleID && surface.OperationKind == requirement.OperationKind &&
			surface.Readiness == RuntimeValidationSurfacePrerequisiteRequired && surface.ProspectiveSatisfactionContractRef == contractRef &&
			surface.ProofApplicability == RuntimeValidationProofDirect && RuntimeEffectIsNonDurable(surface.EffectScope) &&
			runtimeValidationSurfaceHasExplicitReplayPolicy(surface.IdempotencyClass) {
			ret = append(ret, surface.ID)
		}
	}
	return normalizeStrings(ret)
}

func cloneRuntimeValidationRecipesForProspectiveSatisfaction(values []RuntimeValidationRecipe) []RuntimeValidationRecipe {
	values = NormalizeRuntimeValidationRecipes(values)
	ret := make([]RuntimeValidationRecipe, 0, len(values))
	for _, value := range values {
		value.Args = append([]string(nil), value.Args...)
		ret = append(ret, value)
	}
	return nilIfEmpty(ret)
}

func runtimeValidationRecipesContainAll(recipes []RuntimeValidationRecipe, refs []string) bool {
	for _, ref := range refs {
		if _, found := RuntimeValidationRecipeForID(recipes, ref); !found {
			return false
		}
	}
	return true
}

func runtimeObservationDependsOnOwnerCatalogRevision(observation RuntimePrerequisiteObservation, ownerModuleID, revision string) bool {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	revision = strings.TrimSpace(revision)
	for _, dependency := range observation.Normalized().Dependencies {
		if dependency.Domain == RuntimeResourceOwnerCatalog && dependency.OwnerModuleID == ownerModuleID && dependency.Revision == revision {
			return true
		}
	}
	return false
}

func runtimeOwnerCatalogDependencyForProspectiveRequest(request RuntimeOwnerFactProspectiveSatisfactionRequest) (RuntimeFreshnessDependency, bool) {
	request = request.Normalized()
	if request.CatalogObservation == nil {
		return RuntimeFreshnessDependency{}, false
	}
	for _, dependency := range request.CatalogObservation.Dependencies {
		dependency = dependency.Normalized()
		if dependency.Domain == RuntimeResourceOwnerCatalog && dependency.OwnerModuleID == request.OwnerCatalog.OwnerModuleID &&
			dependency.Revision == request.OwnerCatalog.ObservedRevision {
			return dependency, true
		}
	}
	return RuntimeFreshnessDependency{}, false
}

func runtimeStringIntersection(left, right []string) []string {
	ret := make([]string, 0)
	for _, value := range normalizeStrings(left) {
		if containsString(right, value) {
			ret = append(ret, value)
		}
	}
	return nilIfEmpty(ret)
}

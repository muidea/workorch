package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	RuntimeDefinitionSeedSchemaCurrent         = "workorch.runtime_definition_seed"
	RuntimeSemanticPlanManifestSchemaCurrent   = "workorch.runtime_semantic_plan_manifest"
	RuntimeSemanticPlanDocumentSchemaCurrent   = "workorch.runtime_semantic_plan_document"
	RuntimeSemanticFeasibilitySchemaCurrent    = "workorch.runtime_semantic_feasibility"
	RuntimeSemanticPlanningLedgerSchemaCurrent = "workorch.runtime_semantic_planning_ledger"
)

// RuntimeProviderDocumentKind selects one of the two mutually exclusive
// provider document contracts.  It is a wire discriminator, not an action or
// a state-machine transition.
type RuntimeProviderDocumentKind string

const (
	RuntimeProviderDocumentSemanticPlan RuntimeProviderDocumentKind = "semantic_plan"
	RuntimeProviderDocumentOperations   RuntimeProviderDocumentKind = "operations"
)

type RuntimeSemanticPlanStep string

const (
	RuntimeSemanticPlanStepCoverageOrder RuntimeSemanticPlanStep = "coverage_order"
	RuntimeSemanticPlanStepProofBatch    RuntimeSemanticPlanStep = "proof_batch"
)

// RuntimeDefinitionSeed is the only fresh-activation semantic input.  It
// preserves Goal/DoD-derived CWI identity and presentation grouping without
// pretending that application already owns target, recipe or binding facts.
type RuntimeDefinitionSeed struct {
	SchemaVersion      string
	Hash               string
	DefinitionRevision string
	PolicyHash         string
	CoverageItems      []RuntimeCoverageItem
	Slices             []RuntimeAcceptanceSlice
}

func CompileRuntimeDefinitionSeed(definition RuntimeDefinitionContext, activation RuntimeActivationContext, items []RuntimeCoverageItem, slices []RuntimeAcceptanceSlice) (RuntimeDefinitionSeed, error) {
	definition = definition.Normalized()
	activation = activation.Normalized()
	seed := RuntimeDefinitionSeed{
		SchemaVersion:      RuntimeDefinitionSeedSchemaCurrent,
		DefinitionRevision: definition.Revision,
		PolicyHash:         StableRuntimeCandidatePrefixHashWithExecutionContract(definition, activation.ExecutionContract),
		CoverageItems:      NormalizeRuntimeCoverageItems(items),
		Slices:             NormalizeRuntimeAcceptanceSlices(slices),
	}
	seed.Hash = StableRuntimeDefinitionSeedHash(seed)
	if err := seed.Validate(); err != nil {
		return RuntimeDefinitionSeed{}, err
	}
	return seed, nil
}

func (s RuntimeDefinitionSeed) Normalized() RuntimeDefinitionSeed {
	s.SchemaVersion = strings.TrimSpace(s.SchemaVersion)
	s.Hash = strings.TrimSpace(s.Hash)
	s.DefinitionRevision = strings.TrimSpace(s.DefinitionRevision)
	s.PolicyHash = strings.TrimSpace(s.PolicyHash)
	s.CoverageItems = NormalizeRuntimeCoverageItems(s.CoverageItems)
	s.Slices = NormalizeRuntimeAcceptanceSlices(s.Slices)
	return s
}

func (s RuntimeDefinitionSeed) Clone() RuntimeDefinitionSeed {
	s = s.Normalized()
	s.CoverageItems = append([]RuntimeCoverageItem(nil), s.CoverageItems...)
	for idx := range s.CoverageItems {
		s.CoverageItems[idx].ReopenDependencies = cloneRuntimeCoverageReopenDependencies(s.CoverageItems[idx].ReopenDependencies)
	}
	s.Slices = append([]RuntimeAcceptanceSlice(nil), s.Slices...)
	for idx := range s.Slices {
		s.Slices[idx] = s.Slices[idx].Normalized()
	}
	return s
}

func (s RuntimeDefinitionSeed) Validate() error {
	s = s.Normalized()
	if s.SchemaVersion != RuntimeDefinitionSeedSchemaCurrent || s.DefinitionRevision == "" || s.PolicyHash == "" || len(s.CoverageItems) == 0 || len(s.Slices) == 0 {
		return fmt.Errorf("runtime definition seed requires current schema, definition, policy, coverage and slices")
	}
	items := make(map[common.CoverageWorkItemID]RuntimeCoverageItem, len(s.CoverageItems))
	for _, item := range s.CoverageItems {
		if item.ID == "" || item.SliceID == "" || item.SourceScope == "" || item.SourceID == "" {
			return fmt.Errorf("runtime definition seed coverage item requires identity, source and slice")
		}
		if _, duplicate := items[item.ID]; duplicate {
			return fmt.Errorf("runtime definition seed duplicates coverage item %q", item.ID)
		}
		items[item.ID] = item
	}
	if err := ValidateRuntimeCoverageItemDependencies(s.CoverageItems); err != nil {
		return fmt.Errorf("runtime definition seed coverage dependencies: %w", err)
	}
	seenSlices := make(map[string]struct{}, len(s.Slices))
	seenCoverage := make(map[common.CoverageWorkItemID]struct{}, len(items))
	for _, slice := range s.Slices {
		if slice.ID == "" || len(slice.CoverageWorkItemIDs) == 0 {
			return fmt.Errorf("runtime definition seed slice requires identity and coverage")
		}
		if _, duplicate := seenSlices[slice.ID]; duplicate {
			return fmt.Errorf("runtime definition seed duplicates slice %q", slice.ID)
		}
		seenSlices[slice.ID] = struct{}{}
		for _, coverageID := range slice.CoverageWorkItemIDs {
			item, found := items[coverageID]
			if !found || item.SliceID != slice.ID {
				return fmt.Errorf("runtime definition seed slice %q references mismatched coverage item %q", slice.ID, coverageID)
			}
			if _, duplicate := seenCoverage[coverageID]; duplicate {
				return fmt.Errorf("runtime definition seed coverage item %q appears in multiple slices", coverageID)
			}
			seenCoverage[coverageID] = struct{}{}
		}
	}
	if len(seenCoverage) != len(items) {
		return fmt.Errorf("runtime definition seed slices do not cover every coverage item")
	}
	if s.Hash != StableRuntimeDefinitionSeedHash(s) {
		return fmt.Errorf("runtime definition seed hash does not match contents")
	}
	return nil
}

func StableRuntimeDefinitionSeedHash(seed RuntimeDefinitionSeed) string {
	seed = seed.Normalized()
	h := sha256.New()
	writeHashString(h, RuntimeDefinitionSeedSchemaCurrent)
	writeHashString(h, seed.DefinitionRevision)
	writeHashString(h, seed.PolicyHash)
	writeRuntimeSemanticCoverageHash(h, seed.CoverageItems, seed.Slices)
	return "runtime-definition-seed-" + hex.EncodeToString(h.Sum(nil))[:20]
}

// RuntimeSemanticPlanManifest bounds one semantic-plan provider request.  It
// contains only owner-neutral catalogs already persisted for the transaction.
type RuntimeSemanticPlanManifest struct {
	SchemaVersion             string
	Hash                      string
	WorkspaceID               common.WorkspaceID
	RunID                     common.RunID
	DefinitionRevision        string
	DefinitionSeedHash        string
	OperationContractRevision string
	CoverageItems             []RuntimeCoverageItem
	AllowedOwnerModuleIDs     []string
	AllowedOperationKinds     []string
	AllowedTargetPolicies     []RuntimeOperationTargetPolicy
	// OwnerCatalogs are the bounded owner-authored vocabulary observed before
	// provider I/O. They prevent the semantic planner from guessing targets,
	// selectors, constraints, recipes, or catalog revisions and then learning
	// those facts only through a later rejection.
	OwnerCatalogs []RuntimeOperationAdmissibility
}

// CompileRuntimeSemanticPlanManifest compiles the provider-visible semantic
// vocabulary exclusively from activation-sealed definition capabilities. It
// is deliberately pure: owner resource availability is established later by
// typed feasibility observations and is never inferred here.
func CompileRuntimeSemanticPlanManifest(workspaceID common.WorkspaceID, runID common.RunID, definition RuntimeDefinitionContext, seed RuntimeDefinitionSeed, execution RuntimeExecutionContract, admissibilities []RuntimeOperationAdmissibility) (RuntimeSemanticPlanManifest, error) {
	definition = definition.Normalized()
	seed = seed.Clone()
	execution = execution.Normalized()
	if workspaceID == "" || runID == "" {
		return RuntimeSemanticPlanManifest{}, common.ErrInvalidArgument
	}
	if err := definition.Validate(); err != nil {
		return RuntimeSemanticPlanManifest{}, err
	}
	if err := seed.Validate(); err != nil {
		return RuntimeSemanticPlanManifest{}, err
	}
	if err := execution.Validate(); err != nil {
		return RuntimeSemanticPlanManifest{}, err
	}
	if seed.DefinitionRevision != definition.Revision || execution.OperationContractRevision == "" {
		return RuntimeSemanticPlanManifest{}, fmt.Errorf("runtime semantic manifest inputs have mismatched definition or operation contract identity")
	}

	canonicalOwners := map[string]struct{}{
		RuntimeModuleFile: {}, RuntimeModuleShell: {}, RuntimeModuleMCP: {},
		RuntimeModuleGit: {}, RuntimeModuleWeb: {}, RuntimeModuleSSH: {},
	}
	owners := make([]string, 0)
	kinds := make([]string, 0)
	ownerSeen := map[string]struct{}{}
	kindSeen := map[string]struct{}{}
	allowsProspectiveTarget := false
	for _, raw := range definition.CapabilityContract {
		kind := CanonicalRuntimeBlockKind(raw)
		descriptor, found := RuntimeOperationDescriptorForKind(kind)
		if !found {
			return RuntimeSemanticPlanManifest{}, fmt.Errorf("runtime semantic capability %q has no operation descriptor", raw)
		}
		if _, canonical := canonicalOwners[descriptor.OwnerModuleID]; !canonical {
			continue
		}
		if _, duplicate := kindSeen[kind]; !duplicate {
			kindSeen[kind] = struct{}{}
			kinds = append(kinds, kind)
		}
		if _, duplicate := ownerSeen[descriptor.OwnerModuleID]; !duplicate {
			ownerSeen[descriptor.OwnerModuleID] = struct{}{}
			owners = append(owners, descriptor.OwnerModuleID)
		}
		allowsProspectiveTarget = allowsProspectiveTarget || descriptor.AllowsNewTarget
	}
	if len(kinds) == 0 {
		return RuntimeSemanticPlanManifest{}, fmt.Errorf("runtime semantic manifest has no activation-sealed capability-owner operations")
	}
	sort.Strings(owners)
	sort.Strings(kinds)
	catalogs := NormalizeRuntimeOperationAdmissibilities(admissibilities)
	catalogByOwner := make(map[string]RuntimeOperationAdmissibility, len(catalogs))
	for _, catalog := range catalogs {
		if err := catalog.Validate(); err != nil {
			return RuntimeSemanticPlanManifest{}, err
		}
		if _, allowed := ownerSeen[catalog.OwnerModuleID]; !allowed {
			continue
		}
		catalogByOwner[catalog.OwnerModuleID] = catalog
	}
	sealedCatalogs := make([]RuntimeOperationAdmissibility, 0, len(owners))
	for _, owner := range owners {
		catalog, found := catalogByOwner[owner]
		if !found {
			return RuntimeSemanticPlanManifest{}, fmt.Errorf("runtime semantic manifest owner %q has no sealed admissibility catalog", owner)
		}
		for _, kind := range kinds {
			descriptor, _ := RuntimeOperationDescriptorForKind(kind)
			if descriptor.OwnerModuleID == owner && !runtimeOperationKindsContain(catalog.OperationKinds, kind) {
				return RuntimeSemanticPlanManifest{}, fmt.Errorf("runtime semantic manifest operation %q is absent from owner %q catalog", kind, owner)
			}
		}
		sealedCatalogs = append(sealedCatalogs, catalog)
	}
	policies := []RuntimeOperationTargetPolicy{
		RuntimeOperationTargetPolicyExactTarget,
		RuntimeOperationTargetPolicyExistingOwnerScope,
	}
	if allowsProspectiveTarget {
		policies = append(policies, RuntimeOperationTargetPolicyProspectiveOwnerTarget)
	}
	manifest := RuntimeSemanticPlanManifest{
		SchemaVersion: RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   workspaceID, RunID: runID,
		DefinitionRevision: definition.Revision, DefinitionSeedHash: seed.Hash,
		OperationContractRevision: execution.OperationContractRevision,
		CoverageItems:             seed.CoverageItems,
		AllowedOwnerModuleIDs:     owners, AllowedOperationKinds: kinds,
		AllowedTargetPolicies: policies, OwnerCatalogs: sealedCatalogs,
	}
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	if err := manifest.Validate(); err != nil {
		return RuntimeSemanticPlanManifest{}, err
	}
	return manifest, nil
}

// RuntimeSemanticPlanRequestContext is the bounded provider context used
// before any operation focus or binding exists. It deliberately excludes
// RuntimeDecision, coverage facts, dispatch and owner observations.
type RuntimeSemanticPlanRequestContext struct {
	WorkspaceID              common.WorkspaceID
	RunID                    common.RunID
	Definition               RuntimeDefinitionContext
	DefinitionSeed           RuntimeDefinitionSeed
	Activation               RuntimeActivationContext
	ProviderCeiling          *ProviderCeiling
	PlanStep                 RuntimeSemanticPlanStep
	BatchID                  string
	BatchSequence            int
	BatchCoverageWorkItemIDs []common.CoverageWorkItemID
}

func (c RuntimeSemanticPlanRequestContext) Normalized() RuntimeSemanticPlanRequestContext {
	c.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(c.WorkspaceID)))
	c.RunID = common.RunID(strings.TrimSpace(string(c.RunID)))
	c.Definition = c.Definition.Normalized()
	c.DefinitionSeed = c.DefinitionSeed.Clone()
	c.BatchCoverageWorkItemIDs = append([]common.CoverageWorkItemID(nil), c.BatchCoverageWorkItemIDs...)
	c.Activation = c.Activation.Normalized()
	c.PlanStep = RuntimeSemanticPlanStep(strings.TrimSpace(string(c.PlanStep)))
	if c.PlanStep == "" {
		c.PlanStep = RuntimeSemanticPlanStepProofBatch
	}
	c.BatchID = strings.TrimSpace(c.BatchID)
	if c.BatchSequence < 0 {
		c.BatchSequence = 0
	}
	c.BatchCoverageWorkItemIDs = normalizeOrderedCoverageWorkItemIDs(c.BatchCoverageWorkItemIDs)
	if c.ProviderCeiling != nil {
		value := c.ProviderCeiling.Normalized()
		c.ProviderCeiling = &value
	}
	return c
}

func (c RuntimeSemanticPlanRequestContext) Clone() RuntimeSemanticPlanRequestContext {
	c = c.Normalized()
	c.Definition = c.Definition.Clone()
	c.DefinitionSeed = c.DefinitionSeed.Clone()
	if c.ProviderCeiling != nil {
		value := *c.ProviderCeiling
		c.ProviderCeiling = &value
	}
	return c
}

func (c RuntimeSemanticPlanRequestContext) Validate(manifest RuntimeSemanticPlanManifest) error {
	c = c.Normalized()
	manifest = manifest.Normalized()
	if c.WorkspaceID == "" || c.RunID == "" || c.ProviderCeiling == nil {
		return fmt.Errorf("runtime semantic request context requires scope and provider ceiling")
	}
	if err := c.Definition.Validate(); err != nil {
		return err
	}
	if err := c.DefinitionSeed.Validate(); err != nil {
		return err
	}
	if err := c.Activation.ProviderSelection.Target.Validate(); err != nil {
		return err
	}
	if err := c.ProviderCeiling.Validate(); err != nil {
		return err
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	if c.PlanStep != RuntimeSemanticPlanStepCoverageOrder && c.PlanStep != RuntimeSemanticPlanStepProofBatch {
		return fmt.Errorf("runtime semantic request context has invalid plan step %q", c.PlanStep)
	}
	if c.PlanStep == RuntimeSemanticPlanStepProofBatch {
		if c.BatchID == "" || len(c.BatchCoverageWorkItemIDs) == 0 {
			return fmt.Errorf("proof-batch request context requires batch identity and coverage")
		}
	}
	if manifest.WorkspaceID != c.WorkspaceID || manifest.RunID != c.RunID || manifest.DefinitionRevision != c.Definition.Revision ||
		manifest.DefinitionSeedHash != c.DefinitionSeed.Hash || c.DefinitionSeed.DefinitionRevision != c.Definition.Revision ||
		manifest.OperationContractRevision != c.Activation.ExecutionContract.OperationContractRevision || c.ProviderCeiling.Selection.Target != c.Activation.ProviderSelection.Target {
		return fmt.Errorf("runtime semantic request context identity does not match manifest")
	}
	return nil
}

func (m RuntimeSemanticPlanManifest) Normalized() RuntimeSemanticPlanManifest {
	m.SchemaVersion = strings.TrimSpace(m.SchemaVersion)
	m.Hash = strings.TrimSpace(m.Hash)
	m.WorkspaceID = common.WorkspaceID(strings.TrimSpace(string(m.WorkspaceID)))
	m.RunID = common.RunID(strings.TrimSpace(string(m.RunID)))
	m.DefinitionRevision = strings.TrimSpace(m.DefinitionRevision)
	m.DefinitionSeedHash = strings.TrimSpace(m.DefinitionSeedHash)
	m.OperationContractRevision = strings.TrimSpace(m.OperationContractRevision)
	m.CoverageItems = NormalizeRuntimeCoverageItems(m.CoverageItems)
	m.AllowedOperationKinds = canonicalizeOperationKinds(m.AllowedOperationKinds)
	m.AllowedOwnerModuleIDs = normalizeStrings(m.AllowedOwnerModuleIDs)
	m.AllowedTargetPolicies = normalizeRuntimeSemanticTargetPolicies(m.AllowedTargetPolicies)
	m.OwnerCatalogs = NormalizeRuntimeOperationAdmissibilities(m.OwnerCatalogs)
	return m
}

func (m RuntimeSemanticPlanManifest) Clone() RuntimeSemanticPlanManifest {
	m = m.Normalized()
	m.CoverageItems = append([]RuntimeCoverageItem(nil), m.CoverageItems...)
	m.AllowedOwnerModuleIDs = append([]string(nil), m.AllowedOwnerModuleIDs...)
	m.AllowedOperationKinds = append([]string(nil), m.AllowedOperationKinds...)
	m.AllowedTargetPolicies = append([]RuntimeOperationTargetPolicy(nil), m.AllowedTargetPolicies...)
	m.OwnerCatalogs = append([]RuntimeOperationAdmissibility(nil), m.OwnerCatalogs...)
	for idx := range m.OwnerCatalogs {
		m.OwnerCatalogs[idx].TargetRefs = append([]string(nil), m.OwnerCatalogs[idx].TargetRefs...)
		m.OwnerCatalogs[idx].SelectorRefs = append([]string(nil), m.OwnerCatalogs[idx].SelectorRefs...)
		m.OwnerCatalogs[idx].ConstraintRefs = append([]string(nil), m.OwnerCatalogs[idx].ConstraintRefs...)
		m.OwnerCatalogs[idx].ValidationRecipeRefs = append([]string(nil), m.OwnerCatalogs[idx].ValidationRecipeRefs...)
		m.OwnerCatalogs[idx].ValidationRecipes = append([]RuntimeValidationRecipe(nil), m.OwnerCatalogs[idx].ValidationRecipes...)
		for recipeIdx := range m.OwnerCatalogs[idx].ValidationRecipes {
			m.OwnerCatalogs[idx].ValidationRecipes[recipeIdx].Args = append([]string(nil), m.OwnerCatalogs[idx].ValidationRecipes[recipeIdx].Args...)
		}
	}
	return m
}

func (m RuntimeSemanticPlanManifest) Validate() error {
	m = m.Normalized()
	if m.SchemaVersion != RuntimeSemanticPlanManifestSchemaCurrent || m.WorkspaceID == "" || m.RunID == "" || m.DefinitionRevision == "" || m.DefinitionSeedHash == "" || m.OperationContractRevision == "" || len(m.CoverageItems) == 0 {
		return fmt.Errorf("runtime semantic manifest requires current schema, scope, definition, seed, operation contract and coverage")
	}
	if len(m.AllowedOwnerModuleIDs) == 0 || len(m.AllowedOperationKinds) == 0 || len(m.AllowedTargetPolicies) == 0 || len(m.OwnerCatalogs) == 0 {
		return fmt.Errorf("runtime semantic manifest requires bounded owners, operations and target policies")
	}
	owners := make(map[string]struct{}, len(m.AllowedOwnerModuleIDs))
	for _, owner := range m.AllowedOwnerModuleIDs {
		owners[owner] = struct{}{}
	}
	for _, kind := range m.AllowedOperationKinds {
		descriptor, found := RuntimeOperationDescriptorForKind(kind)
		if !found {
			return fmt.Errorf("runtime semantic manifest has unknown operation kind %q", kind)
		}
		if _, allowed := owners[descriptor.OwnerModuleID]; !allowed {
			return fmt.Errorf("runtime semantic manifest operation %q owner %q is not allowed", kind, descriptor.OwnerModuleID)
		}
	}
	catalogOwners := make(map[string]struct{}, len(m.OwnerCatalogs))
	for _, catalog := range m.OwnerCatalogs {
		if err := catalog.Validate(); err != nil {
			return err
		}
		if _, allowed := owners[catalog.OwnerModuleID]; !allowed {
			return fmt.Errorf("runtime semantic manifest catalog owner %q is not allowed", catalog.OwnerModuleID)
		}
		if _, duplicate := catalogOwners[catalog.OwnerModuleID]; duplicate {
			return fmt.Errorf("runtime semantic manifest duplicates catalog owner %q", catalog.OwnerModuleID)
		}
		catalogOwners[catalog.OwnerModuleID] = struct{}{}
	}
	if len(catalogOwners) != len(owners) {
		return fmt.Errorf("runtime semantic manifest does not seal every owner catalog")
	}
	if m.Hash != StableRuntimeSemanticPlanManifestHash(m) {
		return fmt.Errorf("runtime semantic manifest hash does not match contents")
	}
	return nil
}

func StableRuntimeSemanticPlanManifestHash(manifest RuntimeSemanticPlanManifest) string {
	manifest = manifest.Normalized()
	h := sha256.New()
	writeHashString(h, RuntimeSemanticPlanManifestSchemaCurrent)
	writeHashString(h, string(manifest.WorkspaceID))
	writeHashString(h, string(manifest.RunID))
	writeHashString(h, manifest.DefinitionRevision)
	writeHashString(h, manifest.DefinitionSeedHash)
	writeHashString(h, manifest.OperationContractRevision)
	writeRuntimeSemanticCoverageItemsHash(h, manifest.CoverageItems)
	writeHashStrings(h, manifest.AllowedOwnerModuleIDs)
	writeHashStrings(h, manifest.AllowedOperationKinds)
	for _, policy := range manifest.AllowedTargetPolicies {
		writeHashString(h, string(policy))
	}
	for _, catalog := range manifest.OwnerCatalogs {
		writeHashString(h, catalog.SnapshotID)
	}
	return "runtime-semantic-manifest-" + hex.EncodeToString(h.Sum(nil))[:20]
}

// RuntimeSemanticProofProposal is provider-authored and never authoritative.
// SM-2 may only turn it into a binding after matching owner feasibility.
type RuntimeSemanticProofProposal struct {
	CoverageWorkItemID    common.CoverageWorkItemID
	OwnerModuleID         string
	OperationKind         string
	OperationVariantID    string
	Target                RuntimeOperationTargetAuthorization
	RequiredEvidenceKinds []string
	RequiredEffectScopes  []RuntimeEffectScope
	ValidationRecipeID    string
	RequiredCheckRefs     []string
	MetricConstraints     []string
	ArtifactRefs          []string
	DeclarationRef        string
}

func (p RuntimeSemanticProofProposal) Normalized() RuntimeSemanticProofProposal {
	p.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(p.CoverageWorkItemID)))
	p.OwnerModuleID = strings.TrimSpace(p.OwnerModuleID)
	p.OperationKind = CanonicalRuntimeBlockKind(p.OperationKind)
	p.OperationVariantID = strings.TrimSpace(p.OperationVariantID)
	p.Target = p.Target.Normalized()
	p.RequiredEvidenceKinds = normalizeStrings(p.RequiredEvidenceKinds)
	p.RequiredEffectScopes = NormalizeRuntimeEffectScopes(p.RequiredEffectScopes)
	p.ValidationRecipeID = strings.TrimSpace(p.ValidationRecipeID)
	p.RequiredCheckRefs = normalizeStrings(p.RequiredCheckRefs)
	p.MetricConstraints = normalizeStrings(p.MetricConstraints)
	p.ArtifactRefs = normalizeStrings(p.ArtifactRefs)
	p.DeclarationRef = strings.TrimSpace(p.DeclarationRef)
	return p
}

func (p RuntimeSemanticProofProposal) Clone() RuntimeSemanticProofProposal {
	p = p.Normalized()
	p.Target = p.Target.Normalized()
	p.RequiredEvidenceKinds = append([]string(nil), p.RequiredEvidenceKinds...)
	p.RequiredEffectScopes = append([]RuntimeEffectScope(nil), p.RequiredEffectScopes...)
	p.RequiredCheckRefs = append([]string(nil), p.RequiredCheckRefs...)
	p.MetricConstraints = append([]string(nil), p.MetricConstraints...)
	p.ArtifactRefs = append([]string(nil), p.ArtifactRefs...)
	return p
}

func (p RuntimeSemanticProofProposal) Validate() error {
	p = p.Normalized()
	if p.CoverageWorkItemID == "" || p.OwnerModuleID == "" || p.OperationKind == "" || len(p.RequiredEvidenceKinds) == 0 || len(p.RequiredEffectScopes) == 0 || p.DeclarationRef == "" {
		return fmt.Errorf("runtime semantic proposal requires coverage, owner, operation, evidence, effect and declaration")
	}
	descriptor, found := RuntimeOperationDescriptorForKind(p.OperationKind)
	if !found || descriptor.OwnerModuleID != p.OwnerModuleID {
		return fmt.Errorf("runtime semantic proposal operation %q does not belong to owner %q", p.OperationKind, p.OwnerModuleID)
	}
	if err := validateRuntimeSemanticProposalTarget(p.Target); err != nil {
		return err
	}
	for _, kind := range p.RequiredEvidenceKinds {
		switch kind {
		case EvidenceKindStateChange:
			if !runtimeSemanticHasDurableEffect(p.RequiredEffectScopes) {
				return fmt.Errorf("runtime semantic state-change proposal requires a durable effect")
			}
		case EvidenceKindValidation:
			if p.ValidationRecipeID == "" || len(p.RequiredCheckRefs) == 0 {
				return fmt.Errorf("runtime semantic validation proposal requires recipe and checks")
			}
		default:
			return fmt.Errorf("runtime semantic proposal has unsupported evidence kind %q", kind)
		}
	}
	return nil
}

func StableRuntimeSemanticProofProposalHash(proposal RuntimeSemanticProofProposal) string {
	proposal = proposal.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-semantic-proof-proposal")
	writeHashString(h, string(proposal.CoverageWorkItemID))
	writeHashString(h, proposal.OwnerModuleID)
	writeHashString(h, proposal.OperationKind)
	writeHashString(h, proposal.OperationVariantID)
	writeRuntimeSemanticTargetHash(h, proposal.Target)
	writeHashStrings(h, proposal.RequiredEvidenceKinds)
	writeHashStrings(h, RuntimeEffectScopesAsStrings(proposal.RequiredEffectScopes))
	writeHashString(h, proposal.ValidationRecipeID)
	writeHashStrings(h, proposal.RequiredCheckRefs)
	writeHashStrings(h, proposal.MetricConstraints)
	writeHashStrings(h, proposal.ArtifactRefs)
	writeHashString(h, proposal.DeclarationRef)
	return "runtime-semantic-proposal-" + hex.EncodeToString(h.Sum(nil))[:20]
}

type RuntimeSemanticPlanDocument struct {
	SchemaVersion            string
	ManifestHash             string
	DefinitionSeedHash       string
	PlanStep                 RuntimeSemanticPlanStep
	BatchID                  string
	BatchSequence            int
	BatchCoverageWorkItemIDs []common.CoverageWorkItemID
	// CoverageOrder is the provider-authored planning step. It is not an
	// authority source: the orchestrator only accepts it after checking that it
	// is a complete permutation whose dependency edges point backwards.
	CoverageOrder []common.CoverageWorkItemID
	Proposals     []RuntimeSemanticProofProposal
}

func (d RuntimeSemanticPlanDocument) Normalized() RuntimeSemanticPlanDocument {
	d.SchemaVersion = strings.TrimSpace(d.SchemaVersion)
	d.ManifestHash = strings.TrimSpace(d.ManifestHash)
	d.DefinitionSeedHash = strings.TrimSpace(d.DefinitionSeedHash)
	d.PlanStep = RuntimeSemanticPlanStep(strings.TrimSpace(string(d.PlanStep)))
	if d.PlanStep == "" {
		d.PlanStep = RuntimeSemanticPlanStepProofBatch
	}
	d.BatchID = strings.TrimSpace(d.BatchID)
	if d.BatchSequence < 0 {
		d.BatchSequence = 0
	}
	d.BatchCoverageWorkItemIDs = normalizeOrderedCoverageWorkItemIDs(d.BatchCoverageWorkItemIDs)
	d.CoverageOrder = normalizeOrderedCoverageWorkItemIDs(d.CoverageOrder)
	d.Proposals = normalizeRuntimeSemanticProofProposals(d.Proposals)
	return d
}

func (d RuntimeSemanticPlanDocument) Clone() RuntimeSemanticPlanDocument {
	d = d.Normalized()
	d.Proposals = append([]RuntimeSemanticProofProposal(nil), d.Proposals...)
	for idx := range d.Proposals {
		d.Proposals[idx] = d.Proposals[idx].Clone()
	}
	return d
}

func (d RuntimeSemanticPlanDocument) ValidateAgainstManifest(manifest RuntimeSemanticPlanManifest) error {
	d = d.Normalized()
	manifest = manifest.Normalized()
	if err := manifest.Validate(); err != nil {
		return err
	}
	if d.SchemaVersion != RuntimeSemanticPlanDocumentSchemaCurrent || d.ManifestHash != manifest.Hash || d.DefinitionSeedHash != manifest.DefinitionSeedHash {
		return fmt.Errorf("runtime semantic document identity does not match manifest")
	}
	if len(d.CoverageOrder) > 0 {
		if err := validateRuntimeSemanticCoverageOrder(d.CoverageOrder, manifest.CoverageItems); err != nil {
			return err
		}
	}
	switch d.PlanStep {
	case RuntimeSemanticPlanStepCoverageOrder:
		if len(d.CoverageOrder) != len(manifest.CoverageItems) || len(d.Proposals) != 0 {
			return fmt.Errorf("coverage-order semantic plan requires full order and no proposals")
		}
	case RuntimeSemanticPlanStepProofBatch:
	default:
		return fmt.Errorf("runtime semantic plan has invalid step %q", d.PlanStep)
	}
	if d.BatchID != "" {
		if len(d.BatchCoverageWorkItemIDs) == 0 || d.BatchSequence < 0 {
			return fmt.Errorf("runtime semantic batch document requires sequence and coverage identity")
		}
		batchCoverage := make(map[common.CoverageWorkItemID]struct{}, len(d.BatchCoverageWorkItemIDs))
		for _, id := range d.BatchCoverageWorkItemIDs {
			batchCoverage[id] = struct{}{}
		}
		for _, proposal := range d.Proposals {
			if _, ok := batchCoverage[proposal.CoverageWorkItemID]; !ok {
				return fmt.Errorf("runtime semantic batch document proposal %q is outside batch", proposal.CoverageWorkItemID)
			}
		}
		if len(batchCoverage) != len(d.Proposals) {
			return fmt.Errorf("runtime semantic batch document coverage does not match proposals")
		}
	}
	items := make(map[common.CoverageWorkItemID]struct{}, len(manifest.CoverageItems))
	for _, item := range manifest.CoverageItems {
		items[item.ID] = struct{}{}
	}
	owners := runtimeSemanticStringSet(manifest.AllowedOwnerModuleIDs)
	kinds := runtimeSemanticStringSet(manifest.AllowedOperationKinds)
	catalogs := make(map[string]RuntimeOperationAdmissibility, len(manifest.OwnerCatalogs))
	for _, catalog := range manifest.OwnerCatalogs {
		catalogs[catalog.OwnerModuleID] = catalog
	}
	policies := make(map[RuntimeOperationTargetPolicy]struct{}, len(manifest.AllowedTargetPolicies))
	for _, policy := range manifest.AllowedTargetPolicies {
		policies[policy] = struct{}{}
	}
	seen := make(map[common.CoverageWorkItemID]struct{}, len(d.Proposals))
	for _, proposal := range d.Proposals {
		if err := proposal.Validate(); err != nil {
			return err
		}
		if _, found := items[proposal.CoverageWorkItemID]; !found {
			return fmt.Errorf("runtime semantic document proposal coverage %q is outside manifest", proposal.CoverageWorkItemID)
		}
		if _, duplicate := seen[proposal.CoverageWorkItemID]; duplicate {
			return fmt.Errorf("runtime semantic document duplicates coverage %q", proposal.CoverageWorkItemID)
		}
		seen[proposal.CoverageWorkItemID] = struct{}{}
		if _, allowed := owners[proposal.OwnerModuleID]; !allowed {
			return fmt.Errorf("runtime semantic document owner %q is outside manifest", proposal.OwnerModuleID)
		}
		if _, allowed := kinds[proposal.OperationKind]; !allowed {
			return fmt.Errorf("runtime semantic document operation %q is outside manifest", proposal.OperationKind)
		}
		if _, allowed := policies[proposal.Target.Policy]; !allowed {
			return fmt.Errorf("runtime semantic document target policy %q is outside manifest", proposal.Target.Policy)
		}
		catalog, found := catalogs[proposal.OwnerModuleID]
		if !found || !runtimeOperationKindsContain(catalog.OperationKinds, proposal.OperationKind) {
			return fmt.Errorf("runtime semantic document proposal is outside owner catalog")
		}
		if !runtimeSemanticStringsWithin(proposal.Target.OwnerSelectorRefs, catalog.SelectorRefs) ||
			!runtimeSemanticStringsWithin(proposal.Target.ProspectiveConstraintRefs, catalog.ConstraintRefs) {
			return fmt.Errorf("runtime semantic document selector or constraint is outside owner catalog")
		}
		if proposal.Target.Policy == RuntimeOperationTargetPolicyExactTarget && !runtimeSemanticStringsWithin(proposal.Target.ExactTargetRefs, catalog.TargetRefs) {
			return fmt.Errorf("runtime semantic document exact target is outside owner catalog")
		}
		if proposal.Target.Policy == RuntimeOperationTargetPolicyExistingOwnerScope && !runtimeSemanticStringsWithin(proposal.Target.OwnerScopeTargetRefs, catalog.TargetRefs) {
			return fmt.Errorf("runtime semantic document owner-scope target is outside owner catalog")
		}
		// A prospective target is deliberately absent from the current owner
		// inventory. Its authority comes from one sealed owner selector plus
		// sealed prospective constraints, and the owner feasibility observation
		// decides whether the concrete proposed ref is admissible. Requiring the
		// ref to already exist in catalog.TargetRefs would make empty-workspace
		// creation unreachable and collapse this policy into exact_target.
		if proposal.ValidationRecipeID != "" {
			if !runtimeSemanticStringsWithin([]string{proposal.ValidationRecipeID}, catalog.ValidationRecipeRefs) {
				return fmt.Errorf("runtime semantic document validation recipe is outside owner catalog")
			}
		}
	}
	if d.PlanStep == RuntimeSemanticPlanStepCoverageOrder {
		// The order step intentionally carries no proof proposal.
	} else if d.BatchID == "" {
		if len(seen) != len(items) {
			return fmt.Errorf("runtime semantic document does not cover every required CWI")
		}
	} else {
		batchIDs := make(map[common.CoverageWorkItemID]struct{}, len(d.BatchCoverageWorkItemIDs))
		for _, id := range d.BatchCoverageWorkItemIDs {
			if _, known := items[id]; !known {
				return fmt.Errorf("runtime semantic batch document coverage %q is outside manifest", id)
			}
			batchIDs[id] = struct{}{}
		}
		if len(batchIDs) != len(seen) {
			return fmt.Errorf("runtime semantic batch document does not cover its batch exactly")
		}
		for id := range batchIDs {
			if _, ok := seen[id]; !ok {
				return fmt.Errorf("runtime semantic batch document is missing coverage %q", id)
			}
		}
	}
	return nil
}

func runtimeSemanticStringsWithin(values, allowed []string) bool {
	allowedSet := runtimeSemanticStringSet(allowed)
	for _, value := range normalizeStrings(values) {
		if _, found := allowedSet[value]; !found {
			return false
		}
	}
	return true
}

func StableRuntimeSemanticPlanDocumentHash(document RuntimeSemanticPlanDocument) string {
	document = document.Normalized()
	h := sha256.New()
	writeHashString(h, RuntimeSemanticPlanDocumentSchemaCurrent)
	writeHashString(h, document.ManifestHash)
	writeHashString(h, document.DefinitionSeedHash)
	writeHashString(h, string(document.PlanStep))
	writeHashString(h, document.BatchID)
	writeHashString(h, fmt.Sprintf("%d", document.BatchSequence))
	for _, id := range document.BatchCoverageWorkItemIDs {
		writeHashString(h, string(id))
	}
	for _, coverageID := range document.CoverageOrder {
		writeHashString(h, string(coverageID))
	}
	for _, proposal := range document.Proposals {
		writeHashString(h, StableRuntimeSemanticProofProposalHash(proposal))
	}
	return "runtime-semantic-document-" + hex.EncodeToString(h.Sum(nil))[:20]
}

// StableRuntimeSemanticPlanCoverageOrderHash binds the LLM-authored planning
// order without treating it as an authority decision.
func StableRuntimeSemanticPlanCoverageOrderHash(document RuntimeSemanticPlanDocument) string {
	document = document.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-semantic-plan-coverage-order")
	for _, id := range document.CoverageOrder {
		writeHashString(h, string(id))
	}
	return "runtime-semantic-plan-order-" + hex.EncodeToString(h.Sum(nil))[:20]
}

type RuntimeSemanticFeasibilityStatus string

const (
	RuntimeSemanticFeasibilityFeasible             RuntimeSemanticFeasibilityStatus = "feasible"
	RuntimeSemanticFeasibilityStale                RuntimeSemanticFeasibilityStatus = "stale"
	RuntimeSemanticFeasibilityPrerequisiteRequired RuntimeSemanticFeasibilityStatus = "prerequisite_required"
	RuntimeSemanticFeasibilityInfeasible           RuntimeSemanticFeasibilityStatus = "infeasible"
)

// RuntimeSemanticProofFeasibility is an owner-authored fact for exactly one
// proposal.  It cannot select a retry, terminal or coverage outcome.
type RuntimeSemanticProofFeasibility struct {
	SchemaVersion        string
	ManifestHash         string
	DefinitionSeedHash   string
	CoverageWorkItemID   common.CoverageWorkItemID
	ProposalHash         string
	OwnerModuleID        string
	CatalogSnapshotID    string
	CatalogObservationID string
	Status               RuntimeSemanticFeasibilityStatus
	ObservedRevision     string
	TargetRefs           []string
	ValidationRecipeID   string
	RequiredCheckRefs    []string
	OriginalDiagnostic   *RuntimeOriginalDiagnostic
}

func (f RuntimeSemanticProofFeasibility) Normalized() RuntimeSemanticProofFeasibility {
	f.SchemaVersion = strings.TrimSpace(f.SchemaVersion)
	f.ManifestHash = strings.TrimSpace(f.ManifestHash)
	f.DefinitionSeedHash = strings.TrimSpace(f.DefinitionSeedHash)
	f.CoverageWorkItemID = common.CoverageWorkItemID(strings.TrimSpace(string(f.CoverageWorkItemID)))
	f.ProposalHash = strings.TrimSpace(f.ProposalHash)
	f.OwnerModuleID = strings.TrimSpace(f.OwnerModuleID)
	f.CatalogSnapshotID = strings.TrimSpace(f.CatalogSnapshotID)
	f.CatalogObservationID = strings.TrimSpace(f.CatalogObservationID)
	f.Status = RuntimeSemanticFeasibilityStatus(strings.TrimSpace(string(f.Status)))
	f.ObservedRevision = strings.TrimSpace(f.ObservedRevision)
	f.TargetRefs = normalizeStrings(f.TargetRefs)
	f.ValidationRecipeID = strings.TrimSpace(f.ValidationRecipeID)
	f.RequiredCheckRefs = normalizeStrings(f.RequiredCheckRefs)
	f.OriginalDiagnostic = f.OriginalDiagnostic.Normalized()
	return f
}

func (f RuntimeSemanticProofFeasibility) Clone() RuntimeSemanticProofFeasibility {
	originalDiagnostic := f.OriginalDiagnostic.CloneOriginal()
	f = f.Normalized()
	f.TargetRefs = append([]string(nil), f.TargetRefs...)
	f.RequiredCheckRefs = append([]string(nil), f.RequiredCheckRefs...)
	f.OriginalDiagnostic = originalDiagnostic
	return f
}

func (f RuntimeSemanticProofFeasibility) ValidateForProposal(manifest RuntimeSemanticPlanManifest, proposal RuntimeSemanticProofProposal) error {
	f = f.Normalized()
	manifest = manifest.Normalized()
	proposal = proposal.Normalized()
	if f.SchemaVersion != RuntimeSemanticFeasibilitySchemaCurrent || f.ManifestHash != manifest.Hash || f.DefinitionSeedHash != manifest.DefinitionSeedHash ||
		f.CoverageWorkItemID != proposal.CoverageWorkItemID || f.ProposalHash != StableRuntimeSemanticProofProposalHash(proposal) || f.OwnerModuleID != proposal.OwnerModuleID || f.CatalogSnapshotID == "" || f.ObservedRevision == "" {
		return fmt.Errorf("runtime semantic feasibility identity does not match proposal")
	}
	switch f.Status {
	case RuntimeSemanticFeasibilityFeasible:
		if f.OriginalDiagnostic != nil {
			return fmt.Errorf("feasible runtime semantic observation must not carry a failure diagnostic")
		}
	case RuntimeSemanticFeasibilityStale, RuntimeSemanticFeasibilityPrerequisiteRequired, RuntimeSemanticFeasibilityInfeasible:
		if f.OriginalDiagnostic == nil {
			return fmt.Errorf("non-feasible runtime semantic observation requires original diagnostic")
		}
		if err := f.OriginalDiagnostic.Validate(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("runtime semantic feasibility has invalid status %q", f.Status)
	}
	return nil
}

type RuntimeSemanticPlanningPhase string

const (
	RuntimeSemanticPlanningRequired                      RuntimeSemanticPlanningPhase = "semantic_contract_required"
	RuntimeSemanticPlanningOrderRequestOpen              RuntimeSemanticPlanningPhase = "coverage_order_request_open"
	RuntimeSemanticPlanningOrderAccepted                 RuntimeSemanticPlanningPhase = "coverage_order_accepted"
	RuntimeSemanticPlanningProofBatchRequestOpen         RuntimeSemanticPlanningPhase = "proof_batch_request_open"
	RuntimeSemanticPlanningProofBatchDocumentAccepted    RuntimeSemanticPlanningPhase = "proof_batch_document_accepted"
	RuntimeSemanticPlanningProofBatchFeasibilityPending  RuntimeSemanticPlanningPhase = "proof_batch_feasibility_pending"
	RuntimeSemanticPlanningProofBatchFeasibilityComplete RuntimeSemanticPlanningPhase = "proof_batch_feasibility_complete"
	RuntimeSemanticPlanningRequestOpen                   RuntimeSemanticPlanningPhase = "semantic_plan_requested"
	RuntimeSemanticPlanningProposalAccepted              RuntimeSemanticPlanningPhase = "semantic_plan_accepted"
	RuntimeSemanticPlanningFeasibilityRequired           RuntimeSemanticPlanningPhase = "semantic_feasibility_required"
	RuntimeSemanticPlanningCorrectionRequired            RuntimeSemanticPlanningPhase = "semantic_plan_correction_required"
	RuntimeSemanticPlanningSealed                        RuntimeSemanticPlanningPhase = "semantic_contract_sealed"
	RuntimeSemanticPlanningFailed                        RuntimeSemanticPlanningPhase = "semantic_contract_failed"
)

// RuntimeSemanticFeasibilityGap preserves one retired semantic proposal and
// its complete owner-fact lineage. It is audit data only: a gap cannot select
// retry, terminal, or the next semantic proposal.
type RuntimeSemanticFeasibilityGap struct {
	RefreshGeneration  int
	ManifestHash       string
	DocumentHash       string
	FeasibilityBatchID string
	FeasibilitySetHash string
	Feasibilities      []RuntimeSemanticProofFeasibility
	Rejections         []RuntimeOriginalDiagnostic
}

func (g RuntimeSemanticFeasibilityGap) Normalized() RuntimeSemanticFeasibilityGap {
	g.ManifestHash = strings.TrimSpace(g.ManifestHash)
	g.DocumentHash = strings.TrimSpace(g.DocumentHash)
	g.FeasibilityBatchID = strings.TrimSpace(g.FeasibilityBatchID)
	g.Feasibilities = normalizeRuntimeSemanticFeasibilities(g.Feasibilities)
	g.FeasibilitySetHash = strings.TrimSpace(g.FeasibilitySetHash)
	for idx := range g.Rejections {
		if value := g.Rejections[idx].Normalized(); value != nil {
			g.Rejections[idx] = *value
		}
	}
	return g
}

func (g RuntimeSemanticFeasibilityGap) Clone() RuntimeSemanticFeasibilityGap {
	originalFeasibilities := append([]RuntimeSemanticProofFeasibility(nil), g.Feasibilities...)
	originalRejections := append([]RuntimeOriginalDiagnostic(nil), g.Rejections...)
	g = g.Normalized()
	g.Feasibilities = make([]RuntimeSemanticProofFeasibility, len(originalFeasibilities))
	for idx := range originalFeasibilities {
		g.Feasibilities[idx] = originalFeasibilities[idx].Clone()
	}
	g.Rejections = make([]RuntimeOriginalDiagnostic, len(originalRejections))
	for idx := range originalRejections {
		if value := originalRejections[idx].CloneOriginal(); value != nil {
			g.Rejections[idx] = *value
		}
	}
	return g
}

func (g RuntimeSemanticFeasibilityGap) Validate() error {
	g = g.Normalized()
	if g.RefreshGeneration < 0 || g.ManifestHash == "" || g.DocumentHash == "" || g.FeasibilityBatchID == "" || len(g.Feasibilities) == 0 {
		return fmt.Errorf("runtime semantic feasibility gap requires manifest, document, batch and owner facts")
	}
	if g.FeasibilitySetHash != StableRuntimeSemanticFeasibilitySetHash(g.Feasibilities) {
		return fmt.Errorf("runtime semantic feasibility gap set hash does not match owner facts")
	}
	refreshable := false
	for _, feasibility := range g.Feasibilities {
		if feasibility.Status == RuntimeSemanticFeasibilityStale || feasibility.Status == RuntimeSemanticFeasibilityPrerequisiteRequired {
			refreshable = true
		}
		if feasibility.OriginalDiagnostic != nil {
			if err := feasibility.OriginalDiagnostic.Validate(); err != nil {
				return err
			}
		}
	}
	if !refreshable {
		return fmt.Errorf("runtime semantic feasibility gap requires stale or prerequisite-required owner fact")
	}
	for idx := range g.Rejections {
		if err := g.Rejections[idx].Validate(); err != nil {
			return fmt.Errorf("runtime semantic feasibility gap rejection %d: %w", idx, err)
		}
	}
	return nil
}

// RuntimeSemanticPlanningLedger is persisted owner data.  pkg validates its
// value shape only; Orchestrator private internal events own every transition.
type RuntimeSemanticPlanningLedger struct {
	SchemaVersion           string
	Phase                   RuntimeSemanticPlanningPhase
	DefinitionSeed          RuntimeDefinitionSeed
	Manifest                *RuntimeSemanticPlanManifest
	Attempt                 int
	StablePrefixHash        string
	LogicalRequestID        string
	CorrectionHash          string
	Document                *RuntimeSemanticPlanDocument
	DocumentHash            string
	Batches                 []RuntimeSemanticPlanBatch
	ActiveBatchID           string
	NextBatchSequence       int
	PendingCoverageIDs      []common.CoverageWorkItemID
	CompletedBatchIDs       []string
	BatchFeasibilityResults []RuntimeSemanticPlanBatchFeasibilityResult
	BatchDocuments          []RuntimeSemanticPlanDocument
	MergedDocument          *RuntimeSemanticPlanDocument
	MergedDocumentHash      string
	FeasibilityBatchID      string
	Feasibilities           []RuntimeSemanticProofFeasibility
	FeasibilitySetHash      string
	SealedBindingSetHash    string
	SealedLineageRefs       []string
	Rejections              []RuntimeOriginalDiagnostic
	FeasibilityGaps         []RuntimeSemanticFeasibilityGap
	CatalogRefreshAttempts  int
	Failure                 *RuntimeOriginalDiagnostic
}

func NewRuntimeSemanticPlanningLedger(seed RuntimeDefinitionSeed) (RuntimeSemanticPlanningLedger, error) {
	seed = seed.Clone()
	if err := seed.Validate(); err != nil {
		return RuntimeSemanticPlanningLedger{}, err
	}
	return RuntimeSemanticPlanningLedger{SchemaVersion: RuntimeSemanticPlanningLedgerSchemaCurrent, Phase: RuntimeSemanticPlanningRequired, DefinitionSeed: seed}, nil
}

func (l RuntimeSemanticPlanningLedger) Normalized() RuntimeSemanticPlanningLedger {
	l.SchemaVersion = strings.TrimSpace(l.SchemaVersion)
	l.Phase = RuntimeSemanticPlanningPhase(strings.TrimSpace(string(l.Phase)))
	l.DefinitionSeed = l.DefinitionSeed.Clone()
	l.StablePrefixHash = strings.TrimSpace(l.StablePrefixHash)
	l.LogicalRequestID = strings.TrimSpace(l.LogicalRequestID)
	l.CorrectionHash = strings.TrimSpace(l.CorrectionHash)
	if l.Manifest != nil {
		value := l.Manifest.Clone()
		l.Manifest = &value
	}
	if l.Document != nil {
		value := l.Document.Clone()
		l.Document = &value
	}
	l.DocumentHash = strings.TrimSpace(l.DocumentHash)
	l.Batches = normalizeRuntimeSemanticPlanBatches(l.Batches)
	l.ActiveBatchID = strings.TrimSpace(l.ActiveBatchID)
	if l.NextBatchSequence < 0 {
		l.NextBatchSequence = 0
	}
	l.PendingCoverageIDs = normalizeOrderedCoverageWorkItemIDs(l.PendingCoverageIDs)
	l.CompletedBatchIDs = normalizeStrings(l.CompletedBatchIDs)
	l.BatchFeasibilityResults = normalizeRuntimeSemanticPlanBatchFeasibilityResults(l.BatchFeasibilityResults)
	l.BatchDocuments = normalizeRuntimeSemanticPlanDocuments(l.BatchDocuments)
	if l.MergedDocument != nil {
		value := l.MergedDocument.Clone()
		l.MergedDocument = &value
	}
	l.MergedDocumentHash = strings.TrimSpace(l.MergedDocumentHash)
	l.FeasibilityBatchID = strings.TrimSpace(l.FeasibilityBatchID)
	l.Feasibilities = normalizeRuntimeSemanticFeasibilities(l.Feasibilities)
	l.FeasibilitySetHash = strings.TrimSpace(l.FeasibilitySetHash)
	l.SealedBindingSetHash = strings.TrimSpace(l.SealedBindingSetHash)
	l.SealedLineageRefs = normalizeStrings(l.SealedLineageRefs)
	for idx := range l.Rejections {
		if value := l.Rejections[idx].Normalized(); value != nil {
			l.Rejections[idx] = *value
		}
	}
	for idx := range l.FeasibilityGaps {
		l.FeasibilityGaps[idx] = l.FeasibilityGaps[idx].Normalized()
	}
	l.Failure = l.Failure.Normalized()
	return l
}

func (l RuntimeSemanticPlanningLedger) Clone() RuntimeSemanticPlanningLedger {
	originalFeasibilities := append([]RuntimeSemanticProofFeasibility(nil), l.Feasibilities...)
	originalRejections := append([]RuntimeOriginalDiagnostic(nil), l.Rejections...)
	originalGaps := append([]RuntimeSemanticFeasibilityGap(nil), l.FeasibilityGaps...)
	originalFailure := l.Failure.CloneOriginal()
	l = l.Normalized()
	l.Feasibilities = make([]RuntimeSemanticProofFeasibility, len(originalFeasibilities))
	for idx := range originalFeasibilities {
		l.Feasibilities[idx] = originalFeasibilities[idx].Clone()
	}
	l.SealedLineageRefs = append([]string(nil), l.SealedLineageRefs...)
	l.Rejections = make([]RuntimeOriginalDiagnostic, len(originalRejections))
	for idx := range originalRejections {
		if value := originalRejections[idx].CloneOriginal(); value != nil {
			l.Rejections[idx] = *value
		}
	}
	l.FeasibilityGaps = make([]RuntimeSemanticFeasibilityGap, len(originalGaps))
	for idx := range originalGaps {
		l.FeasibilityGaps[idx] = originalGaps[idx].Clone()
	}
	l.Batches = append([]RuntimeSemanticPlanBatch(nil), l.Batches...)
	l.PendingCoverageIDs = append([]common.CoverageWorkItemID(nil), l.PendingCoverageIDs...)
	l.CompletedBatchIDs = append([]string(nil), l.CompletedBatchIDs...)
	l.BatchFeasibilityResults = append([]RuntimeSemanticPlanBatchFeasibilityResult(nil), l.BatchFeasibilityResults...)
	l.BatchDocuments = append([]RuntimeSemanticPlanDocument(nil), l.BatchDocuments...)
	l.Failure = originalFailure
	return l
}

func normalizeRuntimeSemanticPlanBatches(values []RuntimeSemanticPlanBatch) []RuntimeSemanticPlanBatch {
	ret := make([]RuntimeSemanticPlanBatch, 0, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.BatchID != "" {
			ret = append(ret, value)
		}
	}
	sort.SliceStable(ret, func(i, j int) bool { return ret[i].Sequence < ret[j].Sequence })
	return ret
}

func normalizeRuntimeSemanticPlanBatchFeasibilityResults(values []RuntimeSemanticPlanBatchFeasibilityResult) []RuntimeSemanticPlanBatchFeasibilityResult {
	ret := make([]RuntimeSemanticPlanBatchFeasibilityResult, 0, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.BatchID != "" {
			ret = append(ret, value)
		}
	}
	return ret
}

func normalizeRuntimeSemanticPlanDocuments(values []RuntimeSemanticPlanDocument) []RuntimeSemanticPlanDocument {
	ret := make([]RuntimeSemanticPlanDocument, 0, len(values))
	for _, value := range values {
		value = value.Clone()
		if value.BatchID != "" {
			ret = append(ret, value)
		}
	}
	sort.SliceStable(ret, func(i, j int) bool { return ret[i].BatchSequence < ret[j].BatchSequence })
	return ret
}

func (l RuntimeSemanticPlanningLedger) Validate() error {
	l = l.Normalized()
	if l.SchemaVersion != RuntimeSemanticPlanningLedgerSchemaCurrent {
		return fmt.Errorf("runtime semantic planning ledger requires current schema")
	}
	if err := l.DefinitionSeed.Validate(); err != nil {
		return err
	}
	if l.NextBatchSequence < 0 || (l.ActiveBatchID != "" && len(l.Batches) == 0) {
		return fmt.Errorf("runtime semantic planning batch cursor is invalid")
	}
	batchIDs := make(map[string]struct{}, len(l.Batches))
	for idx := range l.Batches {
		batch := l.Batches[idx]
		if _, duplicate := batchIDs[batch.BatchID]; duplicate {
			return fmt.Errorf("runtime semantic planning duplicates batch %q", batch.BatchID)
		}
		batchIDs[batch.BatchID] = struct{}{}
		if l.Manifest != nil {
			if err := batch.ValidateAgainstManifest(*l.Manifest); err != nil {
				return err
			}
		}
	}
	for _, document := range l.BatchDocuments {
		var batch *RuntimeSemanticPlanBatch
		for idx := range l.Batches {
			if l.Batches[idx].BatchID == document.BatchID {
				value := l.Batches[idx]
				batch = &value
				break
			}
		}
		if batch == nil || batch.DocumentHash != StableRuntimeSemanticPlanDocumentHash(document) {
			return fmt.Errorf("runtime semantic batch document hash is not bound to batch")
		}
	}
	if l.ActiveBatchID != "" {
		if _, found := batchIDs[l.ActiveBatchID]; !found {
			return fmt.Errorf("runtime semantic planning active batch is missing")
		}
	}
	seenBatchResults := make(map[string]struct{}, len(l.BatchFeasibilityResults))
	for _, result := range l.BatchFeasibilityResults {
		if _, duplicate := seenBatchResults[result.BatchID]; duplicate {
			return fmt.Errorf("runtime semantic planning duplicates batch feasibility %q", result.BatchID)
		}
		seenBatchResults[result.BatchID] = struct{}{}
		var batch *RuntimeSemanticPlanBatch
		for idx := range l.Batches {
			if l.Batches[idx].BatchID == result.BatchID {
				value := l.Batches[idx]
				batch = &value
				break
			}
		}
		if batch == nil {
			return fmt.Errorf("runtime semantic planning batch feasibility has no matching batch")
		}
		if result.BatchHash != batch.Hash || result.Hash != StableRuntimeSemanticPlanBatchFeasibilityResultHash(result) {
			return fmt.Errorf("runtime semantic planning batch feasibility identity is stale")
		}
	}
	if l.MergedDocument != nil {
		if l.MergedDocumentHash != StableRuntimeSemanticPlanDocumentHash(*l.MergedDocument) {
			return fmt.Errorf("runtime semantic merged document hash does not match contents")
		}
	}
	for idx := range l.Rejections {
		if err := l.Rejections[idx].Validate(); err != nil {
			return fmt.Errorf("runtime semantic planning rejection %d: %w", idx, err)
		}
	}
	if l.CatalogRefreshAttempts < 0 || l.CatalogRefreshAttempts != len(l.FeasibilityGaps) {
		return fmt.Errorf("runtime semantic catalog refresh attempts do not match gap lineage")
	}
	for idx := range l.FeasibilityGaps {
		if err := l.FeasibilityGaps[idx].Validate(); err != nil {
			return fmt.Errorf("runtime semantic feasibility gap %d: %w", idx, err)
		}
		if l.FeasibilityGaps[idx].RefreshGeneration != idx {
			return fmt.Errorf("runtime semantic feasibility gap generation %d is not contiguous", l.FeasibilityGaps[idx].RefreshGeneration)
		}
	}
	switch l.Phase {
	case RuntimeSemanticPlanningRequired:
		if l.Manifest != nil || l.Attempt != 0 || l.StablePrefixHash != "" || l.LogicalRequestID != "" || l.CorrectionHash != "" || l.Document != nil || l.DocumentHash != "" || l.FeasibilityBatchID != "" || len(l.Feasibilities) != 0 || l.FeasibilitySetHash != "" || l.SealedBindingSetHash != "" || len(l.SealedLineageRefs) != 0 || len(l.Rejections) != 0 || l.Failure != nil {
			return fmt.Errorf("required runtime semantic planning ledger has unexpected downstream facts")
		}
	case RuntimeSemanticPlanningOrderRequestOpen, RuntimeSemanticPlanningRequestOpen, RuntimeSemanticPlanningCorrectionRequired:
		batchCorrectionDocument := l.Phase == RuntimeSemanticPlanningCorrectionRequired && l.ActiveBatchID != "" && l.Document != nil && l.Document.PlanStep == RuntimeSemanticPlanStepCoverageOrder && l.DocumentHash == StableRuntimeSemanticPlanDocumentHash(*l.Document)
		if l.Manifest == nil || l.Attempt < 0 || l.Attempt > 1 || l.StablePrefixHash == "" || l.LogicalRequestID == "" || (!batchCorrectionDocument && (l.Document != nil || l.DocumentHash != "")) || l.FeasibilityBatchID != "" || len(l.Feasibilities) != 0 || l.FeasibilitySetHash != "" || l.SealedBindingSetHash != "" || len(l.SealedLineageRefs) != 0 || l.Failure != nil {
			return fmt.Errorf("open runtime semantic planning ledger has invalid shape")
		}
		if (l.Attempt == 0 && l.CorrectionHash != "") || (l.Attempt == 1 && l.CorrectionHash == "") || (l.Phase == RuntimeSemanticPlanningCorrectionRequired && l.Attempt != 1) {
			return fmt.Errorf("runtime semantic planning correction lineage is invalid")
		}
		if ((l.Phase == RuntimeSemanticPlanningOrderRequestOpen || l.Phase == RuntimeSemanticPlanningRequestOpen) && len(l.Rejections) != 0) || (l.Phase == RuntimeSemanticPlanningCorrectionRequired && len(l.Rejections) != 1) {
			return fmt.Errorf("runtime semantic planning rejection lineage is invalid")
		}
		if err := l.Manifest.Validate(); err != nil {
			return err
		}
	case RuntimeSemanticPlanningOrderAccepted, RuntimeSemanticPlanningProofBatchRequestOpen, RuntimeSemanticPlanningProofBatchDocumentAccepted,
		RuntimeSemanticPlanningProposalAccepted, RuntimeSemanticPlanningFeasibilityRequired:
		if l.Manifest == nil || l.Attempt < 0 || l.Attempt > 1 || l.StablePrefixHash == "" || l.LogicalRequestID == "" || l.Document == nil || l.DocumentHash == "" || l.SealedBindingSetHash != "" || len(l.SealedLineageRefs) != 0 || len(l.Rejections) != l.Attempt || l.Failure != nil {
			return fmt.Errorf("accepted runtime semantic planning ledger has invalid shape")
		}
		if err := l.Document.ValidateAgainstManifest(*l.Manifest); err != nil {
			return err
		}
		if l.Phase == RuntimeSemanticPlanningOrderAccepted && l.Document.PlanStep != RuntimeSemanticPlanStepCoverageOrder {
			return fmt.Errorf("order-accepted semantic ledger requires coverage-order document")
		}
		if l.Phase == RuntimeSemanticPlanningProofBatchRequestOpen && (l.Document.PlanStep != RuntimeSemanticPlanStepCoverageOrder || l.ActiveBatchID == "") {
			return fmt.Errorf("proof-batch request ledger requires active batch and order document")
		}
		if l.DocumentHash != StableRuntimeSemanticPlanDocumentHash(*l.Document) {
			return fmt.Errorf("runtime semantic planning document hash does not match contents")
		}
		if l.Phase == RuntimeSemanticPlanningProposalAccepted && len(l.Feasibilities) != 0 {
			return fmt.Errorf("accepted runtime semantic proposal cannot carry owner feasibility")
		}
		if l.Phase == RuntimeSemanticPlanningProposalAccepted && (l.FeasibilityBatchID != "" || l.FeasibilitySetHash != "") {
			return fmt.Errorf("accepted runtime semantic proposal cannot carry feasibility lineage")
		}
		if l.Phase == RuntimeSemanticPlanningFeasibilityRequired {
			feasibilityDocument := l.Document
			if l.ActiveBatchID != "" {
				for idx := range l.BatchDocuments {
					if l.BatchDocuments[idx].BatchID == l.ActiveBatchID {
						value := l.BatchDocuments[idx].Clone()
						feasibilityDocument = &value
						break
					}
				}
			}
			if l.FeasibilityBatchID != StableRuntimeSemanticFeasibilityBatchID(*l.Manifest, *feasibilityDocument) {
				return fmt.Errorf("runtime semantic feasibility batch identity does not match document")
			}
		}
		if l.FeasibilitySetHash != StableRuntimeSemanticFeasibilitySetHash(l.Feasibilities) {
			return fmt.Errorf("runtime semantic feasibility set hash does not match observations")
		}
		feasibilityDocument := l.Document
		if l.ActiveBatchID != "" {
			for idx := range l.BatchDocuments {
				if l.BatchDocuments[idx].BatchID == l.ActiveBatchID {
					value := l.BatchDocuments[idx].Clone()
					feasibilityDocument = &value
					break
				}
			}
		}
		if err := validateRuntimeSemanticFeasibilitySet(*l.Manifest, *feasibilityDocument, l.Feasibilities, false); err != nil {
			return err
		}
	case RuntimeSemanticPlanningProofBatchFeasibilityComplete:
		// A completed proof batch is still an unsealed SM-2 working state.  It
		// carries only the accepted batch document and the complete owner facts
		// for that batch; binding/focus authority is not allowed until the
		// orchestrator performs the explicit merge/seal transition.
		if l.Manifest == nil || l.Attempt < 0 || l.Attempt > 1 || l.StablePrefixHash == "" || l.LogicalRequestID == "" || l.Document == nil || l.DocumentHash == "" || l.ActiveBatchID == "" || l.FeasibilityBatchID == "" || l.SealedBindingSetHash != "" || len(l.SealedLineageRefs) != 0 || len(l.Rejections) != l.Attempt || l.Failure != nil {
			return fmt.Errorf("completed runtime semantic proof batch has invalid shape")
		}
		var batchDocument *RuntimeSemanticPlanDocument
		for idx := range l.BatchDocuments {
			if l.BatchDocuments[idx].BatchID == l.ActiveBatchID {
				value := l.BatchDocuments[idx].Clone()
				batchDocument = &value
				break
			}
		}
		if batchDocument == nil || batchDocument.PlanStep != RuntimeSemanticPlanStepProofBatch || batchDocument.BatchID != l.ActiveBatchID {
			return fmt.Errorf("completed runtime semantic proof batch requires active proof-batch document")
		}
		if err := batchDocument.ValidateAgainstManifest(*l.Manifest); err != nil {
			return err
		}
		if l.Document.PlanStep != RuntimeSemanticPlanStepCoverageOrder || l.DocumentHash != StableRuntimeSemanticPlanDocumentHash(*l.Document) || l.FeasibilityBatchID != StableRuntimeSemanticFeasibilityBatchID(*l.Manifest, *batchDocument) {
			return fmt.Errorf("completed runtime semantic proof batch lineage does not match contents")
		}
		if len(l.Feasibilities) != len(batchDocument.Proposals) || l.FeasibilitySetHash != StableRuntimeSemanticFeasibilitySetHash(l.Feasibilities) {
			return fmt.Errorf("completed runtime semantic proof batch requires complete owner facts")
		}
		if err := validateRuntimeSemanticFeasibilitySet(*l.Manifest, *batchDocument, l.Feasibilities, false); err != nil {
			return err
		}
	case RuntimeSemanticPlanningSealed:
		if l.Manifest == nil || l.StablePrefixHash == "" || l.LogicalRequestID == "" || l.Document == nil || l.DocumentHash == "" || l.FeasibilityBatchID == "" || len(l.Feasibilities) != len(l.Document.Proposals) || l.FeasibilitySetHash == "" || l.SealedBindingSetHash == "" || len(l.SealedLineageRefs) == 0 || len(l.Rejections) != l.Attempt || l.Failure != nil {
			return fmt.Errorf("sealed runtime semantic planning ledger requires document, complete feasibility and binding hash")
		}
		if err := l.Document.ValidateAgainstManifest(*l.Manifest); err != nil {
			return err
		}
		if l.DocumentHash != StableRuntimeSemanticPlanDocumentHash(*l.Document) || l.FeasibilityBatchID != StableRuntimeSemanticFeasibilityBatchID(*l.Manifest, *l.Document) || l.FeasibilitySetHash != StableRuntimeSemanticFeasibilitySetHash(l.Feasibilities) {
			return fmt.Errorf("sealed runtime semantic planning lineage does not match contents")
		}
		if err := validateRuntimeSemanticFeasibilitySet(*l.Manifest, *l.Document, l.Feasibilities, true); err != nil {
			return err
		}
	case RuntimeSemanticPlanningFailed:
		if l.Failure == nil || l.SealedBindingSetHash != "" || len(l.SealedLineageRefs) != 0 {
			return fmt.Errorf("failed runtime semantic planning ledger requires failure and no sealed binding")
		}
		if err := l.Failure.Validate(); err != nil {
			return err
		}
		if l.Manifest == nil {
			if l.Document != nil || l.DocumentHash != "" || l.FeasibilityBatchID != "" || len(l.Feasibilities) != 0 || l.FeasibilitySetHash != "" {
				return fmt.Errorf("failed runtime semantic planning ledger has downstream facts without a manifest")
			}
			break
		}
		if err := l.Manifest.Validate(); err != nil {
			return err
		}
		if l.Document == nil {
			if l.DocumentHash != "" || l.FeasibilityBatchID != "" || len(l.Feasibilities) != 0 || l.FeasibilitySetHash != "" {
				return fmt.Errorf("failed runtime semantic planning ledger has owner facts without a document")
			}
			break
		}
		if err := l.Document.ValidateAgainstManifest(*l.Manifest); err != nil {
			return err
		}
		if l.DocumentHash != StableRuntimeSemanticPlanDocumentHash(*l.Document) {
			return fmt.Errorf("failed runtime semantic planning document hash does not match contents")
		}
		if l.FeasibilityBatchID != "" && l.FeasibilityBatchID != StableRuntimeSemanticFeasibilityBatchID(*l.Manifest, *l.Document) {
			return fmt.Errorf("failed runtime semantic feasibility batch identity does not match document")
		}
		if l.FeasibilitySetHash != StableRuntimeSemanticFeasibilitySetHash(l.Feasibilities) {
			return fmt.Errorf("failed runtime semantic feasibility set hash does not match observations")
		}
		if err := validateRuntimeSemanticFeasibilitySet(*l.Manifest, *l.Document, l.Feasibilities, false); err != nil {
			return err
		}
	default:
		return fmt.Errorf("runtime semantic planning ledger has invalid phase %q", l.Phase)
	}
	return nil
}

// StableRuntimeSemanticFeasibilityBatchID binds one owner-probe fan-out to
// exactly one accepted semantic document. It is lineage only and grants no
// feasibility or coverage outcome.
func StableRuntimeSemanticFeasibilityBatchID(manifest RuntimeSemanticPlanManifest, document RuntimeSemanticPlanDocument) string {
	manifest = manifest.Normalized()
	document = document.Normalized()
	h := sha256.New()
	writeHashString(h, "runtime-semantic-feasibility-batch")
	writeHashString(h, manifest.Hash)
	writeHashString(h, StableRuntimeSemanticPlanDocumentHash(document))
	return "runtime-semantic-feasibility-batch-" + hex.EncodeToString(h.Sum(nil))[:20]
}

// StableRuntimeSemanticPlanCorrectionHash binds the sole correction request to
// the complete producer-authored rejection without turning its text into a
// retry decision. SM-3 decides whether this hash may be used at all.
func StableRuntimeSemanticPlanCorrectionHash(rejection RuntimeOriginalDiagnostic) string {
	diagnostic := rejection.Normalized()
	if diagnostic == nil {
		return ""
	}
	h := sha256.New()
	writeHashString(h, "runtime-semantic-plan-correction")
	writeHashString(h, diagnostic.SchemaVersion)
	writeHashString(h, diagnostic.OwnerModuleID)
	writeHashString(h, diagnostic.OriginalClass)
	writeHashString(h, diagnostic.OriginalCode)
	writeHashString(h, diagnostic.OriginalMessage)
	writeHashString(h, diagnostic.Signature)
	writeHashString(h, diagnostic.OperationID)
	writeHashStrings(h, diagnostic.TargetRefs)
	for _, violation := range diagnostic.FieldViolations {
		writeHashString(h, violation.Path)
		writeHashString(h, violation.Actual)
		writeHashStrings(h, violation.Allowed)
		writeHashStrings(h, violation.Required)
		writeHashStrings(h, violation.Forbidden)
	}
	return "runtime-semantic-correction-" + hex.EncodeToString(h.Sum(nil))[:20]
}

// StableRuntimeSemanticFeasibilitySetHash is the deterministic observation
// identity consumed by SM-2. Empty input intentionally has an empty hash.
func StableRuntimeSemanticFeasibilitySetHash(values []RuntimeSemanticProofFeasibility) string {
	values = normalizeRuntimeSemanticFeasibilities(values)
	if len(values) == 0 {
		return ""
	}
	h := sha256.New()
	writeHashString(h, "runtime-semantic-feasibility-set")
	for _, value := range values {
		writeHashString(h, value.ManifestHash)
		writeHashString(h, value.DefinitionSeedHash)
		writeHashString(h, string(value.CoverageWorkItemID))
		writeHashString(h, value.ProposalHash)
		writeHashString(h, value.OwnerModuleID)
		writeHashString(h, value.CatalogSnapshotID)
		writeHashString(h, value.CatalogObservationID)
		writeHashString(h, string(value.Status))
		writeHashString(h, value.ObservedRevision)
		writeHashStrings(h, value.TargetRefs)
		writeHashString(h, value.ValidationRecipeID)
		writeHashStrings(h, value.RequiredCheckRefs)
		if value.OriginalDiagnostic != nil {
			diagnostic := value.OriginalDiagnostic.Normalized()
			writeHashString(h, diagnostic.OwnerModuleID)
			writeHashString(h, diagnostic.OriginalClass)
			writeHashString(h, diagnostic.OriginalCode)
			writeHashString(h, diagnostic.OriginalMessage)
			writeHashString(h, diagnostic.Signature)
			writeHashString(h, diagnostic.OperationID)
			writeHashStrings(h, diagnostic.TargetRefs)
			writeHashString(h, diagnostic.ExpectedRevision)
			writeHashString(h, diagnostic.ObservedRevision)
			for _, violation := range diagnostic.FieldViolations {
				writeHashString(h, violation.Path)
				writeHashString(h, violation.Actual)
				writeHashStrings(h, violation.Allowed)
				writeHashStrings(h, violation.Required)
				writeHashStrings(h, violation.Forbidden)
			}
		}
	}
	return "runtime-semantic-feasibility-set-" + hex.EncodeToString(h.Sum(nil))[:20]
}

func validateRuntimeSemanticFeasibilitySet(manifest RuntimeSemanticPlanManifest, document RuntimeSemanticPlanDocument, values []RuntimeSemanticProofFeasibility, complete bool) error {
	proposals := make(map[common.CoverageWorkItemID]RuntimeSemanticProofProposal, len(document.Proposals))
	for _, proposal := range document.Normalized().Proposals {
		proposals[proposal.CoverageWorkItemID] = proposal
	}
	seen := make(map[common.CoverageWorkItemID]struct{}, len(values))
	for _, feasibility := range normalizeRuntimeSemanticFeasibilities(values) {
		proposal, found := proposals[feasibility.CoverageWorkItemID]
		if !found {
			return fmt.Errorf("runtime semantic feasibility references unknown coverage %q", feasibility.CoverageWorkItemID)
		}
		if _, duplicate := seen[feasibility.CoverageWorkItemID]; duplicate {
			return fmt.Errorf("runtime semantic feasibility duplicates coverage %q", feasibility.CoverageWorkItemID)
		}
		seen[feasibility.CoverageWorkItemID] = struct{}{}
		if err := feasibility.ValidateForProposal(manifest, proposal); err != nil {
			return err
		}
		if complete && feasibility.Status != RuntimeSemanticFeasibilityFeasible {
			return fmt.Errorf("sealed runtime semantic planning ledger contains non-feasible coverage %q", feasibility.CoverageWorkItemID)
		}
	}
	if complete && len(seen) != len(proposals) {
		return fmt.Errorf("sealed runtime semantic planning ledger does not cover every proposal")
	}
	return nil
}

// ValidateRuntimeSemanticFeasibilitySet exposes the pure full-document
// feasibility contract to the Orchestrator owner adapter.
func ValidateRuntimeSemanticFeasibilitySet(manifest RuntimeSemanticPlanManifest, document RuntimeSemanticPlanDocument, values []RuntimeSemanticProofFeasibility, complete bool) error {
	return validateRuntimeSemanticFeasibilitySet(manifest, document, values, complete)
}

func normalizeRuntimeSemanticProofProposals(values []RuntimeSemanticProofProposal) []RuntimeSemanticProofProposal {
	ret := make([]RuntimeSemanticProofProposal, 0, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.CoverageWorkItemID != "" {
			ret = append(ret, value)
		}
	}
	sort.SliceStable(ret, func(i, j int) bool { return ret[i].CoverageWorkItemID < ret[j].CoverageWorkItemID })
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func normalizeOrderedCoverageWorkItemIDs(values []common.CoverageWorkItemID) []common.CoverageWorkItemID {
	ret := make([]common.CoverageWorkItemID, 0, len(values))
	seen := make(map[common.CoverageWorkItemID]struct{}, len(values))
	for _, value := range values {
		value = common.CoverageWorkItemID(strings.TrimSpace(string(value)))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	return ret
}

func validateRuntimeSemanticCoverageOrder(order []common.CoverageWorkItemID, items []RuntimeCoverageItem) error {
	if len(order) != len(items) {
		return fmt.Errorf("runtime semantic document coverage order must cover every CWI exactly once")
	}
	known := make(map[common.CoverageWorkItemID]RuntimeCoverageItem, len(items))
	position := make(map[common.CoverageWorkItemID]int, len(order))
	for _, item := range items {
		known[item.ID] = item
	}
	for idx, id := range order {
		if _, duplicate := position[id]; duplicate {
			return fmt.Errorf("runtime semantic document coverage order duplicates %q", id)
		}
		if _, ok := known[id]; !ok {
			return fmt.Errorf("runtime semantic document coverage order references unknown %q", id)
		}
		position[id] = idx
	}
	for _, item := range items {
		itemPos := position[item.ID]
		for _, dependency := range item.ReopenDependencies {
			depPos, ok := position[dependency.CoverageWorkItemID]
			if !ok || depPos >= itemPos {
				return fmt.Errorf("runtime semantic document coverage order violates dependency %q -> %q", dependency.CoverageWorkItemID, item.ID)
			}
		}
	}
	return nil
}

func normalizeRuntimeSemanticFeasibilities(values []RuntimeSemanticProofFeasibility) []RuntimeSemanticProofFeasibility {
	ret := make([]RuntimeSemanticProofFeasibility, 0, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.CoverageWorkItemID != "" && value.OwnerModuleID != "" {
			ret = append(ret, value)
		}
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].CoverageWorkItemID != ret[j].CoverageWorkItemID {
			return ret[i].CoverageWorkItemID < ret[j].CoverageWorkItemID
		}
		return ret[i].OwnerModuleID < ret[j].OwnerModuleID
	})
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func normalizeRuntimeSemanticTargetPolicies(values []RuntimeOperationTargetPolicy) []RuntimeOperationTargetPolicy {
	seen := map[RuntimeOperationTargetPolicy]struct{}{}
	for _, value := range values {
		value = RuntimeOperationTargetPolicy(strings.TrimSpace(string(value)))
		switch value {
		case RuntimeOperationTargetPolicyExactTarget, RuntimeOperationTargetPolicyExistingOwnerScope, RuntimeOperationTargetPolicyProspectiveOwnerTarget:
			seen[value] = struct{}{}
		}
	}
	ret := make([]RuntimeOperationTargetPolicy, 0, len(seen))
	for value := range seen {
		ret = append(ret, value)
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i] < ret[j] })
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func validateRuntimeSemanticProposalTarget(target RuntimeOperationTargetAuthorization) error {
	target = target.Normalized()
	switch target.Policy {
	case RuntimeOperationTargetPolicyExactTarget:
		if len(target.ExactTargetRefs) == 0 {
			return fmt.Errorf("runtime semantic exact target proposal requires target refs")
		}
	case RuntimeOperationTargetPolicyExistingOwnerScope:
		if len(target.OwnerScopeTargetRefs) == 0 && len(target.OwnerSelectorRefs) == 0 {
			return fmt.Errorf("runtime semantic owner-scope proposal requires bounded targets or selector")
		}
		if len(target.OwnerSelectorRefs) > 1 {
			return fmt.Errorf("runtime semantic owner-scope proposal permits one canonical owner selector")
		}
	case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		if target.ProspectiveTargetRef == "" || len(target.ProspectiveConstraintRefs) == 0 || len(target.OwnerSelectorRefs) != 1 {
			return fmt.Errorf("runtime semantic prospective proposal requires target, constraints and selector")
		}
	default:
		return fmt.Errorf("runtime semantic proposal requires a supported target policy")
	}
	return nil
}

func runtimeSemanticHasDurableEffect(values []RuntimeEffectScope) bool {
	for _, value := range values {
		if RuntimeEffectIsDurable(value) {
			return true
		}
	}
	return false
}

func writeRuntimeSemanticCoverageHash(h hashStringWriter, items []RuntimeCoverageItem, slices []RuntimeAcceptanceSlice) {
	writeRuntimeSemanticCoverageItemsHash(h, items)
	for _, slice := range NormalizeRuntimeAcceptanceSlices(slices) {
		writeHashString(h, slice.ID)
		writeHashString(h, slice.Title)
		writeHashString(h, slice.Scope)
		writeHashString(h, fmt.Sprintf("%d", slice.Priority))
		for _, id := range slice.CoverageWorkItemIDs {
			writeHashString(h, string(id))
		}
		writeHashStrings(h, slice.RequiredEvidenceKinds)
		writeHashString(h, slice.CompletionPolicy)
		writeHashString(h, slice.ResultPolicy)
	}
}

func writeRuntimeSemanticCoverageItemsHash(h hashStringWriter, items []RuntimeCoverageItem) {
	for _, item := range NormalizeRuntimeCoverageItems(items) {
		writeHashString(h, string(item.ID))
		writeHashString(h, item.SliceID)
		writeHashString(h, item.SourceScope)
		writeHashString(h, item.SourceID)
		writeHashString(h, item.TargetText)
		writeHashString(h, fmt.Sprintf("%d", item.Order))
		for _, dependency := range item.ReopenDependencies {
			writeHashString(h, string(dependency.CoverageWorkItemID))
			writeHashStrings(h, dependency.TriggerEvidenceKinds)
		}
	}
}

func writeRuntimeSemanticTargetHash(h hashStringWriter, target RuntimeOperationTargetAuthorization) {
	target = target.Normalized()
	writeHashString(h, string(target.Policy))
	writeHashStrings(h, target.ExactTargetRefs)
	writeHashStrings(h, target.OwnerScopeTargetRefs)
	writeHashStrings(h, target.OwnerSelectorRefs)
	writeHashStrings(h, target.ProspectiveConstraintRefs)
	writeHashString(h, target.ProspectiveTargetRef)
}

func runtimeSemanticStringSet(values []string) map[string]struct{} {
	ret := make(map[string]struct{}, len(values))
	for _, value := range values {
		ret[value] = struct{}{}
	}
	return ret
}

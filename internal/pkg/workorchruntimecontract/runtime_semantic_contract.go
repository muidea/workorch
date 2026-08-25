package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
		s.CoverageItems[idx].PrerequisiteCoverageWorkItemIDs = cloneRuntimeCoveragePrerequisiteIDs(s.CoverageItems[idx].PrerequisiteCoverageWorkItemIDs)
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
	// ProofObligations are the sole per-CWI Definition/owner-fact authority
	// consumed by schema generation, document admission and final binding.
	// Later aggregate reads must not reintroduce DefinitionFocuses as a second
	// semantic authority.
	ProofObligations []RuntimeSemanticProofObligation
	// OwnerCatalogs are the bounded owner-authored vocabulary observed before
	// provider I/O. They prevent the semantic planner from guessing targets,
	// selectors, constraints, recipes, or catalog revisions and then learning
	// those facts only through a later rejection.
	OwnerCatalogs []RuntimeOperationAdmissibility
	// OwnerFactDependencyCatalogs seal capability-owner facts that are required
	// to judge a prospective owner-fact acquisition but are intentionally not
	// selectable semantic-plan vocabulary. They participate in freshness/CAS
	// and seal guards, while provider projections continue to expose only
	// OwnerCatalogs.
	OwnerFactDependencyCatalogs []RuntimeOperationAdmissibility
}

// CompileRuntimeSemanticPlanManifest compiles the provider-visible semantic
// vocabulary exclusively from activation-sealed definition capabilities. It
// is deliberately pure: owner resource availability is established later by
// typed feasibility observations and is never inferred here.
func CompileRuntimeSemanticPlanManifest(workspaceID common.WorkspaceID, runID common.RunID, definition RuntimeDefinitionContext, seed RuntimeDefinitionSeed, definitionFocuses []RuntimeDefinitionFocus, execution RuntimeExecutionContract, admissibilities []RuntimeOperationAdmissibility) (RuntimeSemanticPlanManifest, error) {
	seed = seed.Clone()
	coverageIDs := make([]common.CoverageWorkItemID, 0, len(seed.CoverageItems))
	for _, item := range seed.CoverageItems {
		coverageIDs = append(coverageIDs, item.ID)
	}
	return CompileRuntimeSemanticPlanManifestForCoverage(workspaceID, runID, definition, seed, definitionFocuses, execution, admissibilities, coverageIDs)
}

// CompileRuntimeSemanticPlanManifestForCoverage compiles a manifest for the
// exact canonical CWI rows that remain eligible for semantic planning. The
// Definition seed hash stays whole-run identity; only provider-visible work
// items are narrowed. This is not a compatibility path and never recreates a
// settled CWI from historical focus state.
func CompileRuntimeSemanticPlanManifestForCoverage(workspaceID common.WorkspaceID, runID common.RunID, definition RuntimeDefinitionContext, seed RuntimeDefinitionSeed, definitionFocuses []RuntimeDefinitionFocus, execution RuntimeExecutionContract, admissibilities []RuntimeOperationAdmissibility, coverageIDs []common.CoverageWorkItemID) (RuntimeSemanticPlanManifest, error) {
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
	definitionFocuses = NormalizeRuntimeDefinitionFocuses(definitionFocuses)
	if err := ValidateRuntimeDefinitionFocuses(seed, definitionFocuses); err != nil {
		return RuntimeSemanticPlanManifest{}, err
	}
	if err := execution.Validate(); err != nil {
		return RuntimeSemanticPlanManifest{}, err
	}
	if seed.DefinitionRevision != definition.Revision || execution.OperationContractRevision == "" {
		return RuntimeSemanticPlanManifest{}, fmt.Errorf("runtime semantic manifest inputs have mismatched definition or operation contract identity")
	}
	wantedCoverage := make(map[common.CoverageWorkItemID]struct{}, len(coverageIDs))
	for _, coverageID := range coverageIDs {
		coverageID = common.CoverageWorkItemID(strings.TrimSpace(string(coverageID)))
		if coverageID == "" {
			return RuntimeSemanticPlanManifest{}, fmt.Errorf("runtime semantic manifest has an empty eligible coverage item")
		}
		if _, duplicate := wantedCoverage[coverageID]; duplicate {
			return RuntimeSemanticPlanManifest{}, fmt.Errorf("runtime semantic manifest duplicates eligible coverage item %q", coverageID)
		}
		wantedCoverage[coverageID] = struct{}{}
	}
	if len(wantedCoverage) == 0 {
		return RuntimeSemanticPlanManifest{}, fmt.Errorf("runtime semantic manifest has no eligible coverage items")
	}
	coverageItems := make([]RuntimeCoverageItem, 0, len(wantedCoverage))
	eligibleCoverage := make(map[common.CoverageWorkItemID]struct{}, len(wantedCoverage))
	for coverageID := range wantedCoverage {
		eligibleCoverage[coverageID] = struct{}{}
	}
	for _, item := range seed.CoverageItems {
		if _, found := wantedCoverage[item.ID]; !found {
			continue
		}
		// Admission prerequisites outside this manifest are already settled
		// canonical CWI facts. Keep the complete graph in DefinitionSeed and the
		// CWI ledger, but do not ask the provider to order or re-plan settled rows.
		prerequisites := make([]common.CoverageWorkItemID, 0, len(item.PrerequisiteCoverageWorkItemIDs))
		for _, prerequisiteID := range item.PrerequisiteCoverageWorkItemIDs {
			if _, stillEligible := eligibleCoverage[prerequisiteID]; stillEligible {
				prerequisites = append(prerequisites, prerequisiteID)
			}
		}
		item.PrerequisiteCoverageWorkItemIDs = prerequisites
		coverageItems = append(coverageItems, item)
		delete(wantedCoverage, item.ID)
	}
	if len(wantedCoverage) != 0 {
		for coverageID := range wantedCoverage {
			return RuntimeSemanticPlanManifest{}, fmt.Errorf("runtime semantic manifest eligible coverage %q is outside definition seed", coverageID)
		}
	}

	canonicalOwners := map[string]struct{}{
		RuntimeModuleFile: {}, RuntimeModuleShell: {}, RuntimeModuleMCP: {},
		RuntimeModuleGit: {}, RuntimeModuleWeb: {}, RuntimeModuleSSH: {},
	}
	owners := make([]string, 0)
	kinds := make([]string, 0)
	ownerSeen := map[string]struct{}{}
	kindSeen := map[string]struct{}{}
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
	// Owner catalogs retain every activation-sealed kind so their immutable
	// snapshot identity can be replayed and probed.  The provider-visible
	// vocabulary is narrower: an operation that structurally requires a
	// currently applicable owner fact must not be offered when that fact is
	// absent.  In particular, a managed-validation session without a confirmed
	// recipe is not a speculative action for semantic planning.
	selectableKinds := make([]string, 0, len(kinds))
	selectableOwnerSet := make(map[string]struct{}, len(owners))
	for _, kind := range kinds {
		descriptor, _ := RuntimeOperationDescriptorForKind(kind)
		catalog := catalogByOwner[descriptor.OwnerModuleID]
		if !runtimeSemanticOperationCurrentlySelectable(kind, catalog) {
			continue
		}
		selectableKinds = append(selectableKinds, kind)
		selectableOwnerSet[descriptor.OwnerModuleID] = struct{}{}
	}
	if len(selectableKinds) == 0 {
		return RuntimeSemanticPlanManifest{}, fmt.Errorf("runtime semantic manifest has no currently selectable owner operation")
	}
	selectableOwners := make([]string, 0, len(selectableOwnerSet))
	selectableCatalogs := make([]RuntimeOperationAdmissibility, 0, len(selectableOwnerSet))
	for _, owner := range owners {
		if _, selectable := selectableOwnerSet[owner]; !selectable {
			continue
		}
		selectableOwners = append(selectableOwners, owner)
		selectableCatalogs = append(selectableCatalogs, catalogByOwner[owner])
	}
	owners, kinds, sealedCatalogs = selectableOwners, selectableKinds, selectableCatalogs
	// Prospective target authority is an owner fact, not a technical descriptor
	// property. Compute the global policy vocabulary only after every owner
	// catalog has been sealed, and retain the narrower kind/variant facts in the
	// manifest for schema/evaluator use.
	policies := []RuntimeOperationTargetPolicy{
		RuntimeOperationTargetPolicyExactTarget,
		RuntimeOperationTargetPolicyExistingOwnerScope,
	}
	allowsProspectiveTarget := false
	for _, kind := range kinds {
		descriptor, found := RuntimeOperationDescriptorForKind(kind)
		if !found {
			continue
		}
		catalog, found := catalogByOwner[descriptor.OwnerModuleID]
		if !found {
			continue
		}
		variantIDs := runtimeSemanticCatalogVariantIDs(catalog, kind)
		if len(variantIDs) == 0 {
			allowed, _, _, _, err := RuntimeOperationAdmissibilityAllowsTargetPolicyForVariants(
				catalog, kind, nil, RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			)
			if err == nil && allowed {
				allowsProspectiveTarget = true
				break
			}
			continue
		}
		for _, variantID := range variantIDs {
			allowed, _, _, _, err := RuntimeOperationAdmissibilityAllowsTargetPolicyForVariants(
				catalog, kind, []string{variantID}, RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			)
			if err == nil && allowed {
				allowsProspectiveTarget = true
				break
			}
		}
		if allowsProspectiveTarget {
			break
		}
	}
	if allowsProspectiveTarget {
		policies = append(policies, RuntimeOperationTargetPolicyProspectiveOwnerTarget)
	}
	proofObligations, err := compileRuntimeSemanticProofObligations(definitionFocuses, coverageItems, sealedCatalogs)
	if err != nil {
		return RuntimeSemanticPlanManifest{}, err
	}
	dependencyCatalogs := make([]RuntimeOperationAdmissibility, 0)
	dependencyOwners := make(map[string]struct{})
	for _, obligation := range proofObligations {
		if obligation.OwnerFactAcquisition == nil || obligation.OwnerFactAcquisition.ProspectiveSatisfactionContractRef == "" {
			continue
		}
		acquisition := obligation.OwnerFactAcquisition.Normalized()
		owner := acquisition.Requirement.OwnerModuleID
		catalog, found := catalogByOwner[owner]
		if !found {
			return RuntimeSemanticPlanManifest{}, fmt.Errorf("runtime semantic manifest prospective owner-fact dependency %q has no sealed catalog", owner)
		}
		if len(runtimeProspectiveSatisfactionContractRecipeRefs(catalog, acquisition.Requirement, acquisition.ProspectiveSatisfactionContractRef)) == 0 {
			return RuntimeSemanticPlanManifest{}, fmt.Errorf("runtime semantic manifest prospective owner-fact dependency %q has no matching satisfaction surface", owner)
		}
		if _, duplicate := dependencyOwners[owner]; duplicate {
			continue
		}
		dependencyOwners[owner] = struct{}{}
		dependencyCatalogs = append(dependencyCatalogs, catalog)
	}
	manifest := RuntimeSemanticPlanManifest{
		SchemaVersion: RuntimeSemanticPlanManifestSchemaCurrent,
		WorkspaceID:   workspaceID, RunID: runID,
		DefinitionRevision: definition.Revision, DefinitionSeedHash: seed.Hash,
		OperationContractRevision: execution.OperationContractRevision,
		CoverageItems:             coverageItems,
		AllowedOwnerModuleIDs:     owners, AllowedOperationKinds: kinds,
		AllowedTargetPolicies: policies, ProofObligations: proofObligations, OwnerCatalogs: sealedCatalogs,
		OwnerFactDependencyCatalogs: dependencyCatalogs,
	}
	manifest.Hash = StableRuntimeSemanticPlanManifestHash(manifest)
	if err := manifest.Validate(); err != nil {
		return RuntimeSemanticPlanManifest{}, err
	}
	return manifest, nil
}

func runtimeSemanticOperationCurrentlySelectable(kind string, catalog RuntimeOperationAdmissibility) bool {
	kind = CanonicalRuntimeBlockKind(kind)
	catalog = catalog.Normalized()
	if kind == RuntimeBlockKindManagedValidationSession {
		return len(catalog.ValidationRecipes) != 0
	}
	return true
}

func runtimeSemanticCatalogVariantIDs(catalog RuntimeOperationAdmissibility, kind string) []string {
	kind = CanonicalRuntimeBlockKind(kind)
	ret := make([]string, 0)
	for _, value := range catalog.OperationVariants {
		value = value.Normalized()
		if value.Kind == kind && value.VariantID != "" {
			ret = append(ret, value.VariantID)
		}
	}
	return normalizeStrings(ret)
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
	m.ProofObligations = normalizeRuntimeSemanticProofObligations(m.ProofObligations)
	m.OwnerCatalogs = NormalizeRuntimeOperationAdmissibilities(m.OwnerCatalogs)
	m.OwnerFactDependencyCatalogs = NormalizeRuntimeOperationAdmissibilities(m.OwnerFactDependencyCatalogs)
	return m
}

func (m RuntimeSemanticPlanManifest) Clone() RuntimeSemanticPlanManifest {
	m = m.Normalized()
	m.CoverageItems = append([]RuntimeCoverageItem(nil), m.CoverageItems...)
	m.AllowedOwnerModuleIDs = append([]string(nil), m.AllowedOwnerModuleIDs...)
	m.AllowedOperationKinds = append([]string(nil), m.AllowedOperationKinds...)
	m.AllowedTargetPolicies = append([]RuntimeOperationTargetPolicy(nil), m.AllowedTargetPolicies...)
	m.ProofObligations = cloneRuntimeSemanticProofObligations(m.ProofObligations)
	m.OwnerCatalogs = cloneRuntimeSemanticManifestCatalogs(m.OwnerCatalogs)
	m.OwnerFactDependencyCatalogs = cloneRuntimeSemanticManifestCatalogs(m.OwnerFactDependencyCatalogs)
	return m
}

func cloneRuntimeSemanticManifestCatalogs(values []RuntimeOperationAdmissibility) []RuntimeOperationAdmissibility {
	values = append([]RuntimeOperationAdmissibility(nil), values...)
	for idx := range values {
		values[idx].OperationKinds = append([]string(nil), values[idx].OperationKinds...)
		values[idx].OperationVariants = append([]RuntimeOperationAdmissibilityVariant(nil), values[idx].OperationVariants...)
		for variantIdx := range values[idx].OperationVariants {
			values[idx].OperationVariants[variantIdx].AllowedTargetPolicies = append([]RuntimeOperationTargetPolicy(nil), values[idx].OperationVariants[variantIdx].AllowedTargetPolicies...)
			values[idx].OperationVariants[variantIdx].SelectorRefs = append([]string(nil), values[idx].OperationVariants[variantIdx].SelectorRefs...)
			values[idx].OperationVariants[variantIdx].ConstraintRefs = append([]string(nil), values[idx].OperationVariants[variantIdx].ConstraintRefs...)
		}
		values[idx].TargetRefs = append([]string(nil), values[idx].TargetRefs...)
		values[idx].SelectorRefs = append([]string(nil), values[idx].SelectorRefs...)
		values[idx].ConstraintRefs = append([]string(nil), values[idx].ConstraintRefs...)
		values[idx].ValidationRecipeRefs = append([]string(nil), values[idx].ValidationRecipeRefs...)
		values[idx].ValidationSurfaces = NormalizeRuntimeValidationSurfaces(values[idx].ValidationSurfaces)
		values[idx].OperationPolicies = NormalizeRuntimeOperationPolicyFacts(values[idx].OperationPolicies)
		values[idx].ValidationRecipes = append([]RuntimeValidationRecipe(nil), values[idx].ValidationRecipes...)
		for recipeIdx := range values[idx].ValidationRecipes {
			values[idx].ValidationRecipes[recipeIdx].Args = append([]string(nil), values[idx].ValidationRecipes[recipeIdx].Args...)
		}
	}
	return nilIfEmpty(values)
}

func (m RuntimeSemanticPlanManifest) Validate() error {
	m = m.Normalized()
	if m.SchemaVersion != RuntimeSemanticPlanManifestSchemaCurrent || m.WorkspaceID == "" || m.RunID == "" || m.DefinitionRevision == "" || m.DefinitionSeedHash == "" || m.OperationContractRevision == "" || len(m.CoverageItems) == 0 {
		return fmt.Errorf("runtime semantic manifest requires current schema, scope, definition, seed, operation contract and coverage")
	}
	if len(m.AllowedOwnerModuleIDs) == 0 || len(m.AllowedOperationKinds) == 0 || len(m.AllowedTargetPolicies) == 0 || len(m.ProofObligations) == 0 || len(m.OwnerCatalogs) == 0 {
		return fmt.Errorf("runtime semantic manifest requires bounded owners, operations, target policies, proof obligations and owner catalogs")
	}
	coverage := make(map[common.CoverageWorkItemID]struct{}, len(m.CoverageItems))
	coverageItems := make(map[common.CoverageWorkItemID]RuntimeCoverageItem, len(m.CoverageItems))
	for _, item := range m.CoverageItems {
		coverage[item.ID] = struct{}{}
		coverageItems[item.ID] = item
	}
	obligationCoverage := make(map[common.CoverageWorkItemID]struct{}, len(m.ProofObligations))
	for _, obligation := range m.ProofObligations {
		if err := obligation.Validate(); err != nil {
			return err
		}
		if _, found := coverage[obligation.CoverageWorkItemID]; !found {
			return fmt.Errorf("runtime semantic manifest obligation coverage %q is outside manifest", obligation.CoverageWorkItemID)
		}
		if _, duplicate := obligationCoverage[obligation.CoverageWorkItemID]; duplicate {
			return fmt.Errorf("runtime semantic manifest duplicates obligation coverage %q", obligation.CoverageWorkItemID)
		}
		recompiled, err := compileRuntimeSemanticProofObligations(
			CompileRuntimeDefinitionFocuses([]CoverageFocus{runtimeSemanticProofObligationSourceFocus(obligation)}),
			[]RuntimeCoverageItem{coverageItems[obligation.CoverageWorkItemID]},
			m.OwnerCatalogs,
		)
		if err != nil {
			return fmt.Errorf("runtime semantic manifest obligation coverage %q is not reproducible from sealed owner facts: %w", obligation.CoverageWorkItemID, err)
		}
		if !sameRuntimeSemanticProofObligations([]RuntimeSemanticProofObligation{obligation}, recompiled) {
			return fmt.Errorf("runtime semantic manifest obligation coverage %q does not match sealed owner facts", obligation.CoverageWorkItemID)
		}
		obligationCoverage[obligation.CoverageWorkItemID] = struct{}{}
	}
	if len(obligationCoverage) != len(coverage) {
		return fmt.Errorf("runtime semantic manifest does not seal every coverage proof obligation")
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
	dependencyOwners := make(map[string]RuntimeOperationAdmissibility, len(m.OwnerFactDependencyCatalogs))
	for _, catalog := range m.OwnerFactDependencyCatalogs {
		if err := catalog.Validate(); err != nil {
			return err
		}
		if _, duplicate := dependencyOwners[catalog.OwnerModuleID]; duplicate {
			return fmt.Errorf("runtime semantic manifest duplicates owner-fact dependency catalog owner %q", catalog.OwnerModuleID)
		}
		dependencyOwners[catalog.OwnerModuleID] = catalog
	}
	referencedDependencies := make(map[string]struct{})
	for _, obligation := range m.ProofObligations {
		if obligation.OwnerFactAcquisition == nil || obligation.OwnerFactAcquisition.ProspectiveSatisfactionContractRef == "" {
			continue
		}
		acquisition := obligation.OwnerFactAcquisition.Normalized()
		owner := acquisition.Requirement.OwnerModuleID
		catalog, found := dependencyOwners[owner]
		if !found {
			return fmt.Errorf("runtime semantic manifest prospective owner-fact acquisition requires dependency catalog %q", owner)
		}
		if len(runtimeProspectiveSatisfactionContractRecipeRefs(catalog, acquisition.Requirement, acquisition.ProspectiveSatisfactionContractRef)) == 0 {
			return fmt.Errorf("runtime semantic manifest owner-fact dependency catalog %q does not seal the prospective satisfaction contract", owner)
		}
		referencedDependencies[owner] = struct{}{}
	}
	if len(referencedDependencies) != len(dependencyOwners) {
		return fmt.Errorf("runtime semantic manifest contains an unreferenced owner-fact dependency catalog")
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
	writeRuntimeSemanticProofObligationsHash(h, manifest.ProofObligations)
	for _, catalog := range manifest.OwnerCatalogs {
		writeHashString(h, catalog.SnapshotID)
	}
	for _, catalog := range manifest.OwnerFactDependencyCatalogs {
		writeHashString(h, catalog.SnapshotID)
	}
	return "runtime-semantic-manifest-" + hex.EncodeToString(h.Sum(nil))[:20]
}

// StableRuntimeSemanticGenerationID binds semantic authority to the exact
// catalog snapshot and logical provider request that produced it. A resource
// rollback may reproduce the same manifest hash, so the manifest alone is not
// a sufficient generation identity for a focus or evidence binding.
func StableRuntimeSemanticGenerationID(manifestHash, logicalRequestID string) string {
	manifestHash = strings.TrimSpace(manifestHash)
	logicalRequestID = strings.TrimSpace(logicalRequestID)
	if manifestHash == "" || logicalRequestID == "" {
		return ""
	}
	h := sha256.New()
	writeHashString(h, "runtime-semantic-generation")
	writeHashString(h, manifestHash)
	writeHashString(h, logicalRequestID)
	return "runtime-semantic-generation-" + hex.EncodeToString(h.Sum(nil))[:20]
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
	DeclarationRef        string
}

// RuntimeSemanticProposalConstraintError is the pure proposal-contract fact
// shared by admission and provider correction. It carries no retry decision.
type RuntimeSemanticProposalConstraintError struct {
	Field    string
	Rule     RuntimeSemanticConstraintRule
	Actual   string
	Required []string
	Allowed  []string
}

func (e *RuntimeSemanticProposalConstraintError) Error() string {
	if e == nil {
		return "runtime semantic proposal violates its contract"
	}
	if e.Field == "ownerModuleId" && e.Rule == RuntimeSemanticConstraintRelational {
		return "runtime semantic proposal operation does not belong to owner"
	}
	return fmt.Sprintf("runtime semantic proposal field %q violates %s", e.Field, e.Rule)
}

func runtimeSemanticProposalConstraint(field string, rule RuntimeSemanticConstraintRule, actual string, required, allowed []string) error {
	return &RuntimeSemanticProposalConstraintError{Field: field, Rule: rule, Actual: actual, Required: required, Allowed: allowed}
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
	p.DeclarationRef = strings.TrimSpace(p.DeclarationRef)
	return p
}

func (p RuntimeSemanticProofProposal) Clone() RuntimeSemanticProofProposal {
	p = p.Normalized()
	p.Target = p.Target.Normalized()
	p.RequiredEvidenceKinds = append([]string(nil), p.RequiredEvidenceKinds...)
	p.RequiredEffectScopes = append([]RuntimeEffectScope(nil), p.RequiredEffectScopes...)
	p.RequiredCheckRefs = append([]string(nil), p.RequiredCheckRefs...)
	return p
}

func (p RuntimeSemanticProofProposal) Validate() error {
	rawEvidenceKinds := append([]string(nil), p.RequiredEvidenceKinds...)
	p = p.Normalized()
	for _, value := range []struct {
		field string
		value string
	}{
		{"coverageWorkItemId", string(p.CoverageWorkItemID)},
		{"ownerModuleId", p.OwnerModuleID},
		{"operationKind", p.OperationKind},
		{"declarationRef", p.DeclarationRef},
	} {
		if value.value == "" {
			return runtimeSemanticProposalConstraint(value.field, RuntimeSemanticConstraintNonEmpty, "empty", []string{"non-empty string"}, nil)
		}
	}
	if len(rawEvidenceKinds) != len(p.RequiredEvidenceKinds) || len(p.RequiredEvidenceKinds) != 1 {
		return runtimeSemanticProposalConstraint("requiredEvidenceKinds", RuntimeSemanticConstraintRelational, strings.Join(p.RequiredEvidenceKinds, ","), []string{"exactly one evidence kind per proof"}, nil)
	}
	if len(p.RequiredEffectScopes) == 0 {
		return runtimeSemanticProposalConstraint("requiredEffectScopes", RuntimeSemanticConstraintNonEmpty, "empty_array", []string{"at least one effect scope"}, nil)
	}
	descriptor, found := RuntimeOperationDescriptorForKind(p.OperationKind)
	if !found || descriptor.OwnerModuleID != p.OwnerModuleID {
		return runtimeSemanticProposalConstraint("ownerModuleId", RuntimeSemanticConstraintRelational, p.OwnerModuleID, []string{descriptor.OwnerModuleID + " owns " + p.OperationKind}, nil)
	}
	if p.OperationVariantID != "" {
		contract, contractFound := RuntimeOperationContractForKind(p.OperationKind)
		if !contractFound || len(contract.Variants) == 0 {
			// Dynamic operations use the owner recipe/capability identity as their
			// discriminator.  A provider-supplied technical variant is invalid,
			// but the correction must not receive a process-wide variant catalog.
			return runtimeSemanticProposalConstraint("operationVariantId", RuntimeSemanticConstraintAllowedValue, p.OperationVariantID,
				[]string{"omit for dynamic operation; use owner recipe/capability identity"}, nil)
		}
		// Static operation variants are owner-admissibility facts.  Do not check
		// membership against the executable operation contract here: doing so
		// would expose the broader technical variant set before the sealed owner
		// catalog is consulted.  ValidateAgainstManifest and the owner evaluator
		// perform that scoped check and return only owner-published values.
	} else if contract, contractFound := RuntimeOperationContractForKind(p.OperationKind); contractFound && len(contract.Variants) != 0 {
		return runtimeSemanticProposalConstraint("operationVariantId", RuntimeSemanticConstraintRequired, "empty", []string{"concrete operation variant for static operation kind"}, nil)
	}
	if err := validateRuntimeSemanticProposalTarget(p.Target); err != nil {
		return err
	}
	for _, kind := range p.RequiredEvidenceKinds {
		switch kind {
		case EvidenceKindStateChange:
			if !runtimeSemanticHasDurableEffect(p.RequiredEffectScopes) {
				return runtimeSemanticProposalConstraint("requiredEffectScopes", RuntimeSemanticConstraintRelational, strings.Join(RuntimeEffectScopesAsStrings(p.RequiredEffectScopes), ","), []string{"durable effect for state_change"}, nil)
			}
		case EvidenceKindValidation:
			if !runtimeSemanticHasNonDurableEffect(p.RequiredEffectScopes) {
				return runtimeSemanticProposalConstraint("requiredEffectScopes", RuntimeSemanticConstraintRelational, strings.Join(RuntimeEffectScopesAsStrings(p.RequiredEffectScopes), ","), []string{"non-durable effect for validation"}, nil)
			}
			if !RuntimeOperationKindSupportsCoverageEvidence(p.OperationKind, EvidenceKindValidation) {
				return runtimeSemanticProposalConstraint("operationKind", RuntimeSemanticConstraintRelational, p.OperationKind, []string{StepKindMCPToolCall, RuntimeBlockKindManagedValidationSession}, nil)
			}
			if p.ValidationRecipeID == "" {
				return runtimeSemanticProposalConstraint("validationRecipeId", RuntimeSemanticConstraintRequired, "empty", []string{"non-empty recipe for validation"}, nil)
			}
			// Validation checks are capability-owner facts. The provider may not
			// declare them; SM-2 requires the matching owner observation before
			// compiling a binding.
		default:
			return runtimeSemanticProposalConstraint("requiredEvidenceKinds", RuntimeSemanticConstraintAllowedValue, kind, nil, []string{EvidenceKindStateChange, EvidenceKindValidation})
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
	raw := d
	d = d.Normalized()
	manifest = manifest.Normalized()
	// Batch membership and provider order are identity-bearing fields.  A
	// decoder/ledger validator must reject duplicates and empty entries rather
	// than normalize them away and accidentally validate a different document.
	if len(raw.BatchCoverageWorkItemIDs) != len(d.BatchCoverageWorkItemIDs) ||
		len(raw.CoverageOrder) != len(d.CoverageOrder) ||
		len(raw.Proposals) != len(d.Proposals) {
		return fmt.Errorf("runtime semantic document contains duplicate or empty identity values")
	}
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
	items := make(map[common.CoverageWorkItemID]struct{}, len(manifest.CoverageItems))
	for _, item := range manifest.CoverageItems {
		items[item.ID] = struct{}{}
	}
	switch d.PlanStep {
	case RuntimeSemanticPlanStepCoverageOrder:
		if len(d.CoverageOrder) != len(manifest.CoverageItems) || len(d.Proposals) != 0 || d.BatchID != "" || len(d.BatchCoverageWorkItemIDs) != 0 || d.BatchSequence != 0 {
			return fmt.Errorf("coverage-order semantic plan requires full order and no proposals")
		}
	case RuntimeSemanticPlanStepProofBatch:
		if d.BatchID == "" || len(d.BatchCoverageWorkItemIDs) == 0 || d.BatchSequence < 0 || len(d.CoverageOrder) != 0 {
			return fmt.Errorf("proof-batch semantic plan requires batch identity, membership and no global coverage order")
		}
	default:
		return fmt.Errorf("runtime semantic plan has invalid step %q", d.PlanStep)
	}
	if d.PlanStep == RuntimeSemanticPlanStepProofBatch {
		batchCoverage := make(map[common.CoverageWorkItemID]struct{}, len(d.BatchCoverageWorkItemIDs))
		seenProposalCoverage := make(map[common.CoverageWorkItemID]struct{}, len(d.Proposals))
		for _, id := range d.BatchCoverageWorkItemIDs {
			if _, known := items[id]; !known {
				return fmt.Errorf("runtime semantic batch document coverage %q is outside manifest", id)
			}
			if _, duplicate := batchCoverage[id]; duplicate {
				return fmt.Errorf("runtime semantic batch document duplicates coverage %q", id)
			}
			batchCoverage[id] = struct{}{}
		}
		for _, proposal := range d.Proposals {
			if _, ok := batchCoverage[proposal.CoverageWorkItemID]; !ok {
				return fmt.Errorf("runtime semantic batch document proposal %q is outside batch", proposal.CoverageWorkItemID)
			}
			seenProposalCoverage[proposal.CoverageWorkItemID] = struct{}{}
		}
		if len(batchCoverage) != len(seenProposalCoverage) {
			return fmt.Errorf("runtime semantic batch document does not cover its batch exactly")
		}
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
	seenCoverage := make(map[common.CoverageWorkItemID]struct{}, len(d.Proposals))
	seenProofKinds := make(map[string]struct{}, len(d.Proposals))
	for proposalIndex, proposal := range d.Proposals {
		if err := proposal.Validate(); err != nil {
			var constraint *RuntimeSemanticProposalConstraintError
			if errors.As(err, &constraint) {
				allowed := append([]string(nil), constraint.Allowed...)
				if len(allowed) == 0 {
					if catalog, found := catalogs[proposal.OwnerModuleID]; found {
						switch constraint.Field {
						case "target.exactTargetRefs", "target.ownerScopeTargetRefs":
							allowed = append([]string(nil), catalog.TargetRefs...)
						case "target.ownerSelectorRefs":
							allowed = append([]string(nil), catalog.SelectorRefs...)
						case "target.prospectiveConstraintRefs":
							allowed = append([]string(nil), catalog.ConstraintRefs...)
						case "operationVariantId":
							allowed = runtimeSemanticCatalogVariantIDs(catalog, proposal.OperationKind)
						}
					}
				}
				return &RuntimeSemanticProposalViolationError{ViolationFact: RuntimeSemanticContractViolation{
					Kind: RuntimeSemanticContractViolationProposalConstraint, Path: fmt.Sprintf("$.proposals[%d].%s", proposalIndex, constraint.Field),
					CoverageWorkItemID: proposal.CoverageWorkItemID, ProposalIndex: proposalIndex, Rule: constraint.Rule,
					Actual: constraint.Actual, Required: append([]string(nil), constraint.Required...), Allowed: allowed,
				}, Cause: err}
			}
			return err
		}
		if _, found := items[proposal.CoverageWorkItemID]; !found {
			return fmt.Errorf("runtime semantic document proposal coverage %q is outside manifest", proposal.CoverageWorkItemID)
		}
		proofKind := proposal.RequiredEvidenceKinds[0]
		proofKey := string(proposal.CoverageWorkItemID) + "\x00" + proofKind
		if _, duplicate := seenProofKinds[proofKey]; duplicate {
			return runtimeSemanticProposalViolation(proposal, proposalIndex, "requiredEvidenceKinds", RuntimeSemanticConstraintRelational,
				proofKind, []string{"one proof per evidence kind for each CWI"}, nil,
				fmt.Errorf("runtime semantic document duplicates proof kind %q for coverage %q", proofKind, proposal.CoverageWorkItemID))
		}
		seenProofKinds[proofKey] = struct{}{}
		seenCoverage[proposal.CoverageWorkItemID] = struct{}{}
		if _, allowed := owners[proposal.OwnerModuleID]; !allowed {
			return runtimeSemanticProposalViolation(proposal, proposalIndex, "ownerModuleId", RuntimeSemanticConstraintAllowedValue, proposal.OwnerModuleID, nil, manifest.AllowedOwnerModuleIDs, fmt.Errorf("runtime semantic document owner %q is outside manifest", proposal.OwnerModuleID))
		}
		if _, allowed := kinds[proposal.OperationKind]; !allowed {
			return runtimeSemanticProposalViolation(proposal, proposalIndex, "operationKind", RuntimeSemanticConstraintAllowedValue, proposal.OperationKind, nil, manifest.AllowedOperationKinds, fmt.Errorf("runtime semantic document operation %q is outside manifest", proposal.OperationKind))
		}
		if _, allowed := policies[proposal.Target.Policy]; !allowed {
			return runtimeSemanticProposalViolation(proposal, proposalIndex, "target.policy", RuntimeSemanticConstraintAllowedValue, string(proposal.Target.Policy), nil, runtimeSemanticTargetPoliciesAsStrings(manifest.AllowedTargetPolicies), fmt.Errorf("runtime semantic document target policy %q is outside manifest", proposal.Target.Policy))
		}
		catalog, found := catalogs[proposal.OwnerModuleID]
		if !found || !runtimeOperationKindsContain(catalog.OperationKinds, proposal.OperationKind) {
			allowedKinds := []string(nil)
			if found {
				allowedKinds = catalog.OperationKinds
			}
			return runtimeSemanticProposalViolation(proposal, proposalIndex, "operationKind", RuntimeSemanticConstraintAllowedValue, proposal.OperationKind, nil, allowedKinds, fmt.Errorf("runtime semantic document proposal is outside owner catalog"))
		}
		if err := validateSemanticProposalAgainstOwnerCatalog(proposal, catalog); err != nil {
			var constraint *RuntimeSemanticProposalConstraintError
			if errors.As(err, &constraint) {
				return runtimeSemanticProposalViolation(proposal, proposalIndex, constraint.Field, constraint.Rule,
					constraint.Actual, constraint.Required, constraint.Allowed, err)
			}
			return runtimeSemanticProposalViolation(proposal, proposalIndex, "operationVariantId", RuntimeSemanticConstraintRelational,
				proposal.OperationVariantID, nil, nil, err)
		}
		if len(catalog.ValidationRecipeRefs) == 0 && containsRuntimeSemanticEvidenceKind(proposal.RequiredEvidenceKinds, EvidenceKindValidation) {
			return runtimeSemanticProposalViolation(proposal, proposalIndex, "requiredEvidenceKinds", RuntimeSemanticConstraintRelational, EvidenceKindValidation, []string{EvidenceKindStateChange}, nil, fmt.Errorf("runtime semantic document owner %q does not provide validation recipes", catalog.OwnerModuleID))
		}
	}
	if d.PlanStep == RuntimeSemanticPlanStepCoverageOrder {
		// The order step intentionally carries no proof proposal.
	} else if d.BatchID == "" {
		if len(seenCoverage) != len(items) {
			return fmt.Errorf("runtime semantic document does not cover every required CWI")
		}
	}
	if d.PlanStep == RuntimeSemanticPlanStepProofBatch {
		if err := validateRuntimeSemanticPlanProofObligations(d, manifest); err != nil {
			return err
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

func containsRuntimeSemanticEvidenceKind(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func runtimeSemanticProposalViolation(proposal RuntimeSemanticProofProposal, proposalIndex int, path string, rule RuntimeSemanticConstraintRule, actual string, required, allowed []string, cause error) error {
	return &RuntimeSemanticProposalViolationError{
		ViolationFact: RuntimeSemanticContractViolation{
			Kind: RuntimeSemanticContractViolationProposalConstraint,
			Path: fmt.Sprintf("$.proposals[%d].%s", proposalIndex, path), CoverageWorkItemID: proposal.CoverageWorkItemID,
			ProposalIndex: proposalIndex, Rule: rule, Actual: actual, Required: append([]string(nil), required...), Allowed: append([]string(nil), allowed...),
		}, Cause: cause,
	}
}

func runtimeSemanticTargetPoliciesAsStrings(values []RuntimeOperationTargetPolicy) []string {
	ret := make([]string, 0, len(values))
	for _, value := range values {
		ret = append(ret, string(value))
	}
	return normalizeStrings(ret)
}

func runtimeSemanticVariantIDs(contract RuntimeOperationContract) []string {
	ret := make([]string, 0, len(contract.Variants))
	for _, variant := range contract.Variants {
		ret = append(ret, variant.ID)
	}
	return normalizeStrings(ret)
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
	SchemaVersion                               string
	ManifestHash                                string
	DefinitionSeedHash                          string
	CoverageWorkItemID                          common.CoverageWorkItemID
	ProposalHash                                string
	OwnerModuleID                               string
	CatalogSnapshotID                           string
	CatalogObservationID                        string
	OwnerPolicy                                 RuntimeOwnerPolicyIdentity
	Status                                      RuntimeSemanticFeasibilityStatus
	ObservedRevision                            string
	TargetRefs                                  []string
	ValidationRecipeID                          string
	RequiredCheckRefs                           []string
	ProspectiveWorkspaceManifestDelta           *RuntimeProspectiveWorkspaceManifestDelta
	OwnerFactProspectiveSatisfactionObservation *RuntimeOwnerFactProspectiveSatisfactionObservation
	OriginalDiagnostic                          *RuntimeOriginalDiagnostic
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
	f.OwnerPolicy = f.OwnerPolicy.Normalized()
	f.Status = RuntimeSemanticFeasibilityStatus(strings.TrimSpace(string(f.Status)))
	f.ObservedRevision = strings.TrimSpace(f.ObservedRevision)
	f.TargetRefs = normalizeStrings(f.TargetRefs)
	f.ValidationRecipeID = strings.TrimSpace(f.ValidationRecipeID)
	f.RequiredCheckRefs = normalizeStrings(f.RequiredCheckRefs)
	if f.ProspectiveWorkspaceManifestDelta != nil {
		value := f.ProspectiveWorkspaceManifestDelta.Normalized()
		f.ProspectiveWorkspaceManifestDelta = &value
	}
	if f.OwnerFactProspectiveSatisfactionObservation != nil {
		value := f.OwnerFactProspectiveSatisfactionObservation.Normalized()
		f.OwnerFactProspectiveSatisfactionObservation = &value
	}
	f.OriginalDiagnostic = f.OriginalDiagnostic.Normalized()
	return f
}

func (f RuntimeSemanticProofFeasibility) Clone() RuntimeSemanticProofFeasibility {
	originalDiagnostic := f.OriginalDiagnostic.CloneOriginal()
	f = f.Normalized()
	f.TargetRefs = append([]string(nil), f.TargetRefs...)
	f.RequiredCheckRefs = append([]string(nil), f.RequiredCheckRefs...)
	if f.ProspectiveWorkspaceManifestDelta != nil {
		value := f.ProspectiveWorkspaceManifestDelta.Clone()
		f.ProspectiveWorkspaceManifestDelta = &value
	}
	if f.OwnerFactProspectiveSatisfactionObservation != nil {
		value := f.OwnerFactProspectiveSatisfactionObservation.Clone()
		f.OwnerFactProspectiveSatisfactionObservation = &value
	}
	f.OriginalDiagnostic = originalDiagnostic
	return f
}

// ValidateOwnerForProposal validates the addressed capability owner's direct
// observation. For a prospective owner-fact acquisition the File-authored
// manifest delta is mandatory, while the required-fact owner's later
// satisfaction observation may still be absent.
func (f RuntimeSemanticProofFeasibility) ValidateOwnerForProposal(manifest RuntimeSemanticPlanManifest, proposal RuntimeSemanticProofProposal) error {
	if f.OwnerFactProspectiveSatisfactionObservation != nil {
		return fmt.Errorf("capability-owner semantic feasibility cannot carry another owner's prospective satisfaction observation")
	}
	return f.validateForProposal(manifest, proposal, false, false, false)
}

// ValidateCompositeForProposal accepts a complete pair of owner observations,
// including typed required-owner stale and would-not-satisfy results. It is
// the persistence gate; ValidateForProposal remains the stricter seal-ready
// gate so a factual rejection can never grant operation authority.
func (f RuntimeSemanticProofFeasibility) ValidateCompositeForProposal(manifest RuntimeSemanticPlanManifest, proposal RuntimeSemanticProofProposal) error {
	return f.validateForProposal(manifest, proposal, true, true, true)
}

func (f RuntimeSemanticProofFeasibility) ValidateForProposal(manifest RuntimeSemanticPlanManifest, proposal RuntimeSemanticProofProposal) error {
	return f.validateForProposal(manifest, proposal, true, false, false)
}

func (f RuntimeSemanticProofFeasibility) validateForProposal(manifest RuntimeSemanticPlanManifest, proposal RuntimeSemanticProofProposal, requireProspectiveObservation, allowProspectiveStale, allowProspectiveWouldNotSatisfy bool) error {
	f = f.Normalized()
	manifest = manifest.Normalized()
	proposal = proposal.Normalized()
	if err := manifest.Validate(); err != nil {
		return err
	}
	if f.SchemaVersion != RuntimeSemanticFeasibilitySchemaCurrent || f.ManifestHash != manifest.Hash || f.DefinitionSeedHash != manifest.DefinitionSeedHash ||
		f.CoverageWorkItemID != proposal.CoverageWorkItemID || f.ProposalHash != StableRuntimeSemanticProofProposalHash(proposal) || f.OwnerModuleID != proposal.OwnerModuleID || f.CatalogSnapshotID == "" || f.ObservedRevision == "" {
		return fmt.Errorf("runtime semantic feasibility identity does not match proposal")
	}
	var catalog RuntimeOperationAdmissibility
	for _, candidate := range manifest.OwnerCatalogs {
		if candidate.OwnerModuleID == f.OwnerModuleID {
			catalog = candidate.Normalized()
			break
		}
	}
	if catalog.OwnerModuleID == "" || f.CatalogSnapshotID != catalog.SnapshotID {
		return fmt.Errorf("runtime semantic feasibility catalog identity does not match sealed owner catalog")
	}
	if f.OwnerPolicy.OwnerModuleID != "" || f.OwnerPolicy.PolicyRevision != "" || f.OwnerPolicy.CatalogSnapshotID != "" {
		if err := f.OwnerPolicy.Validate(); err != nil {
			return err
		}
		if f.OwnerPolicy.OwnerModuleID != f.OwnerModuleID || f.OwnerPolicy.CatalogSnapshotID != f.CatalogSnapshotID {
			return fmt.Errorf("runtime semantic feasibility owner policy identity mismatch")
		}
	}
	if err := validateSemanticProposalAgainstOwnerCatalog(proposal, catalog); err != nil {
		return err
	}
	acquisition, hasAcquisition := runtimeSemanticOwnerFactAcquisitionForCoverage(manifest, proposal.CoverageWorkItemID)
	prospectiveAcquisition := hasAcquisition && acquisition.AcquisitionTargetPolicy == RuntimeOperationTargetPolicyProspectiveOwnerTarget
	switch f.Status {
	case RuntimeSemanticFeasibilityFeasible:
		if f.OriginalDiagnostic != nil {
			return fmt.Errorf("feasible runtime semantic observation must not carry a failure diagnostic")
		}
		if f.ValidationRecipeID != proposal.ValidationRecipeID {
			return fmt.Errorf("feasible runtime semantic observation recipe does not match proposal")
		}
		if f.ValidationRecipeID != "" && !runtimeSemanticStringsWithin([]string{f.ValidationRecipeID}, catalog.ValidationRecipeRefs) {
			return fmt.Errorf("feasible runtime semantic observation recipe is outside sealed owner catalog")
		}
		checkCatalog := []string(nil)
		if surface, found := RuntimeValidationSurfaceForID(catalog.ValidationSurfaces, f.ValidationRecipeID); found {
			surface = surface.Normalized()
			if surface.Readiness == RuntimeValidationSurfaceReady && surface.ProofApplicability == RuntimeValidationProofDirect {
				checkCatalog = append(checkCatalog, surface.CheckRefs...)
			}
		}
		if len(f.RequiredCheckRefs) != 0 && !runtimeSemanticStringsWithin(f.RequiredCheckRefs, checkCatalog) {
			return fmt.Errorf("feasible runtime semantic observation check is outside sealed owner catalog")
		}
		target := proposal.Target.Normalized()
		switch target.Policy {
		case RuntimeOperationTargetPolicyExactTarget:
			if !sameStrings(f.TargetRefs, target.ExactTargetRefs) {
				return fmt.Errorf("feasible runtime semantic observation target does not match exact proposal target")
			}
		case RuntimeOperationTargetPolicyExistingOwnerScope:
			if len(target.OwnerScopeTargetRefs) != 0 && !sameStrings(f.TargetRefs, target.OwnerScopeTargetRefs) {
				return fmt.Errorf("feasible runtime semantic observation target does not match owner scope proposal target")
			}
		case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
			if !sameStrings(f.TargetRefs, []string{target.ProspectiveTargetRef}) {
				return fmt.Errorf("feasible runtime semantic observation target does not match prospective proposal target")
			}
		}
		if target.Policy != RuntimeOperationTargetPolicyProspectiveOwnerTarget && len(catalog.TargetRefs) != 0 && !runtimeSemanticStringsWithin(f.TargetRefs, catalog.TargetRefs) {
			return fmt.Errorf("feasible runtime semantic observation target is outside sealed owner catalog")
		}
		for _, kind := range proposal.RequiredEvidenceKinds {
			if kind == EvidenceKindValidation && (f.ValidationRecipeID == "" || len(f.RequiredCheckRefs) == 0) {
				return fmt.Errorf("feasible runtime semantic observation requires owner-confirmed validation checks")
			}
		}
		if prospectiveAcquisition {
			if f.CatalogObservationID == "" {
				return fmt.Errorf("feasible prospective owner-fact acquisition requires the File catalog observation identity")
			}
			if f.ProspectiveWorkspaceManifestDelta == nil {
				return fmt.Errorf("feasible prospective owner-fact acquisition requires a File workspace manifest delta")
			}
			delta := f.ProspectiveWorkspaceManifestDelta.Normalized()
			if err := delta.Validate(); err != nil {
				return err
			}
			if err := validateRuntimeProspectiveWorkspaceManifestDeltaForProposal(delta, manifest, proposal, acquisition); err != nil {
				return err
			}
			if f.OwnerFactProspectiveSatisfactionObservation == nil {
				if requireProspectiveObservation {
					return fmt.Errorf("feasible prospective owner-fact acquisition requires a required-owner observation")
				}
				break
			}
			prospectiveObservation := f.OwnerFactProspectiveSatisfactionObservation.Normalized()
			if err := validateRuntimeOwnerFactProspectiveSatisfactionObservationForProposal(prospectiveObservation, manifest, proposal, acquisition, delta); err != nil {
				return err
			}
			if prospectiveObservation.Stale != nil && !allowProspectiveStale {
				return fmt.Errorf("feasible prospective owner-fact acquisition cannot seal a stale required-owner observation")
			}
			if prospectiveObservation.Satisfaction != nil && prospectiveObservation.Satisfaction.Normalized().Outcome != RuntimeOwnerFactProspectiveWouldSatisfy && !allowProspectiveWouldNotSatisfy {
				return fmt.Errorf("feasible prospective owner-fact acquisition requires exact would_satisfy outcome")
			}
		} else if f.ProspectiveWorkspaceManifestDelta != nil || f.OwnerFactProspectiveSatisfactionObservation != nil {
			return fmt.Errorf("runtime semantic feasibility without prospective owner-fact acquisition cannot carry prospective satisfaction facts")
		}
	case RuntimeSemanticFeasibilityStale, RuntimeSemanticFeasibilityPrerequisiteRequired, RuntimeSemanticFeasibilityInfeasible:
		if f.ProspectiveWorkspaceManifestDelta != nil || f.OwnerFactProspectiveSatisfactionObservation != nil {
			return fmt.Errorf("non-feasible runtime semantic observation cannot carry prospective satisfaction facts")
		}
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
	RuntimeSemanticPlanningFeasibilityRequired           RuntimeSemanticPlanningPhase = "semantic_feasibility_required"
	RuntimeSemanticPlanningCorrectionRequired            RuntimeSemanticPlanningPhase = "semantic_plan_correction_required"
	RuntimeSemanticPlanningSealed                        RuntimeSemanticPlanningPhase = "semantic_contract_sealed"
	RuntimeSemanticPlanningFailed                        RuntimeSemanticPlanningPhase = "semantic_contract_failed"
)

// RuntimeSemanticPlanningRetirementReason identifies why an otherwise valid
// semantic planning generation stopped being current.  The value is audit
// lineage only; it never selects a retry, operation or terminal outcome.
type RuntimeSemanticPlanningRetirementReason string

const (
	RuntimeSemanticPlanningRetirementResourceInvalidated RuntimeSemanticPlanningRetirementReason = "resource_invalidated"
	RuntimeSemanticPlanningRetirementCatalogRefresh      RuntimeSemanticPlanningRetirementReason = "catalog_refresh"
	// RuntimeSemanticPlanningRetirementCWIReopened records a deliberate SM-2
	// credit invalidation. It is distinct from catalog invalidation: the owner
	// facts can remain current while exactly one settled CWI needs a successor
	// proof binding.
	RuntimeSemanticPlanningRetirementCWIReopened RuntimeSemanticPlanningRetirementReason = "cwi_reopened"
	// RuntimeSemanticPlanningRetirementBindingContradiction records an exact
	// owner-policy contradiction before any dispatch or effect.
	RuntimeSemanticPlanningRetirementBindingContradiction RuntimeSemanticPlanningRetirementReason = "binding_contradiction"
	// RuntimeSemanticPlanningRetirementEligibleClosureChanged records that a
	// settled prerequisite made a CWI eligible after the immutable manifest and
	// coverage order were accepted. The next generation must recompile the
	// complete closure; the old provider order is audit lineage only.
	RuntimeSemanticPlanningRetirementEligibleClosureChanged RuntimeSemanticPlanningRetirementReason = "eligible_closure_changed"
)

// RuntimeSemanticPlanningHistoryLimit bounds immutable semantic lineage kept
// inside the Runtime aggregate.  Exceeding the bound is a contract failure;
// history is never silently discarded to make a new generation fit.
const RuntimeSemanticPlanningHistoryLimit = 16

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
		if feasibility.Status == RuntimeSemanticFeasibilityStale ||
			(feasibility.OwnerFactProspectiveSatisfactionObservation != nil &&
				feasibility.OwnerFactProspectiveSatisfactionObservation.Stale != nil) {
			refreshable = true
		}
		if feasibility.OriginalDiagnostic != nil {
			if err := feasibility.OriginalDiagnostic.Validate(); err != nil {
				return err
			}
		}
	}
	if !refreshable {
		return fmt.Errorf("runtime semantic feasibility gap requires a stale owner fact")
	}
	for idx := range g.Rejections {
		if err := g.Rejections[idx].Validate(); err != nil {
			return fmt.Errorf("runtime semantic feasibility gap rejection %d: %w", idx, err)
		}
	}
	return nil
}

// RuntimeSemanticPlanningGeneration is an immutable audit snapshot of a
// superseded semantic planning ledger.  A receipt or owner freshness change
// may retire the active manifest/order, but it must not erase the provider
// document, owner facts, correction lineage or sealed binding identity that
// explains the already attempted generation.  The nested ledger is a complete
// typed snapshot; its own History must be empty so generations cannot form a
// recursive authority graph.
//
// Generation identity is the pair (Snapshot.Manifest.Hash,
// Snapshot.LogicalRequestID).  The manifest hash is the sealed catalog and
// definition identity; the logical request distinguishes a new semantic
// request after a resource rollback that happens to recreate an older catalog
// snapshot.  Logical request changes within one active generation (for
// example, the bounded correction request or the next focus batch) do not
// append history because they are still covered by the same retirement
// snapshot.  This keeps request-session identity and generation identity
// separate without allowing retired authority to be reused.
type RuntimeSemanticPlanningGeneration struct {
	RetiredAtStateRevision uint64
	Reason                 RuntimeSemanticPlanningRetirementReason
	InvalidationSignatures []string
	PrerequisiteIDs        []string
	// ReopenedCoverageIDs and ReopenTriggerRefs bind a CWI-reopen retirement
	// to the owner-local SM-2 facts that invalidated prior credit. They are
	// audit lineage only and never select an operation or a recovery action.
	ReopenedCoverageIDs []common.CoverageWorkItemID
	ReopenTriggerRefs   []string
	// ChangedCoverageIDs and ClosureChangeTriggerRefs bind an eligibility
	// change to the exact settled prerequisite binding facts that unlocked it.
	// They do not select the next focus or grant proof authority.
	ChangedCoverageIDs       []common.CoverageWorkItemID
	ClosureChangeTriggerRefs []string
	Snapshot                 RuntimeSemanticPlanningLedger
}

func (g RuntimeSemanticPlanningGeneration) Normalized() RuntimeSemanticPlanningGeneration {
	g.Reason = RuntimeSemanticPlanningRetirementReason(strings.TrimSpace(string(g.Reason)))
	g.InvalidationSignatures = normalizeStrings(g.InvalidationSignatures)
	g.PrerequisiteIDs = normalizeStrings(g.PrerequisiteIDs)
	g.ReopenedCoverageIDs = normalizeOrderedCoverageWorkItemIDs(g.ReopenedCoverageIDs)
	g.ReopenTriggerRefs = normalizeStrings(g.ReopenTriggerRefs)
	g.ChangedCoverageIDs = normalizeOrderedCoverageWorkItemIDs(g.ChangedCoverageIDs)
	g.ClosureChangeTriggerRefs = normalizeStrings(g.ClosureChangeTriggerRefs)
	// Detach the generation snapshot without recursively walking a malformed
	// nested History tree.  Validation below rejects any non-nil nested history;
	// normalization must remain bounded so a corrupt persisted record cannot
	// turn startup validation into an unbounded recursive clone.
	nestedHistory := append([]RuntimeSemanticPlanningGeneration(nil), g.Snapshot.History...)
	g.Snapshot.History = nil
	g.Snapshot = g.Snapshot.Clone()
	if nestedHistory != nil {
		g.Snapshot.History = nestedHistory
	}
	return g
}

func (g RuntimeSemanticPlanningGeneration) Clone() RuntimeSemanticPlanningGeneration {
	return g.Normalized()
}

func (g RuntimeSemanticPlanningGeneration) Validate() error {
	raw := g
	g = g.Normalized()
	if len(raw.InvalidationSignatures) != len(g.InvalidationSignatures) || len(raw.PrerequisiteIDs) != len(g.PrerequisiteIDs) ||
		len(raw.ReopenedCoverageIDs) != len(g.ReopenedCoverageIDs) || len(raw.ReopenTriggerRefs) != len(g.ReopenTriggerRefs) ||
		len(raw.ChangedCoverageIDs) != len(g.ChangedCoverageIDs) || len(raw.ClosureChangeTriggerRefs) != len(g.ClosureChangeTriggerRefs) {
		return fmt.Errorf("runtime semantic planning generation contains duplicate or empty lineage values")
	}
	if g.RetiredAtStateRevision == 0 || g.Reason == "" {
		return fmt.Errorf("runtime semantic planning generation requires retirement identity and reason")
	}
	switch g.Reason {
	case RuntimeSemanticPlanningRetirementResourceInvalidated, RuntimeSemanticPlanningRetirementCatalogRefresh:
		if len(g.InvalidationSignatures) == 0 || len(g.PrerequisiteIDs) == 0 || len(g.ReopenedCoverageIDs) != 0 || len(g.ReopenTriggerRefs) != 0 || len(g.ChangedCoverageIDs) != 0 || len(g.ClosureChangeTriggerRefs) != 0 {
			return fmt.Errorf("runtime semantic catalog retirement requires only catalog lineage")
		}
	case RuntimeSemanticPlanningRetirementCWIReopened, RuntimeSemanticPlanningRetirementBindingContradiction:
		if len(g.InvalidationSignatures) != 0 || len(g.PrerequisiteIDs) != 0 || len(g.ReopenedCoverageIDs) == 0 || len(g.ReopenTriggerRefs) == 0 || len(g.ChangedCoverageIDs) != 0 || len(g.ClosureChangeTriggerRefs) != 0 {
			return fmt.Errorf("runtime semantic CWI reopen retirement requires only CWI lineage")
		}
	case RuntimeSemanticPlanningRetirementEligibleClosureChanged:
		if len(g.InvalidationSignatures) != 0 || len(g.PrerequisiteIDs) != 0 || len(g.ReopenedCoverageIDs) != 0 || len(g.ReopenTriggerRefs) != 0 || len(g.ChangedCoverageIDs) == 0 || len(g.ClosureChangeTriggerRefs) == 0 {
			return fmt.Errorf("runtime semantic eligible-closure retirement requires only closure-change lineage")
		}
	default:
		return fmt.Errorf("runtime semantic planning generation has unsupported retirement reason %q", g.Reason)
	}
	if g.Snapshot.History != nil {
		return fmt.Errorf("runtime semantic planning generation snapshot must not contain nested history")
	}
	if g.Snapshot.Phase == RuntimeSemanticPlanningRequired || g.Snapshot.Phase == RuntimeSemanticPlanningFailed {
		return fmt.Errorf("runtime semantic planning generation snapshot is not an active semantic generation")
	}
	if g.Snapshot.Manifest == nil {
		return fmt.Errorf("runtime semantic planning generation snapshot requires a manifest")
	}
	if err := g.Snapshot.Validate(); err != nil {
		return fmt.Errorf("runtime semantic planning generation snapshot: %w", err)
	}
	manifestCatalogs := append([]RuntimeOperationAdmissibility(nil), g.Snapshot.Manifest.OwnerCatalogs...)
	manifestCatalogs = append(manifestCatalogs, g.Snapshot.Manifest.OwnerFactDependencyCatalogs...)
	manifestPrerequisites := make(map[string]struct{}, len(manifestCatalogs))
	for _, catalog := range manifestCatalogs {
		manifestPrerequisites[catalog.PrerequisiteID] = struct{}{}
	}
	seenPrerequisites := make(map[string]struct{}, len(g.PrerequisiteIDs))
	for _, prerequisiteID := range g.PrerequisiteIDs {
		if _, duplicate := seenPrerequisites[prerequisiteID]; duplicate {
			return fmt.Errorf("runtime semantic planning generation duplicates prerequisite %q", prerequisiteID)
		}
		seenPrerequisites[prerequisiteID] = struct{}{}
		if _, found := manifestPrerequisites[prerequisiteID]; !found {
			return fmt.Errorf("runtime semantic planning generation prerequisite %q is outside snapshot manifest", prerequisiteID)
		}
	}
	if g.Reason == RuntimeSemanticPlanningRetirementCWIReopened || g.Reason == RuntimeSemanticPlanningRetirementBindingContradiction || g.Reason == RuntimeSemanticPlanningRetirementEligibleClosureChanged {
		seedCoverage := make(map[common.CoverageWorkItemID]struct{}, len(g.Snapshot.DefinitionSeed.CoverageItems))
		for _, item := range g.Snapshot.DefinitionSeed.CoverageItems {
			seedCoverage[item.ID] = struct{}{}
		}
		for _, coverageID := range g.ReopenedCoverageIDs {
			if _, found := seedCoverage[coverageID]; !found {
				return fmt.Errorf("runtime semantic CWI reopen %q is outside snapshot definition seed", coverageID)
			}
		}
		for _, coverageID := range g.ChangedCoverageIDs {
			if _, found := seedCoverage[coverageID]; !found {
				return fmt.Errorf("runtime semantic closure change %q is outside snapshot definition seed", coverageID)
			}
		}
	}
	return nil
}

// RuntimeSemanticPlanningLedger is persisted owner data.  pkg validates its
// value shape only; Orchestrator private internal events own every transition.
type RuntimeSemanticPlanningLedger struct {
	SchemaVersion       string
	Phase               RuntimeSemanticPlanningPhase
	DefinitionSeed      RuntimeDefinitionSeed
	Manifest            *RuntimeSemanticPlanManifest
	Attempt             int
	StablePrefixHash    string
	LogicalRequestID    string
	CorrectionDirective *RuntimeSemanticCorrectionDirective
	// CoverageOrderDocument is the immutable, provider-authored full CWI order.
	// It survives current-focus proof sealing so the Orchestrator can admit the
	// next focus without asking a later proof batch to restate global authority.
	// It is lineage only: it never grants a binding, operation, or transition.
	CoverageOrderDocument   *RuntimeSemanticPlanDocument
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
	// History is immutable semantic-generation audit lineage. It is owned by
	// Orchestrator SM-2 and never participates in the current provider request.
	// A fresh generation starts with Phase=Required while retaining all prior
	// snapshots here.
	History []RuntimeSemanticPlanningGeneration
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
	if l.CorrectionDirective != nil {
		value := l.CorrectionDirective.Clone()
		l.CorrectionDirective = &value
	}
	if l.Manifest != nil {
		value := l.Manifest.Clone()
		l.Manifest = &value
	}
	if l.CoverageOrderDocument != nil {
		value := l.CoverageOrderDocument.Clone()
		l.CoverageOrderDocument = &value
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
	// Sealed lineage is a positional chain (manifest → order → document →
	// feasibility batch → feasibility set → binding set), not an unordered
	// collection. Preserve order while trimming values so validation can reject
	// a permuted chain instead of silently sorting it into acceptance.
	l.SealedLineageRefs = normalizeOrderedStrings(l.SealedLineageRefs)
	for idx := range l.Rejections {
		if value := l.Rejections[idx].Normalized(); value != nil {
			l.Rejections[idx] = *value
		}
	}
	for idx := range l.FeasibilityGaps {
		l.FeasibilityGaps[idx] = l.FeasibilityGaps[idx].Normalized()
	}
	l.Failure = l.Failure.Normalized()
	if len(l.History) > 0 {
		values := make([]RuntimeSemanticPlanningGeneration, 0, len(l.History))
		for _, value := range l.History {
			values = append(values, value.Normalized())
		}
		// History is append-only semantic lineage. Preserve its order; sorting
		// would turn a causal chain into a presentation list.
		l.History = values
	}
	return l
}

func (l RuntimeSemanticPlanningLedger) Clone() RuntimeSemanticPlanningLedger {
	originalFeasibilities := append([]RuntimeSemanticProofFeasibility(nil), l.Feasibilities...)
	originalRejections := append([]RuntimeOriginalDiagnostic(nil), l.Rejections...)
	originalGaps := append([]RuntimeSemanticFeasibilityGap(nil), l.FeasibilityGaps...)
	originalHistory := append([]RuntimeSemanticPlanningGeneration(nil), l.History...)
	originalFailure := l.Failure.CloneOriginal()
	var originalCoverageOrder *RuntimeSemanticPlanDocument
	if l.CoverageOrderDocument != nil {
		value := l.CoverageOrderDocument.Clone()
		originalCoverageOrder = &value
	}
	l = l.Normalized()
	l.CoverageOrderDocument = originalCoverageOrder
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
	if originalHistory != nil {
		l.History = make([]RuntimeSemanticPlanningGeneration, len(originalHistory))
		for idx := range originalHistory {
			l.History[idx] = originalHistory[idx].Clone()
		}
	} else {
		l.History = nil
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
	raw := l
	rawHistory := append([]RuntimeSemanticPlanningGeneration(nil), l.History...)
	l = l.Normalized()
	if l.SchemaVersion != RuntimeSemanticPlanningLedgerSchemaCurrent {
		return fmt.Errorf("runtime semantic planning ledger requires current schema")
	}
	// Ledger collections are identity-bearing history, not presentation lists.
	// Normalization may remove duplicate/empty values for read models, but a
	// persisted authority ledger must reject that input rather than silently
	// changing its lineage while validating it.
	if len(raw.PendingCoverageIDs) != len(l.PendingCoverageIDs) || len(raw.CompletedBatchIDs) != len(l.CompletedBatchIDs) ||
		len(raw.Batches) != len(l.Batches) || len(raw.BatchDocuments) != len(l.BatchDocuments) || len(raw.BatchFeasibilityResults) != len(l.BatchFeasibilityResults) ||
		len(raw.History) != len(l.History) {
		return fmt.Errorf("runtime semantic planning ledger contains duplicate or empty identity history")
	}
	if len(l.History) > RuntimeSemanticPlanningHistoryLimit {
		return fmt.Errorf("runtime semantic planning history exceeds bounded limit %d", RuntimeSemanticPlanningHistoryLimit)
	}
	type generationIdentity struct {
		manifestHash     string
		logicalRequestID string
	}
	seenHistory := make(map[generationIdentity]struct{}, len(l.History))
	seenInvalidationSignatures := make(map[string]struct{})
	lastRetiredRevision := uint64(0)
	for idx := range l.History {
		if idx >= len(rawHistory) || len(rawHistory[idx].InvalidationSignatures) != len(l.History[idx].InvalidationSignatures) || len(rawHistory[idx].PrerequisiteIDs) != len(l.History[idx].PrerequisiteIDs) ||
			len(rawHistory[idx].ReopenedCoverageIDs) != len(l.History[idx].ReopenedCoverageIDs) || len(rawHistory[idx].ReopenTriggerRefs) != len(l.History[idx].ReopenTriggerRefs) ||
			len(rawHistory[idx].ChangedCoverageIDs) != len(l.History[idx].ChangedCoverageIDs) || len(rawHistory[idx].ClosureChangeTriggerRefs) != len(l.History[idx].ClosureChangeTriggerRefs) {
			return fmt.Errorf("runtime semantic planning history %d contains duplicate or empty lineage values", idx)
		}
		history := l.History[idx]
		if err := history.Validate(); err != nil {
			return fmt.Errorf("runtime semantic planning history %d: %w", idx, err)
		}
		if history.Snapshot.DefinitionSeed.Hash != l.DefinitionSeed.Hash {
			return fmt.Errorf("runtime semantic planning history %d definition seed mismatch", idx)
		}
		if lastRetiredRevision != 0 && history.RetiredAtStateRevision <= lastRetiredRevision {
			return fmt.Errorf("runtime semantic planning history is not append ordered")
		}
		lastRetiredRevision = history.RetiredAtStateRevision
		for _, signature := range history.InvalidationSignatures {
			if _, duplicate := seenInvalidationSignatures[signature]; duplicate {
				return fmt.Errorf("runtime semantic planning history duplicates invalidation signature %q", signature)
			}
			seenInvalidationSignatures[signature] = struct{}{}
		}
		// A document, batch, or phase change under the same sealed catalog and
		// logical request is still one generation and must not be appended as a
		// second history entry.  A later request with the same catalog hash is
		// allowed only when it has a distinct logical identity (resource rollback
		// recovery); it cannot reuse the old snapshot's request/session lineage.
		identity := generationIdentity{manifestHash: history.Snapshot.Manifest.Hash, logicalRequestID: history.Snapshot.LogicalRequestID}
		if _, duplicate := seenHistory[identity]; duplicate {
			return fmt.Errorf("runtime semantic planning history duplicates generation %q", identity)
		}
		seenHistory[identity] = struct{}{}
	}
	for idx := range raw.Batches {
		rawBatch, normalizedBatch := raw.Batches[idx], l.Batches[idx]
		if len(rawBatch.CoverageWorkItemIDs) != len(normalizedBatch.CoverageWorkItemIDs) ||
			len(rawBatch.PendingCoverageWorkItemIDs) != len(normalizedBatch.PendingCoverageWorkItemIDs) ||
			len(rawBatch.DependencyBarrierRefs) != len(normalizedBatch.DependencyBarrierRefs) {
			return fmt.Errorf("runtime semantic planning batch %d contains duplicate or empty identity values", idx)
		}
	}
	if err := l.DefinitionSeed.Validate(); err != nil {
		return err
	}
	if l.CoverageOrderDocument != nil {
		if l.Manifest == nil {
			return fmt.Errorf("runtime semantic coverage order has no manifest")
		}
		order := l.CoverageOrderDocument.Clone()
		if order.PlanStep != RuntimeSemanticPlanStepCoverageOrder || order.BatchID != "" ||
			order.ManifestHash != l.Manifest.Hash || order.DefinitionSeedHash != l.Manifest.DefinitionSeedHash {
			return fmt.Errorf("runtime semantic coverage order lineage does not match manifest")
		}
		if err := order.ValidateAgainstManifest(*l.Manifest); err != nil {
			return err
		}
	}
	if len(l.PendingCoverageIDs) != 0 {
		if l.CoverageOrderDocument == nil {
			return fmt.Errorf("runtime semantic pending coverage has no immutable coverage order")
		}
		ordered := make(map[common.CoverageWorkItemID]struct{}, len(l.CoverageOrderDocument.CoverageOrder))
		for _, id := range l.CoverageOrderDocument.CoverageOrder {
			ordered[id] = struct{}{}
		}
		for _, id := range l.PendingCoverageIDs {
			if _, found := ordered[id]; !found {
				return fmt.Errorf("runtime semantic pending coverage %q is outside immutable coverage order", id)
			}
		}
	}
	if l.NextBatchSequence < 0 || l.NextBatchSequence != len(l.Batches) || (l.ActiveBatchID != "" && len(l.Batches) == 0) {
		return fmt.Errorf("runtime semantic planning batch cursor is invalid")
	}
	if len(l.Batches) != 0 && l.CoverageOrderDocument == nil {
		return fmt.Errorf("runtime semantic planning batches require immutable coverage order")
	}
	batchIDs := make(map[string]struct{}, len(l.Batches))
	batchSequences := make(map[int]struct{}, len(l.Batches))
	maxBatchSequence := -1
	for idx := range l.Batches {
		batch := l.Batches[idx]
		if batch.BatchID == "" {
			return fmt.Errorf("runtime semantic planning batch %d has empty identity", idx)
		}
		if _, duplicate := batchIDs[batch.BatchID]; duplicate {
			return fmt.Errorf("runtime semantic planning duplicates batch %q", batch.BatchID)
		}
		if _, duplicate := batchSequences[batch.Sequence]; duplicate {
			return fmt.Errorf("runtime semantic planning duplicates batch sequence %d", batch.Sequence)
		}
		if batch.Sequence != idx {
			return fmt.Errorf("runtime semantic planning batch sequence %d is not contiguous at index %d", batch.Sequence, idx)
		}
		batchIDs[batch.BatchID] = struct{}{}
		batchSequences[batch.Sequence] = struct{}{}
		if batch.Sequence > maxBatchSequence {
			maxBatchSequence = batch.Sequence
		}
		if l.Manifest != nil {
			if err := batch.ValidateAgainstManifest(*l.Manifest); err != nil {
				return err
			}
		}
		if l.CoverageOrderDocument != nil && batch.CoverageOrderHash != StableRuntimeSemanticPlanCoverageOrderHash(*l.CoverageOrderDocument) {
			return fmt.Errorf("runtime semantic batch %q is not bound to the immutable coverage order", batch.BatchID)
		}
	}
	if maxBatchSequence >= 0 && l.NextBatchSequence != maxBatchSequence+1 {
		return fmt.Errorf("runtime semantic planning next batch sequence is not contiguous with history")
	}
	if l.CoverageOrderDocument != nil {
		orderIDs := append([]common.CoverageWorkItemID(nil), l.CoverageOrderDocument.CoverageOrder...)
		pendingBefore := orderIDs
		items := make(map[common.CoverageWorkItemID]RuntimeCoverageItem, len(l.Manifest.CoverageItems))
		for _, item := range l.Manifest.CoverageItems {
			items[item.ID] = item
		}
		for _, batch := range l.Batches {
			selected := batch.CoverageWorkItemIDs
			selectedSet := make(map[common.CoverageWorkItemID]struct{}, len(selected))
			for _, id := range selected {
				if _, duplicate := selectedSet[id]; duplicate {
					return fmt.Errorf("runtime semantic batch %q duplicates selected coverage %q", batch.BatchID, id)
				}
				selectedSet[id] = struct{}{}
			}
			expectedPending := subtractOrderedRuntimeCoverageIDs(pendingBefore, selectedSet)
			if len(expectedPending)+len(selected) != len(pendingBefore) ||
				!sameOrderedRuntimeCoverageIDs(batch.PendingCoverageWorkItemIDs, expectedPending) {
				return fmt.Errorf("runtime semantic batch %q does not form an exact selected/pending partition", batch.BatchID)
			}
			expectedBarriers := runtimeSemanticPendingDependencyBarriers(items, expectedPending, selectedSet)
			if !sameRuntimeStringSet(batch.DependencyBarrierRefs, expectedBarriers) {
				return fmt.Errorf("runtime semantic batch %q dependency barriers do not match pending lineage", batch.BatchID)
			}
			pendingBefore = expectedPending
		}
		if !sameOrderedRuntimeCoverageIDs(l.PendingCoverageIDs, pendingBefore) {
			return fmt.Errorf("runtime semantic pending coverage does not match the latest batch lineage")
		}
	}
	batchDocuments := make(map[string]RuntimeSemanticPlanDocument, len(l.BatchDocuments))
	for _, document := range l.BatchDocuments {
		if _, duplicate := batchDocuments[document.BatchID]; duplicate {
			return fmt.Errorf("runtime semantic planning duplicates batch document %q", document.BatchID)
		}
		batch, found := runtimeSemanticPlanningBatch(l.Batches, document.BatchID)
		if !found {
			return fmt.Errorf("runtime semantic batch document has no matching batch")
		}
		if document.PlanStep != RuntimeSemanticPlanStepProofBatch || document.BatchSequence != batch.Sequence ||
			!sameOrderedRuntimeCoverageIDs(document.BatchCoverageWorkItemIDs, batch.CoverageWorkItemIDs) ||
			batch.DocumentHash != StableRuntimeSemanticPlanDocumentHash(document) {
			return fmt.Errorf("runtime semantic batch document identity is not bound to batch")
		}
		if l.Manifest == nil {
			return fmt.Errorf("runtime semantic batch document has no manifest")
		}
		if err := document.ValidateAgainstManifest(*l.Manifest); err != nil {
			return err
		}
		if batch.State == RuntimeSemanticPlanBatchPending || batch.State == RuntimeSemanticPlanBatchRequesting || batch.State == RuntimeSemanticPlanBatchFailed {
			return fmt.Errorf("runtime semantic batch document %q is not in a document-bearing batch state", document.BatchID)
		}
		batchDocuments[document.BatchID] = document.Clone()
	}
	if l.ActiveBatchID != "" {
		if _, found := batchIDs[l.ActiveBatchID]; !found {
			return fmt.Errorf("runtime semantic planning active batch is missing")
		}
		if len(l.Batches) == 0 || l.Batches[len(l.Batches)-1].BatchID != l.ActiveBatchID {
			return fmt.Errorf("runtime semantic planning active batch is not the latest batch")
		}
	}
	seenBatchResults := make(map[string]struct{}, len(l.BatchFeasibilityResults))
	for _, result := range l.BatchFeasibilityResults {
		if l.Manifest == nil {
			return fmt.Errorf("runtime semantic planning batch feasibility has no manifest")
		}
		if _, duplicate := seenBatchResults[result.BatchID]; duplicate {
			return fmt.Errorf("runtime semantic planning duplicates batch feasibility %q", result.BatchID)
		}
		seenBatchResults[result.BatchID] = struct{}{}
		batch, found := runtimeSemanticPlanningBatch(l.Batches, result.BatchID)
		if !found {
			return fmt.Errorf("runtime semantic planning batch feasibility has no matching batch")
		}
		document, found := batchDocuments[result.BatchID]
		if !found {
			return fmt.Errorf("runtime semantic planning batch feasibility has no matching document")
		}
		if err := result.Validate(batch, document, *l.Manifest); err != nil {
			return err
		}
	}
	completed := make(map[string]struct{}, len(l.CompletedBatchIDs))
	for idx, id := range l.CompletedBatchIDs {
		if _, duplicate := completed[id]; duplicate {
			return fmt.Errorf("runtime semantic planning duplicates completed batch %q", id)
		}
		batch, found := runtimeSemanticPlanningBatch(l.Batches, id)
		if !found || batch.State != RuntimeSemanticPlanBatchAccepted {
			return fmt.Errorf("runtime semantic completed batch %q is not an accepted batch", id)
		}
		if idx >= len(l.Batches) || l.Batches[idx].BatchID != id {
			return fmt.Errorf("runtime semantic completed batches are not a contiguous prefix")
		}
		if id == l.ActiveBatchID {
			return fmt.Errorf("runtime semantic completed batch %q is still active", id)
		}
		completed[id] = struct{}{}
	}
	if l.CoverageOrderDocument != nil {
		orderHash := StableRuntimeSemanticPlanCoverageOrderHash(*l.CoverageOrderDocument)
		for _, batch := range l.Batches {
			if batch.CoverageOrderHash != orderHash {
				return fmt.Errorf("runtime semantic batch %q has stale coverage-order identity", batch.BatchID)
			}
		}
	}
	orderIDs := runtimeSemanticPlanningCoverageOrder(l.CoverageOrderDocument)
	if l.CoverageOrderDocument != nil {
		if err := validateRuntimeSemanticPendingCoverage(l.PendingCoverageIDs, orderIDs); err != nil {
			return err
		}
		completedCoverage := make(map[common.CoverageWorkItemID]struct{})
		for id := range completed {
			batch, _ := runtimeSemanticPlanningBatch(l.Batches, id)
			for _, coverageID := range batch.CoverageWorkItemIDs {
				completedCoverage[coverageID] = struct{}{}
			}
		}
		for _, coverageID := range l.PendingCoverageIDs {
			if _, done := completedCoverage[coverageID]; done {
				return fmt.Errorf("runtime semantic pending coverage %q belongs to a completed batch", coverageID)
			}
		}
	}
	if l.MergedDocument != nil {
		if l.MergedDocumentHash != StableRuntimeSemanticPlanDocumentHash(*l.MergedDocument) {
			return fmt.Errorf("runtime semantic merged document hash does not match contents")
		}
		if l.MergedDocument.PlanStep != RuntimeSemanticPlanStepProofBatch || l.MergedDocument.BatchID == "" {
			return fmt.Errorf("runtime semantic merged document must be a focus proof batch")
		}
		batch, found := runtimeSemanticPlanningBatch(l.Batches, l.MergedDocument.BatchID)
		if !found || batch.State != RuntimeSemanticPlanBatchAccepted || batch.DocumentHash != l.MergedDocumentHash {
			return fmt.Errorf("runtime semantic merged document is not bound to an accepted batch")
		}
		if document, found := batchDocuments[l.MergedDocument.BatchID]; !found || StableRuntimeSemanticPlanDocumentHash(document) != l.MergedDocumentHash {
			return fmt.Errorf("runtime semantic merged document has no matching batch document")
		}
	}
	for idx := range l.Rejections {
		if err := l.Rejections[idx].Validate(); err != nil {
			return fmt.Errorf("runtime semantic planning rejection %d: %w", idx, err)
		}
	}
	if l.CorrectionDirective != nil {
		if l.Attempt != 1 || len(l.Rejections) != 1 || l.Manifest == nil {
			return fmt.Errorf("runtime semantic correction directive has no exact attempt-0 lineage")
		}
		if err := l.CorrectionDirective.ValidateAgainst(*l.Manifest, l.Rejections[0]); err != nil {
			return err
		}
		if l.CorrectionDirective.StablePrefixHash != l.StablePrefixHash {
			return fmt.Errorf("runtime semantic correction directive stable prefix is stale")
		}
	} else if l.Attempt == 1 {
		return fmt.Errorf("runtime semantic attempt 1 requires a correction directive")
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
		if l.Manifest != nil || l.Attempt != 0 || l.StablePrefixHash != "" || l.LogicalRequestID != "" || l.CorrectionDirective != nil || l.CoverageOrderDocument != nil || l.Document != nil || l.DocumentHash != "" || len(l.Batches) != 0 || l.ActiveBatchID != "" || l.NextBatchSequence != 0 || len(l.PendingCoverageIDs) != 0 || len(l.CompletedBatchIDs) != 0 || len(l.BatchFeasibilityResults) != 0 || len(l.BatchDocuments) != 0 || l.MergedDocument != nil || l.MergedDocumentHash != "" || l.FeasibilityBatchID != "" || len(l.Feasibilities) != 0 || l.FeasibilitySetHash != "" || l.SealedBindingSetHash != "" || len(l.SealedLineageRefs) != 0 || len(l.Rejections) != 0 || l.Failure != nil {
			return fmt.Errorf("required runtime semantic planning ledger has unexpected downstream facts")
		}
	case RuntimeSemanticPlanningOrderRequestOpen, RuntimeSemanticPlanningRequestOpen, RuntimeSemanticPlanningCorrectionRequired:
		batchCorrectionDocument := l.Phase == RuntimeSemanticPlanningCorrectionRequired && l.ActiveBatchID != "" && l.Document != nil && l.Document.PlanStep == RuntimeSemanticPlanStepCoverageOrder && l.DocumentHash == StableRuntimeSemanticPlanDocumentHash(*l.Document)
		if l.Manifest == nil || l.Attempt < 0 || l.Attempt > 1 || l.StablePrefixHash == "" || l.LogicalRequestID == "" || (!batchCorrectionDocument && (l.Document != nil || l.DocumentHash != "")) || l.FeasibilityBatchID != "" || len(l.Feasibilities) != 0 || l.FeasibilitySetHash != "" || l.SealedBindingSetHash != "" || len(l.SealedLineageRefs) != 0 || l.Failure != nil {
			return fmt.Errorf("open runtime semantic planning ledger has invalid shape")
		}
		if (l.Attempt == 0 && l.CorrectionDirective != nil) || (l.Attempt == 1 && l.CorrectionDirective == nil) || (l.Phase == RuntimeSemanticPlanningCorrectionRequired && l.Attempt != 1) {
			return fmt.Errorf("runtime semantic planning correction lineage is invalid")
		}
		if ((l.Phase == RuntimeSemanticPlanningOrderRequestOpen || l.Phase == RuntimeSemanticPlanningRequestOpen) && len(l.Rejections) != 0) || (l.Phase == RuntimeSemanticPlanningCorrectionRequired && len(l.Rejections) != 1) {
			return fmt.Errorf("runtime semantic planning rejection lineage is invalid")
		}
		if err := l.Manifest.Validate(); err != nil {
			return err
		}
		if l.Phase != RuntimeSemanticPlanningCorrectionRequired {
			if l.CoverageOrderDocument != nil || len(l.Batches) != 0 || l.ActiveBatchID != "" || l.NextBatchSequence != 0 || len(l.PendingCoverageIDs) != 0 || len(l.CompletedBatchIDs) != 0 || len(l.BatchDocuments) != 0 || len(l.BatchFeasibilityResults) != 0 || l.MergedDocument != nil || l.MergedDocumentHash != "" {
				return fmt.Errorf("open runtime semantic planning ledger has unexpected batch lineage")
			}
		} else if l.ActiveBatchID != "" {
			batch, found := runtimeSemanticPlanningBatch(l.Batches, l.ActiveBatchID)
			if !found || batch.State != RuntimeSemanticPlanBatchRequesting || l.CoverageOrderDocument == nil || l.Document == nil || l.Document.PlanStep != RuntimeSemanticPlanStepCoverageOrder {
				return fmt.Errorf("proof-batch correction ledger has invalid active batch lineage")
			}
			if err := validateRuntimeSemanticPendingForBatch(l, batch); err != nil {
				return err
			}
		} else if l.CoverageOrderDocument != nil || len(l.Batches) != 0 || l.NextBatchSequence != 0 || len(l.PendingCoverageIDs) != 0 || len(l.CompletedBatchIDs) != 0 || len(l.BatchDocuments) != 0 || len(l.BatchFeasibilityResults) != 0 || l.MergedDocument != nil || l.MergedDocumentHash != "" {
			return fmt.Errorf("coverage-order correction ledger has unexpected batch lineage")
		}
	case RuntimeSemanticPlanningOrderAccepted:
		if err := validateRuntimeSemanticPlanningAcceptedBase(l); err != nil {
			return err
		}
		if l.Document.PlanStep != RuntimeSemanticPlanStepCoverageOrder || l.CoverageOrderDocument == nil ||
			StableRuntimeSemanticPlanDocumentHash(*l.CoverageOrderDocument) != StableRuntimeSemanticPlanDocumentHash(*l.Document) ||
			l.ActiveBatchID != "" || l.FeasibilityBatchID != "" || len(l.Feasibilities) != 0 || l.FeasibilitySetHash != "" ||
			l.MergedDocument != nil || l.MergedDocumentHash != "" {
			return fmt.Errorf("order-accepted semantic ledger has invalid order/batch state")
		}
		if err := validateRuntimeSemanticPendingAfterCompleted(l); err != nil {
			return err
		}
	case RuntimeSemanticPlanningProofBatchRequestOpen:
		if err := validateRuntimeSemanticPlanningAcceptedBase(l); err != nil {
			return err
		}
		batch, found := runtimeSemanticPlanningBatch(l.Batches, l.ActiveBatchID)
		if !found || batch.State != RuntimeSemanticPlanBatchRequesting || l.Document.PlanStep != RuntimeSemanticPlanStepCoverageOrder || l.CoverageOrderDocument == nil {
			return fmt.Errorf("proof-batch request ledger requires requesting active batch and order document")
		}
		if err := validateRuntimeSemanticPendingForBatch(l, batch); err != nil {
			return err
		}
		if l.FeasibilityBatchID != "" || len(l.Feasibilities) != 0 || l.FeasibilitySetHash != "" || l.MergedDocument != nil || l.MergedDocumentHash != "" {
			return fmt.Errorf("proof-batch request ledger has downstream feasibility facts")
		}
	case RuntimeSemanticPlanningProofBatchDocumentAccepted:
		if err := validateRuntimeSemanticPlanningAcceptedBase(l); err != nil {
			return err
		}
		batch, found := runtimeSemanticPlanningBatch(l.Batches, l.ActiveBatchID)
		batchDocument, documentFound := batchDocuments[l.ActiveBatchID]
		if !found || batch.State != RuntimeSemanticPlanBatchDocumentComplete || !documentFound ||
			batchDocument.PlanStep != RuntimeSemanticPlanStepProofBatch || l.Document.PlanStep != RuntimeSemanticPlanStepCoverageOrder || l.CoverageOrderDocument == nil {
			return fmt.Errorf("proof-batch document phase requires complete active batch document")
		}
		if err := validateRuntimeSemanticPendingForBatch(l, batch); err != nil {
			return err
		}
		if l.FeasibilityBatchID != "" || len(l.Feasibilities) != 0 || l.FeasibilitySetHash != "" || l.MergedDocument != nil || l.MergedDocumentHash != "" {
			return fmt.Errorf("proof-batch document phase has downstream feasibility facts")
		}
	case RuntimeSemanticPlanningFeasibilityRequired:
		if err := validateRuntimeSemanticPlanningAcceptedBase(l); err != nil {
			return err
		}
		if l.ActiveBatchID != "" {
			batch, found := runtimeSemanticPlanningBatch(l.Batches, l.ActiveBatchID)
			batchDocument, documentFound := batchDocuments[l.ActiveBatchID]
			if !found || batch.State != RuntimeSemanticPlanBatchFeasibilityPending || !documentFound ||
				batchDocument.PlanStep != RuntimeSemanticPlanStepProofBatch || l.Document.PlanStep != RuntimeSemanticPlanStepCoverageOrder || l.CoverageOrderDocument == nil {
				return fmt.Errorf("semantic feasibility phase requires pending active proof batch")
			}
			if err := validateRuntimeSemanticPendingForBatch(l, batch); err != nil {
				return err
			}
			if l.FeasibilityBatchID != StableRuntimeSemanticFeasibilityBatchID(*l.Manifest, batchDocument) {
				return fmt.Errorf("runtime semantic feasibility batch identity does not match active document")
			}
			if len(l.Feasibilities) > len(batchDocument.Proposals) || l.FeasibilitySetHash != StableRuntimeSemanticFeasibilitySetHash(l.Feasibilities) {
				return fmt.Errorf("runtime semantic feasibility set hash does not match observations")
			}
			if err := validateRuntimeSemanticFeasibilitySet(*l.Manifest, batchDocument, l.Feasibilities, false); err != nil {
				return err
			}
		} else {
			// The active batch is cleared only by the private merge event. This
			// shape is the short-lived seal-ready state; it must retain the exact
			// merged focus document rather than falling back to a full-manifest
			// document with no batch identity.
			if l.MergedDocument == nil || l.Document.PlanStep != RuntimeSemanticPlanStepProofBatch || l.Document.BatchID == "" ||
				l.DocumentHash != l.MergedDocumentHash || StableRuntimeSemanticPlanDocumentHash(*l.Document) != l.MergedDocumentHash {
				return fmt.Errorf("semantic feasibility phase without active batch requires merged focus document")
			}
			batch, found := runtimeSemanticPlanningBatch(l.Batches, l.Document.BatchID)
			if !found || batch.State != RuntimeSemanticPlanBatchAccepted || l.FeasibilityBatchID != StableRuntimeSemanticFeasibilityBatchID(*l.Manifest, *l.Document) {
				return fmt.Errorf("semantic merged focus document is not bound to accepted batch")
			}
			if len(l.Feasibilities) != len(l.Document.Proposals) || l.FeasibilitySetHash != StableRuntimeSemanticFeasibilitySetHash(l.Feasibilities) {
				return fmt.Errorf("merged semantic focus requires complete owner facts")
			}
			if err := validateRuntimeSemanticFeasibilitySet(*l.Manifest, *l.Document, l.Feasibilities, true); err != nil {
				return err
			}
		}
	case RuntimeSemanticPlanningProofBatchFeasibilityComplete:
		// A completed proof batch is still an unsealed SM-2 working state.  It
		// carries only the accepted batch document and the complete owner facts
		// for that batch; binding/focus authority is not allowed until the
		// orchestrator performs the explicit merge/seal transition.
		if l.Manifest == nil || l.Attempt < 0 || l.Attempt > 1 || l.StablePrefixHash == "" || l.LogicalRequestID == "" || l.Document == nil || l.DocumentHash == "" || l.ActiveBatchID == "" || l.FeasibilityBatchID == "" || l.SealedBindingSetHash != "" || len(l.SealedLineageRefs) != 0 || len(l.Rejections) != l.Attempt || l.Failure != nil || l.MergedDocument != nil || l.MergedDocumentHash != "" {
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
		batch, found := runtimeSemanticPlanningBatch(l.Batches, l.ActiveBatchID)
		if !found || batch.State != RuntimeSemanticPlanBatchAccepted || l.Document.PlanStep != RuntimeSemanticPlanStepCoverageOrder || l.CoverageOrderDocument == nil {
			return fmt.Errorf("completed runtime semantic proof batch has invalid accepted batch/order identity")
		}
		if err := validateRuntimeSemanticPendingForBatch(l, batch); err != nil {
			return err
		}
		if len(l.Feasibilities) != len(batchDocument.Proposals) || l.FeasibilitySetHash != StableRuntimeSemanticFeasibilitySetHash(l.Feasibilities) {
			return fmt.Errorf("completed runtime semantic proof batch requires complete owner facts")
		}
		if err := validateRuntimeSemanticFeasibilitySet(*l.Manifest, *batchDocument, l.Feasibilities, true); err != nil {
			return err
		}
	case RuntimeSemanticPlanningSealed:
		if l.Manifest == nil || l.CoverageOrderDocument == nil || l.StablePrefixHash == "" || l.LogicalRequestID == "" || l.Document == nil || l.DocumentHash == "" || l.Document.PlanStep != RuntimeSemanticPlanStepProofBatch || l.Document.BatchID == "" || l.ActiveBatchID != "" || l.MergedDocument == nil || l.FeasibilityBatchID == "" || len(l.Feasibilities) != len(l.Document.Proposals) || l.FeasibilitySetHash == "" || l.SealedBindingSetHash == "" || len(l.SealedLineageRefs) == 0 || len(l.Rejections) != l.Attempt || l.Failure != nil {
			return fmt.Errorf("sealed runtime semantic planning ledger requires document, complete feasibility and binding hash")
		}
		if err := l.Document.ValidateAgainstManifest(*l.Manifest); err != nil {
			return err
		}
		if l.DocumentHash != StableRuntimeSemanticPlanDocumentHash(*l.Document) || l.MergedDocumentHash != l.DocumentHash || StableRuntimeSemanticPlanDocumentHash(*l.MergedDocument) != l.DocumentHash || l.FeasibilityBatchID != StableRuntimeSemanticFeasibilityBatchID(*l.Manifest, *l.Document) || l.FeasibilitySetHash != StableRuntimeSemanticFeasibilitySetHash(l.Feasibilities) {
			return fmt.Errorf("sealed runtime semantic planning lineage does not match contents")
		}
		batch, found := runtimeSemanticPlanningBatch(l.Batches, l.Document.BatchID)
		if !found || batch.State != RuntimeSemanticPlanBatchAccepted || batch.DocumentHash != l.DocumentHash {
			return fmt.Errorf("sealed runtime semantic planning document is not bound to accepted batch")
		}
		if _, completed := completed[l.Document.BatchID]; !completed {
			return fmt.Errorf("sealed runtime semantic planning batch is not in completed history")
		}
		if err := validateRuntimeSemanticPendingAfterCompleted(l); err != nil {
			return err
		}
		expectedLineage := normalizeOrderedStrings([]string{
			l.Manifest.Hash,
			StableRuntimeSemanticPlanCoverageOrderHash(*l.CoverageOrderDocument),
			l.DocumentHash,
			l.FeasibilityBatchID,
			l.FeasibilitySetHash,
			l.SealedBindingSetHash,
		})
		if !sameOrderedRuntimeSemanticStrings(l.SealedLineageRefs, expectedLineage) {
			return fmt.Errorf("sealed runtime semantic lineage references do not match authority")
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
		// During a proof-batch owner probe the canonical ledger document remains
		// the coverage-order document while the active batch document is stored
		// separately.  A typed owner rejection can therefore legitimately leave
		// FeasibilityBatchID bound to that active batch.  Validate the same
		// document that the feasibility dispatcher used instead of treating the
		// failure transition as an internal contract defect.
		feasibilityDocument := l.Document
		hasFeasibilityDocument := true
		if l.FeasibilityBatchID != "" && l.ActiveBatchID != "" {
			hasFeasibilityDocument = false
			for idx := range l.BatchDocuments {
				if l.BatchDocuments[idx].BatchID == l.ActiveBatchID {
					value := l.BatchDocuments[idx].Clone()
					feasibilityDocument = &value
					hasFeasibilityDocument = true
					break
				}
			}
		}
		if hasFeasibilityDocument && l.FeasibilityBatchID != "" && l.FeasibilityBatchID != StableRuntimeSemanticFeasibilityBatchID(*l.Manifest, *feasibilityDocument) {
			return fmt.Errorf("failed runtime semantic feasibility batch identity does not match document")
		}
		if l.FeasibilitySetHash != StableRuntimeSemanticFeasibilitySetHash(l.Feasibilities) {
			return fmt.Errorf("failed runtime semantic feasibility set hash does not match observations")
		}
		if hasFeasibilityDocument {
			if err := validateRuntimeSemanticFeasibilitySet(*l.Manifest, *feasibilityDocument, l.Feasibilities, false); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("runtime semantic planning ledger has invalid phase %q", l.Phase)
	}
	return nil
}

func runtimeSemanticPlanningBatch(values []RuntimeSemanticPlanBatch, batchID string) (RuntimeSemanticPlanBatch, bool) {
	batchID = strings.TrimSpace(batchID)
	for _, value := range values {
		if value.BatchID == batchID {
			return value.Clone(), true
		}
	}
	return RuntimeSemanticPlanBatch{}, false
}

func sameOrderedRuntimeCoverageIDs(left, right []common.CoverageWorkItemID) bool {
	left = normalizeOrderedCoverageWorkItemIDs(left)
	right = normalizeOrderedCoverageWorkItemIDs(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func subtractOrderedRuntimeCoverageIDs(values []common.CoverageWorkItemID, removed map[common.CoverageWorkItemID]struct{}) []common.CoverageWorkItemID {
	ret := make([]common.CoverageWorkItemID, 0, len(values))
	for _, id := range values {
		if _, remove := removed[id]; remove {
			continue
		}
		ret = append(ret, id)
	}
	return ret
}

// sameOrderedRuntimeSemanticStrings compares lineage references as a
// positional chain.  Unlike the set helpers used for unordered vocabulary,
// lineage order is part of the authority identity and must never be sorted or
// deduplicated during comparison.
func sameOrderedRuntimeSemanticStrings(left, right []string) bool {
	left = normalizeOrderedStrings(left)
	right = normalizeOrderedStrings(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func runtimeSemanticPlanningCoverageOrder(document *RuntimeSemanticPlanDocument) []common.CoverageWorkItemID {
	if document == nil {
		return nil
	}
	return append([]common.CoverageWorkItemID(nil), document.CoverageOrder...)
}

func validateRuntimeSemanticPendingCoverage(pending, order []common.CoverageWorkItemID) error {
	if len(pending) == 0 {
		return nil
	}
	allowed := make(map[common.CoverageWorkItemID]struct{}, len(order))
	for _, id := range order {
		allowed[id] = struct{}{}
	}
	seen := make(map[common.CoverageWorkItemID]struct{}, len(pending))
	for _, id := range pending {
		if _, ok := allowed[id]; !ok {
			return fmt.Errorf("runtime semantic pending coverage %q is outside immutable coverage order", id)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("runtime semantic pending coverage duplicates %q", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validateRuntimeSemanticPlanningAcceptedBase(l RuntimeSemanticPlanningLedger) error {
	if l.Manifest == nil || l.Attempt < 0 || l.Attempt > 1 || l.StablePrefixHash == "" || l.LogicalRequestID == "" || l.Document == nil || l.DocumentHash == "" || l.SealedBindingSetHash != "" || len(l.SealedLineageRefs) != 0 || len(l.Rejections) != l.Attempt || l.Failure != nil {
		return fmt.Errorf("accepted runtime semantic planning ledger has invalid shape")
	}
	if err := l.Manifest.Validate(); err != nil {
		return err
	}
	if err := l.Document.ValidateAgainstManifest(*l.Manifest); err != nil {
		return err
	}
	if l.DocumentHash != StableRuntimeSemanticPlanDocumentHash(*l.Document) {
		return fmt.Errorf("runtime semantic planning document hash does not match contents")
	}
	if l.CorrectionDirective != nil && l.Attempt != 1 {
		return fmt.Errorf("runtime semantic accepted ledger has unexpected correction directive")
	}
	return nil
}

func validateRuntimeSemanticPendingForBatch(l RuntimeSemanticPlanningLedger, batch RuntimeSemanticPlanBatch) error {
	if !sameOrderedRuntimeCoverageIDs(l.PendingCoverageIDs, batch.PendingCoverageWorkItemIDs) {
		return fmt.Errorf("runtime semantic pending coverage does not match active batch %q", batch.BatchID)
	}
	return nil
}

func validateRuntimeSemanticPendingAfterCompleted(l RuntimeSemanticPlanningLedger) error {
	if l.CoverageOrderDocument == nil {
		return fmt.Errorf("runtime semantic pending coverage requires immutable order")
	}
	if err := validateRuntimeSemanticPendingCoverage(l.PendingCoverageIDs, l.CoverageOrderDocument.CoverageOrder); err != nil {
		return err
	}
	completedCoverage := make(map[common.CoverageWorkItemID]struct{})
	for _, id := range l.CompletedBatchIDs {
		batch, found := runtimeSemanticPlanningBatch(l.Batches, id)
		if !found {
			return fmt.Errorf("runtime semantic completed batch %q is missing", id)
		}
		for _, coverageID := range batch.CoverageWorkItemIDs {
			completedCoverage[coverageID] = struct{}{}
		}
	}
	for _, coverageID := range l.PendingCoverageIDs {
		if _, done := completedCoverage[coverageID]; done {
			return fmt.Errorf("runtime semantic pending coverage %q is already completed", coverageID)
		}
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
	writeRuntimeSemanticContractViolationsHash(h, diagnostic.SemanticViolations)
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
		writeHashString(h, value.OwnerPolicy.OwnerModuleID)
		writeHashString(h, value.OwnerPolicy.PolicyRevision)
		writeHashString(h, value.OwnerPolicy.CatalogSnapshotID)
		writeHashString(h, string(value.Status))
		writeHashString(h, value.ObservedRevision)
		writeHashStrings(h, value.TargetRefs)
		writeHashString(h, value.ValidationRecipeID)
		writeHashStrings(h, value.RequiredCheckRefs)
		if value.ProspectiveWorkspaceManifestDelta == nil {
			writeHashString(h, "no-prospective-workspace-manifest-delta")
		} else {
			writeHashString(h, StableRuntimeProspectiveWorkspaceManifestDeltaHash(*value.ProspectiveWorkspaceManifestDelta))
		}
		if value.OwnerFactProspectiveSatisfactionObservation == nil {
			writeHashString(h, "no-owner-fact-prospective-satisfaction")
		} else {
			writeHashString(h, StableRuntimeOwnerFactProspectiveSatisfactionObservationHash(*value.OwnerFactProspectiveSatisfactionObservation))
		}
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
	proposals := make(map[string]RuntimeSemanticProofProposal, len(document.Proposals))
	for _, proposal := range document.Normalized().Proposals {
		proposals[StableRuntimeSemanticProofProposalHash(proposal)] = proposal
	}
	seen := make(map[string]struct{}, len(values))
	for _, feasibility := range normalizeRuntimeSemanticFeasibilities(values) {
		proposal, found := proposals[feasibility.ProposalHash]
		if !found {
			return fmt.Errorf("runtime semantic feasibility references unknown proof %q", feasibility.ProposalHash)
		}
		if _, duplicate := seen[feasibility.ProposalHash]; duplicate {
			return fmt.Errorf("runtime semantic feasibility duplicates proof %q", feasibility.ProposalHash)
		}
		seen[feasibility.ProposalHash] = struct{}{}
		if complete {
			if err := feasibility.ValidateForProposal(manifest, proposal); err != nil {
				return err
			}
		} else if err := feasibility.ValidateCompositeForProposal(manifest, proposal); err != nil {
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

// RuntimeSemanticCWIProofSet is the bounded proof group for one canonical
// CWI. It is an ephemeral contract value used while compiling an SM-2 binding;
// the CWI itself remains the single persisted ledger row.
type RuntimeSemanticCWIProofSet struct {
	CoverageWorkItemID common.CoverageWorkItemID
	Proposals          []RuntimeSemanticProofProposal
}

func (s RuntimeSemanticCWIProofSet) EvidenceKinds() []string {
	values := make([]string, 0, len(s.Proposals))
	for _, proposal := range s.Proposals {
		proposal = proposal.Normalized()
		if len(proposal.RequiredEvidenceKinds) == 1 {
			values = append(values, proposal.RequiredEvidenceKinds[0])
		}
	}
	return normalizeStrings(values)
}

// OrderedRuntimeSemanticCWIProofSets groups independently feasible proof
// proposals under their CWI while retaining the persisted batch order. The
// helper creates no authority index and never changes a Runtime aggregate.
func OrderedRuntimeSemanticCWIProofSets(document RuntimeSemanticPlanDocument) ([]RuntimeSemanticCWIProofSet, error) {
	document = document.Normalized()
	byCoverage := make(map[common.CoverageWorkItemID][]RuntimeSemanticProofProposal, len(document.Proposals))
	for _, proposal := range document.Proposals {
		proposal = proposal.Clone()
		byCoverage[proposal.CoverageWorkItemID] = append(byCoverage[proposal.CoverageWorkItemID], proposal)
	}
	order := append([]common.CoverageWorkItemID(nil), document.BatchCoverageWorkItemIDs...)
	if len(order) == 0 {
		order = append(order, document.CoverageOrder...)
	}
	if len(order) == 0 {
		for coverageID := range byCoverage {
			order = append(order, coverageID)
		}
		sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	}
	sets := make([]RuntimeSemanticCWIProofSet, 0, len(order))
	for _, coverageID := range order {
		proofs := byCoverage[coverageID]
		if len(proofs) == 0 {
			return nil, fmt.Errorf("runtime semantic proof set has no proposal for coverage %q", coverageID)
		}
		sort.SliceStable(proofs, func(i, j int) bool {
			return StableRuntimeSemanticProofProposalHash(proofs[i]) < StableRuntimeSemanticProofProposalHash(proofs[j])
		})
		sets = append(sets, RuntimeSemanticCWIProofSet{
			CoverageWorkItemID: coverageID,
			Proposals:          append([]RuntimeSemanticProofProposal(nil), proofs...),
		})
		delete(byCoverage, coverageID)
	}
	if len(byCoverage) != 0 {
		return nil, fmt.Errorf("runtime semantic proof set contains coverage outside declared order")
	}
	return sets, nil
}

// StableRuntimeSemanticCWIProofSetHash gives a generation-independent identity
// to the independent proof declarations that jointly close one CWI. The
// caller validates shape before using this value as a binding suffix.
func StableRuntimeSemanticCWIProofSetHash(coverageID common.CoverageWorkItemID, proposals []RuntimeSemanticProofProposal) string {
	coverageID = common.CoverageWorkItemID(strings.TrimSpace(string(coverageID)))
	values := append([]RuntimeSemanticProofProposal(nil), proposals...)
	sort.SliceStable(values, func(i, j int) bool {
		return StableRuntimeSemanticProofProposalHash(values[i]) < StableRuntimeSemanticProofProposalHash(values[j])
	})
	h := sha256.New()
	writeHashString(h, "runtime-semantic-cwi-proof-set")
	writeHashString(h, string(coverageID))
	for _, proposal := range values {
		writeHashString(h, StableRuntimeSemanticProofProposalHash(proposal))
	}
	return "runtime-semantic-cwi-proof-set-" + hex.EncodeToString(h.Sum(nil))[:20]
}

// RuntimeSemanticProofProposalForHash resolves one proposal only within its
// supplied immutable document. It is intentionally a local scan so semantic
// proof identity never becomes a global registry.
func RuntimeSemanticProofProposalForHash(document RuntimeSemanticPlanDocument, proposalHash string) (RuntimeSemanticProofProposal, bool) {
	proposalHash = strings.TrimSpace(proposalHash)
	for _, proposal := range document.Normalized().Proposals {
		if StableRuntimeSemanticProofProposalHash(proposal) == proposalHash {
			return proposal.Clone(), true
		}
	}
	return RuntimeSemanticProofProposal{}, false
}

func normalizeRuntimeSemanticProofProposals(values []RuntimeSemanticProofProposal) []RuntimeSemanticProofProposal {
	ret := make([]RuntimeSemanticProofProposal, 0, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.CoverageWorkItemID != "" {
			ret = append(ret, value)
		}
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].CoverageWorkItemID != ret[j].CoverageWorkItemID {
			return ret[i].CoverageWorkItemID < ret[j].CoverageWorkItemID
		}
		return StableRuntimeSemanticProofProposalHash(ret[i]) < StableRuntimeSemanticProofProposalHash(ret[j])
	})
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
		for _, dependencyID := range item.PrerequisiteCoverageWorkItemIDs {
			depPos, ok := position[dependencyID]
			if !ok || depPos >= itemPos {
				return &RuntimeSemanticDependencyOrderError{
					RequiredBeforeID:     dependencyID,
					RequiredAfterID:      item.ID,
					ActualBeforePosition: depPos,
					ActualAfterPosition:  itemPos,
				}
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
		if ret[i].ProposalHash != ret[j].ProposalHash {
			return ret[i].ProposalHash < ret[j].ProposalHash
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
			return runtimeSemanticProposalConstraint("target.exactTargetRefs", RuntimeSemanticConstraintNonEmpty, "empty_array", []string{"at least one target ref"}, nil)
		}
	case RuntimeOperationTargetPolicyExistingOwnerScope:
		if len(target.OwnerScopeTargetRefs) == 0 && len(target.OwnerSelectorRefs) == 0 {
			return runtimeSemanticProposalConstraint("target", RuntimeSemanticConstraintRelational, "empty_target_scope", []string{"owner scope target or selector"}, nil)
		}
		if len(target.OwnerSelectorRefs) > 1 {
			return runtimeSemanticProposalConstraint("target.ownerSelectorRefs", RuntimeSemanticConstraintRelational, fmt.Sprintf("%d selectors", len(target.OwnerSelectorRefs)), []string{"at most one owner selector"}, nil)
		}
	case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		if target.ProspectiveTargetRef == "" || len(target.ProspectiveConstraintRefs) == 0 || len(target.OwnerSelectorRefs) != 1 {
			return runtimeSemanticProposalConstraint("target", RuntimeSemanticConstraintRelational, "incomplete_prospective_target", []string{"prospective target, constraints and one selector"}, nil)
		}
	default:
		return runtimeSemanticProposalConstraint("target.policy", RuntimeSemanticConstraintAllowedValue, string(target.Policy), nil, []string{string(RuntimeOperationTargetPolicyExactTarget), string(RuntimeOperationTargetPolicyExistingOwnerScope), string(RuntimeOperationTargetPolicyProspectiveOwnerTarget)})
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

func runtimeSemanticHasNonDurableEffect(values []RuntimeEffectScope) bool {
	for _, value := range values {
		if RuntimeEffectIsNonDurable(value) {
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
		for _, prerequisiteID := range item.PrerequisiteCoverageWorkItemIDs {
			writeHashString(h, "prerequisite:"+string(prerequisiteID))
		}
		for _, dependency := range item.ReopenDependencies {
			// Keep the historical reopen-dependency encoding stable.  The new
			// prerequisite relation has its own tagged section above, so an
			// existing seed containing only ReopenDependencies retains its
			// persisted hash and does not spuriously drift generations.
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

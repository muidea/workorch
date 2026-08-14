package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// RuntimeCandidateContractManifestRevision identifies the canonical pure
// projection used by the provider renderer, strict output boundary and
// correction renderer. It is deliberately separate from the executable
// operation-contract revision: this value tracks the candidate wire shape,
// while RuntimeOperationContractRevision tracks executable owner semantics.
const RuntimeCandidateContractManifestRevision = "candidate-contract-manifest"

// RuntimeOperationTargetPolicy is the only owner-neutral target authorization
// vocabulary sealed into a candidate manifest. Concrete File/Git/Shell/MCP
// semantics enter only as typed owner facts already compiled into one of these
// three policies; evaluators must not invent a fourth policy branch.
type RuntimeOperationTargetPolicy string

const (
	// RuntimeOperationTargetPolicyExactTarget requires every candidate target to
	// belong to a sealed exact set. An empty set with RequireTargetRefs=false is
	// the no-target-obligation form for kinds that do not accept targets.
	RuntimeOperationTargetPolicyExactTarget RuntimeOperationTargetPolicy = "exact_target"
	// RuntimeOperationTargetPolicyExistingOwnerScope requires every candidate
	// target to belong to a complete bounded owner scope, or to be independently
	// verifiable through a closed owner selector.
	RuntimeOperationTargetPolicyExistingOwnerScope RuntimeOperationTargetPolicy = "existing_owner_scope"
	// RuntimeOperationTargetPolicyProspectiveOwnerTarget requires a concrete new
	// target that satisfies the sealed owner create selector/prospective
	// constraint. Only the resource owner judges create feasibility.
	RuntimeOperationTargetPolicyProspectiveOwnerTarget RuntimeOperationTargetPolicy = "prospective_owner_target"
)

// RuntimeOperationTargetAuthorization is the pure target half of one sealed
// operation authorization. Exactly one Policy is active; unused ref slices must
// be empty after normalize.
type RuntimeOperationTargetAuthorization struct {
	Policy                    RuntimeOperationTargetPolicy
	ExactTargetRefs           []string
	OwnerScopeTargetRefs      []string
	OwnerSelectorRefs         []string
	ProspectiveConstraintRefs []string
	ProspectiveTargetRef      string
}

// RuntimeCandidateContractManifest is an immutable-value projection of one
// sealed RuntimeDecision. It contains no provider request, prompt content,
// aggregate mutation or action selection. Consumers must derive their schema,
// output instruction and typed diagnostics from this value rather than keep a
// second operation-field or parameter whitelist.
//
// Operations holds one RuntimeCandidateOperationManifest per allowed kind: the only
// operation-level authorization input for renderer, decoder, evaluator and
// owner feasibility.
type RuntimeCandidateContractManifest struct {
	Revision string
	Hash     string
	State    ConvergenceState
	// EffectPolicy is the canonical convergence-state policy shared by renderer,
	// candidate admission, dispatch seal and receipt evidence adjudication.
	EffectPolicy RuntimeConvergenceEffectPolicy
	// OperationContractRevision is the executable catalog fence compiled into
	// this binary. It is also stored on every operation authorization so a
	// consumer can verify one kind without re-deriving the whole inventory.
	OperationContractRevision string
	RequiredTopLevelFields    []string
	AllowedTopLevelFields     []string
	ForbiddenTopLevelFields   []string
	Bindings                  []CoverageEvidenceBinding
	TargetRefs                []string
	DependenciesAllowed       bool
	RequiredOperationSequence []RuntimeRequiredOperationVariant
	Operations                []RuntimeCandidateOperationManifest
}

// RuntimeCandidateOperationManifest is the sealed operation authorization for
// one allowed kind in the current candidate transaction. Conceptually this is
// the single RuntimeCandidateOperationManifest pure-value record: kind, owner,
// parameter/variant contract, effect/coverage/dependency constraints, one
// TargetPolicy, and snapshot/revision identity. It is not a second state object.
type RuntimeCandidateOperationManifest struct {
	Kind                   string
	OwnerModuleID          string
	AllowedOperationFields []string
	AllowedVariantIDs      []string
	Parameters             []RuntimeOperationParameterContract
	RequireOneOfParameters []string
	// TargetCardinality is projected directly from the executable descriptor.
	// It is the one wire-shape authority for outer targetRefs; parameters must
	// never restate a target path.
	TargetCardinality RuntimeOperationTargetCardinality
	// RequireTargetRefs is the wire-level obligation derived from the sealed
	// TargetPolicy and the executable operation contract. It is not an
	// independent policy source: consumers that need membership rules must read
	// Target, not invent a second allow-list.
	RequireTargetRefs           bool
	ForbiddenParameters         []string
	Variants                    []RuntimeOperationVariantContract
	Rules                       []string
	AllowsDynamicParameters     bool
	DynamicParameterPrefixes    []string
	RequiredOperationReferences []string
	// BindingAuthorizations bind proof credit to the exact operation variant and
	// required-sequence reference. An entry with no AllowedBindingRefs is an
	// explicitly auxiliary operation; callers must not infer credit from Kind.
	BindingAuthorizations []RuntimeOperationBindingAuthorization
	AcceptedEffectScopes  []RuntimeEffectScope
	RequiredEffectScopes  []RuntimeEffectScope
	Target                RuntimeOperationTargetAuthorization
	// PrerequisiteSnapshotRefs bind this authorization to owner observations
	// already sealed into the authority transaction (target selection, etc.).
	// N1 may seal selection identity; full admissibility compilation is N2.
	PrerequisiteSnapshotRefs []string
	// AdmissibilitySnapshotRefs bind this authorization to owner-authored
	// operation admissibility inventories. Empty until N2 compiles them.
	AdmissibilitySnapshotRefs []string
	// ValidationRecipes is populated only for a managed validation operation.
	// It is the activation-sealed, Shell-owner executable selection set; it is
	// not a free-form command/argument contract for the provider.
	ValidationRecipes         []RuntimeValidationRecipe
	OperationContractRevision string
}

// RuntimeOperationBindingAuthorization is one immutable proof-credit rule in
// the sealed candidate manifest. RequiredOperationRef is empty only when the
// decision has no required operation sequence for this kind.
type RuntimeOperationBindingAuthorization struct {
	RequiredOperationRef string
	VariantID            string
	AllowedBindingRefs   []string
}

func (a RuntimeOperationBindingAuthorization) Normalized() RuntimeOperationBindingAuthorization {
	a.RequiredOperationRef = strings.TrimSpace(a.RequiredOperationRef)
	a.VariantID = strings.TrimSpace(a.VariantID)
	a.AllowedBindingRefs = normalizeStrings(a.AllowedBindingRefs)
	return a
}

// RuntimeCandidateManifestOwnerFacts are the already-persisted SM-2 owner
// observations compiled into one sealed candidate authorization. They are pure
// values: the compiler never reacquires inventory, never expands truncated
// lists, and never invents domain paths.
type RuntimeCandidateManifestOwnerFacts struct {
	TargetSelection          *RuntimeTargetSelection
	OperationAdmissibilities []RuntimeOperationAdmissibility
	ValidationRecipes        []RuntimeValidationRecipe
}

// RuntimeCandidateManifestOwnerFactsFromResources projects the sealed resource
// facts used by the single manifest compiler. Callers must pass the same facts
// for Build and Validate so hash identity stays stable across owner boundaries.
func RuntimeCandidateManifestOwnerFactsFromResources(resources RuntimeResourceFacts) RuntimeCandidateManifestOwnerFacts {
	resources = resources.Normalized()
	return RuntimeCandidateManifestOwnerFacts{
		TargetSelection:          resources.TargetSelection,
		OperationAdmissibilities: append([]RuntimeOperationAdmissibility(nil), resources.OperationAdmissibilities...),
		// Recipes are executable only because they are already present in the
		// Shell owner's persisted admissibility snapshot.  Keep this projection
		// with the snapshot itself so every manifest compiler has one source of
		// truth, including restart and persistence paths that only carry facts.
		ValidationRecipes: RuntimeValidationRecipesFromOperationAdmissibilities(resources.OperationAdmissibilities),
	}
}

// RuntimeCandidateManifestOwnerFactsFromContext is the production projection
// for the manifest compiler. Resources are persisted SM-2 facts and recipes
// are immutable activation declarations; this helper performs no acquisition,
// state transition, provider rendering, or owner execution.
func RuntimeCandidateManifestOwnerFactsFromContext(context RuntimeCandidateContext) RuntimeCandidateManifestOwnerFacts {
	context = context.Normalized()
	return RuntimeCandidateManifestOwnerFactsFromResources(context.Resources).Normalized()
}

// RuntimeValidationRecipesFromOperationAdmissibilities returns the
// owner-confirmed managed-validation recipes from current resource facts. The
// helper is intentionally pure and does not turn activation declarations into
// executable authority.
func RuntimeValidationRecipesFromOperationAdmissibilities(values []RuntimeOperationAdmissibility) []RuntimeValidationRecipe {
	ret := make([]RuntimeValidationRecipe, 0)
	for _, value := range NormalizeRuntimeOperationAdmissibilities(values) {
		if value.OwnerModuleID != RuntimeModuleShell || !runtimeOperationKindsContain(value.OperationKinds, RuntimeBlockKindManagedValidationSession) {
			continue
		}
		ret = append(ret, value.ValidationRecipes...)
	}
	return NormalizeRuntimeValidationRecipes(ret)
}

// RuntimeCandidateManagedValidationRecipeSnapshot derives the exact immutable
// recipe selection a sealed dispatch needs to execute managed validation.  The
// manifest is the only membership authority; activation declarations and
// current owner inventories are deliberately not consulted here.
//
// This is a pure contract helper.  It neither selects a Runtime action nor
// executes/preflights a recipe.
func RuntimeCandidateManagedValidationRecipeSnapshot(manifest RuntimeCandidateContractManifest, candidate RuntimeCandidate) ([]RuntimeValidationRecipe, error) {
	manifest = manifest.Normalized()
	if manifest.Revision != RuntimeCandidateContractManifestRevision || manifest.Hash == "" ||
		manifest.Hash != StableRuntimeCandidateContractManifestHash(manifest) {
		return nil, fmt.Errorf("runtime candidate manifest is invalid")
	}
	operations, err := runtimeCandidatePayloadOperations(candidate)
	if err != nil {
		// Assessment and governance candidates have no executable operation
		// selection and therefore no Shell recipe snapshot.
		if candidate.Assessment != nil || candidate.Governance != nil {
			return nil, nil
		}
		return nil, err
	}
	ret := make([]RuntimeValidationRecipe, 0)
	for _, operation := range operations {
		operation = operation.Normalized()
		if operation.Kind != RuntimeBlockKindManagedValidationSession {
			continue
		}
		recipeID := runtimeValidationRecipeID(operation)
		authorization, found := runtimeCandidatePayloadOperationManifest(manifest, operation.Kind)
		if !found {
			return nil, fmt.Errorf("managed validation operation %q has no sealed authorization", operation.ID)
		}
		recipe, found := RuntimeValidationRecipeForID(authorization.ValidationRecipes, recipeID)
		if !found {
			return nil, fmt.Errorf("managed validation operation %q selected unavailable recipe %q", operation.ID, recipeID)
		}
		ret = append(ret, recipe)
	}
	return NormalizeRuntimeValidationRecipes(ret), nil
}

func (f RuntimeCandidateManifestOwnerFacts) Normalized() RuntimeCandidateManifestOwnerFacts {
	if f.TargetSelection != nil {
		selection := f.TargetSelection.Normalized()
		f.TargetSelection = &selection
	}
	f.OperationAdmissibilities = NormalizeRuntimeOperationAdmissibilities(f.OperationAdmissibilities)
	f.ValidationRecipes = NormalizeRuntimeValidationRecipes(f.ValidationRecipes)
	return f
}

// BuildRuntimeCandidateContractManifest creates the single pure contract
// projection for a sealed decision and its owner facts. TargetSelection may
// narrow operation kinds and seal TargetPolicy; OperationAdmissibilities seal
// admissibility snapshot identity and may complete existing_owner_scope /
// prospective_owner_target membership. Incomplete/truncated inventory cannot
// be sealed as scope.
func BuildRuntimeCandidateContractManifest(decision RuntimeDecision, ownerFacts RuntimeCandidateManifestOwnerFacts) (RuntimeCandidateContractManifest, error) {
	ownerFacts = ownerFacts.Normalized()
	required := decision.RequiredOutput.Normalized()
	contractRevision := RuntimeOperationContractRevision()
	manifest := RuntimeCandidateContractManifest{
		Revision:                  RuntimeCandidateContractManifestRevision,
		State:                     decision.State,
		EffectPolicy:              mustRuntimeConvergenceEffectPolicy(decision.State),
		OperationContractRevision: contractRevision,
		TargetRefs:                append([]string(nil), required.TargetRefs...),
		RequiredOperationSequence: cloneRuntimeRequiredOperationVariants(required.RequiredOperationSequence),
	}
	bindings := NormalizeCoverageEvidenceBindings(decision.Focus.CoverageEvidenceBindings)
	manifest.RequiredTopLevelFields, manifest.AllowedTopLevelFields, manifest.ForbiddenTopLevelFields = runtimeCandidateEnvelopeFields(decision.State)
	switch decision.State {
	case ConvergenceStateNeedStateChange, ConvergenceStateNeedValidation:
		if len(required.CoverageWorkItemIDs) != 0 {
			if err := ValidateCoverageEvidenceBindingsForFocus(decision.Focus, decision.AuthoritySnapshot.DefinitionRevision); err != nil {
				return RuntimeCandidateContractManifest{}, fmt.Errorf("coverage_binding_unavailable: %w", err)
			}
			manifest.Bindings = bindings
		}
		if err := ValidateRequiredOutputForState(decision.State, required); err != nil {
			return RuntimeCandidateContractManifest{}, fmt.Errorf("candidate manifest state %q requires a sealed required output", decision.State)
		}
		manifest.DependenciesAllowed = runtimeRequiredOperationSequenceHasDependency(required.RequiredOperationSequence)
		kinds := RuntimeCandidateAllowedOperationKinds(decision, ownerFacts.TargetSelection)
		if len(kinds) == 0 {
			return RuntimeCandidateContractManifest{}, fmt.Errorf("candidate manifest state %q has no allowed operation kinds", decision.State)
		}
		for _, kind := range kinds {
			operation, err := runtimeCandidateOperationManifestForKind(kind, decision.State, required, ownerFacts, contractRevision, bindings)
			if err != nil {
				return RuntimeCandidateContractManifest{}, err
			}
			manifest.Operations = append(manifest.Operations, operation)
		}
	case ConvergenceStateNeedAssessment, ConvergenceStateNeedGovernance:
		// These actions have a closed non-operation envelope. The renderer still
		// consumes the manifest so its top-level field rules are not duplicated.
	default:
		return RuntimeCandidateContractManifest{}, fmt.Errorf("convergence state %q does not accept a candidate manifest", decision.State)
	}
	manifest = manifest.Normalized()
	manifest.Hash = StableRuntimeCandidateContractManifestHash(manifest)
	return manifest, nil
}

// ValidateRuntimeCandidateContractManifestForDecision verifies that a manifest
// carried across an owner boundary is the exact pure projection of the sealed
// decision and its owner facts. It is an integrity check, not a second schema
// compiler: renderer, decoder and admission consume the supplied manifest after
// this check succeeds.
func ValidateRuntimeCandidateContractManifestForDecision(manifest RuntimeCandidateContractManifest, decision RuntimeDecision, ownerFacts RuntimeCandidateManifestOwnerFacts) error {
	manifest = manifest.Normalized()
	if manifest.Revision != RuntimeCandidateContractManifestRevision || manifest.Hash == "" ||
		manifest.Hash != StableRuntimeCandidateContractManifestHash(manifest) {
		return fmt.Errorf("runtime candidate contract manifest is invalid")
	}
	expected, err := BuildRuntimeCandidateContractManifest(decision, ownerFacts)
	if err != nil {
		return err
	}
	if manifest.Hash != expected.Hash {
		return fmt.Errorf("runtime candidate contract manifest does not match sealed authority")
	}
	return nil
}

// RuntimeCandidateAllowedOperationKinds is the owner-neutral C3 narrowing
// shared by every candidate-contract consumer. A File creation observation
// only admits durable File kinds. A File scope additionally admits a kind
// whose descriptor explicitly declares that it can act on an existing,
// File-owner-grounded target. It never grants a target or chooses an action.
func RuntimeCandidateAllowedOperationKinds(decision RuntimeDecision, selection *RuntimeTargetSelection) []string {
	required := decision.RequiredOutput.Normalized()
	kinds := append([]string(nil), required.AcceptedKinds...)
	if decision.State != ConvergenceStateNeedStateChange || selection == nil {
		return normalizeStrings(kinds)
	}
	value := selection.Normalized()
	if value.OwnerModuleID != RuntimeModuleFile ||
		(value.Disposition != RuntimeTargetSelectionCreate && value.Disposition != RuntimeTargetSelectionScope) {
		return normalizeStrings(kinds)
	}
	seen := make(map[string]struct{}, len(kinds)+len(required.RequiredOperationSequence))
	ret := make([]string, 0, len(kinds)+len(required.RequiredOperationSequence))
	for _, raw := range kinds {
		descriptor, ok := RuntimeOperationDescriptorForKind(raw)
		if !ok || !RuntimeEffectIsDurable(descriptor.DefaultEffectScope) {
			continue
		}
		allowsScopeKind := value.Disposition == RuntimeTargetSelectionScope && descriptor.AllowsGroundedWorkspaceTarget
		if descriptor.OwnerModuleID != RuntimeModuleFile && !allowsScopeKind {
			continue
		}
		if _, duplicate := seen[descriptor.Kind]; duplicate {
			continue
		}
		seen[descriptor.Kind] = struct{}{}
		ret = append(ret, descriptor.Kind)
	}
	for _, requirement := range required.RequiredOperationSequence {
		descriptor, ok := RuntimeOperationDescriptorForKind(requirement.Kind)
		if !ok {
			continue
		}
		if _, duplicate := seen[descriptor.Kind]; duplicate {
			continue
		}
		seen[descriptor.Kind] = struct{}{}
		ret = append(ret, descriptor.Kind)
	}
	sort.Strings(ret)
	return ret
}

func (m RuntimeCandidateContractManifest) Normalized() RuntimeCandidateContractManifest {
	m.Revision = strings.TrimSpace(m.Revision)
	m.Hash = strings.TrimSpace(m.Hash)
	m.State = ConvergenceState(strings.TrimSpace(string(m.State)))
	m.EffectPolicy = m.EffectPolicy.Normalized()
	m.OperationContractRevision = strings.TrimSpace(m.OperationContractRevision)
	m.RequiredTopLevelFields = normalizeStrings(m.RequiredTopLevelFields)
	m.AllowedTopLevelFields = normalizeStrings(m.AllowedTopLevelFields)
	m.ForbiddenTopLevelFields = normalizeStrings(m.ForbiddenTopLevelFields)
	m.Bindings = NormalizeCoverageEvidenceBindings(m.Bindings)
	m.TargetRefs = normalizeStrings(m.TargetRefs)
	m.RequiredOperationSequence = cloneRuntimeRequiredOperationVariants(normalizeRequiredOperationVariants(m.RequiredOperationSequence))
	operations := make([]RuntimeCandidateOperationManifest, 0, len(m.Operations))
	seen := make(map[string]struct{}, len(m.Operations))
	for _, operation := range m.Operations {
		operation = operation.Normalized()
		if operation.Kind == "" {
			continue
		}
		if _, exists := seen[operation.Kind]; exists {
			continue
		}
		seen[operation.Kind] = struct{}{}
		operations = append(operations, operation)
	}
	sort.Slice(operations, func(left, right int) bool { return operations[left].Kind < operations[right].Kind })
	m.Operations = operations
	return m
}

func (m RuntimeCandidateOperationManifest) Normalized() RuntimeCandidateOperationManifest {
	m.Kind = CanonicalRuntimeBlockKind(m.Kind)
	m.OwnerModuleID = strings.TrimSpace(m.OwnerModuleID)
	m.AllowedOperationFields = normalizeStrings(m.AllowedOperationFields)
	m.AllowedVariantIDs = normalizeStrings(m.AllowedVariantIDs)
	m.Parameters = cloneRuntimeOperationParameterContracts(m.Parameters)
	m.RequireOneOfParameters = cloneRuntimeCandidateContractStrings(m.RequireOneOfParameters)
	m.TargetCardinality = RuntimeOperationTargetCardinality(strings.TrimSpace(string(m.TargetCardinality)))
	m.ForbiddenParameters = cloneRuntimeCandidateContractStrings(m.ForbiddenParameters)
	m.Variants = cloneRuntimeOperationVariantContracts(m.Variants)
	m.Rules = cloneRuntimeCandidateContractStrings(m.Rules)
	m.DynamicParameterPrefixes = normalizeStrings(m.DynamicParameterPrefixes)
	m.RequiredOperationReferences = normalizeStrings(m.RequiredOperationReferences)
	authorizations := make([]RuntimeOperationBindingAuthorization, 0, len(m.BindingAuthorizations))
	seenAuthorizations := make(map[string]struct{}, len(m.BindingAuthorizations))
	for _, authorization := range m.BindingAuthorizations {
		authorization = authorization.Normalized()
		key := authorization.RequiredOperationRef + "\x00" + authorization.VariantID
		if _, duplicate := seenAuthorizations[key]; duplicate {
			continue
		}
		seenAuthorizations[key] = struct{}{}
		authorizations = append(authorizations, authorization)
	}
	sort.Slice(authorizations, func(i, j int) bool {
		if authorizations[i].RequiredOperationRef != authorizations[j].RequiredOperationRef {
			return authorizations[i].RequiredOperationRef < authorizations[j].RequiredOperationRef
		}
		return authorizations[i].VariantID < authorizations[j].VariantID
	})
	m.BindingAuthorizations = authorizations
	m.AcceptedEffectScopes = NormalizeRuntimeEffectScopes(m.AcceptedEffectScopes)
	m.RequiredEffectScopes = NormalizeRuntimeEffectScopes(m.RequiredEffectScopes)
	m.Target = m.Target.Normalized()
	m.PrerequisiteSnapshotRefs = normalizeStrings(m.PrerequisiteSnapshotRefs)
	m.AdmissibilitySnapshotRefs = normalizeStrings(m.AdmissibilitySnapshotRefs)
	m.ValidationRecipes = NormalizeRuntimeValidationRecipes(m.ValidationRecipes)
	m.OperationContractRevision = strings.TrimSpace(m.OperationContractRevision)
	return m
}

func (t RuntimeOperationTargetAuthorization) Normalized() RuntimeOperationTargetAuthorization {
	t.Policy = RuntimeOperationTargetPolicy(strings.TrimSpace(string(t.Policy)))
	t.ExactTargetRefs = normalizeStrings(t.ExactTargetRefs)
	t.OwnerScopeTargetRefs = normalizeStrings(t.OwnerScopeTargetRefs)
	t.OwnerSelectorRefs = normalizeStrings(t.OwnerSelectorRefs)
	t.ProspectiveConstraintRefs = normalizeStrings(t.ProspectiveConstraintRefs)
	t.ProspectiveTargetRef = strings.TrimSpace(t.ProspectiveTargetRef)
	switch t.Policy {
	case RuntimeOperationTargetPolicyExactTarget:
		t.OwnerScopeTargetRefs = nil
		t.OwnerSelectorRefs = nil
		t.ProspectiveConstraintRefs = nil
		t.ProspectiveTargetRef = ""
	case RuntimeOperationTargetPolicyExistingOwnerScope:
		t.ExactTargetRefs = nil
		t.ProspectiveConstraintRefs = nil
		t.ProspectiveTargetRef = ""
	case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		t.ExactTargetRefs = nil
		t.OwnerScopeTargetRefs = nil
	default:
		t.Policy = ""
		t.ExactTargetRefs = nil
		t.OwnerScopeTargetRefs = nil
		t.OwnerSelectorRefs = nil
		t.ProspectiveConstraintRefs = nil
		t.ProspectiveTargetRef = ""
	}
	return t
}

// RequireTargets reports whether the sealed TargetPolicy obligates a non-empty
// candidate targetRefs field. It is the pure membership of RequireTargetRefs.
func (t RuntimeOperationTargetAuthorization) RequireTargets() bool {
	t = t.Normalized()
	switch t.Policy {
	case RuntimeOperationTargetPolicyExactTarget:
		return len(t.ExactTargetRefs) > 0
	case RuntimeOperationTargetPolicyExistingOwnerScope, RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		return true
	default:
		return false
	}
}

// AllowedTargetRefs returns the bounded membership set a pure evaluator may use
// for target membership checks under the sealed TargetPolicy. Prospective
// create only returns the profile-declared prospective target when sealed.
func (t RuntimeOperationTargetAuthorization) AllowedTargetRefs() []string {
	t = t.Normalized()
	switch t.Policy {
	case RuntimeOperationTargetPolicyExactTarget:
		return append([]string(nil), t.ExactTargetRefs...)
	case RuntimeOperationTargetPolicyExistingOwnerScope:
		if len(t.OwnerScopeTargetRefs) != 0 {
			return append([]string(nil), t.OwnerScopeTargetRefs...)
		}
		return nil
	case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		if t.ProspectiveTargetRef == "" {
			return nil
		}
		return []string{t.ProspectiveTargetRef}
	default:
		return nil
	}
}

func (m RuntimeCandidateContractManifest) Clone() RuntimeCandidateContractManifest {
	return m.Normalized()
}

// StableRuntimeCandidateContractManifestHash excludes the Hash field itself
// and uses only normalized typed values. It is safe to expose in request audit
// metadata; it contains neither provider payload nor workspace content.
func StableRuntimeCandidateContractManifestHash(manifest RuntimeCandidateContractManifest) string {
	manifest = manifest.Normalized()
	manifest.Hash = ""
	h := sha256.New()
	writeHashString(h, manifest.Revision)
	writeHashString(h, string(manifest.State))
	writeHashString(h, string(manifest.EffectPolicy.State))
	writeHashString(h, fmt.Sprintf("%t", manifest.EffectPolicy.ExecutableOperationsAllowed))
	for _, scope := range manifest.EffectPolicy.AllowedOperationEffects {
		writeHashString(h, string(scope))
	}
	for _, scope := range manifest.EffectPolicy.AllowedAcceptedReceiptEffects {
		writeHashString(h, string(scope))
	}
	writeHashString(h, manifest.OperationContractRevision)
	writeHashStrings(h, manifest.RequiredTopLevelFields)
	writeHashStrings(h, manifest.AllowedTopLevelFields)
	writeHashStrings(h, manifest.ForbiddenTopLevelFields)
	for _, binding := range manifest.Bindings {
		writeHashString(h, binding.ID)
		writeHashString(h, binding.Hash)
	}
	writeHashStrings(h, manifest.TargetRefs)
	writeHashString(h, fmt.Sprintf("%t", manifest.DependenciesAllowed))
	for _, requirement := range manifest.RequiredOperationSequence {
		writeHashString(h, requirement.RequirementID)
		writeHashString(h, requirement.Kind)
		writeHashString(h, requirement.VariantID)
		writeHashStrings(h, requirement.DependsOnRequirementIDs)
		writeHashString(h, requirement.Reason)
	}
	for _, operation := range manifest.Operations {
		writeHashString(h, operation.Kind)
		writeHashString(h, operation.OwnerModuleID)
		writeHashStrings(h, operation.AllowedOperationFields)
		writeHashStrings(h, operation.AllowedVariantIDs)
		for _, parameter := range operation.Parameters {
			writeHashString(h, parameter.Name)
			writeHashString(h, string(parameter.Type))
			writeHashString(h, fmt.Sprintf("%t", parameter.Required))
			writeHashString(h, parameter.Description)
		}
		writeHashStrings(h, operation.RequireOneOfParameters)
		writeHashString(h, string(operation.TargetCardinality))
		writeHashString(h, fmt.Sprintf("%t", operation.RequireTargetRefs))
		writeHashStrings(h, operation.ForbiddenParameters)
		for _, variant := range operation.Variants {
			writeHashString(h, variant.ID)
			for _, discriminator := range variant.Discriminators {
				writeHashString(h, discriminator.Name)
				writeHashString(h, discriminator.EqualsString)
			}
			writeHashStrings(h, variant.RequiredParameters)
			writeHashStrings(h, variant.RequiredNonEmpty)
			writeHashStrings(h, variant.ForbiddenParameters)
			for _, pair := range variant.DistinctParameterPairs {
				writeHashString(h, pair.Left)
				writeHashString(h, pair.Right)
			}
			writeHashString(h, string(variant.EffectScope))
			writeHashString(h, string(variant.IdempotencyClass))
		}
		writeHashStrings(h, operation.Rules)
		writeHashString(h, fmt.Sprintf("%t", operation.AllowsDynamicParameters))
		writeHashStrings(h, operation.DynamicParameterPrefixes)
		writeHashStrings(h, operation.RequiredOperationReferences)
		for _, authorization := range operation.BindingAuthorizations {
			writeHashString(h, authorization.RequiredOperationRef)
			writeHashString(h, authorization.VariantID)
			writeHashStrings(h, authorization.AllowedBindingRefs)
		}
		for _, scope := range operation.AcceptedEffectScopes {
			writeHashString(h, string(scope))
		}
		for _, scope := range operation.RequiredEffectScopes {
			writeHashString(h, string(scope))
		}
		writeHashString(h, string(operation.Target.Policy))
		writeHashStrings(h, operation.Target.ExactTargetRefs)
		writeHashStrings(h, operation.Target.OwnerScopeTargetRefs)
		writeHashStrings(h, operation.Target.OwnerSelectorRefs)
		writeHashStrings(h, operation.Target.ProspectiveConstraintRefs)
		writeHashString(h, operation.Target.ProspectiveTargetRef)
		writeHashStrings(h, operation.PrerequisiteSnapshotRefs)
		writeHashStrings(h, operation.AdmissibilitySnapshotRefs)
		for _, recipe := range operation.ValidationRecipes {
			writeHashString(h, recipe.ID)
			writeHashString(h, recipe.Command)
			writeHashStrings(h, recipe.Args)
			writeHashString(h, fmt.Sprintf("%d", recipe.TimeoutMillis))
		}
		writeHashString(h, operation.OperationContractRevision)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func mustRuntimeConvergenceEffectPolicy(state ConvergenceState) RuntimeConvergenceEffectPolicy {
	policy, ok := RuntimeConvergenceEffectPolicyFor(state)
	if !ok {
		return RuntimeConvergenceEffectPolicy{State: state}
	}
	return policy
}

func runtimeCandidateEnvelopeFields(state ConvergenceState) (required, allowed, forbidden []string) {
	forbidden = []string{"candidate", "candidates", "executionGraph", "convergenceState", "focus", "authority", "transaction", "outputMode", "result", "runtimeState"}
	switch state {
	case ConvergenceStateNeedStateChange, ConvergenceStateNeedValidation:
		return []string{"operations"}, []string{"operations"}, append(forbidden, "id", "kind", "requiredOperationRef", "dependsOn", "coverageWorkItemIds", "targetRefs", "parameters")
	case ConvergenceStateNeedAssessment:
		return []string{"focusId", "coverageWorkItemIds", "disposition", "summary"}, []string{"focusId", "sliceId", "coverageWorkItemIds", "disposition", "items", "gap", "summary", "evidenceRefs", "artifactRefs"}, forbidden
	case ConvergenceStateNeedGovernance:
		return []string{"externalDependency"}, []string{"externalDependency"}, append(forbidden, "kind", "subjectRef", "reason")
	default:
		return nil, nil, forbidden
	}
}

func runtimeCandidateOperationManifestForKind(kind string, state ConvergenceState, required RequiredOutputContract, ownerFacts RuntimeCandidateManifestOwnerFacts, contractRevision string, bindings []CoverageEvidenceBinding) (RuntimeCandidateOperationManifest, error) {
	ownerFacts = ownerFacts.Normalized()
	contract, ok := RuntimeOperationContractForKind(kind)
	if !ok {
		return RuntimeCandidateOperationManifest{}, fmt.Errorf("candidate manifest operation kind %q has no operation contract", kind)
	}
	descriptor, ok := RuntimeOperationDescriptorForKind(kind)
	if !ok {
		return RuntimeCandidateOperationManifest{}, fmt.Errorf("candidate manifest operation kind %q has no operation descriptor", kind)
	}
	variants, allowedVariantIDs, err := runtimeCandidateManifestVariants(contract, required)
	if err != nil {
		return RuntimeCandidateOperationManifest{}, err
	}
	references := make([]string, 0)
	for _, requirement := range required.RequiredOperationSequence {
		if CanonicalRuntimeBlockKind(requirement.Kind) == contract.Kind && strings.TrimSpace(requirement.RequirementID) != "" {
			references = append(references, requirement.RequirementID)
		}
	}
	fields := []string{"id", "kind", "targetRefs", "parameters"}
	evidenceKind := ""
	switch state {
	case ConvergenceStateNeedStateChange:
		evidenceKind = EvidenceKindStateChange
	case ConvergenceStateNeedValidation:
		evidenceKind = EvidenceKindValidation
	}
	bindingAuthorizations := runtimeCandidateOperationBindingAuthorizations(required, contract, bindings, evidenceKind)
	if runtimeOperationBindingAuthorizationsHaveCredit(bindingAuthorizations) {
		fields = append(fields, "bindingRefs")
	}
	if len(references) != 0 {
		fields = append(fields, "requiredOperationRef")
	}
	if runtimeRequiredOperationSequenceHasDependency(required.RequiredOperationSequence) {
		fields = append(fields, "dependsOn")
	}
	target, prerequisiteRefs, admissibilityRefs, err := runtimeCandidateTargetAuthorizationForKind(descriptor, contract, required, ownerFacts)
	if err != nil {
		return RuntimeCandidateOperationManifest{}, err
	}
	// Wire-level RequireTargetRefs stays contract-driven for exact_target so
	// kinds without a target field (git init, shell read-only, MCP tool call, …)
	// remain legal even when the focus seals an exact membership set.
	// prospective_owner_target always obligates a concrete candidate target.
	// existing_owner_scope only obligates a target when the sealed scope actually
	// carries membership TargetRefs; selector-only catalog bindings do not.
	requireTargetRefs := contract.RequireTargetRefs
	switch target.Normalized().Policy {
	case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		requireTargetRefs = true
	case RuntimeOperationTargetPolicyExistingOwnerScope:
		if len(target.OwnerScopeTargetRefs) != 0 {
			requireTargetRefs = true
		}
	}
	return RuntimeCandidateOperationManifest{
		Kind:                        contract.Kind,
		OwnerModuleID:               descriptor.OwnerModuleID,
		AllowedOperationFields:      fields,
		AllowedVariantIDs:           allowedVariantIDs,
		Parameters:                  cloneRuntimeOperationParameterContracts(contract.Parameters),
		RequireOneOfParameters:      append([]string(nil), contract.RequireOneOfParameters...),
		TargetCardinality:           descriptor.TargetCardinality,
		RequireTargetRefs:           requireTargetRefs,
		ForbiddenParameters:         append([]string(nil), contract.ForbiddenParameters...),
		Variants:                    variants,
		Rules:                       append([]string(nil), contract.Rules...),
		AllowsDynamicParameters:     runtimeCandidateManifestAllowsDynamicParameters(contract.Kind),
		DynamicParameterPrefixes:    runtimeCandidateManifestDynamicParameterPrefixes(contract.Kind),
		RequiredOperationReferences: references,
		BindingAuthorizations:       bindingAuthorizations,
		AcceptedEffectScopes:        append([]RuntimeEffectScope(nil), required.AcceptedEffectScopes...),
		RequiredEffectScopes:        append([]RuntimeEffectScope(nil), required.RequiredEffectScopes...),
		Target:                      target,
		PrerequisiteSnapshotRefs:    prerequisiteRefs,
		AdmissibilitySnapshotRefs:   admissibilityRefs,
		ValidationRecipes:           runtimeCandidateManifestValidationRecipes(contract.Kind, ownerFacts.ValidationRecipes, bindings, runtimeOperationBindingAuthorizationRefs(bindingAuthorizations)),
		OperationContractRevision:   contractRevision,
	}.Normalized(), nil
}

func runtimeCandidateOperationBindingAuthorizations(required RequiredOutputContract, contract RuntimeOperationContract, bindings []CoverageEvidenceBinding, evidenceKind string) []RuntimeOperationBindingAuthorization {
	requirements := make([]RuntimeRequiredOperationVariant, 0)
	for _, requirement := range required.RequiredOperationSequence {
		if CanonicalRuntimeBlockKind(requirement.Kind) == contract.Kind {
			requirements = append(requirements, requirement)
		}
	}
	ret := make([]RuntimeOperationBindingAuthorization, 0)
	if len(requirements) != 0 {
		for _, requirement := range requirements {
			ret = append(ret, RuntimeOperationBindingAuthorization{
				RequiredOperationRef: requirement.RequirementID,
				VariantID:            requirement.VariantID,
				AllowedBindingRefs:   coverageEvidenceBindingRefsForOperationVariant(bindings, contract.Kind, requirement.VariantID, evidenceKind),
			})
		}
		return ret
	}
	for _, variant := range contract.Variants {
		ret = append(ret, RuntimeOperationBindingAuthorization{
			VariantID:          variant.ID,
			AllowedBindingRefs: coverageEvidenceBindingRefsForOperationVariant(bindings, contract.Kind, variant.ID, evidenceKind),
		})
	}
	if len(ret) == 0 {
		ret = append(ret, RuntimeOperationBindingAuthorization{AllowedBindingRefs: CoverageEvidenceBindingRefsForOperationEvidenceKind(bindings, contract.Kind, evidenceKind)})
	}
	return ret
}

func coverageEvidenceBindingRefsForOperationVariant(bindings []CoverageEvidenceBinding, kind, variantID, evidenceKind string) []string {
	kind = CanonicalRuntimeBlockKind(kind)
	variantID = strings.TrimSpace(variantID)
	ret := make([]string, 0)
	for _, binding := range NormalizeCoverageEvidenceBindings(bindings) {
		proof, found := CoverageEvidenceProofForKind(binding, evidenceKind)
		if !found || !runtimeOperationKindsContain(proof.AllowedOperationKinds, kind) {
			continue
		}
		if len(proof.AllowedOperationVariants) != 0 {
			matched := false
			for _, allowed := range proof.AllowedOperationVariants {
				if CanonicalRuntimeBlockKind(allowed.Kind) == kind && allowed.VariantID == variantID {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		ret = append(ret, binding.ID)
	}
	return normalizeStrings(ret)
}

func runtimeOperationBindingAuthorizationsHaveCredit(values []RuntimeOperationBindingAuthorization) bool {
	return len(runtimeOperationBindingAuthorizationRefs(values)) != 0
}

func runtimeOperationBindingAuthorizationRefs(values []RuntimeOperationBindingAuthorization) []string {
	ret := make([]string, 0)
	for _, value := range values {
		ret = append(ret, value.AllowedBindingRefs...)
	}
	return normalizeStrings(ret)
}

func runtimeCandidateManifestValidationRecipes(kind string, recipes []RuntimeValidationRecipe, bindings []CoverageEvidenceBinding, bindingRefs []string) []RuntimeValidationRecipe {
	if CanonicalRuntimeBlockKind(kind) != RuntimeBlockKindManagedValidationSession {
		return nil
	}
	// An owner admissibility snapshot may contain recipes for several CWI.  A
	// managed-validation operation that elects a coverage binding must only see
	// the exact recipe sealed in that binding; otherwise an unrelated but
	// executable recipe reaches the provider, then fails only after receipt
	// matching.  An operation with no binding remains auxiliary/non-crediting
	// work and retains the owner-confirmed set.
	bindingRefs = normalizeStrings(bindingRefs)
	if len(bindingRefs) == 0 {
		return NormalizeRuntimeValidationRecipes(recipes)
	}
	allowed := make(map[string]struct{}, len(bindingRefs))
	for _, ref := range bindingRefs {
		binding, found := CoverageEvidenceBindingForRef(bindings, ref)
		if !found {
			continue
		}
		proof, found := CoverageEvidenceProofForKind(binding, EvidenceKindValidation)
		if !found || !runtimeOperationKindsContain(proof.AllowedOperationKinds, RuntimeBlockKindManagedValidationSession) || proof.ValidationRecipeID == "" {
			continue
		}
		allowed[proof.ValidationRecipeID] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil
	}
	ret := make([]RuntimeValidationRecipe, 0, len(allowed))
	for _, recipe := range NormalizeRuntimeValidationRecipes(recipes) {
		if _, found := allowed[recipe.ID]; found {
			ret = append(ret, recipe)
		}
	}
	return NormalizeRuntimeValidationRecipes(ret)
}

// runtimeCandidateTargetAuthorizationForKind compiles one of the three sealed
// TargetPolicy values from already-typed decision/selection/admissibility
// facts. It does not invent domain paths, does not call owners, and does not
// expand incomplete inventory into a scope.
//
// Compilation priority:
//  1. decision-level exact TargetRefs when no create/scope owner fact applies;
//  2. TargetSelection Create / Scope / Selected dispositions;
//  3. matching OperationAdmissibility inventory for the kind's owner;
//  4. exact_target with empty membership for kinds without target obligation.
func runtimeCandidateTargetAuthorizationForKind(
	descriptor RuntimeOperationDescriptor,
	contract RuntimeOperationContract,
	required RequiredOutputContract,
	ownerFacts RuntimeCandidateManifestOwnerFacts,
) (RuntimeOperationTargetAuthorization, []string, []string, error) {
	ownerFacts = ownerFacts.Normalized()
	exactRefs := normalizeStrings(required.TargetRefs)
	admissibility, hasAdmissibility := runtimeCandidateAdmissibilityForKind(ownerFacts.OperationAdmissibilities, descriptor.OwnerModuleID, descriptor.Kind)
	var admissibilityRefs []string
	if hasAdmissibility {
		if err := runtimeCandidateValidateAdmissibilityInventory(admissibility); err != nil {
			return RuntimeOperationTargetAuthorization{}, nil, nil, err
		}
		if admissibility.SnapshotID != "" {
			admissibilityRefs = []string{admissibility.SnapshotID}
		}
	}

	if ownerFacts.TargetSelection == nil {
		if target, ok := runtimeCandidateTargetAuthorizationFromAdmissibility(descriptor, contract, required, admissibility, hasAdmissibility, exactRefs, false); ok {
			return target, nil, admissibilityRefs, nil
		}
		return RuntimeOperationTargetAuthorization{
			Policy:          RuntimeOperationTargetPolicyExactTarget,
			ExactTargetRefs: exactRefs,
		}, nil, admissibilityRefs, nil
	}
	value := ownerFacts.TargetSelection.Normalized()
	var prerequisiteRefs []string
	if value.SnapshotID != "" {
		prerequisiteRefs = []string{value.SnapshotID}
	}
	switch value.Disposition {
	case RuntimeTargetSelectionCreate:
		// Create kind-narrowing already admits only durable File kinds (or
		// AllowsNewTarget owners). A sealed ProspectiveTargetRef turns that into
		// prospective_owner_target; without it the authorization stays exact and
		// only kind narrowing applies.
		allowsProspective, ownerSelectors, ownerConstraints := runtimeCandidateAdmissibilityAllowsProspective(required, contract, admissibility, hasAdmissibility)
		prospectiveRef := value.ProspectiveTargetRef
		if prospectiveRef == "" && len(exactRefs) == 1 {
			// Required output may already seal the single create path while the
			// selection disposition still marks create. Prefer prospective so the
			// authorization carries create semantics rather than a second exact set.
			prospectiveRef = exactRefs[0]
		}
		if !allowsProspective || prospectiveRef == "" {
			if target, ok := runtimeCandidateTargetAuthorizationFromAdmissibility(descriptor, contract, required, admissibility, hasAdmissibility, exactRefs, true); ok {
				return target, prerequisiteRefs, admissibilityRefs, nil
			}
			return RuntimeOperationTargetAuthorization{}, nil, nil, fmt.Errorf("candidate manifest prospective target for %q has no explicit owner-admissible operation variant", descriptor.Kind)
		}
		selectorRefs := make([]string, 0, 2)
		if value.SelectorRef != "" {
			selectorRefs = append(selectorRefs, value.SelectorRef)
		}
		constraintRefs := append([]string(nil), value.SelectorCandidateRefs...)
		if hasAdmissibility {
			selectorRefs = normalizeStrings(append(selectorRefs, ownerSelectors...))
			constraintRefs = normalizeStrings(append(constraintRefs, ownerConstraints...))
		}
		return RuntimeOperationTargetAuthorization{
			Policy:                    RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			OwnerSelectorRefs:         selectorRefs,
			ProspectiveConstraintRefs: constraintRefs,
			ProspectiveTargetRef:      prospectiveRef,
		}, prerequisiteRefs, admissibilityRefs, nil
	case RuntimeTargetSelectionScope:
		if len(exactRefs) != 0 {
			// Decision-level exact refs still win over a broader owner scope.
			return RuntimeOperationTargetAuthorization{
				Policy:          RuntimeOperationTargetPolicyExactTarget,
				ExactTargetRefs: exactRefs,
			}, prerequisiteRefs, admissibilityRefs, nil
		}
		scopeRefs := append([]string(nil), value.TargetRefs...)
		if len(scopeRefs) == 0 {
			scopeRefs = append([]string(nil), value.CandidateTargetRefs...)
		}
		if len(scopeRefs) == 0 && hasAdmissibility && len(admissibility.TargetRefs) != 0 {
			scopeRefs = append([]string(nil), admissibility.TargetRefs...)
		}
		if len(scopeRefs) == 0 {
			// Empty-workspace scope without targets is not a membership set.
			// Closed selector-only inventories stay prospective/selector facts,
			// not truncated scopes.
			if hasAdmissibility && len(admissibility.SelectorRefs) != 0 && len(admissibility.TargetRefs) == 0 {
				return RuntimeOperationTargetAuthorization{
					Policy:            RuntimeOperationTargetPolicyExistingOwnerScope,
					OwnerSelectorRefs: append([]string(nil), admissibility.SelectorRefs...),
				}, prerequisiteRefs, admissibilityRefs, nil
			}
			return RuntimeOperationTargetAuthorization{
				Policy: RuntimeOperationTargetPolicyExactTarget,
			}, prerequisiteRefs, admissibilityRefs, nil
		}
		// A non-empty workspace may carry both a bounded existing inventory and
		// an activation-sealed prospective deliverable. When the selected owner
		// kind/variant explicitly authorizes creation, the prospective target is
		// the only valid policy for this authorization. Do not downgrade it to
		// selector-only existing_owner_scope: that policy would erase the
		// prospective target and allow an existing inventory member to satisfy a
		// binding intended for the new deliverable. The finite inventory remains
		// an owner freshness fact and is checked by the owner feasibility probe.
		if value.ProspectiveTargetRef != "" && hasAdmissibility {
			if allowsProspective, ownerSelectors, ownerConstraints := runtimeCandidateAdmissibilityAllowsProspective(required, contract, admissibility, true); allowsProspective {
				selectorRefs := make([]string, 0, 1+len(ownerSelectors))
				if value.SelectorRef != "" {
					selectorRefs = append(selectorRefs, value.SelectorRef)
				}
				selectorRefs = normalizeStrings(append(selectorRefs, ownerSelectors...))
				return RuntimeOperationTargetAuthorization{
					Policy:                    RuntimeOperationTargetPolicyProspectiveOwnerTarget,
					OwnerSelectorRefs:         selectorRefs,
					ProspectiveConstraintRefs: ownerConstraints,
					ProspectiveTargetRef:      value.ProspectiveTargetRef,
				}, prerequisiteRefs, admissibilityRefs, nil
			}
		}
		if value.CandidateCount != 0 && value.CandidateCount != len(scopeRefs) {
			return RuntimeOperationTargetAuthorization{}, nil, nil, fmt.Errorf("candidate manifest existing owner scope requires a complete bounded target set")
		}
		if !descriptor.AllowsGroundedWorkspaceTarget && descriptor.OwnerModuleID != value.OwnerModuleID && !contract.RequireTargetRefs {
			// Non-target kinds outside the scope owner stay exact/empty.
			return RuntimeOperationTargetAuthorization{
				Policy: RuntimeOperationTargetPolicyExactTarget,
			}, prerequisiteRefs, admissibilityRefs, nil
		}
		selectorRefs := make([]string, 0, 2)
		if value.SelectorRef != "" {
			selectorRefs = append(selectorRefs, value.SelectorRef)
		}
		if hasAdmissibility {
			selectorRefs = normalizeStrings(append(selectorRefs, admissibility.SelectorRefs...))
		}
		return RuntimeOperationTargetAuthorization{
			Policy:               RuntimeOperationTargetPolicyExistingOwnerScope,
			OwnerScopeTargetRefs: scopeRefs,
			OwnerSelectorRefs:    selectorRefs,
		}, prerequisiteRefs, admissibilityRefs, nil
	case RuntimeTargetSelectionSelected:
		if len(exactRefs) != 0 {
			return RuntimeOperationTargetAuthorization{
				Policy:          RuntimeOperationTargetPolicyExactTarget,
				ExactTargetRefs: exactRefs,
			}, prerequisiteRefs, admissibilityRefs, nil
		}
		if len(value.TargetRefs) == 0 {
			if target, ok := runtimeCandidateTargetAuthorizationFromAdmissibility(descriptor, contract, required, admissibility, hasAdmissibility, exactRefs, false); ok {
				return target, prerequisiteRefs, admissibilityRefs, nil
			}
			return RuntimeOperationTargetAuthorization{
				Policy: RuntimeOperationTargetPolicyExactTarget,
			}, prerequisiteRefs, admissibilityRefs, nil
		}
		if len(value.TargetRefs) != 1 {
			return RuntimeOperationTargetAuthorization{}, nil, nil, fmt.Errorf("candidate manifest exact target selection requires exactly one sealed target")
		}
		return RuntimeOperationTargetAuthorization{
			Policy:          RuntimeOperationTargetPolicyExactTarget,
			ExactTargetRefs: append([]string(nil), value.TargetRefs...),
		}, prerequisiteRefs, admissibilityRefs, nil
	default:
		if target, ok := runtimeCandidateTargetAuthorizationFromAdmissibility(descriptor, contract, required, admissibility, hasAdmissibility, exactRefs, false); ok {
			return target, prerequisiteRefs, admissibilityRefs, nil
		}
		return RuntimeOperationTargetAuthorization{
			Policy:          RuntimeOperationTargetPolicyExactTarget,
			ExactTargetRefs: exactRefs,
		}, prerequisiteRefs, admissibilityRefs, nil
	}
}

func runtimeCandidateAdmissibilityForKind(values []RuntimeOperationAdmissibility, ownerModuleID, kind string) (RuntimeOperationAdmissibility, bool) {
	kind = CanonicalRuntimeBlockKind(kind)
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	for _, value := range values {
		value = value.Normalized()
		if value.OwnerModuleID != ownerModuleID {
			continue
		}
		for _, operationKind := range value.OperationKinds {
			if operationKind == kind {
				return value, true
			}
		}
	}
	return RuntimeOperationAdmissibility{}, false
}

func runtimeCandidateValidateAdmissibilityInventory(value RuntimeOperationAdmissibility) error {
	value = value.Normalized()
	if err := value.Validate(); err != nil {
		return fmt.Errorf("candidate manifest operation admissibility is invalid: %w", err)
	}
	itemCount := len(value.TargetRefs) + len(value.SelectorRefs) + len(value.ConstraintRefs)
	if !RuntimeOwnerInventoryWithinBudget(DefaultRuntimeOwnerInventoryBudget(), itemCount) {
		return fmt.Errorf("candidate manifest operation admissibility exceeds owner inventory budget")
	}
	// A non-empty TargetRefs set is a complete bounded inventory. Selector-only
	// inventories are closed owner selectors and may seal without TargetRefs.
	// Mixing a non-empty TargetRefs set with a CandidateCount-style incompleteness
	// is rejected earlier by Validate; here we only refuse oversized inventories.
	return nil
}

// runtimeCandidateTargetAuthorizationFromAdmissibility seals TargetPolicy from a
// matching owner admissibility inventory when TargetSelection does not already
// provide create/scope/selected membership. Incomplete inventories never seal.
func runtimeCandidateTargetAuthorizationFromAdmissibility(
	descriptor RuntimeOperationDescriptor,
	contract RuntimeOperationContract,
	required RequiredOutputContract,
	admissibility RuntimeOperationAdmissibility,
	hasAdmissibility bool,
	exactRefs []string,
	creationRequested bool,
) (RuntimeOperationTargetAuthorization, bool) {
	if !hasAdmissibility {
		return RuntimeOperationTargetAuthorization{}, false
	}
	if len(exactRefs) != 0 && !creationRequested {
		return RuntimeOperationTargetAuthorization{
			Policy:          RuntimeOperationTargetPolicyExactTarget,
			ExactTargetRefs: exactRefs,
		}, true
	}
	if len(admissibility.TargetRefs) != 0 && !creationRequested {
		if !descriptor.AllowsGroundedWorkspaceTarget && descriptor.OwnerModuleID != admissibility.OwnerModuleID && !contract.RequireTargetRefs {
			return RuntimeOperationTargetAuthorization{
				Policy: RuntimeOperationTargetPolicyExactTarget,
			}, true
		}
		return RuntimeOperationTargetAuthorization{
			Policy:               RuntimeOperationTargetPolicyExistingOwnerScope,
			OwnerScopeTargetRefs: append([]string(nil), admissibility.TargetRefs...),
			OwnerSelectorRefs:    append([]string(nil), admissibility.SelectorRefs...),
		}, true
	}
	if len(admissibility.SelectorRefs) != 0 || len(admissibility.ConstraintRefs) != 0 {
		allowsProspective, selectorRefs, constraintRefs := runtimeCandidateAdmissibilityAllowsProspective(required, contract, admissibility, true)
		if allowsProspective {
			return RuntimeOperationTargetAuthorization{
				Policy:                    RuntimeOperationTargetPolicyProspectiveOwnerTarget,
				OwnerSelectorRefs:         append([]string(nil), selectorRefs...),
				ProspectiveConstraintRefs: append([]string(nil), constraintRefs...),
			}, true
		}
		if creationRequested {
			return RuntimeOperationTargetAuthorization{}, false
		}
		return RuntimeOperationTargetAuthorization{
			Policy:            RuntimeOperationTargetPolicyExistingOwnerScope,
			OwnerSelectorRefs: append([]string(nil), admissibility.SelectorRefs...),
		}, true
	}
	return RuntimeOperationTargetAuthorization{}, false
}

// runtimeCandidateAdmissibilityAllowsProspective reads only the sealed owner
// catalog.  When a kind has discriminated variants, an explicit required
// variant is mandatory whenever policies differ; the compiler never falls back
// to descriptor owner/effect inference.
func runtimeCandidateAdmissibilityAllowsProspective(required RequiredOutputContract, contract RuntimeOperationContract, admissibility RuntimeOperationAdmissibility, hasAdmissibility bool) (bool, []string, []string) {
	if !hasAdmissibility {
		return false, nil, nil
	}
	required = required.Normalized()
	variantIDs := runtimeCandidateRequiredVariantIDs(contract.Kind, contract, required)
	allowed, selectors, constraints, _, err := RuntimeOperationAdmissibilityAllowsTargetPolicyForVariants(
		admissibility, contract.Kind, variantIDs, RuntimeOperationTargetPolicyProspectiveOwnerTarget,
	)
	if err != nil {
		return false, nil, nil
	}
	return allowed, selectors, constraints
}

func runtimeCandidateRequiredVariantIDs(kind string, contract RuntimeOperationContract, required RequiredOutputContract) []string {
	kind = CanonicalRuntimeBlockKind(kind)
	seen := map[string]struct{}{}
	ret := make([]string, 0)
	for _, ref := range required.AcceptedOperationVariants {
		if CanonicalRuntimeBlockKind(ref.Kind) == kind && strings.TrimSpace(ref.VariantID) != "" {
			if _, ok := seen[ref.VariantID]; !ok {
				seen[ref.VariantID] = struct{}{}
				ret = append(ret, strings.TrimSpace(ref.VariantID))
			}
		}
	}
	for _, ref := range required.RequiredOperationSequence {
		if CanonicalRuntimeBlockKind(ref.Kind) == kind && strings.TrimSpace(ref.VariantID) != "" {
			if _, ok := seen[ref.VariantID]; !ok {
				seen[ref.VariantID] = struct{}{}
				ret = append(ret, strings.TrimSpace(ref.VariantID))
			}
		}
	}
	sort.Strings(ret)
	return ret
}

func runtimeCandidateManifestVariants(contract RuntimeOperationContract, required RequiredOutputContract) ([]RuntimeOperationVariantContract, []string, error) {
	allowed := make(map[string]struct{})
	for _, ref := range required.AcceptedOperationVariants {
		if ref.Kind == contract.Kind {
			allowed[ref.VariantID] = struct{}{}
		}
	}
	for _, requirement := range required.RequiredOperationSequence {
		if CanonicalRuntimeBlockKind(requirement.Kind) == contract.Kind && strings.TrimSpace(requirement.VariantID) != "" {
			allowed[strings.TrimSpace(requirement.VariantID)] = struct{}{}
		}
	}
	variants := make([]RuntimeOperationVariantContract, 0, len(contract.Variants))
	for _, variant := range contract.Variants {
		if len(allowed) != 0 {
			if _, ok := allowed[variant.ID]; !ok {
				continue
			}
		}
		variants = append(variants, cloneRuntimeOperationVariantContract(variant))
	}
	if len(allowed) != 0 && len(variants) != len(allowed) {
		return nil, nil, fmt.Errorf("candidate manifest operation kind %q references an unknown sealed variant", contract.Kind)
	}
	ids := make([]string, 0, len(variants))
	for _, variant := range variants {
		ids = append(ids, variant.ID)
	}
	return variants, normalizeStrings(ids), nil
}

func runtimeRequiredOperationSequenceHasDependency(values []RuntimeRequiredOperationVariant) bool {
	for _, value := range values {
		if len(value.DependsOnRequirementIDs) != 0 {
			return true
		}
	}
	return false
}

func runtimeCandidateManifestAllowsDynamicParameters(kind string) bool {
	return len(runtimeCandidateManifestDynamicParameterPrefixes(kind)) != 0
}

func runtimeCandidateManifestDynamicParameterPrefixes(kind string) []string {
	switch CanonicalRuntimeBlockKind(kind) {
	case StepKindMCPToolCall:
		return []string{"input."}
	case StepKindMCPPromptGet:
		return []string{"arguments."}
	default:
		return nil
	}
}

func cloneRuntimeOperationParameterContracts(values []RuntimeOperationParameterContract) []RuntimeOperationParameterContract {
	return append([]RuntimeOperationParameterContract(nil), values...)
}

func cloneRuntimeOperationVariantContract(value RuntimeOperationVariantContract) RuntimeOperationVariantContract {
	value.Discriminators = append([]RuntimeOperationParameterPredicate(nil), value.Discriminators...)
	value.RequiredParameters = cloneRuntimeCandidateContractStrings(value.RequiredParameters)
	value.RequiredNonEmpty = cloneRuntimeCandidateContractStrings(value.RequiredNonEmpty)
	value.ForbiddenParameters = cloneRuntimeCandidateContractStrings(value.ForbiddenParameters)
	value.DistinctParameterPairs = append([]RuntimeOperationParameterPair(nil), value.DistinctParameterPairs...)
	return value
}

func cloneRuntimeOperationVariantContracts(values []RuntimeOperationVariantContract) []RuntimeOperationVariantContract {
	ret := make([]RuntimeOperationVariantContract, 0, len(values))
	for _, value := range values {
		ret = append(ret, cloneRuntimeOperationVariantContract(value))
	}
	return ret
}

func cloneRuntimeCandidateContractStrings(values []string) []string {
	ret := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	return ret
}

func cloneRuntimeRequiredOperationVariants(values []RuntimeRequiredOperationVariant) []RuntimeRequiredOperationVariant {
	ret := make([]RuntimeRequiredOperationVariant, 0, len(values))
	for _, value := range values {
		value.RequirementID = strings.TrimSpace(value.RequirementID)
		value.Kind = CanonicalRuntimeBlockKind(value.Kind)
		value.VariantID = strings.TrimSpace(value.VariantID)
		value.DependsOnRequirementIDs = normalizeStrings(value.DependsOnRequirementIDs)
		value.Reason = strings.TrimSpace(value.Reason)
		ret = append(ret, value)
	}
	return ret
}

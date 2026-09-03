package workorchruntimecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// RuntimeCandidateContractManifestRevision identifies the canonical pure
// projection used by the provider renderer, strict output boundary and
// correction renderer. It is deliberately separate from the executable
// operation-contract revision: this value tracks the candidate wire shape,
// while RuntimeOperationContractRevision tracks executable owner semantics.
const RuntimeCandidateContractManifestRevision = "candidate-contract-manifest/v6"

// RuntimeCandidateOperationMaxCount is the compiler ceiling for one complete
// provider candidate document. The compiled manifest copies this value into
// MaxOperationCount; every schema, strict decoder and local evaluator must read
// the sealed manifest field rather than infer cardinality from allowed kinds or
// required-operation count.
const RuntimeCandidateOperationMaxCount = 64

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
	ProspectiveTargetRefs     []string
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
	// MaxOperationCount is the single sealed cardinality source for an
	// operation-bearing candidate document. It is zero for non-operation
	// convergence states.
	MaxOperationCount int
	// OperationSetPolicy is the sole candidate-wide authorization rule. It is
	// compiled before provider I/O and prevents consumers from reconstructing
	// owner/target cardinality from mutable facts or global descriptors.
	OperationSetPolicy      RuntimeCandidateOperationSetPolicy
	RequiredTopLevelFields  []string
	AllowedTopLevelFields   []string
	ForbiddenTopLevelFields []string
	Bindings                []CoverageEvidenceBinding
	TargetRefs              []string
	RepairClosureID         string
	RepairPurpose           RuntimeDispatchRepairPurpose
	// AssessmentPurpose is rooted in the earlier semantic provider/owner batch
	// named by AssessmentPurposeManifestHash. Both fields are empty outside an
	// assessment transaction.
	AssessmentPurpose             RuntimeAssessmentPurpose
	AssessmentPurposeManifestHash string
	RequiredOperationSequence     []RuntimeRequiredOperationVariant
	Operations                    []RuntimeCandidateOperationManifest
}

// RuntimeCandidateOperationSetPolicy is the closed, owner-neutral policy for
// relations between operations in one candidate. NeedStateChange operations
// are already required by EffectPolicy to be durable, so the owner/target
// cardinality below cannot accidentally classify an operation by importing an
// owner catalog at admission time.
type RuntimeCandidateOperationSetPolicy struct {
	MaxOperationsPerOwnerTarget           int
	AllowRequiredOperationSequenceOverlap bool
}

func runtimeCandidateOperationSetPolicyForState(state ConvergenceState) RuntimeCandidateOperationSetPolicy {
	if state == ConvergenceStateNeedStateChange {
		return RuntimeCandidateOperationSetPolicy{
			MaxOperationsPerOwnerTarget:           1,
			AllowRequiredOperationSequenceOverlap: true,
		}
	}
	return RuntimeCandidateOperationSetPolicy{}
}

func (p RuntimeCandidateOperationSetPolicy) Validate(state ConvergenceState) error {
	expected := runtimeCandidateOperationSetPolicyForState(state)
	if p != expected {
		return fmt.Errorf("candidate manifest state %q has invalid operation-set policy", state)
	}
	return nil
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
	// RequiredOperationFields is the single wire-level required-field source
	// consumed by schema, prompt and strict payload validation. Authority fields
	// such as operation ID and dependencies are deliberately absent: the
	// contract owner derives them after the provider document is decoded.
	RequiredOperationFields []string
	AllowedVariantIDs       []string
	Parameters              []RuntimeOperationParameterContract
	RequireOneOfParameters  []string
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
	// BindingAuthorizations bind proof credit, or the exact rejected-validation
	// causation lineage of a confirmed repair, to the operation variant and
	// required-sequence reference. An entry with no AllowedBindingRefs is an
	// explicitly auxiliary operation; callers must not infer either relation
	// from Kind.
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
	TargetSelection               *RuntimeTargetSelection
	OperationAdmissibilities      []RuntimeOperationAdmissibility
	ValidationRecipes             []RuntimeValidationRecipe
	AssessmentPurpose             RuntimeAssessmentPurpose
	AssessmentPurposeManifestHash string
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
	f.AssessmentPurpose = RuntimeAssessmentPurpose(strings.TrimSpace(string(f.AssessmentPurpose)))
	f.AssessmentPurposeManifestHash = strings.TrimSpace(f.AssessmentPurposeManifestHash)
	return f
}

// BuildRuntimeCandidateContractManifest creates the single pure contract
// projection for a sealed decision and its owner facts. TargetSelection may
// narrow operation kinds and seal TargetPolicy; OperationAdmissibilities seal
// admissibility snapshot identity and may complete existing_owner_scope /
// prospective_owner_target membership. Incomplete/truncated inventory cannot
// be sealed as scope.
func BuildRuntimeCandidateContractManifest(decision RuntimeDecision, ownerFacts RuntimeCandidateManifestOwnerFacts) (RuntimeCandidateContractManifest, error) {
	if err := decision.RequiredOutput.Validate(); err != nil {
		return RuntimeCandidateContractManifest{}, fmt.Errorf("candidate manifest required operation sequence: %w", err)
	}
	ownerFacts = ownerFacts.Normalized()
	required := decision.RequiredOutput.Normalized()
	contractRevision := RuntimeOperationContractRevision()
	manifest := RuntimeCandidateContractManifest{
		Revision:                  RuntimeCandidateContractManifestRevision,
		State:                     decision.State,
		EffectPolicy:              mustRuntimeConvergenceEffectPolicy(decision.State),
		OperationContractRevision: contractRevision,
		OperationSetPolicy:        runtimeCandidateOperationSetPolicyForState(decision.State),
		TargetRefs:                append([]string(nil), required.TargetRefs...),
		RepairClosureID:           required.RepairClosureID,
		RepairPurpose:             required.RepairPurpose,
		RequiredOperationSequence: cloneRuntimeRequiredOperationVariants(required.RequiredOperationSequence),
	}
	bindings := NormalizeCoverageEvidenceBindings(decision.Focus.CoverageEvidenceBindings)
	manifest.RequiredTopLevelFields, manifest.AllowedTopLevelFields, manifest.ForbiddenTopLevelFields = runtimeCandidateEnvelopeFields(decision.State)
	switch decision.State {
	case ConvergenceStateNeedStateChange, ConvergenceStateNeedValidation:
		manifest.MaxOperationCount = RuntimeCandidateOperationMaxCount
		if len(required.CoverageWorkItemIDs) != 0 {
			if err := ValidateCoverageEvidenceBindingsForFocus(decision.Focus, decision.AuthoritySnapshot.DefinitionRevision); err != nil {
				return RuntimeCandidateContractManifest{}, fmt.Errorf("coverage_binding_unavailable: %w", err)
			}
			manifest.Bindings = bindings
		}
		if err := ValidateRequiredOutputForState(decision.State, required); err != nil {
			return RuntimeCandidateContractManifest{}, fmt.Errorf("candidate manifest state %q requires a sealed required output", decision.State)
		}
		kinds := RuntimeCandidateAllowedOperationKinds(decision, ownerFacts.TargetSelection, ownerFacts.OperationAdmissibilities)
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
	case ConvergenceStateNeedAssessment:
		// Assessment has a closed non-operation envelope, but its post-seal
		// adjudication is still scoped to the exact semantic proof bindings of the
		// current focus. Persist that projection in the pre-seal manifest so the
		// dispatch assessment input cannot reconstruct it from mutable facts.
		if len(bindings) == 0 {
			return RuntimeCandidateContractManifest{}, fmt.Errorf("coverage_binding_unavailable: assessment manifest requires one semantic binding preimage")
		}
		if err := ValidateCoverageEvidenceBindingProjection(decision.Focus.CoverageEvidenceBindings); err != nil {
			return RuntimeCandidateContractManifest{}, fmt.Errorf("coverage_binding_unavailable: %w", err)
		}
		if err := ValidateCoverageEvidenceBindingsForFocus(decision.Focus, decision.AuthoritySnapshot.DefinitionRevision); err != nil {
			return RuntimeCandidateContractManifest{}, fmt.Errorf("coverage_binding_unavailable: %w", err)
		}
		manifest.Bindings = bindings
		if ownerFacts.AssessmentPurpose != RuntimeAssessmentPurposeDefinitionProof &&
			ownerFacts.AssessmentPurpose != RuntimeAssessmentPurposeOwnerFactAcquisition {
			return RuntimeCandidateContractManifest{}, fmt.Errorf("assessment_purpose_unavailable: assessment manifest requires an exact semantic purpose")
		}
		if ownerFacts.AssessmentPurposeManifestHash == "" {
			return RuntimeCandidateContractManifest{}, fmt.Errorf("assessment_purpose_unavailable: assessment manifest requires an exact semantic manifest preimage")
		}
		manifest.AssessmentPurpose = ownerFacts.AssessmentPurpose
		manifest.AssessmentPurposeManifestHash = ownerFacts.AssessmentPurposeManifestHash
	case ConvergenceStateNeedGovernance:
		// Governance has a closed non-operation envelope and carries no semantic
		// proof projection of its own.
	default:
		return RuntimeCandidateContractManifest{}, fmt.Errorf("convergence state %q does not accept a candidate manifest", decision.State)
	}
	manifest = manifest.Normalized()
	if err := ValidateRuntimeCandidateManifestOperationCardinality(manifest); err != nil {
		return RuntimeCandidateContractManifest{}, err
	}
	if err := validateRuntimeCandidateManifestOperationWireFields(manifest); err != nil {
		return RuntimeCandidateContractManifest{}, err
	}
	if err := validateRuntimeConfirmedRepairManifestBindingLineage(manifest); err != nil {
		return RuntimeCandidateContractManifest{}, err
	}
	if err := ValidateRuntimeCandidateOperationCorrectionMetadata(manifest); err != nil {
		return RuntimeCandidateContractManifest{}, err
	}
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
	if err := ValidateRuntimeCandidateOperationCorrectionMetadata(manifest); err != nil {
		return fmt.Errorf("runtime candidate contract manifest correction metadata: %w", err)
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

// ValidateRuntimeCandidateContractManifestForSealedDecision validates an
// already-sealed manifest at a downstream owner boundary. It deliberately
// does not recompile target/catalog or assessment-purpose authority from a
// partial CandidateContext: only the Orchestrator aggregate can replay those
// persisted SM-2 preimages. Downstream renderers/evaluators consume the exact
// manifest body after this intrinsic decision fence succeeds.
func ValidateRuntimeCandidateContractManifestForSealedDecision(manifest RuntimeCandidateContractManifest, decision RuntimeDecision) error {
	if err := ValidateRuntimeCandidateContractManifestRawCanonical(manifest); err != nil {
		return err
	}
	if err := decision.Validate(); err != nil {
		return err
	}
	required := decision.RequiredOutput.Normalized()
	if manifest.State != decision.State || manifest.OperationContractRevision != RuntimeOperationContractRevision() ||
		!reflect.DeepEqual(manifest.EffectPolicy, mustRuntimeConvergenceEffectPolicy(decision.State)) ||
		manifest.OperationSetPolicy != runtimeCandidateOperationSetPolicyForState(decision.State) ||
		!sameStrings(manifest.TargetRefs, required.TargetRefs) || manifest.RepairClosureID != required.RepairClosureID ||
		manifest.RepairPurpose != required.RepairPurpose ||
		!reflect.DeepEqual(manifest.RequiredOperationSequence, cloneRuntimeRequiredOperationVariants(required.RequiredOperationSequence)) {
		return fmt.Errorf("runtime candidate contract manifest does not match sealed authority decision projection")
	}
	requiredFields, allowedFields, forbiddenFields := runtimeCandidateEnvelopeFields(decision.State)
	if !sameStrings(manifest.RequiredTopLevelFields, normalizeStrings(requiredFields)) || !sameStrings(manifest.AllowedTopLevelFields, normalizeStrings(allowedFields)) ||
		!sameStrings(manifest.ForbiddenTopLevelFields, normalizeStrings(forbiddenFields)) {
		return fmt.Errorf("runtime candidate contract manifest envelope does not match sealed decision")
	}
	switch decision.State {
	case ConvergenceStateNeedStateChange, ConvergenceStateNeedValidation:
		if manifest.MaxOperationCount != RuntimeCandidateOperationMaxCount {
			return fmt.Errorf("runtime operation candidate manifest has invalid cardinality ceiling")
		}
	case ConvergenceStateNeedAssessment:
		if manifest.MaxOperationCount != 0 ||
			!RuntimeCoverageEvidenceBindingProjectionsEqual(manifest.Bindings, decision.Focus.CoverageEvidenceBindings) {
			return fmt.Errorf("runtime assessment candidate manifest binding projection mismatch")
		}
	case ConvergenceStateNeedGovernance:
		if manifest.MaxOperationCount != 0 {
			return fmt.Errorf("runtime governance candidate manifest cannot carry operation cardinality")
		}
	default:
		return fmt.Errorf("convergence state %q does not accept a candidate manifest", decision.State)
	}
	if err := ValidateRuntimeCandidateManifestOperationCardinality(manifest); err != nil {
		return err
	}
	if err := validateRuntimeCandidateManifestOperationWireFields(manifest); err != nil {
		return err
	}
	return ValidateRuntimeCandidateOperationCorrectionMetadata(manifest)
}

// RuntimeCandidateAllowedOperationKinds is the owner-neutral C3 narrowing
// shared by every candidate-contract consumer. Creation and existing-scope
// authority come exclusively from the matching owner catalog's typed target
// policy; descriptors contribute only technical ownership and state support.
func RuntimeCandidateAllowedOperationKinds(decision RuntimeDecision, selection *RuntimeTargetSelection, admissibilities []RuntimeOperationAdmissibility) []string {
	required := decision.RequiredOutput.Normalized()
	kinds := append([]string(nil), required.AcceptedKinds...)
	if decision.State != ConvergenceStateNeedStateChange || selection == nil {
		return normalizeStrings(kinds)
	}
	value := selection.Normalized()
	if value.Disposition != RuntimeTargetSelectionCreate && value.Disposition != RuntimeTargetSelectionScope {
		return normalizeStrings(kinds)
	}
	operationPolicies := RuntimeOperationPoliciesFromAdmissibilities(admissibilities)
	seen := make(map[string]struct{}, len(kinds)+len(required.RequiredOperationSequence))
	ret := make([]string, 0, len(kinds)+len(required.RequiredOperationSequence))
	for _, raw := range kinds {
		descriptor, ok := RuntimeOperationDescriptorForKind(raw)
		if !ok || !OperationEffectCompatibleWithPolicies(descriptor.Kind, ConvergenceStateNeedStateChange, operationPolicies) {
			continue
		}
		if value.Disposition == RuntimeTargetSelectionCreate && descriptor.OwnerModuleID != value.OwnerModuleID {
			continue
		}
		catalog, found := runtimeCandidateAdmissibilityForKind(admissibilities, descriptor.OwnerModuleID, descriptor.Kind)
		if !found {
			continue
		}
		contract, _ := RuntimeOperationContractForKind(descriptor.Kind)
		variantIDs := runtimeCandidateRequiredVariantIDs(descriptor.Kind, contract, required)
		policy := RuntimeOperationTargetPolicyProspectiveOwnerTarget
		if value.Disposition == RuntimeTargetSelectionScope && value.ProspectiveTargetRef == "" {
			policy = RuntimeOperationTargetPolicyExistingOwnerScope
		}
		allowed, _, _, _, err := RuntimeOperationAdmissibilityAllowsTargetPolicyForVariants(catalog, descriptor.Kind, variantIDs, policy)
		if err != nil || !allowed {
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
	m.RepairClosureID = strings.TrimSpace(m.RepairClosureID)
	m.RepairPurpose = RuntimeDispatchRepairPurpose(strings.TrimSpace(string(m.RepairPurpose)))
	m.AssessmentPurpose = RuntimeAssessmentPurpose(strings.TrimSpace(string(m.AssessmentPurpose)))
	m.AssessmentPurposeManifestHash = strings.TrimSpace(m.AssessmentPurposeManifestHash)
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
	m.RequiredOperationFields = normalizeStrings(m.RequiredOperationFields)
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
	t.ProspectiveTargetRefs = normalizeStrings(t.ProspectiveTargetRefs)
	switch t.Policy {
	case RuntimeOperationTargetPolicyExactTarget:
		t.OwnerScopeTargetRefs = nil
		t.OwnerSelectorRefs = nil
		t.ProspectiveConstraintRefs = nil
		t.ProspectiveTargetRefs = nil
	case RuntimeOperationTargetPolicyExistingOwnerScope:
		t.ExactTargetRefs = nil
		t.ProspectiveConstraintRefs = nil
		t.ProspectiveTargetRefs = nil
	case RuntimeOperationTargetPolicyProspectiveOwnerTarget:
		t.ExactTargetRefs = nil
		t.OwnerScopeTargetRefs = nil
	default:
		t.Policy = ""
		t.ExactTargetRefs = nil
		t.OwnerScopeTargetRefs = nil
		t.OwnerSelectorRefs = nil
		t.ProspectiveConstraintRefs = nil
		t.ProspectiveTargetRefs = nil
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
// create only returns the complete SM-2 binding-sealed prospective target set.
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
		return append([]string(nil), t.ProspectiveTargetRefs...)
	default:
		return nil
	}
}

func (m RuntimeCandidateContractManifest) Clone() RuntimeCandidateContractManifest {
	return m.Normalized()
}

// ValidateRuntimeCandidateContractManifestRawCanonical rejects persistence
// shapes that would be changed by Normalized. A manifest is an immutable
// pre-seal authority body, so duplicate entries, ordering drift, surrounding
// whitespace and nil/empty substitutions must not be silently repaired while
// loading the aggregate. Stable hashes deliberately operate on normalized
// values and therefore cannot provide this raw-shape fence on their own.
func ValidateRuntimeCandidateContractManifestRawCanonical(manifest RuntimeCandidateContractManifest) error {
	canonical := manifest.Normalized()
	if !reflect.DeepEqual(manifest, canonical) {
		return fmt.Errorf("runtime candidate contract manifest is not raw-canonical")
	}
	if manifest.RepairPurpose != "" &&
		(manifest.RepairPurpose != RuntimeDispatchRepairPurposeConfirmedValidation || manifest.RepairClosureID == "") {
		return fmt.Errorf("runtime candidate contract manifest has invalid repair purpose %q", manifest.RepairPurpose)
	}
	if err := validateRuntimeConfirmedRepairManifestBindingLineage(manifest); err != nil {
		return err
	}
	if manifest.State == ConvergenceStateNeedAssessment {
		if (manifest.AssessmentPurpose != RuntimeAssessmentPurposeDefinitionProof && manifest.AssessmentPurpose != RuntimeAssessmentPurposeOwnerFactAcquisition) ||
			manifest.AssessmentPurposeManifestHash == "" || len(manifest.Bindings) == 0 {
			return fmt.Errorf("runtime candidate contract manifest has invalid assessment purpose preimage")
		}
	} else if manifest.AssessmentPurpose != "" || manifest.AssessmentPurposeManifestHash != "" {
		return fmt.Errorf("non-assessment runtime candidate contract manifest carries assessment purpose")
	}
	if manifest.Hash == "" || manifest.Hash != StableRuntimeCandidateContractManifestHash(manifest) {
		return fmt.Errorf("runtime candidate contract manifest hash mismatch")
	}
	return nil
}

func validateRuntimeConfirmedRepairManifestBindingLineage(manifest RuntimeCandidateContractManifest) error {
	manifest = manifest.Normalized()
	if manifest.RepairPurpose == "" {
		return nil
	}
	if manifest.RepairPurpose != RuntimeDispatchRepairPurposeConfirmedValidation ||
		manifest.State != ConvergenceStateNeedStateChange || len(manifest.Bindings) != 1 {
		return fmt.Errorf("confirmed validation repair manifest requires one source binding in need_state_change")
	}
	binding := manifest.Bindings[0]
	if _, found := CoverageEvidenceProofForKind(binding, EvidenceKindValidation); !found {
		return fmt.Errorf("confirmed validation repair manifest source binding %q has no validation proof", binding.ID)
	}
	if len(manifest.Operations) == 0 {
		return fmt.Errorf("confirmed validation repair manifest has no operation authorization")
	}
	for _, operation := range manifest.Operations {
		if !containsString(operation.AllowedOperationFields, "bindingRefs") ||
			!containsString(operation.RequiredOperationFields, "bindingRefs") || len(operation.BindingAuthorizations) == 0 {
			return fmt.Errorf("confirmed validation repair operation %q does not require its source binding", operation.Kind)
		}
		for _, authorization := range operation.BindingAuthorizations {
			if !sameStrings(authorization.AllowedBindingRefs, []string{binding.ID}) {
				return fmt.Errorf("confirmed validation repair operation %q source binding authorization drifted", operation.Kind)
			}
		}
	}
	return nil
}

// ValidateRuntimeCandidateManifestOperationCardinality verifies the sealed
// operation-document budget without deriving candidate content. Consumers use
// MaxOperationCount after this check; the global compiler ceiling is only a
// bound on manifests and is not a parallel per-consumer cardinality source.
func ValidateRuntimeCandidateManifestOperationCardinality(manifest RuntimeCandidateContractManifest) error {
	manifest = manifest.Normalized()
	if err := manifest.OperationSetPolicy.Validate(manifest.State); err != nil {
		return err
	}
	switch manifest.State {
	case ConvergenceStateNeedStateChange, ConvergenceStateNeedValidation:
		if manifest.MaxOperationCount < 1 || manifest.MaxOperationCount > RuntimeCandidateOperationMaxCount {
			return fmt.Errorf("candidate manifest maxOperationCount must be in 1..%d", RuntimeCandidateOperationMaxCount)
		}
		if len(manifest.RequiredOperationSequence) > manifest.MaxOperationCount {
			return fmt.Errorf("candidate manifest required operation sequence exceeds maxOperationCount %d", manifest.MaxOperationCount)
		}
	default:
		if manifest.MaxOperationCount != 0 {
			return fmt.Errorf("candidate manifest state %q does not accept maxOperationCount", manifest.State)
		}
	}
	return nil
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
	writeHashString(h, fmt.Sprintf("%d", manifest.MaxOperationCount))
	writeHashString(h, fmt.Sprintf("%d", manifest.OperationSetPolicy.MaxOperationsPerOwnerTarget))
	writeHashString(h, fmt.Sprintf("%t", manifest.OperationSetPolicy.AllowRequiredOperationSequenceOverlap))
	writeHashStrings(h, manifest.RequiredTopLevelFields)
	writeHashStrings(h, manifest.AllowedTopLevelFields)
	writeHashStrings(h, manifest.ForbiddenTopLevelFields)
	for _, binding := range manifest.Bindings {
		writeHashString(h, binding.ID)
		writeHashString(h, binding.Hash)
	}
	writeHashStrings(h, manifest.TargetRefs)
	writeHashString(h, manifest.RepairClosureID)
	writeHashString(h, string(manifest.RepairPurpose))
	writeHashString(h, string(manifest.AssessmentPurpose))
	writeHashString(h, manifest.AssessmentPurposeManifestHash)
	for _, requirement := range manifest.RequiredOperationSequence {
		writeHashString(h, requirement.RequirementID)
		writeHashString(h, requirement.Kind)
		writeHashString(h, requirement.VariantID)
		writeHashString(h, requirement.TargetRef)
		writeHashStrings(h, requirement.DependsOnRequirementIDs)
		writeHashString(h, requirement.Reason)
	}
	for _, operation := range manifest.Operations {
		writeHashString(h, operation.Kind)
		writeHashString(h, operation.OwnerModuleID)
		writeHashStrings(h, operation.AllowedOperationFields)
		writeHashStrings(h, operation.RequiredOperationFields)
		writeHashStrings(h, operation.AllowedVariantIDs)
		for _, parameter := range operation.Parameters {
			writeHashString(h, parameter.Name)
			writeHashString(h, string(parameter.Type))
			writeHashString(h, fmt.Sprintf("%t", parameter.Required))
			writeHashString(h, string(parameter.CorrectionRole))
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
			for _, constraint := range NormalizeRuntimeOperationCandidateConstraints(variant.CandidateConstraints) {
				writeHashString(h, constraint.Ref)
				writeHashString(h, string(constraint.Kind))
				writeHashString(h, constraint.ParameterName)
				writeHashString(h, string(constraint.RequiredShape))
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
		writeHashStrings(h, operation.Target.ProspectiveTargetRefs)
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
	fields := []string{"kind", "targetRefs", "parameters"}
	evidenceKind := ""
	switch state {
	case ConvergenceStateNeedStateChange:
		evidenceKind = EvidenceKindStateChange
	case ConvergenceStateNeedValidation:
		evidenceKind = EvidenceKindValidation
	}
	bindingAuthorizations, err := runtimeCandidateOperationBindingAuthorizations(required, contract, bindings, evidenceKind)
	if err != nil {
		return RuntimeCandidateOperationManifest{}, err
	}
	if runtimeOperationBindingAuthorizationsHaveBinding(bindingAuthorizations) {
		fields = append(fields, "bindingRefs")
	}
	if len(references) != 0 {
		fields = append(fields, "requiredOperationRef")
	}
	allowedBindingRefs := runtimeOperationBindingAuthorizationRefs(bindingAuthorizations)
	target, prerequisiteRefs, admissibilityRefs, err := runtimeCandidateTargetAuthorizationForKind(
		descriptor, contract, required, ownerFacts, bindings, evidenceKind, allowedBindingRefs,
	)
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
	operationManifest := RuntimeCandidateOperationManifest{
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
		ValidationRecipes:           runtimeCandidateManifestValidationRecipes(contract.Kind, ownerFacts.ValidationRecipes, bindings, allowedBindingRefs),
		OperationContractRevision:   contractRevision,
	}.Normalized()
	operationManifest.RequiredOperationFields = runtimeCandidateRequiredOperationFields(operationManifest)
	if _, err := RuntimeOperationCorrectionFields(operationManifest); err != nil {
		return RuntimeCandidateOperationManifest{}, fmt.Errorf("candidate manifest operation kind %q correction metadata: %w", kind, err)
	}
	return operationManifest, nil
}

func runtimeCandidateOperationBindingAuthorizations(required RequiredOutputContract, contract RuntimeOperationContract, bindings []CoverageEvidenceBinding, evidenceKind string) ([]RuntimeOperationBindingAuthorization, error) {
	repairSourceBindingRefs, err := runtimeConfirmedValidationRepairSourceBindingRefs(required, bindings)
	if err != nil {
		return nil, err
	}
	bindingRefsForVariant := func(variantID string) []string {
		if repairSourceBindingRefs != nil {
			return append([]string(nil), repairSourceBindingRefs...)
		}
		return coverageEvidenceBindingRefsForOperationVariant(bindings, contract.Kind, variantID, evidenceKind)
	}
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
				AllowedBindingRefs:   bindingRefsForVariant(requirement.VariantID),
			})
		}
		return ret, nil
	}
	for _, variant := range contract.Variants {
		ret = append(ret, RuntimeOperationBindingAuthorization{
			VariantID:          variant.ID,
			AllowedBindingRefs: bindingRefsForVariant(variant.ID),
		})
	}
	if len(ret) == 0 {
		allowedBindingRefs := CoverageEvidenceBindingRefsForOperationEvidenceKind(bindings, contract.Kind, evidenceKind)
		if repairSourceBindingRefs != nil {
			allowedBindingRefs = append([]string(nil), repairSourceBindingRefs...)
		}
		ret = append(ret, RuntimeOperationBindingAuthorization{AllowedBindingRefs: allowedBindingRefs})
	}
	return ret, nil
}

// runtimeConfirmedValidationRepairSourceBindingRefs returns the exact current
// validation binding whose rejected owner receipt caused a confirmed repair.
// This is causation lineage, not state-change proof credit: receipt
// adjudication still requires the repair commitment and fresh validation.
//
// A confirmed repair is scoped to one CWI and therefore to one binding. Fail
// closed here, before rendering a provider request, instead of emitting an
// auxiliary operation that the immutable repair commitment must later reject.
func runtimeConfirmedValidationRepairSourceBindingRefs(required RequiredOutputContract, bindings []CoverageEvidenceBinding) ([]string, error) {
	required = required.Normalized()
	if required.RepairPurpose == "" {
		return nil, nil
	}
	if required.RepairPurpose != RuntimeDispatchRepairPurposeConfirmedValidation {
		return nil, fmt.Errorf("candidate manifest has unsupported repair purpose %q", required.RepairPurpose)
	}
	if len(required.CoverageWorkItemIDs) != 1 {
		return nil, fmt.Errorf("confirmed validation repair requires exactly one source coverage work item")
	}
	wanted := required.CoverageWorkItemIDs[0]
	var matched *CoverageEvidenceBinding
	for _, raw := range NormalizeCoverageEvidenceBindings(bindings) {
		binding := raw.Clone()
		if binding.CoverageWorkItemID != wanted {
			continue
		}
		if matched != nil {
			return nil, fmt.Errorf("confirmed validation repair source coverage %q has multiple bindings", wanted)
		}
		value := binding
		matched = &value
	}
	if matched == nil {
		return nil, fmt.Errorf("confirmed validation repair source coverage %q has no current binding", wanted)
	}
	if _, found := CoverageEvidenceProofForKind(*matched, EvidenceKindValidation); !found {
		return nil, fmt.Errorf("confirmed validation repair source binding %q has no validation proof", matched.ID)
	}
	return []string{matched.ID}, nil
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

func runtimeOperationBindingAuthorizationsHaveBinding(values []RuntimeOperationBindingAuthorization) bool {
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
//  1. proof- and kind-matching sealed prospective binding target set;
//  2. decision-level exact TargetRefs when no create/scope owner fact applies;
//  3. TargetSelection Create / Scope / Selected dispositions;
//  4. matching OperationAdmissibility inventory for the kind's owner;
//  5. exact_target with empty membership for kinds without target obligation.
func runtimeCandidateProspectiveBindingTargets(bindings []CoverageEvidenceBinding, allowedBindingRefs []string, kind, evidenceKind string) ([]string, string, bool, error) {
	allowedBindingRefs = normalizeStrings(allowedBindingRefs)
	if len(allowedBindingRefs) == 0 {
		return nil, "", false, nil
	}
	allowed := make(map[string]struct{}, len(allowedBindingRefs))
	for _, ref := range allowedBindingRefs {
		allowed[ref] = struct{}{}
	}
	targetRefs := make([]string, 0)
	selectorRef := ""
	for _, binding := range NormalizeCoverageEvidenceBindings(bindings) {
		if _, found := allowed[binding.ID]; !found || len(binding.ProspectiveTargetRefs) == 0 {
			continue
		}
		proof, found := CoverageEvidenceProofForKind(binding, evidenceKind)
		if !found || !runtimeOperationKindsContain(proof.AllowedOperationKinds, kind) {
			continue
		}
		if binding.TargetSelectorRef == "" {
			return nil, "", false, fmt.Errorf("candidate manifest prospective binding %q has no sealed owner selector", binding.ID)
		}
		if selectorRef != "" && selectorRef != binding.TargetSelectorRef {
			return nil, "", false, fmt.Errorf("candidate manifest prospective bindings for %q have conflicting owner selectors", kind)
		}
		selectorRef = binding.TargetSelectorRef
		targetRefs = append(targetRefs, binding.ProspectiveTargetRefs...)
	}
	targetRefs = normalizeStrings(targetRefs)
	if len(targetRefs) == 0 {
		return nil, "", false, nil
	}
	if len(targetRefs) > RuntimeProspectiveTargetRefsMaxCount {
		return nil, "", false, fmt.Errorf("candidate manifest prospective binding target set for %q exceeds %d", kind, RuntimeProspectiveTargetRefsMaxCount)
	}
	return targetRefs, selectorRef, true, nil
}

func runtimeCandidateTargetAuthorizationForKind(
	descriptor RuntimeOperationDescriptor,
	contract RuntimeOperationContract,
	required RequiredOutputContract,
	ownerFacts RuntimeCandidateManifestOwnerFacts,
	bindings []CoverageEvidenceBinding,
	evidenceKind string,
	allowedBindingRefs []string,
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
	prospectiveBindingRefs, prospectiveBindingSelector, hasProspectiveBinding, bindingErr := runtimeCandidateProspectiveBindingTargets(
		bindings, allowedBindingRefs, descriptor.Kind, evidenceKind,
	)
	if bindingErr != nil {
		return RuntimeOperationTargetAuthorization{}, nil, nil, bindingErr
	}
	if hasProspectiveBinding {
		allowsProspective, ownerSelectors, ownerConstraints := runtimeCandidateAdmissibilityAllowsProspective(required, contract, admissibility, hasAdmissibility)
		if !allowsProspective || !containsString(ownerSelectors, prospectiveBindingSelector) || len(ownerConstraints) == 0 {
			return RuntimeOperationTargetAuthorization{}, nil, nil, fmt.Errorf("candidate manifest prospective binding for %q is outside matching owner admissibility", descriptor.Kind)
		}
		for _, targetRef := range prospectiveBindingRefs {
			if containsString(admissibility.TargetRefs, targetRef) {
				return RuntimeOperationTargetAuthorization{}, nil, nil, fmt.Errorf("candidate manifest prospective binding target %q already belongs to owner inventory", targetRef)
			}
		}
		return RuntimeOperationTargetAuthorization{
			Policy:                    RuntimeOperationTargetPolicyProspectiveOwnerTarget,
			OwnerSelectorRefs:         []string{prospectiveBindingSelector},
			ProspectiveConstraintRefs: append([]string(nil), ownerConstraints...),
			ProspectiveTargetRefs:     prospectiveBindingRefs,
		}, nil, admissibilityRefs, nil
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
		// Create kind-narrowing admits only an owner fact carrying the exact
		// SM-2 binding-derived prospective target. RequiredOutput.TargetRefs is a
		// downstream projection and must never reconstruct a missing owner fact.
		allowsProspective, ownerSelectors, ownerConstraints := runtimeCandidateAdmissibilityAllowsProspective(required, contract, admissibility, hasAdmissibility)
		prospectiveRef := value.ProspectiveTargetRef
		if prospectiveRef == "" {
			return RuntimeOperationTargetAuthorization{}, nil, nil, fmt.Errorf("candidate manifest create selection for %q requires a binding-derived prospective target", descriptor.Kind)
		}
		if !allowsProspective {
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
			ProspectiveTargetRefs:     []string{prospectiveRef},
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
		// an SM-2 binding-sealed prospective deliverable. When the selected owner
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
					ProspectiveTargetRefs:     []string{value.ProspectiveTargetRef},
				}, prerequisiteRefs, admissibilityRefs, nil
			}
		}
		if value.CandidateCount != 0 && value.CandidateCount != len(scopeRefs) {
			return RuntimeOperationTargetAuthorization{}, nil, nil, fmt.Errorf("candidate manifest existing owner scope requires a complete bounded target set")
		}
		if descriptor.OwnerModuleID != value.OwnerModuleID && !contract.RequireTargetRefs &&
			!runtimeCandidateAdmissibilityAllowsExisting(required, contract, admissibility, hasAdmissibility) {
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
		if !contract.RequireTargetRefs && !runtimeCandidateAdmissibilityAllowsExisting(required, contract, admissibility, hasAdmissibility) {
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

func runtimeCandidateAdmissibilityAllowsExisting(required RequiredOutputContract, contract RuntimeOperationContract, admissibility RuntimeOperationAdmissibility, hasAdmissibility bool) bool {
	if !hasAdmissibility {
		return false
	}
	variantIDs := runtimeCandidateRequiredVariantIDs(contract.Kind, contract, required.Normalized())
	allowed, _, _, _, err := RuntimeOperationAdmissibilityAllowsTargetPolicyForVariants(
		admissibility,
		contract.Kind,
		variantIDs,
		RuntimeOperationTargetPolicyExistingOwnerScope,
	)
	return err == nil && allowed
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

func runtimeCandidateRequiredOperationFields(operation RuntimeCandidateOperationManifest) []string {
	required := []string{"kind"}
	if len(operation.RequiredOperationReferences) != 0 {
		required = append(required, "requiredOperationRef")
	}
	if len(operation.BindingAuthorizations) != 0 {
		allRequireBinding := true
		for _, authorization := range operation.BindingAuthorizations {
			if len(authorization.AllowedBindingRefs) == 0 {
				allRequireBinding = false
				break
			}
		}
		if allRequireBinding {
			required = append(required, "bindingRefs")
		}
	}
	if operation.RequireTargetRefs {
		required = append(required, "targetRefs")
	}
	if len(operation.RequireOneOfParameters) != 0 || len(operation.Variants) != 0 {
		required = append(required, "parameters")
	} else {
		for _, parameter := range operation.Parameters {
			if parameter.Required {
				required = append(required, "parameters")
				break
			}
		}
	}
	return normalizeStrings(required)
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
	value.CandidateConstraints = append([]RuntimeOperationCandidateConstraint(nil), NormalizeRuntimeOperationCandidateConstraints(value.CandidateConstraints)...)
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
		value.TargetRef = strings.TrimSpace(value.TargetRef)
		value.DependsOnRequirementIDs = normalizeStrings(value.DependsOnRequirementIDs)
		value.Reason = strings.TrimSpace(value.Reason)
		ret = append(ret, value)
	}
	return ret
}

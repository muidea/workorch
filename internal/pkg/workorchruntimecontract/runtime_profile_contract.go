package workorchruntimecontract

import (
	"fmt"
	"sort"
	"strings"
)

type RuntimeProfileContract struct {
	ID                      string
	AcceptanceDimensions    []RuntimeAcceptanceDimension
	DeliverableExpectations []RuntimeDeliverableExpectation
	EvidenceRequirements    []RuntimeProfileEvidenceRequirement
	CoverageDependencies    []RuntimeCoverageDependency
	// BootstrapRecipes are profile/template-owned, bounded declarations for
	// initializing an otherwise empty workspace. They seal artifact roles,
	// operation kinds and dependency order, but never an exact prospective
	// target. A semantic proposal chooses the concrete target after activation,
	// and File independently validates it before SM-2 can seal a binding.
	BootstrapRecipes []RuntimeBootstrapRecipe
}

// RuntimeBootstrapRecipe is declarative input for an activation adapter. The
// matcher terms are profile data rather than orchestrator policy; a profile
// that cannot select exactly one bounded recipe must leave artifact-role
// grounding unresolved. The recipe never supplies the concrete target path.
type RuntimeBootstrapRecipe struct {
	ID                string
	RequiredGoalTerms []string
	RequiredDoDTerms  []string
	Artifacts         []RuntimeBootstrapArtifact
}

// RuntimeBootstrapArtifact is one opaque, profile-owned artifact declaration.
// DependsOnArtifactIDs establishes an ordered, finite bootstrap graph; the
// activation adapter compiles that graph before Runtime is started. It has no
// exact target field: the post-seal semantic proposal supplies that value.
type RuntimeBootstrapArtifact struct {
	ID                    string
	TargetRole            string
	AllowedOperationKinds []string
	DependsOnArtifactIDs  []string
}

type RuntimeAcceptanceDimension struct {
	ID          string
	Required    bool
	Description string
}

type RuntimeDeliverableExpectation struct {
	ID           string
	Required     bool
	EvidenceRole string
	Description  string
}

type RuntimeProfileEvidenceRequirement struct {
	ID           string
	Scope        string
	EvidenceRole string
	// TargetRole identifies the declared target role of direct evidence. It is
	// a profile-owned semantic label; Runtime only transports and compares it.
	TargetRole             string
	TargetRefs             []string
	TargetDiscoverySources []string
	TargetSelectorRef      string
	// TargetSelectorOwnerRef and TargetSelectorSubjectRef bind a generic File
	// selection query to the profile declaration which owns its semantics. File
	// only evaluates the bounded candidate refs; it never interprets these values.
	TargetSelectorOwnerRef            string
	TargetSelectorSubjectRef          string
	TargetSelectorCandidateRefs       []string
	RequireTargetGrounding            bool
	RequiredEvidenceKinds             []string
	RequireDirectEvidence             bool
	RequireClaimEvidence              bool
	RequireExecutableBehaviorEvidence bool
	RequireCoverageTargets            bool
	AcceptedStepKinds                 []string
	AcceptedOperationVariants         []RuntimeOperationVariantRef
	// ValidationRecipeID and RequiredCheckRefs are profile-owned proof
	// declarations. Runtime only seals and compares these opaque identities;
	// Shell remains the owner that observes executed/passed checks.
	ValidationRecipeID    string
	RequiredCheckRefs     []string
	MetricConstraints     []string
	ArtifactRefs          []string
	RequiredOwnerFacts    []RuntimeOwnerFactRequirement
	OwnerFactConvergences []RuntimeOwnerFactConvergence
	// ValidationFailureRepairConvergence declares the one ordered all-of
	// recovery path for a typed rejected validation. It is separate from the
	// terminal proof requirement and therefore cannot add state_change to the
	// DoD terminal evidence kinds.
	ValidationFailureRepairConvergence *RuntimeValidationFailureRepairConvergence
	CapabilityStateRequirements        []RuntimeProfileCapabilityStateRequirement
	RejectedStepKinds                  []string
	Description                        string
}

type RuntimeProfileCapabilityStateRequirement struct {
	OwnerModuleID       string
	CapabilityStateKind string
	SubjectRef          string
}

// ResolveRuntimeProfileTargetSelectorIdentity returns the effective
// Definition-owned selector identity for one Profile evidence requirement.
// Profile ID and evidence scope are the deterministic defaults used by the
// activation compiler; keeping that rule here lets pre-seal admission validate
// the exact identity before any Runtime aggregate can be created.
func ResolveRuntimeProfileTargetSelectorIdentity(profileID string, requirement RuntimeProfileEvidenceRequirement) (selectorRef, ownerRef, subjectRef string) {
	selectorRef = strings.TrimSpace(requirement.TargetSelectorRef)
	ownerRef = strings.TrimSpace(requirement.TargetSelectorOwnerRef)
	subjectRef = strings.TrimSpace(requirement.TargetSelectorSubjectRef)
	if selectorRef == "" {
		return selectorRef, ownerRef, subjectRef
	}
	if ownerRef == "" {
		ownerRef = strings.TrimSpace(profileID)
	}
	if subjectRef == "" {
		subjectRef = strings.TrimSpace(requirement.Scope)
	}
	return selectorRef, ownerRef, subjectRef
}

// RuntimeCoverageDependency is a profile-declared relationship between two
// compiled coverage sources. Runtime transports opaque source IDs and only
// applies the sealed evidence-kind trigger; it never infers a dependency from
// a file path, language, command name, or assessment prose.
type RuntimeCoverageDependency struct {
	ID                     string
	SourceScope            string
	SourceRequirementID    string
	DependentScope         string
	DependentRequirementID string
	// RequireSourceSettled gates semantic-plan admission for the dependent
	// coverage row until every compiled source row has current settled credit.
	// It is independent from post-effect invalidation below.
	RequireSourceSettled bool
	// TriggerEvidenceKinds reopens an already credited dependent after matching
	// source evidence is accepted. An empty list means this declaration has no
	// post-effect invalidation behavior.
	TriggerEvidenceKinds []string
}

// RuntimeProfileProofClosureViolation identifies one profile-owned CWI source
// that cannot be closed by the profile's exact, typed evidence declaration.
// It is a pure contract finding: Workspace admission decides whether the
// finding blocks promotion, while Runtime keeps a defensive validation gate.
type RuntimeProfileProofClosureViolation struct {
	ProfileID   string
	SourceScope string
	SourceID    string
	Rule        string
}

func (v RuntimeProfileProofClosureViolation) SourceRef() string {
	return strings.Join([]string{"profile", strings.TrimSpace(v.ProfileID), strings.TrimSpace(v.SourceScope), strings.TrimSpace(v.SourceID)}, "/")
}

// RuntimeProfileProofClosureViolations returns deterministic static findings
// for every profile-required acceptance/deliverable CWI. A required source
// must have one exact evidence declaration with at least one supported proof
// operation for every required evidence kind. Wildcards remain quality policy
// for Definition-derived CWIs and cannot stand in for an exact profile-owned
// terminal proof declaration.
func RuntimeProfileProofClosureViolations(values []RuntimeProfileContract) []RuntimeProfileProofClosureViolation {
	profiles := NormalizeRuntimeProfileContracts(values)
	violations := make([]RuntimeProfileProofClosureViolation, 0)
	requiredSourceRefs := make(map[string][]string)
	for _, profile := range profiles {
		exactRequirements := make(map[string]RuntimeProfileEvidenceRequirement, len(profile.EvidenceRequirements))
		for _, requirement := range profile.EvidenceRequirements {
			exactRequirements[runtimeProfileSourceKey(requirement.Scope, requirement.ID)] = requirement
		}
		sources := make([]RuntimeProfileProofClosureViolation, 0, len(profile.AcceptanceDimensions)+len(profile.DeliverableExpectations))
		for _, dimension := range profile.AcceptanceDimensions {
			if dimension.Required {
				sources = append(sources, RuntimeProfileProofClosureViolation{ProfileID: profile.ID, SourceScope: "acceptanceDimension", SourceID: dimension.ID})
			}
		}
		for _, deliverable := range profile.DeliverableExpectations {
			if deliverable.Required {
				sources = append(sources, RuntimeProfileProofClosureViolation{ProfileID: profile.ID, SourceScope: "deliverable", SourceID: deliverable.ID})
			}
		}
		for _, source := range sources {
			key := runtimeProfileSourceKey(source.SourceScope, source.SourceID)
			requiredSourceRefs[key] = append(requiredSourceRefs[key], source.SourceRef())
			requirement, found := exactRequirements[key]
			if !found {
				source.Rule = "missing_exact_evidence_requirement"
				violations = append(violations, source)
				continue
			}
			if len(requirement.RequiredEvidenceKinds) == 0 {
				source.Rule = "missing_required_evidence_kind"
				violations = append(violations, source)
				continue
			}
			for _, evidenceKind := range requirement.RequiredEvidenceKinds {
				if evidenceKind != EvidenceKindStateChange && evidenceKind != EvidenceKindValidation {
					invalid := source
					invalid.Rule = "unsupported_required_evidence_kind"
					violations = append(violations, invalid)
					continue
				}
				if !runtimeProfileHasProofOperation(requirement.AcceptedStepKinds, evidenceKind) {
					invalid := source
					invalid.Rule = "missing_owner_receipt_proof_operation"
					violations = append(violations, invalid)
				}
			}
		}
	}
	for key, refs := range requiredSourceRefs {
		if len(refs) < 2 {
			continue
		}
		parts := strings.SplitN(key, "|", 2)
		for _, ref := range refs {
			profileID := ""
			segments := strings.Split(ref, "/")
			if len(segments) > 1 {
				profileID = segments[1]
			}
			violations = append(violations, RuntimeProfileProofClosureViolation{
				ProfileID: profileID, SourceScope: parts[0], SourceID: parts[1], Rule: "duplicate_required_profile_source",
			})
		}
	}
	sort.Slice(violations, func(i, j int) bool {
		left := violations[i].SourceRef() + "|" + violations[i].Rule
		right := violations[j].SourceRef() + "|" + violations[j].Rule
		return left < right
	})
	ret := violations[:0]
	last := ""
	for _, violation := range violations {
		key := violation.SourceRef() + "|" + violation.Rule
		if key == last {
			continue
		}
		last = key
		ret = append(ret, violation)
	}
	return nilIfEmpty(ret)
}

func runtimeProfileSourceKey(scope, id string) string {
	return strings.TrimSpace(scope) + "|" + strings.TrimSpace(id)
}

func runtimeProfileHasProofOperation(kinds []string, evidenceKind string) bool {
	for _, kind := range kinds {
		if RuntimeOperationKindSupportsCoverageEvidence(kind, evidenceKind) {
			return true
		}
	}
	return false
}

func NormalizeRuntimeProfileContracts(values []RuntimeProfileContract) []RuntimeProfileContract {
	byID := map[string]RuntimeProfileContract{}
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.AcceptanceDimensions = normalizeRuntimeAcceptanceDimensions(value.AcceptanceDimensions)
		value.DeliverableExpectations = normalizeRuntimeDeliverableExpectations(value.DeliverableExpectations)
		value.EvidenceRequirements = normalizeRuntimeProfileEvidenceRequirements(value.EvidenceRequirements)
		value.CoverageDependencies = normalizeRuntimeCoverageDependencies(value.CoverageDependencies)
		value.BootstrapRecipes = normalizeRuntimeBootstrapRecipes(value.BootstrapRecipes)
		if value.ID != "" {
			byID[value.ID] = value
		}
	}
	keys := make([]string, 0, len(byID))
	for key := range byID {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimeProfileContract, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byID[key])
	}
	return nilIfEmpty(ret)
}

func ValidateRuntimeProfileContracts(values []RuntimeProfileContract) error {
	for _, profile := range NormalizeRuntimeProfileContracts(values) {
		if profile.ID == "" {
			return fmt.Errorf("runtime profile contract requires id")
		}
		seenEvidenceRequirementRefs := make(map[string]struct{}, len(profile.EvidenceRequirements))
		for _, item := range profile.AcceptanceDimensions {
			if item.ID == "" {
				return fmt.Errorf("runtime acceptance dimension requires id")
			}
		}
		for _, item := range profile.DeliverableExpectations {
			if item.ID == "" {
				return fmt.Errorf("runtime deliverable expectation requires id")
			}
		}
		for _, item := range profile.EvidenceRequirements {
			if item.ID == "" || item.Scope == "" {
				return fmt.Errorf("runtime profile evidence requirement requires id and scope")
			}
			sourceRef := runtimeProfileSourceKey(item.Scope, item.ID)
			if _, duplicate := seenEvidenceRequirementRefs[sourceRef]; duplicate {
				return fmt.Errorf("runtime profile %q has duplicate evidence requirement scope+id %q", profile.ID, sourceRef)
			}
			seenEvidenceRequirementRefs[sourceRef] = struct{}{}
			if err := ValidateRuntimeTargetGrounding(item.RequireTargetGrounding, item.TargetRole, item.TargetRefs, item.TargetDiscoverySources, item.TargetSelectorRef); err != nil {
				return fmt.Errorf("runtime profile evidence requirement %q: %w", item.ID, err)
			}
			if len(item.TargetSelectorCandidateRefs) > 0 {
				if !item.RequireTargetGrounding || item.TargetSelectorRef == "" || item.TargetSelectorOwnerRef == "" || item.TargetSelectorSubjectRef == "" {
					return fmt.Errorf("runtime profile evidence requirement %q candidate selector requires grounding and a complete selector", item.ID)
				}
			}
			for _, state := range item.CapabilityStateRequirements {
				if !runtimeProfileCapabilityStateOwnerMatchesAcceptedStepKind(state.OwnerModuleID, item.AcceptedStepKinds) {
					return fmt.Errorf("runtime profile evidence requirement %q capability-state owner %q does not own an accepted operation kind", item.ID, state.OwnerModuleID)
				}
			}
			if err := ValidateRuntimeOwnerFactRequirements(item.RequiredOwnerFacts); err != nil {
				return fmt.Errorf("runtime profile evidence requirement %q: %w", item.ID, err)
			}
			if err := ValidateRuntimeOwnerFactConvergences(item.RequiredOwnerFacts, item.OwnerFactConvergences); err != nil {
				return fmt.Errorf("runtime profile evidence requirement %q: %w", item.ID, err)
			}
			if item.ValidationFailureRepairConvergence != nil {
				if item.Scope != "dodItem" {
					return fmt.Errorf("runtime profile evidence requirement %q validation failure repair convergence is only valid for dodItem coverage", item.ID)
				}
				if len(item.RequiredEvidenceKinds) != 1 || item.RequiredEvidenceKinds[0] != EvidenceKindValidation {
					return fmt.Errorf("runtime profile evidence requirement %q validation failure repair convergence requires validation-only terminal evidence", item.ID)
				}
				if err := item.ValidationFailureRepairConvergence.Validate(); err != nil {
					return fmt.Errorf("runtime profile evidence requirement %q: %w", item.ID, err)
				}
			}
			selectorRef, selectorOwnerRef, selectorSubjectRef := ResolveRuntimeProfileTargetSelectorIdentity(profile.ID, item)
			for _, ownerFact := range item.RequiredOwnerFacts {
				if !runtimeProfileContainsOperationKind(item.AcceptedStepKinds, ownerFact.OperationKind) {
					return fmt.Errorf("runtime profile evidence requirement %q owner fact operation %q is outside accepted operation kinds", item.ID, ownerFact.OperationKind)
				}
			}
			for _, convergence := range item.OwnerFactConvergences {
				convergence = convergence.Normalized()
				if !convergence.RequirePreexisting && (selectorRef != convergence.TargetSelectorRef ||
					selectorOwnerRef != convergence.TargetSelectorOwnerRef ||
					selectorSubjectRef != convergence.TargetSelectorSubjectRef) {
					return fmt.Errorf("runtime profile evidence requirement %q owner-fact acquisition selector does not match its declaration", item.ID)
				}
				if !convergence.RequirePreexisting && !runtimeProfileContainsOperationKind(item.AcceptedStepKinds, convergence.AcquisitionOperationKind) {
					return fmt.Errorf("runtime profile evidence requirement %q owner-fact acquisition %q is outside accepted operation kinds", item.ID, convergence.AcquisitionOperationKind)
				}
			}
			for _, variant := range item.AcceptedOperationVariants {
				if !runtimeProfileContainsOperationKind(item.AcceptedStepKinds, variant.Kind) {
					return fmt.Errorf("runtime profile evidence requirement %q operation variant %q is outside accepted operation kinds", item.ID, variant.VariantID)
				}
			}
			for _, evidenceKind := range item.RequiredEvidenceKinds {
				if evidenceKind != EvidenceKindStateChange && evidenceKind != EvidenceKindValidation {
					return fmt.Errorf("runtime profile evidence requirement %q has unsupported evidence kind %q", item.ID, evidenceKind)
				}
				if !runtimeProfileHasProofOperation(item.AcceptedStepKinds, evidenceKind) {
					return fmt.Errorf("runtime profile evidence requirement %q has no owner receipt operation for evidence kind %q", item.ID, evidenceKind)
				}
			}
		}
		for leftIndex, left := range profile.EvidenceRequirements {
			if left.ValidationFailureRepairConvergence == nil {
				continue
			}
			for _, right := range profile.EvidenceRequirements[leftIndex+1:] {
				if right.ValidationFailureRepairConvergence == nil || left.Scope != right.Scope ||
					(left.ID != "*" && right.ID != "*" && left.ID != right.ID) {
					continue
				}
				if !SameRuntimeValidationFailureRepairConvergence(left.ValidationFailureRepairConvergence, right.ValidationFailureRepairConvergence) {
					return fmt.Errorf("runtime profile evidence requirements %q and %q declare conflicting validation failure repair convergence for scope %q", left.ID, right.ID, left.Scope)
				}
			}
		}
		for _, dependency := range profile.CoverageDependencies {
			if dependency.ID == "" || dependency.SourceScope == "" || dependency.SourceRequirementID == "" ||
				dependency.DependentScope == "" || dependency.DependentRequirementID == "" {
				return fmt.Errorf("runtime coverage dependency requires identity, source and dependent bindings")
			}
			if dependency.SourceScope == dependency.DependentScope && dependency.SourceRequirementID == dependency.DependentRequirementID {
				return fmt.Errorf("runtime coverage dependency %q cannot target itself", dependency.ID)
			}
			if !dependency.RequireSourceSettled && len(dependency.TriggerEvidenceKinds) == 0 {
				return fmt.Errorf("runtime coverage dependency %q requires admission or invalidation semantics", dependency.ID)
			}
		}
		for _, recipe := range profile.BootstrapRecipes {
			if err := recipe.Validate(); err != nil {
				return fmt.Errorf("runtime bootstrap recipe %q: %w", recipe.ID, err)
			}
		}
	}
	if violations := RuntimeProfileProofClosureViolations(values); len(violations) > 0 {
		first := violations[0]
		return fmt.Errorf("runtime profile proof closure %q violates %s", first.SourceRef(), first.Rule)
	}
	return nil
}

func normalizeRuntimeBootstrapRecipes(values []RuntimeBootstrapRecipe) []RuntimeBootstrapRecipe {
	byID := map[string]RuntimeBootstrapRecipe{}
	for _, value := range values {
		value = value.Normalized()
		if value.ID != "" {
			byID[value.ID] = value
		}
	}
	return sortedMapValues(byID)
}

func (r RuntimeBootstrapRecipe) Normalized() RuntimeBootstrapRecipe {
	r.ID = strings.TrimSpace(r.ID)
	r.RequiredGoalTerms = normalizeStrings(r.RequiredGoalTerms)
	r.RequiredDoDTerms = normalizeStrings(r.RequiredDoDTerms)
	byID := map[string]RuntimeBootstrapArtifact{}
	for _, artifact := range r.Artifacts {
		artifact = artifact.Normalized()
		if artifact.ID != "" {
			byID[artifact.ID] = artifact
		}
	}
	r.Artifacts = sortedMapValues(byID)
	return r
}

func (a RuntimeBootstrapArtifact) Normalized() RuntimeBootstrapArtifact {
	a.ID = strings.TrimSpace(a.ID)
	a.TargetRole = strings.TrimSpace(a.TargetRole)
	a.AllowedOperationKinds = normalizeStrings(a.AllowedOperationKinds)
	a.DependsOnArtifactIDs = normalizeStrings(a.DependsOnArtifactIDs)
	return a
}

func (r RuntimeBootstrapRecipe) Validate() error {
	r = r.Normalized()
	if r.ID == "" || len(r.Artifacts) == 0 {
		return fmt.Errorf("runtime bootstrap recipe requires id and artifacts")
	}
	if len(r.RequiredGoalTerms) == 0 && len(r.RequiredDoDTerms) == 0 {
		return fmt.Errorf("runtime bootstrap recipe %q requires bounded goal or DoD matcher terms", r.ID)
	}
	known := make(map[string]RuntimeBootstrapArtifact, len(r.Artifacts))
	for _, artifact := range r.Artifacts {
		if artifact.ID == "" || artifact.TargetRole == "" || len(artifact.AllowedOperationKinds) == 0 {
			return fmt.Errorf("runtime bootstrap artifact requires id, role and allowed operation kinds")
		}
		if _, exists := known[artifact.ID]; exists {
			return fmt.Errorf("runtime bootstrap recipe %q duplicates artifact %q", r.ID, artifact.ID)
		}
		for _, kind := range artifact.AllowedOperationKinds {
			if _, ok := RuntimeOperationDescriptorForKind(CanonicalRuntimeBlockKind(kind)); !ok {
				return fmt.Errorf("runtime bootstrap artifact %q has unknown operation kind %q", artifact.ID, kind)
			}
		}
		known[artifact.ID] = artifact
	}
	for _, artifact := range r.Artifacts {
		for _, dependency := range artifact.DependsOnArtifactIDs {
			if dependency == artifact.ID {
				return fmt.Errorf("runtime bootstrap artifact %q cannot depend on itself", artifact.ID)
			}
			if _, exists := known[dependency]; !exists {
				return fmt.Errorf("runtime bootstrap artifact %q depends on unknown artifact %q", artifact.ID, dependency)
			}
		}
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("runtime bootstrap recipe %q has dependency cycle at %q", r.ID, id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, dependency := range known[id].DependsOnArtifactIDs {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}
	for _, artifact := range r.Artifacts {
		if err := visit(artifact.ID); err != nil {
			return err
		}
	}
	return nil
}

// OrderedArtifacts returns a deterministic dependency order. It is a pure
// value helper so activation can seal the recipe without giving pkg authority
// to transition any Runtime state.
func (r RuntimeBootstrapRecipe) OrderedArtifacts() ([]RuntimeBootstrapArtifact, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	r = r.Normalized()
	byID := make(map[string]RuntimeBootstrapArtifact, len(r.Artifacts))
	for _, artifact := range r.Artifacts {
		byID[artifact.ID] = artifact
	}
	ordered := make([]RuntimeBootstrapArtifact, 0, len(r.Artifacts))
	visited := map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if visited[id] {
			return
		}
		artifact := byID[id]
		for _, dependency := range artifact.DependsOnArtifactIDs {
			visit(dependency)
		}
		visited[id] = true
		ordered = append(ordered, artifact)
	}
	for _, artifact := range r.Artifacts {
		visit(artifact.ID)
	}
	return ordered, nil
}

func runtimeProfileContainsOperationKind(values []string, expected string) bool {
	expected = CanonicalRuntimeBlockKind(expected)
	for _, value := range values {
		if CanonicalRuntimeBlockKind(value) == expected {
			return true
		}
	}
	return false
}

// runtimeProfileCapabilityStateOwnerMatchesAcceptedStepKind keeps capability-state
// prerequisites bound to the same canonical owner contract as the evidence they
// unlock. Profiles declare operation kinds, not EventHub destinations; accepting
// an arbitrary owner string here would otherwise create a sealed prerequisite
// that no owner can receive at runtime.
func runtimeProfileCapabilityStateOwnerMatchesAcceptedStepKind(ownerModuleID string, acceptedStepKinds []string) bool {
	ownerModuleID = strings.TrimSpace(ownerModuleID)
	if ownerModuleID == "" {
		return false
	}
	for _, kind := range acceptedStepKinds {
		descriptor, ok := RuntimeOperationDescriptorForKind(CanonicalRuntimeBlockKind(kind))
		if ok && descriptor.OwnerModuleID == ownerModuleID {
			return true
		}
	}
	return false
}

func SameRuntimeProfileContracts(left []RuntimeProfileContract, right []RuntimeProfileContract) bool {
	left = NormalizeRuntimeProfileContracts(left)
	right = NormalizeRuntimeProfileContracts(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if !sameRuntimeProfileContract(left[idx], right[idx]) {
			return false
		}
	}
	return true
}

func normalizeRuntimeAcceptanceDimensions(values []RuntimeAcceptanceDimension) []RuntimeAcceptanceDimension {
	byID := map[string]RuntimeAcceptanceDimension{}
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.Description = strings.TrimSpace(value.Description)
		if value.ID != "" {
			byID[value.ID] = value
		}
	}
	return sortedMapValues(byID)
}

func normalizeRuntimeDeliverableExpectations(values []RuntimeDeliverableExpectation) []RuntimeDeliverableExpectation {
	byID := map[string]RuntimeDeliverableExpectation{}
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.EvidenceRole = strings.TrimSpace(value.EvidenceRole)
		value.Description = strings.TrimSpace(value.Description)
		if value.ID != "" {
			byID[value.ID] = value
		}
	}
	return sortedMapValues(byID)
}

func normalizeRuntimeProfileEvidenceRequirements(values []RuntimeProfileEvidenceRequirement) []RuntimeProfileEvidenceRequirement {
	ret := make([]RuntimeProfileEvidenceRequirement, 0, len(values))
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.Scope = strings.TrimSpace(value.Scope)
		value.EvidenceRole = strings.TrimSpace(value.EvidenceRole)
		value.TargetRole = strings.TrimSpace(value.TargetRole)
		value.TargetRefs = normalizeStrings(value.TargetRefs)
		value.TargetDiscoverySources = normalizeStrings(value.TargetDiscoverySources)
		value.TargetSelectorRef = strings.TrimSpace(value.TargetSelectorRef)
		value.TargetSelectorOwnerRef = strings.TrimSpace(value.TargetSelectorOwnerRef)
		value.TargetSelectorSubjectRef = strings.TrimSpace(value.TargetSelectorSubjectRef)
		value.TargetSelectorCandidateRefs = normalizeStrings(value.TargetSelectorCandidateRefs)
		value.RequiredEvidenceKinds = normalizeStrings(value.RequiredEvidenceKinds)
		value.AcceptedStepKinds = normalizeStrings(value.AcceptedStepKinds)
		value.AcceptedOperationVariants = NormalizeRuntimeOperationVariantRefs(value.AcceptedOperationVariants)
		value.ValidationRecipeID = strings.TrimSpace(value.ValidationRecipeID)
		value.RequiredCheckRefs = normalizeStrings(value.RequiredCheckRefs)
		value.MetricConstraints = normalizeStrings(value.MetricConstraints)
		value.ArtifactRefs = normalizeStrings(value.ArtifactRefs)
		value.RequiredOwnerFacts = NormalizeRuntimeOwnerFactRequirements(value.RequiredOwnerFacts)
		value.OwnerFactConvergences = NormalizeRuntimeOwnerFactConvergences(value.OwnerFactConvergences)
		value.ValidationFailureRepairConvergence = cloneRuntimeValidationFailureRepairConvergence(value.ValidationFailureRepairConvergence)
		value.CapabilityStateRequirements = normalizeRuntimeProfileCapabilityStateRequirements(value.CapabilityStateRequirements)
		value.RejectedStepKinds = normalizeStrings(value.RejectedStepKinds)
		value.Description = strings.TrimSpace(value.Description)
		ret = append(ret, value)
	}
	sort.SliceStable(ret, func(i, j int) bool {
		return runtimeProfileSourceKey(ret[i].Scope, ret[i].ID) < runtimeProfileSourceKey(ret[j].Scope, ret[j].ID)
	})
	return nilIfEmpty(ret)
}

func normalizeRuntimeProfileCapabilityStateRequirements(values []RuntimeProfileCapabilityStateRequirement) []RuntimeProfileCapabilityStateRequirement {
	byKey := map[string]RuntimeProfileCapabilityStateRequirement{}
	for _, value := range values {
		value.OwnerModuleID = strings.TrimSpace(value.OwnerModuleID)
		value.CapabilityStateKind = strings.TrimSpace(value.CapabilityStateKind)
		value.SubjectRef = strings.TrimSpace(value.SubjectRef)
		if value.OwnerModuleID != "" && value.CapabilityStateKind != "" && value.SubjectRef != "" {
			byKey[value.OwnerModuleID+"|"+value.CapabilityStateKind+"|"+value.SubjectRef] = value
		}
	}
	return sortedMapValues(byKey)
}

func normalizeRuntimeCoverageDependencies(values []RuntimeCoverageDependency) []RuntimeCoverageDependency {
	byKey := map[string]RuntimeCoverageDependency{}
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.SourceScope = strings.TrimSpace(value.SourceScope)
		value.SourceRequirementID = strings.TrimSpace(value.SourceRequirementID)
		value.DependentScope = strings.TrimSpace(value.DependentScope)
		value.DependentRequirementID = strings.TrimSpace(value.DependentRequirementID)
		value.TriggerEvidenceKinds = normalizeStrings(value.TriggerEvidenceKinds)
		key := strings.Join([]string{value.ID, value.SourceScope, value.SourceRequirementID, value.DependentScope, value.DependentRequirementID}, "|")
		byKey[key] = value
	}
	return sortedMapValues(byKey)
}

func sameRuntimeProfileContract(left RuntimeProfileContract, right RuntimeProfileContract) bool {
	leftValues := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{left})
	rightValues := NormalizeRuntimeProfileContracts([]RuntimeProfileContract{right})
	if len(leftValues) != 1 || len(rightValues) != 1 {
		return len(leftValues) == len(rightValues)
	}
	left, right = leftValues[0], rightValues[0]
	if left.ID != right.ID || len(left.AcceptanceDimensions) != len(right.AcceptanceDimensions) ||
		len(left.DeliverableExpectations) != len(right.DeliverableExpectations) || len(left.EvidenceRequirements) != len(right.EvidenceRequirements) ||
		len(left.CoverageDependencies) != len(right.CoverageDependencies) || len(left.BootstrapRecipes) != len(right.BootstrapRecipes) {
		return false
	}
	for idx := range left.AcceptanceDimensions {
		if left.AcceptanceDimensions[idx] != right.AcceptanceDimensions[idx] {
			return false
		}
	}
	for idx := range left.DeliverableExpectations {
		if left.DeliverableExpectations[idx] != right.DeliverableExpectations[idx] {
			return false
		}
	}
	for idx := range left.EvidenceRequirements {
		l, r := left.EvidenceRequirements[idx], right.EvidenceRequirements[idx]
		if l.ID != r.ID || l.Scope != r.Scope || l.EvidenceRole != r.EvidenceRole || l.TargetRole != r.TargetRole || l.TargetSelectorRef != r.TargetSelectorRef || l.TargetSelectorOwnerRef != r.TargetSelectorOwnerRef || l.TargetSelectorSubjectRef != r.TargetSelectorSubjectRef ||
			!sameStrings(l.TargetRefs, r.TargetRefs) || !sameStrings(l.TargetDiscoverySources, r.TargetDiscoverySources) || !sameStrings(l.TargetSelectorCandidateRefs, r.TargetSelectorCandidateRefs) || l.RequireTargetGrounding != r.RequireTargetGrounding ||
			!sameStrings(l.RequiredEvidenceKinds, r.RequiredEvidenceKinds) || l.RequireDirectEvidence != r.RequireDirectEvidence ||
			l.RequireClaimEvidence != r.RequireClaimEvidence || l.RequireExecutableBehaviorEvidence != r.RequireExecutableBehaviorEvidence ||
			l.RequireCoverageTargets != r.RequireCoverageTargets || l.ValidationRecipeID != r.ValidationRecipeID || l.Description != r.Description ||
			!sameStrings(l.AcceptedStepKinds, r.AcceptedStepKinds) || !SameRuntimeOperationVariantRefs(l.AcceptedOperationVariants, r.AcceptedOperationVariants) || !sameStrings(l.RequiredCheckRefs, r.RequiredCheckRefs) || !sameStrings(l.MetricConstraints, r.MetricConstraints) || !sameStrings(l.ArtifactRefs, r.ArtifactRefs) || !sameRuntimeOwnerFactRequirements(l.RequiredOwnerFacts, r.RequiredOwnerFacts) || !SameRuntimeOwnerFactConvergences(l.OwnerFactConvergences, r.OwnerFactConvergences) || !SameRuntimeValidationFailureRepairConvergence(l.ValidationFailureRepairConvergence, r.ValidationFailureRepairConvergence) || !sameStrings(l.RejectedStepKinds, r.RejectedStepKinds) {
			return false
		}
		if len(l.CapabilityStateRequirements) != len(r.CapabilityStateRequirements) {
			return false
		}
		for idx := range l.CapabilityStateRequirements {
			if l.CapabilityStateRequirements[idx] != r.CapabilityStateRequirements[idx] {
				return false
			}
		}
	}
	for idx := range left.CoverageDependencies {
		l, r := left.CoverageDependencies[idx], right.CoverageDependencies[idx]
		if l.ID != r.ID || l.SourceScope != r.SourceScope || l.SourceRequirementID != r.SourceRequirementID ||
			l.DependentScope != r.DependentScope || l.DependentRequirementID != r.DependentRequirementID ||
			l.RequireSourceSettled != r.RequireSourceSettled ||
			!sameStrings(l.TriggerEvidenceKinds, r.TriggerEvidenceKinds) {
			return false
		}
	}
	for idx := range left.BootstrapRecipes {
		l, r := left.BootstrapRecipes[idx], right.BootstrapRecipes[idx]
		if l.ID != r.ID || !sameStrings(l.RequiredGoalTerms, r.RequiredGoalTerms) || !sameStrings(l.RequiredDoDTerms, r.RequiredDoDTerms) || len(l.Artifacts) != len(r.Artifacts) {
			return false
		}
		for artifactIdx := range l.Artifacts {
			la, ra := l.Artifacts[artifactIdx], r.Artifacts[artifactIdx]
			if la.ID != ra.ID || la.TargetRole != ra.TargetRole ||
				!sameStrings(la.AllowedOperationKinds, ra.AllowedOperationKinds) || !sameStrings(la.DependsOnArtifactIDs, ra.DependsOnArtifactIDs) {
				return false
			}
		}
	}
	return true
}

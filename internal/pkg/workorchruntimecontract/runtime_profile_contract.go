package workorchruntimecontract

import (
	"fmt"
	"slices"
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

// CloneRuntimeProfileContracts copies archived declarations without applying
// today's normalization, defaults or deduplication rules.
func CloneRuntimeProfileContracts(values []RuntimeProfileContract) []RuntimeProfileContract {
	return cloneRuntimeStateRawSlice(values, cloneRuntimeProfileContractRaw)
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
	// Optional opt-in; an empty extension retains legacy snapshot semantics.
	RequiredEvidencePurposeContractRef string `json:",omitempty"`
	ID                                 string
	Required                           bool
	EvidenceRole                       string
	Description                        string
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
	TargetSelectorOwnerRef      string
	TargetSelectorSubjectRef    string
	TargetSelectorCandidateRefs []string
	// ProspectiveTargetAllOfConstraintRefs is the immutable pre-seal creation
	// declaration. Every ref is mandatory and exact prospective targets are
	// intentionally absent until an SM-3 proposal is owner-validated.
	ProspectiveTargetAllOfConstraintRefs []string
	RequireTargetGrounding               bool
	RequiredEvidenceKinds                []string
	RequireDirectEvidence                bool
	RequireClaimEvidence                 bool
	RequireExecutableBehaviorEvidence    bool
	RequireCoverageTargets               bool
	AcceptedStepKinds                    []string
	AcceptedOperationVariants            []RuntimeOperationVariantRef
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
	// LineageAssertions 是普通 proof 之外的 all-of guard，不授予独立 evidence credit。
	LineageAssertions []RuntimeLineageAssertionRule `json:",omitempty"`
	// ImplementationWorkPolicy is a separate, default-absent declaration. It
	// cannot borrow the rejected-validation trigger or change terminal proof.
	ImplementationWorkPolicy    *RuntimeImplementationWorkPolicy
	VerificationPolicy          *RuntimeVerificationPolicy `json:",omitempty"`
	CapabilityStateRequirements []RuntimeProfileCapabilityStateRequirement
	RejectedStepKinds           []string
	Description                 string
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
	// These are authority declarations, not an override map. Keep invalid and
	// duplicate entries until admission can reject them; last-write-wins would
	// silently discard required work before Workspace seals the Definition.
	ret := make([]RuntimeProfileContract, 0, len(values))
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.AcceptanceDimensions = normalizeRuntimeAcceptanceDimensions(value.AcceptanceDimensions)
		value.DeliverableExpectations = normalizeRuntimeDeliverableExpectations(value.DeliverableExpectations)
		value.EvidenceRequirements = normalizeRuntimeProfileEvidenceRequirements(value.EvidenceRequirements)
		value.CoverageDependencies = normalizeRuntimeCoverageDependencies(value.CoverageDependencies)
		value.BootstrapRecipes = normalizeRuntimeBootstrapRecipes(value.BootstrapRecipes)
		ret = append(ret, value)
	}
	sort.SliceStable(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	return nilIfEmpty(ret)
}

func ValidateRuntimeProfileContracts(values []RuntimeProfileContract) error {
	seenProfiles := make(map[string]struct{}, len(values))
	for _, profile := range NormalizeRuntimeProfileContracts(values) {
		if profile.ID == "" {
			return fmt.Errorf("runtime profile contract requires id")
		}
		if _, duplicate := seenProfiles[profile.ID]; duplicate {
			return fmt.Errorf("runtime profile contract duplicates id %q", profile.ID)
		}
		seenProfiles[profile.ID] = struct{}{}
		seenEvidenceRequirementRefs := make(map[string]struct{}, len(profile.EvidenceRequirements))
		seenDimensions := make(map[string]struct{}, len(profile.AcceptanceDimensions))
		for _, item := range profile.AcceptanceDimensions {
			if item.ID == "" {
				return fmt.Errorf("runtime acceptance dimension requires id")
			}
			if _, duplicate := seenDimensions[item.ID]; duplicate {
				return fmt.Errorf("runtime profile %q duplicates acceptance dimension %q", profile.ID, item.ID)
			}
			seenDimensions[item.ID] = struct{}{}
		}
		seenDeliverables := make(map[string]struct{}, len(profile.DeliverableExpectations))
		for _, item := range profile.DeliverableExpectations {
			if item.ID == "" {
				return fmt.Errorf("runtime deliverable expectation requires id")
			}
			if _, duplicate := seenDeliverables[item.ID]; duplicate {
				return fmt.Errorf("runtime profile %q duplicates deliverable expectation %q", profile.ID, item.ID)
			}
			seenDeliverables[item.ID] = struct{}{}
			if item.RequiredEvidencePurposeContractRef != "" && (!item.Required || item.RequiredEvidencePurposeContractRef != RuntimeProfileDeliverableEvidencePurposeV2 || strings.TrimSpace(item.Description) == "") {
				return fmt.Errorf("runtime deliverable requires a supported required evidence purpose and original text")
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
			if item.ValidationRecipeID != "" && !containsString(item.RequiredEvidenceKinds, EvidenceKindValidation) {
				return fmt.Errorf("runtime profile evidence requirement %q validation recipe requires validation evidence", item.ID)
			}
			if len(item.RequiredCheckRefs) != 0 && !containsString(item.RequiredEvidenceKinds, EvidenceKindValidation) {
				return fmt.Errorf("runtime profile evidence requirement %q checks require validation evidence", item.ID)
			}
			if duplicate := duplicateRuntimeValidationProofCheckRef(item.RequiredCheckRefs); duplicate != "" {
				return fmt.Errorf("runtime profile evidence requirement %q duplicates check %q", item.ID, duplicate)
			}
			if err := ValidateRuntimeTargetGrounding(item.RequireTargetGrounding, item.TargetRole, item.TargetRefs, item.TargetDiscoverySources, item.TargetSelectorRef); err != nil {
				return fmt.Errorf("runtime profile evidence requirement %q: %w", item.ID, err)
			}
			if len(item.TargetSelectorCandidateRefs) > 0 {
				if !item.RequireTargetGrounding || item.TargetSelectorRef == "" || item.TargetSelectorOwnerRef == "" || item.TargetSelectorSubjectRef == "" {
					return fmt.Errorf("runtime profile evidence requirement %q candidate selector requires grounding and a complete selector", item.ID)
				}
			}
			if len(item.ProspectiveTargetAllOfConstraintRefs) > 0 {
				if !item.RequireTargetGrounding || item.TargetSelectorRef == "" || item.TargetSelectorOwnerRef == "" || item.TargetSelectorSubjectRef == "" {
					return fmt.Errorf("runtime profile evidence requirement %q prospective target all-of declaration requires grounding and a complete selector", item.ID)
				}
				if err := ValidateRuntimeProspectiveTargetAllOfConstraintRefs(item.ProspectiveTargetAllOfConstraintRefs); err != nil {
					return fmt.Errorf("runtime profile evidence requirement %q: %w", item.ID, err)
				}
			}
			seenStates := make(map[RuntimeProfileCapabilityStateRequirement]struct{}, len(item.CapabilityStateRequirements))
			for _, state := range item.CapabilityStateRequirements {
				if state.OwnerModuleID == "" || state.CapabilityStateKind == "" || state.SubjectRef == "" {
					return fmt.Errorf("runtime profile evidence requirement %q requires complete capability-state identity", item.ID)
				}
				if _, duplicate := seenStates[state]; duplicate {
					return fmt.Errorf("runtime profile evidence requirement %q duplicates capability-state identity", item.ID)
				}
				seenStates[state] = struct{}{}
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
			if item.VerificationPolicy != nil {
				if err := item.VerificationPolicy.Validate(); err != nil {
					return fmt.Errorf("runtime profile evidence requirement %q: %w", item.ID, err)
				}
			}
			if err := ValidateRuntimeLineageAssertionRules(item.LineageAssertions); err != nil {
				return fmt.Errorf("runtime profile evidence requirement %q: %w", item.ID, err)
			}
			if len(item.LineageAssertions) != 0 && len(item.RequiredEvidenceKinds) == 0 {
				return fmt.Errorf("lineage assertions cannot provide standalone evidence credit")
			}
			if item.ImplementationWorkPolicy != nil {
				if len(item.RequiredEvidenceKinds) != 1 || item.RequiredEvidenceKinds[0] != EvidenceKindValidation {
					return fmt.Errorf("runtime profile evidence requirement %q implementation work must preserve validation-only terminal evidence", item.ID)
				}
				if err := item.ImplementationWorkPolicy.Validate(); err != nil {
					return fmt.Errorf("runtime profile evidence requirement %q: %w", item.ID, err)
				}
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
			seenVariants := make(map[RuntimeOperationVariantRef]struct{}, len(item.AcceptedOperationVariants))
			for _, variant := range item.AcceptedOperationVariants {
				if variant.Kind == "" || variant.VariantID == "" {
					return fmt.Errorf("runtime profile evidence requirement %q requires complete operation variant identity", item.ID)
				}
				if _, duplicate := seenVariants[variant]; duplicate {
					return fmt.Errorf("runtime profile evidence requirement %q duplicates operation variant identity", item.ID)
				}
				seenVariants[variant] = struct{}{}
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
			for _, right := range profile.EvidenceRequirements[leftIndex+1:] {
				if left.VerificationPolicy != nil && right.VerificationPolicy != nil && left.Scope == right.Scope &&
					(left.ID == "*" || right.ID == "*" || left.ID == right.ID) &&
					!SameRuntimeVerificationPolicy(left.VerificationPolicy, right.VerificationPolicy) {
					return fmt.Errorf("runtime profile evidence requirements %q and %q declare conflicting verification policies", left.ID, right.ID)
				}
				if left.ImplementationWorkPolicy != nil && right.ImplementationWorkPolicy != nil && left.Scope == right.Scope &&
					(left.ID == "*" || right.ID == "*" || left.ID == right.ID) &&
					!SameRuntimeImplementationWorkPolicy(left.ImplementationWorkPolicy, right.ImplementationWorkPolicy) {
					return fmt.Errorf("runtime profile evidence requirements %q and %q declare conflicting implementation work policies", left.ID, right.ID)
				}
			}
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
		seenDependencies := make(map[string]struct{}, len(profile.CoverageDependencies))
		for _, dependency := range profile.CoverageDependencies {
			if dependency.ID == "" || dependency.SourceScope == "" || dependency.SourceRequirementID == "" ||
				dependency.DependentScope == "" || dependency.DependentRequirementID == "" {
				return fmt.Errorf("runtime coverage dependency requires identity, source and dependent bindings")
			}
			if _, duplicate := seenDependencies[dependency.ID]; duplicate {
				return fmt.Errorf("runtime profile %q duplicates coverage dependency %q", profile.ID, dependency.ID)
			}
			seenDependencies[dependency.ID] = struct{}{}
			if dependency.SourceScope == dependency.DependentScope && dependency.SourceRequirementID == dependency.DependentRequirementID {
				return fmt.Errorf("runtime coverage dependency %q cannot target itself", dependency.ID)
			}
			if !dependency.RequireSourceSettled && len(dependency.TriggerEvidenceKinds) == 0 {
				return fmt.Errorf("runtime coverage dependency %q requires admission or invalidation semantics", dependency.ID)
			}
		}
		seenRecipes := make(map[string]struct{}, len(profile.BootstrapRecipes))
		for _, recipe := range profile.BootstrapRecipes {
			if err := recipe.Validate(); err != nil {
				return fmt.Errorf("runtime bootstrap recipe %q: %w", recipe.ID, err)
			}
			if _, duplicate := seenRecipes[recipe.ID]; duplicate {
				return fmt.Errorf("runtime profile %q duplicates bootstrap recipe %q", profile.ID, recipe.ID)
			}
			seenRecipes[recipe.ID] = struct{}{}
		}
	}
	if violations := RuntimeProfileProofClosureViolations(values); len(violations) > 0 {
		first := violations[0]
		return fmt.Errorf("runtime profile proof closure %q violates %s", first.SourceRef(), first.Rule)
	}
	return nil
}

func normalizeRuntimeBootstrapRecipes(values []RuntimeBootstrapRecipe) []RuntimeBootstrapRecipe {
	ret := make([]RuntimeBootstrapRecipe, 0, len(values))
	for _, value := range values {
		ret = append(ret, value.Normalized())
	}
	sort.SliceStable(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	return nilIfEmpty(ret)
}

func (r RuntimeBootstrapRecipe) Normalized() RuntimeBootstrapRecipe {
	r.ID = strings.TrimSpace(r.ID)
	r.RequiredGoalTerms = normalizeStrings(r.RequiredGoalTerms)
	r.RequiredDoDTerms = normalizeStrings(r.RequiredDoDTerms)
	artifacts := make([]RuntimeBootstrapArtifact, 0, len(r.Artifacts))
	for _, artifact := range r.Artifacts {
		artifacts = append(artifacts, artifact.Normalized())
	}
	sort.SliceStable(artifacts, func(i, j int) bool { return artifacts[i].ID < artifacts[j].ID })
	r.Artifacts = nilIfEmpty(artifacts)
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
	ret := make([]RuntimeAcceptanceDimension, 0, len(values))
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.Description = strings.TrimSpace(value.Description)
		ret = append(ret, value)
	}
	sort.SliceStable(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	return nilIfEmpty(ret)
}

func normalizeRuntimeDeliverableExpectations(values []RuntimeDeliverableExpectation) []RuntimeDeliverableExpectation {
	ret := make([]RuntimeDeliverableExpectation, 0, len(values))
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.EvidenceRole = strings.TrimSpace(value.EvidenceRole)
		value.Description = strings.TrimSpace(value.Description)
		ret = append(ret, value)
	}
	sort.SliceStable(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	return nilIfEmpty(ret)
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
		value.ProspectiveTargetAllOfConstraintRefs = NormalizeRuntimeProspectiveTargetAllOfConstraintRefs(value.ProspectiveTargetAllOfConstraintRefs)
		value.RequiredEvidenceKinds = normalizeStrings(value.RequiredEvidenceKinds)
		value.AcceptedStepKinds = normalizeStrings(value.AcceptedStepKinds)
		variants := append([]RuntimeOperationVariantRef(nil), value.AcceptedOperationVariants...)
		for i := range variants {
			variants[i].Kind = CanonicalRuntimeBlockKind(variants[i].Kind)
			variants[i].VariantID = strings.TrimSpace(variants[i].VariantID)
		}
		sort.SliceStable(variants, func(i, j int) bool {
			if variants[i].Kind != variants[j].Kind {
				return variants[i].Kind < variants[j].Kind
			}
			return variants[i].VariantID < variants[j].VariantID
		})
		value.AcceptedOperationVariants = nilIfEmpty(variants)
		value.ValidationRecipeID = strings.TrimSpace(value.ValidationRecipeID)
		value.RequiredCheckRefs = normalizeRuntimeValidationProofCheckRefs(value.RequiredCheckRefs)
		value.MetricConstraints = normalizeStrings(value.MetricConstraints)
		value.ArtifactRefs = normalizeStrings(value.ArtifactRefs)
		// Profile declarations must retain invalid/duplicate facts until admission;
		// the general owner-fact set helper is for merging already-admitted facts.
		facts := append([]RuntimeOwnerFactRequirement(nil), value.RequiredOwnerFacts...)
		for i := range facts {
			facts[i] = facts[i].Normalized()
		}
		sort.SliceStable(facts, func(i, j int) bool {
			return RuntimeOwnerFactRequirementRef(facts[i]) < RuntimeOwnerFactRequirementRef(facts[j])
		})
		value.RequiredOwnerFacts = nilIfEmpty(facts)
		value.OwnerFactConvergences = NormalizeRuntimeOwnerFactConvergences(value.OwnerFactConvergences)
		value.LineageAssertions = slices.Clone(value.LineageAssertions)
		value.ValidationFailureRepairConvergence = cloneRuntimeValidationFailureRepairConvergence(value.ValidationFailureRepairConvergence)
		if value.ImplementationWorkPolicy != nil {
			policy := value.ImplementationWorkPolicy.Normalized()
			value.ImplementationWorkPolicy = &policy
		}
		if value.VerificationPolicy != nil {
			policy := value.VerificationPolicy.Normalized()
			value.VerificationPolicy = &policy
		}
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
	ret := make([]RuntimeProfileCapabilityStateRequirement, 0, len(values))
	for _, value := range values {
		value.OwnerModuleID = strings.TrimSpace(value.OwnerModuleID)
		value.CapabilityStateKind = strings.TrimSpace(value.CapabilityStateKind)
		value.SubjectRef = strings.TrimSpace(value.SubjectRef)
		ret = append(ret, value)
	}
	sort.SliceStable(ret, func(i, j int) bool {
		l, r := ret[i], ret[j]
		if l.OwnerModuleID != r.OwnerModuleID {
			return l.OwnerModuleID < r.OwnerModuleID
		}
		if l.CapabilityStateKind != r.CapabilityStateKind {
			return l.CapabilityStateKind < r.CapabilityStateKind
		}
		return l.SubjectRef < r.SubjectRef
	})
	return nilIfEmpty(ret)
}

func normalizeRuntimeCoverageDependencies(values []RuntimeCoverageDependency) []RuntimeCoverageDependency {
	ret := make([]RuntimeCoverageDependency, 0, len(values))
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.SourceScope = strings.TrimSpace(value.SourceScope)
		value.SourceRequirementID = strings.TrimSpace(value.SourceRequirementID)
		value.DependentScope = strings.TrimSpace(value.DependentScope)
		value.DependentRequirementID = strings.TrimSpace(value.DependentRequirementID)
		value.TriggerEvidenceKinds = normalizeStrings(value.TriggerEvidenceKinds)
		ret = append(ret, value)
	}
	sort.SliceStable(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	return nilIfEmpty(ret)
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
		if !slices.Equal(l.LineageAssertions, r.LineageAssertions) || !SameRuntimeImplementationWorkPolicy(l.ImplementationWorkPolicy, r.ImplementationWorkPolicy) ||
			!SameRuntimeVerificationPolicy(l.VerificationPolicy, r.VerificationPolicy) {
			return false
		}
		if l.ID != r.ID || l.Scope != r.Scope || l.EvidenceRole != r.EvidenceRole || l.TargetRole != r.TargetRole || l.TargetSelectorRef != r.TargetSelectorRef || l.TargetSelectorOwnerRef != r.TargetSelectorOwnerRef || l.TargetSelectorSubjectRef != r.TargetSelectorSubjectRef ||
			!sameStrings(l.TargetRefs, r.TargetRefs) || !sameStrings(l.TargetDiscoverySources, r.TargetDiscoverySources) || !sameStrings(l.TargetSelectorCandidateRefs, r.TargetSelectorCandidateRefs) || !SameRuntimeProspectiveTargetAllOfConstraintRefs(l.ProspectiveTargetAllOfConstraintRefs, r.ProspectiveTargetAllOfConstraintRefs) || l.RequireTargetGrounding != r.RequireTargetGrounding ||
			!sameStrings(l.RequiredEvidenceKinds, r.RequiredEvidenceKinds) || l.RequireDirectEvidence != r.RequireDirectEvidence ||
			l.RequireClaimEvidence != r.RequireClaimEvidence || l.RequireExecutableBehaviorEvidence != r.RequireExecutableBehaviorEvidence ||
			l.RequireCoverageTargets != r.RequireCoverageTargets || l.ValidationRecipeID != r.ValidationRecipeID || l.Description != r.Description ||
			!sameStrings(l.AcceptedStepKinds, r.AcceptedStepKinds) || !slices.Equal(l.AcceptedOperationVariants, r.AcceptedOperationVariants) || !sameStrings(l.RequiredCheckRefs, r.RequiredCheckRefs) || !sameStrings(l.MetricConstraints, r.MetricConstraints) || !sameStrings(l.ArtifactRefs, r.ArtifactRefs) || !slices.Equal(l.RequiredOwnerFacts, r.RequiredOwnerFacts) || !SameRuntimeOwnerFactConvergences(l.OwnerFactConvergences, r.OwnerFactConvergences) || !SameRuntimeValidationFailureRepairConvergence(l.ValidationFailureRepairConvergence, r.ValidationFailureRepairConvergence) || !sameStrings(l.RejectedStepKinds, r.RejectedStepKinds) {
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

// RuntimeSourceEvidencePolicy is the pure exact-source merge product. It is not a binding or execution authority.
type RuntimeSourceEvidencePolicy struct {
	RequiredEvidenceKinds                []string
	AcceptedStepKinds                    []string
	AcceptedOperationVariants            []RuntimeOperationVariantRef
	RejectedStepKinds                    []string
	EvidenceRole                         string
	TargetRole                           string
	TargetRefs                           []string
	TargetDiscoverySources               []string
	TargetSelectorRef                    string
	TargetSelectorOwnerRef               string
	TargetSelectorSubjectRef             string
	TargetSelectorCandidateRefs          []string
	ProspectiveTargetAllOfConstraintRefs []string
	RequireTargetGrounding               bool
	RequireDirectEvidence                bool
	RequireClaimEvidence                 bool
	RequireExecutableBehaviorEvidence    bool
	RequireCoverageTargets               bool
	RequireAssessment                    bool
	RequiredOwnerFacts                   []RuntimeOwnerFactRequirement
	OwnerFactConvergences                []RuntimeOwnerFactConvergence
	ValidationFailureRepairConvergence   *RuntimeValidationFailureRepairConvergence
	LineageAssertions                    []RuntimeLineageAssertionRule
	ImplementationWorkPolicy             *RuntimeImplementationWorkPolicy
	VerificationPolicy                   *RuntimeVerificationPolicy
	ValidationRecipeID                   string
	RequiredCheckRefs                    []string
	MetricConstraints                    []string
	ArtifactRefs                         []string
	DeclarationRef                       string
	SemanticProofDeclarationRef          string
	Description                          string
}

// RuntimeEvidenceDeclarationMatchesSource matches declaration selectors against source identity, never a CWI ID.
func RuntimeEvidenceDeclarationMatchesSource(declaration RuntimeProfileEvidenceRequirement, scope, sourceID string) bool {
	return strings.TrimSpace(declaration.Scope) == strings.TrimSpace(scope) && strings.TrimSpace(sourceID) != "" &&
		(strings.TrimSpace(declaration.ID) == strings.TrimSpace(sourceID) || strings.TrimSpace(declaration.ID) == "*")
}

// CompileRuntimeSourceEvidencePolicy retains all exact and wildcard contributions,
// in the same order used by activation. Wildcard recipe/check identities are not instantiated.
// Additional requirements are adapter-owned policy projections, not owner state.
func CompileRuntimeSourceEvidencePolicy(scope, sourceID string, profiles []RuntimeProfileContract, additional []RuntimeProfileEvidenceRequirement) (RuntimeSourceEvidencePolicy, error) {
	scope, sourceID = strings.TrimSpace(scope), strings.TrimSpace(sourceID)
	if scope == "" || sourceID == "" {
		return RuntimeSourceEvidencePolicy{}, fmt.Errorf("source evidence policy requires exact scope and source identity")
	}
	candidates := make([]RuntimeProfileEvidenceRequirement, 0)
	for _, profile := range runtimeSourcePolicyProfileProjection(scope, sourceID, profiles) {
		for _, requirement := range profile.EvidenceRequirements {
			if !RuntimeEvidenceDeclarationMatchesSource(requirement, scope, sourceID) {
				continue
			}
			requirement.TargetSelectorRef, requirement.TargetSelectorOwnerRef, requirement.TargetSelectorSubjectRef = ResolveRuntimeProfileTargetSelectorIdentity(profile.ID, requirement)
			candidates = append(candidates, requirement)
		}
	}
	for _, requirement := range additional {
		if RuntimeEvidenceDeclarationMatchesSource(requirement, scope, sourceID) {
			candidates = append(candidates, requirement)
		}
	}
	for _, requirement := range candidates {
		if err := validateRuntimeSourceAcceptedKinds(requirement); err != nil {
			return RuntimeSourceEvidencePolicy{}, err
		}
	}
	policy, err := mergeRuntimeSourceEvidencePolicy(RuntimeSourceEvidencePolicy{RequireAssessment: true}, runtimeSourceFilterRequirementsByID(candidates, sourceID))
	if err != nil {
		return RuntimeSourceEvidencePolicy{}, err
	}
	if sourceID != "*" {
		policy, err = mergeRuntimeSourceEvidencePolicy(policy, runtimeSourceWildcardQualityRequirements(runtimeSourceFilterRequirementsByID(candidates, "*")))
		if err != nil {
			return RuntimeSourceEvidencePolicy{}, err
		}
	}
	if len(policy.RequiredEvidenceKinds) == 0 {
		switch scope {
		case "dodItem", "goalCoverage", "acceptanceDimension":
			policy.RequiredEvidenceKinds = []string{EvidenceKindValidation}
		}
	}
	if err := ValidateRuntimeOwnerFactConvergences(policy.RequiredOwnerFacts, policy.OwnerFactConvergences); err != nil {
		return RuntimeSourceEvidencePolicy{}, err
	}
	policy.SemanticProofDeclarationRef = CanonicalRuntimeDefinitionEvidenceDeclarationRef(scope, sourceID)
	return policy, nil
}

// The source compiler reads only profile ID and exact/wildcard requirements.
// Select those by the same raw-trim predicate before normalization so unrelated
// declaration graphs are not repeatedly copied. Stable profile/requirement
// order, duplicates, full matching requirements and all merge gates remain the
// original ones. This is not profile admission or a validation-success cache.
func runtimeSourcePolicyProfileProjection(scope, sourceID string, profiles []RuntimeProfileContract) []RuntimeProfileContract {
	ret := make([]RuntimeProfileContract, 0, len(profiles))
	for _, profile := range profiles {
		var matched []RuntimeProfileEvidenceRequirement
		for _, requirement := range profile.EvidenceRequirements {
			if RuntimeEvidenceDeclarationMatchesSource(requirement, scope, sourceID) {
				matched = append(matched, requirement)
			}
		}
		ret = append(ret, RuntimeProfileContract{ID: strings.TrimSpace(profile.ID), EvidenceRequirements: normalizeRuntimeProfileEvidenceRequirements(matched)})
	}
	sort.SliceStable(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	return nilIfEmpty(ret)
}

func runtimeSourceWildcardQualityRequirements(values []RuntimeProfileEvidenceRequirement) []RuntimeProfileEvidenceRequirement {
	ret := make([]RuntimeProfileEvidenceRequirement, 0, len(values))
	for _, value := range values {
		// A wildcard is a declarative policy template, not Runtime authority. The
		// Application may instantiate its constraints into an already admitted
		// exact CWI, whose later owner catalog and SM-2 binding remain necessary.
		// Only wildcard identity and recipe/check proof identity are discarded:
		// those must come from the canonical exact Definition and owner catalog.
		ret = append(ret, RuntimeProfileEvidenceRequirement{
			EvidenceRole:                         value.EvidenceRole,
			TargetRole:                           value.TargetRole,
			TargetRefs:                           append([]string(nil), value.TargetRefs...),
			TargetDiscoverySources:               append([]string(nil), value.TargetDiscoverySources...),
			TargetSelectorRef:                    value.TargetSelectorRef,
			TargetSelectorOwnerRef:               value.TargetSelectorOwnerRef,
			TargetSelectorSubjectRef:             value.TargetSelectorSubjectRef,
			TargetSelectorCandidateRefs:          append([]string(nil), value.TargetSelectorCandidateRefs...),
			ProspectiveTargetAllOfConstraintRefs: append([]string(nil), value.ProspectiveTargetAllOfConstraintRefs...),
			RequireTargetGrounding:               value.RequireTargetGrounding,
			RequiredEvidenceKinds:                append([]string(nil), value.RequiredEvidenceKinds...),
			RequireDirectEvidence:                value.RequireDirectEvidence,
			RequireClaimEvidence:                 value.RequireClaimEvidence,
			RequireExecutableBehaviorEvidence:    value.RequireExecutableBehaviorEvidence,
			RequireCoverageTargets:               value.RequireCoverageTargets,
			AcceptedStepKinds:                    append([]string(nil), value.AcceptedStepKinds...),
			AcceptedOperationVariants:            append([]RuntimeOperationVariantRef(nil), value.AcceptedOperationVariants...),
			RejectedStepKinds:                    append([]string(nil), value.RejectedStepKinds...),
			MetricConstraints:                    append([]string(nil), value.MetricConstraints...),
			ArtifactRefs:                         append([]string(nil), value.ArtifactRefs...),
			LineageAssertions:                    slices.Clone(value.LineageAssertions),
			VerificationPolicy:                   value.VerificationPolicy,
			RequiredOwnerFacts:                   append([]RuntimeOwnerFactRequirement(nil), value.RequiredOwnerFacts...),
			OwnerFactConvergences:                CloneRuntimeOwnerFactConvergences(value.OwnerFactConvergences),
			Description:                          value.Description,
		})
		if value.ValidationFailureRepairConvergence != nil {
			clone := value.ValidationFailureRepairConvergence.Clone()
			ret[len(ret)-1].ValidationFailureRepairConvergence = &clone
		}
		if value.ImplementationWorkPolicy != nil {
			clone := value.ImplementationWorkPolicy.Clone()
			ret[len(ret)-1].ImplementationWorkPolicy = &clone
		}
	}
	return ret
}

func runtimeSourceFilterRequirementsByID(values []RuntimeProfileEvidenceRequirement, id string) []RuntimeProfileEvidenceRequirement {
	ret := make([]RuntimeProfileEvidenceRequirement, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value.ID) == id {
			ret = append(ret, value)
		}
	}
	return ret
}

func mergeRuntimeSourceEvidencePolicy(base RuntimeSourceEvidencePolicy, requirements []RuntimeProfileEvidenceRequirement) (RuntimeSourceEvidencePolicy, error) {
	for _, requirement := range requirements {
		var lineageErr error
		base.LineageAssertions, lineageErr = mergeRuntimeLineageAssertionRules(base.LineageAssertions, requirement.LineageAssertions)
		if lineageErr != nil {
			return RuntimeSourceEvidencePolicy{}, lineageErr
		}
		// Stable unique union for required evidence / rejected kinds.
		base.RequiredEvidenceKinds = runtimeSourceStableStringUnion(base.RequiredEvidenceKinds, requirement.RequiredEvidenceKinds)
		base.RejectedStepKinds = runtimeSourceStableStringUnion(base.RejectedStepKinds, requirement.RejectedStepKinds)
		// Boolean constraints take OR and cannot be relaxed by lower-priority layers already applied.
		base.RequireDirectEvidence = base.RequireDirectEvidence || requirement.RequireDirectEvidence
		base.RequireClaimEvidence = base.RequireClaimEvidence || requirement.RequireClaimEvidence
		base.RequireExecutableBehaviorEvidence = base.RequireExecutableBehaviorEvidence || requirement.RequireExecutableBehaviorEvidence
		base.RequireCoverageTargets = base.RequireCoverageTargets || requirement.RequireCoverageTargets
		// Roles retain first-non-empty precedence. Operation kinds/variants are
		// proof-shape declarations instead: evidence kinds are compiled into
		// separate binding proofs later, so dropping a lower-priority validation
		// kind merely because a state-change rule appeared first would make a
		// required proof impossible to express.
		if base.EvidenceRole == "" && strings.TrimSpace(requirement.EvidenceRole) != "" {
			base.EvidenceRole = strings.TrimSpace(requirement.EvidenceRole)
		}
		if base.TargetRole == "" && strings.TrimSpace(requirement.TargetRole) != "" {
			base.TargetRole = strings.TrimSpace(requirement.TargetRole)
		}
		if base.TargetSelectorRef == "" && strings.TrimSpace(requirement.TargetSelectorRef) != "" {
			base.TargetSelectorRef = strings.TrimSpace(requirement.TargetSelectorRef)
		}
		if base.TargetSelectorOwnerRef == "" && strings.TrimSpace(requirement.TargetSelectorOwnerRef) != "" {
			base.TargetSelectorOwnerRef = strings.TrimSpace(requirement.TargetSelectorOwnerRef)
		}
		if base.TargetSelectorSubjectRef == "" && strings.TrimSpace(requirement.TargetSelectorSubjectRef) != "" {
			base.TargetSelectorSubjectRef = strings.TrimSpace(requirement.TargetSelectorSubjectRef)
		}
		base.TargetSelectorCandidateRefs = runtimeSourceStableStringUnion(base.TargetSelectorCandidateRefs, requirement.TargetSelectorCandidateRefs)
		base.ProspectiveTargetAllOfConstraintRefs = runtimeSourceStableStringUnion(base.ProspectiveTargetAllOfConstraintRefs, requirement.ProspectiveTargetAllOfConstraintRefs)
		base.TargetRefs = runtimeSourceStableStringUnion(base.TargetRefs, requirement.TargetRefs)
		base.TargetDiscoverySources = runtimeSourceStableStringUnion(base.TargetDiscoverySources, requirement.TargetDiscoverySources)
		base.RequireTargetGrounding = base.RequireTargetGrounding || requirement.RequireTargetGrounding
		base.AcceptedStepKinds = runtimeSourceStableStringUnion(base.AcceptedStepKinds, requirement.AcceptedStepKinds)
		base.AcceptedOperationVariants = NormalizeRuntimeOperationVariantRefs(append(base.AcceptedOperationVariants, requirement.AcceptedOperationVariants...))
		if base.Description == "" && strings.TrimSpace(requirement.Description) != "" {
			base.Description = strings.TrimSpace(requirement.Description)
		}
		if base.ValidationRecipeID == "" && strings.TrimSpace(requirement.ValidationRecipeID) != "" {
			base.ValidationRecipeID = strings.TrimSpace(requirement.ValidationRecipeID)
		}
		base.RequiredCheckRefs = runtimeSourceStableStringUnion(base.RequiredCheckRefs, requirement.RequiredCheckRefs)
		base.MetricConstraints = runtimeSourceStableStringUnion(base.MetricConstraints, requirement.MetricConstraints)
		base.ArtifactRefs = runtimeSourceStableStringUnion(base.ArtifactRefs, requirement.ArtifactRefs)
		base.RequiredOwnerFacts = NormalizeRuntimeOwnerFactRequirements(append(base.RequiredOwnerFacts, requirement.RequiredOwnerFacts...))
		base.OwnerFactConvergences = NormalizeRuntimeOwnerFactConvergences(append(base.OwnerFactConvergences, requirement.OwnerFactConvergences...))
		if requirement.ValidationFailureRepairConvergence != nil {
			if base.ValidationFailureRepairConvergence != nil && !SameRuntimeValidationFailureRepairConvergence(base.ValidationFailureRepairConvergence, requirement.ValidationFailureRepairConvergence) {
				return RuntimeSourceEvidencePolicy{}, fmt.Errorf("coverage evidence policy has conflicting validation failure repair convergence declarations")
			}
			if base.ValidationFailureRepairConvergence == nil {
				clone := requirement.ValidationFailureRepairConvergence.Clone()
				base.ValidationFailureRepairConvergence = &clone
			}
		}
		if requirement.VerificationPolicy != nil {
			if base.VerificationPolicy != nil && !SameRuntimeVerificationPolicy(base.VerificationPolicy, requirement.VerificationPolicy) {
				return RuntimeSourceEvidencePolicy{}, fmt.Errorf("coverage evidence policy has conflicting verification policies")
			}
			base.VerificationPolicy = requirement.VerificationPolicy
		}
		if requirement.ImplementationWorkPolicy != nil {
			if base.ImplementationWorkPolicy != nil && !SameRuntimeImplementationWorkPolicy(base.ImplementationWorkPolicy, requirement.ImplementationWorkPolicy) {
				return RuntimeSourceEvidencePolicy{}, fmt.Errorf("coverage evidence policy has conflicting implementation work declarations")
			}
			if base.ImplementationWorkPolicy == nil {
				clone := requirement.ImplementationWorkPolicy.Clone()
				base.ImplementationWorkPolicy = &clone
			}
		}
		// Profile explicit primary_state_change implies state-change evidence.
		if requirement.EvidenceRole == "primary_state_change" || len(requirement.AcceptedOperationVariants) > 0 {
			base.RequiredEvidenceKinds = runtimeSourceStableStringUnion(base.RequiredEvidenceKinds, []string{EvidenceKindStateChange})
		}
	}
	return base, nil
}

func validateRuntimeSourceAcceptedKinds(requirement RuntimeProfileEvidenceRequirement) error {
	for _, kind := range requirement.AcceptedStepKinds {
		canonical := CanonicalRuntimeBlockKind(kind)
		if canonical == "" {
			continue
		}
		if _, ok := RuntimeOperationDescriptorForKind(canonical); !ok {
			return fmt.Errorf("profile accepted step kind %q has no runtime operation descriptor", kind)
		}
	}
	for _, kind := range requirement.RejectedStepKinds {
		// Rejected kinds may include non-dispatch kinds; only validate non-empty trim.
		if strings.TrimSpace(kind) == "" {
			return fmt.Errorf("profile rejected step kind must be non-empty")
		}
	}
	return nil
}

func runtimeSourceStableStringUnion(values ...[]string) []string {
	seen := map[string]struct{}{}
	ret := []string{}
	for _, group := range values {
		for _, value := range group {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			ret = append(ret, value)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	// Keep first-seen order for authority stability.
	return ret
}

// RuntimeSourceEvidencePolicyRetainsDeclaration checks the authored contribution,
// not mere selector existence or fallback validation. This is static applicability,
// never evidence that an owner has an executable recipe, passed checks, or credit.
func RuntimeSourceEvidencePolicyRetainsDeclaration(policy RuntimeSourceEvidencePolicy, declaration RuntimeProfileEvidenceRequirement, scope, sourceID string) bool {
	if !RuntimeEvidenceDeclarationMatchesSource(declaration, scope, sourceID) || len(declaration.RequiredEvidenceKinds) == 0 || len(declaration.AcceptedStepKinds) == 0 {
		return false
	}
	// These concrete identities are deliberately discarded from wildcard templates.
	if declaration.ID == "*" && (declaration.ValidationRecipeID != "" || len(declaration.RequiredCheckRefs) > 0) {
		return false
	}
	for _, kind := range declaration.RequiredEvidenceKinds {
		if !runtimeProfileHasProofOperation(declaration.AcceptedStepKinds, kind) {
			return false
		}
	}
	for _, pair := range [][2]string{
		{declaration.EvidenceRole, policy.EvidenceRole}, {declaration.TargetRole, policy.TargetRole},
		{declaration.TargetSelectorRef, policy.TargetSelectorRef}, {declaration.TargetSelectorOwnerRef, policy.TargetSelectorOwnerRef},
		{declaration.TargetSelectorSubjectRef, policy.TargetSelectorSubjectRef}, {declaration.ValidationRecipeID, policy.ValidationRecipeID},
	} {
		if pair[0] != "" && pair[0] != pair[1] {
			return false
		}
	}
	for _, pair := range [][2][]string{
		{declaration.RequiredEvidenceKinds, policy.RequiredEvidenceKinds}, {declaration.AcceptedStepKinds, policy.AcceptedStepKinds},
		{declaration.RejectedStepKinds, policy.RejectedStepKinds}, {declaration.TargetRefs, policy.TargetRefs},
		{declaration.TargetDiscoverySources, policy.TargetDiscoverySources}, {declaration.TargetSelectorCandidateRefs, policy.TargetSelectorCandidateRefs},
		{declaration.ProspectiveTargetAllOfConstraintRefs, policy.ProspectiveTargetAllOfConstraintRefs},
		{declaration.RequiredCheckRefs, policy.RequiredCheckRefs}, {declaration.MetricConstraints, policy.MetricConstraints}, {declaration.ArtifactRefs, policy.ArtifactRefs},
	} {
		for _, value := range pair[0] {
			if !containsString(pair[1], value) {
				return false
			}
		}
	}
	if declaration.RequireTargetGrounding && !policy.RequireTargetGrounding || declaration.RequireDirectEvidence && !policy.RequireDirectEvidence ||
		declaration.RequireClaimEvidence && !policy.RequireClaimEvidence || declaration.RequireExecutableBehaviorEvidence && !policy.RequireExecutableBehaviorEvidence ||
		declaration.RequireCoverageTargets && !policy.RequireCoverageTargets {
		return false
	}
	if !SameRuntimeOperationVariantRefs(policy.AcceptedOperationVariants, append(slices.Clone(policy.AcceptedOperationVariants), declaration.AcceptedOperationVariants...)) ||
		!sameRuntimeOwnerFactRequirements(policy.RequiredOwnerFacts, append(slices.Clone(policy.RequiredOwnerFacts), declaration.RequiredOwnerFacts...)) {
		return false
	}
	for _, required := range declaration.OwnerFactConvergences {
		found := false
		for _, actual := range policy.OwnerFactConvergences {
			if SameRuntimeOwnerFactConvergences([]RuntimeOwnerFactConvergence{required}, []RuntimeOwnerFactConvergence{actual}) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if declaration.ValidationFailureRepairConvergence != nil && !SameRuntimeValidationFailureRepairConvergence(policy.ValidationFailureRepairConvergence, declaration.ValidationFailureRepairConvergence) {
		return false
	}
	if declaration.VerificationPolicy != nil && !SameRuntimeVerificationPolicy(policy.VerificationPolicy, declaration.VerificationPolicy) {
		return false
	}
	if declaration.ImplementationWorkPolicy != nil && !SameRuntimeImplementationWorkPolicy(policy.ImplementationWorkPolicy, declaration.ImplementationWorkPolicy) {
		return false
	}
	mergedRules, lineageErr := mergeRuntimeLineageAssertionRules(policy.LineageAssertions, declaration.LineageAssertions)
	if lineageErr != nil || !slices.Equal(mergedRules, policy.LineageAssertions) {
		return false
	}
	// Capability-state identities have no item-level projection in this compiler.
	return len(declaration.CapabilityStateRequirements) == 0
}

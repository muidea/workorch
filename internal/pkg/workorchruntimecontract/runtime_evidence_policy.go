package workorchruntimecontract

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

type RuntimeEvidencePolicy struct {
	ID          string
	Profile     string
	Rule        string
	AppliesTo   string
	CommandName string
	ArgsContain []string
	// RecipeArgs declares a profile validation recipe. The ordered command line
	// is sealed at activation, then must be preflight-confirmed by the Shell
	// owner before it becomes selectable by an LLM; an LLM may never reconstruct
	// command arguments or a test-name filter.
	//
	// RecipeDeclared is deliberately separate from RecipeArgs: a valid
	// executable such as `true` can have no arguments, while a policy with a
	// CommandName and no recipe declaration remains an observation policy only.
	// This distinction is part of the current activation contract.
	RecipeDeclared      bool
	RecipeArgs          []string
	RecipeTimeoutMillis int64
	// RequiredArgs is a command-argument set that a
	// Shell validation candidate must include when it selects CommandName. It
	// is a profile-owned observable-validation recipe, not an Orchestrator
	// action rule. Keeping it on the sealed evidence policy lets the candidate
	// boundary reject a command that cannot yield the observation the profile
	// requires before any side effect or validation session is started.
	RequiredArgs         []string
	ZeroWorkMarkers      []string
	WorkEvidenceMarkers  []string
	QualityOnZeroWork    string
	MarksZeroWork        bool
	EvidenceTags         []string
	EvidenceReason       string
	CoverageRequirements []RuntimeCoverageEvidenceRequirement
	Description          string
}

type RuntimeCoverageEvidenceRequirement struct {
	ID           string
	Scope        string
	EvidenceRole string
	// Target fields are intentionally repeated on an evidence-policy
	// requirement instead of being inferred from its surrounding command
	// policy.  They are declarative input compiled at activation into the
	// immutable coverage binding; a command, candidate, or receipt cannot
	// supply or widen them later.
	TargetRole                           string
	TargetRefs                           []string
	TargetDiscoverySources               []string
	TargetSelectorRef                    string
	TargetSelectorOwnerRef               string
	TargetSelectorSubjectRef             string
	TargetSelectorCandidateRefs          []string
	ProspectiveTargetAllOfConstraintRefs []string
	RequireTargetGrounding               bool
	RequiredEvidenceKinds                []string
	RequireDirectEvidence                bool
	RequireClaimEvidence                 bool
	RequireExecutableBehaviorEvidence    bool
	RequireCoverageTargets               bool
	AcceptedStepKinds                    []string
	AcceptedOperationVariants            []RuntimeOperationVariantRef
	RejectedStepKinds                    []string
	ValidationRecipeID                   string
	RequiredCheckRefs                    []string
	MetricConstraints                    []string
	ArtifactRefs                         []string
	Description                          string
}

// CloneRuntimeEvidencePolicies preserves declaration order and multiplicity
// while detaching every nested collection.
func CloneRuntimeEvidencePolicies(values []RuntimeEvidencePolicy) []RuntimeEvidencePolicy {
	if len(values) == 0 {
		return nil
	}
	ret := make([]RuntimeEvidencePolicy, 0, len(values))
	for _, value := range values {
		value.ArgsContain = append([]string(nil), value.ArgsContain...)
		value.RecipeArgs = append([]string(nil), value.RecipeArgs...)
		value.RequiredArgs = append([]string(nil), value.RequiredArgs...)
		value.ZeroWorkMarkers = append([]string(nil), value.ZeroWorkMarkers...)
		value.WorkEvidenceMarkers = append([]string(nil), value.WorkEvidenceMarkers...)
		value.EvidenceTags = append([]string(nil), value.EvidenceTags...)
		if len(value.CoverageRequirements) > 0 {
			requirements := make([]RuntimeCoverageEvidenceRequirement, 0, len(value.CoverageRequirements))
			for _, requirement := range value.CoverageRequirements {
				requirement.TargetRefs = append([]string(nil), requirement.TargetRefs...)
				requirement.TargetDiscoverySources = append([]string(nil), requirement.TargetDiscoverySources...)
				requirement.TargetSelectorCandidateRefs = append([]string(nil), requirement.TargetSelectorCandidateRefs...)
				requirement.ProspectiveTargetAllOfConstraintRefs = append([]string(nil), requirement.ProspectiveTargetAllOfConstraintRefs...)
				requirement.RequiredEvidenceKinds = append([]string(nil), requirement.RequiredEvidenceKinds...)
				requirement.AcceptedStepKinds = append([]string(nil), requirement.AcceptedStepKinds...)
				requirement.AcceptedOperationVariants = append([]RuntimeOperationVariantRef(nil), requirement.AcceptedOperationVariants...)
				requirement.RejectedStepKinds = append([]string(nil), requirement.RejectedStepKinds...)
				requirement.RequiredCheckRefs = append([]string(nil), requirement.RequiredCheckRefs...)
				requirement.MetricConstraints = append([]string(nil), requirement.MetricConstraints...)
				requirement.ArtifactRefs = append([]string(nil), requirement.ArtifactRefs...)
				requirements = append(requirements, requirement)
			}
			value.CoverageRequirements = requirements
		}
		ret = append(ret, value)
	}
	return ret
}

func (p RuntimeEvidencePolicy) Normalized() RuntimeEvidencePolicy {
	p.ID = strings.TrimSpace(p.ID)
	p.Profile = strings.TrimSpace(p.Profile)
	p.Rule = strings.TrimSpace(p.Rule)
	p.AppliesTo = strings.TrimSpace(p.AppliesTo)
	p.CommandName = strings.TrimSpace(p.CommandName)
	p.ArgsContain = normalizeStrings(p.ArgsContain)
	p.RecipeArgs = normalizeOrderedStrings(p.RecipeArgs)
	p.RequiredArgs = normalizeStrings(p.RequiredArgs)
	p.ZeroWorkMarkers = normalizeStrings(p.ZeroWorkMarkers)
	p.WorkEvidenceMarkers = normalizeStrings(p.WorkEvidenceMarkers)
	p.QualityOnZeroWork = strings.TrimSpace(p.QualityOnZeroWork)
	p.EvidenceTags = normalizeStrings(p.EvidenceTags)
	p.EvidenceReason = strings.TrimSpace(p.EvidenceReason)
	p.Description = strings.TrimSpace(p.Description)
	p.CoverageRequirements = normalizeRuntimeCoverageEvidenceRequirements(p.CoverageRequirements)
	return p
}

func (p RuntimeEvidencePolicy) Validate() error {
	p = p.Normalized()
	if p.ID == "" || p.AppliesTo == "" {
		return fmt.Errorf("runtime evidence policy requires id and applies-to kind")
	}
	if len(p.ZeroWorkMarkers) > 0 && p.QualityOnZeroWork == "" {
		return fmt.Errorf("runtime evidence policy with zero-work markers requires a quality")
	}
	if len(p.RequiredArgs) > 0 && (p.AppliesTo != StepKindShellCommand || p.CommandName == "") {
		return fmt.Errorf("runtime evidence policy required args require a shell command name")
	}
	if p.RecipeDeclared && (p.AppliesTo != StepKindShellCommand || p.CommandName == "") {
		return fmt.Errorf("runtime evidence policy validation recipe requires a shell command name")
	}
	if !p.RecipeDeclared && (len(p.RecipeArgs) > 0 || p.RecipeTimeoutMillis != 0) {
		return fmt.Errorf("runtime evidence policy recipe fields require an explicit recipe declaration")
	}
	if p.RecipeTimeoutMillis != 0 && (p.RecipeTimeoutMillis < 1000 || p.RecipeTimeoutMillis > 600000) {
		return fmt.Errorf("runtime evidence policy recipe timeout must be between 1000 and 600000")
	}
	for _, requirement := range p.CoverageRequirements {
		if requirement.ID == "" || requirement.Scope == "" {
			return fmt.Errorf("runtime coverage evidence requirement requires id and scope")
		}
		if err := ValidateRuntimeTargetGrounding(requirement.RequireTargetGrounding, requirement.TargetRole, requirement.TargetRefs, requirement.TargetDiscoverySources, requirement.TargetSelectorRef); err != nil {
			return fmt.Errorf("runtime coverage evidence requirement %q: %w", requirement.ID, err)
		}
		if len(requirement.TargetSelectorCandidateRefs) > 0 && (!requirement.RequireTargetGrounding || requirement.TargetSelectorRef == "" || requirement.TargetSelectorOwnerRef == "" || requirement.TargetSelectorSubjectRef == "") {
			return fmt.Errorf("runtime coverage evidence requirement %q candidate selector requires grounding and a complete selector", requirement.ID)
		}
		if len(requirement.ProspectiveTargetAllOfConstraintRefs) > 0 {
			if !requirement.RequireTargetGrounding || requirement.TargetSelectorRef == "" || requirement.TargetSelectorOwnerRef == "" || requirement.TargetSelectorSubjectRef == "" {
				return fmt.Errorf("runtime coverage evidence requirement %q prospective target all-of declaration requires grounding and a complete selector", requirement.ID)
			}
			if err := ValidateRuntimeProspectiveTargetAllOfConstraintRefs(requirement.ProspectiveTargetAllOfConstraintRefs); err != nil {
				return fmt.Errorf("runtime coverage evidence requirement %q: %w", requirement.ID, err)
			}
		}
	}
	return nil
}

// RuntimeValidationRecipe is an immutable profile declaration or
// Shell-owner-confirmed command recipe. It is a DTO plus pure
// normalization/validation only: selecting a Runtime action remains
// Orchestrator authority and observing/executing this recipe remains Shell
// owner authority.
type RuntimeValidationRecipe struct {
	ID            string
	Command       string
	Args          []string
	TimeoutMillis int64
}

func (r RuntimeValidationRecipe) Normalized() RuntimeValidationRecipe {
	r.ID = strings.TrimSpace(r.ID)
	r.Command = strings.TrimSpace(r.Command)
	r.Args = normalizeOrderedStrings(r.Args)
	return r
}

func (r RuntimeValidationRecipe) Validate() error {
	r = r.Normalized()
	if r.ID == "" || r.Command == "" {
		return fmt.Errorf("runtime validation recipe requires id and command")
	}
	if r.TimeoutMillis != 0 && (r.TimeoutMillis < 1000 || r.TimeoutMillis > 600000) {
		return fmt.Errorf("runtime validation recipe timeout must be between 1000 and 600000")
	}
	return nil
}

// RuntimeValidationRecipeDeclarationsFromEvidencePolicies derives the profile
// declarations that the Orchestrator submits to the Shell owner for preflight.
// The result is not executable authority and must not be exposed to a provider
// or used by Shell execution until an admissibility snapshot confirms it.
func RuntimeValidationRecipeDeclarationsFromEvidencePolicies(policies []RuntimeEvidencePolicy) []RuntimeValidationRecipe {
	ret := make([]RuntimeValidationRecipe, 0, len(policies))
	for _, policy := range NormalizeRuntimeEvidencePolicies(policies) {
		if !policy.RecipeDeclared || policy.AppliesTo != StepKindShellCommand || policy.CommandName == "" {
			continue
		}
		recipe := RuntimeValidationRecipe{ID: policy.ID, Command: policy.CommandName, Args: append([]string(nil), policy.RecipeArgs...), TimeoutMillis: policy.RecipeTimeoutMillis}.Normalized()
		if recipe.Validate() == nil {
			ret = append(ret, recipe)
		}
	}
	return NormalizeRuntimeValidationRecipes(ret)
}

// RuntimeValidationRecipeForID selects one already-sealed recipe by stable
// identity. It is intentionally a pure lookup so a capability owner can use
// a candidate manifest's recipe set without reinterpreting profile policy.
func RuntimeValidationRecipeForID(recipes []RuntimeValidationRecipe, id string) (RuntimeValidationRecipe, bool) {
	id = strings.TrimSpace(id)
	for _, recipe := range NormalizeRuntimeValidationRecipes(recipes) {
		if recipe.ID == id {
			return recipe, true
		}
	}
	return RuntimeValidationRecipe{}, false
}

// NormalizeRuntimeValidationRecipes keeps the sealed recipe set deterministic
// without changing command argument order. It deliberately accepts only valid
// recipes: an invalid profile declaration cannot accidentally become a valid
// Shell preflight input or candidate-manifest authority.
func NormalizeRuntimeValidationRecipes(values []RuntimeValidationRecipe) []RuntimeValidationRecipe {
	byID := make(map[string]RuntimeValidationRecipe, len(values))
	for _, value := range values {
		value = value.Normalized()
		if value.Validate() != nil {
			continue
		}
		byID[value.ID] = value
	}
	keys := make([]string, 0, len(byID))
	for id := range byID {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	ret := make([]RuntimeValidationRecipe, 0, len(keys))
	for _, id := range keys {
		ret = append(ret, byID[id])
	}
	return nilIfEmpty(ret)
}

// ValidateRuntimeValidationRecipeProjection verifies the canonical recipe
// values in an immutable admission or dispatch projection without deciding
// which recipes a candidate is allowed to select. Candidate membership is a
// separate SM-3 admission check.
func ValidateRuntimeValidationRecipeProjection(values []RuntimeValidationRecipe) error {
	seenRecipeIDs := make(map[string]struct{}, len(values))
	for _, raw := range values {
		if !reflect.DeepEqual(raw, raw.Normalized()) {
			return fmt.Errorf("validation recipe projection requires canonical raw recipe fields")
		}
		if err := raw.Validate(); err != nil {
			return fmt.Errorf("validation recipe projection: %w", err)
		}
		recipe := raw.Normalized()
		if _, duplicate := seenRecipeIDs[recipe.ID]; duplicate {
			return fmt.Errorf("validation recipe projection duplicates recipe %q", recipe.ID)
		}
		seenRecipeIDs[recipe.ID] = struct{}{}
	}
	return nil
}

// RuntimeValidationRecipeProjectionsEqual compares complete immutable recipe
// bodies while treating their collection as an ID-keyed projection. Argument
// order within each recipe remains significant.
func RuntimeValidationRecipeProjectionsEqual(left, right []RuntimeValidationRecipe) bool {
	return reflect.DeepEqual(NormalizeRuntimeValidationRecipes(left), NormalizeRuntimeValidationRecipes(right))
}

// ValidateRuntimeCandidateManagedValidationRecipeSnapshot verifies that a
// persisted dispatch snapshot contains one valid recipe for every managed
// validation operation and no unselected recipe.  It intentionally does not
// compare against activation policy: membership against the manifest is
// checked when the snapshot is sealed.
func ValidateRuntimeCandidateManagedValidationRecipeSnapshot(candidate RuntimeCandidate, values []RuntimeValidationRecipe) error {
	if err := ValidateRuntimeValidationRecipeProjection(values); err != nil {
		return fmt.Errorf("managed validation recipe snapshot: %w", err)
	}
	selected := make(map[string]struct{})
	collect := func(operations []RuntimeOperation) error {
		for _, operation := range operations {
			operation = operation.Normalized()
			if operation.Kind != RuntimeBlockKindManagedValidationSession {
				continue
			}
			id := runtimeValidationRecipeID(operation)
			if id == "" {
				return fmt.Errorf("managed validation operation %q has no recipeId", operation.ID)
			}
			selected[id] = struct{}{}
		}
		return nil
	}
	if candidate.StateChange != nil {
		operations := make([]RuntimeOperation, 0, len(candidate.StateChange.Operations))
		for _, operation := range candidate.StateChange.Operations {
			operations = append(operations, operation.Operation)
		}
		if err := collect(operations); err != nil {
			return err
		}
	}
	if candidate.Validation != nil {
		operations := make([]RuntimeOperation, 0, len(candidate.Validation.Operations))
		for _, operation := range candidate.Validation.Operations {
			operations = append(operations, operation.Operation)
		}
		if err := collect(operations); err != nil {
			return err
		}
	}
	values = NormalizeRuntimeValidationRecipes(values)
	if len(values) != len(selected) {
		return fmt.Errorf("managed validation recipe snapshot does not match selected operation count")
	}
	for _, recipe := range values {
		if _, found := selected[recipe.ID]; !found {
			return fmt.Errorf("managed validation recipe snapshot contains unselected recipe %q", recipe.ID)
		}
	}
	for id := range selected {
		if _, found := RuntimeValidationRecipeForID(values, id); !found {
			return fmt.Errorf("managed validation recipe snapshot omits selected recipe %q", id)
		}
	}
	return nil
}

func CloneRuntimeValidationRecipes(values []RuntimeValidationRecipe) []RuntimeValidationRecipe {
	if values == nil {
		return nil
	}
	ret := make([]RuntimeValidationRecipe, len(values))
	for idx := range values {
		ret[idx] = values[idx]
		ret[idx].Args = append([]string(nil), values[idx].Args...)
	}
	return ret
}

func NormalizeRuntimeEvidencePolicies(values []RuntimeEvidencePolicy) []RuntimeEvidencePolicy {
	byID := map[string]RuntimeEvidencePolicy{}
	for _, value := range values {
		value = value.Normalized()
		if value.ID != "" {
			byID[value.ID] = value
		}
	}
	keys := make([]string, 0, len(byID))
	for key := range byID {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimeEvidencePolicy, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byID[key])
	}
	return nilIfEmpty(ret)
}

func ValidateRuntimeEvidencePolicies(values []RuntimeEvidencePolicy) error {
	for _, value := range NormalizeRuntimeEvidencePolicies(values) {
		if err := value.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func SameRuntimeEvidencePolicies(left []RuntimeEvidencePolicy, right []RuntimeEvidencePolicy) bool {
	left = NormalizeRuntimeEvidencePolicies(left)
	right = NormalizeRuntimeEvidencePolicies(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if !sameRuntimeEvidencePolicy(left[idx], right[idx]) {
			return false
		}
	}
	return true
}

func normalizeRuntimeCoverageEvidenceRequirements(values []RuntimeCoverageEvidenceRequirement) []RuntimeCoverageEvidenceRequirement {
	byKey := map[string]RuntimeCoverageEvidenceRequirement{}
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
		value.AcceptedOperationVariants = NormalizeRuntimeOperationVariantRefs(value.AcceptedOperationVariants)
		value.RejectedStepKinds = normalizeStrings(value.RejectedStepKinds)
		value.ValidationRecipeID = strings.TrimSpace(value.ValidationRecipeID)
		value.RequiredCheckRefs = normalizeStrings(value.RequiredCheckRefs)
		value.MetricConstraints = normalizeStrings(value.MetricConstraints)
		value.ArtifactRefs = normalizeStrings(value.ArtifactRefs)
		value.Description = strings.TrimSpace(value.Description)
		if value.ID != "" && value.Scope != "" {
			byKey[value.Scope+"|"+value.ID] = value
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ret := make([]RuntimeCoverageEvidenceRequirement, 0, len(keys))
	for _, key := range keys {
		ret = append(ret, byKey[key])
	}
	return nilIfEmpty(ret)
}

func sameRuntimeEvidencePolicy(left RuntimeEvidencePolicy, right RuntimeEvidencePolicy) bool {
	left = left.Normalized()
	right = right.Normalized()
	if left.ID != right.ID || left.Profile != right.Profile || left.Rule != right.Rule || left.AppliesTo != right.AppliesTo ||
		left.CommandName != right.CommandName || left.RecipeTimeoutMillis != right.RecipeTimeoutMillis || left.QualityOnZeroWork != right.QualityOnZeroWork || left.MarksZeroWork != right.MarksZeroWork ||
		left.EvidenceReason != right.EvidenceReason || left.Description != right.Description ||
		left.RecipeDeclared != right.RecipeDeclared ||
		!sameStrings(left.ArgsContain, right.ArgsContain) || !sameStrings(left.RecipeArgs, right.RecipeArgs) || !sameStrings(left.RequiredArgs, right.RequiredArgs) || !sameStrings(left.ZeroWorkMarkers, right.ZeroWorkMarkers) ||
		!sameStrings(left.WorkEvidenceMarkers, right.WorkEvidenceMarkers) || !sameStrings(left.EvidenceTags, right.EvidenceTags) ||
		len(left.CoverageRequirements) != len(right.CoverageRequirements) {
		return false
	}
	for idx := range left.CoverageRequirements {
		l, r := left.CoverageRequirements[idx], right.CoverageRequirements[idx]
		if l.ID != r.ID || l.Scope != r.Scope || l.EvidenceRole != r.EvidenceRole || l.TargetRole != r.TargetRole || l.TargetSelectorRef != r.TargetSelectorRef ||
			l.TargetSelectorOwnerRef != r.TargetSelectorOwnerRef || l.TargetSelectorSubjectRef != r.TargetSelectorSubjectRef ||
			l.RequireTargetGrounding != r.RequireTargetGrounding || l.RequireDirectEvidence != r.RequireDirectEvidence || l.RequireClaimEvidence != r.RequireClaimEvidence ||
			l.RequireExecutableBehaviorEvidence != r.RequireExecutableBehaviorEvidence || l.RequireCoverageTargets != r.RequireCoverageTargets || l.ValidationRecipeID != r.ValidationRecipeID || l.Description != r.Description ||
			!sameStrings(l.TargetRefs, r.TargetRefs) || !sameStrings(l.TargetDiscoverySources, r.TargetDiscoverySources) || !sameStrings(l.TargetSelectorCandidateRefs, r.TargetSelectorCandidateRefs) || !SameRuntimeProspectiveTargetAllOfConstraintRefs(l.ProspectiveTargetAllOfConstraintRefs, r.ProspectiveTargetAllOfConstraintRefs) ||
			!sameStrings(l.RequiredEvidenceKinds, r.RequiredEvidenceKinds) || !sameStrings(l.AcceptedStepKinds, r.AcceptedStepKinds) || !SameRuntimeOperationVariantRefs(l.AcceptedOperationVariants, r.AcceptedOperationVariants) ||
			!sameStrings(l.RequiredCheckRefs, r.RequiredCheckRefs) || !sameStrings(l.MetricConstraints, r.MetricConstraints) || !sameStrings(l.ArtifactRefs, r.ArtifactRefs) || !sameStrings(l.RejectedStepKinds, r.RejectedStepKinds) {
			return false
		}
	}
	return true
}

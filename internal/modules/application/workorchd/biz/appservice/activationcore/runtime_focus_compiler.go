package activationcore

import (
	"fmt"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type compiledCoverage struct {
	Focuses           []runtimecontract.CoverageFocus
	DefinitionFocuses []runtimecontract.RuntimeDefinitionFocus
	Slices            []runtimecontract.RuntimeAcceptanceSlice
	CoverageItems     []runtimecontract.RuntimeCoverageItem
}

type workUnit struct {
	sourceScope       string
	sourceID          string
	text              string
	evidenceRole      string
	bootstrapProfile  string
	bootstrapRecipe   string
	bootstrapArtifact *runtimecontract.RuntimeBootstrapArtifact
	order             int
	itemID            common.CoverageWorkItemID
}

type resolvedCoverageItem struct {
	unit   workUnit
	itemID common.CoverageWorkItemID
	policy compiledEvidencePolicy
}

type coverageSliceBucket struct {
	group runtimecontract.RuntimeAcceptanceSliceGroupPolicy
	items []resolvedCoverageItem
}

// compiledEvidencePolicy is the neutral merge result for one coverage focus.
// Description is audit-only and never enters reducer authority.
type compiledEvidencePolicy struct {
	RequiredEvidenceKinds                []string
	AcceptedStepKinds                    []string
	AcceptedOperationVariants            []runtimecontract.RuntimeOperationVariantRef
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
	RequiredOwnerFacts                   []runtimecontract.RuntimeOwnerFactRequirement
	OwnerFactConvergences                []runtimecontract.RuntimeOwnerFactConvergence
	ValidationFailureRepairConvergence   *runtimecontract.RuntimeValidationFailureRepairConvergence
	ImplementationWorkPolicy             *runtimecontract.RuntimeImplementationWorkPolicy
	VerificationPolicy                   *runtimecontract.RuntimeVerificationPolicy
	ValidationRecipeID                   string
	RequiredCheckRefs                    []string
	MetricConstraints                    []string
	ArtifactRefs                         []string
	DeclarationRef                       string
	SemanticProofDeclarationRef          string
	Description                          string
}

// compileRuntimeCoverageFocuses builds the deterministic focus queue for one activation.
// Evidence policy precedence is exact(scope,id) > wildcard(scope,*) > scope default > neutral default.
func compileRuntimeCoverageFocuses(goal *workspacemodel.Goal, dod *workspacemodel.DoD, activation runtimecontract.RuntimeActivationContext) ([]runtimecontract.CoverageFocus, error) {
	compiled, err := compileRuntimeCoverage(goal, dod, activation)
	if err != nil {
		return nil, err
	}
	return compiled.Focuses, nil
}

// compileRuntimeCoverage builds focuses, acceptance slices and item-level coverage identities.
func compileRuntimeCoverage(goal *workspacemodel.Goal, dod *workspacemodel.DoD, activation runtimecontract.RuntimeActivationContext) (compiledCoverage, error) {
	if goal == nil || dod == nil {
		return compiledCoverage{}, common.ErrInvalidArgument
	}
	activation = activation.Normalized()
	if err := runtimecontract.ValidateRuntimeProfileContracts(activation.ExecutionContract.ProfileContracts); err != nil {
		return compiledCoverage{}, fmt.Errorf("runtime profile proof closure: %w", err)
	}
	units, err := compileRuntimeCoverageWorkUnits(goal, dod, activation.ExecutionContract.ProfileContracts)
	if err != nil {
		return compiledCoverage{}, err
	}
	allUnits := append([]workUnit(nil), units...)
	resolved, err := resolveRuntimeCoverageItems(units, activation)
	if err != nil {
		return compiledCoverage{}, err
	}
	slicePolicy, groups, err := runtimeAcceptanceSlicePlan(activation, resolved)
	if err != nil {
		return compiledCoverage{}, err
	}
	buckets, err := assignRuntimeCoverageBuckets(resolved, groups)
	if err != nil {
		return compiledCoverage{}, err
	}
	compiled, err := compileRuntimeCoverageBuckets(buckets, slicePolicy)
	if err != nil {
		return compiledCoverage{}, err
	}
	return finalizeRuntimeCoverage(compiled, allUnits, activation.ExecutionContract.ProfileContracts, slicePolicy.MaxFocuses)
}

func compileRuntimeCoverageWorkUnits(goal *workspacemodel.Goal, dod *workspacemodel.DoD, profiles []runtimecontract.RuntimeProfileContract) ([]workUnit, error) {
	// Profile-owned bootstrap artifact roles are compiled before ordinary
	// definition units. Runtime receives sealed role/operation/dependency
	// declarations, while the post-seal semantic proposal chooses a concrete
	// target within owner-published selector constraints.
	units, err := compileRuntimeBootstrapWorkUnits(goal, dod, profiles)
	if err != nil {
		return nil, err
	}
	for idx, item := range dod.Items {
		if item = strings.TrimSpace(item); item != "" {
			units = append(units, workUnit{sourceScope: "dodItem", sourceID: fmt.Sprintf("dod-%03d", idx+1), text: item})
		}
	}
	units = appendRuntimeProfileWorkUnits(units, profiles)
	for idx := range units {
		units[idx].order = idx + 1
		units[idx].itemID = common.CoverageWorkItemID(fmt.Sprintf("cwi-%03d", idx+1))
	}
	if len(units) == 0 {
		return nil, fmt.Errorf("runtime coverage focus compiler produced no coverage items")
	}
	return units, nil
}

func appendRuntimeProfileWorkUnits(units []workUnit, profiles []runtimecontract.RuntimeProfileContract) []workUnit {
	for _, profile := range runtimecontract.NormalizeRuntimeProfileContracts(profiles) {
		for _, dimension := range profile.AcceptanceDimensions {
			if dimension.Required && strings.TrimSpace(dimension.ID) != "" {
				units = append(units, workUnit{sourceScope: "acceptanceDimension", sourceID: dimension.ID, text: dimension.Description})
			}
		}
		units = appendRuntimeDeliverableWorkUnits(units, profile.DeliverableExpectations)
	}
	return units
}

func appendRuntimeDeliverableWorkUnits(units []workUnit, deliverables []runtimecontract.RuntimeDeliverableExpectation) []workUnit {
	for _, evidenceRole := range []string{"primary_state_change", "supplemental_evidence", ""} {
		for _, deliverable := range deliverables {
			if deliverable.Required && deliverable.EvidenceRole == evidenceRole && strings.TrimSpace(deliverable.ID) != "" {
				units = append(units, workUnit{
					sourceScope: "deliverable", sourceID: deliverable.ID, text: deliverable.Description, evidenceRole: deliverable.EvidenceRole,
				})
			}
		}
	}
	return units
}

func resolveRuntimeCoverageItems(units []workUnit, activation runtimecontract.RuntimeActivationContext) ([]resolvedCoverageItem, error) {
	stateChangeRequired := activation.ExecutionContract.StateChangePolicy == runtimecontract.RuntimeStateChangeMustProduce
	resolved := make([]resolvedCoverageItem, 0, len(units))
	for _, unit := range units {
		policy, err := resolveRuntimeCoverageItemPolicy(unit, activation, stateChangeRequired)
		if err != nil {
			return nil, err
		}
		if unit.evidenceRole != "" {
			policy.EvidenceRole = unit.evidenceRole
		}
		// The work unit is now an admitted exact Definition item. Assign its
		// canonical declaration here so profile wildcards and bootstrap recipes
		// can never escape into Runtime as proof authority.
		policy.SemanticProofDeclarationRef = runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef(unit.sourceScope, unit.sourceID)
		itemID := unit.itemID
		if itemID == "" {
			itemID = common.CoverageWorkItemID(fmt.Sprintf("cwi-%03d", unit.order))
		}
		resolved = append(resolved, resolvedCoverageItem{unit: unit, itemID: itemID, policy: policy})
	}
	return resolved, nil
}

func resolveRuntimeCoverageItemPolicy(unit workUnit, activation runtimecontract.RuntimeActivationContext, stateChangeRequired bool) (compiledEvidencePolicy, error) {
	if unit.bootstrapArtifact != nil {
		return compileRuntimeBootstrapArtifactPolicy(unit)
	}
	return resolveCoverageEvidencePolicy(unit.sourceScope, unit.sourceID, activation, stateChangeRequired)
}

func runtimeAcceptanceSlicePlan(activation runtimecontract.RuntimeActivationContext, resolved []resolvedCoverageItem) (runtimecontract.RuntimeAcceptanceSlicePolicy, []runtimecontract.RuntimeAcceptanceSliceGroupPolicy, error) {
	policy := runtimecontract.RuntimeAcceptanceSlicePolicy{
		SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent,
		MaxWorkItems:  runtimecontract.RuntimeAcceptanceSliceDefaultMaxItems, MaxFocuses: runtimecontract.RuntimeAcceptanceFocusMaxCount,
		MaxCandidateContextItems: runtimecontract.RuntimeCandidateContextItemsMaxCount,
		CompletionPolicy:         runtimecontract.RuntimeAcceptanceSliceCompletionAll,
		ResultPolicy:             runtimecontract.RuntimeAcceptanceSliceResultAllowed,
	}
	if activation.ExecutionContract.AcceptanceSlicePolicy != nil {
		policy = activation.ExecutionContract.AcceptanceSlicePolicy.Normalized()
	}
	if err := policy.Validate(); err != nil {
		return runtimecontract.RuntimeAcceptanceSlicePolicy{}, nil, err
	}
	groups := policy.Groups
	if len(groups) == 0 {
		groups = []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{
			ID: "slice-default", Title: "Default acceptance", Scope: "default", Priority: 100,
			MaxWorkItems: policy.MaxWorkItems, Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true},
		}}
	}
	if runtimeCoverageHasBootstrapArtifact(resolved) {
		groups = append([]runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{
			ID: "slice-bootstrap-artifact", Title: "Profile bootstrap artifacts", Scope: "bootstrap_artifact", Priority: -1,
			MaxWorkItems: 1, Match: runtimecontract.RuntimeAcceptanceSliceMatch{SourceScopes: []string{"bootstrapArtifact"}},
		}}, groups...)
	}
	return policy, groups, nil
}

func runtimeCoverageHasBootstrapArtifact(items []resolvedCoverageItem) bool {
	for _, item := range items {
		if item.unit.bootstrapArtifact != nil {
			return true
		}
	}
	return false
}

func assignRuntimeCoverageBuckets(resolved []resolvedCoverageItem, groups []runtimecontract.RuntimeAcceptanceSliceGroupPolicy) ([]coverageSliceBucket, error) {
	assigned := make([]bool, len(resolved))
	buckets := make([]coverageSliceBucket, 0, len(groups))
	for _, group := range groups {
		bucket := coverageSliceBucket{group: group}
		for idx, item := range resolved {
			if !assigned[idx] && matchAcceptanceSliceGroup(group, item.unit.sourceScope, item.unit.sourceID, item.policy) {
				assigned[idx] = true
				bucket.items = append(bucket.items, item)
			}
		}
		if len(bucket.items) != 0 {
			buckets = append(buckets, bucket)
		}
	}
	for idx, ok := range assigned {
		if !ok {
			return nil, fmt.Errorf("coverage item %s/%s matched no acceptance slice group", resolved[idx].unit.sourceScope, resolved[idx].unit.sourceID)
		}
	}
	return buckets, nil
}

func compileRuntimeCoverageBuckets(buckets []coverageSliceBucket, policy runtimecontract.RuntimeAcceptanceSlicePolicy) (compiledCoverage, error) {
	compiled := compiledCoverage{}
	focusSeq := 0
	for _, bucket := range buckets {
		maxItems := policy.EffectiveMaxWorkItems(bucket.group)
		for start, ordinal := 0, 1; start < len(bucket.items); start, ordinal = start+maxItems, ordinal+1 {
			end := min(start+maxItems, len(bucket.items))
			var err error
			focusSeq, err = appendRuntimeCoverageChunk(&compiled, bucket, bucket.items[start:end], policy, ordinal, focusSeq)
			if err != nil {
				return compiledCoverage{}, err
			}
		}
	}
	return compiled, nil
}

func appendRuntimeCoverageChunk(compiled *compiledCoverage, bucket coverageSliceBucket, items []resolvedCoverageItem, policy runtimecontract.RuntimeAcceptanceSlicePolicy, ordinal, focusSeq int) (int, error) {
	sliceID := fmt.Sprintf("%s-%03d", bucket.group.ID, ordinal)
	ids := make([]common.CoverageWorkItemID, 0, len(items))
	requiredKinds := []string{}
	for _, item := range items {
		ids = append(ids, item.itemID)
		requiredKinds = stableStringUnion(requiredKinds, item.policy.RequiredEvidenceKinds)
		compiled.CoverageItems = append(compiled.CoverageItems, runtimecontract.RuntimeCoverageItem{
			ID: item.itemID, SliceID: sliceID, SourceScope: item.unit.sourceScope, SourceID: item.unit.sourceID,
			TargetText: item.unit.text, Order: item.unit.order,
		}.Normalized())
	}
	compiled.Slices = append(compiled.Slices, runtimecontract.RuntimeAcceptanceSlice{
		ID: sliceID, Title: bucket.group.Title, Scope: bucket.group.Scope, Priority: bucket.group.Priority,
		CoverageWorkItemIDs: ids, RequiredEvidenceKinds: requiredKinds,
		CompletionPolicy: policy.CompletionPolicy, ResultPolicy: policy.ResultPolicy,
	}.Normalized())
	for _, item := range items {
		focusSeq++
		focus := newCompiledCoverageFocus(fmt.Sprintf("focus-%03d", focusSeq), sliceID, bucket.group.Scope, item)
		if err := runtimecontract.ValidateRuntimeTargetGrounding(focus.RequireTargetGrounding, focus.TargetRole, focus.TargetRefs, focus.TargetDiscoverySources, focus.TargetSelectorRef); err != nil {
			return focusSeq, fmt.Errorf("compiled focus %s: %w", focus.ID, err)
		}
		compiled.Focuses = append(compiled.Focuses, focus)
		definitionFocus, err := newCompiledRuntimeDefinitionFocus(focus, item.policy)
		if err != nil {
			return focusSeq, err
		}
		compiled.DefinitionFocuses = append(compiled.DefinitionFocuses, definitionFocus)
	}
	return focusSeq, nil
}

func finalizeRuntimeCoverage(compiled compiledCoverage, allUnits []workUnit, profiles []runtimecontract.RuntimeProfileContract, maxFocuses int) (compiledCoverage, error) {
	coverageItems, err := compileRuntimeCoverageDependencies(compiled.CoverageItems, runtimeCoverageItemsFromWorkUnits(allUnits), profiles)
	if err != nil {
		return compiledCoverage{}, err
	}
	if err := runtimecontract.ValidateRuntimeCoverageItemDependencies(coverageItems); err != nil {
		return compiledCoverage{}, fmt.Errorf("compiled coverage dependencies: %w", err)
	}
	if len(compiled.Focuses) == 0 {
		return compiledCoverage{}, fmt.Errorf("runtime coverage focus compiler produced no focuses")
	}
	if len(compiled.DefinitionFocuses) != len(compiled.Focuses) {
		return compiledCoverage{}, fmt.Errorf("runtime coverage compiler produced mismatched mutable focus and immutable Definition declaration cardinality")
	}
	if len(compiled.Focuses) > maxFocuses {
		return compiledCoverage{}, fmt.Errorf("runtime coverage focus compiler produced %d focuses, exceeding sealed policy limit %d; configure bounded acceptance slices", len(compiled.Focuses), maxFocuses)
	}
	compiled.CoverageItems = coverageItems
	if err := runtimecontract.ValidateRuntimeDefinitionImplementationWorkPolicies(compiled.DefinitionFocuses, profiles); err != nil {
		return compiledCoverage{}, err
	}
	if err := runtimecontract.ValidateRuntimeDefinitionVerificationPolicies(compiled.DefinitionFocuses, profiles); err != nil {
		return compiledCoverage{}, err
	}
	return compiled, nil
}

func newCompiledCoverageFocus(id, sliceID, groupScope string, item resolvedCoverageItem) runtimecontract.CoverageFocus {
	policy := item.policy
	assessmentScope, assessmentID := groupScope, sliceID
	if item.unit.sourceScope != "" {
		assessmentScope, assessmentID = item.unit.sourceScope, item.unit.sourceID
	}
	return runtimecontract.CoverageFocus{
		ID: id, SliceID: sliceID,
		AssessmentScope: assessmentScope, AssessmentID: assessmentID, AssessmentText: item.unit.text,
		CoverageWorkItemIDs: []common.CoverageWorkItemID{item.itemID}, RequiredEvidenceKinds: policy.RequiredEvidenceKinds,
		EvidenceRole: policy.EvidenceRole, TargetRole: policy.TargetRole, TargetRefs: policy.TargetRefs,
		TargetDiscoverySources: policy.TargetDiscoverySources, TargetSelectorRef: policy.TargetSelectorRef,
		TargetSelectorOwnerRef: policy.TargetSelectorOwnerRef, TargetSelectorSubjectRef: policy.TargetSelectorSubjectRef,
		TargetSelectorCandidateRefs:          policy.TargetSelectorCandidateRefs,
		ProspectiveTargetAllOfConstraintRefs: policy.ProspectiveTargetAllOfConstraintRefs,
		RequireTargetGrounding:               policy.RequireTargetGrounding, AcceptedStepKinds: policy.AcceptedStepKinds,
		AcceptedOperationVariants: policy.AcceptedOperationVariants, RejectedStepKinds: policy.RejectedStepKinds,
		RequireDirectEvidence: policy.RequireDirectEvidence, RequireClaimEvidence: policy.RequireClaimEvidence,
		RequireExecutableBehaviorEvidence: policy.RequireExecutableBehaviorEvidence, RequireCoverageTargets: policy.RequireCoverageTargets,
		RequireAssessment: policy.RequireAssessment, RequiredOwnerFacts: policy.RequiredOwnerFacts, OwnerFactConvergences: policy.OwnerFactConvergences, ValidationRecipeID: policy.ValidationRecipeID,
		RequiredCheckRefs: policy.RequiredCheckRefs, MetricConstraints: policy.MetricConstraints, ArtifactRefs: policy.ArtifactRefs,
		EvidenceDeclarationRef: policy.SemanticProofDeclarationRef,
	}.Normalized()
}

// newCompiledRuntimeDefinitionFocus transfers Profile-owned declarations onto
// the immutable activation surface while the mutable CoverageFocus remains a
// generation cursor. In particular, validation-failure repair convergence is
// never copied into mutable focus state.
func newCompiledRuntimeDefinitionFocus(focus runtimecontract.CoverageFocus, policy compiledEvidencePolicy) (runtimecontract.RuntimeDefinitionFocus, error) {
	compiled := runtimecontract.CompileRuntimeDefinitionFocuses([]runtimecontract.CoverageFocus{focus})
	if len(compiled) != 1 {
		return runtimecontract.RuntimeDefinitionFocus{}, fmt.Errorf("runtime Definition focus compiler requires exactly one focus")
	}
	definition := compiled[0]
	if policy.ValidationFailureRepairConvergence != nil {
		value := policy.ValidationFailureRepairConvergence.Clone()
		definition.ValidationFailureRepairConvergence = &value
	}
	if policy.ImplementationWorkPolicy != nil {
		value := policy.ImplementationWorkPolicy.Clone()
		definition.ImplementationWorkPolicy = &value
	}
	if policy.VerificationPolicy != nil {
		definition.VerificationPolicy = policy.VerificationPolicy
	}
	return definition.Normalized(), nil
}

// compileRuntimeBootstrapWorkUnits resolves a declarative profile recipe using
// only activation inputs. A recipe is either exactly selected or not selected;
// ambiguous profile declarations are rejected before a Run can seal an
// arbitrary bootstrap graph. Concrete targets remain post-seal semantic
// proposals. No filesystem or capability-owner access occurs here, preserving
// File as the owner of path/manifest observations.
func compileRuntimeBootstrapWorkUnits(goal *workspacemodel.Goal, dod *workspacemodel.DoD, profiles []runtimecontract.RuntimeProfileContract) ([]workUnit, error) {
	goalText := ""
	if goal != nil {
		goalText = goal.Text
	}
	dodText := ""
	if dod != nil {
		dodText = strings.Join(dod.Items, "\n")
	}
	type selectedRecipe struct {
		profileID string
		recipe    runtimecontract.RuntimeBootstrapRecipe
	}
	selected := []selectedRecipe{}
	for _, profile := range runtimecontract.NormalizeRuntimeProfileContracts(profiles) {
		for _, recipe := range profile.BootstrapRecipes {
			if runtimeBootstrapRecipeMatches(recipe, goalText, dodText) {
				selected = append(selected, selectedRecipe{profileID: profile.ID, recipe: recipe})
			}
		}
	}
	if len(selected) == 0 {
		return nil, nil
	}
	if len(selected) != 1 {
		ids := make([]string, 0, len(selected))
		for _, item := range selected {
			ids = append(ids, item.profileID+"/"+item.recipe.ID)
		}
		return nil, fmt.Errorf("runtime bootstrap recipe selection is ambiguous: %s", strings.Join(ids, ","))
	}
	artifacts, err := selected[0].recipe.OrderedArtifacts()
	if err != nil {
		return nil, err
	}
	units := make([]workUnit, 0, len(artifacts))
	for _, artifact := range artifacts {
		artifact := artifact
		units = append(units, workUnit{
			sourceScope: "bootstrapArtifact", sourceID: selected[0].recipe.ID + "/" + artifact.ID,
			text: "profile bootstrap artifact " + artifact.ID, evidenceRole: "bootstrap_artifact",
			bootstrapProfile: selected[0].profileID, bootstrapRecipe: selected[0].recipe.ID, bootstrapArtifact: &artifact,
		})
	}
	return units, nil
}

func runtimeBootstrapRecipeMatches(recipe runtimecontract.RuntimeBootstrapRecipe, goalText, dodText string) bool {
	recipe = recipe.Normalized()
	return runtimeBootstrapTextContainsAll(goalText, recipe.RequiredGoalTerms) && runtimeBootstrapTextContainsAll(dodText, recipe.RequiredDoDTerms)
}

func runtimeBootstrapTextContainsAll(text string, terms []string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	for _, term := range terms {
		if !strings.Contains(text, strings.ToLower(strings.TrimSpace(term))) {
			return false
		}
	}
	return true
}

func compileRuntimeBootstrapArtifactPolicy(unit workUnit) (compiledEvidencePolicy, error) {
	if unit.bootstrapArtifact == nil || strings.TrimSpace(unit.bootstrapProfile) == "" || strings.TrimSpace(unit.bootstrapRecipe) == "" {
		return compiledEvidencePolicy{}, fmt.Errorf("bootstrap work unit requires profile, recipe and artifact")
	}
	artifact := unit.bootstrapArtifact.Normalized()
	if artifact.ID == "" || artifact.TargetRole == "" || len(artifact.AllowedOperationKinds) == 0 {
		return compiledEvidencePolicy{}, fmt.Errorf("bootstrap artifact work unit is incomplete")
	}
	return compiledEvidencePolicy{
		RequiredEvidenceKinds:    []string{runtimecontract.EvidenceKindStateChange},
		AcceptedStepKinds:        append([]string(nil), artifact.AllowedOperationKinds...),
		EvidenceRole:             "bootstrap_artifact",
		TargetRole:               artifact.TargetRole,
		TargetDiscoverySources:   []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest, runtimecontract.RuntimeValidationDiscoverySourceProjectFiles},
		TargetSelectorRef:        "file/workspace_deliverable",
		TargetSelectorOwnerRef:   unit.bootstrapProfile,
		TargetSelectorSubjectRef: unit.bootstrapRecipe + "/" + artifact.ID,
		RequireTargetGrounding:   true,
		RequireDirectEvidence:    true,
		RequireCoverageTargets:   true,
		Description:              "profile-declared workspace bootstrap artifact",
	}, nil
}

func matchAcceptanceSliceGroup(group runtimecontract.RuntimeAcceptanceSliceGroupPolicy, sourceScope, sourceID string, policy compiledEvidencePolicy) bool {
	match := group.Match
	if match.All {
		return true
	}
	// Different non-empty fields are AND; values inside one array are OR.
	checks := 0
	ok := true
	if len(match.EvidenceRoles) > 0 {
		checks++
		found := false
		for _, role := range match.EvidenceRoles {
			if role == policy.EvidenceRole {
				found = true
				break
			}
		}
		ok = ok && found
	}
	if len(match.MissingEvidenceKinds) > 0 {
		checks++
		found := false
		for _, kind := range match.MissingEvidenceKinds {
			for _, required := range policy.RequiredEvidenceKinds {
				if kind == required {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		ok = ok && found
	}
	if len(match.SourceRequirementIDs) > 0 {
		checks++
		found := false
		for _, id := range match.SourceRequirementIDs {
			if id == sourceID {
				found = true
				break
			}
		}
		ok = ok && found
	}
	if len(match.SourceScopes) > 0 {
		checks++
		found := false
		for _, scope := range match.SourceScopes {
			if scope == sourceScope || normalizeEvidenceScope(scope) == sourceScope {
				found = true
				break
			}
		}
		ok = ok && found
	}
	if checks == 0 {
		return false
	}
	return ok
}

// compileRuntimeCoverageDependencies resolves profile-owned opaque source
// references into the stable coverage IDs persisted in a RuntimeBootstrap.
// It is activation-only compilation: the helper does not infer domain
// relationships, alter focus state, or access workspace/capability owners.
func compileRuntimeCoverageDependencies(items []runtimecontract.RuntimeCoverageItem, declaredItems []runtimecontract.RuntimeCoverageItem, profiles []runtimecontract.RuntimeProfileContract) ([]runtimecontract.RuntimeCoverageItem, error) {
	items = runtimecontract.NormalizeRuntimeCoverageItems(items)
	declaredItems = runtimecontract.NormalizeRuntimeCoverageItems(declaredItems)
	if len(items) == 0 {
		return nil, nil
	}
	for _, profile := range runtimecontract.NormalizeRuntimeProfileContracts(profiles) {
		for _, dependency := range profile.CoverageDependencies {
			sources := coverageItemsForDependencyBinding(items, dependency.SourceScope, dependency.SourceRequirementID)
			declaredSources := coverageItemsForDependencyBinding(declaredItems, dependency.SourceScope, dependency.SourceRequirementID)
			if len(declaredSources) == 0 {
				return nil, fmt.Errorf("runtime coverage dependency %q source %s/%s matched no coverage item", dependency.ID, dependency.SourceScope, dependency.SourceRequirementID)
			}
			dependents := coverageItemsForDependencyBinding(items, dependency.DependentScope, dependency.DependentRequirementID)
			declaredDependents := coverageItemsForDependencyBinding(declaredItems, dependency.DependentScope, dependency.DependentRequirementID)
			if len(declaredDependents) == 0 {
				return nil, fmt.Errorf("runtime coverage dependency %q dependent %s/%s matched no coverage item", dependency.ID, dependency.DependentScope, dependency.DependentRequirementID)
			}
			// A predecessor may have fully closed both endpoints. They are valid
			// declarations but absent from this continuation; no source effect in
			// this Run can reopen them, so do not manufacture a relationship.
			if len(sources) == 0 || len(dependents) == 0 {
				continue
			}
			for _, source := range sources {
				for _, dependent := range dependents {
					if source.ID == dependent.ID {
						return nil, fmt.Errorf("runtime coverage dependency %q resolves source and dependent to coverage item %s", dependency.ID, source.ID)
					}
					if dependency.RequireSourceSettled {
						for idx := range items {
							if items[idx].ID != dependent.ID {
								continue
							}
							items[idx].PrerequisiteCoverageWorkItemIDs = append(items[idx].PrerequisiteCoverageWorkItemIDs, source.ID)
							items[idx] = items[idx].Normalized()
							break
						}
					}
					if len(dependency.TriggerEvidenceKinds) > 0 {
						for idx := range items {
							if items[idx].ID != source.ID {
								continue
							}
							items[idx].ReopenDependencies = append(items[idx].ReopenDependencies, runtimecontract.RuntimeCoverageReopenDependency{
								CoverageWorkItemID: dependent.ID, TriggerEvidenceKinds: dependency.TriggerEvidenceKinds,
							})
							items[idx] = items[idx].Normalized()
							break
						}
					}
				}
			}
		}
	}
	items = runtimecontract.NormalizeRuntimeCoverageItems(items)
	if err := runtimecontract.ValidateRuntimeCoverageItemDependencies(items); err != nil {
		return nil, fmt.Errorf("runtime coverage dependencies: %w", err)
	}
	return items, nil
}

func runtimeCoverageItemsFromWorkUnits(units []workUnit) []runtimecontract.RuntimeCoverageItem {
	ret := make([]runtimecontract.RuntimeCoverageItem, 0, len(units))
	for _, unit := range units {
		ret = append(ret, runtimecontract.RuntimeCoverageItem{
			ID: unit.itemID, SourceScope: unit.sourceScope, SourceID: unit.sourceID, TargetText: unit.text, Order: unit.order,
		})
	}
	return runtimecontract.NormalizeRuntimeCoverageItems(ret)
}

func coverageItemsForDependencyBinding(items []runtimecontract.RuntimeCoverageItem, scope, requirementID string) []runtimecontract.RuntimeCoverageItem {
	scope = normalizeEvidenceScope(scope)
	requirementID = strings.TrimSpace(requirementID)
	ret := make([]runtimecontract.RuntimeCoverageItem, 0, len(items))
	for _, item := range items {
		item = item.Normalized()
		if item.SourceScope != scope || (requirementID != "*" && item.SourceID != requirementID) {
			continue
		}
		ret = append(ret, item)
	}
	return ret
}

func resolveCoverageEvidencePolicy(scope, id string, activation runtimecontract.RuntimeActivationContext, stateChangeRequired bool) (compiledEvidencePolicy, error) {
	scope = strings.TrimSpace(scope)
	id = strings.TrimSpace(id)
	if scope == "" {
		return compiledEvidencePolicy{}, fmt.Errorf("coverage evidence policy requires scope")
	}
	// Neutral runtime default: Goal/DoD/acceptance coverage requires assessment.
	// Validation is only the fallback proof when the selected template/profile
	// has no explicit evidence kind for this CWI. Applying it before explicit
	// declarations would turn a state-change CWI into an impossible cross-owner
	// state_change+validation binding. Run-level must_produce_state_change is NOT
	// applied to every DoD focus; only an explicit declaration or later repair
	// elevates state_change.
	policy := compiledEvidencePolicy{RequireAssessment: true}
	defaultEvidenceKinds := []string{}
	switch scope {
	case "dodItem", "goalCoverage", "acceptanceDimension":
		defaultEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	default:
		// unknown scopes keep assessment-only neutral default
	}
	_ = stateChangeRequired

	// Collect profile requirements and evidence-policy coverage requirements as neutral inputs.
	candidates := make([]runtimecontract.RuntimeProfileEvidenceRequirement, 0, 8)
	for _, profile := range runtimecontract.NormalizeRuntimeProfileContracts(activation.ExecutionContract.ProfileContracts) {
		for _, requirement := range profile.EvidenceRequirements {
			if strings.TrimSpace(requirement.Scope) != scope {
				continue
			}
			reqID := strings.TrimSpace(requirement.ID)
			if reqID == "" {
				return compiledEvidencePolicy{}, fmt.Errorf("profile evidence requirement for scope %q has empty id", scope)
			}
			if reqID != id && reqID != "*" {
				continue
			}
			if err := validateProfileAcceptedKinds(requirement); err != nil {
				return compiledEvidencePolicy{}, err
			}
			requirement.TargetSelectorRef, requirement.TargetSelectorOwnerRef, requirement.TargetSelectorSubjectRef =
				runtimecontract.ResolveRuntimeProfileTargetSelectorIdentity(profile.ID, requirement)
			candidates = append(candidates, requirement)
		}
	}
	for _, evidencePolicy := range runtimecontract.NormalizeRuntimeEvidencePolicies(activation.EvidencePolicies) {
		for _, requirement := range evidencePolicy.CoverageRequirements {
			reqScope := normalizeEvidenceScope(requirement.Scope)
			if reqScope != scope {
				continue
			}
			reqID := strings.TrimSpace(requirement.ID)
			if reqID == "" {
				return compiledEvidencePolicy{}, fmt.Errorf("evidence policy coverage requirement for scope %q has empty id", scope)
			}
			if reqID != id && reqID != "*" {
				continue
			}
			mapped := runtimecontract.RuntimeProfileEvidenceRequirement{
				ID:                                   reqID,
				Scope:                                reqScope,
				EvidenceRole:                         requirement.EvidenceRole,
				TargetRole:                           requirement.TargetRole,
				TargetRefs:                           append([]string(nil), requirement.TargetRefs...),
				TargetDiscoverySources:               append([]string(nil), requirement.TargetDiscoverySources...),
				TargetSelectorRef:                    requirement.TargetSelectorRef,
				TargetSelectorOwnerRef:               requirement.TargetSelectorOwnerRef,
				TargetSelectorSubjectRef:             requirement.TargetSelectorSubjectRef,
				TargetSelectorCandidateRefs:          append([]string(nil), requirement.TargetSelectorCandidateRefs...),
				ProspectiveTargetAllOfConstraintRefs: append([]string(nil), requirement.ProspectiveTargetAllOfConstraintRefs...),
				RequireTargetGrounding:               requirement.RequireTargetGrounding,
				RequiredEvidenceKinds:                append([]string(nil), requirement.RequiredEvidenceKinds...),
				RequireDirectEvidence:                requirement.RequireDirectEvidence,
				RequireClaimEvidence:                 requirement.RequireClaimEvidence,
				RequireExecutableBehaviorEvidence:    requirement.RequireExecutableBehaviorEvidence,
				RequireCoverageTargets:               requirement.RequireCoverageTargets,
				AcceptedStepKinds:                    append([]string(nil), requirement.AcceptedStepKinds...),
				AcceptedOperationVariants:            append([]runtimecontract.RuntimeOperationVariantRef(nil), requirement.AcceptedOperationVariants...),
				RejectedStepKinds:                    append([]string(nil), requirement.RejectedStepKinds...),
				ValidationRecipeID:                   requirement.ValidationRecipeID,
				RequiredCheckRefs:                    append([]string(nil), requirement.RequiredCheckRefs...),
				MetricConstraints:                    append([]string(nil), requirement.MetricConstraints...),
				ArtifactRefs:                         append([]string(nil), requirement.ArtifactRefs...),
				VerificationPolicy:                   requirement.VerificationPolicy,
				Description:                          requirement.Description,
			}
			if err := validateProfileAcceptedKinds(mapped); err != nil {
				return compiledEvidencePolicy{}, err
			}
			candidates = append(candidates, mapped)
		}
	}

	// Precedence within same scope: exact id first, then wildcard "*".
	// Within the same precedence tier, merge with stable union / first-non-empty rules.
	exact := filterRequirementsByID(candidates, id)
	wildcard := filterRequirementsByID(candidates, "*")
	var err error
	if id != "*" {
		policy, err = mergeCompiledEvidencePolicy(policy, exact)
		if err != nil {
			return compiledEvidencePolicy{}, err
		}
		policy, err = mergeCompiledEvidencePolicy(policy, wildcardQualityRequirements(wildcard))
		if err != nil {
			return compiledEvidencePolicy{}, err
		}
	} else {
		policy, err = mergeCompiledEvidencePolicy(policy, exact)
		if err != nil {
			return compiledEvidencePolicy{}, err
		}
	}
	if len(policy.RequiredEvidenceKinds) == 0 {
		policy.RequiredEvidenceKinds = defaultEvidenceKinds
	}
	// This exact identity is compiled only after the Application has resolved an
	// admitted CWI. A wildcard may contribute declarative constraints above,
	// including an owner-fact postcondition, but never survives as Definition
	// identity or supplies recipe/check/current-owner authority for a CWI.
	policy.SemanticProofDeclarationRef = runtimecontract.CanonicalRuntimeDefinitionEvidenceDeclarationRef(scope, id)
	return policy, nil
}

func wildcardQualityRequirements(values []runtimecontract.RuntimeProfileEvidenceRequirement) []runtimecontract.RuntimeProfileEvidenceRequirement {
	ret := make([]runtimecontract.RuntimeProfileEvidenceRequirement, 0, len(values))
	for _, value := range values {
		// A wildcard is a declarative policy template, not Runtime authority. The
		// Application may instantiate its constraints into an already admitted
		// exact CWI, whose later owner catalog and SM-2 binding remain necessary.
		// Only wildcard identity and recipe/check proof identity are discarded:
		// those must come from the canonical exact Definition and owner catalog.
		ret = append(ret, runtimecontract.RuntimeProfileEvidenceRequirement{
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
			AcceptedOperationVariants:            append([]runtimecontract.RuntimeOperationVariantRef(nil), value.AcceptedOperationVariants...),
			RejectedStepKinds:                    append([]string(nil), value.RejectedStepKinds...),
			MetricConstraints:                    append([]string(nil), value.MetricConstraints...),
			ArtifactRefs:                         append([]string(nil), value.ArtifactRefs...),
			VerificationPolicy:                   value.VerificationPolicy,
			RequiredOwnerFacts:                   append([]runtimecontract.RuntimeOwnerFactRequirement(nil), value.RequiredOwnerFacts...),
			OwnerFactConvergences:                runtimecontract.CloneRuntimeOwnerFactConvergences(value.OwnerFactConvergences),
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

func filterRequirementsByID(values []runtimecontract.RuntimeProfileEvidenceRequirement, id string) []runtimecontract.RuntimeProfileEvidenceRequirement {
	ret := make([]runtimecontract.RuntimeProfileEvidenceRequirement, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value.ID) == id {
			ret = append(ret, value)
		}
	}
	return ret
}

func mergeCompiledEvidencePolicy(base compiledEvidencePolicy, requirements []runtimecontract.RuntimeProfileEvidenceRequirement) (compiledEvidencePolicy, error) {
	for _, requirement := range requirements {
		// Stable unique union for required evidence / rejected kinds.
		base.RequiredEvidenceKinds = stableStringUnion(base.RequiredEvidenceKinds, requirement.RequiredEvidenceKinds)
		base.RejectedStepKinds = stableStringUnion(base.RejectedStepKinds, requirement.RejectedStepKinds)
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
		base.TargetSelectorCandidateRefs = stableStringUnion(base.TargetSelectorCandidateRefs, requirement.TargetSelectorCandidateRefs)
		base.ProspectiveTargetAllOfConstraintRefs = stableStringUnion(base.ProspectiveTargetAllOfConstraintRefs, requirement.ProspectiveTargetAllOfConstraintRefs)
		base.TargetRefs = stableStringUnion(base.TargetRefs, requirement.TargetRefs)
		base.TargetDiscoverySources = stableStringUnion(base.TargetDiscoverySources, requirement.TargetDiscoverySources)
		base.RequireTargetGrounding = base.RequireTargetGrounding || requirement.RequireTargetGrounding
		base.AcceptedStepKinds = stableStringUnion(base.AcceptedStepKinds, requirement.AcceptedStepKinds)
		base.AcceptedOperationVariants = runtimecontract.NormalizeRuntimeOperationVariantRefs(append(base.AcceptedOperationVariants, requirement.AcceptedOperationVariants...))
		if base.Description == "" && strings.TrimSpace(requirement.Description) != "" {
			base.Description = strings.TrimSpace(requirement.Description)
		}
		if base.ValidationRecipeID == "" && strings.TrimSpace(requirement.ValidationRecipeID) != "" {
			base.ValidationRecipeID = strings.TrimSpace(requirement.ValidationRecipeID)
		}
		base.RequiredCheckRefs = stableStringUnion(base.RequiredCheckRefs, requirement.RequiredCheckRefs)
		base.MetricConstraints = stableStringUnion(base.MetricConstraints, requirement.MetricConstraints)
		base.ArtifactRefs = stableStringUnion(base.ArtifactRefs, requirement.ArtifactRefs)
		base.RequiredOwnerFacts = runtimecontract.NormalizeRuntimeOwnerFactRequirements(append(base.RequiredOwnerFacts, requirement.RequiredOwnerFacts...))
		base.OwnerFactConvergences = runtimecontract.NormalizeRuntimeOwnerFactConvergences(append(base.OwnerFactConvergences, requirement.OwnerFactConvergences...))
		if requirement.ValidationFailureRepairConvergence != nil {
			if base.ValidationFailureRepairConvergence != nil && !runtimecontract.SameRuntimeValidationFailureRepairConvergence(base.ValidationFailureRepairConvergence, requirement.ValidationFailureRepairConvergence) {
				return compiledEvidencePolicy{}, fmt.Errorf("coverage evidence policy has conflicting validation failure repair convergence declarations")
			}
			if base.ValidationFailureRepairConvergence == nil {
				clone := requirement.ValidationFailureRepairConvergence.Clone()
				base.ValidationFailureRepairConvergence = &clone
			}
		}
		if requirement.VerificationPolicy != nil {
			if base.VerificationPolicy != nil && !runtimecontract.SameRuntimeVerificationPolicy(base.VerificationPolicy, requirement.VerificationPolicy) {
				return compiledEvidencePolicy{}, fmt.Errorf("coverage evidence policy has conflicting verification policies")
			}
			base.VerificationPolicy = requirement.VerificationPolicy
		}
		if requirement.ImplementationWorkPolicy != nil {
			if base.ImplementationWorkPolicy != nil && !runtimecontract.SameRuntimeImplementationWorkPolicy(base.ImplementationWorkPolicy, requirement.ImplementationWorkPolicy) {
				return compiledEvidencePolicy{}, fmt.Errorf("coverage evidence policy has conflicting implementation work declarations")
			}
			if base.ImplementationWorkPolicy == nil {
				clone := requirement.ImplementationWorkPolicy.Clone()
				base.ImplementationWorkPolicy = &clone
			}
		}
		// Profile explicit primary_state_change implies state-change evidence.
		if requirement.EvidenceRole == "primary_state_change" || len(requirement.AcceptedOperationVariants) > 0 {
			base.RequiredEvidenceKinds = stableStringUnion(base.RequiredEvidenceKinds, []string{runtimecontract.EvidenceKindStateChange})
		}
	}
	return base, nil
}

func applyCompiledEvidencePolicy(focus runtimecontract.CoverageFocus, policy compiledEvidencePolicy) runtimecontract.CoverageFocus {
	focus.RequiredEvidenceKinds = append([]string(nil), policy.RequiredEvidenceKinds...)
	focus.AcceptedStepKinds = append([]string(nil), policy.AcceptedStepKinds...)
	focus.AcceptedOperationVariants = append([]runtimecontract.RuntimeOperationVariantRef(nil), policy.AcceptedOperationVariants...)
	focus.RejectedStepKinds = append([]string(nil), policy.RejectedStepKinds...)
	if policy.EvidenceRole != "" {
		focus.EvidenceRole = policy.EvidenceRole
	}
	focus.TargetRole = policy.TargetRole
	focus.TargetSelectorRef = policy.TargetSelectorRef
	focus.TargetSelectorOwnerRef = policy.TargetSelectorOwnerRef
	focus.TargetSelectorSubjectRef = policy.TargetSelectorSubjectRef
	focus.TargetSelectorCandidateRefs = append([]string(nil), policy.TargetSelectorCandidateRefs...)
	focus.ProspectiveTargetAllOfConstraintRefs = append([]string(nil), policy.ProspectiveTargetAllOfConstraintRefs...)
	focus.TargetRefs = append([]string(nil), policy.TargetRefs...)
	focus.TargetDiscoverySources = append([]string(nil), policy.TargetDiscoverySources...)
	focus.RequireTargetGrounding = policy.RequireTargetGrounding
	focus.RequireDirectEvidence = policy.RequireDirectEvidence
	focus.RequireClaimEvidence = policy.RequireClaimEvidence
	focus.RequireExecutableBehaviorEvidence = policy.RequireExecutableBehaviorEvidence
	focus.RequireCoverageTargets = policy.RequireCoverageTargets
	focus.ValidationRecipeID = policy.ValidationRecipeID
	focus.RequiredCheckRefs = append([]string(nil), policy.RequiredCheckRefs...)
	focus.MetricConstraints = append([]string(nil), policy.MetricConstraints...)
	focus.ArtifactRefs = append([]string(nil), policy.ArtifactRefs...)
	focus.RequiredOwnerFacts = append([]runtimecontract.RuntimeOwnerFactRequirement(nil), policy.RequiredOwnerFacts...)
	focus.OwnerFactConvergences = runtimecontract.CloneRuntimeOwnerFactConvergences(policy.OwnerFactConvergences)
	focus.RequireAssessment = policy.RequireAssessment || focus.RequireAssessment
	return focus.Normalized()
}

func validateProfileAcceptedKinds(requirement runtimecontract.RuntimeProfileEvidenceRequirement) error {
	for _, kind := range requirement.AcceptedStepKinds {
		canonical := runtimecontract.CanonicalRuntimeBlockKind(kind)
		if canonical == "" {
			continue
		}
		if _, ok := runtimecontract.RuntimeOperationDescriptorForKind(canonical); !ok {
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

func normalizeEvidenceScope(scope string) string {
	switch strings.TrimSpace(scope) {
	case "dod_item":
		return "dodItem"
	case "goal_coverage":
		return "goalCoverage"
	default:
		return strings.TrimSpace(scope)
	}
}

func stableStringUnion(values ...[]string) []string {
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

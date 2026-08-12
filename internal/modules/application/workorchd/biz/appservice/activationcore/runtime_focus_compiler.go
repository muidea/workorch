package activationcore

import (
	"fmt"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type compiledCoverage struct {
	Focuses       []runtimecontract.CoverageFocus
	Slices        []runtimecontract.RuntimeAcceptanceSlice
	CoverageItems []runtimecontract.RuntimeCoverageItem
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

// compiledEvidencePolicy is the neutral merge result for one coverage focus.
// Description is audit-only and never enters reducer authority.
type compiledEvidencePolicy struct {
	RequiredEvidenceKinds             []string
	AcceptedStepKinds                 []string
	AcceptedOperationVariants         []runtimecontract.RuntimeOperationVariantRef
	RejectedStepKinds                 []string
	EvidenceRole                      string
	TargetRole                        string
	TargetRefs                        []string
	TargetDiscoverySources            []string
	TargetSelectorRef                 string
	TargetSelectorOwnerRef            string
	TargetSelectorSubjectRef          string
	TargetSelectorCandidateRefs       []string
	ProspectiveTargetRef              string
	RequireTargetGrounding            bool
	RequireDirectEvidence             bool
	RequireClaimEvidence              bool
	RequireExecutableBehaviorEvidence bool
	RequireCoverageTargets            bool
	RequireAssessment                 bool
	ValidationRecipeID                string
	RequiredCheckRefs                 []string
	MetricConstraints                 []string
	ArtifactRefs                      []string
	DeclarationRef                    string
	SemanticProofDeclarationRef       string
	Description                       string
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
	stateChangeRequired := activation.ExecutionContract.StateChangePolicy == runtimecontract.RuntimeStateChangeMustProduce

	// 1) Compile profile-owned bootstrap artifacts first. This is intentionally
	// done in activation, before Runtime exists: the runtime only receives
	// ordinary sealed Focus contracts and never derives a project layout from
	// Goal prose, file suffixes, or provider output.
	units := make([]workUnit, 0, len(dod.Items)+8)
	bootstrapUnits, err := compileRuntimeBootstrapWorkUnits(goal, dod, activation.ExecutionContract.ProfileContracts)
	if err != nil {
		return compiledCoverage{}, err
	}
	units = append(units, bootstrapUnits...)
	// 2) Generate item-level work units in definition order (one item identity per DoD/profile source).
	for idx, item := range dod.Items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		units = append(units, workUnit{
			sourceScope: "dodItem",
			sourceID:    fmt.Sprintf("dod-%03d", idx+1),
			text:        item,
			order:       len(units) + 1,
		})
	}
	// profile required dimensions/deliverables keep stable source identity.
	for _, profile := range runtimecontract.NormalizeRuntimeProfileContracts(activation.ExecutionContract.ProfileContracts) {
		for _, dimension := range profile.AcceptanceDimensions {
			if !dimension.Required || strings.TrimSpace(dimension.ID) == "" {
				continue
			}
			units = append(units, workUnit{
				sourceScope: "acceptanceDimension", sourceID: dimension.ID, text: dimension.Description, order: len(units) + 1,
			})
		}
		for _, evidenceRole := range []string{"primary_state_change", "supplemental_evidence", ""} {
			for _, deliverable := range profile.DeliverableExpectations {
				if !deliverable.Required || deliverable.EvidenceRole != evidenceRole || strings.TrimSpace(deliverable.ID) == "" {
					continue
				}
				units = append(units, workUnit{
					sourceScope: "deliverable", sourceID: deliverable.ID, text: deliverable.Description,
					evidenceRole: deliverable.EvidenceRole, order: len(units) + 1,
				})
			}
		}
	}
	// Stable item IDs are assigned from original definition order before continuation filtering.
	for idx := range units {
		units[idx].order = idx + 1
		units[idx].itemID = common.CoverageWorkItemID(fmt.Sprintf("cwi-%03d", units[idx].order))
	}
	allUnits := append([]workUnit(nil), units...)
	if len(units) == 0 {
		return compiledCoverage{}, fmt.Errorf("runtime coverage focus compiler produced no coverage items")
	}

	// Resolve per-item evidence policy first (item identity independent of slice size).
	resolved := make([]resolvedCoverageItem, 0, len(units))
	for _, unit := range units {
		var policy compiledEvidencePolicy
		if unit.bootstrapArtifact != nil {
			policy, err = compileRuntimeBootstrapArtifactPolicy(unit)
		} else {
			policy, err = resolveCoverageEvidencePolicy(unit.sourceScope, unit.sourceID, activation, stateChangeRequired)
		}
		if err != nil {
			return compiledCoverage{}, err
		}
		if unit.evidenceRole != "" {
			policy.EvidenceRole = unit.evidenceRole
		}
		itemID := unit.itemID
		if itemID == "" {
			itemID = common.CoverageWorkItemID(fmt.Sprintf("cwi-%03d", unit.order))
		}
		resolved = append(resolved, resolvedCoverageItem{
			unit:   unit,
			itemID: itemID,
			policy: policy,
		})
	}

	// 2) Group into acceptance slices using typed policy (or 1 item = 1 slice default).
	slicePolicy := runtimecontract.RuntimeAcceptanceSlicePolicy{
		SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent,
		MaxWorkItems:  runtimecontract.RuntimeAcceptanceSliceDefaultMaxItems, MaxFocuses: runtimecontract.RuntimeAcceptanceFocusMaxCount,
		MaxCandidateContextItems: runtimecontract.RuntimeCandidateContextItemsMaxCount,
	}
	if activation.ExecutionContract.AcceptanceSlicePolicy != nil {
		slicePolicy = activation.ExecutionContract.AcceptanceSlicePolicy.Normalized()
	}
	if err := slicePolicy.Validate(); err != nil {
		return compiledCoverage{}, err
	}

	groups := slicePolicy.Groups
	if len(groups) == 0 {
		// Neutral fallback group: all remaining items, one focus per item when max=1.
		groups = []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{
			ID: "slice-default", Title: "Default acceptance", Scope: "default", Priority: 100,
			MaxWorkItems: slicePolicy.MaxWorkItems, Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true},
		}}
	}
	// A matched bootstrap recipe is an activation-time dependency, not a normal
	// acceptance category.  In particular, an empty workspace cannot satisfy a
	// generic source-change focus until its profile-declared first artifact has
	// been selected and created.  Do not let a profile's ordinary acceptance
	// groups (for example primary_state_change before remaining_acceptance)
	// reorder that dependency behind an ungrounded focus.
	//
	// This remains an application compilation rule: the runtime only receives
	// the resulting ordered Focus contracts and never interprets profile or
	// artifact semantics.
	hasBootstrapArtifacts := false
	for _, item := range resolved {
		if item.unit.bootstrapArtifact != nil {
			hasBootstrapArtifacts = true
			break
		}
	}
	if hasBootstrapArtifacts {
		groups = append([]runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{
			ID: "slice-bootstrap-artifact", Title: "Profile bootstrap artifacts", Scope: "bootstrap_artifact", Priority: -1,
			MaxWorkItems: 1, Match: runtimecontract.RuntimeAcceptanceSliceMatch{SourceScopes: []string{"bootstrapArtifact"}},
		}}, groups...)
	}

	assigned := make([]bool, len(resolved))
	type sliceBucket struct {
		group runtimecontract.RuntimeAcceptanceSliceGroupPolicy
		items []resolvedCoverageItem
	}
	buckets := make([]sliceBucket, 0, len(groups))
	for _, group := range groups {
		bucket := sliceBucket{group: group}
		for idx, item := range resolved {
			if assigned[idx] {
				continue
			}
			if !matchAcceptanceSliceGroup(group, item.unit.sourceScope, item.unit.sourceID, item.policy) {
				continue
			}
			assigned[idx] = true
			bucket.items = append(bucket.items, item)
		}
		if len(bucket.items) > 0 {
			buckets = append(buckets, bucket)
		}
	}
	for idx, ok := range assigned {
		if !ok {
			return compiledCoverage{}, fmt.Errorf("coverage item %s/%s matched no acceptance slice group", resolved[idx].unit.sourceScope, resolved[idx].unit.sourceID)
		}
	}

	focuses := make([]runtimecontract.CoverageFocus, 0, len(resolved))
	slices := make([]runtimecontract.RuntimeAcceptanceSlice, 0, len(buckets))
	coverageItems := make([]runtimecontract.RuntimeCoverageItem, 0, len(resolved))
	focusSeq := 0
	for _, bucket := range buckets {
		maxItems := slicePolicy.EffectiveMaxWorkItems(bucket.group)
		// A slice is a bounded presentation and context unit.  It is deliberately
		// not a coverage-credit unit: a single owner receipt must never acquire
		// credit for multiple semantic CWI merely because they appear together in
		// the UI/request batch.
		ordinal := 0
		for start := 0; start < len(bucket.items); start += maxItems {
			end := start + maxItems
			if end > len(bucket.items) {
				end = len(bucket.items)
			}
			presentationChunk := bucket.items[start:end]
			ordinal++
			sliceID := fmt.Sprintf("%s-%03d", bucket.group.ID, ordinal)
			ids := make([]common.CoverageWorkItemID, 0, len(presentationChunk))
			requiredKinds := []string{}
			for _, item := range presentationChunk {
				ids = append(ids, item.itemID)
				requiredKinds = stableStringUnion(requiredKinds, item.policy.RequiredEvidenceKinds)
				coverageItems = append(coverageItems, runtimecontract.RuntimeCoverageItem{
					ID: item.itemID, SliceID: sliceID, SourceScope: item.unit.sourceScope, SourceID: item.unit.sourceID,
					TargetText: item.unit.text, Order: item.unit.order,
				}.Normalized())
			}
			slices = append(slices, runtimecontract.RuntimeAcceptanceSlice{
				ID: sliceID, Title: bucket.group.Title, Scope: bucket.group.Scope, Priority: bucket.group.Priority,
				CoverageWorkItemIDs: ids, RequiredEvidenceKinds: requiredKinds,
				CompletionPolicy: slicePolicy.CompletionPolicy, ResultPolicy: slicePolicy.ResultPolicy,
			}.Normalized())

			// Slices bound presentation and request batching only. Runtime
			// authority has one focus per canonical CWI so a semantic binding,
			// receipt, evidence transition, reopen and reconciliation entry all
			// share one unambiguous identity.
			for _, item := range presentationChunk {
				focusSeq++
				focus := newCompiledCoverageFocus(fmt.Sprintf("focus-%03d", focusSeq), sliceID, bucket.group.Scope, item)
				if err := runtimecontract.ValidateRuntimeTargetGrounding(focus.RequireTargetGrounding, focus.TargetRole, focus.TargetRefs, focus.TargetDiscoverySources, focus.TargetSelectorRef); err != nil {
					return compiledCoverage{}, fmt.Errorf("compiled focus %s: %w", focus.ID, err)
				}
				focuses = append(focuses, focus)
			}
		}
	}
	coverageItems, err = compileRuntimeCoverageDependencies(coverageItems, runtimeCoverageItemsFromWorkUnits(allUnits), activation.ExecutionContract.ProfileContracts)
	if err != nil {
		return compiledCoverage{}, err
	}
	if err := runtimecontract.ValidateRuntimeCoverageItemDependencies(coverageItems); err != nil {
		return compiledCoverage{}, fmt.Errorf("compiled coverage dependencies: %w", err)
	}
	if len(focuses) == 0 {
		return compiledCoverage{}, fmt.Errorf("runtime coverage focus compiler produced no focuses")
	}
	if len(focuses) > slicePolicy.MaxFocuses {
		return compiledCoverage{}, fmt.Errorf("runtime coverage focus compiler produced %d focuses, exceeding sealed policy limit %d; configure bounded acceptance slices", len(focuses), slicePolicy.MaxFocuses)
	}
	return compiledCoverage{Focuses: focuses, Slices: slices, CoverageItems: coverageItems}, nil
}

// compiledEvidencePolicyRequiresIndependentFocus identifies declarations that
// need a CWI-specific semantic proof declaration. It intentionally lives in
// activationcore: it interprets profile/template policy while
// workorchruntimecontract only validates the resulting immutable values.
func compiledEvidencePolicyRequiresIndependentFocus(policy compiledEvidencePolicy) bool {
	return containsRuntimeEvidenceKind(policy.RequiredEvidenceKinds, runtimecontract.EvidenceKindValidation) ||
		policy.RequireExecutableBehaviorEvidence || len(policy.MetricConstraints) != 0 || len(policy.ArtifactRefs) != 0
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
		TargetSelectorCandidateRefs: policy.TargetSelectorCandidateRefs, ProspectiveTargetRef: policy.ProspectiveTargetRef,
		RequireTargetGrounding: policy.RequireTargetGrounding, AcceptedStepKinds: policy.AcceptedStepKinds,
		AcceptedOperationVariants: policy.AcceptedOperationVariants, RejectedStepKinds: policy.RejectedStepKinds,
		RequireDirectEvidence: policy.RequireDirectEvidence, RequireClaimEvidence: policy.RequireClaimEvidence,
		RequireExecutableBehaviorEvidence: policy.RequireExecutableBehaviorEvidence, RequireCoverageTargets: policy.RequireCoverageTargets,
		RequireAssessment: policy.RequireAssessment, ValidationRecipeID: policy.ValidationRecipeID,
		RequiredCheckRefs: policy.RequiredCheckRefs, MetricConstraints: policy.MetricConstraints, ArtifactRefs: policy.ArtifactRefs,
		EvidenceDeclarationRef: policy.SemanticProofDeclarationRef,
	}.Normalized()
}

// compileRuntimeBootstrapWorkUnits resolves a declarative profile recipe using
// only activation inputs. A recipe is either exactly selected or not selected;
// ambiguous profile declarations are rejected before a Run can seal an
// arbitrary bootstrap target. No filesystem or capability-owner access occurs
// here, preserving File as the owner of path/manifest observations.
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
	if artifact.ID == "" || artifact.TargetRole == "" || artifact.TargetRef == "" || len(artifact.AllowedOperationKinds) == 0 {
		return compiledEvidencePolicy{}, fmt.Errorf("bootstrap artifact work unit is incomplete")
	}
	return compiledEvidencePolicy{
		RequiredEvidenceKinds:       []string{runtimecontract.EvidenceKindStateChange},
		AcceptedStepKinds:           append([]string(nil), artifact.AllowedOperationKinds...),
		EvidenceRole:                "bootstrap_artifact",
		TargetRole:                  artifact.TargetRole,
		TargetDiscoverySources:      []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest, runtimecontract.RuntimeValidationDiscoverySourceProjectFiles},
		TargetSelectorRef:           "file/profile_declared_candidates",
		TargetSelectorOwnerRef:      unit.bootstrapProfile,
		TargetSelectorSubjectRef:    unit.bootstrapRecipe + "/" + artifact.ID,
		TargetSelectorCandidateRefs: []string{artifact.TargetRef},
		ProspectiveTargetRef:        artifact.TargetRef,
		RequireTargetGrounding:      true,
		RequireDirectEvidence:       true,
		RequireCoverageTargets:      true,
		Description:                 "profile-declared workspace bootstrap artifact",
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
	return runtimecontract.NormalizeRuntimeCoverageItems(items), nil
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
			if strings.TrimSpace(requirement.TargetSelectorRef) != "" {
				if strings.TrimSpace(requirement.TargetSelectorOwnerRef) == "" {
					requirement.TargetSelectorOwnerRef = profile.ID
				}
				if strings.TrimSpace(requirement.TargetSelectorSubjectRef) == "" {
					requirement.TargetSelectorSubjectRef = scope
				}
			}
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
				ID:                                reqID,
				Scope:                             reqScope,
				EvidenceRole:                      requirement.EvidenceRole,
				TargetRole:                        requirement.TargetRole,
				TargetRefs:                        append([]string(nil), requirement.TargetRefs...),
				TargetDiscoverySources:            append([]string(nil), requirement.TargetDiscoverySources...),
				TargetSelectorRef:                 requirement.TargetSelectorRef,
				TargetSelectorOwnerRef:            requirement.TargetSelectorOwnerRef,
				TargetSelectorSubjectRef:          requirement.TargetSelectorSubjectRef,
				TargetSelectorCandidateRefs:       append([]string(nil), requirement.TargetSelectorCandidateRefs...),
				ProspectiveTargetRef:              requirement.ProspectiveTargetRef,
				RequireTargetGrounding:            requirement.RequireTargetGrounding,
				RequiredEvidenceKinds:             append([]string(nil), requirement.RequiredEvidenceKinds...),
				RequireDirectEvidence:             requirement.RequireDirectEvidence,
				RequireClaimEvidence:              requirement.RequireClaimEvidence,
				RequireExecutableBehaviorEvidence: requirement.RequireExecutableBehaviorEvidence,
				RequireCoverageTargets:            requirement.RequireCoverageTargets,
				AcceptedStepKinds:                 append([]string(nil), requirement.AcceptedStepKinds...),
				AcceptedOperationVariants:         append([]runtimecontract.RuntimeOperationVariantRef(nil), requirement.AcceptedOperationVariants...),
				RejectedStepKinds:                 append([]string(nil), requirement.RejectedStepKinds...),
				ValidationRecipeID:                requirement.ValidationRecipeID,
				RequiredCheckRefs:                 append([]string(nil), requirement.RequiredCheckRefs...),
				MetricConstraints:                 append([]string(nil), requirement.MetricConstraints...),
				ArtifactRefs:                      append([]string(nil), requirement.ArtifactRefs...),
				Description:                       requirement.Description,
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
	if id != "*" {
		policy = mergeCompiledEvidencePolicy(policy, exact)
		policy = mergeCompiledEvidencePolicy(policy, wildcard)
	} else {
		policy = mergeCompiledEvidencePolicy(policy, exact)
	}
	if len(policy.RequiredEvidenceKinds) == 0 {
		policy.RequiredEvidenceKinds = defaultEvidenceKinds
	}
	if compiledEvidencePolicyRequiresIndependentFocus(policy) {
		if hasCWISemanticProofDeclaration(exact) {
			policy.SemanticProofDeclarationRef = "profile/" + scope + "/" + id
		} else {
			// Wildcard rules remain useful quality gates, but their recipe/check
			// cannot be copied into an arbitrary CWI binding and later grant
			// semantic coverage credit.
			policy.SemanticProofDeclarationRef = "profile/" + scope + "/*"
		}
	}
	return policy, nil
}

func hasCWISemanticProofDeclaration(values []runtimecontract.RuntimeProfileEvidenceRequirement) bool {
	for _, value := range values {
		if strings.TrimSpace(value.ValidationRecipeID) != "" || len(value.RequiredCheckRefs) != 0 ||
			len(value.MetricConstraints) != 0 || len(value.ArtifactRefs) != 0 {
			return true
		}
	}
	return false
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

func mergeCompiledEvidencePolicy(base compiledEvidencePolicy, requirements []runtimecontract.RuntimeProfileEvidenceRequirement) compiledEvidencePolicy {
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
		if base.ProspectiveTargetRef == "" && strings.TrimSpace(requirement.ProspectiveTargetRef) != "" {
			base.ProspectiveTargetRef = strings.TrimSpace(requirement.ProspectiveTargetRef)
		}
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
		// Profile explicit primary_state_change implies state-change evidence.
		if requirement.EvidenceRole == "primary_state_change" || len(requirement.AcceptedOperationVariants) > 0 {
			base.RequiredEvidenceKinds = stableStringUnion(base.RequiredEvidenceKinds, []string{runtimecontract.EvidenceKindStateChange})
		}
	}
	return base
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
	focus.ProspectiveTargetRef = policy.ProspectiveTargetRef
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

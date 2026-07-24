package activationcore

import (
	"fmt"
	"sort"
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
	Description                       string
}

// compileRuntimeV2CoverageFocuses builds the deterministic focus queue for one activation.
// Evidence policy precedence is exact(scope,id) > wildcard(scope,*) > scope default > neutral default.
func compileRuntimeV2CoverageFocuses(goal *workspacemodel.Goal, dod *workspacemodel.DoD, activation runtimecontract.RuntimeActivationContext) ([]runtimecontract.CoverageFocus, error) {
	compiled, err := compileRuntimeV2Coverage(goal, dod, activation)
	if err != nil {
		return nil, err
	}
	return compiled.Focuses, nil
}

// compileRuntimeV2Coverage builds focuses, acceptance slices and item-level coverage identities.
func compileRuntimeV2Coverage(goal *workspacemodel.Goal, dod *workspacemodel.DoD, activation runtimecontract.RuntimeActivationContext) (compiledCoverage, error) {
	if goal == nil || dod == nil {
		return compiledCoverage{}, common.ErrInvalidArgument
	}
	activation = activation.Normalized()
	stateChangeRequired := activation.ExecutionContract.StateChangePolicy == runtimecontract.RuntimeStateChangeMustProduce

	// 1) Compile profile-owned bootstrap artifacts first. This is intentionally
	// done in activation, before Runtime V2 exists: the runtime only receives
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
	// Continuation rehydration: keep unresolved coverage only when definition revision is stable.
	units, gapAnalysis, err := applyContinuationToCoverageUnits(units, activation)
	if err != nil {
		return compiledCoverage{}, err
	}
	if gapAnalysis {
		// Neutral gap-analysis focus when continuation lacks structured unresolved IDs or definition changed unsafely.
		units = []workUnit{{
			sourceScope: "continuation_gap_analysis", sourceID: "continuation-gap-analysis",
			text: "Analyze predecessor rejected result gaps and emit structured unresolved coverage", order: 1,
			itemID: common.CoverageWorkItemID("cwi-001"),
		}}
	} else {
		// If a continuation must re-run a source coverage item, its declared
		// dependent proof must join the successor even when predecessor output
		// recorded that proof as satisfied.  Otherwise the source can change
		// without a ledger entry available to reopen.
		units, err = expandContinuationCoverageDependencies(units, allUnits, activation.ExecutionContract.ProfileContracts)
		if err != nil {
			return compiledCoverage{}, err
		}
	}
	if len(units) == 0 {
		return compiledCoverage{}, fmt.Errorf("runtime coverage focus compiler produced no coverage items")
	}

	// Resolve per-item evidence policy first (item identity independent of slice size).
	type resolvedItem struct {
		unit   workUnit
		itemID common.CoverageWorkItemID
		policy compiledEvidencePolicy
	}
	resolved := make([]resolvedItem, 0, len(units))
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
		resolved = append(resolved, resolvedItem{
			unit:   unit,
			itemID: itemID,
			policy: policy,
		})
	}

	// 2) Group into acceptance slices using typed policy (or 1 item = 1 slice default).
	slicePolicy := runtimecontract.RuntimeAcceptanceSlicePolicy{
		SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2,
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

	assigned := make([]bool, len(resolved))
	type sliceBucket struct {
		group runtimecontract.RuntimeAcceptanceSliceGroupPolicy
		items []resolvedItem
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
	partitionCompatibleItems := func(items []resolvedItem) [][]resolvedItem {
		if len(items) == 0 {
			return nil
		}
		ret := make([][]resolvedItem, 0, len(items))
		current := []resolvedItem{items[0]}
		currentKey := compiledEvidencePolicyCompatibilityKey(items[0].policy)
		for _, item := range items[1:] {
			key := compiledEvidencePolicyCompatibilityKey(item.policy)
			if key != currentKey {
				ret = append(ret, current)
				current = []resolvedItem{item}
				currentKey = key
				continue
			}
			current = append(current, item)
		}
		return append(ret, current)
	}
	focusSeq := 0
	for _, bucket := range buckets {
		maxItems := slicePolicy.EffectiveMaxWorkItems(bucket.group)
		// Focuses may contain only work that shares the same operational and
		// evidence policy. A slice group is a bounded presentation policy, not
		// permission to merge different target/evidence contracts into one SM-2
		// authority.
		ordinal := 0
		for _, compatibleItems := range partitionCompatibleItems(bucket.items) {
			for start := 0; start < len(compatibleItems); start += maxItems {
				end := start + maxItems
				if end > len(compatibleItems) {
					end = len(compatibleItems)
				}
				chunk := compatibleItems[start:end]
				ordinal++
				sliceID := fmt.Sprintf("%s-%03d", bucket.group.ID, ordinal)
				ids := make([]common.CoverageWorkItemID, 0, len(chunk))
				requiredKinds := []string{}
				// The compatibility partition makes the shared policy exact. Keep
				// unions below only for fields that are intentionally set-like.
				var acceptedKinds, rejectedKinds []string
				var acceptedVariants []runtimecontract.RuntimeOperationVariantRef
				var evidenceRole, targetRole, targetSelectorRef, targetSelectorOwnerRef, targetSelectorSubjectRef, prospectiveTargetRef string
				var targetRefs, targetDiscoverySources, targetSelectorCandidateRefs []string
				requireDirect, requireClaim, requireExec, requireTargets, requireTargetGrounding, requireAssessment := false, false, false, false, false, false
				texts := make([]string, 0, len(chunk))
				for _, item := range chunk {
					ids = append(ids, item.itemID)
					requiredKinds = stableStringUnion(requiredKinds, item.policy.RequiredEvidenceKinds)
					rejectedKinds = stableStringUnion(rejectedKinds, item.policy.RejectedStepKinds)
					if len(acceptedKinds) == 0 && len(item.policy.AcceptedStepKinds) > 0 {
						acceptedKinds = append([]string(nil), item.policy.AcceptedStepKinds...)
					}
					if len(acceptedVariants) == 0 && len(item.policy.AcceptedOperationVariants) > 0 {
						acceptedVariants = append([]runtimecontract.RuntimeOperationVariantRef(nil), item.policy.AcceptedOperationVariants...)
					}
					if evidenceRole == "" && item.policy.EvidenceRole != "" {
						evidenceRole = item.policy.EvidenceRole
					}
					targetRole = item.policy.TargetRole
					targetSelectorRef = item.policy.TargetSelectorRef
					targetSelectorOwnerRef = item.policy.TargetSelectorOwnerRef
					targetSelectorSubjectRef = item.policy.TargetSelectorSubjectRef
					prospectiveTargetRef = item.policy.ProspectiveTargetRef
					targetRefs = stableStringUnion(targetRefs, item.policy.TargetRefs)
					targetDiscoverySources = stableStringUnion(targetDiscoverySources, item.policy.TargetDiscoverySources)
					targetSelectorCandidateRefs = stableStringUnion(targetSelectorCandidateRefs, item.policy.TargetSelectorCandidateRefs)
					requireDirect = requireDirect || item.policy.RequireDirectEvidence
					requireClaim = requireClaim || item.policy.RequireClaimEvidence
					requireExec = requireExec || item.policy.RequireExecutableBehaviorEvidence
					requireTargets = requireTargets || item.policy.RequireCoverageTargets
					requireTargetGrounding = requireTargetGrounding || item.policy.RequireTargetGrounding
					requireAssessment = requireAssessment || item.policy.RequireAssessment
					texts = append(texts, item.unit.text)
					coverageItems = append(coverageItems, runtimecontract.RuntimeCoverageItem{
						ID: item.itemID, SliceID: sliceID, SourceScope: item.unit.sourceScope, SourceID: item.unit.sourceID,
						TargetText: item.unit.text, Order: item.unit.order,
					}.Normalized())
				}
				focusSeq++
				assessmentScope := bucket.group.Scope
				assessmentID := sliceID
				if len(chunk) == 1 {
					assessmentScope = chunk[0].unit.sourceScope
					assessmentID = chunk[0].unit.sourceID
				}
				focus := runtimecontract.CoverageFocus{
					ID: fmt.Sprintf("focus-%03d", focusSeq), SliceID: sliceID,
					AssessmentScope: assessmentScope, AssessmentID: assessmentID,
					AssessmentText: strings.Join(texts, "; "), CoverageWorkItemIDs: ids, RequiredEvidenceKinds: requiredKinds,
					EvidenceRole: evidenceRole, TargetRole: targetRole, TargetRefs: targetRefs,
					TargetDiscoverySources: targetDiscoverySources, TargetSelectorRef: targetSelectorRef, TargetSelectorOwnerRef: targetSelectorOwnerRef, TargetSelectorSubjectRef: targetSelectorSubjectRef, TargetSelectorCandidateRefs: targetSelectorCandidateRefs, ProspectiveTargetRef: prospectiveTargetRef,
					RequireTargetGrounding: requireTargetGrounding, AcceptedStepKinds: acceptedKinds,
					AcceptedOperationVariants: acceptedVariants, RejectedStepKinds: rejectedKinds,
					RequireDirectEvidence: requireDirect, RequireClaimEvidence: requireClaim,
					RequireExecutableBehaviorEvidence: requireExec, RequireCoverageTargets: requireTargets,
					RequireAssessment: requireAssessment,
				}.Normalized()
				if err := runtimecontract.ValidateRuntimeTargetGrounding(focus.RequireTargetGrounding, focus.TargetRole, focus.TargetRefs, focus.TargetDiscoverySources, focus.TargetSelectorRef); err != nil {
					return compiledCoverage{}, fmt.Errorf("compiled focus %s: %w", focus.ID, err)
				}
				focuses = append(focuses, focus)
				slices = append(slices, runtimecontract.RuntimeAcceptanceSlice{
					ID: sliceID, Title: bucket.group.Title, Scope: bucket.group.Scope, Priority: bucket.group.Priority,
					CoverageWorkItemIDs: ids, RequiredEvidenceKinds: requiredKinds,
					CompletionPolicy: slicePolicy.CompletionPolicy, ResultPolicy: slicePolicy.ResultPolicy,
				}.Normalized())
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

// compiledEvidencePolicyCompatibilityKey contains every policy field that is
// consumed by a sealed focus/authority transaction.  It deliberately excludes
// Description: prose is audit context and must not split otherwise identical
// Runtime V2 authority.
func compiledEvidencePolicyCompatibilityKey(policy compiledEvidencePolicy) string {
	policy.RequiredEvidenceKinds = stableStringUnion(policy.RequiredEvidenceKinds)
	policy.AcceptedStepKinds = stableStringUnion(policy.AcceptedStepKinds)
	policy.AcceptedOperationVariants = runtimecontract.NormalizeRuntimeOperationVariantRefs(policy.AcceptedOperationVariants)
	policy.RejectedStepKinds = stableStringUnion(policy.RejectedStepKinds)
	policy.TargetRefs = stableStringUnion(policy.TargetRefs)
	policy.TargetDiscoverySources = stableStringUnion(policy.TargetDiscoverySources)
	policy.TargetSelectorCandidateRefs = stableStringUnion(policy.TargetSelectorCandidateRefs)
	variants := make([]string, 0, len(policy.AcceptedOperationVariants))
	for _, variant := range policy.AcceptedOperationVariants {
		variants = append(variants, variant.Kind+"/"+variant.VariantID)
	}
	return strings.Join([]string{
		strings.Join(policy.RequiredEvidenceKinds, ","),
		strings.Join(policy.AcceptedStepKinds, ","),
		strings.Join(variants, ","),
		strings.Join(policy.RejectedStepKinds, ","),
		strings.TrimSpace(policy.EvidenceRole),
		strings.TrimSpace(policy.TargetRole),
		strings.Join(policy.TargetRefs, ","),
		strings.Join(policy.TargetDiscoverySources, ","),
		strings.TrimSpace(policy.TargetSelectorRef),
		strings.TrimSpace(policy.TargetSelectorOwnerRef),
		strings.TrimSpace(policy.TargetSelectorSubjectRef),
		strings.Join(policy.TargetSelectorCandidateRefs, ","),
		strings.TrimSpace(policy.ProspectiveTargetRef),
		fmt.Sprintf("%t,%t,%t,%t,%t,%t", policy.RequireTargetGrounding, policy.RequireDirectEvidence,
			policy.RequireClaimEvidence, policy.RequireExecutableBehaviorEvidence, policy.RequireCoverageTargets,
			policy.RequireAssessment),
	}, "\x1f")
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

func expandContinuationCoverageDependencies(units []workUnit, allUnits []workUnit, profiles []runtimecontract.RuntimeProfileContract) ([]workUnit, error) {
	if len(units) == 0 {
		return nil, nil
	}
	active := map[common.CoverageWorkItemID]struct{}{}
	for _, unit := range units {
		active[unit.itemID] = struct{}{}
	}
	for changed := true; changed; {
		changed = false
		for _, profile := range runtimecontract.NormalizeRuntimeProfileContracts(profiles) {
			for _, dependency := range profile.CoverageDependencies {
				sourceKnown := matchingWorkUnitsForDependencyBinding(allUnits, dependency.SourceScope, dependency.SourceRequirementID)
				dependentKnown := matchingWorkUnitsForDependencyBinding(allUnits, dependency.DependentScope, dependency.DependentRequirementID)
				if len(sourceKnown) == 0 || len(dependentKnown) == 0 {
					continue // compileRuntimeCoverageDependencies returns the exact activation error.
				}
				sourceActive := false
				for _, source := range sourceKnown {
					if _, ok := active[source.itemID]; ok {
						sourceActive = true
						break
					}
				}
				if !sourceActive {
					continue
				}
				for _, dependent := range dependentKnown {
					if _, exists := active[dependent.itemID]; exists {
						continue
					}
					active[dependent.itemID] = struct{}{}
					units = append(units, dependent)
					changed = true
				}
			}
		}
	}
	// Restored dependencies must retain definition order/identity rather than
	// the order in which graph traversal encountered them.
	sort.SliceStable(units, func(i, j int) bool { return units[i].order < units[j].order })
	return units, nil
}

func matchingWorkUnitsForDependencyBinding(units []workUnit, scope, requirementID string) []workUnit {
	scope = normalizeEvidenceScope(scope)
	requirementID = strings.TrimSpace(requirementID)
	ret := make([]workUnit, 0, len(units))
	for _, unit := range units {
		if unit.sourceScope == scope && (requirementID == "*" || unit.sourceID == requirementID) {
			ret = append(ret, unit)
		}
	}
	return ret
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

// applyContinuationToCoverageUnits filters compiled work units to unresolved coverage from the
// predecessor rejected result when definition revision is unchanged. Returns gapAnalysis=true when
// the successor must open a neutral assessment focus instead of replaying the full DoD queue.
func applyContinuationToCoverageUnits(units []workUnit, activation runtimecontract.RuntimeActivationContext) ([]workUnit, bool, error) {
	continuation := activation.Continuation.Normalized()
	// Definition-change supersede is checked first: Normalized() clears RejectedResult in that case.
	if continuation.DefinitionChangeSupersedesPrior {
		return units, true, nil
	}
	if continuation.RejectedResult == nil {
		return units, false, nil
	}
	rejected := continuation.RejectedResult.Normalized()
	unresolved := map[common.CoverageWorkItemID]struct{}{}
	for _, item := range rejected.UnsatisfiedDoDItems {
		if item.CoverageWorkItemID != "" {
			unresolved[item.CoverageWorkItemID] = struct{}{}
		}
	}
	for _, gap := range rejected.CoverageGaps {
		for _, id := range gap.CoverageWorkItemIDs {
			if id != "" {
				unresolved[id] = struct{}{}
			}
		}
	}
	if len(unresolved) == 0 {
		// Summary-only legacy rejected result: do not silently restart from first DoD.
		return units, true, nil
	}
	// Filter by stable item IDs assigned from original definition order.
	filtered := make([]workUnit, 0, len(units))
	for _, unit := range units {
		id := unit.itemID
		if id == "" {
			id = common.CoverageWorkItemID(fmt.Sprintf("cwi-%03d", unit.order))
		}
		if _, ok := unresolved[id]; ok {
			filtered = append(filtered, unit)
		}
	}
	if len(filtered) == 0 {
		// Unresolved IDs do not map onto current definition items: baseline mismatch.
		return units, true, nil
	}
	// Preserve original item IDs/order; do not renumber.
	return filtered, false, nil
}

func resolveCoverageEvidencePolicy(scope, id string, activation runtimecontract.RuntimeActivationContext, stateChangeRequired bool) (compiledEvidencePolicy, error) {
	scope = strings.TrimSpace(scope)
	id = strings.TrimSpace(id)
	if scope == "" {
		return compiledEvidencePolicy{}, fmt.Errorf("coverage evidence policy requires scope")
	}
	// Neutral runtime default: Goal/DoD/acceptance coverage requires assessment.
	// Run-level must_produce_state_change is NOT applied to every DoD focus; only
	// explicit deliverable/source-change policy or later repair elevates state_change.
	policy := compiledEvidencePolicy{RequireAssessment: true}
	switch scope {
	case "dodItem", "goalCoverage", "acceptanceDimension":
		policy.RequiredEvidenceKinds = []string{runtimecontract.EvidenceKindValidation}
	case "continuation_gap_analysis":
		// assessment-only neutral focus for structured gap rehydration
		policy.RequireAssessment = true
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
			if len(requirement.TargetSelectorCandidateRefs) > 0 && strings.TrimSpace(requirement.TargetSelectorOwnerRef) == "" {
				requirement.TargetSelectorOwnerRef = profile.ID
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
				RequiredEvidenceKinds:             append([]string(nil), requirement.RequiredEvidenceKinds...),
				RequireDirectEvidence:             requirement.RequireDirectEvidence,
				RequireClaimEvidence:              requirement.RequireClaimEvidence,
				RequireExecutableBehaviorEvidence: requirement.RequireExecutableBehaviorEvidence,
				AcceptedStepKinds:                 append([]string(nil), requirement.AcceptedStepKinds...),
				RejectedStepKinds:                 append([]string(nil), requirement.RejectedStepKinds...),
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
	return policy, nil
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
		// First non-empty highest-priority value wins for role / accepted kinds / variants.
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
		if len(base.AcceptedStepKinds) == 0 && len(requirement.AcceptedStepKinds) > 0 {
			base.AcceptedStepKinds = append([]string(nil), requirement.AcceptedStepKinds...)
		}
		if len(base.AcceptedOperationVariants) == 0 && len(requirement.AcceptedOperationVariants) > 0 {
			base.AcceptedOperationVariants = append([]runtimecontract.RuntimeOperationVariantRef(nil), requirement.AcceptedOperationVariants...)
		}
		if base.Description == "" && strings.TrimSpace(requirement.Description) != "" {
			base.Description = strings.TrimSpace(requirement.Description)
		}
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

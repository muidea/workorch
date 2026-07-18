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
	sourceScope  string
	sourceID     string
	text         string
	evidenceRole string
	order        int
	itemID       common.CoverageWorkItemID
}

// compiledEvidencePolicy is the neutral merge result for one coverage focus.
// Description is audit-only and never enters reducer authority.
type compiledEvidencePolicy struct {
	RequiredEvidenceKinds             []string
	AcceptedStepKinds                 []string
	AcceptedOperationVariants         []runtimecontract.RuntimeOperationVariantRef
	RejectedStepKinds                 []string
	EvidenceRole                      string
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

	// 1) Generate item-level work units in definition order (one item identity per DoD/profile source).
	units := make([]workUnit, 0, len(dod.Items)+8)
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
		policy, err := resolveCoverageEvidencePolicy(unit.sourceScope, unit.sourceID, activation, stateChangeRequired)
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
	slicePolicy := runtimecontract.RuntimeAcceptanceSlicePolicy{MaxWorkItems: runtimecontract.RuntimeAcceptanceSliceDefaultMaxItems}
	if activation.ExecutionContract.AcceptanceSlicePolicy != nil {
		slicePolicy = activation.ExecutionContract.AcceptanceSlicePolicy.Normalized()
		if err := slicePolicy.Validate(); err != nil {
			return compiledCoverage{}, err
		}
		// New activation must be explicit v1 when a policy object is present.
		if slicePolicy.SchemaVersion == "" {
			return compiledCoverage{}, fmt.Errorf("acceptance slice policy requires schemaVersion %s", runtimecontract.RuntimeAcceptanceSlicePolicySchemaV1)
		}
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
	focusSeq := 0
	for _, bucket := range buckets {
		maxItems := slicePolicy.EffectiveMaxWorkItems(bucket.group)
		// Split by maxItems in original item order; always keep -%03d suffix.
		ordinal := 0
		for start := 0; start < len(bucket.items); start += maxItems {
			end := start + maxItems
			if end > len(bucket.items) {
				end = len(bucket.items)
			}
			chunk := bucket.items[start:end]
			ordinal++
			sliceID := fmt.Sprintf("%s-%03d", bucket.group.ID, ordinal)
			ids := make([]common.CoverageWorkItemID, 0, len(chunk))
			requiredKinds := []string{}
			// Focus-level accepted/rejected kinds: first item's non-empty values; rejected/required kinds union.
			var acceptedKinds, rejectedKinds []string
			var acceptedVariants []runtimecontract.RuntimeOperationVariantRef
			var evidenceRole string
			requireDirect, requireClaim, requireExec, requireTargets, requireAssessment := false, false, false, false, false
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
				requireDirect = requireDirect || item.policy.RequireDirectEvidence
				requireClaim = requireClaim || item.policy.RequireClaimEvidence
				requireExec = requireExec || item.policy.RequireExecutableBehaviorEvidence
				requireTargets = requireTargets || item.policy.RequireCoverageTargets
				requireAssessment = requireAssessment || item.policy.RequireAssessment
				texts = append(texts, item.unit.text)
				coverageItems = append(coverageItems, runtimecontract.RuntimeCoverageItem{
					ID: item.itemID, SliceID: sliceID, SourceScope: item.unit.sourceScope, SourceID: item.unit.sourceID, Order: item.unit.order,
				}.Normalized())
			}
			focusSeq++
			assessmentScope := bucket.group.Scope
			assessmentID := sliceID
			// Single-item slices preserve source identity for policy/reducer continuity.
			if len(chunk) == 1 {
				assessmentScope = chunk[0].unit.sourceScope
				assessmentID = chunk[0].unit.sourceID
			}
			focus := runtimecontract.CoverageFocus{
				ID: fmt.Sprintf("focus-%03d", focusSeq), SliceID: sliceID,
				AssessmentScope: assessmentScope, AssessmentID: assessmentID,
				AssessmentText:        strings.Join(texts, "; "),
				CoverageWorkItemIDs:   ids,
				RequiredEvidenceKinds: requiredKinds,
				EvidenceRole:          evidenceRole,
				AcceptedStepKinds:     acceptedKinds, AcceptedOperationVariants: acceptedVariants, RejectedStepKinds: rejectedKinds,
				RequireDirectEvidence: requireDirect, RequireClaimEvidence: requireClaim,
				RequireExecutableBehaviorEvidence: requireExec, RequireCoverageTargets: requireTargets,
				RequireAssessment: requireAssessment,
			}.Normalized()
			focuses = append(focuses, focus)
			slices = append(slices, runtimecontract.RuntimeAcceptanceSlice{
				ID: sliceID, Title: bucket.group.Title, Scope: bucket.group.Scope, Priority: bucket.group.Priority,
				CoverageWorkItemIDs: ids, RequiredEvidenceKinds: requiredKinds,
				CompletionPolicy: slicePolicy.CompletionPolicy, ResultPolicy: slicePolicy.ResultPolicy,
			}.Normalized())
		}
	}
	if len(focuses) == 0 {
		return compiledCoverage{}, fmt.Errorf("runtime coverage focus compiler produced no focuses")
	}
	if len(focuses) > runtimecontract.RuntimeAcceptanceFocusMaxCount {
		return compiledCoverage{}, fmt.Errorf("runtime coverage focus compiler produced %d focuses, exceeding neutral limit %d; configure bounded acceptance slices", len(focuses), runtimecontract.RuntimeAcceptanceFocusMaxCount)
	}
	return compiledCoverage{Focuses: focuses, Slices: slices, CoverageItems: coverageItems}, nil
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

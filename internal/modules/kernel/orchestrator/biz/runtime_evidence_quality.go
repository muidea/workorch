package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type runtimeEvidenceQuality struct {
	Quality  common.EvidenceQuality
	Reason   string
	ZeroWork bool
	Tags     []string
}

type doDAssessmentEvidenceQualityError struct {
	cause error
	items []map[string]any
	scope string
}

type profileEvidenceKindRequirement struct {
	Scope                             string
	ID                                string
	EvidenceRole                      string
	RequiredEvidenceKinds             []string
	AcceptedStepKinds                 []string
	AcceptedOperations                []string
	RejectedStepKinds                 []string
	Description                       string
	RequireDirect                     bool
	RequireClaimEvidence              bool
	RequireExecutableBehaviorEvidence bool
}

func (e *doDAssessmentEvidenceQualityError) Error() string {
	if e == nil || e.cause == nil {
		return ""
	}
	return e.cause.Error()
}

func (e *doDAssessmentEvidenceQualityError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *doDAssessmentEvidenceQualityError) RuntimeFailureDiagnostics() map[string]any {
	if e == nil || len(e.items) == 0 {
		return nil
	}
	ret := map[string]any{
		"kind":                        "dod_evidence_quality_gap",
		"scope":                       firstNonEmptyString(e.scope, "dod_item"),
		"evidenceQualityGapItems":     e.items,
		"claimAlignedEvidenceNeeded":  evidenceQualityGapRequiresClaimAlignedEvidence(e.items),
		"stateChangingEvidenceNeeded": evidenceQualityGapRequiresStateChangingEvidence(e.items),
	}
	switch normalizeEvidenceKindRequirementScope(e.scope) {
	case "goal_coverage":
		ret["missingGoalCoverage"] = e.items
	case "deliverable":
		ret["missingDeliverableCoverage"] = e.items
	case "dod_item", "":
		ret["missingDoDItems"] = e.items
	default:
		ret["missingDoDItems"] = e.items
	}
	return ret
}

func evidenceQualityGapRequiresStateChangingEvidence(items []map[string]any) bool {
	for _, item := range items {
		if coverageWorkItemRequiresEvidenceKind(item, "state_change") {
			return true
		}
		for _, kind := range stringSliceFromValue(item["acceptedStepKinds"]) {
			if runtimeStepKindIsStateChangingEvidence(kind) {
				return true
			}
		}
		for _, operation := range stringSliceFromValue(item["acceptedOperations"]) {
			if strings.TrimSpace(strings.ToLower(operation)) == "commit" {
				return true
			}
		}
	}
	return false
}

func evidenceQualityGapRequiresClaimAlignedEvidence(items []map[string]any) bool {
	for _, item := range items {
		source := strings.TrimSpace(stringFromMap(item, "source"))
		reason := strings.ToLower(strings.TrimSpace(stringFromMap(item, "reason")))
		if source == "claimEvidenceAlignment" ||
			strings.Contains(reason, "coveragetargets") ||
			strings.Contains(reason, "evidence text") ||
			strings.Contains(reason, "aligned with the satisfied claim") {
			return true
		}
	}
	return false
}

func evidenceKindRequirementEvidenceKinds(requirement profileEvidenceKindRequirement) []string {
	if kinds := compactUniqueStrings(requirement.RequiredEvidenceKinds); len(kinds) > 0 {
		return kinds
	}
	ret := make([]string, 0, 2)
	hasValidation := requirement.RequireDirect || requirement.RequireExecutableBehaviorEvidence || len(requirement.AcceptedStepKinds) > 0
	hasStateChange := profileRequirementRequiresStateChangingEvidence(requirement)
	for _, operation := range requirement.AcceptedOperations {
		if strings.TrimSpace(strings.ToLower(operation)) == "commit" {
			hasStateChange = true
		}
	}
	if hasValidation {
		ret = append(ret, "validation")
	}
	if hasStateChange {
		ret = append(ret, "state_change")
	}
	return compactUniqueStrings(ret)
}

func runtimeStepKindIsStateChangingEvidence(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "patch/apply", "patch/preview", "file/edit", "file/write", "git/write":
		return true
	default:
		return false
	}
}

func runtimeStepEvidenceQuality(run *runModel.Run, step *runModel.Step, state common.ValidationResultState, output map[string]any) runtimeEvidenceQuality {
	if state == common.ValidationResultStateFailed {
		return runtimeEvidenceQuality{Quality: common.EvidenceQualityInvalid, Reason: "runtime step failed"}
	}
	return evidenceQualityFromRuntimeStepOutput(step, output)
}

func validateDoDAssessmentEvidenceQuality(
	ctx context.Context,
	steps repository.StepRepository,
	evidence RuntimeEvidenceRepositories,
	run *runModel.Run,
	assessment map[string]any,
) error {
	if run == nil || assessment == nil {
		return nil
	}
	if err := validateDefinitionChangeAssessmentCurrentRunEvidence(ctx, steps, evidence, run, assessment); err != nil {
		return err
	}
	if err := validateArtifactContinuationAssessmentEvidenceRefs(ctx, steps, evidence.ValidationResults, assessment); err != nil {
		return err
	}
	items, ok := doDAssessmentItemMaps(assessment["items"])
	if ok && evidence.ValidationResults != nil {
		for _, item := range items {
			if strings.TrimSpace(stringFromMap(item, "state")) != "satisfied" {
				continue
			}
			for _, ref := range evidenceRefsFromValue(item["evidenceRefs"]) {
				verdict, ok, err := evidenceRefQualityVerdictFromRepository(ctx, evidence.ValidationResults, ref)
				if err != nil {
					return err
				}
				if !ok || validationEvidenceCanSatisfyDoD(verdict) {
					continue
				}
				itemName := strings.TrimSpace(stringFromMap(item, "item"))
				reason := strings.TrimSpace(verdict.Reason)
				cause := fmt.Errorf("%w: satisfied dodAssessment item %q references %s evidence %q: %s", common.ErrInvalidArgument, itemName, verdict.Quality, ref, reason)
				return &doDAssessmentEvidenceQualityError{
					cause: cause,
					items: []map[string]any{{
						"kind":         "leaf",
						"item":         itemName,
						"state":        "failed",
						"source":       "evidenceQuality",
						"evidenceRefs": []string{ref},
						"reason":       firstNonEmptyString(reason, fmt.Sprintf("Referenced evidence quality is %s.", verdict.Quality)),
						"impact":       "This DoD item cannot be marked satisfied until concrete verification evidence is produced.",
						"suggestedNextActions": []string{
							"Add or run effective verification that exercises this DoD item.",
							"Replace weak, zero-work, or inconclusive evidence with a passing command that executes real assertions.",
						},
					}},
				}
			}
		}
	}
	if err := validateDoDAssessmentEvidenceKindRequirements(ctx, steps, evidence.ValidationResults, run, assessment); err != nil {
		return err
	}
	return nil
}

func validateArtifactContinuationAssessmentEvidenceRefs(ctx context.Context, steps repository.StepRepository, validationResults repository.ValidationResultRepository, assessment map[string]any) error {
	if assessment == nil || steps == nil {
		return nil
	}
	for _, scoped := range definitionChangeAssessmentCoverageItems(assessment) {
		if strings.ToLower(strings.TrimSpace(stringFromMap(scoped.item, "state"))) != "satisfied" {
			continue
		}
		for _, ref := range evidenceRefsFromValue(scoped.item["evidenceRefs"]) {
			step, ok, err := evidenceRefStep(ctx, steps, validationResults, ref)
			if err != nil {
				return err
			}
			if !ok || step == nil || !artifactContinuationChunkEvidenceStep(step) {
				continue
			}
			itemName := evidenceKindRequirementItemName(profileEvidenceKindRequirement{Scope: scoped.scope}, scoped.item)
			cause := fmt.Errorf("%w: satisfied %s item %q references artifact chunk evidence %q; cite artifact apply or validation evidence instead", common.ErrInvalidArgument, scoped.scope, itemName, ref)
			return &doDAssessmentEvidenceQualityError{
				cause: cause,
				scope: scoped.scope,
				items: []map[string]any{{
					"kind":         scoped.scope,
					"item":         itemName,
					"state":        "failed",
					"source":       "artifactContinuationEvidence",
					"evidenceRefs": []string{ref},
					"reason":       "Artifact chunk evidence is intermediate internal state and cannot directly satisfy acceptance.",
					"impact":       "Acceptance must be backed by state-changing artifact apply evidence or validation evidence.",
					"suggestedNextActions": []string{
						"Cite the artifact apply evidenceRef after the assembled artifact is applied.",
						"Run validation and cite the resulting validation evidenceRef.",
					},
				}},
			}
		}
	}
	return nil
}

func artifactContinuationChunkEvidenceStep(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	continuation := mapFromValue(step.Input["artifactContinuationContext"])
	if len(continuation) == 0 {
		continuation = mapFromValue(step.Output["artifactContinuationContext"])
	}
	if strings.TrimSpace(stringFromMap(continuation, "state")) == string(common.ArtifactContinuationStateChunking) {
		return true
	}
	stage := strings.TrimSpace(stringFromMap(step.Output, "stage"))
	return stage == "llm_artifact_chunk" || stage == artifactOutputPhaseChunk
}

func validateDefinitionChangeAssessmentCurrentRunEvidence(ctx context.Context, steps repository.StepRepository, evidence RuntimeEvidenceRepositories, run *runModel.Run, assessment map[string]any) error {
	if run == nil || assessment == nil || !runHasDefinitionChangeImpact(run) {
		return nil
	}
	for _, scoped := range definitionChangeAssessmentCoverageItems(assessment) {
		if strings.ToLower(strings.TrimSpace(stringFromMap(scoped.item, "state"))) != "satisfied" {
			continue
		}
		refs := evidenceRefsFromValue(scoped.item["evidenceRefs"])
		hasCurrentRunEvidence := false
		for _, ref := range refs {
			if currentRunEvidenceRefUsable(ctx, steps, evidence, run.ID, ref) {
				hasCurrentRunEvidence = true
				break
			}
		}
		if hasCurrentRunEvidence {
			continue
		}
		name := evidenceKindRequirementItemName(profileEvidenceKindRequirement{Scope: scoped.scope}, scoped.item)
		if name == "" {
			name = scoped.scope
		}
		cause := fmt.Errorf("%w: definition-changed %s item %q requires current run evidenceRefs", common.ErrInvalidArgument, scoped.scope, name)
		return &doDAssessmentEvidenceQualityError{
			cause: cause,
			items: []map[string]any{{
				"kind":         scoped.scope,
				"item":         name,
				"state":        "failed",
				"source":       "definitionChangeCurrentRunEvidence",
				"evidenceRefs": refs,
				"reason":       "The workspace Goal or DoD changed in this activation, so historical evidence can only be used as lineage and cannot by itself close the latest definition.",
				"impact":       "The latest effective Goal and DoD may not have been reprocessed or revalidated in the current run.",
				"suggestedNextActions": []string{
					"Run or produce bounded current-run evidence for this coverage item.",
					"Return failed/skipped assessment if the latest definition is not yet implemented or verified.",
				},
			}},
		}
	}
	return nil
}

type definitionChangeCoverageItem struct {
	scope string
	item  map[string]any
}

func definitionChangeAssessmentCoverageItems(assessment map[string]any) []definitionChangeCoverageItem {
	ret := make([]definitionChangeCoverageItem, 0)
	if items, ok := doDAssessmentItemMaps(assessment["items"]); ok {
		for _, item := range items {
			ret = append(ret, definitionChangeCoverageItem{scope: "dod_item", item: item})
		}
	}
	for _, item := range assessmentItemsForEvidenceKindRequirement("goal_coverage", assessment) {
		ret = append(ret, definitionChangeCoverageItem{scope: "goal_coverage", item: item})
	}
	for _, item := range assessmentItemsForEvidenceKindRequirement("acceptance_dimension", assessment) {
		ret = append(ret, definitionChangeCoverageItem{scope: "acceptance_dimension", item: item})
	}
	for _, item := range assessmentItemsForEvidenceKindRequirement("deliverable", assessment) {
		ret = append(ret, definitionChangeCoverageItem{scope: "deliverable", item: item})
	}
	return ret
}

func runHasDefinitionChangeImpact(run *runModel.Run) bool {
	if run == nil {
		return false
	}
	return boolValue(mapFromValue(run.ExecutionContext["definitionChangeImpact"])["changed"])
}

func currentRunEvidenceRefUsable(ctx context.Context, steps repository.StepRepository, evidence RuntimeEvidenceRepositories, runID common.RunID, ref string) bool {
	ref = strings.Trim(strings.TrimSpace(ref), "/")
	if ref == "" || runID == "" {
		return false
	}
	prefix := string(runID) + "/"
	if ref != string(runID) && !strings.HasPrefix(ref, prefix) {
		return false
	}
	if evidence.ValidationResults != nil {
		if item, err := evidence.ValidationResults.Get(ctx, common.ValidationResultID(ref)); err == nil && item != nil {
			return item.RunID == runID && validationEvidenceCanSatisfyDoD(qualityFromValidationResult(item))
		}
	}
	if ok, err := patchEvidenceRefUsable(ctx, evidence.Patches, ref); err == nil && ok {
		return true
	}
	if step, ok, err := stepByEvidenceRef(ctx, steps, ref); err == nil && ok {
		return step.RunID == runID && completionEvidenceStepFallbackUsable(step)
	}
	return false
}

func validateDoDAssessmentEvidenceKindRequirements(ctx context.Context, steps repository.StepRepository, validationResults repository.ValidationResultRepository, run *runModel.Run, assessment map[string]any) error {
	requirements := profileEvidenceKindRequirements(run)
	if len(requirements) == 0 || steps == nil {
		return nil
	}
	for _, requirement := range requirements {
		items := assessmentItemsForEvidenceKindRequirement(requirement.Scope, assessment)
		if len(items) == 0 {
			continue
		}
		for _, item := range items {
			if strings.ToLower(strings.TrimSpace(stringFromMap(item, "state"))) != "satisfied" {
				continue
			}
			if !evidenceKindRequirementMatchesItem(requirement, item) {
				continue
			}
			if err := validateEvidenceRefsStepKinds(ctx, steps, validationResults, requirement, item); err != nil {
				return err
			}
		}
	}
	return nil
}

func profileEvidenceKindRequirements(run *runModel.Run) []profileEvidenceKindRequirement {
	if run == nil || len(run.ExecutionContext) == 0 {
		return nil
	}
	return profileEvidenceKindRequirementsFromExecutionContext(run.ExecutionContext)
}

func profileEvidenceKindRequirementsFromExecutionContext(executionContext map[string]any) []profileEvidenceKindRequirement {
	if len(executionContext) == 0 {
		return nil
	}
	executionContract := mapFromValue(executionContext["executionContract"])
	profileContracts := mapFromValue(executionContract["profileContracts"])
	ret := make([]profileEvidenceKindRequirement, 0)
	for _, rawProfile := range profileContracts {
		profile := mapFromValue(rawProfile)
		for _, raw := range mapSliceFromValue(profile["evidenceRequirements"]) {
			if req, ok := profileEvidenceKindRequirementFromMap(raw); ok {
				ret = append(ret, req)
			}
		}
	}
	for _, policy := range mapSliceFromValue(executionContext["evidencePolicies"]) {
		for _, raw := range mapSliceFromValue(firstNonNil(
			policy["coverageRequirements"],
			policy["coverageBindings"],
		)) {
			if req, ok := profileEvidenceKindRequirementFromMap(raw); ok {
				ret = append(ret, req)
			}
		}
	}
	return ret
}

func profileEvidenceKindRequirementFromMap(raw map[string]any) (profileEvidenceKindRequirement, bool) {
	id := strings.TrimSpace(stringFromMap(raw, "id"))
	if id == "" {
		id = strings.TrimSpace(stringFromMap(raw, "dimension"))
	}
	scope := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(raw, "scope"),
		stringFromMap(raw, "appliesTo"),
	))
	if id == "" || scope == "" {
		return profileEvidenceKindRequirement{}, false
	}
	req := profileEvidenceKindRequirement{
		Scope:                             scope,
		ID:                                id,
		EvidenceRole:                      normalizedCoverageEvidenceRole(stringFromMap(raw, "evidenceRole")),
		RequiredEvidenceKinds:             stringSliceFromValue(raw["requiredEvidenceKinds"]),
		AcceptedStepKinds:                 stringSliceFromValue(raw["acceptedStepKinds"]),
		AcceptedOperations:                stringSliceFromValue(raw["acceptedOperations"]),
		RejectedStepKinds:                 stringSliceFromValue(raw["rejectedStepKinds"]),
		Description:                       strings.TrimSpace(stringFromMap(raw, "description")),
		RequireDirect:                     boolValue(raw["requireDirectEvidence"]),
		RequireClaimEvidence:              boolValue(raw["requireClaimEvidence"]) || boolValue(raw["requireClaimEvidenceAlignment"]),
		RequireExecutableBehaviorEvidence: boolValue(raw["requireExecutableBehaviorEvidence"]) || boolValue(raw["requireRuntimeBehaviorEvidence"]),
	}
	if len(req.RequiredEvidenceKinds) == 0 &&
		len(req.AcceptedStepKinds) == 0 &&
		len(req.AcceptedOperations) == 0 &&
		len(req.RejectedStepKinds) == 0 &&
		!req.RequireDirect &&
		!req.RequireClaimEvidence &&
		!req.RequireExecutableBehaviorEvidence {
		return profileEvidenceKindRequirement{}, false
	}
	return req, true
}

func assessmentItemsForEvidenceKindRequirement(scope string, assessment map[string]any) []map[string]any {
	switch normalizeEvidenceKindRequirementScope(scope) {
	case "acceptance_dimension":
		return acceptanceDimensionItemsFromAssessment(assessment)
	case "deliverable":
		return deliverableCoverageItemsFromAssessment(assessment)
	case "goal_coverage":
		coverage := mapFromValue(assessment["goalCoverage"])
		if len(coverage) == 0 {
			coverage = mapFromValue(assessment["goalOutcomeCoverage"])
		}
		return mapSliceFromValue(coverage["items"])
	case "dod_item":
		items, ok := doDAssessmentItemMaps(assessment["items"])
		if !ok {
			return nil
		}
		return items
	default:
		return nil
	}
}

func normalizeEvidenceKindRequirementScope(scope string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	scope = strings.ReplaceAll(scope, "-", "_")
	scope = strings.ReplaceAll(scope, " ", "_")
	switch scope {
	case "acceptance_dimension", "acceptance_dimensions", "dimension", "dimension_coverage", "acceptancedimension":
		return "acceptance_dimension"
	case "acceptancedimensions", "dimensioncoverage":
		return "acceptance_dimension"
	case "deliverable", "deliverables", "deliverable_coverage", "produced_artifacts":
		return "deliverable"
	case "deliverablecoverage", "producedartifacts":
		return "deliverable"
	case "goal", "goal_leaf", "goal_leaves", "goal_coverage", "goalcoverage", "goal_outcome_coverage", "goaloutcomecoverage":
		return "goal_coverage"
	case "dod", "dod_item", "doditem", "items", "assessment_item", "assessmentitem":
		return "dod_item"
	case "dodassessment":
		return "dod_item"
	default:
		return scope
	}
}

func evidenceKindRequirementMatchesItem(requirement profileEvidenceKindRequirement, item map[string]any) bool {
	want := strings.TrimSpace(requirement.ID)
	if want == "" {
		return false
	}
	if want == "*" || strings.EqualFold(want, "all") {
		return true
	}
	for _, value := range evidenceKindRequirementItemAliases(item) {
		if value == want || strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

func evidenceKindRequirementItemAliases(item map[string]any) []string {
	if len(item) == 0 {
		return nil
	}
	ret := make([]string, 0)
	seen := map[string]bool{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
		if base, _ := coverageTargetScopeAnnotatedText(value); base != "" && !seen[base] {
			seen[base] = true
			ret = append(ret, base)
		}
	}
	for _, key := range []string{
		"id",
		"dimension",
		"key",
		"name",
		"deliverable",
		"path",
		"goal",
		"goalItem",
		"item",
		"raw",
		"requirement",
		"sourceRequirementId",
		"targetText",
		"coverageTargetToken",
	} {
		add(stringFromMap(item, key))
	}
	for _, value := range stringSliceFromValue(item["sourceRequirementAliases"]) {
		add(value)
	}
	return ret
}

func coverageTargetScopeAnnotatedText(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	type bracket struct {
		open  string
		close string
	}
	for _, pair := range []bracket{{open: "（", close: "）"}, {open: "(", close: ")"}} {
		if !strings.HasSuffix(value, pair.close) {
			continue
		}
		idx := strings.LastIndex(value, pair.open)
		if idx <= 0 {
			continue
		}
		base := strings.TrimSpace(value[:idx])
		scopeText := strings.TrimSpace(strings.TrimSuffix(value[idx+len(pair.open):], pair.close))
		scope := normalizeEvidenceKindRequirementScope(scopeText)
		if base == "" || !knownEvidenceKindRequirementScope(scope) {
			continue
		}
		return base, scope
	}
	return value, ""
}

func knownEvidenceKindRequirementScope(scope string) bool {
	switch normalizeEvidenceKindRequirementScope(scope) {
	case "acceptance_dimension", "deliverable", "goal_coverage", "dod_item":
		return true
	default:
		return false
	}
}

func validateEvidenceRefsStepKinds(ctx context.Context, steps repository.StepRepository, validationResults repository.ValidationResultRepository, requirement profileEvidenceKindRequirement, item map[string]any) error {
	refs := evidenceRefsFromValue(item["evidenceRefs"])
	if len(refs) == 0 {
		if requirement.RequireDirect {
			itemName := evidenceKindRequirementItemName(requirement, item)
			return evidenceKindRequirementError(requirement, itemName, "", "", "requires at least one traceable direct evidenceRef")
		}
		return nil
	}
	accepted := mapStringSet(requirement.AcceptedStepKinds)
	acceptedOperations := mapStringSet(lowerStringSlice(requirement.AcceptedOperations))
	rejected := mapStringSet(requirement.RejectedStepKinds)
	hasAcceptedRequirement := len(accepted) > 0
	hasAcceptedEvidence := false
	knownEvidenceRefs := 0
	rejectedRefs := make([]string, 0)
	rejectedKind := ""
	rejectedReason := ""
	acceptedKindWrongOperationRefs := make([]string, 0)
	acceptedWeakRefs := make([]string, 0)
	acceptedSteps := make([]*runModel.Step, 0)
	itemName := evidenceKindRequirementItemName(requirement, item)
	for _, ref := range refs {
		step, ok, err := evidenceRefStep(ctx, steps, validationResults, ref)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		kind := strings.TrimSpace(step.Kind)
		knownEvidenceRefs++
		if rejected[kind] {
			rejectedRefs = append(rejectedRefs, ref)
			if rejectedKind == "" {
				rejectedKind = kind
			}
		}
		if accepted[kind] {
			if reason := behaviorEvidenceRejectReason(requirement, itemName, step); reason != "" {
				rejectedRefs = append(rejectedRefs, ref)
				if rejectedKind == "" {
					rejectedKind = kind
				}
				if rejectedReason == "" {
					rejectedReason = reason
				}
				continue
			}
			if len(acceptedOperations) > 0 {
				operation := stepEvidenceOperation(step)
				if !acceptedOperations[operation] {
					acceptedKindWrongOperationRefs = append(acceptedKindWrongOperationRefs, ref)
					continue
				}
			}
			if !validationEvidenceCanSatisfyDoD(evidenceQualityFromRuntimeStep(step)) {
				acceptedWeakRefs = append(acceptedWeakRefs, ref)
				continue
			}
			hasAcceptedEvidence = true
			acceptedSteps = append(acceptedSteps, step)
		}
	}
	if requirement.RequireDirect && knownEvidenceRefs == 0 {
		return evidenceKindRequirementError(requirement, itemName, strings.Join(refs, ", "), "", "requires at least one traceable direct evidenceRef")
	}
	if !(hasAcceptedRequirement && hasAcceptedEvidence) {
		if len(rejectedRefs) > 0 {
			reason := rejectedReason
			if reason == "" {
				reason = fmt.Sprintf("step kind %q is not sufficient for this evidence requirement", rejectedKind)
				if hasAcceptedRequirement {
					reason = fmt.Sprintf("step kind %q is not sufficient by itself for this evidence requirement", rejectedKind)
				}
			}
			return evidenceKindRequirementError(requirement, itemName, strings.Join(rejectedRefs, ", "), rejectedKind, reason)
		}
		if hasAcceptedRequirement && knownEvidenceRefs > 0 && !hasAcceptedEvidence {
			if len(acceptedKindWrongOperationRefs) > 0 && len(acceptedOperations) > 0 {
				return evidenceKindRequirementError(requirement, itemName, strings.Join(acceptedKindWrongOperationRefs, ", "), "", fmt.Sprintf("requires evidence operation: %s", strings.Join(requirement.AcceptedOperations, ", ")))
			}
			if len(acceptedWeakRefs) > 0 {
				return evidenceKindRequirementError(requirement, itemName, strings.Join(acceptedWeakRefs, ", "), "", "requires non-zero-work valid evidence from an accepted step")
			}
			return evidenceKindRequirementError(requirement, itemName, strings.Join(refs, ", "), "", fmt.Sprintf("requires evidence from step kinds: %s", strings.Join(requirement.AcceptedStepKinds, ", ")))
		}
	}
	if requirement.RequireClaimEvidence && claimEvidenceRequiresAlignment(itemName) {
		claimSteps := acceptedSteps
		if !hasAcceptedRequirement {
			for _, ref := range refs {
				step, ok, err := evidenceRefStep(ctx, steps, validationResults, ref)
				if err != nil {
					return err
				}
				if ok {
					claimSteps = append(claimSteps, step)
				}
			}
		}
		if !stepsContainClaimAlignedEvidence(itemName, claimSteps) {
			return evidenceKindRequirementError(requirement, itemName, strings.Join(refs, ", "), "", "requires evidence text or coverageTargets aligned with the satisfied claim")
		}
	}
	return nil
}

func stepEvidenceOperation(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	if operation := strings.ToLower(strings.TrimSpace(stringFromMap(step.Output, "operation"))); operation != "" {
		return operation
	}
	return strings.ToLower(strings.TrimSpace(stringFromMap(step.Input, "operation")))
}

func lowerStringSlice(values []string) []string {
	ret := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			ret = append(ret, value)
		}
	}
	return ret
}

func evidenceRefStep(ctx context.Context, steps repository.StepRepository, validationResults repository.ValidationResultRepository, ref string) (*runModel.Step, bool, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || steps == nil {
		return nil, false, nil
	}
	if step, ok, err := stepKindByRef(ctx, steps, ref); err != nil || ok {
		return step, ok, err
	}
	if validationResults != nil {
		if item, err := validationResults.Get(ctx, common.ValidationResultID(ref)); err == nil {
			if step, ok, err := stepKindByRef(ctx, steps, string(item.StepID)); err != nil || ok {
				return step, ok, err
			}
		} else if err != common.ErrNotFound {
			return nil, false, err
		}
	}
	for _, marker := range []string{"/validation/", "/tool-call/"} {
		if idx := strings.Index(ref, marker); idx > 0 {
			if step, ok, err := stepKindByRef(ctx, steps, ref[:idx]); err != nil || ok {
				return step, ok, err
			}
		}
	}
	return nil, false, nil
}

func stepKindByRef(ctx context.Context, steps repository.StepRepository, ref string) (*runModel.Step, bool, error) {
	if steps == nil {
		return nil, false, nil
	}
	step, err := steps.Get(ctx, common.StepID(ref))
	if err == nil {
		return step, true, nil
	}
	if err == common.ErrNotFound {
		return nil, false, nil
	}
	return nil, false, err
}

func evidenceKindRequirementItemName(requirement profileEvidenceKindRequirement, item map[string]any) string {
	for _, key := range []string{"id", "dimension", "key", "name", "deliverable", "path", "goal", "goalItem", "item", "requirement"} {
		if value := strings.TrimSpace(stringFromMap(item, key)); value != "" {
			return value
		}
	}
	return requirement.ID
}

func behaviorEvidenceRejectReason(requirement profileEvidenceKindRequirement, itemName string, step *runModel.Step) string {
	if step == nil || !requirement.RequireExecutableBehaviorEvidence || !evidenceRequirementTargetsRuntimeBehavior(requirement, itemName) {
		return ""
	}
	switch strings.TrimSpace(step.Kind) {
	case "patch/apply":
		return "patch/apply proves an implementation change but cannot by itself prove runtime behavior; run executable validation through the deliverable entrypoint, targeted tests, or a controlled substitute"
	case "shell/command":
		if shellStepIsStaticValidationOnly(step) {
			return "static or inspection-only evidence cannot by itself prove runtime behavior; provide executable validation through the deliverable entrypoint, targeted behavior checks, or a controlled substitute"
		}
	}
	return ""
}

func evidenceRequirementTargetsRuntimeBehavior(requirement profileEvidenceKindRequirement, itemName string) bool {
	id := strings.ToLower(strings.TrimSpace(requirement.ID))
	switch id {
	case "functional_behavior", "external_contract", "runtime_entrypoint", "persistence_or_side_effect", "error_behavior", "streaming_or_long_running", "concurrency", "quantitative_behavior":
		return true
	}
	if id != "*" && id != "all" {
		return false
	}
	return claimTextMentionsRuntimeBehavior(itemName)
}

func claimTextMentionsRuntimeBehavior(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	normalized := normalizeClaimEvidenceText(text)
	for _, marker := range []string{
		"proxy", "forward", "stream", "streaming", "token", "extract", "passthrough", "pass through",
		"error", "status", "usage", "append", "console", "concurrency", "concurrent", "race", "lock",
		"request", "response", "api", "http", "endpoint", "route", "cli", "command", "service",
		"config", "env", "persist", "audit", "side effect",
		"代理", "转发", "流式", "令牌", "提取", "透传", "错误", "状态码", "追加", "写入",
		"持久化", "并发", "竞态", "请求", "响应", "接口", "配置", "命令", "服务", "入口", "控制台",
	} {
		if strings.Contains(text, marker) || strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func shellStepIsStaticValidationOnly(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	for _, source := range []map[string]any{mapFromValue(step.Input["validationEvidence"]), mapFromValue(step.Output["validationEvidence"])} {
		if validationEvidenceDeclaresStaticOnly(source) {
			return true
		}
	}
	for _, source := range []map[string]any{step.Input, step.Output} {
		for _, tag := range stringListFromValue(source["evidenceTags"]) {
			if validationEvidenceTagDeclaresStaticOnly(tag) {
				return true
			}
		}
	}
	return false
}

func validationEvidenceDeclaresStaticOnly(evidence map[string]any) bool {
	if len(evidence) == 0 {
		return false
	}
	for _, key := range []string{"mode", "observationType", "kind"} {
		if validationEvidenceTagDeclaresStaticOnly(stringFromMap(evidence, key)) {
			return true
		}
	}
	return false
}

func validationEvidenceTagDeclaresStaticOnly(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	for _, marker := range []string{
		"static",
		"source_inspection",
		"read_only_inspection",
		"inspection_only",
		"syntax_only",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func evidenceKindRequirementError(requirement profileEvidenceKindRequirement, itemName, ref, kind, reason string) error {
	message := fmt.Sprintf("satisfied dodAssessment %s item %q references insufficient evidence", normalizeEvidenceKindRequirementScope(requirement.Scope), itemName)
	if ref != "" {
		message += fmt.Sprintf(" %q", ref)
	}
	if kind != "" {
		message += fmt.Sprintf(" with step kind %q", kind)
	}
	if reason != "" {
		message += ": " + reason
	}
	source := "evidenceKind"
	if requirement.RequireClaimEvidence && strings.Contains(strings.ToLower(reason), "aligned with the satisfied claim") {
		source = "claimEvidenceAlignment"
	}
	cause := fmt.Errorf("%w: %s", common.ErrInvalidArgument, message)
	scope := normalizeEvidenceKindRequirementScope(requirement.Scope)
	return &doDAssessmentEvidenceQualityError{
		scope: scope,
		cause: cause,
		items: []map[string]any{{
			"kind":                  "leaf",
			"item":                  itemName,
			"state":                 "failed",
			"source":                source,
			"sourceScope":           scope,
			"sourceRequirementId":   itemName,
			"targetText":            itemName,
			"evidenceRefs":          evidenceRefsFromValue([]string{ref}),
			"reason":                firstNonEmptyString(reason, requirement.Description, "Referenced evidence step kind cannot satisfy this claim."),
			"currentGap":            firstNonEmptyString(reason, requirement.Description, "Referenced evidence step kind cannot satisfy this claim."),
			"acceptedStepKinds":     compactUniqueStrings(requirement.AcceptedStepKinds),
			"acceptedOperations":    compactUniqueStrings(requirement.AcceptedOperations),
			"rejectedStepKinds":     compactUniqueStrings(requirement.RejectedStepKinds),
			"rejectedEvidenceKind":  strings.TrimSpace(kind),
			"requiredEvidenceKinds": evidenceKindRequirementEvidenceKinds(requirement),
			"impact":                "This claim cannot be marked satisfied until it is supported by evidence of an appropriate runtime action.",
			"suggestedNextActions": []string{
				"Run or produce concrete evidence matching the required runtime action.",
				"Replace read-only or unrelated evidence with a step that verifies the claimed behavior or side effect.",
				"When the cited evidence is generic, add coverageTargets or evidence text aligned with this exact claim.",
			},
		}},
	}
}

func mapStringSet(values []string) map[string]bool {
	ret := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		ret[value] = true
	}
	return ret
}

func claimEvidenceRequiresAlignment(claim string) bool {
	normalized := normalizeClaimEvidenceText(claim)
	if len([]rune(normalized)) >= 12 {
		return true
	}
	return len(claimEvidenceTokens(claim)) >= 2
}

func stepsContainClaimAlignedEvidence(claim string, steps []*runModel.Step) bool {
	for _, step := range steps {
		if step == nil {
			continue
		}
		if coverageTargetsContainClaim(claim, step.Input) || coverageTargetsContainClaim(claim, step.Output) {
			return true
		}
		if claimTextAligned(claim, runtimeStepClaimEvidenceText(step)) {
			return true
		}
	}
	return false
}

func coverageTargetsContainClaim(claim string, payload map[string]any) bool {
	for _, target := range collectCoverageTargetTexts(payload) {
		if claimTextAligned(claim, target) {
			return true
		}
	}
	return false
}

func collectCoverageTargetTexts(value any) []string {
	ret := make([]string, 0)
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, value := range typed {
				normalizedKey := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "_", ""))
				if normalizedKey == "coveragetargets" || normalizedKey == "coveragetarget" || normalizedKey == "targetcoverage" {
					ret = append(ret, textualValues(value)...)
					continue
				}
				walk(value)
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		case []map[string]any:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(value)
	return ret
}

func textualValues(value any) []string {
	switch typed := value.(type) {
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			return []string{text}
		}
	case []string:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(item); text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, textualValues(item)...)
		}
		return ret
	case map[string]any:
		ret := make([]string, 0, len(typed))
		for _, key := range []string{"id", "item", "goal", "goalItem", "requirement", "target", "text", "summary", "description"} {
			if text := strings.TrimSpace(stringFromMap(typed, key)); text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	}
	return nil
}

func runtimeStepClaimEvidenceText(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	parts := []string{string(step.ID), strings.TrimSpace(step.Kind)}
	if len(step.Input) > 0 {
		if encoded, err := json.Marshal(step.Input); err == nil {
			parts = append(parts, string(encoded))
		}
	}
	if len(step.Output) > 0 {
		if encoded, err := json.Marshal(step.Output); err == nil {
			parts = append(parts, string(encoded))
		}
	}
	return strings.Join(parts, "\n")
}

func claimTextAligned(claim, evidence string) bool {
	claimNorm := normalizeClaimEvidenceText(claim)
	evidenceNorm := normalizeClaimEvidenceText(evidence)
	if claimNorm == "" || evidenceNorm == "" {
		return false
	}
	if len([]rune(claimNorm)) >= 12 && strings.Contains(evidenceNorm, claimNorm) {
		return true
	}
	if len([]rune(evidenceNorm)) >= 12 && strings.Contains(claimNorm, evidenceNorm) {
		return true
	}
	tokens := claimEvidenceTokens(claim)
	if len(tokens) == 0 {
		return false
	}
	required := len(tokens)
	if required > 3 {
		required = 3
	}
	matched := 0
	seen := map[string]bool{}
	for _, token := range tokens {
		if seen[token] {
			continue
		}
		seen[token] = true
		if strings.Contains(evidenceNorm, token) {
			matched++
		}
	}
	return matched >= required
}

func normalizeClaimEvidenceText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	var b strings.Builder
	lastSpace := false
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func claimEvidenceTokens(text string) []string {
	normalized := normalizeClaimEvidenceText(text)
	if normalized == "" {
		return nil
	}
	stop := map[string]bool{
		"functional": true, "technical": true, "data": true, "logging": true,
		"must": true, "should": true, "with": true, "and": true, "the": true,
	}
	ret := make([]string, 0)
	for _, token := range strings.Fields(normalized) {
		if len([]rune(token)) < 3 || stop[token] {
			continue
		}
		if containsNonASCII(token) {
			continue
		}
		ret = append(ret, token)
	}
	return ret
}

func containsNonASCII(text string) bool {
	for _, r := range text {
		if r > unicode.MaxASCII {
			return true
		}
	}
	return false
}

func evidenceRefQualityVerdictFromRepository(ctx context.Context, validationResults repository.ValidationResultRepository, ref string) (runtimeEvidenceQuality, bool, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || validationResults == nil {
		return runtimeEvidenceQuality{}, false, nil
	}
	if item, err := validationResults.Get(ctx, common.ValidationResultID(ref)); err == nil {
		return qualityFromValidationResult(item), true, nil
	} else if err != common.ErrNotFound {
		return runtimeEvidenceQuality{}, false, err
	}
	items, err := validationResults.ListByStep(ctx, common.StepID(ref))
	if err != nil {
		return runtimeEvidenceQuality{}, false, err
	}
	if len(items) == 0 {
		return runtimeEvidenceQuality{}, false, nil
	}
	for _, item := range items {
		quality := qualityFromValidationResult(item)
		if !validationEvidenceCanSatisfyDoD(quality) {
			return quality, true, nil
		}
	}
	return qualityFromValidationResult(items[0]), true, nil
}

func evidenceQualityFromRuntimeOutput(output map[string]any) runtimeEvidenceQuality {
	if output == nil {
		return runtimeEvidenceQuality{Quality: common.EvidenceQualityValid}
	}
	quality := common.EvidenceQuality(strings.TrimSpace(stringFromMap(output, "evidenceQuality")))
	if quality == "" {
		quality = common.EvidenceQualityValid
	}
	return runtimeEvidenceQuality{
		Quality:  quality,
		Reason:   strings.TrimSpace(stringFromMap(output, "evidenceReason")),
		ZeroWork: boolFromValue(output["zeroWork"]),
		Tags:     stringSliceFromValue(output["evidenceTags"]),
	}
}

func evidenceQualityFromRuntimeStep(step *runModel.Step) runtimeEvidenceQuality {
	if step == nil {
		return runtimeEvidenceQuality{Quality: common.EvidenceQualityInvalid, Reason: "runtime step is missing"}
	}
	return evidenceQualityFromRuntimeStepOutput(step, step.Output)
}

func evidenceQualityFromRuntimeStepOutput(step *runModel.Step, output map[string]any) runtimeEvidenceQuality {
	quality := evidenceQualityFromRuntimeOutput(output)
	quality = applyRuntimeEvidenceVerdictFromOutput(quality, output)
	if step != nil && output == nil {
		output = step.Output
	}
	if validationEvidenceCanSatisfyDoD(quality) {
		if criteria := runtimeValidationCriteriaEvaluation(step, output); criteria.HasCriteria && len(criteria.Failed) > 0 {
			quality.Quality = common.EvidenceQualityInconclusive
			quality.Reason = "validation evidence criteria failed: " + strings.Join(criteria.Failed, "; ")
			quality.Tags = compactUniqueStrings(append(quality.Tags, "validation_criteria_failed"))
		}
	}
	if validationEvidenceCanSatisfyDoD(quality) && runtimeStepHasNegativeScalarValidationOutput(step) {
		quality.Quality = common.EvidenceQualityInconclusive
		quality.Reason = "validation step produced a negative scalar output"
		quality.Tags = compactUniqueStrings(append(quality.Tags, "negative_scalar_validation_output"))
	}
	if validationEvidenceCanSatisfyDoD(quality) && runtimeValidationObservedClaimsContradictedByOutput(step, output) {
		quality.Quality = common.EvidenceQualityInvalid
		quality.Reason = "validation observedClaims are contradicted by runtime output"
		quality.Tags = compactUniqueStrings(append(quality.Tags, "validation_observed_claims_contradicted"))
	}
	if validationEvidenceCanSatisfyDoD(quality) && runtimeStepHasDeliverableMutatingControlledSubstitute(step, output) {
		quality.Quality = common.EvidenceQualityInconclusive
		quality.Reason = "controlled substitute mutates a deliverable copy and cannot close current workspace validation"
		quality.Tags = compactUniqueStrings(append(quality.Tags, "controlled_substitute_deliverable_mutation"))
	}
	return quality
}

func runtimeValidationObservedClaimsContradictedByOutput(step *runModel.Step, output map[string]any) bool {
	claims := runtimeValidationObservedClaims(step, output)
	if len(claims) == 0 {
		return false
	}
	transcript := strings.ToLower(strings.Join([]string{
		stringFromMap(output, "stdout"),
		stringFromMap(output, "stderr"),
		stringFromMap(output, "output"),
		stringFromMap(output, "stdoutPreview"),
		stringFromMap(output, "stderrPreview"),
	}, "\n"))
	if strings.TrimSpace(transcript) == "" {
		return false
	}
	for _, claim := range claims {
		if absenceClaimContradictedByPresenceOutput(claim, transcript) {
			return true
		}
	}
	return false
}

func runtimeValidationObservedClaims(step *runModel.Step, output map[string]any) []string {
	var claims []string
	for _, source := range []map[string]any{
		mapFromValue(output["validationEvidence"]),
		output,
	} {
		claims = append(claims, stringSliceFromValue(source["observedClaims"])...)
		claims = append(claims, stringSliceFromValue(source["claims"])...)
	}
	if step != nil {
		for _, source := range []map[string]any{
			mapFromValue(step.Input["validationEvidence"]),
			mapFromValue(step.Output["validationEvidence"]),
			step.Input,
			step.Output,
		} {
			claims = append(claims, stringSliceFromValue(source["observedClaims"])...)
			claims = append(claims, stringSliceFromValue(source["claims"])...)
		}
	}
	return compactUniqueStrings(claims)
}

func absenceClaimContradictedByPresenceOutput(claim string, transcript string) bool {
	claim = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(claim)), " "))
	if claim == "" || strings.TrimSpace(transcript) == "" {
		return false
	}
	if !(strings.Contains(claim, "clean") ||
		strings.Contains(claim, "no change") ||
		strings.Contains(claim, "no uncommitted") ||
		strings.Contains(claim, "without uncommitted") ||
		strings.Contains(claim, "no pending") ||
		strings.Contains(claim, "nothing to commit")) {
		return false
	}
	for _, indicator := range []string{
		"modified:",
		"untracked",
		"changes not staged",
		"changes to be committed",
		"no changes added",
		"pending changes",
		"dirty",
	} {
		if strings.Contains(transcript, indicator) {
			return true
		}
	}
	return false
}

type runtimeValidationCriteriaResult struct {
	HasCriteria bool
	Failed      []string
}

type runtimeValidationMetricSourceSet struct {
	All               map[string]any
	StdoutKeyValue    map[string]any
	OutputMetrics     map[string]any
	ValidationMetrics map[string]any
	VerdictMetrics    map[string]any
	AdapterMetrics    map[string]any
}

func applyRuntimeEvidenceVerdictFromOutput(quality runtimeEvidenceQuality, output map[string]any) runtimeEvidenceQuality {
	if output == nil || !validationEvidenceCanSatisfyDoD(quality) {
		return quality
	}
	verdict := firstEvidenceVerdictMap(output)
	if len(verdict) == 0 {
		return quality
	}
	quality.Tags = compactUniqueStrings(append(quality.Tags, stringSliceFromValue(verdict["tags"])...))
	quality.Tags = compactUniqueStrings(append(quality.Tags, stringSliceFromValue(verdict["evidenceTags"])...))
	if reason := firstNonEmptyString(stringFromMap(verdict, "reason"), stringFromMap(verdict, "message")); reason != "" {
		quality.Reason = reason
	}
	if verdictQuality := common.EvidenceQuality(strings.TrimSpace(firstNonEmptyString(stringFromMap(verdict, "quality"), stringFromMap(verdict, "evidenceQuality")))); verdictQuality != "" {
		quality.Quality = verdictQuality
	}
	state := strings.ToLower(strings.TrimSpace(firstNonEmptyString(stringFromMap(verdict, "state"), stringFromMap(verdict, "status"))))
	failedCriteria := stringSliceFromValue(verdict["failedCriteria"])
	for _, item := range mapSliceFromValue(verdict["failedCriteria"]) {
		failedCriteria = append(failedCriteria, runtimeValidationCriteriaLabel(item))
	}
	failedCriteria = compactUniqueStrings(failedCriteria)
	switch {
	case len(failedCriteria) > 0:
		quality.Quality = common.EvidenceQualityInconclusive
		quality.Reason = firstNonEmptyString(quality.Reason, "validation evidence verdict reported failed criteria: "+strings.Join(failedCriteria, "; "))
		quality.Tags = compactUniqueStrings(append(quality.Tags, "validation_criteria_failed"))
	case state == "failed" || state == "failure" || state == "invalid":
		quality.Quality = common.EvidenceQualityInvalid
		quality.Reason = firstNonEmptyString(quality.Reason, "validation evidence verdict failed")
	case state == "inconclusive" || state == "unknown" || state == "skipped":
		quality.Quality = common.EvidenceQualityInconclusive
		quality.Reason = firstNonEmptyString(quality.Reason, "validation evidence verdict is inconclusive")
	}
	return quality
}

func firstEvidenceVerdictMap(output map[string]any) map[string]any {
	for _, key := range []string{"validationEvidenceVerdict", "evidenceVerdict", "verdict"} {
		if verdict := mapFromValue(output[key]); len(verdict) > 0 {
			return verdict
		}
	}
	return nil
}

func runtimeValidationCriteriaEvaluation(step *runModel.Step, output map[string]any) runtimeValidationCriteriaResult {
	evidence := runtimeValidationEvidenceForCriteria(step, output)
	requiredMetrics := criteriaMapSliceFromValue(firstNonNil(evidence["requiredMetrics"], outputValue(output, "requiredMetrics")))
	requiredAssertions := criteriaMapSliceFromValue(firstNonNil(evidence["requiredAssertions"], outputValue(output, "requiredAssertions")))
	successCriteria := criteriaMapSliceFromValue(firstNonNil(evidence["successCriteria"], outputValue(output, "successCriteria")))
	for _, policy := range runtimeValidationEvidencePoliciesForCriteria(step, output) {
		requiredMetrics = append(requiredMetrics, criteriaMapSliceFromValue(policy["requiredMetrics"])...)
		requiredAssertions = append(requiredAssertions, criteriaMapSliceFromValue(policy["requiredAssertions"])...)
		successCriteria = append(successCriteria, criteriaMapSliceFromValue(policy["successCriteria"])...)
	}
	result := runtimeValidationCriteriaResult{
		HasCriteria: len(requiredMetrics) > 0 || len(requiredAssertions) > 0 || len(successCriteria) > 0,
	}
	if !result.HasCriteria {
		return result
	}
	metrics := collectRuntimeValidationMetricSources(step, output, evidence)
	for _, metric := range requiredMetrics {
		if reason := evaluateRuntimeValidationMetricCriterion(metric, metrics); reason != "" {
			result.Failed = append(result.Failed, reason)
		}
	}
	for _, assertion := range requiredAssertions {
		if reason := evaluateRuntimeValidationAssertionCriterion(assertion); reason != "" {
			result.Failed = append(result.Failed, reason)
		}
	}
	for _, criterion := range successCriteria {
		if reason := evaluateRuntimeValidationSuccessCriterion(criterion, metrics); reason != "" {
			result.Failed = append(result.Failed, reason)
		}
	}
	result.Failed = compactUniqueStrings(result.Failed)
	return result
}

func runtimeValidationEvidenceForCriteria(step *runModel.Step, output map[string]any) map[string]any {
	if output != nil {
		if evidence := mapFromValue(output["validationEvidence"]); len(evidence) > 0 {
			return evidence
		}
	}
	if step == nil {
		return nil
	}
	if evidence := mapFromValue(step.Output["validationEvidence"]); len(evidence) > 0 {
		return evidence
	}
	return mapFromValue(step.Input["validationEvidence"])
}

func runtimeValidationEvidencePoliciesForCriteria(step *runModel.Step, output map[string]any) []map[string]any {
	policies := make([]map[string]any, 0)
	if output != nil {
		policies = append(policies, mapSliceFromValue(output["evidencePolicies"])...)
	}
	if step != nil {
		policies = append(policies, mapSliceFromValue(step.Output["evidencePolicies"])...)
		policies = append(policies, mapSliceFromValue(step.Input["evidencePolicies"])...)
	}
	if len(policies) == 0 {
		return nil
	}
	stepKind := ""
	if step != nil {
		stepKind = strings.TrimSpace(step.Kind)
	}
	ret := make([]map[string]any, 0, len(policies))
	for _, policy := range policies {
		if !runtimeValidationEvidencePolicyAppliesToStep(policy, stepKind) {
			continue
		}
		if len(criteriaMapSliceFromValue(policy["requiredMetrics"])) == 0 &&
			len(criteriaMapSliceFromValue(policy["requiredAssertions"])) == 0 &&
			len(criteriaMapSliceFromValue(policy["successCriteria"])) == 0 {
			continue
		}
		ret = append(ret, policy)
	}
	return ret
}

func runtimeValidationEvidencePolicyAppliesToStep(policy map[string]any, stepKind string) bool {
	if len(policy) == 0 {
		return false
	}
	appliesTo := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(policy, "appliesTo"),
		stringFromMap(policy, "stepKind"),
		stringFromMap(policy, "kind"),
	))
	if appliesTo == "" || appliesTo == "*" || strings.EqualFold(appliesTo, "all") {
		return true
	}
	if appliesTo == stepKind {
		return true
	}
	for _, value := range stringSliceFromValue(policy["appliesTo"]) {
		if strings.TrimSpace(value) == stepKind || strings.TrimSpace(value) == "*" {
			return true
		}
	}
	for _, value := range stringSliceFromValue(policy["acceptedStepKinds"]) {
		if strings.TrimSpace(value) == stepKind || strings.TrimSpace(value) == "*" {
			return true
		}
	}
	return false
}

func outputValue(output map[string]any, key string) any {
	if output == nil {
		return nil
	}
	return output[key]
}

func criteriaMapSliceFromValue(value any) []map[string]any {
	if single := mapFromValue(value); len(single) > 0 {
		return []map[string]any{single}
	}
	return mapSliceFromValue(value)
}

func collectRuntimeValidationMetricSources(step *runModel.Step, output map[string]any, evidence map[string]any) runtimeValidationMetricSourceSet {
	stdout := ""
	if output != nil {
		stdout = stringFromMap(output, "stdout")
	}
	if stdout == "" && step != nil {
		stdout = stringFromMap(step.Output, "stdout")
	}
	sources := runtimeValidationMetricSourceSet{
		All:               map[string]any{},
		StdoutKeyValue:    parseRuntimeValidationStdoutMetrics(stdout),
		OutputMetrics:     mapFromValue(outputValue(output, "metrics")),
		ValidationMetrics: mapFromValue(evidence["metrics"]),
		VerdictMetrics:    mapFromValue(firstEvidenceVerdictMap(output)["metrics"]),
		AdapterMetrics:    mapFromValue(outputValue(output, "adapterMetrics")),
	}
	for _, metrics := range []map[string]any{
		sources.ValidationMetrics,
		sources.OutputMetrics,
		sources.VerdictMetrics,
		sources.AdapterMetrics,
		sources.StdoutKeyValue,
	} {
		for key, value := range metrics {
			if strings.TrimSpace(key) != "" {
				sources.All[key] = value
			}
		}
	}
	return sources
}

func parseRuntimeValidationStdoutMetrics(stdout string) map[string]any {
	ret := map[string]any{}
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Count(line, "=") != 1 {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		if !runtimeValidationMetricNameIsSafe(key) {
			continue
		}
		ret[key] = strings.TrimSpace(parts[1])
	}
	return ret
}

func runtimeValidationMetricNameIsSafe(name string) bool {
	if name == "" || len(name) > 120 {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func evaluateRuntimeValidationMetricCriterion(criterion map[string]any, sources runtimeValidationMetricSourceSet) string {
	name := strings.TrimSpace(firstNonEmptyString(stringFromMap(criterion, "name"), stringFromMap(criterion, "key"), stringFromMap(criterion, "metric")))
	if name == "" {
		return "metric criterion is missing name"
	}
	actual, ok := runtimeValidationMetricValue(name, stringFromMap(criterion, "source"), sources)
	if !ok {
		return fmt.Sprintf("metric %q is missing", name)
	}
	op := strings.ToLower(strings.TrimSpace(firstNonEmptyString(stringFromMap(criterion, "op"), stringFromMap(criterion, "operator"))))
	if op == "" {
		op = "exists"
	}
	if runtimeValidationCriterionMatches(op, actual, criterion["value"]) {
		return ""
	}
	return fmt.Sprintf("metric %q did not satisfy %s", name, op)
}

func runtimeValidationMetricValue(name string, source string, sources runtimeValidationMetricSourceSet) (any, bool) {
	source = strings.ToLower(strings.TrimSpace(source))
	source = strings.ReplaceAll(source, "-", "_")
	source = strings.ReplaceAll(source, ".", "_")
	candidates := []map[string]any{sources.All}
	switch source {
	case "stdout", "stdout_key_value", "stdout_key_values", "stdout_metrics":
		candidates = []map[string]any{sources.StdoutKeyValue}
	case "output_metrics", "metrics", "output":
		candidates = []map[string]any{sources.OutputMetrics}
	case "validation_evidence_metrics", "validation_metrics", "validationevidence_metrics":
		candidates = []map[string]any{sources.ValidationMetrics}
	case "verdict", "verdict_metrics":
		candidates = []map[string]any{sources.VerdictMetrics}
	case "adapter", "adapter_metrics":
		candidates = []map[string]any{sources.AdapterMetrics}
	}
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		value, ok := candidate[name]
		if ok {
			return value, true
		}
	}
	return nil, false
}

func evaluateRuntimeValidationAssertionCriterion(criterion map[string]any) string {
	label := runtimeValidationCriteriaLabel(criterion)
	if expected, ok := criterion["expected"].(bool); ok {
		actual, actualOK := runtimeValidationAssertionBoolValue(criterion)
		if !actualOK {
			return fmt.Sprintf("assertion %q is missing actual result", label)
		}
		if actual != expected {
			return fmt.Sprintf("assertion %q did not satisfy expected result", label)
		}
		return ""
	}
	if passed, ok := criterion["passed"].(bool); ok && !passed {
		return fmt.Sprintf("assertion %q failed", label)
	}
	if state := strings.ToLower(strings.TrimSpace(firstNonEmptyString(stringFromMap(criterion, "state"), stringFromMap(criterion, "status")))); state == "failed" || state == "invalid" || state == "inconclusive" {
		return fmt.Sprintf("assertion %q is %s", label, state)
	}
	return ""
}

func runtimeValidationAssertionBoolValue(criterion map[string]any) (bool, bool) {
	for _, key := range []string{"actual", "value", "observed", "passed", "result"} {
		if value, ok := criterion[key].(bool); ok {
			return value, true
		}
	}
	return false, false
}

func evaluateRuntimeValidationSuccessCriterion(criterion map[string]any, sources runtimeValidationMetricSourceSet) string {
	if strings.TrimSpace(firstNonEmptyString(stringFromMap(criterion, "name"), stringFromMap(criterion, "key"), stringFromMap(criterion, "metric"))) != "" {
		return evaluateRuntimeValidationMetricCriterion(criterion, sources)
	}
	return evaluateRuntimeValidationAssertionCriterion(criterion)
}

func runtimeValidationCriteriaLabel(criterion map[string]any) string {
	label := strings.TrimSpace(firstNonEmptyString(stringFromMap(criterion, "id"), stringFromMap(criterion, "name"), stringFromMap(criterion, "key"), stringFromMap(criterion, "description")))
	if label == "" {
		return "unnamed"
	}
	return label
}

func runtimeValidationCriterionMatches(op string, actual any, expected any) bool {
	switch op {
	case "exists":
		return true
	case "eq", "equals", "==":
		return runtimeValidationCompareEqual(actual, expected)
	case "ne", "not_equals", "!=":
		return !runtimeValidationCompareEqual(actual, expected)
	case "gt", ">":
		return runtimeValidationCompareNumeric(actual, expected, func(a, b float64) bool { return a > b })
	case "gte", ">=", "ge":
		return runtimeValidationCompareNumeric(actual, expected, func(a, b float64) bool { return a >= b })
	case "lt", "<":
		return runtimeValidationCompareNumeric(actual, expected, func(a, b float64) bool { return a < b })
	case "lte", "<=", "le":
		return runtimeValidationCompareNumeric(actual, expected, func(a, b float64) bool { return a <= b })
	case "nonzero", "not_zero":
		return runtimeValidationValueIsNonZero(actual)
	case "true":
		return runtimeValidationValueIsBool(actual, true)
	case "false":
		return runtimeValidationValueIsBool(actual, false)
	default:
		return false
	}
}

func runtimeValidationCompareEqual(actual any, expected any) bool {
	if actualNumber, ok := runtimeValidationFloatValue(actual); ok {
		if expectedNumber, expectedOK := runtimeValidationFloatValue(expected); expectedOK {
			return actualNumber == expectedNumber
		}
	}
	return strings.TrimSpace(fmt.Sprint(actual)) == strings.TrimSpace(fmt.Sprint(expected))
}

func runtimeValidationCompareNumeric(actual any, expected any, compare func(float64, float64) bool) bool {
	actualNumber, ok := runtimeValidationFloatValue(actual)
	if !ok {
		return false
	}
	expectedNumber, ok := runtimeValidationFloatValue(expected)
	if !ok {
		return false
	}
	return compare(actualNumber, expectedNumber)
}

func runtimeValidationFloatValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case json.Number:
		ret, err := typed.Float64()
		return ret, err == nil
	case string:
		ret, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return ret, err == nil
	default:
		return 0, false
	}
}

func runtimeValidationValueIsNonZero(value any) bool {
	if number, ok := runtimeValidationFloatValue(value); ok {
		return number != 0
	}
	text := strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
	return text != "" && text != "false" && text != "null" && text != "[]" && text != "{}" && text != "0"
}

func runtimeValidationValueIsBool(value any, expected bool) bool {
	if typed, ok := value.(bool); ok {
		return typed == expected
	}
	text := strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
	if expected {
		return text == "true" || text == "1" || text == "yes" || text == "pass" || text == "passed"
	}
	return text == "false" || text == "0" || text == "no" || text == "fail" || text == "failed"
}

func qualityFromValidationResult(item *runModel.ValidationResult) runtimeEvidenceQuality {
	if item == nil {
		return runtimeEvidenceQuality{Quality: common.EvidenceQualityInvalid, Reason: "validation result is missing"}
	}
	quality := item.EvidenceQuality
	if quality == "" {
		if item.State == common.ValidationResultStateFailed {
			quality = common.EvidenceQualityInvalid
		} else {
			quality = common.EvidenceQualityValid
		}
	}
	return runtimeEvidenceQuality{
		Quality:  quality,
		Reason:   item.EvidenceReason,
		ZeroWork: item.ZeroWork,
		Tags:     append([]string(nil), item.EvidenceTags...),
	}
}

func runtimeStepHasNegativeScalarValidationOutput(step *runModel.Step) bool {
	if step == nil || strings.TrimSpace(step.Kind) != "shell/command" {
		return false
	}
	stdout := strings.ToLower(strings.TrimSpace(stringFromMap(step.Output, "stdout")))
	switch stdout {
	case "0", "false", "null", "[]", "{}":
		return true
	default:
		return false
	}
}

func runtimeStepHasDeliverableMutatingControlledSubstitute(step *runModel.Step, output map[string]any) bool {
	if step == nil || strings.TrimSpace(step.Kind) != "shell/command" {
		return false
	}
	evidence := runtimeValidationEvidenceForCriteria(step, output)
	if !validationEvidenceDeclaresControlledSubstitute(evidence) {
		return false
	}
	if !shellCommandTextLooksMutating(step, output) {
		return false
	}
	return controlledSubstituteTextSuggestsDeliverablePatch(evidence)
}

func validationEvidenceDeclaresControlledSubstitute(evidence map[string]any) bool {
	mode := strings.ToLower(strings.TrimSpace(stringFromMap(evidence, "mode")))
	observation := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
		stringFromMap(evidence, "observationType"),
		stringFromMap(evidence, "observation"),
	)))
	return mode == "controlled_substitute" || observation == "controlled_substitute_observation"
}

func shellCommandTextLooksMutating(step *runModel.Step, output map[string]any) bool {
	text := strings.ToLower(strings.Join(runtimeStepCommandTextFragments(step, output), "\n"))
	if text == "" {
		return false
	}
	markers := []string{
		"sed -i", "perl -pi", "cat >", "cat <<", "tee ", ">>", " > ",
		"cp -r", "cp -a", "cp ", "mv ",
	}
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func runtimeStepCommandTextFragments(step *runModel.Step, output map[string]any) []string {
	fragments := make([]string, 0, 8)
	addPayload := func(payload map[string]any) {
		if len(payload) == 0 {
			return
		}
		if value := strings.TrimSpace(stringFromMap(payload, "command")); value != "" {
			fragments = append(fragments, value)
		}
		for _, arg := range stringSliceFromValue(payload["args"]) {
			if arg = strings.TrimSpace(arg); arg != "" {
				fragments = append(fragments, arg)
			}
		}
	}
	if step != nil {
		addPayload(step.Input)
		addPayload(step.Output)
	}
	addPayload(output)
	return fragments
}

func controlledSubstituteTextSuggestsDeliverablePatch(evidence map[string]any) bool {
	textParts := []string{
		stringFromMap(evidence, "substituteFor"),
		stringFromMap(evidence, "substitute"),
		stringFromMap(evidence, "reason"),
		stringFromMap(evidence, "evidenceReason"),
	}
	textParts = append(textParts, stringSliceFromValue(evidence["observedClaims"])...)
	textParts = append(textParts, stringSliceFromValue(evidence["claims"])...)
	text := strings.ToLower(strings.Join(textParts, "\n"))
	if text == "" {
		return false
	}
	deliverableTerms := []string{
		"deliverable", "workspace source", "workspace file", "current workspace",
		"source file", "project source",
	}
	patchTerms := []string{
		"after fixing", "after patch", "patched copy", "fixing ", "modified copy",
		"repairing ", "replacing ", "updated copy",
	}
	hasDeliverableTerm := false
	for _, term := range deliverableTerms {
		if strings.Contains(text, term) {
			hasDeliverableTerm = true
			break
		}
	}
	if !hasDeliverableTerm {
		return false
	}
	for _, term := range patchTerms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func validationEvidenceCanSatisfyDoD(item runtimeEvidenceQuality) bool {
	if item.ZeroWork {
		return false
	}
	switch item.Quality {
	case "", common.EvidenceQualityValid:
		return true
	default:
		return false
	}
}

func injectRuntimeEvidencePolicies(input map[string]any, executionContext map[string]any) {
	if input == nil || input["evidencePolicies"] != nil || executionContext == nil {
		return
	}
	policies := mapSliceFromValue(executionContext["evidencePolicies"])
	if len(policies) == 0 {
		return
	}
	input["evidencePolicies"] = policies
}

package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
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
	Scope                string
	ID                   string
	EvidenceRole         string
	AcceptedStepKinds    []string
	AcceptedOperations   []string
	RejectedStepKinds    []string
	Description          string
	RequireDirect        bool
	RequireClaimEvidence bool
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
		"kind":                       "dod_evidence_quality_gap",
		"scope":                      firstNonEmptyString(e.scope, "dod_item"),
		"evidenceQualityGapItems":    e.items,
		"claimAlignedEvidenceNeeded": evidenceQualityGapRequiresClaimAlignedEvidence(e.items),
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

func runtimeStepEvidenceQuality(run *runModel.Run, step *runModel.Step, state common.ValidationResultState, output map[string]any) runtimeEvidenceQuality {
	if state == common.ValidationResultStateFailed {
		return runtimeEvidenceQuality{Quality: common.EvidenceQualityInvalid, Reason: "runtime step failed"}
	}
	return evidenceQualityFromRuntimeOutput(output)
}

func (o *RuntimeOrchestrator) validateDoDAssessmentEvidenceQuality(ctx context.Context, run *runModel.Run, assessment map[string]any) error {
	if o == nil || run == nil || assessment == nil {
		return nil
	}
	if err := o.validateDefinitionChangeAssessmentCurrentRunEvidence(ctx, run, assessment); err != nil {
		return err
	}
	if err := o.validateArtifactContinuationAssessmentEvidenceRefs(ctx, assessment); err != nil {
		return err
	}
	items, ok := doDAssessmentItemMaps(assessment["items"])
	if ok && o.evidence.ValidationResults != nil {
		for _, item := range items {
			if strings.TrimSpace(stringFromMap(item, "state")) != "satisfied" {
				continue
			}
			for _, ref := range evidenceRefsFromValue(item["evidenceRefs"]) {
				verdict, ok, err := evidenceRefQualityVerdictFromRepository(ctx, o.evidence.ValidationResults, ref)
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
	if err := o.validateDoDAssessmentEvidenceKindRequirements(ctx, run, assessment); err != nil {
		return err
	}
	return nil
}

func (o *RuntimeOrchestrator) validateArtifactContinuationAssessmentEvidenceRefs(ctx context.Context, assessment map[string]any) error {
	if o == nil || assessment == nil || o.steps == nil {
		return nil
	}
	for _, scoped := range definitionChangeAssessmentCoverageItems(assessment) {
		if strings.ToLower(strings.TrimSpace(stringFromMap(scoped.item, "state"))) != "satisfied" {
			continue
		}
		for _, ref := range evidenceRefsFromValue(scoped.item["evidenceRefs"]) {
			step, ok, err := evidenceRefStep(ctx, o.steps, o.evidence.ValidationResults, ref)
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

func (o *RuntimeOrchestrator) validateDefinitionChangeAssessmentCurrentRunEvidence(ctx context.Context, run *runModel.Run, assessment map[string]any) error {
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
			if currentRunEvidenceRefUsable(ctx, o.steps, o.evidence, run.ID, ref) {
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

func (o *RuntimeOrchestrator) validateDoDAssessmentEvidenceKindRequirements(ctx context.Context, run *runModel.Run, assessment map[string]any) error {
	requirements := profileEvidenceKindRequirements(run)
	if len(requirements) == 0 || o == nil || o.steps == nil {
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
			if err := o.validateEvidenceRefsStepKinds(ctx, requirement, item); err != nil {
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
	if len(profileContracts) == 0 {
		return nil
	}
	ret := make([]profileEvidenceKindRequirement, 0)
	for _, rawProfile := range profileContracts {
		profile := mapFromValue(rawProfile)
		if len(profile) == 0 {
			continue
		}
		for _, raw := range mapSliceFromValue(profile["evidenceRequirements"]) {
			id := strings.TrimSpace(stringFromMap(raw, "id"))
			if id == "" {
				id = strings.TrimSpace(stringFromMap(raw, "dimension"))
			}
			scope := strings.TrimSpace(firstNonEmptyString(
				stringFromMap(raw, "scope"),
				stringFromMap(raw, "appliesTo"),
			))
			if id == "" || scope == "" {
				continue
			}
			req := profileEvidenceKindRequirement{
				Scope:                scope,
				ID:                   id,
				EvidenceRole:         normalizedCoverageEvidenceRole(stringFromMap(raw, "evidenceRole")),
				AcceptedStepKinds:    stringSliceFromValue(raw["acceptedStepKinds"]),
				AcceptedOperations:   stringSliceFromValue(raw["acceptedOperations"]),
				RejectedStepKinds:    stringSliceFromValue(raw["rejectedStepKinds"]),
				Description:          strings.TrimSpace(stringFromMap(raw, "description")),
				RequireDirect:        boolValue(raw["requireDirectEvidence"]),
				RequireClaimEvidence: boolValue(raw["requireClaimEvidence"]) || boolValue(raw["requireClaimEvidenceAlignment"]),
			}
			if len(req.AcceptedStepKinds) == 0 && len(req.AcceptedOperations) == 0 && len(req.RejectedStepKinds) == 0 && !req.RequireDirect && !req.RequireClaimEvidence {
				continue
			}
			ret = append(ret, req)
		}
	}
	return ret
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
	case "goal", "goal_coverage", "goalcoverage", "goal_outcome_coverage", "goaloutcomecoverage":
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

func (o *RuntimeOrchestrator) validateEvidenceRefsStepKinds(ctx context.Context, requirement profileEvidenceKindRequirement, item map[string]any) error {
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
	acceptedKindWrongOperationRefs := make([]string, 0)
	acceptedWeakRefs := make([]string, 0)
	acceptedSteps := make([]*runModel.Step, 0)
	itemName := evidenceKindRequirementItemName(requirement, item)
	for _, ref := range refs {
		step, ok, err := evidenceRefStep(ctx, o.steps, o.evidence.ValidationResults, ref)
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
			if len(acceptedOperations) > 0 {
				operation := stepEvidenceOperation(step)
				if !acceptedOperations[operation] {
					acceptedKindWrongOperationRefs = append(acceptedKindWrongOperationRefs, ref)
					continue
				}
			}
			if !validationEvidenceCanSatisfyDoD(evidenceQualityFromRuntimeOutput(step.Output)) {
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
			reason := fmt.Sprintf("step kind %q is not sufficient for this evidence requirement", rejectedKind)
			if hasAcceptedRequirement {
				reason = fmt.Sprintf("step kind %q is not sufficient by itself for this evidence requirement", rejectedKind)
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
		steps := acceptedSteps
		if !hasAcceptedRequirement {
			for _, ref := range refs {
				step, ok, err := evidenceRefStep(ctx, o.steps, o.evidence.ValidationResults, ref)
				if err != nil {
					return err
				}
				if ok {
					steps = append(steps, step)
				}
			}
		}
		if !stepsContainClaimAlignedEvidence(itemName, steps) {
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
	return &doDAssessmentEvidenceQualityError{
		scope: normalizeEvidenceKindRequirementScope(requirement.Scope),
		cause: cause,
		items: []map[string]any{{
			"kind":         "leaf",
			"item":         itemName,
			"state":        "failed",
			"source":       source,
			"evidenceRefs": evidenceRefsFromValue([]string{ref}),
			"reason":       firstNonEmptyString(reason, requirement.Description, "Referenced evidence step kind cannot satisfy this claim."),
			"impact":       "This claim cannot be marked satisfied until it is supported by evidence of an appropriate runtime action.",
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

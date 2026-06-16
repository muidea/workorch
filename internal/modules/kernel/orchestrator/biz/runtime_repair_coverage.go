package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func terminalAssessmentOnlyDecisionContractFailure(step *runModel.Step) bool {
	if step == nil || !isLLMDecisionContractFailureStage(stringFromMap(step.Output, "stage")) {
		return false
	}
	if assessmentOnlySchemaInvalidRepairRequired(step) {
		return false
	}
	return boolValue(step.Input["providerRetryAssessmentOnly"]) ||
		boolValue(step.Input[stepInputKeyAssessmentOnly]) ||
		boolValue(step.Input["assessmentOnlyContractCorrection"])
}

func doDCoverageContractCorrectionRequired(step *runModel.Step) bool {
	if step == nil || step.Output == nil {
		return false
	}
	if strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass)) != failureClassDoDCoverageMissing {
		return false
	}
	assessment := doDAssessmentFromOutput(step.Output)
	if assessment == nil {
		return false
	}
	satisfied, ok := assessment["satisfied"].(bool)
	if ok && satisfied {
		return true
	}
	if assessmentOnlyCoverageCorrectionFailedWithActionableGaps(step) {
		return true
	}
	return assessmentOnlyGoalCoverageContradictionRepairRequired(step)
}

func coverageCorrectionNeedsConcreteEvidence(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	if assessmentOutputContractRepairRequired(step) {
		return false
	}
	missingGoal := missingGoalCoverageFromInput(step.Input)
	if len(missingGoal) > 0 {
		for _, item := range missingGoal {
			if goalCoverageGapCanReuseSatisfiedAssessmentEvidence(step, item) {
				continue
			}
			if len(mapSliceFromValue(item["evidenceCandidates"])) == 0 {
				return true
			}
		}
	}
	missingDoD := mapSliceFromValue(step.Input["missingDoDItems"])
	if len(missingDoD) == 0 {
		missingDoD = missingDoDItemsFromStep(step)
	}
	if len(missingDoD) > 0 {
		for _, item := range missingDoD {
			if len(filterTraceableEvidenceRefs(evidenceRefsFromValue(item["evidenceRefs"]))) == 0 &&
				len(mapSliceFromValue(item["evidenceCandidates"])) == 0 {
				return true
			}
		}
	}
	return false
}

func assessmentOutputContractRepairRequired(step *runModel.Step) bool {
	if step == nil || strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass)) != failureClassDoDCoverageMissing {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		stringFromMap(step.Output, "error"),
		stringFromMap(step.Output, "reason"),
		stringFromMap(step.Output, failureOutputKeySignature),
	}, " ")))
	for _, marker := range []string{
		"dodassessment does not cover all current goal requirements",
		"dodassessment.goalcoverage item and state are required",
		"dodassessment.goalcoverage.items are required",
		"dodassessment.goalcoverage.satisfied is required",
		"satisfied dodassessment.goalcoverage item requires evidencerefs",
		"dodassessment.deliverablecoverage missing required deliverables",
		"dodassessment.deliverablecoverage item",
		"dodassessment.deliverablecoverage.items",
		"satisfied dodassessment.deliverablecoverage item",
		"dodassessment.acceptancedimensions missing required dimensions",
		"dodassessment.acceptancedimensions item",
		"dodassessment.acceptancedimensions.items",
		"satisfied dodassessment.acceptancedimensions item",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func assessmentOnlyCoverageCorrectionFailedWithActionableGaps(step *runModel.Step) bool {
	if step == nil || !boolValue(step.Input[stepInputKeyAssessmentOnly]) {
		return false
	}
	if strings.TrimSpace(stringFromMap(step.Input, "repairObjective")) != "complete_dod_goal_coverage_assessment" &&
		strings.TrimSpace(stringFromMap(step.Input, "assessmentOnlyReason")) != "dod_coverage_contract_correction" {
		return false
	}
	if strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass)) != failureClassDoDCoverageMissing {
		return false
	}
	assessment := doDAssessmentFromOutput(step.Output)
	if len(assessment) == 0 {
		return false
	}
	if satisfied, ok := assessment["satisfied"].(bool); ok && satisfied {
		return false
	}
	return len(missingDoDItemsFromStep(step)) > 0 ||
		len(missingGoalCoverageFromFailureOutput(step.Output)) > 0 ||
		len(missingDeliverableCoverageFromFailureOutput(step.Output)) > 0
}

func goalCoverageGapCanReuseSatisfiedAssessmentEvidence(step *runModel.Step, missing map[string]any) bool {
	if step == nil || len(missing) == 0 {
		return false
	}
	assessment := doDAssessmentFromOutput(step.Output)
	if len(assessment) == 0 {
		return false
	}
	target := normalizeDoDCoverageText(missingGoalCoverageRequirementText(missing))
	if target == "" {
		return false
	}
	items, ok := doDAssessmentItemMaps(assessment["items"])
	if !ok {
		return false
	}
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(stringFromMap(item, "state"))) != "satisfied" {
			continue
		}
		if len(evidenceRefsFromValue(item["evidenceRefs"])) == 0 && len(stringSliceFromValue(item["artifactRefs"])) == 0 {
			continue
		}
		itemText := normalizeDoDCoverageText(firstNonEmptyString(
			stringFromMap(item, "item"),
			stringFromMap(item, "goal"),
			stringFromMap(item, "goalItem"),
			stringFromMap(item, "requirement"),
		))
		if itemText == "" {
			continue
		}
		if strings.Contains(itemText, target) || strings.Contains(target, itemText) || coverageTextsLikelyEquivalent(itemText, target) {
			return true
		}
	}
	return false
}

func coverageTextsLikelyEquivalent(left string, right string) bool {
	left = normalizeDoDCoverageText(left)
	right = normalizeDoDCoverageText(right)
	if left == "" || right == "" {
		return false
	}
	if left == right {
		return true
	}
	leftTerms := coverageRequirementTerms(left)
	rightTerms := coverageRequirementTerms(right)
	if len(leftTerms) == 0 || len(rightTerms) == 0 {
		return false
	}
	leftSet := map[string]bool{}
	for _, term := range leftTerms {
		leftSet[term] = true
	}
	matches := 0
	for _, term := range rightTerms {
		if leftSet[term] {
			matches++
		}
	}
	minTerms := min(len(leftTerms), len(rightTerms))
	if matches > 0 && matches*2 >= minTerms {
		return true
	}
	return coverageTextsShareStrongCJKNGrams(left, right)
}

func coverageTextsShareStrongCJKNGrams(left string, right string) bool {
	if utf8.RuneCountInString(left) < 2 || utf8.RuneCountInString(right) < 2 {
		return false
	}
	leftGrams := coverageTextNGrams(left, 2)
	rightGrams := coverageTextNGrams(right, 2)
	if len(leftGrams) == 0 || len(rightGrams) == 0 {
		return false
	}
	shared := 0
	for gram := range leftGrams {
		if rightGrams[gram] {
			shared++
		}
	}
	minGrams := min(len(leftGrams), len(rightGrams))
	return shared >= 2 && shared*2 >= minGrams
}

func coverageTextNGrams(value string, size int) map[string]bool {
	runes := []rune(value)
	if len(runes) < size {
		return nil
	}
	ret := map[string]bool{}
	for idx := 0; idx+size <= len(runes); idx++ {
		gram := string(runes[idx : idx+size])
		if strings.TrimSpace(gram) == "" {
			continue
		}
		ret[gram] = true
	}
	return ret
}

func missingGoalCoverageFromFailureOutput(output map[string]any) []map[string]any {
	diagnostics := mapFromValue(output["runtimeDiagnostics"])
	items := mapSliceFromValue(diagnostics["missingGoalCoverage"])
	if len(items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		ret = append(ret, cloneStepPayload(item))
	}
	return ret
}

func missingDoDItemsFromStep(step *runModel.Step) []map[string]any {
	if step == nil {
		return nil
	}
	if missing := missingDoDItemsFromFailureOutput(step.Output); len(missing) > 0 {
		return missing
	}
	return missingDoDItemsFromDoDAssessment(doDAssessmentFromOutput(step.Output))
}

func missingDeliverableCoverageFromFailureOutput(output map[string]any) []map[string]any {
	diagnostics := mapFromValue(output["runtimeDiagnostics"])
	items := mapSliceFromValue(diagnostics["missingDeliverableCoverage"])
	if len(items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		ret = append(ret, cloneStepPayload(item))
	}
	return ret
}

func missingDeliverableCoverageFromStep(step *runModel.Step) []map[string]any {
	if step == nil {
		return nil
	}
	if missing := missingDeliverableCoverageFromFailureOutput(step.Output); len(missing) > 0 {
		return mergeMissingDeliverableCoverageInputCandidates(missing, step.Input)
	}
	if missing := missingDeliverableCoverageFromDoDAssessment(doDAssessmentFromOutput(step.Output)); len(missing) > 0 {
		return mergeMissingDeliverableCoverageInputCandidates(missing, step.Input)
	}
	return mergeMissingDeliverableCoverageInputCandidates(missingDeliverableCoverageFromInput(step.Input), step.Input)
}

func missingGoalCoverageFromStep(step *runModel.Step) []map[string]any {
	if step == nil {
		return nil
	}
	if missing := missingGoalCoverageFromFailureOutput(step.Output); len(missing) > 0 {
		return mergeMissingGoalCoverageInputCandidates(missing, step.Input)
	}
	if missing := missingGoalCoverageFromDoDAssessment(doDAssessmentFromOutput(step.Output)); len(missing) > 0 {
		return mergeMissingGoalCoverageInputCandidates(missing, step.Input)
	}
	return missingGoalCoverageFromInput(step.Input)
}

func missingGoalCoverageFromInput(input map[string]any) []map[string]any {
	items := mapSliceFromValue(input["missingGoalCoverage"])
	if len(items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		ret = append(ret, cloneStepPayload(item))
	}
	return ret
}

func missingDeliverableCoverageFromInput(input map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	return mergeMissingDeliverableCoverageInputCandidates(mapSliceFromValue(input["missingDeliverableCoverage"]), input)
}

func mergeMissingDeliverableCoverageInputCandidates(missing []map[string]any, input map[string]any) []map[string]any {
	if len(missing) == 0 {
		return nil
	}
	candidates := deliverableCoverageCandidatesFromInput(input)
	ret := make([]map[string]any, 0, len(missing))
	for _, item := range missing {
		next := cloneStepPayload(item)
		if len(candidates) > 0 {
			if candidate := matchingDeliverableCoverageCandidate(next, candidates); len(candidate) > 0 {
				for _, key := range []string{
					"evidenceRole",
					"requiredEvidenceKinds",
					"missingEvidenceKinds",
					"requiredEvidenceCounts",
					"source",
					"sourceScope",
					"sourceRequirementId",
					"sourcePath",
					"targetText",
					"coverageWorkItemId",
					"coverageTargetToken",
				} {
					if _, exists := next[key]; exists {
						continue
					}
					if value, ok := candidate[key]; ok && value != nil {
						next[key] = compactLLMContextValue(value, 1)
					}
				}
			}
		}
		ret = append(ret, next)
	}
	return ret
}

func deliverableCoverageCandidatesFromInput(input map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0)
	for _, item := range coverageWorkItemCandidateItemsFromContext(coverageWorkItemContextFromInput(input)) {
		if strings.TrimSpace(stringFromMap(item, "sourceScope")) == "deliverable" ||
			strings.TrimSpace(stringFromMap(item, "kind")) == "deliverable" {
			ret = append(ret, item)
		}
	}
	for _, item := range mapSliceFromValue(input["coverageTargetSet"]) {
		if strings.TrimSpace(stringFromMap(item, "sourceScope")) == "deliverable" ||
			strings.TrimSpace(stringFromMap(item, "kind")) == "deliverable" {
			ret = append(ret, item)
		}
	}
	return ret
}

func matchingDeliverableCoverageCandidate(item map[string]any, candidates []map[string]any) map[string]any {
	token := normalizeCoverageGapRepairToken(firstNonEmptyString(
		stringFromMap(item, "sourceRequirementId"),
		stringFromMap(item, "id"),
		stringFromMap(item, "item"),
		stringFromMap(item, "expectationId"),
		stringFromMap(item, "requirement"),
		stringFromMap(item, "targetText"),
	))
	if token == "" {
		return nil
	}
	for _, candidate := range candidates {
		for _, candidateText := range []string{
			stringFromMap(candidate, "sourceRequirementId"),
			stringFromMap(candidate, "id"),
			stringFromMap(candidate, "item"),
			stringFromMap(candidate, "coverageTargetToken"),
			stringFromMap(candidate, "targetText"),
		} {
			if normalizeCoverageGapRepairToken(candidateText) == token {
				return candidate
			}
		}
	}
	return nil
}

func assessmentOnlyGoalCoverageContradictionRepairRequired(step *runModel.Step) bool {
	if step == nil || !boolValue(step.Input[stepInputKeyAssessmentOnly]) {
		return false
	}
	if strings.TrimSpace(stringFromMap(step.Input, "repairObjective")) != "complete_dod_goal_coverage_assessment" &&
		!boolValue(step.Input[stepInputKeyGraphLimitFollowUp]) &&
		strings.TrimSpace(stringFromMap(step.Input, "assessmentOnlyReason")) != "graph_follow_up_expansion_limit_exceeded" {
		return false
	}
	if len(missingGoalCoverageFromStep(step)) == 0 {
		return false
	}
	if boolValue(step.Input[stepInputKeyGraphLimitFollowUp]) ||
		strings.TrimSpace(stringFromMap(step.Input, "assessmentOnlyReason")) == "graph_follow_up_expansion_limit_exceeded" {
		return true
	}
	return missingGoalCoverageInputHasEvidenceCandidates(step.Input)
}

func missingGoalCoverageInputHasEvidenceCandidates(input map[string]any) bool {
	for _, item := range mapSliceFromValue(input["missingGoalCoverage"]) {
		if len(mapSliceFromValue(item["evidenceCandidates"])) > 0 {
			return true
		}
	}
	return false
}

func mergeMissingGoalCoverageInputCandidates(missing []map[string]any, input map[string]any) []map[string]any {
	if len(missing) == 0 || len(input) == 0 {
		return missing
	}
	prior := missingGoalCoverageFromInput(input)
	if len(prior) == 0 {
		return missing
	}
	candidatesByKey := map[string][]map[string]any{}
	for _, item := range prior {
		candidates := mapSliceFromValue(item["evidenceCandidates"])
		if len(candidates) == 0 {
			continue
		}
		for _, key := range missingGoalCoverageMatchKeys(item) {
			candidatesByKey[key] = candidates
		}
	}
	if len(candidatesByKey) == 0 {
		return missing
	}
	for _, item := range missing {
		if len(mapSliceFromValue(item["evidenceCandidates"])) > 0 {
			continue
		}
		for _, key := range missingGoalCoverageMatchKeys(item) {
			if candidates := candidatesByKey[key]; len(candidates) > 0 {
				item["evidenceCandidates"] = candidates
				break
			}
		}
	}
	return missing
}

func missingGoalCoverageMatchKeys(item map[string]any) []string {
	if len(item) == 0 {
		return nil
	}
	ret := make([]string, 0, 2)
	if id := strings.TrimSpace(stringFromMap(item, "goalRequirementId")); id != "" {
		ret = append(ret, "id:"+id)
	}
	if normalized := normalizeDoDCoverageText(missingGoalCoverageRequirementText(item)); normalized != "" {
		ret = append(ret, "text:"+normalized)
	}
	return ret
}

func missingGoalCoverageWithEvidenceCandidates(
	ctx context.Context,
	steps repository.StepRepository,
	run *runModel.Run,
	failedStep *runModel.Step,
	missing []map[string]any,
) []map[string]any {
	if len(missing) == 0 {
		return nil
	}
	runSteps := coverageCandidateRunSteps(ctx, steps, run)
	ret := make([]map[string]any, 0, len(missing))
	for idx, item := range missing {
		next := cloneStepPayload(item)
		requirement := missingGoalCoverageRequirementText(next)
		normalized := normalizeDoDCoverageText(requirement)
		if strings.TrimSpace(stringFromMap(next, "goalRequirementId")) == "" && normalized != "" {
			next["goalRequirementId"] = goalCoverageRequirementID(idx+1, normalized)
		}
		if len(mapSliceFromValue(next["evidenceCandidates"])) == 0 {
			if candidates := goalCoverageEvidenceCandidatesForRequirementInSteps(runSteps, failedStep, requirement); len(candidates) > 0 {
				next["evidenceCandidates"] = candidates
			}
		}
		if len(mapSliceFromValue(next["evidenceCandidates"])) == 0 {
			next["evidenceCandidates"] = latestCoverageBoundActionEvidenceCandidatesInSteps(runSteps, failedStep, requirement, 5)
		}
		ret = append(ret, next)
	}
	return ret
}

func missingDoDItemsWithEvidenceCandidates(
	ctx context.Context,
	steps repository.StepRepository,
	run *runModel.Run,
	failedStep *runModel.Step,
	missing []map[string]any,
) []map[string]any {
	if len(missing) == 0 {
		return nil
	}
	runSteps := coverageCandidateRunSteps(ctx, steps, run)
	ret := make([]map[string]any, 0, len(missing))
	for _, item := range missing {
		next := cloneStepPayload(item)
		if len(mapSliceFromValue(next["evidenceCandidates"])) == 0 {
			requirement := strings.TrimSpace(stringFromMap(next, "item"))
			if candidates := goalCoverageEvidenceCandidatesForRequirementInSteps(runSteps, failedStep, requirement); len(candidates) > 0 {
				next["evidenceCandidates"] = candidates
			}
		}
		if len(mapSliceFromValue(next["evidenceCandidates"])) == 0 {
			next["evidenceCandidates"] = latestCoverageBoundActionEvidenceCandidatesInSteps(runSteps, failedStep, strings.TrimSpace(stringFromMap(next, "item")), 5)
		}
		ret = append(ret, next)
	}
	return ret
}

func missingGoalCoverageRequirementText(item map[string]any) string {
	return strings.TrimSpace(firstNonEmptyString(
		stringFromMap(item, "raw"),
		stringFromMap(item, "item"),
		stringFromMap(item, "goal"),
		stringFromMap(item, "goalItem"),
		stringFromMap(item, "requirement"),
	))
}

func goalCoverageEvidenceCandidatesForRequirement(
	ctx context.Context,
	steps repository.StepRepository,
	run *runModel.Run,
	failedStep *runModel.Step,
	requirement string,
) []map[string]any {
	return goalCoverageEvidenceCandidatesForRequirementInSteps(coverageCandidateRunSteps(ctx, steps, run), failedStep, requirement)
}

func coverageCandidateRunSteps(ctx context.Context, steps repository.StepRepository, run *runModel.Run) []*runModel.Step {
	if steps == nil || run == nil {
		return nil
	}
	runSteps, err := steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil
	}
	return runSteps
}

func goalCoverageEvidenceCandidatesForRequirementInSteps(runSteps []*runModel.Step, failedStep *runModel.Step, requirement string) []map[string]any {
	terms := coverageRequirementTerms(requirement)
	if len(terms) == 0 {
		return nil
	}
	type candidate struct {
		score int
		item  map[string]any
	}
	candidates := make([]candidate, 0)
	for _, step := range runSteps {
		if step == nil || (failedStep != nil && step.ID == failedStep.ID) {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(step.Kind), "llm/") {
			continue
		}
		if step.State != common.StepStateCompleted && step.State != common.StepStateFailed {
			continue
		}
		if !coverageEvidenceQualityCandidateAllowed(stringFromMap(step.Output, "evidenceQuality")) {
			continue
		}
		text := normalizeDoDCoverageText(strings.Join([]string{
			string(step.ID),
			step.Kind,
			string(step.State),
			flattenCoverageEvidenceText(step.Output, 0),
		}, "\n"))
		if text == "" {
			continue
		}
		matched := matchedCoverageTerms(terms, text)
		if len(matched) == 0 {
			continue
		}
		source := "runtime_step_text_match"
		evidenceQuality := "indirect"
		if stepDeclaresCoverageTarget(step, requirement) {
			source = "coverage_bound_step_text_match"
			evidenceQuality = "direct"
		}
		item := map[string]any{
			"stepId":          string(step.ID),
			"kind":            step.Kind,
			"state":           string(step.State),
			"matchedTerms":    matched,
			"matchScore":      len(matched),
			"source":          source,
			"evidenceQuality": evidenceQuality,
		}
		if nodeID := graphNodeID(step); nodeID != "" {
			item["graphNodeId"] = nodeID
		}
		if rootStepID := graphRootStepID(step); rootStepID != "" && rootStepID != step.ID {
			item["graphRootStepId"] = string(rootStepID)
		}
		if output := compactStepOutputForLLMContext(step.Output); len(output) > 0 {
			item["output"] = output
		}
		candidates = append(candidates, candidate{score: len(matched), item: item})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return stringFromMap(candidates[i].item, "stepId") < stringFromMap(candidates[j].item, "stepId")
	})
	limit := min(len(candidates), 5)
	ret := make([]map[string]any, 0, limit)
	for idx := 0; idx < limit; idx++ {
		ret = append(ret, candidates[idx].item)
	}
	return ret
}

func coverageEvidenceQualityCandidateAllowed(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "inconclusive", "invalid", "smoke", "unrelated":
		return false
	default:
		return true
	}
}

func latestActionEvidenceCandidates(
	ctx context.Context,
	steps repository.StepRepository,
	run *runModel.Run,
	failedStep *runModel.Step,
	limit int,
) []map[string]any {
	return latestActionEvidenceCandidatesInSteps(coverageCandidateRunSteps(ctx, steps, run), failedStep, limit)
}

func latestActionEvidenceCandidatesInSteps(runSteps []*runModel.Step, failedStep *runModel.Step, limit int) []map[string]any {
	if limit <= 0 || len(runSteps) == 0 {
		return nil
	}
	runSteps = orderedRuntimeContextSteps(runSteps)
	ret := make([]map[string]any, 0, limit)
	for idx := len(runSteps) - 1; idx >= 0 && len(ret) < limit; idx-- {
		step := runSteps[idx]
		if step == nil || (failedStep != nil && step.ID == failedStep.ID) {
			continue
		}
		if !completionEvidenceStepFallbackUsable(step) {
			continue
		}
		item := map[string]any{
			"stepId":     string(step.ID),
			"kind":       step.Kind,
			"state":      string(step.State),
			"source":     "latest_successful_action_evidence",
			"matchScore": 0,
		}
		if nodeID := graphNodeID(step); nodeID != "" {
			item["graphNodeId"] = nodeID
		}
		if rootStepID := graphRootStepID(step); rootStepID != "" && rootStepID != step.ID {
			item["graphRootStepId"] = string(rootStepID)
		}
		if output := compactStepOutputForLLMContext(step.Output); len(output) > 0 {
			item["output"] = output
		}
		ret = append(ret, item)
	}
	return ret
}

func latestCoverageBoundActionEvidenceCandidates(
	ctx context.Context,
	steps repository.StepRepository,
	run *runModel.Run,
	failedStep *runModel.Step,
	coverageTarget string,
	limit int,
) []map[string]any {
	return latestCoverageBoundActionEvidenceCandidatesInSteps(coverageCandidateRunSteps(ctx, steps, run), failedStep, coverageTarget, limit)
}

func latestCoverageBoundActionEvidenceCandidatesInSteps(runSteps []*runModel.Step, failedStep *runModel.Step, coverageTarget string, limit int) []map[string]any {
	if limit <= 0 || len(runSteps) == 0 {
		return nil
	}
	coverageTarget = normalizeCoverageGapRepairToken(coverageTarget)
	if coverageTarget == "" {
		return nil
	}
	runSteps = orderedRuntimeContextSteps(runSteps)
	ret := make([]map[string]any, 0, limit)
	for idx := len(runSteps) - 1; idx >= 0 && len(ret) < limit; idx-- {
		step := runSteps[idx]
		if step == nil || (failedStep != nil && step.ID == failedStep.ID) {
			continue
		}
		if !completionEvidenceStepFallbackUsable(step) {
			continue
		}
		if !stepDeclaresCoverageTarget(step, coverageTarget) {
			continue
		}
		ret = append(ret, latestActionEvidenceCandidateFromStep(step))
	}
	return ret
}

func latestActionEvidenceCandidateFromStep(step *runModel.Step) map[string]any {
	if step == nil {
		return nil
	}
	item := map[string]any{
		"stepId":          string(step.ID),
		"kind":            step.Kind,
		"state":           string(step.State),
		"source":          "latest_coverage_bound_action_evidence",
		"matchScore":      1,
		"evidenceQuality": "direct",
	}
	if nodeID := graphNodeID(step); nodeID != "" {
		item["graphNodeId"] = nodeID
	}
	if rootStepID := graphRootStepID(step); rootStepID != "" && rootStepID != step.ID {
		item["graphRootStepId"] = string(rootStepID)
	}
	if output := compactStepOutputForLLMContext(step.Output); len(output) > 0 {
		item["output"] = output
	}
	return item
}

func stepDeclaresCoverageTarget(step *runModel.Step, coverageTarget string) bool {
	if step == nil || coverageTarget == "" {
		return false
	}
	for _, value := range coverageTargetTokensFromStep(step) {
		value = normalizeCoverageGapRepairToken(value)
		if value == "" {
			continue
		}
		if value == coverageTarget || coverageTextsLikelyEquivalent(value, coverageTarget) {
			return true
		}
	}
	return false
}

func coverageTargetTokensFromStep(step *runModel.Step) []string {
	if step == nil {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := normalizeCoverageGapRepairToken(value)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		ret = append(ret, value)
	}
	for _, value := range patchNodeCoverageTargetTokens(step.Input) {
		add(value)
	}
	for _, value := range patchNodeCoverageTargetTokens(step.Output) {
		add(value)
	}
	for _, patch := range mapSliceFromValue(step.Output["patches"]) {
		for _, value := range patchNodeCoverageTargetTokens(patch) {
			add(value)
		}
	}
	if apply := mapFromValue(step.Output["artifactContinuationApply"]); len(apply) > 0 {
		for _, value := range patchNodeCoverageTargetTokens(apply) {
			add(value)
		}
	}
	if decision := mapFromValue(step.Output[stepOutputKeyLLMDecision]); len(decision) > 0 {
		for _, request := range mapSliceFromValue(decision["patchRequests"]) {
			for _, value := range patchNodeCoverageTargetTokens(request) {
				add(value)
			}
		}
	}
	return ret
}

func runtimeRepairCurrentExecutionProgress(
	ctx context.Context,
	steps repository.StepRepository,
	repair *repairSubsystem,
	run *runModel.Run,
	failedStep *runModel.Step,
	input map[string]any,
) map[string]any {
	if run == nil || failedStep == nil {
		return nil
	}
	ret := map[string]any{
		"runId":            string(run.ID),
		"failedStepId":     string(failedStep.ID),
		"failedStepKind":   failedStep.Kind,
		"failedStepState":  string(failedStep.State),
		"coverageGapCount": runtimeRepairCoverageGapCount(input),
	}
	if class := strings.TrimSpace(firstNonEmptyString(stringFromMap(input, failureOutputKeyClass), stringFromMap(failedStep.Output, failureOutputKeyClass))); class != "" {
		ret["failureClass"] = class
	}
	if stage := strings.TrimSpace(stringFromMap(failedStep.Output, "stage")); stage != "" {
		ret["failureStage"] = stage
	}
	if reason := strings.TrimSpace(firstNonEmptyString(stringFromMap(input, "failureReason"), stringFromMap(failedStep.Output, "error"), stringFromMap(failedStep.Output, "reason"))); reason != "" {
		ret["failureReason"] = truncateLLMContextString(reason)
	}
	if evidence := runtimeRepairProgressEvidenceSummaries(mapSliceFromValue(input[stepInputKeyGraphEvidence])); len(evidence) > 0 {
		ret["completedGraphEvidence"] = evidence
	}
	if latest := runtimeRepairProgressEvidenceSummaries(latestActionEvidenceCandidates(ctx, steps, run, failedStep, 6)); len(latest) > 0 {
		ret["latestActionEvidence"] = latest
	}
	if attempts := runtimeRepairAttemptProgressSummaries(repair.recentRuntimeRepairAttemptSummaries(ctx, run.ID, failedStep, 5)); len(attempts) > 0 {
		ret["recentRepairAttempts"] = attempts
	}
	if targets := coverageTargetSetFromRepairInput(input); len(targets) > 0 {
		ret["unresolvedCoverageTargets"] = targets
	}
	if len(ret) <= 5 {
		return nil
	}
	return compactLLMContextMap(ret, 0)
}

func runtimeRepairCoverageGapCount(input map[string]any) int {
	if len(input) == 0 {
		return 0
	}
	return len(mapSliceFromValue(input["missingDoDItems"])) +
		len(mapSliceFromValue(input["missingGoalCoverage"])) +
		len(mapSliceFromValue(input["missingDeliverableCoverage"]))
}

func runtimeRepairProgressEvidenceSummaries(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	limit := min(len(items), 8)
	ret := make([]map[string]any, 0, limit)
	for idx := 0; idx < limit; idx++ {
		item := items[idx]
		next := map[string]any{}
		for _, key := range []string{"stepId", "nodeId", "graphNodeId", "graphRootStepId", "kind", "state", "source", "matchScore"} {
			if value, ok := item[key]; ok && value != nil {
				next[key] = compactLLMContextValue(value, 1)
			}
		}
		output := mapFromValue(item["output"])
		if len(output) > 0 {
			summary := map[string]any{}
			for _, key := range []string{"status", "stage", "command", "args", "exitCode", "durationMillis", "targetRef", "patchId", "patchType", "summary", "error", "reason"} {
				if value, ok := output[key]; ok && value != nil {
					summary[key] = compactLLMContextValue(value, 1)
				}
			}
			if stdout := strings.TrimSpace(stringFromMap(output, "stdout")); stdout != "" {
				summary["stdoutPreview"] = truncateLLMContextString(stdout)
			}
			if stderr := strings.TrimSpace(stringFromMap(output, "stderr")); stderr != "" {
				summary["stderrPreview"] = truncateLLMContextString(stderr)
			}
			if patches := mapSliceFromValue(output["patches"]); len(patches) > 0 {
				summary["patches"] = runtimeRepairPatchEvidenceSummaries(patches)
			}
			if len(summary) > 0 {
				next["outputSummary"] = summary
			}
		}
		if len(next) > 0 {
			ret = append(ret, next)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func runtimeRepairPatchEvidenceSummaries(patches []map[string]any) []map[string]any {
	if len(patches) == 0 {
		return nil
	}
	limit := min(len(patches), 5)
	ret := make([]map[string]any, 0, limit)
	for idx := 0; idx < limit; idx++ {
		patch := patches[idx]
		next := map[string]any{}
		for _, key := range []string{"targetRef", "patchType", "summary", "baseHash", "contentHash"} {
			if value, ok := patch[key]; ok && value != nil {
				next[key] = compactLLMContextValue(value, 1)
			}
		}
		if _, ok := patch["content"]; ok {
			next["contentOmitted"] = true
		}
		if _, ok := patch["diff"]; ok {
			next["diffOmitted"] = true
		}
		if len(next) > 0 {
			ret = append(ret, next)
		}
	}
	return ret
}

func runtimeRepairAttemptProgressSummaries(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	limit := min(len(items), 5)
	ret := make([]map[string]any, 0, limit)
	for idx := 0; idx < limit; idx++ {
		item := items[idx]
		next := map[string]any{}
		for _, key := range []string{"stepId", "state", "attempt", "failureClass", "failureStage", "failureSummary", "rejectedOutputSummaryOmitted"} {
			if value, ok := item[key]; ok && value != nil {
				next[key] = compactLLMContextValue(value, 1)
			}
		}
		if missing := mapSliceFromValue(item["missingDoDItems"]); len(missing) > 0 {
			next["missingDoDCount"] = len(missing)
		}
		if len(next) > 0 {
			ret = append(ret, next)
		}
	}
	return ret
}

func coverageRequirementTerms(requirement string) []string {
	requirement = strings.ToLower(strings.TrimSpace(requirement))
	if requirement == "" {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0)
	add := func(term string) {
		term = normalizeDoDCoverageText(term)
		if term == "" || seen[term] {
			return
		}
		seen[term] = true
		ret = append(ret, term)
	}
	for _, term := range strings.FieldsFunc(requirement, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		term = strings.TrimSpace(term)
		if !coverageRequirementTermIsUseful(term) {
			continue
		}
		add(term)
		if strings.HasSuffix(term, "ing") && len(term) > 5 {
			add(strings.TrimSuffix(term, "ing"))
		}
		if strings.HasSuffix(term, "ed") && len(term) > 4 {
			add(strings.TrimSuffix(term, "ed"))
		}
	}
	if len(ret) > 20 {
		ret = ret[:20]
	}
	return ret
}

func coverageRequirementTermIsUseful(term string) bool {
	if term == "" {
		return false
	}
	allASCII := true
	runeCount := 0
	for _, r := range term {
		runeCount++
		if r > 127 {
			allASCII = false
		}
	}
	if allASCII {
		return len(term) >= 3
	}
	return runeCount >= 2
}

func matchedCoverageTerms(terms []string, normalizedEvidenceText string) []string {
	if len(terms) == 0 || normalizedEvidenceText == "" {
		return nil
	}
	ret := make([]string, 0)
	for _, term := range terms {
		if term == "" {
			continue
		}
		if strings.Contains(normalizedEvidenceText, term) {
			ret = append(ret, term)
		}
	}
	return ret
}

func flattenCoverageEvidenceText(value any, depth int) string {
	if value == nil || depth > 4 {
		return ""
	}
	switch v := value.(type) {
	case string:
		return truncateLLMContextString(v)
	case []string:
		return truncateLLMContextString(strings.Join(v, "\n"))
	case []any:
		parts := make([]string, 0, min(len(v), maxLLMContextSliceItems))
		for idx := 0; idx < len(v) && idx < maxLLMContextSliceItems; idx++ {
			if text := flattenCoverageEvidenceText(v[idx], depth+1); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case []map[string]any:
		parts := make([]string, 0, min(len(v), maxLLMContextSliceItems))
		for idx := 0; idx < len(v) && idx < maxLLMContextSliceItems; idx++ {
			if text := flattenCoverageEvidenceText(v[idx], depth+1); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			if shouldSkipLLMContextKey(key) {
				continue
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, min(len(keys), maxLLMContextMapEntries))
		for idx := 0; idx < len(keys) && idx < maxLLMContextMapEntries; idx++ {
			key := keys[idx]
			if text := flattenCoverageEvidenceText(v[key], depth+1); text != "" {
				parts = append(parts, key, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprint(v)
	}
}

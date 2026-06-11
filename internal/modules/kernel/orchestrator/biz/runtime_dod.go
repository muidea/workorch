package orchestrator

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"unicode"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func completionDoDAssessmentError(run *runModel.Run, output map[string]any) error {
	_, _, err := validatedCompletionDoDAssessment(run, output)
	return err
}

func validatedCompletionDoDAssessment(run *runModel.Run, output map[string]any) (map[string]any, string, error) {
	if !requiresDoDAssessmentCompletionGate(run) {
		return nil, "", nil
	}
	assessment := doDAssessmentFromOutput(output)
	if assessment == nil {
		return nil, "", fmt.Errorf("%w: dodAssessment is required for llm-driven completion", common.ErrInvalidArgument)
	}
	satisfied, ok := assessment["satisfied"].(bool)
	if !ok {
		return nil, "", fmt.Errorf("%w: dodAssessment.satisfied is required", common.ErrInvalidArgument)
	}
	if !satisfied {
		return nil, "", fmt.Errorf("%w: dodAssessment is not satisfied", common.ErrInvalidArgument)
	}
	summary, _ := assessment["summary"].(string)
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil, "", fmt.Errorf("%w: dodAssessment.summary is required for llm-driven completion", common.ErrInvalidArgument)
	}
	if err := validateDoDAssessmentItems(assessment["items"]); err != nil {
		return nil, "", err
	}
	if err := validateDoDAssessmentHasNoOpenEvidenceGaps(assessment); err != nil {
		return nil, "", err
	}
	if err := validateDoDAssessmentCoversRunDoD(run, assessment); err != nil {
		return nil, "", err
	}
	if err := validateDoDAssessmentCoversRunGoal(run, assessment); err != nil {
		return nil, "", err
	}
	if err := validateDoDAssessmentCoversRequiredProfileDimensions(run, assessment); err != nil {
		return nil, "", err
	}
	if err := validateDoDAssessmentCoversRequiredDeliverables(run, assessment); err != nil {
		return nil, "", err
	}
	return assessment, summary, nil
}

func requiresDoDAssessmentCompletionGate(run *runModel.Run) bool {
	return run != nil && run.DoDID != "" && initialStepKind(run) == llmWorkspaceExecutionStepKind
}

func doDAssessmentFromOutput(output map[string]any) map[string]any {
	if output == nil {
		return nil
	}
	if assessment := mapFromValue(output[stepOutputKeyDoDAssessment]); assessment != nil {
		return assessment
	}
	decision := mapFromValue(output[stepOutputKeyLLMDecision])
	if decision == nil {
		return nil
	}
	return mapFromValue(decision[stepOutputKeyDoDAssessment])
}

func normalizeCompletionGateDoDAssessmentOutput(run *runModel.Run, step *runModel.Step, output map[string]any) map[string]any {
	if run == nil || step == nil || len(output) == 0 || !requiresDoDAssessmentCompletionGate(run) {
		return output
	}
	assessment := doDAssessmentFromOutput(output)
	if assessment == nil {
		return output
	}
	normalized := normalizeDoDAssessmentGoalCoverageForRun(assessment, run)
	normalized = normalizeDoDAssessmentDeliverableCoverageForRun(normalized, run)
	if !boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) && !boolValue(step.Input[stepInputKeyProviderRetryFollowUp]) {
		normalized = completeDoDAssessmentDeliverableCoverageForRun(normalized, run, step)
	}
	if boolValue(step.Input[stepInputKeyAssessmentOnly]) &&
		strings.TrimSpace(stringFromMap(step.Input, "repairObjective")) == "complete_dod_goal_coverage_assessment" {
		normalized = normalizeDoDAssessmentGoalCoverageForRepairInput(normalized, step.Input)
	}
	normalized = normalizeDoDAssessmentTopLevelSatisfied(normalized)
	if len(normalized) == 0 {
		return output
	}
	if reflect.DeepEqual(assessment, normalized) {
		return output
	}
	ret := cloneStepPayload(output)
	if _, ok := ret[stepOutputKeyDoDAssessment]; ok {
		ret[stepOutputKeyDoDAssessment] = normalized
		return ret
	}
	decision := mapFromValue(ret[stepOutputKeyLLMDecision])
	if len(decision) == 0 {
		return output
	}
	decision[stepOutputKeyDoDAssessment] = normalized
	ret[stepOutputKeyLLMDecision] = decision
	return ret
}

func normalizeDoDAssessmentTopLevelSatisfied(assessment map[string]any) map[string]any {
	if len(assessment) == 0 {
		return assessment
	}
	current, ok := assessment["satisfied"].(bool)
	if ok && current {
		return assessment
	}
	if !doDAssessmentStructurallySatisfied(assessment) {
		return assessment
	}
	normalized := cloneStepPayload(assessment)
	normalized["satisfied"] = true
	return normalized
}

func doDAssessmentStructurallySatisfied(assessment map[string]any) bool {
	if len(assessment) == 0 {
		return false
	}
	if len(mapSliceFromValue(assessment["evidenceGaps"])) > 0 ||
		len(mapSliceFromValue(assessment["verificationEvidenceGaps"])) > 0 {
		return false
	}
	items, ok := doDAssessmentItemMaps(assessment["items"])
	if !ok || len(items) == 0 || !assessmentItemsAllSatisfied(items, false) {
		return false
	}
	for _, key := range []string{"goalCoverage", "goalOutcomeCoverage", "acceptanceDimensions", "dimensionCoverage", "deliverableCoverage", "deliverables", "producedArtifacts", "branchCoverage"} {
		value := assessment[key]
		if value == nil {
			continue
		}
		if !coverageContainerStructurallySatisfied(value) {
			return false
		}
	}
	return true
}

func coverageContainerStructurallySatisfied(value any) bool {
	if mapped := mapFromValue(value); len(mapped) > 0 {
		if satisfied, ok := mapped["satisfied"].(bool); ok && !satisfied {
			return false
		}
		items, ok := doDAssessmentItemMaps(mapped["items"])
		if !ok || len(items) == 0 {
			return false
		}
		return assessmentItemsAllSatisfied(items, true)
	}
	items, ok := doDAssessmentItemMaps(value)
	if !ok || len(items) == 0 {
		return false
	}
	return assessmentItemsAllSatisfied(items, true)
}

func assessmentItemsAllSatisfied(items []map[string]any, allowDeliverableArtifactRefs bool) bool {
	for _, item := range items {
		state := strings.ToLower(strings.TrimSpace(stringFromMap(item, "state")))
		switch state {
		case "satisfied":
		case "available", "produced":
			if !allowDeliverableArtifactRefs {
				return false
			}
		default:
			return false
		}
		if len(evidenceRefsFromValue(item["evidenceRefs"])) == 0 &&
			(!allowDeliverableArtifactRefs || len(stringSliceFromValue(item["artifactRefs"])) == 0) {
			return false
		}
	}
	return true
}

func normalizeDoDAssessmentGoalCoverageForRun(assessment map[string]any, run *runModel.Run) map[string]any {
	if len(assessment) == 0 || run == nil {
		return assessment
	}
	required := normalizedRunGoalCoverageItems(run)
	if len(required) == 0 {
		return assessment
	}
	coverage := mapFromValue(assessment["goalCoverage"])
	coverageKey := "goalCoverage"
	if len(coverage) == 0 {
		coverage = mapFromValue(assessment["goalOutcomeCoverage"])
		coverageKey = "goalOutcomeCoverage"
	}
	if len(coverage) == 0 {
		return assessment
	}
	items, ok := doDAssessmentItemMaps(coverage["items"])
	if !ok || len(items) == 0 {
		return assessment
	}
	changed := false
	parentSatisfied := boolValue(coverage["satisfied"])
	for idx, item := range items {
		requiredItem, ok := matchingRunGoalCoverageRequirement(item, required)
		if !ok {
			continue
		}
		if normalizeGoalCoverageAssessmentItemFromRequired(item, requiredItem, parentSatisfied) {
			changed = true
		}
		items[idx] = item
	}
	if !changed {
		return assessment
	}
	normalizedAssessment := cloneStepPayload(assessment)
	coverage["items"] = items
	normalizedAssessment[coverageKey] = coverage
	return normalizedAssessment
}

func normalizeDoDAssessmentDeliverableCoverageForRun(assessment map[string]any, run *runModel.Run) map[string]any {
	if len(assessment) == 0 || run == nil {
		return assessment
	}
	required := requiredProfileDeliverableExpectations(run)
	if len(required) == 0 {
		return assessment
	}
	coverageKey, coverage := deliverableCoverageContainerFromAssessment(assessment)
	if len(coverage) == 0 {
		return assessment
	}
	items, ok := doDAssessmentItemMaps(coverage["items"])
	if !ok || len(items) != len(required) {
		return assessment
	}
	changed := false
	for idx, item := range items {
		if strings.TrimSpace(firstNonEmptyString(
			stringFromMap(item, "id"),
			stringFromMap(item, "deliverable"),
			stringFromMap(item, "name"),
			stringFromMap(item, "path"),
			stringFromMap(item, "item"),
		)) == "" {
			item["id"] = required[idx]
			changed = true
		}
		if strings.TrimSpace(stringFromMap(item, "item")) == "" {
			item["item"] = required[idx]
			changed = true
		}
		items[idx] = item
	}
	if !changed {
		return assessment
	}
	normalizedAssessment := cloneStepPayload(assessment)
	coverage["items"] = items
	normalizedAssessment[coverageKey] = coverage
	return normalizedAssessment
}

func completeDoDAssessmentDeliverableCoverageForRun(assessment map[string]any, run *runModel.Run, step *runModel.Step) map[string]any {
	if len(assessment) == 0 || run == nil || step == nil {
		return assessment
	}
	required := requiredProfileDeliverableExpectations(run)
	if len(required) == 0 {
		return assessment
	}
	if !doDAssessmentStructurallySatisfiedExceptDeliverableCoverage(assessment) {
		return assessment
	}
	fallbackRefs := doDAssessmentEvidenceRefs(step.ID, assessment)
	if len(fallbackRefs) == 0 && step.ID != "" {
		fallbackRefs = []string{string(step.ID)}
	}
	if len(fallbackRefs) == 0 {
		return assessment
	}

	coverageKey, coverage := deliverableCoverageContainerFromAssessment(assessment)
	items, ok := doDAssessmentItemMaps(coverage["items"])
	if len(coverage) > 0 {
		if satisfied, ok := coverage["satisfied"].(bool); ok && !satisfied {
			return assessment
		}
	}
	if len(coverage) > 0 && (!ok || !deliverableCoverageItemsRecoverable(items)) {
		return assessment
	}

	existing := map[string]bool{}
	for _, item := range items {
		id := strings.TrimSpace(firstNonEmptyString(
			stringFromMap(item, "id"),
			stringFromMap(item, "item"),
			stringFromMap(item, "deliverable"),
			stringFromMap(item, "name"),
			stringFromMap(item, "path"),
		))
		if id == "" {
			continue
		}
		existing[id] = true
	}
	missing := make([]string, 0)
	for _, id := range required {
		if !existing[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return assessment
	}
	if coverageKey == "" {
		coverageKey = "deliverableCoverage"
	}
	if len(coverage) == 0 {
		coverage = map[string]any{
			"satisfied": true,
			"summary":   "Required deliverable coverage was derived from the accepted DoD and goal evidence.",
		}
	} else {
		coverage = cloneStepPayload(coverage)
		if _, ok := coverage["satisfied"].(bool); !ok {
			coverage["satisfied"] = true
		}
	}

	completed := make([]map[string]any, 0, len(items)+len(missing))
	for _, item := range items {
		completed = append(completed, item)
	}
	for _, id := range missing {
		completed = append(completed, map[string]any{
			"id":           id,
			"item":         id,
			"state":        "satisfied",
			"evidenceRefs": append([]string(nil), fallbackRefs...),
			"reason":       "auto-derived from accepted DoD/Goal evidence and profile deliverable contract",
		})
	}
	coverage["items"] = completed

	normalizedAssessment := cloneStepPayload(assessment)
	normalizedAssessment[coverageKey] = coverage
	return normalizedAssessment
}

func doDAssessmentStructurallySatisfiedExceptDeliverableCoverage(assessment map[string]any) bool {
	if len(assessment) == 0 {
		return false
	}
	current, ok := assessment["satisfied"].(bool)
	if ok && !current {
		return false
	}
	if len(mapSliceFromValue(assessment["evidenceGaps"])) > 0 ||
		len(mapSliceFromValue(assessment["verificationEvidenceGaps"])) > 0 {
		return false
	}
	items, ok := doDAssessmentItemMaps(assessment["items"])
	if !ok || len(items) == 0 || !assessmentItemsAllSatisfied(items, false) {
		return false
	}
	for _, key := range []string{"goalCoverage", "goalOutcomeCoverage", "acceptanceDimensions", "dimensionCoverage", "branchCoverage"} {
		value := assessment[key]
		if value == nil {
			continue
		}
		if !coverageContainerStructurallySatisfied(value) {
			return false
		}
	}
	return true
}

func deliverableCoverageItemsRecoverable(items []map[string]any) bool {
	for _, item := range items {
		state := strings.ToLower(strings.TrimSpace(stringFromMap(item, "state")))
		switch state {
		case "", "satisfied", "available", "produced":
		default:
			return false
		}
		if state != "" && len(evidenceRefsFromValue(item["evidenceRefs"])) == 0 && len(stringSliceFromValue(item["artifactRefs"])) == 0 {
			return false
		}
	}
	return true
}

func deliverableCoverageContainerFromAssessment(assessment map[string]any) (string, map[string]any) {
	if assessment == nil {
		return "", nil
	}
	for _, key := range []string{"deliverableCoverage", "deliverables", "producedArtifacts"} {
		if mapped := mapFromValue(assessment[key]); len(mapped) > 0 {
			return key, mapped
		}
	}
	return "", nil
}

func matchingRunGoalCoverageRequirement(item map[string]any, required []normalizedDoDItem) (normalizedDoDItem, bool) {
	if len(item) == 0 || len(required) == 0 {
		return normalizedDoDItem{}, false
	}
	id := strings.TrimSpace(firstNonEmptyString(stringFromMap(item, "goalRequirementId"), stringFromMap(item, "id")))
	if id != "" {
		for _, candidate := range required {
			if candidate.id == id {
				return candidate, true
			}
		}
		return normalizedDoDItem{}, false
	}
	text := normalizeDoDCoverageText(firstNonEmptyString(
		stringFromMap(item, "goal"),
		stringFromMap(item, "goalItem"),
		stringFromMap(item, "item"),
		stringFromMap(item, "requirement"),
	))
	if text == "" {
		return normalizedDoDItem{}, false
	}
	var matched normalizedDoDItem
	count := 0
	for _, candidate := range required {
		if normalizedGoalCoverageTextMatches(text, candidate.normalized) {
			matched = candidate
			count++
		}
	}
	return matched, count == 1
}

func normalizeGoalCoverageAssessmentItemFromRequired(item map[string]any, required normalizedDoDItem, parentSatisfied bool) bool {
	if len(item) == 0 || strings.TrimSpace(required.raw) == "" {
		return false
	}
	changed := false
	if strings.TrimSpace(firstNonEmptyString(
		stringFromMap(item, "goal"),
		stringFromMap(item, "goalItem"),
		stringFromMap(item, "item"),
		stringFromMap(item, "requirement"),
	)) == "" {
		item["item"] = required.raw
		changed = true
	}
	if strings.TrimSpace(stringFromMap(item, "goalRequirementId")) == "" && strings.TrimSpace(required.id) != "" {
		item["goalRequirementId"] = required.id
		changed = true
	}
	if parentSatisfied && strings.TrimSpace(stringFromMap(item, "state")) == "" &&
		len(evidenceRefsFromValue(item["evidenceRefs"])) > 0 {
		item["state"] = "satisfied"
		changed = true
	}
	return changed
}

func normalizeDoDAssessmentGoalCoverageForRepairInput(assessment map[string]any, input map[string]any) map[string]any {
	if len(assessment) == 0 || len(input) == 0 {
		return assessment
	}
	missing := missingGoalCoverageFromInput(input)
	if len(missing) == 0 {
		return assessment
	}
	coverage := mapFromValue(assessment["goalCoverage"])
	coverageKey := "goalCoverage"
	if len(coverage) == 0 {
		coverage = mapFromValue(assessment["goalOutcomeCoverage"])
		coverageKey = "goalOutcomeCoverage"
	}
	if len(coverage) == 0 {
		return assessment
	}
	items, ok := doDAssessmentItemMaps(coverage["items"])
	if !ok || len(items) == 0 {
		return assessment
	}
	changed := false
	parentSatisfied := boolValue(coverage["satisfied"])
	for idx, item := range items {
		missingItem := missingGoalCoverageItemForAssessmentItem(item, missing)
		if len(missingItem) == 0 && idx < len(missing) {
			missingItem = missing[idx]
		}
		if len(missingItem) > 0 {
			if normalizeGoalCoverageAssessmentItemFromMissing(item, missingItem, parentSatisfied) {
				changed = true
			}
		}
		if normalizeAssessmentItemEvidenceRefs(item, missingItem) {
			changed = true
		}
		items[idx] = item
	}
	if !changed {
		return assessment
	}
	normalizedAssessment := cloneStepPayload(assessment)
	coverage["items"] = items
	normalizedAssessment[coverageKey] = coverage
	return normalizedAssessment
}

func normalizeGoalCoverageAssessmentItemFromMissing(item map[string]any, missing map[string]any, parentSatisfied bool) bool {
	if len(item) == 0 || len(missing) == 0 {
		return false
	}
	changed := false
	req := missingGoalCoverageRequirementText(missing)
	if strings.TrimSpace(firstNonEmptyString(
		stringFromMap(item, "goal"),
		stringFromMap(item, "goalItem"),
		stringFromMap(item, "item"),
		stringFromMap(item, "requirement"),
	)) == "" && req != "" {
		item["requirement"] = req
		changed = true
	}
	if strings.TrimSpace(stringFromMap(item, "goalRequirementId")) == "" {
		if id := strings.TrimSpace(firstNonEmptyString(stringFromMap(missing, "goalRequirementId"), stringFromMap(missing, "id"))); id != "" {
			item["goalRequirementId"] = id
			changed = true
		} else if normalized := normalizeDoDCoverageText(req); normalized != "" {
			item["goalRequirementId"] = goalCoverageRequirementID(0, normalized)
			changed = true
		}
	}
	if parentSatisfied && strings.TrimSpace(stringFromMap(item, "state")) == "" &&
		(len(evidenceRefsFromValue(item["evidenceRefs"])) > 0 || len(evidenceCandidateStepRefs(missing)) > 0) {
		item["state"] = "satisfied"
		changed = true
	}
	return changed
}

func missingGoalCoverageItemForAssessmentItem(item map[string]any, missing []map[string]any) map[string]any {
	if len(item) == 0 || len(missing) == 0 {
		return nil
	}
	var matched map[string]any
	matches := 0
	for _, candidate := range missing {
		if stringFromMap(item, "goalRequirementId") != "" &&
			stringFromMap(item, "goalRequirementId") == firstNonEmptyString(stringFromMap(candidate, "goalRequirementId"), stringFromMap(candidate, "id")) {
			return candidate
		}
		itemText := normalizeDoDCoverageText(firstNonEmptyString(
			stringFromMap(item, "goal"),
			stringFromMap(item, "goalItem"),
			stringFromMap(item, "item"),
			stringFromMap(item, "requirement"),
		))
		candidateText := normalizeDoDCoverageText(missingGoalCoverageRequirementText(candidate))
		if normalizedRepairGoalCoverageTextMatches(itemText, candidateText) {
			matched = candidate
			matches++
		}
	}
	if matches == 1 {
		return matched
	}
	return nil
}

func normalizedRepairGoalCoverageTextMatches(actual string, required string) bool {
	actual = strings.TrimSpace(actual)
	required = strings.TrimSpace(required)
	if actual == "" || required == "" {
		return false
	}
	if normalizedCoverageTextMatches(actual, required) || coverageTextsLikelyEquivalent(actual, required) {
		return true
	}
	shorter, longer := actual, required
	if len([]rune(shorter)) > len([]rune(longer)) {
		shorter, longer = longer, shorter
	}
	shorterLen := len([]rune(shorter))
	if shorterLen < 8 {
		return false
	}
	lcs := longestCommonRuneSubsequenceLength(shorter, longer)
	return float64(lcs)/float64(shorterLen) >= 0.8
}

func normalizeAssessmentItemEvidenceRefs(item map[string]any, missing map[string]any) bool {
	if len(item) == 0 {
		return false
	}
	refs := evidenceRefsFromValue(item["evidenceRefs"])
	filtered := filterTraceableEvidenceRefs(refs)
	if len(filtered) == 0 && strings.ToLower(strings.TrimSpace(stringFromMap(item, "state"))) == "satisfied" {
		filtered = evidenceCandidateStepRefs(missing)
	}
	if stringSlicesEqual(refs, filtered) {
		return false
	}
	item["evidenceRefs"] = filtered
	return true
}

func filterTraceableEvidenceRefs(refs []string) []string {
	if len(refs) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		if !traceableEvidenceRef(ref) {
			continue
		}
		seen[ref] = true
		ret = append(ret, ref)
	}
	return ret
}

func traceableEvidenceRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.ContainsAny(ref, " \t\n\r") {
		return false
	}
	if strings.Contains(ref, "/") {
		return true
	}
	return governanceOwnerEvidenceRef(ref)
}

func governanceOwnerEvidenceRef(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.Contains(ref, "/") || strings.ContainsAny(ref, " \t\n\r") {
		return false
	}
	if !strings.ContainsAny(ref, "-_:") {
		return false
	}
	return coverageEvidenceRefLooksDecision(ref) || coverageEvidenceRefLooksApproval(ref)
}

func evidenceCandidateStepRefs(missing map[string]any) []string {
	if len(missing) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0)
	for _, candidate := range mapSliceFromValue(missing["evidenceCandidates"]) {
		ref := strings.TrimSpace(stringFromMap(candidate, "stepId"))
		if ref == "" || seen[ref] || !traceableEvidenceRef(ref) {
			continue
		}
		seen[ref] = true
		ret = append(ret, ref)
	}
	return ret
}

func stringSlicesEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if strings.TrimSpace(left[idx]) != strings.TrimSpace(right[idx]) {
			return false
		}
	}
	return true
}

func outputWithDoDAssessmentDiagnostics(output map[string]any) map[string]any {
	if len(output) == 0 {
		return output
	}
	missing := missingDoDItemsFromDoDAssessment(doDAssessmentFromOutput(output))
	missingGoalCoverage := missingGoalCoverageFromDoDAssessment(doDAssessmentFromOutput(output))
	if len(missing) == 0 && len(missingGoalCoverage) == 0 {
		return output
	}
	ret := cloneStepPayload(output)
	diagnostics := mapFromValue(ret["runtimeDiagnostics"])
	if diagnostics == nil {
		diagnostics = map[string]any{}
	}
	diagnostics["kind"] = failureClassDoDCoverageMissing
	if len(missing) > 0 {
		diagnostics["missingDoDItems"] = missing
		ret["missingDoDItems"] = missing
	}
	if len(missingGoalCoverage) > 0 {
		diagnostics["missingGoalCoverage"] = missingGoalCoverage
		ret["missingGoalCoverage"] = missingGoalCoverage
	}
	ret["runtimeDiagnostics"] = diagnostics
	return ret
}

func missingDoDItemsFromDoDAssessment(assessment map[string]any) []map[string]any {
	if len(assessment) == 0 {
		return nil
	}
	items, ok := doDAssessmentItemMaps(assessment["items"])
	if !ok || len(items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0)
	for _, item := range items {
		state := strings.ToLower(strings.TrimSpace(stringFromMap(item, "state")))
		if state == "" || state == "satisfied" {
			continue
		}
		name := strings.TrimSpace(stringFromMap(item, "item"))
		if name == "" {
			continue
		}
		missing := map[string]any{
			"kind":         "leaf",
			"item":         name,
			"state":        state,
			"source":       "dodAssessment",
			"evidenceRefs": evidenceRefsFromValue(item["evidenceRefs"]),
		}
		if reason := strings.TrimSpace(firstNonEmptyString(stringFromMap(item, "reason"), stringFromMap(item, "summary"))); reason != "" {
			missing["reason"] = reason
		}
		if impact := strings.TrimSpace(stringFromMap(item, "impact")); impact != "" {
			missing["impact"] = impact
		}
		if actions := stringSliceFromValue(item["nextActions"]); len(actions) > 0 {
			missing["suggestedNextActions"] = actions
		}
		ret = append(ret, missing)
	}
	return ret
}

func llmDecisionCompletionStateFromOutput(output map[string]any) string {
	if output == nil {
		return ""
	}
	decision := mapFromValue(output[stepOutputKeyLLMDecision])
	if decision == nil {
		return ""
	}
	state, _ := decision["completionState"].(string)
	return strings.TrimSpace(strings.ToLower(state))
}

func validateDoDAssessmentItems(value any) error {
	items, ok := doDAssessmentItemMaps(value)
	if !ok || len(items) == 0 {
		return fmt.Errorf("%w: dodAssessment.items are required", common.ErrInvalidArgument)
	}
	for _, item := range items {
		name, _ := item["item"].(string)
		state, _ := item["state"].(string)
		name = strings.TrimSpace(name)
		state = strings.TrimSpace(state)
		if name == "" || state == "" {
			return fmt.Errorf("%w: dodAssessment item and state are required", common.ErrInvalidArgument)
		}
		if state == "satisfied" && len(evidenceRefsFromValue(item["evidenceRefs"])) == 0 {
			return fmt.Errorf("%w: satisfied dodAssessment item requires evidenceRefs", common.ErrInvalidArgument)
		}
	}
	return nil
}

func validateDoDAssessmentHasNoOpenEvidenceGaps(assessment map[string]any) error {
	if assessment == nil {
		return nil
	}
	for _, key := range []string{"evidenceGaps", "verificationEvidenceGaps"} {
		if gaps := mapSliceFromValue(assessment[key]); len(gaps) > 0 {
			return fmt.Errorf("%w: dodAssessment.%s must be empty for accepted result", common.ErrInvalidArgument, key)
		}
	}
	branchCoverage := mapFromValue(assessment["branchCoverage"])
	items := mapSliceFromValue(assessment["branchCoverage"])
	if len(branchCoverage) > 0 {
		items = mapSliceFromValue(branchCoverage["items"])
	}
	for _, item := range items {
		if !boolValue(item["required"]) {
			continue
		}
		name := strings.TrimSpace(firstNonEmptyString(
			stringFromMap(item, "id"),
			stringFromMap(item, "branch"),
			stringFromMap(item, "name"),
		))
		if name == "" {
			name = "unnamed"
		}
		state := strings.ToLower(strings.TrimSpace(stringFromMap(item, "state")))
		if state != "satisfied" {
			return fmt.Errorf("%w: required dodAssessment.branchCoverage item %q is not satisfied", common.ErrInvalidArgument, name)
		}
		if len(evidenceRefsFromValue(item["evidenceRefs"])) == 0 {
			return fmt.Errorf("%w: satisfied dodAssessment.branchCoverage item %q requires evidenceRefs", common.ErrInvalidArgument, name)
		}
	}
	return nil
}

func validateDoDAssessmentCoversRunDoD(run *runModel.Run, assessment map[string]any) error {
	requiredItems := normalizedRunDoDItems(run)
	if len(requiredItems) == 0 {
		return nil
	}
	assessmentItems, ok := doDAssessmentItemMaps(assessment["items"])
	if !ok || len(assessmentItems) == 0 {
		return fmt.Errorf("%w: dodAssessment.items are required", common.ErrInvalidArgument)
	}
	normalizedAssessmentItems := make([]string, 0, len(assessmentItems))
	for _, item := range assessmentItems {
		if strings.TrimSpace(stringFromMap(item, "state")) != "satisfied" {
			continue
		}
		normalized := normalizeDoDCoverageText(stringFromMap(item, "item"))
		if normalized != "" {
			normalizedAssessmentItems = append(normalizedAssessmentItems, normalized)
		}
	}
	missing := make([]normalizedDoDItem, 0)
	for _, required := range requiredItems {
		if !required.requiredForCoverage() || required.normalized == "" {
			continue
		}
		matched := false
		for _, actual := range normalizedAssessmentItems {
			if actual == "" {
				continue
			}
			if normalizedCoverageTextMatches(actual, required.normalized) {
				matched = true
				break
			}
		}
		if !matched {
			missing = append(missing, required)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return newDoDCoverageMissingError(missing)
}

func validateDoDAssessmentCoversRunGoal(run *runModel.Run, assessment map[string]any) error {
	requiredItems := normalizedRunGoalCoverageItems(run)
	if len(requiredItems) == 0 {
		return nil
	}
	covered, err := validatedGoalCoverageItemCoverage(assessment)
	if err != nil {
		return newGoalCoverageContractError(err, requiredItems)
	}
	if len(covered) == 0 {
		return newGoalCoverageMissingError(requiredItems)
	}
	missing := make([]normalizedDoDItem, 0)
	for _, required := range requiredItems {
		if required.normalized == "" {
			continue
		}
		matched := false
		for _, actual := range covered {
			if actual.normalized == "" && actual.id == "" {
				continue
			}
			if required.id != "" && actual.id != "" && required.id == actual.id {
				matched = true
				break
			}
			if normalizedGoalCoverageTextMatches(actual.normalized, required.normalized) {
				matched = true
				break
			}
		}
		if !matched {
			missing = append(missing, required)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return newGoalCoverageMissingError(missing)
}

func validatedGoalCoverageItems(assessment map[string]any) ([]string, error) {
	items, err := validatedGoalCoverageItemCoverage(assessment)
	if err != nil {
		return nil, err
	}
	ret := make([]string, 0, len(items))
	for _, item := range items {
		if item.normalized != "" {
			ret = append(ret, item.normalized)
		}
	}
	return ret, nil
}

func validatedGoalCoverageItemCoverage(assessment map[string]any) ([]normalizedDoDItem, error) {
	if assessment == nil {
		return nil, fmt.Errorf("%w: dodAssessment.goalCoverage is required when Goal has uncovered leaf requirements", common.ErrInvalidArgument)
	}
	coverage := mapFromValue(assessment["goalCoverage"])
	if len(coverage) == 0 {
		coverage = mapFromValue(assessment["goalOutcomeCoverage"])
	}
	if len(coverage) == 0 {
		return nil, fmt.Errorf("%w: dodAssessment.goalCoverage is required when Goal has uncovered leaf requirements", common.ErrInvalidArgument)
	}
	satisfied, ok := coverage["satisfied"].(bool)
	if !ok {
		return nil, fmt.Errorf("%w: dodAssessment.goalCoverage.satisfied is required", common.ErrInvalidArgument)
	}
	if !satisfied {
		return nil, fmt.Errorf("%w: dodAssessment.goalCoverage is not satisfied", common.ErrInvalidArgument)
	}
	items, ok := doDAssessmentItemMaps(coverage["items"])
	if !ok || len(items) == 0 {
		return nil, fmt.Errorf("%w: dodAssessment.goalCoverage.items are required", common.ErrInvalidArgument)
	}
	ret := make([]normalizedDoDItem, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(firstNonEmptyString(
			stringFromMap(item, "goal"),
			stringFromMap(item, "goalItem"),
			stringFromMap(item, "item"),
			stringFromMap(item, "requirement"),
		))
		state := strings.ToLower(strings.TrimSpace(stringFromMap(item, "state")))
		if name == "" || state == "" {
			return nil, fmt.Errorf("%w: dodAssessment.goalCoverage item and state are required", common.ErrInvalidArgument)
		}
		if state != "satisfied" {
			return nil, fmt.Errorf("%w: dodAssessment.goalCoverage contains unsatisfied item %q", common.ErrInvalidArgument, name)
		}
		if len(evidenceRefsFromValue(item["evidenceRefs"])) == 0 {
			return nil, fmt.Errorf("%w: satisfied dodAssessment.goalCoverage item requires evidenceRefs", common.ErrInvalidArgument)
		}
		ret = append(ret, normalizedDoDItem{
			id:         strings.TrimSpace(firstNonEmptyString(stringFromMap(item, "goalRequirementId"), stringFromMap(item, "id"))),
			raw:        name,
			normalized: normalizeDoDCoverageText(name),
			kind:       doDItemKindLeaf,
		})
	}
	return ret, nil
}

func validateDoDAssessmentCoversRequiredProfileDimensions(run *runModel.Run, assessment map[string]any) error {
	required := requiredProfileAcceptanceDimensions(run)
	if len(required) == 0 {
		return nil
	}
	items := acceptanceDimensionItemsFromAssessment(assessment)
	covered := map[string]bool{}
	for _, item := range items {
		id := strings.TrimSpace(firstNonEmptyString(
			stringFromMap(item, "id"),
			stringFromMap(item, "dimension"),
			stringFromMap(item, "key"),
			stringFromMap(item, "name"),
		))
		if id == "" {
			continue
		}
		state := strings.ToLower(strings.TrimSpace(stringFromMap(item, "state")))
		if state != "satisfied" {
			continue
		}
		if len(evidenceRefsFromValue(item["evidenceRefs"])) == 0 {
			return fmt.Errorf("%w: satisfied dodAssessment.acceptanceDimensions item %q requires evidenceRefs", common.ErrInvalidArgument, id)
		}
		covered[id] = true
	}
	missing := make([]string, 0)
	for _, id := range required {
		if !covered[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%w: dodAssessment.acceptanceDimensions missing required dimensions: %s", common.ErrInvalidArgument, strings.Join(missing, ", "))
}

func requiredProfileAcceptanceDimensions(run *runModel.Run) []string {
	if run == nil || len(run.ExecutionContext) == 0 {
		return nil
	}
	executionContract := mapFromValue(run.ExecutionContext["executionContract"])
	profileContracts := mapFromValue(executionContract["profileContracts"])
	if len(profileContracts) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0)
	for _, rawProfile := range profileContracts {
		profile := mapFromValue(rawProfile)
		if len(profile) == 0 {
			continue
		}
		for _, item := range mapSliceFromValue(profile["acceptanceDimensions"]) {
			id := strings.TrimSpace(stringFromMap(item, "id"))
			if id == "" || seen[id] || !boolValue(item["required"]) {
				continue
			}
			seen[id] = true
			ret = append(ret, id)
		}
	}
	return ret
}

func acceptanceDimensionItemsFromAssessment(assessment map[string]any) []map[string]any {
	if assessment == nil {
		return nil
	}
	for _, key := range []string{"acceptanceDimensions", "dimensionCoverage"} {
		value := assessment[key]
		if mapped := mapFromValue(value); len(mapped) > 0 {
			if items := mapSliceFromValue(mapped["items"]); len(items) > 0 {
				return items
			}
		}
		if items := mapSliceFromValue(value); len(items) > 0 {
			return items
		}
	}
	return nil
}

func validateDoDAssessmentCoversRequiredDeliverables(run *runModel.Run, assessment map[string]any) error {
	required := requiredProfileDeliverableExpectations(run)
	if len(required) == 0 {
		return nil
	}
	items := deliverableCoverageItemsFromAssessment(assessment)
	covered := map[string]bool{}
	for _, item := range items {
		id := strings.TrimSpace(firstNonEmptyString(
			stringFromMap(item, "id"),
			stringFromMap(item, "item"),
			stringFromMap(item, "deliverable"),
			stringFromMap(item, "name"),
			stringFromMap(item, "path"),
		))
		if id == "" {
			continue
		}
		state := strings.ToLower(strings.TrimSpace(stringFromMap(item, "state")))
		if state != "satisfied" && state != "available" && state != "produced" {
			continue
		}
		if len(evidenceRefsFromValue(item["evidenceRefs"])) == 0 && len(stringSliceFromValue(item["artifactRefs"])) == 0 {
			return fmt.Errorf("%w: satisfied dodAssessment.deliverableCoverage item %q requires evidenceRefs or artifactRefs", common.ErrInvalidArgument, id)
		}
		covered[id] = true
	}
	missing := make([]string, 0)
	for _, id := range required {
		if !covered[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return newDeliverableCoverageMissingError(missing)
}

func requiredProfileDeliverableExpectations(run *runModel.Run) []string {
	if run == nil || len(run.ExecutionContext) == 0 {
		return nil
	}
	executionContract := mapFromValue(run.ExecutionContext["executionContract"])
	profileContracts := mapFromValue(executionContract["profileContracts"])
	if len(profileContracts) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0)
	for _, rawProfile := range profileContracts {
		profile := mapFromValue(rawProfile)
		if len(profile) == 0 {
			continue
		}
		for _, item := range mapSliceFromValue(profile["deliverableExpectations"]) {
			id := strings.TrimSpace(stringFromMap(item, "id"))
			if id == "" || seen[id] || !boolValue(item["required"]) {
				continue
			}
			seen[id] = true
			ret = append(ret, id)
		}
	}
	return ret
}

func deliverableCoverageItemsFromAssessment(assessment map[string]any) []map[string]any {
	if assessment == nil {
		return nil
	}
	for _, key := range []string{"deliverableCoverage", "deliverables", "producedArtifacts"} {
		value := assessment[key]
		if mapped := mapFromValue(value); len(mapped) > 0 {
			if items := mapSliceFromValue(mapped["items"]); len(items) > 0 {
				return items
			}
		}
		if items := mapSliceFromValue(value); len(items) > 0 {
			return items
		}
	}
	return nil
}

func normalizedSatisfiedAssessmentCoverageItems(assessment map[string]any) []string {
	assessmentItems, ok := doDAssessmentItemMaps(assessment["items"])
	if !ok || len(assessmentItems) == 0 {
		return nil
	}
	ret := make([]string, 0, len(assessmentItems))
	for _, item := range assessmentItems {
		if strings.TrimSpace(stringFromMap(item, "state")) != "satisfied" {
			continue
		}
		for _, key := range []string{"item", "goal", "goalItem", "requirement"} {
			if normalized := normalizeDoDCoverageText(stringFromMap(item, key)); normalized != "" {
				ret = append(ret, normalized)
			}
		}
	}
	return ret
}

func goalCoverageFromAssessment(assessment map[string]any) []string {
	if assessment == nil {
		return nil
	}
	coverage := mapFromValue(assessment["goalCoverage"])
	if len(coverage) == 0 {
		coverage = mapFromValue(assessment["goalOutcomeCoverage"])
	}
	if len(coverage) == 0 {
		return nil
	}
	if satisfied, ok := coverage["satisfied"].(bool); ok && !satisfied {
		return nil
	}
	items, ok := doDAssessmentItemMaps(coverage["items"])
	if !ok || len(items) == 0 {
		return nil
	}
	ret := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(stringFromMap(item, "state")) != "satisfied" {
			continue
		}
		if len(evidenceRefsFromValue(item["evidenceRefs"])) == 0 {
			continue
		}
		for _, key := range []string{"goal", "goalItem", "item", "requirement"} {
			if normalized := normalizeDoDCoverageText(stringFromMap(item, key)); normalized != "" {
				ret = append(ret, normalized)
			}
		}
	}
	return ret
}

func normalizedRunGoalCoverageItems(run *runModel.Run) []normalizedDoDItem {
	if run == nil || len(run.ExecutionContext) == 0 {
		return nil
	}
	doDItems := normalizedRunDoDItems(run)
	doDCoverage := make([]string, 0, len(doDItems))
	for _, item := range doDItems {
		if item.normalized != "" {
			doDCoverage = append(doDCoverage, item.normalized)
		}
	}
	goalTexts := runGoalTexts(run)
	ret := make([]normalizedDoDItem, 0)
	seen := map[string]bool{}
	for _, text := range goalTexts {
		for _, leaf := range goalCoverageLeafItems(text) {
			normalized := normalizeDoDCoverageText(leaf.Text)
			if normalized == "" || seen[normalized] || goalItemCoveredByDoD(normalized, doDCoverage) {
				continue
			}
			seen[normalized] = true
			ret = append(ret, normalizedDoDItem{
				id:                   goalCoverageRequirementID(len(ret)+1, normalized),
				raw:                  leaf.Text,
				normalized:           normalized,
				kind:                 doDItemKindLeaf,
				requiredEvidenceHint: neutralCoverageEvidenceHint("Goal requirement"),
			})
		}
	}
	return ret
}

func goalItemCoveredByDoD(goalItem string, doDItems []string) bool {
	for _, item := range doDItems {
		if item == "" {
			continue
		}
		if normalizedCoverageTextMatches(item, goalItem) {
			return true
		}
	}
	return false
}

func runGoalTexts(run *runModel.Run) []string {
	if run == nil || len(run.ExecutionContext) == 0 {
		return nil
	}
	add := func(seen map[string]bool, ret []string, value string) []string {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return ret
		}
		seen[value] = true
		return append(ret, value)
	}
	seen := map[string]bool{}
	ret := make([]string, 0)
	if goal := mapFromValue(run.ExecutionContext["goal"]); len(goal) > 0 {
		ret = add(seen, ret, firstNonEmptyString(stringFromMap(goal, "text"), stringFromMap(goal, "Text")))
	}
	for _, goal := range mapSliceFromValue(run.ExecutionContext["goals"]) {
		ret = add(seen, ret, firstNonEmptyString(stringFromMap(goal, "text"), stringFromMap(goal, "Text")))
	}
	ret = add(seen, ret, strings.TrimSpace(stringFromAny(run.ExecutionContext["goalText"])))
	ret = add(seen, ret, strings.TrimSpace(stringFromAny(run.ExecutionContext["effectiveGoalView"])))
	return ret
}

func goalCoverageRawItems(text string) []string {
	leaves := goalCoverageLeafItems(text)
	ret := make([]string, 0, len(leaves))
	for _, leaf := range leaves {
		ret = append(ret, leaf.Text)
	}
	if len(ret) == 0 {
		text = strings.TrimSpace(text)
		if text != "" {
			ret = append(ret, text)
		}
	}
	return ret
}

type doDItemKind string

const (
	doDItemKindHeading doDItemKind = "heading"
	doDItemKindParent  doDItemKind = "parent"
	doDItemKindLeaf    doDItemKind = "leaf"
)

type normalizedDoDItem struct {
	id                   string
	raw                  string
	normalized           string
	kind                 doDItemKind
	requiredEvidenceHint string
}

func (item normalizedDoDItem) requiredForCoverage() bool {
	return item.kind == doDItemKindLeaf
}

type doDCoverageMissingError struct {
	cause   error
	missing []normalizedDoDItem
}

type goalCoverageMissingError struct {
	cause   error
	missing []normalizedDoDItem
}

type deliverableCoverageMissingError struct {
	cause   error
	missing []string
}

func newGoalCoverageContractError(cause error, missing []normalizedDoDItem) error {
	if cause == nil {
		return newGoalCoverageMissingError(missing)
	}
	items := make([]normalizedDoDItem, 0, len(missing))
	for _, item := range missing {
		if item.normalized == "" {
			continue
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return cause
	}
	return &goalCoverageMissingError{cause: cause, missing: items}
}

func newDoDCoverageMissingError(missing []normalizedDoDItem) error {
	items := make([]normalizedDoDItem, 0, len(missing))
	rawItems := make([]string, 0, len(missing))
	for _, item := range missing {
		raw := strings.TrimSpace(item.raw)
		if raw == "" {
			continue
		}
		items = append(items, item)
		rawItems = append(rawItems, raw)
	}
	return &doDCoverageMissingError{
		cause:   fmt.Errorf("%w: dodAssessment does not cover all current DoD items; missing: %s", common.ErrInvalidArgument, strings.Join(rawItems, "; ")),
		missing: items,
	}
}

func (e *doDCoverageMissingError) Error() string {
	if e == nil || e.cause == nil {
		return ""
	}
	return e.cause.Error()
}

func (e *doDCoverageMissingError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *doDCoverageMissingError) RuntimeFailureDiagnostics() map[string]any {
	if e == nil {
		return nil
	}
	return map[string]any{
		"kind":            failureClassDoDCoverageMissing,
		"missingDoDItems": missingDoDItemDiagnostics(e.missing),
	}
}

func newGoalCoverageMissingError(missing []normalizedDoDItem) error {
	items := make([]normalizedDoDItem, 0, len(missing))
	rawItems := make([]string, 0, len(missing))
	for _, item := range missing {
		raw := strings.TrimSpace(item.raw)
		if raw == "" {
			continue
		}
		items = append(items, item)
		rawItems = append(rawItems, raw)
	}
	return &goalCoverageMissingError{
		cause:   fmt.Errorf("%w: dodAssessment does not cover all current Goal requirements; missing: %s", common.ErrInvalidArgument, strings.Join(rawItems, "; ")),
		missing: items,
	}
}

func (e *goalCoverageMissingError) Error() string {
	if e == nil || e.cause == nil {
		return ""
	}
	return e.cause.Error()
}

func (e *goalCoverageMissingError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *goalCoverageMissingError) RuntimeFailureDiagnostics() map[string]any {
	if e == nil {
		return nil
	}
	return map[string]any{
		"kind":                failureClassDoDCoverageMissing,
		"missingGoalCoverage": missingDoDItemDiagnostics(e.missing),
	}
}

func newDeliverableCoverageMissingError(missing []string) error {
	items := make([]string, 0, len(missing))
	for _, item := range missing {
		item = strings.TrimSpace(item)
		if item != "" {
			items = append(items, item)
		}
	}
	return &deliverableCoverageMissingError{
		cause:   fmt.Errorf("%w: dodAssessment.deliverableCoverage missing required deliverables: %s", common.ErrInvalidArgument, strings.Join(items, ", ")),
		missing: items,
	}
}

func (e *deliverableCoverageMissingError) Error() string {
	if e == nil || e.cause == nil {
		return ""
	}
	return e.cause.Error()
}

func (e *deliverableCoverageMissingError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *deliverableCoverageMissingError) RuntimeFailureDiagnostics() map[string]any {
	if e == nil {
		return nil
	}
	items := make([]map[string]any, 0, len(e.missing))
	for _, id := range e.missing {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		items = append(items, map[string]any{
			"id":    id,
			"kind":  "deliverableExpectation",
			"state": "missing",
		})
	}
	return map[string]any{
		"kind":                       failureClassDoDCoverageMissing,
		"missingDeliverableCoverage": items,
	}
}

func missingGoalCoverageFromDoDAssessment(assessment map[string]any) []map[string]any {
	if len(assessment) == 0 {
		return nil
	}
	coverage := mapFromValue(assessment["goalCoverage"])
	if len(coverage) == 0 {
		return nil
	}
	items, ok := doDAssessmentItemMaps(coverage["items"])
	if !ok || len(items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0)
	for _, item := range items {
		state := strings.ToLower(strings.TrimSpace(stringFromMap(item, "state")))
		if state == "" || state == "satisfied" {
			continue
		}
		name := firstNonEmptyString(stringFromMap(item, "goal"), stringFromMap(item, "goalItem"), stringFromMap(item, "item"), stringFromMap(item, "requirement"))
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		missing := map[string]any{
			"kind":         "goal",
			"item":         name,
			"state":        state,
			"source":       "goalCoverage",
			"evidenceRefs": evidenceRefsFromValue(item["evidenceRefs"]),
		}
		if id := strings.TrimSpace(firstNonEmptyString(stringFromMap(item, "goalRequirementId"), stringFromMap(item, "id"))); id != "" {
			missing["goalRequirementId"] = id
		} else if normalized := normalizeDoDCoverageText(name); normalized != "" {
			missing["goalRequirementId"] = goalCoverageRequirementID(0, normalized)
		}
		if reason := strings.TrimSpace(firstNonEmptyString(stringFromMap(item, "reason"), stringFromMap(item, "summary"))); reason != "" {
			missing["reason"] = reason
		}
		if actions := stringSliceFromValue(item["nextActions"]); len(actions) > 0 {
			missing["suggestedNextActions"] = actions
		}
		ret = append(ret, missing)
	}
	return ret
}

func missingDeliverableCoverageFromDoDAssessment(assessment map[string]any) []map[string]any {
	if len(assessment) == 0 {
		return nil
	}
	items := deliverableCoverageItemsFromAssessment(assessment)
	if len(items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0)
	for _, item := range items {
		state := strings.ToLower(strings.TrimSpace(stringFromMap(item, "state")))
		if state == "" || state == "satisfied" {
			continue
		}
		name := strings.TrimSpace(firstNonEmptyString(
			stringFromMap(item, "item"),
			stringFromMap(item, "id"),
			stringFromMap(item, "expectationId"),
			stringFromMap(item, "requirement"),
		))
		if name == "" {
			continue
		}
		missing := map[string]any{
			"kind":         "deliverable",
			"item":         name,
			"state":        state,
			"source":       "deliverableCoverage",
			"evidenceRefs": evidenceRefsFromValue(item["evidenceRefs"]),
		}
		if reason := strings.TrimSpace(firstNonEmptyString(stringFromMap(item, "reason"), stringFromMap(item, "summary"))); reason != "" {
			missing["reason"] = reason
		}
		if actions := stringSliceFromValue(item["nextActions"]); len(actions) > 0 {
			missing["suggestedNextActions"] = actions
		}
		ret = append(ret, missing)
	}
	return ret
}

func missingDoDItemDiagnostics(items []normalizedDoDItem) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw := strings.TrimSpace(item.raw)
		normalized := strings.TrimSpace(item.normalized)
		if raw == "" && normalized == "" {
			continue
		}
		entry := map[string]any{
			"raw":        raw,
			"normalized": normalized,
			"kind":       string(item.kind),
		}
		if id := strings.TrimSpace(item.id); id != "" {
			entry["id"] = id
			if strings.HasPrefix(id, "goal-req-") {
				entry["goalRequirementId"] = id
			}
		}
		if hint := strings.TrimSpace(item.requiredEvidenceHint); hint != "" {
			entry["requiredEvidenceHint"] = hint
		}
		ret = append(ret, entry)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func goalCoverageRequirementID(index int, normalized string) string {
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return ""
	}
	sum := sha1.Sum([]byte(normalized))
	hash := hex.EncodeToString(sum[:])[:8]
	if index > 0 {
		return fmt.Sprintf("goal-req-%03d-%s", index, hash)
	}
	return fmt.Sprintf("goal-req-%s", hash)
}

func normalizedRunDoDItems(run *runModel.Run) []normalizedDoDItem {
	if run == nil || len(run.ExecutionContext) == 0 {
		return nil
	}
	items := stringSliceFromValue(mapFromValue(run.ExecutionContext["dod"])["items"])
	if len(items) == 0 {
		for _, dod := range mapSliceFromValue(run.ExecutionContext["dods"]) {
			items = append(items, stringSliceFromValue(dod["items"])...)
		}
	}
	ret := make([]normalizedDoDItem, 0, len(items))
	seen := map[string]bool{}
	for _, raw := range items {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		kind := classifyDoDItemKind(raw)
		normalized := normalizeDoDCoverageText(raw)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		ret = append(ret, normalizedDoDItem{
			raw:                  raw,
			normalized:           normalized,
			kind:                 kind,
			requiredEvidenceHint: doDRequiredEvidenceHint(raw),
		})
	}
	return ret
}

func isDoDHeading(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.HasPrefix(trimmed, "#")
}

func classifyDoDItemKind(value string) doDItemKind {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return doDItemKindLeaf
	}
	if isDoDHeading(trimmed) {
		return doDItemKindHeading
	}
	content := trimDoDItemText(trimmed)
	if content == "" || strings.HasSuffix(content, ":") || strings.HasSuffix(content, "：") {
		return doDItemKindParent
	}
	return doDItemKindLeaf
}

func doDRequiredEvidenceHint(raw string) string {
	return neutralCoverageEvidenceHint("DoD item")
}

func neutralCoverageEvidenceHint(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		subject = "coverage item"
	}
	return "Provide concrete evidence refs for this " + subject + " according to the active runtime stage and profile contract."
}

func normalizeDoDCoverageText(value string) string {
	value = trimDoDItemText(value)
	value = strings.NewReplacer("**", "", "__", "", "`", "", "（", "(", "）", ")").Replace(value)
	var builder strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func normalizedCoverageTextMatches(actual string, required string) bool {
	actual = strings.TrimSpace(actual)
	required = strings.TrimSpace(required)
	if actual == "" || required == "" {
		return false
	}
	if actual == required || strings.Contains(actual, required) || strings.Contains(required, actual) {
		return true
	}
	shorter, longer := actual, required
	if len([]rune(shorter)) > len([]rune(longer)) {
		shorter, longer = longer, shorter
	}
	shorterLen := len([]rune(shorter))
	longerLen := len([]rune(longer))
	if shorterLen < 24 || longerLen == 0 {
		return false
	}
	if float64(shorterLen)/float64(longerLen) < 0.75 {
		return false
	}
	lcs := longestCommonRuneSubsequenceLength(shorter, longer)
	return float64(lcs)/float64(shorterLen) >= 0.9
}

func normalizedGoalCoverageTextMatches(actual string, required string) bool {
	actual = strings.TrimSpace(actual)
	required = strings.TrimSpace(required)
	if actual == "" || required == "" {
		return false
	}
	if actual == required {
		return true
	}
	if normalizedGoalCoverageTextHasBoundedAffixMatch(actual, required) {
		return true
	}
	shorter, longer := actual, required
	if len([]rune(shorter)) > len([]rune(longer)) {
		shorter, longer = longer, shorter
	}
	shorterLen := len([]rune(shorter))
	longerLen := len([]rune(longer))
	if shorterLen < 24 || longerLen == 0 {
		return false
	}
	// Goal coverage represents independent leaf requirements. A broad item that
	// merely contains a newer leaf must not satisfy that leaf; otherwise a changed
	// Goal can be accepted with stale evidence by merging multiple leaves.
	if float64(shorterLen)/float64(longerLen) < 0.75 {
		return false
	}
	lcs := longestCommonRuneSubsequenceLength(shorter, longer)
	return float64(lcs)/float64(shorterLen) >= 0.9
}

func normalizedGoalCoverageTextHasBoundedAffixMatch(actual string, required string) bool {
	actualLen := len([]rune(actual))
	requiredLen := len([]rune(required))
	if actualLen < 12 || requiredLen < 12 {
		return false
	}
	shorter, longer := actual, required
	shorterLen, longerLen := actualLen, requiredLen
	if shorterLen > longerLen {
		shorter, longer = longer, shorter
		shorterLen, longerLen = longerLen, shorterLen
	}
	idx := strings.Index(longer, shorter)
	if idx < 0 {
		return false
	}
	prefix := longer[:idx]
	suffix := longer[idx+len(shorter):]
	prefixLen := len([]rune(prefix))
	suffixLen := len([]rune(suffix))
	maxAffixLen := max(16, shorterLen/2)
	if prefixLen > maxAffixLen || suffixLen > maxAffixLen {
		return false
	}
	affixText := prefix + suffix
	for _, marker := range []string{"并", "以及", "或者", "或", "且", "同时"} {
		if strings.Contains(affixText, marker) {
			return false
		}
	}
	return true
}

func longestCommonRuneSubsequenceLength(left string, right string) int {
	a := []rune(left)
	b := []rune(right)
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				curr[j] = prev[j-1] + 1
				continue
			}
			if prev[j] > curr[j-1] {
				curr[j] = prev[j]
			} else {
				curr[j] = curr[j-1]
			}
		}
		prev, curr = curr, prev
		for j := range curr {
			curr[j] = 0
		}
	}
	return prev[len(b)]
}

func trimDoDItemText(value string) string {
	value = strings.TrimSpace(value)
	for {
		next := strings.TrimSpace(value)
		lower := strings.ToLower(next)
		switch {
		case strings.HasPrefix(lower, "- [ ]"):
			value = strings.TrimSpace(next[len("- [ ]"):])
		case strings.HasPrefix(lower, "- [x]"):
			value = strings.TrimSpace(next[len("- [x]"):])
		case strings.HasPrefix(lower, "* [ ]"):
			value = strings.TrimSpace(next[len("* [ ]"):])
		case strings.HasPrefix(lower, "* [x]"):
			value = strings.TrimSpace(next[len("* [x]"):])
		case strings.HasPrefix(next, "-"), strings.HasPrefix(next, "*"), strings.HasPrefix(next, "•"):
			value = strings.TrimSpace(next[1:])
		default:
			return strings.TrimSpace(value)
		}
	}
}

func doDAssessmentItemMaps(value any) ([]map[string]any, bool) {
	switch typed := value.(type) {
	case []map[string]any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneStepPayload(item))
		}
		return ret, true
	case []any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			mapped, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			ret = append(ret, cloneStepPayload(mapped))
		}
		return ret, true
	case map[string]any:
		if nested, ok := doDAssessmentItemMaps(typed["items"]); ok {
			return nested, true
		}
		return nil, false
	default:
		return nil, false
	}
}

func evidenceRefsFromValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			ret = appendEvidenceRefsFromString(ret, item)
		}
		return ret
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil
			}
			ret = appendEvidenceRefsFromString(ret, text)
		}
		return ret
	default:
		return nil
	}
}

func appendEvidenceRefsFromString(ret []string, text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ret
	}
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			ret = append(ret, part)
		}
	}
	return ret
}

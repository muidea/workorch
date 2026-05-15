package orchestrator

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
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
	if !boolValue(step.Input[stepInputKeyAssessmentOnly]) ||
		strings.TrimSpace(stringFromMap(step.Input, "repairObjective")) != "complete_dod_goal_coverage_assessment" {
		return output
	}
	assessment := doDAssessmentFromOutput(output)
	if assessment == nil {
		return output
	}
	normalized := normalizeDoDAssessmentGoalCoverageForRepairInput(assessment, step.Input)
	if len(normalized) == 0 {
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
	for idx, item := range items {
		missingItem := missingGoalCoverageItemForAssessmentItem(item, missing)
		if len(missingItem) == 0 && idx < len(missing) {
			missingItem = missing[idx]
		}
		if len(missingItem) > 0 {
			if normalizeGoalCoverageAssessmentItemFromMissing(item, missingItem) {
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

func normalizeGoalCoverageAssessmentItemFromMissing(item map[string]any, missing map[string]any) bool {
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
	return changed
}

func missingGoalCoverageItemForAssessmentItem(item map[string]any, missing []map[string]any) map[string]any {
	if len(item) == 0 || len(missing) == 0 {
		return nil
	}
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
		if itemText != "" && candidateText != "" && (strings.Contains(itemText, candidateText) || strings.Contains(candidateText, itemText)) {
			return candidate
		}
	}
	return nil
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
	return strings.Contains(ref, "/")
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
			if strings.Contains(actual, required.normalized) || strings.Contains(required.normalized, actual) {
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
	covered, err := validatedGoalCoverageItems(assessment)
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
			if actual == "" {
				continue
			}
			if strings.Contains(actual, required.normalized) || strings.Contains(required.normalized, actual) {
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
	ret := make([]string, 0, len(items))
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
		if normalized := normalizeDoDCoverageText(name); normalized != "" {
			ret = append(ret, normalized)
		}
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
	return fmt.Errorf("%w: dodAssessment.deliverableCoverage missing required deliverables: %s", common.ErrInvalidArgument, strings.Join(missing, ", "))
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
		for _, raw := range goalCoverageRawItems(text) {
			normalized := normalizeDoDCoverageText(raw)
			if normalized == "" || seen[normalized] || goalItemCoveredByDoD(normalized, doDCoverage) {
				continue
			}
			seen[normalized] = true
			ret = append(ret, normalizedDoDItem{
				id:                   goalCoverageRequirementID(len(ret)+1, normalized),
				raw:                  raw,
				normalized:           normalized,
				kind:                 doDItemKindLeaf,
				requiredEvidenceHint: "Provide concrete implementation and verification evidence refs for this Goal requirement.",
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
		if strings.Contains(item, goalItem) || strings.Contains(goalItem, item) {
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
	lines := strings.Split(text, "\n")
	ret := make([]string, 0, len(lines))
	for _, line := range lines {
		line = trimDoDItemText(strings.TrimSpace(line))
		if line == "" || isDoDHeading(line) {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if classifyDoDItemKind(line) != doDItemKindLeaf {
			continue
		}
		ret = append(ret, line)
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
	text := strings.ToLower(raw)
	switch {
	case strings.Contains(text, "test"), strings.Contains(text, "测试"), strings.Contains(text, "验证"), strings.Contains(text, "verification"):
		return "Provide validation command evidence with explicit assertions for this DoD item."
	case strings.Contains(text, "config"), strings.Contains(text, "配置"), strings.Contains(text, "yaml"), strings.Contains(text, "env"), strings.Contains(text, "环境变量"):
		return "Provide implementation evidence and configuration-loading verification evidence for this DoD item."
	case strings.Contains(text, "并发"), strings.Contains(text, "concurrency"), strings.Contains(text, "race"):
		return "Provide concurrency/race-path verification evidence for this DoD item."
	default:
		return "Provide concrete implementation and verification evidence refs for this DoD item."
	}
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
	default:
		return nil, false
	}
}

func evidenceRefsFromValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil
			}
			text = strings.TrimSpace(text)
			if text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

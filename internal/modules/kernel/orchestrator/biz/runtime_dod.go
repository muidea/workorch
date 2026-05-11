package orchestrator

import (
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
	if err := validateDoDAssessmentCoversRunDoD(run, assessment); err != nil {
		return nil, "", err
	}
	if err := validateDoDAssessmentCoversRunGoal(run, assessment); err != nil {
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
	covered := normalizedSatisfiedAssessmentCoverageItems(assessment)
	if coverage := goalCoverageFromAssessment(assessment); len(coverage) > 0 {
		covered = append(covered, coverage...)
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

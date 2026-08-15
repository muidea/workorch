package governance

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

var structuredValuePattern = regexp.MustCompile(`([A-Za-z0-9_\-\p{Han}]{2,32})\s*[:：=]\s*([^\s,，。;；]+)`)

func inferMissingInfoAnswer(
	ctxMap workorcheventpayload.Payload,
	question string,
	reason string,
	goalText string,
	dod *workspace.DoD,
	memoryItems []*memorymodel.MemoryItem,
) (string, float64, string, []string) {
	evidenceRefs := []string{}
	if value := firstNonEmptyMapValue(ctxMap, []string{"answer", "value", "target", "input", "selection", "requested", "default", "path", "command", "url", "branch", "language"}); value != "" {
		return value, 0.97, "answer inferred from structured ticket context", evidenceRefs
	}
	if value := singleStringMapValue(ctxMap); value != "" {
		return value, 0.9, "answer inferred from single ticket context value", evidenceRefs
	}
	if value, confidence, ok := structuredCandidateAnswer(ctxMap); ok {
		return value, confidence, "answer inferred from structured ticket candidates", evidenceRefs
	}
	if value, confidence, ok := continuationCandidateAnswer(ctxMap); ok {
		return value, confidence, "answer inferred from continuable ticket candidate", evidenceRefs
	}
	if hasCandidateValues(ctxMap) {
		return "", 0, "", evidenceRefs
	}

	query := normalize(strings.Join([]string{question, reason}, " "))
	if value, ok := findStructuredValueInText(query, goalText); ok {
		return value, 0.9, "answer inferred from goal text", evidenceRefs
	}
	if dod != nil {
		for _, item := range dod.Items {
			if value, ok := findStructuredValueInText(query, item); ok {
				return value, 0.87, "answer inferred from DoD items", evidenceRefs
			}
		}
	}
	for i := len(memoryItems) - 1; i >= 0; i-- {
		item := memoryItems[i]
		if item == nil {
			continue
		}
		text := strings.TrimSpace(strings.Join([]string{item.Content, item.Summary, item.EmbeddingText}, " "))
		if value, ok := findStructuredValueInText(query, text); ok {
			evidence := string(item.ID)
			if evidence != "" {
				evidenceRefs = append(evidenceRefs, evidence)
			}
			return value, 0.85, "answer inferred from workspace memory", evidenceRefs
		}
	}
	return "", 0.0, "", evidenceRefs
}

func hasCandidateValues(input workorcheventpayload.Payload) bool {
	_, ok := input.Field("candidateValues")
	return ok
}

func continuationCandidateAnswer(input workorcheventpayload.Payload) (string, float64, bool) {
	candidates, ok := input.Object("candidateValues")
	if !ok || len(candidates.Fields) == 0 {
		return "", 0, false
	}
	confidences := confidenceMapFromMap(input, "confidenceByField")
	selected := map[string]string{}
	confidence := 1.0
	confidenceSeen := false
	for _, fieldValue := range candidates.Fields {
		field := strings.TrimSpace(fieldValue.Name)
		if field == "" {
			continue
		}
		candidate := preferredAutoContinuationCandidate(fieldValue.Value)
		if candidate == "" {
			continue
		}
		selected[field] = candidate
		if fieldConfidence, ok := confidences[normalize(field)]; ok {
			confidence = minFloat(confidence, fieldConfidence)
			confidenceSeen = true
		}
	}
	if len(selected) == 0 {
		return "", 0, false
	}
	if !confidenceSeen {
		confidence = 0.9
	}
	answer := joinStructuredAnswer(selected, sortedMapKeys(selected))
	return answer, confidence, answer != ""
}

func preferredAutoContinuationCandidate(value workorcheventpayload.Value) string {
	switch value.Kind {
	case workorcheventpayload.ValueString:
		if isAutoContinuationCandidateText(value.String) {
			return strings.TrimSpace(value.String)
		}
	case workorcheventpayload.ValueArray:
		for _, item := range value.Array {
			if candidate := preferredAutoContinuationCandidate(item); candidate != "" {
				return candidate
			}
		}
	case workorcheventpayload.ValueObject:
		object := workorcheventpayload.Payload{Fields: value.Object}
		for _, key := range []string{"value", "id", "mode", "target", "name"} {
			if nested, ok := object.Field(key); ok {
				if candidate := preferredAutoContinuationCandidate(nested); candidate != "" {
					return candidate
				}
			}
		}
		for _, nested := range value.Object {
			if candidate := preferredAutoContinuationCandidate(nested.Value); candidate != "" {
				return candidate
			}
		}
	}
	return ""
}

func isAutoContinuationCandidateText(value string) bool {
	normalized := normalizeContinuationCandidateText(value)
	if normalized == "" {
		return false
	}
	for _, marker := range []string{
		"init", "initialize", "bootstrap", "fromscratch", "newscratch", "newproject",
		"currentworkspace", "currentdirectory",
		"初始化", "从零", "新项目", "当前工作区", "当前目录",
	} {
		if strings.Contains(normalized, normalizeContinuationCandidateText(marker)) {
			return true
		}
	}
	return false
}

func normalizeContinuationCandidateText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "", "-", "", "_", "", "/", "", "\\", "", ".", "")
	return replacer.Replace(value)
}

func structuredCandidateAnswer(input workorcheventpayload.Payload) (string, float64, bool) {
	if boolFromMap(input, "humanRequired") ||
		boolFromMap(input, "requiresHuman") ||
		boolFromMap(input, "manualRequired") ||
		len(stringSliceFromMap(input, "requiresHumanFields")) > 0 {
		return "", 0, false
	}
	knownValues := stringMapFromMap(input, "knownValues")
	candidateValues := stringMapFromMap(input, "candidateValues")
	resolvedValues := stringMapFromMap(input, "resolvedValues")
	missingFields := stringSliceFromMap(input, "missingFields")
	confidenceByField := confidenceMapFromMap(input, "confidenceByField")
	topConfidenceValue, hasTopConfidence := input.Field("confidence")
	topConfidence, hasTopConfidence := confidenceValue(topConfidenceValue, hasTopConfidence)

	values := make(map[string]string, len(knownValues)+len(candidateValues)+len(resolvedValues))
	for key, value := range knownValues {
		values[key] = value
	}
	for key, value := range candidateValues {
		values[key] = value
	}
	for key, value := range resolvedValues {
		values[key] = value
	}
	if len(values) == 0 {
		return "", 0, false
	}
	if len(missingFields) > 0 {
		for _, field := range missingFields {
			normalized := normalize(field)
			if normalized == "" {
				continue
			}
			if _, ok := mapValueCaseInsensitive(candidateValues, normalized); !ok {
				if _, ok := mapValueCaseInsensitive(resolvedValues, normalized); !ok {
					return "", 0, false
				}
			}
		}
	}

	confidence := 1.0
	confidenceSeen := false
	for key := range candidateValues {
		if value, ok := confidenceByField[normalize(key)]; ok {
			confidence = minFloat(confidence, value)
			confidenceSeen = true
		}
	}
	for key := range resolvedValues {
		if value, ok := confidenceByField[normalize(key)]; ok {
			confidence = minFloat(confidence, value)
			confidenceSeen = true
		}
	}
	if !confidenceSeen {
		if !hasTopConfidence {
			return "", 0, false
		}
		confidence = topConfidence
	}

	answer := joinStructuredAnswer(values, append(missingFields, sortedMapKeys(values)...))
	if answer == "" {
		return "", 0, false
	}
	return answer, confidence, true
}

func joinStructuredAnswer(values map[string]string, preferredOrder []string) string {
	if len(values) == 0 {
		return ""
	}
	seen := map[string]bool{}
	parts := make([]string, 0, len(values))
	appendField := func(field string) {
		key := normalize(field)
		if key == "" || seen[key] {
			return
		}
		value, ok := mapValueCaseInsensitive(values, key)
		if !ok {
			return
		}
		if value == "" {
			return
		}
		seen[key] = true
		parts = append(parts, strings.TrimSpace(field)+"="+value)
	}
	for _, key := range preferredOrder {
		appendField(key)
	}
	for _, key := range sortedMapKeys(values) {
		appendField(key)
	}
	return strings.Join(parts, ", ")
}

func mapValueCaseInsensitive(input map[string]string, normalizedKey string) (string, bool) {
	for key, value := range input {
		if normalize(key) == normalizedKey {
			return strings.TrimSpace(value), true
		}
	}
	return "", false
}

func sortedMapKeys(input map[string]string) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func findStructuredValueInText(query string, text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	matches := structuredValuePattern.FindAllStringSubmatch(text, -1)
	for _, pair := range matches {
		if len(pair) < 3 {
			continue
		}
		key := normalize(pair[1])
		value := strings.TrimSpace(pair[2])
		if key == "" || value == "" {
			continue
		}
		if strings.Contains(query, key) {
			return value, true
		}
	}
	return "", false
}

func requiresHumanByQuestion(question string, reason string) bool {
	combined := normalize(question + " " + reason)
	if strings.Contains(combined, "是否") ||
		strings.Contains(combined, "approve") ||
		strings.Contains(combined, "批准") ||
		strings.Contains(combined, "拒绝") {
		return true
	}
	return false
}

func hasManualRequiredMarker(input workorcheventpayload.Payload) bool {
	return boolFromMap(input, "humanRequired") ||
		boolFromMap(input, "requiresHuman") ||
		boolFromMap(input, "manualRequired") ||
		len(stringSliceFromMap(input, "requiresHumanFields")) > 0
}

func runtimeRecoveryRequiresExternalOperator(ctxMap workorcheventpayload.Payload, question string, reason string) bool {
	scope := normalize(stringFromMap(ctxMap, "scope"))
	dependency := normalize(stringFromMap(ctxMap, "dependency"))
	policy := normalize(stringFromMap(ctxMap, "recoveryPolicy"))
	failureClass := normalize(stringFromMap(ctxMap, "failureClass"))
	if scope == "workorch_runtime" && dependency == "llm_provider" {
		return true
	}
	if policy == "system_config_required" || policy == "config_only" {
		return true
	}
	if strings.HasPrefix(failureClass, "provider_") ||
		failureClass == "context_too_large" ||
		failureClass == "rate_limited" ||
		failureClass == "auth_or_config_error" {
		return true
	}
	text := normalize(question + "\n" + reason)
	return strings.Contains(text, "llm provider") || strings.Contains(text, "provider retry")
}

func stringSliceContainsFold(values []string, want string) bool {
	want = normalize(want)
	for _, value := range values {
		if normalize(value) == want {
			return true
		}
	}
	return false
}

func normalize(input string) string {
	return strings.ToLower(strings.TrimSpace(input))
}

func firstNonEmptyMapValue(input workorcheventpayload.Payload, keys []string) string {
	for _, key := range keys {
		value := strings.TrimSpace(stringFromMap(input, key))
		if value != "" {
			return value
		}
	}
	return ""
}

func singleStringMapValue(input workorcheventpayload.Payload) string {
	if len(input.Fields) != 1 {
		return ""
	}
	field := input.Fields[0]
	if field.Value.Kind != workorcheventpayload.ValueString {
		return ""
	}
	return strings.TrimSpace(field.Value.String)
}

func stringMapFromMap(input workorcheventpayload.Payload, key string) map[string]string {
	ret := map[string]string{}
	object, ok := input.Object(key)
	if !ok {
		return ret
	}
	for _, field := range object.Fields {
		if value := stringifyScalar(field.Value); value != "" {
			ret[strings.TrimSpace(field.Name)] = value
		}
	}
	return ret
}

func confidenceMapFromMap(input workorcheventpayload.Payload, key string) map[string]float64 {
	ret := map[string]float64{}
	object, ok := input.Object(key)
	if !ok {
		return ret
	}
	for _, field := range object.Fields {
		if value, ok := confidenceValue(field.Value, true); ok {
			ret[normalize(field.Name)] = value
		}
	}
	return ret
}

func stringSliceFromMap(input workorcheventpayload.Payload, key string) []string {
	values, ok := input.StringSlice(key)
	if !ok {
		return nil
	}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			ret = append(ret, value)
		}
	}
	return ret
}

func boolFromMap(input workorcheventpayload.Payload, key string) bool {
	value, ok := input.Field(key)
	if !ok {
		return false
	}
	switch value.Kind {
	case workorcheventpayload.ValueBool:
		return value.Bool
	case workorcheventpayload.ValueString:
		parsed, err := strconv.ParseBool(strings.TrimSpace(value.String))
		return err == nil && parsed
	}
	return false
}

func confidenceValue(input workorcheventpayload.Value, present bool) (float64, bool) {
	if !present {
		return 0, false
	}
	switch input.Kind {
	case workorcheventpayload.ValueFloat:
		return input.Float, true
	case workorcheventpayload.ValueInt:
		return float64(input.Int), true
	case workorcheventpayload.ValueUint:
		return float64(input.Uint), true
	case workorcheventpayload.ValueString:
		switch normalize(input.String) {
		case "high":
			return 0.9, true
		case "medium":
			return 0.75, true
		case "low":
			return 0.5, true
		default:
			value, err := strconv.ParseFloat(strings.TrimSpace(input.String), 64)
			return value, err == nil
		}
	}
	return 0, false
}

func stringifyScalar(input workorcheventpayload.Value) string {
	switch input.Kind {
	case workorcheventpayload.ValueString:
		return strings.TrimSpace(input.String)
	case workorcheventpayload.ValueFloat:
		return strconv.FormatFloat(input.Float, 'g', -1, 64)
	case workorcheventpayload.ValueInt:
		return strconv.FormatInt(input.Int, 10)
	case workorcheventpayload.ValueUint:
		return strconv.FormatUint(input.Uint, 10)
	case workorcheventpayload.ValueBool:
		return strconv.FormatBool(input.Bool)
	}
	return ""
}

func minFloat(a float64, b float64) float64 {
	if b < a {
		return b
	}
	return a
}

func stringFromMap(input workorcheventpayload.Payload, key string) string {
	value, _ := input.String(key)
	return strings.TrimSpace(value)
}

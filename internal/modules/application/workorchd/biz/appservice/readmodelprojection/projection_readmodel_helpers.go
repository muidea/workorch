package readmodelprojection

import "strings"

// WD5 mechanical consolidation; source files are listed to preserve review traceability.

// source: dod_leaf_match.go

func isDoDLeafCovered(requiredNormalized string, covered []string) bool {
	for _, actual := range covered {
		if actual == "" {
			continue
		}
		if strings.Contains(actual, requiredNormalized) || strings.Contains(requiredNormalized, actual) {
			return true
		}
	}
	return false
}

// source: dod_text_trim.go

func trimReadModelDoDItemText(value string) string {
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

// source: evidence_ref_accumulator.go

type runEvidenceRefs map[string]struct{}

func (refs runEvidenceRefs) add(ref string) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return
	}
	refs[ref] = struct{}{}
}

// source: execution_evidence_ref_mutators.go

func (r *executionEvidenceRefs) add(target map[string][]string, stepID string, refs ...string) {
	stepID = strings.TrimSpace(stepID)
	if stepID == "" {
		return
	}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		target[stepID] = append(target[stepID], ref)
		r.evidenceByStep[stepID] = append(r.evidenceByStep[stepID], ref)
	}
}

func (r *executionEvidenceRefs) compact() {
	for stepID, refs := range r.evidenceByStep {
		r.evidenceByStep[stepID] = compactUniqueStrings(refs)
	}
}

// source: governance_cursor.go

const governanceCursorSeparator = "\x00"

func governanceScopedCursor(scope string, id string) string {
	scope = strings.TrimSpace(scope)
	id = strings.TrimSpace(id)
	if scope == "" {
		return id
	}
	return scope + governanceCursorSeparator + id
}

func governanceCursorMatches(cursor string, scope string, id string) bool {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return false
	}
	return cursor == governanceScopedCursor(scope, id)
}

// source: governance_pagination_compare.go

func governancePageCreatedIDBefore(leftCreated string, rightCreated string, leftID string, rightID string) bool {
	leftCreated = strings.TrimSpace(leftCreated)
	rightCreated = strings.TrimSpace(rightCreated)
	if leftCreated != "" || rightCreated != "" {
		if leftCreated != rightCreated {
			return leftCreated > rightCreated
		}
	}
	return leftID > rightID
}

// source: progress_recovered_state.go

func recoveredProgressItemState(state string, recoveredHistoricalFailures bool) string {
	if recoveredHistoricalFailures && strings.TrimSpace(state) == "failed" {
		return "recovered"
	}
	return state
}

// source: readmodel_values.go

func readModelStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

// source: repair_failure_signature_targets.go

func readModelTargetsFromFailureSignature(signature string) []string {
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return nil
	}
	var ret []string
	for _, segment := range strings.Split(signature, "|") {
		key, value, ok := strings.Cut(segment, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "failureTargets":
			for _, target := range strings.Split(value, ",") {
				if target = strings.TrimSpace(target); target != "" {
					ret = append(ret, target)
				}
			}
		case "targetRef", "conflictRef":
			if value != "" {
				ret = append(ret, value)
			}
		}
	}
	return compactUniqueStrings(ret)
}

// source: repair_structured_signal_sources.go

func readModelFallbackFailureSourceMessages(output map[string]any, failureClass string, failureSignature string) ([]string, []string) {
	sources := []string{}
	messages := []string{}
	addSourceMessage := func(source string, value string) {
		value = strings.TrimSpace(value)
		if source == "" || value == "" {
			return
		}
		sources = append(sources, source)
		messages = append(messages, readModelTruncateString(value, 512))
	}
	addSourceMessage("failureClass", readModelFirstNonEmptyString(failureClass, readModelStringValue(output["failureClass"])))
	addSourceMessage("failureSignature", readModelFirstNonEmptyString(failureSignature, readModelStringValue(output["failureSignature"])))
	for _, key := range []string{"error", "reason", "stderr", "stdout", "stage"} {
		addSourceMessage(key, readModelStringValue(output[key]))
	}
	return sources, messages
}

// source: repair_structured_signal_targets.go

func readModelFallbackFailureTargetRefs(output map[string]any, failureSignature string) []string {
	targetRefs := []string{}
	addTarget := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			targetRefs = append(targetRefs, value)
		}
	}
	addTarget(readModelStringValue(output["targetRef"]))
	addTarget(readModelStringValue(output["path"]))
	for _, scope := range []string{
		readModelStringValue(output["failureTargetScope"]),
		readModelStringValue(output["targetScope"]),
	} {
		for _, target := range readModelTargetsFromFailureSignature(scope) {
			addTarget(target)
		}
	}
	diagnostics := readModelMapValue(output["runtimeDiagnostics"])
	for _, key := range []string{"targetRef", "conflictRef"} {
		addTarget(readModelStringValue(diagnostics[key]))
	}
	progress := readModelMapValue(output["currentExecutionProgress"])
	lastFailure := readModelMapValue(progress["lastObservedFailure"])
	for _, source := range []map[string]any{progress, lastFailure} {
		addTarget(readModelStringValue(source["targetRef"]))
		for _, scope := range []string{
			readModelStringValue(source["failureTargetScope"]),
			readModelStringValue(source["targetScope"]),
		} {
			for _, target := range readModelTargetsFromFailureSignature(scope) {
				addTarget(target)
			}
		}
	}
	for _, signature := range []string{failureSignature, readModelStringValue(output["failureSignature"])} {
		for _, target := range readModelTargetsFromFailureSignature(signature) {
			addTarget(target)
		}
	}
	return targetRefs
}

// source: repair_values.go

func readModelLimitStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func readModelTruncateString(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func readModelJSONSize(value any) int {
	return readModelApproxValueSize(value, 0)
}

func readModelFirstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func readModelApproxValueSize(value any, depth int) int {
	if value == nil || depth > 8 {
		return 0
	}
	switch typed := value.(type) {
	case string:
		return len(typed)
	case bool:
		return 1
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return 8
	case []string:
		size := 0
		for _, item := range typed {
			size += len(item)
		}
		return size
	case []any:
		size := 0
		for _, item := range typed {
			size += readModelApproxValueSize(item, depth+1)
		}
		return size
	case []map[string]any:
		size := 0
		for _, item := range typed {
			size += readModelApproxValueSize(item, depth+1)
		}
		return size
	case map[string]any:
		size := 0
		for key, item := range typed {
			size += len(key) + readModelApproxValueSize(item, depth+1)
		}
		return size
	default:
		return 0
	}
}

// source: step_output.go

func projectStepOutput(kind string, output map[string]any) map[string]any {
	if len(output) == 0 {
		return nil
	}
	ret := map[string]any{}
	for key, value := range output {
		if !isStepOutputAllowedKey(key) {
			continue
		}
		if projected := compactTraceValue(value, 0); projected != nil {
			ret[key] = projected
		}
	}
	if strings.TrimSpace(kind) == "mcp/tool_call" {
		if result := projectMCPToolCallResult(output["result"]); len(result) > 0 {
			ret["result"] = result
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: task_execution_root_step.go

func graphRootStepIDFromStepID(stepID string) string {
	idx := strings.LastIndex(stepID, "/node/")
	if idx <= 0 {
		return ""
	}
	return stepID[:idx]
}

// source: weak_validation_refs.go

func allEvidenceRefsWeakValidation(refs []string, weakRefIndex map[string]bool) bool {
	if len(refs) == 0 || len(weakRefIndex) == 0 {
		return false
	}
	seen := false
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		seen = true
		if !weakRefIndex[ref] {
			return false
		}
	}
	return seen
}

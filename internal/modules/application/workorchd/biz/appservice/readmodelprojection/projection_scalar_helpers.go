package readmodelprojection

// WD5 mechanical consolidation: package-private scalar, key, and value helpers
// remain in their original source order and keep the same package boundary.

// source: closure_trace_values.go

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func missingDoDItemsFromStepOutput(output map[string]any) []map[string]any {
	if len(output) == 0 {
		return nil
	}
	diagnostics := readModelMapValue(output["runtimeDiagnostics"])
	if items := readModelMapSliceValue(diagnostics["missingDoDItems"]); len(items) > 0 {
		return items
	}
	if failedOutput := readModelMapValue(output["failedStepOutput"]); len(failedOutput) > 0 {
		if items := missingDoDItemsFromStepOutput(failedOutput); len(items) > 0 {
			return items
		}
	}
	return nil
}

// source: context_bundle.go

// source: detail.go

// source: execution_evidence_ref_accumulator.go

type executionEvidenceRefs struct {
	evidenceByStep      map[string][]string
	decisionsByStep     map[string][]string
	approvalsByStep     map[string][]string
	toolCallsByStep     map[string][]string
	patchesByStep       map[string][]string
	validationsByStep   map[string][]string
	resultsByStep       map[string][]string
	artifactsByStep     map[string][]string
	memoryRecallsByStep map[string][]string
	memoryIDsByStep     map[string][]string
}

func newExecutionEvidenceRefs() executionEvidenceRefs {
	return executionEvidenceRefs{
		evidenceByStep:      map[string][]string{},
		decisionsByStep:     map[string][]string{},
		approvalsByStep:     map[string][]string{},
		toolCallsByStep:     map[string][]string{},
		patchesByStep:       map[string][]string{},
		validationsByStep:   map[string][]string{},
		resultsByStep:       map[string][]string{},
		artifactsByStep:     map[string][]string{},
		memoryRecallsByStep: map[string][]string{},
		memoryIDsByStep:     map[string][]string{},
	}
}

// source: governance.go

const (
	decisionPageLimit    = 100
	maxDecisionPageLimit = 500
	approvalPageLimit    = 100
	maxApprovalPageLimit = 500
)

// source: outcome.go
// Package readmodelprojection contains pure projections from owner read models
// into app-facing read-model snapshots and views.

// source: page_limits.go

const (
	stepPageLimit          = 100
	maxStepPageLimit       = 500
	toolCallPageLimit      = 100
	maxToolCallPageLimit   = 500
	checkpointPageLimit    = 100
	maxCheckpointPageLimit = 500
	patchPageLimit         = 100
	maxPatchPageLimit      = 500
	validationPageLimit    = 100
	maxValidationPageLimit = 500
	resultPageLimit        = 100
	maxResultPageLimit     = 500
	artifactPageLimit      = 100
	maxArtifactPageLimit   = 500
)

// source: page_selection.go

// selectProjectedPage applies the common cursor window used by bounded run
// resource projections. Source identity and projection stay application-local
// so this helper never exposes owner models through a shared package.
func selectProjectedPage[S any, P any](
	items []*S,
	limit int,
	cursorAfterID string,
	sourceID func(*S) string,
	project func(*S) P,
) ([]P, string, bool) {
	started := cursorAfterID == ""
	selected := make([]P, 0, min(limit, len(items)))
	nextCursor := ""
	for _, item := range items {
		if item == nil {
			continue
		}
		itemID := sourceID(item)
		if !started {
			if itemID == cursorAfterID {
				started = true
			}
			continue
		}
		if len(selected) >= limit {
			return selected, nextCursor, true
		}
		selected = append(selected, project(item))
		nextCursor = itemID
	}
	return selected, nextCursor, false
}

// source: progress_context.go

// source: progress_count_state.go

func progressStateFromCount(count int, failed bool, completed bool) string {
	if failed {
		return "failed"
	}
	if completed || count > 0 {
		return "completed"
	}
	return "pending"
}

// source: progress_labels.go

type workspaceTaskProgressLabels struct {
	NoActiveRun      string
	ActiveRun        string
	WorkspaceDefined string
	FormalResult     string
	Runs             string
	Results          string
	Artifacts        string
}

type runTaskProgressLabels struct {
	RecoveryNextAction string
	RunCreated         string
	ExecutionSteps     string
	Governance         string
	Validation         string
	FormalResult       string
}

// source: readmodel_float_values.go

func readModelFloatValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int8:
		return float64(typed)
	case int16:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	case uint:
		return float64(typed)
	case uint8:
		return float64(typed)
	case uint16:
		return float64(typed)
	case uint32:
		return float64(typed)
	case uint64:
		return float64(typed)
	default:
		return 0
	}
}

// source: readmodel_map_slices.go

func readModelMapSliceValue(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		if len(typed) == 0 {
			return nil
		}
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneReadModelMap(item))
		}
		return ret
	case []any:
		if len(typed) == 0 {
			return nil
		}
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped := readModelMapValue(item); len(mapped) > 0 {
				ret = append(ret, mapped)
			}
		}
		return ret
	default:
		return nil
	}
}

// source: readmodel_maps.go

func cloneReadModelMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneReadModelMapSlice(input []map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		ret = append(ret, cloneReadModelMap(item))
	}
	return ret
}

func cloneReadModelIntMap(input map[string]int) map[string]int {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[string]int, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func readModelMapValue(value any) map[string]any {
	typed, ok := value.(map[string]any)
	if !ok || len(typed) == 0 {
		return nil
	}
	return cloneReadModelMap(typed)
}

// source: readmodel_numeric_values.go

func readModelIntValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint:
		return int(typed)
	case uint8:
		return int(typed)
	case uint16:
		return int(typed)
	case uint32:
		return int(typed)
	case uint64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func readModelBoolValue(value any) bool {
	typed, ok := value.(bool)
	return ok && typed
}

// source: readmodel_scalar_projection.go

func projectReadModelScalarMapValue(value any) (any, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case string:
		if len(typed) <= eventPayloadStringLimit {
			return typed, true
		}
		return typed[:eventPayloadStringLimit] + "...", true
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return typed, true
	case map[string]any:
		ret := map[string]any{}
		for key, item := range typed {
			if projected, ok := projectReadModelScalarMapValue(item); ok {
				ret[key] = projected
			}
		}
		if len(ret) == 0 {
			return nil, false
		}
		return ret, true
	default:
		return nil, false
	}
}

// source: readmodel_string_slices.go

func cloneReadModelStringSlice(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	ret := make([]string, len(input))
	copy(ret, input)
	return ret
}

func readModelStringSliceValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		if len(typed) == 0 {
			return nil
		}
		ret := make([]string, len(typed))
		copy(ret, typed)
		return ret
	case []any:
		if len(typed) == 0 {
			return nil
		}
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			if str := readModelStringValue(item); str != "" {
				ret = append(ret, str)
			}
		}
		if len(ret) == 0 {
			return nil
		}
		return ret
	default:
		return nil
	}
}

func readModelStringSliceMapValue(value any) map[string][]string {
	ret := map[string][]string{}
	switch typed := value.(type) {
	case map[string][]string:
		for key, values := range typed {
			if key == "" {
				continue
			}
			if refs := readModelStringSliceValue(values); len(refs) > 0 {
				ret[key] = refs
			}
		}
	case map[string]any:
		for key, values := range typed {
			if key == "" {
				continue
			}
			if refs := readModelStringSliceValue(values); len(refs) > 0 {
				ret[key] = refs
			}
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: repair_structured_signal_fallback.go

func readModelFallbackStructuredFailureSignals(output map[string]any, failureClass string, failureSignature string) map[string]any {
	if output == nil {
		return nil
	}
	sources, messages := readModelFallbackFailureSourceMessages(output, failureClass, failureSignature)
	targetRefs := readModelFallbackFailureTargetRefs(output, failureSignature)
	return readModelStructuredFailureSignalMap(sources, targetRefs, messages)
}

// source: repair_structured_signal_map.go

func readModelStructuredFailureSignalMap(sources []string, targetRefs []string, messages []string) map[string]any {
	ret := map[string]any{}
	if sources = compactUniqueStrings(sources); len(sources) > 0 {
		ret["sources"] = readModelLimitStrings(sources, 8)
	}
	if targetRefs = compactUniqueStrings(targetRefs); len(targetRefs) > 0 {
		ret["targetRefs"] = readModelLimitStrings(targetRefs, 16)
	}
	if messages = compactUniqueStrings(messages); len(messages) > 0 {
		ret["messages"] = readModelLimitStrings(messages, 10)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: result_quality_dod_refs.go

func unlinkedDoDEvidenceRefs(refs []string, evidenceIndex map[string]struct{}) []string {
	missingRefs := make([]string, 0, len(refs))
	for _, ref := range refs {
		if _, ok := evidenceIndex[ref]; !ok {
			missingRefs = append(missingRefs, ref)
		}
	}
	return missingRefs
}

// source: runtime_capabilities.go

// source: step_contract_audit.go

func projectStepContractAudit(sources ...map[string]any) map[string]any {
	var input map[string]any
	for _, source := range sources {
		if len(source) == 0 {
			continue
		}
		if readModelStringValue(source["schemaVersion"]) == "workorch.step_contract_audit" ||
			len(readModelMapValue(source["requiredOutput"])) > 0 ||
			readModelStringValue(source["currentActionKind"]) != "" ||
			readModelStringValue(source["currentFocusId"]) != "" {
			if len(input) == 0 {
				input = source
				continue
			}
		}
		if len(readModelMapValue(source["currentActionContract"])) > 0 ||
			len(readModelMapValue(source["coverageWorkItemContext"])) > 0 ||
			len(readModelMapValue(source["convergenceAuthority"])) > 0 {
			input = source
			break
		}
	}
	if len(input) == 0 {
		return nil
	}
	if readModelStringValue(input["schemaVersion"]) == "workorch.step_contract_audit" {
		return cloneReadModelMap(input)
	}

	currentActionContract := readModelMapValue(input["currentActionContract"])
	coverageContext := readModelMapValue(input["coverageWorkItemContext"])
	authority := readModelMapValue(input["convergenceAuthority"])
	if len(coverageContext) == 0 {
		coverageContext = readModelMapValue(authority["coverageWorkItemContext"])
	}
	requiredOutput, requiredOutputSource := selectStepContractRequiredOutput(input, currentActionContract)
	currentFocus := firstNonEmptyMap(
		readModelMapValue(currentActionContract["currentFocus"]),
		readModelMapValue(coverageContext["currentFocus"]),
		readModelMapValue(authority["currentFocus"]),
	)
	nextAction := firstNonEmptyMap(
		readModelMapValue(currentActionContract["nextAction"]),
		readModelMapValue(currentActionContract["prerequisite"]),
		readModelMapValue(authority["nextAction"]),
	)

	ret := map[string]any{
		"schemaVersion": "workorch.step_contract_audit",
	}
	if stage := firstNonEmptyString(
		readModelStringValue(input["runtimeStage"]),
		readModelStringValue(currentActionContract["runtimeStage"]),
	); stage != "" {
		ret["runtimeStage"] = stage
	}
	if kind := firstNonEmptyString(
		readModelStringValue(nextAction["kind"]),
		readModelStringValue(nextAction["type"]),
		readModelStringValue(requiredOutput["nextActionKind"]),
		readModelStringValue(requiredOutput["actionKind"]),
	); kind != "" {
		ret["currentActionKind"] = kind
	}
	if focusID := firstNonEmptyString(
		focusIDFromMap(currentFocus),
		readModelStringValue(requiredOutput["coverageFocusId"]),
		readModelStringValue(requiredOutput["focusId"]),
	); focusID != "" {
		ret["currentFocusId"] = focusID
	}
	if ids := compactUniqueStrings(append(readModelStringSliceValue(currentFocus["coverageWorkItemIds"]), readModelStringSliceValue(requiredOutput["coverageWorkItemIds"])...)); len(ids) > 0 {
		ret["coverageWorkItemIds"] = ids
	}
	if kinds := compactUniqueStrings(firstNonEmptyStringSlice(
		readModelStringSliceValue(currentFocus["missingEvidenceKinds"]),
		readModelStringSliceValue(requiredOutput["missingEvidenceKinds"]),
		readModelStringSliceValue(currentFocus["requiredEvidenceKinds"]),
		readModelStringSliceValue(requiredOutput["requiredEvidenceKinds"]),
	)); len(kinds) > 0 {
		ret["missingEvidenceKinds"] = kinds
	}
	if len(requiredOutput) > 0 {
		ret["requiredOutputSource"] = requiredOutputSource
		ret["requiredOutput"] = compactStepRequiredOutputForAudit(requiredOutput)
	}
	if diagnostic := selectStepContractValidationFailureDiagnostic(input, currentActionContract); len(diagnostic) > 0 {
		ret["validationFailureDiagnostic"] = diagnostic
	}
	return ret
}

func selectStepContractValidationFailureDiagnostic(input map[string]any, currentActionContract map[string]any) map[string]any {
	for _, source := range []map[string]any{
		input,
		readModelMapValue(input["runtimeDiagnostics"]),
		readModelMapValue(input["lastFailureDiagnostics"]),
		currentActionContract,
	} {
		if diagnostic := readModelMapValue(source["validationFailureDiagnostic"]); len(diagnostic) > 0 {
			return compactStepValidationFailureDiagnosticForAudit(diagnostic)
		}
	}
	return nil
}

func selectStepContractRequiredOutput(input map[string]any, currentActionContract map[string]any) (map[string]any, string) {
	candidates := []struct {
		name  string
		value map[string]any
	}{
		{name: "currentActionContract.requiredOutput", value: readModelMapValue(currentActionContract["requiredOutput"])},
		{name: "requiredOutput", value: readModelMapValue(input["requiredOutput"])},
		{name: "repairActionPackage.requiredOutput", value: readModelMapValue(readModelMapValue(input["repairActionPackage"])["requiredOutput"])},
	}
	for _, candidate := range candidates {
		if len(candidate.value) > 0 {
			return candidate.value, candidate.name
		}
	}
	return nil, ""
}

func compactStepValidationFailureDiagnosticForAudit(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"schemaVersion",
		"class",
		"recommendedEvidenceTargetType",
		"stage",
		"operation",
		"commandKind",
		"reason",
		"violatedRule",
		"failureSignature",
	} {
		if value := readModelStringValue(input[key]); value != "" {
			ret[key] = value
		}
	}
	for _, key := range []string{
		"commandPolicyViolation",
		"stateChangingEvidenceNeeded",
		"implementationGap",
	} {
		if value, ok := input[key].(bool); ok {
			ret[key] = value
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func compactStepRequiredOutputForAudit(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"schemaVersion",
		"kind",
		"mode",
		"expectedState",
		"currentState",
		"coverageFocusId",
		"focusId",
		"nextActionKind",
		"actionKind",
	} {
		if value := readModelStringValue(input[key]); value != "" {
			ret[key] = value
		}
	}
	for _, key := range []string{
		"coverageWorkItemIds",
		"acceptedKinds",
		"allowedKinds",
		"forbiddenKinds",
		"mustNotProduceKinds",
		"requiredEvidenceKinds",
		"missingEvidenceKinds",
		"structuredTargetRefs",
		"targetRefs",
	} {
		if values := compactUniqueStrings(readModelStringSliceValue(input[key])); len(values) > 0 {
			ret[key] = values
		}
	}
	for _, key := range []string{
		"mustProduceConcreteValidationEvidence",
		"mustProduceStateChange",
		"mustReturnExecutionGraph",
		"mustReturnValidationEvidence",
		"mustReturnArtifactChunk",
		"mustReturnArtifactContextGraph",
		"assessmentAllowed",
		"decisionAllowed",
	} {
		if value, ok := input[key].(bool); ok {
			ret[key] = value
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func firstNonEmptyStringSlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

// source: step_output_allowed_keys.go

var stepOutputAllowedKeys = map[string]struct{}{
	"category":                     {},
	"completionState":              {},
	"error":                        {},
	"operation":                    {},
	"reason":                       {},
	"stage":                        {},
	"state":                        {},
	"status":                       {},
	"summary":                      {},
	"target":                       {},
	"targetRef":                    {},
	"workspaceExecutionSummary":    {},
	"workspaceExecutionSummaryRef": {},
}

func isStepOutputAllowedKey(key string) bool {
	if _, ok := stepOutputAllowedKeys[key]; ok {
		return true
	}
	if _, ok := stepOutputGovernanceAllowedKeys[key]; ok {
		return true
	}
	if _, ok := stepOutputRuntimeAllowedKeys[key]; ok {
		return true
	}
	_, ok := stepOutputLLMAllowedKeys[key]
	return ok
}

// source: step_output_governance_allowed_keys.go

var stepOutputGovernanceAllowedKeys = map[string]struct{}{
	"approvalId":           {},
	"assessmentOnly":       {},
	"assessmentOnlyReason": {},
	"blockingApprovalId":   {},
	"blockingTicketId":     {},
	"decisionId":           {},
	"dodAssessment":        {},
	"validationId":         {},
}

// source: step_output_llm_allowed_keys.go

var stepOutputLLMAllowedKeys = map[string]struct{}{
	"contextBudget":     {},
	"failureClass":      {},
	"failureReason":     {},
	"llmCallRefs":       {},
	"llmUsage":          {},
	"prompt":            {},
	"repairAttempt":     {},
	"streamDiagnostics": {},
}

// source: step_output_mcp.go

func projectMCPToolCallResult(value any) map[string]any {
	raw, ok := value.(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	ret := map[string]any{}
	if structured := projectMCPStructuredContent(raw["structuredContent"]); len(structured) > 0 {
		ret["structuredContent"] = structured
	}
	if isError, ok := raw["isError"].(bool); ok {
		ret["isError"] = isError
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: step_output_mcp_structured_content.go

func projectMCPStructuredContent(value any) map[string]any {
	raw, ok := value.(map[string]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"arguments": {},
		"name":      {},
		"ok":        {},
		"tool":      {},
		"transport": {},
	}
	ret := map[string]any{}
	for key, item := range raw {
		if _, ok := allowed[key]; !ok {
			continue
		}
		if projected, ok := projectReadModelScalarMapValue(item); ok {
			ret[key] = projected
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: step_output_runtime_allowed_keys.go

var stepOutputRuntimeAllowedKeys = map[string]struct{}{
	"capabilityId": {},
	"memoryRefs":   {},
	"nextKind":     {},
	"patchId":      {},
	"resultId":     {},
	"serverId":     {},
	"tool":         {},
	"toolCallId":   {},
	"transport":    {},
	"uri":          {},
}

// source: step_selection.go

// source: stream_timeline_window.go

const (
	maxRawStreamTimelineItems  = 32
	streamTimelineHeadItemKeep = 12
	streamTimelineTailItemKeep = 12
	streamTimelineDeltaRunes   = 512
)

func streamTimelineItemCapacity(total int) int {
	if total <= maxRawStreamTimelineItems {
		return total
	}
	return streamTimelineHeadItemKeep + streamTimelineTailItemKeep + 1
}

func streamTimelineEventOmitted(idx int, total int) bool {
	if total <= maxRawStreamTimelineItems {
		return false
	}
	return idx >= streamTimelineHeadItemKeep && idx < total-streamTimelineTailItemKeep
}

func streamEventOccurredAt(event map[string]any) string {
	for _, key := range []string{"occurredAt", "timestamp", "time"} {
		if value := readModelStringValue(event[key]); value != "" {
			return value
		}
	}
	return ""
}

func boundedStreamTimelineEvent(event map[string]any) map[string]any {
	ret := cloneReadModelMap(event)
	if len(ret) == 0 {
		return nil
	}
	if delta := readModelStringValue(ret["delta"]); delta != "" {
		preview, truncated, runeCount := streamTimelineDeltaPreview(delta)
		ret["delta"] = preview
		if truncated {
			ret["deltaTruncated"] = true
			ret["deltaRuneCount"] = runeCount
		}
	}
	if payload := readModelMapValue(ret["payload"]); len(payload) > 0 {
		if delta := readModelStringValue(payload["delta"]); delta != "" {
			preview, truncated, runeCount := streamTimelineDeltaPreview(delta)
			payload["delta"] = preview
			if truncated {
				payload["deltaTruncated"] = true
				payload["deltaRuneCount"] = runeCount
			}
		}
		ret["payload"] = payload
	}
	return ret
}

func boundedStreamStepOutput(output map[string]any) map[string]any {
	ret := cloneReadModelMap(output)
	if len(ret) == 0 {
		return nil
	}
	if events := readModelMapSliceValue(ret["streamEvents"]); len(events) > 0 {
		delete(ret, "streamEvents")
		ret["streamEventsOmitted"] = true
		ret["streamEventCount"] = len(events)
	}
	return ret
}

func streamTimelineDeltaPreview(value string) (string, bool, int) {
	runes := []rune(value)
	if len(runes) <= streamTimelineDeltaRunes {
		return value, false, len(runes)
	}
	return string(runes[:streamTimelineDeltaRunes]) + "...[truncated]", true, len(runes)
}

// source: string_compaction.go

func compactUniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	ret := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: task_decomposition_title.go

func taskNodeTitle(raw map[string]any) string {
	if title := readModelStringValue(raw["summary"]); title != "" {
		return title
	}
	input, _ := raw["input"].(map[string]any)
	if command := readModelStringValue(input["command"]); command != "" {
		return command
	}
	if tool := readModelStringValue(input["tool"]); tool != "" {
		return tool
	}
	if operation := readModelStringValue(input["operation"]); operation != "" {
		return operation
	}
	return readModelStringValue(raw["kind"])
}

// source: task_graph_value.go

func taskGraphFromValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneReadModelMap(typed)
	default:
		return nil
	}
}

// source: timeline_stream.go

// source: timeline_string_values.go

func firstNonEmptyTimelineString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// source: web_policy_decision.go

func readModelPolicyDecision(value any) string {
	policy := readModelMapValue(value)
	if len(policy) == 0 {
		return ""
	}
	return readModelStringValue(policy["decision"])
}

// source: workspace_projection.go

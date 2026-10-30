package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

// WD5 mechanical consolidation; source files are listed to preserve review traceability.

// source: decision_context_view.go

const (
	decisionContextStringLimit      = 512
	decisionContextSliceLimit       = 8
	decisionContextMapLimit         = 16
	decisionContextNestedMapLimit   = 10
	decisionContextRecentErrorLimit = 3
)

var decisionContextViewKeys = []string{
	"scope",
	"dependency",
	"recoveryPolicy",
	"resultPolicy",
	"allowedRecoveryActions",
	"preferredRecoveryActions",
	"blockerKind",
	"attempts",
	"maxAttempts",
	"failureClass",
	"failureSignature",
	"failureTargetScope",
	"failedStepId",
	"repairOfRootStepId",
	"failedStepKind",
	"missingDoDItems",
	"recommendedNextActions",
	"runLevelIneffectiveConvergence",
	"structuredFailureSignals",
}

func projectDecisionContextView(input workorcheventpayload.Payload) workorcheventpayload.Payload {
	if len(input.Fields) == 0 {
		return workorcheventpayload.Payload{}
	}
	ret := make([]workorcheventpayload.Field, 0, len(decisionContextViewKeys))
	for _, key := range decisionContextViewKeys {
		value, ok := input.Field(key)
		if !ok {
			continue
		}
		if projected, ok := projectDecisionContextValue(key, value, 0); ok {
			ret = append(ret, workorcheventpayload.Field{Name: key, Value: projected})
		}
	}
	return workorcheventpayload.New(ret...)
}

func projectDecisionPayloadView(input workorcheventpayload.Payload) workorcheventpayload.Payload {
	projected, ok := projectDecisionContextMap(input, 1)
	if !ok {
		return workorcheventpayload.Payload{}
	}
	return projected
}

func projectDecisionContextValue(key string, value workorcheventpayload.Value, depth int) (workorcheventpayload.Value, bool) {
	if depth > 2 {
		return workorcheventpayload.Value{}, false
	}
	switch value.Kind {
	case workorcheventpayload.ValueNull:
		return workorcheventpayload.Value{}, false
	case workorcheventpayload.ValueString:
		value.String = trimDecisionContextString(value.String)
		return value, true
	case workorcheventpayload.ValueBool, workorcheventpayload.ValueInt, workorcheventpayload.ValueUint, workorcheventpayload.ValueFloat:
		return value, true
	case workorcheventpayload.ValueArray:
		limit := decisionContextSliceLimit
		if key == "recentFailures" {
			limit = decisionContextRecentErrorLimit
		}
		ret := make([]workorcheventpayload.Value, 0, min(len(value.Array), limit))
		for idx, item := range value.Array {
			if idx >= limit {
				break
			}
			projected, ok := projectDecisionContextValue(key, item, depth+1)
			if ok {
				ret = append(ret, projected)
			}
		}
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray, Array: ret}, len(ret) > 0
	case workorcheventpayload.ValueObject:
		projected, ok := projectDecisionContextMap(workorcheventpayload.Payload{Fields: value.Object}, depth+1)
		return workorcheventpayload.Value{Kind: workorcheventpayload.ValueObject, Object: projected.Fields}, ok
	default:
		return workorcheventpayload.Value{}, false
	}
}

func projectDecisionContextMap(input workorcheventpayload.Payload, depth int) (workorcheventpayload.Payload, bool) {
	if len(input.Fields) == 0 || depth > 2 {
		return workorcheventpayload.Payload{}, false
	}
	limit := decisionContextNestedMapLimit
	if depth == 1 {
		limit = decisionContextMapLimit
	}
	ret := make([]workorcheventpayload.Field, 0, min(len(input.Fields), limit))
	count := 0
	for _, key := range orderedDecisionContextMapKeys(input) {
		if count >= limit {
			break
		}
		value, _ := input.Field(key)
		projected, ok := projectDecisionContextValue(key, value, depth)
		if !ok {
			continue
		}
		ret = append(ret, workorcheventpayload.Field{Name: key, Value: projected})
		count++
	}
	return workorcheventpayload.New(ret...), len(ret) > 0
}

func orderedDecisionContextMapKeys(input workorcheventpayload.Payload) []string {
	preferred := []string{
		"totalSteps",
		"recoverySteps",
		"contractFailures",
		"providerFailures",
		"failedLLMSteps",
		"minRecoverySteps",
		"minContractFailures",
		"recentFailures",
		"stepId",
		"stage",
		"failureClass",
		"failureSignature",
		"identifiers",
		"messages",
		"sources",
	}
	ret := make([]string, 0, len(input.Fields))
	seen := map[string]struct{}{}
	for _, key := range preferred {
		if _, ok := input.Field(key); ok {
			ret = append(ret, key)
			seen[key] = struct{}{}
		}
	}
	for _, field := range input.Fields {
		if _, ok := seen[field.Name]; ok {
			continue
		}
		ret = append(ret, field.Name)
	}
	return ret
}

func trimDecisionContextString(input string) string {
	if len(input) <= decisionContextStringLimit {
		return input
	}
	return input[:decisionContextStringLimit] + "..."
}

// source: event_payload_list_projection.go

func projectEventPayloadArray(values []workorcheventpayload.Value) []workorcheventpayload.Value {
	limit := min(len(values), eventPayloadListLimit)
	ret := make([]workorcheventpayload.Value, 0, limit)
	for _, item := range values[:limit] {
		if projected, ok := projectEventPayloadValue(item); ok {
			ret = append(ret, projected)
		}
	}
	return ret
}

func projectEventPayloadObject(fields []workorcheventpayload.Field) []workorcheventpayload.Field {
	ret := make([]workorcheventpayload.Field, 0, len(fields))
	for _, field := range fields {
		if projected, ok := projectEventPayloadValue(field.Value); ok {
			ret = append(ret, workorcheventpayload.Field{Name: field.Name, Value: projected})
		}
	}
	return ret
}

// source: event_payload_projection.go

const (
	eventPayloadStringLimit = 512
	eventPayloadListLimit   = 16
)

func projectEventPayload(payload workorcheventpayload.Payload) workorcheventpayload.Payload {
	if len(payload.Fields) == 0 {
		return workorcheventpayload.Payload{}
	}
	fields := make([]workorcheventpayload.Field, 0, len(payload.Fields))
	for _, field := range payload.Fields {
		if !isEventPayloadAllowedKey(field.Name) {
			continue
		}
		if projected, ok := projectEventPayloadValue(field.Value); ok {
			fields = append(fields, workorcheventpayload.Field{Name: field.Name, Value: projected})
		}
	}
	return workorcheventpayload.New(fields...)
}

// source: event_payload_value_projection.go

func projectEventPayloadValue(value workorcheventpayload.Value) (workorcheventpayload.Value, bool) {
	switch value.Kind {
	case workorcheventpayload.ValueNull:
		return workorcheventpayload.Value{}, false
	case workorcheventpayload.ValueString:
		if len(value.String) > eventPayloadStringLimit {
			value.String = value.String[:eventPayloadStringLimit] + "..."
		}
		return value, true
	case workorcheventpayload.ValueBool, workorcheventpayload.ValueInt, workorcheventpayload.ValueUint, workorcheventpayload.ValueFloat:
		return value, true
	case workorcheventpayload.ValueArray:
		value.Array = projectEventPayloadArray(value.Array)
		return value, len(value.Array) > 0
	case workorcheventpayload.ValueObject:
		value.Object = projectEventPayloadObject(value.Object)
		return value, len(value.Object) > 0
	default:
		return workorcheventpayload.Value{}, false
	}
}

// source: llm_context_budget_audit_sanitize.go

// sanitizeLLMContextBudgetAudit keeps the read model on the bounded audit
// contract. Event records are persisted data and therefore must not turn an
// unknown nested field into a second prompt or owner-payload transport.
//
// The allow-list mirrors llmaudit.ContextBudget.Payload. It intentionally
// excludes raw prompt/profile/cache-plan objects; section identity is carried
// through the bounded sectionHashes projection instead.
func sanitizeLLMContextBudgetAudit(input workorcheventpayload.Payload) workorcheventpayload.Payload {
	if len(input.Fields) == 0 {
		return workorcheventpayload.Payload{}
	}
	fields := make([]workorcheventpayload.Field, 0, len(llmContextBudgetAuditFieldNames)+1)
	for _, name := range llmContextBudgetAuditFieldNames {
		if value, ok := input.Field(name); ok {
			fields = append(fields, workorcheventpayload.Field{Name: name, Value: value})
		}
	}
	// Keep versioned groups closed as well: nested arbitrary payloads must
	// never become a prompt-content transport through the read model.
	for _, group := range []struct {
		name   string
		fields []string
	}{
		{"processing", []string{"includedSections", "normalizedSections", "compressedSections", "omittedSections", "redactedPaths", "protectedFields", "integrityWarnings", "sourceRefs"}},
		{"cacheObservation", []string{"schemaVersion", "renderer", "renderContractRef", "messageLayout", "stage", "metadataState", "sourceSectionCount", "recordedSectionCount", "requestedHints", "effectiveHints"}},
	} {
		if object, ok := input.Object(group.name); ok {
			var safe []workorcheventpayload.Field
			for _, name := range group.fields {
				if value, ok := object.Field(name); ok {
					safe = append(safe, workorcheventpayload.Field{Name: name, Value: value})
				}
			}
			fields = append(fields, workorcheventpayload.ObjectField(group.name, workorcheventpayload.New(safe...)))
		}
	}
	if value, ok := input.Field("sectionHashes"); ok && value.Kind == workorcheventpayload.ValueArray {
		sections := make([]workorcheventpayload.Value, 0, len(value.Array))
		for _, item := range value.Array {
			if item.Kind != workorcheventpayload.ValueObject {
				continue
			}
			section := workorcheventpayload.Payload{Fields: item.Object}
			id, ok := section.String("id")
			if !ok || id == "" {
				continue
			}
			sectionFields := []workorcheventpayload.Field{workorcheventpayload.StringField("id", id)}
			if hash, ok := section.String("hash"); ok {
				sectionFields = append(sectionFields, workorcheventpayload.StringField("hash", hash))
			}
			if size, ok := section.Int64("sizeBytes"); ok {
				sectionFields = append(sectionFields, workorcheventpayload.IntField("sizeBytes", size))
			}
			if refs, ok := section.StringSlice("sourceRefs"); ok {
				sectionFields = append(sectionFields, workorcheventpayload.StringArrayField("sourceRefs", refs))
			}
			sections = append(sections, workorcheventpayload.Value{Kind: workorcheventpayload.ValueObject, Object: workorcheventpayload.New(sectionFields...).Fields})
		}
		if len(sections) > 0 {
			fields = append(fields, workorcheventpayload.Field{Name: "sectionHashes", Value: workorcheventpayload.Value{Kind: workorcheventpayload.ValueArray, Array: sections}})
		}
	}
	return workorcheventpayload.New(fields...)
}

var llmContextBudgetAuditFieldNames = []string{
	"schemaVersion",
	"providerId", "modelId", "capabilityRevision", "configurationRevision",
	"contextWindowTokens", "providerMaxOutputTokens", "effectiveMaxOutputTokens",
	"maxBytes", "actualBytes", "maxTokens", "estimatedInputTokens",
	"estimatedOutputTokens", "actualTokens", "maxRequestBytes", "profileMode",
	"capabilitySource", "providerSubmitted", "overBudget", "truncated",
	"stablePrefixHash", "renderedStablePrefixHash", "cacheStrategy",
	"contextGateAction", "contextGateReason", "budgetFailureClass",
	"includedSections", "normalizedSections", "compressedSections", "omittedSections",
	"redactedPaths", "protectedFields", "integrityWarnings", "sourceRefs",
}

// source: llm_context_budget_flags.go

func llmContextBudgetTruncated(budget workorcheventpayload.Payload) bool {
	value, _ := budget.Bool("truncated")
	return value
}

func llmContextBudgetGoalDoDCompressed(budget workorcheventpayload.Payload) bool {
	value, _ := budget.Bool("goalDoDCompressed")
	return value
}

func llmContextBudgetOverBudget(budget workorcheventpayload.Payload) bool {
	value, _ := budget.Bool("overBudget")
	return value
}

func llmContextBudgetP0Overflow(budget workorcheventpayload.Payload) bool {
	value, _ := budget.Bool("p0Overflow")
	return value
}

func llmContextBudgetPromptCaptured(budget workorcheventpayload.Payload) bool {
	value, _ := budget.Bool("promptCaptured")
	return value
}

func llmContextBudgetProviderSubmitted(budget workorcheventpayload.Payload) bool {
	value, _ := budget.Bool("providerSubmitted")
	return value
}

// source: llm_context_budget_identity.go

func llmContextBudgetProviderID(budget workorcheventpayload.Payload, usage workorcheventpayload.Payload) string {
	return firstNonEmptyTimelineString(
		llmContextBudgetString(budget, "providerId"),
		llmContextBudgetString(usage, "providerId"),
		llmContextBudgetString(usage, "providerID"),
	)
}

func llmContextBudgetModel(budget workorcheventpayload.Payload, usage workorcheventpayload.Payload) string {
	return firstNonEmptyTimelineString(
		llmContextBudgetString(budget, "modelId"),
		llmContextBudgetString(budget, "model"),
		llmContextBudgetString(usage, "model"),
		llmContextBudgetString(usage, "modelId"),
	)
}

func llmContextBudgetCapabilityRevision(budget workorcheventpayload.Payload) string {
	return firstNonEmptyTimelineString(
		llmContextBudgetString(budget, "capabilityRevision"),
		llmContextBudgetString(budget, "CapabilityRevision"),
	)
}

func llmContextBudgetConfigurationRevision(budget workorcheventpayload.Payload) string {
	return firstNonEmptyTimelineString(
		llmContextBudgetString(budget, "configurationRevision"),
		llmContextBudgetString(budget, "ConfigurationRevision"),
	)
}

// source: llm_context_budget_metadata.go

func llmContextBudgetString(budget workorcheventpayload.Payload, key string) string {
	value, _ := budget.String(key)
	return value
}

func llmContextBudgetProfileMode(budget workorcheventpayload.Payload) string {
	return llmContextBudgetString(budget, "profileMode")
}
func llmContextBudgetCapabilitySource(budget workorcheventpayload.Payload) string {
	return llmContextBudgetString(budget, "capabilitySource")
}
func llmContextBudgetContextUtilization(budget workorcheventpayload.Payload) float64 {
	value, _ := budget.Float64("contextUtilizationRatio")
	return value
}
func llmContextBudgetMaxRequestBytes(budget workorcheventpayload.Payload) int {
	return llmContextBudgetInt(budget, "maxRequestBytes")
}
func llmContextBudgetStageBudgetRatio(budget workorcheventpayload.Payload) float64 {
	value, _ := budget.Float64("stageBudgetRatio")
	return value
}
func llmContextBudgetStageBudgetSource(budget workorcheventpayload.Payload) string {
	return llmContextBudgetString(budget, "stageBudgetSource")
}
func llmContextBudgetTracePolicy(budget workorcheventpayload.Payload) string {
	return llmContextBudgetString(budget, "tracePolicy")
}
func llmContextBudgetEstimator(budget workorcheventpayload.Payload) string {
	return llmContextBudgetString(budget, "estimator")
}
func llmContextBudgetContextGateAction(budget workorcheventpayload.Payload) string {
	return llmContextBudgetString(budget, "contextGateAction")
}
func llmContextBudgetContextGateReason(budget workorcheventpayload.Payload) string {
	return llmContextBudgetString(budget, "contextGateReason")
}
func llmContextBudgetBudgetFailureClass(budget workorcheventpayload.Payload) string {
	return llmContextBudgetString(budget, "budgetFailureClass")
}

// source: llm_context_budget_numbers.go

func llmContextBudgetInt(budget workorcheventpayload.Payload, key string) int {
	value, _ := budget.Int64(key)
	return int(value)
}

func llmContextBudgetMaxBytes(budget workorcheventpayload.Payload) int {
	return llmContextBudgetInt(budget, "maxBytes")
}
func llmContextBudgetActualBytes(budget workorcheventpayload.Payload) int {
	return llmContextBudgetInt(budget, "actualBytes")
}
func llmContextBudgetMaxTokens(budget workorcheventpayload.Payload) int {
	return llmContextBudgetInt(budget, "maxTokens")
}
func llmContextBudgetEstimatedInputTokens(budget workorcheventpayload.Payload) int {
	return llmContextBudgetInt(budget, "estimatedInputTokens")
}
func llmContextBudgetEstimatedOutputTokens(budget workorcheventpayload.Payload) int {
	return llmContextBudgetInt(budget, "estimatedOutputTokens")
}
func llmContextBudgetActualTokens(budget workorcheventpayload.Payload) int {
	return llmContextBudgetInt(budget, "actualTokens")
}
func llmContextBudgetShrinkLevel(budget workorcheventpayload.Payload) int {
	return llmContextBudgetInt(budget, "shrinkLevel")
}

// source: llm_context_budget_payload_maps.go

func llmContextBudgetProfile(budget workorcheventpayload.Payload) workorcheventpayload.Payload {
	value, _ := budget.Object("profile")
	return value
}

func llmContextBudgetCachePlan(budget workorcheventpayload.Payload) workorcheventpayload.Payload {
	value, _ := budget.Object("cachePlan")
	return value
}

// source: typed_event_payload_values.go

func eventPayloadString(payload workorcheventpayload.Payload, key string) string {
	value, _ := payload.String(key)
	return value
}

func eventPayloadInt(payload workorcheventpayload.Payload, key string) int {
	value, _ := payload.Int64(key)
	return int(value)
}

func eventPayloadBool(payload workorcheventpayload.Payload, key string) bool {
	value, _ := payload.Bool(key)
	return value
}

func eventPayloadMap(payload workorcheventpayload.Payload, key string) map[string]any {
	value, ok := payload.Object(key)
	if !ok {
		return nil
	}
	return value.Map()
}

package orchestrator

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const (
	failureClassProviderUnavailable      = "provider_unavailable"
	failureClassProviderSchemaDrift      = "provider_schema_drift"
	failureClassProviderProtocolInvalid  = "provider_protocol_invalid"
	failureClassProviderTimeout          = "provider_timeout"
	failureClassContextTooLarge          = "context_too_large"
	failureClassRateLimited              = "rate_limited"
	failureClassAuthOrConfigError        = "auth_or_config_error"
	failureClassProviderStreamIncomplete = "provider_stream_incomplete"
	failureClassLLMDecisionInvalid       = "llm_decision_invalid"
	failureClassDoDCoverageMissing       = "dod_coverage_missing"
	failureClassCoverageActionableGap    = "coverage_actionable_gap"
	failureClassWorkspacePath            = "workspace_path_conflict"
	failureClassShellCommand             = "shell_command_failed"
	failureClassPatchApply               = "patch_apply_failed"
	failureClassMCPCapabilityDenied      = "mcp_capability_denied"
	failureClassMCPRuntime               = "mcp_runtime_failed"
	failureClassRuntime                  = "runtime_failed"

	failureOutputKeyClass     = "failureClass"
	failureOutputKeySignature = "failureSignature"
)

type runtimeFailureClassification struct {
	Class     string
	Stage     string
	Signature string
}

func classifyRuntimeFailure(step *runModel.Step, output map[string]any, summary string, execErr error) runtimeFailureClassification {
	stage := strings.TrimSpace(stringFromMap(output, "stage"))
	diagnostics := mapFromValue(output["runtimeDiagnostics"])
	if kind := strings.TrimSpace(stringFromMap(diagnostics, "kind")); kind == failureClassWorkspacePath {
		classification := runtimeFailureClassification{Class: failureClassWorkspacePath, Stage: stage}
		classification.Signature = runtimeFailureSignature(classification.Class, step, output, execErr)
		return classification
	}

	class := failureClassRuntime
	lowerStage := strings.ToLower(stage)
	lowerSummary := strings.ToLower(strings.TrimSpace(summary))
	if isLLMProviderTimeoutFailure(lowerStage, execErr, output) {
		class = failureClassProviderTimeout
	} else if isLLMContextTooLargeFailure(lowerStage, execErr, output) {
		class = failureClassContextTooLarge
	} else if isLLMProviderRateLimitedFailure(lowerStage, execErr, output) {
		class = failureClassRateLimited
	} else if isLLMProviderStreamIncompleteFailure(lowerStage, execErr, output) {
		class = failureClassProviderStreamIncomplete
	} else if isDoDCoverageMissingFailure(lowerStage, execErr, output) {
		class = failureClassDoDCoverageMissing
	} else if isRuntimeContractFailure(lowerStage, execErr, output) {
		class = failureClassRuntime
	} else if isLLMProviderAuthOrConfigFailure(lowerStage, execErr, output) {
		class = failureClassAuthOrConfigError
	} else if isLLMProviderProtocolInvalidFailure(lowerStage) {
		class = failureClassProviderProtocolInvalid
	} else if isLLMDecisionContractFailureStage(lowerStage) {
		class = failureClassLLMDecisionInvalid
	} else if isLLMProviderSchemaFailure(lowerStage) {
		class = failureClassProviderSchemaDrift
	} else if isLLMProviderUnavailableFailure(lowerStage) {
		class = failureClassProviderUnavailable
	} else if strings.Contains(lowerSummary, "patch application") {
		class = failureClassPatchApply
	} else if isMCPRejectedFailure(lowerStage) {
		class = failureClassMCPCapabilityDenied
	} else if isMCPRuntimeFailure(lowerStage) {
		class = failureClassMCPRuntime
	} else if isShellCommandFailure(step, output) {
		class = failureClassShellCommand
	}

	classification := runtimeFailureClassification{Class: class, Stage: stage}
	classification.Signature = runtimeFailureSignature(classification.Class, step, output, execErr)
	return classification
}

func attachRuntimeFailureClassification(output map[string]any, classification runtimeFailureClassification) {
	if output == nil {
		return
	}
	if classification.Class != "" {
		output[failureOutputKeyClass] = classification.Class
	}
	if classification.Signature != "" {
		output[failureOutputKeySignature] = classification.Signature
	}
}

func isLLMProviderTimeoutFailure(lowerStage string, execErr error, output map[string]any) bool {
	if !strings.HasPrefix(lowerStage, "llm_") {
		return false
	}
	if lowerStage == "llm_provider_timeout" || strings.Contains(lowerStage, "timeout") {
		return true
	}
	errText := providerFailureErrorText(execErr, output)
	return strings.Contains(errText, "deadline exceeded") || strings.Contains(errText, "timeout")
}

func isLLMContextTooLargeFailure(lowerStage string, execErr error, output map[string]any) bool {
	if !strings.HasPrefix(lowerStage, "llm_") {
		return false
	}
	if lowerStage == "llm_provider_request_too_large" {
		return true
	}
	errText := providerFailureErrorText(execErr, output)
	return strings.Contains(errText, "context length") ||
		strings.Contains(errText, "maximum context") ||
		strings.Contains(errText, "context window") ||
		strings.Contains(errText, "too many tokens") ||
		strings.Contains(errText, "token limit") ||
		strings.Contains(errText, "request too large") ||
		strings.Contains(errText, "input is too long") ||
		strings.Contains(errText, "prompt is too long")
}

func isLLMProviderRateLimitedFailure(lowerStage string, execErr error, output map[string]any) bool {
	if !strings.HasPrefix(lowerStage, "llm_") {
		return false
	}
	if lowerStage == "llm_provider_rate_limited" {
		return true
	}
	errText := providerFailureErrorText(execErr, output)
	return strings.Contains(errText, "rate limit") ||
		strings.Contains(errText, "too many requests") ||
		strings.Contains(errText, "quota exceeded") ||
		strings.Contains(errText, "http status 429")
}

func isLLMProviderStreamIncompleteFailure(lowerStage string, execErr error, output map[string]any) bool {
	if !strings.HasPrefix(lowerStage, "llm_") {
		return false
	}
	if lowerStage == "llm_provider_stream_incomplete" {
		return true
	}
	errText := providerFailureErrorText(execErr, output)
	return strings.Contains(errText, "stream incomplete") ||
		strings.Contains(errText, "ended before [done]") ||
		strings.Contains(errText, "without [done]")
}

func isDoDCoverageMissingFailure(lowerStage string, execErr error, output map[string]any) bool {
	if diagnostics := mapFromValue(output["runtimeDiagnostics"]); strings.TrimSpace(stringFromMap(diagnostics, "kind")) == failureClassDoDCoverageMissing {
		return true
	}
	if lowerStage != "llm_dod_assessment" && lowerStage != "llm_execution_graph_plan" {
		return false
	}
	errText := strings.ToLower(firstNonEmptyString(errorString(execErr), stringFromMap(output, "error"), stringFromMap(output, "reason")))
	return strings.Contains(errText, "dodassessment does not cover all current dod items") ||
		strings.Contains(errText, "dodassessment does not cover all current goal requirements") ||
		strings.Contains(errText, "dodassessment.goalcoverage item and state are required") ||
		strings.Contains(errText, "dodassessment.goalcoverage.items are required") ||
		strings.Contains(errText, "dodassessment.goalcoverage.satisfied is required") ||
		strings.Contains(errText, "satisfied dodassessment.goalcoverage item requires evidencerefs") ||
		strings.Contains(errText, "dodassessment.deliverablecoverage missing required deliverables") ||
		strings.Contains(errText, "acceptancedimensions missing required dimensions")
}

func isLLMProviderAuthOrConfigFailure(lowerStage string, execErr error, output map[string]any) bool {
	if !strings.HasPrefix(lowerStage, "llm_provider_") {
		return false
	}
	if lowerStage == "llm_provider_auth_or_config_error" {
		return true
	}
	errText := providerFailureErrorText(execErr, output)
	if providerFailureHTTPStatus(errText) >= 400 {
		return false
	}
	return strings.Contains(errText, "unauthorized") ||
		strings.Contains(errText, "forbidden") ||
		strings.Contains(errText, "invalid api key") ||
		strings.Contains(errText, "authentication") ||
		strings.Contains(errText, "api key")
}

func isRuntimeContractFailure(lowerStage string, execErr error, output map[string]any) bool {
	if lowerStage == "llm_context_integrity_failed" {
		return true
	}
	if lowerStage != "llm_execution_graph_plan" &&
		lowerStage != "llm_dod_assessment" &&
		lowerStage != "patch_apply" {
		return false
	}
	errText := providerFailureErrorText(execErr, output)
	if errText == "" {
		return false
	}
	for _, marker := range []string{
		"strategyreassessment.required",
		"state-changing llm repair",
		"state-changing execution-stage",
		"runtime repair",
		"scoped state-changing runtime repair",
		"executiongraph patch/apply",
		"patch/apply input invalid",
		"patch describes a non-substantive",
		"llm decision json invalid",
	} {
		if strings.Contains(errText, marker) {
			return true
		}
	}
	return false
}

func isLLMProviderSchemaFailure(lowerStage string) bool {
	switch lowerStage {
	case "llm_provider_schema_drift",
		"llm_provider_response_schema_invalid":
		return true
	default:
		return false
	}
}

func isLLMProviderProtocolInvalidFailure(lowerStage string) bool {
	return lowerStage == "llm_provider_protocol_invalid"
}

func isLLMDecisionContractFailureStage(stage string) bool {
	switch strings.ToLower(strings.TrimSpace(stage)) {
	case "llm_schema_invalid", "llm_decision_invalid":
		return true
	default:
		return false
	}
}

func isLLMProviderUnavailableFailure(lowerStage string) bool {
	switch lowerStage {
	case "llm_provider_unavailable",
		"llm_provider_client_unavailable",
		"llm_provider_operation_unsupported",
		"llm_provider_timeout",
		"llm_provider_request_too_large",
		"llm_provider_rate_limited",
		"llm_provider_auth_or_config_error",
		"llm_provider_stream_incomplete",
		"llm_provider_error",
		"llm_provider_stream_error",
		"llm_provider_request_failed":
		return true
	default:
		return false
	}
}

func providerFailureErrorText(execErr error, output map[string]any) string {
	return strings.ToLower(firstNonEmptyString(errorString(execErr), stringFromMap(output, "error"), stringFromMap(output, "reason")))
}

func providerFailureHTTPStatus(text string) int {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return 0
	}
	match := providerFailureHTTPStatusPattern.FindStringSubmatch(text)
	if len(match) != 2 {
		return 0
	}
	status, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return status
}

func isMCPRejectedFailure(lowerStage string) bool {
	return strings.HasPrefix(lowerStage, "mcp_") && strings.HasSuffix(lowerStage, "_rejected")
}

func isMCPRuntimeFailure(lowerStage string) bool {
	if !strings.HasPrefix(lowerStage, "mcp_") {
		return false
	}
	return strings.HasSuffix(lowerStage, "_failed") ||
		strings.HasSuffix(lowerStage, "_timeout") ||
		strings.HasSuffix(lowerStage, "_unavailable") ||
		strings.HasSuffix(lowerStage, "_server_unavailable") ||
		strings.HasSuffix(lowerStage, "_invalid_endpoint") ||
		strings.HasSuffix(lowerStage, "_unsupported_transport")
}

func isShellCommandFailure(step *runModel.Step, output map[string]any) bool {
	if step == nil || step.Kind != "shell/command" {
		return false
	}
	if strings.TrimSpace(stringFromMap(output, "command")) != "" {
		return true
	}
	if _, ok := output["exitCode"]; ok {
		return true
	}
	return strings.TrimSpace(stringFromMap(output, "stderr")) != ""
}

func runtimeFailureSignature(class string, step *runModel.Step, output map[string]any, execErr error) string {
	parts := []string{
		"class=" + strings.TrimSpace(class),
		"kind=" + normalizedSignatureText(stepKind(step)),
		"stage=" + normalizedSignatureText(stringFromMap(output, "stage")),
	}
	if nodeID := normalizedSignatureText(graphNodeID(step)); nodeID != "" {
		parts = append(parts, "node="+nodeID)
	}
	for _, key := range []string{"targetRef", "conflictRef", "serverId", "tool", "uri", "prompt", "command"} {
		if value := normalizedSignatureText(stringFromMap(output, key)); value != "" {
			parts = append(parts, key+"="+value)
		}
	}
	if diagnostics := mapFromValue(output["runtimeDiagnostics"]); diagnostics != nil {
		for _, key := range []string{"targetRef", "conflictRef", "conflictKind", "operation"} {
			if value := normalizedSignatureText(stringFromMap(diagnostics, key)); value != "" {
				parts = append(parts, key+"="+value)
			}
		}
	}
	if targets := failureTargetsForSignature(output); len(targets) > 0 {
		parts = append(parts, "failureTargets="+strings.Join(targets, ","))
	}
	if exitCode, ok := normalizedSignatureNumber(output["exitCode"]); ok {
		parts = append(parts, "exitCode="+exitCode)
	}
	errText := firstNonEmptyString(stringFromMap(output, "error"), stringFromMap(output, "reason"), errorString(execErr), stringFromMap(output, "stderr"))
	if value := normalizedSignatureText(errText); value != "" {
		parts = append(parts, "error="+value)
	}
	return strings.Join(parts, "|")
}

func failureTargetsForSignature(output map[string]any) []string {
	if len(output) == 0 {
		return nil
	}
	if targets := explicitFailureTargetsForSignature(output); len(targets) > 0 {
		return targets
	}
	if externalCapabilityFailureHasCallIdentity(output) {
		return nil
	}
	text := strings.Join([]string{
		stringFromMap(output, "stderr"),
		stringFromMap(output, "stdout"),
		stringFromMap(output, "error"),
		stringFromMap(output, "reason"),
	}, "\n")
	matches := failureTargetLocatorPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var ret []string
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		target := normalizedFailureTarget(match[1])
		if target == "" || !runtimeRepairTargetRefAdmissible(target) || seen[target] {
			continue
		}
		seen[target] = true
		ret = append(ret, target)
		if len(ret) >= 5 {
			break
		}
	}
	return ret
}

var failureTargetLocatorPattern = regexp.MustCompile(`(?m)(?:^|[\s"'(])([A-Za-z0-9_.\/\\-]+\.[A-Za-z0-9_+-]+):[0-9]+(?::[0-9]+)?`)

func explicitFailureTargetsForSignature(output map[string]any) []string {
	seen := map[string]bool{}
	ret := []string{}
	add := func(value string) {
		target := normalizedFailureTarget(value)
		if target == "" || !runtimeRepairTargetRefAdmissible(target) || seen[target] {
			return
		}
		seen[target] = true
		ret = append(ret, target)
	}
	add(stringFromMap(output, "targetRef"))
	add(stringFromMap(output, "conflictRef"))
	for _, target := range repairPatchTargetRefsFromScopeText(firstNonEmptyString(
		stringFromMap(output, "failureTargetScope"),
		stringFromMap(output, "targetScope"),
	)) {
		add(target)
	}
	for _, target := range stringSliceFromValue(output["failureTargets"]) {
		add(target)
	}
	addPatchPayloadFailureTargets(output, add)
	if diagnostics := mapFromValue(output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		add(stringFromMap(diagnostics, "targetRef"))
		add(stringFromMap(diagnostics, "conflictRef"))
		for _, target := range repairPatchTargetRefsFromScopeText(firstNonEmptyString(
			stringFromMap(diagnostics, "failureTargetScope"),
			stringFromMap(diagnostics, "targetScope"),
		)) {
			add(target)
		}
		for _, target := range stringSliceFromValue(diagnostics["failureTargets"]) {
			add(target)
		}
	}
	for _, target := range capabilityInputFailureTargets(output) {
		add(target)
	}
	return limitedStrings(ret, 5)
}

func addPatchPayloadFailureTargets(value map[string]any, add func(string)) {
	if len(value) == 0 || add == nil {
		return
	}
	for _, key := range []string{"patches", "patchRequests"} {
		for _, item := range mapSliceFromValue(value[key]) {
			add(stringFromMap(item, "targetRef"))
			add(stringFromMap(item, "conflictRef"))
		}
	}
	if decision := mapFromValue(value[stepOutputKeyLLMDecision]); len(decision) > 0 {
		addPatchPayloadFailureTargets(decision, add)
	}
}

func capabilityInputFailureTargets(output map[string]any) []string {
	if len(output) == 0 {
		return nil
	}
	if !externalCapabilityFailureHasCallIdentity(output) {
		return nil
	}
	input := mapFromValue(output["input"])
	if len(input) == 0 {
		return nil
	}
	targets := []string{}
	for _, key := range []string{"targetRef", "path", "file"} {
		if value := stringFromMap(input, key); strings.TrimSpace(value) != "" {
			targets = append(targets, value)
		}
	}
	return targets
}

func externalCapabilityFailureHasCallIdentity(output map[string]any) bool {
	if len(output) == 0 {
		return false
	}
	if mapHasExternalCapabilityCallIdentity(output) {
		return true
	}
	return mapHasExternalCapabilityCallIdentity(mapFromValue(output["runtimeDiagnostics"]))
}

func mapHasExternalCapabilityCallIdentity(values map[string]any) bool {
	if len(values) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromMap(values, "serverId")) != "" ||
		strings.TrimSpace(stringFromMap(values, "tool")) != "" {
		return true
	}
	for _, key := range []string{"kind", "operation"} {
		value := strings.TrimSpace(stringFromMap(values, key))
		if value == "mcp/tool_call" {
			return true
		}
	}
	return strings.HasPrefix(strings.TrimSpace(stringFromMap(values, "stage")), "mcp_")
}

func normalizedFailureTarget(value string) string {
	value = strings.TrimSpace(strings.Trim(value, `"'()[]{}<>`))
	value = strings.ReplaceAll(value, `\`, `/`)
	value = strings.TrimPrefix(value, "./")
	return normalizedSignatureText(value)
}

func normalizedSignatureNumber(value any) (string, bool) {
	switch typed := value.(type) {
	case int:
		return strconv.Itoa(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case float64:
		return strconv.FormatInt(int64(typed), 10), true
	case jsonNumber:
		return typed.String(), true
	default:
		return "", false
	}
}

type jsonNumber interface {
	String() string
}

var signatureWhitespace = regexp.MustCompile(`\s+`)

func normalizedSignatureText(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	value = signatureWhitespace.ReplaceAllString(value, " ")
	if len(value) > 240 {
		value = value[:240]
	}
	return value
}

func stepKind(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	return step.Kind
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func shouldBlockOnDuplicateRuntimeFailure(
	ctx context.Context,
	steps repository.StepRepository,
	repair *repairSubsystem,
	policy RuntimePolicy,
	canCreateDecisionTicket bool,
	run *runModel.Run,
	failedStep *runModel.Step,
) bool {
	if steps == nil || repair == nil || run == nil || failedStep == nil || !canCreateDecisionTicket {
		return false
	}
	signature := strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeySignature))
	class := strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeyClass))
	targetScope := runtimeFailureTargetScopeFromStep(failedStep)
	attempts := repair.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep)
	if signature == "" || !runtimeFailureEligibleForDuplicateEscalation(failedStep, class) || attempts == 0 {
		return false
	}
	maxAttempts := repair.effectiveMaxRuntimeRepairAttempts(ctx, policy, run, failedStep)
	if requiresDoDAssessmentCompletionGate(run) && schemaInvalidRepairCanContinueWithinBudget(failedStep) && attempts < maxAttempts {
		return false
	}
	if runtimeFailureCanContinueMechanicallyBeforeDuplicateEscalation(class, targetScope, attempts, maxAttempts) {
		return false
	}
	stepsForRun, err := steps.ListByRunSummary(ctx, run.ID)
	if err != nil {
		return false
	}
	duplicateFound := false
	targetDiagnosticDuplicateFound := false
	for _, step := range stepsForRun {
		if step == nil || step.ID == failedStep.ID || step.State != common.StepStateFailed {
			continue
		}
		if strings.TrimSpace(stringFromMap(step.Output, failureOutputKeySignature)) == signature {
			duplicateFound = true
			if runtimeFailureHasRepeatedTargetDiagnostic(class, failedStep, step) {
				targetDiagnosticDuplicateFound = true
			}
			break
		}
		if targetScope != "" &&
			strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass)) == class &&
			runtimeFailureTargetScopeFromStep(step) == targetScope {
			duplicateFound = true
			if runtimeFailureHasRepeatedTargetDiagnostic(class, failedStep, step) {
				targetDiagnosticDuplicateFound = true
			}
			break
		}
	}
	if !duplicateFound {
		return false
	}
	if runtimeFailureRepeatedTargetDiagnosticCanEscalate(class, targetScope, attempts, maxAttempts, targetDiagnosticDuplicateFound) {
		return true
	}
	if requiresDoDAssessmentCompletionGate(run) && attempts < maxAttempts &&
		!runtimeFailureTargetUsesRunBudget(class, targetScope) &&
		class != failureClassLLMDecisionInvalid &&
		!runtimeFailureIsLLMContractLoop(failedStep) {
		return false
	}
	if runtimeFailureNeedsNoNewEvidenceCheck(class, failedStep) &&
		runtimeFailureRunHasCompletedActionEvidence(stepsForRun, failedStep.ID) {
		return false
	}
	return true
}

func runtimeFailureRepeatedTargetDiagnosticCanEscalate(class string, targetScope string, attempts int, maxAttempts int, duplicateFound bool) bool {
	if !duplicateFound || maxAttempts <= 0 || attempts < maxAttempts || strings.TrimSpace(targetScope) == "" {
		return false
	}
	switch strings.TrimSpace(class) {
	case failureClassShellCommand, failureClassPatchApply, failureClassMCPRuntime:
		return true
	default:
		return false
	}
}

func runtimeFailureHasRepeatedTargetDiagnostic(class string, current *runModel.Step, previous *runModel.Step) bool {
	if current == nil || previous == nil {
		return false
	}
	currentSignals := structuredFailureSignalsFromFailureOutput(current.Output, nil, class, stringFromMap(current.Output, failureOutputKeySignature))
	if !structuredFailureSignalsHaveConcreteTargetEvidence(currentSignals) {
		return false
	}
	previousSignals := structuredFailureSignalsFromFailureOutput(previous.Output, nil, class, stringFromMap(previous.Output, failureOutputKeySignature))
	if !structuredFailureSignalsHaveConcreteTargetEvidence(previousSignals) {
		return false
	}
	return structuredFailureSignalSetsOverlap(currentSignals, previousSignals)
}

func structuredFailureSignalSetsOverlap(left map[string]any, right map[string]any) bool {
	leftTargets := structuredFailureSignalTargetSet(left)
	rightTargets := structuredFailureSignalTargetSet(right)
	for target := range leftTargets {
		if rightTargets[target] {
			return true
		}
	}
	leftMessages := stringSetFromSlice(stringSliceFromValue(left["messages"]))
	rightMessages := stringSetFromSlice(stringSliceFromValue(right["messages"]))
	for message := range leftMessages {
		if rightMessages[message] {
			return true
		}
	}
	leftIdentifiers := stringSetFromSlice(stringSliceFromValue(left["identifiers"]))
	rightIdentifiers := stringSetFromSlice(stringSliceFromValue(right["identifiers"]))
	for identifier := range leftIdentifiers {
		if rightIdentifiers[identifier] {
			return true
		}
	}
	return false
}

func structuredFailureSignalTargetSet(signals map[string]any) map[string]bool {
	ret := map[string]bool{}
	for _, ref := range stringSliceFromValue(signals["targetRefs"]) {
		if ref = strings.TrimSpace(ref); ref != "" {
			ret[ref] = true
		}
	}
	for _, lineRef := range mapSliceFromValue(signals["lineRefs"]) {
		if ref := strings.TrimSpace(stringFromMap(lineRef, "targetRef")); ref != "" {
			ret[ref] = true
		}
	}
	return ret
}

func stringSetFromSlice(values []string) map[string]bool {
	ret := map[string]bool{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			ret[value] = true
		}
	}
	return ret
}

func runtimeFailureCanContinueMechanicallyBeforeDuplicateEscalation(class string, targetScope string, attempts int, maxAttempts int) bool {
	if attempts <= 0 || maxAttempts <= 0 || attempts >= maxAttempts {
		return false
	}
	if strings.TrimSpace(targetScope) == "" {
		return false
	}
	switch strings.TrimSpace(class) {
	case failureClassPatchApply:
		return true
	default:
		return false
	}
}

func duplicateRepairDecisionFailureClass(class string) bool {
	switch strings.TrimSpace(class) {
	case failureClassWorkspacePath,
		failureClassShellCommand,
		failureClassPatchApply,
		failureClassLLMDecisionInvalid,
		failureClassMCPCapabilityDenied,
		failureClassMCPRuntime:
		return true
	default:
		return false
	}
}

func runtimeFailureEligibleForDuplicateEscalation(step *runModel.Step, class string) bool {
	if duplicateRepairDecisionFailureClass(class) {
		return true
	}
	if strings.TrimSpace(class) != failureClassRuntime {
		return false
	}
	return runtimeFailureIsLLMContractLoop(step)
}

func runtimeFailureNeedsNoNewEvidenceCheck(class string, step *runModel.Step) bool {
	return strings.TrimSpace(class) == failureClassRuntime && runtimeFailureIsLLMContractLoop(step)
}

func runtimeFailureIsLLMContractLoop(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	if strings.TrimSpace(step.Kind) != llmWorkspaceExecutionStepKind {
		return false
	}
	stage := strings.ToLower(strings.TrimSpace(stringFromMap(step.Output, "stage")))
	if stage != "llm_execution_graph_plan" && stage != "llm_schema_invalid" && stage != "llm_decision_invalid" {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		stringFromMap(step.Output, "error"),
		stringFromMap(step.Output, "reason"),
	}, " ")))
	for _, marker := range []string{
		"executiongraph patch/apply input invalid",
		"patch diff contains redacted runtime markers",
		"patch content contains redacted runtime markers",
		"patch describes a non-substantive",
		"is non-substantive",
		"patch text_replace oldtext/newtext must describe",
		"invalid llm decision json",
		"strategyreassessment.required=true",
		"executiongraph patch/apply must not depend on same-graph file/read",
		"file_write cannot replace existing target",
		"validation follow-up llm step returned read-only executiongraph without concrete validation evidence",
		"concrete validation evidence repair must return bounded",
		"concrete validation evidence graph must use non-destructive bounded validation",
		"concrete validation evidence graph must not start detached or background processes",
		"state-changing repair contract",
		"scoped state-changing runtime repair requires bounded verification",
		"primary diagnostic",
		"state-changing llm repair step returned",
		"execution-stage llm graph node returned executiongraph without concrete output",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func runtimeFailureRunHasCompletedActionEvidence(steps []*runModel.Step, failedStepID common.StepID) bool {
	for _, step := range steps {
		if step == nil || step.State != common.StepStateCompleted {
			continue
		}
		if step.ID == failedStepID {
			continue
		}
		switch strings.TrimSpace(step.Kind) {
		case "patch/apply", "shell/command", "mcp/tool_call":
			return true
		}
		if len(mapSliceFromValue(step.Output[stepOutputKeyPatches])) > 0 {
			return true
		}
	}
	return false
}

func (o *RuntimeOrchestrator) blockOnDuplicateRuntimeFailure(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	execErr error,
) error {
	if run == nil || failedStep == nil {
		return common.ErrInvalidArgument
	}
	class := strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeyClass))
	signature := strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeySignature))
	language := runDialogLanguage(run)
	var reasonErr error
	if execErr != nil && !errors.Is(execErr, common.ErrInvalidArgument) {
		reasonErr = execErr
	}
	diagnostics := runtimeFailureDecisionDiagnostics(failedStep, execErr)
	recommendedNextActions := []string{
		"Use a different grounded recovery strategy instead of repeating the same repair.",
		"Read current target state and reconcile it with the repeated diagnostics before patching.",
		"Choose replan when the failed target is no longer the smallest correct repair scope.",
	}
	if actions := stringSliceFromValue(diagnostics["recommendedNextActions"]); len(actions) > 0 {
		recommendedNextActions = actions
	}
	decisionContext := map[string]any{
		decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
		decisionContextKeyResultPolicy:   decisionContextResultPolicyRejected,
		"allowedRecoveryActions":         []string{"replan"},
		"runtimeRepairLoopState":         "duplicate_failure_escalated",
		"failureClass":                   class,
		"failureSignature":               signature,
		"failureTargetScope":             runtimeFailureTargetScope(signature),
		"failedStepId":                   string(failedStep.ID),
		"failedStepKind":                 failedStep.Kind,
		"failedStepInput":                o.llmContextManager().compactStepInput(failedStep.Input),
		"failedStepOutput":               o.llmContextManager().compactStepOutput(failedStep.Output),
		"recommendedNextActions":         recommendedNextActions,
	}
	for key, value := range diagnostics {
		if key == "recommendedNextActions" {
			continue
		}
		decisionContext[key] = value
	}
	spec := runtimeDecisionBlockSpec{
		TicketID:     common.TicketID(runtimeChildID(failedStep.ID, "duplicate-failure-decision")),
		Reason:       localizedDuplicateRuntimeFailureReason(language, signature, reasonErr),
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     localizedDuplicateRuntimeFailureQuestion(language),
		Context:      decisionContext,
	}
	if err := o.createDecisionTicket(ctx, run, failedStep, spec); err != nil {
		return err
	}
	if err := o.resultManager().recordRejectedResultForDecisionBlock(ctx, run, failedStep, failedStep.Output, spec); err != nil {
		return err
	}
	return o.blockOnDecisionAtStep(ctx, run, nil, spec.TicketID, failedStep.ID)
}

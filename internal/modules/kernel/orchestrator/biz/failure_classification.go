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
)

const (
	failureClassProviderUnavailable = "provider_unavailable"
	failureClassProviderSchemaDrift = "provider_schema_drift"
	failureClassProviderTimeout     = "provider_timeout"
	failureClassWorkspacePath       = "workspace_path_conflict"
	failureClassShellCommand        = "shell_command_failed"
	failureClassPatchApply          = "patch_apply_failed"
	failureClassMCPCapabilityDenied = "mcp_capability_denied"
	failureClassMCPRuntime          = "mcp_runtime_failed"
	failureClassRuntime             = "runtime_failed"

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
	if strings.Contains(lowerStage, "timeout") {
		return true
	}
	errText := strings.ToLower(firstNonEmptyString(errorString(execErr), stringFromMap(output, "error"), stringFromMap(output, "reason")))
	return strings.Contains(errText, "deadline exceeded") || strings.Contains(errText, "timeout")
}

func isLLMProviderSchemaFailure(lowerStage string) bool {
	switch lowerStage {
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
		"llm_provider_error",
		"llm_provider_stream_error",
		"llm_provider_request_failed":
		return true
	default:
		return false
	}
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
	if exitCode, ok := normalizedSignatureNumber(output["exitCode"]); ok {
		parts = append(parts, "exitCode="+exitCode)
	}
	errText := firstNonEmptyString(stringFromMap(output, "error"), stringFromMap(output, "reason"), errorString(execErr), stringFromMap(output, "stderr"))
	if value := normalizedSignatureText(errText); value != "" {
		parts = append(parts, "error="+value)
	}
	return strings.Join(parts, "|")
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

func (o *RuntimeOrchestrator) shouldBlockOnDuplicateRuntimeFailure(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) bool {
	if o == nil || run == nil || failedStep == nil || o.steps == nil || o.tickets == nil {
		return false
	}
	signature := strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeySignature))
	class := strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeyClass))
	if signature == "" || !duplicateRepairDecisionFailureClass(class) || o.runtimeRepairAttemptCount(ctx, run.ID) == 0 {
		return false
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return false
	}
	for _, step := range steps {
		if step == nil || step.ID == failedStep.ID || step.State != common.StepStateFailed {
			continue
		}
		if strings.TrimSpace(stringFromMap(step.Output, failureOutputKeySignature)) == signature {
			return true
		}
	}
	return false
}

func duplicateRepairDecisionFailureClass(class string) bool {
	switch strings.TrimSpace(class) {
	case failureClassWorkspacePath,
		failureClassShellCommand,
		failureClassPatchApply,
		failureClassMCPCapabilityDenied,
		failureClassMCPRuntime:
		return true
	default:
		return false
	}
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
	spec := runtimeDecisionBlockSpec{
		TicketID:     common.TicketID(runtimeChildID(failedStep.ID, "duplicate-failure-decision")),
		Reason:       localizedDuplicateRuntimeFailureReason(language, signature, reasonErr),
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     localizedDuplicateRuntimeFailureQuestion(language),
		Context: map[string]any{
			"failureClass":     class,
			"failureSignature": signature,
			"failedStepId":     string(failedStep.ID),
			"failedStepKind":   failedStep.Kind,
			"failedStepInput":  cloneStepPayload(failedStep.Input),
			"failedStepOutput": cloneStepPayload(failedStep.Output),
		},
	}
	if err := o.createDecisionTicket(ctx, run, failedStep, spec); err != nil {
		return err
	}
	return o.BlockOnDecision(ctx, run.ID, spec.TicketID)
}

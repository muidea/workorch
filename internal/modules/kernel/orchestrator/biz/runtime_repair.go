package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

const maxPatchRepairSnapshotBytes = 120 * 1024

const (
	repairObjectiveStateChangingOutput        = "produce_state_changing_output"
	repairObjectiveConcreteValidationEvidence = "produce_concrete_validation_evidence"
	repairObjectiveTransientExecutionRetry    = "retry_or_adjust_transient_execution"
	repairObjectiveAcquireFailureEvidence     = "acquire_concrete_failure_evidence"
)

func runtimeFailureOutput(output map[string]any, execErr error) map[string]any {
	ret := cloneStepPayload(output)
	if ret == nil {
		ret = map[string]any{}
	}
	if execErr != nil {
		ret["reason"] = execErr.Error()
		ret["error"] = execErr.Error()
		var diagnosticErr runtimeFailureDiagnosticError
		if errors.As(execErr, &diagnosticErr) {
			if diagnostics := diagnosticErr.RuntimeFailureDiagnostics(); len(diagnostics) > 0 {
				ret["runtimeDiagnostics"] = diagnostics
			}
		}
	}
	return ret
}

func (o *RuntimeOrchestrator) handleRuntimeStepFailure(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	output map[string]any,
	summary string,
	execErr error,
) error {
	if run == nil || step == nil {
		return common.ErrInvalidArgument
	}
	step.State = common.StepStateFailed
	step.Output = runtimeFailureOutput(outputWithDoDAssessmentDiagnostics(output), execErr)
	classification := classifyRuntimeFailure(step, step.Output, summary, execErr)
	attachRuntimeFailureClassification(step.Output, classification)
	attachRuntimeFailureDiagnostics(step, classification, execErr)
	_ = o.recordToolCallFailed(ctx, toolCall, execErr, step.Output)
	errText := ""
	if execErr != nil {
		errText = execErr.Error()
	}
	evidenceRef := ""
	if toolCall != nil {
		evidenceRef = string(toolCall.ID)
	}
	_ = o.recordValidationResult(ctx, run, step, common.ValidationResultStateFailed, summary, evidenceRef, errText)
	_ = o.recordStepCheckpoint(ctx, run, step, "step_failed")
	_ = o.steps.Update(ctx, step)
	_ = o.publishStepState(ctx, run, step, events.TopicStepStateChanged)
	if graphBatchStep(step) {
		if execErr != nil {
			return execErr
		}
		return common.ErrInvalidArgument
	}
	if shouldCloseCompletionGateFailureWithoutRepair(summary, step, step.Output) {
		reason := "runtime step failed"
		if execErr != nil {
			reason = execErr.Error()
		} else if step.Output != nil {
			reason = stringFromMap(step.Output, "error")
			if reason == "" {
				reason = stringFromMap(step.Output, "reason")
			}
		}
		return o.failRunWithRejectedCompletionResult(ctx, run, step, reason)
	}
	return o.dispatchRuntimeRepairFollowUp(ctx, run, step, execErr)
}

func attachRuntimeFailureDiagnostics(step *runModel.Step, classification runtimeFailureClassification, execErr error) {
	if step == nil || len(step.Output) == 0 {
		return
	}
	diagnostics := map[string]any{
		"stepId":           string(step.ID),
		"stepKind":         step.Kind,
		"failureClass":     firstNonEmptyString(classification.Class, stringFromMap(step.Output, failureOutputKeyClass)),
		"failureSignature": firstNonEmptyString(classification.Signature, stringFromMap(step.Output, failureOutputKeySignature)),
		"stage":            firstNonEmptyString(classification.Stage, stringFromMap(step.Output, "stage")),
		"error": firstNonEmptyString(
			stringFromMap(step.Output, "error"),
			stringFromMap(step.Output, "reason"),
			errorString(execErr),
		),
	}
	if command := strings.TrimSpace(stringFromMap(step.Output, "command")); command != "" {
		diagnostics["command"] = command
	}
	if args := stringSliceFromValue(step.Output["args"]); len(args) > 0 {
		diagnostics["args"] = args
	}
	if exitCode, ok := step.Output["exitCode"]; ok {
		diagnostics["exitCode"] = exitCode
	}
	if targets := failureTargetsForSignature(step.Output); len(targets) > 0 {
		diagnostics["failureTargets"] = targets
	}
	if stderr := strings.TrimSpace(stringFromMap(step.Output, "stderr")); stderr != "" {
		diagnostics["stderrPreview"] = truncateLLMContextString(stderr)
	}
	if stdout := strings.TrimSpace(stringFromMap(step.Output, "stdout")); stdout != "" {
		diagnostics["stdoutPreview"] = truncateLLMContextString(stdout)
	}
	if runtimeDiagnostics := mapFromValue(step.Output["runtimeDiagnostics"]); len(runtimeDiagnostics) > 0 {
		diagnostics["runtimeDiagnostics"] = compactLLMContextMap(runtimeDiagnostics, 1)
	}
	step.Output["lastFailureDiagnostics"] = diagnostics
}

func shouldCloseCompletionGateFailureWithoutRepair(summary string, step *runModel.Step, output map[string]any) bool {
	if strings.TrimSpace(summary) != "runtime completion gate failed" {
		return false
	}
	if step != nil && boolValue(step.Input[stepInputKeyNoActionableFollowUp]) {
		return false
	}
	assessment := doDAssessmentFromOutput(output)
	if assessment == nil {
		return true
	}
	if step != nil && boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) {
		if convertedDecisionGateFailure(output) {
			return false
		}
		if doDEvidenceQualityRepairRequired(step) {
			return false
		}
		if doDCoverageContractCorrectionRequired(step) {
			return false
		}
		return true
	}
	if strings.TrimSpace(stringFromMap(assessment, "summary")) == "" {
		return true
	}
	return false
}

func convertedDecisionGateFailure(output map[string]any) bool {
	if len(output) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromMap(output, "convertedDecisionGate")) != "" {
		return true
	}
	for _, key := range []string{
		"assessmentOnlyDecisionConvertedToRepair",
		"continuableDecisionConvertedToRepair",
		"actionableFailedAssessmentDecisionConvertedToRepair",
	} {
		if boolValue(output[key]) {
			return true
		}
	}
	return false
}

func doDEvidenceQualityRepairRequired(step *runModel.Step) bool {
	if step == nil || step.Output == nil {
		return false
	}
	diagnostics := mapFromValue(step.Output["runtimeDiagnostics"])
	return strings.TrimSpace(stringFromMap(diagnostics, "kind")) == "dod_evidence_quality_gap"
}

func (o *RuntimeOrchestrator) dispatchRuntimeRepairFollowUp(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	execErr error,
) error {
	if summary, ok := o.rejectedRunResultSummary(ctx, run.ID); ok {
		reason := strings.TrimSpace(summary)
		if reason == "" {
			reason = "run has rejected result"
		}
		return o.FailRun(ctx, run.ID, reason)
	}
	switch o.recoveryActionForFailedStep(ctx, run, failedStep) {
	case runtimeRecoveryActionProviderRetry:
		rootStepID := providerRetryRootStepID(failedStep)
		attempt := o.providerRetryAttemptCount(ctx, run.ID, rootStepID) + 1
		input := o.providerRetryStepInput(failedStep, attempt, execErr)
		stepID, err := o.uniqueDispatchStepID(ctx, run.ID, providerRetryStepID(run.ID, rootStepID, attempt))
		if err != nil {
			return err
		}
		return o.dispatchStep(ctx, run, stepID, failedStep.Kind, input)
	case runtimeRecoveryActionProviderRetryExhausted:
		return o.blockOnProviderRetryExhaustion(ctx, run, failedStep, execErr)
	case runtimeRecoveryActionRuntimeRepairExhausted:
		return o.blockOnRuntimeRepairExhaustion(ctx, run, failedStep, execErr)
	case runtimeRecoveryActionDuplicateFailure:
		return o.blockOnDuplicateRuntimeFailure(ctx, run, failedStep, execErr)
	case runtimeRecoveryActionContextIntegrityBlocked:
		return o.blockOnContextIntegrityGate(ctx, run, failedStep, execErr)
	case runtimeRecoveryActionRuntimeRepair:
		attempt := o.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep) + 1
		input := o.runtimeRepairFollowUpStepInput(ctx, run, failedStep, attempt, execErr)
		stepID, err := o.uniqueDispatchStepID(ctx, run.ID, runtimeRepairFollowUpStepID(run.ID, failedStep, attempt))
		if err != nil {
			return err
		}
		return o.dispatchStep(ctx, run, stepID, initialStepKind(run), input)
	case runtimeRecoveryActionFailRun:
		reason := "runtime step failed"
		if execErr != nil {
			reason = execErr.Error()
		} else if failedStep != nil && failedStep.Output != nil {
			reason = stringFromMap(failedStep.Output, "error")
			if reason == "" {
				reason = stringFromMap(failedStep.Output, "reason")
			}
		}
		return o.failRunWithRejectedCompletionResult(ctx, run, failedStep, reason)
	default:
		return common.ErrInvalidArgument
	}
}

type runtimeRecoveryAction string

const (
	runtimeRecoveryActionProviderRetry           runtimeRecoveryAction = "provider_retry"
	runtimeRecoveryActionProviderRetryExhausted  runtimeRecoveryAction = "provider_retry_exhausted"
	runtimeRecoveryActionRuntimeRepairExhausted  runtimeRecoveryAction = "runtime_repair_exhausted"
	runtimeRecoveryActionDuplicateFailure        runtimeRecoveryAction = "duplicate_failure"
	runtimeRecoveryActionContextIntegrityBlocked runtimeRecoveryAction = "context_integrity_blocked"
	runtimeRecoveryActionRuntimeRepair           runtimeRecoveryAction = "runtime_repair"
	runtimeRecoveryActionFailRun                 runtimeRecoveryAction = "fail_run"
)

func (o *RuntimeOrchestrator) recoveryActionForFailedStep(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) runtimeRecoveryAction {
	if o == nil || run == nil || failedStep == nil {
		return runtimeRecoveryActionFailRun
	}
	if o.shouldBlockOnProviderConfigFailure(ctx, run, failedStep) {
		return runtimeRecoveryActionProviderRetryExhausted
	}
	if o.shouldAttemptProviderRetry(ctx, run, failedStep) {
		return runtimeRecoveryActionProviderRetry
	}
	if o.providerRetryBudgetExhausted(ctx, run, failedStep) {
		return runtimeRecoveryActionProviderRetryExhausted
	}
	if isLLMProviderRuntimeFailure(run, failedStep) {
		return runtimeRecoveryActionFailRun
	}
	if runtimeContextIntegrityGateFailure(failedStep) && !runtimeContextIntegrityGateCanRecoverWithCoverageTargets(failedStep) {
		return runtimeRecoveryActionContextIntegrityBlocked
	}
	if o.shouldEscalateRuntimeRepairExhaustion(ctx, run, failedStep) {
		return runtimeRecoveryActionRuntimeRepairExhausted
	}
	if o.shouldBlockOnDuplicateRuntimeFailure(ctx, run, failedStep) {
		return runtimeRecoveryActionDuplicateFailure
	}
	if o.shouldAttemptRuntimeRepair(ctx, run, failedStep) {
		return runtimeRecoveryActionRuntimeRepair
	}
	return runtimeRecoveryActionFailRun
}

func (o *RuntimeOrchestrator) shouldBlockOnProviderConfigFailure(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) bool {
	if o == nil || run == nil || failedStep == nil || o.tickets == nil {
		return false
	}
	if !isLLMProviderRuntimeFailure(run, failedStep) {
		return false
	}
	switch stringFromMap(failedStep.Output, failureOutputKeyClass) {
	case failureClassProviderSchemaDrift, failureClassAuthOrConfigError:
		return true
	case failureClassProviderUnavailable:
		return providerUnavailableFailureRequiresConfigBlock(failedStep)
	default:
		return false
	}
}

func providerUnavailableFailureRequiresConfigBlock(failedStep *runModel.Step) bool {
	if failedStep == nil {
		return false
	}
	stage := strings.TrimSpace(stringFromMap(failedStep.Output, "stage"))
	switch stage {
	case "llm_provider_stream_error":
		return false
	case "llm_provider_client_unavailable",
		"llm_provider_operation_unsupported",
		"llm_provider_protocol_invalid":
		return true
	case "llm_provider_error",
		"llm_provider_unavailable",
		"llm_provider_request_failed":
		return providerUnavailableFailureTextRequiresConfigBlock(failedStep)
	default:
		return false
	}
}

func providerUnavailableFailureTextRequiresConfigBlock(failedStep *runModel.Step) bool {
	if failedStep == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		stringFromMap(failedStep.Output, "error"),
		stringFromMap(failedStep.Output, "reason"),
	}, " ")))
	if text == "" {
		return false
	}
	configMarkers := []string{
		"auth_unavailable",
		"no auth available",
		"authentication",
		"unauthorized",
		"forbidden",
		"invalid api key",
		"api key",
		"missing api",
		"missing provider",
		"no provider",
		"not configured",
		"provider not configured",
		"model not found",
		"unknown model",
		"unsupported model",
		"operation unsupported",
		"unsupported operation",
		"http status 400",
		"http status 401",
		"http status 403",
		"http status 404",
	}
	for _, marker := range configMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func (o *RuntimeOrchestrator) blockRecoveryActionFromFailedStep(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	execErr error,
	action runtimeRecoveryAction,
) error {
	if o == nil || o.runs == nil || o.tickets == nil {
		return common.ErrBlocked
	}
	switch action {
	case runtimeRecoveryActionProviderRetryExhausted:
		if err := o.blockOnProviderRetryExhaustion(ctx, run, failedStep, execErr); err != nil {
			return err
		}
		return common.ErrBlocked
	case runtimeRecoveryActionRuntimeRepairExhausted:
		if err := o.blockOnRuntimeRepairExhaustion(ctx, run, failedStep, execErr); err != nil {
			return err
		}
		return common.ErrBlocked
	case runtimeRecoveryActionDuplicateFailure:
		if err := o.blockOnDuplicateRuntimeFailure(ctx, run, failedStep, execErr); err != nil {
			return err
		}
		return common.ErrBlocked
	case runtimeRecoveryActionContextIntegrityBlocked:
		if err := o.blockOnContextIntegrityGate(ctx, run, failedStep, execErr); err != nil {
			return err
		}
		return common.ErrBlocked
	default:
		return common.ErrInvalidArgument
	}
}

func (o *RuntimeOrchestrator) failRunWithRejectedCompletionResult(ctx context.Context, run *runModel.Run, step *runModel.Step, reason string) error {
	if run == nil {
		return common.ErrInvalidArgument
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "runtime step failed"
	}
	if requiresDoDAssessmentCompletionGate(run) && step != nil {
		_ = o.recordRejectedCompletionDoDAssessment(ctx, run, step, step.Output)
		reason = rejectedCompletionFailureReason(step, reason)
	}
	return o.FailRun(ctx, run.ID, reason)
}

func rejectedCompletionFailureReason(step *runModel.Step, fallback string) string {
	fallback = strings.TrimSpace(fallback)
	if step == nil {
		return fallback
	}
	assessment := doDAssessmentFromOutput(step.Output)
	if assessment == nil {
		return fallback
	}
	if satisfied, _ := assessment["satisfied"].(bool); satisfied {
		return fallback
	}
	summary := strings.TrimSpace(stringFromMap(assessment, "summary"))
	if summary == "" {
		return fallback
	}
	return summary
}

func (o *RuntimeOrchestrator) shouldEscalateRuntimeRepairExhaustion(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) bool {
	if o == nil || run == nil || failedStep == nil || o.tickets == nil {
		return false
	}
	if !requiresDoDAssessmentCompletionGate(run) || isLLMProviderRuntimeFailure(run, failedStep) {
		return false
	}
	return o.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep) >= o.maxRuntimeRepairAttempts()
}

func (o *RuntimeOrchestrator) blockOnRuntimeRepairExhaustion(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	execErr error,
) error {
	if run == nil || failedStep == nil {
		return common.ErrInvalidArgument
	}
	language := runDialogLanguage(run)
	maxAttempts := o.maxRuntimeRepairAttempts()
	reason := localizedRuntimeRepairExhaustionReason(language, maxAttempts, execErr)
	spec := runtimeDecisionBlockSpec{
		TicketID:     common.TicketID(runtimeChildID(failedStep.ID, "repair-exhausted-decision")),
		Reason:       reason,
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     localizedRuntimeRepairExhaustionQuestion(language),
		Context: map[string]any{
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
			decisionContextKeyResultPolicy:   decisionContextResultPolicyRejected,
			"allowedRecoveryActions":         []string{"retry_same_step", "replan", "provide_information", "terminate"},
			"attempts":                       o.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep),
			"maxAttempts":                    maxAttempts,
			"failureClass":                   stringFromMap(failedStep.Output, failureOutputKeyClass),
			"failureSignature":               stringFromMap(failedStep.Output, failureOutputKeySignature),
			"failureTargetScope":             runtimeFailureTargetScopeFromStep(failedStep),
			"failedStepId":                   string(failedStep.ID),
			"repairOfRootStepId":             string(runtimeRepairRootStepID(failedStep)),
			"failedStepKind":                 failedStep.Kind,
			"failedStepInput":                compactStepInputForLLMContext(failedStep.Input),
			"failedStepOutput":               compactStepOutputForLLMContext(failedStep.Output),
			"missingDoDItems":                missingDoDItemsFromFailureOutput(failedStep.Output),
			"recentRepairAttempts":           o.recentRuntimeRepairAttemptSummaries(ctx, run.ID, failedStep, maxAttempts),
			"recommendedNextActions": []string{
				"Map each missing DoD item to concrete affected resources, state-changing operations, and verification actions.",
				"Prioritize minimal target changes, then rerun targeted verification.",
				"If key constraints remain unclear, provide a focused decision payload with only missing information.",
			},
		},
	}
	if err := o.recordRejectedResultForDecisionBlock(ctx, run, failedStep, failedStep.Output, spec); err != nil {
		return err
	}
	if err := o.createDecisionTicket(ctx, run, failedStep, spec); err != nil {
		return err
	}
	return o.BlockOnDecision(ctx, run.ID, spec.TicketID)
}

func (o *RuntimeOrchestrator) shouldAttemptRuntimeRepair(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) bool {
	if o == nil || run == nil || failedStep == nil || o.runtime == nil || o.steps == nil {
		return false
	}
	if !requiresDoDAssessmentCompletionGate(run) {
		return false
	}
	if terminalAssessmentOnlyDecisionContractFailure(failedStep) {
		return false
	}
	if runtimeContextIntegrityGateFailure(failedStep) && !runtimeContextIntegrityGateCanRecoverWithCoverageTargets(failedStep) {
		return false
	}
	if o.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep) >= o.maxRuntimeRepairAttempts() {
		return false
	}
	return !isLLMProviderRuntimeFailure(run, failedStep)
}

func (o *RuntimeOrchestrator) shouldAttemptProviderRetry(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) bool {
	if o == nil || run == nil || failedStep == nil || o.runtime == nil || o.steps == nil {
		return false
	}
	attempts := o.providerRetryAttemptCount(ctx, run.ID, providerRetryRootStepID(failedStep))
	if attempts >= o.maxProviderRetryAttempts() {
		return false
	}
	if isRetryableLLMProviderRuntimeFailure(run, failedStep) {
		return true
	}
	return false
}

func (o *RuntimeOrchestrator) shouldBlockOnProviderRetryExhaustion(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) bool {
	if o == nil || run == nil || failedStep == nil || o.tickets == nil {
		return false
	}
	return o.providerRetryBudgetExhausted(ctx, run, failedStep)
}

func (o *RuntimeOrchestrator) providerRetryBudgetExhausted(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) bool {
	if o == nil || run == nil || failedStep == nil {
		return false
	}
	if !isRetryableLLMProviderRuntimeFailure(run, failedStep) {
		return false
	}
	return o.providerRetryAttemptCount(ctx, run.ID, providerRetryRootStepID(failedStep)) >= o.maxProviderRetryAttempts()
}

func (o *RuntimeOrchestrator) blockOnProviderRetryExhaustion(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	execErr error,
) error {
	if run == nil || failedStep == nil {
		return common.ErrInvalidArgument
	}
	language := runDialogLanguage(run)
	maxAttempts := o.maxProviderRetryAttempts()
	rootStepID := providerRetryRootStepID(failedStep)
	attempts := o.providerRetryAttemptCount(ctx, run.ID, rootStepID)
	if attempts > maxAttempts {
		attempts = maxAttempts
	}
	spec := runtimeDecisionBlockSpec{
		TicketID:     common.TicketID(runtimeChildID(failedStep.ID, "provider-retry-exhausted-decision")),
		Reason:       localizedProviderRetryExhaustionReason(language, attempts, execErr),
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     localizedProviderRetryExhaustionQuestion(language),
		Context: map[string]any{
			decisionContextKeyScope:          decisionContextScopeWorkorchRuntime,
			decisionContextKeyDependency:     decisionContextDependencyLLMProvider,
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyConfigOnly,
			decisionContextKeyResultPolicy:   decisionContextResultPolicyNone,
			"attempts":                       attempts,
			"maxAttempts":                    maxAttempts,
			"failureClass":                   stringFromMap(failedStep.Output, failureOutputKeyClass),
			"failureSignature":               stringFromMap(failedStep.Output, failureOutputKeySignature),
			"failedStepId":                   string(failedStep.ID),
			"failedStepKind":                 failedStep.Kind,
			"failedStepInput":                compactStepInputForLLMContext(failedStep.Input),
			"failedStepOutput":               compactStepOutputForLLMContext(failedStep.Output),
		},
	}
	ticketID, err := o.createOrReuseRuntimeBlockerTicket(ctx, run, failedStep, spec)
	if err != nil {
		return err
	}
	return o.BlockOnDecision(ctx, run.ID, ticketID)
}

func (o *RuntimeOrchestrator) blockOnContextIntegrityGate(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	execErr error,
) error {
	if run == nil || failedStep == nil {
		return common.ErrInvalidArgument
	}
	language := runDialogLanguage(run)
	budget := mapFromValue(failedStep.Output["contextBudget"])
	gateAction := strings.TrimSpace(stringFromMap(budget, "contextGateAction"))
	gateReason := strings.TrimSpace(stringFromMap(budget, "contextGateReason"))
	if gateReason == "" {
		gateReason = firstNonEmptyString(
			stringFromMap(failedStep.Output, "reason"),
			stringFromMap(failedStep.Output, "error"),
			errorString(execErr),
		)
	}
	spec := runtimeDecisionBlockSpec{
		TicketID:     common.TicketID(runtimeChildID(failedStep.ID, "context-integrity-decision")),
		Reason:       localizedContextIntegrityGateReason(language, gateAction, gateReason, execErr),
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     localizedContextIntegrityGateQuestion(language),
		Context: map[string]any{
			decisionContextKeyScope:          decisionContextScopeWorkorchRuntime,
			decisionContextKeyDependency:     decisionContextDependencyContextGate,
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyContext,
			decisionContextKeyResultPolicy:   decisionContextResultPolicyNone,
			"allowedRecoveryActions":         []string{"provide_information", "replan", "terminate"},
			"contextGateAction":              gateAction,
			"contextGateReason":              gateReason,
			"failureClass":                   stringFromMap(failedStep.Output, failureOutputKeyClass),
			"failureSignature":               stringFromMap(failedStep.Output, failureOutputKeySignature),
			"failedStepId":                   string(failedStep.ID),
			"failedStepKind":                 failedStep.Kind,
			"contextBudget":                  compactLLMContextValue(budget, 1),
			"failedStepInput":                compactStepInputForLLMContext(failedStep.Input),
			"failedStepOutput":               compactStepOutputForLLMContext(failedStep.Output),
		},
	}
	ticketID, err := o.createOrReuseRuntimeBlockerTicket(ctx, run, failedStep, spec)
	if err != nil {
		return err
	}
	return o.BlockOnDecision(ctx, run.ID, ticketID)
}

func (o *RuntimeOrchestrator) createOrReuseRuntimeBlockerTicket(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	spec runtimeDecisionBlockSpec,
) (common.TicketID, error) {
	if o == nil || o.tickets == nil || run == nil || failedStep == nil || spec.TicketID == "" {
		return "", common.ErrInvalidArgument
	}
	baseID := spec.TicketID
	if err := o.createDecisionTicket(ctx, run, failedStep, spec); err == nil {
		return spec.TicketID, nil
	} else if !errors.Is(err, common.ErrConflict) {
		return "", err
	}

	existing, getErr := o.tickets.Get(ctx, baseID)
	if getErr != nil && !errors.Is(getErr, common.ErrNotFound) {
		return "", getErr
	}
	if existing != nil &&
		existing.RunID == run.ID &&
		existing.StepID == failedStep.ID &&
		existing.State != common.DecisionTicketStateResolved {
		return existing.ID, nil
	}

	for seq := 2; seq <= 100; seq++ {
		nextSpec := spec
		nextSpec.TicketID = common.TicketID(fmt.Sprintf("%s-%d", baseID, seq))
		nextSpec.Context = cloneStepPayload(spec.Context)
		if nextSpec.Context == nil {
			nextSpec.Context = map[string]any{}
		}
		nextSpec.Context["previousTicketId"] = string(baseID)
		nextSpec.Context["repeatBlocker"] = true
		if err := o.createDecisionTicket(ctx, run, failedStep, nextSpec); err == nil {
			return nextSpec.TicketID, nil
		} else if !errors.Is(err, common.ErrConflict) {
			return "", err
		}
		existing, getErr := o.tickets.Get(ctx, nextSpec.TicketID)
		if getErr != nil && !errors.Is(getErr, common.ErrNotFound) {
			return "", getErr
		}
		if existing != nil &&
			existing.RunID == run.ID &&
			existing.StepID == failedStep.ID &&
			existing.State != common.DecisionTicketStateResolved {
			return existing.ID, nil
		}
	}
	return "", common.ErrConflict
}

func (o *RuntimeOrchestrator) providerRetryAttemptCount(ctx context.Context, runID common.RunID, rootStepID common.StepID) int {
	if o == nil || o.steps == nil || runID == "" || rootStepID == "" {
		return 0
	}
	steps, err := o.steps.ListByRun(ctx, runID)
	if err != nil {
		return 0
	}
	count := 0
	for _, step := range steps {
		if isFailedProviderRetryAttempt(rootStepID, step) {
			count++
		}
	}
	return count
}

func isFailedProviderRetryAttempt(rootStepID common.StepID, step *runModel.Step) bool {
	if step == nil || rootStepID == "" || !boolValue(step.Input[stepInputKeyProviderRetryFollowUp]) || providerRetryRootStepID(step) != rootStepID {
		return false
	}
	return step.State == common.StepStateFailed && isLLMProviderFailureStage(stringFromMap(step.Output, "stage"))
}

func providerRetryStepInput(failedStep *runModel.Step, attempt int, execErr error) map[string]any {
	return providerRetryStepInputWithMaxAttempts(failedStep, attempt, execErr, maxProviderRetryAttempts)
}

func (o *RuntimeOrchestrator) providerRetryStepInput(failedStep *runModel.Step, attempt int, execErr error) map[string]any {
	return providerRetryStepInputWithMaxAttempts(failedStep, attempt, execErr, o.maxProviderRetryAttempts())
}

func providerRetryStepInputWithMaxAttempts(failedStep *runModel.Step, attempt int, execErr error, maxAttempts int) map[string]any {
	if maxAttempts <= 0 {
		maxAttempts = maxProviderRetryAttempts
	}
	input := cloneStepPayload(failedStep.Input)
	if input == nil {
		input = map[string]any{}
	}
	sanitizeProviderRetryBaseInput(input)
	delete(input, stepInputKeyRuntimeRepairFollowUp)
	delete(input, stepInputKeyRuntimeRepairAttempt)
	rootStepID := providerRetryRootStepID(failedStep)
	input[stepInputKeyProviderRetryFollowUp] = true
	input[stepInputKeyProviderRetryAttempt] = attempt
	input[stepInputKeyProviderRetryRootStep] = string(rootStepID)
	setRuntimeStage(input, runtimeStageRepair)
	input[stepInputKeyRecoveryAction] = string(runtimeRecoveryActionProviderRetry)
	input[stepInputKeyRecoveryRootStepID] = string(rootStepID)
	input[stepInputKeyRecoveryLineageType] = "provider_retry"
	input[stepInputKeyRecoveryBudgetScope] = recoveryBudgetScope("provider_retry", rootStepID, stringFromMap(failedStep.Output, failureOutputKeyClass), stringFromMap(failedStep.Output, failureOutputKeySignature))
	input["maxProviderRetryAttempts"] = maxAttempts
	input["failedStepId"] = string(rootStepID)
	input["failedStepKind"] = failedStep.Kind
	input["failedStepState"] = string(failedStep.State)
	input["failedStepOutput"] = compactStepOutputForLLMContext(failedStep.Output)
	input["failureReason"] = errorString(execErr)
	input[failureOutputKeyClass] = stringFromMap(failedStep.Output, failureOutputKeyClass)
	input[failureOutputKeySignature] = stringFromMap(failedStep.Output, failureOutputKeySignature)
	if stateChangingRepairRequired(nil, failedStep, execErr) || repairObjectiveRequiresStateChange(stringFromMap(input, "repairObjective")) {
		input["repairObjective"] = repairObjectiveStateChangingOutput
		input["stateChangingOutputRequired"] = true
		if _, ok := input["repairGuidance"]; !ok {
			input["repairGuidance"] = stateChangingRepairGuidance()
		}
		narrowStateChangingRecoveryInput(input, failedStep, execErr, "provider_retry")
	}
	if completionGateFollowUpStep(failedStep) {
		input["providerRetryMode"] = "completion_assessment_only"
		input["providerRetryAssessmentOnly"] = true
		input["completionFollowUp"] = true
		setRuntimeStage(input, runtimeStageVerification)
		delete(input, stepOutputKeyExecutionGraph)
		delete(input, "previousStepOutput")
	}
	return input
}

func sanitizeProviderRetryBaseInput(input map[string]any) {
	if input == nil {
		return
	}
	for _, key := range []string{
		"failedStepInput",
		"failedStepOutput",
		"llmProviderId",
		"previousStepOutput",
		stepInputKeyExecutionContextPackage,
		stepInputKeyWorkspaceFileManifest,
		stepOutputKeyExecutionGraph,
	} {
		delete(input, key)
	}
	if executionContext := slimExecutionContextForStepInput(mapFromValue(input["executionContext"])); len(executionContext) > 0 {
		input["executionContext"] = executionContext
	} else {
		delete(input, "executionContext")
	}
}

func completionGateFollowUpStep(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	return boolValue(step.Input["graphFollowUp"]) ||
		boolValue(step.Input["completionFollowUp"]) ||
		boolValue(step.Input[stepInputKeyNoActionableFollowUp])
}

func providerRetryStepID(runID common.RunID, rootStepID common.StepID, attempt int) common.StepID {
	if attempt < 1 {
		attempt = 1
	}
	rootLabel := runtimeReadableStepLabel(runID, rootStepID)
	if rootLabel == "" {
		rootLabel = "root"
	}
	return common.StepID(fmt.Sprintf("%s/provider-retry-%03d-of-%s", runID, attempt, rootLabel))
}

func providerRetryRootStepID(step *runModel.Step) common.StepID {
	if step == nil {
		return ""
	}
	if root := common.StepID(strings.TrimSpace(stringFromMap(step.Input, stepInputKeyProviderRetryRootStep))); root != "" {
		return normalizeRecoveryRootStepIDFromInput(step.Input, root)
	}
	if boolValue(step.Input[stepInputKeyProviderRetryFollowUp]) {
		if root := common.StepID(strings.TrimSpace(stringFromMap(step.Input, "failedStepId"))); root != "" {
			return normalizeRecoveryRootStepIDFromInput(step.Input, root)
		}
	}
	return normalizeRecoveryRootStepIDFromInput(step.Input, step.ID)
}

func normalizeRecoveryRootStepIDFromInput(input map[string]any, root common.StepID) common.StepID {
	if root == "" {
		return ""
	}
	rootText := string(root)
	if !recoveryRootNeedsNormalization(rootText) {
		return root
	}
	if repairRoot := common.StepID(strings.TrimSpace(stringFromMap(input, stepInputKeyRepairOfRootStepID))); repairRoot != "" && !recoveryRootNeedsNormalization(string(repairRoot)) {
		return repairRoot
	}
	if recoveryRoot := common.StepID(strings.TrimSpace(stringFromMap(input, stepInputKeyRecoveryRootStepID))); recoveryRoot != "" && recoveryRoot != root {
		return normalizeRecoveryRootStepIDFromInput(input, recoveryRoot)
	}
	if providerRoot := common.StepID(strings.TrimSpace(stringFromMap(input, stepInputKeyProviderRetryRootStep))); providerRoot != "" && providerRoot != root {
		return normalizeRecoveryRootStepIDFromInput(input, providerRoot)
	}
	if strings.Contains(rootText, "/provider-retry-") || strings.Contains(rootText, "of-provider-retry-") {
		if repairRoot := common.StepID(strings.TrimSpace(stringFromMap(input, stepInputKeyRepairOfRootStepID))); repairRoot != "" {
			return repairRoot
		}
	}
	return root
}

func recoveryRootNeedsNormalization(root string) bool {
	return strings.Contains(root, "/provider-retry-") ||
		strings.Contains(root, "of-provider-retry-")
}

func isRetryableLLMProviderRuntimeFailure(run *runModel.Run, step *runModel.Step) bool {
	if !isLLMProviderRuntimeFailure(run, step) {
		return false
	}
	if stringFromMap(step.Output, failureOutputKeyClass) == failureClassProviderUnavailable &&
		!providerUnavailableFailureRequiresConfigBlock(step) {
		return true
	}
	switch stringFromMap(step.Output, failureOutputKeyClass) {
	case failureClassProviderTimeout, failureClassContextTooLarge, failureClassRateLimited, failureClassProviderStreamIncomplete:
		return true
	default:
		return false
	}
}

func isLLMProviderRuntimeFailure(run *runModel.Run, step *runModel.Step) bool {
	if run == nil || step == nil || step.Kind != initialStepKind(run) {
		return false
	}
	return isLLMProviderFailureStage(stringFromMap(step.Output, "stage"))
}

func (o *RuntimeOrchestrator) runtimeRepairAttemptCount(ctx context.Context, runID common.RunID) int {
	if o == nil || o.steps == nil || runID == "" {
		return 0
	}
	steps, err := o.steps.ListByRun(ctx, runID)
	if err != nil {
		return 0
	}
	count := 0
	for _, step := range steps {
		if isRuntimeRepairBudgetStep(runID, step) {
			count++
		}
	}
	return count
}

func (o *RuntimeOrchestrator) runtimeRepairAttemptCountForStep(ctx context.Context, runID common.RunID, failedStep *runModel.Step) int {
	if o == nil || o.steps == nil || runID == "" || failedStep == nil {
		return 0
	}
	scope := runtimeRepairScopeFromFailedStep(failedStep)
	return o.runtimeRepairAttemptCountForScopeInWindow(ctx, runID, scope, runtimeRepairTargetBudgetWindowRoot(failedStep))
}

func (o *RuntimeOrchestrator) runtimeRepairAttemptCountForEffectiveScope(ctx context.Context, runID common.RunID, failedStep *runModel.Step) int {
	exactAttempts := o.runtimeRepairAttemptCountForStep(ctx, runID, failedStep)
	targetScope := runtimeFailureTargetScopeFromStep(failedStep)
	if targetScope == "" {
		rootClassAttempts := o.runtimeRepairAttemptCountForRootClassInWindow(ctx, runID, failedStep)
		if rootClassAttempts > exactAttempts {
			return rootClassAttempts
		}
		return exactAttempts
	}
	class, _ := runtimeRepairScopeClassAndSignature(failedStep)
	targetAttempts := o.runtimeRepairAttemptCountForFailureTargetInWindow(ctx, runID, class, targetScope, runtimeRepairTargetBudgetWindowRootForFailureTarget(failedStep))
	if targetAttempts > exactAttempts {
		return targetAttempts
	}
	return exactAttempts
}

func (o *RuntimeOrchestrator) runtimeRepairAttemptCountForScope(ctx context.Context, runID common.RunID, scope runtimeRepairScope) int {
	return o.runtimeRepairAttemptCountForScopeInWindow(ctx, runID, scope, "")
}

func (o *RuntimeOrchestrator) runtimeRepairAttemptCountForScopeInWindow(ctx context.Context, runID common.RunID, scope runtimeRepairScope, windowRoot common.StepID) int {
	if o == nil || o.steps == nil || runID == "" {
		return 0
	}
	steps, err := o.steps.ListByRun(ctx, runID)
	if err != nil {
		return 0
	}
	count := 0
	for _, step := range steps {
		if !isRuntimeRepairBudgetStep(runID, step) {
			continue
		}
		if windowRoot != "" && runtimeRepairTargetBudgetWindowRoot(step) != windowRoot {
			continue
		}
		if runtimeRepairStepMatchesScope(step, scope) {
			count++
		}
	}
	return count
}

func (o *RuntimeOrchestrator) runtimeRepairAttemptCountForRootClassInWindow(ctx context.Context, runID common.RunID, failedStep *runModel.Step) int {
	if o == nil || o.steps == nil || runID == "" || failedStep == nil || !boolValue(failedStep.Input[stepInputKeyRuntimeRepairFollowUp]) {
		return 0
	}
	windowRoot := runtimeRepairTargetBudgetWindowRoot(failedStep)
	if windowRoot == "" {
		return 0
	}
	failureClass, _ := runtimeRepairScopeClassAndSignature(failedStep)
	failureClass = strings.TrimSpace(failureClass)
	if failureClass == "" {
		return 0
	}
	steps, err := o.steps.ListByRun(ctx, runID)
	if err != nil {
		return 0
	}
	count := 0
	for _, step := range steps {
		if !isRuntimeRepairBudgetStep(runID, step) {
			continue
		}
		if runtimeRepairTargetBudgetWindowRoot(step) != windowRoot {
			continue
		}
		stepClass := strings.TrimSpace(firstNonEmptyString(
			stringFromMap(step.Input, stepInputKeyRepairOfFailureClass),
			stringFromMap(step.Input, failureOutputKeyClass),
			stringFromMap(step.Output, failureOutputKeyClass),
		))
		if stepClass == failureClass {
			count++
		}
	}
	return count
}

func (o *RuntimeOrchestrator) runtimeRepairAttemptCountForFailureTarget(ctx context.Context, runID common.RunID, failureClass string, targetScope string) int {
	return o.runtimeRepairAttemptCountForFailureTargetInWindow(ctx, runID, failureClass, targetScope, "")
}

func (o *RuntimeOrchestrator) runtimeRepairAttemptCountForFailureTargetInWindow(ctx context.Context, runID common.RunID, failureClass string, targetScope string, windowRoot common.StepID) int {
	if o == nil || o.steps == nil || runID == "" {
		return 0
	}
	targetScope = strings.TrimSpace(targetScope)
	if targetScope == "" {
		return 0
	}
	failureClass = strings.TrimSpace(failureClass)
	steps, err := o.steps.ListByRun(ctx, runID)
	if err != nil {
		return 0
	}
	count := 0
	for _, step := range steps {
		if !isRuntimeRepairBudgetStep(runID, step) {
			continue
		}
		if runtimeFailureTargetScopeFromStep(step) != targetScope {
			continue
		}
		if windowRoot != "" && runtimeRepairTargetBudgetWindowRoot(step) != windowRoot {
			continue
		}
		if failureClass != "" {
			stepClass := strings.TrimSpace(firstNonEmptyString(
				stringFromMap(step.Input, stepInputKeyRepairOfFailureClass),
				stringFromMap(step.Input, failureOutputKeyClass),
				stringFromMap(step.Output, failureOutputKeyClass),
			))
			if stepClass != "" && stepClass != failureClass {
				continue
			}
		}
		count++
	}
	return count
}

func (o *RuntimeOrchestrator) duplicateRuntimeFailureBlockerCanAutoResume(ctx context.Context, run *runModel.Run, ticket *decisionmodel.Ticket) bool {
	if o == nil || run == nil || ticket == nil || o.steps == nil {
		return false
	}
	if !requiresDoDAssessmentCompletionGate(run) || ticket.State != common.DecisionTicketStateOpen {
		return false
	}
	if !strings.HasSuffix(string(ticket.ID), "duplicate-failure-decision") {
		return false
	}
	failedStep, err := o.steps.Get(ctx, ticket.StepID)
	if err != nil || failedStep == nil || failedStep.State != common.StepStateFailed {
		return false
	}
	class := strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeyClass))
	signature := strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeySignature))
	if signature == "" || !duplicateRepairDecisionFailureClass(class) {
		return false
	}
	attempts := o.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep)
	return attempts > 0 && attempts < o.maxRuntimeRepairAttempts()
}

func (o *RuntimeOrchestrator) runtimeRepairExhaustionBlockerCanAutoResume(ctx context.Context, run *runModel.Run, ticket *decisionmodel.Ticket) bool {
	if o == nil || run == nil || ticket == nil || o.steps == nil {
		return false
	}
	if !requiresDoDAssessmentCompletionGate(run) || ticket.State != common.DecisionTicketStateOpen {
		return false
	}
	if ticket.Context[decisionContextKeyRecoveryPolicy] != decisionContextRecoveryPolicyRepairPlan {
		return false
	}
	if !strings.HasSuffix(string(ticket.ID), "repair-exhausted-decision") {
		return false
	}
	if attempts, ok := ticket.Context["attempts"]; !ok || intFromAny(attempts) < o.maxRuntimeRepairAttempts() {
		return false
	}
	failedStep, err := o.steps.Get(ctx, ticket.StepID)
	if err != nil || failedStep == nil || failedStep.State != common.StepStateFailed {
		return false
	}
	if isLLMProviderRuntimeFailure(run, failedStep) {
		return false
	}
	targetScope := runtimeFailureTargetScopeFromStep(failedStep)
	if targetScope == "" {
		return false
	}
	if runtimeRepairTargetBudgetWindowRootForFailureTarget(failedStep) == "" {
		class, _ := runtimeRepairScopeClassAndSignature(failedStep)
		if !runtimeFailureTargetUsesRunBudget(class, targetScope) {
			return false
		}
	}
	return o.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep) < o.maxRuntimeRepairAttempts()
}

func (o *RuntimeOrchestrator) knownReadableDecisionBlockerCanAutoRepair(ctx context.Context, run *runModel.Run, ticket *decisionmodel.Ticket) bool {
	if o == nil || run == nil || ticket == nil || o.steps == nil {
		return false
	}
	if !requiresDoDAssessmentCompletionGate(run) || ticket.State != common.DecisionTicketStateOpen {
		return false
	}
	if strings.TrimSpace(ticket.DecisionKind) != "" &&
		strings.TrimSpace(ticket.DecisionKind) != decisioncommon.ResolutionTypeProvideMissingInfo {
		return false
	}
	step, err := o.steps.Get(ctx, ticket.StepID)
	if err != nil || step == nil || step.State != common.StepStateCompleted {
		return false
	}
	context := o.enrichDecisionContextWithReadableWorkspaceResources(ctx, run, nil, ticket.Context, ticket.Question, ticket.Reason)
	return knownReadableWorkspaceResourceDecisionContext(context)
}

func (o *RuntimeOrchestrator) runtimeRepairExhaustionBlockerHasSuccessEvidence(ctx context.Context, ticket *decisionmodel.Ticket) bool {
	if o == nil || o.steps == nil || ticket == nil || ticket.RunID == "" {
		return false
	}
	if ticket.Context[decisionContextKeyRecoveryPolicy] != decisionContextRecoveryPolicyRepairPlan {
		return false
	}
	scope := runtimeRepairScopeFromDecisionContext(ticket.Context)
	if scope.failureSignature == "" && scope.rootStepID == "" && scope.failedStepID == "" {
		return false
	}
	steps, err := o.steps.ListByRun(ctx, ticket.RunID)
	if err != nil {
		return false
	}
	for _, step := range steps {
		if !isRuntimeRepairSuccessEvidenceStep(step, scope) {
			continue
		}
		if ticket.StepID != "" && step.ID == ticket.StepID {
			continue
		}
		return true
	}
	return false
}

func isRuntimeRepairSuccessEvidenceStep(step *runModel.Step, scope runtimeRepairScope) bool {
	if step == nil || step.State != common.StepStateCompleted || step.Input == nil {
		return false
	}
	if !runtimeRepairStepMatchesScope(step, scope) {
		return false
	}
	if boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) {
		return false
	}
	if isProviderFailureClass(stringFromMap(step.Output, failureOutputKeyClass)) || isLLMProviderFailureStage(stringFromMap(step.Output, "stage")) {
		return false
	}
	return true
}

func isRuntimeRepairBudgetStep(runID common.RunID, step *runModel.Step) bool {
	if step == nil || step.RunID != runID || !boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) {
		return false
	}
	if boolValue(step.Input[stepInputKeyProviderRetryFollowUp]) {
		return false
	}
	return true
}

type runtimeRepairScope struct {
	failedStepID     common.StepID
	rootStepID       common.StepID
	failureClass     string
	failureSignature string
}

func runtimeRepairScopeFromFailedStep(step *runModel.Step) runtimeRepairScope {
	if step == nil {
		return runtimeRepairScope{}
	}
	class, signature := runtimeRepairScopeClassAndSignature(step)
	return runtimeRepairScope{
		failedStepID:     step.ID,
		rootStepID:       runtimeRepairRootStepID(step),
		failureClass:     class,
		failureSignature: signature,
	}
}

func runtimeRepairScopeClassAndSignature(step *runModel.Step) (string, string) {
	if step == nil {
		return "", ""
	}
	outputClass := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass))
	outputSignature := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeySignature))
	inputClass := strings.TrimSpace(stringFromMap(step.Input, stepInputKeyRepairOfFailureClass))
	inputSignature := strings.TrimSpace(stringFromMap(step.Input, stepInputKeyRepairOfFailureSig))
	if outputSignature != "" && outputClass != "" && !isProviderFailureClass(outputClass) {
		return outputClass, outputSignature
	}
	if outputSignature != "" && (!boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) || inputSignature == "" || failureSignatureHasTargetScope(outputSignature)) {
		return firstNonEmptyString(outputClass, inputClass), outputSignature
	}
	return firstNonEmptyString(inputClass, outputClass), firstNonEmptyString(inputSignature, outputSignature)
}

func failureSignatureHasTargetScope(signature string) bool {
	return runtimeFailureTargetScope(signature) != ""
}

func runtimeFailureTargetScopeFromStep(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	_, signature := runtimeRepairScopeClassAndSignature(step)
	if targetScope := runtimeFailureTargetScope(signature); targetScope != "" {
		return targetScope
	}
	return runtimeFailureTargetScope(firstNonEmptyString(
		stringFromMap(step.Input, stepInputKeyRepairOfFailureSig),
		stringFromMap(step.Input, failureOutputKeySignature),
		stringFromMap(step.Output, failureOutputKeySignature),
	))
}

func runtimeRepairTargetBudgetWindowRoot(step *runModel.Step) common.StepID {
	if step == nil || step.Input == nil {
		return ""
	}
	for _, key := range []string{
		stepInputKeyRepairOfRootStepID,
		stepInputKeyGraphRootStepID,
		stepInputKeyRecoveryRootStepID,
		stepInputKeyProviderRetryRootStep,
	} {
		root := common.StepID(strings.TrimSpace(stringFromMap(step.Input, key)))
		if root != "" {
			return normalizeRecoveryRootStepIDFromInput(step.Input, root)
		}
	}
	return ""
}

func runtimeRepairTargetBudgetWindowRootForFailureTarget(step *runModel.Step) common.StepID {
	class, _ := runtimeRepairScopeClassAndSignature(step)
	if runtimeFailureTargetUsesRunBudget(class, runtimeFailureTargetScopeFromStep(step)) {
		return ""
	}
	return runtimeRepairTargetBudgetWindowRoot(step)
}

func runtimeFailureTargetUsesRunBudget(class string, targetScope string) bool {
	if strings.TrimSpace(targetScope) == "" {
		return false
	}
	switch strings.TrimSpace(class) {
	case failureClassMCPRuntime, failureClassMCPCapabilityDenied, failureClassPatchApply:
		return true
	default:
		return false
	}
}

func runtimeFailureTargetScope(signature string) string {
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return ""
	}
	parts := strings.Split(signature, "|")
	values := map[string]string{}
	for _, part := range parts {
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		value = normalizeRuntimeFailureTargetValue(value)
		if name == "" || value == "" {
			continue
		}
		values[name] = value
	}
	for _, key := range []string{"failureTargets", "targetRef", "conflictRef"} {
		if value := values[key]; value != "" {
			return key + "=" + value
		}
	}
	if values["serverId"] != "" && values["tool"] != "" {
		return "serverId=" + values["serverId"] + "|tool=" + values["tool"]
	}
	for _, key := range []string{"serverId", "tool", "uri"} {
		if value := values[key]; value != "" {
			return key + "=" + value
		}
	}
	return ""
}

func normalizeRuntimeFailureTargetValue(value string) string {
	items := strings.Split(value, ",")
	ret := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		item = strings.TrimPrefix(item, "./")
		if item == "" {
			continue
		}
		ret = append(ret, item)
	}
	return strings.Join(ret, ",")
}

func runtimeRepairScopeFromDecisionContext(context map[string]any) runtimeRepairScope {
	if len(context) == 0 {
		return runtimeRepairScope{}
	}
	return runtimeRepairScope{
		failedStepID:     common.StepID(strings.TrimSpace(stringFromMap(context, "failedStepId"))),
		rootStepID:       common.StepID(strings.TrimSpace(firstNonEmptyString(stringFromMap(context, "repairOfRootStepId"), stringFromMap(context, stepInputKeyRepairOfRootStepID)))),
		failureClass:     strings.TrimSpace(stringFromMap(context, "failureClass")),
		failureSignature: strings.TrimSpace(stringFromMap(context, "failureSignature")),
	}
}

func runtimeRepairRootStepID(step *runModel.Step) common.StepID {
	if step == nil {
		return ""
	}
	if root := common.StepID(strings.TrimSpace(stringFromMap(step.Input, stepInputKeyRepairOfRootStepID))); root != "" {
		return normalizeRecoveryRootStepIDFromInput(step.Input, root)
	}
	if boolValue(step.Input[stepInputKeyProviderRetryFollowUp]) {
		if root := common.StepID(strings.TrimSpace(stringFromMap(step.Input, stepInputKeyProviderRetryRootStep))); root != "" {
			return normalizeRecoveryRootStepIDFromInput(step.Input, root)
		}
	}
	if root := graphRootStepID(step); root != "" {
		return normalizeRecoveryRootStepIDFromInput(step.Input, root)
	}
	return normalizeRecoveryRootStepIDFromInput(step.Input, step.ID)
}

func runtimeRepairStepMatchesScope(step *runModel.Step, scope runtimeRepairScope) bool {
	if step == nil {
		return false
	}
	if scope.failureSignature != "" {
		stepSignature := strings.TrimSpace(firstNonEmptyString(
			stringFromMap(step.Input, stepInputKeyRepairOfFailureSig),
			stringFromMap(step.Input, failureOutputKeySignature),
			stringFromMap(step.Output, failureOutputKeySignature),
		))
		if stepSignature != scope.failureSignature {
			if stepSignature == "" && !runtimeRepairStepHasExplicitScope(step) {
				return true
			}
			return false
		}
		if scope.failureClass != "" {
			stepClass := strings.TrimSpace(firstNonEmptyString(
				stringFromMap(step.Input, stepInputKeyRepairOfFailureClass),
				stringFromMap(step.Input, failureOutputKeyClass),
				stringFromMap(step.Output, failureOutputKeyClass),
			))
			if stepClass != "" && stepClass != scope.failureClass {
				return false
			}
		}
		return true
	}
	if scope.rootStepID != "" {
		stepRoot := common.StepID(strings.TrimSpace(firstNonEmptyString(
			stringFromMap(step.Input, stepInputKeyRepairOfRootStepID),
			string(graphRootStepID(step)),
		)))
		if stepRoot == scope.rootStepID {
			return true
		}
	}
	if scope.failedStepID != "" {
		if common.StepID(strings.TrimSpace(stringFromMap(step.Input, stepInputKeyRepairOfStepID))) == scope.failedStepID ||
			common.StepID(strings.TrimSpace(stringFromMap(step.Input, "failedStepId"))) == scope.failedStepID {
			return true
		}
	}
	if scope.failureSignature == "" && !runtimeRepairStepHasExplicitScope(step) {
		return true
	}
	return false
}

func runtimeRepairStepHasExplicitScope(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	for _, key := range []string{
		stepInputKeyRepairOfStepID,
		stepInputKeyRepairOfRootStepID,
		stepInputKeyRepairOfFailureClass,
		stepInputKeyRepairOfFailureSig,
		failureOutputKeyClass,
		failureOutputKeySignature,
		"failedStepId",
	} {
		if strings.TrimSpace(stringFromMap(step.Input, key)) != "" || strings.TrimSpace(stringFromMap(step.Output, key)) != "" {
			return true
		}
	}
	return false
}

func isProviderFailureClass(class string) bool {
	switch class {
	case failureClassProviderUnavailable,
		failureClassProviderSchemaDrift,
		failureClassProviderTimeout,
		failureClassContextTooLarge,
		failureClassRateLimited,
		failureClassProviderStreamIncomplete,
		failureClassAuthOrConfigError:
		return true
	default:
		return false
	}
}

func isLLMProviderFailureStage(stage string) bool {
	switch stage {
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
		"llm_provider_request_failed",
		"llm_provider_schema_drift",
		"llm_provider_response_schema_invalid",
		"llm_provider_protocol_invalid":
		return true
	default:
		return false
	}
}

func terminalAssessmentOnlyDecisionContractFailure(step *runModel.Step) bool {
	if step == nil || !isLLMDecisionContractFailureStage(stringFromMap(step.Output, "stage")) {
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
		"dodassessment.goalcoverage item and state are required",
		"dodassessment.goalcoverage.items are required",
		"dodassessment.goalcoverage.satisfied is required",
		"satisfied dodassessment.goalcoverage item requires evidencerefs",
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
	return len(missingDoDItemsFromFailureOutput(step.Output)) > 0 ||
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
		return missing
	}
	return missingDeliverableCoverageFromDoDAssessment(doDAssessmentFromOutput(step.Output))
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

func (o *RuntimeOrchestrator) missingGoalCoverageWithEvidenceCandidates(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	missing []map[string]any,
) []map[string]any {
	if len(missing) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(missing))
	for idx, item := range missing {
		next := cloneStepPayload(item)
		requirement := missingGoalCoverageRequirementText(next)
		normalized := normalizeDoDCoverageText(requirement)
		if strings.TrimSpace(stringFromMap(next, "goalRequirementId")) == "" && normalized != "" {
			next["goalRequirementId"] = goalCoverageRequirementID(idx+1, normalized)
		}
		if len(mapSliceFromValue(next["evidenceCandidates"])) == 0 {
			if candidates := o.goalCoverageEvidenceCandidatesForRequirement(ctx, run, failedStep, requirement); len(candidates) > 0 {
				next["evidenceCandidates"] = candidates
			}
		}
		if len(mapSliceFromValue(next["evidenceCandidates"])) == 0 {
			next["evidenceCandidates"] = o.latestCoverageBoundActionEvidenceCandidates(ctx, run, failedStep, requirement, 5)
		}
		ret = append(ret, next)
	}
	return ret
}

func (o *RuntimeOrchestrator) missingDoDItemsWithEvidenceCandidates(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	missing []map[string]any,
) []map[string]any {
	if len(missing) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(missing))
	for _, item := range missing {
		next := cloneStepPayload(item)
		if len(mapSliceFromValue(next["evidenceCandidates"])) == 0 {
			requirement := strings.TrimSpace(stringFromMap(next, "item"))
			if candidates := o.goalCoverageEvidenceCandidatesForRequirement(ctx, run, failedStep, requirement); len(candidates) > 0 {
				next["evidenceCandidates"] = candidates
			}
		}
		if len(mapSliceFromValue(next["evidenceCandidates"])) == 0 {
			next["evidenceCandidates"] = o.latestCoverageBoundActionEvidenceCandidates(ctx, run, failedStep, strings.TrimSpace(stringFromMap(next, "item")), 5)
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

func (o *RuntimeOrchestrator) goalCoverageEvidenceCandidatesForRequirement(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	requirement string,
) []map[string]any {
	if o == nil || o.steps == nil || run == nil {
		return nil
	}
	terms := coverageRequirementTerms(requirement)
	if len(terms) == 0 {
		return nil
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil
	}
	type candidate struct {
		score int
		item  map[string]any
	}
	candidates := make([]candidate, 0)
	for _, step := range steps {
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

func (o *RuntimeOrchestrator) latestActionEvidenceCandidates(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	limit int,
) []map[string]any {
	if o == nil || o.steps == nil || run == nil || limit <= 0 {
		return nil
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil || len(steps) == 0 {
		return nil
	}
	steps = orderedRuntimeContextSteps(steps)
	ret := make([]map[string]any, 0, limit)
	for idx := len(steps) - 1; idx >= 0 && len(ret) < limit; idx-- {
		step := steps[idx]
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

func (o *RuntimeOrchestrator) latestCoverageBoundActionEvidenceCandidates(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	coverageTarget string,
	limit int,
) []map[string]any {
	if o == nil || o.steps == nil || run == nil || limit <= 0 {
		return nil
	}
	coverageTarget = normalizeCoverageGapRepairToken(coverageTarget)
	if coverageTarget == "" {
		return nil
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil || len(steps) == 0 {
		return nil
	}
	steps = orderedRuntimeContextSteps(steps)
	ret := make([]map[string]any, 0, limit)
	for idx := len(steps) - 1; idx >= 0 && len(ret) < limit; idx-- {
		step := steps[idx]
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
	if decision := mapFromValue(step.Output[stepOutputKeyLLMDecision]); len(decision) > 0 {
		for _, request := range mapSliceFromValue(decision["patchRequests"]) {
			for _, value := range patchNodeCoverageTargetTokens(request) {
				add(value)
			}
		}
	}
	return ret
}

func (o *RuntimeOrchestrator) runtimeRepairCurrentExecutionProgress(
	ctx context.Context,
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
	if latest := runtimeRepairProgressEvidenceSummaries(o.latestActionEvidenceCandidates(ctx, run, failedStep, 6)); len(latest) > 0 {
		ret["latestActionEvidence"] = latest
	}
	if attempts := runtimeRepairAttemptProgressSummaries(o.recentRuntimeRepairAttemptSummaries(ctx, run.ID, failedStep, 5)); len(attempts) > 0 {
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

func runtimeRepairFollowUpStepID(runID common.RunID, failedStep *runModel.Step, attempt int) common.StepID {
	base := ""
	if failedStep != nil {
		base = runtimeReadableStepLabel(runID, runtimeRepairIDLabelStepID(failedStep))
	}
	if base == "" {
		base = "repair"
	}
	if attempt < 1 {
		attempt = 1
	}
	return common.StepID(fmt.Sprintf("%s/repair-%03d-of-%s", runID, attempt, base))
}

func runtimeRepairIDLabelStepID(failedStep *runModel.Step) common.StepID {
	if failedStep == nil {
		return ""
	}
	if runtimeRepairIDShouldUseFailedStep(failedStep) {
		return failedStep.ID
	}
	if repairOf := common.StepID(strings.TrimSpace(stringFromMap(failedStep.Input, stepInputKeyRepairOfStepID))); runtimeRepairIDShouldUseRepairOfStepID(failedStep, repairOf) {
		return repairOf
	}
	if graphLineage := runtimeRepairGraphLineageLabelStepID(failedStep); graphLineage != "" {
		return graphLineage
	}
	return runtimeRepairRootStepID(failedStep)
}

func runtimeRepairIDShouldUseFailedStep(failedStep *runModel.Step) bool {
	if failedStep == nil {
		return false
	}
	if boolValue(failedStep.Input[stepInputKeyProviderRetryFollowUp]) ||
		boolValue(failedStep.Input[stepInputKeyRuntimeRepairFollowUp]) ||
		boolValue(failedStep.Input["assessmentOnlyContractCorrection"]) {
		return false
	}
	if graphNodeID(failedStep) == "" {
		return false
	}
	return strings.Contains(strings.TrimSpace(string(failedStep.ID)), "/graph-")
}

func runtimeRepairIDShouldUseRepairOfStepID(failedStep *runModel.Step, repairOf common.StepID) bool {
	if failedStep == nil || repairOf == "" {
		return false
	}
	if boolValue(failedStep.Input[stepInputKeyProviderRetryFollowUp]) ||
		boolValue(failedStep.Input["assessmentOnlyContractCorrection"]) {
		return false
	}
	return strings.Contains(strings.TrimSpace(string(repairOf)), "/graph-")
}

func runtimeRepairGraphLineageLabelStepID(failedStep *runModel.Step) common.StepID {
	if failedStep == nil ||
		boolValue(failedStep.Input[stepInputKeyProviderRetryFollowUp]) ||
		boolValue(failedStep.Input["assessmentOnlyContractCorrection"]) {
		return ""
	}
	for _, candidate := range []common.StepID{
		common.StepID(strings.TrimSpace(stringFromMap(failedStep.Input, stepInputKeyRepairOfStepID))),
		common.StepID(strings.TrimSpace(stringFromMap(failedStep.Input, "failedStepId"))),
		failedStep.ID,
	} {
		if label := runtimeEmbeddedGraphLineageLabel(failedStep.RunID, candidate); label != "" {
			return label
		}
	}
	return ""
}

func runtimeEmbeddedGraphLineageLabel(runID common.RunID, stepID common.StepID) common.StepID {
	raw := strings.TrimSpace(string(stepID))
	if raw == "" {
		return ""
	}
	if runID != "" {
		raw = strings.TrimPrefix(raw, strings.TrimSpace(string(runID))+"/")
	}
	raw = strings.Trim(raw, "/")
	if strings.HasPrefix(raw, "graph-") {
		return common.StepID(raw)
	}
	if idx := strings.Index(raw, "-of-graph-"); idx >= 0 {
		return common.StepID(strings.TrimSpace(raw[idx+len("-of-"):]))
	}
	return ""
}

func (o *RuntimeOrchestrator) runtimeRepairFollowUpStepInput(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	attempt int,
	execErr error,
) map[string]any {
	previousStepID := common.StepID("")
	if failedStep != nil {
		previousStepID = failedStep.ID
	}
	input := nextStepInput(run, previousStepID)
	input[stepInputKeySoulStage] = soulStageExecution
	setRuntimeStage(input, runtimeStageRepair)
	input[stepInputKeyRuntimeRepairFollowUp] = true
	input[stepInputKeyRuntimeRepairAttempt] = attempt
	input["maxRuntimeRepairAttempts"] = o.maxRuntimeRepairAttempts()
	if execErr != nil {
		input["failureReason"] = execErr.Error()
	}
	if failedStep == nil {
		return input
	}
	input["failedStepId"] = string(failedStep.ID)
	input[stepInputKeyRepairOfStepID] = string(failedStep.ID)
	rootStepID := runtimeRepairRootStepID(failedStep)
	input[stepInputKeyRepairOfRootStepID] = string(rootStepID)
	input[stepInputKeyRecoveryAction] = string(runtimeRecoveryActionRuntimeRepair)
	input[stepInputKeyRecoveryRootStepID] = string(rootStepID)
	input[stepInputKeyRecoveryLineageType] = "runtime_repair"
	input["failedStepKind"] = failedStep.Kind
	input["failedStepState"] = string(failedStep.State)
	input["failedStepInput"] = compactStepInputForLLMContext(failedStep.Input)
	input["failedStepOutput"] = compactStepOutputForLLMContext(failedStep.Output)
	if runtimeRepairShouldReturnToRejectedResultContinuation(run, failedStep) {
		input = resetRuntimeRepairInputToRejectedResultContinuation(input, run, failedStep)
		contract := BuildConvergenceContract(run, nil, failedStep, map[string]any{"input": input}, nil)
		input = ApplyConvergenceContract(input, contract)
		return input
	}
	if diagnostics := runtimeRepairLastFailureDiagnostics(failedStep); len(diagnostics) > 0 {
		input["lastFailureDiagnostics"] = diagnostics
	}
	if diagnostics := mapFromValue(failedStep.Output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		input["runtimeDiagnostics"] = cloneStepPayload(diagnostics)
		if stringFromMap(diagnostics, "kind") == "dod_evidence_quality_gap" {
			input["repairObjective"] = repairObjectiveConcreteValidationEvidence
			input["evidenceRepairRequired"] = true
			input["verificationEvidenceGap"] = cloneStepPayload(diagnostics)
			input["repairGuidance"] = concreteValidationEvidenceRepairGuidance()
		}
	}
	assessmentOnlyCoverageRepair := doDCoverageContractCorrectionRequired(failedStep)
	if assessmentOnlyCoverageRepair {
		if missingGoalCoverage := missingGoalCoverageFromStep(failedStep); len(missingGoalCoverage) > 0 {
			input["missingGoalCoverage"] = o.missingGoalCoverageWithEvidenceCandidates(ctx, run, failedStep, missingGoalCoverage)
		}
	}
	if missingDoD := missingDoDItemsFromStep(failedStep); len(missingDoD) > 0 {
		input["missingDoDItems"] = o.missingDoDItemsWithEvidenceCandidates(ctx, run, failedStep, missingDoD)
	}
	if missingDeliverableCoverage := missingDeliverableCoverageFromStep(failedStep); len(missingDeliverableCoverage) > 0 &&
		len(mapSliceFromValue(input["missingDeliverableCoverage"])) == 0 {
		input["missingDeliverableCoverage"] = missingDeliverableCoverage
	}
	if inheritCoverageRepairContext(input, failedStep) {
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Preserve inherited coverage gaps from the previous repair attempt; a contract failure on the repair wrapper does not clear the original Goal/DoD/deliverable gaps.",
			"Use missingDoDItems, missingGoalCoverage, and missingDeliverableCoverage as the current unresolved scope unless current evidence explicitly closes an item.",
		)
	}
	assessmentOnlyFailedCoverageRepair := assessmentOnlyCoverageCorrectionFailedWithActionableGaps(failedStep)
	coverageNeedsConcreteEvidence := assessmentOnlyCoverageRepair && (coverageCorrectionNeedsConcreteEvidence(&runModel.Step{
		Input:  input,
		Output: failedStep.Output,
	}) || assessmentOnlyFailedCoverageRepair)
	coverageValidationEvidenceRepair := !assessmentOnlyCoverageRepair &&
		coverageValidationEvidenceRepairEligible(failedStep) &&
		coverageVerificationGapPresent(input, failedStep) &&
		!repairHasImplementationTargetEvidence(input, mapFromValue(input["repairActionPackage"])) &&
		!repairHasImplementationTargetEvidence(failedStep.Input, mapFromValue(failedStep.Input["repairActionPackage"]))
	validationRepairEscalatesToStateChanging := coverageValidationEvidenceRepair &&
		o.coverageValidationRepairShouldEscalateToStateChanging(ctx, run, failedStep, input)
	if validationRepairEscalatesToStateChanging {
		input["validationRepairEscalatedToImplementation"] = true
		input["repairObjective"] = repairObjectiveStateChangingOutput
		input["stateChangingOutputRequired"] = true
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"This coverage gap has already been repaired through validation-only attempts without concrete implementation target evidence.",
			"Switch to a state-changing implementation repair before another verification pass; do not continue the validation-only loop.",
			"Produce the smallest state-changing artifact that makes the next bounded verification meaningful.",
		)
		delete(input, stepInputKeyAssessmentOnly)
		delete(input, "assessmentOnlyReason")
		delete(input, "boundedValidationRequired")
		delete(input, "evidenceRepairSource")
	}
	coverageValidationEvidenceRepair = coverageValidationEvidenceRepair && !validationRepairEscalatesToStateChanging
	concreteEvidenceRepair := coverageNeedsConcreteEvidence || coverageValidationEvidenceRepair || (!assessmentOnlyCoverageRepair && doDVerificationEvidenceRepairRequired(failedStep))
	if concreteEvidenceRepair {
		claimAlignedEvidenceNeeded := false
		if diagnostics := mapFromValue(failedStep.Output["runtimeDiagnostics"]); len(diagnostics) > 0 {
			claimAlignedEvidenceNeeded = boolValue(diagnostics["claimAlignedEvidenceNeeded"])
		}
		input["repairObjective"] = repairObjectiveConcreteValidationEvidence
		input["evidenceRepairRequired"] = true
		input["verificationEvidenceGap"] = map[string]any{
			"kind":                       "dod_verification_evidence_gap",
			"failedStepId":               string(failedStep.ID),
			"failedStepKind":             string(failedStep.Kind),
			"missingDoD":                 missingDoDItemsFromFailureOutput(failedStep.Output),
			"missingGoal":                missingGoalCoverageFromStep(failedStep),
			"missingDeliverable":         missingDeliverableCoverageFromStep(failedStep),
			"claimAlignedEvidenceNeeded": claimAlignedEvidenceNeeded,
		}
		input["repairGuidance"] = concreteValidationEvidenceRepairGuidance()
		delete(input, "stateChangingOutputRequired")
	}
	if assessmentOnlyCoverageRepair && !coverageNeedsConcreteEvidence && !validationRepairEscalatesToStateChanging {
		input[stepInputKeySoulStage] = soulStageValidation
		setRuntimeStage(input, runtimeStageVerification)
		input[stepInputKeyAssessmentOnly] = true
		input["completionFollowUp"] = true
		input["assessmentOnlyReason"] = "dod_coverage_contract_correction"
		input["repairObjective"] = "complete_dod_goal_coverage_assessment"
		input["repairGuidance"] = []string{
			"Return only completed or failed dodAssessment, or a real governance blocker.",
			"Do not return executionGraph, shellCommands, patchRequests, mcpToolCalls, memoryWrites, or other executable work.",
			"Preserve every still-valid DoD item assessment and add any missing DoD items exactly as listed in missingDoDItems.",
			"Add one independent dodAssessment.goalCoverage.items entry for every missingGoalCoverage entry; copy its goalRequirementId when present and cover the exact raw requirement without merging multiple missing entries into a broader item.",
			"Add one independent dodAssessment.deliverableCoverage.items entry for every missingDeliverableCoverage entry or required deliverableExpectation id; each satisfied entry must copy the exact id and cite evidenceRefs or artifactRefs.",
			"Treat evidenceCandidates as neutral candidates, not automatic proof; use them only when their concrete output supports the exact Goal requirement.",
			"When latestClosureEvidence is present, compare the latest successful evidence against older failed evidence in the same neutral scope; do not reject solely because an earlier superseded failure exists.",
			"If latestClosureEvidence is still insufficient, explain the exact missing dimension and cite the latest evidence that was considered.",
			"Each satisfied dodAssessment.goalCoverage item must cite concrete evidenceRefs from the candidate evidence or other available evidence.",
			"If evidence does not support a Goal or DoD item, mark it failed/skipped with reason, residualRisks, and nextActions instead of claiming satisfaction.",
		}
		if closure := o.latestCompletionClosureEvidence(ctx, run, failedStep); len(closure) > 0 {
			input["latestClosureEvidence"] = closure
			if superseded := mapSliceFromValue(closure["supersededFailures"]); len(superseded) > 0 {
				input["supersededFailureEvidence"] = superseded
			}
		}
		if missingDeliverableCoverage := missingDeliverableCoverageFromStep(failedStep); len(missingDeliverableCoverage) > 0 {
			input["missingDeliverableCoverage"] = missingDeliverableCoverage
		}
		if requiredDeliverables := requiredProfileDeliverableExpectations(run); len(requiredDeliverables) > 0 {
			input["requiredDeliverableCoverage"] = requiredDeliverables
		}
	}
	if coverageNeedsConcreteEvidence {
		delete(input, stepInputKeyAssessmentOnly)
		delete(input, "assessmentOnlyReason")
		input["completionFollowUp"] = false
		input["evidenceRepairSource"] = "coverage_gap_without_candidate_evidence"
		if assessmentOnlyFailedCoverageRepair {
			input["evidenceRepairSource"] = "failed_assessment_only_coverage_gap"
		}
		if closure := o.latestCompletionClosureEvidence(ctx, run, failedStep); len(closure) > 0 {
			input["latestClosureEvidence"] = closure
			if superseded := mapSliceFromValue(closure["supersededFailures"]); len(superseded) > 0 {
				input["supersededFailureEvidence"] = superseded
			}
		}
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"At least one Goal/DoD/deliverable coverage gap still lacks acceptable concrete evidence; produce bounded verification evidence before another dodAssessment correction.",
			"When latestClosureEvidence is present, prefer the latest successful evidence over older superseded failures in the same neutral scope.",
			"If new verification evidence closes a specific Goal requirement, ensure the next dodAssessment cites that exact evidenceRef and goalRequirementId.",
		)
	}
	if coverageValidationEvidenceRepair {
		input["boundedValidationRequired"] = true
		input["evidenceRepairSource"] = "coverage_gap_without_implementation_target"
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Unresolved Goal/DoD/deliverable coverage is currently a validation evidence gap because no concrete implementation target evidence is available.",
			"Produce bounded shell/tool/MCP validation evidence that maps to the coverageTargetSet before attempting another assessment.",
			"When an external dependency, credential, service, network endpoint, port, clock, filesystem path, or equivalent runtime prerequisite is unavailable, first try a neutral controlled substitute such as a temporary fixture, local stub/mock, alternate temporary config, isolated working directory, or available ephemeral port through the allowed validation tools; do not mark the item skipped or request a decision until such bounded substitute validation is impossible or would violate the Goal/DoD.",
			"Do not return patch/apply unless a later validation failure provides concrete target refs, line refs, diagnostics, or current target snapshots.",
		)
	}
	ensureCoverageRepairTargetSet(input)
	transientExecutionTimeout := runtimeRepairFailureIsTransientExecutionTimeout(failedStep, execErr)
	if transientExecutionTimeout {
		input["repairObjective"] = repairObjectiveTransientExecutionRetry
		input["executionTimeoutRetryAllowed"] = true
		input["repairGuidance"] = transientExecutionRetryGuidance()
	}
	if !assessmentOnlyCoverageRepair && !concreteEvidenceRepair && stateChangingRepairRequired(run, failedStep, execErr) {
		input["repairObjective"] = repairObjectiveStateChangingOutput
		input["stateChangingOutputRequired"] = true
		input["repairGuidance"] = stateChangingRepairGuidance()
	}
	if contract := capabilityPolicyRepairContractFromFailure(run, failedStep, execErr); len(contract) > 0 {
		input["capabilityPolicyRepairRequired"] = true
		input["repairObjective"] = "repair_capability_policy_follow_up"
		input["stateChangingOutputRequired"] = true
		input["capabilityPolicyRepairContract"] = contract
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"The previous execution graph violated a declared capability usage policy after a trigger node.",
			"Repair the graph contract directly: for each triggered policy, add a dependent mcp/tool_call that matches the policy preferredTools, or declare a structured capabilityPolicySkips item with policyId/category and a non-empty reason.",
			"Do not return assessment-only output for this repair; the failure is a mechanical execution-graph contract violation that can continue through a corrected graph or structured skip.",
		)
		delete(input, stepInputKeyAssessmentOnly)
		delete(input, "assessmentOnlyReason")
	}
	if boolValue(input["stateChangingOutputRequired"]) &&
		(len(mapSliceFromValue(input["missingDoDItems"])) > 0 || len(missingGoalCoverageFromInput(input)) > 0) {
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"When missingDoDItems or missingGoalCoverage are present, at least one patch/apply node must declare coverageTargets and each entry must match the exact missing DoD item text or goalRequirementId/item it is intended to close.",
			"Do not return a generic skeleton patch. If a patch cannot be tied to a concrete missing coverage target, return a failed dodAssessment or blocker instead.",
		)
	}
	class, signature := runtimeRepairScopeClassAndSignature(failedStep)
	if class != "" {
		input[failureOutputKeyClass] = class
		input[stepInputKeyRepairOfFailureClass] = class
	}
	if signature != "" {
		input[failureOutputKeySignature] = signature
		input[stepInputKeyRepairOfFailureSig] = signature
	}
	if signals := structuredFailureSignalsFromFailureOutput(failedStep.Output, execErr, class, signature); len(signals) > 0 {
		input["structuredFailureSignals"] = signals
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Use structuredFailureSignals as mechanical failure evidence only; paths, line refs, and identifiers must be reconciled with current target snapshots or fresh reads before patching.",
			"Do not treat identifier-like tokens in failure text as proof that a named element should be invented; ground every referenced name in current resources or introduce it explicitly in the same patch.",
		)
		if structuredFailureSignalsContainShapeConflict(signals) {
			if primary := primaryDiagnosticRepairContractFromSignals(signals); len(primary) > 0 {
				input["primaryDiagnosticRepair"] = primary
			}
			input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
				"Prioritize the primary diagnostics from structuredFailureSignals before secondary cleanup: fix reported unresolved names, invalid attributes, shape mismatches, contract-entry mismatches, and line-referenced target errors first.",
				"When diagnostics identify a concrete target snapshot, reconcile that target with current visible resource or contract shape before changing secondary validation details, evidence wording, or unrelated quality improvements.",
			)
		}
	}
	targetScope := runtimeFailureTargetScope(signature)
	if targetScope != "" {
		input["failureTargetScope"] = targetScope
		input["targetRepairAttempts"] = o.runtimeRepairAttemptCountForFailureTargetInWindow(ctx, run.ID, class, targetScope, runtimeRepairTargetBudgetWindowRootForFailureTarget(failedStep))
		if boolValue(input["stateChangingOutputRequired"]) {
			input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
				"When failureTargetScope is present, verificationPlan must stay bounded to that scope and explain the exact targeted checks.",
				"Do not run unbounded verification for scoped repairs unless an explicit target scope or filter is provided.",
			)
		}
	}
	if reassessment := repairStrategyReassessmentFromInput(input); len(reassessment) > 0 {
		input["strategyReassessment"] = reassessment
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"strategyReassessment.required is true: explicitly choose repairMode as continue_same_target_repair or shift_root_cause_target, or return a failed assessment/blocker.",
			"Explain the next repair target decision in neutral terms: failedSurface, rootCauseTarget, repairTargetDecision, repairMode, forbiddenRepair, and verificationPlan.",
			"When continuing the same failed target, patch only if the current target snapshot and primary diagnostics show a concrete same-target correction; otherwise choose a grounded root-cause target or return a blocker.",
			"When shifting root-cause target, patch a neutral grounded root-cause resource and declare relatedFailureTargets for the failed surface.",
			"Do not weaken still-valid Goal or DoD constraints, remove valid evidence, or reduce the requested outcome to make verification pass.",
		)
	}
	budgetScope := recoveryBudgetScope("runtime_repair", rootStepID, class, signature)
	if targetScope != "" {
		budgetScope["failureTargetScope"] = targetScope
	}
	input[stepInputKeyRecoveryBudgetScope] = budgetScope
	input["previousStepKind"] = failedStep.Kind
	input["previousStepOutput"] = compactStepOutputForLLMContext(failedStep.Output)
	if nodeID := graphNodeID(failedStep); nodeID != "" {
		input[stepInputKeyGraphPreviousNodeID] = nodeID
		input["failedGraphNodeId"] = nodeID
	}
	if completed := graphCompletedNodesFromStep(failedStep); len(completed) > 0 {
		input[stepInputKeyGraphCompletedNodes] = completed
	}
	if branch := graphBranch(failedStep); branch != "" {
		input[stepOutputKeyGraphBranch] = branch
	}
	if graph, ok := runtimeGraphSpecFromStep(failedStep); ok {
		input["failedExecutionGraph"] = compactExecutionGraphForLLMContext(graph)
		input[stepInputKeyGraphRootStepID] = string(graphRootStepID(failedStep))
	}
	if history := o.currentGovernanceHistoryForLLMInput(ctx, run); len(history) > 0 {
		input["governanceHistory"] = history
	}
	if evidence := o.completedGraphEvidence(ctx, run, failedStep); len(evidence) > 0 {
		input[stepInputKeyGraphEvidence] = evidence
	}
	if progress := o.runtimeRepairCurrentExecutionProgress(ctx, run, failedStep, input); len(progress) > 0 {
		input["currentExecutionProgress"] = progress
	}
	patchContext := o.runtimeRepairPatchContext(ctx, run, failedStep)
	if len(patchContext) > 0 {
		input["patchRepairContext"] = patchContext
		if suggestions := o.runtimeRepairPatchSuggestions(ctx, run, failedStep); len(suggestions) > 0 {
			input["patchRepairSuggestions"] = suggestions
		}
		if satisfied := o.runtimeRepairPatchAlreadySatisfiedSignals(ctx, run, failedStep); len(satisfied) > 0 {
			input["currentTargetAlreadySatisfiesFailedPatch"] = satisfied
			input["stateChangeAlreadySatisfiedByCurrentTarget"] = true
		}
		if len(mapSliceFromValue(input["currentTargetAlreadySatisfiesFailedPatch"])) == 0 &&
			!boolValue(input["concreteFailureEvidenceRequired"]) &&
			stringFromMap(input, "repairObjective") != repairObjectiveConcreteValidationEvidence &&
			stringFromMap(input, "repairObjective") != repairObjectiveTransientExecutionRetry {
			input["repairObjective"] = repairObjectiveStateChangingOutput
			input["stateChangingOutputRequired"] = true
		}
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Use patchRepairContext as the current target snapshot for patch repair; treat the failed patch text as stale or untrusted.",
			"When the failure is a diff context mismatch or file/edit oldText conflict, do not retry the same stale hunk or oldText; rebuild the smallest patch from patchRepairContext.content/contentHash.",
			"When using baseHash, copy the literal contentHash value from the current target snapshot; never output unresolved references such as ${...}, {{...}}, or symbolic placeholders.",
			"Use complete patch/apply file_write only when the current snapshot is marked safeForFileWrite/contentComplete and contains no redaction markers; otherwise use patch/apply text_replace with exact oldText/newText/baseHash, a minimal unified_diff with exact visible context, file/edit with exact oldText, or first read the target.",
			"Never copy runtime redaction marker tokens, omitted sections, or truncated snapshot text into patch/apply content or diff.",
			"If a previous patch was rejected for redacted runtime markers, discard that diff entirely and rebuild from current unredacted file/read or patchRepairContext content; do not transform redacted marker text into source content.",
			"If patchRepairSuggestions are present, prefer patch/apply text_replace with exact oldText/newText/baseHash, or use that evidence to construct the smallest current unified_diff/file/edit repair.",
			"If patch application failed because of malformed unified_diff syntax, return patch/apply text_replace with exact oldText/newText/baseHash or a complete standard unified diff with explicit line ranges such as @@ -12,3 +12,4 @@.",
			"Before adding new target content, reconcile the current snapshot and modify an existing equivalent item instead of creating duplicate named artifacts.",
			"Any MCP tool call that synchronizes modified file content must include the complete current file content in input.content together with the target path; do not emit path-only, root-only, or partial sync requests.",
			"When patching resources that reference existing named elements, patch against names that already exist in current workspace resources unless the Goal explicitly requires introducing a new named element.",
			"If the current target snapshot proves the failed patch is no longer required, return a bounded verification evidence graph instead of another patch; do not use read-only inspection as the only evidence.",
		)
		if len(mapSliceFromValue(input["currentTargetAlreadySatisfiesFailedPatch"])) > 0 {
			input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
				"currentTargetAlreadySatisfiesFailedPatch is a mechanical signal that the failed artifact's intended line changes are already reflected in the current target snapshot.",
				"Do not repeat the same failed patch when this signal is present; return a bounded concrete validation evidence graph, or a failed dodAssessment/blocker if validation cannot be produced.",
			)
		}
	}
	targetContext := o.runtimeRepairFailureTargetContext(ctx, run, failedStep)
	if len(targetContext) > 0 {
		input["failureTargetRepairContext"] = targetContext
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Use failureTargetRepairContext as the current snapshot for targets identified by the failed evidence.",
			"When failed evidence points at a concrete target resource, generate the smallest auditable patch/apply state change against that current snapshot before rerunning verification.",
			"Use workspaceFileManifest to reconcile target resources with other known workspace resources; avoid replacing unrelated existing artifacts just to satisfy one failure target.",
			"When a failure mentions missing names, invalid attributes, or contract-entry mismatches, do not invent new names in the target patch; first reconcile the target with currently visible resource shape from relevant existing resources, or read the relevant resource before patching.",
			"When the failure text contains line-referenced unresolved names, invalid attributes, shape mismatches, or equivalent resource-shape diagnostics in the failure target, repair those reported blockers before secondary validation, formatting, or evidence refinements.",
		)
	}
	if shapeContext := o.runtimeRepairResourceShapeAlignmentContext(ctx, run, failedStep); len(shapeContext) > 0 {
		input["resourceShapeAlignmentContext"] = shapeContext
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Use resourceShapeAlignmentContext as current same-directory resource evidence when diagnostics report unresolved names, invalid attributes, shape mismatches, contract-entry mismatches, or equivalent resource-shape conflicts.",
			"Before introducing new named elements or attributes, reconcile the failed target with existing current workspace resources in resourceShapeAlignmentContext.",
			"If the failed target disagrees with current resource shape, prefer the smallest coherent state change that aligns the target and existing resources; do not invent replacement contracts from failure text alone.",
		)
	}
	if rootCauseCandidates := o.runtimeRepairRootCauseCandidateContext(ctx, run, failedStep); len(rootCauseCandidates) > 0 {
		input["rootCauseCandidateContext"] = rootCauseCandidates
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Use rootCauseCandidateContext to evaluate grounded adjacent or referenced resources when the failed surface may be unblocked by changing a related current workspace resource.",
			"Do not assume the failed surface itself is the root cause when current workspace resources show adjacent or referenced resources that the failed surface depends on.",
			"If choosing a rootCauseTarget that differs from the failed surface, patch that resource directly and declare relatedFailureTargets for the failed surface being unblocked.",
		)
	}
	if context := o.convertedDecisionReadableResourceContext(ctx, run, failedStep); len(context) > 0 {
		input["readableResourceRepairContext"] = context
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"The previous decision requested information that is already available as a readable workspace resource.",
			"Use readableResourceRepairContext as current evidence; do not ask the operator to paste the same resource content.",
		)
	}
	if shouldIncludeWorkspaceManifestInRuntimeRepair(failedStep) || len(targetContext) > 0 || len(patchContext) > 0 {
		manifest := o.workspaceFileManifest(ctx, run)
		if len(manifest) > 0 {
			input[stepInputKeyWorkspaceFileManifest] = manifest
		}
	}
	inheritPriorRepairContext(input, failedStep)
	if boolValue(input["stateChangingOutputRequired"]) {
		if _, ok := input["lastFailureDiagnostics"]; !ok {
			if fallback := runtimeRepairLastFailureDiagnostics(failedStep); len(fallback) > 0 {
				input["lastFailureDiagnostics"] = fallback
			}
		}
		narrowStateChangingRecoveryInput(input, failedStep, execErr, "runtime_repair")
		if pkg := repairActionPackageFromRepairInput(input, failedStep.Output); len(pkg) > 0 {
			if repairActionPackageRequiresEvidenceAcquisition(pkg) {
				delete(input, "stateChangingOutputRequired")
				input["repairObjective"] = repairObjectiveAcquireFailureEvidence
				input["concreteFailureEvidenceRequired"] = true
				input["evidenceAcquisitionReason"] = stringFromMap(pkg, "evidenceAcquisitionReason")
				input["repairGuidance"] = concreteFailureEvidenceAcquisitionGuidance()
			}
			input["repairActionPackage"] = pkg
			compactStateChangingRepairInputAroundActionPackage(input)
		}
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Use repairActionPackage as the primary actionable repair contract; it contains the required output kind, rejected output summary, and current target snapshots.",
		)
		if !boolValue(input["concreteFailureEvidenceRequired"]) {
			input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
				"When repairActionPackage.requiredOutput.mustIncludeStateChangingNode is true, the next output must include an accepted state-changing operation before verification, or return a failed dodAssessment/blocker.",
			)
		}
		if stateChangingSchemaInvalidRepair(failedStep) {
			input["schemaInvalidRepairMode"] = "single_valid_json_decision"
			input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
				"Previous repair output was rejected before execution because the LLM decision JSON was invalid; rebuild a single valid JSON object from repairActionPackage and current target snapshots.",
				"Do not copy any rawContentPreview or repairRawContentPreview from the rejected output; do not include markdown, comments, duplicate JSON objects, trailing braces, or partial arrays.",
				"If returning executionGraph, every node must be a complete JSON object and must follow repairActionPackage.requiredOutput acceptedKinds.",
			)
		}
	}
	if boolValue(input["validationRepairEscalatedToImplementation"]) {
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Repeated validation-only repair is no longer an acceptable next action for this coverage gap.",
			"If concrete target evidence is still missing, acquire bounded implementation target evidence first; otherwise produce a state-changing repair before the next validation.",
		)
	}
	preserveConcreteValidationSchemaInvalidRepairContract(input, failedStep)
	contract := BuildConvergenceContract(run, nil, failedStep, map[string]any{"input": input}, nil)
	input = ApplyConvergenceContract(input, contract)
	return input
}

func preserveConcreteValidationSchemaInvalidRepairContract(input map[string]any, failedStep *runModel.Step) bool {
	if len(input) == 0 || failedStep == nil || !stateChangingSchemaInvalidRepair(failedStep) || !inputRequiresConcreteValidationEvidence(failedStep.Input) {
		return false
	}
	inheritConcreteValidationRepairContract(input, failedStep)
	input["schemaInvalidRepairMode"] = "single_valid_json_decision"
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	pkg["schemaInvalidRecovery"] = map[string]any{
		"required": true,
		"reason":   "previous_llm_decision_json_invalid",
		"requirements": []string{
			"return exactly one valid JSON object",
			"do not reuse rejected raw output previews",
			"do not include markdown, comments, duplicate objects, trailing braces, or partial arrays",
			"make every executionGraph node a complete JSON object",
		},
	}
	required := concreteValidationEvidenceRequiredOutput(mapFromValue(pkg["requiredOutput"]))
	required["mustReturnValidJSON"] = true
	required["mustNotReuseRejectedJSON"] = true
	required["forbiddenKinds"] = compactUniqueStrings(append(stringSliceFromValue(required["forbiddenKinds"]),
		"malformed_json",
		"markdown_wrapped_json",
		"raw_rejected_json_reuse",
		"partial_json_array",
	))
	pkg["requiredOutput"] = required
	input["repairActionPackage"] = pkg
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"Previous validation-evidence repair output was rejected because the JSON shape was invalid; only fix the JSON/schema shape and preserve the concrete validation evidence contract.",
		"Do not downgrade to evidence acquisition, assessment-only, or state-changing patch output; return one valid JSON object whose executionGraph follows repairActionPackage.requiredOutput.",
	)
	return true
}

func runtimeRepairLastFailureDiagnostics(failedStep *runModel.Step) map[string]any {
	if failedStep == nil {
		return nil
	}
	if diagnostics := mapFromValue(failedStep.Output["lastFailureDiagnostics"]); len(diagnostics) > 0 {
		return compactLLMContextMap(diagnostics, 1)
	}
	ret := map[string]any{
		"stepId":           string(failedStep.ID),
		"stepKind":         failedStep.Kind,
		"failureClass":     strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeyClass)),
		"failureSignature": strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeySignature)),
		"stage":            strings.TrimSpace(stringFromMap(failedStep.Output, "stage")),
		"error": firstNonEmptyString(
			stringFromMap(failedStep.Output, "error"),
			stringFromMap(failedStep.Output, "reason"),
		),
	}
	if command := strings.TrimSpace(stringFromMap(failedStep.Output, "command")); command != "" {
		ret["command"] = command
	}
	if args := stringSliceFromValue(failedStep.Output["args"]); len(args) > 0 {
		ret["args"] = args
	}
	if exitCode, ok := failedStep.Output["exitCode"]; ok {
		ret["exitCode"] = exitCode
	}
	if stderr := strings.TrimSpace(stringFromMap(failedStep.Output, "stderr")); stderr != "" {
		ret["stderrPreview"] = truncateLLMContextString(stderr)
	}
	if stdout := strings.TrimSpace(stringFromMap(failedStep.Output, "stdout")); stdout != "" {
		ret["stdoutPreview"] = truncateLLMContextString(stdout)
	}
	for key, value := range ret {
		if strings.TrimSpace(stringFromAny(value)) == "" {
			delete(ret, key)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return compactLLMContextMap(ret, 1)
}

func runtimeRepairShouldReturnToRejectedResultContinuation(run *runModel.Run, failedStep *runModel.Step) bool {
	if run == nil || failedStep == nil || failedStep.Input == nil {
		return false
	}
	if len(mapFromValue(run.ExecutionContext["rejectedResultContinuation"])) == 0 {
		return false
	}
	failedInputContext := mapFromValue(failedStep.Input["executionContext"])
	if len(mapFromValue(failedInputContext["rejectedResultContinuation"])) == 0 {
		return false
	}
	replan := mapFromValue(failedInputContext["replanContinuation"])
	if len(replan) == 0 || !boolValue(replan["historicalOnly"]) {
		return false
	}
	if boolValue(failedStep.Input["replanStateChangingContinuation"]) {
		return true
	}
	replanTargets := repairPatchTargetRefsFromScopeText(firstNonEmptyString(
		stringFromMap(mapFromValue(replan["failureContext"]), "failureTargetScope"),
		stringFromMap(mapFromValue(replan["ticketContext"]), "failureTargetScope"),
		stringFromMap(mapFromValue(replan["failureContext"]), failureOutputKeySignature),
		stringFromMap(mapFromValue(replan["ticketContext"]), failureOutputKeySignature),
	))
	if len(replanTargets) == 0 {
		return false
	}
	failedTargets := compactUniqueStrings(append(failureTargetRepairRefs(failedStep), repairPatchTargetRefsFromScopeText(stringFromMap(failedStep.Output, "error"))...))
	if len(failedTargets) == 0 {
		return false
	}
	targetSet := map[string]bool{}
	for _, target := range replanTargets {
		target = normalizeRepairPatchTargetRef(target)
		if target != "" {
			targetSet[target] = true
		}
	}
	for _, target := range failedTargets {
		if targetSet[normalizeRepairPatchTargetRef(target)] {
			return true
		}
	}
	return false
}

func resetRuntimeRepairInputToRejectedResultContinuation(input map[string]any, run *runModel.Run, failedStep *runModel.Step) map[string]any {
	if input == nil {
		input = map[string]any{}
	}
	continuation := mapFromValue(run.ExecutionContext["rejectedResultContinuation"])
	timeline := processEvidenceTimelineFromRejectedResultContinuation(continuation)
	digest := currentProgressDigestFromRejectedResultContinuation(continuation, timeline)
	input[stepInputKeyProcessEvidenceTimeline] = compactProcessEvidenceTimelineForRuntimeContext(timeline)
	input[stepInputKeyCurrentProgressDigest] = digest
	input["currentExecutionProgress"] = compactLLMContextMap(digest, 0)
	if currentFailure := currentFailureFromProgressDigest(digest); len(currentFailure) > 0 {
		input["currentFailure"] = currentFailure
	}
	delete(input, "replanStateChangingContinuation")
	delete(input, "stateChangingOutputRequired")
	delete(input, "patchRepairContext")
	delete(input, "failureTargetRepairContext")
	delete(input, "targetRepairAttempts")
	delete(input, "strategyReassessment")
	input["repairObjective"] = repairObjectiveAcquireFailureEvidence
	input["concreteFailureEvidenceRequired"] = true
	input["historicalRepairAttempt"] = map[string]any{
		"stepId":       string(failedStep.ID),
		"kind":         failedStep.Kind,
		"state":        string(failedStep.State),
		"reason":       firstNonEmptyString(stringFromMap(failedStep.Output, "reason"), stringFromMap(failedStep.Output, "error")),
		"supersededBy": "rejectedResultContinuation",
	}
	pkg := map[string]any{
		"requiredOutput":              evidenceAcquisitionRequiredOutput(stateChangingRepairRequiredOutput()),
		"evidenceAcquisitionRequired": true,
		"currentExecutionProgress":    compactLLMContextMap(digest, 0),
	}
	if currentFailure := currentFailureFromProgressDigest(digest); len(currentFailure) > 0 {
		pkg["currentFailure"] = currentFailure
	}
	input["repairActionPackage"] = pkg
	input["repairGuidance"] = appendRepairGuidance(concreteFailureEvidenceAcquisitionGuidance(),
		"Rejected result continuation supersedes the historical replan failure; treat the historical repair attempt as lineage only.",
		"Acquire or produce the current evidence required by rejectedResultContinuation before choosing a new state-changing target.",
	)
	return input
}

func inheritPriorRepairContext(input map[string]any, failedStep *runModel.Step) {
	if input == nil || failedStep == nil || failedStep.Input == nil {
		return
	}
	inherit := func(key string) bool {
		if len(mapSliceFromValue(input[key])) > 0 {
			return false
		}
		context := mapSliceFromValue(failedStep.Input[key])
		if len(context) == 0 {
			return false
		}
		input[key] = context
		return true
	}
	inheritedTarget := inherit("failureTargetRepairContext")
	inheritedPatch := inherit("patchRepairContext")
	inheritedReadable := inherit("readableResourceRepairContext")
	inheritedManifest := inherit(stepInputKeyWorkspaceFileManifest)
	inheritedStrategy := false
	if len(mapFromValue(input["strategyReassessment"])) == 0 {
		if reassessment := mapFromValue(failedStep.Input["strategyReassessment"]); len(reassessment) > 0 {
			input["strategyReassessment"] = reassessment
			inheritedStrategy = true
		}
	}
	if len(mapFromValue(input["repairActionPackage"])) == 0 {
		if pkg := mapFromValue(failedStep.Input["repairActionPackage"]); len(pkg) > 0 {
			input["repairActionPackage"] = cloneStepPayload(pkg)
		}
	}
	inheritedRootCauseCandidates := inherit("rootCauseCandidateContext")
	if _, ok := input["targetRepairAttempts"]; !ok {
		if attempts := int64FromValue(failedStep.Input["targetRepairAttempts"]); attempts > 0 {
			input["targetRepairAttempts"] = attempts
		}
	}
	if strings.TrimSpace(stringFromMap(input, "failureTargetScope")) == "" {
		if scope := strings.TrimSpace(stringFromMap(failedStep.Input, "failureTargetScope")); scope != "" {
			input["failureTargetScope"] = scope
		}
	}
	if inheritedTarget || inheritedPatch || inheritedReadable || inheritedManifest {
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Preserve inherited repair context from the previous repair attempt; it contains the current target snapshots needed for direct state-changing recovery.",
			"Use inherited target snapshots and workspace manifest to produce accepted state-changing artifacts instead of retrying rejected undeclared tool or read-only recovery output.",
			"When a snapshot exposes contentHash, use that literal hash as baseHash or omit baseHash; do not emit template or interpolation syntax in patch fields.",
			"Do not generate file_write from inherited snapshots unless the snapshot is complete and not redacted; use exact minimal diffs or file/read when content is incomplete.",
		)
	}
	if inheritedStrategy || inheritedRootCauseCandidates {
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Preserve inherited strategy reassessment from the previous repair attempt; a runtime contract failure on the repair wrapper does not clear the original repeated failed-surface repair contract.",
			"Use inherited root-cause candidate context to choose the next repair target; do not patch the same failed surface again unless surfaceContractCorrection explains why that surface itself is the correction target.",
		)
	}
}

func inheritCoverageRepairContext(input map[string]any, failedStep *runModel.Step) bool {
	if input == nil || failedStep == nil {
		return false
	}
	inherited := false
	inheritSlice := func(key string) {
		if len(mapSliceFromValue(input[key])) > 0 {
			return
		}
		if values := mapSliceFromValue(failedStep.Input[key]); len(values) > 0 {
			input[key] = cloneMapSlice(values)
			inherited = true
			return
		}
		if values := mapSliceFromValue(failedStep.Output[key]); len(values) > 0 {
			input[key] = cloneMapSlice(values)
			inherited = true
			return
		}
		if failedOutput := mapFromValue(failedStep.Output["failedStepOutput"]); len(failedOutput) > 0 {
			if values := mapSliceFromValue(failedOutput[key]); len(values) > 0 {
				input[key] = cloneMapSlice(values)
				inherited = true
			}
		}
		if diagnostics := mapFromValue(failedStep.Output["runtimeDiagnostics"]); len(diagnostics) > 0 {
			if values := mapSliceFromValue(diagnostics[key]); len(values) > 0 {
				input[key] = cloneMapSlice(values)
				inherited = true
			}
		}
	}
	inheritSlice("missingDoDItems")
	inheritSlice("missingGoalCoverage")
	inheritSlice("missingDeliverableCoverage")
	if !inherited {
		return false
	}
	input["coverageRepairContextInherited"] = true
	input["coverageRepairRequired"] = true
	return true
}

func structuredFailureSignalsContainShapeConflict(signals map[string]any) bool {
	for _, key := range []string{"messages", "identifiers"} {
		for _, value := range stringSliceFromValue(signals[key]) {
			if textContainsShapeConflictDiagnostic(value) {
				return true
			}
		}
	}
	return false
}

func primaryDiagnosticRepairContractFromSignals(signals map[string]any) map[string]any {
	if len(signals) == 0 || !structuredFailureSignalsContainShapeConflict(signals) {
		return nil
	}
	lineRefs := mapSliceFromValue(signals["lineRefs"])
	targetRefs := primaryDiagnosticTargetRefs(signals, lineRefs)
	identifiers := stringSliceFromValue(signals["identifiers"])
	messages := stringSliceFromValue(signals["messages"])
	if len(lineRefs) == 0 && len(targetRefs) == 0 {
		return nil
	}
	ret := map[string]any{
		"required": true,
		"reason":   "line_or_shape_diagnostic_blocks_current_validation",
		"requirements": []string{
			"address line-referenced resource shape blockers before secondary changes",
			"patch the diagnostic target lines or declare a matching primaryDiagnosticResolution/addressesFailureSignals contract",
			"when patching a related target, declare relatedFailureTargets and reference the diagnostic identifiers or line refs being unblocked",
		},
	}
	if len(targetRefs) > 0 {
		ret["targetRefs"] = targetRefs
	}
	if len(lineRefs) > 0 {
		ret["lineRefs"] = lineRefs
	}
	if len(identifiers) > 0 {
		ret["identifiers"] = limitedStrings(compactUniqueStrings(identifiers), maxLLMContextSliceItems)
	}
	if len(messages) > 0 {
		ret["messages"] = limitedStrings(compactUniqueStrings(messages), 5)
	}
	return ret
}

func primaryDiagnosticTargetRefs(signals map[string]any, lineRefs []map[string]any) []string {
	seen := map[string]bool{}
	var ret []string
	add := func(value string) {
		value = normalizeStructuredFailureTargetRef(value)
		if value == "" || !runtimeRepairTargetRefAdmissible(value) || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, ref := range lineRefs {
		add(stringFromMap(ref, "targetRef"))
	}
	for _, value := range stringSliceFromValue(signals["targetRefs"]) {
		add(value)
	}
	return limitedStrings(ret, maxLLMContextSliceItems)
}

func textContainsShapeConflictDiagnostic(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return false
	}
	for _, token := range []string{
		"undefined:",
		"undefined ",
		"unknown field",
		"type mismatch",
		"cannot use",
		"missing field",
		"missing method",
		"does not implement",
		"unresolved",
		"unresolved reference",
		"no such field",
		"no such method",
		"schema key",
	} {
		if strings.Contains(normalized, token) {
			return true
		}
	}
	return false
}

func stateChangingRepairGuidance() []string {
	return []string{
		"Return concrete state-changing output with complete content, or return a failed dodAssessment if the required state change cannot be completed.",
		"If multiple dependent operations are needed, executionGraph must contain an auditable state-changing operation accepted by repairActionPackage.requiredOutput; patch/apply is preferred when it can represent the change.",
		"Do not return read-only, inspection-only, or LLM-gated executionGraph.",
		"patch/apply supports file_write, unified_diff, and text_replace; file_write content must be non-empty and must not be used as a delete operation.",
		"When changing an existing target file, prefer text_replace with exact oldText/newText or the smallest unified_diff over file_write; use file_write primarily to create missing targets or replace a known full file snapshot intentionally.",
		"unified_diff patches must include explicit hunk line ranges such as @@ -12,3 +12,4 @@; never emit a bare @@ hunk.",
		"Do not use shell/command or mcp/tool_call to modify workspace files when a patch/apply state-change artifact can represent the change; when acceptedKinds explicitly allows them, declare stateChanging=true plus targetRef, relatedFailureTargets, stateChangingReason, or boundedReason, then add bounded verification that depends on the operation.",
		"For scoped repair verification, shell/command input must declare targetRef, relatedFailureTargets, verificationScope, or boundedReason, or depend on a patch/apply node that directly targets the current failure scope.",
		"Any MCP tool call that synchronizes modified file content must include the complete current file content in input.content together with the target path; do not emit path-only, root-only, or partial sync requests.",
		"When failedStepOutput identifies concrete target resources, treat those target snapshots as authoritative and repair the target directly instead of changing unrelated prerequisite metadata.",
		"When repair target snapshots or failure target scope are present, patch/apply must target that failure scope/current repair snapshot, or explicitly declare relatedFailureTargets that cite the current failure target being validated or unblocked; unrelated marker/no-op patches are invalid.",
		"When a workspace already contains known resources, reconcile the repair with those existing resources and prefer the smallest coherent state change over broad replacement.",
		"When repair context contains redacted or truncated content, do not copy it into patch/apply; first read the target or produce a minimal exact diff from visible current evidence.",
		"When fixing failures caused by missing names, invalid attributes, or contract-entry mismatches, patch against names that already exist in current workspace resources unless the Goal explicitly requires creating a new named element.",
		"When failure diagnostics include concrete line refs or resource-shape mismatches, fix the reported execution/validation blockers in the failure target before secondary validation, formatting, or evidence refinements.",
		"Do not return self-inspection, run snapshot, observability, or context rehydration graphs when effective Goal and DoD are already present.",
		"If a target file is missing in an empty or new workspace, create it with patch/apply file_write instead of reading it again.",
		"Do not use placeholders such as __FROM_PREVIOUS_STEP_OUTPUT__, <<...>>, ${...}, or {{...}} in patch fields; use concrete literal values or omit optional fields.",
	}
}

func concreteFailureEvidenceAcquisitionGuidance() []string {
	return []string{
		"Acquire bounded concrete failure evidence before producing a patch.",
		"Use only targeted file/list, file/read, shell/command, mcp/tool_call, or equivalent concrete verification capabilities.",
		"Do not return patch/apply until current failure evidence identifies a concrete target, line ref, repair target set, coverage target, or current target snapshot.",
		"Do not use self-inspection, run snapshot, observability-only, context rehydration, or nested LLM graphs as evidence acquisition.",
		"If bounded verification produces target refs, line refs, stdout/stderr, exit code, or current snapshots, let the next repair use that evidence for target-scoped state-changing repair.",
	}
}

func repairStrategyReassessmentFromInput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	targetAttempts := int64FromValue(input["targetRepairAttempts"])
	repairAttempt := int64FromValue(input[stepInputKeyRuntimeRepairAttempt])
	targetScope := strings.TrimSpace(stringFromMap(input, "failureTargetScope"))
	failureClass := strings.TrimSpace(stringFromMap(input, failureOutputKeyClass))
	failureSignature := strings.TrimSpace(stringFromMap(input, failureOutputKeySignature))
	if targetAttempts < 2 && !(repairAttempt >= 3 && (targetScope != "" || failureClass != "" || failureSignature != "")) {
		return nil
	}
	reason := "repeated_repair_failure"
	if targetAttempts >= 2 {
		reason = "same_target_scope_repeated_failure"
	}
	failedSurface := repairStrategyFailedSurfaceFromInput(input)
	ret := map[string]any{
		"required":             true,
		"reason":               reason,
		"failedSurface":        failedSurface,
		"repairAttempt":        repairAttempt,
		"targetRepairAttempts": targetAttempts,
		"requiredDecisionFields": []string{
			"failedSurface",
			"rootCauseTarget",
			"repairTargetDecision",
			"repairMode",
			"forbiddenRepair",
			"verificationPlan",
		},
		"requirements": []string{
			"choose repairMode as continue_same_target_repair, shift_root_cause_target, or return blocker/failed assessment",
			"explain whether the next change targets state-change artifact, evidence, verification, or blocker handling",
			"when continuing the same target, address the current primary diagnostics using current target evidence and bounded verification",
			"when shifting root-cause target, use a neutral grounded candidate and relate it to the failed surface",
			"do not weaken still-valid Goal or DoD constraints to make verification pass",
		},
	}
	if failureClass != "" {
		ret["failureClass"] = failureClass
	}
	if targetScope != "" {
		ret["failureTargetScope"] = targetScope
	}
	return ret
}

func refreshRepairStrategyReassessment(input map[string]any) {
	if len(input) == 0 {
		return
	}
	refreshed := repairStrategyReassessmentFromInput(input)
	if len(refreshed) == 0 {
		return
	}
	current := mapFromValue(input["strategyReassessment"])
	if len(current) == 0 || repairStrategySurfaceMoreSpecific(refreshed, current) {
		input["strategyReassessment"] = refreshed
	}
}

func repairStrategyFailedSurfaceFromInput(input map[string]any) string {
	targetScope := strings.TrimSpace(stringFromMap(input, "failureTargetScope"))
	if targetScope != "" {
		return targetScope
	}
	targetRefs := repairStrategyTargetRefsFromInput(input)
	if len(targetRefs) == 1 {
		return "targetRef=" + targetRefs[0]
	}
	if len(targetRefs) > 1 {
		return "failureTargets=" + strings.Join(limitedStrings(targetRefs, maxLLMContextSliceItems), ",")
	}
	failureSignature := strings.TrimSpace(stringFromMap(input, failureOutputKeySignature))
	failureClass := strings.TrimSpace(stringFromMap(input, failureOutputKeyClass))
	return firstNonEmptyString(failureSignature, failureClass, strings.TrimSpace(stringFromMap(input, "failedStepKind")))
}

func repairStrategyTargetRefsFromInput(input map[string]any) []string {
	seen := map[string]bool{}
	var refs []string
	add := func(ref string) {
		ref = filepath.ToSlash(filepath.Clean(strings.TrimSpace(ref)))
		if ref == "" || ref == "." || seen[ref] {
			return
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	for _, ref := range stringSliceFromValue(input["repairTargetSet"]) {
		add(ref)
	}
	for _, key := range []string{"targetSnapshots", "currentTargetSnapshots", "failureTargetRepairContext", "patchRepairContext"} {
		for _, item := range mapSliceFromValue(input[key]) {
			add(stringFromMap(item, "targetRef"))
		}
	}
	return refs
}

func repairStrategySurfaceMoreSpecific(next map[string]any, current map[string]any) bool {
	nextSurface := strings.TrimSpace(stringFromMap(next, "failedSurface"))
	currentSurface := strings.TrimSpace(stringFromMap(current, "failedSurface"))
	if nextSurface == "" || nextSurface == currentSurface {
		return false
	}
	return len(strategySurfaceTargetRefs(nextSurface)) > 0 && len(strategySurfaceTargetRefs(currentSurface)) == 0
}

func concreteValidationEvidenceRepairGuidance() []string {
	return []string{
		"Do not repeat assessment-only validation.",
		"Produce concrete verification evidence for the failed Goal/DoD coverage before attempting state-changing repair.",
		"Return a bounded executionGraph that runs available verification capabilities, or return a failed dodAssessment/blocker if concrete evidence cannot be produced.",
		"If a dependency or environment prerequisite is unavailable, prefer a neutral local fixture, mock/stub, temporary config, isolated working directory, or alternate available port when that can validate the same contract without changing deliverables.",
		"Do not use read-only inspection, self-inspection, run snapshot, observability, or context rehydration graphs as evidence repair.",
		"If a verification command or tool later fails with concrete diagnostics, that failure may drive a separate state-changing repair.",
	}
}

func narrowStateChangingRecoveryInput(input map[string]any, failedStep *runModel.Step, execErr error, source string) {
	if input == nil || failedStep == nil || !boolValue(input["stateChangingOutputRequired"]) {
		return
	}
	class := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(input, failureOutputKeyClass),
		stringFromMap(failedStep.Output, failureOutputKeyClass),
	))
	stage := strings.TrimSpace(stringFromMap(failedStep.Output, "stage"))
	errorText := strings.Join([]string{
		errorString(execErr),
		stringFromMap(failedStep.Output, "error"),
		stringFromMap(failedStep.Output, "reason"),
	}, "\n")
	if class != failureClassProviderTimeout && !stateChangingReadOnlyGraphFailureText(errorText) {
		return
	}
	input["stateChangingRecoveryMode"] = "direct_state_change"
	input["stateChangingRecoverySource"] = strings.TrimSpace(source)
	input["providerTimeoutStateChangingRetry"] = class == failureClassProviderTimeout || stage == "llm_provider_timeout"
	input["forbiddenRecoveryOutputs"] = []string{
		"read_only_graph",
		"self_inspection_graph",
		"run_snapshot_graph",
		"observability_graph",
		"context_rehydration_graph",
		"llm_gated_replan_graph",
	}
	input["requiredRecoveryOutputs"] = []string{
		"state_changing_artifact",
		"failed_dod_assessment",
		"real_decision_or_approval_blocker",
	}
	input["failedStepOutput"] = compactStateChangingRecoveryFailureOutput(failedStep.Output)
	delete(input, "previousStepOutput")
	delete(input, "failedExecutionGraph")
	delete(input, stepInputKeyGraphEvidence)
	delete(input, "runEvidenceSummary")
	if strings.TrimSpace(source) == "provider_retry" {
		delete(input, stepInputKeyWorkspaceFileManifest)
	}
	delete(input, stepOutputKeyExecutionGraph)
}

func repairActionPackageFromRepairInput(input map[string]any, rawFailedOutput ...map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	enrichRepairInputWithProgressFailureSignals(input)
	refreshRepairStrategyReassessment(input)
	priorPackage := mapFromValue(input["repairActionPackage"])
	requiredOutput := stateChangingRepairRequiredOutput()
	pkg := map[string]any{
		"requiredOutput": requiredOutput,
	}
	mergePriorRepairActionPackageEvidence(pkg, requiredOutput, priorPackage)
	if currentFailure := repairActionCurrentFailure(input, rawFailedOutput...); len(currentFailure) > 0 {
		pkg["currentFailure"] = currentFailure
	}
	if diagnostics := repairActionLastFailureDiagnostics(input, rawFailedOutput...); len(diagnostics) > 0 {
		pkg["lastFailureDiagnostics"] = diagnostics
	}
	for _, key := range []string{
		failureOutputKeyClass,
		failureOutputKeySignature,
		"failureTargetScope",
		"failedStepId",
		"failedStepKind",
		"failureReason",
		"repairObjective",
		"runtimeRepairAttempt",
		"maxRuntimeRepairAttempts",
		"targetRepairAttempts",
		"strategyReassessment",
	} {
		if value, ok := input[key]; ok && value != nil {
			pkg[key] = compactLLMContextValue(value, 1)
		}
	}
	if len(rawFailedOutput) > 0 && len(rawFailedOutput[0]) > 0 {
		pkg["previousRejectedOutputSummary"] = compactStateChangingRecoveryFailureOutput(rawFailedOutput[0])
	} else if failedOutput := mapFromValue(input["failedStepOutput"]); len(failedOutput) > 0 {
		pkg["previousRejectedOutputSummary"] = compactStateChangingRecoveryFailureOutput(failedOutput)
	}
	if signals := mapFromValue(input["structuredFailureSignals"]); len(signals) > 0 {
		pkg["structuredFailureSignals"] = compactStructuredFailureSignalsForRepairActionPackage(signals)
	} else if len(rawFailedOutput) > 0 && len(rawFailedOutput[0]) > 0 {
		if signals := structuredFailureSignalsFromFailureOutput(rawFailedOutput[0], nil, "", ""); len(signals) > 0 {
			pkg["structuredFailureSignals"] = compactStructuredFailureSignalsForRepairActionPackage(signals)
		}
	}
	if primary := mapFromValue(input["primaryDiagnosticRepair"]); len(primary) > 0 {
		pkg["primaryDiagnosticRepair"] = compactLLMContextMap(primary, 1)
		requiredOutput["mustAddressPrimaryDiagnostics"] = true
		requiredOutput["primaryDiagnosticMatchRule"] = "state-changing patch/apply must address line-referenced target diagnostics, or declare matching primaryDiagnosticResolution/addressesFailureSignals with relatedFailureTargets for a related target"
	} else if signals := mapFromValue(pkg["structuredFailureSignals"]); len(signals) > 0 {
		if primary := primaryDiagnosticRepairContractFromSignals(signals); len(primary) > 0 {
			pkg["primaryDiagnosticRepair"] = compactLLMContextMap(primary, 1)
			requiredOutput["mustAddressPrimaryDiagnostics"] = true
			requiredOutput["primaryDiagnosticMatchRule"] = "state-changing patch/apply must address line-referenced target diagnostics, or declare matching primaryDiagnosticResolution/addressesFailureSignals with relatedFailureTargets for a related target"
		}
	}
	if forbidden := forbiddenRepairContentReuseFromFailure(input, rawFailedOutput...); len(forbidden) > 0 {
		pkg["forbiddenContentReuse"] = forbidden
		requiredOutput["mustRebuildFromCurrentEvidence"] = true
	}
	if targets := repairActionTargetSnapshots(input); len(targets) > 0 {
		pkg["targetSnapshots"] = targets
		pkg["currentTargetSnapshots"] = repairActionTargetSnapshotMetadata(targets)
		requiredOutput["mustTargetFailureScope"] = true
		requiredOutput["targetRelation"] = "failure_target_repair_snapshot_or_declared_related_failure_target"
		requiredOutput["mustUseCurrentTargetSnapshot"] = true
		requiredOutput["forbiddenStalePatchReuse"] = true
		requiredOutput["fileWriteExistingTargetRule"] = "only allowed when the target snapshot is complete/safe and patch/apply file_write includes baseHash equal to that snapshot contentHash; otherwise use patch/apply text_replace with exact oldText/newText/baseHash, bounded unified_diff, or file/edit"
	}
	if candidates := mapSliceFromValue(input["rootCauseCandidateContext"]); len(candidates) > 0 {
		pkg["rootCauseCandidateContext"] = compactLLMContextValue(candidates, 1)
		requiredOutput["rootCauseCandidateRule"] = "a grounded rootCauseCandidateContext target may be patched when the patch declares relatedFailureTargets for the failed surface or the candidate is included in repairTargetSet"
	}
	if targetSet := repairActionTargetSet(input, pkg, rawFailedOutput...); len(targetSet) > 0 {
		pkg["repairTargetSet"] = targetSet
		requiredOutput["mustTargetRepairTargetSet"] = true
	}
	if coverageTargets := coverageTargetSetFromRepairInput(input); len(coverageTargets) > 0 {
		pkg["coverageRepairRequired"] = true
		pkg["coverageTargetSet"] = coverageTargets
		requiredOutput["mustDeclareCoverageTargets"] = true
		requiredOutput["coverageTargetMatchRule"] = "patch/apply coverageTargets must match one unresolved DoD item text, goalRequirementId, goal item, or deliverable item from coverageTargetSet"
	}
	if progress := mapFromValue(input["currentExecutionProgress"]); len(progress) > 0 {
		pkg["currentExecutionProgress"] = compactLLMContextMap(progress, 0)
	}
	if digest := failedPatchDigestFromRepairFailure(input, rawFailedOutput...); len(digest) > 0 {
		pkg["failedPatchDigest"] = digest
	}
	if satisfied := mapSliceFromValue(input["currentTargetAlreadySatisfiesFailedPatch"]); len(satisfied) > 0 {
		pkg["currentTargetAlreadySatisfiesFailedPatch"] = satisfied
		pkg["stateChangeAlreadySatisfiedByCurrentTarget"] = true
		requiredOutput["mustIncludeStateChangingNode"] = false
		requiredOutput["mustProduceConcreteValidationEvidence"] = true
		requiredOutput["acceptedKinds"] = []string{"shell/command", "mcp/tool_call"}
		requiredOutput["forbiddenKinds"] = append(stringSliceFromValue(requiredOutput["forbiddenKinds"]), "repeat_same_failed_patch")
	}
	forbiddenActions := repairActionForbiddenActions(input, pkg)
	if len(forbiddenActions) > 0 {
		pkg["forbiddenActions"] = forbiddenActions
	}
	if mode := repairActionRequiredRepairMode(input, pkg, rawFailedOutput...); mode != "" {
		pkg["requiredRepairMode"] = mode
	}
	if contract := mapFromValue(input["capabilityPolicyRepairContract"]); len(contract) > 0 {
		pkg["capabilityPolicyRepairRequired"] = true
		pkg["capabilityPolicyRepairContract"] = compactLLMContextMap(contract, 1)
		requiredOutput["mustRepairCapabilityPolicyFollowUp"] = true
		requiredOutput["capabilityPolicyFollowUpRule"] = "when an execution graph includes a trigger kind from capabilityPolicyRepairContract.triggeredPolicies.requiredAfterKinds, it must include a dependent mcp/tool_call matching preferredTools or output a structured capabilityPolicySkips item with policyId/category and non-empty reason"
		requiredOutput["acceptedKinds"] = compactUniqueStrings(append(stringSliceFromValue(requiredOutput["acceptedKinds"]), "mcp/tool_call"))
		requiredOutput["forbiddenKinds"] = compactUniqueStrings(append(stringSliceFromValue(requiredOutput["forbiddenKinds"]), "assessment_only_capability_policy_repair"))
	}
	if budget := repairActionBudgetState(input, pkg); len(budget) > 0 {
		pkg["repairBudget"] = budget
	}
	if repairActionIsSchemaInvalidRepair(input, rawFailedOutput...) {
		pkg["schemaInvalidRecovery"] = map[string]any{
			"required": true,
			"reason":   "previous_llm_decision_json_invalid",
			"requirements": []string{
				"return exactly one valid JSON object",
				"do not reuse rejected raw output previews",
				"do not include markdown, comments, duplicate objects, trailing braces, or partial arrays",
				"make every executionGraph node a complete JSON object",
			},
		}
		requiredOutput["mustReturnValidJSON"] = true
		requiredOutput["mustNotReuseRejectedJSON"] = true
		requiredOutput["forbiddenKinds"] = append(stringSliceFromValue(requiredOutput["forbiddenKinds"]),
			"malformed_json",
			"markdown_wrapped_json",
			"raw_rejected_json_reuse",
			"partial_json_array",
		)
	}
	pkg["exitCriteria"] = []string{
		"state_changing_artifact_applied_then_verify",
		"failed_assessment_or_blocker_when_not_repairable",
	}
	if manifest := repairActionWorkspaceManifest(input); len(manifest) > 0 {
		pkg["workspaceFileManifest"] = manifest
	}
	if diagnostics := repairActionTargetRelevanceDiagnostics(input, pkg); len(diagnostics) > 0 {
		pkg["targetRelevanceDiagnostics"] = diagnostics
	}
	if int64FromValue(input["targetRepairAttempts"]) >= 2 {
		pkg["repeatedTargetRepairMode"] = map[string]any{
			"enabled":                           true,
			"reason":                            "same_target_scope_repeated_failure",
			"maxTargetScopedRepairInputBytes":   120 * 1024,
			"mustPreferExactTargetSnapshot":     true,
			"mustOmitRejectedRawFailureProduct": true,
			"mustReassessRepeatedFailedSurface": true,
		}
		requiredOutput["mustUseCurrentTargetSnapshot"] = true
		requiredOutput["mustRebuildFromCurrentEvidence"] = true
		if len(mapFromValue(pkg["strategyReassessment"])) == 0 {
			if reassessment := repairStrategyReassessmentFromInput(input); len(reassessment) > 0 {
				pkg["strategyReassessment"] = reassessment
			}
		}
	}
	applyConcreteRepairEvidenceOutputContract(input, pkg, requiredOutput)
	addStrategyReassessmentRequiredOutputContract(pkg, requiredOutput)
	if len(pkg) <= 1 {
		return nil
	}
	return pkg
}

func mergePriorRepairActionPackageEvidence(pkg map[string]any, requiredOutput map[string]any, prior map[string]any) {
	if len(pkg) == 0 || len(prior) == 0 {
		return
	}
	for _, key := range []string{
		"targetSnapshots",
		"currentTargetSnapshots",
		"repairTargetSet",
		"coverageTargetSet",
		"structuredFailureSignals",
		"primaryDiagnosticRepair",
		"failureTargetRepairContext",
		"patchRepairContext",
		"currentTargetAlreadySatisfiesFailedPatch",
		"stateChangeAlreadySatisfiedByCurrentTarget",
	} {
		if _, ok := pkg[key]; ok || prior[key] == nil {
			continue
		}
		pkg[key] = compactLLMContextValue(prior[key], 1)
	}
	if len(mapSliceFromValue(pkg["targetSnapshots"])) > 0 {
		requiredOutput["mustTargetFailureScope"] = true
		requiredOutput["mustUseCurrentTargetSnapshot"] = true
		requiredOutput["forbiddenStalePatchReuse"] = true
	}
	if len(stringSliceFromValue(pkg["repairTargetSet"])) > 0 {
		requiredOutput["mustTargetRepairTargetSet"] = true
	}
	if len(mapSliceFromValue(pkg["coverageTargetSet"])) > 0 {
		requiredOutput["mustDeclareCoverageTargets"] = true
	}
}

func stateChangingRepairRequiredOutput() map[string]any {
	return map[string]any{
		"mustIncludeStateChangingNode": true,
		"acceptedKinds":                stateChangingRepairAcceptedKinds(),
		"forbiddenKinds": []string{
			"read_only_graph",
			"llm_gated_graph",
			"self_inspection_graph",
			"context_rehydration_graph",
		},
	}
}

func stateChangingRepairAcceptedKinds() []string {
	return []string{"patch/apply", "shell/command", "mcp/tool_call"}
}

func capabilityPolicyRepairContractFromFailure(run *runModel.Run, failedStep *runModel.Step, execErr error) map[string]any {
	if run == nil || failedStep == nil {
		return nil
	}
	failureText := strings.ToLower(strings.Join([]string{
		errorString(execErr),
		stringFromMap(failedStep.Output, "error"),
		stringFromMap(failedStep.Output, "reason"),
		stringFromMap(failedStep.Output, failureOutputKeySignature),
	}, "\n"))
	if !strings.Contains(failureText, "capability usage policy") {
		return nil
	}
	policies := capabilityUsagePoliciesForStep(run, failedStep)
	if len(policies) == 0 {
		return nil
	}
	triggered := make([]map[string]any, 0, len(policies))
	for _, policy := range policies {
		policyID := strings.TrimSpace(stringFromMap(policy, "id"))
		if policyID != "" && !strings.Contains(failureText, strings.ToLower(policyID)) {
			continue
		}
		item := map[string]any{}
		for _, key := range []string{
			"id",
			"appliesToCategory",
			"usageMode",
			"triggerStages",
			"preferredTools",
			"requiredAfterKinds",
			"fallbackPolicy",
			"evidenceStrength",
		} {
			if value, ok := policy[key]; ok && value != nil {
				item[key] = compactLLMContextValue(value, 1)
			}
		}
		if len(item) > 0 {
			triggered = append(triggered, item)
		}
	}
	if len(triggered) == 0 {
		return nil
	}
	contract := map[string]any{
		"triggeredPolicies": triggered,
		"requiredOutput": map[string]any{
			"mustReturnCorrectedExecutionGraph": true,
			"mustUseDependentFollowUpOrSkip":    true,
			"acceptedKinds":                     []string{"mcp/tool_call", "patch/apply", "shell/command"},
			"skipField":                         "capabilityPolicySkips",
		},
		"repairRule":              "for each triggered policy, any graph containing requiredAfterKinds must include a dependent mcp/tool_call matching preferredTools, or declare capabilityPolicySkips with policyId/category and non-empty reason",
		"assessmentOnlyForbidden": true,
	}
	if graph, ok := runtimeGraphSpecFromStep(failedStep); ok {
		contract["rejectedExecutionGraph"] = rejectedGraphSummary(graph)
	}
	return contract
}

func evidenceAcquisitionRequiredOutput(existing map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range existing {
		switch key {
		case "mustReturnValidJSON", "mustNotReuseRejectedJSON", "forbiddenStalePatchReuse":
			out[key] = value
		}
	}
	out["mustIncludeStateChangingNode"] = false
	out["mustProduceConcreteFailureEvidence"] = true
	out["acceptedKinds"] = evidenceAcquisitionAcceptedKinds()
	out["forbiddenKinds"] = compactUniqueStrings(append(stringSliceFromValue(existing["forbiddenKinds"]),
		"patch/apply",
		"llm_gated_graph",
		"self_inspection_graph",
		"context_rehydration_graph",
		"run_snapshot_graph",
		"observability_only_graph",
	))
	out["evidenceAcquisitionRule"] = "bounded evidence acquisition is required because no concrete state-changing repair target evidence is available"
	return out
}

func evidenceAcquisitionAcceptedKinds() []string {
	kinds := []string{"file/list", "file/read", "shell/command", "mcp/tool_call"}
	cfg, err := workorchconfig.Current()
	if err != nil || workorchconfig.GitRuntimeEnabled(cfg.Runtime.Git) {
		kinds = append(kinds, gitcommon.StepKindRead)
	}
	return kinds
}

func concreteValidationEvidenceRequiredOutput(existing map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range existing {
		switch key {
		case "mustReturnValidJSON", "mustNotReuseRejectedJSON":
			out[key] = value
		}
	}
	out["mustIncludeStateChangingNode"] = false
	out["mustProduceConcreteValidationEvidence"] = true
	out["mustPreferExecutableBehaviorValidation"] = true
	out["staticInspectionOnlyMayNotSatisfyRuntimeBehavior"] = true
	out["mustAttemptControlledSubstituteWhenPrerequisiteUnavailable"] = true
	out["mustDeclareValidationEvidenceMode"] = true
	out["mustDeclareValidationEvidenceObservation"] = true
	out["acceptedKinds"] = []string{"shell/command", "mcp/tool_call"}
	out["forbiddenKinds"] = compactUniqueStrings(append(stringSliceFromValue(existing["forbiddenKinds"]),
		"patch/apply",
		"read_only_graph",
		"llm_gated_graph",
		"self_inspection_graph",
		"context_rehydration_graph",
		"run_snapshot_graph",
		"observability_only_graph",
	))
	out["validationEvidenceRule"] = "bounded validation evidence is required for unresolved Goal/DoD/deliverable coverage; patch/apply is only valid after separate concrete implementation target evidence appears"
	out["controlledSubstituteRule"] = "when a dependency or environment prerequisite is unavailable, first try a neutral local fixture, mock/stub, temporary config, isolated working directory, or alternate available port through accepted validation tools; static inspection alone does not close runtime-behavior coverage"
	out["validationEvidenceModeRule"] = "each shell/command or mcp/tool_call node intended to close runtime-behavior coverage must declare input.validationEvidence.mode as runtime_behavior, controlled_substitute, executable_contract, or dynamic_execution; static_inspection/source_inspection metadata cannot close runtime-behavior coverage"
	out["validationEvidenceObservationRule"] = "each concrete validation node must also declare input.validationEvidence.observationType as runtime_observation, controlled_substitute_observation, executable_contract_observation, or dynamic_execution_observation; controlled_substitute must declare validationEvidence.substituteFor or validationEvidence.substitute"
	return out
}

func applyConcreteRepairEvidenceOutputContract(input map[string]any, pkg map[string]any, requiredOutput map[string]any) {
	if len(pkg) == 0 || len(requiredOutput) == 0 {
		return
	}
	if !boolValue(requiredOutput["mustIncludeStateChangingNode"]) {
		return
	}
	if boolValue(pkg["capabilityPolicyRepairRequired"]) {
		requiredOutput["mustIncludeStateChangingNode"] = true
		requiredOutput["mustRepairCapabilityPolicyFollowUp"] = true
		requiredOutput["acceptedKinds"] = compactUniqueStrings(append(stringSliceFromValue(requiredOutput["acceptedKinds"]), "mcp/tool_call"))
		return
	}
	if stateChangingRepairHasConcreteTargetEvidence(input, pkg) {
		requiredOutput["mustIncludeStateChangingNode"] = true
		acceptedKinds := compactUniqueStrings(append(stringSliceFromValue(requiredOutput["acceptedKinds"]), stateChangingRepairAcceptedKinds()...))
		requiredOutput["acceptedKinds"] = acceptedKinds
		return
	}
	next := evidenceAcquisitionRequiredOutput(requiredOutput)
	for key := range requiredOutput {
		delete(requiredOutput, key)
	}
	for key, value := range next {
		requiredOutput[key] = value
	}
	pkg["evidenceAcquisitionRequired"] = true
	pkg["evidenceAcquisitionReason"] = "state_changing_repair_target_evidence_missing"
	pkg["exitCriteria"] = []string{
		"concrete_failure_evidence_acquired_then_target_scoped_repair",
		"failed_assessment_or_blocker_when_evidence_cannot_be_acquired",
	}
}

func stateChangingRepairHasConcreteTargetEvidence(input map[string]any, pkg map[string]any) bool {
	if len(pkg) > 0 && boolValue(pkg["capabilityPolicyRepairRequired"]) {
		return true
	}
	return repairHasImplementationTargetEvidence(input, pkg)
}

func repairHasImplementationTargetEvidence(input map[string]any, pkg map[string]any) bool {
	requireCurrentSnapshot := boolValue(input["requiresCurrentTargetSnapshotForStateChange"]) ||
		boolValue(pkg["requiresCurrentTargetSnapshotForStateChange"])
	if len(input) > 0 {
		for _, key := range []string{"failureTargetRepairContext", "patchRepairContext", "rootCauseCandidateContext"} {
			if repairContextHasAdmissibleTargetEvidence(input[key]) {
				return true
			}
		}
		if len(repairActionTargetSnapshots(input)) > 0 {
			return true
		}
		if !requireCurrentSnapshot {
			if signals := mapFromValue(input["structuredFailureSignals"]); len(signals) > 0 && structuredFailureSignalsHaveConcreteTargetEvidence(signals) {
				return true
			}
			if progress := mapFromValue(input["currentExecutionProgress"]); progressHasConcreteTargetEvidence(progress) {
				return true
			}
		}
	}
	if len(pkg) > 0 {
		if repairContextHasAdmissibleTargetEvidence(pkg["targetSnapshots"]) {
			return true
		}
		if !requireCurrentSnapshot {
			if len(filterAdmissibleRuntimeRepairTargetRefs(stringSliceFromValue(pkg["repairTargetSet"]))) > 0 {
				return true
			}
			if primary := mapFromValue(pkg["primaryDiagnosticRepair"]); len(primary) > 0 && structuredFailureSignalsHaveConcreteTargetEvidence(primary) {
				return true
			}
			if signals := mapFromValue(pkg["structuredFailureSignals"]); len(signals) > 0 && structuredFailureSignalsHaveConcreteTargetEvidence(signals) {
				return true
			}
			if progress := mapFromValue(pkg["currentExecutionProgress"]); progressHasConcreteTargetEvidence(progress) {
				return true
			}
		}
	}
	return false
}

func repairContextHasAdmissibleTargetEvidence(value any) bool {
	for _, item := range mapSliceFromValue(value) {
		if runtimeRepairTargetRefAdmissible(stringFromMap(item, "targetRef")) && !boolValue(item["isBinary"]) {
			return true
		}
	}
	return false
}

func sanitizeRepairActionPackageTargetEvidence(pkg map[string]any) {
	if len(pkg) == 0 {
		return
	}
	if targets, ok := pkg["repairTargetSet"]; ok {
		if filtered := filterAdmissibleRuntimeRepairTargetRefs(stringSliceFromValue(targets)); len(filtered) > 0 {
			pkg["repairTargetSet"] = filtered
		} else {
			delete(pkg, "repairTargetSet")
		}
	}
	for _, key := range []string{
		"targetSnapshots",
		"currentTargetSnapshots",
		"failureTargetRepairContext",
		"patchRepairContext",
		"rootCauseCandidateContext",
	} {
		items := mapSliceFromValue(pkg[key])
		if len(items) == 0 {
			continue
		}
		filtered := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if runtimeRepairTargetRefAdmissible(stringFromMap(item, "targetRef")) && !boolValue(item["isBinary"]) {
				filtered = append(filtered, item)
			}
		}
		if len(filtered) > 0 {
			pkg[key] = filtered
		} else {
			delete(pkg, key)
		}
	}
	if signals := mapFromValue(pkg["structuredFailureSignals"]); len(signals) > 0 {
		if compact := compactStructuredFailureSignalsForRepairActionPackage(signals); len(compact) > 0 {
			pkg["structuredFailureSignals"] = compact
		} else {
			delete(pkg, "structuredFailureSignals")
		}
	}
}

func enrichRepairInputWithProgressFailureSignals(input map[string]any) {
	if len(input) == 0 {
		return
	}
	progress := mapFromValue(input["currentExecutionProgress"])
	if len(progress) == 0 {
		if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
			progress = mapFromValue(pkg["currentExecutionProgress"])
		}
	}
	signals := structuredFailureSignalsFromProgressEvidence(progress)
	if len(signals) == 0 {
		return
	}
	input["structuredFailureSignals"] = mergeStructuredFailureSignals(mapFromValue(input["structuredFailureSignals"]), signals)
	if targetSet := filterAdmissibleRuntimeRepairTargetRefs(stringSliceFromValue(signals["targetRefs"])); len(targetSet) > 0 {
		if existing := stringSliceFromValue(input["repairTargetSet"]); len(existing) > 0 {
			targetSet = append(existing, targetSet...)
		}
		input["repairTargetSet"] = limitedStrings(compactUniqueStrings(targetSet), maxLLMContextSliceItems)
	}
	if strings.TrimSpace(stringFromMap(input, "failureTargetScope")) == "" {
		if targetRefs := stringSliceFromValue(signals["targetRefs"]); len(targetRefs) > 0 {
			input["failureTargetScope"] = "failureTargets=" + strings.Join(limitedStrings(targetRefs, maxLLMContextSliceItems), ",")
		}
	}
}

func progressHasConcreteTargetEvidence(progress map[string]any) bool {
	return len(progress) > 0 && structuredFailureSignalsHaveConcreteTargetEvidence(structuredFailureSignalsFromProgressEvidence(progress))
}

func structuredFailureSignalsFromProgressEvidence(progress map[string]any) map[string]any {
	if len(progress) == 0 {
		return nil
	}
	merged := map[string]any{}
	add := func(output map[string]any) {
		if len(output) == 0 {
			return
		}
		signals := structuredFailureSignalsFromFailureOutput(output, nil, "", "")
		if len(signals) == 0 {
			return
		}
		merged = mergeStructuredFailureSignals(merged, signals)
	}
	for _, key := range []string{"latestActionEvidence", "completedGraphEvidence"} {
		for _, item := range mapSliceFromValue(progress[key]) {
			output := mapFromValue(item["outputSummary"])
			if len(output) == 0 {
				output = mapFromValue(item["output"])
			}
			add(output)
		}
	}
	add(map[string]any{
		"error":                   stringFromMap(progress, "failureReason"),
		failureOutputKeyClass:     stringFromMap(progress, "failureClass"),
		failureOutputKeySignature: stringFromMap(progress, failureOutputKeySignature),
	})
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func structuredFailureSignalsHaveConcreteTargetEvidence(signals map[string]any) bool {
	if len(signals) == 0 {
		return false
	}
	for _, ref := range stringSliceFromValue(signals["targetRefs"]) {
		if structuredFailureTextTargetRefAllowed(ref) {
			return true
		}
	}
	for _, ref := range mapSliceFromValue(signals["lineRefs"]) {
		if structuredFailureTextTargetRefAllowed(stringFromMap(ref, "targetRef")) {
			return true
		}
	}
	return false
}

func repairActionPackageRequiresEvidenceAcquisition(pkg map[string]any) bool {
	return len(pkg) > 0 && boolValue(pkg["evidenceAcquisitionRequired"])
}

func repairActionPackageRequiresStateChangingOutput(pkg map[string]any) bool {
	if len(pkg) == 0 {
		return false
	}
	return boolValue(mapFromValue(pkg["requiredOutput"])["mustIncludeStateChangingNode"])
}

func addStrategyReassessmentRequiredOutputContract(pkg map[string]any, requiredOutput map[string]any) {
	if len(pkg) == 0 || len(requiredOutput) == 0 {
		return
	}
	if !boolValue(requiredOutput["mustIncludeStateChangingNode"]) {
		return
	}
	reassessment := mapFromValue(pkg["strategyReassessment"])
	if !boolValue(reassessment["required"]) {
		return
	}
	requiredFields := stringSliceFromValue(reassessment["requiredDecisionFields"])
	if len(requiredFields) == 0 {
		requiredFields = []string{
			"failedSurface",
			"rootCauseTarget",
			"repairTargetDecision",
			"repairMode",
			"forbiddenRepair",
			"verificationPlan",
		}
	}
	requiredOutput["mustDeclareStrategyReassessmentFields"] = true
	requiredOutput["strategyReassessmentRequiredFields"] = requiredFields
	requiredOutput["strategyReassessmentRepairModes"] = []string{"continue_same_target_repair", "shift_root_cause_target"}
	requiredOutput["strategyReassessmentMatchRule"] = "each state-changing patch/apply node must declare these fields inside the patch/apply input object, not as top-level decision fields or node sibling fields; failedSurface must align with strategyReassessment.failedSurface, repairMode must be explicit, and rootCauseTarget must identify the resource or scope being changed"
	if strategyReassessmentRequiresRootCauseTargetShift(reassessment) {
		requiredOutput["mustReassessRepeatedFailedSurface"] = true
		requiredOutput["sameTargetContinuationRule"] = "repairMode=continue_same_target_repair may patch the failed target only when it addresses current primary diagnostics with current target evidence and bounded verification"
		requiredOutput["rootCauseTargetShiftRule"] = "repairMode=shift_root_cause_target must patch a distinct neutral grounded root-cause target or return a failed assessment/blocker"
	}
}

func compactStructuredFailureSignalsForRepairActionPackage(signals map[string]any) map[string]any {
	if len(signals) == 0 {
		return nil
	}
	ret := compactLLMContextMap(signals, 0)
	targetRefs := stringSliceFromValue(ret["targetRefs"])
	var filtered []string
	for _, ref := range targetRefs {
		ref = normalizeStructuredFailureTargetRef(ref)
		if ref == "" || !structuredFailureTextTargetRefAllowed(ref) {
			continue
		}
		filtered = append(filtered, ref)
	}
	filtered = limitedStrings(compactUniqueStrings(filtered), maxLLMContextSliceItems)
	if len(filtered) > 0 {
		ret["targetRefs"] = filtered
	} else {
		delete(ret, "targetRefs")
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func repairActionCurrentFailure(input map[string]any, rawFailedOutput ...map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	output := firstRawFailureOutput(input, rawFailedOutput...)
	ret := map[string]any{}
	add := func(key string, value string) {
		if value = strings.TrimSpace(value); value != "" {
			ret[key] = value
		}
	}
	add("failureClass", firstNonEmptyString(stringFromMap(input, failureOutputKeyClass), stringFromMap(output, failureOutputKeyClass)))
	add("failureSignature", firstNonEmptyString(stringFromMap(input, failureOutputKeySignature), stringFromMap(output, failureOutputKeySignature)))
	add("failedStepId", stringFromMap(input, "failedStepId"))
	add("failedStepKind", stringFromMap(input, "failedStepKind"))
	add("stage", stringFromMap(output, "stage"))
	add("targetRef", firstNonEmptyString(
		stringFromMap(output, "targetRef"),
		stringFromMap(output, "path"),
		repairActionTargetRefFromScope(stringFromMap(input, "failureTargetScope")),
	))
	add("targetScope", stringFromMap(input, "failureTargetScope"))
	add("error", firstNonEmptyString(stringFromMap(output, "error"), stringFromMap(input, "failureReason")))
	add("reason", firstNonEmptyString(stringFromMap(output, "reason"), stringFromMap(input, "failureReason")))
	if diagnostics := repairActionLastFailureDiagnostics(input, rawFailedOutput...); len(diagnostics) > 0 {
		add("stderrPreview", stringFromMap(diagnostics, "stderrPreview"))
		add("stdoutPreview", stringFromMap(diagnostics, "stdoutPreview"))
	}
	if diagnostics := mapFromValue(output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		add("diagnosticKind", stringFromMap(diagnostics, "kind"))
		add("diagnosticStage", stringFromMap(diagnostics, "stage"))
		add("diagnosticErrorDetail", stringFromMap(diagnostics, "errorDetail"))
		if _, ok := ret["targetRef"]; !ok {
			add("targetRef", stringFromMap(diagnostics, "targetRef"))
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return compactLLMContextMap(ret, 1)
}

func repairActionLastFailureDiagnostics(input map[string]any, rawFailedOutput ...map[string]any) map[string]any {
	if len(input) == 0 && len(rawFailedOutput) == 0 {
		return nil
	}
	if diagnostics := mapFromValue(input["lastFailureDiagnostics"]); len(diagnostics) > 0 {
		return compactFailureStepOutputForRuntimeContext(diagnostics)
	}
	output := firstRawFailureOutput(input, rawFailedOutput...)
	if len(output) == 0 {
		return nil
	}
	if diagnostics := mapFromValue(output["lastFailureDiagnostics"]); len(diagnostics) > 0 {
		return compactFailureStepOutputForRuntimeContext(diagnostics)
	}
	compact := compactFailureStepOutputForRuntimeContext(output)
	if len(compact) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"error",
		"reason",
		"stage",
		"command",
		"args",
		"exitCode",
		failureOutputKeyClass,
		failureOutputKeySignature,
		"stdoutPreview",
		"stderrPreview",
	} {
		if value, ok := compact[key]; ok && value != nil {
			ret[key] = value
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return compactLLMContextMap(ret, 1)
}

func stateChangingSchemaInvalidRepair(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	return repairActionIsSchemaInvalidRepair(map[string]any{
		failureOutputKeyClass: stringFromMap(step.Output, failureOutputKeyClass),
	}, step.Output)
}

func repairActionIsSchemaInvalidRepair(input map[string]any, rawFailedOutput ...map[string]any) bool {
	output := firstRawFailureOutput(input, rawFailedOutput...)
	class := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(input, failureOutputKeyClass),
		stringFromMap(output, failureOutputKeyClass),
	))
	stage := strings.TrimSpace(stringFromMap(output, "stage"))
	if class == failureClassLLMDecisionInvalid || stage == "llm_schema_invalid" {
		return true
	}
	text := strings.ToLower(strings.Join([]string{
		stringFromMap(input, "failureReason"),
		stringFromMap(output, "error"),
		stringFromMap(output, "reason"),
	}, "\n"))
	return strings.Contains(text, "invalid llm decision json") ||
		strings.Contains(text, "invalid character") && strings.Contains(text, "json")
}

func firstRawFailureOutput(input map[string]any, rawFailedOutput ...map[string]any) map[string]any {
	if len(rawFailedOutput) > 0 && len(rawFailedOutput[0]) > 0 {
		return rawFailedOutput[0]
	}
	if failedOutput := mapFromValue(input["failedStepOutput"]); len(failedOutput) > 0 {
		return failedOutput
	}
	return nil
}

func repairActionTargetSet(input map[string]any, pkg map[string]any, rawFailedOutput ...map[string]any) []string {
	seen := map[string]bool{}
	var ret []string
	manifest := repairPatchRelevantWorkspaceManifest(input)
	add := func(value string) {
		value = normalizeStructuredFailureTargetRef(value)
		if value == "" {
			return
		}
		if canonical, ok := canonicalRepairTargetRefFromManifest(manifest, value); ok {
			value = canonical
		} else if len(manifest) > 0 {
			return
		}
		if seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	addStructuredFailureSignalTargetRef := func(value string) {
		value = normalizeStructuredFailureTargetRef(value)
		if value == "" || !structuredFailureTextTargetRefAllowed(value) {
			return
		}
		add(value)
	}
	add(repairActionTargetRefFromScope(stringFromMap(input, "failureTargetScope")))
	if currentFailure := mapFromValue(pkg["currentFailure"]); len(currentFailure) > 0 {
		add(stringFromMap(currentFailure, "targetRef"))
		add(repairActionTargetRefFromScope(stringFromMap(currentFailure, "targetScope")))
	}
	output := firstRawFailureOutput(input, rawFailedOutput...)
	add(stringFromMap(output, "targetRef"))
	add(stringFromMap(output, "path"))
	if diagnostics := mapFromValue(output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		add(stringFromMap(diagnostics, "targetRef"))
		add(stringFromMap(diagnostics, "conflictRef"))
	}
	for _, key := range []string{"failureTargetRepairContext", "readableResourceRepairContext", "patchRepairContext", "targetSnapshots", "patchRepairSuggestions"} {
		for _, item := range mapSliceFromValue(input[key]) {
			add(stringFromMap(item, "targetRef"))
		}
	}
	for _, key := range []string{"rootCauseCandidateContext"} {
		for _, item := range mapSliceFromValue(input[key]) {
			add(stringFromMap(item, "targetRef"))
		}
	}
	for _, item := range mapSliceFromValue(pkg["targetSnapshots"]) {
		add(stringFromMap(item, "targetRef"))
	}
	for _, item := range mapSliceFromValue(pkg["currentTargetSnapshots"]) {
		add(stringFromMap(item, "targetRef"))
	}
	for _, item := range mapSliceFromValue(pkg["rootCauseCandidateContext"]) {
		add(stringFromMap(item, "targetRef"))
	}
	if signals := mapFromValue(input["structuredFailureSignals"]); len(signals) > 0 {
		for _, ref := range stringSliceFromValue(signals["targetRefs"]) {
			addStructuredFailureSignalTargetRef(ref)
		}
	}
	if signals := mapFromValue(pkg["structuredFailureSignals"]); len(signals) > 0 {
		for _, ref := range stringSliceFromValue(signals["targetRefs"]) {
			addStructuredFailureSignalTargetRef(ref)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return limitedStrings(ret, maxLLMContextSliceItems)
}

func coverageTargetSetFromRepairInput(input map[string]any) []map[string]any {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]map[string]any, 0)
	add := func(kind string, item map[string]any, targetText string) {
		targetText = strings.TrimSpace(targetText)
		if targetText == "" {
			return
		}
		token := normalizeCoverageGapRepairToken(firstNonEmptyString(stringFromMap(item, "goalRequirementId"), targetText))
		if token == "" || seen[kind+":"+token] {
			return
		}
		seen[kind+":"+token] = true
		next := map[string]any{
			"kind":       kind,
			"targetText": targetText,
			"state":      firstNonEmptyString(stringFromMap(item, "state"), "failed"),
		}
		if id := strings.TrimSpace(stringFromMap(item, "goalRequirementId")); id != "" {
			next["goalRequirementId"] = id
			next["coverageTargetToken"] = id
		} else {
			next["coverageTargetToken"] = targetText
		}
		for _, key := range []string{"item", "raw", "requirement", "source"} {
			if value, ok := item[key]; ok && value != nil {
				next[key] = compactLLMContextValue(value, 1)
			}
		}
		if refs := evidenceRefsFromValue(item["evidenceRefs"]); len(refs) > 0 {
			next["evidenceRefs"] = refs
		}
		if candidates := runtimeRepairProgressEvidenceSummaries(mapSliceFromValue(item["evidenceCandidates"])); len(candidates) > 0 {
			next["evidenceCandidates"] = candidates
		}
		ret = append(ret, next)
	}
	for _, item := range mapSliceFromValue(input["missingDoDItems"]) {
		add("dod", item, stringFromMap(item, "item"))
	}
	for _, item := range missingGoalCoverageFromInput(input) {
		add("goal", item, firstNonEmptyString(
			stringFromMap(item, "goalRequirementId"),
			missingGoalCoverageRequirementText(item),
		))
	}
	for _, item := range mapSliceFromValue(input["missingDeliverableCoverage"]) {
		add("deliverable", item, firstNonEmptyString(
			stringFromMap(item, "item"),
			stringFromMap(item, "id"),
			stringFromMap(item, "expectationId"),
			stringFromMap(item, "requirement"),
		))
	}
	if len(ret) == 0 {
		return nil
	}
	if len(ret) > maxLLMContextSliceItems {
		ret = ret[:maxLLMContextSliceItems]
	}
	return ret
}

func ensureCoverageRepairTargetSet(input map[string]any) {
	if len(input) == 0 {
		return
	}
	targets := coverageTargetSetFromRepairInput(input)
	if len(targets) == 0 {
		return
	}
	input["coverageRepairRequired"] = true
	input["coverageTargetSet"] = targets
	if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
		pkg["coverageRepairRequired"] = true
		pkg["coverageTargetSet"] = targets
		required := mapFromValue(pkg["requiredOutput"])
		if len(required) == 0 {
			required = map[string]any{}
		}
		required["mustDeclareCoverageTargets"] = true
		if strings.TrimSpace(stringFromMap(input, "repairObjective")) == repairObjectiveConcreteValidationEvidence ||
			boolValue(input["boundedValidationRequired"]) ||
			boolValue(required["mustProduceConcreteValidationEvidence"]) {
			required["coverageTargetMatchRule"] = "validation evidence must map to one unresolved DoD item text, goalRequirementId, goal item, or deliverable item from coverageTargetSet"
		} else {
			required["coverageTargetMatchRule"] = "patch/apply coverageTargets must match one unresolved DoD item text, goalRequirementId, goal item, or deliverable item from coverageTargetSet"
		}
		pkg["requiredOutput"] = required
		input["repairActionPackage"] = pkg
	}
}

func repairActionTargetRefFromScope(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return ""
	}
	_, value, ok := strings.Cut(scope, "=")
	if !ok {
		return scope
	}
	return strings.TrimSpace(value)
}

func failedPatchDigestFromRepairFailure(input map[string]any, rawFailedOutput ...map[string]any) map[string]any {
	output := firstRawFailureOutput(input, rawFailedOutput...)
	text := strings.ToLower(strings.Join(repairForbiddenReuseTextValues(input, output), "\n"))
	class := strings.TrimSpace(firstNonEmptyString(stringFromMap(input, failureOutputKeyClass), stringFromMap(output, failureOutputKeyClass)))
	if class != failureClassPatchApply &&
		!strings.Contains(text, "patch") &&
		!strings.Contains(text, "hunk") &&
		!strings.Contains(text, "oldtext") &&
		!strings.Contains(text, "context mismatch") {
		return nil
	}
	ret := map[string]any{}
	add := func(key string, value string) {
		if value = strings.TrimSpace(value); value != "" {
			ret[key] = value
		}
	}
	add("patchId", firstNonEmptyString(stringFromMap(output, "patchId"), stringFromMap(input, "patchId")))
	add("targetRef", firstNonEmptyString(
		stringFromMap(output, "targetRef"),
		stringFromMap(input, "targetRef"),
		repairActionTargetRefFromScope(stringFromMap(input, "failureTargetScope")),
	))
	add("patchType", firstNonEmptyString(stringFromMap(output, "patchType"), stringFromMap(input, "patchType")))
	add("failureReason", firstNonEmptyString(stringFromMap(output, "reason"), stringFromMap(output, "error"), stringFromMap(input, "failureReason")))
	if diagnostics := mapFromValue(output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		add("failureDetail", firstNonEmptyString(stringFromMap(diagnostics, "errorDetail"), stringFromMap(diagnostics, "guidance")))
		if stringFromMap(ret, "targetRef") == "" {
			add("targetRef", stringFromMap(diagnostics, "targetRef"))
		}
		if stringFromMap(ret, "patchType") == "" {
			add("patchType", stringFromMap(diagnostics, "patchType"))
		}
	}
	if repairActionFailureIndicatesStalePatch(input, output) {
		ret["stalePatchRejected"] = true
	}
	if len(ret) == 0 {
		return nil
	}
	return compactLLMContextMap(ret, 1)
}

func repairActionForbiddenActions(input map[string]any, pkg map[string]any) []string {
	ret := []string{"read_only_graph_for_state_changing_repair"}
	if len(stringSliceFromValue(pkg["repairTargetSet"])) > 0 {
		ret = append(ret, "modify_target_outside_repair_target_set")
	}
	forbiddenReuse := stringSliceFromValue(pkg["forbiddenContentReuse"])
	if repairActionFailureIndicatesStalePatch(input, firstRawFailureOutput(input)) ||
		stringSliceContains(forbiddenReuse, "stale_patch_text") ||
		stringSliceContains(forbiddenReuse, "missing_old_text") ||
		stringSliceContains(forbiddenReuse, "malformed_diff_text") {
		ret = append(ret, "reuse_stale_patch_hunk")
	}
	if len(mapSliceFromValue(pkg["currentTargetAlreadySatisfiesFailedPatch"])) > 0 {
		ret = append(ret, "repeat_same_failed_patch")
	}
	return limitedStrings(compactUniqueStrings(ret), 8)
}

func repairActionRequiredRepairMode(input map[string]any, pkg map[string]any, rawFailedOutput ...map[string]any) string {
	output := firstRawFailureOutput(input, rawFailedOutput...)
	if !repairActionFailureIndicatesStalePatch(input, output) &&
		!stringSliceContains(stringSliceFromValue(pkg["forbiddenContentReuse"]), "stale_patch_text") &&
		!stringSliceContains(stringSliceFromValue(pkg["forbiddenContentReuse"]), "missing_old_text") &&
		!stringSliceContains(stringSliceFromValue(pkg["forbiddenContentReuse"]), "malformed_diff_text") {
		return ""
	}
	if int64FromValue(input["targetRepairAttempts"]) >= 2 {
		return "exact_replacement_against_current_snapshot"
	}
	return "patch_current_snapshot_only"
}

func repairActionFailureIndicatesStalePatch(input map[string]any, output map[string]any) bool {
	text := strings.ToLower(strings.Join(repairForbiddenReuseTextValues(input, output), "\n"))
	return strings.Contains(text, "context mismatch") ||
		strings.Contains(text, "patch does not apply") ||
		strings.Contains(text, "stale") ||
		strings.Contains(text, "oldtext not found") ||
		strings.Contains(text, "old text not found") ||
		strings.Contains(text, "hunk header") ||
		strings.Contains(text, "malformed unified") ||
		strings.Contains(text, "invalid unified")
}

func repairActionBudgetState(input map[string]any, pkg map[string]any) map[string]any {
	ret := map[string]any{}
	if attempt := int64FromValue(input[stepInputKeyRuntimeRepairAttempt]); attempt > 0 {
		ret["attempt"] = attempt
	}
	if maxAttempts := int64FromValue(input["maxRuntimeRepairAttempts"]); maxAttempts > 0 {
		ret["maxAttempts"] = maxAttempts
	}
	if targetAttempts := int64FromValue(input["targetRepairAttempts"]); targetAttempts > 0 {
		ret["targetRepairAttempts"] = targetAttempts
	}
	class := strings.TrimSpace(firstNonEmptyString(stringFromMap(input, failureOutputKeyClass), stringFromMap(pkg, failureOutputKeyClass)))
	switch class {
	case failureClassProviderTimeout, failureClassProviderUnavailable, failureClassContextTooLarge, failureClassRateLimited, failureClassProviderStreamIncomplete:
		ret["budgetKind"] = "provider_retry"
	case failureClassLLMDecisionInvalid:
		ret["budgetKind"] = "llm_schema_repair"
	case failureClassPatchApply:
		ret["budgetKind"] = "patch_apply_repair"
	case failureClassShellCommand:
		ret["budgetKind"] = "verification_repair"
	case failureClassRuntime:
		ret["budgetKind"] = "state_changing_contract_repair"
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func compactStateChangingRepairInputAroundActionPackage(input map[string]any) {
	if len(input) == 0 || len(mapFromValue(input["repairActionPackage"])) == 0 {
		return
	}
	omitted := []string{}
	omit := func(key string) {
		if _, ok := input[key]; !ok {
			return
		}
		delete(input, key)
		omitted = append(omitted, key)
	}
	for _, key := range []string{
		"failedStepInput",
		"failedStepOutput",
		"previousStepOutput",
		"failedExecutionGraph",
		stepInputKeyGraphEvidence,
		"runEvidenceSummary",
		stepOutputKeyExecutionGraph,
	} {
		omit(key)
	}
	if len(omitted) > 0 {
		input["repairInputCompaction"] = map[string]any{
			"mode":                  "repair_action_package_primary",
			"omittedFields":         omitted,
			"primaryField":          "repairActionPackage",
			"reason":                "repair uses bounded current failure package instead of duplicated historical payloads",
			"targetScopedByteLimit": 120 * 1024,
		}
	}
}

func repairActionTargetRelevanceDiagnostics(input map[string]any, pkg map[string]any) map[string]any {
	if len(input) == 0 && len(pkg) == 0 {
		return nil
	}
	ret := map[string]any{}
	if scope := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(input, "failureTargetScope"),
		stringFromMap(pkg, "failureTargetScope"),
	)); scope != "" {
		ret["failureTargetScope"] = scope
	}
	if targets := mapSliceFromValue(pkg["targetSnapshots"]); len(targets) > 0 {
		ret["targetSnapshotCount"] = len(targets)
		ret["targetRefs"] = repairActionTargetRefs(targets)
	}
	if manifest := mapSliceFromValue(pkg["workspaceFileManifest"]); len(manifest) > 0 {
		ret["workspaceManifestEntryCount"] = len(manifest)
	}
	if signals := mapFromValue(pkg["structuredFailureSignals"]); len(signals) > 0 {
		if refs := stringSliceFromValue(signals["targetRefs"]); len(refs) > 0 {
			ret["failureSignalTargetRefs"] = limitedStrings(refs, maxLLMContextSliceItems)
		}
		if ids := stringSliceFromValue(signals["identifiers"]); len(ids) > 0 {
			ret["failureSignalIdentifierCount"] = len(ids)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func repairActionTargetRefs(targets []map[string]any) []string {
	if len(targets) == 0 {
		return nil
	}
	refs := make([]string, 0, len(targets))
	for _, target := range targets {
		if ref := stringFromMap(target, "targetRef"); ref != "" {
			refs = append(refs, ref)
		}
	}
	return limitedStrings(compactUniqueStrings(refs), maxLLMContextSliceItems)
}

func structuredFailureSignalsFromFailureOutput(output map[string]any, execErr error, failureClass string, failureSignature string) map[string]any {
	texts := repairFailureSignalTexts(output, execErr, failureClass, failureSignature)
	if len(texts) == 0 {
		return nil
	}
	sources := make([]string, 0, len(texts))
	targets := []string{}
	externalRefs := []string{}
	identifiers := []string{}
	messages := []string{}
	lineRefs := []map[string]any{}
	seenLineRefs := map[string]bool{}
	for source, text := range texts {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		sources = append(sources, source)
		for _, ref := range extractStructuredFailureLineRefs(text) {
			key := fmt.Sprintf("%s:%v:%v", ref["targetRef"], ref["line"], ref["column"])
			if seenLineRefs[key] {
				continue
			}
			seenLineRefs[key] = true
			lineRefs = append(lineRefs, ref)
			if targetRef := stringFromMap(ref, "targetRef"); targetRef != "" {
				targets = append(targets, targetRef)
			}
			if len(lineRefs) >= maxLLMContextSliceItems {
				break
			}
		}
		for _, targetRef := range extractStructuredFailureTargetRefs(text) {
			if structuredFailureRefLooksExternalReference(targetRef) {
				externalRefs = append(externalRefs, targetRef)
				continue
			}
			if structuredFailureTargetRefGroundedBySourceOrText(source, text, targetRef) {
				targets = append(targets, targetRef)
			}
		}
		identifiers = append(identifiers, extractStructuredFailureIdentifiers(text)...)
		messages = append(messages, extractStructuredFailureMessages(text)...)
	}
	ret := map[string]any{}
	if sources = limitedStrings(compactUniqueStrings(sources), 8); len(sources) > 0 {
		ret["sources"] = sources
	}
	if targets = limitedStrings(compactUniqueStrings(targets), maxLLMContextSliceItems); len(targets) > 0 {
		ret["targetRefs"] = targets
	}
	if externalRefs = limitedStrings(compactUniqueStrings(externalRefs), maxLLMContextSliceItems); len(externalRefs) > 0 {
		ret["externalReferences"] = externalRefs
	}
	if len(lineRefs) > 0 {
		ret["lineRefs"] = lineRefs
	}
	if identifiers = limitedStrings(compactUniqueStrings(identifiers), maxLLMContextSliceItems); len(identifiers) > 0 {
		ret["identifiers"] = identifiers
	}
	if messages = limitedStrings(compactUniqueStrings(messages), 10); len(messages) > 0 {
		ret["messages"] = messages
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func repairFailureSignalTexts(output map[string]any, execErr error, failureClass string, failureSignature string) map[string]string {
	ret := map[string]string{}
	add := func(source string, value string) {
		value = strings.TrimSpace(value)
		if source == "" || value == "" || containsRuntimeRedactionMarker(value) {
			return
		}
		ret[source] = value
	}
	for _, key := range []string{"stderr", "stdout", "stderrPreview", "stdoutPreview", "error", "reason", failureOutputKeySignature, "failureCategory"} {
		add(key, stringFromMap(output, key))
	}
	if diagnostics := mapFromValue(output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		for _, key := range []string{"kind", "reason", "error", "targetRef"} {
			add("runtimeDiagnostics."+key, stringFromMap(diagnostics, key))
		}
	}
	if diagnostics := mapFromValue(output["streamDiagnostics"]); len(diagnostics) > 0 {
		for _, key := range []string{"kind", "reason", "error"} {
			add("streamDiagnostics."+key, stringFromMap(diagnostics, key))
		}
	}
	if diagnostics := mapFromValue(output["lastFailureDiagnostics"]); len(diagnostics) > 0 {
		for _, key := range []string{"targetRef", "stdoutPreview", "stderrPreview", "error", "reason", failureOutputKeySignature} {
			add("lastFailureDiagnostics."+key, stringFromMap(diagnostics, key))
		}
	}
	if failedOutput := mapFromValue(output["failedStepOutput"]); len(failedOutput) > 0 {
		for _, key := range []string{"stderr", "stdout", "error", "reason", failureOutputKeySignature} {
			add("failedStepOutput."+key, stringFromMap(failedOutput, key))
		}
	}
	add("execErr", errorString(execErr))
	add("failureClass", failureClass)
	add(failureOutputKeySignature, failureSignature)
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func extractStructuredFailureLineRefs(text string) []map[string]any {
	matches := structuredFailureLineRefPattern.FindAllStringSubmatch(text, maxLLMContextSliceItems)
	if len(matches) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		targetRef := normalizeStructuredFailureTargetRef(match[1])
		if targetRef == "" || structuredFailureRefLooksExternalReference(targetRef) {
			continue
		}
		line, _ := strconv.Atoi(match[2])
		if line <= 0 {
			continue
		}
		item := map[string]any{
			"targetRef": targetRef,
			"line":      line,
		}
		if len(match) > 3 && strings.TrimSpace(match[3]) != "" {
			if column, _ := strconv.Atoi(match[3]); column > 0 {
				item["column"] = column
			}
		}
		ret = append(ret, item)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func extractStructuredFailureTargetRefs(text string) []string {
	matches := structuredFailureTargetRefPattern.FindAllStringSubmatch(text, maxLLMContextSliceItems*2)
	if len(matches) == 0 {
		return nil
	}
	ret := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		if targetRef := normalizeStructuredFailureTargetRef(match[1]); targetRef != "" && structuredFailureTextTargetRefAllowed(targetRef) {
			ret = append(ret, targetRef)
		}
	}
	return ret
}

func structuredFailureTargetRefGroundedBySourceOrText(source string, text string, targetRef string) bool {
	targetRef = normalizeStructuredFailureTargetRef(targetRef)
	if targetRef == "" {
		return false
	}
	source = strings.ToLower(strings.TrimSpace(source))
	if strings.Contains(source, "signature") || strings.HasSuffix(source, "targetref") {
		return true
	}
	text = strings.ToLower(strings.TrimSpace(text))
	return structuredFailureTextGroundsPathDiagnostic(text) ||
		strings.Contains(text, "targetref=") ||
		strings.Contains(text, "failuretargets=") ||
		strings.Contains(text, "conflictref=")
}

func structuredFailureTextGroundsPathDiagnostic(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return false
	}
	for _, marker := range []string{
		"no such file",
		"file does not exist",
		"cannot find",
		"not found",
		"lstat ",
		"stat ",
		"open ",
		"read ",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func normalizeStructuredFailureTargetRef(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "targetRef=")
	value = strings.TrimPrefix(value, "failureTargets=")
	value = strings.TrimPrefix(value, "conflictRef=")
	value = strings.Trim(value, "`'\"()[]{}<>;,")
	value = strings.ReplaceAll(value, "\\", "/")
	for strings.HasPrefix(value, "./") {
		value = strings.TrimPrefix(value, "./")
	}
	value = strings.TrimPrefix(value, "a/")
	value = strings.TrimPrefix(value, "b/")
	value = strings.Trim(value, "/")
	if value == "" || strings.Contains(value, "://") || strings.Contains(value, " ") || !runtimeRepairTargetRefAdmissible(value) {
		return ""
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http:") || strings.HasPrefix(lower, "https:") {
		return ""
	}
	if !strings.Contains(value, ".") {
		return ""
	}
	return value
}

func structuredFailureTextTargetRefAllowed(value string) bool {
	value = normalizeStructuredFailureTargetRef(value)
	if value == "" || !runtimeRepairTargetRefAdmissible(value) {
		return false
	}
	if strings.Contains(value, "/") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(value))
	if ext == "" {
		return false
	}
	if len(ext) <= 1 || len(ext) > 7 {
		return false
	}
	for _, r := range ext[1:] {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') {
			return false
		}
	}
	base := strings.TrimSuffix(value, ext)
	if base == "" || strings.HasSuffix(base, ".") {
		return false
	}
	if structuredFailureBareTargetLooksLikeNumericMeasurement(base, ext) {
		return false
	}
	return true
}

func runtimeRepairTargetRefAdmissible(value string) bool {
	original := strings.TrimSpace(strings.Trim(value, `"'()[]{}<>;,`))
	if original == "" {
		return false
	}
	original = strings.ReplaceAll(original, "\\", "/")
	if filepath.IsAbs(original) || strings.HasPrefix(original, "/") || strings.HasPrefix(original, "~") {
		return false
	}
	clean := normalizeRepairPatchTargetRef(original)
	if clean == "" || clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, "../") {
		return false
	}
	lower := strings.ToLower(filepath.ToSlash(clean))
	if strings.Contains(lower, "://") || strings.Contains(lower, " ") || strings.Contains(lower, ":") {
		return false
	}
	for _, marker := range []string{
		".workorch-data/",
		".workorch-workspaces/",
		"/.workorch-data/",
		"/.workorch-workspaces/",
	} {
		if strings.Contains(lower, marker) || strings.HasPrefix(lower, strings.TrimSuffix(marker, "/")) {
			return false
		}
	}
	if runtimeRepairTargetLooksLikeLocalAbsolutePath(lower) {
		return false
	}
	if isRuntimeContractFieldTargetRef(lower) {
		return false
	}
	if isReservedWorkspacePatchTarget(clean) || isRuntimeDecisionArtifactTarget(clean) {
		return false
	}
	return true
}

func isRuntimeContractFieldTargetRef(cleanLower string) bool {
	cleanLower = strings.TrimSpace(filepath.ToSlash(cleanLower))
	if cleanLower == "" || strings.Contains(cleanLower, "/") {
		return false
	}
	for _, prefix := range []string{
		"dodcoverage.",
		"dodassessment.",
		"validationevidence.",
		"governance.",
		"executiongraph.",
		"llmexecutiondecision.",
		"convergencecontract.",
		"repairactionpackage.",
		"strategyreassessment.",
		"runtimecontext.",
		"runtimediagnostics.",
		"currentexecutionprogress.",
		"contextbudget.",
		"llmusage.",
	} {
		if strings.HasPrefix(cleanLower, prefix) {
			return true
		}
	}
	return false
}

func runtimeRepairTargetLooksLikeLocalAbsolutePath(cleanLower string) bool {
	cleanLower = strings.Trim(cleanLower, "/")
	if cleanLower == "" {
		return false
	}
	parts := strings.Split(cleanLower, "/")
	if len(parts) < 3 {
		return false
	}
	switch parts[0] {
	case "home", "users", "var", "tmp", "private", "opt", "usr":
		return true
	default:
		return false
	}
}

func isRuntimeDecisionArtifactTarget(targetRef string) bool {
	clean := strings.ToLower(filepath.ToSlash(filepath.Clean(strings.TrimSpace(targetRef))))
	clean = strings.TrimPrefix(clean, "./")
	base := filepath.Base(clean)
	if base == "llmexecutiondecision.plan" || strings.HasPrefix(base, "llmexecutiondecision.") {
		return true
	}
	switch base {
	case "request.json", "response.json", "repair-response.json", "usage.json", "metadata.json", "output.json", "input.json", "run.json":
		return true
	default:
		return false
	}
}

func filterAdmissibleRuntimeRepairTargetRefs(targetRefs []string) []string {
	if len(targetRefs) == 0 {
		return nil
	}
	seen := map[string]bool{}
	ret := make([]string, 0, len(targetRefs))
	for _, targetRef := range targetRefs {
		targetRef = normalizeRepairPatchTargetRef(targetRef)
		if targetRef == "" || !runtimeRepairTargetRefAdmissible(targetRef) || seen[targetRef] {
			continue
		}
		seen[targetRef] = true
		ret = append(ret, targetRef)
	}
	return ret
}

func structuredFailureRefLooksExternalReference(value string) bool {
	value = normalizeStructuredFailureTargetRef(value)
	if value == "" || !strings.Contains(value, "/") {
		return false
	}
	parts := strings.Split(value, "/")
	if len(parts) < 2 || !strings.Contains(parts[0], ".") {
		return false
	}
	return true
}

func structuredFailureBareTargetLooksLikeNumericMeasurement(base string, ext string) bool {
	base = strings.TrimSpace(base)
	ext = strings.TrimPrefix(strings.TrimSpace(ext), ".")
	if base == "" || ext == "" {
		return false
	}
	baseNumeric := true
	for _, r := range base {
		if r != '.' && (r < '0' || r > '9') {
			baseNumeric = false
			break
		}
	}
	if !baseNumeric {
		return false
	}
	hasDigit := false
	hasLetter := false
	for _, r := range ext {
		if r >= '0' && r <= '9' {
			hasDigit = true
			continue
		}
		if r >= 'a' && r <= 'z' {
			hasLetter = true
			continue
		}
		return false
	}
	return hasDigit || !hasLetter
}

func extractStructuredFailureIdentifiers(text string) []string {
	matches := structuredFailureIdentifierPattern.FindAllString(text, maxLLMContextSliceItems*4)
	if len(matches) == 0 {
		return nil
	}
	ret := make([]string, 0, len(matches))
	for _, value := range matches {
		value = strings.TrimSpace(value)
		if !structuredFailureIdentifierUseful(value) {
			continue
		}
		ret = append(ret, value)
	}
	return ret
}

func structuredFailureIdentifierUseful(value string) bool {
	if value == "" || len(value) < 3 || len(value) > 96 {
		return false
	}
	lower := strings.ToLower(value)
	if structuredFailureIdentifierStopWords[lower] {
		return false
	}
	hasShape := false
	for _, r := range value {
		if unicode.IsUpper(r) || unicode.IsDigit(r) || r == '_' {
			hasShape = true
			break
		}
	}
	return hasShape
}

func extractStructuredFailureMessages(text string) []string {
	lines := strings.Split(text, "\n")
	ret := make([]string, 0, min(len(lines), 10))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || containsRuntimeRedactionMarker(line) {
			continue
		}
		ret = append(ret, truncateLLMContextString(line))
		if len(ret) >= 10 {
			break
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func forbiddenRepairContentReuseFromFailure(input map[string]any, rawFailedOutput ...map[string]any) []string {
	output := map[string]any(nil)
	if len(rawFailedOutput) > 0 && len(rawFailedOutput[0]) > 0 {
		output = rawFailedOutput[0]
	} else if failedOutput := mapFromValue(input["failedStepOutput"]); len(failedOutput) > 0 {
		output = failedOutput
	}
	text := strings.ToLower(strings.Join(repairForbiddenReuseTextValues(input, output), "\n"))
	ret := []string{}
	if llmDecisionInvalidOutput(output) ||
		strings.Contains(text, "invalid json") ||
		strings.Contains(text, "invalid character") ||
		strings.Contains(text, "schema invalid") ||
		strings.Contains(text, "llm decision invalid") {
		ret = append(ret, "raw_invalid_llm_decision_content")
	}
	if runtimeValueContainsRedactionMarker(output, 0) ||
		runtimeValueContainsRedactionMarker(input["failedStepOutput"], 0) ||
		strings.Contains(text, "redacted runtime marker") ||
		strings.Contains(text, "runtime redaction marker") {
		ret = append(ret, "redacted_runtime_marker_text")
	}
	if strings.Contains(text, "malformed unified") ||
		strings.Contains(text, "invalid unified") ||
		strings.Contains(text, "hunk header") ||
		strings.Contains(text, "bare @@") {
		ret = append(ret, "malformed_diff_text")
	}
	if strings.Contains(text, "oldtext not found") ||
		strings.Contains(text, "old text not found") ||
		strings.Contains(text, "file/edit oldtext") {
		ret = append(ret, "missing_old_text")
	}
	if strings.Contains(text, "context mismatch") ||
		strings.Contains(text, "patch does not apply") ||
		strings.Contains(text, "stale") ||
		strings.Contains(text, "conflict") {
		ret = append(ret, "stale_patch_text")
	}
	return limitedStrings(compactUniqueStrings(ret), 8)
}

func repairForbiddenReuseTextValues(input map[string]any, output map[string]any) []string {
	values := []string{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			values = append(values, value)
		}
	}
	for _, source := range []map[string]any{input, output} {
		for _, key := range []string{
			"stage",
			"status",
			"error",
			"reason",
			"failureReason",
			failureOutputKeyClass,
			failureOutputKeySignature,
			"failureCategory",
		} {
			add(stringFromMap(source, key))
		}
	}
	for _, key := range []string{"runtimeDiagnostics", "streamDiagnostics"} {
		diagnostics := mapFromValue(output[key])
		for _, diagnosticKey := range []string{"kind", "reason", "error"} {
			add(stringFromMap(diagnostics, diagnosticKey))
		}
	}
	return values
}

func runtimeValueContainsRedactionMarker(value any, depth int) bool {
	if depth > 4 || value == nil {
		return false
	}
	switch typed := value.(type) {
	case string:
		return containsRuntimeRedactionMarker(typed)
	case map[string]any:
		for _, nested := range typed {
			if runtimeValueContainsRedactionMarker(nested, depth+1) {
				return true
			}
		}
	case []map[string]any:
		for _, nested := range typed {
			if runtimeValueContainsRedactionMarker(nested, depth+1) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if runtimeValueContainsRedactionMarker(nested, depth+1) {
				return true
			}
		}
	case []string:
		for _, nested := range typed {
			if containsRuntimeRedactionMarker(nested) {
				return true
			}
		}
	}
	return false
}

var structuredFailureLineRefPattern = regexp.MustCompile(`(?:^|[\s"'(\[])([A-Za-z0-9_./\\-]+\.[A-Za-z0-9_./\\-]+):([0-9]+)(?::([0-9]+))?`)
var structuredFailureTargetRefPattern = regexp.MustCompile(`(?:^|[\s"'(\[=])([A-Za-z0-9_./\\-]+\.[A-Za-z0-9_./\\-]+)(?::[0-9]+(?::[0-9]+)?)?`)
var structuredFailureIdentifierPattern = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)

var structuredFailureIdentifierStopWords = map[string]bool{
	"error": true, "failed": true, "failure": true, "status": true, "stage": true,
	"stderr": true, "stdout": true, "reason": true, "class": true, "targetref": true,
	"undefined": true, "unknown": true, "field": true, "literal": true, "type": true,
	"struct": true, "exit": true, "runtime": true, "patch": true, "diff": true,
	"content": true, "context": true, "mismatch": true, "invalid": true,
}

func repairActionTargetSnapshots(input map[string]any) []map[string]any {
	var ret []map[string]any
	seen := map[string]bool{}
	manifest := repairPatchRelevantWorkspaceManifest(input)
	add := func(items []map[string]any, source string) {
		for _, item := range items {
			targetRef := strings.TrimSpace(stringFromMap(item, "targetRef"))
			if canonical, ok := canonicalRepairTargetRefFromManifest(manifest, targetRef); ok {
				if canonical != normalizeRepairPatchTargetRef(targetRef) && !boolValue(item["exists"]) && strings.TrimSpace(stringFromMap(item, "content")) == "" {
					continue
				}
				targetRef = canonical
			}
			if targetRef == "" || !runtimeRepairTargetRefAdmissible(targetRef) || boolValue(item["isBinary"]) || seen[targetRef] {
				continue
			}
			seen[targetRef] = true
			snapshot := repairActionTargetSnapshot(item)
			if len(snapshot) == 0 || boolValue(snapshot["isBinary"]) {
				continue
			}
			if original := strings.TrimSpace(stringFromMap(snapshot, "targetRef")); original != "" && original != targetRef {
				snapshot["diagnosticTargetRef"] = original
			}
			snapshot["targetRef"] = targetRef
			if source != "" {
				snapshot["contextSource"] = source
			}
			ret = append(ret, snapshot)
			if len(ret) >= maxLLMContextSliceItems {
				return
			}
		}
	}
	add(mapSliceFromValue(input["failureTargetRepairContext"]), "failure_target")
	if len(ret) < maxLLMContextSliceItems {
		add(mapSliceFromValue(input["readableResourceRepairContext"]), "readable_resource")
	}
	if len(ret) < maxLLMContextSliceItems {
		add(mapSliceFromValue(input["patchRepairContext"]), "patch_target")
	}
	if len(ret) < maxLLMContextSliceItems {
		add(mapSliceFromValue(input["targetSnapshots"]), "lineage_target")
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func repairActionTargetSnapshot(item map[string]any) map[string]any {
	ret := map[string]any{}
	for _, key := range []string{
		"targetRef",
		"diagnosticTargetRef",
		"exists",
		"contentHash",
		"contentComplete",
		"safeForFileWrite",
		"redacted",
		"truncated",
		"isBinary",
		"size",
		"source",
		"error",
	} {
		if value, ok := item[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	content := strings.TrimSpace(stringFromMap(item, "content"))
	if content != "" && !containsRuntimeRedactionMarker(content) {
		if boolValue(item["contentComplete"]) && boolValue(item["safeForFileWrite"]) {
			ret["content"] = content
		} else {
			ret["contentPreview"] = truncateLLMContextString(content)
		}
	}
	return ret
}

func repairActionTargetSnapshotMetadata(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(items))
	for _, item := range items {
		next := map[string]any{}
		for _, key := range []string{
			"targetRef",
			"diagnosticTargetRef",
			"exists",
			"contentHash",
			"contentComplete",
			"safeForFileWrite",
			"redacted",
			"truncated",
			"isBinary",
			"size",
			"source",
			"contextSource",
			"error",
		} {
			if value, ok := item[key]; ok && value != nil {
				next[key] = compactLLMContextValue(value, 1)
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

func repairActionWorkspaceManifest(input map[string]any) []map[string]any {
	manifest := mapSliceFromValue(input[stepInputKeyWorkspaceFileManifest])
	if len(manifest) == 0 {
		return nil
	}
	limit := min(len(manifest), maxLLMContextSliceItems)
	ret := make([]map[string]any, 0, limit)
	for idx := 0; idx < limit; idx++ {
		item := manifest[idx]
		entry := map[string]any{}
		for _, key := range []string{"path", "kind", "size", "contentHash"} {
			if value, ok := item[key]; ok && value != nil {
				entry[key] = compactLLMContextValue(value, 1)
			}
		}
		if len(entry) > 0 {
			ret = append(ret, entry)
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func stateChangingReadOnlyGraphFailureText(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(value, "state-changing") &&
		(strings.Contains(value, "read-only executiongraph") ||
			strings.Contains(value, "without patch/apply") ||
			strings.Contains(value, "inspection-only") ||
			strings.Contains(value, "llm-gated"))
}

func compactStateChangingRecoveryFailureOutput(output map[string]any) map[string]any {
	if len(output) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"stage",
		"status",
		"error",
		"reason",
		failureOutputKeyClass,
		failureOutputKeySignature,
		"failureCategory",
	} {
		if value, ok := output[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	if diagnostics := mapFromValue(output["streamDiagnostics"]); len(diagnostics) > 0 {
		ret["streamDiagnostics"] = compactLLMContextMap(diagnostics, 0)
	}
	if llmDecisionInvalidOutput(output) {
		if _, ok := output["rawContentPreview"]; ok {
			ret["rawContentPreviewOmitted"] = true
		}
		if _, ok := output["repairRawContentPreview"]; ok {
			ret["repairRawContentPreviewOmitted"] = true
		}
		if len(mapSliceFromValue(output["streamEvents"])) > 0 {
			ret["streamEventsOmitted"] = true
		}
		if ret["rawContentPreviewOmitted"] == true ||
			ret["repairRawContentPreviewOmitted"] == true ||
			ret["streamEventsOmitted"] == true {
			ret["omittedContentReason"] = "invalid or rejected LLM decision content is not reusable repair input; rebuild from current context, target snapshots, or fresh reads"
		}
	} else if preview := streamEventsPreview(mapSliceFromValue(output["streamEvents"]), 1200); preview != "" {
		if containsRuntimeRedactionMarker(preview) {
			ret["partialOutputPreviewOmitted"] = true
			ret["omittedContentReason"] = "rejected or redacted runtime output is not reusable repair input; rebuild from current target snapshots or fresh reads"
		} else {
			ret["partialOutputPreview"] = compactLLMContextString(preview)
		}
	}
	if graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok {
		ret["rejectedExecutionGraph"] = rejectedGraphSummary(graph)
	} else if graph, ok := runtimeGraphSpecFromValue(mapFromValue(output["llmDecision"])[stepOutputKeyExecutionGraph]); ok {
		ret["rejectedExecutionGraph"] = rejectedGraphSummary(graph)
	}
	return ret
}

func streamEventsPreview(events []map[string]any, limit int) string {
	if limit <= 0 || len(events) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, event := range events {
		delta := stringFromMap(event, "delta")
		if delta == "" {
			continue
		}
		remaining := limit - builder.Len()
		if remaining <= 0 {
			break
		}
		if len(delta) > remaining {
			builder.WriteString(delta[:remaining])
			break
		}
		builder.WriteString(delta)
	}
	return strings.TrimSpace(builder.String())
}

func rejectedGraphSummary(graph runtimemodel.BlockExecutionGraph) map[string]any {
	if len(graph.Nodes) == 0 {
		return nil
	}
	kinds := make([]string, 0, len(graph.Nodes))
	nodeIDs := make([]string, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if node.Kind != "" {
			kinds = append(kinds, node.Kind)
		}
		if node.ID != "" {
			nodeIDs = append(nodeIDs, node.ID)
		}
	}
	return map[string]any{
		"nodeCount": len(graph.Nodes),
		"nodeIds":   limitedStrings(compactUniqueStrings(nodeIDs), 12),
		"kinds":     limitedStrings(compactUniqueStrings(kinds), 12),
	}
}

func limitedStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return append([]string(nil), values[:limit]...)
}

func repairObjectiveRequiresStateChange(value string) bool {
	switch strings.TrimSpace(value) {
	case repairObjectiveStateChangingOutput:
		return true
	default:
		return false
	}
}

func recoveryBudgetScope(lineageType string, rootStepID common.StepID, failureClass string, failureSignature string) map[string]any {
	ret := map[string]any{
		"lineageType": lineageType,
		"rootStepId":  string(rootStepID),
	}
	if failureClass = strings.TrimSpace(failureClass); failureClass != "" {
		ret["failureClass"] = failureClass
	}
	if failureSignature = strings.TrimSpace(failureSignature); failureSignature != "" {
		ret["failureSignature"] = failureSignature
	}
	return ret
}

func stateChangingRepairRequired(run *runModel.Run, failedStep *runModel.Step, execErr error) bool {
	if failedStep == nil {
		return false
	}
	if runtimeContextIntegrityGateFailure(failedStep) {
		return false
	}
	if doDVerificationEvidenceRepairRequired(failedStep) {
		return false
	}
	if boolValue(failedStep.Input["stateChangingOutputRequired"]) {
		return true
	}
	if failedStepInputHasMissingDoDItems(failedStep) && (run == nil || runtimeStageFromStepInput(failedStep.Input) == runtimeStageRepair) {
		return true
	}
	if runExecutionContractRequiresStateChange(run) && failedStepHasMissingDoDItems(failedStep) {
		return true
	}
	if runtimeRepairFailureIsTransientExecutionTimeout(failedStep, execErr) {
		return false
	}
	if runExecutionContractRequiresStateChange(run) &&
		runtimeStageFromStepInput(failedStep.Input) == runtimeStageExecution &&
		!runtimeRepairFailureIsReadOnlyObservation(failedStep) {
		return true
	}
	text := strings.ToLower(strings.Join([]string{
		errorString(execErr),
		stringFromMap(failedStep.Output, "error"),
		stringFromMap(failedStep.Output, "reason"),
		stringFromMap(failedStep.Output, "stage"),
		stringFromMap(failedStep.Output, "ignoredExecutionGraphReason"),
		stringFromMap(mapFromValue(failedStep.Output["runtimeDiagnostics"]), "kind"),
		stringFromMap(mapFromValue(failedStep.Output["runtimeDiagnostics"]), "reason"),
	}, "\n"))
	if boolValue(failedStep.Output["executionStageContractViolation"]) {
		return true
	}
	if requiresDoDAssessmentCompletionGate(run) && missingWorkspaceFileReadFailure(failedStep, execErr) {
		return true
	}
	for _, marker := range []string{
		"execution-stage llm graph node returned executiongraph",
		"state-changing llm repair step returned executiongraph",
		"state-changing llm repair step returned no state-changing output",
		"state-changing llm repair step returned executiongraph without patch/apply",
		"state-changing execution-stage llm graph node returned no state-changing output",
		"state-changing executiongraph patch/apply depends on llm/workspace_execution before patch",
		"planning executiongraph patch/apply depends on llm/workspace_execution before patch",
		"targetref is required",
		"targetref must be set",
		"patch content contains unresolved runtime template",
		"patch diff contains unresolved runtime template",
		"patch content is required",
		"patch diff is required",
		"patch_dispatch_invalid",
		"file_edit_conflict",
		"file/edit oldtext not found",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func runtimeContextIntegrityGateFailure(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	if strings.TrimSpace(step.Kind) != llmWorkspaceExecutionStepKind {
		return false
	}
	if strings.TrimSpace(stringFromMap(step.Output, "stage")) != "llm_context_integrity_failed" {
		return false
	}
	budget := mapFromValue(step.Output["contextBudget"])
	action := strings.TrimSpace(stringFromMap(budget, "contextGateAction"))
	if action == "read_required" || action == "fail_fast" {
		return true
	}
	text := strings.ToLower(strings.Join([]string{
		stringFromMap(step.Output, "error"),
		stringFromMap(step.Output, "reason"),
	}, " "))
	return strings.Contains(text, "llm context gate")
}

func runtimeContextIntegrityGateCanRecoverWithCoverageTargets(step *runModel.Step) bool {
	if step == nil || !runtimeContextIntegrityGateFailure(step) {
		return false
	}
	reason := strings.TrimSpace(stringFromMap(mapFromValue(step.Output["contextBudget"]), "contextGateReason"))
	if reason == "" {
		reason = strings.ToLower(strings.Join([]string{
			stringFromMap(step.Output, "error"),
			stringFromMap(step.Output, "reason"),
		}, " "))
	}
	if !strings.Contains(reason, "coverage_repair_required_without_coverage_target_set") &&
		!strings.Contains(reason, "must_declare_coverage_targets_without_target_set") {
		return false
	}
	probe := cloneStepPayload(step.Input)
	if probe == nil {
		probe = map[string]any{}
	}
	inheritCoverageRepairContext(probe, step)
	ensureCoverageRepairTargetSet(probe)
	return len(coverageTargetSetFromRepairInput(probe)) > 0
}

func doDVerificationEvidenceRepairRequired(failedStep *runModel.Step) bool {
	if failedStep == nil || runtimeStageFromStepInput(failedStep.Input) != runtimeStageVerification {
		return false
	}
	class := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(failedStep.Output, failureOutputKeyClass),
		stringFromMap(mapFromValue(failedStep.Output["runtimeDiagnostics"]), failureOutputKeyClass),
	))
	signature := strings.ToLower(strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeySignature)))
	if class != failureClassDoDCoverageMissing && !strings.Contains(signature, "class="+failureClassDoDCoverageMissing) {
		return false
	}
	if len(missingDoDItemsFromFailureOutput(failedStep.Output)) == 0 &&
		len(missingGoalCoverageFromStep(failedStep)) == 0 &&
		len(missingDeliverableCoverageFromStep(failedStep)) == 0 {
		return false
	}
	diagnostics := mapFromValue(failedStep.Output["runtimeDiagnostics"])
	return strings.TrimSpace(stringFromMap(diagnostics, "kind")) == "dod_evidence_quality_gap"
}

func coverageVerificationGapPresent(input map[string]any, failedStep *runModel.Step) bool {
	if len(mapSliceFromValue(input["coverageTargetSet"])) > 0 ||
		len(coverageTargetSetFromRepairInput(input)) > 0 ||
		len(mapSliceFromValue(input["missingDoDItems"])) > 0 ||
		len(missingGoalCoverageFromInput(input)) > 0 ||
		len(mapSliceFromValue(input["missingDeliverableCoverage"])) > 0 {
		return true
	}
	if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
		if boolValue(pkg["coverageRepairRequired"]) || len(mapSliceFromValue(pkg["coverageTargetSet"])) > 0 {
			return true
		}
	}
	if failedStep == nil {
		return false
	}
	diagnostics := mapFromValue(failedStep.Output["runtimeDiagnostics"])
	if strings.TrimSpace(stringFromMap(diagnostics, "kind")) == "dod_evidence_quality_gap" {
		return true
	}
	class := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(failedStep.Output, failureOutputKeyClass),
		stringFromMap(diagnostics, failureOutputKeyClass),
	))
	signature := strings.ToLower(strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeySignature)))
	if class != failureClassDoDCoverageMissing && !strings.Contains(signature, "class="+failureClassDoDCoverageMissing) {
		return false
	}
	return len(missingDoDItemsFromFailureOutput(failedStep.Output)) > 0 ||
		len(missingGoalCoverageFromStep(failedStep)) > 0 ||
		len(missingDeliverableCoverageFromStep(failedStep)) > 0
}

func coverageValidationEvidenceRepairEligible(failedStep *runModel.Step) bool {
	if failedStep == nil || strings.TrimSpace(failedStep.Kind) != llmWorkspaceExecutionStepKind {
		return false
	}
	stage := strings.TrimSpace(stringFromMap(failedStep.Output, "stage"))
	if stage == "llm_dod_assessment" {
		return true
	}
	text := strings.ToLower(strings.Join([]string{
		stringFromMap(failedStep.Output, "error"),
		stringFromMap(failedStep.Output, "reason"),
		stringFromMap(failedStep.Output, failureOutputKeySignature),
	}, "\n"))
	return stateChangingReadOnlyGraphFailureText(text) ||
		strings.Contains(text, "without patch/apply") ||
		strings.Contains(text, "coverage")
}

func (o *RuntimeOrchestrator) coverageValidationRepairShouldEscalateToStateChanging(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	input map[string]any,
) bool {
	if o == nil || run == nil || failedStep == nil {
		return false
	}
	if !coverageValidationEvidenceRepairEligible(failedStep) ||
		!coverageVerificationGapPresent(input, failedStep) {
		return false
	}
	if o.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep) == 0 {
		return false
	}
	if repairHasImplementationTargetEvidence(input, mapFromValue(input["repairActionPackage"])) {
		return false
	}
	if repairHasImplementationTargetEvidence(failedStep.Input, mapFromValue(failedStep.Input["repairActionPackage"])) {
		return false
	}
	if len(mapSliceFromValue(input["currentTargetAlreadySatisfiesFailedPatch"])) > 0 ||
		boolValue(input["stateChangeAlreadySatisfiedByCurrentTarget"]) {
		return false
	}
	return true
}

func runtimeRepairFailureIsTransientExecutionTimeout(step *runModel.Step, execErr error) bool {
	if step == nil || strings.HasPrefix(strings.TrimSpace(step.Kind), "llm/") {
		return false
	}
	text := strings.ToLower(strings.Join([]string{
		errorString(execErr),
		stringFromMap(step.Output, "error"),
		stringFromMap(step.Output, "reason"),
		stringFromMap(step.Output, "status"),
		stringFromMap(step.Output, "stage"),
		stringFromMap(step.Output, "stderr"),
	}, "\n"))
	return strings.Contains(text, "context deadline exceeded") ||
		strings.Contains(text, "deadline exceeded") ||
		strings.Contains(text, "timed out") ||
		strings.Contains(text, "timeout")
}

func transientExecutionRetryGuidance() []string {
	return []string{
		"The previous non-LLM execution timed out before producing decisive evidence.",
		"Prefer a bounded retry or a narrower verification/action that can complete within runtime limits.",
		"Do not fabricate success; return failed assessment or blocker if the timeout is caused by external constraints.",
	}
}

func runtimeRepairFailureIsReadOnlyObservation(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	switch strings.TrimSpace(step.Kind) {
	case "file/list",
		"file/read",
		"web/search",
		"web/fetch",
		"mcp/resource_read",
		"mcp/prompt_get":
		return true
	default:
		return false
	}
}

func failedStepHasMissingDoDItems(step *runModel.Step) bool {
	return failedStepInputHasMissingDoDItems(step) || len(missingDoDItemsFromFailureOutput(step.Output)) > 0
}

func failedStepInputHasMissingDoDItems(step *runModel.Step) bool {
	return step != nil && len(mapSliceFromValue(step.Input["missingDoDItems"])) > 0
}

func missingWorkspaceFileReadFailure(failedStep *runModel.Step, execErr error) bool {
	if failedStep == nil {
		return false
	}
	text := strings.ToLower(strings.Join([]string{
		errorString(execErr),
		failedStep.Kind,
		stringFromMap(failedStep.Output, "stage"),
		stringFromMap(failedStep.Output, "error"),
		stringFromMap(failedStep.Output, "reason"),
		stringFromMap(failedStep.Output, failureOutputKeySignature),
	}, "\n"))
	if !strings.Contains(text, "file/read") && !strings.Contains(text, "file_read") {
		return false
	}
	return strings.Contains(text, "no such file") ||
		strings.Contains(text, "cannot find the file") ||
		strings.Contains(text, "file_read_invalid_path") ||
		strings.Contains(text, "lstat ")
}

func (o *RuntimeOrchestrator) runtimeRepairPatchContext(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) []map[string]any {
	if o == nil || o.workspaces == nil || run == nil || failedStep == nil {
		return nil
	}
	if !shouldIncludePatchRepairContext(failedStep) {
		return nil
	}
	workspace, err := o.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil || strings.TrimSpace(workspace.DeliverableDirectory) == "" {
		return nil
	}
	targetRefs := patchRepairTargetRefs(failedStep)
	if len(targetRefs) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, min(len(targetRefs), 3))
	for idx, targetRef := range targetRefs {
		if idx >= 3 {
			break
		}
		item := patchRepairTargetSnapshot(workspace.DeliverableDirectory, targetRef)
		if len(item) > 0 {
			ret = append(ret, item)
		}
	}
	return ret
}

func (o *RuntimeOrchestrator) runtimeRepairPatchSuggestions(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) []map[string]any {
	if o == nil || o.workspaces == nil || run == nil || failedStep == nil {
		return nil
	}
	workspace, err := o.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil || strings.TrimSpace(workspace.DeliverableDirectory) == "" {
		return nil
	}
	targetRefs := patchRepairTargetRefs(failedStep)
	if len(targetRefs) == 0 {
		return nil
	}
	patches := patchRepairRequestsFromStep(failedStep)
	if len(patches) == 0 {
		return nil
	}
	allowedTargets := map[string]bool{}
	for _, targetRef := range targetRefs {
		allowedTargets[strings.TrimSpace(targetRef)] = true
	}
	var ret []map[string]any
	for _, request := range patches {
		if len(ret) >= 5 {
			break
		}
		if request.PatchType != "unified_diff" || strings.TrimSpace(request.Diff) == "" {
			continue
		}
		if len(allowedTargets) > 0 && !allowedTargets[request.TargetRef] {
			continue
		}
		ret = append(ret, patchRepairSuggestionsForDiff(workspace.DeliverableDirectory, request.TargetRef, request.Diff)...)
	}
	if len(ret) > 5 {
		return ret[:5]
	}
	return ret
}

func (o *RuntimeOrchestrator) runtimeRepairPatchAlreadySatisfiedSignals(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) []map[string]any {
	if o == nil || o.workspaces == nil || run == nil || failedStep == nil {
		return nil
	}
	workspace, err := o.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil || strings.TrimSpace(workspace.DeliverableDirectory) == "" {
		return nil
	}
	targetRefs := patchRepairTargetRefs(failedStep)
	if len(targetRefs) == 0 {
		return nil
	}
	patches := patchRepairRequestsFromStep(failedStep)
	if len(patches) == 0 {
		return nil
	}
	allowedTargets := map[string]bool{}
	for _, targetRef := range targetRefs {
		allowedTargets[strings.TrimSpace(targetRef)] = true
	}
	var ret []map[string]any
	for _, request := range patches {
		if len(ret) >= 3 {
			break
		}
		if request.PatchType != "unified_diff" || strings.TrimSpace(request.Diff) == "" {
			continue
		}
		if len(allowedTargets) > 0 && !allowedTargets[request.TargetRef] {
			continue
		}
		if signal := patchRepairAlreadySatisfiedSignalForDiff(workspace.DeliverableDirectory, request.TargetRef, request.Diff); len(signal) > 0 {
			ret = append(ret, signal)
		}
	}
	return ret
}

func (o *RuntimeOrchestrator) runtimeRepairFailureTargetContext(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) []map[string]any {
	if o == nil || o.workspaces == nil || run == nil || failedStep == nil {
		return nil
	}
	workspace, err := o.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil || strings.TrimSpace(workspace.DeliverableDirectory) == "" {
		return nil
	}
	files, err := workspaceManifestFiles(workspace.DeliverableDirectory)
	if err != nil {
		files = nil
	}
	targetRefs := append([]string{}, failureTargetRepairRefs(failedStep)...)
	targetRefs = append(targetRefs, manifestGroundedFailureSignalTargetRefs(failedStep, files)...)
	targetRefs = compactUniqueStrings(targetRefs)
	if len(targetRefs) == 0 {
		return nil
	}
	targetRefs = resolveWorkspaceTargetRefs(workspace.DeliverableDirectory, targetRefs)
	targetRefs = compactUniqueStrings(targetRefs)
	targetRefs = filterAdmissibleRuntimeRepairTargetRefs(targetRefs)
	ret := make([]map[string]any, 0, min(len(targetRefs), 3))
	for idx, targetRef := range targetRefs {
		if idx >= 3 {
			break
		}
		item := patchRepairTargetSnapshot(workspace.DeliverableDirectory, targetRef)
		if len(item) == 0 || !runtimeRepairTargetRefAdmissible(targetRef) || boolValue(item["isBinary"]) {
			continue
		}
		item["source"] = "failure_target_current_workspace_file"
		item["guidance"] = "Use this current failure target snapshot for repair; failed stderr/stdout location is the authoritative target signal."
		ret = append(ret, item)
	}
	return ret
}

func (o *RuntimeOrchestrator) runtimeRepairResourceShapeAlignmentContext(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) []map[string]any {
	if o == nil || o.workspaces == nil || run == nil || failedStep == nil || !runtimeRepairNeedsResourceShapeAlignment(failedStep) {
		return nil
	}
	targetRefs := failureTargetRepairRefs(failedStep)
	if len(targetRefs) == 0 {
		return nil
	}
	workspace, err := o.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil || strings.TrimSpace(workspace.DeliverableDirectory) == "" {
		return nil
	}
	files, err := workspaceManifestFiles(workspace.DeliverableDirectory)
	if err != nil || len(files) == 0 {
		return nil
	}
	targetRefs = resolveWorkspaceTargetRefs(workspace.DeliverableDirectory, targetRefs)
	failedSet := map[string]bool{}
	failedDirs := map[string]bool{}
	failedStems := map[string]bool{}
	for _, target := range targetRefs {
		target = normalizeRepairPatchTargetRef(target)
		if target == "" {
			continue
		}
		failedSet[target] = true
		failedDirs[filepath.Dir(target)] = true
		failedStems[normalizedPathStem(target)] = true
	}
	if len(failedDirs) == 0 {
		return nil
	}
	candidates := make([]workspaceDeliverableFile, 0, len(files))
	for _, file := range files {
		path := normalizeRepairPatchTargetRef(file.Path)
		if path == "" || failedSet[path] || !sameDirectoryResourceCandidate(path, failedDirs) {
			continue
		}
		candidates = append(candidates, file)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		leftScore := resourceShapeAlignmentCandidateScore(candidates[i].Path, failedStems)
		rightScore := resourceShapeAlignmentCandidateScore(candidates[j].Path, failedStems)
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		if candidates[i].Size != candidates[j].Size {
			return candidates[i].Size < candidates[j].Size
		}
		return candidates[i].Path < candidates[j].Path
	})
	ret := make([]map[string]any, 0, min(len(candidates), 3))
	for _, candidate := range candidates {
		if len(ret) >= 3 {
			break
		}
		item := patchRepairTargetSnapshot(workspace.DeliverableDirectory, candidate.Path)
		if len(item) == 0 || !boolValue(item["exists"]) {
			continue
		}
		item["source"] = "same_directory_resource_for_shape_conflict"
		item["guidance"] = "Use this current same-directory resource to reconcile resource or contract shape before inventing names, attributes, or contract entries."
		ret = append(ret, item)
	}
	return ret
}

func runtimeRepairNeedsResourceShapeAlignment(failedStep *runModel.Step) bool {
	if failedStep == nil {
		return false
	}
	class := strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeyClass))
	signature := strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeySignature))
	if signals := structuredFailureSignalsFromFailureOutput(failedStep.Output, nil, class, signature); structuredFailureSignalsContainShapeConflict(signals) {
		return true
	}
	for _, key := range []string{"stderr", "stdout", "error", "reason", "summary", "message"} {
		if textContainsShapeConflictDiagnostic(stringFromMap(failedStep.Output, key)) {
			return true
		}
	}
	if failedOutput := mapFromValue(failedStep.Output["failedStepOutput"]); len(failedOutput) > 0 {
		for _, key := range []string{"stderr", "stdout", "error", "reason", "summary", "message"} {
			if textContainsShapeConflictDiagnostic(stringFromMap(failedOutput, key)) {
				return true
			}
		}
	}
	return false
}

func sameDirectoryResourceCandidate(path string, failedDirs map[string]bool) bool {
	path = normalizeRepairPatchTargetRef(path)
	if path == "" || len(failedDirs) == 0 {
		return false
	}
	return failedDirs[filepath.Dir(path)]
}

func resourceShapeAlignmentCandidateScore(path string, failedStems map[string]bool) int {
	path = normalizeRepairPatchTargetRef(path)
	if path == "" {
		return 0
	}
	base := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(base))
	stem := strings.TrimSuffix(base, ext)
	score := 50
	if ext != "" {
		score += 5
	}
	for failedStem := range failedStems {
		if failedStem == "" {
			continue
		}
		switch {
		case stem == failedStem:
			score += 30
		case strings.HasPrefix(failedStem, stem+"_") || strings.HasPrefix(failedStem, stem+"-") || strings.HasPrefix(stem, failedStem+"_") || strings.HasPrefix(stem, failedStem+"-"):
			score += 25
		case strings.Contains(failedStem, stem) || strings.Contains(stem, failedStem):
			score += 10
		}
	}
	if !strings.Contains(stem, "_") && !strings.Contains(stem, "-") && !strings.Contains(stem, ".") {
		score += 5
	}
	return score
}

func normalizedPathStem(path string) string {
	base := strings.ToLower(filepath.Base(normalizeRepairPatchTargetRef(path)))
	if base == "." || base == "" {
		return ""
	}
	return strings.TrimSuffix(base, strings.ToLower(filepath.Ext(base)))
}

func (o *RuntimeOrchestrator) runtimeRepairRootCauseCandidateContext(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) []map[string]any {
	if o == nil || o.workspaces == nil || run == nil || failedStep == nil {
		return nil
	}
	reassessment := repairStrategyReassessmentFromInput(map[string]any{
		"targetRepairAttempts":           o.runtimeRepairAttemptCountForFailureTargetInWindow(ctx, run.ID, strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeyClass)), runtimeFailureTargetScopeFromStep(failedStep), runtimeRepairTargetBudgetWindowRootForFailureTarget(failedStep)),
		stepInputKeyRuntimeRepairAttempt: int64FromValue(failedStep.Input[stepInputKeyRuntimeRepairAttempt]),
		"failureTargetScope":             runtimeFailureTargetScopeFromStep(failedStep),
		failureOutputKeyClass:            strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeyClass)),
		failureOutputKeySignature:        strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeySignature)),
		"failedStepKind":                 failedStep.Kind,
	})
	if !strategyReassessmentRequiresRootCauseTargetShift(reassessment) {
		if inherited := mapFromValue(failedStep.Input["strategyReassessment"]); strategyReassessmentRequiresRootCauseTargetShift(inherited) {
			reassessment = inherited
		}
	}
	includeSurfaceCandidates := runtimeRepairShouldIncludeRootCauseCandidates(failedStep)
	if (len(reassessment) == 0 || !strategyReassessmentRequiresRootCauseTargetShift(reassessment)) && !includeSurfaceCandidates {
		return nil
	}
	workspace, err := o.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil || strings.TrimSpace(workspace.DeliverableDirectory) == "" {
		return nil
	}
	files, err := workspaceManifestFiles(workspace.DeliverableDirectory)
	if err != nil || len(files) == 0 {
		return nil
	}
	failedTargets := resolveWorkspaceTargetRefs(workspace.DeliverableDirectory, failureTargetRepairRefs(failedStep))
	if len(failedTargets) == 0 {
		failedTargets = resolveWorkspaceTargetRefs(workspace.DeliverableDirectory, failureTargetRepairRefsFromStrategyReassessment(reassessment))
	}
	if len(failedTargets) == 0 {
		return nil
	}
	failedSet := map[string]bool{}
	failedDirs := map[string]bool{}
	failedStems := map[string]bool{}
	for _, target := range failedTargets {
		target = normalizeRepairPatchTargetRef(target)
		if target == "" {
			continue
		}
		failedSet[target] = true
		failedDirs[filepath.Dir(target)] = true
		if stem := normalizedPathStem(target); stem != "" {
			failedStems[stem] = true
		}
	}
	if len(failedSet) == 0 {
		return nil
	}
	var ret []map[string]any
	addCandidate := func(path string, source string) {
		path = normalizeRepairPatchTargetRef(path)
		if path == "" || failedSet[path] || !runtimeRepairTargetRefAdmissible(path) {
			return
		}
		for _, existing := range ret {
			if normalizeRepairPatchTargetRef(stringFromMap(existing, "targetRef")) == path {
				return
			}
		}
		item := patchRepairTargetSnapshot(workspace.DeliverableDirectory, path)
		if len(item) == 0 || !boolValue(item["exists"]) || boolValue(item["isBinary"]) {
			return
		}
		item["source"] = source
		item["guidance"] = "Use this as a grounded root-cause candidate resource when repairing the failed surface requires changing an adjacent or referenced current workspace resource."
		ret = append(ret, item)
	}
	signals := structuredFailureSignalsFromFailureOutput(failedStep.Output, nil, strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeyClass)), strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeySignature)))
	for _, target := range stringSliceFromValue(signals["targetRefs"]) {
		addCandidate(resolveWorkspaceTargetRefFromManifest(files, target), "structured_failure_target_ref")
	}
	for _, item := range runtimeRepairContextItemsFromStep(failedStep) {
		addCandidate(stringFromMap(item, "targetRef"), "patch_repair_context")
	}
	sameDirectoryCandidates := make([]workspaceDeliverableFile, 0)
	for _, file := range files {
		path := normalizeRepairPatchTargetRef(file.Path)
		if path == "" || failedSet[path] {
			continue
		}
		if failedDirs[filepath.Dir(path)] {
			sameDirectoryCandidates = append(sameDirectoryCandidates, file)
		}
	}
	sort.SliceStable(sameDirectoryCandidates, func(i, j int) bool {
		leftScore := resourceShapeAlignmentCandidateScore(sameDirectoryCandidates[i].Path, failedStems)
		rightScore := resourceShapeAlignmentCandidateScore(sameDirectoryCandidates[j].Path, failedStems)
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		if sameDirectoryCandidates[i].Size != sameDirectoryCandidates[j].Size {
			return sameDirectoryCandidates[i].Size < sameDirectoryCandidates[j].Size
		}
		return sameDirectoryCandidates[i].Path < sameDirectoryCandidates[j].Path
	})
	for _, file := range sameDirectoryCandidates {
		addCandidate(file.Path, "same_directory_resource")
		if len(ret) >= 6 {
			break
		}
	}
	if len(ret) > 6 {
		return ret[:6]
	}
	return ret
}

func runtimeRepairShouldIncludeRootCauseCandidates(failedStep *runModel.Step) bool {
	if failedStep == nil {
		return false
	}
	if strings.TrimSpace(string(failedStep.Kind)) != "shell/command" {
		return false
	}
	class := strings.TrimSpace(stringFromMap(failedStep.Output, failureOutputKeyClass))
	if class != "" && class != failureClassShellCommand {
		return false
	}
	return len(failureTargetRepairRefs(failedStep)) > 0
}

func failureTargetRepairRefsFromStrategyReassessment(reassessment map[string]any) []string {
	if len(reassessment) == 0 {
		return nil
	}
	var ret []string
	for _, key := range []string{"failureTargetScope", "failedSurface"} {
		ret = append(ret, repairPatchTargetRefsFromScopeText(stringFromMap(reassessment, key))...)
	}
	return compactUniqueStrings(ret)
}

func runtimeRepairContextItemsFromStep(step *runModel.Step) []map[string]any {
	if step == nil || step.Input == nil {
		return nil
	}
	ret := mapSliceFromValue(step.Input["patchRepairContext"])
	ret = append(ret, mapSliceFromValue(step.Input["failureTargetRepairContext"])...)
	return ret
}

func resolveWorkspaceTargetRefFromManifest(files []workspaceDeliverableFile, targetRef string) string {
	targetRef = normalizedFailureTarget(targetRef)
	if targetRef == "" {
		return ""
	}
	for _, file := range files {
		if normalizeRepairPatchTargetRef(file.Path) == normalizeRepairPatchTargetRef(targetRef) {
			return file.Path
		}
	}
	var match string
	for _, file := range files {
		path := normalizeRepairPatchTargetRef(file.Path)
		if strings.HasSuffix(path, "/"+targetRef) || filepath.Base(path) == filepath.Base(targetRef) {
			if match != "" {
				return ""
			}
			match = file.Path
		}
	}
	return match
}

func manifestGroundedFailureSignalTargetRefs(step *runModel.Step, files []workspaceDeliverableFile) []string {
	if step == nil || len(files) == 0 {
		return nil
	}
	class := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass))
	signature := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeySignature))
	texts := repairFailureSignalTexts(step.Output, nil, class, signature)
	if len(texts) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var ret []string
	add := func(value string) {
		resolved := resolveWorkspaceTargetRefFromManifest(files, value)
		resolved = normalizeRepairPatchTargetRef(resolved)
		if resolved == "" || seen[resolved] {
			return
		}
		seen[resolved] = true
		ret = append(ret, resolved)
	}
	for _, text := range texts {
		for _, target := range extractStructuredFailureTargetRefs(text) {
			add(target)
		}
	}
	return ret
}

func resolveWorkspaceTargetRefs(deliverableDirectory string, targetRefs []string) []string {
	if strings.TrimSpace(deliverableDirectory) == "" || len(targetRefs) == 0 {
		return targetRefs
	}
	files, err := workspaceManifestFiles(deliverableDirectory)
	if err != nil || len(files) == 0 {
		return targetRefs
	}
	seen := map[string]bool{}
	ret := make([]string, 0, len(targetRefs))
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, targetRef := range targetRefs {
		resolved := resolveWorkspaceTargetRef(files, targetRef)
		if resolved == "" {
			resolved = targetRef
		}
		add(resolved)
	}
	return ret
}

func resolveWorkspaceTargetRef(files []workspaceDeliverableFile, targetRef string) string {
	targetRef = normalizedFailureTarget(targetRef)
	if targetRef == "" {
		return ""
	}
	for _, file := range files {
		if normalizeRepairPatchTargetRef(file.Path) == normalizeRepairPatchTargetRef(targetRef) {
			return file.Path
		}
	}
	var match string
	for _, file := range files {
		path := normalizeRepairPatchTargetRef(file.Path)
		if path == "" {
			continue
		}
		if strings.HasSuffix(path, "/"+targetRef) || strings.HasSuffix(targetRef, "/"+path) {
			if match != "" {
				return targetRef
			}
			match = file.Path
		}
	}
	if match != "" {
		return match
	}
	return targetRef
}

func workspaceManifestFilesFromMaps(manifest []map[string]any) []workspaceDeliverableFile {
	if len(manifest) == 0 {
		return nil
	}
	ret := make([]workspaceDeliverableFile, 0, len(manifest))
	for _, item := range manifest {
		path := normalizeRepairPatchTargetRef(stringFromMap(item, "path"))
		if path == "" {
			continue
		}
		ret = append(ret, workspaceDeliverableFile{
			Path:       path,
			Size:       int64FromValue(item["size"]),
			Executable: boolValue(item["executable"]),
		})
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func canonicalRepairTargetRefFromManifest(manifest []map[string]any, targetRef string) (string, bool) {
	files := workspaceManifestFilesFromMaps(manifest)
	if len(files) == 0 {
		normalized := normalizeRepairPatchTargetRef(targetRef)
		return normalized, normalized != ""
	}
	normalized := normalizeRepairPatchTargetRef(targetRef)
	if normalized == "" {
		return "", false
	}
	resolved := normalizeRepairPatchTargetRef(resolveWorkspaceTargetRef(files, normalized))
	if resolved == "" {
		return "", false
	}
	if resolved != normalized {
		return resolved, true
	}
	for _, file := range files {
		if normalizeRepairPatchTargetRef(file.Path) == normalized {
			return normalized, true
		}
	}
	return normalized, false
}

func failureTargetRepairRefs(step *runModel.Step) []string {
	if step == nil {
		return nil
	}
	seen := map[string]bool{}
	var ret []string
	add := func(value string) {
		value = normalizeStructuredFailureTargetRef(value)
		if value == "" || !runtimeRepairTargetRefAdmissible(value) || seen[value] {
			return
		}
		seen[value] = true
		ret = append(ret, value)
	}
	for _, target := range failureTargetsForSignature(step.Output) {
		add(target)
	}
	for _, target := range failureTargetsFromSignatureText(stringFromMap(step.Output, failureOutputKeySignature)) {
		add(target)
	}
	for _, target := range failureTargetsFromSignatureText(stringFromMap(step.Input, stepInputKeyRepairOfFailureSig)) {
		add(target)
	}
	for _, target := range failureTargetsFromSignatureText(stringFromMap(step.Input, failureOutputKeySignature)) {
		add(target)
	}
	if reassessment := mapFromValue(step.Input["strategyReassessment"]); len(reassessment) > 0 {
		for _, target := range failureTargetRepairRefsFromStrategyReassessment(reassessment) {
			add(target)
		}
	}
	failedOutput := mapFromValue(step.Output["failedStepOutput"])
	for _, target := range failureTargetsForSignature(failedOutput) {
		add(target)
	}
	for _, target := range failureTargetsFromSignatureText(stringFromMap(failedOutput, failureOutputKeySignature)) {
		add(target)
	}
	class := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass))
	signature := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeySignature))
	if signals := structuredFailureSignalsFromFailureOutput(step.Output, nil, class, signature); len(signals) > 0 {
		for _, target := range stringSliceFromValue(signals["targetRefs"]) {
			add(target)
		}
	}
	return ret
}

func failureTargetsFromSignatureText(signature string) []string {
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return nil
	}
	segments := strings.Split(signature, "|")
	var ret []string
	for _, segment := range segments {
		key, value, ok := strings.Cut(segment, "=")
		if !ok || strings.TrimSpace(key) != "failureTargets" {
			continue
		}
		for _, target := range strings.Split(value, ",") {
			if normalized := normalizedFailureTarget(target); normalized != "" {
				ret = append(ret, normalized)
			}
		}
	}
	return ret
}

func shouldIncludePatchRepairContext(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	switch strings.TrimSpace(step.Kind) {
	case "patch/preview", "patch/apply", "file/edit":
		return true
	}
	if strings.HasPrefix(strings.TrimSpace(stringFromMap(step.Output, "stage")), "patch_") {
		return true
	}
	if strings.HasPrefix(strings.TrimSpace(stringFromMap(step.Output, "stage")), "file_edit") {
		return true
	}
	if strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass)) == failureClassPatchApply {
		return true
	}
	failedOutput := mapFromValue(step.Output["failedStepOutput"])
	if strings.HasPrefix(strings.TrimSpace(stringFromMap(failedOutput, "stage")), "patch_") {
		return true
	}
	failedInput := mapFromValue(step.Output["failedStepInput"])
	switch strings.TrimSpace(stringFromMap(failedInput, "kind")) {
	case "patch/preview", "patch/apply", "file/edit":
		return true
	}
	return false
}

func patchRepairTargetRefs(step *runModel.Step) []string {
	if step == nil {
		return nil
	}
	seen := map[string]bool{}
	add := func(value string) []string {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return nil
		}
		seen[value] = true
		return []string{value}
	}
	var ret []string
	ret = append(ret, add(stringFromMap(step.Input, "targetRef"))...)
	ret = append(ret, add(stringFromMap(step.Input, "path"))...)
	ret = append(ret, add(stringFromMap(step.Output, "targetRef"))...)
	ret = append(ret, add(stringFromMap(step.Output, "path"))...)
	if patches := mapSliceFromValue(step.Output["patches"]); len(patches) > 0 {
		for _, patch := range patches {
			ret = append(ret, add(stringFromMap(patch, "targetRef"))...)
		}
	}
	if failedInput := mapFromValue(step.Output["failedStepInput"]); len(failedInput) > 0 {
		ret = append(ret, add(stringFromMap(failedInput, "targetRef"))...)
		ret = append(ret, add(stringFromMap(failedInput, "path"))...)
	}
	if failedOutput := mapFromValue(step.Output["failedStepOutput"]); len(failedOutput) > 0 {
		ret = append(ret, add(stringFromMap(failedOutput, "targetRef"))...)
		ret = append(ret, add(stringFromMap(failedOutput, "path"))...)
		if patches := mapSliceFromValue(failedOutput["patches"]); len(patches) > 0 {
			for _, patch := range patches {
				ret = append(ret, add(stringFromMap(patch, "targetRef"))...)
			}
		}
	}
	return ret
}

func patchRepairTargetSnapshot(deliverableDirectory string, targetRef string) map[string]any {
	target, err := workspaceTargetPath(deliverableDirectory, targetRef)
	if err != nil {
		return map[string]any{
			"targetRef": targetRef,
			"error":     err.Error(),
		}
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return map[string]any{
			"targetRef": targetRef,
			"exists":    false,
			"error":     err.Error(),
		}
	}
	item := map[string]any{
		"targetRef":   targetRef,
		"exists":      true,
		"size":        len(data),
		"contentHash": sha256HexBytes(data),
		"source":      "current_workspace_file",
		"guidance":    "Use this current target snapshot for repair; failed patch text may be stale or syntactically invalid.",
	}
	if patchRepairLooksBinary(data) {
		item["isBinary"] = true
		item["contentComplete"] = false
		item["safeForFileWrite"] = false
		return item
	}
	content := string(data)
	contentComplete := true
	if len(content) > maxPatchRepairSnapshotBytes {
		content = content[:maxPatchRepairSnapshotBytes]
		item["truncated"] = true
		contentComplete = false
	}
	item["content"] = content
	item["contentComplete"] = contentComplete
	item["redacted"] = containsRuntimeRedactionMarker(content)
	item["safeForFileWrite"] = contentComplete && !containsRuntimeRedactionMarker(content)
	return item
}

func patchRepairRequestsFromStep(step *runModel.Step) []runtimePatchRequest {
	if step == nil {
		return nil
	}
	var ret []runtimePatchRequest
	add := func(value map[string]any) {
		if len(value) == 0 {
			return
		}
		if strings.TrimSpace(stringFromMap(value, "patchType")) == "" &&
			strings.TrimSpace(stringFromMap(value, "operation")) == "" &&
			strings.TrimSpace(stringFromMap(value, "diff")) != "" {
			value = cloneStepPayload(value)
			value["patchType"] = "unified_diff"
		}
		request, err := runtimePatchRequestFromMap(value)
		if err != nil {
			return
		}
		ret = append(ret, request)
	}
	add(step.Input)
	add(step.Output)
	for _, patch := range mapSliceFromValue(step.Output["patches"]) {
		add(patch)
	}
	if failedInput := mapFromValue(step.Output["failedStepInput"]); len(failedInput) > 0 {
		add(failedInput)
	}
	if failedOutput := mapFromValue(step.Output["failedStepOutput"]); len(failedOutput) > 0 {
		add(failedOutput)
		for _, patch := range mapSliceFromValue(failedOutput["patches"]) {
			add(patch)
		}
	}
	return ret
}

func patchRepairSuggestionsForDiff(deliverableDirectory string, targetRef string, diff string) []map[string]any {
	target, err := workspaceTargetPath(deliverableDirectory, targetRef)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(target)
	if err != nil || patchRepairLooksBinary(data) {
		return nil
	}
	filePatch, err := parseSingleFileUnifiedDiff(diff)
	if err != nil || !diffTargetMatches(targetRef, filePatch) {
		return nil
	}
	content := string(data)
	contentHash := sha256HexBytes(data)
	var ret []map[string]any
	seen := map[string]bool{}
	for _, hunk := range filePatch.Hunks {
		for _, replacement := range patchRepairReplacementCandidates(hunk, content) {
			oldText := replacement["oldText"]
			newText := replacement["newText"]
			if oldText == "" || oldText == newText {
				continue
			}
			if containsRuntimeRedactionMarker(oldText) || containsRuntimeRedactionMarker(newText) {
				continue
			}
			if strings.Count(content, oldText) != 1 {
				continue
			}
			key := oldText + "\x00" + newText
			if seen[key] {
				continue
			}
			seen[key] = true
			ret = append(ret, map[string]any{
				"targetRef":      targetRef,
				"patchType":      "unified_diff",
				"oldText":        oldText,
				"newText":        newText,
				"contentHash":    contentHash,
				"matchCount":     1,
				"source":         replacement["source"],
				"guidance":       "Use this exact current oldText/newText replacement to construct the smallest unified_diff or file/edit repair; do not copy redacted or truncated snapshot content.",
				"safeForMinimal": true,
			})
		}
	}
	return ret
}

func patchRepairAlreadySatisfiedSignalForDiff(deliverableDirectory string, targetRef string, diff string) map[string]any {
	target, err := workspaceTargetPath(deliverableDirectory, targetRef)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(target)
	if err != nil || patchRepairLooksBinary(data) {
		return nil
	}
	filePatch, err := parseSingleFileUnifiedDiff(diff)
	if err != nil || !diffTargetMatches(targetRef, filePatch) {
		return nil
	}
	content := string(data)
	addedLines, deletedLines := patchRepairChangedLines(filePatch)
	if len(addedLines) == 0 && len(deletedLines) == 0 {
		return nil
	}
	if patchRepairLinesAlreadySatisfied(content, addedLines, deletedLines) {
		return map[string]any{
			"targetRef":          targetRef,
			"patchType":          "unified_diff",
			"contentHash":        sha256HexBytes(data),
			"addedLineCount":     len(addedLines),
			"deletedLineCount":   len(deletedLines),
			"safeForValidation":  true,
			"reason":             "current_target_already_matches_failed_patch_intent",
			"guidance":           "The current target snapshot already reflects the failed patch's intended line-level state. Do not repeat the stale patch; run bounded concrete validation or return a failed assessment/blocker.",
			"mechanicalSignal":   true,
			"businessJudgment":   false,
			"matchedCurrentFile": true,
		}
	}
	return nil
}

func patchRepairChangedLines(filePatch unifiedDiffFilePatch) (added []string, deleted []string) {
	seenAdded := map[string]bool{}
	seenDeleted := map[string]bool{}
	add := func(dst *[]string, seen map[string]bool, value string) {
		if strings.TrimSpace(value) == "" || containsRuntimeRedactionMarker(value) {
			return
		}
		if seen[value] {
			return
		}
		seen[value] = true
		*dst = append(*dst, value)
	}
	for _, hunk := range filePatch.Hunks {
		for _, line := range hunk.Lines {
			if len(line) == 0 {
				continue
			}
			switch line[0] {
			case '+':
				add(&added, seenAdded, line[1:])
			case '-':
				add(&deleted, seenDeleted, line[1:])
			}
		}
	}
	return added, deleted
}

func patchRepairLinesAlreadySatisfied(content string, added []string, deleted []string) bool {
	for _, line := range added {
		if !patchRepairContentContainsLine(content, line) {
			return false
		}
	}
	for _, line := range deleted {
		if patchRepairContentContainsLine(content, line) {
			return false
		}
	}
	return len(added) > 0 || len(deleted) > 0
}

func patchRepairContentContainsLine(content string, expected string) bool {
	expected = strings.TrimRight(expected, "\r")
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimRight(line, "\r") == expected {
			return true
		}
	}
	return false
}

func patchRepairReplacementText(hunk unifiedDiffHunk) (string, string) {
	var oldLines []string
	var newLines []string
	for _, line := range hunk.Lines {
		if line == "" {
			continue
		}
		switch line[0] {
		case '-':
			oldLines = append(oldLines, line[1:])
		case '+':
			newLines = append(newLines, line[1:])
		}
	}
	return strings.Join(oldLines, "\n"), strings.Join(newLines, "\n")
}

func patchRepairReplacementCandidates(hunk unifiedDiffHunk, content string) []map[string]string {
	var ret []map[string]string
	hasAddition := false
	hasDeletion := false
	for _, line := range hunk.Lines {
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case '+':
			hasAddition = true
		case '-':
			hasDeletion = true
		}
	}
	add := func(oldText string, newText string, source string) {
		oldText = strings.TrimSuffix(oldText, "\n")
		newText = strings.TrimSuffix(newText, "\n")
		if oldText == "" || oldText == newText {
			return
		}
		ret = append(ret, map[string]string{
			"oldText": oldText,
			"newText": newText,
			"source":  source,
		})
	}
	oldText, newText := patchRepairReplacementText(hunk)
	add(oldText, newText, "failed_patch_current_snapshot_unique_replacement")

	if strings.Count(content, oldText) != 1 {
		sourceLines, sourceHadFinalNewline := splitTextLines(content)
		start, err := unifiedDiffHunkStart(sourceLines, 0, hunk)
		if err == nil {
			oldLines := unifiedDiffHunkOldLines(hunk)
			if start >= 0 && start+len(oldLines) <= len(sourceLines) {
				nextLines := append([]string(nil), sourceLines[:start]...)
				for _, line := range hunk.Lines {
					if line == "" {
						continue
					}
					switch line[0] {
					case ' ', '+':
						nextLines = append(nextLines, line[1:])
					}
				}
				nextLines = append(nextLines, sourceLines[start+len(oldLines):]...)
				nextContent := joinTextLines(nextLines, sourceHadFinalNewline)
				add(content, nextContent, "failed_patch_current_snapshot_full_hunk_replay")
			}
		}
	}

	if hasAddition || !hasDeletion {
		return ret
	}
	for _, line := range hunk.Lines {
		if len(line) < 2 || line[0] != '-' {
			continue
		}
		oldLine := line[1:]
		if strings.Count(content, oldLine) == 1 {
			add(oldLine, "", "failed_patch_current_snapshot_unique_line_removal")
		}
	}
	return ret
}

func patchRepairLooksBinary(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func (o *RuntimeOrchestrator) convertedDecisionReadableResourceContext(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) []map[string]any {
	if o == nil || o.workspaces == nil || run == nil || failedStep == nil {
		return nil
	}
	decision := mapFromValue(failedStep.Output["convertedDecision"])
	context := mapFromValue(decision["context"])
	workspace, err := o.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil || strings.TrimSpace(workspace.DeliverableDirectory) == "" {
		return nil
	}
	context = o.enrichDecisionContextWithReadableWorkspaceResources(ctx, run, workspace, context, stringFromMap(decision, "question"), stringFromMap(decision, "reason"))
	if !knownReadableWorkspaceResourceDecisionContext(context) {
		return nil
	}
	resources := referencedReadableWorkspaceResources(context)
	if len(resources) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, min(len(resources), 5))
	for idx, targetRef := range resources {
		if idx >= 5 {
			break
		}
		item := patchRepairTargetSnapshot(workspace.DeliverableDirectory, targetRef)
		if len(item) == 0 {
			continue
		}
		item["source"] = "converted_decision_known_workspace_resource"
		item["guidance"] = "This resource was already known to the runtime; use it for automated repair instead of requesting manual input."
		ret = append(ret, item)
	}
	return ret
}

func (o *RuntimeOrchestrator) enrichDecisionContextWithReadableWorkspaceResources(
	ctx context.Context,
	run *runModel.Run,
	workspace *workspaceModel.Workspace,
	context map[string]any,
	decisionQuestion string,
	decisionReason string,
) map[string]any {
	if o == nil || run == nil {
		return context
	}
	if workspace == nil && o.workspaces != nil {
		if loaded, err := o.workspaces.Get(ctx, run.WorkspaceID); err == nil {
			workspace = loaded
		}
	}
	deliverableDirectory := ""
	if workspace != nil {
		deliverableDirectory = strings.TrimSpace(workspace.DeliverableDirectory)
	}
	if deliverableDirectory == "" {
		return context
	}
	fields := readableResourceReferenceTexts(context)
	if decisionQuestion = strings.TrimSpace(decisionQuestion); decisionQuestion != "" {
		fields = append(fields, decisionQuestion)
	}
	if decisionReason = strings.TrimSpace(decisionReason); decisionReason != "" {
		fields = append(fields, decisionReason)
	}
	if len(fields) == 0 {
		return context
	}
	manifest := workspaceFileManifestFromDirectory(deliverableDirectory)
	if len(manifest) == 0 {
		return context
	}
	var resources []string
	seen := map[string]bool{}
	for _, item := range manifest {
		path := stringFromMap(item, "path")
		if path == "" {
			continue
		}
		for _, field := range fields {
			if !decisionFieldReferencesKnownResource(field, []string{path}) {
				continue
			}
			if !seen[path] {
				seen[path] = true
				resources = append(resources, path)
			}
			break
		}
	}
	if len(resources) == 0 {
		return context
	}
	enriched := cloneStepPayload(context)
	if enriched == nil {
		enriched = map[string]any{}
	}
	if decisionQuestion != "" {
		enriched[decisionContextKeyQuestion] = decisionQuestion
	}
	if decisionReason != "" {
		enriched[decisionContextKeyReason] = decisionReason
	}
	existing := stringListFromValue(enriched["readableResources"])
	for _, resource := range resources {
		if !stringSliceContains(existing, resource) {
			existing = append(existing, resource)
		}
	}
	enriched["readableResources"] = existing
	return enriched
}

func stringSliceContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func referencedReadableWorkspaceResources(context map[string]any) []string {
	resources := knownReadableWorkspaceResourceNames(context)
	if len(resources) == 0 {
		return nil
	}
	fields := readableResourceReferenceTexts(context)
	seen := map[string]bool{}
	var ret []string
	for _, resource := range resources {
		for _, field := range fields {
			if !decisionFieldReferencesKnownResource(field, []string{resource}) {
				continue
			}
			if !seen[resource] {
				seen[resource] = true
				ret = append(ret, resource)
			}
			break
		}
	}
	return ret
}

func appendRepairGuidance(existing any, items ...string) []string {
	ret := stringListFromValue(existing)
	seen := map[string]bool{}
	compact := make([]string, 0, len(ret)+len(items))
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		compact = append(compact, value)
	}
	for _, item := range ret {
		add(item)
	}
	for _, item := range items {
		add(item)
	}
	return compact
}

func shouldIncludeWorkspaceManifestInRuntimeRepair(failedStep *runModel.Step) bool {
	if failedStep == nil || failedStep.Output == nil {
		return false
	}
	if len(failureTargetRepairRefs(failedStep)) > 0 {
		return true
	}
	if stringFromMap(failedStep.Output, failureOutputKeyClass) == failureClassWorkspacePath {
		return true
	}
	diagnostics := mapFromValue(failedStep.Output["runtimeDiagnostics"])
	if stringFromMap(diagnostics, "kind") == "workspace_path_conflict" {
		return true
	}
	failedOutput := mapFromValue(failedStep.Output["failedStepOutput"])
	if stringFromMap(failedOutput, failureOutputKeyClass) == failureClassWorkspacePath {
		return true
	}
	if diagnostics := mapFromValue(failedOutput["runtimeDiagnostics"]); stringFromMap(diagnostics, "kind") == "workspace_path_conflict" {
		return true
	}
	if boolValue(failedStep.Input["stateChangingOutputRequired"]) &&
		(failedStepInputHasMissingDoDItems(failedStep) ||
			len(missingDoDItemsFromFailureOutput(failedStep.Output)) > 0 ||
			runtimeRepairInputHasResourceEvidence(failedStep)) {
		return true
	}
	return false
}

func runtimeRepairInputHasResourceEvidence(failedStep *runModel.Step) bool {
	if failedStep == nil {
		return false
	}
	for _, evidence := range mapSliceFromValue(failedStep.Input[stepInputKeyGraphEvidence]) {
		if strings.TrimSpace(stringFromMap(evidence, "kind")) == "file/list" {
			return true
		}
		output := mapFromValue(evidence["output"])
		if strings.TrimSpace(stringFromMap(output, "stage")) == "file_list" || len(mapSliceFromValue(output["entries"])) > 0 {
			return true
		}
	}
	return false
}

func missingDoDItemsFromFailureOutput(output map[string]any) []map[string]any {
	if len(output) == 0 {
		return nil
	}
	if items := mapSliceFromValue(output["missingDoDItems"]); len(items) > 0 {
		return items
	}
	if diagnostics := mapFromValue(output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		if items := mapSliceFromValue(diagnostics["missingDoDItems"]); len(items) > 0 {
			return items
		}
	}
	if failedOutput := mapFromValue(output["failedStepOutput"]); len(failedOutput) > 0 {
		if items := missingDoDItemsFromFailureOutput(failedOutput); len(items) > 0 {
			return items
		}
	}
	return nil
}

func (o *RuntimeOrchestrator) recentRuntimeRepairAttemptSummaries(ctx context.Context, runID common.RunID, failedStep *runModel.Step, limit int) []map[string]any {
	if o == nil || o.steps == nil || runID == "" || failedStep == nil || limit <= 0 {
		return nil
	}
	steps, err := o.steps.ListByRun(ctx, runID)
	if err != nil || len(steps) == 0 {
		return nil
	}
	steps = orderedRuntimeContextSteps(steps)
	scope := runtimeRepairScopeFromFailedStep(failedStep)
	targetScope := runtimeFailureTargetScopeFromStep(failedStep)
	windowRoot := runtimeRepairTargetBudgetWindowRootForFailureTarget(failedStep)
	failureClass := strings.TrimSpace(scope.failureClass)
	ret := make([]map[string]any, 0, limit)
	for _, step := range steps {
		if !isRuntimeRepairBudgetStep(runID, step) {
			continue
		}
		if targetScope != "" {
			if runtimeFailureTargetScopeFromStep(step) != targetScope {
				continue
			}
			if windowRoot != "" && runtimeRepairTargetBudgetWindowRoot(step) != windowRoot {
				continue
			}
			if failureClass != "" {
				stepClass := strings.TrimSpace(firstNonEmptyString(
					stringFromMap(step.Input, stepInputKeyRepairOfFailureClass),
					stringFromMap(step.Input, failureOutputKeyClass),
					stringFromMap(step.Output, failureOutputKeyClass),
				))
				if stepClass != "" && stepClass != failureClass {
					continue
				}
			}
		} else if !runtimeRepairStepMatchesScope(step, scope) {
			continue
		}
		item := map[string]any{
			"stepId":       string(step.ID),
			"state":        string(step.State),
			"attempt":      runtimeRepairAttemptValue(step),
			"failureClass": stringFromMap(step.Output, failureOutputKeyClass),
			"failureStage": stringFromMap(step.Output, "stage"),
		}
		if summary := runtimeRepairAttemptFailureSummary(step); summary != "" {
			item["failureSummary"] = summary
		}
		if rejectedLLMDecisionOutputForProgress(step) {
			item["rejectedOutputSummaryOmitted"] = true
		}
		if missing := missingDoDItemsFromFailureOutput(step.Output); len(missing) > 0 {
			item["missingDoDItems"] = missing
		}
		ret = append(ret, item)
	}
	if len(ret) == 0 {
		return nil
	}
	if len(ret) > limit {
		ret = ret[len(ret)-limit:]
	}
	return ret
}

func runtimeRepairAttemptFailureSummary(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	if rejectedLLMDecisionOutputForProgress(step) {
		return firstNonEmptyString(stringFromMap(step.Output, "error"), stringFromMap(step.Output, "reason"), stringFromMap(step.Output, failureOutputKeySignature))
	}
	return firstNonEmptyString(stringFromMap(step.Output, "summary"), stringFromMap(step.Output, "error"), stringFromMap(step.Output, "reason"))
}

func runtimeRepairAttemptValue(step *runModel.Step) int {
	if step == nil {
		return 0
	}
	switch typed := step.Input[stepInputKeyRuntimeRepairAttempt].(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func (o *RuntimeOrchestrator) workspaceFileManifest(ctx context.Context, run *runModel.Run) []map[string]any {
	if o == nil || o.workspaces == nil || run == nil {
		return nil
	}
	workspace, err := o.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil {
		return nil
	}
	return workspaceFileManifestFromDirectory(workspace.DeliverableDirectory)
}

func workspaceFileManifestFromDirectory(deliverableDirectory string) []map[string]any {
	files, err := workspaceManifestFiles(deliverableDirectory)
	if err != nil || len(files) == 0 {
		return nil
	}
	manifest := make([]map[string]any, 0, len(files))
	for _, file := range files {
		item := map[string]any{
			"path": file.Path,
			"kind": "file",
			"size": file.Size,
		}
		if file.Executable {
			item["executable"] = true
		}
		manifest = append(manifest, item)
	}
	return manifest
}

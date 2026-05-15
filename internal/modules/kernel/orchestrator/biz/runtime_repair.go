package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const maxPatchRepairSnapshotBytes = 8192

const (
	repairObjectiveStateChangingOutput        = "produce_state_changing_output"
	legacyRepairObjectiveStateChangingImpl    = "produce_state_changing_implementation"
	repairObjectiveConcreteValidationEvidence = "produce_concrete_validation_evidence"
	repairObjectiveTransientExecutionRetry    = "retry_or_adjust_transient_execution"
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
		return true
	}
	if strings.TrimSpace(stringFromMap(assessment, "summary")) == "" {
		return true
	}
	return false
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
	case runtimeRecoveryActionRuntimeRepair:
		attempt := o.runtimeRepairAttemptCountForStep(ctx, run.ID, failedStep) + 1
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
	runtimeRecoveryActionProviderRetry          runtimeRecoveryAction = "provider_retry"
	runtimeRecoveryActionProviderRetryExhausted runtimeRecoveryAction = "provider_retry_exhausted"
	runtimeRecoveryActionRuntimeRepairExhausted runtimeRecoveryAction = "runtime_repair_exhausted"
	runtimeRecoveryActionDuplicateFailure       runtimeRecoveryAction = "duplicate_failure"
	runtimeRecoveryActionRuntimeRepair          runtimeRecoveryAction = "runtime_repair"
	runtimeRecoveryActionFailRun                runtimeRecoveryAction = "fail_run"
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
	}
	return o.FailRun(ctx, run.ID, reason)
}

func (o *RuntimeOrchestrator) shouldEscalateRuntimeRepairExhaustion(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) bool {
	if o == nil || run == nil || failedStep == nil || o.tickets == nil {
		return false
	}
	if !requiresDoDAssessmentCompletionGate(run) || isLLMProviderRuntimeFailure(run, failedStep) {
		return false
	}
	return o.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep) >= maxRuntimeRepairAttempts
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
	reason := localizedRuntimeRepairExhaustionReason(language, maxRuntimeRepairAttempts, execErr)
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
			"maxAttempts":                    maxRuntimeRepairAttempts,
			"failureClass":                   stringFromMap(failedStep.Output, failureOutputKeyClass),
			"failureSignature":               stringFromMap(failedStep.Output, failureOutputKeySignature),
			"failureTargetScope":             runtimeFailureTargetScopeFromStep(failedStep),
			"failedStepId":                   string(failedStep.ID),
			"repairOfRootStepId":             string(runtimeRepairRootStepID(failedStep)),
			"failedStepKind":                 failedStep.Kind,
			"failedStepInput":                compactStepInputForLLMContext(failedStep.Input),
			"failedStepOutput":               compactStepOutputForLLMContext(failedStep.Output),
			"missingDoDItems":                missingDoDItemsFromFailureOutput(failedStep.Output),
			"recentRepairAttempts":           o.recentRuntimeRepairAttemptSummaries(ctx, run.ID, failedStep, maxRuntimeRepairAttempts),
			"recommendedNextActions": []string{
				"Map each missing DoD item to concrete affected files, patch operations, and verification commands.",
				"Prioritize minimal implementation and test updates, then rerun targeted verification.",
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
	if o.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep) >= maxRuntimeRepairAttempts {
		return false
	}
	return !isLLMProviderRuntimeFailure(run, failedStep)
}

func (o *RuntimeOrchestrator) shouldAttemptProviderRetry(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) bool {
	if o == nil || run == nil || failedStep == nil || o.runtime == nil || o.steps == nil {
		return false
	}
	if !isRetryableLLMProviderRuntimeFailure(run, failedStep) {
		return false
	}
	return o.providerRetryAttemptCount(ctx, run.ID, providerRetryRootStepID(failedStep)) < o.maxProviderRetryAttempts()
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
		return exactAttempts
	}
	class, _ := runtimeRepairScopeClassAndSignature(failedStep)
	targetAttempts := o.runtimeRepairAttemptCountForFailureTargetInWindow(ctx, runID, class, targetScope, runtimeRepairTargetBudgetWindowRoot(failedStep))
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
	return attempts > 0 && attempts < maxRuntimeRepairAttempts
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
	if attempts, ok := ticket.Context["attempts"]; !ok || intFromAny(attempts) < maxRuntimeRepairAttempts {
		return false
	}
	failedStep, err := o.steps.Get(ctx, ticket.StepID)
	if err != nil || failedStep == nil || failedStep.State != common.StepStateFailed {
		return false
	}
	if isLLMProviderRuntimeFailure(run, failedStep) {
		return false
	}
	if runtimeFailureTargetScopeFromStep(failedStep) == "" || runtimeRepairTargetBudgetWindowRoot(failedStep) == "" {
		return false
	}
	return o.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep) < maxRuntimeRepairAttempts
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

func runtimeFailureTargetScope(signature string) string {
	signature = strings.TrimSpace(signature)
	if signature == "" {
		return ""
	}
	parts := strings.Split(signature, "|")
	for _, key := range []string{"failureTargets", "targetRef", "conflictRef", "serverId", "tool", "uri"} {
		for _, part := range parts {
			name, value, ok := strings.Cut(part, "=")
			if !ok || strings.TrimSpace(name) != key {
				continue
			}
			value = normalizeRuntimeFailureTargetValue(value)
			if value == "" {
				continue
			}
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
	return ok && satisfied
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
	input["maxRuntimeRepairAttempts"] = maxRuntimeRepairAttempts
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
	concreteEvidenceRepair := !assessmentOnlyCoverageRepair && doDVerificationEvidenceRepairRequired(failedStep)
	if concreteEvidenceRepair {
		input["repairObjective"] = repairObjectiveConcreteValidationEvidence
		input["evidenceRepairRequired"] = true
		input["verificationEvidenceGap"] = map[string]any{
			"kind":           "dod_verification_evidence_gap",
			"failedStepId":   string(failedStep.ID),
			"failedStepKind": string(failedStep.Kind),
			"missingDoD":     missingDoDItemsFromFailureOutput(failedStep.Output),
		}
		input["repairGuidance"] = concreteValidationEvidenceRepairGuidance()
		delete(input, "stateChangingOutputRequired")
	}
	if assessmentOnlyCoverageRepair {
		input[stepInputKeySoulStage] = soulStageValidation
		setRuntimeStage(input, runtimeStageVerification)
		input[stepInputKeyAssessmentOnly] = true
		input["completionFollowUp"] = true
		input["assessmentOnlyReason"] = "dod_coverage_contract_correction"
		input["repairObjective"] = "complete_dod_goal_coverage_assessment"
		input["repairGuidance"] = []string{
			"Return only completed or failed dodAssessment, or a real governance blocker.",
			"Do not return executionGraph, shellCommands, patchRequests, mcpToolCalls, memoryWrites, or other executable work.",
			"Preserve the current DoD item assessment when still valid and add missing Goal coverage mapping with concrete evidenceRefs.",
			"If evidence does not support a Goal or DoD item, mark it failed/skipped with reason, residualRisks, and nextActions instead of claiming satisfaction.",
		}
		if missingGoalCoverage := missingGoalCoverageFromFailureOutput(failedStep.Output); len(missingGoalCoverage) > 0 {
			input["missingGoalCoverage"] = missingGoalCoverage
		}
	}
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
	if missingDoD := missingDoDItemsFromFailureOutput(failedStep.Output); len(missingDoD) > 0 {
		input["missingDoDItems"] = missingDoD
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
	targetScope := runtimeFailureTargetScope(signature)
	if targetScope != "" {
		input["failureTargetScope"] = targetScope
		input["targetRepairAttempts"] = o.runtimeRepairAttemptCountForFailureTargetInWindow(ctx, run.ID, class, targetScope, runtimeRepairTargetBudgetWindowRoot(failedStep))
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
	patchContext := o.runtimeRepairPatchContext(ctx, run, failedStep)
	if len(patchContext) > 0 {
		input["patchRepairContext"] = patchContext
		if suggestions := o.runtimeRepairPatchSuggestions(ctx, run, failedStep); len(suggestions) > 0 {
			input["patchRepairSuggestions"] = suggestions
		}
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Use patchRepairContext as the current target snapshot for patch repair; treat the failed patch text as stale or untrusted.",
			"Use complete patch/apply file_write only when the current snapshot is marked safeForFileWrite/contentComplete and contains no redaction markers; otherwise use a minimal unified_diff with exact visible context, file/edit with exact oldText, or first read the target.",
			"Never copy [REDACTED], [SECRET], [MASKED], omitted sections, or truncated snapshot text into patch/apply content or diff.",
			"If patchRepairSuggestions are present, prefer their exact oldText/newText replacement evidence to construct the smallest current unified_diff or file/edit repair.",
			"If the current target snapshot proves the failed patch is no longer required, return a bounded verification evidence graph instead of another patch; do not use read-only inspection as the only evidence.",
		)
	}
	targetContext := o.runtimeRepairFailureTargetContext(ctx, run, failedStep)
	if len(targetContext) > 0 {
		input["failureTargetRepairContext"] = targetContext
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Use failureTargetRepairContext as the current snapshot for targets identified by the failed evidence.",
			"When failed evidence points at a concrete target resource, generate the smallest auditable patch/apply state change against that current snapshot before rerunning verification.",
			"Use workspaceFileManifest to reconcile target resources with other known workspace resources; avoid replacing unrelated existing artifacts just to satisfy one failure target.",
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
		narrowStateChangingRecoveryInput(input, failedStep, execErr, "runtime_repair")
	}
	if context := o.convertedDecisionReadableResourceContext(ctx, run, failedStep); len(context) > 0 {
		input["readableResourceRepairContext"] = context
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"The previous decision requested information that is already available as a readable workspace resource.",
			"Use readableResourceRepairContext as current evidence; do not ask the operator to paste the same resource content.",
		)
	}
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
	if inheritedTarget || inheritedPatch || inheritedReadable || inheritedManifest {
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Preserve inherited repair context from the previous repair attempt; it contains the current target snapshots needed for direct state-changing recovery.",
			"Use inherited target snapshots and workspace manifest to produce patch/apply artifacts instead of retrying rejected shell-only or read-only recovery output.",
			"Do not generate file_write from inherited snapshots unless the snapshot is complete and not redacted; use exact minimal diffs or file/read when content is incomplete.",
		)
	}
}

func stateChangingRepairGuidance() []string {
	return []string{
		"Return concrete state-changing output with complete content, or return a failed dodAssessment if the required state change cannot be completed.",
		"If multiple dependent operations are needed, executionGraph is allowed only when it contains at least one valid patch/apply node that does not depend on llm/workspace_execution before the patch.",
		"Do not return read-only, inspection-only, shell-only executionGraph, or LLM-gated executionGraph.",
		"patch/apply supports file_write and unified_diff only; file_write content must be non-empty and must not be used as a delete operation.",
		"When changing an existing target file, prefer the smallest unified_diff or exact replacement over file_write; use file_write primarily to create missing targets or replace a known full file snapshot intentionally.",
		"Do not use shell/command to modify workspace files when a patch/apply state-change artifact can represent the change; use shell/command for bounded preparation or verification after the patch.",
		"When failedStepOutput identifies concrete target resources, treat those target snapshots as authoritative and repair the target directly instead of changing unrelated prerequisite metadata.",
		"When a workspace already contains known resources, reconcile the repair with those existing resources and prefer the smallest coherent state change over broad replacement.",
		"When repair context contains redacted or truncated content, do not copy it into patch/apply; first read the target or produce a minimal exact diff from visible current evidence.",
		"Do not return self-inspection, run snapshot, observability, or context rehydration graphs when effective Goal and DoD are already present.",
		"If a target file is missing in an empty or new workspace, create it with patch/apply file_write instead of reading it again.",
		"Do not use placeholders such as __FROM_PREVIOUS_STEP_OUTPUT__ or <<...>>.",
	}
}

func concreteValidationEvidenceRepairGuidance() []string {
	return []string{
		"Do not repeat assessment-only validation.",
		"Produce concrete verification evidence for the failed Goal/DoD coverage before changing implementation.",
		"Return a bounded executionGraph that runs available verification capabilities, or return a failed dodAssessment/blocker if concrete evidence cannot be produced.",
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
	if preview := streamEventsPreview(mapSliceFromValue(output["streamEvents"]), 1200); preview != "" {
		ret["partialOutputPreview"] = preview
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
	case repairObjectiveStateChangingOutput, legacyRepairObjectiveStateChangingImpl:
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
		"implementation planning llm step returned read-only executiongraph without patch output",
		"validation follow-up llm step returned read-only executiongraph without patch output",
		"execution-stage llm graph node returned executiongraph",
		"state-changing llm repair step returned executiongraph",
		"state-changing llm repair step returned no state-changing output",
		"state-changing llm repair step returned executiongraph without patch/apply",
		"state-changing execution-stage llm graph node returned no state-changing output",
		"implementation execution-stage llm graph node returned no state-changing output",
		"state-changing executiongraph patch/apply depends on llm/workspace_execution before patch",
		"planning executiongraph patch/apply depends on llm/workspace_execution before patch",
		"targetref is required",
		"targetref must be set",
		"patch content contains unresolved runtime template",
		"patch diff contains unresolved runtime template",
		"patch content is required",
		"patch diff is required",
		"patch_dispatch_invalid",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
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
	if len(missingDoDItemsFromFailureOutput(failedStep.Output)) == 0 && len(missingGoalCoverageFromFailureOutput(failedStep.Output)) == 0 {
		return false
	}
	diagnostics := mapFromValue(failedStep.Output["runtimeDiagnostics"])
	return strings.TrimSpace(stringFromMap(diagnostics, "kind")) == "dod_evidence_quality_gap"
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
	if err != nil || workspace == nil || strings.TrimSpace(workspace.WorkingDirectory) == "" {
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
		item := patchRepairTargetSnapshot(workspace.WorkingDirectory, targetRef)
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
	if err != nil || workspace == nil || strings.TrimSpace(workspace.WorkingDirectory) == "" {
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
		ret = append(ret, patchRepairSuggestionsForDiff(workspace.WorkingDirectory, request.TargetRef, request.Diff)...)
	}
	if len(ret) > 5 {
		return ret[:5]
	}
	return ret
}

func (o *RuntimeOrchestrator) runtimeRepairFailureTargetContext(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) []map[string]any {
	if o == nil || o.workspaces == nil || run == nil || failedStep == nil {
		return nil
	}
	targetRefs := failureTargetRepairRefs(failedStep)
	if len(targetRefs) == 0 {
		return nil
	}
	workspace, err := o.workspaces.Get(ctx, run.WorkspaceID)
	if err != nil || workspace == nil || strings.TrimSpace(workspace.WorkingDirectory) == "" {
		return nil
	}
	targetRefs = resolveWorkspaceTargetRefs(workspace.WorkingDirectory, targetRefs)
	ret := make([]map[string]any, 0, min(len(targetRefs), 3))
	for idx, targetRef := range targetRefs {
		if idx >= 3 {
			break
		}
		item := patchRepairTargetSnapshot(workspace.WorkingDirectory, targetRef)
		if len(item) == 0 {
			continue
		}
		item["source"] = "failure_target_current_workspace_file"
		item["guidance"] = "Use this current failure target snapshot for repair; failed stderr/stdout location is the authoritative target signal."
		ret = append(ret, item)
	}
	return ret
}

func resolveWorkspaceTargetRefs(workingDirectory string, targetRefs []string) []string {
	if strings.TrimSpace(workingDirectory) == "" || len(targetRefs) == 0 {
		return targetRefs
	}
	files, err := workspaceManifestFiles(workingDirectory)
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
		if file.Path == targetRef {
			return targetRef
		}
	}
	var match string
	for _, file := range files {
		if strings.HasSuffix(file.Path, "/"+targetRef) {
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

func failureTargetRepairRefs(step *runModel.Step) []string {
	if step == nil {
		return nil
	}
	seen := map[string]bool{}
	var ret []string
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
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
	failedOutput := mapFromValue(step.Output["failedStepOutput"])
	for _, target := range failureTargetsForSignature(failedOutput) {
		add(target)
	}
	for _, target := range failureTargetsFromSignatureText(stringFromMap(failedOutput, failureOutputKeySignature)) {
		add(target)
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
	case "patch/preview", "patch/apply":
		return true
	}
	if strings.HasPrefix(strings.TrimSpace(stringFromMap(step.Output, "stage")), "patch_") {
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
	case "patch/preview", "patch/apply":
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
	ret = append(ret, add(stringFromMap(step.Output, "targetRef"))...)
	if patches := mapSliceFromValue(step.Output["patches"]); len(patches) > 0 {
		for _, patch := range patches {
			ret = append(ret, add(stringFromMap(patch, "targetRef"))...)
		}
	}
	if failedInput := mapFromValue(step.Output["failedStepInput"]); len(failedInput) > 0 {
		ret = append(ret, add(stringFromMap(failedInput, "targetRef"))...)
	}
	if failedOutput := mapFromValue(step.Output["failedStepOutput"]); len(failedOutput) > 0 {
		ret = append(ret, add(stringFromMap(failedOutput, "targetRef"))...)
		if patches := mapSliceFromValue(failedOutput["patches"]); len(patches) > 0 {
			for _, patch := range patches {
				ret = append(ret, add(stringFromMap(patch, "targetRef"))...)
			}
		}
	}
	return ret
}

func patchRepairTargetSnapshot(workingDirectory string, targetRef string) map[string]any {
	target, err := workspaceTargetPath(workingDirectory, targetRef)
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

func patchRepairSuggestionsForDiff(workingDirectory string, targetRef string, diff string) []map[string]any {
	target, err := workspaceTargetPath(workingDirectory, targetRef)
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
	if err != nil || workspace == nil || strings.TrimSpace(workspace.WorkingDirectory) == "" {
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
		item := patchRepairTargetSnapshot(workspace.WorkingDirectory, targetRef)
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
	workingDirectory := ""
	if workspace != nil {
		workingDirectory = strings.TrimSpace(workspace.WorkingDirectory)
	}
	if workingDirectory == "" {
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
	manifest := workspaceFileManifestFromDirectory(workingDirectory)
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
	scope := runtimeRepairScopeFromFailedStep(failedStep)
	targetScope := runtimeFailureTargetScopeFromStep(failedStep)
	windowRoot := runtimeRepairTargetBudgetWindowRoot(failedStep)
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
			"stepId":         string(step.ID),
			"state":          string(step.State),
			"attempt":        runtimeRepairAttemptValue(step),
			"failureClass":   stringFromMap(step.Output, failureOutputKeyClass),
			"failureStage":   stringFromMap(step.Output, "stage"),
			"failureSummary": firstNonEmptyString(stringFromMap(step.Output, "summary"), stringFromMap(step.Output, "error"), stringFromMap(step.Output, "reason")),
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
	return workspaceFileManifestFromDirectory(workspace.WorkingDirectory)
}

func workspaceFileManifestFromDirectory(workingDirectory string) []map[string]any {
	files, err := workspaceManifestFiles(workingDirectory)
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

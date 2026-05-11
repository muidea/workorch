package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

const maxPatchRepairSnapshotBytes = 8192

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
		return o.FailRun(ctx, run.ID, reason)
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
	if o.shouldAttemptProviderRetry(ctx, run, failedStep) {
		rootStepID := providerRetryRootStepID(failedStep)
		attempt := o.providerRetryAttemptCount(ctx, run.ID, rootStepID) + 1
		input := providerRetryStepInput(failedStep, attempt, execErr)
		return o.dispatchStep(ctx, run, providerRetryStepID(rootStepID, attempt), failedStep.Kind, input)
	}
	if o.shouldBlockOnProviderRetryExhaustion(ctx, run, failedStep) {
		return o.blockOnProviderRetryExhaustion(ctx, run, failedStep, execErr)
	}
	if o.shouldEscalateRuntimeRepairExhaustion(ctx, run, failedStep) {
		return o.blockOnRuntimeRepairExhaustion(ctx, run, failedStep, execErr)
	}
	if o.shouldBlockOnDuplicateRuntimeFailure(ctx, run, failedStep) {
		return o.blockOnDuplicateRuntimeFailure(ctx, run, failedStep, execErr)
	}
	if !o.shouldAttemptRuntimeRepair(ctx, run, failedStep) {
		reason := "runtime step failed"
		if execErr != nil {
			reason = execErr.Error()
		} else if failedStep != nil && failedStep.Output != nil {
			reason = stringFromMap(failedStep.Output, "error")
			if reason == "" {
				reason = stringFromMap(failedStep.Output, "reason")
			}
		}
		return o.FailRun(ctx, run.ID, reason)
	}
	attempt := o.runtimeRepairAttemptCountForStep(ctx, run.ID, failedStep) + 1
	input := o.runtimeRepairFollowUpStepInput(ctx, run, failedStep, attempt, execErr)
	return o.dispatchStep(ctx, run, runtimeRepairFollowUpStepID(run.ID, failedStep, attempt), initialStepKind(run), input)
}

func (o *RuntimeOrchestrator) shouldEscalateRuntimeRepairExhaustion(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) bool {
	if o == nil || run == nil || failedStep == nil || o.tickets == nil {
		return false
	}
	if !requiresDoDAssessmentCompletionGate(run) || isLLMProviderRuntimeFailure(run, failedStep) {
		return false
	}
	return o.runtimeRepairAttemptCountForStep(ctx, run.ID, failedStep) >= maxRuntimeRepairAttempts
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
			"allowedRecoveryActions":         []string{"retry_same_step", "replan", "provide_information", "terminate"},
			"attempts":                       o.runtimeRepairAttemptCountForStep(ctx, run.ID, failedStep),
			"maxAttempts":                    maxRuntimeRepairAttempts,
			"failureClass":                   stringFromMap(failedStep.Output, failureOutputKeyClass),
			"failureSignature":               stringFromMap(failedStep.Output, failureOutputKeySignature),
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
	if o.runtimeRepairAttemptCountForStep(ctx, run.ID, failedStep) >= maxRuntimeRepairAttempts {
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
	return o.providerRetryAttemptCount(ctx, run.ID, providerRetryRootStepID(failedStep)) < maxProviderRetryAttempts
}

func (o *RuntimeOrchestrator) shouldBlockOnProviderRetryExhaustion(ctx context.Context, run *runModel.Run, failedStep *runModel.Step) bool {
	if o == nil || run == nil || failedStep == nil || o.tickets == nil {
		return false
	}
	if !isRetryableLLMProviderRuntimeFailure(run, failedStep) {
		return false
	}
	return o.providerRetryAttemptCount(ctx, run.ID, providerRetryRootStepID(failedStep)) >= maxProviderRetryAttempts
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
	spec := runtimeDecisionBlockSpec{
		TicketID:     common.TicketID(runtimeChildID(failedStep.ID, "provider-retry-exhausted-decision")),
		Reason:       localizedProviderRetryExhaustionReason(language, maxProviderRetryAttempts, execErr),
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     localizedProviderRetryExhaustionQuestion(language),
		Context: map[string]any{
			decisionContextKeyScope:          decisionContextScopeWorkorchRuntime,
			decisionContextKeyDependency:     decisionContextDependencyLLMProvider,
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyConfigOnly,
			"attempts":                       maxProviderRetryAttempts,
			"failureClass":                   stringFromMap(failedStep.Output, failureOutputKeyClass),
			"failureSignature":               stringFromMap(failedStep.Output, failureOutputKeySignature),
			"failedStepId":                   string(failedStep.ID),
			"failedStepKind":                 failedStep.Kind,
			"failedStepInput":                compactStepInputForLLMContext(failedStep.Input),
			"failedStepOutput":               compactStepOutputForLLMContext(failedStep.Output),
		},
	}
	if err := o.createDecisionTicket(ctx, run, failedStep, spec); err != nil {
		return err
	}
	return o.BlockOnDecision(ctx, run.ID, spec.TicketID)
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
	input := cloneStepPayload(failedStep.Input)
	if input == nil {
		input = map[string]any{}
	}
	delete(input, stepInputKeyRuntimeRepairFollowUp)
	delete(input, stepInputKeyRuntimeRepairAttempt)
	rootStepID := providerRetryRootStepID(failedStep)
	input[stepInputKeyProviderRetryFollowUp] = true
	input[stepInputKeyProviderRetryAttempt] = attempt
	input["maxProviderRetryAttempts"] = maxProviderRetryAttempts
	input["failedStepId"] = string(rootStepID)
	input["failedStepKind"] = failedStep.Kind
	input["failedStepState"] = string(failedStep.State)
	input["failedStepOutput"] = compactStepOutputForLLMContext(failedStep.Output)
	input["failureReason"] = errorString(execErr)
	input[failureOutputKeyClass] = stringFromMap(failedStep.Output, failureOutputKeyClass)
	input[failureOutputKeySignature] = stringFromMap(failedStep.Output, failureOutputKeySignature)
	if completionGateFollowUpStep(failedStep) {
		input["providerRetryMode"] = "completion_assessment_only"
		input["providerRetryAssessmentOnly"] = true
		input["completionFollowUp"] = true
		delete(input, stepOutputKeyExecutionGraph)
		delete(input, "previousStepOutput")
	}
	return input
}

func completionGateFollowUpStep(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	return boolValue(step.Input["graphFollowUp"]) ||
		boolValue(step.Input["completionFollowUp"]) ||
		boolValue(step.Input[stepInputKeyNoActionableFollowUp])
}

func providerRetryStepID(rootStepID common.StepID, attempt int) common.StepID {
	return common.StepID(runtimeChildID(rootStepID, fmt.Sprintf("provider-retry-%d", attempt)))
}

func providerRetryRootStepID(step *runModel.Step) common.StepID {
	if step == nil {
		return ""
	}
	if root := common.StepID(stringFromMap(step.Input, "failedStepId")); root != "" {
		return root
	}
	return step.ID
}

func isRetryableLLMProviderRuntimeFailure(run *runModel.Run, step *runModel.Step) bool {
	if !isLLMProviderRuntimeFailure(run, step) {
		return false
	}
	switch stringFromMap(step.Output, failureOutputKeyClass) {
	case failureClassProviderTimeout, failureClassContextTooLarge, failureClassRateLimited:
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
	return o.runtimeRepairAttemptCountForScope(ctx, runID, scope)
}

func (o *RuntimeOrchestrator) runtimeRepairAttemptCountForScope(ctx context.Context, runID common.RunID, scope runtimeRepairScope) int {
	if o == nil || o.steps == nil || runID == "" {
		return 0
	}
	steps, err := o.steps.ListByRun(ctx, runID)
	if err != nil {
		return 0
	}
	count := 0
	for _, step := range steps {
		if isRuntimeRepairBudgetStep(runID, step) && runtimeRepairStepMatchesScope(step, scope) {
			count++
		}
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
	attempts := o.runtimeRepairAttemptCountForStep(ctx, run.ID, failedStep)
	return attempts > 0 && attempts < maxRuntimeRepairAttempts
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
	if isProviderFailureClass(stringFromMap(step.Input, failureOutputKeyClass)) ||
		isProviderFailureClass(stringFromMap(step.Output, failureOutputKeyClass)) {
		return false
	}
	return !isLLMProviderFailureStage(stringFromMap(step.Output, "stage"))
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
	if outputSignature != "" && (inputSignature == "" || failureSignatureHasTargetScope(outputSignature)) {
		return firstNonEmptyString(outputClass, inputClass), outputSignature
	}
	return firstNonEmptyString(inputClass, outputClass), firstNonEmptyString(inputSignature, outputSignature)
}

func failureSignatureHasTargetScope(signature string) bool {
	return strings.Contains(signature, "failureTargets=") ||
		strings.Contains(signature, "targetRef=") ||
		strings.Contains(signature, "conflictRef=") ||
		strings.Contains(signature, "serverId=") ||
		strings.Contains(signature, "tool=") ||
		strings.Contains(signature, "uri=")
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
		return root
	}
	if root := graphRootStepID(step); root != "" {
		return root
	}
	return step.ID
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
		"llm_provider_error",
		"llm_provider_stream_error",
		"llm_provider_request_failed",
		"llm_schema_invalid",
		"llm_decision_invalid",
		"llm_failed":
		return true
	default:
		return false
	}
}

func runtimeRepairFollowUpStepID(runID common.RunID, failedStep *runModel.Step, attempt int) common.StepID {
	base := ""
	if failedStep != nil {
		base = runtimeChildID(failedStep.ID, "repair")
	}
	if base == "" {
		base = "repair"
	}
	if attempt < 1 {
		attempt = 1
	}
	return common.StepID(fmt.Sprintf("%s/zz-llm-repair-%s-%d", runID, base, attempt))
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
	input[stepInputKeyRepairOfRootStepID] = string(runtimeRepairRootStepID(failedStep))
	input["failedStepKind"] = failedStep.Kind
	input["failedStepState"] = string(failedStep.State)
	input["failedStepInput"] = compactStepInputForLLMContext(failedStep.Input)
	input["failedStepOutput"] = compactStepOutputForLLMContext(failedStep.Output)
	if diagnostics := mapFromValue(failedStep.Output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		input["runtimeDiagnostics"] = cloneStepPayload(diagnostics)
		if stringFromMap(diagnostics, "kind") == "dod_evidence_quality_gap" {
			input["repairObjective"] = "produce_concrete_validation_evidence"
			input["evidenceRepairRequired"] = true
			input["verificationEvidenceGap"] = cloneStepPayload(diagnostics)
			input["repairGuidance"] = []string{
				"Do not repeat assessment-only validation.",
				"Add or run concrete implementation and tests/assertions for the missing DoD evidence.",
				"Rerun the targeted verification command and use that new step as evidence.",
			}
		}
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
	if patchContext := o.runtimeRepairPatchContext(ctx, run, failedStep); len(patchContext) > 0 {
		input["patchRepairContext"] = patchContext
	}
	if shouldIncludeWorkspaceManifestInRuntimeRepair(failedStep) {
		manifest := o.workspaceFileManifest(ctx, run)
		if len(manifest) > 0 {
			input[stepInputKeyWorkspaceFileManifest] = manifest
		}
	}
	return input
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
		return item
	}
	content := string(data)
	if len(content) > maxPatchRepairSnapshotBytes {
		content = content[:maxPatchRepairSnapshotBytes]
		item["truncated"] = true
	}
	item["content"] = content
	return item
}

func patchRepairLooksBinary(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func shouldIncludeWorkspaceManifestInRuntimeRepair(failedStep *runModel.Step) bool {
	if failedStep == nil || failedStep.Output == nil {
		return false
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
	return false
}

func missingDoDItemsFromFailureOutput(output map[string]any) []map[string]any {
	if len(output) == 0 {
		return nil
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
	ret := make([]map[string]any, 0, limit)
	for _, step := range steps {
		if !isRuntimeRepairBudgetStep(runID, step) || !runtimeRepairStepMatchesScope(step, scope) {
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

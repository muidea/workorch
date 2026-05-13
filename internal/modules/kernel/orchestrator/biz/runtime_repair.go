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
	case failureClassProviderUnavailable, failureClassProviderSchemaDrift, failureClassAuthOrConfigError:
		return true
	default:
		return false
	}
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
	spec := runtimeDecisionBlockSpec{
		TicketID:     common.TicketID(runtimeChildID(failedStep.ID, "provider-retry-exhausted-decision")),
		Reason:       localizedProviderRetryExhaustionReason(language, maxAttempts, execErr),
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     localizedProviderRetryExhaustionQuestion(language),
		Context: map[string]any{
			decisionContextKeyScope:          decisionContextScopeWorkorchRuntime,
			decisionContextKeyDependency:     decisionContextDependencyLLMProvider,
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyConfigOnly,
			decisionContextKeyResultPolicy:   decisionContextResultPolicyNone,
			"attempts":                       maxAttempts,
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
	return o.runtimeRepairAttemptCountForScope(ctx, runID, scope)
}

func (o *RuntimeOrchestrator) runtimeRepairAttemptCountForEffectiveScope(ctx context.Context, runID common.RunID, failedStep *runModel.Step) int {
	exactAttempts := o.runtimeRepairAttemptCountForStep(ctx, runID, failedStep)
	targetScope := runtimeFailureTargetScopeFromStep(failedStep)
	if targetScope == "" {
		return exactAttempts
	}
	class, _ := runtimeRepairScopeClassAndSignature(failedStep)
	targetAttempts := o.runtimeRepairAttemptCountForFailureTarget(ctx, runID, class, targetScope)
	if targetAttempts > exactAttempts {
		return targetAttempts
	}
	return exactAttempts
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

func (o *RuntimeOrchestrator) runtimeRepairAttemptCountForFailureTarget(ctx context.Context, runID common.RunID, failureClass string, targetScope string) int {
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
	if outputSignature != "" && (inputSignature == "" || failureSignatureHasTargetScope(outputSignature)) {
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
			input["repairGuidance"] = []string{
				"Do not repeat assessment-only validation.",
				"Add or run concrete implementation and tests/assertions for the missing DoD evidence.",
				"Rerun the targeted verification command and use that new step as evidence.",
			}
		}
	}
	transientExecutionTimeout := runtimeRepairFailureIsTransientExecutionTimeout(failedStep, execErr)
	if transientExecutionTimeout {
		input["repairObjective"] = repairObjectiveTransientExecutionRetry
		input["executionTimeoutRetryAllowed"] = true
		input["repairGuidance"] = transientExecutionRetryGuidance()
	}
	if stateChangingRepairRequired(run, failedStep, execErr) {
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
		input["targetRepairAttempts"] = o.runtimeRepairAttemptCountForFailureTarget(ctx, run.ID, class, targetScope)
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
	if patchContext := o.runtimeRepairPatchContext(ctx, run, failedStep); len(patchContext) > 0 {
		input["patchRepairContext"] = patchContext
	}
	if shouldIncludeWorkspaceManifestInRuntimeRepair(failedStep) {
		manifest := o.workspaceFileManifest(ctx, run)
		if len(manifest) > 0 {
			input[stepInputKeyWorkspaceFileManifest] = manifest
		}
	}
	if boolValue(input["stateChangingOutputRequired"]) {
		narrowStateChangingRecoveryInput(input, failedStep, execErr, "runtime_repair")
	}
	return input
}

func stateChangingRepairGuidance() []string {
	return []string{
		"Return concrete state-changing output with complete content, or return a failed dodAssessment if the required state change cannot be completed.",
		"If multiple dependent operations are needed, executionGraph is allowed only when it contains at least one valid patch/apply node that does not depend on llm/workspace_execution before the patch.",
		"Do not return read-only, inspection-only, shell-only executionGraph, or LLM-gated executionGraph.",
		"Do not return self-inspection, run snapshot, observability, or context rehydration graphs when effective Goal and DoD are already present.",
		"If a target file is missing in an empty or new workspace, create it with patch/apply file_write instead of reading it again.",
		"Do not use placeholders such as __FROM_PREVIOUS_STEP_OUTPUT__ or <<...>>.",
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
	errorText := firstNonEmptyString(errorString(execErr), stringFromMap(failedStep.Output, "error"), stringFromMap(failedStep.Output, "reason"))
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
	delete(input, stepInputKeyWorkspaceFileManifest)
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
		"execution-stage llm graph node returned executiongraph",
		"state-changing llm repair step returned executiongraph",
		"state-changing llm repair step returned no state-changing output",
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

package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

const maxPatchRepairSnapshotBytes = 120 * 1024
const outputKeyContractViolationType = "contractViolationType"

type repairSubsystem struct {
	steps      repository.StepRepository
	workspaces repository.WorkspaceRepository
}

type repairDispatchSpecBuilder interface {
	runtimeRepairFollowUpStepInput(ctx context.Context, run *runModel.Run, failedStep *runModel.Step, attempt int, execErr error) map[string]any
}

type repairBlockedRecoveryExecutor interface {
	blockOnProviderRetryExhaustion(ctx context.Context, run *runModel.Run, failedStep *runModel.Step, execErr error) error
	blockOnRuntimeRepairExhaustion(ctx context.Context, run *runModel.Run, failedStep *runModel.Step, execErr error) error
	blockOnDuplicateRuntimeFailure(ctx context.Context, run *runModel.Run, failedStep *runModel.Step, execErr error) error
	blockOnContextIntegrityGate(ctx context.Context, run *runModel.Run, failedStep *runModel.Step, execErr error) error
}

func newRepairSubsystem(steps repository.StepRepository, workspaces repository.WorkspaceRepository) *repairSubsystem {
	return &repairSubsystem{
		steps:      steps,
		workspaces: workspaces,
	}
}

const (
	repairObjectiveStateChangingOutput        = "produce_state_changing_output"
	repairObjectiveConcreteValidationEvidence = "produce_concrete_validation_evidence"
	repairObjectiveTransientExecutionRetry    = "retry_or_adjust_transient_execution"
	repairObjectiveAcquireFailureEvidence     = "acquire_concrete_failure_evidence"
)

const (
	contractViolationGraphNodeReturnedGraph         = "graph_node_returned_graph"
	contractViolationRepairReturnedGraph            = "repair_returned_graph"
	contractViolationRepairNoStateChangingOutput    = "repair_no_state_changing_output"
	contractViolationRepairGraphWithoutPatchApply   = "repair_graph_without_patch_apply"
	contractViolationExecutionNoStateChangingOutput = "execution_no_state_changing_output"
	contractViolationGraphDependsOnLLMBeforePatch   = "graph_depends_on_llm_before_patch"
	contractViolationPatchTargetRefRequired         = "patch_target_ref_required"
	contractViolationPatchContentRequired           = "patch_content_required"
	contractViolationPatchUnresolvedTemplate        = "patch_unresolved_template"
	contractViolationPatchDispatchInvalid           = "patch_dispatch_invalid"
	contractViolationFileEditConflict               = "file_edit_conflict"
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
	attachContractViolationType(step.Output, execErr)
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
	_ = o.checkpointManager().recordStepCheckpoint(ctx, run, step, "step_failed")
	if o.coverageLedger().updateFromFailedStep(run, step, errText) && o.runs != nil {
		_ = o.runs.Update(ctx, run)
	}
	_ = o.steps.Update(ctx, step)
	_ = o.eventPublisher().publishStepState(ctx, run, step, events.TopicStepStateChanged)
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

func attachContractViolationType(output map[string]any, execErr error) {
	if output == nil || strings.TrimSpace(stringFromMap(output, outputKeyContractViolationType)) != "" {
		return
	}
	if violationType := contractViolationTypeFromFailure(output, execErr); violationType != "" {
		output[outputKeyContractViolationType] = violationType
	}
}

func contractViolationTypeFromFailure(output map[string]any, execErr error) string {
	text := strings.ToLower(strings.Join([]string{
		errorString(execErr),
		stringFromMap(output, "error"),
		stringFromMap(output, "reason"),
		stringFromMap(output, "stage"),
		stringFromMap(output, "ignoredExecutionGraphReason"),
		stringFromMap(mapFromValue(output["runtimeDiagnostics"]), "kind"),
		stringFromMap(mapFromValue(output["runtimeDiagnostics"]), "reason"),
	}, "\n"))
	switch {
	case boolValue(output["executionStageContractViolation"]) ||
		strings.Contains(text, "execution-stage llm graph node returned executiongraph"):
		return contractViolationGraphNodeReturnedGraph
	case strings.Contains(text, "state-changing llm repair step returned executiongraph without patch/apply"):
		return contractViolationRepairGraphWithoutPatchApply
	case strings.Contains(text, "state-changing llm repair step returned executiongraph"):
		return contractViolationRepairReturnedGraph
	case strings.Contains(text, "state-changing llm repair step returned no state-changing output"):
		return contractViolationRepairNoStateChangingOutput
	case strings.Contains(text, "state-changing execution-stage llm graph node returned no state-changing output"):
		return contractViolationExecutionNoStateChangingOutput
	case strings.Contains(text, "executiongraph patch/apply depends on llm/workspace_execution before patch"):
		return contractViolationGraphDependsOnLLMBeforePatch
	case strings.Contains(text, "targetref is required") ||
		strings.Contains(text, "targetref must be set"):
		return contractViolationPatchTargetRefRequired
	case strings.Contains(text, "patch content contains unresolved runtime template") ||
		strings.Contains(text, "patch diff contains unresolved runtime template") ||
		strings.Contains(text, "patch text_replace contains unresolved runtime template"):
		return contractViolationPatchUnresolvedTemplate
	case strings.Contains(text, "patch content is required") ||
		strings.Contains(text, "patch diff is required") ||
		strings.Contains(text, "patch oldtext is required"):
		return contractViolationPatchContentRequired
	case strings.Contains(text, "patch_dispatch_invalid"):
		return contractViolationPatchDispatchInvalid
	case strings.Contains(text, "file_edit_conflict") ||
		strings.Contains(text, "file/edit oldtext not found"):
		return contractViolationFileEditConflict
	default:
		return ""
	}
}

func shouldCloseCompletionGateFailureWithoutRepair(summary string, step *runModel.Step, output map[string]any) bool {
	if strings.TrimSpace(summary) != "runtime completion gate failed" {
		return false
	}
	if step != nil && boolValue(step.Input[stepInputKeyNoActionableFollowUp]) {
		return false
	}
	if step != nil && boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) &&
		assessmentOnlySchemaInvalidRepairRequired(step) {
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

func assessmentOnlySchemaInvalidRepairRequired(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	if !repairActionIsSchemaInvalidRepair(step.Input, step.Output) {
		return false
	}
	if schemaInvalidRepairMustPreserveExecutableContract(step.Input) {
		return false
	}
	if schemaInvalidOutputLooksLikeAssessment(step.Output) &&
		(strings.TrimSpace(stringFromMap(step.Input, stepInputKeyRuntimeStage)) == runtimeStageVerification ||
			len(missingGoalCoverageFromInput(step.Input)) > 0 ||
			len(mapSliceFromValue(step.Input["missingDeliverableCoverage"])) > 0 ||
			len(mapSliceFromValue(step.Input["coverageTargetSet"])) > 0) {
		return true
	}
	if !boolValue(step.Input[stepInputKeyAssessmentOnly]) {
		return false
	}
	contract := mapFromValue(step.Input["convergenceContract"])
	return strings.TrimSpace(stringFromMap(step.Input, "repairObjective")) == "complete_dod_goal_coverage_assessment" ||
		strings.TrimSpace(stringFromMap(contract, "state")) == convergenceStateNeedAssessment ||
		strings.TrimSpace(stringFromMap(step.Input, stepInputKeyRuntimeStage)) == runtimeStageVerification
}

func schemaInvalidOutputLooksLikeAssessment(output map[string]any) bool {
	if len(output) == 0 {
		return false
	}
	if len(doDAssessmentFromOutput(output)) > 0 {
		return true
	}
	text := strings.ToLower(strings.Join([]string{
		stringFromMap(output, "rawContentPreview"),
		stringFromMap(output, "repairRawContentPreview"),
		stringFromMap(output, "error"),
		stringFromMap(output, "reason"),
	}, "\n"))
	return strings.Contains(text, "dodassessment") ||
		strings.Contains(text, "\"goalcoverage\"") ||
		strings.Contains(text, "\"deliverablecoverage\"") ||
		strings.Contains(text, "\"acceptancedimensions\"")
}

func schemaInvalidRepairMustPreserveExecutableContract(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if inputRequiresConcreteValidationEvidence(input) ||
		inputRequiresStateChangingOutput(input) ||
		boolValue(input["concreteFailureEvidenceRequired"]) ||
		strings.TrimSpace(stringFromMap(input, "repairObjective")) == repairObjectiveAcquireFailureEvidence ||
		repairActionPackageRequiresEvidenceAcquisition(mapFromValue(input["repairActionPackage"])) {
		return true
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	return boolValue(required["mustProduceConcreteFailureEvidence"]) ||
		boolValue(required["mustIncludeStateChangingNode"])
}

func schemaInvalidRepairCanContinueWithinBudget(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	return repairActionIsSchemaInvalidRepair(step.Input, step.Output)
}

func (o *RuntimeOrchestrator) dispatchRuntimeRepairFollowUp(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	execErr error,
) error {
	if summary, ok := o.resultManager().rejectedRunResultSummary(ctx, run.ID); ok {
		reason := strings.TrimSpace(summary)
		if reason == "" {
			reason = "run has rejected result"
		}
		return o.FailRun(ctx, run.ID, reason)
	}
	switch runtimeRecoveryActionForFailedStep(ctx, o.repairManager(), o.policy, o.runtime != nil, o.tickets != nil, run, failedStep) {
	case runtimeRecoveryActionProviderRetry:
		spec, ok, err := o.repairManager().recoveryDispatchSpec(ctx, run, failedStep, execErr, runtimeRecoveryActionProviderRetry, maxProviderRetryAttemptsForPolicy(o.policy), o, true)
		if err != nil {
			return err
		}
		if !ok {
			return common.ErrInvalidArgument
		}
		return o.dispatchStepWithWorkspace(ctx, run, spec.ID, spec.Kind, spec.Input, nil)
	case runtimeRecoveryActionProviderRetryExhausted:
		return o.blockOnProviderRetryExhaustion(ctx, run, failedStep, execErr)
	case runtimeRecoveryActionRuntimeRepairExhausted:
		return o.blockOnRuntimeRepairExhaustion(ctx, run, failedStep, execErr)
	case runtimeRecoveryActionDuplicateFailure:
		return o.blockOnDuplicateRuntimeFailure(ctx, run, failedStep, execErr)
	case runtimeRecoveryActionContextIntegrityBlocked:
		return o.blockOnContextIntegrityGate(ctx, run, failedStep, execErr)
	case runtimeRecoveryActionRuntimeRepair:
		spec, ok, err := o.repairManager().recoveryDispatchSpec(ctx, run, failedStep, execErr, runtimeRecoveryActionRuntimeRepair, maxProviderRetryAttemptsForPolicy(o.policy), o, true)
		if err != nil {
			return err
		}
		if !ok {
			return common.ErrInvalidArgument
		}
		return o.dispatchStepWithWorkspace(ctx, run, spec.ID, spec.Kind, spec.Input, nil)
	case runtimeRecoveryActionFailRun:
		return o.failRunWithRejectedCompletionResult(ctx, run, failedStep, o.repairManager().failRunReasonFromFailedStep(failedStep, execErr))
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

func (r *repairSubsystem) recoveryActionForFailedStep(ctx context.Context, run *runModel.Run, failedStep *runModel.Step, runtimePolicy RuntimePolicy, runtimeAvailable bool, canCreateDecisionTicket bool) runtimeRecoveryAction {
	if run == nil || failedStep == nil {
		return runtimeRecoveryActionFailRun
	}
	if shouldBlockOnProviderConfigFailure(canCreateDecisionTicket, run, failedStep) {
		return runtimeRecoveryActionProviderRetryExhausted
	}
	if shouldAttemptProviderRetry(ctx, r, runtimePolicy, runtimeAvailable, run, failedStep) {
		return runtimeRecoveryActionProviderRetry
	}
	if providerRetryBudgetExhausted(ctx, r, runtimePolicy, run, failedStep) {
		return runtimeRecoveryActionProviderRetryExhausted
	}
	if isLLMProviderRuntimeFailure(run, failedStep) {
		return runtimeRecoveryActionFailRun
	}
	if runtimeContextIntegrityGateFailure(failedStep) && !runtimeContextIntegrityGateCanRecoverWithCoverageTargets(failedStep) {
		return runtimeRecoveryActionContextIntegrityBlocked
	}
	if shouldEscalateRuntimeRepairExhaustion(ctx, r, runtimePolicy, canCreateDecisionTicket, run, failedStep) {
		return runtimeRecoveryActionRuntimeRepairExhausted
	}
	if shouldBlockOnDuplicateRuntimeFailure(ctx, r.steps, r, runtimePolicy, canCreateDecisionTicket, run, failedStep) {
		return runtimeRecoveryActionDuplicateFailure
	}
	if shouldAttemptRuntimeRepair(ctx, r, runtimePolicy, runtimeAvailable, run, failedStep) {
		return runtimeRecoveryActionRuntimeRepair
	}
	return runtimeRecoveryActionFailRun
}

func runtimeRecoveryActionForFailedStep(ctx context.Context, repair *repairSubsystem, policy RuntimePolicy, runtimeAvailable bool, canCreateDecisionTicket bool, run *runModel.Run, failedStep *runModel.Step) runtimeRecoveryAction {
	if repair == nil {
		return runtimeRecoveryActionFailRun
	}
	return repair.recoveryActionForFailedStep(ctx, run, failedStep, policy, runtimeAvailable, canCreateDecisionTicket)
}

func (r *repairSubsystem) blockRecoveryActionFromFailedStep(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	execErr error,
	action runtimeRecoveryAction,
	executor repairBlockedRecoveryExecutor,
) error {
	if executor == nil {
		return common.ErrBlocked
	}
	switch action {
	case runtimeRecoveryActionProviderRetryExhausted:
		if err := executor.blockOnProviderRetryExhaustion(ctx, run, failedStep, execErr); err != nil {
			return err
		}
		return common.ErrBlocked
	case runtimeRecoveryActionRuntimeRepairExhausted:
		if err := executor.blockOnRuntimeRepairExhaustion(ctx, run, failedStep, execErr); err != nil {
			return err
		}
		return common.ErrBlocked
	case runtimeRecoveryActionDuplicateFailure:
		if err := executor.blockOnDuplicateRuntimeFailure(ctx, run, failedStep, execErr); err != nil {
			return err
		}
		return common.ErrBlocked
	case runtimeRecoveryActionContextIntegrityBlocked:
		if err := executor.blockOnContextIntegrityGate(ctx, run, failedStep, execErr); err != nil {
			return err
		}
		return common.ErrBlocked
	default:
		return common.ErrConflict
	}
}

func (r *repairSubsystem) failRunReasonFromFailedStep(failedStep *runModel.Step, execErr error) string {
	reason := "runtime step failed"
	if execErr != nil {
		reason = execErr.Error()
	} else if failedStep != nil && failedStep.Output != nil {
		reason = stringFromMap(failedStep.Output, "error")
		if reason == "" {
			reason = stringFromMap(failedStep.Output, "reason")
		}
	}
	return reason
}

func (r *repairSubsystem) recoveryDispatchSpec(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	execErr error,
	action runtimeRecoveryAction,
	maxProviderRetryAttempts int,
	builder repairDispatchSpecBuilder,
	useEffectiveRepairScope bool,
) (stepDispatchSpec, bool, error) {
	if r == nil || run == nil || failedStep == nil || builder == nil {
		return stepDispatchSpec{}, false, common.ErrInvalidArgument
	}
	switch action {
	case runtimeRecoveryActionProviderRetry:
		rootStepID := providerRetryRootStepID(failedStep)
		attempt := r.providerRetryAttemptCount(ctx, run.ID, rootStepID) + 1
		stepID, err := uniqueDispatchStepID(ctx, r.steps, run.ID, providerRetryStepID(run.ID, rootStepID, attempt))
		if err != nil {
			return stepDispatchSpec{}, true, err
		}
		return stepDispatchSpec{
			ID:    stepID,
			Kind:  failedStep.Kind,
			Input: providerRetryStepInputWithMaxAttempts(failedStep, attempt, execErr, maxProviderRetryAttempts),
		}, true, nil
	case runtimeRecoveryActionRuntimeRepair:
		attempts := r.runtimeRepairAttemptCountForStep(ctx, run.ID, failedStep)
		if useEffectiveRepairScope {
			attempts = r.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep)
		}
		attempt := attempts + 1
		stepID, err := uniqueDispatchStepID(ctx, r.steps, run.ID, runtimeRepairFollowUpStepID(run.ID, failedStep, attempt))
		if err != nil {
			return stepDispatchSpec{}, true, err
		}
		return stepDispatchSpec{
			ID:    stepID,
			Kind:  initialStepKind(run),
			Input: builder.runtimeRepairFollowUpStepInput(ctx, run, failedStep, attempt, execErr),
		}, true, nil
	default:
		return stepDispatchSpec{}, false, nil
	}
}

func shouldBlockOnProviderConfigFailure(canCreateDecisionTicket bool, run *runModel.Run, failedStep *runModel.Step) bool {
	if !canCreateDecisionTicket || run == nil || failedStep == nil {
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
	if providerFailureHTTPStatus(text) >= 400 {
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
	err := o.repairManager().blockRecoveryActionFromFailedStep(ctx, run, failedStep, execErr, action, o)
	if errors.Is(err, common.ErrConflict) {
		return common.ErrInvalidArgument
	}
	return err
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
		_ = o.resultManager().recordRejectedCompletionDoDAssessment(ctx, run, step, step.Output)
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

func shouldEscalateRuntimeRepairExhaustion(ctx context.Context, repair *repairSubsystem, policy RuntimePolicy, canCreateDecisionTicket bool, run *runModel.Run, failedStep *runModel.Step) bool {
	if repair == nil || run == nil || failedStep == nil || !canCreateDecisionTicket {
		return false
	}
	if !requiresDoDAssessmentCompletionGate(run) || isLLMProviderRuntimeFailure(run, failedStep) {
		return false
	}
	return repair.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep) >= maxRuntimeRepairAttemptsForPolicy(policy)
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
	maxAttempts := maxRuntimeRepairAttemptsForPolicy(o.policy)
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
			"attempts":                       o.repairManager().runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep),
			"maxAttempts":                    maxAttempts,
			"failureClass":                   stringFromMap(failedStep.Output, failureOutputKeyClass),
			"failureSignature":               stringFromMap(failedStep.Output, failureOutputKeySignature),
			"failureTargetScope":             runtimeFailureTargetScopeFromStep(failedStep),
			"failedStepId":                   string(failedStep.ID),
			"repairOfRootStepId":             string(runtimeRepairRootStepID(failedStep)),
			"failedStepKind":                 failedStep.Kind,
			"failedStepInput":                o.llmContextManager().compactStepInput(failedStep.Input),
			"failedStepOutput":               o.llmContextManager().compactStepOutput(failedStep.Output),
			"missingDoDItems":                missingDoDItemsFromFailureOutput(failedStep.Output),
			"recentRepairAttempts":           o.repairManager().recentRuntimeRepairAttemptSummaries(ctx, run.ID, failedStep, maxAttempts),
			"recommendedNextActions": []string{
				"Map each missing DoD item to concrete affected resources, state-changing operations, and verification actions.",
				"Prioritize minimal target changes, then rerun targeted verification.",
				"If key constraints remain unclear, provide a focused decision payload with only missing information.",
			},
		},
	}
	if err := o.resultManager().recordRejectedResultForDecisionBlock(ctx, run, failedStep, failedStep.Output, spec); err != nil {
		return err
	}
	if err := o.createDecisionTicket(ctx, run, failedStep, spec); err != nil {
		return err
	}
	return o.BlockOnDecision(ctx, run.ID, spec.TicketID)
}

func shouldAttemptRuntimeRepair(ctx context.Context, repair *repairSubsystem, policy RuntimePolicy, runtimeAvailable bool, run *runModel.Run, failedStep *runModel.Step) bool {
	if repair == nil || run == nil || failedStep == nil || !runtimeAvailable {
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
	if repair.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep) >= maxRuntimeRepairAttemptsForPolicy(policy) {
		return false
	}
	return !isLLMProviderRuntimeFailure(run, failedStep)
}

func shouldAttemptProviderRetry(ctx context.Context, repair *repairSubsystem, policy RuntimePolicy, runtimeAvailable bool, run *runModel.Run, failedStep *runModel.Step) bool {
	if repair == nil || run == nil || failedStep == nil || !runtimeAvailable {
		return false
	}
	attempts := repair.providerRetryAttemptCount(ctx, run.ID, providerRetryRootStepID(failedStep))
	if attempts >= maxProviderRetryAttemptsForPolicy(policy) {
		return false
	}
	if isRetryableLLMProviderRuntimeFailure(run, failedStep) {
		return true
	}
	return false
}

func providerRetryBudgetExhausted(ctx context.Context, repair *repairSubsystem, policy RuntimePolicy, run *runModel.Run, failedStep *runModel.Step) bool {
	if repair == nil || run == nil || failedStep == nil {
		return false
	}
	if !isRetryableLLMProviderRuntimeFailure(run, failedStep) {
		return false
	}
	return repair.providerRetryAttemptCount(ctx, run.ID, providerRetryRootStepID(failedStep)) >= maxProviderRetryAttemptsForPolicy(policy)
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
	maxAttempts := maxProviderRetryAttemptsForPolicy(o.policy)
	rootStepID := providerRetryRootStepID(failedStep)
	attempts := o.repairManager().providerRetryAttemptCount(ctx, run.ID, rootStepID)
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
			"failedStepInput":                o.llmContextManager().compactStepInput(failedStep.Input),
			"failedStepOutput":               o.llmContextManager().compactStepOutput(failedStep.Output),
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
			"failedStepInput":                o.llmContextManager().compactStepInput(failedStep.Input),
			"failedStepOutput":               o.llmContextManager().compactStepOutput(failedStep.Output),
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

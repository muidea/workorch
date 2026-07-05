package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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
	blockOnContractViolationRecoveryExhaustion(ctx context.Context, run *runModel.Run, failedStep *runModel.Step, execErr error) error
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
	stepInputKeyDecisionEligibilityDenied       = "decisionEligibilityDenied"
	stepInputKeyDecisionEligibilityDeniedReason = "decisionEligibilityDeniedReason"
)

const (
	contractViolationGraphNodeReturnedGraph         = "graph_node_returned_graph"
	contractViolationRepairReturnedGraph            = "repair_returned_graph"
	contractViolationRepairNoStateChangingOutput    = "repair_no_state_changing_output"
	contractViolationRepairGraphWithoutPatchApply   = "repair_graph_without_patch_apply"
	contractViolationExecutionNoStateChangingOutput = "execution_no_state_changing_output"
	contractViolationGraphDependsOnLLMBeforePatch   = "graph_depends_on_llm_before_patch"
	contractViolationPatchTargetRefRequired         = "patch_target_ref_required"
	contractViolationCoverageTargetsRequired        = "coverage_targets_required"
	contractViolationValidationEvidenceRequired     = "validation_evidence_required"
	contractViolationPatchContentRequired           = "patch_content_required"
	contractViolationPatchUnresolvedTemplate        = "patch_unresolved_template"
	contractViolationPatchDispatchInvalid           = "patch_dispatch_invalid"
	contractViolationFileEditConflict               = "file_edit_conflict"
	contractViolationDoDAssessmentEvidenceMissing   = "dod_assessment_missing_evidence_refs"
	contractViolationCoverageMapIncomplete          = "coverage_map_incomplete"
	contractViolationAssessmentOnlyExecutableOutput = "assessment_only_executable_output"
	contractViolationAvailableSnapshotIgnored       = "available_snapshot_ignored"
	contractViolationLLMSchemaInvalid               = "llm_schema_invalid"
	contractViolationFocusBindingMismatch           = "focus_binding_mismatch"
)

func runtimeFailureOutput(output map[string]any, execErr error) map[string]any {
	ret := cloneStepPayload(output)
	if ret == nil {
		ret = map[string]any{}
	}
	normalizeRuntimeFailureStatus(ret)
	if execErr != nil {
		ret["reason"] = execErr.Error()
		ret["error"] = execErr.Error()
		var diagnosticErr runtimeFailureDiagnosticError
		if errors.As(execErr, &diagnosticErr) {
			if diagnostics := diagnosticErr.RuntimeFailureDiagnostics(); len(diagnostics) > 0 {
				mergeRuntimeFailureDiagnostics(ret, diagnostics)
			}
		}
	}
	return ret
}

func normalizeRuntimeFailureStatus(output map[string]any) {
	if output == nil {
		return
	}
	status := strings.TrimSpace(stringFromMap(output, "status"))
	switch strings.ToLower(status) {
	case "completed", "complete", "succeeded", "success", "ok":
		output["runtimeResultStatus"] = status
		output["status"] = "failed"
	case "":
		output["status"] = "failed"
	}
}

func mergeRuntimeFailureDiagnostics(output map[string]any, diagnostics map[string]any) {
	if output == nil || len(diagnostics) == 0 {
		return
	}
	merged := cloneStepPayload(mapFromValue(output["runtimeDiagnostics"]))
	if merged == nil {
		merged = map[string]any{}
	}
	for key, value := range diagnostics {
		merged[key] = cloneStepInputValue(value)
	}
	output["runtimeDiagnostics"] = merged
	for _, key := range []string{"missingDoDItems", "missingGoalCoverage", "missingDeliverableCoverage", "evidenceQualityGapItems"} {
		items := mapSliceFromValue(merged[key])
		if len(items) == 0 || len(mapSliceFromValue(output[key])) > 0 {
			continue
		}
		output[key] = cloneStepInputValue(items)
	}
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
	finalizeManagedValidationResourcesForStep(step, "failed", "failed_step")
	attachContractViolationType(step.Output, execErr)
	attachRuntimeFailureStage(step.Output, summary)
	classification := classifyRuntimeFailure(step, step.Output, summary, execErr)
	attachRuntimeFailureClassification(step.Output, classification)
	attachRuntimeFailureDiagnostics(step, classification, execErr)
	attachContractViolationDiagnostics(run, step, execErr)
	if err := o.recordRejectedDiagnosticEvidenceForContractViolation(ctx, run, step); err != nil {
		return err
	}
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
	_ = o.eventPublisher().publishContractViolationRecorded(ctx, run, step)
	if o.coverageLedger().updateFromFailedStep(run, step, errText, ctx) && o.runs != nil {
		_ = o.persistRunProgressUpdate(ctx, run)
	}
	if changed, bound := o.reconcileCoverageValidationEvidence(ctx, run); changed && o.runs != nil {
		_ = o.persistRunProgressUpdate(ctx, run)
		logCoverageReconcilerBinding(run.ID, bound)
	}
	_ = o.steps.Update(ctx, step)
	_ = o.eventPublisher().publishStepState(ctx, run, step, runevents.TopicStepStateChanged)
	if graphBatchStep(step) {
		if o.runtime != nil {
			return o.dispatchRuntimeRepairFollowUpOrBlock(ctx, run, step, execErr)
		}
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
	return o.dispatchRuntimeRepairFollowUpOrBlock(ctx, run, step, execErr)
}

func (o *RuntimeOrchestrator) dispatchRuntimeRepairFollowUpOrBlock(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	execErr error,
) error {
	if o != nil && o.tickets != nil && failedStepHasExternalRuntimeDependency(failedStep) {
		return o.blockOnRuntimeRepairExhaustion(ctx, run, failedStep, execErr)
	}
	err := o.dispatchRuntimeRepairFollowUp(ctx, run, failedStep, execErr)
	if err == nil || errors.Is(err, common.ErrBlocked) || failedStep == nil || graphNodeID(failedStep) == "" {
		return err
	}
	if failedStep.Output == nil {
		failedStep.Output = map[string]any{}
	}
	failedStep.Output["repairDispatchError"] = err.Error()
	if o.steps != nil {
		_ = o.steps.Update(ctx, failedStep)
	}
	return o.blockOnRuntimeRepairExhaustion(ctx, run, failedStep, fmt.Errorf("runtime repair dispatch failed: %w", err))
}

func attachRuntimeFailureStage(output map[string]any, summary string) {
	if output == nil || strings.TrimSpace(stringFromMap(output, "stage")) != "" {
		return
	}
	if kind := strings.TrimSpace(stringFromMap(mapFromValue(output["runtimeDiagnostics"]), "kind")); kind != "" {
		output["stage"] = kind
		return
	}
	lowerSummary := strings.ToLower(strings.TrimSpace(summary))
	switch {
	case strings.Contains(lowerSummary, "runtime execution contract failed") ||
		strings.Contains(lowerSummary, "runtime state-changing contract gate failed") ||
		strings.Contains(lowerSummary, "runtime completion gate failed") ||
		strings.TrimSpace(stringFromMap(output, outputKeyContractViolationType)) != "":
		output["stage"] = "runtime_contract_failed"
	case strings.Contains(lowerSummary, "runtime patch application failed") ||
		strings.Contains(lowerSummary, "runtime patch revert failed"):
		output["stage"] = "patch_apply_failed"
	case strings.Contains(lowerSummary, "runtime patch dispatch gate failed"):
		output["stage"] = "patch_dispatch_invalid"
	case strings.Contains(lowerSummary, "runtime memory write failed"):
		output["stage"] = "memory_write_failed"
	case strings.Contains(lowerSummary, "runtime step failed"):
		output["stage"] = "runtime_step_failed"
	}
}

func attachRuntimeFailureDiagnostics(step *runModel.Step, classification runtimeFailureClassification, execErr error) {
	if step == nil || len(step.Output) == 0 {
		return
	}
	if diagnostic := validationFailureDiagnosticFromStep(step, execErr); len(diagnostic) > 0 {
		step.Output["validationFailureDiagnostic"] = diagnostic
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
	if diagnostic := mapFromValue(step.Output["validationFailureDiagnostic"]); len(diagnostic) > 0 {
		diagnostics["validationFailureDiagnostic"] = compactLLMContextMap(diagnostic, 1)
	}
	step.Output["lastFailureDiagnostics"] = diagnostics
}

func attachContractViolationType(output map[string]any, execErr error) {
	if output == nil || strings.TrimSpace(stringFromMap(output, outputKeyContractViolationType)) != "" {
		return
	}
	var structured runtimeContractViolationError
	if errors.As(execErr, &structured) && structured.violationType != "" {
		output[outputKeyContractViolationType] = structured.violationType
		output["contractViolationRule"] = structured.rule
		output["contractViolationRecoverability"] = structured.recoverability
		return
	}
	if violationType := contractViolationTypeFromFailure(output, execErr); violationType != "" {
		output[outputKeyContractViolationType] = violationType
		rule := contractViolationRuleFromType(violationType)
		output["contractViolationRule"] = rule
		output["contractViolationRecoverability"] = contractViolationRecoverabilityForRule(rule)
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
	case strings.Contains(text, "state-changing patch/apply must declare coveragetargets") ||
		strings.Contains(text, "patch/apply must declare coveragetargets matching"):
		return contractViolationCoverageTargetsRequired
	case strings.Contains(text, "repair_focus_mismatch"):
		return contractViolationFocusBindingMismatch
	case strings.Contains(text, "validation follow-up llm step returned read-only executiongraph without concrete validation evidence") ||
		strings.Contains(text, "concrete validation evidence repair must return bounded") ||
		strings.Contains(text, "concrete validation evidence graph must use non-destructive bounded validation") ||
		strings.Contains(text, "concrete validation evidence graph must not start detached or background processes"):
		return contractViolationValidationEvidenceRequired
	case strings.Contains(text, "satisfied dodassessment item requires evidencerefs") ||
		strings.Contains(text, "satisfied dodassessment.goalcoverage item requires evidencerefs") ||
		strings.Contains(text, "satisfied dodassessment.branchcoverage item") && strings.Contains(text, "requires evidencerefs") ||
		strings.Contains(text, "satisfied dodassessment.acceptancedimensions item") && strings.Contains(text, "requires evidencerefs") ||
		strings.Contains(text, "satisfied dodassessment.deliverablecoverage item") && strings.Contains(text, "requires evidencerefs"):
		return contractViolationDoDAssessmentEvidenceMissing
	case strings.Contains(text, "dodassessment.items are required") ||
		strings.Contains(text, "dodassessment.goalcoverage is required") ||
		strings.Contains(text, "dodassessment.goalcoverage.items are required") ||
		strings.Contains(text, "dodassessment does not cover all current") ||
		strings.Contains(text, "dodassessment.acceptancedimensions missing required dimensions") ||
		strings.Contains(text, "dodassessment.deliverablecoverage missing required deliverables"):
		return contractViolationCoverageMapIncomplete
	case strings.Contains(text, "patch content contains unresolved runtime template") ||
		strings.Contains(text, "patch diff contains unresolved runtime template") ||
		strings.Contains(text, "patch text_replace contains unresolved runtime template"):
		return contractViolationPatchUnresolvedTemplate
	case strings.Contains(text, "patch content is required") ||
		strings.Contains(text, "patch diff is required") ||
		strings.Contains(text, "patch oldtext is required") ||
		strings.Contains(text, "is non-substantive"):
		return contractViolationPatchContentRequired
	case strings.Contains(text, "patch_dispatch_invalid"):
		return contractViolationPatchDispatchInvalid
	case strings.Contains(text, "file_edit_conflict") ||
		strings.Contains(text, "file/edit oldtext not found"):
		return contractViolationFileEditConflict
	case strings.Contains(text, "file_write cannot replace existing target"):
		return contractViolationPatchDispatchInvalid
	case strings.Contains(text, "executiongraph patch/apply must not depend on same-graph file/read") ||
		strings.Contains(text, "patch/apply must not depend on same-graph file/read"):
		return contractViolationGraphDependsOnLLMBeforePatch
	case strings.Contains(text, "patch text_replace oldtext/newtext must describe") ||
		strings.Contains(text, "oldtext/newtext must describe a substantive change") ||
		strings.Contains(text, "patch describes a non-substantive"):
		return contractViolationPatchContentRequired
	case strings.Contains(text, "strategyreassessment.required=true"):
		return contractViolationPatchDispatchInvalid
	case strings.TrimSpace(stringFromMap(output, "stage")) == contractViolationLLMSchemaInvalid ||
		strings.Contains(text, "invalid llm decision json"):
		return contractViolationLLMSchemaInvalid
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
	policy := o.RuntimePolicy()
	switch runtimeRecoveryActionForFailedStep(ctx, o.repairManager(), policy, o.runtime != nil, o.tickets != nil, run, failedStep) {
	case runtimeRecoveryActionProviderRetry:
		spec, ok, err := o.repairManager().recoveryDispatchSpec(ctx, run, failedStep, execErr, runtimeRecoveryActionProviderRetry, o.repairManager().effectiveMaxProviderRetryAttempts(ctx, policy, run, failedStep), o, true)
		if err != nil {
			return err
		}
		if !ok {
			return common.ErrInvalidArgument
		}
		return o.requestDispatchStep(ctx, run.ID, run.WorkspaceID, spec)
	case runtimeRecoveryActionProviderRetryExhausted:
		return o.blockOnProviderRetryExhaustion(ctx, run, failedStep, execErr)
	case runtimeRecoveryActionRuntimeRepairExhausted:
		return o.blockOnRuntimeRepairExhaustion(ctx, run, failedStep, execErr)
	case runtimeRecoveryActionDuplicateFailure:
		return o.blockOnDuplicateRuntimeFailure(ctx, run, failedStep, execErr)
	case runtimeRecoveryActionContextIntegrityBlocked:
		return o.blockOnContextIntegrityGate(ctx, run, failedStep, execErr)
	case runtimeRecoveryActionContractViolationRetry:
		spec, ok, err := o.repairManager().recoveryDispatchSpec(ctx, run, failedStep, execErr, runtimeRecoveryActionContractViolationRetry, maxProviderRetryAttemptsForPolicy(policy), o, true)
		if err != nil {
			return err
		}
		if !ok {
			return common.ErrInvalidArgument
		}
		if contractViolationExhaustionDecisionNotEligible(ctx, o.repairManager(), policy, run, failedStep) {
			spec.Input = annotateDecisionEligibilityDeniedRetry(spec.Input, "continuable_contract_violation_retry_available")
		}
		if err := o.eventPublisher().publishContractViolationRetryRequested(ctx, run, failedStep, spec.ID, mapFromValue(spec.Input[stepInputKeyContractViolationRecovery])); err != nil {
			return err
		}
		return o.requestDispatchStep(ctx, run.ID, run.WorkspaceID, spec)
	case runtimeRecoveryActionContractViolationExhausted:
		return o.blockOnContractViolationRecoveryExhaustion(ctx, run, failedStep, execErr)
	case runtimeRecoveryActionRuntimeRepair:
		spec, ok, err := o.repairManager().recoveryDispatchSpec(ctx, run, failedStep, execErr, runtimeRecoveryActionRuntimeRepair, maxProviderRetryAttemptsForPolicy(policy), o, true)
		if err != nil {
			return err
		}
		if !ok {
			return common.ErrInvalidArgument
		}
		if runtimeRepairExhaustionDecisionNotEligible(run, failedStep) {
			spec.Input = annotateDecisionEligibilityDeniedRetry(spec.Input, "continuable_runtime_repair_available")
		}
		return o.requestDispatchStep(ctx, run.ID, run.WorkspaceID, spec)
	case runtimeRecoveryActionFailRun:
		return o.failRunWithRejectedCompletionResult(ctx, run, failedStep, o.repairManager().failRunReasonFromFailedStep(failedStep, execErr))
	default:
		return common.ErrInvalidArgument
	}
}

type runtimeRecoveryAction string

const (
	runtimeRecoveryActionProviderRetry              runtimeRecoveryAction = "provider_retry"
	runtimeRecoveryActionProviderRetryExhausted     runtimeRecoveryAction = "provider_retry_exhausted"
	runtimeRecoveryActionRuntimeRepairExhausted     runtimeRecoveryAction = "runtime_repair_exhausted"
	runtimeRecoveryActionDuplicateFailure           runtimeRecoveryAction = "duplicate_failure"
	runtimeRecoveryActionContextIntegrityBlocked    runtimeRecoveryAction = "context_integrity_blocked"
	runtimeRecoveryActionContractViolationRetry     runtimeRecoveryAction = "contract_violation_retry"
	runtimeRecoveryActionContractViolationExhausted runtimeRecoveryAction = "contract_violation_exhausted"
	runtimeRecoveryActionRuntimeRepair              runtimeRecoveryAction = "runtime_repair"
	runtimeRecoveryActionFailRun                    runtimeRecoveryAction = "fail_run"
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
	if runtimeAvailable &&
		failedStepRequestsContinuableDecision(failedStep) &&
		!failedStepHasExternalDecisionDependency(failedStep) &&
		failedStepHasConvertedReadableWorkspaceResource(failedStep) {
		return runtimeRecoveryActionRuntimeRepair
	}
	if failedStepHasRecoverableContractViolation(failedStep) {
		if runtimeAvailable && contractViolationRetryWithinBudget(ctx, r, runtimePolicy, run, failedStep) {
			return runtimeRecoveryActionContractViolationRetry
		}
		if runtimeAvailable && contractViolationExhaustionDecisionNotEligible(ctx, r, runtimePolicy, run, failedStep) {
			return runtimeRecoveryActionRuntimeRepair
		}
		if canCreateDecisionTicket {
			return runtimeRecoveryActionContractViolationExhausted
		}
	}
	if canCreateDecisionTicket && failedStepHasExternalRuntimeDependency(failedStep) {
		return runtimeRecoveryActionRuntimeRepairExhausted
	}
	if shouldEscalateRuntimeRepairExhaustion(ctx, r, runtimePolicy, canCreateDecisionTicket, run, failedStep) {
		if runtimeAvailable && runtimeRepairExhaustionDecisionNotEligible(run, failedStep) {
			return runtimeRecoveryActionRuntimeRepair
		}
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

func contractViolationExhaustionDecisionNotEligible(ctx context.Context, repair *repairSubsystem, policy RuntimePolicy, run *runModel.Run, failedStep *runModel.Step) bool {
	if repair == nil || run == nil || failedStep == nil || decisionEligibilityDeniedAlreadyAttempted(failedStep) {
		return false
	}
	if !failedStepHasRecoverableContractViolation(failedStep) || !contractViolationCurrentActionAllowsLLMRetry(run, failedStep) {
		return false
	}
	if contractViolationRetryWithinBudget(ctx, repair, policy, run, failedStep) {
		return false
	}
	return contractViolationExhaustionCanContinueRuntimeRepair(failedStep)
}

func runtimeRepairExhaustionDecisionNotEligible(run *runModel.Run, failedStep *runModel.Step) bool {
	if run == nil || failedStep == nil || decisionEligibilityDeniedAlreadyAttempted(failedStep) {
		return false
	}
	if failedStepRequestsContinuableDecision(failedStep) && !failedStepHasExternalDecisionDependency(failedStep) {
		return true
	}
	switch strings.TrimSpace(convergenceNextActionKind(run, failedStep.Input)) {
	case "assess_gap", "repair", "acquire_target_snapshot", "state_change", "produce_state_change",
		"validate", "validation", "produce_validation_evidence",
		"assessment", "complete_coverage_assessment",
		"artifact", "produce_artifact_evidence":
		return !failedStepHasExternalDecisionDependency(failedStep)
	default:
		return false
	}
}

func continuableContractViolationRule(rule string) bool {
	switch strings.TrimSpace(rule) {
	case contractViolationRuleValidationStaticOnly:
		return true
	default:
		return false
	}
}

func contractViolationExhaustionCanContinueRuntimeRepair(failedStep *runModel.Step) bool {
	if failedStep == nil || !continuableContractViolationRule(contractViolationRuleFromStep(failedStep)) {
		return false
	}
	if strings.TrimSpace(stringFromMap(failedStep.Input, "repairObjective")) != repairObjectiveConcreteValidationEvidence {
		return false
	}
	switch runtimeStageFromStepInput(failedStep.Input) {
	case runtimeStageVerification, runtimeStageRepair:
		return true
	default:
		return false
	}
}

func failedStepHasExternalRuntimeDependency(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	for _, source := range []map[string]any{
		mapFromValue(step.Output["validationFailureDiagnostic"]),
		mapFromValue(mapFromValue(step.Output["runtimeDiagnostics"])["validationFailureDiagnostic"]),
		mapFromValue(mapFromValue(step.Output["lastFailureDiagnostics"])["validationFailureDiagnostic"]),
	} {
		switch strings.TrimSpace(stringFromMap(source, "class")) {
		case runtimemodel.ValidationDiagnosticExternalDependency,
			runtimemodel.ValidationDiagnosticProviderTransient:
			return true
		}
	}
	return shellValidationFailureLooksExternalDependencyUnavailable(step.Output, nil)
}

func failedStepRequestsContinuableDecision(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		stringFromMap(step.Output, "error"),
		stringFromMap(step.Output, "reason"),
		stringFromMap(step.Output, "summary"),
	}, " ")))
	return strings.Contains(text, "decision requests a continuable candidate choice") ||
		boolValue(step.Output["continuableDecisionConvertedToRepair"])
}

func failedStepHasConvertedReadableWorkspaceResource(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	for _, decision := range []map[string]any{
		mapFromValue(step.Output["convertedDecision"]),
		mapFromValue(mapFromValue(step.Output["runtimeDiagnostics"])["convertedDecision"]),
	} {
		context := mapFromValue(decision["context"])
		if len(context) == 0 {
			continue
		}
		if knownReadableWorkspaceResourceDecisionContext(context) {
			return true
		}
		if len(stringListFromValue(context["readableResources"])) > 0 {
			return true
		}
	}
	return false
}

func failedStepHasExternalDecisionDependency(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	for _, source := range []map[string]any{
		step.Output,
		mapFromValue(step.Output[stepOutputKeyBlockDecisionContext]),
		mapFromValue(step.Output["runtimeDiagnostics"]),
		mapFromValue(step.Input["ticketContext"]),
	} {
		if len(source) == 0 {
			continue
		}
		if strings.TrimSpace(stringFromMap(source, decisionContextKeyDependency)) != "" ||
			strings.TrimSpace(stringFromMap(source, "externalDependency")) != "" ||
			boolValue(source["requiresExternalInput"]) ||
			boolValue(source["requiresUserChoice"]) ||
			boolValue(source["requiresCredential"]) ||
			boolValue(source["requiresPermission"]) {
			return true
		}
	}
	return false
}

func decisionEligibilityDeniedAlreadyAttempted(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	return boolValue(step.Input[stepInputKeyDecisionEligibilityDenied])
}

func annotateDecisionEligibilityDeniedRetry(input map[string]any, reason string) map[string]any {
	if input == nil {
		input = map[string]any{}
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "continuable_next_action_available"
	}
	input[stepInputKeyDecisionEligibilityDenied] = true
	input[stepInputKeyDecisionEligibilityDeniedReason] = reason
	input["decisionNotEligibleReason"] = reason
	input["decisionEligibilityGate"] = "automatic_next_action"
	return input
}

// blockRecoveryActionFromFailedStep first lets the executor persist the selected
// blocker state, then returns ErrBlocked as the dispatch-loop stop signal.
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
	case runtimeRecoveryActionContractViolationExhausted:
		if err := executor.blockOnContractViolationRecoveryExhaustion(ctx, run, failedStep, execErr); err != nil {
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
	case runtimeRecoveryActionContractViolationRetry:
		attempt := contractViolationRetryAttemptCount(ctx, r, run.ID, failedStep) + 1
		stepID, err := uniqueDispatchStepID(ctx, r.steps, run.ID, contractViolationRetryStepID(run.ID, failedStep, attempt))
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

func contractViolationRetryStepID(runID common.RunID, failedStep *runModel.Step, attempt int) common.StepID {
	base := ""
	if failedStep != nil {
		base = runtimeReadableStepLabel(runID, runtimeRepairIDLabelStepID(failedStep))
	}
	if base == "" {
		base = "contract"
	}
	if attempt < 1 {
		attempt = 1
	}
	return common.StepID(fmt.Sprintf("%s/contract-retry-%03d-of-%s", runID, attempt, base))
}

func (o *RuntimeOrchestrator) blockOnContractViolationRecoveryExhaustion(
	ctx context.Context,
	run *runModel.Run,
	failedStep *runModel.Step,
	execErr error,
) error {
	if run == nil || failedStep == nil {
		return common.ErrInvalidArgument
	}
	language := runDialogLanguage(run)
	policy := NormalizeRuntimePolicy(o.RuntimePolicy())
	attempts := contractViolationRetryAttemptCount(ctx, o.repairManager(), run.ID, failedStep)
	runAttempts := contractViolationRunRetryCount(ctx, o.repairManager(), run.ID)
	signatureRepeats := contractViolationSignatureRepeatCount(ctx, o.repairManager(), run.ID, failedStep)
	violation := contractViolationFromFailedOutput(failedStep)
	correction := contractViolationEscalationCorrectionContext(failedStep, violation)
	violation = contractViolationWithEscalationCorrection(violation, correction)
	context := map[string]any{
		decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
		decisionContextKeyResultPolicy:   decisionContextResultPolicyRejected,
		"allowedRecoveryActions":         []string{"replan", "terminate"},
		"runtimeRepairLoopState":         "contract_violation_recovery_exhausted",
		"failedStepId":                   string(failedStep.ID),
		"failedStepKind":                 failedStep.Kind,
		"failureClass":                   stringFromMap(failedStep.Output, failureOutputKeyClass),
		"failureSignature":               stringFromMap(failedStep.Output, failureOutputKeySignature),
		"attempts":                       attempts,
		"runAttempts":                    runAttempts,
		"signatureRepeatCount":           signatureRepeats,
		"maxRetriesPerStep":              policy.MaxContractViolationRetriesPerStep,
		"maxRetriesPerRun":               policy.MaxContractViolationRetriesPerRun,
		"maxRepeatedViolationSignature":  policy.MaxRepeatedContractViolationSignature,
		"contractViolation":              violation,
		"correctionProfileId":            stringFromMap(correction, "correctionProfileId"),
		"correctionMode":                 stringFromMap(correction, "correctionMode"),
		"correctionTargetScope":          stringFromMap(correction, "targetScope"),
		"correctionContract":             mapFromValue(correction["correctionContract"]),
		"failedStepInput":                o.llmContextManager().compactStepInput(failedStep.Input),
		"failedStepOutput":               o.llmContextManager().compactStepOutput(failedStep.Output),
	}
	if execErr != nil {
		context["error"] = execErr.Error()
	}
	spec := runtimeDecisionBlockSpec{
		TicketID:     common.TicketID(runtimeChildID(failedStep.ID, "contract-violation-decision")),
		Reason:       localizedContractViolationRecoveryExhaustedReason(language, violation),
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     localizedContractViolationRecoveryExhaustedQuestion(language),
		Context:      context,
	}
	if !o.decisionTicketEligible(run, failedStep, spec) {
		return o.failRunWithRejectedCompletionResult(ctx, run, failedStep, localizedContractViolationRecoveryExhaustedReason(language, violation))
	}
	if err := o.createDecisionTicket(ctx, run, failedStep, spec); err != nil {
		return err
	}
	if err := o.resultManager().recordRejectedResultForDecisionBlock(ctx, run, failedStep, failedStep.Output, spec); err != nil {
		return err
	}
	if err := o.blockOnDecisionAtStep(ctx, run, nil, spec.TicketID, failedStep.ID); err != nil {
		return err
	}
	return o.eventPublisher().publishContractViolationEscalated(ctx, run, failedStep, spec.TicketID, violation, context)
}

func contractViolationEscalationCorrectionContext(failedStep *runModel.Step, violation map[string]any) map[string]any {
	if failedStep == nil {
		return nil
	}
	recovery := mapFromValue(failedStep.Input[stepInputKeyContractViolationRecovery])
	if len(recovery) == 0 {
		recovery = violation
	}
	if len(recovery) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{"correctionProfileId", "correctionMode", "targetScope"} {
		if value := strings.TrimSpace(stringFromMap(recovery, key)); value != "" {
			ret[key] = value
		}
	}
	if contract := mapFromValue(recovery["correctionContract"]); len(contract) > 0 {
		ret["correctionContract"] = contract
	}
	return ret
}

func contractViolationWithEscalationCorrection(violation map[string]any, correction map[string]any) map[string]any {
	if len(violation) == 0 || len(correction) == 0 {
		return violation
	}
	ret := cloneStepPayload(violation)
	for _, key := range []string{"correctionProfileId", "correctionMode", "targetScope"} {
		if value := strings.TrimSpace(stringFromMap(correction, key)); value != "" {
			ret[key] = value
		}
	}
	if contract := mapFromValue(correction["correctionContract"]); len(contract) > 0 {
		ret["correctionContract"] = contract
	}
	return ret
}

func localizedContractViolationRecoveryExhaustedReason(language string, violation map[string]any) string {
	rule := stringFromMap(violation, "violatedRule")
	if rule == "" {
		rule = "contract_violation"
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "zh") {
		return "LLM 输出连续违反运行期合同，自动修正预算已耗尽: " + rule
	}
	return "LLM output repeatedly violated the runtime contract and automatic correction budget is exhausted: " + rule
}

func localizedContractViolationRecoveryExhaustedQuestion(language string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "zh") {
		return "请提供新的 replan 指令、补充缺失信息，或调整目标/验收口径后重新推进。"
	}
	return "Provide a new replan instruction, missing information, or adjusted goal/acceptance context before continuing."
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
		return stringFromMap(failedStep.Output, failureOutputKeyClass) == failureClassProviderUnavailable &&
			providerUnavailableFailureTextRequiresConfigBlock(failedStep)
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
	if runtimeRepairPatchFailureAlreadySatisfied(ctx, repair, run, failedStep) {
		return false
	}
	if runtimeRepairPatchFailureCanRefreshCurrentTarget(ctx, repair, run, failedStep) {
		return false
	}
	if convergenceBudgetRequiresEscalation(run) {
		return true
	}
	if repair.runLevelIneffectiveConvergenceExceeded(ctx, run, failedStep) {
		return true
	}
	return repair.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep) >= repair.effectiveMaxRuntimeRepairAttempts(ctx, policy, run, failedStep)
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
	maxAttempts := o.repairManager().effectiveMaxRuntimeRepairAttempts(ctx, o.RuntimePolicy(), run, failedStep)
	reason := localizedRuntimeRepairExhaustionReason(language, maxAttempts, execErr)
	diagnostics := runtimeFailureDecisionDiagnostics(failedStep, execErr)
	if diagnostics == nil {
		diagnostics = map[string]any{}
	}
	runLevelStats := o.repairManager().runLevelIneffectiveConvergenceStats(ctx, run, failedStep)
	blockerKind := "runtime_repair_contract_exhausted"
	decisionScope := decisionContextScopeWorkorchRuntime
	decisionDependency := decisionContextDependencyRuntimeContract
	allowedRecoveryActions := []string{"retry_same_step", "replan", "provide_information", "terminate"}
	preferredRecoveryActions := []string{"retry_same_step", "replan", "terminate"}
	externalDependencyUnavailable := boolValue(diagnostics["externalDependencyUnavailable"]) || failedStepHasExternalRuntimeDependency(failedStep)
	if externalDependencyUnavailable {
		blockerKind = "external_dependency_unavailable"
		decisionScope = decisionContextScopeExternalDependency
		decisionDependency = "external_dependency"
		allowedRecoveryActions = []string{"provide_information", "retry_same_step", "replan", "terminate"}
		preferredRecoveryActions = []string{"provide_information", "retry_same_step", "replan"}
	}
	if runLevelStats.exceeded() && !externalDependencyUnavailable {
		blockerKind = "runtime_ineffective_convergence"
		allowedRecoveryActions = []string{"replan", "provide_information", "terminate"}
		preferredRecoveryActions = []string{"replan", "terminate"}
	}
	if budgetContext := convergenceBudgetEscalationContext(run); len(budgetContext) > 0 && !externalDependencyUnavailable {
		blockerKind = "runtime_convergence_budget_exhausted"
		allowedRecoveryActions = []string{"replan", "provide_information", "terminate"}
		preferredRecoveryActions = []string{"replan", "terminate"}
		reason = localizedRuntimeConvergenceBudgetExhaustionReason(language, budgetContext, execErr)
		diagnostics["convergenceBudget"] = budgetContext
	}
	recommendedNextActions := []string{
		"Map each missing DoD item to concrete affected resources, state-changing operations, and verification actions.",
		"Prioritize minimal target changes, then rerun targeted verification.",
		"If key constraints remain unclear, provide a focused decision payload with only missing information.",
	}
	if actions := stringSliceFromValue(diagnostics["recommendedNextActions"]); len(actions) > 0 {
		recommendedNextActions = actions
	}
	decisionContext := map[string]any{
		decisionContextKeyScope:          decisionScope,
		decisionContextKeyDependency:     decisionDependency,
		decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
		decisionContextKeyResultPolicy:   decisionContextResultPolicyRejected,
		"allowedRecoveryActions":         allowedRecoveryActions,
		"preferredRecoveryActions":       preferredRecoveryActions,
		"blockerKind":                    blockerKind,
		"attempts":                       o.repairManager().runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep),
		"maxAttempts":                    maxAttempts,
		"runLevelIneffectiveConvergence": map[string]any{
			"totalSteps":          runLevelStats.TotalSteps,
			"recoverySteps":       runLevelStats.RecoverySteps,
			"contractFailures":    runLevelStats.ContractFailures,
			"coverageFailures":    runLevelStats.CoverageFailures,
			"providerFailures":    runLevelStats.ProviderFailures,
			"failedLLMSteps":      runLevelStats.FailedLLMSteps,
			"currentIsContract":   runLevelStats.CurrentIsContract,
			"currentIsCoverage":   runLevelStats.CurrentIsCoverage,
			"minRecoverySteps":    minIneffectiveConvergenceRecoverySteps,
			"minContractFailures": minIneffectiveConvergenceContractFailures,
			"recentFailures":      runLevelStats.RecentFailures,
		},
		"contractViolationType":  stringFromMap(failedStep.Output, outputKeyContractViolationType),
		"failureClass":           stringFromMap(failedStep.Output, failureOutputKeyClass),
		"failureSignature":       stringFromMap(failedStep.Output, failureOutputKeySignature),
		"failureTargetScope":     runtimeFailureTargetScopeFromStep(failedStep),
		"failedStepId":           string(failedStep.ID),
		"repairOfRootStepId":     string(runtimeRepairRootStepID(failedStep)),
		"failedStepKind":         failedStep.Kind,
		"failedStepInput":        o.llmContextManager().compactStepInput(failedStep.Input),
		"failedStepOutput":       o.llmContextManager().compactStepOutput(failedStep.Output),
		"missingDoDItems":        missingDoDItemsFromFailureOutput(failedStep.Output),
		"recentRepairAttempts":   o.repairManager().recentRuntimeRepairAttemptSummaries(ctx, run.ID, failedStep, maxAttempts),
		"recommendedNextActions": recommendedNextActions,
	}
	for key, value := range diagnostics {
		if key == "recommendedNextActions" {
			continue
		}
		decisionContext[key] = value
	}
	spec := runtimeDecisionBlockSpec{
		TicketID:     common.TicketID(runtimeChildID(failedStep.ID, "repair-exhausted-decision")),
		Reason:       reason,
		DecisionKind: decisioncommon.ResolutionTypeProvideMissingInfo,
		Question:     localizedRuntimeRepairExhaustionQuestion(language),
		Context:      decisionContext,
	}
	if !o.decisionTicketEligible(run, failedStep, spec) {
		return o.failRunWithRejectedCompletionResult(ctx, run, failedStep, reason)
	}
	if err := o.resultManager().recordRejectedResultForDecisionBlock(ctx, run, failedStep, failedStep.Output, spec); err != nil {
		return err
	}
	if err := o.createDecisionTicket(ctx, run, failedStep, spec); err != nil {
		return err
	}
	return o.blockOnDecisionAtStep(ctx, run, nil, spec.TicketID, failedStep.ID)
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
	if runtimeRepairPatchFailureAlreadySatisfied(ctx, repair, run, failedStep) {
		return true
	}
	if runtimeRepairPatchFailureCanRefreshCurrentTarget(ctx, repair, run, failedStep) {
		return true
	}
	if repair.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep) >= repair.effectiveMaxRuntimeRepairAttempts(ctx, policy, run, failedStep) {
		return false
	}
	return !isLLMProviderRuntimeFailure(run, failedStep)
}

func runtimeRepairPatchFailureAlreadySatisfied(ctx context.Context, repair *repairSubsystem, run *runModel.Run, failedStep *runModel.Step) bool {
	if repair == nil || run == nil || failedStep == nil {
		return false
	}
	return len(repair.runtimeRepairPatchAlreadySatisfiedSignals(ctx, run, failedStep)) > 0
}

func runtimeRepairPatchFailureCanRefreshCurrentTarget(ctx context.Context, repair *repairSubsystem, run *runModel.Run, failedStep *runModel.Step) bool {
	if repair == nil || run == nil || failedStep == nil {
		return false
	}
	return len(repair.runtimeRepairPatchRefreshableSignals(ctx, run, failedStep)) > 0
}

func shouldAttemptProviderRetry(ctx context.Context, repair *repairSubsystem, policy RuntimePolicy, runtimeAvailable bool, run *runModel.Run, failedStep *runModel.Step) bool {
	if repair == nil || run == nil || failedStep == nil || !runtimeAvailable {
		return false
	}
	attempts := repair.providerRetryAttemptCount(ctx, run.ID, providerRetryRootStepID(failedStep))
	if attempts >= repair.effectiveMaxProviderRetryAttempts(ctx, policy, run, failedStep) {
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
	return repair.providerRetryAttemptCount(ctx, run.ID, providerRetryRootStepID(failedStep)) >= repair.effectiveMaxProviderRetryAttempts(ctx, policy, run, failedStep)
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
	maxAttempts := o.repairManager().effectiveMaxProviderRetryAttempts(ctx, o.RuntimePolicy(), run, failedStep)
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
	return o.blockOnDecisionAtStep(ctx, run, nil, ticketID, failedStep.ID)
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
			"allowedRecoveryActions":         contextGateAllowedRecoveryActions(gateAction),
			"recommendedRecoveryAction":      contextGateRecommendedRecoveryAction(gateAction),
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
	return o.blockOnDecisionAtStep(ctx, run, nil, ticketID, failedStep.ID)
}

func contextGateAllowedRecoveryActions(action string) []string {
	switch strings.TrimSpace(action) {
	case "read_required":
		return []string{"provide_information", "retry_context", "terminate"}
	case "degrade_to_assessment":
		return []string{"degrade_to_assessment", "replan", "terminate"}
	case "blocker_required":
		return []string{"provide_information", "replan", "terminate"}
	case "replan_required":
		return []string{"replan", "provide_information", "terminate"}
	case "fail_fast":
		return []string{"terminate", "replan"}
	default:
		return []string{"provide_information", "replan", "terminate"}
	}
}

func contextGateRecommendedRecoveryAction(action string) string {
	switch strings.TrimSpace(action) {
	case "read_required",
		"degrade_to_assessment",
		"blocker_required",
		"replan_required",
		"fail_fast":
		return strings.TrimSpace(action)
	default:
		return "provide_information"
	}
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

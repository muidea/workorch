package orchestrator

import (
	"context"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func stateChangingRepairRequired(run *runModel.Run, failedStep *runModel.Step, execErr error) bool {
	if failedStep == nil {
		return false
	}
	if capabilityInputLocatorInvalidFailure(failedStep) {
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
	if contractViolationRequiresStateChangingRepair(stringFromMap(failedStep.Output, outputKeyContractViolationType)) {
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

func contractViolationRequiresStateChangingRepair(violationType string) bool {
	switch strings.TrimSpace(violationType) {
	case contractViolationGraphNodeReturnedGraph,
		contractViolationRepairReturnedGraph,
		contractViolationRepairNoStateChangingOutput,
		contractViolationRepairGraphWithoutPatchApply,
		contractViolationExecutionNoStateChangingOutput,
		contractViolationGraphDependsOnLLMBeforePatch,
		contractViolationPatchTargetRefRequired,
		contractViolationPatchContentRequired,
		contractViolationPatchUnresolvedTemplate,
		contractViolationPatchDispatchInvalid,
		contractViolationFileEditConflict:
		return true
	default:
		return false
	}
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

func coverageValidationRepairShouldEscalateToStateChanging(
	ctx context.Context,
	repair *repairSubsystem,
	run *runModel.Run,
	failedStep *runModel.Step,
	input map[string]any,
) bool {
	if run == nil || failedStep == nil {
		return false
	}
	if !coverageValidationEvidenceRepairEligible(failedStep) ||
		!coverageVerificationGapPresent(input, failedStep) {
		return false
	}
	if repair.runtimeRepairAttemptCountForEffectiveScope(ctx, run.ID, failedStep) == 0 {
		return false
	}
	if len(mapSliceFromValue(input["currentTargetAlreadySatisfiesFailedPatch"])) > 0 ||
		boolValue(input["stateChangeAlreadySatisfiedByCurrentTarget"]) {
		return false
	}
	if coverageRepairInputRequiresStateChange(input) &&
		!runtimeRepairRunHasCompletedStateChangeEvidence(ctx, repair, run.ID) {
		return true
	}
	return runtimeRepairHasConcreteImplementationTargetEvidence(input, failedStep) ||
		runtimeRepairInputHasEmptyWorkspaceSurfaceEvidence(failedStep)
}

func coverageRepairInputRequiresStateChange(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if coverageWorkItemSetRequiresEvidenceKind(mapSliceFromValue(input["coverageTargetSet"]), "state_change") ||
		coverageWorkItemSetRequiresEvidenceKind(mapSliceFromValue(input["unresolvedCoverageWorkItems"]), "state_change") ||
		coverageWorkItemSetRequiresEvidenceKind(mapSliceFromValue(input["missingDoDItems"]), "state_change") ||
		coverageWorkItemSetRequiresEvidenceKind(mapSliceFromValue(input["missingGoalCoverage"]), "state_change") {
		return true
	}
	for _, key := range []string{"coverageWorkItemContext", "runtimeDiagnostics"} {
		context := mapFromValue(input[key])
		if coverageWorkItemSetRequiresEvidenceKind(mapSliceFromValue(context["currentFocusItems"]), "state_change") ||
			coverageWorkItemSetRequiresEvidenceKind(mapSliceFromValue(context["unresolvedItems"]), "state_change") ||
			coverageWorkItemRequiresEvidenceKind(mapFromValue(context["currentFocus"]), "state_change") {
			return true
		}
	}
	if pkg := mapFromValue(input["repairActionPackage"]); len(pkg) > 0 {
		return coverageRepairInputRequiresStateChange(pkg)
	}
	return false
}

func coverageWorkItemSetRequiresEvidenceKind(items []map[string]any, kind string) bool {
	for _, item := range items {
		if coverageWorkItemRequiresEvidenceKind(item, kind) {
			return true
		}
	}
	return false
}

func coverageWorkItemRequiresEvidenceKind(item map[string]any, kind string) bool {
	if len(item) == 0 || strings.TrimSpace(kind) == "" {
		return false
	}
	for _, key := range []string{"missingEvidenceKinds", "requiredEvidenceKinds"} {
		for _, value := range stringSliceFromValue(item[key]) {
			if strings.TrimSpace(value) == kind {
				return true
			}
		}
	}
	return false
}

func runtimeRepairRunHasCompletedStateChangeEvidence(ctx context.Context, repair *repairSubsystem, runID common.RunID) bool {
	if repair == nil || repair.steps == nil || runID == "" {
		return false
	}
	steps, err := repair.steps.ListByRunSummary(ctx, runID)
	if err != nil {
		return false
	}
	for _, step := range steps {
		if runtimeStepIsCompletedStateChangeEvidence(step) {
			return true
		}
	}
	return false
}

func runtimeStepIsCompletedStateChangeEvidence(step *runModel.Step) bool {
	if step == nil || step.State != common.StepStateCompleted {
		return false
	}
	if boolValue(step.Output["zeroWork"]) {
		return false
	}
	switch strings.TrimSpace(step.Kind) {
	case "patch/apply", "file/edit", "file/write":
		return true
	case "git/write":
		return strings.TrimSpace(stringFromMap(step.Output, "operation")) == "commit"
	case "shell/command", "mcp/tool_call":
		return boolValue(step.Input["stateChanging"]) || boolValue(step.Output["stateChanging"])
	default:
		return false
	}
}

func emptyWorkspaceEvidenceShouldStartStateChangingRepair(
	run *runModel.Run,
	failedStep *runModel.Step,
	input map[string]any,
) bool {
	if run == nil || failedStep == nil || !runExecutionContractRequiresStateChange(run) {
		return false
	}
	if !stepRequiresConcreteFailureEvidenceAcquisition(failedStep) {
		return false
	}
	class := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(failedStep.Output, failureOutputKeyClass),
		stringFromMap(input, failureOutputKeyClass),
	))
	if class != failureClassDoDCoverageMissing {
		return false
	}
	if len(missingDoDItemsFromFailureOutput(failedStep.Output)) == 0 &&
		len(missingGoalCoverageFromFailureOutput(failedStep.Output)) == 0 &&
		len(missingGoalCoverageFromStep(failedStep)) == 0 {
		return false
	}
	return repairInputHasEmptyWorkspaceSurfaceEvidence(input) ||
		runtimeRepairInputHasEmptyWorkspaceSurfaceEvidence(failedStep)
}

func runtimeRepairHasConcreteImplementationTargetEvidence(input map[string]any, failedStep *runModel.Step) bool {
	if repairHasImplementationTargetEvidence(input, mapFromValue(input["repairActionPackage"])) {
		return true
	}
	if repairInputHasEmptyWorkspaceSurfaceEvidence(input) {
		return true
	}
	if failedStep == nil {
		return false
	}
	if runtimeRepairInputHasEmptyWorkspaceSurfaceEvidence(failedStep) {
		return true
	}
	return repairHasImplementationTargetEvidence(failedStep.Input, mapFromValue(failedStep.Input["repairActionPackage"]))
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

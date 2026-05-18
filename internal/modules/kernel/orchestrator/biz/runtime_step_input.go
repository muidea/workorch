package orchestrator

import (
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func nextStepSpec(output map[string]any) (common.StepID, string) {
	if output == nil {
		return "", ""
	}
	nextStep, _ := output[stepOutputKeyNextStep].(string)
	nextKind, _ := output[stepOutputKeyNextKind].(string)
	return common.StepID(nextStep), nextKind
}

func initialStepKind(run *runModel.Run) string {
	if run != nil && strings.TrimSpace(run.InitialStepKind) != "" {
		return strings.TrimSpace(run.InitialStepKind)
	}
	return defaultInitialStepKind
}

func initialStepInput(run *runModel.Run) map[string]any {
	input := nextStepInput(run, "")
	if input == nil {
		return nil
	}
	if initialStepKind(run) == llmWorkspaceExecutionStepKind {
		input[stepInputKeySoulStage] = soulStagePlanning
		setRuntimeStage(input, runtimeStagePlanning)
		if replanRunRequiresStateChangingOutput(run) {
			input[stepInputKeySoulStage] = soulStageExecution
			setRuntimeStage(input, runtimeStageRepair)
			input["stateChangingOutputRequired"] = true
			input["repairObjective"] = repairObjectiveStateChangingOutput
			input["repairGuidance"] = stateChangingRepairGuidance()
			input["replanStateChangingContinuation"] = true
			input["repairActionPackage"] = repairActionPackageFromReplanContinuation(run)
		}
	}
	return input
}

func replanRunRequiresStateChangingOutput(run *runModel.Run) bool {
	if run == nil || !runExecutionContractRequiresStateChange(run) {
		return false
	}
	return len(mapFromValue(run.ExecutionContext["replanContinuation"])) > 0
}

func repairActionPackageFromReplanContinuation(run *runModel.Run) map[string]any {
	if run == nil {
		return nil
	}
	continuation := mapFromValue(run.ExecutionContext["replanContinuation"])
	if len(continuation) == 0 {
		return nil
	}
	failureContext := mapFromValue(continuation["failureContext"])
	if len(failureContext) == 0 {
		failureContext = mapFromValue(continuation["ticketContext"])
	}
	pkg := map[string]any{
		"requiredOutput": map[string]any{
			"mustIncludeStateChangingNode": true,
			"acceptedKinds":                []string{"patch/apply"},
			"forbiddenKinds": []string{
				"read_only_graph",
				"shell_only_graph",
				"llm_gated_graph",
				"self_inspection_graph",
				"context_rehydration_graph",
			},
		},
		"predecessorRunId": stringFromMap(continuation, "predecessorRunId"),
		"reason":           stringFromMap(continuation, "reason"),
	}
	for _, key := range []string{
		failureOutputKeyClass,
		failureOutputKeySignature,
		"failureTargetScope",
		"failedStepId",
		"failedStepKind",
		"attempts",
		"maxAttempts",
		"repairOfRootStepId",
	} {
		if value, ok := failureContext[key]; ok && value != nil {
			pkg[key] = compactLLMContextValue(value, 1)
		}
	}
	if len(mapFromValue(failureContext["failedStepOutput"])) > 0 {
		pkg["previousRejectedOutputSummary"] = compactStateChangingRecoveryFailureOutput(mapFromValue(failureContext["failedStepOutput"]))
	}
	if targets := repairActionTargetSnapshots(failureContext); len(targets) > 0 {
		pkg["targetSnapshots"] = targets
	}
	if len(pkg) <= 3 {
		return nil
	}
	return pkg
}

func nextStepInputFromOutput(run *runModel.Run, previousStep *runModel.Step) map[string]any {
	if previousStep == nil {
		return nextStepInput(run, "")
	}
	input := nextStepInput(run, previousStep.ID)
	inheritWorkspaceFileManifest(input, previousStep)
	inheritStableExecutionContextPackage(input, previousStep)
	inheritRuntimeRepairLineage(input, previousStep)
	if nextInput := mapFromValue(previousStep.Output[stepOutputKeyNextInput]); nextInput != nil {
		for key, value := range nextInput {
			input[key] = value
		}
	}
	return input
}

func inheritRuntimeRepairLineage(input map[string]any, previousStep *runModel.Step) {
	if input == nil || previousStep == nil || previousStep.Input == nil {
		return
	}
	for _, key := range []string{
		stepInputKeyRepairOfStepID,
		stepInputKeyRepairOfRootStepID,
		stepInputKeyRepairOfFailureClass,
		stepInputKeyRepairOfFailureSig,
	} {
		if value, ok := previousStep.Input[key]; ok {
			input[key] = value
		}
	}
}

func inheritWorkspaceFileManifest(input map[string]any, previousStep *runModel.Step) {
	if input == nil || previousStep == nil {
		return
	}
	manifest := mapSliceFromValue(previousStep.Input[stepInputKeyWorkspaceFileManifest])
	if len(manifest) == 0 {
		if pkg := mapFromValue(previousStep.Input[stepInputKeyExecutionContextPackage]); pkg != nil {
			manifest = mapSliceFromValue(pkg[stepInputKeyWorkspaceFileManifest])
		}
	}
	if len(manifest) == 0 {
		return
	}
	input[stepInputKeyWorkspaceFileManifest] = compactWorkspaceFileManifest(manifest)
}

func inheritStableExecutionContextPackage(input map[string]any, previousStep *runModel.Step) {
	if input == nil || previousStep == nil {
		return
	}
	stablePayload := stableExecutionContextPackagePayload(stableExecutionContextPackageFromStep(previousStep))
	if len(stablePayload) == 0 {
		return
	}
	input[stepInputKeyExecutionContextPackage] = stablePayload
}

func runtimeChildID(parent common.StepID, suffix string) string {
	base := strings.TrimSpace(string(parent))
	base = strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(base)
	suffix = strings.TrimSpace(suffix)
	if base == "" {
		return suffix
	}
	if suffix == "" {
		return base
	}
	return base + "-" + suffix
}

func runtimeReadableIDPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-", "_", "-").Replace(value)
	value = strings.Trim(value, "-")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	if len(value) > 64 {
		value = strings.Trim(value[:64], "-")
	}
	return value
}

func runtimeReadableStepLabel(runID common.RunID, stepID common.StepID) string {
	raw := strings.TrimSpace(string(stepID))
	if raw == "" {
		return ""
	}
	runPrefix := strings.TrimSpace(string(runID)) + "/"
	if strings.TrimSpace(string(runID)) != "" && strings.HasPrefix(raw, runPrefix) {
		raw = strings.TrimPrefix(raw, runPrefix)
	}
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return "run-root"
	}
	if raw == "step-1" {
		return "planning-001"
	}
	parts := strings.Split(raw, "/")
	first := strings.TrimSpace(parts[0])
	switch {
	case strings.HasPrefix(first, "planning-"),
		strings.HasPrefix(first, "repair-"),
		strings.HasPrefix(first, "provider-retry-"),
		strings.HasPrefix(first, "follow-up-"),
		strings.HasPrefix(first, "assessment-correction-"):
		return runtimeReadableStageLabel(first)
	case strings.HasPrefix(first, "graph-") && len(parts) > 1:
		return runtimeReadableIDPart(first + "-" + parts[len(parts)-1])
	}
	return runtimeReadableIDPart(first)
}

func runtimeReadableStageLabel(value string) string {
	label := runtimeReadableIDPart(value)
	if strings.Contains(label, "provider-retry-") {
		if idx := strings.Index(label, "-of-"); idx > 0 {
			label = strings.Trim(label[:idx], "-")
		}
	}
	return label
}

func nextStepInput(run *runModel.Run, previousStepID common.StepID) map[string]any {
	input := runExecutionInput(run, previousStepID)
	if input == nil {
		return nil
	}
	if previousStepID == "" {
		delete(input, "previousStepId")
	}
	return input
}

func runExecutionInput(run *runModel.Run, previousStepID common.StepID) map[string]any {
	if run == nil {
		return nil
	}
	input := map[string]any{
		"workspaceId":    string(run.WorkspaceID),
		"previousStepId": string(previousStepID),
	}
	if run.GoalID != "" {
		input["goalId"] = string(run.GoalID)
	}
	if run.DoDID != "" {
		input["doDId"] = string(run.DoDID)
	}
	if executionContext := slimExecutionContextForStepInput(run.ExecutionContext); len(executionContext) > 0 {
		input["executionContext"] = executionContext
	}
	return input
}

func slimExecutionContextForStepInput(executionContext map[string]any) map[string]any {
	if len(executionContext) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{
		"dialogLanguage",
		"language",
		"locale",
		"systemLanguage",
		"systemLocale",
		"llmOperation",
		"soulId",
	} {
		if value, ok := executionContext[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	for _, key := range []string{
		"compressedContext",
		"soul",
		"soulRouting",
	} {
		if value, ok := executionContext[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	if value := mapFromValue(executionContext["replanContinuation"]); len(value) > 0 {
		ret["replanContinuation"] = compactReplanContinuationForRuntimeContext(value)
	}
	if value := mapFromValue(executionContext["rejectedResultContinuation"]); len(value) > 0 {
		ret["rejectedResultContinuation"] = compactRejectedResultContinuationForRuntimeContext(value)
	}
	if value := mapSliceFromValue(executionContext["governanceHistory"]); len(value) > 0 {
		ret["governanceHistory"] = compactGovernanceHistoryForRuntimeContext(value, 6)
	}
	return ret
}

func cloneStepPayload(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

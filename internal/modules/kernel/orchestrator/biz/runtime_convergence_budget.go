package orchestrator

import (
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func updateConvergenceBudgetFromStep(run *runModel.Run, step *runModel.Step, coverageDelta bool) bool {
	if run == nil || step == nil || run.ExecutionContext == nil {
		return false
	}
	focus := mapFromValue(run.ExecutionContext[coverageFocusKey])
	if len(focus) == 0 {
		return false
	}
	focusID := strings.TrimSpace(stringFromMap(focus, "id"))
	if focusID == "" {
		return false
	}
	budget := mapFromValue(run.ExecutionContext[convergenceBudgetKey])
	if len(budget) == 0 {
		budget = convergenceBudgetFromPolicy(acceptancePolicyFromExecutionContext(run.ExecutionContext), nil)
	}
	before := stableCoverageJSON(budget)
	ensureConvergenceBudgetFocusScope(budget, focus)
	if convergenceBudgetCountsLLMRound(run, step) {
		budget["llmRounds"] = int64FromValue(budget["llmRounds"]) + 1
	}
	if convergenceBudgetCountsRepairAttempt(step) {
		budget["repairAttempts"] = int64FromValue(budget["repairAttempts"]) + 1
		focus["attempt"] = int64FromValue(budget["repairAttempts"])
	}
	if coverageDelta {
		budget["noCoverageDeltaRounds"] = int64(0)
		delete(budget, "lastNoDeltaReason")
		if convergenceBudgetEscalatedDecision(stringFromMap(budget, "decision")) {
			budget["decision"] = "continue"
			budget["state"] = "tracking"
			delete(budget, "lastDecisionReason")
		}
	} else if convergenceBudgetCountsNoDelta(step) {
		budget["noCoverageDeltaRounds"] = int64FromValue(budget["noCoverageDeltaRounds"]) + 1
		budget["lastNoDeltaReason"] = convergenceBudgetNoDeltaReason(step)
	}
	if step.State == common.StepStateFailed {
		signature := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeySignature))
		if signature != "" {
			if strings.TrimSpace(stringFromMap(budget, "lastFailureSignature")) == signature {
				budget["repeatedFailureSignatures"] = int64FromValue(budget["repeatedFailureSignatures"]) + 1
			} else {
				budget["repeatedFailureSignatures"] = int64(1)
				budget["lastFailureSignature"] = signature
			}
			focus["failureSignature"] = signature
		}
		if class := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass)); class != "" {
			focus["failureClass"] = class
		}
		if convergenceBudgetCountsValidationOnlyRetry(step) {
			budget["validationOnlyRetries"] = int64FromValue(budget["validationOnlyRetries"]) + 1
		}
	}
	applyConvergenceBudgetDecision(budget)
	run.ExecutionContext[convergenceBudgetKey] = budget
	run.ExecutionContext[coverageFocusKey] = focus
	return before != stableCoverageJSON(budget)
}

func resetConvergenceBudgetForReplanSuccessor(executionContext map[string]any) bool {
	if len(executionContext) == 0 {
		return false
	}
	existing := mapFromValue(executionContext[convergenceBudgetKey])
	if len(existing) == 0 {
		return false
	}
	reset := map[string]any{}
	for key := range defaultConvergenceBudgetLimits() {
		if value := positiveIntFromValue(existing[key], 0); value > 0 {
			reset[key] = value
		}
	}
	if focusID := strings.TrimSpace(stringFromMap(existing, "focusId")); focusID != "" {
		reset["focusId"] = focusID
	}
	if sliceID := strings.TrimSpace(stringFromMap(existing, "sliceId")); sliceID != "" {
		reset["sliceId"] = sliceID
	}
	reset["state"] = "tracking"
	reset["decision"] = "continue"
	if stableCoverageJSON(existing) == stableCoverageJSON(reset) {
		return false
	}
	executionContext[convergenceBudgetKey] = reset
	return true
}

func ensureConvergenceBudgetFocusScope(budget map[string]any, focus map[string]any) {
	if len(budget) == 0 || len(focus) == 0 {
		return
	}
	focusID := strings.TrimSpace(stringFromMap(focus, "id"))
	sliceID := strings.TrimSpace(stringFromMap(focus, "sliceId"))
	if strings.TrimSpace(stringFromMap(budget, "focusId")) == focusID &&
		strings.TrimSpace(stringFromMap(budget, "sliceId")) == sliceID {
		return
	}
	for _, key := range []string{
		"repairAttempts",
		"llmRounds",
		"repeatedFailureSignatures",
		"validationOnlyRetries",
		"noCoverageDeltaRounds",
		"lastFailureSignature",
		"lastNoDeltaReason",
		"lastDecisionReason",
	} {
		delete(budget, key)
	}
	budget["focusId"] = focusID
	budget["sliceId"] = sliceID
	budget["state"] = "tracking"
	budget["decision"] = "continue"
	focus["attempt"] = int64(0)
}

func convergenceBudgetCountsLLMRound(run *runModel.Run, step *runModel.Step) bool {
	if run == nil || step == nil {
		return false
	}
	return strings.TrimSpace(step.Kind) == initialStepKind(run)
}

func convergenceBudgetCountsRepairAttempt(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	if canonicalLLMWorkspaceStepKind(strings.TrimSpace(step.Kind)) != llmWorkspaceExecutionStepKind {
		return false
	}
	if stepIsConcreteFailureEvidenceAcquisitionGraphNode(step) {
		return false
	}
	if stepRequiresConcreteFailureEvidenceAcquisition(step) && !boolValue(step.Input["stateChangingOutputRequired"]) {
		return false
	}
	return runtimeStageFromStepInput(step.Input) == runtimeStageRepair ||
		boolValue(step.Input[stepInputKeyRuntimeRepairFollowUp]) ||
		len(mapFromValue(step.Input["repairActionPackage"])) > 0
}

func convergenceBudgetCountsNoDelta(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	if convergenceBudgetSkipsNoDeltaForPendingExecutionGraph(step) {
		return false
	}
	if convergenceBudgetSkipsNoDeltaForIntermediateCompletedGraphNode(step) {
		return false
	}
	if stepIsConcreteFailureEvidenceAcquisitionGraphNode(step) {
		return false
	}
	if convergenceBudgetSkipsNoDeltaForMechanicalPatchConflict(step) {
		return false
	}
	return step.State == common.StepStateCompleted || step.State == common.StepStateFailed
}

func convergenceBudgetSkipsNoDeltaForPendingExecutionGraph(step *runModel.Step) bool {
	if step == nil || step.State != common.StepStateCompleted {
		return false
	}
	if canonicalLLMWorkspaceStepKind(strings.TrimSpace(step.Kind)) != llmWorkspaceExecutionStepKind {
		return false
	}
	graph, ok := runtimeGraphSpecFromValue(step.Output[stepOutputKeyExecutionGraph])
	return ok && len(graph.Nodes) > 0
}

func convergenceBudgetSkipsNoDeltaForIntermediateCompletedGraphNode(step *runModel.Step) bool {
	if step == nil || step.State != common.StepStateCompleted || graphNodeID(step) == "" {
		return false
	}
	return len(mapFromValue(step.Input["validationEvidence"])) == 0 &&
		len(mapFromValue(step.Output["validationEvidence"])) == 0
}

func convergenceBudgetSkipsNoDeltaForMechanicalPatchConflict(step *runModel.Step) bool {
	if step == nil || step.State != common.StepStateFailed {
		return false
	}
	if strings.TrimSpace(step.Kind) != "patch/apply" &&
		strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass)) != failureClassPatchApply {
		return false
	}
	return runtimeMechanicalPatchConflictFailure(step)
}

func runtimeMechanicalPatchConflictFailure(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	textValues := repairForbiddenReuseTextValues(step.Input, step.Output)
	if diagnostics := mapFromValue(step.Output["runtimeDiagnostics"]); len(diagnostics) > 0 {
		textValues = append(textValues,
			stringFromMap(diagnostics, "errorDetail"),
			stringFromMap(diagnostics, "guidance"),
		)
	}
	text := strings.ToLower(strings.Join(textValues, "\n"))
	return strings.Contains(text, "basehash mismatch") ||
		(strings.Contains(text, "basehash") && strings.Contains(text, "mismatch")) ||
		strings.Contains(text, "oldtext not found") ||
		(strings.Contains(text, "oldtext") && strings.Contains(text, "not found"))
}

func stepIsConcreteFailureEvidenceAcquisitionGraphNode(step *runModel.Step) bool {
	if step == nil || step.Input == nil || graphNodeID(step) == "" {
		return false
	}
	return stepRequiresConcreteFailureEvidenceAcquisition(step) && !boolValue(step.Input["stateChangingOutputRequired"])
}

func convergenceBudgetCountsValidationOnlyRetry(step *runModel.Step) bool {
	if step == nil || step.State != common.StepStateFailed {
		return false
	}
	if diagnostic := mapFromValue(step.Output["validationFailureDiagnostic"]); len(diagnostic) > 0 {
		return true
	}
	if required := mapFromValue(mapFromValue(step.Input["repairActionPackage"])["requiredOutput"]); boolValue(required["mustProduceConcreteValidationEvidence"]) {
		return true
	}
	return strings.TrimSpace(step.Kind) == "shell/command" || strings.TrimSpace(step.Kind) == "mcp/tool_call"
}

func convergenceBudgetNoDeltaReason(step *runModel.Step) string {
	if step == nil {
		return "no_coverage_delta"
	}
	if step.State == common.StepStateFailed {
		if class := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass)); class != "" {
			return "failed_without_coverage_delta:" + class
		}
		return "failed_without_coverage_delta"
	}
	return "completed_without_coverage_delta"
}

func applyConvergenceBudgetDecision(budget map[string]any) {
	if len(budget) == 0 {
		return
	}
	decision, reason := convergenceBudgetDecision(budget)
	if decision == "" {
		budget["decision"] = "continue"
		if strings.TrimSpace(stringFromMap(budget, "state")) == "" {
			budget["state"] = "tracking"
		}
		return
	}
	budget["decision"] = decision
	budget["state"] = "escalated"
	budget["lastDecisionReason"] = reason
}

func convergenceBudgetDecision(budget map[string]any) (string, string) {
	checks := []struct {
		countKey string
		maxKey   string
		reason   string
	}{
		{"repairAttempts", "maxRepairAttemptsPerFocus", "max_repair_attempts_per_focus_exceeded"},
		{"llmRounds", "maxLLMRoundsPerSlice", "max_llm_rounds_per_slice_exceeded"},
		{"repeatedFailureSignatures", "maxRepeatedFailureSignature", "max_repeated_failure_signature_exceeded"},
		{"validationOnlyRetries", "maxValidationOnlyRetries", "max_validation_only_retries_exceeded"},
		{"noCoverageDeltaRounds", "maxNoCoverageDeltaRounds", "max_no_coverage_delta_rounds_exceeded"},
	}
	for _, check := range checks {
		max := int64FromValue(budget[check.maxKey])
		if max <= 0 {
			continue
		}
		if int64FromValue(budget[check.countKey]) >= max {
			return "escalate_decision", check.reason
		}
	}
	return "", ""
}

func convergenceBudgetEscalatedDecision(decision string) bool {
	switch strings.TrimSpace(decision) {
	case "", "continue":
		return false
	default:
		return true
	}
}

func convergenceBudgetRequiresEscalation(run *runModel.Run) bool {
	if run == nil {
		return false
	}
	return convergenceBudgetEscalatedDecision(stringFromMap(mapFromValue(run.ExecutionContext[convergenceBudgetKey]), "decision"))
}

func convergenceBudgetEscalationContext(run *runModel.Run) map[string]any {
	if run == nil {
		return nil
	}
	budget := mapFromValue(run.ExecutionContext[convergenceBudgetKey])
	if !convergenceBudgetEscalatedDecision(stringFromMap(budget, "decision")) {
		return nil
	}
	return compactLLMContextMap(budget, 1)
}

package orchestrator

import (
	"fmt"
	"strings"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func validateLLMFollowUpGraphStateChangingContract(run *runModel.Run, step *runModel.Step, output map[string]any) error {
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok {
		return nil
	}
	stage := runtimeStageFromStepInput(step.Input)
	if boolValue(step.Input[stepInputKeyAssessmentOnly]) || boolValue(step.Input["providerRetryAssessmentOnly"]) {
		return nil
	}
	if stepRequiresConcreteFailureEvidenceAcquisition(step) {
		if graphIsConcreteFailureEvidenceAcquisitionGraph(step, graph) {
			return nil
		}
		return newRuntimeContractViolationError(contractViolationValidationEvidenceRequired, "concrete failure evidence acquisition must return bounded file/read, file/list, git/read, shell/command, or mcp/tool_call executionGraph without patch/apply or nested llm nodes")
	}
	if graphHasStateChangingPatchNode(graph) {
		return validateStateChangingExecutionGraph(graph)
	}
	if graphHasUngatedStateChangingEvidenceNode(graph) {
		return nil
	}
	if allowAcceptedStateChangingEvidenceGraph(step, graph) {
		return nil
	}
	if allowDeclaredToolStateChangingRepairGraph(step, graph) {
		return nil
	}
	if hasStateChangingLLMExecutionOutput(output) {
		return nil
	}
	if stage == runtimeStageVerification && graphIsConcreteValidationEvidenceGraph(graph) {
		return nil
	}
	if stage == runtimeStageVerification {
		return newRuntimeContractViolationError(contractViolationValidationEvidenceRequired, "validation follow-up llm step returned read-only executionGraph without concrete validation evidence")
	}
	if llmFollowUpGraphRequiresStateChangingPatch(step) {
		return newRuntimeContractViolationError(contractViolationRepairGraphWithoutPatchApply, "state-changing llm repair step returned executionGraph without patch/apply or declared auditable state-changing operation")
	}
	return nil
}

func graphIsConcreteFailureEvidenceAcquisitionGraph(step *runModel.Step, graph runtimemodel.BlockExecutionGraph) bool {
	if len(graph.Nodes) == 0 {
		return false
	}
	accepted := mapStringSet([]string{
		"file/list",
		"file/read",
		"git/read",
		"shell/command",
		"mcp/tool_call",
	})
	if step != nil && step.Input != nil {
		required := mapFromValue(mapFromValue(step.Input["repairActionPackage"])["requiredOutput"])
		if configured := mapStringSet(stringSliceFromValue(required["acceptedKinds"])); len(configured) > 0 {
			accepted = configured
		}
	}
	stateChangingForbidden := false
	if step != nil && step.Input != nil {
		required := mapFromValue(mapFromValue(step.Input["repairActionPackage"])["requiredOutput"])
		stateChangingForbidden = boolValue(required["stateChangingForbidden"])
	}
	hasEvidenceProducer := false
	for _, node := range graph.Nodes {
		kind := canonicalRuntimeGraphNodeKind(node.Kind)
		if kind == "" || kind == "patch/apply" || kind == "patch/preview" || kind == llmWorkspaceExecutionStepKind {
			return false
		}
		if !accepted[kind] {
			return false
		}
		if stateChangingForbidden && graphNodeDeclaresStateChangingIntent(node) {
			return false
		}
		hasEvidenceProducer = true
	}
	return hasEvidenceProducer
}

func graphHasUngatedStateChangingEvidenceNode(graph runtimemodel.BlockExecutionGraph) bool {
	nodes := graphNodesByID(graph)
	for _, node := range graph.Nodes {
		if !stepKindCanProduceStateChangingEvidence(node.Kind) {
			continue
		}
		if graphNodeHasAncestorKind(node, nodes, llmWorkspaceExecutionStepKind, map[string]bool{}) {
			continue
		}
		return true
	}
	return false
}

func llmFollowUpGraphRequiresStateChangingPatch(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	return boolValue(step.Input["graphFollowUp"]) ||
		boolValue(step.Input["completionFollowUp"]) ||
		boolValue(step.Input["continuationFollowUp"]) ||
		boolValue(step.Input[stepInputKeyGraphLimitFollowUp])
}

func runExecutionContractRequiresStateChange(run *runModel.Run) bool {
	if run == nil {
		return false
	}
	executionContract := mapFromValue(run.ExecutionContext["executionContract"])
	return strings.TrimSpace(stringFromMap(executionContract, "stateChangePolicy")) == "must_produce_state_change"
}

func graphHasStateChangingPatchNode(graph runtimemodel.BlockExecutionGraph) bool {
	for _, node := range graph.Nodes {
		switch strings.TrimSpace(node.Kind) {
		case "patch/apply":
			if _, err := runtimePatchRequestFromMap(node.Input); err == nil {
				return true
			}
		}
	}
	return false
}

func validateStateChangingExecutionGraph(graph runtimemodel.BlockExecutionGraph) error {
	if !graphHasStateChangingPatchNode(graph) {
		return newRuntimeContractViolationError(contractViolationRepairGraphWithoutPatchApply, "state-changing llm repair step returned executionGraph without patch/apply or declared auditable state-changing operation; read-only or inspection-only graphs cannot satisfy the state-changing contract")
	}
	if !graphHasUngatedStateChangingPatchNode(graph) {
		return newRuntimeContractViolationError(contractViolationGraphDependsOnLLMBeforePatch, "state-changing executionGraph patch/apply depends on llm/workspace_execution before patch")
	}
	if graphHasPatchApplyDependingOnSameGraphRead(graph) {
		return newRuntimeContractViolationError(contractViolationGraphDependsOnLLMBeforePatch, "state-changing executionGraph patch/apply must not depend on same-graph file/read")
	}
	return nil
}

func graphIsConcreteStateChangingContinuation(graph runtimemodel.BlockExecutionGraph) bool {
	if graphHasNodeKind(graph, llmWorkspaceExecutionStepKind) {
		return false
	}
	return validateStateChangingExecutionGraph(graph) == nil
}

func graphHasNodeKind(graph runtimemodel.BlockExecutionGraph, kind string) bool {
	kind = canonicalRuntimeGraphNodeKind(kind)
	if kind == "" {
		return false
	}
	for _, node := range graph.Nodes {
		if canonicalRuntimeGraphNodeKind(node.Kind) == kind {
			return true
		}
	}
	return false
}

func allowPatchRepairValidationEvidenceGraph(step *runModel.Step, graph runtimemodel.BlockExecutionGraph) bool {
	if step == nil || step.Input == nil || runtimeStageFromStepInput(step.Input) != runtimeStageRepair {
		return false
	}
	if !inputRequiresStateChangingOutput(step.Input) {
		return false
	}
	if len(mapSliceFromValue(step.Input["patchRepairContext"])) == 0 || !stepRepairsPatchCapabilityFailure(step) {
		return false
	}
	return graphIsConcreteValidationEvidenceGraph(graph)
}

func allowDeclaredToolStateChangingRepairGraph(step *runModel.Step, graph runtimemodel.BlockExecutionGraph) bool {
	if step == nil || step.Input == nil || runtimeStageFromStepInput(step.Input) != runtimeStageRepair {
		return false
	}
	if !inputRequiresStateChangingOutput(step.Input) {
		return false
	}
	required := mapFromValue(mapFromValue(step.Input["repairActionPackage"])["requiredOutput"])
	accepted := stringSliceFromValue(required["acceptedKinds"])
	if !stringSliceContains(accepted, "shell/command") && !stringSliceContains(accepted, "mcp/tool_call") {
		return false
	}
	return graphHasDeclaredUngatedStateChangingToolNode(graph) && graphHasBoundedVerificationDependingOnDeclaredStateChangingTool(graph)
}

func allowAcceptedStateChangingEvidenceGraph(step *runModel.Step, graph runtimemodel.BlockExecutionGraph) bool {
	if step == nil || step.Input == nil || runtimeStageFromStepInput(step.Input) != runtimeStageRepair {
		return false
	}
	if !inputRequiresStateChangingOutput(step.Input) {
		return false
	}
	required := mapFromValue(mapFromValue(step.Input["repairActionPackage"])["requiredOutput"])
	if !boolValue(required["mustIncludeStateChangingNode"]) {
		return false
	}
	if boolValue(required["mustIncludePrimaryStateChangingNode"]) {
		return false
	}
	accepted := mapStringSet(stringSliceFromValue(required["acceptedKinds"]))
	if len(accepted) == 0 {
		return false
	}
	acceptedOperations := mapStringSet(lowerStringSlice(stringSliceFromValue(required["acceptedOperations"])))
	nodes := graphNodesByID(graph)
	for _, node := range graph.Nodes {
		kind := canonicalRuntimeGraphNodeKind(node.Kind)
		if !accepted[kind] || !stepKindCanProduceStateChangingEvidence(kind) {
			continue
		}
		if len(acceptedOperations) > 0 {
			operation := strings.ToLower(strings.TrimSpace(stringFromMap(node.Input, "operation")))
			if !acceptedOperations[operation] {
				continue
			}
		}
		if graphNodeHasAncestorKind(node, nodes, llmWorkspaceExecutionStepKind, map[string]bool{}) {
			continue
		}
		return true
	}
	return false
}

func stepRepairsPatchCapabilityFailure(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	failedKind := strings.TrimSpace(stringFromMap(step.Input, "failedStepKind"))
	if failedKind == "patch/preview" || failedKind == "patch/apply" {
		return true
	}
	failedOutput := mapFromValue(step.Input["failedStepOutput"])
	stage := strings.TrimSpace(stringFromMap(failedOutput, "stage"))
	if stage == "patch_preview" || strings.HasPrefix(stage, "patch_apply") {
		return true
	}
	signature := strings.ToLower(strings.TrimSpace(stringFromMap(failedOutput, failureOutputKeySignature)))
	return strings.Contains(signature, "kind=patch/preview") ||
		strings.Contains(signature, "kind=patch/apply") ||
		strings.Contains(signature, "class="+failureClassPatchApply)
}

func graphIsConcreteValidationEvidenceGraph(graph runtimemodel.BlockExecutionGraph) bool {
	if len(graph.Nodes) == 0 {
		return false
	}
	hasEvidenceProducer := false
	for _, node := range graph.Nodes {
		kind := canonicalRuntimeGraphNodeKind(node.Kind)
		if graphNodeIsReadOnlyEvidenceSupportKind(kind) {
			continue
		}
		switch kind {
		case "shell/command", "mcp/tool_call":
			hasEvidenceProducer = true
		case "patch/preview", "patch/apply", llmWorkspaceExecutionStepKind:
			return false
		default:
			return false
		}
	}
	return hasEvidenceProducer
}

func graphNodeIsReadOnlyEvidenceSupportKind(kind string) bool {
	kind = canonicalRuntimeGraphNodeKind(kind)
	switch kind {
	case "file/read", "file/list", "mcp/resource_read", "mcp/prompt_get":
		return true
	default:
		return strings.HasSuffix(kind, "/read") || strings.HasSuffix(kind, "/list")
	}
}

func graphHasDeclaredUngatedStateChangingToolNode(graph runtimemodel.BlockExecutionGraph) bool {
	nodes := graphNodesByID(graph)
	for _, node := range graph.Nodes {
		if !graphNodeIsStateChangingTool(node) || !graphNodeDeclaresStateChangingIntent(node) {
			continue
		}
		if !graphNodeHasAncestorKind(node, nodes, llmWorkspaceExecutionStepKind, map[string]bool{}) {
			return true
		}
	}
	return false
}

func graphHasBoundedVerificationDependingOnDeclaredStateChangingTool(graph runtimemodel.BlockExecutionGraph) bool {
	nodes := graphNodesByID(graph)
	for _, node := range graph.Nodes {
		if !graphNodeIsVerificationTool(node) || !shellCommandInputHasExplicitVerificationScope(node.Input) {
			continue
		}
		if graphNodeHasDeclaredStateChangingToolAncestor(node, nodes, map[string]bool{}) {
			return true
		}
	}
	return false
}

func graphNodeHasDeclaredStateChangingToolAncestor(node runtimemodel.BlockExecutionNode, nodes map[string]runtimemodel.BlockExecutionNode, seen map[string]bool) bool {
	for _, depID := range node.DependsOn {
		depID = strings.TrimSpace(depID)
		if depID == "" || seen[depID] {
			continue
		}
		seen[depID] = true
		parent, ok := nodes[depID]
		if !ok {
			continue
		}
		if graphNodeIsStateChangingTool(parent) && graphNodeDeclaresStateChangingIntent(parent) {
			return true
		}
		if graphNodeHasDeclaredStateChangingToolAncestor(parent, nodes, seen) {
			return true
		}
	}
	return false
}

func graphNodeIsStateChangingTool(node runtimemodel.BlockExecutionNode) bool {
	switch strings.TrimSpace(node.Kind) {
	case "shell/command", "mcp/tool_call":
		return true
	default:
		return false
	}
}

func stepKindCanProduceStateChangingEvidence(kind string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		return false
	}
	if kind == "patch/apply" || kind == artifactApplyStepKind {
		return true
	}
	return strings.HasSuffix(kind, "/write")
}

func graphNodeIsVerificationTool(node runtimemodel.BlockExecutionNode) bool {
	switch strings.TrimSpace(node.Kind) {
	case "shell/command", "mcp/tool_call":
		return true
	default:
		return false
	}
}

func graphNodeDeclaresStateChangingIntent(node runtimemodel.BlockExecutionNode) bool {
	if len(node.Input) == 0 {
		return false
	}
	if boolValue(node.Input["stateChanging"]) || boolValue(node.Input["stateChange"]) {
		return true
	}
	for _, key := range []string{"stateChangingReason", "stateChangeReason"} {
		if strings.TrimSpace(stringFromAny(node.Input[key])) != "" {
			return true
		}
	}
	return false
}

func graphNodesByID(graph runtimemodel.BlockExecutionGraph) map[string]runtimemodel.BlockExecutionNode {
	nodes := map[string]runtimemodel.BlockExecutionNode{}
	for _, node := range graph.Nodes {
		if id := strings.TrimSpace(node.ID); id != "" {
			nodes[id] = node
		}
	}
	return nodes
}

func graphHasUngatedStateChangingPatchNode(graph runtimemodel.BlockExecutionGraph) bool {
	nodes := map[string]runtimemodel.BlockExecutionNode{}
	for _, node := range graph.Nodes {
		id := strings.TrimSpace(node.ID)
		if id != "" {
			nodes[id] = node
		}
	}
	for _, node := range graph.Nodes {
		if canonicalRuntimeGraphNodeKind(node.Kind) != "patch/apply" {
			continue
		}
		if _, err := runtimePatchRequestFromMap(node.Input); err != nil {
			continue
		}
		if !graphNodeHasAncestorKind(node, nodes, llmWorkspaceExecutionStepKind, map[string]bool{}) {
			return true
		}
	}
	return false
}

func graphNodeHasAncestorKind(node runtimemodel.BlockExecutionNode, nodes map[string]runtimemodel.BlockExecutionNode, kind string, seen map[string]bool) bool {
	kind = canonicalRuntimeGraphNodeKind(kind)
	for _, dep := range node.DependsOn {
		dep = strings.TrimSpace(dep)
		if dep == "" || seen[dep] {
			continue
		}
		seen[dep] = true
		parent, ok := nodes[dep]
		if !ok {
			continue
		}
		if canonicalRuntimeGraphNodeKind(parent.Kind) == kind {
			return true
		}
		if graphNodeHasAncestorKind(parent, nodes, kind, seen) {
			return true
		}
	}
	return false
}

func graphHasPatchApplyDependingOnSameGraphRead(graph runtimemodel.BlockExecutionGraph) bool {
	nodes := map[string]runtimemodel.BlockExecutionNode{}
	for _, node := range graph.Nodes {
		id := strings.TrimSpace(node.ID)
		if id != "" {
			nodes[id] = node
		}
	}
	for _, node := range graph.Nodes {
		if strings.TrimSpace(node.Kind) != "patch/apply" {
			continue
		}
		if _, err := runtimePatchRequestFromMap(node.Input); err != nil {
			continue
		}
		if graphNodeHasAncestorKind(node, nodes, "file/read", map[string]bool{}) {
			return true
		}
	}
	return false
}

func llmExecutionGraphNodeReturnedOnlyGraph(run *runModel.Run, step *runModel.Step, output map[string]any) bool {
	if run == nil || step == nil || output == nil {
		return false
	}
	if step.Kind != initialStepKind(run) || graphNodeID(step) == "" {
		return false
	}
	if runtimeStageFromStepInput(step.Input) != runtimeStageExecution {
		return false
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	if !ok {
		return false
	}
	if graphIsConcreteStateChangingContinuation(graph) {
		return false
	}
	return !hasConcreteLLMExecutionOutput(output)
}

func hasConcreteLLMExecutionOutput(output map[string]any) bool {
	if output == nil {
		return false
	}
	if nextStepID, _ := nextStepSpec(output); nextStepID != "" {
		return true
	}
	if doDAssessmentFromOutput(output) != nil {
		return true
	}
	if hasStateChangingLLMExecutionOutput(output) {
		return true
	}
	if spec := decisionBlockSpec(output); spec.Reason != "" {
		return true
	}
	if spec := approvalBlockSpec(output); spec.Reason != "" {
		return true
	}
	return false
}

func graphNodeRequiresStateChangingOutput(step *runModel.Step) bool {
	return step != nil && boolValue(step.Input["stateChangingOutputRequired"])
}

func hasStateChangingLLMExecutionOutput(output map[string]any) bool {
	if output == nil {
		return false
	}
	if outputItemCount(output[stepOutputKeyPatches]) > 0 ||
		outputItemCount(output["patchRequests"]) > 0 ||
		outputItemCount(mapFromValue(output[stepOutputKeyLLMDecision])["patchRequests"]) > 0 {
		return true
	}
	_, nextKind := nextStepSpec(output)
	switch strings.TrimSpace(nextKind) {
	case "patch/apply":
		return true
	default:
		return false
	}
}

func validatePatchStepInputBeforeRuntime(kind string, input map[string]any) error {
	switch strings.TrimSpace(kind) {
	case "patch/preview", "patch/apply":
		request, err := runtimePatchRequestFromMap(input)
		if err != nil {
			return err
		}
		if request.PatchType == "unified_diff" {
			if _, err := parseSingleFileUnifiedDiff(request.Diff); err != nil {
				return wrapPatchApplyParseError(request.TargetRef, request.PatchType, err)
			}
		}
		return nil
	default:
		return nil
	}
}

func assessmentOnlyContractCorrectionStepID(runID common.RunID, step *runModel.Step) common.StepID {
	stepID := runtimeChildID(common.StepID(runtimeReadableStepLabel(runID, step.ID)), "assessment-contract-correction")
	if stepID == "" {
		stepID = "assessment-contract-correction"
	}
	return common.StepID(fmt.Sprintf("%s/assessment-correction-%s", runID, stepID))
}

func assessmentOnlyContractCorrectionStepInput(step *runModel.Step, output map[string]any, violation error) map[string]any {
	input := cloneStepPayload(step.Input)
	if input == nil {
		input = map[string]any{}
	}
	input[stepInputKeyAssessmentOnly] = true
	input["assessmentOnlyContractCorrection"] = true
	setRuntimeStage(input, runtimeStageVerification)
	input["assessmentOnlyContractViolationStepId"] = string(step.ID)
	input["assessmentOnlyContractViolationReason"] = errorString(violation)
	input["allowedAssessmentOnlyCompletionStates"] = []string{"completed", "failed", "blocked_on_decision", "blocked_on_approval"}
	stripAssessmentOnlyRecoveryMarkers(input)
	stripAssessmentOnlyCorrectionHistory(input)
	sanitizeAssessmentOnlyCorrectionFields(input)
	for _, key := range []string{"previousStepOutput", "failedStepOutput"} {
		if nested := mapFromValue(input[key]); len(nested) > 0 {
			sanitizeAssessmentOnlyCorrectionFields(nested)
			input[key] = nested
		}
	}
	if sanitized := cloneStepPayload(output); len(sanitized) > 0 {
		sanitizeAssessmentOnlyCorrectionFields(sanitized)
		sanitized["assessmentOnlyContractViolation"] = true
		input["previousInvalidAssessmentOutput"] = compactStepOutputForLLMContext(sanitized)
	}
	contract := BuildConvergenceContract(nil, step, step, map[string]any{"input": input}, nil)
	input = ApplyConvergenceContract(input, contract)
	return input
}

func stripAssessmentOnlyRecoveryMarkers(input map[string]any) {
	if input == nil {
		return
	}
	for _, key := range []string{
		stepInputKeyProviderRetryFollowUp,
		stepInputKeyProviderRetryAttempt,
		stepInputKeyProviderRetryRootStep,
		"providerRetryAssessmentOnly",
		"providerRetryMode",
		stepInputKeyRuntimeRepairFollowUp,
		stepInputKeyRuntimeRepairAttempt,
		stepInputKeyRepairOfStepID,
		stepInputKeyRepairOfRootStepID,
		stepInputKeyRepairOfFailureClass,
		stepInputKeyRepairOfFailureSig,
		stepInputKeyRecoveryAction,
		stepInputKeyRecoveryRootStepID,
		stepInputKeyRecoveryLineageType,
		stepInputKeyRecoveryBudgetScope,
		"stateChangingOutputRequired",
		"repairObjective",
		"repairGuidance",
		"repairActionPackage",
		"patchRepairContext",
		"patchRepairSuggestions",
		"failureTargetRepairContext",
		"readableResourceRepairContext",
		"structuredFailureSignals",
		"targetRepairAttempts",
		"failureTargetScope",
	} {
		delete(input, key)
	}
}

func stripAssessmentOnlyCorrectionHistory(input map[string]any) {
	if input == nil {
		return
	}
	for _, key := range []string{
		"governanceHistory",
		"replanContinuation",
		"runtimeRepairContinuation",
		"providerRetryContinuation",
		"recoveryContinuation",
		"failedStepInput",
		"failedStepOutput",
		"previousStepOutput",
		"failedExecutionGraph",
		stepInputKeyGraphEvidence,
	} {
		delete(input, key)
	}
}

func sanitizeAssessmentOnlyCorrectionFields(payload map[string]any) {
	sanitizeAssessmentOnlyExecutableFields(payload)
	stripAssessmentOnlyCorrectionDecisionAndRecoveryFields(payload)
}

func stripAssessmentOnlyCorrectionDecisionAndRecoveryFields(payload map[string]any) {
	if payload == nil {
		return
	}
	for _, key := range []string{
		stepOutputKeyLLMDecision,
		"convertedDecision",
		"requiredDecision",
		"requiredApproval",
		stepOutputKeyBlockDecisionReason,
		stepOutputKeyBlockDecisionTicketID,
		stepOutputKeyBlockDecisionKind,
		stepOutputKeyBlockDecisionQuestion,
		stepOutputKeyBlockDecisionContext,
		"governanceHistory",
		"replanContinuation",
		"runtimeRepairContinuation",
		"providerRetryContinuation",
		"recoveryContinuation",
		"resolutionPayload",
		"ticketContext",
		"recoveryPolicy",
		"recoveryAction",
		"repairActionPackage",
		"patchRepairContext",
		"patchRepairSuggestions",
		"failureTargetRepairContext",
		"readableResourceRepairContext",
		"structuredFailureSignals",
	} {
		delete(payload, key)
	}
	for key, value := range payload {
		switch typed := value.(type) {
		case map[string]any:
			stripAssessmentOnlyCorrectionDecisionAndRecoveryFields(typed)
			payload[key] = typed
		case []any:
			for idx, item := range typed {
				if nested, ok := item.(map[string]any); ok {
					stripAssessmentOnlyCorrectionDecisionAndRecoveryFields(nested)
					typed[idx] = nested
				}
			}
			payload[key] = typed
		case []map[string]any:
			for idx := range typed {
				stripAssessmentOnlyCorrectionDecisionAndRecoveryFields(typed[idx])
			}
			payload[key] = typed
		}
	}
}

func sanitizeAssessmentOnlyExecutableFields(payload map[string]any) {
	if payload == nil {
		return
	}
	for _, key := range []string{
		stepOutputKeyExecutionGraph,
		"shellCommands",
		"patchRequests",
		"mcpToolCalls",
		"memoryWrites",
		"nextAction",
		"nextStep",
		"nextKind",
		"nextInput",
		"patches",
		stepOutputKeyMemoryRefs,
	} {
		delete(payload, key)
	}
	for key, value := range payload {
		switch typed := value.(type) {
		case map[string]any:
			sanitizeAssessmentOnlyExecutableFields(typed)
			payload[key] = typed
		case []any:
			for idx, item := range typed {
				if nested, ok := item.(map[string]any); ok {
					sanitizeAssessmentOnlyExecutableFields(nested)
					typed[idx] = nested
				}
			}
			payload[key] = typed
		case []map[string]any:
			for idx := range typed {
				sanitizeAssessmentOnlyExecutableFields(typed[idx])
			}
			payload[key] = typed
		}
	}
}

package orchestrator

import (
	"context"
	"encoding/json"
	"strings"

	orchestratormodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

const (
	outputModeExecutionGraph      = "executionGraph"
	outputModeIncrementalArtifact = "incrementalArtifact"
)

type outputModeSelection struct {
	Mode              string
	Priority          string
	Reason            string
	Continuation      map[string]any
	Policy            LLMOutputRuntimePolicy
	StateChangeNeeded bool
}

func (o *RuntimeOrchestrator) applyLLMOutputModeSelection(ctx context.Context, run *runModel.Run, input map[string]any, failedStep *runModel.Step, attempt int) {
	if len(input) == 0 {
		return
	}
	inheritOutputModeSignalsFromRunExecutionContext(input, run)
	selection := o.selectOutputMode(ctx, run, input, failedStep, attempt)
	if !selection.StateChangeNeeded {
		return
	}
	if artifactContinuationContextTerminal(mapFromValue(input["artifactContinuationContext"])) {
		delete(input, "artifactContinuationContext")
	}
	input["outputMode"] = selection.Mode
	input["outputModeSelector"] = map[string]any{
		"priority": selection.Priority,
		"reason":   selection.Reason,
		"mode":     selection.Mode,
	}
	if len(selection.Continuation) > 0 {
		input["artifactContinuationContext"] = selection.Continuation
	}
	applyOutputModeRequiredOutput(input, selection)
}

func inheritOutputModeSignalsFromRunExecutionContext(input map[string]any, run *runModel.Run) {
	if len(input) == 0 || run == nil || len(run.ExecutionContext) == 0 {
		return
	}
	for _, key := range []string{"requestedOutputMode"} {
		if _, exists := input[key]; !exists && run.ExecutionContext[key] != nil {
			input[key] = cloneStepInputValue(run.ExecutionContext[key])
		}
	}
	sourcePkg := mapFromValue(run.ExecutionContext["repairActionPackage"])
	sourceRequired := mapFromValue(sourcePkg["requiredOutput"])
	if len(sourceRequired) == 0 {
		return
	}
	targetPkg := mapFromValue(input["repairActionPackage"])
	if len(targetPkg) == 0 {
		targetPkg = map[string]any{}
	}
	targetRequired := mapFromValue(targetPkg["requiredOutput"])
	if len(targetRequired) == 0 {
		targetRequired = map[string]any{}
	}
	changed := false
	for _, key := range outputModeSignalKeys() {
		if _, exists := targetRequired[key]; exists {
			continue
		}
		if value, ok := sourceRequired[key]; ok && value != nil {
			targetRequired[key] = cloneStepInputValue(value)
			changed = true
		}
	}
	if changed {
		targetPkg["requiredOutput"] = targetRequired
		input["repairActionPackage"] = targetPkg
	}
}

func (o *RuntimeOrchestrator) selectOutputMode(ctx context.Context, run *runModel.Run, input map[string]any, failedStep *runModel.Step, attempt int) outputModeSelection {
	policy := NormalizeLLMOutputRuntimePolicy(o.policy.LLMOutput)
	selection := outputModeSelection{
		Mode:              outputModeExecutionGraph,
		Priority:          "P6",
		Reason:            "default_execution_graph",
		Policy:            policy,
		StateChangeNeeded: inputRequiresStateChangingOutput(input),
	}
	if !selection.StateChangeNeeded {
		selection.Priority = "P0"
		selection.Reason = "state_changing_output_not_allowed"
		return selection
	}
	if continuation := o.openArtifactContinuationContext(ctx, run); len(continuation) > 0 {
		selection.Mode = outputModeIncrementalArtifact
		selection.Priority = "P1"
		selection.Reason = "open_artifact_continuation"
		selection.Continuation = continuation
		return selection
	}
	if continuation := mapFromValue(input["artifactContinuationContext"]); len(continuation) > 0 {
		if artifactContinuationContextTerminal(continuation) {
			selection.Priority = "P2"
			selection.Reason = "terminal_artifact_continuation_returns_to_graph"
			return selection
		}
		selection.Mode = outputModeIncrementalArtifact
		selection.Priority = "P2"
		selection.Reason = "artifact_continuation_recovery"
		selection.Continuation = continuation
		return selection
	}
	if schemaInvalidLargePreview(failedStep, policy, attempt) {
		selection.Mode = outputModeIncrementalArtifact
		selection.Priority = "P3"
		selection.Reason = "large_schema_invalid_preview"
		return selection
	}
	if estimatedOutputExceedsGraphBudget(input, policy) {
		selection.Mode = outputModeIncrementalArtifact
		selection.Priority = "P4"
		selection.Reason = "estimated_graph_output_exceeds_budget"
		return selection
	}
	if strings.TrimSpace(stringFromMap(input, "requestedOutputMode")) == outputModeIncrementalArtifact &&
		requestedIncrementalArtifactEligible(input, policy) {
		selection.Mode = outputModeIncrementalArtifact
		selection.Priority = "P5"
		selection.Reason = "requested_incremental_artifact_with_budget_signal"
		return selection
	}
	return selection
}

func outputModeSignalKeys() []string {
	return []string{
		"estimatedPatchContentBytes",
		"estimatedGraphBytes",
		"expectedStateChangingArtifactCount",
		"maxArtifactChunkBytes",
		"previousChunkTailBytes",
	}
}

func (o *RuntimeOrchestrator) openArtifactContinuationContext(ctx context.Context, run *runModel.Run) map[string]any {
	if o == nil || o.evidence.ArtifactContinuations == nil || run == nil || run.ID == "" {
		return nil
	}
	items, err := o.evidence.ArtifactContinuations.ListOpenByRun(ctx, run.ID)
	if err != nil || len(items) == 0 || items[0] == nil {
		return nil
	}
	return artifactContinuationContext(items[0])
}

func artifactContinuationContext(item *orchestratormodel.ArtifactContinuation) map[string]any {
	if item == nil {
		return nil
	}
	ret := map[string]any{
		"id":               string(item.ID),
		"runId":            string(item.RunID),
		"state":            string(item.State),
		"artifactId":       item.ArtifactID,
		"targetRef":        item.TargetRef,
		"expectedSequence": item.ExpectedSequence,
		"finalReceived":    item.FinalReceived,
		"bytesWritten":     item.BytesWritten,
	}
	if item.RootStepID != "" {
		ret["rootStepId"] = string(item.RootStepID)
	}
	if item.Operation != "" {
		ret["operation"] = item.Operation
	}
	if len(item.CoverageTargets) > 0 {
		ret["coverageTargets"] = append([]string(nil), item.CoverageTargets...)
	}
	if len(item.ReceivedSequences) > 0 {
		ret["receivedSequences"] = append([]int(nil), item.ReceivedSequences...)
	}
	if len(item.ReceivedChunkHashes) > 0 {
		ret["receivedChunkHashes"] = cloneArtifactContinuationChunkHashes(item.ReceivedChunkHashes)
	}
	if item.RollingHash != "" {
		ret["rollingHash"] = item.RollingHash
	}
	if item.LastChunkHash != "" {
		ret["lastChunkHash"] = item.LastChunkHash
	}
	if item.PreviousChunkTail != "" {
		ret["previousChunkTailBytes"] = len([]byte(item.PreviousChunkTail))
		ret["previousChunkTailHash"] = sha256HexBytes([]byte(item.PreviousChunkTail))
	}
	return ret
}

func cloneArtifactContinuationChunkHashes(input map[int]string) map[int]string {
	if len(input) == 0 {
		return nil
	}
	ret := make(map[int]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func artifactContinuationContextTerminal(continuation map[string]any) bool {
	if len(continuation) == 0 {
		return false
	}
	switch common.ArtifactContinuationState(strings.TrimSpace(stringFromMap(continuation, "state"))) {
	case common.ArtifactContinuationStateApplied, common.ArtifactContinuationStateFailed:
		return true
	default:
		return false
	}
}

func schemaInvalidLargePreview(step *runModel.Step, policy LLMOutputRuntimePolicy, attempt int) bool {
	if step == nil || !stepHasSchemaInvalidOutput(step) {
		return false
	}
	threshold := policy.SchemaInvalidLargePreviewThreshold
	if threshold <= 0 {
		threshold = defaultSchemaInvalidPreviewBytes
	}
	for _, key := range []string{"rawContentPreview", "repairRawContentPreview", "rawPreview", "responsePreview"} {
		if len([]byte(stringFromMap(step.Output, key))) >= threshold {
			return true
		}
	}
	if attempt > 1 && outputContainsContentLikePreview(step.Output, threshold/2) {
		return true
	}
	return false
}

func stepHasSchemaInvalidOutput(step *runModel.Step) bool {
	if step == nil {
		return false
	}
	stage := strings.ToLower(strings.TrimSpace(stringFromMap(step.Output, "stage")))
	if stage == "llm_schema_invalid" || strings.Contains(stage, "schema") {
		return true
	}
	text := strings.ToLower(strings.Join([]string{
		stringFromMap(step.Output, "error"),
		stringFromMap(step.Output, "reason"),
		stringFromMap(step.Output, failureOutputKeySignature),
	}, "\n"))
	return strings.Contains(text, "invalid llm decision json") || strings.Contains(text, "schema")
}

func outputContainsContentLikePreview(output map[string]any, threshold int) bool {
	if threshold <= 0 {
		threshold = defaultSchemaInvalidPreviewBytes / 2
	}
	for _, key := range []string{"rawContentPreview", "repairRawContentPreview"} {
		preview := stringFromMap(output, key)
		if len([]byte(preview)) < threshold {
			continue
		}
		lower := strings.ToLower(preview)
		if strings.Contains(lower, `"content"`) ||
			strings.Contains(lower, `"patch"`) ||
			strings.Contains(lower, `"diff"`) ||
			strings.Contains(lower, `"executiongraph"`) {
			return true
		}
	}
	return false
}

func estimatedOutputExceedsGraphBudget(input map[string]any, policy LLMOutputRuntimePolicy) bool {
	if maxIntFromOutputModeSignal(input, "estimatedPatchContentBytes") > policy.MaxPatchContentBytes {
		return true
	}
	if maxIntFromOutputModeSignal(input, "estimatedGraphBytes") > policy.MaxGraphBytes {
		return true
	}
	if maxIntFromOutputModeSignal(input, "expectedStateChangingArtifactCount") > policy.MaxGraphFileWrites {
		return true
	}
	if graph := mapFromValue(input[stepOutputKeyExecutionGraph]); len(graph) > 0 {
		if data, err := json.Marshal(graph); err == nil && len(data) > policy.MaxGraphBytes {
			return true
		}
	}
	return false
}

func requestedIncrementalArtifactEligible(input map[string]any, policy LLMOutputRuntimePolicy) bool {
	return maxIntFromOutputModeSignal(input, "estimatedPatchContentBytes") > policy.MaxPatchContentBytes ||
		maxIntFromOutputModeSignal(input, "estimatedGraphBytes") > policy.MaxGraphBytes ||
		maxIntFromOutputModeSignal(input, "expectedStateChangingArtifactCount") > policy.MaxGraphFileWrites
}

func maxIntFromOutputModeSignal(input map[string]any, key string) int {
	maxValue := intFromAny(input[key])
	visit := func(source map[string]any) {
		if value := intFromAny(source[key]); value > maxValue {
			maxValue = value
		}
	}
	pkg := mapFromValue(input["repairActionPackage"])
	required := mapFromValue(pkg["requiredOutput"])
	visit(pkg)
	visit(required)
	visit(mapFromValue(input["executionContract"]))
	visit(mapFromValue(mapFromValue(input["executionContext"])["executionContract"]))
	visit(mapFromValue(input["replanContinuation"]))
	visit(mapFromValue(mapFromValue(input["replanContinuation"])["repairActionPackage"]))
	return maxValue
}

func applyOutputModeRequiredOutput(input map[string]any, selection outputModeSelection) {
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	required := mapFromValue(pkg["requiredOutput"])
	if len(required) == 0 {
		required = stateChangingRepairRequiredOutput()
	}
	required["outputMode"] = selection.Mode
	switch selection.Mode {
	case outputModeIncrementalArtifact:
		if continuation := mapFromValue(input["artifactContinuationContext"]); len(continuation) > 0 && !artifactContinuationContextTerminal(continuation) {
			applyArtifactChunkRequiredOutput(input, required, selection, continuation)
			return
		}
		required["mustReturnArtifactPlan"] = true
		required["stateChangingEvidenceDeferredUntilApply"] = true
		required["maxArtifactChunkBytes"] = selection.Policy.MaxArtifactChunkBytes
		required["previousChunkTailBytes"] = selection.Policy.PreviousChunkTailBytes
		required["forbiddenTopLevelOutputs"] = []string{"executionGraph", "dodAssessment", "artifactChunk", "memoryWrites"}
		required["artifactPlanSchema"] = artifactPlanRequiredOutputSchema()
		delete(required, "mustIncludeStateChangingNode")
	default:
		required["mustIncludeStateChangingNode"] = true
		if len(stringSliceFromValue(required["acceptedKinds"])) == 0 {
			required["acceptedKinds"] = stateChangingRepairAcceptedKinds()
		}
	}
	pkg["requiredOutput"] = required
	input["repairActionPackage"] = pkg
	if selection.Mode == outputModeIncrementalArtifact {
		input["artifactOutputContract"] = map[string]any{
			"mode":                       outputModeIncrementalArtifact,
			"phase":                      "artifact_plan",
			"mustReturnArtifactPlan":     true,
			"maxArtifactChunkBytes":      selection.Policy.MaxArtifactChunkBytes,
			"previousChunkTailBytes":     selection.Policy.PreviousChunkTailBytes,
			"stateChangingEvidenceStage": "artifact_apply",
			"schema":                     artifactPlanRequiredOutputSchema(),
		}
	}
}

func applyArtifactChunkRequiredOutput(input map[string]any, required map[string]any, selection outputModeSelection, continuation map[string]any) {
	required["outputMode"] = outputModeIncrementalArtifact
	required["mustReturnArtifactChunk"] = true
	required["stateChangingEvidenceDeferredUntilApply"] = true
	required["maxArtifactChunkBytes"] = selection.Policy.MaxArtifactChunkBytes
	required["previousChunkTailBytes"] = selection.Policy.PreviousChunkTailBytes
	required["forbiddenTopLevelOutputs"] = []string{"executionGraph", "dodAssessment", "artifactPlan", "memoryWrites"}
	required["artifactChunkSchema"] = artifactChunkRequiredOutputSchema()
	delete(required, "mustIncludeStateChangingNode")
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	pkg["requiredOutput"] = required
	input["repairActionPackage"] = pkg
	input["artifactOutputContract"] = map[string]any{
		"mode":                   outputModeIncrementalArtifact,
		"phase":                  artifactOutputPhaseChunk,
		"artifactId":             stringFromMap(continuation, "artifactId"),
		"targetRef":              stringFromMap(continuation, "targetRef"),
		"expectedSequence":       intFromAny(continuation["expectedSequence"]),
		"maxArtifactChunkBytes":  selection.Policy.MaxArtifactChunkBytes,
		"previousChunkTailBytes": selection.Policy.PreviousChunkTailBytes,
		"schema":                 artifactChunkRequiredOutputSchema(),
	}
}

func artifactPlanRequiredOutputSchema() map[string]any {
	return map[string]any{
		"completionState": "continue",
		"requiredFields": []string{
			"artifactPlan.artifacts[].artifactId",
			"artifactPlan.artifacts[].targetRef",
			"artifactPlan.artifacts[].operation",
			"artifactPlan.artifacts[].coverageTargets[]",
			"artifactPlan.artifacts[].estimatedChunks",
			"artifactPlan.artifacts[].estimatedBytes",
		},
		"mutuallyExclusiveWith": []string{"executionGraph", "artifactChunk", "dodAssessment", "memoryWrites"},
	}
}

func artifactChunkRequiredOutputSchema() map[string]any {
	return map[string]any{
		"completionState": "continue",
		"requiredFields": []string{
			"artifactChunk.artifactId",
			"artifactChunk.targetRef",
			"artifactChunk.sequence",
			"artifactChunk.content",
			"artifactChunk.chunkHash",
			"artifactChunk.isFinal",
		},
		"mutuallyExclusiveWith": []string{"executionGraph", "artifactPlan", "dodAssessment", "memoryWrites"},
	}
}

func outputModeFromInput(input map[string]any) string {
	mode := strings.TrimSpace(stringFromMap(input, "outputMode"))
	if mode == "" {
		return outputModeExecutionGraph
	}
	return mode
}

func outputModeIsIncrementalArtifact(input map[string]any) bool {
	return outputModeFromInput(input) == outputModeIncrementalArtifact
}

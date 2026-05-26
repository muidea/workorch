package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (o *RuntimeOrchestrator) hasGraphContinuation(ctx context.Context, run *runModel.Run, step *runModel.Step) bool {
	return len(o.readyGraphStepSpecs(ctx, run, step)) > 0
}

func (o *RuntimeOrchestrator) nextGraphStepSpec(
	ctx context.Context,
	run *runModel.Run,
	last *runModel.Step,
) (common.StepID, string, map[string]any, bool) {
	specs := o.readyGraphStepSpecs(ctx, run, last)
	if len(specs) == 0 {
		return "", "", nil, false
	}
	return specs[0].ID, specs[0].Kind, specs[0].Input, true
}

func (o *RuntimeOrchestrator) readyGraphStepSpecs(
	ctx context.Context,
	run *runModel.Run,
	last *runModel.Step,
) []stepDispatchSpec {
	specs, _ := o.readyGraphStepSpecsWithSteps(ctx, run, last)
	return specs
}

func (o *RuntimeOrchestrator) readyGraphStepSpecsWithSteps(
	ctx context.Context,
	run *runModel.Run,
	last *runModel.Step,
) ([]stepDispatchSpec, []*runModel.Step) {
	if o == nil || run == nil || last == nil {
		return nil, nil
	}
	graph, ok := runtimeGraphSpecFromStep(last)
	if !ok || len(graph.Nodes) == 0 {
		return nil, nil
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil, nil
	}
	return o.readyGraphStepSpecsFromSteps(ctx, run, last, graph, steps), steps
}

func (o *RuntimeOrchestrator) readyGraphStepSpecsFromSteps(
	ctx context.Context,
	run *runModel.Run,
	last *runModel.Step,
	graph runtimemodel.BlockExecutionGraph,
	steps []*runModel.Step,
) []stepDispatchSpec {
	rootStepID := graphDispatchRootStepID(last)
	completed := completedGraphNodes(steps, rootStepID)
	started := startedGraphNodes(steps, rootStepID)
	branch := selectedGraphBranch(steps)
	graphSeq := graphScopeSeqFromSteps(steps, rootStepID)
	ret := make([]stepDispatchSpec, 0, len(graph.Nodes))
	for idx, node := range graph.Nodes {
		if node.ID == "" || completed[node.ID] || started[node.ID] {
			continue
		}
		if node.Branch != "" && node.Branch != branch {
			continue
		}
		if !graphNodeDependenciesSatisfied(node, completed) {
			continue
		}
		input := nextGraphStepInput(run, last, graph, node, completed, branch, steps, rootStepID)
		input[stepInputKeyGraphRootStepID] = string(rootStepID)
		input["graphSeq"] = graphSeq
		input["graphNodeSeq"] = idx + 1
		ret = append(ret, stepDispatchSpec{
			ID:    graphScopedStepID(run.ID, graphSeq, idx+1, node.ID),
			Kind:  node.Kind,
			Input: input,
		})
	}
	return ret
}

func (o *RuntimeOrchestrator) dispatchGraphStepBatch(ctx context.Context, run *runModel.Run, specs []stepDispatchSpec) error {
	return o.dispatchGraphStepBatchWithWorkspace(ctx, run, specs, nil)
}

func (o *RuntimeOrchestrator) dispatchGraphStepBatchWithWorkspace(ctx context.Context, run *runModel.Run, specs []stepDispatchSpec, workspace *workspaceModel.Workspace) error {
	if len(specs) == 0 {
		return common.ErrConflict
	}
	if len(specs) == 1 {
		return o.dispatchStepWithWorkspace(ctx, run, specs[0].ID, specs[0].Kind, specs[0].Input, workspace)
	}

	errCh := make(chan error, len(specs))
	var wg sync.WaitGroup
	for _, spec := range specs {
		spec := spec
		spec.Input = cloneStepPayload(spec.Input)
		if spec.Input == nil {
			spec.Input = map[string]any{}
		}
		spec.Input[stepInputKeyGraphBatch] = true
		runCopy := *run
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- o.dispatchStepWithWorkspace(ctx, &runCopy, spec.ID, spec.Kind, spec.Input, workspace)
		}()
	}
	wg.Wait()
	close(errCh)
	var firstErr error
	for err := range errCh {
		if err != nil && !errors.Is(err, common.ErrConflict) {
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	currentRun, err := o.runs.Get(ctx, run.ID)
	if err != nil {
		return err
	}
	if currentRun.State != common.RunStateRunning {
		return nil
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return err
	}
	if firstErr != nil {
		if failed := latestFailedRepairableGraphStep(steps); failed != nil {
			return o.dispatchRuntimeRepairFollowUp(ctx, currentRun, failed, firstErr)
		}
		return o.FailRun(ctx, currentRun.ID, firstErr.Error())
	}
	last := latestCompletedGraphBatchStep(steps, specs)
	if last == nil {
		return common.ErrConflict
	}
	if graph, ok := runtimeGraphSpecForContinuation(steps, last); ok {
		if nextSpecs := o.readyGraphStepSpecsFromSteps(ctx, currentRun, last, graph, steps); len(nextSpecs) > 0 {
			return o.dispatchGraphStepBatchWithWorkspace(ctx, currentRun, nextSpecs, workspace)
		}
	}
	if shouldDispatchGraphFollowUp(currentRun, last, last.Output) {
		return o.dispatchUniqueStepWithWorkspace(ctx, currentRun, graphFollowUpStepID(currentRun.ID, last), initialStepKind(currentRun), o.graphFollowUpStepInputFromSteps(currentRun, last, steps), workspace)
	}
	if shouldDispatchLLMCompletionFollowUp(currentRun, last, last.Output) {
		return o.dispatchUniqueStepWithWorkspace(ctx, currentRun, llmCompletionFollowUpStepID(currentRun.ID, last), initialStepKind(currentRun), llmCompletionFollowUpStepInputFromSteps(currentRun, last, steps), workspace)
	}
	if shouldDispatchNoActionableContinuationFollowUp(currentRun, last, last.Output) {
		return o.dispatchUniqueStepWithWorkspace(ctx, currentRun, noActionableContinuationFollowUpStepID(currentRun.ID, last), initialStepKind(currentRun), noActionableContinuationFollowUpStepInputFromSteps(currentRun, last, steps), workspace)
	}
	if completionState := llmDecisionCompletionStateFromOutput(last.Output); completionStateRequiresActionableContinuation(last.Output, completionState) {
		return o.handleGraphBatchCompletionGateFailure(
			ctx,
			currentRun,
			last,
			last.Output,
			"runtime completion gate failed",
			fmt.Errorf("%w: completionState %s has no actionable continuation", common.ErrInvalidArgument, completionState),
		)
	}
	last.Output = normalizeCompletionGateDoDAssessmentOutput(currentRun, last, last.Output)
	normalizedOutput, evidenceNormalizeErr := o.normalizeCompletionGateDoDAssessmentEvidenceRefs(ctx, currentRun, last.Output)
	if evidenceNormalizeErr != nil {
		return evidenceNormalizeErr
	}
	last.Output = normalizedOutput
	_ = o.steps.Update(ctx, last)
	assessment, summary, err := validatedCompletionDoDAssessment(currentRun, last.Output)
	if err != nil {
		return o.handleGraphBatchCompletionGateFailure(ctx, currentRun, last, last.Output, "runtime completion gate failed", err)
	}
	if err := o.validateDoDAssessmentEvidenceQuality(ctx, currentRun, assessment); err != nil {
		return o.handleGraphBatchCompletionGateFailure(ctx, currentRun, last, last.Output, "runtime completion gate failed", err)
	}
	if err := o.recordCompletionEvidenceFromValidatedDoDAssessment(ctx, currentRun, last, assessment, summary, workspace); err != nil {
		return err
	}
	return o.validateAndCompleteRunWithWorkspace(ctx, currentRun, last.ID, workspace)
}

func (o *RuntimeOrchestrator) handleGraphBatchCompletionGateFailure(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
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
	attachRuntimeFailureDiagnostics(step, classification, execErr)
	errText := errorString(execErr)
	_ = o.recordValidationResult(ctx, run, step, common.ValidationResultStateFailed, summary, "", errText)
	_ = o.recordStepCheckpoint(ctx, run, step, "step_failed")
	_ = o.steps.Update(ctx, step)
	_ = o.publishStepState(ctx, run, step, events.TopicStepStateChanged)
	if shouldCloseCompletionGateFailureWithoutRepair(summary, step, step.Output) {
		reason := errText
		if reason == "" {
			reason = firstNonEmptyString(stringFromMap(step.Output, "error"), stringFromMap(step.Output, "reason"), "runtime step failed")
		}
		return o.failRunWithRejectedCompletionResult(ctx, run, step, reason)
	}
	return o.dispatchRuntimeRepairFollowUp(ctx, run, step, execErr)
}

func runtimeGraphSpecFromStep(step *runModel.Step) (runtimemodel.BlockExecutionGraph, bool) {
	if step == nil {
		return runtimemodel.BlockExecutionGraph{}, false
	}
	if graph, ok := runtimeGraphSpecFromValue(step.Output[stepOutputKeyExecutionGraph]); ok {
		return graph, true
	}
	return runtimeGraphSpecFromValue(step.Input[stepOutputKeyExecutionGraph])
}

func runtimeGraphSpecForContinuation(steps []*runModel.Step, last *runModel.Step) (runtimemodel.BlockExecutionGraph, bool) {
	if rootStepID := graphRootStepID(last); rootStepID != "" {
		for _, step := range steps {
			if step != nil && step.ID == rootStepID {
				if graph, ok := runtimeGraphSpecFromStep(step); ok {
					return graph, true
				}
				break
			}
		}
	}
	return runtimeGraphSpecFromStep(last)
}

func runtimeGraphSpecFromValue(value any) (runtimemodel.BlockExecutionGraph, bool) {
	switch typed := value.(type) {
	case runtimemodel.BlockExecutionGraph:
		return typed, len(typed.Nodes) > 0
	case map[string]any:
		nodes, ok := runtimeGraphNodesFromValue(typed["nodes"])
		if !ok {
			return runtimemodel.BlockExecutionGraph{}, false
		}
		return runtimemodel.BlockExecutionGraph{Nodes: nodes}, true
	default:
		return runtimemodel.BlockExecutionGraph{}, false
	}
}

func runtimeGraphNodesFromValue(value any) ([]runtimemodel.BlockExecutionNode, bool) {
	switch typed := value.(type) {
	case []runtimemodel.BlockExecutionNode:
		return append([]runtimemodel.BlockExecutionNode(nil), typed...), len(typed) > 0
	case []map[string]any:
		ret := make([]runtimemodel.BlockExecutionNode, 0, len(typed))
		for _, item := range typed {
			node, ok := runtimeGraphNodeFromMap(item)
			if !ok {
				return nil, false
			}
			ret = append(ret, node)
		}
		return ret, len(ret) > 0
	case []any:
		ret := make([]runtimemodel.BlockExecutionNode, 0, len(typed))
		for _, item := range typed {
			mapped, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			node, ok := runtimeGraphNodeFromMap(mapped)
			if !ok {
				return nil, false
			}
			ret = append(ret, node)
		}
		return ret, len(ret) > 0
	default:
		return nil, false
	}
}

func runtimeGraphNodeFromMap(item map[string]any) (runtimemodel.BlockExecutionNode, bool) {
	id, _ := item["id"].(string)
	kind, _ := item["kind"].(string)
	branch, _ := item["branch"].(string)
	id = strings.TrimSpace(id)
	kind = strings.TrimSpace(kind)
	if id == "" || kind == "" {
		return runtimemodel.BlockExecutionNode{}, false
	}
	return runtimemodel.BlockExecutionNode{
		ID:        id,
		Kind:      kind,
		DependsOn: stringSliceFromValue(item["dependsOn"]),
		Branch:    strings.TrimSpace(branch),
		Input:     mapFromValue(item["input"]),
	}, true
}

func stringSliceFromValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil
			}
			text = strings.TrimSpace(text)
			if text != "" {
				ret = append(ret, text)
			}
		}
		return ret
	default:
		return nil
	}
}

func mapFromValue(value any) map[string]any {
	typed, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return cloneStepPayload(typed)
}

func completedGraphNodes(steps []*runModel.Step, rootStepID common.StepID) map[string]bool {
	ret := map[string]bool{}
	for _, step := range steps {
		if step == nil || step.State != common.StepStateCompleted {
			continue
		}
		if !sameGraphRoot(step, rootStepID) {
			continue
		}
		nodeID, _ := step.Input[stepInputKeyGraphNodeID].(string)
		nodeID = strings.TrimSpace(nodeID)
		if nodeID != "" {
			ret[nodeID] = true
		}
	}
	return ret
}

func startedGraphNodes(steps []*runModel.Step, rootStepID common.StepID) map[string]bool {
	ret := map[string]bool{}
	for _, step := range steps {
		if !sameGraphRoot(step, rootStepID) {
			continue
		}
		nodeID := graphNodeID(step)
		if nodeID != "" {
			ret[nodeID] = true
		}
	}
	return ret
}

func selectedGraphBranch(steps []*runModel.Step) string {
	branch := ""
	for _, step := range steps {
		if step == nil || step.State != common.StepStateCompleted {
			continue
		}
		if value, _ := step.Output[stepOutputKeyGraphBranch].(string); strings.TrimSpace(value) != "" {
			branch = strings.TrimSpace(value)
			continue
		}
		if value, _ := step.Input[stepOutputKeyGraphBranch].(string); strings.TrimSpace(value) != "" {
			branch = strings.TrimSpace(value)
		}
	}
	return branch
}

func sameGraphRoot(step *runModel.Step, rootStepID common.StepID) bool {
	if step == nil || rootStepID == "" {
		return false
	}
	return graphRootStepID(step) == rootStepID
}

func graphRootStepID(step *runModel.Step) common.StepID {
	if step == nil {
		return ""
	}
	if step.Input != nil {
		root, _ := step.Input[stepInputKeyGraphRootStepID].(string)
		if strings.TrimSpace(root) != "" {
			return common.StepID(strings.TrimSpace(root))
		}
	}
	if graphNodeID(step) == "" {
		return step.ID
	}
	if previousStepID := previousExecutionStepID(step); previousStepID != "" {
		return previousStepID
	}
	return step.ID
}

func graphDispatchRootStepID(step *runModel.Step) common.StepID {
	if step == nil {
		return ""
	}
	if _, ok := runtimeGraphSpecFromValue(step.Output[stepOutputKeyExecutionGraph]); ok {
		return step.ID
	}
	return graphRootStepID(step)
}

func graphNodeDependenciesSatisfied(node runtimemodel.BlockExecutionNode, completed map[string]bool) bool {
	for _, dependency := range node.DependsOn {
		if !completed[dependency] {
			return false
		}
	}
	return true
}

func nextGraphStepInput(
	run *runModel.Run,
	last *runModel.Step,
	graph runtimemodel.BlockExecutionGraph,
	node runtimemodel.BlockExecutionNode,
	completed map[string]bool,
	branch string,
	steps []*runModel.Step,
	rootStepID common.StepID,
) map[string]any {
	input := nextStepInput(run, last.ID)
	inheritRuntimeRepairLineage(input, last)
	renderedNodeInput, _ := renderRuntimeTemplateValue(node.Input, graphRuntimeTemplateContext(steps, rootStepID)).(map[string]any)
	for key, value := range renderedNodeInput {
		input[key] = value
	}
	input[stepInputKeySoulStage] = soulStageExecution
	setRuntimeStage(input, runtimeStageExecution)
	if graphLLMNodeRequiresStateChangingOutput(run, node) {
		input["stateChangingOutputRequired"] = true
		input["repairObjective"] = repairObjectiveStateChangingOutput
		if _, ok := input["repairGuidance"]; !ok {
			input["repairGuidance"] = stateChangingRepairGuidance()
		}
	}
	input[stepOutputKeyExecutionGraph] = graph
	input[stepInputKeyGraphNodeID] = node.ID
	input[stepInputKeyGraphRootStepID] = string(graphDispatchRootStepID(last))
	input[stepInputKeyGraphPreviousNodeID] = graphNodeID(last)
	input[stepInputKeyGraphCompletedNodes] = completedGraphNodeList(completed)
	if evidence := completedDependencyGraphEvidence(node, steps, rootStepID); len(evidence) > 0 {
		input[stepInputKeyGraphEvidence] = evidence
	}
	if dependency := singleCompletedDependencyStep(node, steps, rootStepID); dependency != nil && dependency.Output != nil {
		input["previousStepKind"] = dependency.Kind
		input["previousStepOutput"] = compactStepOutputForLLMContext(dependency.Output)
	}
	if branch != "" {
		input[stepOutputKeyGraphBranch] = branch
	}
	return input
}

func completedDependencyGraphEvidence(node runtimemodel.BlockExecutionNode, steps []*runModel.Step, rootStepID common.StepID) []map[string]any {
	if len(node.DependsOn) == 0 || len(steps) == 0 {
		return nil
	}
	dependencySet := map[string]struct{}{}
	for _, dependency := range node.DependsOn {
		dependency = strings.TrimSpace(dependency)
		if dependency != "" {
			dependencySet[dependency] = struct{}{}
		}
	}
	if len(dependencySet) == 0 {
		return nil
	}
	evidenceByNode := map[string]map[string]any{}
	for _, step := range steps {
		if step == nil || step.State != common.StepStateCompleted || !sameGraphRoot(step, rootStepID) {
			continue
		}
		nodeID := graphNodeID(step)
		if _, ok := dependencySet[nodeID]; !ok {
			continue
		}
		evidenceByNode[nodeID] = map[string]any{
			"nodeId": nodeID,
			"stepId": string(step.ID),
			"kind":   step.Kind,
			"output": compactStepOutputForLLMContext(step.Output),
		}
	}
	if len(evidenceByNode) == 0 {
		return nil
	}
	ret := make([]map[string]any, 0, len(node.DependsOn))
	for _, dependency := range node.DependsOn {
		if item := evidenceByNode[strings.TrimSpace(dependency)]; item != nil {
			ret = append(ret, item)
		}
	}
	return ret
}

func singleCompletedDependencyStep(node runtimemodel.BlockExecutionNode, steps []*runModel.Step, rootStepID common.StepID) *runModel.Step {
	if len(node.DependsOn) != 1 || len(steps) == 0 {
		return nil
	}
	dependency := strings.TrimSpace(node.DependsOn[0])
	if dependency == "" {
		return nil
	}
	for _, step := range steps {
		if step == nil || step.State != common.StepStateCompleted || !sameGraphRoot(step, rootStepID) {
			continue
		}
		if graphNodeID(step) == dependency {
			return step
		}
	}
	return nil
}

func graphLLMNodeRequiresStateChangingOutput(run *runModel.Run, node runtimemodel.BlockExecutionNode) bool {
	if run == nil || !runExecutionContractRequiresStateChange(run) {
		return false
	}
	return strings.TrimSpace(node.Kind) == initialStepKind(run)
}

var runtimeTemplatePattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_-]+)\.([A-Za-z0-9_.-]+)\s*\}\}`)

func graphRuntimeTemplateContext(steps []*runModel.Step, rootStepID common.StepID) map[string]map[string]any {
	ret := make(map[string]map[string]any)
	for _, step := range steps {
		if step == nil || step.State != common.StepStateCompleted {
			continue
		}
		if rootStepID != "" && !sameGraphRoot(step, rootStepID) {
			continue
		}
		nodeID := graphNodeID(step)
		if nodeID == "" || step.Output == nil {
			continue
		}
		ret[nodeID] = cloneStepPayload(step.Output)
	}
	return ret
}

func renderRuntimeTemplateValue(value any, context map[string]map[string]any) any {
	switch typed := value.(type) {
	case map[string]any:
		ret := make(map[string]any, len(typed))
		for key, item := range typed {
			ret[key] = renderRuntimeTemplateValue(item, context)
		}
		return ret
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, renderRuntimeTemplateValue(item, context))
		}
		return ret
	case []map[string]any:
		ret := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			rendered, _ := renderRuntimeTemplateValue(item, context).(map[string]any)
			ret = append(ret, rendered)
		}
		return ret
	case []string:
		ret := make([]string, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, renderRuntimeTemplateString(item, context))
		}
		return ret
	case string:
		return renderRuntimeTemplateString(typed, context)
	default:
		return value
	}
}

func renderRuntimeTemplateString(value string, context map[string]map[string]any) string {
	if strings.TrimSpace(value) == "" || len(context) == 0 {
		return value
	}
	return runtimeTemplatePattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := runtimeTemplatePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		output := context[parts[1]]
		if output == nil {
			return match
		}
		resolved, ok := nestedMapValue(output, strings.Split(parts[2], "."))
		if !ok {
			return match
		}
		return evidenceValueString(resolved)
	})
}

func nestedMapValue(input map[string]any, path []string) (any, bool) {
	if len(path) == 0 {
		return nil, false
	}
	var current any = input
	for _, part := range path {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		mapped, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = mapped[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func graphScopedStepID(runID common.RunID, graphSeq int, nodeSeq int, nodeID string) common.StepID {
	if graphSeq < 1 {
		graphSeq = 1
	}
	if nodeSeq < 1 {
		nodeSeq = 1
	}
	nodeLabel := runtimeReadableIDPart(nodeID)
	if nodeLabel == "" {
		nodeLabel = "node"
	}
	return common.StepID(fmt.Sprintf("%s/graph-%03d/%03d-%s", runID, graphSeq, nodeSeq, nodeLabel))
}

func graphScopeSeqFromSteps(steps []*runModel.Step, rootStepID common.StepID) int {
	if rootStepID == "" {
		return 1
	}
	seen := map[common.StepID]bool{}
	seq := 0
	for _, step := range steps {
		if step == nil || graphNodeID(step) == "" {
			continue
		}
		root := graphRootStepID(step)
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		seq++
		if root == rootStepID {
			return seq
		}
	}
	return seq + 1
}

func graphNodeID(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	nodeID, _ := step.Input[stepInputKeyGraphNodeID].(string)
	return strings.TrimSpace(nodeID)
}

func graphBranch(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	if branch, _ := step.Output[stepOutputKeyGraphBranch].(string); strings.TrimSpace(branch) != "" {
		return strings.TrimSpace(branch)
	}
	branch, _ := step.Input[stepOutputKeyGraphBranch].(string)
	return strings.TrimSpace(branch)
}

func graphCompletedNodesFromStep(step *runModel.Step) []string {
	if step == nil {
		return nil
	}
	return stringSliceFromValue(step.Input[stepInputKeyGraphCompletedNodes])
}

func graphBatchStep(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	value, _ := step.Input[stepInputKeyGraphBatch].(bool)
	return value
}

func latestCompletedGraphStep(steps []*runModel.Step) *runModel.Step {
	for idx := len(steps) - 1; idx >= 0; idx-- {
		step := steps[idx]
		if step != nil && step.State == common.StepStateCompleted && graphNodeID(step) != "" {
			return step
		}
	}
	return nil
}

func latestCompletedGraphBatchStep(steps []*runModel.Step, specs []stepDispatchSpec) *runModel.Step {
	if len(specs) == 0 {
		return latestCompletedGraphStep(steps)
	}
	dispatched := map[common.StepID]bool{}
	var rootStepID common.StepID
	for _, spec := range specs {
		if spec.ID != "" {
			dispatched[spec.ID] = true
		}
		if rootStepID == "" && spec.Input != nil {
			root := strings.TrimSpace(stringFromMap(spec.Input, stepInputKeyGraphRootStepID))
			if root != "" {
				rootStepID = common.StepID(root)
			}
		}
	}
	for idx := len(steps) - 1; idx >= 0; idx-- {
		step := steps[idx]
		if step != nil && step.State == common.StepStateCompleted && dispatched[step.ID] && graphNodeID(step) != "" {
			return step
		}
	}
	if rootStepID == "" {
		return latestCompletedGraphStep(steps)
	}
	for idx := len(steps) - 1; idx >= 0; idx-- {
		step := steps[idx]
		if step != nil && step.State == common.StepStateCompleted && graphNodeID(step) != "" && sameGraphRoot(step, rootStepID) {
			return step
		}
	}
	return latestCompletedGraphStep(steps)
}

func latestFailedRepairableGraphStep(steps []*runModel.Step) *runModel.Step {
	for idx := len(steps) - 1; idx >= 0; idx-- {
		step := steps[idx]
		if step != nil && step.State == common.StepStateFailed && graphNodeID(step) != "" &&
			!graphFailureSupersededByLaterSuccess(steps, idx, step) {
			return step
		}
	}
	return nil
}

func graphFailureSupersededByLaterSuccess(steps []*runModel.Step, failedIdx int, failedStep *runModel.Step) bool {
	if failedStep == nil || failedIdx < 0 || failedIdx >= len(steps)-1 {
		return false
	}
	scope := completionEvidenceStepScope(failedStep)
	if scope == "" {
		return false
	}
	for idx := failedIdx + 1; idx < len(steps); idx++ {
		step := steps[idx]
		if step == nil || step.State != common.StepStateCompleted {
			continue
		}
		if completionEvidenceStepScope(step) == scope {
			return true
		}
	}
	return false
}

func completedGraphNodeList(completed map[string]bool) []string {
	ret := make([]string, 0, len(completed))
	for nodeID := range completed {
		ret = append(ret, nodeID)
	}
	sort.Strings(ret)
	return ret
}

func shouldDispatchGraphFollowUp(run *runModel.Run, step *runModel.Step, output map[string]any) bool {
	if !requiresDoDAssessmentCompletionGate(run) || step == nil || output == nil {
		return false
	}
	if graphNodeID(step) == "" {
		return false
	}
	return doDAssessmentFromOutput(output) == nil
}

func shouldDispatchLLMCompletionFollowUp(run *runModel.Run, step *runModel.Step, output map[string]any) bool {
	if !requiresDoDAssessmentCompletionGate(run) || step == nil || output == nil {
		return false
	}
	if graphNodeID(step) != "" {
		return false
	}
	if doDAssessmentFromOutput(output) != nil {
		return false
	}
	if step.Kind != initialStepKind(run) {
		return true
	}
	return hasCompletionFollowUpSideEffect(output)
}

func shouldDispatchNoActionableContinuationFollowUp(run *runModel.Run, step *runModel.Step, output map[string]any) bool {
	if !requiresDoDAssessmentCompletionGate(run) || step == nil || output == nil {
		return false
	}
	if boolValue(step.Input[stepInputKeyNoActionableFollowUp]) {
		return false
	}
	completionState := llmDecisionCompletionStateFromOutput(output)
	if completionState == "" || completionState == "completed" {
		return false
	}
	if doDAssessmentFromOutput(output) == nil {
		return true
	}
	return false
}

func completionStateRequiresActionableContinuation(output map[string]any, completionState string) bool {
	completionState = strings.TrimSpace(completionState)
	if completionState == "" || completionState == "completed" {
		return false
	}
	return doDAssessmentFromOutput(output) == nil
}

func failedAssessmentHasActionableCoverageGap(output map[string]any) bool {
	assessment := doDAssessmentFromOutput(output)
	if len(assessment) == 0 {
		return false
	}
	if satisfied, ok := assessment["satisfied"].(bool); ok && satisfied {
		return false
	}
	if len(missingDoDItemsFromDoDAssessment(assessment)) > 0 ||
		len(missingGoalCoverageFromDoDAssessment(assessment)) > 0 ||
		len(missingDeliverableCoverageFromDoDAssessment(assessment)) > 0 ||
		len(missingDoDItemsFromFailureOutput(output)) > 0 ||
		len(missingGoalCoverageFromFailureOutput(output)) > 0 ||
		len(missingDeliverableCoverageFromFailureOutput(output)) > 0 {
		return true
	}
	if assessmentItemHasActionableFailure(assessment) {
		return true
	}
	if coverage := mapFromValue(assessment["goalCoverage"]); assessmentItemHasActionableFailure(coverage) {
		return true
	}
	if coverage := mapFromValue(assessment["deliverableCoverage"]); assessmentItemHasActionableFailure(coverage) {
		return true
	}
	if coverage := mapFromValue(assessment["acceptanceDimensions"]); assessmentItemHasActionableFailure(coverage) {
		return true
	}
	return false
}

func assessmentItemHasActionableFailure(container map[string]any) bool {
	items, ok := doDAssessmentItemMaps(container["items"])
	if !ok {
		return false
	}
	for _, item := range items {
		state := strings.ToLower(strings.TrimSpace(stringFromMap(item, "state")))
		if state == "" || state == "satisfied" {
			continue
		}
		if strings.TrimSpace(firstNonEmptyString(
			stringFromMap(item, "item"),
			stringFromMap(item, "goal"),
			stringFromMap(item, "goalItem"),
			stringFromMap(item, "requirement"),
			stringFromMap(item, "reason"),
			stringFromMap(item, "summary"),
		)) != "" {
			return true
		}
		if len(stringSliceFromValue(item["nextActions"])) > 0 ||
			len(mapSliceFromValue(item["evidenceCandidates"])) > 0 ||
			len(evidenceRefsFromValue(item["evidenceRefs"])) > 0 {
			return true
		}
	}
	return false
}

func graphFollowUpExpansionStep(step *runModel.Step) bool {
	if step == nil || step.Input == nil || step.Output == nil {
		return false
	}
	if !boolValue(step.Input["graphFollowUp"]) {
		return false
	}
	_, ok := runtimeGraphSpecFromValue(step.Output[stepOutputKeyExecutionGraph])
	return ok
}

func (o *RuntimeOrchestrator) graphFollowUpExpansionCount(ctx context.Context, runID common.RunID) int {
	if o == nil || o.steps == nil || runID == "" {
		return 0
	}
	steps, err := o.steps.ListByRun(ctx, runID)
	if err != nil {
		return 0
	}
	count := 0
	for _, step := range steps {
		if graphFollowUpExpansionStep(step) {
			count++
		}
	}
	return count
}

func hasCompletionFollowUpSideEffect(output map[string]any) bool {
	return outputItemCount(output[stepOutputKeyPatches]) > 0 ||
		outputItemCount(output[stepOutputKeyMemoryWrites]) > 0 ||
		outputItemCount(output[stepOutputKeyMemoryRefs]) > 0
}

func outputItemCount(value any) int {
	switch typed := value.(type) {
	case []map[string]any:
		return len(typed)
	case []string:
		return len(typed)
	case []any:
		return len(typed)
	default:
		return 0
	}
}

func boolValue(value any) bool {
	typed, _ := value.(bool)
	return typed
}

func llmCompletionFollowUpStepID(runID common.RunID, step *runModel.Step) common.StepID {
	stepID := runtimeChildID(common.StepID(runtimeReadableStepLabel(runID, step.ID)), "completion")
	if stepID == "" {
		stepID = "completion"
	}
	return common.StepID(fmt.Sprintf("%s/follow-up-%s", runID, stepID))
}

func llmCompletionFollowUpStepInput(run *runModel.Run, step *runModel.Step) map[string]any {
	input := nextStepInput(run, step.ID)
	input[stepInputKeySoulStage] = soulStageValidation
	setRuntimeStage(input, runtimeStageVerification)
	input["completionFollowUp"] = true
	inheritRuntimeRepairLineage(input, step)
	inheritWorkspaceFileManifest(input, step)
	inheritStableExecutionContextPackage(input, step)
	if step.Kind != "" {
		input["previousStepKind"] = step.Kind
	}
	if step.Output != nil {
		input["previousStepOutput"] = compactStepOutputForLLMContext(step.Output)
	}
	return input
}

func llmCompletionFollowUpStepInputFromSteps(run *runModel.Run, step *runModel.Step, steps []*runModel.Step) map[string]any {
	input := llmCompletionFollowUpStepInput(run, step)
	return enrichRunEvidenceSummaryInput(input, runEvidenceSummaryFromRunSteps(run, steps))
}

func graphExpansionLimitAssessmentFollowUpStepID(runID common.RunID, step *runModel.Step) common.StepID {
	stepID := runtimeChildID(common.StepID(runtimeReadableStepLabel(runID, step.ID)), "assessment")
	if stepID == "" {
		stepID = "assessment"
	}
	return common.StepID(fmt.Sprintf("%s/follow-up-graph-limit-%s", runID, stepID))
}

func (o *RuntimeOrchestrator) graphExpansionLimitAssessmentFollowUpStepInputFromSteps(ctx context.Context, run *runModel.Run, step *runModel.Step, steps []*runModel.Step, count int) map[string]any {
	input := llmCompletionFollowUpStepInputFromSteps(run, step, steps)
	input[stepInputKeyAssessmentOnly] = true
	input[stepInputKeyGraphLimitFollowUp] = true
	setRuntimeStage(input, runtimeStageVerification)
	input["assessmentOnlyReason"] = "graph_follow_up_expansion_limit_exceeded"
	input["graphFollowUpExpansionCount"] = count
	input["graphFollowUpExpansionLimit"] = maxGraphFollowUpExpansionAttempts
	input["allowedAssessmentOnlyCompletionStates"] = []string{"completed", "failed", "blocked_on_decision", "blocked_on_approval"}
	delete(input, stepOutputKeyExecutionGraph)
	if previous := mapFromValue(input["previousStepOutput"]); len(previous) > 0 {
		delete(previous, stepOutputKeyExecutionGraph)
		previous["graphFollowUpExpansionLimitExceeded"] = true
		input["previousStepOutput"] = previous
	}
	if closure := o.latestCompletionClosureEvidence(ctx, run, step); len(closure) > 0 {
		input["latestClosureEvidence"] = closure
		if superseded := mapSliceFromValue(closure["supersededFailures"]); len(superseded) > 0 {
			input["supersededFailureEvidence"] = superseded
		}
	}
	input["assessmentEvidenceGuidance"] = []string{
		"Return only a completed or failed dodAssessment, or a real governance blocker.",
		"Use concrete step IDs from runEvidenceSummary or latestClosureEvidence as evidenceRefs; do not cite only the run ID or memory refs for satisfied items.",
		"Prefer later successful evidence in the same neutral scope over earlier failed evidence.",
		"If the available evidence is insufficient, mark only the unsupported item failed/skipped and state the exact missing evidence.",
	}
	return input
}

func noActionableContinuationFollowUpStepID(runID common.RunID, step *runModel.Step) common.StepID {
	stepID := runtimeChildID(common.StepID(runtimeReadableStepLabel(runID, step.ID)), "continuation")
	if stepID == "" {
		stepID = "continuation"
	}
	return common.StepID(fmt.Sprintf("%s/follow-up-no-actionable-%s", runID, stepID))
}

func noActionableContinuationFollowUpStepInput(run *runModel.Run, step *runModel.Step) map[string]any {
	input := llmCompletionFollowUpStepInput(run, step)
	input[stepInputKeyNoActionableFollowUp] = true
	input["completionFollowUp"] = false
	input["continuationFollowUp"] = true
	setRuntimeStage(input, runtimeStageVerification)
	stateChangingContractInherited := inheritStateChangingRepairContract(input, step)
	if !stateChangingContractInherited {
		inheritConcreteValidationRepairContract(input, step)
	}
	if completionState := llmDecisionCompletionStateFromOutput(step.Output); completionState != "" {
		input["previousCompletionState"] = completionState
		input["continuationFailureReason"] = fmt.Sprintf("completionState %s has no actionable continuation", completionState)
	}
	if failedAssessmentHasActionableCoverageGap(step.Output) {
		input["actionableFailedAssessmentContinuation"] = true
		if !stateChangingContractInherited {
			input["repairObjective"] = repairObjectiveConcreteValidationEvidence
			input["evidenceRepairRequired"] = true
		} else {
			input["coverageRepairRequired"] = true
		}
		input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
			"Previous output returned a failed assessment with actionable coverage gaps; convert the gaps into bounded validation evidence or targeted repair instead of repeating a descriptive failed assessment.",
			"Preserve missing DoD, Goal, and deliverable coverage targets and cite concrete evidenceRefs when a target is closed.",
		)
		if missing := missingDoDItemsFromStep(step); len(missing) > 0 {
			input["missingDoDItems"] = missing
		}
		if missing := missingGoalCoverageFromStep(step); len(missing) > 0 {
			input["missingGoalCoverage"] = missing
		}
		if missing := missingDeliverableCoverageFromStep(step); len(missing) > 0 {
			input["missingDeliverableCoverage"] = missing
		}
		ensureCoverageRepairTargetSet(input)
	}
	return input
}

func inheritStateChangingRepairContract(input map[string]any, previousStep *runModel.Step) bool {
	if len(input) == 0 || previousStep == nil || !inputRequiresStateChangingOutput(previousStep.Input) {
		return false
	}
	for _, key := range []string{
		"repairObjective",
		"stateChangingOutputRequired",
		"coverageRepairRequired",
		"coverageTargetSet",
		"missingDoDItems",
		"missingGoalCoverage",
		"missingDeliverableCoverage",
		"failureTargetRepairContext",
		"patchRepairContext",
		"rootCauseCandidateContext",
		"structuredFailureSignals",
		"repairTargetSet",
		"currentExecutionProgress",
		"processEvidenceTimeline",
		"convergenceContract",
		"requiresCurrentTargetSnapshotForStateChange",
	} {
		if value, ok := previousStep.Input[key]; ok && value != nil {
			input[key] = cloneStepInputValue(value)
		}
	}
	setRuntimeStage(input, runtimeStageRepair)
	input[stepInputKeySoulStage] = soulStageExecution
	input["repairObjective"] = repairObjectiveStateChangingOutput
	input["stateChangingOutputRequired"] = true
	delete(input, "boundedValidationRequired")
	delete(input, "evidenceRepairRequired")
	delete(input, "concreteFailureEvidenceRequired")
	if pkg := mapFromValue(previousStep.Input["repairActionPackage"]); len(pkg) > 0 {
		next := cloneStepPayload(pkg)
		required := mapFromValue(next["requiredOutput"])
		if len(required) == 0 {
			required = stateChangingRepairRequiredOutput()
		}
		if !boolValue(required["mustIncludeStateChangingNode"]) {
			required["mustIncludeStateChangingNode"] = true
		}
		if len(stringSliceFromValue(required["acceptedKinds"])) == 0 {
			required["acceptedKinds"] = stateChangingRepairAcceptedKinds()
		}
		next["requiredOutput"] = required
		input["repairActionPackage"] = next
	}
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"Previous step was governed by a state-changing repair contract; preserve repairObjective, convergenceContract, repairActionPackage.requiredOutput, and target evidence.",
		"Do not convert this continuation into a stage-choice decision while accepted state-changing outputs or bounded evidence acquisition remain available to runtime.",
	)
	ensureCoverageRepairTargetSet(input)
	return true
}

func inheritConcreteValidationRepairContract(input map[string]any, previousStep *runModel.Step) bool {
	if len(input) == 0 || previousStep == nil || !inputRequiresConcreteValidationEvidence(previousStep.Input) {
		return false
	}
	for _, key := range []string{
		"repairObjective",
		"boundedValidationRequired",
		"evidenceRepairRequired",
		"coverageRepairRequired",
		"coverageTargetSet",
		"missingDoDItems",
		"missingGoalCoverage",
		"missingDeliverableCoverage",
		"verificationEvidenceGap",
		"currentExecutionProgress",
		"processEvidenceTimeline",
		"convergenceContract",
	} {
		if value, ok := previousStep.Input[key]; ok && value != nil {
			input[key] = cloneStepInputValue(value)
		}
	}
	input["repairObjective"] = repairObjectiveConcreteValidationEvidence
	input["boundedValidationRequired"] = true
	input["evidenceRepairRequired"] = true
	delete(input, "stateChangingOutputRequired")
	delete(input, "concreteFailureEvidenceRequired")
	if pkg := mapFromValue(previousStep.Input["repairActionPackage"]); len(pkg) > 0 {
		next := cloneStepPayload(pkg)
		required := concreteValidationEvidenceRequiredOutput(mapFromValue(next["requiredOutput"]))
		next["requiredOutput"] = required
		next["evidenceRepairRequired"] = true
		input["repairActionPackage"] = next
	}
	input["repairGuidance"] = appendRepairGuidance(input["repairGuidance"],
		"Previous step was governed by a concrete validation evidence contract; preserve repairObjective, convergenceContract, requiredOutput, and coverage targets.",
		"Do not downgrade this continuation to assessment-only or repeat a plain failed dodAssessment; return bounded shell/command or mcp/tool_call validation evidence, or a real governance blocker.",
	)
	ensureCoverageRepairTargetSet(input)
	return true
}

func inputRequiresStateChangingOutput(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if boolValue(input["stateChangingOutputRequired"]) ||
		strings.TrimSpace(stringFromMap(input, "repairObjective")) == repairObjectiveStateChangingOutput {
		return true
	}
	return repairActionPackageRequiresStateChangingOutput(mapFromValue(input["repairActionPackage"]))
}

func inputRequiresConcreteValidationEvidence(input map[string]any) bool {
	if len(input) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromMap(input, "repairObjective")) == repairObjectiveConcreteValidationEvidence ||
		boolValue(input["boundedValidationRequired"]) ||
		boolValue(input["evidenceRepairRequired"]) {
		return true
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	return boolValue(required["mustProduceConcreteValidationEvidence"])
}

func cloneStepInputValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneStepPayload(typed)
	case []map[string]any:
		return cloneMapSlice(typed)
	default:
		return typed
	}
}

func noActionableContinuationFollowUpStepInputFromSteps(run *runModel.Run, step *runModel.Step, steps []*runModel.Step) map[string]any {
	input := noActionableContinuationFollowUpStepInput(run, step)
	return enrichRunEvidenceSummaryInput(input, runEvidenceSummaryFromRunSteps(run, steps))
}

func (o *RuntimeOrchestrator) noActionableContinuationFollowUpStepInput(ctx context.Context, run *runModel.Run, step *runModel.Step) map[string]any {
	input := noActionableContinuationFollowUpStepInput(run, step)
	if o == nil || o.steps == nil || run == nil {
		return input
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return input
	}
	return enrichRunEvidenceSummaryInput(input, runEvidenceSummaryFromRunSteps(run, steps))
}

func (o *RuntimeOrchestrator) llmCompletionFollowUpStepInput(ctx context.Context, run *runModel.Run, step *runModel.Step) map[string]any {
	input := llmCompletionFollowUpStepInput(run, step)
	if o == nil || o.steps == nil || run == nil {
		return input
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return input
	}
	return enrichRunEvidenceSummaryInput(input, runEvidenceSummaryFromRunSteps(run, steps))
}

func graphFollowUpStepID(runID common.RunID, step *runModel.Step) common.StepID {
	nodeID := runtimeChildID(common.StepID(graphNodeID(step)), "")
	if nodeID == "" {
		nodeID = runtimeChildID(common.StepID(runtimeReadableStepLabel(runID, step.ID)), "graph")
	}
	if rootStepID := graphRootStepID(step); rootStepID != "" {
		nodeID = runtimeChildID(common.StepID(runtimeReadableStepLabel(runID, rootStepID)), nodeID)
	}
	return common.StepID(fmt.Sprintf("%s/follow-up-graph-%s", runID, nodeID))
}

func (o *RuntimeOrchestrator) graphFollowUpStepInput(ctx context.Context, run *runModel.Run, step *runModel.Step) map[string]any {
	if o == nil || o.steps == nil || run == nil {
		return graphFollowUpStepInput(run, step)
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return graphFollowUpStepInput(run, step)
	}
	return o.graphFollowUpStepInputFromSteps(run, step, steps)
}

func (o *RuntimeOrchestrator) graphFollowUpStepInputFromSteps(run *runModel.Run, step *runModel.Step, steps []*runModel.Step) map[string]any {
	input := graphFollowUpStepInput(run, step)
	evidence := o.completedGraphEvidenceFromSteps(run, step, steps)
	input = enrichGraphFollowUpInputWithEvidence(input, evidence)
	input = restoreRepairContractAfterConcreteEvidenceAcquisition(input, evidenceAcquisitionSourceStep(step, steps), evidence)
	return enrichRunEvidenceSummaryInput(input, runEvidenceSummaryFromRunSteps(run, steps))
}

func enrichGraphFollowUpInputWithEvidence(input map[string]any, evidence []map[string]any) map[string]any {
	if len(evidence) == 0 {
		return input
	}
	input[stepInputKeyGraphEvidence] = evidence
	completed := make([]string, 0, len(evidence))
	for _, item := range evidence {
		nodeID, _ := item["nodeId"].(string)
		nodeID = strings.TrimSpace(nodeID)
		if nodeID != "" {
			completed = append(completed, nodeID)
		}
	}
	if len(completed) > 0 {
		input[stepInputKeyGraphCompletedNodes] = completed
	}
	return input
}

func evidenceAcquisitionSourceStep(anchor *runModel.Step, steps []*runModel.Step) *runModel.Step {
	if stepRequiresConcreteFailureEvidenceAcquisition(anchor) {
		return anchor
	}
	rootStepID := graphRootStepID(anchor)
	if rootStepID == "" {
		return anchor
	}
	for _, step := range steps {
		if step == nil || step.ID != rootStepID {
			continue
		}
		if stepRequiresConcreteFailureEvidenceAcquisition(step) {
			return step
		}
		break
	}
	return anchor
}

func restoreRepairContractAfterConcreteEvidenceAcquisition(input map[string]any, source *runModel.Step, evidence []map[string]any) map[string]any {
	if len(input) == 0 || source == nil || source.Input == nil || !stepRequiresConcreteFailureEvidenceAcquisition(source) {
		return input
	}
	snapshots := targetSnapshotsFromConcreteFailureEvidence(evidence)
	sourcePackage := mapFromValue(source.Input["repairActionPackage"])
	if sourcePackage == nil {
		sourcePackage = map[string]any{}
	}
	probe := cloneStepPayload(source.Input)
	if probe == nil {
		probe = map[string]any{}
	}
	if len(snapshots) > 0 {
		probe["failureTargetRepairContext"] = snapshots
	}
	evidenceSignals := structuredFailureSignalsFromConcreteFailureEvidence(evidence)
	if len(evidenceSignals) > 0 {
		probe["structuredFailureSignals"] = mergeStructuredFailureSignals(mapFromValue(probe["structuredFailureSignals"]), evidenceSignals)
		sourcePackage["structuredFailureSignals"] = mergeStructuredFailureSignals(mapFromValue(sourcePackage["structuredFailureSignals"]), evidenceSignals)
	}
	actionPackage := repairActionPackageForEvidenceAcquisitionFollowUp(probe, sourcePackage, snapshots)
	if !stateChangingRepairHasConcreteTargetEvidence(probe, actionPackage) {
		return input
	}
	input[stepInputKeySoulStage] = soulStageExecution
	setRuntimeStage(input, runtimeStageRepair)
	input[stepInputKeyRuntimeRepairFollowUp] = true
	input["evidenceAcquisitionFollowUp"] = true
	input["stateChangingOutputRequired"] = true
	input["repairObjective"] = repairObjectiveStateChangingOutput
	delete(input, "concreteFailureEvidenceRequired")
	input["repairGuidance"] = appendRepairGuidance(stateChangingRepairGuidance(),
		"Concrete failure evidence has been acquired; continue with a target-scoped state-changing repair or return a failed assessment/blocker if the evidence proves the target is not repairable.",
		"Use concreteFailureEvidence and any targetSnapshots as current evidence; do not ask for an operator decision when the target, diagnostic, and output contract are already available.",
	)
	if len(evidence) > 0 {
		input["concreteFailureEvidence"] = compactLLMContextValue(evidence, 1)
	}
	if len(snapshots) > 0 {
		input["failureTargetRepairContext"] = snapshots
	}
	if value, ok := source.Input[failureOutputKeyClass]; ok && input[failureOutputKeyClass] == nil {
		input[failureOutputKeyClass] = compactLLMContextValue(value, 1)
	}
	if value, ok := source.Input[failureOutputKeySignature]; ok && input[failureOutputKeySignature] == nil {
		input[failureOutputKeySignature] = compactLLMContextValue(value, 1)
	}
	if value, ok := source.Input["failureTargetScope"]; ok && input["failureTargetScope"] == nil {
		input["failureTargetScope"] = compactLLMContextValue(value, 1)
	}
	if signals := mapFromValue(sourcePackage["structuredFailureSignals"]); len(signals) > 0 && len(mapFromValue(input["structuredFailureSignals"])) == 0 {
		input["structuredFailureSignals"] = compactLLMContextMap(signals, 1)
	}
	input["repairActionPackage"] = actionPackage
	contract := BuildConvergenceContract(nil, nil, source, map[string]any{"input": input}, map[string]any{"sourceRef": string(source.ID)})
	input = ApplyConvergenceContract(input, contract)
	return input
}

func stepRequiresConcreteFailureEvidenceAcquisition(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	if boolValue(step.Input["concreteFailureEvidenceRequired"]) {
		return true
	}
	if strings.TrimSpace(stringFromMap(step.Input, "repairObjective")) == repairObjectiveAcquireFailureEvidence {
		return true
	}
	return repairActionPackageRequiresEvidenceAcquisition(mapFromValue(step.Input["repairActionPackage"]))
}

func targetSnapshotsFromConcreteFailureEvidence(evidence []map[string]any) []map[string]any {
	seen := map[string]bool{}
	var ret []map[string]any
	for _, item := range evidence {
		output := mapFromValue(item["output"])
		if len(output) == 0 || strings.TrimSpace(stringFromMap(output, "stage")) != "file_read" || !boolValue(output["exists"]) {
			continue
		}
		targetRef := strings.TrimSpace(stringFromMap(output, "path"))
		if targetRef == "" || seen[targetRef] || boolValue(output["isBinary"]) {
			continue
		}
		seen[targetRef] = true
		snapshot := map[string]any{
			"targetRef":        targetRef,
			"exists":           true,
			"contentHash":      stringFromMap(output, "contentHash"),
			"content":          stringFromMap(output, "content"),
			"size":             output["size"],
			"source":           "concrete_failure_evidence",
			"contentComplete":  !boolValue(output["truncated"]),
			"safeForFileWrite": !boolValue(output["truncated"]) && !containsRuntimeRedactionMarker(stringFromMap(output, "content")),
		}
		if value, ok := output["truncated"]; ok {
			snapshot["truncated"] = value
		}
		if value, ok := output["isBinary"]; ok {
			snapshot["isBinary"] = value
		}
		ret = append(ret, repairActionTargetSnapshot(snapshot))
		if len(ret) >= maxLLMContextSliceItems {
			break
		}
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func structuredFailureSignalsFromConcreteFailureEvidence(evidence []map[string]any) map[string]any {
	if len(evidence) == 0 {
		return nil
	}
	merged := map[string]any{}
	for _, item := range evidence {
		output := mapFromValue(item["output"])
		if len(output) == 0 {
			continue
		}
		signals := structuredFailureSignalsFromFailureOutput(output, nil, "", "")
		if fileListEvidenceHasEntries(item, output) {
			for _, entry := range mapSliceFromValue(output["entries"]) {
				if strings.EqualFold(strings.TrimSpace(stringFromMap(entry, "kind")), "directory") || boolValue(entry["isBinary"]) {
					continue
				}
				targetRef := strings.TrimSpace(stringFromMap(entry, "path"))
				if structuredFailureTextTargetRefAllowed(targetRef) {
					signals = appendStructuredFailureSignalTarget(signals, targetRef)
				}
			}
		}
		if path := strings.TrimSpace(stringFromMap(output, "path")); path != "" && structuredFailureTextTargetRefAllowed(path) {
			signals = appendStructuredFailureSignalTarget(signals, path)
		}
		if len(signals) > 0 {
			merged = mergeStructuredFailureSignals(merged, signals)
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func fileListEvidenceHasEntries(item map[string]any, output map[string]any) bool {
	if len(output) == 0 || len(mapSliceFromValue(output["entries"])) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromMap(output, "stage")) == "file_list" {
		return true
	}
	return strings.TrimSpace(stringFromMap(item, "kind")) == "file/list"
}

func appendStructuredFailureSignalTarget(signals map[string]any, targetRef string) map[string]any {
	targetRef = normalizeStructuredFailureTargetRef(targetRef)
	if targetRef == "" {
		return signals
	}
	if signals == nil {
		signals = map[string]any{}
	}
	targets := stringSliceFromValue(signals["targetRefs"])
	targets = append(targets, targetRef)
	signals["targetRefs"] = limitedStrings(compactUniqueStrings(targets), maxLLMContextSliceItems)
	return signals
}

func mergeStructuredFailureSignals(first map[string]any, second map[string]any) map[string]any {
	if len(first) == 0 && len(second) == 0 {
		return nil
	}
	ret := cloneStepPayload(first)
	if ret == nil {
		ret = map[string]any{}
	}
	for _, key := range []string{"sources", "targetRefs", "externalReferences", "identifiers", "messages"} {
		values := append(stringSliceFromValue(ret[key]), stringSliceFromValue(second[key])...)
		if values = limitedStrings(compactUniqueStrings(values), maxLLMContextSliceItems); len(values) > 0 {
			ret[key] = values
		}
	}
	if lineRefs := mergeStructuredFailureLineRefs(mapSliceFromValue(ret["lineRefs"]), mapSliceFromValue(second["lineRefs"])); len(lineRefs) > 0 {
		ret["lineRefs"] = lineRefs
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func mergeStructuredFailureLineRefs(first []map[string]any, second []map[string]any) []map[string]any {
	seen := map[string]bool{}
	ret := make([]map[string]any, 0, len(first)+len(second))
	add := func(items []map[string]any) {
		for _, item := range items {
			targetRef := normalizeStructuredFailureTargetRef(stringFromMap(item, "targetRef"))
			if targetRef == "" {
				continue
			}
			line := intFromValue(item["line"])
			column := intFromValue(item["column"])
			key := fmt.Sprintf("%s:%d:%d", targetRef, line, column)
			if seen[key] {
				continue
			}
			seen[key] = true
			next := cloneStepPayload(item)
			next["targetRef"] = targetRef
			ret = append(ret, next)
			if len(ret) >= maxLLMContextSliceItems {
				return
			}
		}
	}
	add(first)
	add(second)
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func intFromValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		value, _ := typed.Int64()
		return int(value)
	default:
		return 0
	}
}

func repairActionPackageForEvidenceAcquisitionFollowUp(input map[string]any, sourcePackage map[string]any, snapshots []map[string]any) map[string]any {
	pkg := cloneStepPayload(sourcePackage)
	if pkg == nil {
		pkg = map[string]any{}
	}
	delete(pkg, "evidenceAcquisitionRequired")
	delete(pkg, "evidenceAcquisitionReason")
	required := stateChangingRepairRequiredOutput()
	for key, value := range mapFromValue(pkg["requiredOutput"]) {
		switch key {
		case "mustReturnValidJSON", "mustNotReuseRejectedJSON", "forbiddenStalePatchReuse", "mustDeclareStrategyReassessmentFields", "strategyReassessmentRequiredFields", "strategyReassessmentMatchRule", "mustReassessRepeatedFailedSurface":
			required[key] = value
		}
	}
	if len(snapshots) > 0 {
		existing := mapSliceFromValue(pkg["targetSnapshots"])
		pkg["targetSnapshots"] = mergeRepairTargetSnapshots(existing, snapshots)
		pkg["currentTargetSnapshots"] = repairActionTargetSnapshotMetadata(mapSliceFromValue(pkg["targetSnapshots"]))
		required["mustTargetFailureScope"] = true
		required["targetRelation"] = "failure_target_repair_snapshot_or_declared_related_failure_target"
		required["mustUseCurrentTargetSnapshot"] = true
	}
	pkg["requiredOutput"] = required
	if targetSet := repairActionTargetSet(input, pkg); len(targetSet) > 0 {
		pkg["repairTargetSet"] = targetSet
		required["mustTargetRepairTargetSet"] = true
	}
	if len(mapFromValue(pkg["currentFailure"])) == 0 {
		if currentFailure := currentFailureForEvidenceAcquisitionFollowUp(input, pkg); len(currentFailure) > 0 {
			pkg["currentFailure"] = currentFailure
		}
	}
	if len(mapFromValue(pkg["currentExecutionProgress"])) == 0 {
		if progress := currentExecutionProgressForEvidenceAcquisitionFollowUp(input, pkg); len(progress) > 0 {
			pkg["currentExecutionProgress"] = progress
		}
	}
	pkg["evidenceAcquisitionCompleted"] = true
	pkg["exitCriteria"] = []string{
		"state_changing_artifact_applied_then_verify",
		"failed_assessment_or_blocker_when_not_repairable",
	}
	return pkg
}

func currentFailureForEvidenceAcquisitionFollowUp(input map[string]any, pkg map[string]any) map[string]any {
	for _, digest := range []map[string]any{
		mapFromValue(input["currentProgressDigest"]),
		mapFromValue(pkg["currentExecutionProgress"]),
		mapFromValue(input["currentExecutionProgress"]),
	} {
		if currentProgressDigestIsActionableGap(digest) {
			if currentFailure := currentFailureFromProgressDigest(digest); len(currentFailure) > 0 {
				return currentFailure
			}
		}
	}
	if currentFailure := repairActionCurrentFailure(input); len(currentFailure) > 0 {
		return currentFailure
	}
	for _, source := range []map[string]any{
		pkg,
		replanContinuationFailureContextForCurrentProgress(mapFromValue(input["executionContextPackage"])),
		replanContinuationTicketContextForCurrentProgress(mapFromValue(input["executionContextPackage"])),
		replanContinuationForCurrentProgress(mapFromValue(input["executionContextPackage"])),
		replanContinuationFailureContextForCurrentProgress(mapFromValue(input["executionContext"])),
		replanContinuationTicketContextForCurrentProgress(mapFromValue(input["executionContext"])),
		replanContinuationForCurrentProgress(mapFromValue(input["executionContext"])),
	} {
		if len(source) == 0 {
			continue
		}
		ret := map[string]any{}
		add := func(key string, value string) {
			if value = strings.TrimSpace(value); value != "" {
				ret[key] = value
			}
		}
		add("failedStepId", stringFromMap(source, "failedStepId"))
		add("failedStepKind", stringFromMap(source, "failedStepKind"))
		add("failureClass", stringFromMap(source, failureOutputKeyClass))
		add("failureSignature", stringFromMap(source, failureOutputKeySignature))
		add("targetScope", stringFromMap(source, "failureTargetScope"))
		add("targetRef", repairActionTargetRefFromScope(stringFromMap(source, "failureTargetScope")))
		add("reason", firstNonEmptyString(stringFromMap(source, "failureReason"), stringFromMap(source, "reason")))
		add("error", stringFromMap(source, "error"))
		if output := mapFromValue(source["failedStepOutput"]); len(output) > 0 {
			add("stage", stringFromMap(output, "stage"))
			add("failureClass", firstNonEmptyString(stringFromMap(source, failureOutputKeyClass), stringFromMap(output, failureOutputKeyClass)))
			add("failureSignature", firstNonEmptyString(stringFromMap(source, failureOutputKeySignature), stringFromMap(output, failureOutputKeySignature)))
			add("reason", firstNonEmptyString(stringFromMap(output, "reason"), stringFromMap(source, "failureReason"), stringFromMap(source, "reason")))
			add("error", firstNonEmptyString(stringFromMap(output, "error"), stringFromMap(source, "error")))
		}
		if len(ret) > 0 {
			return compactLLMContextMap(ret, 1)
		}
	}
	return nil
}

func currentExecutionProgressForEvidenceAcquisitionFollowUp(input map[string]any, pkg map[string]any) map[string]any {
	for _, source := range []map[string]any{
		mapFromValue(input["currentExecutionProgress"]),
		mapFromValue(pkg["currentExecutionProgress"]),
		mapFromValue(input["currentProgressDigest"]),
		replanContinuationCurrentProgressDigest(mapFromValue(input["executionContextPackage"])),
		replanContinuationCurrentProgressDigest(mapFromValue(input["executionContext"])),
	} {
		if repairProgressDigestIsInformative(source) {
			return compactLLMContextMap(source, 0)
		}
	}
	return nil
}

func replanContinuationForCurrentProgress(container map[string]any) map[string]any {
	replan := mapFromValue(container["replanContinuation"])
	if len(replan) == 0 || !replanContinuationCanDriveCurrentProgress(container, replan) {
		return nil
	}
	return replan
}

func replanContinuationFailureContextForCurrentProgress(container map[string]any) map[string]any {
	return mapFromValue(replanContinuationForCurrentProgress(container)["failureContext"])
}

func replanContinuationTicketContextForCurrentProgress(container map[string]any) map[string]any {
	return mapFromValue(replanContinuationForCurrentProgress(container)["ticketContext"])
}

func replanContinuationCurrentProgressDigest(container map[string]any) map[string]any {
	return mapFromValue(replanContinuationForCurrentProgress(container)["currentProgressDigest"])
}

func replanContinuationCanDriveCurrentProgress(container map[string]any, replan map[string]any) bool {
	if len(replan) == 0 {
		return false
	}
	if boolValue(replan["historicalOnly"]) || replan["useAsCurrentProgress"] == false {
		return false
	}
	if containerHasRejectedResultContinuation(container) {
		return false
	}
	return true
}

func containerHasRejectedResultContinuation(container map[string]any) bool {
	if len(container) == 0 {
		return false
	}
	if len(mapFromValue(container["rejectedResultContinuation"])) > 0 {
		return true
	}
	return len(mapFromValue(mapFromValue(container["executionContext"])["rejectedResultContinuation"])) > 0
}

func repairProgressDigestIsInformative(source map[string]any) bool {
	if len(source) == 0 {
		return false
	}
	currentGap := strings.TrimSpace(stringFromMap(source, "currentGap"))
	if currentGap == "" || currentGap == "none_observed_in_recent_process_evidence" {
		if len(mapFromValue(source["lastObservedFailure"])) == 0 &&
			len(mapFromValue(source["lastAttempt"])) == 0 {
			return false
		}
	}
	return true
}

func mergeRepairTargetSnapshots(first []map[string]any, second []map[string]any) []map[string]any {
	seen := map[string]bool{}
	ret := make([]map[string]any, 0, len(first)+len(second))
	add := func(items []map[string]any) {
		for _, item := range items {
			targetRef := strings.TrimSpace(stringFromMap(item, "targetRef"))
			if targetRef == "" || seen[targetRef] {
				continue
			}
			seen[targetRef] = true
			ret = append(ret, repairActionTargetSnapshot(item))
			if len(ret) >= maxLLMContextSliceItems {
				return
			}
		}
	}
	add(first)
	add(second)
	if len(ret) == 0 {
		return nil
	}
	return ret
}

func graphFollowUpStepInput(run *runModel.Run, step *runModel.Step) map[string]any {
	input := nextStepInput(run, step.ID)
	input[stepInputKeySoulStage] = soulStageValidation
	setRuntimeStage(input, runtimeStageVerification)
	input["graphFollowUp"] = true
	inheritRuntimeRepairLineage(input, step)
	inheritWorkspaceFileManifest(input, step)
	inheritStableExecutionContextPackage(input, step)
	if nodeID := graphNodeID(step); nodeID != "" {
		input[stepInputKeyGraphPreviousNodeID] = nodeID
	}
	if completed := graphCompletedNodesFromStep(step); len(completed) > 0 {
		input[stepInputKeyGraphCompletedNodes] = completed
	}
	if branch := graphBranch(step); branch != "" {
		input[stepOutputKeyGraphBranch] = branch
	}
	if graph, ok := runtimeGraphSpecFromStep(step); ok {
		input[stepOutputKeyExecutionGraph] = compactExecutionGraphForLLMContext(graph)
		input[stepInputKeyGraphRootStepID] = string(graphRootStepID(step))
	}
	return input
}

func (o *RuntimeOrchestrator) completedGraphEvidence(ctx context.Context, run *runModel.Run, anchor *runModel.Step) []map[string]any {
	if o == nil || o.steps == nil || run == nil || anchor == nil {
		return nil
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil
	}
	return o.completedGraphEvidenceFromSteps(run, anchor, steps)
}

func (o *RuntimeOrchestrator) completedGraphEvidenceFromSteps(run *runModel.Run, anchor *runModel.Step, steps []*runModel.Step) []map[string]any {
	graph, ok := runtimeGraphSpecFromStep(anchor)
	if !ok || len(graph.Nodes) == 0 {
		return nil
	}
	completedByNode := map[string]*runModel.Step{}
	for _, step := range steps {
		if step == nil || step.State != common.StepStateCompleted {
			continue
		}
		nodeID := graphNodeID(step)
		if nodeID == "" {
			continue
		}
		completedByNode[nodeID] = step
	}
	evidence := make([]map[string]any, 0, len(completedByNode))
	for _, node := range graph.Nodes {
		step := completedByNode[node.ID]
		if step == nil {
			continue
		}
		evidence = append(evidence, map[string]any{
			"nodeId":    node.ID,
			"stepId":    string(step.ID),
			"kind":      step.Kind,
			"output":    compactStepOutputForLLMContext(step.Output),
			"outputRef": stepOutputRefForLLMContext(run, step),
		})
	}
	return evidence
}

func enrichRunEvidenceSummaryInput(input map[string]any, evidence []map[string]any) map[string]any {
	if len(input) == 0 || len(evidence) == 0 {
		return input
	}
	input[stepInputKeyRunEvidenceSummary] = evidence
	return input
}

func runEvidenceSummaryFromSteps(steps []*runModel.Step) []map[string]any {
	return runEvidenceSummaryFromRunSteps(nil, steps)
}

func runEvidenceSummaryFromRunSteps(run *runModel.Run, steps []*runModel.Step) []map[string]any {
	if len(steps) == 0 {
		return nil
	}
	selected := steps
	if len(steps) > maxLLMContextSliceItems {
		selected = prioritizedRunEvidenceSummarySteps(steps, maxLLMContextSliceItems)
	}
	items := make([]map[string]any, 0, min(len(selected), maxLLMContextSliceItems))
	for _, step := range selected {
		if step == nil {
			continue
		}
		if step.State != common.StepStateCompleted && step.State != common.StepStateFailed {
			continue
		}
		item := map[string]any{
			"stepId": string(step.ID),
			"kind":   step.Kind,
			"state":  string(step.State),
		}
		if nodeID := graphNodeID(step); nodeID != "" {
			item["graphNodeId"] = nodeID
		}
		if rootStepID := graphRootStepID(step); rootStepID != "" && rootStepID != step.ID {
			item["graphRootStepId"] = string(rootStepID)
		}
		if outputRef := stepOutputRefForLLMContext(run, step); outputRef != "" {
			item["outputRef"] = outputRef
		}
		if branch := graphBranch(step); branch != "" {
			item["graphBranch"] = branch
		}
		if output := compactStepOutputForRunEvidenceSummary(step.Output); len(output) > 0 {
			item["output"] = output
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func (o *RuntimeOrchestrator) currentRunProcessEvidenceContext(ctx context.Context, run *runModel.Run, currentStepID common.StepID) ([]map[string]any, map[string]any) {
	if o == nil || o.steps == nil || run == nil || run.ID == "" {
		return nil, nil
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil || len(steps) == 0 {
		return nil, nil
	}
	timeline := processEvidenceTimelineFromRunSteps(run, steps, currentStepID)
	if len(timeline) == 0 {
		return nil, nil
	}
	return timeline, currentProgressDigestFromProcessEvidenceTimeline(timeline)
}

func processEvidenceTimelineFromRunSteps(run *runModel.Run, steps []*runModel.Step, currentStepID common.StepID) []map[string]any {
	if len(steps) == 0 {
		return nil
	}
	ordered := orderedRuntimeContextSteps(steps)
	candidates := make([]*runModel.Step, 0, len(ordered))
	for _, step := range ordered {
		if step == nil || step.ID == "" || step.ID == currentStepID {
			continue
		}
		if step.State != common.StepStateCompleted && step.State != common.StepStateFailed {
			continue
		}
		candidates = append(candidates, step)
	}
	if len(candidates) == 0 {
		return nil
	}
	selected := candidates
	if len(candidates) > maxLLMContextSliceItems {
		selected = prioritizedRunEvidenceSummarySteps(candidates, maxLLMContextSliceItems)
	}
	items := make([]map[string]any, 0, min(len(selected), maxLLMContextSliceItems))
	for idx, step := range selected {
		item := processEvidenceTimelineItem(run, step, idx+1)
		if len(item) > 0 {
			items = append(items, item)
		}
	}
	return items
}

func processEvidenceTimelineItem(run *runModel.Run, step *runModel.Step, order int) map[string]any {
	if step == nil || step.ID == "" {
		return nil
	}
	item := map[string]any{
		"order":      order,
		"sourceRef":  "step://" + string(step.ID),
		"stepId":     string(step.ID),
		"kind":       step.Kind,
		"actionKind": processEvidenceActionKind(step.Kind),
		"state":      string(step.State),
	}
	if stage := processEvidenceStage(step); stage != "" {
		item["stage"] = stage
	}
	if nodeID := graphNodeID(step); nodeID != "" {
		item["graphNodeId"] = nodeID
	}
	if rootStepID := graphRootStepID(step); rootStepID != "" && rootStepID != step.ID {
		item["graphRootStepId"] = string(rootStepID)
	}
	if repairOfStepID := strings.TrimSpace(stringFromMap(step.Input, stepInputKeyRepairOfStepID)); repairOfStepID != "" {
		item[stepInputKeyRepairOfStepID] = repairOfStepID
	}
	if branch := graphBranch(step); branch != "" {
		item["graphBranch"] = branch
	}
	if outputRef := stepOutputRefForLLMContext(run, step); outputRef != "" {
		item["payloadRef"] = outputRef
		item["outputRef"] = outputRef
	}
	if step.State == common.StepStateCompleted && completedShellCommandOutputHasFailureSignal(step) {
		item["state"] = string(common.StepStateFailed)
		item["failureClass"] = failureClassShellCommand
		if signature := runtimeFailureSignature(failureClassShellCommand, step, step.Output, nil); signature != "" {
			item["failureSignature"] = compactLLMContextString(signature)
		}
		if reason := processEvidenceShellFailureReason(step.Output); reason != "" {
			item["failureReason"] = reason
			item["summary"] = reason
			item["outputSummary"] = reason
		}
		item["evidenceValidity"] = "actionable_failure"
	} else if summary := processEvidenceOutputSummaryForStep(step); summary != "" {
		item["summary"] = summary
		item["outputSummary"] = summary
	} else if rejectedLLMDecisionOutputForProgress(step) {
		item["rejectedOutputSummaryOmitted"] = true
		item["omittedContentReason"] = "rejected LLM decision summary is not reusable progress evidence; use failureReason, current evidence, and target snapshots"
	}
	if inputSummary := processEvidenceInputSummary(step.Input); inputSummary != "" {
		item["inputSummary"] = inputSummary
	}
	if step.State == common.StepStateFailed {
		if class := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass)); class != "" {
			item["failureClass"] = class
		}
		if signature := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeySignature)); signature != "" {
			item["failureSignature"] = compactLLMContextString(signature)
		}
		if reason := processEvidenceFailureReason(step.Output); reason != "" {
			item["failureReason"] = reason
		}
		if diagnostic := processEvidenceConcreteFailureDiagnostic(step.Output); diagnostic != "" {
			item["concreteFailureDiagnostic"] = diagnostic
		}
		if validity := processEvidenceValidity(step); validity != "" {
			item["evidenceValidity"] = validity
		}
	}
	if refs := processEvidenceRefs(step); len(refs) > 0 {
		item["evidenceRefs"] = refs
	}
	if output := compactStepOutputForRunEvidenceSummary(step.Output); len(output) > 0 {
		item["output"] = output
	}
	if toolEvidence := compactMCPToolEvidenceSummary(step); len(toolEvidence) > 0 {
		item["toolEvidence"] = toolEvidence
	}
	return item
}

func completedShellCommandOutputHasFailureSignal(step *runModel.Step) bool {
	if step == nil || step.Kind != "shell/command" || step.State != common.StepStateCompleted || len(step.Output) == 0 {
		return false
	}
	text := shellCommandFailureSignalText(step.Output)
	if strings.TrimSpace(text) == "" {
		return false
	}
	for _, marker := range []string{
		"\n--- fail:",
		"\nfail\t",
		"\nfail\n",
		"\nfailed\n",
		"assertionerror",
		"error trace:",
		"\tnot equal:",
		"\texpected:",
		"\tactual  :",
		"test failed",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func shellCommandFailureSignalText(output map[string]any) string {
	if len(output) == 0 {
		return ""
	}
	parts := []string{
		stringFromMap(output, "stderr"),
		stringFromMap(output, "stdout"),
		stringFromMap(output, "stderrPreview"),
		stringFromMap(output, "stdoutPreview"),
		stringFromMap(output, "error"),
		stringFromMap(output, "reason"),
	}
	return "\n" + strings.ToLower(strings.Join(parts, "\n")) + "\n"
}

func processEvidenceShellFailureReason(output map[string]any) string {
	if len(output) == 0 {
		return ""
	}
	return truncateLLMContextString(firstNonEmptyString(
		stringFromMap(output, "stderr"),
		stringFromMap(output, "stdout"),
		stringFromMap(output, "stderrPreview"),
		stringFromMap(output, "stdoutPreview"),
		stringFromMap(output, "error"),
		stringFromMap(output, "reason"),
		"shell command output contains failure markers despite zero exit status",
	))
}

func processEvidenceValidity(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	class := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass))
	stage := strings.TrimSpace(stringFromMap(step.Output, "stage"))
	if rejectedLLMDecisionOutputForProgress(step) {
		return "stale_or_rejected"
	}
	switch class {
	case failureClassProviderTimeout, failureClassProviderUnavailable, failureClassRateLimited, failureClassProviderStreamIncomplete:
		return "diagnostic_only"
	case failureClassLLMDecisionInvalid:
		return "stale_or_rejected"
	case failureClassDoDCoverageMissing:
		return "actionable_gap"
	}
	if stage == "llm_schema_invalid" || strings.Contains(strings.ToLower(stage), "schema") {
		return "stale_or_rejected"
	}
	if class == failureClassShellCommand || class == failureClassPatchApply || class == failureClassRuntime {
		return "actionable_failure"
	}
	return "diagnostic_only"
}

func rejectedLLMDecisionOutputForProgress(step *runModel.Step) bool {
	if step == nil || strings.TrimSpace(step.Kind) != llmWorkspaceExecutionStepKind {
		return false
	}
	if step.State != common.StepStateFailed {
		return false
	}
	if llmDecisionInvalidOutput(step.Output) {
		return true
	}
	stage := strings.ToLower(strings.TrimSpace(stringFromMap(step.Output, "stage")))
	if stage != "llm_execution_graph_plan" && stage != "llm_decision_invalid" && stage != "llm_schema_invalid" {
		return false
	}
	class := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass))
	if class != failureClassRuntime && class != failureClassLLMDecisionInvalid {
		return false
	}
	return strings.TrimSpace(firstNonEmptyString(
		stringFromMap(step.Output, "error"),
		stringFromMap(step.Output, "reason"),
		stringFromMap(step.Output, failureOutputKeySignature),
	)) != ""
}

func processEvidenceActionKind(kind string) string {
	kind = strings.TrimSpace(kind)
	switch {
	case kind == llmWorkspaceExecutionStepKind:
		return "llm_decision"
	case kind == "patch/apply":
		return "artifact_write"
	case strings.Contains(kind, "validation") || strings.Contains(kind, "verify"):
		return "validation"
	case strings.Contains(kind, "tool") || strings.Contains(kind, "command") || strings.Contains(kind, "shell") || strings.Contains(kind, "mcp"):
		return "tool_call"
	case strings.Contains(kind, "read") || strings.Contains(kind, "list"):
		return "resource_read"
	default:
		return "runtime_step"
	}
}

func processEvidenceStage(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	return firstNonEmptyString(
		stringFromMap(step.Input, stepInputKeyRuntimeStage),
		stringFromMap(step.Input, stepInputKeySoulStage),
		stringFromMap(step.Output, "runtimeStage"),
		stringFromMap(step.Output, "stage"),
	)
}

func processEvidenceInputSummary(input map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	parts := []string{}
	for _, key := range []string{
		stepInputKeyRuntimeStage,
		stepInputKeyRuntimeStageMode,
		stepInputKeyGraphNodeID,
		stepInputKeyGraphRootStepID,
		stepOutputKeyGraphBranch,
		stepInputKeyRuntimeRepairFollowUp,
		stepInputKeyProviderRetryFollowUp,
		stepInputKeyAssessmentOnly,
	} {
		if value, ok := input[key]; ok && value != nil {
			parts = append(parts, key+"="+fmt.Sprint(value))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return truncateLLMContextString(strings.Join(parts, "; "))
}

func processEvidenceOutputSummary(output map[string]any) string {
	if len(output) == 0 {
		return ""
	}
	return truncateLLMContextString(firstNonEmptyString(
		stringFromMap(output, "summary"),
		stringFromMap(output, "status"),
		stringFromMap(output, "reason"),
		stringFromMap(output, "error"),
		stringFromMap(output, "stderr"),
		stringFromMap(output, "stdout"),
		stringFromMap(output, failureOutputKeySignature),
	))
}

func processEvidenceOutputSummaryForStep(step *runModel.Step) string {
	if step == nil || rejectedLLMDecisionOutputForProgress(step) {
		return ""
	}
	return processEvidenceOutputSummary(step.Output)
}

func processEvidenceFailureReason(output map[string]any) string {
	if len(output) == 0 {
		return ""
	}
	if reason := processEvidenceConcreteFailureDiagnostic(output); reason != "" {
		return reason
	}
	return truncateLLMContextString(firstNonEmptyString(
		stringFromMap(output, "error"),
		stringFromMap(output, "reason"),
		stringFromMap(output, "stderr"),
		stringFromMap(output, "stdout"),
		stringFromMap(output, "stderrPreview"),
		stringFromMap(output, "stdoutPreview"),
		stringFromMap(output, failureOutputKeySignature),
	))
}

func processEvidenceConcreteFailureDiagnostic(output map[string]any) string {
	if len(output) == 0 {
		return ""
	}
	for _, candidate := range []string{
		stringFromMap(output, "stderr"),
		stringFromMap(output, "stdout"),
		stringFromMap(output, "stderrPreview"),
		stringFromMap(output, "stdoutPreview"),
		stringFromMap(output, "reason"),
		stringFromMap(output, "error"),
	} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if structuredFailureSignalsHaveConcreteTargetEvidence(structuredFailureSignalsFromFailureOutput(map[string]any{
			"stderr": candidate,
		}, nil, "", "")) {
			return truncateLLMContextString(candidate)
		}
	}
	return ""
}

func processEvidenceRefs(step *runModel.Step) []string {
	if step == nil {
		return nil
	}
	refs := []string{string(step.ID)}
	if ref := strings.TrimSpace(stringFromMap(step.Output, "evidenceRef")); ref != "" {
		refs = append(refs, ref)
	}
	refs = append(refs, stringSliceFromValue(step.Output["evidenceRefs"])...)
	return uniqueProcessEvidenceStrings(refs)
}

func uniqueProcessEvidenceStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	ret := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	return ret
}

func currentProgressDigestFromProcessEvidenceTimeline(timeline []map[string]any) map[string]any {
	if len(timeline) == 0 {
		return nil
	}
	digest := map[string]any{}
	existingOutputs := []string{}
	var lastAttempt, lastFailure, lastActionableFailure map[string]any
	for _, item := range timeline {
		if len(item) == 0 {
			continue
		}
		lastAttempt = item
		if strings.TrimSpace(stringFromMap(item, "state")) == string(common.StepStateCompleted) {
			if summary := strings.TrimSpace(stringFromMap(item, "summary")); summary != "" {
				existingOutputs = append(existingOutputs, summary)
			} else if stepID := strings.TrimSpace(stringFromMap(item, "stepId")); stepID != "" {
				existingOutputs = append(existingOutputs, stepID)
			}
			if lastActionableFailureSupersededByCompletedItem(lastActionableFailure, item) {
				lastActionableFailure = nil
			}
			if lastFailureSupersededByCompletedItem(lastFailure, item) {
				lastFailure = nil
			}
		}
		if strings.TrimSpace(stringFromMap(item, "state")) == string(common.StepStateFailed) {
			lastFailure = item
			if processEvidenceItemIsActionableFailure(item) {
				lastActionableFailure = item
			}
		}
	}
	if len(existingOutputs) > maxLLMContextSliceItems {
		existingOutputs = existingOutputs[len(existingOutputs)-maxLLMContextSliceItems:]
	}
	if len(existingOutputs) > 0 {
		digest["existingOutputs"] = existingOutputs
	}
	if len(lastAttempt) > 0 {
		digest["lastAttempt"] = processEvidenceDigestStep(lastAttempt)
	}
	if len(lastActionableFailure) > 0 {
		lastFailure = lastActionableFailure
	}
	if len(lastFailure) > 0 {
		digest["lastObservedFailure"] = processEvidenceDigestStep(lastFailure)
		digest["currentGap"] = firstNonEmptyString(
			stringFromMap(lastFailure, "concreteFailureDiagnostic"),
			stringFromMap(lastFailure, "failureReason"),
			stringFromMap(lastFailure, "summary"),
			stringFromMap(lastFailure, "failureClass"),
		)
		digest["nextRequiredAction"] = "address_last_failure_under_current_stage_contract"
	} else {
		digest["currentGap"] = "none_observed_in_recent_process_evidence"
		digest["nextRequiredAction"] = "continue_current_stage_contract"
	}
	return digest
}

func lastActionableFailureSupersededByCompletedItem(failure map[string]any, completed map[string]any) bool {
	if !processEvidenceItemIsActionableFailure(failure) {
		return false
	}
	return lastFailureSupersededByCompletedItem(failure, completed)
}

func lastFailureSupersededByCompletedItem(failure map[string]any, completed map[string]any) bool {
	if len(failure) == 0 || len(completed) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromMap(completed, "state")) != string(common.StepStateCompleted) {
		return false
	}
	failedStepID := strings.TrimSpace(stringFromMap(failure, "stepId"))
	if failedStepID != "" && strings.TrimSpace(stringFromMap(completed, stepInputKeyRepairOfStepID)) == failedStepID {
		return true
	}
	failedNodeID := strings.TrimSpace(stringFromMap(failure, "graphNodeId"))
	completedNodeID := strings.TrimSpace(stringFromMap(completed, "graphNodeId"))
	if failedNodeID != "" && completedNodeID != "" && failedNodeID == completedNodeID {
		return true
	}
	return false
}

func processEvidenceItemIsActionableFailure(item map[string]any) bool {
	if len(item) == 0 {
		return false
	}
	validity := strings.TrimSpace(stringFromMap(item, "evidenceValidity"))
	if validity == "actionable_failure" || validity == "actionable_gap" {
		return true
	}
	if validity == "stale_or_rejected" || validity == "diagnostic_only" {
		return false
	}
	switch strings.TrimSpace(stringFromMap(item, "failureClass")) {
	case failureClassShellCommand, failureClassPatchApply, failureClassRuntime:
		return true
	default:
		return false
	}
}

func processEvidenceDigestStep(item map[string]any) map[string]any {
	if len(item) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{"order", "sourceRef", "stepId", "kind", "actionKind", "state", "stage", "failureClass", "failureSignature", "failureReason", "concreteFailureDiagnostic", "evidenceValidity", "rejectedOutputSummaryOmitted", "omittedContentReason"} {
		if value, ok := item[key]; ok && value != nil {
			ret[key] = value
		}
	}
	if strings.TrimSpace(stringFromMap(item, "evidenceValidity")) != "stale_or_rejected" {
		if value, ok := item["summary"]; ok && value != nil {
			ret["summary"] = value
		}
	}
	return ret
}

func stepOutputRefForLLMContext(run *runModel.Run, step *runModel.Step) string {
	if run == nil || step == nil || run.WorkspaceID == "" || run.ID == "" || step.ID == "" {
		return ""
	}
	return runtimeRawPayloadRef(run.WorkspaceID, run.ID, "steps", string(step.ID), "output.json")
}

func prioritizedRunEvidenceSummarySteps(steps []*runModel.Step, limit int) []*runModel.Step {
	if limit <= 0 || len(steps) == 0 {
		return nil
	}
	type candidate struct {
		step     *runModel.Step
		index    int
		priority int
	}
	candidates := make([]candidate, 0, len(steps))
	for idx, step := range steps {
		if step == nil {
			continue
		}
		if step.State != common.StepStateCompleted && step.State != common.StepStateFailed {
			continue
		}
		candidates = append(candidates, candidate{
			step:     step,
			index:    idx,
			priority: runEvidenceSummaryStepPriority(step),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority > candidates[j].priority
		}
		return candidates[i].index > candidates[j].index
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].index < candidates[j].index
	})
	ret := make([]*runModel.Step, 0, len(candidates))
	for _, candidate := range candidates {
		ret = append(ret, candidate.step)
	}
	return ret
}

func runEvidenceSummaryStepPriority(step *runModel.Step) int {
	if step == nil {
		return 0
	}
	priority := 1
	if step.State == common.StepStateFailed {
		priority += 10
	}
	kind := strings.TrimSpace(step.Kind)
	if !strings.HasPrefix(kind, "llm/") {
		priority += 20
	}
	if completionEvidenceStepFallbackUsable(step) {
		priority += 30
	}
	switch kind {
	case "patch/apply", "shell/command", "mcp/tool_call":
		priority += 20
	}
	return priority
}

func compactStepOutputForRunEvidenceSummary(output map[string]any) map[string]any {
	ret := compactStepOutputForLLMContext(output)
	if len(ret) == 0 {
		return nil
	}
	delete(ret, stepOutputKeyExecutionGraph)
	delete(ret, stepOutputKeyNextInput)
	if strings.TrimSpace(stringFromMap(ret, "operation")) == "mcp/tool_call" {
		delete(ret, "result")
		delete(ret, "streamEvents")
	}
	return ret
}

func compactMCPToolEvidenceSummary(step *runModel.Step) map[string]any {
	if step == nil || strings.TrimSpace(step.Kind) != "mcp/tool_call" || len(step.Output) == 0 {
		return nil
	}
	output := step.Output
	ret := map[string]any{
		"operation": firstNonEmptyString(stringFromMap(output, "operation"), "mcp/tool_call"),
	}
	for _, key := range []string{
		"serverId",
		"tool",
		"status",
		"stage",
		"durationMillis",
		"statusCode",
		"error",
		failureOutputKeyClass,
		failureOutputKeySignature,
	} {
		if value, ok := output[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	if stderr := strings.TrimSpace(stringFromMap(output, "stderr")); stderr != "" {
		ret["stderrPreview"] = truncateLLMContextString(stderr)
	}
	if result := compactMCPToolResultSummary(output["result"]); len(result) > 0 {
		ret["resultSummary"] = result
	}
	if events := mapSliceFromValue(output["streamEvents"]); len(events) > 0 {
		ret["streamEventCount"] = len(events)
	}
	if input := mapFromValue(output["input"]); len(input) > 0 {
		ret["inputSummary"] = compactMCPToolInputSummary(input)
	}
	return ret
}

func compactMCPToolInputSummary(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{"path", "uri", "root_path", "lang_id", "line", "col", "symbol", "query"} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	if len(ret) == 0 {
		ret["fieldCount"] = len(input)
	}
	return ret
}

func compactMCPToolResultSummary(value any) map[string]any {
	switch typed := value.(type) {
	case nil:
		return nil
	case map[string]any:
		return compactMCPToolResultMapSummary(typed)
	case []map[string]any:
		return map[string]any{"itemCount": len(typed), "items": compactMCPToolResultItems(typed)}
	case []any:
		return map[string]any{"itemCount": len(typed), "items": compactMCPToolResultAnyItems(typed)}
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		return map[string]any{"textPreview": compactLLMContextString(text)}
	default:
		return map[string]any{"value": compactLLMContextValue(value, 1)}
	}
}

func compactMCPToolResultMapSummary(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	ret := map[string]any{}
	for _, key := range []string{"status", "summary", "message", "truncated", "total", "limit", "severity", "source"} {
		if value, ok := input[key]; ok && value != nil {
			ret[key] = compactLLMContextValue(value, 1)
		}
	}
	for _, key := range []string{"diagnostics", "items", "references", "locations", "results"} {
		if items := compactMCPToolResultItems(mapSliceFromValue(input[key])); len(items) > 0 {
			ret[key] = items
			ret[key+"Count"] = len(mapSliceFromValue(input[key]))
		}
	}
	for _, key := range []string{"content", "structuredContent"} {
		if nested := mapFromValue(input[key]); len(nested) > 0 {
			ret[key] = compactMCPToolResultMapSummary(nested)
		}
	}
	if len(ret) == 0 {
		ret["fields"] = sortedMapKeys(input, 12)
	}
	return ret
}

func compactMCPToolResultItems(items []map[string]any) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	limit := min(len(items), 8)
	ret := make([]map[string]any, 0, limit)
	for idx := 0; idx < limit; idx++ {
		item := items[idx]
		next := map[string]any{}
		for _, key := range []string{"path", "uri", "file", "line", "col", "range", "severity", "source", "code", "message", "name", "kind", "truncated"} {
			if value, ok := item[key]; ok && value != nil {
				next[key] = compactLLMContextValue(value, 1)
			}
		}
		if len(next) == 0 {
			next["fields"] = sortedMapKeys(item, 8)
		}
		ret = append(ret, next)
	}
	return ret
}

func compactMCPToolResultAnyItems(items []any) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	limit := min(len(items), 8)
	ret := make([]map[string]any, 0, limit)
	for idx := 0; idx < limit; idx++ {
		if item := mapFromValue(items[idx]); len(item) > 0 {
			ret = append(ret, compactMCPToolResultItems([]map[string]any{item})[0])
			continue
		}
		ret = append(ret, map[string]any{"value": compactLLMContextValue(items[idx], 1)})
	}
	return ret
}

func sortedMapKeys(input map[string]any, limit int) []string {
	if len(input) == 0 || limit <= 0 {
		return nil
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		key = strings.TrimSpace(key)
		if key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) > limit {
		keys = keys[:limit]
	}
	return keys
}

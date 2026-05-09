package orchestrator

import (
	"context"
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
	"github.com/muidea/workorch/internal/pkg/repository"
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
	ret := make([]stepDispatchSpec, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
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
		ret = append(ret, stepDispatchSpec{
			ID:    graphStepIDForRootFromSteps(steps, run.ID, rootStepID, node.ID),
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
	last := latestCompletedGraphStep(steps)
	if last == nil {
		return common.ErrConflict
	}
	if graph, ok := runtimeGraphSpecFromStep(last); ok {
		if nextSpecs := o.readyGraphStepSpecsFromSteps(ctx, currentRun, last, graph, steps); len(nextSpecs) > 0 {
			return o.dispatchGraphStepBatchWithWorkspace(ctx, currentRun, nextSpecs, workspace)
		}
	}
	if shouldDispatchGraphFollowUp(currentRun, last, last.Output) {
		return o.dispatchStepWithWorkspace(ctx, currentRun, graphFollowUpStepID(currentRun.ID, last), initialStepKind(currentRun), o.graphFollowUpStepInputFromSteps(currentRun, last, steps), workspace)
	}
	if shouldDispatchLLMCompletionFollowUp(currentRun, last, last.Output) {
		return o.dispatchStepWithWorkspace(ctx, currentRun, llmCompletionFollowUpStepID(currentRun.ID, last), initialStepKind(currentRun), llmCompletionFollowUpStepInputFromSteps(currentRun, last, steps), workspace)
	}
	if shouldDispatchNoActionableContinuationFollowUp(currentRun, last, last.Output) {
		return o.dispatchStepWithWorkspace(ctx, currentRun, noActionableContinuationFollowUpStepID(currentRun.ID, last), initialStepKind(currentRun), noActionableContinuationFollowUpStepInputFromSteps(currentRun, last, steps), workspace)
	}
	if completionState := llmDecisionCompletionStateFromOutput(last.Output); completionState != "" && completionState != "completed" {
		last.State = common.StepStateFailed
		last.Output = runtimeFailureOutput(last.Output, fmt.Errorf("%w: completionState %s has no actionable continuation", common.ErrInvalidArgument, completionState))
		_ = o.steps.Update(ctx, last)
		return o.FailRun(ctx, currentRun.ID, fmt.Sprintf("%s: completionState %s has no actionable continuation", common.ErrInvalidArgument, completionState))
	}
	assessment, summary, err := validatedCompletionDoDAssessment(currentRun, last.Output)
	if err != nil {
		_ = o.recordRejectedCompletionDoDAssessment(ctx, currentRun, last, last.Output)
		last.State = common.StepStateFailed
		last.Output = runtimeFailureOutput(last.Output, err)
		_ = o.steps.Update(ctx, last)
		return o.FailRun(ctx, currentRun.ID, err.Error())
	}
	if err := o.recordCompletionEvidenceFromValidatedDoDAssessment(ctx, currentRun, last, assessment, summary, workspace); err != nil {
		return err
	}
	return o.validateAndCompleteRunWithWorkspace(ctx, currentRun, last.ID, workspace)
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
	if graphNodeID(step) == "" {
		if _, ok := runtimeGraphSpecFromValue(step.Output[stepOutputKeyExecutionGraph]); ok {
			return step.ID
		}
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
	renderedNodeInput, _ := renderRuntimeTemplateValue(node.Input, graphRuntimeTemplateContext(steps, rootStepID)).(map[string]any)
	for key, value := range renderedNodeInput {
		input[key] = value
	}
	input[stepInputKeySoulStage] = soulStageExecution
	input[stepOutputKeyExecutionGraph] = graph
	input[stepInputKeyGraphNodeID] = node.ID
	input[stepInputKeyGraphRootStepID] = string(graphDispatchRootStepID(last))
	input[stepInputKeyGraphPreviousNodeID] = graphNodeID(last)
	input[stepInputKeyGraphCompletedNodes] = completedGraphNodeList(completed)
	if branch != "" {
		input[stepOutputKeyGraphBranch] = branch
	}
	return input
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

func graphStepID(runID common.RunID, nodeID string) common.StepID {
	return common.StepID(fmt.Sprintf("%s/block/%s", runID, nodeID))
}

func graphStepIDForRoot(ctx context.Context, steps repository.StepRepository, runID common.RunID, rootStepID common.StepID, nodeID string) common.StepID {
	base := graphStepID(runID, nodeID)
	if steps == nil || rootStepID == "" {
		return base
	}
	existing, err := steps.Get(ctx, base)
	if errors.Is(err, common.ErrNotFound) || err != nil || existing == nil || graphRootStepID(existing) == rootStepID {
		return base
	}
	rootSuffix := runtimeChildID(rootStepID, nodeID)
	if rootSuffix == "" {
		return base
	}
	return common.StepID(fmt.Sprintf("%s/block/%s", runID, rootSuffix))
}

func graphStepIDForRootFromSteps(steps []*runModel.Step, runID common.RunID, rootStepID common.StepID, nodeID string) common.StepID {
	base := graphStepID(runID, nodeID)
	if rootStepID == "" {
		return base
	}
	for _, step := range steps {
		if step == nil || step.ID != base {
			continue
		}
		if graphRootStepID(step) == rootStepID {
			return base
		}
		rootSuffix := runtimeChildID(rootStepID, nodeID)
		if rootSuffix == "" {
			return base
		}
		return common.StepID(fmt.Sprintf("%s/block/%s", runID, rootSuffix))
	}
	return base
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

func latestFailedRepairableGraphStep(steps []*runModel.Step) *runModel.Step {
	for idx := len(steps) - 1; idx >= 0; idx-- {
		step := steps[idx]
		if step != nil && step.State == common.StepStateFailed && graphNodeID(step) != "" {
			return step
		}
	}
	return nil
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
	if doDAssessmentFromOutput(output) != nil {
		return false
	}
	completionState := llmDecisionCompletionStateFromOutput(output)
	return completionState != "" && completionState != "completed"
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
	stepID := runtimeChildID(step.ID, "completion")
	if stepID == "" {
		stepID = "completion"
	}
	return common.StepID(fmt.Sprintf("%s/zz-llm-after-step-%s", runID, stepID))
}

func llmCompletionFollowUpStepInput(run *runModel.Run, step *runModel.Step) map[string]any {
	input := nextStepInput(run, step.ID)
	input[stepInputKeySoulStage] = soulStageValidation
	input["completionFollowUp"] = true
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
	return enrichRunEvidenceSummaryInput(input, runEvidenceSummaryFromSteps(steps))
}

func noActionableContinuationFollowUpStepID(runID common.RunID, step *runModel.Step) common.StepID {
	stepID := runtimeChildID(step.ID, "continuation")
	if stepID == "" {
		stepID = "continuation"
	}
	return common.StepID(fmt.Sprintf("%s/zz-llm-after-no-actionable-continuation-%s", runID, stepID))
}

func noActionableContinuationFollowUpStepInput(run *runModel.Run, step *runModel.Step) map[string]any {
	input := llmCompletionFollowUpStepInput(run, step)
	input[stepInputKeyNoActionableFollowUp] = true
	input["completionFollowUp"] = false
	input["continuationFollowUp"] = true
	if completionState := llmDecisionCompletionStateFromOutput(step.Output); completionState != "" {
		input["previousCompletionState"] = completionState
		input["continuationFailureReason"] = fmt.Sprintf("completionState %s has no actionable continuation", completionState)
	}
	return input
}

func noActionableContinuationFollowUpStepInputFromSteps(run *runModel.Run, step *runModel.Step, steps []*runModel.Step) map[string]any {
	input := noActionableContinuationFollowUpStepInput(run, step)
	return enrichRunEvidenceSummaryInput(input, runEvidenceSummaryFromSteps(steps))
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
	return enrichRunEvidenceSummaryInput(input, runEvidenceSummaryFromSteps(steps))
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
	return enrichRunEvidenceSummaryInput(input, runEvidenceSummaryFromSteps(steps))
}

func graphFollowUpStepID(runID common.RunID, step *runModel.Step) common.StepID {
	nodeID := runtimeChildID(common.StepID(graphNodeID(step)), "")
	if nodeID == "" {
		nodeID = runtimeChildID(step.ID, "graph")
	}
	if rootStepID := graphRootStepID(step); rootStepID != "" {
		nodeID = runtimeChildID(rootStepID, nodeID)
	}
	return common.StepID(fmt.Sprintf("%s/zz-llm-after-graph-%s", runID, nodeID))
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
	evidence := o.completedGraphEvidenceFromSteps(step, steps)
	input = enrichGraphFollowUpInputWithEvidence(input, evidence)
	return enrichRunEvidenceSummaryInput(input, runEvidenceSummaryFromSteps(steps))
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

func graphFollowUpStepInput(run *runModel.Run, step *runModel.Step) map[string]any {
	input := nextStepInput(run, step.ID)
	input[stepInputKeySoulStage] = soulStageValidation
	input["graphFollowUp"] = true
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
	if o == nil || run == nil || anchor == nil {
		return nil
	}
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil
	}
	return o.completedGraphEvidenceFromSteps(anchor, steps)
}

func (o *RuntimeOrchestrator) completedGraphEvidenceFromSteps(anchor *runModel.Step, steps []*runModel.Step) []map[string]any {
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
			"nodeId": node.ID,
			"stepId": string(step.ID),
			"kind":   step.Kind,
			"output": compactStepOutputForLLMContext(step.Output),
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
	if len(steps) == 0 {
		return nil
	}
	items := make([]map[string]any, 0, min(len(steps), maxLLMContextSliceItems))
	start := 0
	if len(steps) > maxLLMContextSliceItems {
		start = len(steps) - maxLLMContextSliceItems
	}
	for _, step := range steps[start:] {
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

func compactStepOutputForRunEvidenceSummary(output map[string]any) map[string]any {
	ret := compactStepOutputForLLMContext(output)
	if len(ret) == 0 {
		return nil
	}
	delete(ret, stepOutputKeyExecutionGraph)
	delete(ret, stepOutputKeyNextInput)
	return ret
}

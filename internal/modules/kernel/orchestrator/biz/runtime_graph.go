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

	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type graphSubsystem struct {
	steps    repository.StepRepository
	evidence RuntimeEvidenceRepositories
}

func newGraphSubsystem(steps repository.StepRepository, evidence RuntimeEvidenceRepositories) *graphSubsystem {
	return &graphSubsystem{
		steps:    steps,
		evidence: evidence,
	}
}

func (g *graphSubsystem) hasGraphContinuation(ctx context.Context, run *runModel.Run, step *runModel.Step) bool {
	return len(g.readyGraphStepSpecs(ctx, run, step)) > 0
}

func (g *graphSubsystem) nextGraphStepSpec(
	ctx context.Context,
	run *runModel.Run,
	last *runModel.Step,
) (common.StepID, string, map[string]any, bool) {
	specs := g.readyGraphStepSpecs(ctx, run, last)
	if len(specs) == 0 {
		return "", "", nil, false
	}
	return specs[0].ID, specs[0].Kind, specs[0].Input, true
}

func (g *graphSubsystem) readyGraphStepSpecs(
	ctx context.Context,
	run *runModel.Run,
	last *runModel.Step,
) []stepDispatchSpec {
	specs, _ := g.readyGraphStepSpecsWithSteps(ctx, run, last)
	return specs
}

func (g *graphSubsystem) readyGraphStepSpecsWithSteps(
	ctx context.Context,
	run *runModel.Run,
	last *runModel.Step,
) ([]stepDispatchSpec, []*runModel.Step) {
	if g == nil || run == nil || last == nil {
		return nil, nil
	}
	if !graphSourceStepAdmitsSideEffects(last) {
		return nil, nil
	}
	graph, ok := runtimeGraphSpecFromStep(last)
	if !ok || len(graph.Nodes) == 0 {
		return nil, nil
	}
	if g.steps == nil {
		return nil, nil
	}
	steps, err := g.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil, nil
	}
	return g.readyGraphStepSpecsFromSteps(ctx, run, last, graph, steps), steps
}

func graphSourceStepAdmitsSideEffects(step *runModel.Step) bool {
	if step == nil || step.State != common.StepStateCompleted {
		return false
	}
	if strings.TrimSpace(stringFromMap(step.Output, outputKeyContractViolationType)) != "" {
		return false
	}
	if len(mapFromValue(step.Output[stepOutputKeyContractViolation])) > 0 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(stringFromMap(step.Output, "status"))) {
	case "failed", "failure", "rejected":
		return false
	default:
		return true
	}
}

func (g *graphSubsystem) hasActiveGraphWork(ctx context.Context, run *runModel.Run, last *runModel.Step) bool {
	if g == nil || g.steps == nil || run == nil || last == nil || graphNodeID(last) == "" {
		return false
	}
	rootStepID := graphRootStepID(last)
	if rootStepID == "" {
		return false
	}
	steps, err := g.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return false
	}
	for _, step := range steps {
		if step == nil || step.ID == last.ID || !sameGraphRoot(step, rootStepID) || graphNodeID(step) == "" {
			continue
		}
		switch step.State {
		case common.StepStatePending, common.StepStateDispatched:
			return true
		}
	}
	return false
}

func (g *graphSubsystem) readyGraphStepSpecsFromSteps(
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
		stepID := graphScopedStepID(run.ID, graphSeq, idx+1, node.ID)
		input = normalizeGraphStateChangingDispatchAuthority(run, stepID, input, node)
		input = normalizeGraphValidationDispatchAuthority(run, stepID, input, node)
		ret = append(ret, stepDispatchSpec{
			ID:    stepID,
			Kind:  node.Kind,
			Input: input,
		})
	}
	return ret
}

func (o *RuntimeOrchestrator) dispatchGraphStepBatchWithWorkspace(ctx context.Context, run *runModel.Run, specs []stepDispatchSpec, workspace *workspaceModel.Workspace) error {
	if len(specs) == 0 {
		return common.ErrConflict
	}
	specs = dedupeConcurrentLLMGraphDispatchSpecs(specs)
	specs = selectDispatchSpecsForRunLane(specs)
	if len(specs) == 0 {
		return common.ErrConflict
	}
	batchRootStepID := graphBatchRootStepID(specs)
	if o.eventDrivenDispatch {
		workspaceID := workspaceIDForDispatch(run, workspace)
		for _, spec := range specs {
			spec.Input = cloneStepPayload(spec.Input)
			spec.Input = graphBatchInputWithRoot(spec.Input, batchRootStepID)
			var err error
			spec.Input, err = finalizeCurrentAuthorityDispatchInput(spec.Input)
			if err != nil {
				return err
			}
			if err := o.requestDispatchStep(ctx, run.ID, workspaceID, spec); err != nil {
				if errors.Is(err, common.ErrConflict) && graphDispatchConflictHasPersistedStep(ctx, o.steps, run.ID, spec.ID) {
					continue
				}
				return err
			}
		}
		return nil
	}
	var firstErr error
	if len(specs) == 1 {
		spec := specs[0]
		spec.Input = cloneStepPayload(spec.Input)
		if spec.Input == nil {
			spec.Input = map[string]any{}
		}
		spec.Input = graphBatchInputWithRoot(spec.Input, batchRootStepID)
		var err error
		spec.Input, err = finalizeCurrentAuthorityDispatchInput(spec.Input)
		if err != nil {
			return err
		}
		specs[0].Input = spec.Input
		if err := o.dispatchStepWithWorkspace(ctx, run, spec.ID, spec.Kind, spec.Input, workspace); err != nil && !errors.Is(err, common.ErrConflict) {
			firstErr = err
		}
	} else {
		errCh := make(chan error, len(specs))
		var wg sync.WaitGroup
		for idx, spec := range specs {
			spec := spec
			spec.Input = cloneStepPayload(spec.Input)
			if spec.Input == nil {
				spec.Input = map[string]any{}
			}
			spec.Input = graphBatchInputWithRoot(spec.Input, batchRootStepID)
			var err error
			spec.Input, err = finalizeCurrentAuthorityDispatchInput(spec.Input)
			if err != nil {
				errCh <- err
				continue
			}
			specs[idx].Input = spec.Input
			runCopy := *run
			wg.Add(1)
			go func() {
				defer wg.Done()
				errCh <- o.dispatchStepWithWorkspace(ctx, &runCopy, spec.ID, spec.Kind, spec.Input, workspace)
			}()
		}
		wg.Wait()
		close(errCh)
		for err := range errCh {
			if err != nil && !errors.Is(err, common.ErrConflict) {
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}

	currentRun, err := o.runtimeEventRun(common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID), run.ID)
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
	return o.continueGraphBatchAfterCompletedStep(ctx, currentRun, last, steps, workspace)
}

func graphDispatchConflictHasPersistedStep(ctx context.Context, steps repository.StepRepository, runID common.RunID, stepID common.StepID) bool {
	if steps == nil || runID == "" || stepID == "" {
		return false
	}
	step, err := steps.Get(ctx, stepID)
	return err == nil && step != nil && step.RunID == runID
}

func (o *RuntimeOrchestrator) continueGraphBatchAfterCompletedStep(
	ctx context.Context,
	currentRun *runModel.Run,
	last *runModel.Step,
	steps []*runModel.Step,
	workspace *workspaceModel.Workspace,
) error {
	if currentRun == nil || last == nil {
		return common.ErrInvalidArgument
	}
	if anchor, terminal := o.graphManager().graphTerminalContinuationAnchor(ctx, currentRun, last, steps); terminal && anchor != nil {
		last = anchor
	}
	if graph, ok := runtimeGraphSpecForContinuation(steps, last); ok {
		if nextSpecs := o.graphManager().readyGraphStepSpecsFromSteps(ctx, currentRun, last, graph, steps); len(nextSpecs) > 0 {
			if input, ok := o.graphManager().concreteEvidenceAcquisitionFollowUpInputFromSteps(currentRun, last, steps); ok {
				return o.RequestUniqueDispatchStep(ctx, currentRun, graphFollowUpStepID(currentRun.ID, last), initialStepKind(currentRun), input, workspaceIDForDispatch(currentRun, workspace))
			}
			if err := o.dispatchGraphStepBatchWithWorkspace(ctx, currentRun, nextSpecs, workspace); err != nil {
				return o.handleGraphBatchCompletionGateFailure(ctx, currentRun, last, last.Output, "runtime graph continuation dispatch failed", err)
			}
			return nil
		}
	}
	if handled, err := o.resumeArtifactContinuationAfterContextGraph(ctx, currentRun, last, last.Output, workspace); handled || err != nil {
		return err
	}
	if shouldDispatchGraphFollowUp(currentRun, last, last.Output) {
		return o.RequestUniqueDispatchStep(ctx, currentRun, graphFollowUpStepID(currentRun.ID, last), initialStepKind(currentRun), o.graphManager().graphFollowUpStepInputFromSteps(currentRun, last, steps), workspaceIDForDispatch(currentRun, workspace))
	}
	if shouldDispatchLLMCompletionFollowUp(currentRun, last, last.Output) {
		return o.RequestUniqueDispatchStep(ctx, currentRun, llmCompletionFollowUpStepID(currentRun.ID, last), initialStepKind(currentRun), llmCompletionFollowUpStepInputFromSteps(currentRun, last, steps), workspaceIDForDispatch(currentRun, workspace))
	}
	if shouldDispatchNoActionableContinuationFollowUp(currentRun, last, last.Output) {
		return o.RequestUniqueDispatchStep(ctx, currentRun, noActionableContinuationFollowUpStepID(currentRun.ID, last), initialStepKind(currentRun), noActionableContinuationFollowUpStepInputFromSteps(currentRun, last, steps), workspaceIDForDispatch(currentRun, workspace))
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
	normalizedOutput, evidenceNormalizeErr := normalizeCompletionGateDoDAssessmentEvidenceRefs(ctx, o.steps, o.evidence, currentRun, last.Output, last)
	if evidenceNormalizeErr != nil {
		return evidenceNormalizeErr
	}
	last.Output = normalizedOutput
	_ = o.steps.Update(ctx, last)
	if err := o.recordCoverageProgressFromCompletionAssessment(ctx, currentRun, last, last.Output); err != nil {
		return o.handleGraphBatchCompletionGateFailure(ctx, currentRun, last, last.Output, "runtime completion gate failed", err)
	}
	assessment, summary, err := validatedCompletionDoDAssessmentForStep(currentRun, last, last.Output)
	if err != nil {
		return o.handleGraphBatchCompletionGateFailure(ctx, currentRun, last, last.Output, "runtime completion gate failed", err)
	}
	if err := validateDoDAssessmentEvidenceQuality(ctx, o.steps, o.evidence, currentRun, assessment); err != nil {
		return o.handleGraphBatchCompletionGateFailure(ctx, currentRun, last, last.Output, "runtime completion gate failed", err)
	}
	if o.coverageLedger().updateFromAssessment(currentRun, last, assessment, ctx) && o.runs != nil {
		if err := o.persistRunProgressUpdate(ctx, currentRun); err != nil {
			return err
		}
	}
	if err := o.resultManager().recordAcceptedSliceResultsFromRun(ctx, currentRun, last); err != nil {
		return err
	}
	if err := o.coverageLedger().validateReadyForAcceptance(currentRun, ctx); err != nil {
		return o.handleGraphBatchCompletionGateFailure(ctx, currentRun, last, last.Output, "runtime completion gate failed", err)
	}
	if err := o.resultManager().recordCompletionEvidenceFromValidatedDoDAssessment(ctx, currentRun, last, assessment, summary, workspace); err != nil {
		return err
	}
	return o.validateAndCompleteRunWithWorkspace(ctx, currentRun, last.ID, workspace)
}

func (g *graphSubsystem) graphTerminalContinuationAnchor(ctx context.Context, run *runModel.Run, last *runModel.Step, steps []*runModel.Step) (*runModel.Step, bool) {
	if g == nil || run == nil || last == nil || len(steps) == 0 {
		return nil, false
	}
	rootStepID := graphRootStepID(last)
	if rootStepID == "" {
		return nil, false
	}
	anchor := latestCompletedGraphBatchStepForRoot(steps, rootStepID)
	if anchor == nil {
		return nil, false
	}
	graph, ok := runtimeGraphSpecForContinuation(steps, anchor)
	if !ok || len(graph.Nodes) == 0 {
		return anchor, false
	}
	if len(g.readyGraphStepSpecsFromSteps(ctx, run, anchor, graph, steps)) > 0 {
		return anchor, false
	}
	if graphHasActiveWorkFromSteps(steps, anchor) {
		return anchor, false
	}
	if !graphRelevantNodesCompleted(graph, steps, graphRootStepID(anchor)) {
		return anchor, false
	}
	return anchor, true
}

func graphBatchRootStepID(specs []stepDispatchSpec) common.StepID {
	for _, spec := range specs {
		root := strings.TrimSpace(stringFromMap(spec.Input, stepInputKeyGraphRootStepID))
		if root != "" {
			return common.StepID(root)
		}
	}
	return ""
}

func graphBatchInputWithRoot(input map[string]any, rootStepID common.StepID) map[string]any {
	if input == nil {
		input = map[string]any{}
	}
	input[stepInputKeyGraphBatch] = true
	if rootStepID != "" {
		input[stepInputKeyGraphBatchRootStepID] = string(rootStepID)
	}
	return input
}

func dedupeConcurrentLLMGraphDispatchSpecs(specs []stepDispatchSpec) []stepDispatchSpec {
	if len(specs) < 2 {
		return specs
	}
	seenLLMGraphScopes := map[string]bool{}
	ret := make([]stepDispatchSpec, 0, len(specs))
	for _, spec := range specs {
		scope := llmGraphDispatchScope(spec.Kind, spec.Input)
		if scope != "" {
			if seenLLMGraphScopes[scope] {
				continue
			}
			seenLLMGraphScopes[scope] = true
		}
		ret = append(ret, spec)
	}
	return ret
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
	finalizeManagedValidationResourcesForStep(step, "failed", "graph_batch_completion_gate_failed")
	attachContractViolationType(step.Output, execErr)
	classification := classifyRuntimeFailure(step, step.Output, summary, execErr)
	attachRuntimeFailureClassification(step.Output, classification)
	attachRuntimeFailureDiagnostics(step, classification, execErr)
	attachContractViolationDiagnostics(run, step, execErr)
	errText := errorString(execErr)
	if o.coverageLedger().updateFromFailedStep(run, step, errText, ctx) && o.runs != nil {
		_ = o.persistRunProgressUpdate(ctx, run)
	}
	attachCoverageWorkItemFailureDiagnostics(run, step)
	if err := o.recordRejectedDiagnosticEvidenceForContractViolation(ctx, run, step); err != nil {
		return err
	}
	_ = o.recordValidationResult(ctx, run, step, common.ValidationResultStateFailed, summary, "", errText)
	_ = o.checkpointManager().recordStepCheckpoint(ctx, run, step, "step_failed")
	_ = o.steps.Update(ctx, step)
	_ = o.eventPublisher().publishContractViolationRecorded(ctx, run, step)
	_ = o.eventPublisher().publishStepState(ctx, run, step, runevents.TopicStepStateChanged)
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

func latestCompletedGraphBatchStepForRoot(steps []*runModel.Step, rootStepID common.StepID) *runModel.Step {
	if rootStepID == "" {
		return nil
	}
	for idx := len(steps) - 1; idx >= 0; idx-- {
		step := steps[idx]
		if step == nil || step.State != common.StepStateCompleted || !graphBatchStep(step) || graphNodeID(step) == "" {
			continue
		}
		if sameGraphRoot(step, rootStepID) {
			return step
		}
	}
	return nil
}

func graphHasActiveWorkFromSteps(steps []*runModel.Step, anchor *runModel.Step) bool {
	if anchor == nil || graphNodeID(anchor) == "" {
		return false
	}
	rootStepID := graphRootStepID(anchor)
	if rootStepID == "" {
		return false
	}
	for _, step := range steps {
		if step == nil || step.ID == anchor.ID || graphNodeID(step) == "" || !sameGraphRoot(step, rootStepID) {
			continue
		}
		switch step.State {
		case common.StepStatePending, common.StepStateDispatched:
			return true
		}
	}
	return false
}

func graphRelevantNodesCompleted(graph runtimemodel.BlockExecutionGraph, steps []*runModel.Step, rootStepID common.StepID) bool {
	if len(graph.Nodes) == 0 || rootStepID == "" {
		return false
	}
	completed := completedGraphNodes(steps, rootStepID)
	branch := selectedGraphBranchForRoot(steps, rootStepID)
	relevant := 0
	for _, node := range graph.Nodes {
		nodeID := strings.TrimSpace(node.ID)
		if nodeID == "" {
			continue
		}
		if node.Branch != "" && node.Branch != branch {
			continue
		}
		relevant++
		if !completed[nodeID] {
			return false
		}
	}
	return relevant > 0
}

func selectedGraphBranchForRoot(steps []*runModel.Step, rootStepID common.StepID) string {
	branch := ""
	for _, step := range steps {
		if step == nil || step.State != common.StepStateCompleted || !sameGraphRoot(step, rootStepID) {
			continue
		}
		if value := strings.TrimSpace(stringFromMap(step.Output, stepOutputKeyGraphBranch)); value != "" {
			branch = value
			continue
		}
		if value := strings.TrimSpace(stringFromMap(step.Input, stepOutputKeyGraphBranch)); value != "" {
			branch = value
		}
	}
	return branch
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
	newCoverageSubsystem().ensure(run)
	refreshRunConvergenceAuthority(run)
	input := nextStepInput(run, last.ID)
	inheritRuntimeRepairLineage(input, last)
	renderedNodeInput, _ := renderRuntimeTemplateValue(node.Input, graphRuntimeTemplateContext(steps, rootStepID)).(map[string]any)
	for key, value := range renderedNodeInput {
		input[key] = value
	}
	input = normalizeDispatchInputAuthorityFocus(input)
	applyGraphScopedValidationScratchLifecycle(input)
	input[stepInputKeySoulStage] = soulStageExecution
	setRuntimeStage(input, runtimeStageExecution)
	inheritRuntimeRepairGraphContract(input, last, node.Kind)
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

func applyGraphScopedValidationScratchLifecycle(input map[string]any) {
	if input == nil || strings.TrimSpace(stringFromMap(input, validationScratchDirKey)) == "" {
		return
	}
	input["validationScratchAutoCleanup"] = false
	input["validationScratchScope"] = "execution_graph"
	input["validationScratchCleanupPolicy"] = "retain_until_run_terminal"
}

func normalizeGraphStateChangingDispatchAuthority(run *runModel.Run, stepID common.StepID, input map[string]any, node runtimemodel.BlockExecutionNode) map[string]any {
	if len(input) == 0 || !graphNodeInputRequiresStateChangingAuthority(input, node) {
		return input
	}
	input["graphLocalDispatchAuthority"] = true
	setRuntimeStage(input, runtimeStageRepair)
	input[stepInputKeySoulStage] = soulStageExecution
	input["repairObjective"] = repairObjectiveStateChangingOutput
	input["stateChangingOutputRequired"] = true
	for _, key := range []string{
		"boundedValidationRequired",
		"evidenceRepairRequired",
		"concreteFailureEvidenceRequired",
		stepInputKeyAssessmentOnly,
		"assessmentOnlyReason",
	} {
		delete(input, key)
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	required := mapFromValue(pkg["requiredOutput"])
	if len(required) == 0 {
		required = stateChangingRepairRequiredOutput()
	}
	if !requiredOutputRequiresStateChanging(required) {
		pkg["requiredOutput"] = required
		required = enforceStateChangingRequiredOutputForConvergence(pkg, input)
	}
	pkg["requiredOutput"] = cloneStepPayload(required)
	pkg["repairObjective"] = repairObjectiveStateChangingOutput
	pkg["stateChangingOutputRequired"] = true
	for _, key := range []string{
		"evidenceAcquisitionRequired",
		"concreteFailureEvidenceRequired",
		"evidenceAcquisitionReason",
		"boundedValidationRequired",
		"outputDecisionContract",
		"exitCriteria",
	} {
		delete(pkg, key)
	}
	input["repairActionPackage"] = pkg
	currentActionSetRequiredOutput(input, required)
	input["convergenceContract"] = map[string]any{
		"state":                       convergenceStateNeedStateChange,
		"runtimeStage":                runtimeStageRepair,
		"runtimeStageMode":            firstNonEmptyString(stringFromMap(input, stepInputKeyRuntimeStageMode), "strict_convergence"),
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
	}
	action := map[string]any{
		"kind":   runtimemodel.NextActionTypeStateChange,
		"reason": "current execution graph node requires state-changing output",
	}
	ensureGraphLocalAuthorityMaps(run, input, action)
	currentActionSetNextAction(input, action)
	input = normalizeDispatchInputAuthorityFocus(input)
	input = attachStagePacketToStepInput(run, stepID, input)
	return attachCurrentActionContractToInput(run, stepID, input)
}

func normalizeGraphValidationDispatchAuthority(run *runModel.Run, stepID common.StepID, input map[string]any, node runtimemodel.BlockExecutionNode) map[string]any {
	if len(input) == 0 || !graphNodeInputRequiresValidationAuthority(input, node) {
		return input
	}
	normalizeGraphValidationEvidenceBinding(input)
	input["graphLocalDispatchAuthority"] = true
	setRuntimeStage(input, runtimeStageVerification)
	input[stepInputKeySoulStage] = soulStageValidation
	input["repairObjective"] = repairObjectiveConcreteValidationEvidence
	input["boundedValidationRequired"] = true
	input["evidenceRepairRequired"] = true
	for _, key := range []string{
		"stateChangingOutputRequired",
		"concreteFailureEvidenceRequired",
		stepInputKeyAssessmentOnly,
		"assessmentOnlyReason",
	} {
		delete(input, key)
	}
	pkg := mapFromValue(input["repairActionPackage"])
	if len(pkg) == 0 {
		pkg = map[string]any{}
	}
	pkg["repairObjective"] = repairObjectiveConcreteValidationEvidence
	pkg["boundedValidationRequired"] = true
	for _, key := range []string{
		"stateChangingOutputRequired",
		"evidenceAcquisitionRequired",
		"concreteFailureEvidenceRequired",
		"evidenceAcquisitionReason",
		"outputDecisionContract",
		"exitCriteria",
	} {
		delete(pkg, key)
	}
	required := concreteValidationEvidenceRequiredOutputWithScratch(input, pkg, validationRequiredOutputSeed(mapFromValue(pkg["requiredOutput"])))
	pkg["requiredOutput"] = cloneStepPayload(required)
	input["repairActionPackage"] = pkg
	currentActionSetRequiredOutput(input, required)
	input["convergenceContract"] = map[string]any{
		"state":            convergenceStateNeedValidation,
		"runtimeStage":     runtimeStageVerification,
		"runtimeStageMode": firstNonEmptyString(stringFromMap(input, stepInputKeyRuntimeStageMode), "strict_convergence"),
		"repairObjective":  repairObjectiveConcreteValidationEvidence,
	}
	action := map[string]any{
		"kind":   runtimemodel.NextActionTypeValidate,
		"reason": "current execution graph node requires concrete validation evidence",
	}
	ensureGraphLocalAuthorityMaps(run, input, action)
	currentActionSetNextAction(input, action)
	input = normalizeDispatchInputAuthorityFocus(input)
	input = attachStagePacketToStepInput(run, stepID, input)
	return attachCurrentActionContractToInput(run, stepID, input)
}

func ensureGraphLocalAuthorityMaps(run *runModel.Run, input map[string]any, action map[string]any) {
	if len(input) == 0 || len(action) == 0 {
		return
	}
	focus := dispatchAuthorityCurrentFocus(input)
	if len(focus) == 0 {
		focus = mapFromValue(input[coverageFocusKey])
	}
	ensure := func(container map[string]any, key string, schema string) {
		if len(container) == 0 {
			return
		}
		source := mapFromValue(container[key])
		if len(source) == 0 {
			source = map[string]any{"schemaVersion": schema}
			if run != nil {
				source["workspaceId"] = string(run.WorkspaceID)
				source["runId"] = string(run.ID)
			}
			if len(focus) > 0 {
				source["currentFocus"] = cloneStepPayload(focus)
			}
			source["nextAction"] = cloneStepPayload(action)
			container[key] = source
		}
	}
	ensure(input, convergenceAuthorityKey, convergenceAuthoritySchema)
	ensure(input, gapBaselineKey, gapBaselineSchema)
	if executionContext := mapFromValue(input["executionContext"]); len(executionContext) > 0 {
		ensure(executionContext, convergenceAuthorityKey, convergenceAuthoritySchema)
		ensure(executionContext, gapBaselineKey, gapBaselineSchema)
		input["executionContext"] = executionContext
	}
}

func graphNodeInputRequiresStateChangingAuthority(input map[string]any, node runtimemodel.BlockExecutionNode) bool {
	if len(input) == 0 {
		return false
	}
	if boolValue(input["stateChanging"]) || boolValue(node.Input["stateChanging"]) {
		return true
	}
	requiresStateChange := strings.TrimSpace(stringFromMap(input, "repairObjective")) == repairObjectiveStateChangingOutput ||
		boolValue(input["stateChangingOutputRequired"]) ||
		requiredOutputRequiresStateChanging(activeRequiredOutputForCurrentAction(input)) ||
		requiredOutputRequiresStateChanging(mapFromValue(input["requiredOutput"])) ||
		requiredOutputRequiresStateChanging(mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"]))
	if !requiresStateChange {
		return false
	}
	switch canonicalRuntimeGraphNodeKind(node.Kind) {
	case llmWorkspaceExecutionStepKind, runtimemodel.RuntimeBlockKindPatchApply:
		return true
	case runtimemodel.StepKindShellCommand, runtimemodel.StepKindMCPToolCall:
		return requiredOutputAcceptsKind(activeRequiredOutputForCurrentAction(input), canonicalRuntimeGraphNodeKind(node.Kind)) &&
			boolValue(input["stateChanging"])
	default:
		kind := canonicalRuntimeGraphNodeKind(node.Kind)
		return stepKindCanProduceStateChangingEvidence(kind) &&
			requiredOutputAcceptsKind(activeRequiredOutputForCurrentAction(input), kind)
	}
}

func graphNodeInputRequiresValidationAuthority(input map[string]any, node runtimemodel.BlockExecutionNode) bool {
	if len(input) == 0 || boolValue(input["stateChanging"]) || boolValue(node.Input["stateChanging"]) {
		return false
	}
	if !runtimeStepKindCanProduceValidationEvidence(node.Kind) {
		return false
	}
	return len(mapFromValue(input["validationEvidence"])) > 0 ||
		requiredOutputValidationOnly(mapFromValue(input["requiredOutput"])) ||
		strings.TrimSpace(stringFromMap(currentActionNextActionFromInput(input), "kind")) == runtimemodel.NextActionTypeValidate ||
		strings.TrimSpace(stringFromMap(mapFromValue(input["convergenceContract"]), "state")) == convergenceStateNeedValidation
}

func normalizeGraphValidationEvidenceBinding(input map[string]any) {
	evidence := mapFromValue(input["validationEvidence"])
	if len(evidence) == 0 {
		return
	}
	focus := dispatchAuthorityCurrentFocus(input)
	required := activeRequiredOutputForCurrentAction(input)
	workItemIDs := compactUniqueStrings(append(
		append(stringSliceFromValue(evidence["coverageWorkItemIds"]), stringSliceFromValue(input["coverageWorkItemIds"])...),
		append(stringSliceFromValue(required["coverageWorkItemIds"]), stringSliceFromValue(focus["coverageWorkItemIds"])...)...,
	))
	if id := strings.TrimSpace(firstNonEmptyString(stringFromMap(evidence, "coverageWorkItemId"), stringFromMap(input, "coverageWorkItemId"))); id != "" {
		workItemIDs = compactUniqueStrings(append(workItemIDs, id))
	}
	targetRefs := compactUniqueStrings(append(
		append(append(stringSliceFromValue(evidence["targetRefs"]), stringSliceFromValue(evidence["structuredTargetRefs"])...), stringSliceFromValue(input["targetRefs"])...),
		append(append(stringSliceFromValue(input["coverageTargets"]), stringSliceFromValue(required["targetRefs"])...), append(stringSliceFromValue(focus["targetRefs"]), workItemIDs...)...)...,
	))
	if ref := strings.TrimSpace(firstNonEmptyString(stringFromMap(evidence, "targetRef"), stringFromMap(input, "targetRef"))); ref != "" {
		targetRefs = compactUniqueStrings(append(targetRefs, ref))
	}
	if len(workItemIDs) > 0 {
		evidence["coverageWorkItemIds"] = workItemIDs
		input["coverageWorkItemIds"] = workItemIDs
	}
	if len(targetRefs) > 0 {
		evidence["targetRefs"] = targetRefs
		evidence["structuredTargetRefs"] = targetRefs
		input["targetRefs"] = targetRefs
	}
	if len(nodeValidationEvidenceObservedClaims(runtimemodel.BlockExecutionNode{Input: input})) == 0 {
		claim := firstNonEmptyString(
			stringFromMap(input, "boundedReason"),
			stringFromMap(input, "verificationScope"),
			stringFromMap(evidence, "observationType"),
			"bounded validation observes the declared coverage focus",
		)
		evidence["observedClaims"] = []string{claim}
	}
	if strings.TrimSpace(stringFromMap(evidence, "mode")) == "controlled_substitute" && !validationEvidenceHasControlledSubstitute(evidence) {
		evidence["substituteFor"] = firstNonEmptyString(strings.TrimSpace(strings.Join(targetRefs, ",")), "declared coverage focus")
	}
	input["validationEvidence"] = evidence
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
	return canonicalRuntimeGraphNodeKind(node.Kind) == initialStepKind(run)
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

func completedGraphBatchStepHasDownstreamStep(steps []*runModel.Step, last *runModel.Step) bool {
	if last == nil || !graphBatchStep(last) {
		return false
	}
	rootStepID := graphRootStepID(last)
	if rootStepID == "" {
		return false
	}
	lastID := string(last.ID)
	for _, step := range steps {
		if step == nil || step.ID == last.ID || !graphBatchStep(step) || graphNodeID(step) == "" || !sameGraphRoot(step, rootStepID) {
			continue
		}
		if string(step.ID) <= lastID {
			continue
		}
		switch step.State {
		case common.StepStatePending, common.StepStateDispatched, common.StepStateCompleted, common.StepStateFailed:
			return true
		}
	}
	return false
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
	return graphFollowUpExpansionCandidate(step, step.Output)
}

func graphFollowUpExpansionCandidate(step *runModel.Step, output map[string]any) bool {
	if step == nil || step.Input == nil || output == nil || !boolValue(step.Input["graphFollowUp"]) {
		return false
	}
	if _, exists := output[stepOutputKeyExecutionGraph]; exists {
		return true
	}
	_, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	return ok
}

func graphFollowUpConcreteValidationExpansionStep(step *runModel.Step) bool {
	if !graphFollowUpExpansionStep(step) {
		return false
	}
	graph, ok := runtimeGraphSpecFromValue(step.Output[stepOutputKeyExecutionGraph])
	return ok && graphIsConcreteValidationEvidenceGraph(graph)
}

func graphFollowUpConcreteValidationExpansionCandidate(step *runModel.Step, output map[string]any) bool {
	if !graphFollowUpExpansionCandidate(step, output) {
		return false
	}
	graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
	return ok && graphIsConcreteValidationEvidenceGraph(graph)
}

func (g *graphSubsystem) graphFollowUpExpansionCount(ctx context.Context, runID common.RunID) int {
	if g == nil || g.steps == nil || runID == "" {
		return 0
	}
	steps, err := g.steps.ListByRun(ctx, runID)
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

func (g *graphSubsystem) graphFollowUpExpansionCountIncludingCandidate(ctx context.Context, runID common.RunID, candidate *runModel.Step, output map[string]any) int {
	count := 0
	if g != nil && g.steps != nil && runID != "" {
		if steps, err := g.steps.ListByRun(ctx, runID); err == nil {
			for _, step := range steps {
				if candidate != nil && step != nil && step.ID == candidate.ID {
					continue
				}
				if graphFollowUpExpansionStep(step) {
					count++
				}
			}
		}
	}
	if graphFollowUpExpansionCandidate(candidate, output) {
		count++
	}
	return count
}

func (g *graphSubsystem) graphFollowUpConcreteValidationExpansionCount(ctx context.Context, runID common.RunID) int {
	if g == nil || g.steps == nil || runID == "" {
		return 0
	}
	steps, err := g.steps.ListByRun(ctx, runID)
	if err != nil {
		return 0
	}
	count := 0
	for _, step := range steps {
		if graphFollowUpConcreteValidationExpansionStep(step) {
			count++
		}
	}
	return count
}

func (g *graphSubsystem) graphFollowUpConcreteValidationExpansionCountIncludingCandidate(ctx context.Context, runID common.RunID, candidate *runModel.Step, output map[string]any) int {
	count := 0
	if g != nil && g.steps != nil && runID != "" {
		if steps, err := g.steps.ListByRun(ctx, runID); err == nil {
			for _, step := range steps {
				if candidate != nil && step != nil && step.ID == candidate.ID {
					continue
				}
				if graphFollowUpConcreteValidationExpansionStep(step) {
					count++
				}
			}
		}
	}
	if graphFollowUpConcreteValidationExpansionCandidate(candidate, output) {
		count++
	}
	return count
}

func hasCompletionFollowUpSideEffect(output map[string]any) bool {
	return outputItemCount(output[stepOutputKeyPatches]) > 0 ||
		outputItemCount(output[stepOutputKeyMemoryWrites]) > 0 ||
		outputItemCount(output[stepOutputKeyMemoryRefs]) > 0 ||
		len(mapFromValue(output["artifactContinuationApply"])) > 0
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
	delete(input, "graphFollowUp")
	delete(input, stepInputKeyGraphPreviousNodeID)
	delete(input, stepInputKeyGraphCompletedNodes)
	delete(input, stepInputKeyGraphRootStepID)
	input[stepInputKeyAssessmentOnly] = true
	input["assessmentOnlyReason"] = "completion_follow_up"
	delete(input, "stateChangingOutputRequired")
	delete(input, "repairObjective")
	delete(input, "repairActionPackage")
	delete(input, "outputMode")
	delete(input, "requestedOutputMode")
	delete(input, "outputModeSelector")
	delete(input, "artifactOutputContract")
	delete(input, "artifactContinuationContext")
	delete(input, "artifactContinuationId")
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
	input = enrichRunEvidenceSummaryInput(input, runEvidenceSummaryFromRunSteps(run, steps))
	return restoreCoverageWorkItemContinuationConvergenceContract(run, input)
}

func graphExpansionLimitAssessmentFollowUpStepID(runID common.RunID, step *runModel.Step) common.StepID {
	stepID := runtimeChildID(common.StepID(runtimeReadableStepLabel(runID, step.ID)), "assessment")
	if stepID == "" {
		stepID = "assessment"
	}
	return common.StepID(fmt.Sprintf("%s/follow-up-graph-limit-%s", runID, stepID))
}

func (g *graphSubsystem) graphExpansionLimitAssessmentFollowUpStepInputFromSteps(ctx context.Context, run *runModel.Run, step *runModel.Step, steps []*runModel.Step, count int) map[string]any {
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
	if g != nil {
		if closure := latestCompletionClosureEvidence(ctx, g.steps, g.evidence, run, step); len(closure) > 0 {
			input["latestClosureEvidence"] = closure
			if superseded := mapSliceFromValue(closure["supersededFailures"]); len(superseded) > 0 {
				input["supersededFailureEvidence"] = superseded
			}
		}
	}
	input["assessmentEvidenceGuidance"] = []string{
		"Return only a completed or failed dodAssessment, or a real governance blocker.",
		"Use concrete step IDs from runEvidenceSummary or latestClosureEvidence as evidenceRefs; do not cite only the run ID or memory refs for satisfied items.",
		"Prefer later successful evidence in the same neutral scope over earlier failed evidence.",
		"If the available evidence is insufficient, mark only the unsupported item failed/skipped and state the exact missing evidence.",
	}
	return restoreCoverageWorkItemContinuationConvergenceContract(run, input)
}

func restoreCoverageWorkItemContinuationConvergenceContract(run *runModel.Run, input map[string]any) map[string]any {
	if len(input) == 0 {
		return input
	}
	context := coverageWorkItemContextFromInput(input)
	if !coverageWorkItemContinuationHasUnresolvedEvidenceGap(context) {
		return input
	}
	applyCoverageWorkItemContext(input, context)
	if graphLimitFollowUpCanUseAssessmentOnlyCoverageClosure(input, context) {
		input[stepInputKeyAssessmentOnly] = true
		input["convergenceContract"] = compactConvergenceContract(ConvergenceContract{
			State:            convergenceStateNeedAssessment,
			RuntimeStage:     runtimeStageVerification,
			RuntimeStageMode: "strict_convergence",
			RepairObjective:  "complete_dod_goal_coverage_assessment",
			TargetEvidence:   convergenceTargetEvidence(input, mapFromValue(input["repairActionPackage"])),
		})
		clearExecutableCoverageContinuationContract(input)
		return input
	}
	if len(mapFromValue(input["repairActionPackage"])) == 0 {
		input["repairActionPackage"] = map[string]any{}
	}
	ensureCoverageRepairTargetSet(input)
	if completionFollowUpCanUseAssessmentOnlyCoverageClosure(input, context) {
		input["convergenceContract"] = compactConvergenceContract(ConvergenceContract{
			State:            convergenceStateNeedAssessment,
			RuntimeStage:     runtimeStageVerification,
			RuntimeStageMode: "strict_convergence",
			RepairObjective:  "complete_dod_goal_coverage_assessment",
			TargetEvidence:   convergenceTargetEvidence(input, mapFromValue(input["repairActionPackage"])),
		})
		return input
	}
	contract := BuildConvergenceContract(run, nil, nil, map[string]any{"input": input}, nil)
	return ApplyConvergenceContract(input, contract)
}

func graphLimitFollowUpCanUseAssessmentOnlyCoverageClosure(input map[string]any, context map[string]any) bool {
	if !boolValue(input[stepInputKeyGraphLimitFollowUp]) || !boolValue(input[stepInputKeyAssessmentOnly]) {
		return false
	}
	return len(coverageWorkItemContinuationFocusItems(context)) > 0
}

func clearExecutableCoverageContinuationContract(input map[string]any) {
	if len(input) == 0 {
		return
	}
	for _, key := range []string{
		"repairObjective",
		"boundedValidationRequired",
		"evidenceRepairRequired",
		"coverageRepairRequired",
		"stateChangingOutputRequired",
		"concreteFailureEvidenceRequired",
		"repairActionPackage",
		"requiredOutput",
		"outputMode",
		"requestedOutputMode",
		"outputModeSelector",
		"artifactOutputContract",
	} {
		delete(input, key)
	}
}

func completionFollowUpCanUseAssessmentOnlyCoverageClosure(input map[string]any, context map[string]any) bool {
	if !boolValue(input["completionFollowUp"]) || !boolValue(input[stepInputKeyAssessmentOnly]) {
		return false
	}
	if boolValue(input["graphFollowUp"]) || boolValue(input[stepInputKeyGraphLimitFollowUp]) || boolValue(input[stepInputKeyNoActionableFollowUp]) {
		return false
	}
	items := coverageWorkItemContinuationFocusItems(context)
	if len(items) == 0 {
		return false
	}
	var ready bool
	for _, item := range items {
		if len(item) == 0 {
			continue
		}
		if coverageWorkItemFocusHasActionableFailure(item) {
			return false
		}
		state := deriveCoverageWorkItemState(item)
		missing := stringSet(state.MissingEvidenceKinds)
		if missing["state_change"] {
			return false
		}
		for kind := range missing {
			if kind != "validation" {
				return false
			}
		}
		if stringSliceContains(state.RequiredEvidenceKinds, "state_change") &&
			coverageWorkItemAcceptedEvidenceCountForKind(item, "state_change", state.RequiredEvidenceKinds) == 0 {
			return false
		}
		if len(missing) == 0 || missing["validation"] {
			ready = true
		}
	}
	return ready
}

func coverageWorkItemContinuationHasUnresolvedEvidenceGap(context map[string]any) bool {
	if len(context) == 0 {
		return false
	}
	for _, item := range coverageWorkItemContinuationFocusItems(context) {
		if len(item) == 0 {
			continue
		}
		status := strings.TrimSpace(stringFromMap(item, "status"))
		if status == "satisfied" {
			continue
		}
		if coverageWorkItemFocusHasActionableFailure(item) {
			return true
		}
		missing := coverageWorkItemMissingEvidenceKindSet(item)
		for _, kind := range []string{"state_change", "validation", "artifact", "assessment", "decision", "approval", "external_dependency"} {
			if missing[kind] {
				return true
			}
		}
	}
	return false
}

func coverageWorkItemContinuationFocusItems(context map[string]any) []map[string]any {
	items := make([]map[string]any, 0)
	if focus := mapFromValue(context["currentFocus"]); len(focus) > 0 {
		items = append(items, focus)
	}
	for _, item := range mapSliceFromValue(context["currentFocusItems"]) {
		if len(item) > 0 {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		for _, item := range mapSliceFromValue(context["unresolvedItems"]) {
			if len(item) > 0 {
				items = append(items, item)
			}
		}
	}
	return items
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
		required := concreteValidationEvidenceRequiredOutputWithScratch(input, next, mapFromValue(next["requiredOutput"]))
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
	if inputHasStateChangingOutputRequirement(input) {
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

func inputHasStateChangingOutputRequirement(input map[string]any) bool {
	if len(input) == 0 || !inputRequiresStateChangingOutput(input) {
		return false
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if boolValue(required["mustIncludeStateChangingNode"]) {
		return true
	}
	if outputModeIsIncrementalArtifact(input) &&
		(boolValue(required["mustReturnArtifactPlan"]) ||
			boolValue(required["mustReturnArtifactChunk"]) ||
			boolValue(required["stateChangingEvidenceDeferredUntilApply"])) {
		return true
	}
	return false
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

func (g *graphSubsystem) noActionableContinuationFollowUpStepInput(ctx context.Context, run *runModel.Run, step *runModel.Step) map[string]any {
	input := noActionableContinuationFollowUpStepInput(run, step)
	if g == nil || g.steps == nil || run == nil {
		return input
	}
	steps, err := g.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return input
	}
	input = enrichRunEvidenceSummaryInput(input, runEvidenceSummaryFromRunSteps(run, steps))
	return restoreCoverageWorkItemContinuationConvergenceContract(run, input)
}

func (g *graphSubsystem) llmCompletionFollowUpStepInput(ctx context.Context, run *runModel.Run, step *runModel.Step) map[string]any {
	input := llmCompletionFollowUpStepInput(run, step)
	if g == nil || g.steps == nil || run == nil {
		return restoreCoverageWorkItemContinuationConvergenceContract(run, input)
	}
	steps, err := g.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return restoreCoverageWorkItemContinuationConvergenceContract(run, input)
	}
	input = enrichRunEvidenceSummaryInput(input, runEvidenceSummaryFromRunSteps(run, steps))
	return restoreCoverageWorkItemContinuationConvergenceContract(run, input)
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

func (g *graphSubsystem) graphFollowUpStepInput(ctx context.Context, run *runModel.Run, step *runModel.Step) map[string]any {
	if g == nil || g.steps == nil || run == nil {
		return restoreCoverageWorkItemContinuationConvergenceContract(run, graphFollowUpStepInput(run, step))
	}
	steps, err := g.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return restoreCoverageWorkItemContinuationConvergenceContract(run, graphFollowUpStepInput(run, step))
	}
	return g.graphFollowUpStepInputFromSteps(run, step, steps)
}

func (g *graphSubsystem) graphFollowUpStepInputFromSteps(run *runModel.Run, step *runModel.Step, steps []*runModel.Step) map[string]any {
	input := graphFollowUpStepInput(run, step)
	evidence := g.completedGraphEvidenceFromSteps(run, step, steps)
	input = enrichGraphFollowUpInputWithEvidence(input, evidence)
	input = restoreRepairContractAfterConcreteEvidenceAcquisition(input, evidenceAcquisitionSourceStep(step, steps), evidence)
	input = enrichRunEvidenceSummaryInput(input, runEvidenceSummaryFromRunSteps(run, steps))
	return restoreCoverageWorkItemContinuationConvergenceContract(run, input)
}

func (g *graphSubsystem) concreteEvidenceAcquisitionFollowUpInputFromSteps(run *runModel.Run, step *runModel.Step, steps []*runModel.Step) (map[string]any, bool) {
	if step == nil {
		return nil, false
	}
	if graph, ok := runtimeGraphSpecFromValue(step.Output[stepOutputKeyExecutionGraph]); ok &&
		inputRequiresStateChangingOutput(step.Input) &&
		graphIsConcreteStateChangingContinuation(graph) {
		return nil, false
	}
	source := evidenceAcquisitionSourceStep(step, steps)
	if !stepRequiresConcreteFailureEvidenceAcquisition(source) {
		return nil, false
	}
	input := g.graphFollowUpStepInputFromSteps(run, step, steps)
	if !boolValue(input["evidenceAcquisitionFollowUp"]) ||
		!boolValue(input["stateChangingOutputRequired"]) ||
		strings.TrimSpace(stringFromMap(input, "repairObjective")) != repairObjectiveStateChangingOutput {
		return nil, false
	}
	return input, true
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
	rootStepID := graphRootStepID(anchor)
	if rootStepID == "" {
		if stepRequiresConcreteFailureEvidenceAcquisition(anchor) {
			return anchor
		}
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
	if stepRequiresConcreteFailureEvidenceAcquisition(anchor) {
		return anchor
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
	emptyWorkspaceSurface := repairInputHasEmptyWorkspaceSurfaceEvidence(map[string]any{
		stepInputKeyGraphEvidence: evidence,
	}) && coverageRepairInputRequiresStateChange(probe)
	if emptyWorkspaceSurface {
		probe["emptyWorkspaceEvidenceEscalatedToImplementation"] = true
	}
	if diagnostics := concreteFailureEvidenceAcquisitionDiagnostics(source.Input, sourcePackage); len(diagnostics) > 0 {
		probe["lastFailureDiagnostics"] = diagnostics
		sourcePackage["lastFailureDiagnostics"] = diagnostics
	}
	evidenceSignals := structuredFailureSignalsFromConcreteFailureEvidence(evidence)
	if len(evidenceSignals) > 0 {
		probe["structuredFailureSignals"] = mergeStructuredFailureSignals(mapFromValue(probe["structuredFailureSignals"]), evidenceSignals)
		sourcePackage["structuredFailureSignals"] = mergeStructuredFailureSignals(mapFromValue(sourcePackage["structuredFailureSignals"]), evidenceSignals)
	}
	actionPackage := repairActionPackageForEvidenceAcquisitionFollowUp(probe, sourcePackage, snapshots)
	if !stateChangingRepairHasActionableImplementationSurface(probe, actionPackage) {
		return input
	}
	input[stepInputKeySoulStage] = soulStageExecution
	setRuntimeStage(input, runtimeStageRepair)
	input[stepInputKeyRuntimeRepairFollowUp] = true
	input["evidenceAcquisitionFollowUp"] = true
	if emptyWorkspaceSurface {
		input["emptyWorkspaceEvidenceEscalatedToImplementation"] = true
	}
	input["stateChangingOutputRequired"] = true
	input["repairObjective"] = repairObjectiveStateChangingOutput
	delete(input, "concreteFailureEvidenceRequired")
	input["repairGuidance"] = appendRepairGuidance(stateChangingRepairGuidance(),
		"Concrete failure evidence has been acquired; continue with a target-scoped state-changing repair or return a real governance blocker if the evidence proves runtime cannot proceed without external input.",
		"Use concreteFailureEvidence and any targetSnapshots as current evidence; do not ask for an operator decision when the target, diagnostic, and output contract are already available.",
	)
	if len(evidence) > 0 {
		input["concreteFailureEvidence"] = compactLLMContextValue(evidence, 1)
	}
	if diagnostics := mapFromValue(actionPackage["lastFailureDiagnostics"]); len(diagnostics) > 0 {
		input["lastFailureDiagnostics"] = diagnostics
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
	if len(mapFromValue(actionPackage["currentExecutionProgress"])) == 0 {
		if progress := currentExecutionProgressForEvidenceAcquisitionFollowUp(input, actionPackage); len(progress) > 0 {
			actionPackage["currentExecutionProgress"] = progress
			input["currentExecutionProgress"] = progress
		}
	}
	input["repairActionPackage"] = actionPackage
	contract := BuildConvergenceContract(nil, nil, source, map[string]any{"input": input}, map[string]any{"sourceRef": string(source.ID)})
	input = ApplyConvergenceContract(input, contract)
	return input
}

func concreteFailureEvidenceAcquisitionDiagnostics(input map[string]any, sourcePackage map[string]any) map[string]any {
	for _, candidate := range []map[string]any{
		mapFromValue(input["lastFailureDiagnostics"]),
		mapFromValue(mapFromValue(input["failedStepOutput"])["lastFailureDiagnostics"]),
		mapFromValue(input["failedStepOutput"]),
		mapFromValue(sourcePackage["lastFailureDiagnostics"]),
		mapFromValue(sourcePackage["currentFailure"]),
	} {
		if runtimeRepairDiagnosticsHasSignal(candidate) {
			return compactFailureStepOutputForRuntimeContext(candidate)
		}
	}
	return nil
}

func stepRequiresConcreteFailureEvidenceAcquisition(step *runModel.Step) bool {
	if step == nil || step.Input == nil {
		return false
	}
	if boolValue(step.Input["stateChangingOutputRequired"]) ||
		strings.TrimSpace(stringFromMap(step.Input, "repairObjective")) == repairObjectiveStateChangingOutput ||
		strings.TrimSpace(stringFromMap(mapFromValue(step.Input["convergenceContract"]), "state")) == convergenceStateNeedStateChange ||
		requiredOutputRequiresStateChanging(mapFromValue(step.Input["requiredOutput"])) {
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
	for _, context := range []map[string]any{
		mapFromValue(input["coverageWorkItemContext"]),
		mapFromValue(pkg["coverageWorkItemContext"]),
		mapFromValue(mapFromValue(input["executionContextPackage"])["coverageWorkItemContext"]),
		mapFromValue(mapFromValue(input["executionContext"])["coverageWorkItemContext"]),
	} {
		if currentFailure := currentFailureFromCoverageWorkItemContext(context); len(currentFailure) > 0 {
			return currentFailure
		}
	}
	for _, source := range []map[string]any{
		input,
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
		add("failureClass", firstNonEmptyString(stringFromMap(source, failureOutputKeyClass), stringFromMap(source, stepInputKeyRepairOfFailureClass)))
		signature := firstNonEmptyString(stringFromMap(source, failureOutputKeySignature), stringFromMap(source, stepInputKeyRepairOfFailureSig))
		add("failureSignature", signature)
		add("targetScope", stringFromMap(source, "failureTargetScope"))
		add("targetRef", repairActionTargetRefFromScope(stringFromMap(source, "failureTargetScope")))
		if strings.TrimSpace(stringFromMap(source, "failureTargetScope")) == "" && strings.TrimSpace(signature) != "" {
			scope := runtimeFailureTargetScope(signature)
			add("targetScope", scope)
			add("targetRef", repairActionTargetRefFromScope(scope))
		}
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

func currentFailureFromCoverageWorkItemContext(context map[string]any) map[string]any {
	focus := mapFromValue(context["currentFocus"])
	if len(focus) == 0 {
		for _, item := range mapSliceFromValue(context["currentFocusItems"]) {
			if coverageWorkItemHasOpenGap(item) {
				focus = item
				break
			}
		}
	}
	if !coverageWorkItemHasOpenGap(focus) {
		return nil
	}
	reason := firstNonEmptyString(
		stringFromMap(focus, "currentGap"),
		coverageWorkItemMissingEvidenceReason(focus),
		"coverage work item requires additional evidence",
	)
	ret := map[string]any{
		"failureClass":        failureClassCoverageActionableGap,
		"failureReason":       truncateLLMContextString(reason),
		"reason":              truncateLLMContextString(reason),
		"source":              "coverageWorkItemContext",
		"evidenceValidity":    "actionable_gap",
		"coverageWorkItemId":  stringFromMap(focus, "id"),
		"sourceRequirementId": stringFromMap(focus, "sourceRequirementId"),
		"sourceScope":         stringFromMap(focus, "sourceScope"),
		"sourcePath":          stringFromMap(focus, "sourcePath"),
		"targetRef":           stringFromMap(focus, "targetText"),
		"targetScope":         stringFromMap(focus, "sourceScope"),
	}
	if refs := stringSliceFromValue(focus["acceptedEvidenceRefs"]); len(refs) > 0 {
		ret["acceptedEvidenceRefs"] = limitedStrings(refs, 8)
	}
	if missing := deriveCoverageWorkItemState(focus).MissingEvidenceKinds; len(missing) > 0 {
		ret["missingEvidenceKinds"] = limitedStrings(missing, 8)
	}
	if required := stringSliceFromValue(focus["requiredEvidenceKinds"]); len(required) > 0 {
		ret["requiredEvidenceKinds"] = limitedStrings(required, 8)
	}
	return compactLLMContextMap(ret, 1)
}

func coverageWorkItemHasOpenGap(item map[string]any) bool {
	return deriveCoverageWorkItemState(item).HasActionableEvidenceGap()
}

func coverageWorkItemMissingEvidenceReason(item map[string]any) string {
	state := deriveCoverageWorkItemState(item)
	missing := state.MissingEvidenceKinds
	if len(missing) == 0 {
		return ""
	}
	return "missing accepted evidence binding for required evidence kind: " + strings.Join(limitedStrings(missing, 8), ", ")
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

func (g *graphSubsystem) completedGraphEvidence(ctx context.Context, run *runModel.Run, anchor *runModel.Step) []map[string]any {
	if g == nil || g.steps == nil || run == nil || anchor == nil {
		return nil
	}
	steps, err := g.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return nil
	}
	return g.completedGraphEvidenceFromSteps(run, anchor, steps)
}

func (g *graphSubsystem) completedGraphEvidenceFromSteps(run *runModel.Run, anchor *runModel.Step, steps []*runModel.Step) []map[string]any {
	graphSource := anchor
	if rootStepID := graphRootStepID(anchor); rootStepID != "" && rootStepID != anchor.ID {
		for _, step := range steps {
			if step != nil && step.ID == rootStepID {
				graphSource = step
				break
			}
		}
	}
	graph, ok := runtimeGraphSpecFromStep(graphSource)
	if !ok || len(graph.Nodes) == 0 {
		return nil
	}
	graphRootID := graphRootStepID(graphSource)
	if graphRootID == "" {
		graphRootID = graphSource.ID
	}
	completedByNode := map[string]*runModel.Step{}
	for _, step := range steps {
		if step == nil || step.State != common.StepStateCompleted {
			continue
		}
		if graphNodeID(step) != "" && graphRootStepID(step) != graphRootID {
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

func (g *graphSubsystem) currentRunProcessEvidenceContext(ctx context.Context, run *runModel.Run, currentStepID common.StepID) ([]map[string]any, map[string]any) {
	if g == nil || g.steps == nil || run == nil || run.ID == "" {
		return nil, nil
	}
	steps, err := g.steps.ListByRunSummary(ctx, run.ID)
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
	ordered := orderedRuntimeEvidenceSteps(steps)
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
	var historicalSummary map[string]any
	if len(candidates) > maxLLMContextSliceItems {
		recentLimit := maxLLMContextSliceItems - 1
		historical := candidates[:len(candidates)-recentLimit]
		selected = candidates[len(candidates)-recentLimit:]
		historicalSummary = compactHistoricalProcessEvidenceTimelineSummary(run, historical)
	}
	items := make([]map[string]any, 0, min(len(selected)+1, maxLLMContextSliceItems))
	if len(historicalSummary) > 0 {
		historicalSummary["order"] = 1
		items = append(items, historicalSummary)
	}
	orderOffset := len(items)
	for idx, step := range selected {
		item := processEvidenceTimelineItem(run, step, idx+1+orderOffset)
		if len(item) > 0 {
			items = append(items, item)
		}
	}
	return items
}

func compactHistoricalProcessEvidenceTimelineSummary(run *runModel.Run, steps []*runModel.Step) map[string]any {
	if len(steps) == 0 {
		return nil
	}
	summary := map[string]any{
		"sourceRef":      processEvidenceHistorySourceRef(run),
		"summaryKind":    "compact_history",
		"actionKind":     "history_summary",
		"historicalOnly": true,
		"itemCount":      len(steps),
	}
	stateCounts := map[string]int{}
	kindCounts := map[string]int{}
	stageCounts := map[string]int{}
	sourceRefs := []string{}
	var firstRef, lastRef string
	var latestFailure map[string]any
	for _, step := range steps {
		if step == nil || step.ID == "" {
			continue
		}
		ref := "step://" + string(step.ID)
		if firstRef == "" {
			firstRef = ref
		}
		lastRef = ref
		sourceRefs = append(sourceRefs, ref)
		if state := strings.TrimSpace(string(step.State)); state != "" {
			stateCounts[state]++
		}
		if kind := strings.TrimSpace(step.Kind); kind != "" {
			kindCounts[kind]++
		}
		if stage := processEvidenceStage(step); stage != "" {
			stageCounts[stage]++
		}
		if step.State == common.StepStateFailed {
			latestFailure = compactHistoricalProcessEvidenceFailure(step)
		}
	}
	if firstRef != "" {
		summary["firstSourceRef"] = firstRef
	}
	if lastRef != "" {
		summary["lastSourceRef"] = lastRef
	}
	if refs := limitedStrings(sourceRefs, maxLLMContextSliceItems); len(refs) > 0 {
		summary["sourceRefs"] = refs
	}
	if len(stateCounts) > 0 {
		summary["stateCounts"] = stateCounts
	}
	if len(kindCounts) > 0 {
		summary["kindCounts"] = kindCounts
	}
	if len(stageCounts) > 0 {
		summary["stageCounts"] = stageCounts
	}
	if len(latestFailure) > 0 {
		summary["latestFailure"] = latestFailure
	}
	summary["summary"] = "Older process evidence is compacted; use sourceRefs for lineage and recent detailed timeline for current action."
	return summary
}

func processEvidenceHistorySourceRef(run *runModel.Run) string {
	if run == nil || run.ID == "" {
		return "history://run/process-evidence"
	}
	return "history://runs/" + string(run.ID) + "/process-evidence"
}

func compactHistoricalProcessEvidenceFailure(step *runModel.Step) map[string]any {
	if step == nil {
		return nil
	}
	ret := map[string]any{
		"sourceRef": "step://" + string(step.ID),
		"stepId":    string(step.ID),
		"state":     string(step.State),
	}
	if step.Kind != "" {
		ret["kind"] = step.Kind
	}
	if class := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeyClass)); class != "" {
		ret["failureClass"] = class
	}
	if signature := strings.TrimSpace(stringFromMap(step.Output, failureOutputKeySignature)); signature != "" {
		ret["failureSignature"] = compactLLMContextString(signature)
	}
	if reason := processEvidenceFailureReason(step.Output); reason != "" {
		ret["failureReason"] = reason
	}
	if targetScope := processEvidenceTargetScope(step); targetScope != "" {
		ret["targetScope"] = targetScope
		ret["failureTargetScope"] = targetScope
	}
	return ret
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
	if artifact := processEvidenceArtifactContinuationSummary(step); len(artifact) > 0 {
		item["artifactContinuation"] = artifact
	}
	if targetRef := processEvidenceTargetRef(step); targetRef != "" {
		item["targetRef"] = targetRef
	}
	if targetScope := processEvidenceTargetScope(step); targetScope != "" {
		item["targetScope"] = targetScope
		item["failureTargetScope"] = targetScope
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
	if validationEvidence := processEvidenceValidationEvidence(step); len(validationEvidence) > 0 {
		item["validationEvidence"] = validationEvidence
	}
	if toolEvidence := compactMCPToolEvidenceSummary(step); len(toolEvidence) > 0 {
		item["toolEvidence"] = toolEvidence
	}
	return item
}

func processEvidenceValidationEvidence(step *runModel.Step) map[string]any {
	if step == nil {
		return nil
	}
	if evidence := mapFromValue(step.Input["validationEvidence"]); len(evidence) > 0 {
		return compactLLMContextMap(evidence, 1)
	}
	if evidence := mapFromValue(step.Output["validationEvidence"]); len(evidence) > 0 {
		return compactLLMContextMap(evidence, 1)
	}
	return nil
}

func processEvidenceTargetRef(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	return firstNonEmptyString(
		stringFromMap(step.Output, "targetRef"),
		stringFromMap(step.Input, "targetRef"),
	)
}

func processEvidenceTargetScope(step *runModel.Step) string {
	if step == nil {
		return ""
	}
	return firstNonEmptyString(
		stringFromMap(step.Output, "failureTargetScope"),
		stringFromMap(step.Output, "targetScope"),
		stringFromMap(step.Input, "failureTargetScope"),
		stringFromMap(step.Input, "targetScope"),
		runtimeFailureTargetScope(stringFromMap(step.Output, failureOutputKeySignature)),
	)
}

func processEvidenceArtifactContinuationSummary(step *runModel.Step) map[string]any {
	if step == nil || len(step.Output) == 0 {
		return nil
	}
	for _, key := range []string{"artifactContinuationApply", "artifactContinuationAssemble", "artifactContinuation"} {
		item := mapFromValue(step.Output[key])
		if len(item) == 0 {
			continue
		}
		ret := map[string]any{}
		for _, field := range []string{
			"id",
			"artifactId",
			"targetRef",
			"state",
			"bytes",
			"rollingHash",
		} {
			if value, ok := item[field]; ok && value != nil {
				ret[field] = compactLLMContextValue(value, 1)
			}
		}
		if refs := stringSliceFromValue(item["sourceChunkRefs"]); len(refs) > 0 {
			ret["sourceChunkRefs"] = limitedStrings(refs, 16)
		}
		if refs := stringSliceFromValue(item["applyEvidenceRefs"]); len(refs) > 0 {
			ret["applyEvidenceRefs"] = limitedStrings(refs, 16)
		}
		if len(ret) > 0 {
			ret["summaryKind"] = key
			return ret
		}
	}
	return nil
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
	case failureClassProviderTimeout, failureClassProviderUnavailable, failureClassRateLimited, failureClassProviderStreamIncomplete, failureClassProviderProtocolInvalid:
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
	kind = canonicalLLMWorkspaceStepKind(kind)
	switch {
	case isLLMWorkspaceStepKind(kind):
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
	var lastAttempt, lastFailure, lastActionableFailure, lastConcreteActionableFailure map[string]any
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
			if lastActionableFailureSupersededByCompletedItem(lastConcreteActionableFailure, item) {
				lastConcreteActionableFailure = nil
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
			if processEvidenceItemIsConcreteActionableFailure(item) {
				lastConcreteActionableFailure = item
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
	if len(lastConcreteActionableFailure) > 0 && processEvidenceItemIsActionableGap(lastActionableFailure) {
		lastFailure = lastConcreteActionableFailure
	} else if len(lastActionableFailure) > 0 {
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
	failedScope := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(failure, "targetScope"),
		stringFromMap(failure, "failureTargetScope"),
	))
	completedScope := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(completed, "targetScope"),
		stringFromMap(completed, "failureTargetScope"),
	))
	if failedScope != "" && completedScope != "" && failedScope == completedScope {
		return true
	}
	return false
}

func processEvidenceItemIsActionableGap(item map[string]any) bool {
	return len(item) > 0 && strings.TrimSpace(stringFromMap(item, "evidenceValidity")) == "actionable_gap"
}

func processEvidenceItemIsConcreteActionableFailure(item map[string]any) bool {
	if len(item) == 0 || processEvidenceItemIsActionableGap(item) {
		return false
	}
	if validity := strings.TrimSpace(stringFromMap(item, "evidenceValidity")); validity != "" && validity != "actionable_failure" {
		return false
	}
	switch strings.TrimSpace(stringFromMap(item, "kind")) {
	case "shell/command", "patch/apply", "mcp/tool_call":
		return true
	}
	switch strings.TrimSpace(stringFromMap(item, "failureClass")) {
	case failureClassShellCommand, failureClassPatchApply, failureClassMCPRuntime:
		return true
	default:
		return false
	}
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
	for _, key := range []string{"order", "sourceRef", "stepId", "kind", "actionKind", "state", "stage", "graphNodeId", "graphRootStepId", stepInputKeyRepairOfStepID, "targetRef", "targetScope", "failureTargetScope", "failureClass", "failureSignature", "failureReason", "concreteFailureDiagnostic", "evidenceValidity", "rejectedOutputSummaryOmitted", "omittedContentReason"} {
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

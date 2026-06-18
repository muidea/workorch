package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceModel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (o *RuntimeOrchestrator) dispatchStepWithWorkspace(
	ctx context.Context,
	run *runModel.Run,
	stepID common.StepID,
	kind string,
	input map[string]any,
	workspace *workspaceModel.Workspace,
) error {
	if run == nil || run.ID == "" {
		return common.ErrInvalidArgument
	}
	if !boolValue(input["allowDispatchAfterRejectedResult"]) {
		if summary, ok := o.resultManager().rejectedRunResultSummary(ctx, run.ID); ok {
			reason := strings.TrimSpace(summary)
			if reason == "" {
				reason = "run has rejected result"
			}
			return fmt.Errorf("%w: run has rejected result: %s", common.ErrConflict, reason)
		}
	}
	kind = canonicalLLMWorkspaceStepKind(kind)
	step, err := o.steps.Get(ctx, stepID)
	if err != nil {
		if err != common.ErrNotFound {
			return err
		}
		step = &runModel.Step{
			ID:    stepID,
			RunID: run.ID,
			Kind:  kind,
			State: common.StepStateDispatched,
			Input: cloneStepPayload(input),
		}
		if err := o.steps.Create(ctx, step); err != nil {
			return err
		}
	} else {
		if step.RunID != run.ID {
			return common.ErrConflict
		}
		switch step.State {
		case common.StepStateCompleted, common.StepStateFailed:
			return fmt.Errorf("%w: runtime step %s is already terminal", common.ErrConflict, step.ID)
		}
		step.State = common.StepStateDispatched
		if kind != "" {
			step.Kind = kind
		}
		if input != nil {
			step.Input = cloneStepPayload(input)
		} else if step.Input == nil {
			step.Input = map[string]any{}
		}
		if err := o.steps.Update(ctx, step); err != nil {
			return err
		}
	}
	if workspace == nil {
		workspace, err = o.workspaces.Get(ctx, run.WorkspaceID)
		if err != nil {
			return err
		}
	} else if workspace.ID != run.WorkspaceID {
		return common.ErrConflict
	}
	o.enrichLLMStepInputContext(ctx, run, step, workspace)
	if err := o.steps.Update(ctx, step); err != nil {
		return err
	}
	if err := o.eventPublisher().publishStepState(ctx, run, step, runevents.TopicStepDispatched); err != nil {
		return err
	}
	toolCall, err := o.recordStepDispatchEvidence(ctx, run, step)
	if err != nil {
		return err
	}
	if blocked, err := o.blockMCPToolCallForOperatorApproval(ctx, run, step, workspace); blocked || err != nil {
		return err
	}
	if handled, err := o.dispatchInternalArtifactContinuationStep(ctx, run, step, toolCall, workspace); handled || err != nil {
		return err
	}

	runtimeInput := cloneStepPayload(step.Input)
	if err := validatePatchStepInputBeforeRuntime(step.Kind, runtimeInput); err != nil {
		output := map[string]any{
			"status": "failed",
			"stage":  "patch_dispatch_invalid",
			"error":  err.Error(),
		}
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime patch dispatch gate failed", err)
	}
	injectRuntimeEvidencePolicies(runtimeInput, run.ExecutionContext)
	request := runtimemodel.StepExecutionRequest{
		WorkspaceID:                   run.WorkspaceID,
		WorkspaceDeliverableDirectory: workspace.DeliverableDirectory,
		WorkspaceInternalDirectory:    workspace.InternalDirectory,
		RunID:                         run.ID,
		StepID:                        step.ID,
		Kind:                          step.Kind,
		Input:                         runtimeInput,
	}
	if requester, ok := o.runtime.(RuntimeExecutionRequester); ok {
		accepted, err := requester.RequestRuntimeStepExecution(ctx, request)
		if err == nil {
			if checkpointErr := o.checkpointManager().recordStepProgressCheckpoint(ctx, run, step, "runtime_execute_accepted", map[string]any{
				"requestId":  accepted.RequestID,
				"queueKey":   accepted.QueueKey,
				"acceptedAt": accepted.AcceptedAt,
			}); checkpointErr != nil {
				return checkpointErr
			}
			return nil
		}
		if !errors.Is(err, common.ErrNotFound) {
			return o.handleRuntimeStepFailure(ctx, run, step, toolCall, map[string]any{
				"status": "failed",
				"stage":  "runtime_request_failed",
				"error":  err.Error(),
			}, "runtime request failed", err)
		}
	}
	if o.eventDrivenDispatch {
		err := fmt.Errorf("%w: runtime execution requester is not available for step kind %s", common.ErrInvalidArgument, step.Kind)
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, map[string]any{
			"status": "failed",
			"stage":  "runtime_request_unavailable",
			"kind":   step.Kind,
			"error":  err.Error(),
		}, "runtime request unavailable", err)
	}
	request.Progress = o.runtimeStepProgressRecorder(ctx, run, step)
	result, err := o.runtime.ExecuteRuntimeStep(ctx, request)
	return o.finalizeRuntimeStepResultWithWorkspace(ctx, run, step, toolCall, workspace, result, err)
}

func (o *RuntimeOrchestrator) finalizeRuntimeStepResultWithWorkspace(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	workspace *workspaceModel.Workspace,
	result runtimemodel.StepExecutionResult,
	execErr error,
) error {
	if run == nil || step == nil || workspace == nil {
		return common.ErrInvalidArgument
	}
	unlock := o.lockRuntimeStepFinalize(step.ID)
	defer unlock()
	if current, err := o.steps.Get(ctx, step.ID); err == nil {
		step = current
		if step.State == common.StepStateCompleted || step.State == common.StepStateFailed {
			return nil
		}
	} else if err != common.ErrNotFound {
		return err
	}
	if execErr != nil {
		if degradedOutput, ok := degradedRuntimeFailureOutput(run, step, result.Output, execErr); ok {
			result.Output = degradedOutput
		} else {
			return o.handleRuntimeStepFailure(ctx, run, step, toolCall, result.Output, "runtime step failed", execErr)
		}
	}
	output := result.Output
	if err := validateRepairPatchOutputTargetRelevance(step, output); err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime patch output contract failed", err)
	}
	if err := validateLLMExecutionOutputContract(run, step, output); err != nil {
		if outputModeIsIncrementalArtifact(step.Input) {
			_ = o.publishArtifactContinuationRejected(ctx, run, step, output, err)
		}
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime execution contract failed", err)
	}
	if err := validateRuntimeStateChangingOutputBeforeSideEffects(run, step, output); err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime state-changing contract gate failed", err)
	}
	if handled, err := o.handleArtifactOutputContinuation(ctx, run, step, toolCall, output, workspace); handled || err != nil {
		return err
	}
	patches := o.patchManager()
	if err := patches.publishPreviewFromOutput(ctx, run, step, toolCall, output); err != nil {
		return err
	}
	language := runDialogLanguage(run)
	patchApproval, err := patches.approvalBlockSpec(ctx, run.ID, language, output)
	if err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime patch approval gate failed", err)
	}
	if patchApproval.Reason != "" {
		applyPatchApprovalBlockOutput(output, patchApproval)
	} else if err := patches.apply(ctx, run, step, toolCall, output, workspace.DeliverableDirectory); err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime patch application failed", err)
	}
	patchRevertApproval, err := patches.revertApprovalBlockSpec(ctx, run.ID, language, output)
	if err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime patch revert approval gate failed", err)
	}
	if patchRevertApproval.Reason != "" {
		applyPatchRevertApprovalBlockOutput(output, patchRevertApproval)
	} else if err := patches.revert(ctx, run, step, toolCall, output, workspace.DeliverableDirectory); err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime patch revert failed", err)
	}
	memoryRefs, err := o.recordMemoryWrites(ctx, run, step, output)
	if err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime memory write failed", err)
	}
	if len(memoryRefs) > 0 {
		output[stepOutputKeyMemoryRefs] = memoryRefs
	}

	step.State = common.StepStateCompleted
	step.Output = output
	if err := o.recordCompletedRuntimeStepEvidence(ctx, run, step, toolCall, output); err != nil {
		return err
	}
	if o.coverageLedger().updateFromCompletedStep(run, step) && o.runs != nil {
		if err := o.runs.Update(ctx, run); err != nil {
			return err
		}
	}
	if err := o.persistCompletedStepState(ctx, run, step); err != nil {
		return err
	}
	if err := o.eventPublisher().publishStepState(ctx, run, step, runevents.TopicStepStateChanged); err != nil {
		return err
	}

	nextStepID, nextKind := nextStepSpec(output)
	if decisionSpec := decisionBlockSpec(output); decisionSpec.Reason != "" {
		decisionSpec.Context = enrichDecisionContextWithReadableWorkspaceResources(ctx, o.workspaces, run, workspace, decisionSpec.Context, decisionSpec.Question, decisionSpec.Reason)
		decisionOutput := decisionOutputFromSpec(decisionSpec)
		convergenceContract := o.convergenceManager().build(run, step, step, map[string]any{"input": step.Input}, map[string]any{"llmOutput": decisionOutput})
		if failedAssessmentDecisionShouldRepair(step, output, decisionSpec) ||
			assessmentOnlyDecisionShouldRepair(step, decisionSpec) ||
			continuableDecisionShouldRepair(step, decisionSpec) ||
			!o.convergenceManager().shouldCreateDecisionTicket(step.Input, decisionOutput, convergenceContract) {
			if assessmentOnlyDecisionShouldRepair(step, decisionSpec) {
				output["assessmentOnlyDecisionConvertedToRepair"] = true
			} else if failedAssessmentDecisionShouldRepair(step, output, decisionSpec) {
				output["actionableFailedAssessmentDecisionConvertedToRepair"] = true
			} else {
				output["continuableDecisionConvertedToRepair"] = true
			}
			output["convertedDecisionGate"] = "convergence_contract"
			output["convergenceContract"] = o.convergenceManager().compact(convergenceContract)
			output["convertedDecision"] = convertedDecisionPayload(decisionSpec)
			output[stepOutputKeyDoDAssessment] = assessmentOnlyDecisionRepairAssessment(run, step, decisionSpec)
			delete(output, stepOutputKeyBlockDecisionReason)
			delete(output, stepOutputKeyBlockDecisionTicketID)
			delete(output, stepOutputKeyBlockDecisionKind)
			delete(output, stepOutputKeyBlockDecisionQuestion)
			delete(output, stepOutputKeyBlockDecisionContext)
			return o.handleRuntimeStepFailure(
				ctx,
				run,
				step,
				toolCall,
				output,
				"runtime completion gate failed",
				fmt.Errorf("%w: decision requests a continuable candidate choice", common.ErrInvalidArgument),
			)
		}
		if nextStepID == "" && !o.graphManager().hasGraphContinuation(ctx, run, step) {
			return o.FailRun(ctx, run.ID, fmt.Sprintf("runtime step %s blocked without next step", step.ID))
		}
		ticketID := decisionSpec.TicketID
		if ticketID == "" {
			ticketID = common.TicketID(runtimeChildID(step.ID, "decision"))
		}
		decisionSpec.TicketID = ticketID
		if err := o.createDecisionTicket(ctx, run, step, decisionSpec); err != nil {
			return err
		}
		if err := o.resultManager().recordRejectedResultForDecisionBlock(ctx, run, step, output, decisionSpec); err != nil {
			return err
		}
		return o.blockOnDecisionAtStep(ctx, run, workspace, ticketID, step.ID)
	}
	if approvalSpec := approvalBlockSpec(output); approvalSpec.Reason != "" {
		if nextStepID == "" && !o.graphManager().hasGraphContinuation(ctx, run, step) {
			return o.FailRun(ctx, run.ID, fmt.Sprintf("runtime step %s blocked without next step", step.ID))
		}
		if approvalSpec.ApprovalID == "" {
			approvalSpec.ApprovalID = common.ApprovalID(runtimeChildID(step.ID, "approval"))
		}
		if err := o.createApproval(ctx, run, step, approvalSpec.ApprovalID, approvalSpec.Reason, approvalSpec.Action, approvalSpec.Context); err != nil {
			return err
		}
		return o.blockOnApprovalAtStep(ctx, run, workspace, approvalSpec.ApprovalID, step.ID)
	}
	if graphBatchStep(step) {
		return nil
	}
	if nextStepID != "" {
		return o.RequestUniqueDispatchStep(ctx, run, nextStepID, nextKind, nextStepInputFromOutput(run, step), workspaceIDForDispatch(run, workspace))
	}
	if handled, err := o.validateGraphExpansionBoundary(ctx, run, step, toolCall, output, workspace); handled || err != nil {
		return err
	}
	graphSpecs, graphSteps := o.graphManager().readyGraphStepSpecsWithSteps(ctx, run, step)
	if len(graphSpecs) > 0 {
		return o.dispatchGraphStepBatchWithWorkspace(ctx, run, graphSpecs, workspace)
	}
	if o.graphManager().hasActiveGraphWork(ctx, run, step) {
		return nil
	}
	if handled, err := o.resumeArtifactContinuationAfterContextGraph(ctx, run, step, output, workspace); handled || err != nil {
		return err
	}
	if shouldDispatchGraphFollowUp(run, step, output) {
		var input map[string]any
		if len(graphSteps) > 0 {
			input = o.graphManager().graphFollowUpStepInputFromSteps(run, step, graphSteps)
		} else {
			input = o.graphManager().graphFollowUpStepInput(ctx, run, step)
		}
		return o.RequestUniqueDispatchStep(ctx, run, graphFollowUpStepID(run.ID, step), initialStepKind(run), input, workspaceIDForDispatch(run, workspace))
	}
	if shouldDispatchLLMCompletionFollowUp(run, step, output) {
		return o.RequestUniqueDispatchStep(ctx, run, llmCompletionFollowUpStepID(run.ID, step), initialStepKind(run), o.graphManager().llmCompletionFollowUpStepInput(ctx, run, step), workspaceIDForDispatch(run, workspace))
	}
	if shouldDispatchNoActionableContinuationFollowUp(run, step, output) {
		return o.RequestUniqueDispatchStep(ctx, run, noActionableContinuationFollowUpStepID(run.ID, step), initialStepKind(run), o.graphManager().noActionableContinuationFollowUpStepInput(ctx, run, step), workspaceIDForDispatch(run, workspace))
	}
	if completionState := llmDecisionCompletionStateFromOutput(output); completionStateRequiresActionableContinuation(output, completionState) {
		return o.handleRuntimeStepFailure(
			ctx,
			run,
			step,
			toolCall,
			output,
			"runtime completion gate failed",
			fmt.Errorf("%w: completionState %s has no actionable continuation", common.ErrInvalidArgument, completionState),
		)
	}
	output = normalizeCompletionGateDoDAssessmentOutput(run, step, output)
	var evidenceNormalizeErr error
	output, evidenceNormalizeErr = normalizeCompletionGateDoDAssessmentEvidenceRefs(ctx, o.steps, o.evidence, run, output)
	if evidenceNormalizeErr != nil {
		return evidenceNormalizeErr
	}
	step.Output = output
	if err := o.steps.Update(ctx, step); err != nil {
		return err
	}
	assessment, summary, err := validatedCompletionDoDAssessment(run, output)
	if err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime completion gate failed", err)
	}
	if err := validateDoDAssessmentEvidenceQuality(ctx, o.steps, o.evidence, run, assessment); err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime completion gate failed", err)
	}
	if o.coverageLedger().updateFromAssessment(run, step, assessment) && o.runs != nil {
		if err := o.runs.Update(ctx, run); err != nil {
			return err
		}
	}
	if changed, bound := o.reconcileCoverageValidationEvidence(ctx, run); changed && o.runs != nil {
		if err := o.runs.Update(ctx, run); err != nil {
			return err
		}
		logCoverageReconcilerBinding(run.ID, bound)
	}
	if err := o.coverageLedger().validateReadyForAcceptance(run); err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime completion gate failed", err)
	}
	if err := o.resultManager().recordCompletionEvidenceFromValidatedDoDAssessment(ctx, run, step, assessment, summary, workspace); err != nil {
		return err
	}
	return o.validateAndCompleteRunWithWorkspace(ctx, run, step.ID, workspace)
}

func (o *RuntimeOrchestrator) lockRuntimeStepFinalize(stepID common.StepID) func() {
	if o == nil || stepID == "" {
		return func() {}
	}
	value, _ := o.finalizeMu.LoadOrStore(stepID, &sync.Mutex{})
	mu, _ := value.(*sync.Mutex)
	if mu == nil {
		return func() {}
	}
	mu.Lock()
	return func() {
		mu.Unlock()
		o.finalizeMu.Delete(stepID)
	}
}

var runtimeStepProgressMinInterval = 5 * time.Second

func (o *RuntimeOrchestrator) runtimeStepProgressRecorder(ctx context.Context, run *runModel.Run, step *runModel.Step) runtimemodel.StepExecutionProgressFunc {
	if o == nil || run == nil || step == nil || o.checkpointManager() == nil {
		return nil
	}
	var mu sync.Mutex
	last := time.Time{}
	lastSignature := ""
	return func(progress runtimemodel.StepExecutionProgress) error {
		event := strings.TrimSpace(progress.Event)
		if event == "" {
			event = "step_progress"
		}
		terminal := event == "runtime_step_done" || event == "runtime_step_failed"
		now := time.Now().UTC()
		if !terminal && !last.IsZero() && now.Sub(last) < runtimeStepProgressMinInterval {
			return nil
		}
		mu.Lock()
		defer mu.Unlock()
		if !terminal && !last.IsZero() && now.Sub(last) < runtimeStepProgressMinInterval {
			return nil
		}
		data := map[string]any{}
		for key, value := range progress.Data {
			if key == "" || value == nil {
				continue
			}
			data[key] = value
		}
		if stage := strings.TrimSpace(progress.Stage); stage != "" {
			data["stage"] = stage
		}
		if summary := strings.TrimSpace(progress.Summary); summary != "" {
			data["summary"] = summary
		}
		signature := runtimeStepProgressSignature(event, data)
		if !terminal && signature != "" && signature == lastSignature {
			return nil
		}
		last = now
		lastSignature = signature
		return o.checkpointManager().recordStepProgressCheckpoint(ctx, run, step, event, data)
	}
}

func runtimeStepProgressSignature(event string, data map[string]any) string {
	payload := map[string]any{
		"event": event,
		"data":  data,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("%s:%v", event, data)
	}
	return string(raw)
}

func (o *RuntimeOrchestrator) validateGraphExpansionBoundary(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	output map[string]any,
	workspace *workspaceModel.Workspace,
) (bool, error) {
	if run == nil || step == nil || output == nil || !requiresDoDAssessmentCompletionGate(run) {
		return false, nil
	}
	if llmExecutionGraphNodeReturnedOnlyGraph(run, step, output) {
		graph, _ := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph])
		err := fmt.Errorf("%w: execution-stage llm graph node returned executionGraph without concrete output", common.ErrInvalidArgument)
		output["executionStageContractViolation"] = true
		output["ignoredExecutionGraph"] = true
		output["ignoredExecutionGraphNodeCount"] = len(graph.Nodes)
		output["ignoredExecutionGraphReason"] = err.Error()
		delete(output, stepOutputKeyExecutionGraph)
		step.Output = output
		_ = o.steps.Update(ctx, step)
		return true, o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime execution contract failed", err)
	}
	if boolValue(step.Input["providerRetryAssessmentOnly"]) || boolValue(step.Input[stepInputKeyAssessmentOnly]) {
		if ignoredGraph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok {
			err := fmt.Errorf("%w: assessment-only step returned executionGraph", common.ErrInvalidArgument)
			if boolValue(step.Input["providerRetryAssessmentOnly"]) {
				err = fmt.Errorf("%w: provider retry assessment-only step returned executionGraph", common.ErrInvalidArgument)
			}
			output["assessmentOnlyContractViolation"] = true
			output["ignoredExecutionGraph"] = true
			output["ignoredExecutionGraphNodeCount"] = len(ignoredGraph.Nodes)
			output["ignoredExecutionGraphReason"] = err.Error()
			delete(output, stepOutputKeyExecutionGraph)
			if doDAssessmentFromOutput(output) != nil {
				step.Output = output
				_ = o.steps.Update(ctx, step)
				return false, nil
			}
			if !boolValue(step.Input["assessmentOnlyContractCorrection"]) {
				output["contractCorrectionStepDispatched"] = true
				step.Output = output
				_ = o.steps.Update(ctx, step)
				input := assessmentOnlyContractCorrectionStepInput(step, output, err)
				return true, o.RequestUniqueDispatchStep(ctx, run, assessmentOnlyContractCorrectionStepID(run.ID, step), initialStepKind(run), input, workspaceIDForDispatch(run, workspace))
			}
			return true, o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime completion gate failed", err)
		}
	}
	if !graphFollowUpExpansionStep(step) {
		return false, nil
	}
	if graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok {
		if !stepRequiresConcreteFailureEvidenceAcquisition(step) && graphIsConcreteValidationEvidenceGraph(graph) {
			return false, nil
		}
		if inputRequiresStateChangingOutput(step.Input) && graphIsConcreteStateChangingContinuation(graph) {
			return false, nil
		}
	}
	count := o.graphManager().graphFollowUpExpansionCount(ctx, run.ID)
	if count <= maxGraphFollowUpExpansionAttempts {
		return false, nil
	}
	output["graphFollowUpExpansionLimitExceeded"] = true
	output["graphFollowUpExpansionCount"] = count
	output["graphFollowUpExpansionLimit"] = maxGraphFollowUpExpansionAttempts
	output["ignoredExecutionGraph"] = true
	step.Output = output
	_ = o.steps.Update(ctx, step)
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return true, err
	}
	input := o.graphManager().graphExpansionLimitAssessmentFollowUpStepInputFromSteps(ctx, run, step, steps, count)
	return true, o.RequestUniqueDispatchStep(ctx, run, graphExpansionLimitAssessmentFollowUpStepID(run.ID, step), initialStepKind(run), input, workspaceIDForDispatch(run, workspace))
}

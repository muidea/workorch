package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	workspaceModel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
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
	var unlockLLMDispatch func()
	if kind == llmWorkspaceExecutionStepKind {
		if err := o.ensureGapBaselineBeforeDispatch(ctx, run); err != nil {
			return err
		}
		if gated, err := gapBaselineDispatchGate(run); gated || err != nil {
			return err
		}
		if err := o.hydrateStateChangeTargetSnapshotsBeforeDispatch(ctx, run, input); err != nil {
			return err
		}
		var rebuildErr error
		input, rebuildErr = rebuildDispatchInputFromConvergenceAuthority(run, stepID, cloneStepPayload(input))
		if rebuildErr != nil {
			return rebuildErr
		}
		unlockLLMDispatch = o.lockRunLLMDispatch(run.ID)
		defer func() {
			if unlockLLMDispatch != nil {
				unlockLLMDispatch()
			}
		}()
		if active, err := o.hasOtherActiveLLMWorkspaceExecution(ctx, run.ID, stepID); err != nil {
			return err
		} else if active {
			return nil
		}
		if duplicate, err := o.hasInFlightRuntimeRepairForSameScope(ctx, run.ID, stepID, input); err != nil {
			return err
		} else if duplicate {
			return nil
		}
		if duplicate, err := o.hasInFlightLLMGraphDispatchForSameScope(ctx, run.ID, stepID, kind, input); err != nil {
			return err
		} else if duplicate {
			return nil
		}
	} else if dispatchInputRequiresConvergenceDispatchValidation(input) {
		var finalizeErr error
		input, finalizeErr = finalizeCurrentAuthorityDispatchInputForRun(run, stepID, cloneStepPayload(input))
		if finalizeErr != nil {
			return finalizeErr
		}
	}
	step, err := o.steps.Get(ctx, stepID)
	if err != nil {
		if err != common.ErrNotFound {
			return err
		}
		step = &runModel.Step{
			ID:    stepID,
			RunID: run.ID,
			Kind:  kind,
			State: common.StepStateExecuting,
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
		case common.StepStateSucceeded, common.StepStateFailed:
			return fmt.Errorf("%w: runtime step %s is already terminal", common.ErrConflict, step.ID)
		}
		step.State = common.StepStateExecuting
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
	if unlockLLMDispatch != nil {
		unlockLLMDispatch()
		unlockLLMDispatch = nil
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
	if kind == llmWorkspaceExecutionStepKind {
		var finalizeErr error
		step.Input, finalizeErr = finalizeLLMDispatchAuthorityInput(run, step.ID, step.Input)
		if finalizeErr != nil {
			output := map[string]any{
				"status":              "failed",
				"stage":               failureClassRuntimeContractMisalign,
				"error":               finalizeErr.Error(),
				failureOutputKeyClass: failureClassRuntimeContractMisalign,
				failureOutputKeySignature: runtimeFailureSignature(failureClassRuntimeContractMisalign, step, map[string]any{
					"stage": failureClassRuntimeContractMisalign,
					"error": finalizeErr.Error(),
				}, finalizeErr),
			}
			return o.handleRuntimeStepFailure(ctx, run, step, nil, output, "runtime convergence authority dispatch gate failed", finalizeErr)
		}
	} else if dispatchInputRequiresConvergenceDispatchValidation(step.Input) {
		var finalizeErr error
		step.Input, finalizeErr = finalizeCurrentAuthorityDispatchInputForRun(run, step.ID, step.Input)
		if finalizeErr != nil {
			output := map[string]any{
				"status":              "failed",
				"stage":               failureClassRuntimeContractMisalign,
				"error":               finalizeErr.Error(),
				failureOutputKeyClass: failureClassRuntimeContractMisalign,
				failureOutputKeySignature: runtimeFailureSignature(failureClassRuntimeContractMisalign, step, map[string]any{
					"stage": failureClassRuntimeContractMisalign,
					"error": finalizeErr.Error(),
				}, finalizeErr),
			}
			return o.handleRuntimeStepFailure(ctx, run, step, nil, output, "runtime convergence authority dispatch gate failed", finalizeErr)
		}
	}
	runtimeInput := cloneStepPayload(step.Input)
	if err := validateRuntimeStepKindRequiredOutputBeforeRuntime(step.Kind, runtimeInput); err != nil {
		output := map[string]any{
			"status":              "failed",
			"stage":               "runtime_required_output_kind_rejected",
			"error":               err.Error(),
			"failureReason":       "runtime_required_output_kind_invalid",
			failureOutputKeyClass: failureClassRuntimeContractMisalign,
			failureOutputKeySignature: runtimeFailureSignature(failureClassRuntimeContractMisalign, step, map[string]any{
				"stage": "runtime_required_output_kind_rejected",
				"error": err.Error(),
			}, err),
			"runtimeDiagnostics": map[string]any{
				"kind":      "runtime_required_output_kind_rejected",
				"operation": step.Kind,
				"reason":    "runtime_step_kind_not_accepted_by_required_output",
			},
		}
		return o.handleRuntimeStepFailure(ctx, run, step, nil, output, "runtime required output kind contract failed", err)
	}
	if normalized, changed := normalizeRuntimeValidationStepInputBeforeRuntime(step.Kind, runtimeInput); changed {
		runtimeInput = normalized
		step.Input = cloneStepPayload(normalized)
	}
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
	if handled, err := o.dispatchRuntimeManagedTargetSnapshotAcquisition(ctx, run, step, toolCall, workspace); handled || err != nil {
		return err
	}

	if err := validateRuntimeValidationStepInputBeforeRuntime(step.Kind, runtimeInput); err != nil {
		output := map[string]any{
			"status":              "failed",
			"stage":               "runtime_validation_dispatch_contract_rejected",
			"error":               err.Error(),
			"failureReason":       "validation_dispatch_contract_invalid",
			failureOutputKeyClass: failureClassRuntimeContractMisalign,
			failureOutputKeySignature: runtimeFailureSignature(failureClassRuntimeContractMisalign, step, map[string]any{
				"stage": "runtime_validation_dispatch_contract_rejected",
				"error": err.Error(),
			}, err),
			"runtimeDiagnostics": map[string]any{
				"kind":      "runtime_validation_dispatch_contract_rejected",
				"operation": step.Kind,
				"reason":    "validation_dispatch_contract_invalid",
			},
		}
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime validation dispatch contract failed", err)
	}
	if err := validatePatchStepInputBeforeRuntime(step.Kind, runtimeInput); err != nil {
		output := map[string]any{
			"status": "failed",
			"stage":  "patch_dispatch_invalid",
			"error":  err.Error(),
		}
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime patch dispatch gate failed", err)
	}
	if blocked, err := o.blockPatchApplyForApprovalBeforeRuntime(ctx, run, step, toolCall, workspace, runtimeInput); blocked || err != nil {
		return err
	}
	if err := validateMCPToolCallStepInputBeforeRuntime(run, step, runtimeInput); err != nil {
		output := map[string]any{
			"status":    "failed",
			"stage":     "mcp_tool_call_rejected",
			"error":     err.Error(),
			"operation": step.Kind,
			"serverId":  stringFromMap(runtimeInput, "serverId"),
			"tool":      stringFromMap(runtimeInput, "tool"),
			"input":     cloneStepPayload(mapFromValue(runtimeInput["input"])),
			"runtimeDiagnostics": map[string]any{
				"operation": step.Kind,
				"serverId":  stringFromMap(runtimeInput, "serverId"),
				"tool":      stringFromMap(runtimeInput, "tool"),
				"reason":    "mcp_tool_input_schema_invalid",
			},
		}
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime mcp dispatch gate failed", err)
	}
	injectRuntimeEvidencePolicies(runtimeInput, run.ExecutionContext)
	recoveryPolicy := runtimemodel.RuntimeRecoveryPolicyFromInput(runtimeInput)
	if _, err := ensureRuntimeValidationScratchDir(runtimeInput); err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, map[string]any{
			"status": "failed",
			"stage":  "runtime_validation_scratch_prepare_failed",
			"error":  err.Error(),
		}, "runtime validation scratch prepare failed", err)
	}
	request := runtimemodel.StepExecutionRequest{
		WorkspaceID:                   run.WorkspaceID,
		WorkspaceDeliverableDirectory: workspace.DeliverableDirectory,
		WorkspaceInternalDirectory:    workspace.InternalDirectory,
		RunID:                         run.ID,
		StepID:                        step.ID,
		Kind:                          step.Kind,
		Input:                         runtimeInput,
		RecoveryPolicy:                recoveryPolicy,
	}
	if attachManagedValidationResourceOwnership(runtimeInput, request, time.Now().UTC()) {
		request.Input = runtimeInput
		step.Input = cloneStepPayload(runtimeInput)
		if err := o.steps.Update(ctx, step); err != nil {
			return err
		}
	}
	if requester, ok := o.runtime.(RuntimeExecutionRequester); ok {
		accepted, err := requester.RequestRuntimeStepExecution(ctx, request)
		if err == nil {
			acceptedPolicy := accepted.RecoveryPolicy
			if acceptedPolicy == (runtimemodel.RuntimeRecoveryPolicy{}) {
				acceptedPolicy = request.RecoveryPolicy
			}
			checkpointData := map[string]any{
				"requestId":  accepted.RequestID,
				"queueKey":   accepted.QueueKey,
				"acceptedAt": accepted.AcceptedAt,
			}
			policy := acceptedPolicy.Normalized()
			checkpointData["runtimeRecoveryPolicy"] = policy.ToMap()
			checkpointData["recoverySafety"] = policy.WatchdogRecoverySafety()
			if checkpointErr := o.checkpointManager().recordStepProgressCheckpoint(ctx, run, step, "runtime_execute_accepted", checkpointData); checkpointErr != nil {
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

func (o *RuntimeOrchestrator) hydrateStateChangeTargetSnapshotsBeforeDispatch(ctx context.Context, run *runModel.Run, input map[string]any) error {
	if o == nil || run == nil || !dispatchRequiresStateChangeTargetSnapshotHydration(run, input) {
		return nil
	}
	before := stateChangeTargetSnapshotDispatchFingerprint(run)
	if err := o.hydrateCurrentTargetSnapshotsFromAcceptedEvidence(ctx, run); err != nil {
		return err
	}
	if after := stateChangeTargetSnapshotDispatchFingerprint(run); after != before && o.runs != nil {
		return o.persistRunProgressUpdate(ctx, run)
	}
	return nil
}

func dispatchRequiresStateChangeTargetSnapshotHydration(run *runModel.Run, input map[string]any) bool {
	for _, kind := range []string{
		latestRunAuthorityNextActionKind(run),
		strings.TrimSpace(stringFromMap(currentActionNextActionFromInput(input), "kind")),
		strings.TrimSpace(convergenceNextActionKind(run, input)),
	} {
		switch kind {
		case runtimemodel.NextActionTypeStateChange, "produce_state_change":
			return true
		}
	}
	for _, contract := range []map[string]any{
		mapFromValue(input["convergenceContract"]),
		runExecutionContextMapValue(run, "convergenceContract"),
	} {
		if strings.TrimSpace(stringFromMap(contract, "state")) == convergenceStateNeedStateChange {
			return true
		}
	}
	return false
}

func runExecutionContextMapValue(run *runModel.Run, key string) map[string]any {
	if run == nil || run.ExecutionContext == nil {
		return nil
	}
	return mapFromValue(run.ExecutionContext[key])
}

func stateChangeTargetSnapshotDispatchFingerprint(run *runModel.Run) string {
	if run == nil || run.ExecutionContext == nil {
		return ""
	}
	pkg := mapFromValue(run.ExecutionContext["repairActionPackage"])
	return stableCoverageJSON(map[string]any{
		"repairActionPackageTargetSnapshots":        mapSliceFromValue(pkg["targetSnapshots"]),
		"repairActionPackageCurrentTargetSnapshots": mapSliceFromValue(pkg["currentTargetSnapshots"]),
		"targetSnapshots":                           mapSliceFromValue(run.ExecutionContext["targetSnapshots"]),
		"currentTargetSnapshots":                    mapSliceFromValue(run.ExecutionContext["currentTargetSnapshots"]),
	})
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
		if step.State == common.StepStateSucceeded {
			return o.resumeCompletedGraphBatchStepContinuation(ctx, run, step, workspace)
		}
		if step.State == common.StepStateFailed {
			return nil
		}
	} else if err != common.ErrNotFound {
		return err
	}
	currentRun, acceptsResult, err := o.currentRunForRuntimeStepResult(ctx, run, step)
	if err != nil {
		return err
	}
	if !acceptsResult {
		return o.persistLateRuntimeStepResultNotAccepted(ctx, currentRun, step, toolCall, result, execErr)
	}
	run = currentRun
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
	if err := patches.preflightApprovalCandidates(output, workspace.DeliverableDirectory); err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime patch preflight failed", err)
	}
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
	} else {
		if handled, patchedOutput, err := o.applyPatchCandidatesThroughRuntimeBlocks(ctx, run, step, toolCall, output, workspace); handled || err != nil {
			if patchedOutput != nil {
				output = patchedOutput
			}
			if err != nil {
				return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime patch application failed", err)
			}
		} else if err := patches.apply(ctx, run, step, toolCall, output, workspace.DeliverableDirectory); err != nil {
			return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime patch application failed", err)
		}
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

	step.Output = output
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
		if err := o.finalizeGovernedRuntimeStep(ctx, run, step, toolCall, output); err != nil {
			return err
		}
		return o.blockOnDecisionAtStep(ctx, run, workspace, ticketID, step.ID)
	}
	if approvalSpec := approvalBlockSpec(output); approvalSpec.Reason != "" {
		if approvalSpec.ApprovalID == "" {
			approvalSpec.ApprovalID = common.ApprovalID(runtimeChildID(step.ID, "approval"))
		}
		if err := o.createApproval(ctx, run, step, approvalSpec.ApprovalID, approvalSpec.Reason, approvalSpec.Action, approvalSpec.Context); err != nil {
			return err
		}
		if err := o.finalizeGovernedRuntimeStep(ctx, run, step, toolCall, output); err != nil {
			return err
		}
		return o.blockOnApprovalAtStep(ctx, run, workspace, approvalSpec.ApprovalID, step.ID)
	}
	if graphBatchStep(step) {
		if err := o.finalizeAcceptedRuntimeStep(ctx, run, step, toolCall, output); err != nil {
			return err
		}
		if !o.eventDrivenDispatch {
			return nil
		}
		steps, err := o.steps.ListByRun(ctx, run.ID)
		if err != nil {
			return err
		}
		return o.continueGraphBatchAfterCompletedStep(ctx, run, step, steps, workspace)
	}
	if nextStepID != "" {
		if err := o.finalizeAcceptedRuntimeStep(ctx, run, step, toolCall, output); err != nil {
			return err
		}
		return o.RequestUniqueDispatchStep(ctx, run, nextStepID, nextKind, nextStepInputFromOutput(run, step), workspaceIDForDispatch(run, workspace))
	}
	if handled, err := o.validateGraphExpansionBoundary(ctx, run, step, toolCall, output, workspace); handled || err != nil {
		return err
	}
	graphSpecs, graphSteps := []stepDispatchSpec(nil), []*runModel.Step(nil)
	if graph, ok := runtimeGraphSpecFromValue(output[stepOutputKeyExecutionGraph]); ok && len(graph.Nodes) > 0 {
		if err := o.finalizeAcceptedRuntimeStep(ctx, run, step, toolCall, output); err != nil {
			return err
		}
		graphSpecs, graphSteps = o.graphManager().readyGraphStepSpecsWithSteps(ctx, run, step)
		if len(graphSpecs) > 0 {
			if input, ok := o.graphManager().concreteEvidenceAcquisitionFollowUpInputFromSteps(run, step, graphSteps); ok {
				return o.RequestUniqueDispatchStep(ctx, run, graphFollowUpStepID(run.ID, step), initialStepKind(run), input, workspaceIDForDispatch(run, workspace))
			}
			return o.dispatchGraphStepBatchOrCurrentAuthorityFollowUp(ctx, run, step, graphSteps, graphSpecs, workspace)
		}
	} else {
		graphSpecs, graphSteps = o.graphManager().readyGraphStepSpecsWithSteps(ctx, run, step)
		if len(graphSpecs) > 0 {
			if err := o.finalizeAcceptedRuntimeStep(ctx, run, step, toolCall, output); err != nil {
				return err
			}
			if input, ok := o.graphManager().concreteEvidenceAcquisitionFollowUpInputFromSteps(run, step, graphSteps); ok {
				return o.RequestUniqueDispatchStep(ctx, run, graphFollowUpStepID(run.ID, step), initialStepKind(run), input, workspaceIDForDispatch(run, workspace))
			}
			return o.dispatchGraphStepBatchOrCurrentAuthorityFollowUp(ctx, run, step, graphSteps, graphSpecs, workspace)
		}
	}
	if o.graphManager().hasActiveGraphWork(ctx, run, step) {
		if step.State != common.StepStateSucceeded {
			if err := o.finalizeAcceptedRuntimeStep(ctx, run, step, toolCall, output); err != nil {
				return err
			}
		}
		return nil
	}
	if handled, err := o.resumeArtifactContinuationAfterContextGraph(ctx, run, step, output, workspace); handled || err != nil {
		if handled && err == nil {
			if finalizeErr := o.finalizeAcceptedRuntimeStep(ctx, run, step, toolCall, output); finalizeErr != nil {
				return finalizeErr
			}
		}
		return err
	}
	if shouldDispatchGraphFollowUp(run, step, output) {
		if err := o.finalizeAcceptedRuntimeStep(ctx, run, step, toolCall, output); err != nil {
			return err
		}
		var input map[string]any
		if len(graphSteps) > 0 {
			input = o.graphManager().graphFollowUpStepInputFromSteps(run, step, graphSteps)
		} else {
			input = o.graphManager().graphFollowUpStepInput(ctx, run, step)
		}
		return o.RequestUniqueDispatchStep(ctx, run, graphFollowUpStepID(run.ID, step), initialStepKind(run), input, workspaceIDForDispatch(run, workspace))
	}
	if shouldDispatchLLMCompletionFollowUp(run, step, output) {
		if err := o.finalizeAcceptedRuntimeStep(ctx, run, step, toolCall, output); err != nil {
			return err
		}
		return o.RequestUniqueDispatchStep(ctx, run, llmCompletionFollowUpStepID(run.ID, step), initialStepKind(run), o.graphManager().llmCompletionFollowUpStepInput(ctx, run, step), workspaceIDForDispatch(run, workspace))
	}
	if shouldDispatchNoActionableContinuationFollowUp(run, step, output) {
		if err := o.finalizeAcceptedRuntimeStep(ctx, run, step, toolCall, output); err != nil {
			return err
		}
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
	output, evidenceNormalizeErr = normalizeCompletionGateDoDAssessmentEvidenceRefs(ctx, o.steps, o.evidence, run, output, step)
	if evidenceNormalizeErr != nil {
		return evidenceNormalizeErr
	}
	step.Output = output
	if err := o.steps.Update(ctx, step); err != nil {
		return err
	}
	if err := o.recordCoverageProgressFromCompletionAssessment(ctx, run, step, output); err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime completion gate failed", err)
	}
	assessment, summary, err := validatedCompletionDoDAssessmentForStep(run, step, output)
	if err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime completion gate failed", err)
	}
	if err := validateDoDAssessmentEvidenceQuality(ctx, o.steps, o.evidence, run, assessment); err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime completion gate failed", err)
	}
	if o.coverageLedger().updateFromAssessment(run, step, assessment, ctx) && o.runs != nil {
		if err := o.persistRunProgressUpdate(ctx, run); err != nil {
			return err
		}
	}
	if err := o.resultManager().recordAcceptedSliceResultsFromRun(ctx, run, step); err != nil {
		return err
	}
	if changed, bound := o.reconcileCoverageValidationEvidence(ctx, run); changed && o.runs != nil {
		if err := o.persistRunProgressUpdate(ctx, run); err != nil {
			return err
		}
		logCoverageReconcilerBinding(run.ID, bound)
	}
	if err := o.coverageLedger().validateReadyForAcceptance(run, ctx); err != nil {
		return o.handleRuntimeStepFailure(ctx, run, step, toolCall, output, "runtime completion gate failed", err)
	}
	if err := o.finalizeAcceptedRuntimeStep(ctx, run, step, toolCall, output); err != nil {
		return err
	}
	assessment = completionAssessmentWithCurrentStepValidationEvidenceRef(assessment, step.ID)
	if err := o.resultManager().recordCompletionEvidenceFromValidatedDoDAssessment(ctx, run, step, assessment, summary, workspace); err != nil {
		return err
	}
	return o.validateAndCompleteRunWithWorkspace(ctx, run, step.ID, workspace)
}

func (o *RuntimeOrchestrator) resumeCompletedGraphBatchStepContinuation(ctx context.Context, run *runModel.Run, step *runModel.Step, workspace *workspaceModel.Workspace) error {
	if o == nil || run == nil || step == nil {
		return common.ErrInvalidArgument
	}
	if !o.eventDrivenDispatch || !graphBatchStep(step) {
		return nil
	}
	currentRun, acceptsResult, err := o.currentRunForRuntimeStepResult(ctx, run, step)
	if err != nil {
		return err
	}
	if !acceptsResult {
		return nil
	}
	if workspace == nil || workspace.ID != currentRun.WorkspaceID {
		if o.workspaces == nil {
			return common.ErrInvalidArgument
		}
		workspace, err = o.workspaces.Get(ctx, currentRun.WorkspaceID)
		if err != nil {
			return err
		}
	}
	steps, err := o.steps.ListByRun(ctx, currentRun.ID)
	if err != nil {
		return err
	}
	if completedGraphBatchStepHasDownstreamStep(steps, step) {
		anchor, terminal := o.graphManager().graphTerminalContinuationAnchor(ctx, currentRun, step, steps)
		if !terminal || anchor == nil {
			return nil
		}
		step = anchor
	}
	return o.continueGraphBatchAfterCompletedStep(ctx, currentRun, step, steps, workspace)
}

func (o *RuntimeOrchestrator) persistLateRuntimeStepResultNotAccepted(
	ctx context.Context,
	run *runModel.Run,
	step *runModel.Step,
	toolCall *runModel.ToolCall,
	result runtimemodel.StepExecutionResult,
	execErr error,
) error {
	if o == nil || run == nil || step == nil {
		return common.ErrInvalidArgument
	}
	switch step.State {
	case common.StepStateSucceeded, common.StepStateFailed:
		return nil
	}
	output := compactLateRuntimeResultNotAcceptedOutput(run, result.Output, execErr)
	step.State = common.StepStateFailed
	step.Output = output
	finalizeManagedValidationResourcesForStep(step, "skipped", "late_result_not_accepted")
	_ = o.recordToolCallFailed(ctx, toolCall, execErr, output)
	if err := o.steps.Update(ctx, step); err != nil {
		return err
	}
	if err := o.checkpointManager().recordStepCheckpoint(ctx, run, step, "step_failed"); err != nil {
		return err
	}
	return o.eventPublisher().publishStepState(ctx, run, step, runevents.TopicStepStateChanged)
}

func compactLateRuntimeResultNotAcceptedOutput(run *runModel.Run, output map[string]any, execErr error) map[string]any {
	ret := map[string]any{
		"status":       "ignored",
		"stage":        "runtime_result_not_accepted",
		"reason":       "runtime result arrived after run state stopped accepting step results",
		"failureClass": "runtime_result_not_accepted",
	}
	if run != nil {
		ret["runState"] = string(run.State)
		if run.BlockingTicketID != "" {
			ret["blockingTicketId"] = string(run.BlockingTicketID)
		}
		if run.BlockingApprovalID != "" {
			ret["blockingApprovalId"] = string(run.BlockingApprovalID)
		}
	}
	if execErr != nil {
		ret["error"] = execErr.Error()
	}
	if len(output) > 0 {
		ret["runtimeResultAvailable"] = true
		for _, key := range []string{"status", "stage", "summary", "completionState", "failureClass"} {
			value := strings.TrimSpace(stringFromMap(output, key))
			if value == "" {
				continue
			}
			ret["runtimeResult"+upperFirst(key)] = value
		}
		if nodes := executionGraphNodeCount(output["executionGraph"]); nodes > 0 {
			ret["runtimeResultExecutionGraphNodeCount"] = nodes
		}
		if refs := stringSliceFromValue(output["llmCallRefs"]); len(refs) > 0 {
			ret["llmCallRefs"] = refs
		}
	}
	return ret
}

func upperFirst(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func executionGraphNodeCount(value any) int {
	switch typed := value.(type) {
	case []any:
		return len(typed)
	case []map[string]any:
		return len(typed)
	case map[string]any:
		if nodes, ok := typed["nodes"].([]any); ok {
			return len(nodes)
		}
	}
	return 0
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

func (o *RuntimeOrchestrator) lockRunLLMDispatch(runID common.RunID) func() {
	if o == nil || runID == "" {
		return func() {}
	}
	value, _ := o.llmDispatchMu.LoadOrStore(runID, &sync.Mutex{})
	mu, _ := value.(*sync.Mutex)
	if mu == nil {
		return func() {}
	}
	mu.Lock()
	return func() {
		mu.Unlock()
	}
}

func (o *RuntimeOrchestrator) currentRunForRuntimeStepResult(ctx context.Context, run *runModel.Run, step *runModel.Step) (*runModel.Run, bool, error) {
	if o == nil || o.runs == nil || run == nil || step == nil || run.ID == "" || step.ID == "" {
		return nil, false, common.ErrInvalidArgument
	}
	current, err := o.runs.GetByWorkspaceID(ctx, run.WorkspaceID, run.ID)
	if err != nil {
		return nil, false, err
	}
	if current.WorkspaceID != run.WorkspaceID || step.RunID != current.ID {
		return nil, false, common.ErrConflict
	}
	switch current.State {
	case common.RunStateActive:
		return current, true, nil
	case common.RunStateBlockedOnDecision, common.RunStateBlockedOnApproval:
		accepted, err := o.acceptResolvedGovernanceStepResult(ctx, current, step)
		if err != nil || accepted {
			return current, accepted, err
		}
		return current, false, nil
	default:
		return current, false, nil
	}
}

func (o *RuntimeOrchestrator) acceptResolvedGovernanceStepResult(ctx context.Context, run *runModel.Run, step *runModel.Step) (bool, error) {
	if o == nil || run == nil || step == nil || step.RunID != run.ID {
		return false, nil
	}
	switch run.State {
	case common.RunStateBlockedOnDecision:
		if run.BlockingTicketID == "" || o.tickets == nil || len(mapFromValue(step.Input["decisionResolution"])) == 0 {
			return false, nil
		}
		scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID)
		ticket, err := o.tickets.Get(scopedCtx, run.BlockingTicketID)
		if err != nil {
			return false, err
		}
		if ticket == nil || ticket.ID != run.BlockingTicketID || ticket.RunID != run.ID || ticket.State != common.DecisionTicketStateResolved {
			return false, nil
		}
	case common.RunStateBlockedOnApproval:
		if run.BlockingApprovalID == "" || o.approvals == nil || len(mapFromValue(step.Input["approvalResolution"])) == 0 {
			return false, nil
		}
		scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID)
		approval, err := o.approvals.Get(scopedCtx, run.BlockingApprovalID)
		if err != nil {
			return false, err
		}
		if approval == nil || approval.ID != run.BlockingApprovalID || approval.RunID != run.ID || approval.State != common.ApprovalStateApproved {
			return false, nil
		}
	default:
		return false, nil
	}
	run.State = common.RunStateActive
	run.BlockingTicketID = ""
	run.BlockingApprovalID = ""
	run.FailureReason = ""
	workspace, err := o.runStateManager().syncRunAndWorkspaceForActiveState(ctx, run, common.WorkspaceStateActive, nil)
	if err != nil {
		return false, err
	}
	if err := o.eventPublisher().publishRunStateChanged(ctx, run); err != nil {
		return false, err
	}
	if err := o.eventPublisher().publishWorkspaceStateChanged(ctx, workspace, run.ID); err != nil {
		return false, err
	}
	return true, nil
}

func (o *RuntimeOrchestrator) currentRunCanRecordRuntimeProgress(ctx context.Context, run *runModel.Run, step *runModel.Step) (bool, error) {
	_, ok, err := o.currentRunForRuntimeStepResult(ctx, run, step)
	if err == nil {
		return ok, nil
	}
	if errors.Is(err, common.ErrNotFound) && run != nil {
		switch run.State {
		case common.RunStateActive:
			return true, nil
		}
		return false, nil
	}
	return false, err
}

func (o *RuntimeOrchestrator) hasInFlightRuntimeRepairForSameScope(ctx context.Context, runID common.RunID, stepID common.StepID, input map[string]any) (bool, error) {
	if o == nil || o.steps == nil || runID == "" || stepID == "" {
		return false, common.ErrInvalidArgument
	}
	scope := runtimeRepairDispatchScope(input)
	if scope == "" {
		return false, nil
	}
	steps, err := o.steps.ListByRunSummary(ctx, runID)
	if err != nil {
		return false, err
	}
	for _, item := range steps {
		if item == nil || item.ID == stepID {
			continue
		}
		switch item.State {
		case common.StepStateSucceeded, common.StepStateFailed:
			continue
		}
		if runtimeRepairDispatchScope(item.Input) == scope {
			return true, nil
		}
	}
	return false, nil
}

func (o *RuntimeOrchestrator) hasOtherActiveLLMWorkspaceExecution(ctx context.Context, runID common.RunID, stepID common.StepID) (bool, error) {
	if o == nil || o.steps == nil || runID == "" || stepID == "" {
		return false, common.ErrInvalidArgument
	}
	steps, err := o.steps.ListByRunSummary(ctx, runID)
	if err != nil {
		return false, err
	}
	for _, item := range steps {
		if item == nil || item.ID == stepID {
			continue
		}
		if canonicalLLMWorkspaceStepKind(item.Kind) != llmWorkspaceExecutionStepKind {
			continue
		}
		switch item.State {
		case common.StepStateSucceeded, common.StepStateFailed:
			continue
		default:
			return true, nil
		}
	}
	return false, nil
}

func (o *RuntimeOrchestrator) hasInFlightLLMGraphDispatchForSameScope(ctx context.Context, runID common.RunID, stepID common.StepID, kind string, input map[string]any) (bool, error) {
	if o == nil || o.steps == nil || runID == "" || stepID == "" {
		return false, common.ErrInvalidArgument
	}
	scope := llmGraphDispatchScope(kind, input)
	if scope == "" {
		return false, nil
	}
	steps, err := o.steps.ListByRunSummary(ctx, runID)
	if err != nil {
		return false, err
	}
	for _, item := range steps {
		if item == nil || item.ID == stepID {
			continue
		}
		switch item.State {
		case common.StepStateSucceeded, common.StepStateFailed:
			continue
		}
		if llmGraphDispatchScope(item.Kind, item.Input) == scope {
			return true, nil
		}
	}
	return false, nil
}

func llmGraphDispatchScope(kind string, input map[string]any) string {
	if canonicalLLMWorkspaceStepKind(kind) != llmWorkspaceExecutionStepKind || len(input) == 0 {
		return ""
	}
	root := strings.TrimSpace(stringFromMap(input, stepInputKeyGraphRootStepID))
	if root == "" {
		return ""
	}
	return "llmGraphRoot:" + root
}

func runtimeRepairDispatchScope(input map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	repairOf := strings.TrimSpace(firstNonEmptyString(
		stringFromMap(input, stepInputKeyRepairOfStepID),
		stringFromMap(input, "failedStepId"),
	))
	if repairOf == "" {
		return ""
	}
	return "repairOfStepId:" + repairOf
}

var runtimeStepProgressMinInterval = 30 * time.Second

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
		if acceptsProgress, err := o.currentRunCanRecordRuntimeProgress(ctx, run, step); err != nil || !acceptsProgress {
			return err
		}
		last = now
		lastSignature = signature
		if err := o.checkpointManager().recordStepProgressCheckpoint(ctx, run, step, event, data); err != nil {
			return err
		}
		return o.eventPublisher().publishRuntimeStepProgress(ctx, run, step, event, data)
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
				input := assessmentOnlyContractCorrectionStepInput(step, output, err)
				if finalizeErr := o.finalizeRejectedRuntimeStepForContinuation(ctx, run, step, toolCall, output, "runtime completion gate failed", err); finalizeErr != nil {
					return true, finalizeErr
				}
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
			count := o.graphManager().graphFollowUpConcreteValidationExpansionCountIncludingCandidate(ctx, run.ID, step, output)
			if count <= maxGraphFollowUpExpansionAttempts {
				return false, nil
			}
			output["graphFollowUpExpansionCount"] = count
			output["graphFollowUpExpansionKind"] = "concrete_validation_evidence"
		} else if inputRequiresStateChangingOutput(step.Input) && graphIsConcreteStateChangingContinuation(graph) {
			return false, nil
		}
	}
	count := o.graphManager().graphFollowUpExpansionCountIncludingCandidate(ctx, run.ID, step, output)
	if concreteValidationCount := int(int64FromValue(output["graphFollowUpExpansionCount"])); concreteValidationCount > count {
		count = concreteValidationCount
	}
	if count <= maxGraphFollowUpExpansionAttempts {
		return false, nil
	}
	output["graphFollowUpExpansionLimitExceeded"] = true
	output["graphFollowUpExpansionCount"] = count
	output["graphFollowUpExpansionLimit"] = maxGraphFollowUpExpansionAttempts
	output["ignoredExecutionGraph"] = true
	output["status"] = "completed"
	output["stage"] = "graph_follow_up_expansion_limit"
	output["summary"] = "graph follow-up expansion limit reached; transitioned to assessment-only next-focus evaluation"
	steps, err := o.steps.ListByRun(ctx, run.ID)
	if err != nil {
		return true, err
	}
	step.Output = output
	input := o.graphManager().graphExpansionLimitAssessmentFollowUpStepInputFromSteps(ctx, run, step, steps, count)
	if finalizeErr := o.finalizeGovernedRuntimeStep(ctx, run, step, toolCall, output); finalizeErr != nil {
		return true, finalizeErr
	}
	return true, o.RequestUniqueDispatchStep(ctx, run, graphExpansionLimitAssessmentFollowUpStepID(run.ID, step), initialStepKind(run), input, workspaceIDForDispatch(run, workspace))
}

package orchestrator

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisioncommon "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/common"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	resultcommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMergeResumeInputPreservesProviderRetryRecoveryContract(t *testing.T) {
	specs := []stepDispatchSpec{{
		ID:   "run-provider-retry-merge/provider-retry-003-of-repair-001",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			"repairObjective":                 repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired":     true,
			"repairGuidance":                  []string{"retry existing state-changing contract"},
		},
	}}
	resumeInput := map[string]any{
		"repairObjective":                 repairObjectiveAcquireFailureEvidence,
		"concreteFailureEvidenceRequired": true,
		"repairGuidance":                  []string{"stale convergence wrapper"},
		"convergenceContract":             map[string]any{"state": convergenceStateNeedEvidence},
		"decisionResolution":              map[string]any{"resolutionId": "resolution-provider-retry-merge"},
	}

	mergeResumeInput(specs, resumeInput)

	if specs[0].Input["repairObjective"] != repairObjectiveStateChangingOutput ||
		specs[0].Input["stateChangingOutputRequired"] != true ||
		specs[0].Input["concreteFailureEvidenceRequired"] != nil ||
		mapFromValue(specs[0].Input["convergenceContract"]) != nil {
		t.Fatalf("provider retry input = %+v, want retry contract preserved over resume wrapper", specs[0].Input)
	}
	if got := stringSliceFromValue(specs[0].Input["repairGuidance"]); len(got) != 1 || got[0] != "retry existing state-changing contract" {
		t.Fatalf("repairGuidance = %+v, want provider retry guidance preserved", specs[0].Input["repairGuidance"])
	}
	if mapFromValue(specs[0].Input["decisionResolution"]) == nil {
		t.Fatalf("decisionResolution missing from merged provider retry input: %+v", specs[0].Input)
	}
}

func TestMergeResumeInputPreservesExistingRepairContractAgainstDecisionWrapper(t *testing.T) {
	specs := []stepDispatchSpec{{
		ID:   "run-repair-merge/repair-004-of-step-1",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyRuntimeStage:      runtimeStageRepair,
			stepInputKeyRuntimeStageMode:  "strict_convergence",
			stepInputKeySoulStage:         soulStageExecution,
			"repairObjective":             repairObjectiveStateChangingOutput,
			"stateChangingOutputRequired": true,
			"outputMode":                  outputModeExecutionGraph,
			"repairActionPackage": map[string]any{
				"requiredOutput": map[string]any{
					"outputMode":                   outputModeExecutionGraph,
					"mustIncludeStateChangingNode": true,
					"acceptedKinds":                []string{"patch/apply"},
				},
			},
			"convergenceContract": map[string]any{
				"state":        convergenceStateNeedStateChange,
				"runtimeStage": runtimeStageRepair,
			},
		},
	}}
	resumeInput := map[string]any{
		"repairObjective":                 repairObjectiveAcquireFailureEvidence,
		"concreteFailureEvidenceRequired": true,
		"outputMode":                      outputModeIncrementalArtifact,
		"outputModeSelector": map[string]any{
			"mode": outputModeIncrementalArtifact,
		},
		"artifactOutputContract": map[string]any{
			"mode":  outputModeIncrementalArtifact,
			"phase": artifactOutputPhasePlan,
		},
		"repairActionPackage": map[string]any{
			"requiredOutput": map[string]any{
				"outputMode":             outputModeIncrementalArtifact,
				"mustReturnArtifactPlan": true,
			},
		},
		"convergenceContract": map[string]any{
			"state":        convergenceStateNeedEvidence,
			"runtimeStage": runtimeStageRepair,
		},
		"decisionResolution": map[string]any{
			"resolutionId": "resolution-repair-merge",
		},
	}

	mergeResumeInput(specs, resumeInput)

	input := specs[0].Input
	if input["repairObjective"] != repairObjectiveStateChangingOutput ||
		input["stateChangingOutputRequired"] != true ||
		input["outputMode"] != outputModeExecutionGraph {
		t.Fatalf("merged input = %+v, want existing repair contract preserved", input)
	}
	if input["concreteFailureEvidenceRequired"] != nil ||
		mapFromValue(input["artifactOutputContract"]) != nil {
		t.Fatalf("merged input = %+v, want stale wrapper contract ignored", input)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustReturnArtifactPlan"] != nil || required["outputMode"] != outputModeExecutionGraph {
		t.Fatalf("requiredOutput = %+v, want original executionGraph repair contract preserved", required)
	}
	if stringFromMap(mapFromValue(input["convergenceContract"]), "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want original state-changing contract preserved", input["convergenceContract"])
	}
	if mapFromValue(input["decisionResolution"]) == nil {
		t.Fatalf("decisionResolution missing from merged repair input: %+v", input)
	}
}

func TestMergeResumeInputDoesNotOverrideExistingStageWithResolutionStage(t *testing.T) {
	specs := []stepDispatchSpec{{
		ID:   "run-next-step-merge/next",
		Kind: llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeySoulStage:        soulStageExecution,
			stepInputKeyRuntimeStage:     runtimeStageExecution,
			stepInputKeyRuntimeStageMode: "strict_convergence",
		},
	}}
	resumeInput := map[string]any{
		stepInputKeySoulStage:        soulStageDecision,
		stepInputKeyRuntimeStage:     runtimeStageDecision,
		stepInputKeyRuntimeStageMode: "structured_governance",
		"decisionResolution": map[string]any{
			"resolutionId": "resolution-next-step-merge",
			"answer":       "continue",
		},
		"ticketContext": map[string]any{
			"question": "continue?",
		},
	}

	mergeResumeInput(specs, resumeInput)

	input := specs[0].Input
	if input[stepInputKeySoulStage] != soulStageExecution ||
		input[stepInputKeyRuntimeStage] != runtimeStageExecution ||
		input[stepInputKeyRuntimeStageMode] != "strict_convergence" {
		t.Fatalf("merged input = %+v, want existing execution stage preserved", input)
	}
	if mapFromValue(input["decisionResolution"]) == nil || mapFromValue(input["ticketContext"]) == nil {
		t.Fatalf("merged input = %+v, want governance envelope merged", input)
	}
}

func TestRuntimeOrchestrator_ResumeRunClearsResolvedDecisionBeforeGapBaselineRefresh(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()

	workspaceID := common.WorkspaceID("ws-resume-gap-baseline")
	runID := common.RunID("run-resume-gap-baseline")
	stepID := common.StepID("run-resume-gap-baseline/step-1")
	nextStepID := common.StepID("run-resume-gap-baseline/llm-after-decision")
	ticketID := common.TicketID("ticket-resume-gap-baseline")
	var runtimeObservedState common.RunState
	var runtimeObservedTicket common.TicketID
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		if request.StepID != nextStepID {
			return map[string]any{"status": "unexpected step"}, nil
		}
		current, err := runRepo.Get(ctx, runID)
		if err != nil {
			return nil, err
		}
		runtimeObservedState = current.State
		runtimeObservedTicket = current.BlockingTicketID
		return map[string]any{"status": "ok"}, nil
	}}
	orch := NewRuntimeOrchestratorWithResolutions(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		events.NewMemoryHub(),
	)

	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateBlocked, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	coverageItem := map[string]any{
		"id":                    "cwi-resume-1",
		"source":                "goal",
		"sourcePath":            "goal.leaves[0]",
		"sourceScope":           "goal_leaf",
		"sourceRequirementId":   "goal-resume-1",
		"targetText":            "resolved decision continuation",
		"status":                "open",
		"requiredEvidenceKinds": []string{"validation"},
		"missingEvidenceKinds":  []string{"validation"},
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
		InitialStepKind:  llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			coverageWorkItemsKey: []map[string]any{coverageItem},
			coverageWorkItemContextKey: map[string]any{
				"definitionRevision":    "rev-resume-gap-baseline",
				"coverageBaselineHash":  "sha256:resume-gap-baseline",
				"unresolvedItems":       []map[string]any{coverageItem},
				"currentFocusItems":     []map[string]any{coverageItem},
				"currentFocus":          coverageItem,
				"coverageWorkItemCount": 1,
			},
		},
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateCompleted,
		Output: map[string]any{
			"status":   "completed",
			"nextStep": string(nextStepID),
			"nextKind": llmWorkspaceExecutionStepKind,
		},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      stepID,
		State:       common.DecisionTicketStateResolved,
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-resume-gap-baseline"),
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}

	if runtimeObservedState != common.RunStateRunning || runtimeObservedTicket != "" {
		t.Fatalf("runtime observed run state=%s ticket=%q, want running with no blocker", runtimeObservedState, runtimeObservedTicket)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted || runItem.BlockingTicketID != "" {
		t.Fatalf("run = %+v, want completed with cleared decision blocker", runItem)
	}
	nextStep, err := stepRepo.Get(ctx, nextStepID)
	if err != nil {
		t.Fatalf("get next step: %v", err)
	}
	if nextStep.State != common.StepStateCompleted || nextStep.Output["status"] == "ignored" {
		t.Fatalf("next step = %+v, want accepted completed output", nextStep)
	}
}

func TestRuntimeOrchestrator_ResumeContextGateResolutionRegeneratesRepairContext(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status":          "completed",
		"completionState": "completed",
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": true,
			"summary":   "context gate retry completed",
			"items": []map[string]any{{
				"item":         "context gate retry",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-context-gate-resume/follow-up-read-config-context-gate-retry"},
			}},
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutions(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-context-gate-resume")
	runID := common.RunID("run-context-gate-resume")
	failedStepID := common.StepID(string(runID) + "/follow-up-read-config")
	ticketID := common.TicketID(string(failedStepID) + "-context-integrity-decision")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateWaitingDecision, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		GoalID:           "goal-context-gate-resume",
		DoDID:            "dod-context-gate-resume",
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	failedInput := map[string]any{
		stepInputKeyRuntimeStage:      runtimeStageRepair,
		stepInputKeyRuntimeStageMode:  "strict_convergence",
		stepInputKeySoulStage:         soulStageExecution,
		"repairObjective":             repairObjectiveStateChangingOutput,
		"stateChangingOutputRequired": true,
		"currentProgressDigest": map[string]any{
			"currentGap": "none_observed_in_recent_process_evidence",
		},
		"repairActionPackage": map[string]any{
			"failedStepId":            string(runID) + "/repair-001",
			"failedStepKind":          llmWorkspaceExecutionStepKind,
			failureOutputKeyClass:     failureClassLLMDecisionInvalid,
			failureOutputKeySignature: "class=llm_decision_invalid|kind=llm/workspace_execution|stage=llm_schema_invalid",
			"requiredOutput": map[string]any{
				"mustIncludeStateChangingNode": true,
			},
		},
		stepInputKeyExecutionContextPackage: map[string]any{
			"replanContinuation": map[string]any{
				"currentProgressDigest": map[string]any{
					"currentGap":         "invalid llm decision json",
					"nextRequiredAction": "address_last_failure_under_current_stage_contract",
				},
			},
		},
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: failedInput,
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "llm_context_integrity_failed",
			"error":                   "invalid argument: llm context gate fail_fast: runtime_repair_current_failure_missing",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_context_integrity_failed",
			"contextBudget": map[string]any{
				"contextGateAction": "fail_fast",
				"contextGateReason": "runtime_repair_current_failure_missing",
			},
		},
	}); err != nil {
		t.Fatalf("seed failed context gate step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      failedStepID,
		State:       common.DecisionTicketStateResolved,
		Context: map[string]any{
			decisionContextKeyDependency:     decisionContextDependencyContextGate,
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyContext,
			"failedStepId":                   string(failedStepID),
			"failedStepKind":                 llmWorkspaceExecutionStepKind,
			"failedStepInput":                failedInput,
			"failedStepOutput": map[string]any{
				"stage":                   "llm_context_integrity_failed",
				"error":                   "invalid argument: llm context gate fail_fast: runtime_repair_current_failure_missing",
				failureOutputKeyClass:     failureClassRuntime,
				failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_context_integrity_failed",
			},
		},
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       "resolution-context-gate-resume",
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"answer": "继续处理", "recoveryAction": "retry_same_step"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume context gate resolution: %v", err)
	}
	if len(runtime.calls) == 0 || !strings.Contains(string(runtime.calls[0]), "context-gate-retry") {
		t.Fatalf("runtime calls = %#v, want regenerated context-gate retry step", runtime.calls)
	}
	retry, err := stepRepo.Get(ctx, runtime.calls[0])
	if err != nil {
		t.Fatalf("get retry step: %v", err)
	}
	pkg := mapFromValue(retry.Input["repairActionPackage"])
	currentFailure := mapFromValue(pkg["currentFailure"])
	if currentFailure["failedStepId"] != string(runID)+"/repair-001" ||
		currentFailure["failureClass"] != failureClassLLMDecisionInvalid {
		t.Fatalf("currentFailure = %+v, want original repair failure", currentFailure)
	}
	progress := mapFromValue(pkg["currentExecutionProgress"])
	if progress["currentGap"] != "invalid llm decision json" {
		t.Fatalf("currentExecutionProgress = %+v, want original progress gap", progress)
	}
	if retry.Input["contextGateRecovery"] == nil {
		t.Fatalf("retry input = %+v, want contextGateRecovery marker", retry.Input)
	}
}

func TestRuntimeOrchestrator_ResumeRepairPlanRetryDispatchesRuntimeRepair(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &requestCapturingRuntimeAdapter{fn: func(request runtimemodel.StepExecutionRequest) map[string]any {
		return map[string]any{
			"status": "completed",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"summary":   "decision-authorized repair retry completed",
				"items": []map[string]any{
					{
						"item":         "decision-authorized repair retry",
						"state":        "satisfied",
						"evidenceRefs": []string{string(request.StepID)},
					},
				},
			},
		}
	}}
	orch := NewRuntimeOrchestratorWithResolutions(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-repair-plan-retry")
	runID := common.RunID("run-repair-plan-retry")
	failedStepID := common.StepID(string(runID) + "/repair-010-of-verify")
	ticketID := common.TicketID(string(failedStepID) + "-repair-exhausted-decision")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateWaitingDecision, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		DoDID:            "dod-repair-plan-retry",
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    failedStepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeRepairAttempt:  defaultMaxRuntimeRepairAttempts,
			stepInputKeyRepairOfRootStepID:    string(runID) + "/step-1",
			stepInputKeyRepairOfFailureClass:  failureClassRuntime,
			stepInputKeyRepairOfFailureSig:    "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|target=validation",
		},
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "llm_execution_graph_plan",
			"error":                   "runtime repair exhausted",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|target=validation",
		},
	}); err != nil {
		t.Fatalf("seed failed repair step: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      failedStepID,
		State:       common.DecisionTicketStateResolved,
		Context: map[string]any{
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
			"attempts":                       defaultMaxRuntimeRepairAttempts,
			"failedStepId":                   string(failedStepID),
			"failureClass":                   failureClassRuntime,
			"failureSignature":               "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|target=validation",
		},
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       "resolution-repair-plan-retry",
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload:  map[string]any{"answer": "继续处理", "recoveryAction": "retry_same_step"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume repair plan retry: %v", err)
	}
	if len(runtime.requests) != 1 {
		t.Fatalf("runtime requests = %+v, want one decision-authorized repair retry", runtime.requests)
	}
	input := runtime.requests[0].Input
	if input["decisionAuthorizedRetry"] != true ||
		input[stepInputKeyRuntimeRepairFollowUp] != true ||
		input[stepInputKeyRuntimeStage] != runtimeStageRepair {
		t.Fatalf("retry input = %+v, want decision-authorized runtime repair", input)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State == common.RunStateFailed || runItem.FailureReason == "conflict" {
		t.Fatalf("run = %+v, want no conflict terminal failure", runItem)
	}
}

func TestRuntimeOrchestrator_ResumeRunRejectsDecisionWithoutDispatchingStep(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-decision-reject")
	runID := common.RunID("run-decision-reject")
	ticketID := common.TicketID("ticket-decision-reject")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateBlockedOnDecision, BlockingTicketID: ticketID}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{ID: ticketID, WorkspaceID: workspaceID, RunID: runID, State: common.DecisionTicketStateResolved}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-decision-reject"),
		TicketID: ticketID,
		Type:     "reject_current_path",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume rejected decision: %v", err)
	}

	r, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if r.State != common.RunStateFailed {
		t.Fatalf("run state = %q, want failed", r.State)
	}
	if r.BlockingTicketID != "" || r.BlockingApprovalID != "" {
		t.Fatalf("blocking refs = ticket=%q approval=%q, want empty", r.BlockingTicketID, r.BlockingApprovalID)
	}
	if _, err := stepRepo.Get(ctx, common.StepID(string(runID)+"/step-1")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("expected no dispatched step, got %v", err)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("runtime calls = %#v, want none", runtime.calls)
	}
	last := hub.Published()[len(hub.Published())-1]
	if last.Topic != workspaceevents.TopicWorkspaceStateChanged || last.Payload[events.PayloadKeyActiveRunID] != "" {
		t.Fatalf("last event = %#v, want workspace release event", last)
	}
}

func TestRuntimeOrchestrator_ResumeRunReplanSupersedesRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "completed",
		"dodAssessment": map[string]any{
			"satisfied": true,
			"summary":   "replan successor completed",
			"items": []map[string]any{{
				"item":         "successor run",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-decision-replan-replan-001/step-1"},
			}},
		},
	}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{Results: resultRepo},
		hub,
	)

	workspaceID := common.WorkspaceID("ws-decision-replan")
	runID := common.RunID("run-decision-replan")
	ticketID := common.TicketID("ticket-decision-replan")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		GoalID:           "goal-decision-replan",
		DoDID:            "dod-decision-replan",
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{"compressedContext": map[string]any{"runId": string(runID)}},
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{ID: ticketID, WorkspaceID: workspaceID, RunID: runID, State: common.DecisionTicketStateResolved}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-decision-replan"),
		TicketID: ticketID,
		Type:     "replan",
		Payload:  map[string]any{"operator": "tester"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume replan decision: %v", err)
	}

	r, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if r.State != common.RunStateSuperseded {
		t.Fatalf("run state = %q, want superseded", r.State)
	}
	if r.BlockingTicketID != "" || r.BlockingApprovalID != "" {
		t.Fatalf("blocking refs = ticket=%q approval=%q, want empty", r.BlockingTicketID, r.BlockingApprovalID)
	}
	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.ActiveRunID != "" {
		t.Fatalf("workspace active run = %q, want empty after completed successor", ws.ActiveRunID)
	}
	successor, err := runRepo.Get(ctx, "run-decision-replan-replan-001")
	if err != nil {
		t.Fatalf("get successor run: %v", err)
	}
	if successor.State != common.RunStateCompleted ||
		successor.WorkspaceID != workspaceID ||
		successor.GoalID != "goal-decision-replan" ||
		successor.DoDID != "dod-decision-replan" ||
		mapFromValue(successor.ExecutionContext["replanContinuation"])["predecessorRunId"] != string(runID) {
		t.Fatalf("successor = %+v, want completed replan continuation with inherited definitions/context", successor)
	}
	if _, ok := successor.ExecutionContext["compressedContext"]; ok {
		compressed := mapFromValue(successor.ExecutionContext["compressedContext"])
		if compressed["runId"] != string(successor.ID) || compressed["workspaceId"] != string(workspaceID) {
			t.Fatalf("successor compressedContext = %+v, want successor run identity", compressed)
		}
	} else {
		t.Fatalf("successor missing compressedContext")
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != "run-decision-replan-replan-001/step-1" {
		t.Fatalf("runtime calls = %#v, want successor step dispatched", runtime.calls)
	}
	if !publishedRunCreatedFor(hub.Published(), "run-decision-replan-replan-001") {
		t.Fatalf("published events = %+v, want run-created event for successor", hub.Published())
	}
}

func TestRuntimeOrchestrator_ReplanSuccessorRecoversConcreteFailureFromAncestorLineage(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &requestCapturingRuntimeAdapter{output: map[string]any{
		"status": "completed",
		"dodAssessment": map[string]any{
			"satisfied": true,
			"summary":   "captured",
			"items": []map[string]any{{
				"item":         "captured",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-wrapper-replan-replan-001/step-1"},
			}},
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutions(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, resolutionRepo, approvalRepo, events.NewMemoryHub())

	workspaceID := common.WorkspaceID("ws-replan-ancestor-primary")
	rootRunID := common.RunID("run-root-primary")
	runID := common.RunID("run-wrapper-replan")
	ticketID := common.TicketID("ticket-wrapper-replan")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:          rootRunID,
		WorkspaceID: workspaceID,
		State:       common.RunStateSuperseded,
		ExecutionContext: map[string]any{
			"replanContinuation": map[string]any{
				"predecessorRunId": "run-before-root",
				stepInputKeyProcessEvidenceTimeline: []map[string]any{{
					"stepId":                    "run-before-root/graph-001/009-build",
					"kind":                      shellcommon.StepKindCommand,
					"state":                     string(common.StepStateFailed),
					"failureClass":              failureClassShellCommand,
					"failureSignature":          "class=shell_command_failed|kind=shell/command|failureTargets=pkg/service.go,cmd/main.go|exitCode=1",
					"failureReason":             "pkg/service.go:10:2: undefined: Service\ncmd/main.go:12:8: undefined: NewServer",
					"concreteFailureDiagnostic": "pkg/service.go:10:2: undefined: Service\ncmd/main.go:12:8: undefined: NewServer",
					"evidenceValidity":          "actionable_failure",
				}},
			},
		},
	}); err != nil {
		t.Fatalf("seed root run: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
			"rejectedResultContinuation": map[string]any{
				"summary":          "generic rejected result gap",
				"requiredEvidence": []string{"produce accepted evidence"},
			},
			"replanContinuation": map[string]any{
				"predecessorRunId": string(rootRunID),
				stepInputKeyCurrentProgressDigest: map[string]any{
					"currentGap": "invalid argument: state-changing llm repair step returned executionGraph without patch/apply",
					"lastObservedFailure": map[string]any{
						"stepId":           string(runID) + "/repair-003",
						"kind":             llmcommon.StepKindWorkspaceExecution,
						"failureClass":     failureClassRuntime,
						"failureReason":    "invalid argument: state-changing llm repair step returned executionGraph without patch/apply",
						"evidenceValidity": "actionable_gap",
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{ID: ticketID, WorkspaceID: workspaceID, RunID: runID, State: common.DecisionTicketStateResolved}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-wrapper-replan"),
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeReplan,
		Payload:  map[string]any{"operator": "tester", "recoveryAction": "replan"},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume replan: %v", err)
	}
	if len(runtime.requests) != 1 {
		t.Fatalf("runtime requests = %d, want one successor initial step", len(runtime.requests))
	}
	input := runtime.requests[0].Input
	currentFailure := mapFromValue(input["currentFailure"])
	if currentFailure["failureClass"] != failureClassShellCommand ||
		currentFailure["failedStepId"] != "run-before-root/graph-001/009-build" ||
		!strings.Contains(stringFromMap(currentFailure, "reason"), "Service") {
		t.Fatalf("currentFailure = %+v, want ancestor concrete failure", currentFailure)
	}
	if strings.Contains(stringFromMap(currentFailure, "reason"), "without patch/apply") {
		t.Fatalf("currentFailure = %+v, want wrapper failure suppressed", currentFailure)
	}
	signals := mapFromValue(mapFromValue(input["repairActionPackage"])["structuredFailureSignals"])
	if refs := stringSliceFromValue(signals["targetRefs"]); !stringSliceContains(refs, "pkg/service.go") || !stringSliceContains(refs, "cmd/main.go") {
		t.Fatalf("structuredFailureSignals = %+v, want ancestor target refs", signals)
	}
	successor, err := runRepo.Get(ctx, "run-wrapper-replan-replan-001")
	if err != nil {
		t.Fatalf("get successor: %v", err)
	}
	replan := mapFromValue(successor.ExecutionContext["replanContinuation"])
	digest := mapFromValue(replan[stepInputKeyCurrentProgressDigest])
	if stringFromMap(mapFromValue(digest["lastObservedFailure"]), "failureClass") != failureClassShellCommand {
		t.Fatalf("successor replan digest = %+v, want inherited concrete failure persisted", digest)
	}
}

func TestRuntimeOrchestrator_ResumeRunLegacyRepairResolutionPayloadReplans(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	resolutionRepo := memory.NewDecisionResolutionRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "completed",
		"dodAssessment": map[string]any{
			"satisfied": true,
			"summary":   "legacy repair replan successor completed",
			"items": []map[string]any{{
				"item":         "legacy successor run",
				"state":        "satisfied",
				"evidenceRefs": []string{"run-legacy-repair-replan-replan-001/step-1"},
			}},
		},
	}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		resolutionRepo,
		approvalRepo,
		RuntimeEvidenceRepositories{Results: resultRepo},
		hub,
	)

	workspaceID := common.WorkspaceID("ws-legacy-repair-replan")
	runID := common.RunID("run-legacy-repair-replan")
	ticketID := common.TicketID("ticket-legacy-repair-replan")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:               runID,
		WorkspaceID:      workspaceID,
		GoalID:           "goal-legacy-repair-replan",
		DoDID:            "dod-legacy-repair-replan",
		InitialStepKind:  llmcommon.StepKindWorkspaceExecution,
		State:            common.RunStateBlockedOnDecision,
		BlockingTicketID: ticketID,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := ticketRepo.Create(ctx, &decisionmodel.Ticket{
		ID:          ticketID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		State:       common.DecisionTicketStateResolved,
		Context: map[string]any{
			decisionContextKeyRecoveryPolicy: decisionContextRecoveryPolicyRepairPlan,
			"failureClass":                   failureClassShellCommand,
			"failureSignature":               "class=shell_command_failed|kind=shell/command|failureTargets=handler.go|exitCode=1",
			"failedStepOutput": map[string]any{
				"error": "exit status 1",
			},
		},
	}); err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	if err := resolutionRepo.Create(ctx, &decisionmodel.Resolution{
		ID:       common.ResolutionID("resolution-legacy-repair-replan"),
		TicketID: ticketID,
		Type:     decisioncommon.ResolutionTypeProvideMissingInfo,
		Payload: map[string]any{
			"answer":         "replan",
			"recoveryAction": "replan",
		},
	}); err != nil {
		t.Fatalf("seed resolution: %v", err)
	}

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume legacy repair replan decision: %v", err)
	}

	r, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if r.State != common.RunStateSuperseded {
		t.Fatalf("run state = %q, want superseded", r.State)
	}
	ws, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if ws.ActiveRunID != "" {
		t.Fatalf("workspace active run = %q, want empty after completed successor", ws.ActiveRunID)
	}
	successor, err := runRepo.Get(ctx, "run-legacy-repair-replan-replan-001")
	if err != nil {
		t.Fatalf("get successor run: %v", err)
	}
	if successor.State != common.RunStateCompleted ||
		successor.WorkspaceID != workspaceID ||
		successor.GoalID != "goal-legacy-repair-replan" ||
		successor.DoDID != "dod-legacy-repair-replan" ||
		mapFromValue(successor.ExecutionContext["replanContinuation"])["predecessorRunId"] != string(runID) {
		t.Fatalf("successor = %+v, want completed replan continuation with inherited definitions/context", successor)
	}
	replanContext := mapFromValue(successor.ExecutionContext["replanContinuation"])
	ticketContext := mapFromValue(replanContext["ticketContext"])
	if ticketContext["failureSignature"] != "class=shell_command_failed|kind=shell/command|failureTargets=handler.go|exitCode=1" ||
		mapFromValue(ticketContext["failedStepOutput"])["error"] != "exit status 1" {
		t.Fatalf("replan ticketContext = %+v, want failure evidence carried to successor", ticketContext)
	}
	if len(runtime.calls) != 1 || runtime.calls[0] != "run-legacy-repair-replan-replan-001/step-1" {
		t.Fatalf("runtime calls = %#v, want successor step dispatched", runtime.calls)
	}
}

func publishedRunCreatedFor(items []events.Envelope, runID common.RunID) bool {
	for _, item := range items {
		if item.Topic == runevents.TopicRunCreated && item.RunID == string(runID) {
			return true
		}
	}
	return false
}

func TestRuntimeOrchestrator_ResumeRunRequiresResolvedApproval(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-1")
	runID := common.RunID("run-1")
	approvalID := common.ApprovalID("approval-1")
	_ = workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID})
	_ = runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateBlockedOnApproval, BlockingApprovalID: approvalID})
	_ = approvalRepo.Create(ctx, &approvalmodel.Approval{ID: approvalID, WorkspaceID: workspaceID, RunID: runID, State: common.ApprovalStatePending})

	if err := orch.ResumeRun(ctx, runID); !errors.Is(err, common.ErrBlocked) {
		t.Fatalf("resume blocked by pending approval err = %v, want blocked", err)
	}

	approval, _ := approvalRepo.Get(ctx, approvalID)
	approval.State = common.ApprovalStateApproved
	_ = approvalRepo.Update(ctx, approval)

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after approval resolved: %v", err)
	}

	r, _ := runRepo.Get(ctx, runID)
	if r.State != common.RunStateCompleted {
		t.Fatalf("run state = %q, want completed", r.State)
	}
	if r.BlockingApprovalID != "" {
		t.Fatalf("blocking approval = %q, want empty", r.BlockingApprovalID)
	}
}

func TestRuntimeOrchestrator_ResumeRunRejectedApprovalFailsRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "ok"}}
	hub := events.NewMemoryHub()
	orch := NewRuntimeOrchestrator(runtime, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, hub)

	workspaceID := common.WorkspaceID("ws-1")
	runID := common.RunID("run-1")
	approvalID := common.ApprovalID("approval-1")
	_ = workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID})
	_ = runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateBlockedOnApproval, BlockingApprovalID: approvalID})
	_ = approvalRepo.Create(ctx, &approvalmodel.Approval{ID: approvalID, WorkspaceID: workspaceID, RunID: runID, State: common.ApprovalStateRejected})

	if err := orch.ResumeRun(ctx, runID); err != nil {
		t.Fatalf("resume after approval rejection should fail run in-repo, got err: %v", err)
	}

	r, _ := runRepo.Get(ctx, runID)
	if r.State != common.RunStateFailed {
		t.Fatalf("run state = %q, want failed", r.State)
	}
	if r.BlockingApprovalID != "" {
		t.Fatalf("blocking approval = %q, want empty", r.BlockingApprovalID)
	}

	ws, _ := workspaceRepo.Get(ctx, workspaceID)
	if ws.ActiveRunID != "" {
		t.Fatalf("workspace active run id = %q, want empty", ws.ActiveRunID)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("runtime calls = %#v, want no execution after rejection", runtime.calls)
	}
}

func TestRuntimeOrchestrator_ResumeRunRejectsMissingBlockingReference(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-missing-ref")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	for _, tc := range []struct {
		name string
		run  *runmodel.Run
	}{
		{
			name: "decision missing ticket",
			run:  &runmodel.Run{ID: common.RunID("run-missing-ticket"), WorkspaceID: workspaceID, State: common.RunStateBlockedOnDecision},
		},
		{
			name: "approval missing approval",
			run:  &runmodel.Run{ID: common.RunID("run-missing-approval"), WorkspaceID: workspaceID, State: common.RunStateBlockedOnApproval},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := runRepo.Create(ctx, tc.run); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := orch.ResumeRun(ctx, tc.run.ID); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("resume err = %v, want conflict", err)
			}
		})
	}
}

func TestRuntimeOrchestrator_CompleteRunAndFailRun_ValidateArguments(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	if err := orch.CompleteRun(ctx, ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("complete empty run err = %v, want invalid argument", err)
	}
	if err := orch.FailRun(ctx, "", "reason"); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("fail empty run err = %v, want invalid argument", err)
	}
	if err := orch.FailRun(ctx, common.RunID("run-1"), ""); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("fail empty reason err = %v, want invalid argument", err)
	}
}

func TestRuntimeOrchestrator_CompleteRunRejectsNonRunningState(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-complete-conflict")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	for _, tc := range []struct {
		name string
		run  *runmodel.Run
	}{
		{name: "created", run: &runmodel.Run{ID: common.RunID("run-complete-created"), WorkspaceID: workspaceID, State: common.RunStateCreated}},
		{name: "blocked decision", run: &runmodel.Run{ID: common.RunID("run-complete-blocked-decision"), WorkspaceID: workspaceID, State: common.RunStateBlockedOnDecision, BlockingTicketID: common.TicketID("ticket-1")}},
		{name: "blocked approval", run: &runmodel.Run{ID: common.RunID("run-complete-blocked-approval"), WorkspaceID: workspaceID, State: common.RunStateBlockedOnApproval, BlockingApprovalID: common.ApprovalID("approval-1")}},
		{name: "completed", run: &runmodel.Run{ID: common.RunID("run-complete-completed"), WorkspaceID: workspaceID, State: common.RunStateCompleted}},
		{name: "failed", run: &runmodel.Run{ID: common.RunID("run-complete-failed"), WorkspaceID: workspaceID, State: common.RunStateFailed}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := runRepo.Create(ctx, tc.run); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := orch.CompleteRun(ctx, tc.run.ID); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("complete run err = %v, want conflict", err)
			}
			got, err := runRepo.Get(ctx, tc.run.ID)
			if err != nil {
				t.Fatalf("get run: %v", err)
			}
			if !reflect.DeepEqual(got, tc.run) {
				t.Fatalf("run mutated after conflict: got=%+v want=%+v", *got, *tc.run)
			}
		})
	}
}

func TestRuntimeOrchestrator_CompleteRunRequiresAcceptedResultForDoDGatedRun(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{output: map[string]any{"status": "ok"}},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{Results: resultRepo},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-complete-result-gate")
	runID := common.RunID("run-complete-result-gate")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:          workspaceID,
		Title:       "ws",
		State:       common.WorkspaceStateRunning,
		ActiveRunID: runID,
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-complete-result-gate",
		State:           common.RunStateValidating,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.CompleteRun(ctx, runID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("complete without accepted result err = %v, want conflict", err)
	}
	got, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run after rejected complete: %v", err)
	}
	if got.State != common.RunStateValidating {
		t.Fatalf("run state = %q, want validating after rejected complete", got.State)
	}

	if err := resultRepo.Create(ctx, &resultmodel.Result{
		ID:          "run-complete-result-gate/result/dod-assessment",
		WorkspaceID: workspaceID,
		RunID:       runID,
		DoDID:       "dod-complete-result-gate",
		State:       common.ResultStateAccepted,
		Summary:     "accepted result exists but is not a formal DoD assessment",
	}); err != nil {
		t.Fatalf("seed weak accepted result: %v", err)
	}
	if err := orch.CompleteRun(ctx, runID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("complete with weak accepted result err = %v, want conflict", err)
	}
	if err := resultRepo.Update(ctx, &resultmodel.Result{
		ID:           "run-complete-result-gate/result/dod-assessment",
		WorkspaceID:  workspaceID,
		RunID:        runID,
		DoDID:        "dod-complete-result-gate",
		State:        common.ResultStateAccepted,
		Summary:      "formal DoD assessment accepted",
		SourceType:   resultcommon.SourceTypeDoDAssessment,
		SourceRef:    "run-complete-result-gate/step-1",
		EvidenceRefs: []string{"run-complete-result-gate/step-1/validation/runtime-step"},
		DoDAssessment: resultmodel.DoDAssessment{
			Satisfied: true,
			Summary:   "formal DoD assessment accepted",
			Items: []resultmodel.DoDAssessmentItem{{
				Item:         "stdio MCP tool call completes",
				State:        "satisfied",
				EvidenceRefs: []string{"run-complete-result-gate/step-1/validation/runtime-step"},
			}},
		},
	}); err != nil {
		t.Fatalf("upgrade formal accepted result: %v", err)
	}
	if err := orch.CompleteRun(ctx, runID); err != nil {
		t.Fatalf("complete with formal accepted result: %v", err)
	}
	got, err = runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get completed run: %v", err)
	}
	if got.State != common.RunStateCompleted {
		t.Fatalf("run state = %q, want completed", got.State)
	}
}

func TestRuntimeOrchestrator_FailRunRejectsTerminalState(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	orch := NewRuntimeOrchestrator(&stubRuntimeAdapter{output: map[string]any{"status": "ok"}}, runRepo, stepRepo, workspaceRepo, ticketRepo, approvalRepo, nil)

	workspaceID := common.WorkspaceID("ws-fail-conflict")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	for _, tc := range []struct {
		name string
		run  *runmodel.Run
	}{
		{name: "completed", run: &runmodel.Run{ID: common.RunID("run-fail-completed"), WorkspaceID: workspaceID, State: common.RunStateCompleted}},
		{name: "failed", run: &runmodel.Run{ID: common.RunID("run-fail-failed"), WorkspaceID: workspaceID, State: common.RunStateFailed}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := runRepo.Create(ctx, tc.run); err != nil {
				t.Fatalf("seed run: %v", err)
			}
			if err := orch.FailRun(ctx, tc.run.ID, "manual fail"); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("fail run err = %v, want conflict", err)
			}
			got, err := runRepo.Get(ctx, tc.run.ID)
			if err != nil {
				t.Fatalf("get run: %v", err)
			}
			if !reflect.DeepEqual(got, tc.run) {
				t.Fatalf("run mutated after conflict: got=%+v want=%+v", *got, *tc.run)
			}
		})
	}
}

func hasRunCheckpointState(items []*runmodel.RuntimeCheckpoint, state common.RunState) bool {
	for _, item := range items {
		if item != nil && item.RunStateSnapshot == state {
			return true
		}
	}
	return false
}

func hasCheckpointEvent(items []*runmodel.RuntimeCheckpoint, event string) bool {
	for _, item := range items {
		if item == nil || item.Data == nil {
			continue
		}
		if item.Data["event"] == event {
			return true
		}
	}
	return false
}

func hasCheckpointGraphBranch(items []*runmodel.RuntimeCheckpoint, branch string) bool {
	for _, item := range items {
		if item == nil || item.Data == nil {
			continue
		}
		if item.Data["graphBranch"] == branch {
			return true
		}
	}
	return false
}

func hasCheckpointFailureClass(items []*runmodel.RuntimeCheckpoint, class string) bool {
	for _, item := range items {
		if item == nil || item.Data == nil {
			continue
		}
		if item.Data["failureClass"] == class {
			return true
		}
	}
	return false
}

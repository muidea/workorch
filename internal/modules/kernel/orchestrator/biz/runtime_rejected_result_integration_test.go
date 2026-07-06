package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	resultcommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestRuntimeOrchestrator_RecordsRejectedResultForUnsatisfiedDoDAssessment(t *testing.T) {
	ctx := context.Background()
	resultRepo := memory.NewResultRepository()
	orch := &RuntimeOrchestrator{evidence: RuntimeEvidenceRepositories{Results: resultRepo}}
	runID := common.RunID("run-rejected-dod")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-rejected-dod",
		GoalID:          "goal-rejected-dod",
		DoDID:           "dod-rejected-dod",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{ID: common.StepID(string(runID) + "/step-1"), RunID: runID}

	if err := orch.resultManager().recordRejectedCompletionDoDAssessment(ctx, run, step, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": false,
			"summary":   "DoD not satisfied; missing effective tests.",
			"items": []map[string]any{
				{
					"item":          "effective tests",
					"state":         "failed",
					"failureReason": "no effective assertions",
				},
			},
		},
	}); err != nil {
		t.Fatalf("record rejected result: %v", err)
	}

	result, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment"))
	if err != nil {
		t.Fatalf("get rejected result: %v", err)
	}
	if result.State != common.ResultStateRejected || result.Summary != "DoD not satisfied; missing effective tests." {
		t.Fatalf("result = %+v, want rejected DoD result", result)
	}
	if result.DoDAssessment.Satisfied {
		t.Fatalf("DoDAssessment = %+v, want unsatisfied", result.DoDAssessment)
	}
	if len(result.DoDAssessment.Items) != 1 ||
		!reflect.DeepEqual(result.DoDAssessment.Items[0].EvidenceRefs, []string{string(step.ID)}) {
		t.Fatalf("DoDAssessment items = %+v, want failed item to cite fallback step evidence", result.DoDAssessment.Items)
	}
}

func TestRuntimeOrchestrator_FailRunPrefersUnsatisfiedDoDSummary(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	resultRepo := memory.NewResultRepository()
	runID := common.RunID("run-unsatisfied-dod-summary-failure")
	workspaceID := common.WorkspaceID("ws-unsatisfied-dod-summary-failure")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-unsatisfied-dod-summary-failure",
		State:           common.RunStateActive,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runs:       runRepo,
		workspaces: workspaceRepo,
		evidence:   RuntimeEvidenceRepositories{Results: resultRepo},
	}
	summary := "无法完成目标，因为所需外部能力当前不可用。"
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/repair-001"),
		RunID: runID,
		Output: map[string]any{
			"error": "invalid argument: completionState failed has no actionable continuation",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   summary,
				"items": []map[string]any{{
					"item":          "produce requested result",
					"state":         "failed",
					"failureReason": "external capability unavailable",
				}},
			},
		},
	}

	if err := orch.failRunWithRejectedCompletionResult(ctx, run, step, "invalid argument: completionState failed has no actionable continuation"); err != nil {
		t.Fatalf("failRunWithRejectedCompletionResult: %v", err)
	}
	closed, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if closed.State != common.RunStateFailed || closed.FailureReason != summary {
		t.Fatalf("run = %+v, want failed with DoD summary reason %q", closed, summary)
	}
}

func TestRuntimeOrchestrator_RecordsRejectedResultForWeakSatisfiedDoDAssessment(t *testing.T) {
	ctx := context.Background()
	resultRepo := memory.NewResultRepository()
	orch := &RuntimeOrchestrator{evidence: RuntimeEvidenceRepositories{Results: resultRepo}}
	runID := common.RunID("run-rejected-weak-dod")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-rejected-weak-dod",
		GoalID:          "goal-rejected-weak-dod",
		DoDID:           "dod-rejected-weak-dod",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{ID: common.StepID(string(runID) + "/step-1"), RunID: runID}

	if err := orch.resultManager().recordRejectedCompletionDoDAssessment(ctx, run, step, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied":           true,
			"summary":             "validation completed with inconclusive evidence",
			"goalOutcome":         "done",
			"verificationSummary": "runtime adapter reported inconclusive evidence",
			"items": []map[string]any{
				{
					"item":         "tests pass",
					"state":        "satisfied",
					"evidenceRefs": []string{string(step.ID)},
				},
			},
		},
	}); err != nil {
		t.Fatalf("record rejected result: %v", err)
	}

	result, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment"))
	if err != nil {
		t.Fatalf("get rejected result: %v", err)
	}
	if result.State != common.ResultStateRejected || result.DoDAssessment.Satisfied {
		t.Fatalf("result = %+v, want rejected unsatisfied DoD result", result)
	}
	if !strings.Contains(result.Summary, "DoD assessment rejected") {
		t.Fatalf("result summary = %q, want rejected weak evidence summary", result.Summary)
	}
}

func TestRuntimeOrchestrator_RejectedResultPublishesRejectedEventID(t *testing.T) {
	ctx := context.Background()
	resultRepo := memory.NewResultRepository()
	eventHub := events.NewMemoryHub()
	orch := &RuntimeOrchestrator{
		eventHub: eventHub,
		evidence: RuntimeEvidenceRepositories{
			Results: resultRepo,
		},
	}
	runID := common.RunID("run-rejected-event-id")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-rejected-event-id",
		GoalID:          "goal-rejected-event-id",
		DoDID:           "dod-rejected-event-id",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{ID: common.StepID(string(runID) + "/assessment"), RunID: runID}

	if err := orch.resultManager().recordRejectedCompletionDoDAssessment(ctx, run, step, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": false,
			"summary":   "not satisfied",
			"items": []map[string]any{{
				"item":  "required outcome",
				"state": "failed",
			}},
		},
	}); err != nil {
		t.Fatalf("record rejected result: %v", err)
	}

	published := eventHub.Published()
	if len(published) != 1 {
		t.Fatalf("published events = %+v, want one result event", published)
	}
	if !strings.HasSuffix(published[0].ID, "/event/rejected") {
		t.Fatalf("event ID = %q, want rejected suffix", published[0].ID)
	}
	if strings.HasSuffix(published[0].ID, "/event/accepted") {
		t.Fatalf("event ID = %q, rejected result must not use accepted marker", published[0].ID)
	}
}

func TestRuntimeOrchestrator_RecordsSyntheticRejectedResultWithoutDoDAssessment(t *testing.T) {
	ctx := context.Background()
	resultRepo := memory.NewResultRepository()
	orch := &RuntimeOrchestrator{evidence: RuntimeEvidenceRepositories{Results: resultRepo}}
	runID := common.RunID("run-rejected-synthetic-dod")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-rejected-synthetic-dod",
		GoalID:          "goal-rejected-synthetic-dod",
		DoDID:           "dod-rejected-synthetic-dod",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{ID: common.StepID(string(runID) + "/block/implement"), RunID: runID}

	if err := orch.resultManager().recordRejectedCompletionDoDAssessment(ctx, run, step, map[string]any{
		"error": "state-changing execution-stage llm graph node returned no state-changing output",
	}); err != nil {
		t.Fatalf("record synthetic rejected result: %v", err)
	}

	result, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment"))
	if err != nil {
		t.Fatalf("get synthetic rejected result: %v", err)
	}
	if result.State != common.ResultStateRejected || result.DoDAssessment.Satisfied {
		t.Fatalf("result = %+v, want rejected unsatisfied synthetic DoD result", result)
	}
	if !strings.Contains(result.Summary, "no state-changing output") {
		t.Fatalf("result summary = %q, want contract failure reason", result.Summary)
	}
	if len(result.DoDAssessment.Items) == 0 || !strings.Contains(result.DoDAssessment.Items[0].Item, "runtime execution contract") {
		t.Fatalf("DoDAssessment = %+v, want synthetic runtime contract item", result.DoDAssessment)
	}
}

func TestRuntimeOrchestrator_FailRunWritesSyntheticRejectedResultForLLMDecisionContractTerminalFailure(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	resultRepo := memory.NewResultRepository()
	runID := common.RunID("run-llm-decision-contract-terminal-result")
	workspaceID := common.WorkspaceID("ws-llm-decision-contract-terminal-result")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          "goal-llm-decision-contract-terminal-result",
		DoDID:           "dod-llm-decision-contract-terminal-result",
		State:           common.RunStateActive,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runtime:    &stubRuntimeAdapter{},
		runs:       runRepo,
		steps:      stepRepo,
		workspaces: workspaceRepo,
		evidence:   RuntimeEvidenceRepositories{Results: resultRepo},
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/provider-retry-1"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{"providerRetryAssessmentOnly": true},
		Output: map[string]any{
			"status": "failed",
			"stage":  "llm_schema_invalid",
			"error":  "invalid argument: executionGraph patch/apply targetRef is required",
		},
	}

	if err := orch.dispatchRuntimeRepairFollowUp(ctx, run, failedStep, errors.New("invalid argument: executionGraph patch/apply targetRef is required")); err != nil {
		t.Fatalf("dispatchRuntimeRepairFollowUp: %v", err)
	}

	closed, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if closed.State != common.RunStateFailed {
		t.Fatalf("run state = %s, want failed", closed.State)
	}
	result, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment"))
	if err != nil {
		t.Fatalf("get synthetic rejected result: %v", err)
	}
	if result.State != common.ResultStateRejected || !strings.Contains(result.Summary, "patch/apply targetRef is required") {
		t.Fatalf("result = %+v, want rejected result with schema failure reason", result)
	}
}

func TestRuntimeOrchestrator_RejectedResultAttributesProviderRetryToTerminalStep(t *testing.T) {
	ctx := context.Background()
	resultRepo := memory.NewResultRepository()
	orch := &RuntimeOrchestrator{evidence: RuntimeEvidenceRepositories{Results: resultRepo}}
	runID := common.RunID("run-provider-retry-rejected-source")
	rootStepID := common.StepID(runID + "/follow-up-001/dod-gate")
	retryStepID := providerRetryStepID(runID, rootStepID, 1)
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-provider-retry-rejected-source",
		GoalID:          "goal-provider-retry-rejected-source",
		DoDID:           "dod-provider-retry-rejected-source",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:    retryStepID,
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryRootStep: string(rootStepID),
		},
		Output: map[string]any{
			"stage": "llm_schema_invalid",
			"error": "invalid argument: executionGraph patch/apply targetRef is required",
		},
	}

	if err := orch.resultManager().recordRejectedCompletionDoDAssessment(ctx, run, step, step.Output); err != nil {
		t.Fatalf("record rejected result: %v", err)
	}

	result, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment"))
	if err != nil {
		t.Fatalf("get rejected result: %v", err)
	}
	if result.SourceRef != string(retryStepID) {
		t.Fatalf("result sourceRef = %q, want terminal step %q", result.SourceRef, retryStepID)
	}
	if !containsString(result.EvidenceRefs, string(rootStepID)) || !containsString(result.EvidenceRefs, string(retryStepID)) {
		t.Fatalf("result evidenceRefs = %+v, want root and provider retry evidence", result.EvidenceRefs)
	}
}

func TestRuntimeOrchestrator_DispatchNextClosesRunningRunWithRejectedResult(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	resultRepo := memory.NewResultRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{"status": "completed"}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Results: resultRepo},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-rejected-result-boundary")
	runID := common.RunID("run-rejected-result-boundary")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-rejected-result-boundary",
		State:           common.RunStateActive,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     common.StepID(string(runID) + "/step-1"),
		RunID:  runID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateSucceeded,
		Output: map[string]any{"status": "completed"},
	}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if err := resultRepo.Create(ctx, &resultmodel.Result{
		ID:          common.ResultID(string(runID) + "/result/dod-assessment"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		DoDID:       "dod-rejected-result-boundary",
		State:       common.ResultStateRejected,
		Title:       "Rejected Result",
		Summary:     "DoD not satisfied; stop execution.",
		SourceType:  resultcommon.SourceTypeDoDAssessment,
		SourceRef:   string(runID) + "/step-1",
	}); err != nil {
		t.Fatalf("seed rejected result: %v", err)
	}

	err := orch.DispatchNextStep(ctx, runID)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("DispatchNextStep err = %v, want conflict after closing rejected run", err)
	}
	if len(runtime.calls) != 0 {
		t.Fatalf("runtime calls = %+v, want no dispatch after rejected result", runtime.calls)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateFailed ||
		runItem.FailureReason != "DoD not satisfied; stop execution." {
		t.Fatalf("run = %+v, want failed with rejected result summary", runItem)
	}
}

func TestRuntimeOrchestrator_RejectedResultSupersededByRecoveryProgress(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-rejected-result-superseded")
	workspaceID := common.WorkspaceID("ws-rejected-result-superseded")
	sourceStepID := common.StepID(runID + "/graph-001/003-build")
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	resultRepo := memory.NewResultRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Results: resultRepo},
		events.NewMemoryHub(),
	)
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, DoDID: "dod", State: common.RunStateActive, InitialStepKind: llmWorkspaceExecutionStepKind}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{ID: sourceStepID, RunID: runID, Kind: shellcommon.StepKindCommand, State: common.StepStateFailed, Output: map[string]any{"error": "exit status 1"}}); err != nil {
		t.Fatalf("seed source step: %v", err)
	}
	resultID := common.ResultID(runID + "/result/dod-assessment")
	if err := resultRepo.Create(ctx, &resultmodel.Result{
		ID:          resultID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		DoDID:       "dod",
		State:       common.ResultStateRejected,
		Title:       "Rejected Result",
		Summary:     "exit status 1",
		SourceType:  resultcommon.SourceTypeDoDAssessment,
		SourceRef:   string(sourceStepID),
	}); err != nil {
		t.Fatalf("seed rejected result: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001-of-graph-001-003-build"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateSucceeded,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			"failedStepId":                    string(sourceStepID),
		},
		Output: map[string]any{"status": "completed"},
	}); err != nil {
		t.Fatalf("seed repair step: %v", err)
	}

	if summary, ok := orch.resultManager().rejectedRunResultSummary(ctx, runID); ok {
		t.Fatalf("rejectedRunResultSummary = %q, true; want superseded stale result", summary)
	}
	result, err := resultRepo.Get(ctx, resultID)
	if err != nil {
		t.Fatalf("get result: %v", err)
	}
	if result.State != common.ResultStateSuperseded {
		t.Fatalf("result state = %s, want superseded", result.State)
	}
}

func TestRuntimeOrchestrator_RejectedResultFromConvertedDecisionIsDiagnosticOnly(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-rejected-result-converted-decision")
	workspaceID := common.WorkspaceID("ws-rejected-result-converted-decision")
	sourceStepID := common.StepID(runID + "/contract-retry-001")
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	resultRepo := memory.NewResultRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Results: resultRepo},
		events.NewMemoryHub(),
	)
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, DoDID: "dod", State: common.RunStateActive, InitialStepKind: llmWorkspaceExecutionStepKind}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    sourceStepID,
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			"status":                               "failed",
			"continuableDecisionConvertedToRepair": true,
			"convertedDecisionGate":                "convergence_contract",
		},
	}); err != nil {
		t.Fatalf("seed source step: %v", err)
	}
	resultID := common.ResultID(runID + "/result/dod-assessment")
	if err := resultRepo.Create(ctx, &resultmodel.Result{
		ID:          resultID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		DoDID:       "dod",
		State:       common.ResultStateRejected,
		Title:       "Rejected Result",
		Summary:     "continuable decision converted to repair",
		SourceType:  resultcommon.SourceTypeDoDAssessment,
		SourceRef:   string(sourceStepID),
	}); err != nil {
		t.Fatalf("seed rejected result: %v", err)
	}

	if summary, ok := orch.resultManager().rejectedRunResultSummary(ctx, runID); ok {
		t.Fatalf("rejectedRunResultSummary = %q, true; want converted decision diagnostic ignored", summary)
	}
	result, err := resultRepo.Get(ctx, resultID)
	if err != nil {
		t.Fatalf("get result: %v", err)
	}
	if result.State != common.ResultStateSuperseded {
		t.Fatalf("result state = %s, want superseded diagnostic result", result.State)
	}
}

func TestRuntimeOrchestrator_RejectedResultEvidenceRefDiagnosticOnly(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-rejected-result-evidence-ref-diagnostic")
	workspaceID := common.WorkspaceID("ws-rejected-result-evidence-ref-diagnostic")
	rootStepID := common.StepID(runID + "/step-1")
	diagnosticStepID := common.StepID(runID + "/contract-retry-001")
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	resultRepo := memory.NewResultRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Results: resultRepo},
		events.NewMemoryHub(),
	)
	if err := runRepo.Create(ctx, &runmodel.Run{ID: runID, WorkspaceID: workspaceID, DoDID: "dod", State: common.RunStateActive, InitialStepKind: llmWorkspaceExecutionStepKind}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{ID: rootStepID, RunID: runID, Kind: llmWorkspaceExecutionStepKind, State: common.StepStateSucceeded}); err != nil {
		t.Fatalf("seed root step: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    diagnosticStepID,
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Output: map[string]any{
			"status":                               "failed",
			"continuableDecisionConvertedToRepair": true,
			"convertedDecisionGate":                "convergence_contract",
		},
	}); err != nil {
		t.Fatalf("seed diagnostic step: %v", err)
	}
	resultID := common.ResultID(runID + "/result/dod-assessment")
	if err := resultRepo.Create(ctx, &resultmodel.Result{
		ID:           resultID,
		WorkspaceID:  workspaceID,
		RunID:        runID,
		DoDID:        "dod",
		State:        common.ResultStateRejected,
		Title:        "Rejected Result",
		Summary:      "continuable decision converted to repair",
		SourceType:   resultcommon.SourceTypeDoDAssessment,
		SourceRef:    string(rootStepID),
		EvidenceRefs: []string{string(rootStepID), string(diagnosticStepID)},
	}); err != nil {
		t.Fatalf("seed rejected result: %v", err)
	}

	if summary, ok := orch.resultManager().rejectedRunResultSummary(ctx, runID); ok {
		t.Fatalf("rejectedRunResultSummary = %q, true; want evidence-ref diagnostic ignored", summary)
	}
	result, err := resultRepo.Get(ctx, resultID)
	if err != nil {
		t.Fatalf("get result: %v", err)
	}
	if result.State != common.ResultStateSuperseded {
		t.Fatalf("result state = %s, want superseded diagnostic result", result.State)
	}
}

func TestRuntimeOrchestrator_DoesNotRecordRejectedResultForDiagnosticOnlyStep(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-skip-diagnostic-rejected-result")
	resultRepo := memory.NewResultRepository()
	orch := &RuntimeOrchestrator{evidence: RuntimeEvidenceRepositories{Results: resultRepo}}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-skip-diagnostic-rejected-result",
		GoalID:          "goal-skip-diagnostic-rejected-result",
		DoDID:           "dod-skip-diagnostic-rejected-result",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/contract-retry-001"),
		RunID: runID,
		Output: map[string]any{
			"continuableDecisionConvertedToRepair": true,
			"convertedDecisionGate":                "convergence_contract",
		},
	}

	if err := orch.resultManager().recordRejectedCompletionDoDAssessment(ctx, run, step, map[string]any{
		stepOutputKeyDoDAssessment: map[string]any{
			"satisfied": false,
			"summary":   "continuable diagnostic should not become formal result",
		},
	}); err != nil {
		t.Fatalf("record rejected result: %v", err)
	}
	if _, err := resultRepo.Get(ctx, common.ResultID(string(runID)+"/result/dod-assessment")); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("result err = %v, want not found for diagnostic-only rejected result", err)
	}
}

func TestRuntimeOrchestrator_DispatchStepRejectsActiveRejectedResult(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-dispatch-rejected-result")
	workspaceID := common.WorkspaceID("ws-dispatch-rejected-result")
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	workspaceRepo := memory.NewWorkspaceRepository()
	resultRepo := memory.NewResultRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		memory.NewDecisionTicketRepository(),
		nil,
		memory.NewApprovalRepository(),
		RuntimeEvidenceRepositories{Results: resultRepo},
		events.NewMemoryHub(),
	)
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{
		ID:                   workspaceID,
		State:                common.WorkspaceStateActive,
		DeliverableDirectory: t.TempDir(),
	}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{ID: runID, WorkspaceID: workspaceID, State: common.RunStateActive, InitialStepKind: llmWorkspaceExecutionStepKind}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := resultRepo.Create(ctx, &resultmodel.Result{
		ID:          common.ResultID(runID + "/result/rejected"),
		WorkspaceID: workspaceID,
		RunID:       runID,
		State:       common.ResultStateRejected,
		Summary:     "active rejected result",
		SourceType:  resultcommon.SourceTypeDoDAssessment,
		SourceRef:   string(runID + "/step-1"),
	}); err != nil {
		t.Fatalf("seed rejected result: %v", err)
	}

	err := orch.dispatchStepWithWorkspace(ctx, run, common.StepID(runID+"/step-2"), llmWorkspaceExecutionStepKind, map[string]any{}, nil)
	if !errors.Is(err, common.ErrConflict) || !strings.Contains(fmt.Sprint(err), "active rejected result") {
		t.Fatalf("dispatch err = %v, want rejected result conflict", err)
	}
	if steps, _ := stepRepo.ListByRun(ctx, runID); len(steps) != 0 {
		t.Fatalf("steps = %+v, want no dispatch after rejected result", steps)
	}
}

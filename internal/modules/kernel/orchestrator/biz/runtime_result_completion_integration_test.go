package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"

	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	memorycommon "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/common"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	resultcommon "github.com/muidea/workorch/internal/modules/kernel/result/pkg/common"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeOrchestrator_CompletesLLMRunWithSatisfiedDoDAssessment(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	artifactRepo := memory.NewArtifactRepository()
	validationRepo := memory.NewValidationResultRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "completed",
		"dodAssessment": map[string]any{
			"satisfied": true,
			"items": []map[string]any{
				{
					"item":         "tests pass",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-dod-gate-complete/step-1/validation/runtime-step"},
				},
			},
			"summary": "DoD satisfied",
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			Results:           resultRepo,
			Artifacts:         artifactRepo,
			ValidationResults: validationRepo,
			MemoryItems:       memoryRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-dod-gate-complete")
	runID := common.RunID("run-dod-gate-complete")
	deliverableDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write deliverable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deliverableDirectory, "README.md"), []byte("# Delivery\n"), 0o644); err != nil {
		t.Fatalf("write deliverable readme: %v", err)
	}
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: deliverableDirectory, State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-1"),
		State:           common.RunStatePending,
		InitialStepKind: "llm/workspace_execution",
		ExecutionContext: map[string]any{
			"skillIds": []string{"skill-dod-gate"},
		},
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	if err := orch.StartRun(ctx, runID); err != nil {
		t.Fatalf("start run: %v", err)
	}

	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want one accepted result", results)
	}
	result := results[0]
	if result.State != common.ResultStateAccepted || result.Summary != "DoD satisfied" {
		t.Fatalf("result = %+v, want accepted DoD result", result)
	}
	if result.WorkspaceID != workspaceID || result.RunID != runID || result.DoDID != "dod-1" {
		t.Fatalf("result refs = %+v, want workspace/run/dod refs", result)
	}
	if result.SourceType != "dod_assessment" || result.SourceRef != "run-dod-gate-complete/step-1" {
		t.Fatalf("result source = %+v, want DoD assessment step source", result)
	}
	if !reflect.DeepEqual(result.EvidenceRefs, []string{"run-dod-gate-complete/step-1", "run-dod-gate-complete/step-1/validation/runtime-step"}) {
		t.Fatalf("result evidence = %+v, want step and validation refs", result.EvidenceRefs)
	}
	if !result.DoDAssessment.Satisfied || result.DoDAssessment.Summary != "DoD satisfied" || len(result.DoDAssessment.Items) != 1 {
		t.Fatalf("result DoD assessment = %+v, want satisfied item-level assessment", result.DoDAssessment)
	}
	if result.SummaryDetail.GoalOutcome != "DoD satisfied" ||
		result.SummaryDetail.ImplementationSummary != "DoD satisfied" ||
		!reflect.DeepEqual(result.SummaryDetail.ArtifactRefs, []common.ArtifactID{
			"run-dod-gate-complete/artifact/result-summary",
			"run-dod-gate-complete/artifact/deliverable-manifest",
		}) ||
		!reflect.DeepEqual(result.SummaryDetail.EvidenceRefs, []string{"run-dod-gate-complete/step-1", "run-dod-gate-complete/step-1/validation/runtime-step"}) {
		t.Fatalf("result summary detail = %+v, want structured accepted result summary", result.SummaryDetail)
	}
	if result.DoDAssessment.Items[0].Item != "tests pass" ||
		result.DoDAssessment.Items[0].State != "satisfied" ||
		!reflect.DeepEqual(result.DoDAssessment.Items[0].EvidenceRefs, []string{"run-dod-gate-complete/step-1/validation/runtime-step"}) {
		t.Fatalf("result DoD assessment items = %+v, want item state with evidence", result.DoDAssessment.Items)
	}
	if !reflect.DeepEqual(result.ArtifactRefs, []common.ArtifactID{
		"run-dod-gate-complete/artifact/result-summary",
		"run-dod-gate-complete/artifact/deliverable-manifest",
	}) {
		t.Fatalf("result artifacts = %+v, want summary and deliverable manifest artifact refs", result.ArtifactRefs)
	}
	artifacts, err := artifactRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("artifacts = %+v, want summary and deliverable manifest artifacts", artifacts)
	}
	var summaryArtifact *resultmodel.Artifact
	var manifestArtifact *resultmodel.Artifact
	for _, artifact := range artifacts {
		switch artifact.ID {
		case "run-dod-gate-complete/artifact/result-summary":
			summaryArtifact = artifact
		case "run-dod-gate-complete/artifact/deliverable-manifest":
			manifestArtifact = artifact
		}
	}
	if summaryArtifact == nil || summaryArtifact.ResultID != result.ID || summaryArtifact.ArtifactType != "text" || summaryArtifact.State != common.ArtifactStateAvailable {
		t.Fatalf("summary artifact = %+v, want available text artifact linked to result", summaryArtifact)
	}
	if summaryArtifact.Category != resultcommon.ArtifactCategoryFinalSummary ||
		!strings.Contains(summaryArtifact.Summary, "Accepted Result Summary") ||
		!strings.Contains(summaryArtifact.Summary, "DoD satisfied") ||
		summaryArtifact.ContentType != "text/markdown; charset=utf-8" {
		t.Fatalf("summary artifact content = %+v, want DoD summary text artifact", summaryArtifact)
	}
	if manifestArtifact == nil || manifestArtifact.ResultID != result.ID || manifestArtifact.ArtifactType != "report" || manifestArtifact.State != common.ArtifactStateAvailable {
		t.Fatalf("manifest artifact = %+v, want available report artifact linked to result", manifestArtifact)
	}
	if manifestArtifact.Category != resultcommon.ArtifactCategoryRuntimeEvidence {
		t.Fatalf("manifest artifact category = %q, want runtime evidence", manifestArtifact.Category)
	}
	if !strings.Contains(manifestArtifact.Summary, "Workspace Deliverable Manifest") ||
		!strings.Contains(manifestArtifact.Summary, "No workspace file deliverables were produced in this run.") ||
		strings.Contains(manifestArtifact.Summary, "| `main.go` | source |") ||
		strings.Contains(manifestArtifact.Summary, "| `README.md` | documentation |") ||
		!strings.Contains(manifestArtifact.Summary, "## DoD Relation") ||
		!strings.Contains(manifestArtifact.Summary, "run-dod-gate-complete/step-1/validation/runtime-step") {
		t.Fatalf("manifest summary = %q, want scoped manifest and evidence refs", manifestArtifact.Summary)
	}
	memories, err := memoryRepo.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memory: %v", err)
	}
	if !hasSkillFeedbackMemory(memories, runID, "skill-dod-gate") {
		t.Fatalf("memories = %+v, want skill feedback memory", memories)
	}
}

func TestResultSubsystemRecordsAcceptedSliceResultWithoutFinalCompletion(t *testing.T) {
	ctx := context.Background()
	resultRepo := memory.NewResultRepository()
	run := &runmodel.Run{
		ID:          "run-slice-result",
		WorkspaceID: "ws-slice-result",
		GoalID:      "goal-slice-result",
		DoDID:       "dod-slice-result",
		ExecutionContext: map[string]any{
			"dod": map[string]any{"items": []string{"first required item", "second required item"}},
			"executionContract": map[string]any{
				"acceptanceSlicePolicy": map[string]any{
					"maxWorkItems": 1,
					"resultPolicy": "slice_result_allowed",
				},
			},
		},
	}
	coverage := newCoverageSubsystem()
	coverage.ensure(run)
	step := &runmodel.Step{
		ID:    "run-slice-result/001-assess",
		RunID: run.ID,
		State: common.StepStateSucceeded,
	}
	if !coverage.updateFromAssessment(run, step, map[string]any{
		"items": []map[string]any{{
			"item":         "first required item",
			"state":        "satisfied",
			"evidenceRefs": []string{"run-slice-result/001-validate"},
		}},
	}) {
		t.Fatalf("coverage.updateFromAssessment changed=false, want first slice accepted")
	}
	orch := &RuntimeOrchestrator{evidence: RuntimeEvidenceRepositories{Results: resultRepo}}
	if err := orch.resultManager().recordAcceptedSliceResultsFromRun(ctx, run, step); err != nil {
		t.Fatalf("record accepted slice result: %v", err)
	}
	results, err := resultRepo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want one accepted slice result", results)
	}
	sliceResult := results[0]
	if sliceResult.SourceType != resultcommon.SourceTypeAcceptanceSlice || sliceResult.State != common.ResultStateAccepted {
		t.Fatalf("slice result = %+v, want accepted acceptance-slice result", sliceResult)
	}
	if acceptedCompletionResultMatchesRun(sliceResult, run) {
		t.Fatalf("accepted slice result must not satisfy final completion result contract")
	}
	if diagnostics := sliceResult.SummaryDetail.Diagnostics; diagnostics["resultScope"] != "acceptance_slice" || diagnostics["finalCompletionGateState"] != "not_final_result" {
		t.Fatalf("slice result diagnostics = %+v, want slice scope markers", diagnostics)
	}
	if err := coverage.validateReadyForAcceptance(run); err == nil || !strings.Contains(err.Error(), "coverageWorkItems not ready") {
		t.Fatalf("validateReadyForAcceptance err = %v, want final gate blocked by second slice", err)
	}

	secondStep := &runmodel.Step{
		ID:    "run-slice-result/002-assess",
		RunID: run.ID,
		State: common.StepStateSucceeded,
	}
	if !coverage.updateFromAssessment(run, secondStep, map[string]any{
		"items": []map[string]any{{
			"item":         "second required item",
			"state":        "satisfied",
			"evidenceRefs": []string{"run-slice-result/002-validate"},
		}},
	}) {
		t.Fatalf("coverage.updateFromAssessment changed=false, want second slice accepted")
	}
	if err := orch.resultManager().recordAcceptedSliceResultsFromRun(ctx, run, secondStep); err != nil {
		t.Fatalf("record second accepted slice result: %v", err)
	}
	results, err = resultRepo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list final slice results: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %+v, want two slice results before final result", results)
	}
	if err := coverage.validateReadyForAcceptance(run); err != nil {
		t.Fatalf("validateReadyForAcceptance after all slices accepted: %v", err)
	}
}

func TestRuntimeOrchestratorRecordsSliceResultBeforeFinalCompletionGate(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	toolCallRepo := memory.NewToolCallRepository()
	resultRepo := memory.NewResultRepository()
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		&stubRuntimeAdapter{},
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			ToolCalls:          toolCallRepo,
			Results:            resultRepo,
			RuntimeCheckpoints: memory.NewRuntimeCheckpointRepository(),
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-partial-slice-mainline")
	runID := common.RunID("run-partial-slice-mainline")
	stepID := common.StepID("run-partial-slice-mainline/step-1")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          "goal-partial-slice-mainline",
		DoDID:           "dod-partial-slice-mainline",
		State:           common.RunStateActive,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"goal": map[string]any{"text": "first required item\nsecond required item"},
			"dod":  map[string]any{"items": []string{"first required item", "second required item"}},
			"executionContract": map[string]any{
				"acceptanceSlicePolicy": map[string]any{
					"maxWorkItems": 1,
					"resultPolicy": "slice_result_allowed",
				},
			},
		},
	}
	newCoverageSubsystem().ensure(run)
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{ID: stepID, RunID: runID, Kind: llmcommon.StepKindWorkspaceExecution, State: common.StepStateExecuting, Input: map[string]any{}}); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if err := toolCallRepo.Create(ctx, &runmodel.ToolCall{ID: common.ToolCallID(string(stepID) + "/tool-call/runtime-execute"), WorkspaceID: workspaceID, RunID: runID, StepID: stepID, ToolName: string(llmcommon.StepKindWorkspaceExecution), State: common.ToolCallStateRunning}); err != nil {
		t.Fatalf("seed tool call: %v", err)
	}

	output := map[string]any{
		"status": "completed",
		"dodAssessment": map[string]any{
			"satisfied": false,
			"summary":   "First acceptance slice is satisfied; second slice remains open.",
			"items": []map[string]any{
				{"item": "first required item", "state": "satisfied", "evidenceRefs": []string{string(stepID)}},
				{"item": "second required item", "state": "failed", "reason": "second slice remains open"},
			},
			"goalCoverage": map[string]any{
				"satisfied": false,
				"items": []map[string]any{
					{"requirement": "first required item", "state": "satisfied", "evidenceRefs": []string{string(stepID)}},
					{"requirement": "second required item", "state": "failed", "reason": "second slice remains open"},
				},
			},
		},
	}
	if err := orch.FinalizeRuntimeStepResult(ctx, runID, stepID, runtimemodel.StepExecutionResult{Output: output}, nil); err != nil {
		t.Fatalf("finalize partial completion result: %v", err)
	}

	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	var sliceResults, finalAcceptedResults int
	for _, result := range results {
		if result.SourceType == resultcommon.SourceTypeAcceptanceSlice && result.State == common.ResultStateAccepted {
			sliceResults++
		}
		if acceptedCompletionResultMatchesRun(result, run) {
			finalAcceptedResults++
		}
	}
	if sliceResults < 1 {
		t.Fatalf("results = %+v, want at least one accepted slice result", results)
	}
	if finalAcceptedResults != 0 {
		t.Fatalf("results = %+v, want no final accepted DoD result", results)
	}
	storedRun, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if err := newCoverageSubsystem().validateReadyForAcceptance(storedRun); err == nil {
		t.Fatal("coverage ready for acceptance after partial slice, want final gate blocked")
	}
}

func TestRuntimeOrchestratorRejectsFinalResultWhenCoverageOpen(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-final-result-open-coverage")
	runID := common.RunID("run-final-result-open-coverage")
	stepID := common.StepID("run-final-result-open-coverage/step-1")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-final-result-open-coverage",
		State:           common.RunStateActive,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			coverageWorkItemsKey: []any{map[string]any{
				"id":                   "cwi-open",
				"status":               "open",
				"missingEvidenceKinds": []any{runtimemodel.EvidenceKindValidation},
			}},
		},
	}
	step := &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
	}
	assessment := map[string]any{
		"satisfied": true,
		"items": []map[string]any{{
			"item":         "generic requirement",
			"state":        "satisfied",
			"evidenceRefs": []string{string(stepID)},
		}},
		"summary": "current focus accepted",
	}
	resultRepo := memory.NewResultRepository()
	artifactRepo := memory.NewArtifactRepository()
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			Results:   resultRepo,
			Artifacts: artifactRepo,
		},
	}

	err := orch.resultManager().recordRunResultFromValidatedDoDAssessment(ctx, run, step, assessment, "current focus accepted", nil)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("record final result err = %v, want conflict from open coverage", err)
	}
	results, listErr := resultRepo.ListByRun(ctx, runID)
	if listErr != nil {
		t.Fatalf("list results: %v", listErr)
	}
	if len(results) != 0 {
		t.Fatalf("results = %+v, want no final accepted result", results)
	}
	artifacts, listErr := artifactRepo.ListByRun(ctx, runID)
	if listErr != nil {
		t.Fatalf("list artifacts: %v", listErr)
	}
	if len(artifacts) != 0 {
		t.Fatalf("artifacts = %+v, want no final result artifacts", artifacts)
	}
}

func TestRuntimeOrchestrator_ReusesValidatedDoDAssessmentForCompletionEvidence(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-dod-assessment-reuse")
	workspaceID := common.WorkspaceID("ws-dod-assessment-reuse")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-dod-assessment-reuse",
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"skillIds": []string{"skill-dod-assessment-reuse"},
		},
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/validation"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
	}
	assessment := map[string]any{
		"satisfied": true,
		"items": []map[string]any{
			{
				"item":         "validated assessment reused",
				"state":        "satisfied",
				"evidenceRefs": []string{string(step.ID)},
			},
		},
		"summary": "  validated DoD summary  ",
	}
	output := map[string]any{stepOutputKeyDoDAssessment: assessment}
	validated, summary, err := validatedCompletionDoDAssessment(run, output)
	if err != nil {
		t.Fatalf("validated DoD assessment: %v", err)
	}
	validated["summary"] = "validated DoD summary"
	if summary != "validated DoD summary" {
		t.Fatalf("summary=%q, want trimmed validated summary", summary)
	}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			Results:     memory.NewResultRepository(),
			Artifacts:   memory.NewArtifactRepository(),
			MemoryItems: memory.NewMemoryItemRepository(),
		},
	}

	if err := orch.resultManager().recordRunResultFromValidatedDoDAssessment(ctx, run, step, validated, summary, nil); err != nil {
		t.Fatalf("record result from validated assessment: %v", err)
	}
	if err := orch.resultManager().recordSkillFeedbackMemoryFromValidatedDoDAssessment(ctx, run, step, validated, summary); err != nil {
		t.Fatalf("record skill feedback from validated assessment: %v", err)
	}
	if err := orch.resultManager().recordRunSummaryMemoryFromValidatedDoDAssessment(ctx, run, step, validated, summary); err != nil {
		t.Fatalf("record run summary memory from validated assessment: %v", err)
	}
	results, err := orch.evidence.Results.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 || results[0].Summary != "validated DoD summary" {
		t.Fatalf("results = %+v, want accepted result from validated assessment", results)
	}
	memories, err := orch.evidence.MemoryItems.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if !hasSkillFeedbackMemory(memories, runID, "skill-dod-assessment-reuse") {
		t.Fatalf("memories = %+v, want skill feedback from same validated assessment", memories)
	}
	if !hasCompletionRunSummaryMemory(memories, runID, step.ID, "validated DoD summary") {
		t.Fatalf("memories = %+v, want completion run summary memory from same validated assessment", memories)
	}
}

func TestRuntimeOrchestrator_DoesNotRecordCompletionEvidenceForCreatedRun(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-created-result-gate")
	runID := common.RunID("run-created-result-gate")
	step := &runmodel.Step{ID: common.StepID(string(runID) + "/step-1"), RunID: runID}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		GoalID:          "goal-created-result-gate",
		DoDID:           "dod-created-result-gate",
		State:           common.RunStatePending,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	assessment := map[string]any{
		"satisfied": true,
		"summary":   "DoD satisfied before run activation",
		"items": []map[string]any{{
			"item":         "activation gated result",
			"state":        "satisfied",
			"evidenceRefs": []string{string(step.ID)},
		}},
	}
	resultRepo := memory.NewResultRepository()
	artifactRepo := memory.NewArtifactRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			Results:     resultRepo,
			Artifacts:   artifactRepo,
			MemoryItems: memoryRepo,
		},
	}

	err := orch.resultManager().recordCompletionEvidenceFromValidatedDoDAssessment(ctx, run, step, assessment, "DoD satisfied before run activation", nil)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("record completion evidence err = %v, want conflict", err)
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("results = %+v, want no formal result for created run", results)
	}
	artifacts, err := artifactRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifacts) != 0 {
		t.Fatalf("artifacts = %+v, want no artifacts for created run", artifacts)
	}
	memories, err := memoryRepo.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(memories) != 0 {
		t.Fatalf("memories = %+v, want no memory side effects for created run", memories)
	}
}

func TestRuntimeOrchestrator_RecordsValidationCommandMemoriesFromDoDAssessment(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-validation-command-memory")
	workspaceID := common.WorkspaceID("ws-validation-command-memory")
	evidenceStepID := common.StepID(string(runID) + "/block/verify")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-validation-command-memory",
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/validation"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
	}
	stepRepo := memory.NewStepRepository()
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    evidenceStepID,
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateSucceeded,
		Output: map[string]any{
			"command":  "go test ./...",
			"exitCode": 1,
			"stdout":   "E2E assertions passed\ncoverage ok",
			"stderr":   "compile error in internal/app.go",
		},
	}); err != nil {
		t.Fatalf("seed evidence step: %v", err)
	}
	assessment := map[string]any{
		"satisfied": true,
		"summary":   "DoD satisfied after repair",
		"items": []map[string]any{
			{
				"item":         "verification command tracked",
				"state":        "satisfied",
				"evidenceRefs": []string{string(evidenceStepID)},
			},
		},
	}
	memoryRepo := memory.NewMemoryItemRepository()
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			MemoryItems: memoryRepo,
		},
	}
	if err := orch.resultManager().recordValidationCommandMemoriesFromValidatedDoDAssessment(ctx, run, step, assessment); err != nil {
		t.Fatalf("recordValidationCommandMemoriesFromValidatedDoDAssessment: %v", err)
	}
	memories, err := memoryRepo.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	var commandMemory *memorymodel.MemoryItem
	var lessonMemory *memorymodel.MemoryItem
	for _, item := range memories {
		if item == nil || item.RunID != runID {
			continue
		}
		switch item.MemoryType {
		case memorycommon.MemoryTypeProjectContext:
			if item.Summary == "Validation command baseline captured" {
				commandMemory = item
			}
		case memorycommon.MemoryTypeExecutionLesson:
			if item.Summary == "Validation failure signals captured for repair" {
				lessonMemory = item
			}
		}
	}
	if commandMemory == nil ||
		!strings.Contains(commandMemory.Content, "go test ./...") ||
		!strings.Contains(commandMemory.Content, string(evidenceStepID)) ||
		!strings.Contains(commandMemory.Content, "signal=E2E assertions passed") {
		t.Fatalf("command memory = %+v, want command with provenance evidence", commandMemory)
	}
	if lessonMemory == nil ||
		!strings.Contains(lessonMemory.Content, "compile error in internal/app.go") ||
		!strings.Contains(lessonMemory.Content, "exitCode=1") {
		t.Fatalf("lesson memory = %+v, want failure lesson content", lessonMemory)
	}
}

func TestRuntimeOrchestrator_ResultSummaryIncludesRuntimeEvidenceOutput(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-result-summary-evidence-output")
	workspaceID := common.WorkspaceID("ws-result-summary-evidence-output")
	evidenceStepID := common.StepID("run-result-summary-evidence-output/block/query_server_config")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-result-summary-evidence-output",
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"dialogLanguage": "zh-CN",
		},
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/validation"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
	}
	stepRepo := memory.NewStepRepository()
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    evidenceStepID,
		RunID: runID,
		Kind:  "ssh/command",
		State: common.StepStateSucceeded,
		Output: map[string]any{
			"command":  "lscpu && free -h && df -h && docker --version",
			"exitCode": 0,
			"stdout": strings.Join([]string{
				"Model name:                              Intel(R) Core(TM) i7-10750H CPU @ 2.60GHz",
				"CPU(s):                                  12",
				"Mem:            15Gi       1.5Gi        11Gi       1.8Mi       3.0Gi        14Gi",
				"/dev/mapper/ubuntu--vg-ubuntu--lv  466G   23G  419G   6% /",
				"Docker version 29.2.1, build a5c7197",
			}, "\n"),
		},
	}); err != nil {
		t.Fatalf("seed evidence step: %v", err)
	}
	assessment := map[string]any{
		"satisfied": true,
		"items": []map[string]any{
			{
				"item":         "输出包含服务器配置",
				"state":        "satisfied",
				"evidenceRefs": []string{string(evidenceStepID)},
			},
		},
		"summary":               "目标已完成，成功查询到服务配置信息（CPU、内存、磁盘、Docker版本）。",
		"goalOutcome":           "已成功查询远程服务器配置信息。",
		"implementationSummary": "通过 SSH 获取 CPU、内存、磁盘和 Docker 版本。",
		"verificationSummary":   "命令执行成功，exit code 0。",
	}
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			Results:   memory.NewResultRepository(),
			Artifacts: memory.NewArtifactRepository(),
		},
	}

	if err := orch.resultManager().recordRunResultFromValidatedDoDAssessment(ctx, run, step, assessment, "目标已完成，成功查询到服务配置信息（CPU、内存、磁盘、Docker版本）。", nil); err != nil {
		t.Fatalf("record result from validated assessment: %v", err)
	}
	artifacts, err := orch.evidence.Artifacts.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	var summaryArtifact *resultmodel.Artifact
	for _, artifact := range artifacts {
		if artifact.ID == common.ArtifactID(string(runID)+"/artifact/result-summary") {
			summaryArtifact = artifact
			break
		}
	}
	if summaryArtifact == nil {
		t.Fatalf("artifacts = %+v, want result summary artifact", artifacts)
	}
	for _, want := range []string{
		"## 证据输出",
		"Intel(R) Core(TM) i7-10750H CPU @ 2.60GHz",
		"Mem:            15Gi",
		"/dev/mapper/ubuntu--vg-ubuntu--lv  466G",
		"Docker version 29.2.1",
	} {
		if !strings.Contains(summaryArtifact.Summary, want) {
			t.Fatalf("summary artifact missing %q:\n%s", want, summaryArtifact.Summary)
		}
	}
}

func TestRuntimeOrchestrator_ResultArtifactsIncludePatchDiff(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-result-patch-diff")
	workspaceID := common.WorkspaceID("ws-result-patch-diff")
	stepID := common.StepID(string(runID) + "/validation")
	patchID := common.PatchID(string(runID) + "/step-1/patch/1")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-result-patch-diff",
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"dialogLanguage": "zh-CN",
		},
	}
	step := &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
	}
	patchRepo := memory.NewPatchRepository()
	if err := patchRepo.Create(ctx, &runmodel.Patch{
		ID:          patchID,
		WorkspaceID: workspaceID,
		RunID:       runID,
		StepID:      common.StepID(string(runID) + "/step-1"),
		PatchType:   "unified_diff",
		TargetRef:   "internal/example.go",
		Diff:        "--- a/internal/example.go\n+++ b/internal/example.go\n@@ -1 +1 @@\n-before\n+after\n",
		Summary:     "update example",
		State:       common.PatchStateApplied,
		CreatedAt:   "2026-05-04T00:00:00Z",
		AppliedAt:   "2026-05-04T00:00:01Z",
	}); err != nil {
		t.Fatalf("seed patch: %v", err)
	}
	assessment := map[string]any{
		"satisfied": true,
		"items": []map[string]any{
			{
				"item":         "代码变更可追溯",
				"state":        "satisfied",
				"evidenceRefs": []string{string(stepID)},
			},
		},
		"summary":               "本次仅执行 assessment-only 评估，无新增可执行操作。",
		"implementationSummary": "本步为评估收敛，不包含新的执行性修复。",
		"verificationSummary":   "Patch was applied and captured as evidence.",
	}
	artifactRepo := memory.NewArtifactRepository()
	resultRepo := memory.NewResultRepository()
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			Results:   resultRepo,
			Artifacts: artifactRepo,
			Patches:   patchRepo,
		},
	}

	if err := orch.resultManager().recordRunResultFromValidatedDoDAssessment(ctx, run, step, assessment, "本次仅执行 assessment-only 评估，无新增可执行操作。", nil); err != nil {
		t.Fatalf("record result from validated assessment: %v", err)
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want one accepted result", results)
	}
	if strings.Contains(results[0].Summary, "无新增可执行") ||
		strings.Contains(results[0].DoDAssessment.Summary, "无新增可执行") ||
		strings.Contains(results[0].SummaryDetail.ImplementationSummary, "不包含新的执行性修复") ||
		!strings.Contains(results[0].Summary, "runtime 记录到 1 个已应用的 workspace 变更") ||
		!strings.Contains(results[0].SummaryDetail.ImplementationSummary, "internal/example.go") {
		t.Fatalf("result summary/detail = %+v, want runtime fact-corrected applied workspace change summary", results[0])
	}
	if !reflect.DeepEqual(results[0].ArtifactRefs, []common.ArtifactID{
		common.ArtifactID(string(runID) + "/artifact/result-summary"),
		common.ArtifactID(string(runID) + "/artifact/deliverable-manifest"),
		common.ArtifactID(string(runID) + "/artifact/workspace-change-diff"),
	}) {
		t.Fatalf("result artifact refs = %+v, want summary, manifest and patch diff", results[0].ArtifactRefs)
	}
	artifacts, err := artifactRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	var summaryArtifact *resultmodel.Artifact
	var manifestArtifact *resultmodel.Artifact
	var diffArtifact *resultmodel.Artifact
	for _, artifact := range artifacts {
		switch artifact.ID {
		case common.ArtifactID(string(runID) + "/artifact/result-summary"):
			summaryArtifact = artifact
		case common.ArtifactID(string(runID) + "/artifact/deliverable-manifest"):
			manifestArtifact = artifact
		case common.ArtifactID(string(runID) + "/artifact/workspace-change-diff"):
			diffArtifact = artifact
		}
	}
	if diffArtifact == nil || diffArtifact.ArtifactType != resultcommon.ArtifactTypePatch || diffArtifact.PatchID != patchID {
		t.Fatalf("diff artifact = %+v, want patch artifact linked to patch", diffArtifact)
	}
	for _, want := range []string{"```diff", "--- a/internal/example.go", "+after"} {
		if !strings.Contains(diffArtifact.Summary, want) {
			t.Fatalf("diff artifact missing %q:\n%s", want, diffArtifact.Summary)
		}
	}
	if summaryArtifact == nil || !strings.Contains(summaryArtifact.Summary, string(diffArtifact.ID)) {
		t.Fatalf("summary artifact = %+v, want patch diff artifact ref", summaryArtifact)
	}
	if strings.Contains(summaryArtifact.Summary, "无新增可执行") ||
		strings.Contains(summaryArtifact.Summary, "不包含新的执行性修复") ||
		!strings.Contains(summaryArtifact.Summary, "runtime 记录到 1 个已应用的 workspace 变更") {
		t.Fatalf("summary artifact = %s, want runtime fact-corrected content", summaryArtifact.Summary)
	}
	if manifestArtifact == nil || !strings.Contains(manifestArtifact.Summary, "## Workspace Change Evidence") || !strings.Contains(manifestArtifact.Summary, string(patchID)) {
		t.Fatalf("manifest artifact = %+v, want patch evidence section", manifestArtifact)
	}
}

func TestRuntimeOrchestrator_ResultArtifactsIncludeWebSourceIndex(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-result-web-source-index")
	workspaceID := common.WorkspaceID("ws-result-web-source-index")
	stepID := common.StepID(string(runID) + "/validation")
	searchStepID := common.StepID(string(runID) + "/step-search")
	fetchStepID := common.StepID(string(runID) + "/step-fetch")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-result-web-source-index",
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	step := &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
	}
	stepRepo := memory.NewStepRepository()
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    searchStepID,
		RunID: runID,
		Kind:  webcommon.StepKindSearch,
		State: common.StepStateSucceeded,
		Output: map[string]any{
			"provider":   "default-http",
			"query":      "reference latest",
			"searchedAt": "2026-05-05T00:00:00Z",
			"policy":     map[string]any{"decision": "allowed"},
			"results": []map[string]any{{
				"title":       "Reference Downloads",
				"url":         "https://example.com/reference",
				"snippet":     "Reference data",
				"source":      "example.com",
				"publishedAt": "2026-05-01",
				"rank":        1,
				"contentHash": "search-hash",
			}},
		},
	}); err != nil {
		t.Fatalf("seed search step: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    fetchStepID,
		RunID: runID,
		Kind:  webcommon.StepKindFetch,
		State: common.StepStateSucceeded,
		Output: map[string]any{
			"provider":    "default-http",
			"url":         "https://go.dev/dl/",
			"finalUrl":    "https://go.dev/dl/",
			"title":       "Downloads",
			"source":      "go.dev",
			"retrievedAt": "2026-05-05T00:00:01Z",
			"contentHash": "fetch-hash",
			"policy":      map[string]any{"decision": "allowed"},
		},
	}); err != nil {
		t.Fatalf("seed fetch step: %v", err)
	}
	assessment := map[string]any{
		"satisfied": true,
		"items": []map[string]any{
			{
				"item":         "web source is cited",
				"state":        "satisfied",
				"evidenceRefs": []string{string(fetchStepID)},
			},
		},
		"summary":               "DoD satisfied with web source",
		"implementationSummary": "Verified the external source.",
		"verificationSummary":   "Source was fetched and indexed.",
	}
	artifactRepo := memory.NewArtifactRepository()
	resultRepo := memory.NewResultRepository()
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			Results:   resultRepo,
			Artifacts: artifactRepo,
		},
	}

	if err := orch.resultManager().recordRunResultFromValidatedDoDAssessment(ctx, run, step, assessment, "DoD satisfied with web source", nil); err != nil {
		t.Fatalf("record result from validated assessment: %v", err)
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want one accepted result", results)
	}
	if !reflect.DeepEqual(results[0].ArtifactRefs, []common.ArtifactID{
		common.ArtifactID(string(runID) + "/artifact/result-summary"),
		common.ArtifactID(string(runID) + "/artifact/deliverable-manifest"),
		common.ArtifactID(string(runID) + "/artifact/web-source-index"),
	}) {
		t.Fatalf("result artifact refs = %+v, want summary, manifest and web source index", results[0].ArtifactRefs)
	}
	artifacts, err := artifactRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	var sourceArtifact *resultmodel.Artifact
	var summaryArtifact *resultmodel.Artifact
	for _, artifact := range artifacts {
		switch artifact.ID {
		case common.ArtifactID(string(runID) + "/artifact/web-source-index"):
			sourceArtifact = artifact
		case common.ArtifactID(string(runID) + "/artifact/result-summary"):
			summaryArtifact = artifact
		}
	}
	if sourceArtifact == nil ||
		sourceArtifact.Category != resultcommon.ArtifactCategorySource ||
		sourceArtifact.ArtifactType != resultcommon.ArtifactTypeReport ||
		!reflect.DeepEqual(sourceArtifact.EvidenceRefs, []string{string(fetchStepID), string(searchStepID)}) {
		t.Fatalf("source artifact = %+v, want web source report linked to web evidence", sourceArtifact)
	}
	for _, want := range []string{"Web Source Index", "https://go.dev/dl/", string(searchStepID), string(fetchStepID)} {
		if !strings.Contains(sourceArtifact.Summary, want) {
			t.Fatalf("source artifact missing %q:\n%s", want, sourceArtifact.Summary)
		}
	}
	if summaryArtifact == nil || !strings.Contains(summaryArtifact.Summary, string(sourceArtifact.ID)) {
		t.Fatalf("summary artifact = %+v, want web source artifact ref", summaryArtifact)
	}

	memoryRepo := memory.NewMemoryItemRepository()
	orch.evidence.MemoryItems = memoryRepo
	if err := orch.resultManager().recordRunSummaryMemoryFromValidatedDoDAssessment(ctx, run, step, assessment, "DoD satisfied with web source"); err != nil {
		t.Fatalf("record run summary memory: %v", err)
	}
	memories, err := memoryRepo.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(memories) != 1 ||
		!strings.Contains(memories[0].Content, "Web sources:") ||
		!strings.Contains(memories[0].Content, "https://go.dev/dl/") ||
		!strings.Contains(memories[0].Content, "fetch-hash") {
		t.Fatalf("memory = %+v, want source summary refs", memories)
	}
	if strings.Contains(memories[0].Content, "Download Go") {
		t.Fatalf("memory content = %q, must not store raw search snippet/page body", memories[0].Content)
	}
}

func TestRuntimeOrchestrator_ResultArtifactsIncludeGitOperationIndex(t *testing.T) {
	ctx := context.Background()
	workspaceID := common.WorkspaceID("ws-git-result-artifact")
	runID := common.RunID("run-git-result-artifact")
	stepID := common.StepID("step-dod-git")
	gitStepID := common.StepID("step-git-commit")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-git-result-artifact",
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}
	step := &runmodel.Step{
		ID:    stepID,
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
	}
	stepRepo := memory.NewStepRepository()
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    gitStepID,
		RunID: runID,
		Kind:  gitcommon.StepKindWrite,
		State: common.StepStateSucceeded,
		Output: map[string]any{
			"operation":        "commit",
			"status":           "completed",
			"branch":           "feature/builtin-git-tool",
			"headCommitBefore": "1111111111111111111111111111111111111111",
			"headCommitAfter":  "2222222222222222222222222222222222222222",
			"summary":          "created commit 222222222222",
			"commit": map[string]any{
				"hash":    "2222222222222222222222222222222222222222",
				"subject": "Add builtin git tool",
			},
			"changedFiles": []map[string]any{{
				"path":   "docs/workspace-builtin-git-tool-capability-plan.md",
				"status": "modified",
			}},
		},
	}); err != nil {
		t.Fatalf("seed git step: %v", err)
	}
	assessment := map[string]any{
		"satisfied": true,
		"items": []map[string]any{{
			"item":         "git commit is recorded",
			"state":        "satisfied",
			"evidenceRefs": []string{string(gitStepID)},
		}},
		"summary":               "DoD satisfied with git evidence",
		"implementationSummary": "Committed the implementation.",
		"verificationSummary":   "Git evidence was indexed.",
	}
	artifactRepo := memory.NewArtifactRepository()
	resultRepo := memory.NewResultRepository()
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			Results:   resultRepo,
			Artifacts: artifactRepo,
		},
	}

	if err := orch.resultManager().recordRunResultFromValidatedDoDAssessment(ctx, run, step, assessment, "DoD satisfied with git evidence", nil); err != nil {
		t.Fatalf("record result from validated assessment: %v", err)
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want one accepted result", results)
	}
	if !reflect.DeepEqual(results[0].ArtifactRefs, []common.ArtifactID{
		common.ArtifactID(string(runID) + "/artifact/result-summary"),
		common.ArtifactID(string(runID) + "/artifact/deliverable-manifest"),
		common.ArtifactID(string(runID) + "/artifact/git-operation-index"),
	}) {
		t.Fatalf("result artifact refs = %+v, want summary, manifest and git operation index", results[0].ArtifactRefs)
	}
	artifacts, err := artifactRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	var gitArtifact *resultmodel.Artifact
	var summaryArtifact *resultmodel.Artifact
	for _, artifact := range artifacts {
		switch artifact.ID {
		case common.ArtifactID(string(runID) + "/artifact/git-operation-index"):
			gitArtifact = artifact
		case common.ArtifactID(string(runID) + "/artifact/result-summary"):
			summaryArtifact = artifact
		}
	}
	if gitArtifact == nil ||
		gitArtifact.Category != resultcommon.ArtifactCategoryRuntimeEvidence ||
		gitArtifact.ArtifactType != resultcommon.ArtifactTypeReport ||
		!reflect.DeepEqual(gitArtifact.EvidenceRefs, []string{string(gitStepID)}) {
		t.Fatalf("git artifact = %+v, want git operation report linked to git evidence", gitArtifact)
	}
	for _, want := range []string{"Git Operation Index", "git/write", "commit", "feature/builtin-git-tool", "222222222222", "docs/workspace-builtin-git-tool-capability-plan.md"} {
		if !strings.Contains(gitArtifact.Summary, want) {
			t.Fatalf("git artifact missing %q:\n%s", want, gitArtifact.Summary)
		}
	}
	if summaryArtifact == nil || !strings.Contains(summaryArtifact.Summary, string(gitArtifact.ID)) {
		t.Fatalf("summary artifact = %+v, want git operation artifact ref", summaryArtifact)
	}
}

func TestRuntimeOrchestrator_CompletionEvidenceWritesResultAndSkillFeedbackConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	artifactRelease := make(chan struct{})
	memoryRelease := make(chan struct{})
	resultRepo := memory.NewResultRepository()
	artifactRepo := &blockingArtifactRepository{inner: memory.NewArtifactRepository(), started: started, release: artifactRelease}
	memoryRepo := &blockingMemoryItemRepository{inner: memory.NewMemoryItemRepository(), started: started, release: memoryRelease}
	orch := &RuntimeOrchestrator{
		workspaces: memory.NewWorkspaceRepository(),
		evidence: RuntimeEvidenceRepositories{
			Results:     resultRepo,
			Artifacts:   artifactRepo,
			MemoryItems: memoryRepo,
		},
	}

	workspaceID := common.WorkspaceID("ws-completion-evidence-concurrent")
	runID := common.RunID("run-completion-evidence-concurrent")
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-completion-evidence-concurrent"),
		State:           common.RunStateActive,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
		ExecutionContext: map[string]any{
			"skillIds": []string{"skill-completion-evidence"},
		},
	}
	step := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/validation"),
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
	}
	assessment := map[string]any{
		"satisfied": true,
		"items": []map[string]any{
			{
				"item":         "completion evidence writes are concurrent",
				"state":        "satisfied",
				"evidenceRefs": []string{string(step.ID)},
			},
		},
		"summary": "DoD satisfied",
	}

	done := make(chan error, 1)
	go func() {
		done <- orch.resultManager().recordCompletionEvidenceFromValidatedDoDAssessment(ctx, run, step, assessment, "DoD satisfied", workspace)
	}()

	seen := map[string]bool{}
	for !seen["artifact-create:run-completion-evidence-concurrent/artifact/result-summary"] ||
		!seen["artifact-create:run-completion-evidence-concurrent/artifact/deliverable-manifest"] ||
		!seen["memory-create:run-completion-evidence-concurrent/memory/skill-feedback"] {
		select {
		case name := <-started:
			seen[name] = true
		case err := <-done:
			t.Fatalf("recordCompletionEvidenceFromValidatedDoDAssessment returned before all writes started: %v", err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for concurrent completion evidence writes, saw %+v: %v", seen, ctx.Err())
		}
	}
	close(artifactRelease)
	close(memoryRelease)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("recordCompletionEvidenceFromValidatedDoDAssessment: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("recordCompletionEvidenceFromValidatedDoDAssessment did not complete after release: %v", ctx.Err())
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 || len(results[0].ArtifactRefs) != 2 {
		t.Fatalf("results = %+v, want accepted result with both artifacts", results)
	}
	memories, err := memoryRepo.ListByWorkspaceID(ctx, workspaceID)
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if !hasSkillFeedbackMemory(memories, runID, "skill-completion-evidence") {
		t.Fatalf("memories = %+v, want skill feedback memory", memories)
	}
}

func TestRuntimeOrchestrator_FinalRunAndWorkspaceCompletionWritesConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	release := make(chan struct{})
	workspaceInner := memory.NewWorkspaceRepository()
	runInner := memory.NewRunRepository()
	workspaceRepo := &blockingWorkspaceRepository{inner: workspaceInner, started: started, updateRelease: release}
	runRepo := &blockingCompletedRunRepository{inner: runInner, started: started, release: release}
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	artifactRepo := memory.NewArtifactRepository()
	hub := events.NewMemoryHub()
	runtime := &stubRuntimeAdapter{output: map[string]any{
		"status": "completed",
		"dodAssessment": map[string]any{
			"satisfied": true,
			"items": []map[string]any{
				{
					"item":         "completion writes are concurrent",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-terminal-write-concurrent/step-1/validation/runtime-step"},
				},
			},
			"summary": "DoD satisfied",
		},
	}}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			Results:   resultRepo,
			Artifacts: artifactRepo,
		},
		hub,
	)

	workspaceID := common.WorkspaceID("ws-terminal-write-concurrent")
	runID := common.RunID("run-terminal-write-concurrent")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateActive}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-terminal-write-concurrent"),
		State:           common.RunStatePending,
		InitialStepKind: llmcommon.StepKindWorkspaceExecution,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- orch.StartRun(ctx, runID)
	}()

	seen := map[string]bool{}
	for !seen["run-completed-update"] || !seen["workspace-completed-update"] {
		select {
		case name := <-started:
			seen[name] = true
		case err := <-done:
			t.Fatalf("StartRun returned before both terminal writes started: %v", err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for concurrent terminal writes, saw %+v: %v", seen, ctx.Err())
		}
	}
	publishedBeforeRelease := hub.Published()
	for _, item := range publishedBeforeRelease {
		if item.Topic == runevents.TopicRunStateChanged && item.Payload[events.PayloadKeyState] == string(common.RunStateCompleted) {
			t.Fatalf("published completed run event before terminal writes were released: %+v", publishedBeforeRelease)
		}
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StartRun: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("StartRun did not complete after releasing terminal writes: %v", ctx.Err())
	}
	published := hub.Published()
	var completedRunIndex = -1
	var completedWorkspaceIndex = -1
	for idx, item := range published {
		if item.Topic == runevents.TopicRunStateChanged && item.Payload[events.PayloadKeyState] == string(common.RunStateCompleted) {
			completedRunIndex = idx
		}
		if item.Topic == workspaceevents.TopicWorkspaceStateChanged && item.Payload[events.PayloadKeyState] == string(common.WorkspaceStateActive) {
			completedWorkspaceIndex = idx
		}
	}
	if completedRunIndex < 0 || completedWorkspaceIndex < 0 || completedRunIndex > completedWorkspaceIndex {
		t.Fatalf("published events = %+v, want run completed before workspace completed", published)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State != common.RunStateCompleted {
		t.Fatalf("run state = %s, want completed", runItem.State)
	}
	workspaceItem, err := workspaceRepo.Get(ctx, workspaceID)
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if workspaceItem.State != common.WorkspaceStateActive || workspaceItem.ActiveRunID != "" {
		t.Fatalf("workspace = %+v, want completed with active run cleared", workspaceItem)
	}
}

func TestRuntimeOrchestrator_ResultArtifactsWriteConcurrentlyBeforeAcceptedResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan string, 4)
	artifactRelease := make(chan struct{})
	resultRepo := memory.NewResultRepository()
	artifactRepo := &blockingArtifactRepository{inner: memory.NewArtifactRepository(), started: started, release: artifactRelease}
	orch := &RuntimeOrchestrator{
		workspaces: memory.NewWorkspaceRepository(),
		evidence: RuntimeEvidenceRepositories{
			Results:   resultRepo,
			Artifacts: artifactRepo,
		},
	}

	workspaceID := common.WorkspaceID("ws-result-manifest-prefetch")
	runID := common.RunID("run-result-manifest-prefetch")
	workspace := &workspacemodel.Workspace{ID: workspaceID, Title: "ws", DeliverableDirectory: t.TempDir(), State: common.WorkspaceStateActive}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           common.DoDID("dod-result-manifest-prefetch"),
		State:           common.RunStateActive,
		InitialStepKind: "llm/workspace_execution",
	}
	step := &runmodel.Step{
		ID:    "run-result-manifest-prefetch/step-1",
		RunID: runID,
		Kind:  llmcommon.StepKindWorkspaceExecution,
		Input: map[string]any{
			stepInputKeyExecutionContextPackage: map[string]any{
				stepInputKeyWorkspaceFileManifest: []map[string]any{
					{"path": "README.md", "size": 32},
				},
			},
		},
	}
	output := map[string]any{
		"status": "completed",
		"dodAssessment": map[string]any{
			"satisfied": true,
			"items": []map[string]any{
				{
					"item":         "manifest generated while summary artifact is writing",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-result-manifest-prefetch/step-1/validation/runtime-step"},
				},
			},
			"summary": "DoD satisfied",
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- orch.resultManager().recordRunResultFromDoDAssessment(ctx, run, step, output, workspace)
	}()

	seen := map[string]bool{}
	for !seen["artifact-create:run-result-manifest-prefetch/artifact/result-summary"] ||
		!seen["artifact-create:run-result-manifest-prefetch/artifact/deliverable-manifest"] {
		select {
		case name := <-started:
			seen[name] = true
		case <-ctx.Done():
			t.Fatalf("events before release = %+v, want both artifact writes: %v", seen, ctx.Err())
		}
	}
	close(artifactRelease)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("recordRunResultFromDoDAssessment: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("recordRunResultFromDoDAssessment did not complete after release: %v", ctx.Err())
	}
	artifacts, err := artifactRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("artifacts = %+v, want summary and manifest artifacts", artifacts)
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) != 1 || results[0].State != common.ResultStateAccepted || len(results[0].ArtifactRefs) != 2 {
		t.Fatalf("results = %+v, want one accepted result with both artifact refs", results)
	}
}

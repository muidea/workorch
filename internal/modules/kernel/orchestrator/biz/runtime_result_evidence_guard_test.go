package orchestrator

import (
	"context"
	"errors"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestAcceptedCompletionOutputsRejectRejectedEvidenceRefs(t *testing.T) {
	ctx := context.Background()
	run := guardTestRun("run-rejected-evidence-output-guard")
	step := guardTestStep(run.ID, "complete")
	rejectedRef := "rejected-contract-diagnostic"
	results := memory.NewResultRepository()
	artifacts := memory.NewArtifactRepository()
	memories := memory.NewMemoryItemRepository()
	evidenceRecords := memory.NewEvidenceRecordRepository()
	seedRejectedEvidenceRecord(t, ctx, evidenceRecords, run, step, rejectedRef)
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			Results:         results,
			Artifacts:       artifacts,
			MemoryItems:     memories,
			EvidenceRecords: evidenceRecords,
		},
	}

	err := orch.resultManager().recordCompletionEvidenceFromValidatedDoDAssessment(ctx, run, step, satisfiedAssessmentWithEvidenceRef(rejectedRef), "should not be accepted", nil)
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("record completion evidence err = %v, want invalid argument", err)
	}
	assertNoAcceptedOutputs(t, ctx, run, results, artifacts, memories)
}

func TestAcceptedSliceResultRejectsRejectedEvidenceRefs(t *testing.T) {
	ctx := context.Background()
	run := guardTestRun("run-rejected-slice-output-guard")
	step := guardTestStep(run.ID, "slice")
	rejectedRef := "rejected-slice-diagnostic"
	results := memory.NewResultRepository()
	evidenceRecords := memory.NewEvidenceRecordRepository()
	seedRejectedEvidenceRecord(t, ctx, evidenceRecords, run, step, rejectedRef)
	run.ExecutionContext = map[string]any{
		acceptanceSlicesKey: []map[string]any{{
			"id":                  "slice-1",
			"state":               "accepted",
			"coverageWorkItemIds": []string{"item-1"},
		}},
		coverageWorkItemsKey: []map[string]any{{
			"id":                   "item-1",
			"status":               "satisfied",
			"targetText":           "target item",
			"acceptedEvidenceRefs": []string{rejectedRef},
		}},
	}
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			Results:         results,
			EvidenceRecords: evidenceRecords,
		},
	}

	err := orch.resultManager().recordAcceptedSliceResultsFromRun(ctx, run, step)
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("record accepted slice result err = %v, want invalid argument", err)
	}
	items, listErr := results.ListByRun(ctx, run.ID)
	if listErr != nil || len(items) != 0 {
		t.Fatalf("results = %+v err=%v, want no accepted slice result", items, listErr)
	}
}

func TestAvailableArtifactRejectsRejectedEvidenceRefs(t *testing.T) {
	ctx := context.Background()
	run := guardTestRun("run-rejected-artifact-output-guard")
	step := guardTestStep(run.ID, "artifact")
	rejectedRef := "rejected-artifact-diagnostic"
	artifacts := memory.NewArtifactRepository()
	evidenceRecords := memory.NewEvidenceRecordRepository()
	seedRejectedEvidenceRecord(t, ctx, evidenceRecords, run, step, rejectedRef)
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			Artifacts:       artifacts,
			EvidenceRecords: evidenceRecords,
		},
	}

	_, err := orch.resultManager().recordResultSummaryArtifact(ctx, run, step, "summary", satisfiedAssessmentWithEvidenceRef(rejectedRef), []string{rejectedRef}, nil, runtimeEvidenceTimestamp())
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("record artifact err = %v, want invalid argument", err)
	}
	items, listErr := artifacts.ListByRun(ctx, run.ID)
	if listErr != nil || len(items) != 0 {
		t.Fatalf("artifacts = %+v err=%v, want no available artifact", items, listErr)
	}
}

func TestCompletionMemoryRejectsRejectedEvidenceRefs(t *testing.T) {
	ctx := context.Background()
	run := guardTestRun("run-rejected-memory-output-guard")
	step := guardTestStep(run.ID, "memory")
	rejectedRef := "rejected-memory-diagnostic"
	memories := memory.NewMemoryItemRepository()
	evidenceRecords := memory.NewEvidenceRecordRepository()
	seedRejectedEvidenceRecord(t, ctx, evidenceRecords, run, step, rejectedRef)
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			MemoryItems:     memories,
			EvidenceRecords: evidenceRecords,
		},
	}

	err := orch.resultManager().recordRunSummaryMemoryFromValidatedDoDAssessment(ctx, run, step, satisfiedAssessmentWithEvidenceRef(rejectedRef), "should not become memory")
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("record memory err = %v, want invalid argument", err)
	}
	items, listErr := memories.ListByWorkspaceID(ctx, run.WorkspaceID)
	if listErr != nil || len(items) != 0 {
		t.Fatalf("memories = %+v err=%v, want no completion memory", items, listErr)
	}
}

func TestRejectedCompletionResultMayReferenceRejectedEvidence(t *testing.T) {
	ctx := context.Background()
	run := guardTestRun("run-rejected-result-output-guard")
	step := guardTestStep(run.ID, "rejected-result")
	rejectedRef := "rejected-result-diagnostic"
	results := memory.NewResultRepository()
	evidenceRecords := memory.NewEvidenceRecordRepository()
	seedRejectedEvidenceRecord(t, ctx, evidenceRecords, run, step, rejectedRef)
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			Results:         results,
			EvidenceRecords: evidenceRecords,
		},
	}

	if err := orch.resultManager().recordRunResultFromRejectedDoDAssessment(ctx, run, step, rejectedAssessmentWithEvidenceRef(rejectedRef), "rejected result is allowed"); err != nil {
		t.Fatalf("record rejected result: %v", err)
	}
	items, err := results.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(items) != 1 || items[0].State != common.ResultStateRejected || items[0].EvidenceRefs[0] != rejectedRef {
		t.Fatalf("results = %+v, want rejected result retaining rejected evidence ref", items)
	}
}

func guardTestRun(runID common.RunID) *runmodel.Run {
	return &runmodel.Run{
		ID:              runID,
		WorkspaceID:     common.WorkspaceID("ws-" + string(runID)),
		DoDID:           common.DoDID(string(runID) + "-dod"),
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"skillIds": []string{"guard-skill"},
		},
	}
}

func guardTestStep(runID common.RunID, label string) *runmodel.Step {
	return &runmodel.Step{
		ID:     common.StepID(string(runID) + "/" + label),
		RunID:  runID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateCompleted,
		Input:  map[string]any{},
		Output: map[string]any{},
	}
}

func seedRejectedEvidenceRecord(t *testing.T, ctx context.Context, repo *memory.EvidenceRecordRepository, run *runmodel.Run, step *runmodel.Step, sourceRef string) {
	t.Helper()
	if err := repo.Create(ctx, &orchestratormodel.EvidenceRecord{
		ID:          common.EvidenceRecordID(sourceRef + "-record"),
		WorkspaceID: run.WorkspaceID,
		RunID:       run.ID,
		StepID:      step.ID,
		Kind:        orchestratormodel.EvidenceKindDiagnostic,
		Status:      orchestratormodel.EvidenceStatusRejected,
		SourceType:  evidenceRecordSourceTypeContractViolation,
		SourceRef:   sourceRef,
		SubjectType: evidenceRecordSubjectRuntimeStep,
		SubjectID:   string(step.ID),
	}); err != nil {
		t.Fatalf("seed rejected evidence: %v", err)
	}
}

func assertNoAcceptedOutputs(t *testing.T, ctx context.Context, run *runmodel.Run, results *memory.ResultRepository, artifacts *memory.ArtifactRepository, memories *memory.MemoryItemRepository) {
	t.Helper()
	resultItems, resultErr := results.ListByRun(ctx, run.ID)
	if resultErr != nil || len(resultItems) != 0 {
		t.Fatalf("results = %+v err=%v, want no results", resultItems, resultErr)
	}
	artifactItems, artifactErr := artifacts.ListByRun(ctx, run.ID)
	if artifactErr != nil || len(artifactItems) != 0 {
		t.Fatalf("artifacts = %+v err=%v, want no artifacts", artifactItems, artifactErr)
	}
	memoryItems, memoryErr := memories.ListByWorkspaceID(ctx, run.WorkspaceID)
	if memoryErr != nil || len(memoryItems) != 0 {
		t.Fatalf("memories = %+v err=%v, want no memories", memoryItems, memoryErr)
	}
}

func satisfiedAssessmentWithEvidenceRef(ref string) map[string]any {
	return map[string]any{
		"satisfied": true,
		"summary":   "accepted output candidate",
		"items": []map[string]any{{
			"item":         "item",
			"state":        "satisfied",
			"evidenceRefs": []string{ref},
		}},
	}
}

func rejectedAssessmentWithEvidenceRef(ref string) map[string]any {
	assessment := satisfiedAssessmentWithEvidenceRef(ref)
	assessment["satisfied"] = false
	assessment["summary"] = "rejected output"
	assessment["items"] = []map[string]any{{
		"item":          "item",
		"state":         "failed",
		"evidenceRefs":  []string{ref},
		"failureReason": "candidate output was rejected",
	}}
	return assessment
}

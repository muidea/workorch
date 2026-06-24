package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRecordValidationResultWritesAcceptedAndRejectedEvidenceRecords(t *testing.T) {
	ctx := context.Background()
	validationRepo := memory.NewValidationResultRepository()
	evidenceRepo := memory.NewEvidenceRecordRepository()
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			ValidationResults: validationRepo,
			EvidenceRecords:   evidenceRepo,
		},
	}
	run := &runmodel.Run{
		ID:                 "run-validation-evidence",
		WorkspaceID:        "ws-validation-evidence",
		DefinitionRevision: "rev-validation-evidence",
	}
	passedStep := &runmodel.Step{
		ID:     "run-validation-evidence/step-passed",
		RunID:  run.ID,
		State:  common.StepStateCompleted,
		Output: map[string]any{"status": "ok"},
	}
	failedStep := &runmodel.Step{
		ID:     "run-validation-evidence/step-failed",
		RunID:  run.ID,
		State:  common.StepStateFailed,
		Output: map[string]any{"status": "failed", "error": "runtime validation failed"},
	}

	if err := orch.recordValidationResult(ctx, run, passedStep, common.ValidationResultStatePassed, "runtime step completed", "tool-call-passed", ""); err != nil {
		t.Fatalf("record passed validation: %v", err)
	}
	if err := orch.recordValidationResult(ctx, run, failedStep, common.ValidationResultStateFailed, "runtime step failed", "tool-call-failed", "runtime validation failed"); err != nil {
		t.Fatalf("record failed validation: %v", err)
	}

	validations, err := validationRepo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list validations: %v", err)
	}
	if len(validations) != 2 {
		t.Fatalf("validations = %+v, want two validation results", validations)
	}
	records, err := evidenceRepo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list evidence records: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %+v, want two validation evidence records", records)
	}
	recordsBySource := map[string]*orchestratormodel.EvidenceRecord{}
	for _, record := range records {
		if record.ID == "" || strings.ContainsAny(string(record.ID), `/\`) {
			t.Fatalf("record ID = %q, want file-repository-safe ID", record.ID)
		}
		recordsBySource[record.SourceRef] = record
		if record.WorkspaceID != run.WorkspaceID ||
			record.RunID != run.ID ||
			record.DefinitionRevision != run.DefinitionRevision ||
			record.Kind != orchestratormodel.EvidenceKindValidation ||
			record.SourceType != evidenceRecordSourceTypeValidationResult ||
			record.SubjectType != evidenceRecordSubjectRuntimeStep {
			t.Fatalf("record = %+v, want scoped validation evidence", record)
		}
		if record.Metadata["validationKind"] != "runtime_step" ||
			record.Metadata["targetRef"] != record.SubjectID ||
			record.Metadata["validationResultId"] != record.SourceRef {
			t.Fatalf("metadata = %+v, want validation trace metadata", record.Metadata)
		}
	}

	passedID := "run-validation-evidence/step-passed/validation/runtime-step"
	passedRecord := recordsBySource[passedID]
	if passedRecord == nil ||
		passedRecord.Status != orchestratormodel.EvidenceStatusAccepted ||
		passedRecord.StepID != passedStep.ID ||
		passedRecord.SubjectID != string(passedStep.ID) ||
		passedRecord.Metadata["validationState"] != string(common.ValidationResultStatePassed) ||
		passedRecord.Metadata["evidenceRef"] != "tool-call-passed" {
		t.Fatalf("passed record = %+v, want accepted validation evidence", passedRecord)
	}
	failedID := "run-validation-evidence/step-failed/validation/runtime-step"
	failedRecord := recordsBySource[failedID]
	if failedRecord == nil ||
		failedRecord.Status != orchestratormodel.EvidenceStatusRejected ||
		failedRecord.StepID != failedStep.ID ||
		failedRecord.SubjectID != string(failedStep.ID) ||
		failedRecord.Metadata["validationState"] != string(common.ValidationResultStateFailed) ||
		failedRecord.Metadata["evidenceRef"] != "tool-call-failed" {
		t.Fatalf("failed record = %+v, want rejected validation evidence", failedRecord)
	}
	bySource, err := evidenceRepo.ListBySourceRef(ctx, run.ID, failedID)
	if err != nil {
		t.Fatalf("list failed evidence by sourceRef: %v", err)
	}
	if len(bySource) != 1 || bySource[0].ID != failedRecord.ID {
		t.Fatalf("bySource = %+v, want failed record %q", bySource, failedRecord.ID)
	}
}

func TestRecordContractViolationWritesRejectedDiagnosticEvidenceRecord(t *testing.T) {
	ctx := context.Background()
	evidenceRepo := memory.NewEvidenceRecordRepository()
	resultRepo := memory.NewResultRepository()
	artifactRepo := memory.NewArtifactRepository()
	memoryRepo := memory.NewMemoryItemRepository()
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			EvidenceRecords: evidenceRepo,
			Results:         resultRepo,
			Artifacts:       artifactRepo,
			MemoryItems:     memoryRepo,
		},
	}
	run := &runmodel.Run{
		ID:                 "run-diagnostic-evidence",
		WorkspaceID:        "ws-diagnostic-evidence",
		DefinitionRevision: "rev-diagnostic-evidence",
		InitialStepKind:    llmWorkspaceExecutionStepKind,
	}
	failedStep := contractViolationFailedStep(run.ID, "failed-diagnostic")
	attachContractViolationDiagnostics(run, failedStep, errors.New("state-changing llm repair step returned no state-changing output"))

	if err := orch.recordRejectedDiagnosticEvidenceForContractViolation(ctx, run, failedStep); err != nil {
		t.Fatalf("record rejected diagnostic evidence: %v", err)
	}
	if err := orch.recordRejectedDiagnosticEvidenceForContractViolation(ctx, run, failedStep); err != nil {
		t.Fatalf("record rejected diagnostic evidence again: %v", err)
	}

	records, err := evidenceRepo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list evidence records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %+v, want one idempotent diagnostic evidence record", records)
	}
	record := records[0]
	if record.ID == "" || strings.ContainsAny(string(record.ID), `/\`) {
		t.Fatalf("record ID = %q, want file-repository-safe ID", record.ID)
	}
	if record.WorkspaceID != run.WorkspaceID ||
		record.RunID != run.ID ||
		record.StepID != failedStep.ID ||
		record.DefinitionRevision != run.DefinitionRevision ||
		record.Kind != orchestratormodel.EvidenceKindDiagnostic ||
		record.Status != orchestratormodel.EvidenceStatusRejected ||
		record.SourceType != evidenceRecordSourceTypeContractViolation ||
		record.SubjectType != evidenceRecordSubjectRuntimeStep ||
		record.SubjectID != string(failedStep.ID) {
		t.Fatalf("record = %+v, want scoped rejected diagnostic evidence", record)
	}
	if record.Metadata["violatedRule"] != contractViolationRuleStateChangingRequiredMissing ||
		record.Metadata["violationType"] != contractViolationRepairNoStateChangingOutput ||
		record.Metadata["failureClass"] != failureClassRuntime ||
		record.Metadata["rejectedOutputRef"] == "" {
		t.Fatalf("metadata = %+v, want contract violation diagnostic refs", record.Metadata)
	}
	if len(mapFromValue(record.Metadata["contractViolation"])) == 0 {
		t.Fatalf("metadata = %+v, want nested contractViolation snapshot", record.Metadata)
	}
	bySource, err := evidenceRepo.ListBySourceRef(ctx, run.ID, record.SourceRef)
	if err != nil {
		t.Fatalf("list evidence by sourceRef: %v", err)
	}
	if len(bySource) != 1 || bySource[0].ID != record.ID {
		t.Fatalf("bySource = %+v, want diagnostic record %q", bySource, record.ID)
	}
	if results, err := resultRepo.ListByRun(ctx, run.ID); err != nil || len(results) != 0 {
		t.Fatalf("results = %+v err=%v, want no result writes", results, err)
	}
	if artifacts, err := artifactRepo.ListByRun(ctx, run.ID); err != nil || len(artifacts) != 0 {
		t.Fatalf("artifacts = %+v err=%v, want no artifact writes", artifacts, err)
	}
	if memories, err := memoryRepo.ListByWorkspaceID(ctx, run.WorkspaceID); err != nil || len(memories) != 0 {
		t.Fatalf("memories = %+v err=%v, want no memory writes", memories, err)
	}
}

func TestHandleRuntimeStepFailureRecordsRejectedDiagnosticEvidence(t *testing.T) {
	ctx := context.Background()
	stepRepo := memory.NewStepRepository()
	evidenceRepo := memory.NewEvidenceRecordRepository()
	validationRepo := memory.NewValidationResultRepository()
	orch := &RuntimeOrchestrator{
		steps: stepRepo,
		evidence: RuntimeEvidenceRepositories{
			EvidenceRecords:   evidenceRepo,
			ValidationResults: validationRepo,
		},
	}
	run := &runmodel.Run{
		ID:                 "run-diagnostic-failure-path",
		WorkspaceID:        "ws-diagnostic-failure-path",
		DefinitionRevision: "rev-diagnostic-failure-path",
		InitialStepKind:    llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:     "run-diagnostic-failure-path/step-runtime-contract",
		RunID:  run.ID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateDispatched,
		Input:  cloneStepPayload(contractViolationFailedStep(run.ID, "seed").Input),
		Output: map[string]any{},
	}
	step.Input[stepInputKeyGraphBatch] = true
	if err := stepRepo.Create(ctx, step); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	err := orch.handleRuntimeStepFailure(ctx, run, step, nil, map[string]any{"actualOutputSummary": "rejected"}, "runtime execution contract failed", newRuntimeContractViolationError(contractViolationRepairNoStateChangingOutput, "state-changing llm repair step returned no state-changing output"))
	if err == nil {
		t.Fatalf("handleRuntimeStepFailure err = nil, want graph-batch contract error")
	}

	records, err := evidenceRepo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list evidence records: %v", err)
	}
	diagnostics := 0
	validations := 0
	for _, record := range records {
		switch record.Kind {
		case orchestratormodel.EvidenceKindDiagnostic:
			diagnostics++
			if record.Status != orchestratormodel.EvidenceStatusRejected ||
				record.SourceType != evidenceRecordSourceTypeContractViolation ||
				record.SubjectID != string(step.ID) ||
				record.Metadata["violatedRule"] != contractViolationRuleStateChangingRequiredMissing {
				t.Fatalf("diagnostic record = %+v, want rejected contract violation evidence", record)
			}
		case orchestratormodel.EvidenceKindValidation:
			validations++
		}
	}
	if diagnostics != 1 || validations != 1 {
		t.Fatalf("records = %+v, want one diagnostic and one validation evidence record", records)
	}
}

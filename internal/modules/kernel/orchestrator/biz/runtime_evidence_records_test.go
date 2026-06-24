package orchestrator

import (
	"context"
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

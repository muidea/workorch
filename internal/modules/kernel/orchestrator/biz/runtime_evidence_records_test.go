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

func TestRecordAcceptedTargetSnapshotEvidenceForCompletedFileRead(t *testing.T) {
	ctx := context.Background()
	evidenceRepo := memory.NewEvidenceRecordRepository()
	orch := &RuntimeOrchestrator{
		evidence: RuntimeEvidenceRepositories{
			EvidenceRecords: evidenceRepo,
		},
	}
	run := &runmodel.Run{
		ID:                 "run-target-snapshot-evidence",
		WorkspaceID:        "ws-target-snapshot-evidence",
		DefinitionRevision: "rev-target-snapshot-evidence",
	}
	step := &runmodel.Step{
		ID:    "run-target-snapshot-evidence/read-target",
		RunID: run.ID,
		Kind:  "file/read",
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyGraphRootStepID: "run-target-snapshot-evidence/root",
			stepInputKeyGraphNodeID:     "read-target-node",
		},
		Output: map[string]any{
			"stage":            "file_read",
			"path":             "target.txt",
			"exists":           true,
			"content":          "current target content\n",
			"contentHash":      "hash-target",
			"contentComplete":  true,
			"safeForFileWrite": true,
			"truncated":        false,
			"isBinary":         false,
			"size":             23,
		},
	}

	if err := orch.recordAcceptedTargetSnapshotEvidenceForStep(ctx, run, step); err != nil {
		t.Fatalf("record target snapshot evidence: %v", err)
	}
	if err := orch.recordAcceptedTargetSnapshotEvidenceForStep(ctx, run, step); err != nil {
		t.Fatalf("record target snapshot evidence again: %v", err)
	}

	records, err := evidenceRepo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list evidence records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %+v, want one idempotent target snapshot evidence record", records)
	}
	record := records[0]
	if record.ID == "" || strings.ContainsAny(string(record.ID), `/\`) {
		t.Fatalf("record ID = %q, want file-repository-safe ID", record.ID)
	}
	if record.WorkspaceID != run.WorkspaceID ||
		record.RunID != run.ID ||
		record.StepID != step.ID ||
		record.DefinitionRevision != run.DefinitionRevision ||
		record.Kind != orchestratormodel.EvidenceKindTargetSnapshot ||
		record.Status != orchestratormodel.EvidenceStatusAccepted ||
		record.SourceType != evidenceRecordSourceTypeTargetSnapshot ||
		record.SubjectType != evidenceRecordSubjectWorkspaceItem ||
		record.SubjectID != "target.txt" {
		t.Fatalf("record = %+v, want scoped accepted target snapshot evidence", record)
	}
	if record.Metadata["targetRef"] != "target.txt" ||
		record.Metadata["contentHash"] != "hash-target" ||
		record.Metadata["contentComplete"] != true ||
		record.Metadata["safeForFileWrite"] != true ||
		record.Metadata["snapshotIntegrity"] != "complete_unredacted_text" ||
		record.Metadata["snapshotValidity"] != "valid_for_recorded_step_output" ||
		record.Metadata["capturedAt"] != record.CreatedAt ||
		record.Metadata["graphRootStepId"] != "run-target-snapshot-evidence/root" ||
		record.Metadata["graphNodeId"] != "read-target-node" ||
		record.Metadata["sourceOutputRef"] != record.SourceRef {
		t.Fatalf("metadata = %+v, want target snapshot trace metadata", record.Metadata)
	}
	if _, ok := record.Metadata["content"]; ok {
		t.Fatalf("metadata = %+v, want content kept in step output ref rather than evidence metadata", record.Metadata)
	}
	bySource, err := evidenceRepo.ListBySourceRef(ctx, run.ID, record.SourceRef)
	if err != nil {
		t.Fatalf("list evidence by sourceRef: %v", err)
	}
	if len(bySource) != 1 || bySource[0].ID != record.ID {
		t.Fatalf("bySource = %+v, want target snapshot record %q", bySource, record.ID)
	}
}

func TestRecordAcceptedTargetSnapshotEvidenceRejectsUnsafeFileRead(t *testing.T) {
	ctx := context.Background()
	run := &runmodel.Run{
		ID:                 "run-unsafe-target-snapshot-evidence",
		WorkspaceID:        "ws-unsafe-target-snapshot-evidence",
		DefinitionRevision: "rev-unsafe-target-snapshot-evidence",
	}
	cases := map[string]map[string]any{
		"truncated": {
			"stage":            "file_read",
			"path":             "target-truncated.txt",
			"exists":           true,
			"content":          "partial",
			"contentComplete":  false,
			"safeForFileWrite": false,
			"truncated":        true,
		},
		"redacted": {
			"stage":            "file_read",
			"path":             "target-redacted.txt",
			"exists":           true,
			"content":          "[workorch redacted runtime content]",
			"contentComplete":  true,
			"safeForFileWrite": false,
			"redacted":         true,
		},
		"binary": {
			"stage":    "file_read",
			"path":     "target-binary.bin",
			"exists":   true,
			"isBinary": true,
		},
		"missing": {
			"stage":  "file_read",
			"path":   "target-missing.txt",
			"exists": false,
		},
	}
	for name, output := range cases {
		t.Run(name, func(t *testing.T) {
			evidenceRepo := memory.NewEvidenceRecordRepository()
			orch := &RuntimeOrchestrator{
				evidence: RuntimeEvidenceRepositories{
					EvidenceRecords: evidenceRepo,
				},
			}
			step := &runmodel.Step{
				ID:     common.StepID("run-unsafe-target-snapshot-evidence/" + name),
				RunID:  run.ID,
				Kind:   "file/read",
				State:  common.StepStateCompleted,
				Output: output,
			}
			if err := orch.recordAcceptedTargetSnapshotEvidenceForStep(ctx, run, step); err != nil {
				t.Fatalf("record target snapshot evidence: %v", err)
			}
			records, err := evidenceRepo.ListByRun(ctx, run.ID)
			if err != nil {
				t.Fatalf("list evidence records: %v", err)
			}
			if len(records) != 0 {
				t.Fatalf("records = %+v, want no accepted target snapshot evidence for unsafe output", records)
			}
		})
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

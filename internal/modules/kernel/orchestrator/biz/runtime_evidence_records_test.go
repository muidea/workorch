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
		ID:    "run-validation-evidence/step-failed",
		RunID: run.ID,
		State: common.StepStateFailed,
		Output: map[string]any{
			"status": "failed",
			"error":  "runtime validation failed",
			"validationFailureDiagnostic": map[string]any{
				"class": orchestratormodel.ValidationDiagnosticBuildFailed,
			},
			failureOutputKeySignature: "validation-failure-signature",
		},
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
	if diagnostic := mapFromValue(failedRecord.Metadata["validationFailureDiagnostic"]); diagnostic["class"] != orchestratormodel.ValidationDiagnosticBuildFailed ||
		failedRecord.Metadata["failureSignature"] != "validation-failure-signature" {
		t.Fatalf("failed record metadata = %+v, want validation failure diagnostic", failedRecord.Metadata)
	}
	bySource, err := evidenceRepo.ListBySourceRef(ctx, run.ID, failedID)
	if err != nil {
		t.Fatalf("list failed evidence by sourceRef: %v", err)
	}
	if len(bySource) != 1 || bySource[0].ID != failedRecord.ID {
		t.Fatalf("bySource = %+v, want failed record %q", bySource, failedRecord.ID)
	}
}

func TestRecordCompletedRuntimeStepEvidenceWritesScopedStateChangeForDeclaredStateChangingStep(t *testing.T) {
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
		ID:                 "run-runtime-state-change-evidence",
		WorkspaceID:        "ws-runtime-state-change-evidence",
		DefinitionRevision: "rev-runtime-state-change-evidence",
	}
	step := &runmodel.Step{
		ID:    "run-runtime-state-change-evidence/step-state-change",
		RunID: run.ID,
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Input: map[string]any{
			"stateChanging":       true,
			"coverageFocusId":     "focus-cwi-001",
			"coverageWorkItemIds": []string{"cwi-001"},
			"coverageTargets":     []string{"neutral-deliverable"},
			"targetRefs":          []string{"workspace://neutral-deliverable"},
			"validationEvidence": map[string]any{
				"mode":            "runtime_behavior",
				"observationType": "runtime_observation",
				"targetRefs":      []string{"workspace://neutral-deliverable"},
				"observedClaims":  []string{"neutral deliverable behavior was observed"},
			},
		},
		Output: map[string]any{"exitCode": 0},
	}
	toolCall := &runmodel.ToolCall{ID: "run-runtime-state-change-evidence/step-state-change/tool-call/runtime-execute"}

	if err := orch.recordCompletedRuntimeStepEvidence(ctx, run, step, toolCall, step.Output); err != nil {
		t.Fatalf("record completed runtime step evidence: %v", err)
	}
	if err := orch.recordCompletedRuntimeStepEvidence(ctx, run, step, toolCall, step.Output); err != nil {
		t.Fatalf("record completed runtime step evidence again: %v", err)
	}

	records, err := evidenceRepo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list evidence records: %v", err)
	}
	var stateChanges []*orchestratormodel.EvidenceRecord
	var validations []*orchestratormodel.EvidenceRecord
	for _, record := range records {
		switch record.Kind {
		case orchestratormodel.EvidenceKindStateChange:
			stateChanges = append(stateChanges, record)
		case orchestratormodel.EvidenceKindValidation:
			validations = append(validations, record)
		}
	}
	if len(stateChanges) != 1 || len(validations) != 1 {
		t.Fatalf("records = %+v, want exactly one state_change and one validation", records)
	}
	stateChange := stateChanges[0]
	if stateChange.Status != orchestratormodel.EvidenceStatusAccepted ||
		stateChange.SourceType != evidenceRecordSourceTypeRuntimeStep ||
		stateChange.StepID != step.ID ||
		stateChange.Metadata["stateChanging"] != true ||
		stateChange.Metadata["coverageFocusId"] != "focus-cwi-001" ||
		stateChange.Metadata["toolCallId"] != string(toolCall.ID) ||
		stateChange.Metadata["sourceOutputRef"] != stateChange.SourceRef {
		t.Fatalf("state_change record = %+v, want scoped accepted runtime step evidence", stateChange)
	}
	if got := stateChange.CoverageItemRefs; len(got) != 1 || got[0] != "cwi-001" {
		t.Fatalf("CoverageItemRefs = %+v, want cwi-001", got)
	}
	if got := stringSliceFromValue(stateChange.Metadata["coverageTargets"]); len(got) != 1 || got[0] != "neutral-deliverable" {
		t.Fatalf("state_change metadata coverageTargets = %+v, want neutral-deliverable", stateChange.Metadata)
	}
	validation := validations[0]
	if validation.Status != orchestratormodel.EvidenceStatusAccepted ||
		validation.Metadata["coverageFocusId"] != "focus-cwi-001" ||
		validation.Metadata["mode"] != "runtime_behavior" ||
		validation.Metadata["observationType"] != "runtime_observation" {
		t.Fatalf("validation record = %+v, want accepted validation with coverage scope", validation)
	}
	if got := validation.CoverageItemRefs; len(got) != 1 || got[0] != "cwi-001" {
		t.Fatalf("validation CoverageItemRefs = %+v, want cwi-001", got)
	}
	if got := stringSliceFromValue(validation.Metadata["targetRefs"]); !stringSliceContains(got, "workspace://neutral-deliverable") {
		t.Fatalf("validation targetRefs = %+v, want proof target ref", got)
	}
	if got := stringSliceFromValue(validation.Metadata["observedClaims"]); !stringSliceContains(got, "neutral deliverable behavior was observed") {
		t.Fatalf("validation observedClaims = %+v, want proof claim", got)
	}
}

func TestRecordCompletedRuntimeStepEvidenceBindsGitWriteCommitFromRequiredOutput(t *testing.T) {
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
		ID:                 "run-git-write-state-change-evidence",
		WorkspaceID:        "ws-git-write-state-change-evidence",
		DefinitionRevision: "rev-git-write-state-change-evidence",
	}
	step := &runmodel.Step{
		ID:    "run-git-write-state-change-evidence/graph-001/002-git-commit",
		RunID: run.ID,
		Kind:  orchestratormodel.StepKindGitWrite,
		State: common.StepStateCompleted,
		Input: map[string]any{
			"operation": "commit",
			"message":   "record deliverable snapshot",
			"coverageFocus": map[string]any{
				"id":                  "cwi-git-snapshot",
				"coverageWorkItemIds": []string{"cwi-git-snapshot"},
				"targetRefs":          []string{"deliverable:version_snapshot"},
				"structuredTargetRefs": []string{
					"cwi-git-snapshot",
					"deliverable:version_snapshot",
				},
			},
			"requiredOutput": map[string]any{
				"acceptedKinds":                []string{orchestratormodel.StepKindGitWrite},
				"mustIncludeStateChangingNode": true,
				"coverageWorkItemIds":          []string{"cwi-git-snapshot"},
				"targetRefs":                   []string{"cwi-git-snapshot", "deliverable:version_snapshot"},
				"structuredTargetRefs":         []string{"cwi-git-snapshot", "deliverable:version_snapshot"},
			},
		},
		Output: map[string]any{
			"operation": "commit",
			"summary":   "git commit completed",
		},
	}

	if err := orch.recordCompletedRuntimeStepEvidence(ctx, run, step, nil, step.Output); err != nil {
		t.Fatalf("record completed runtime step evidence: %v", err)
	}

	records, err := evidenceRepo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list evidence records: %v", err)
	}
	var stateChange *orchestratormodel.EvidenceRecord
	for _, record := range records {
		if record.Kind == orchestratormodel.EvidenceKindStateChange {
			stateChange = record
			break
		}
	}
	if stateChange == nil {
		t.Fatalf("records = %+v, want git/write commit state_change evidence", records)
	}
	if got := stateChange.CoverageItemRefs; len(got) != 1 || got[0] != "cwi-git-snapshot" {
		t.Fatalf("CoverageItemRefs = %+v, want cwi-git-snapshot", got)
	}
	if stateChange.Metadata["operation"] != "commit" ||
		stateChange.Metadata["stepKind"] != orchestratormodel.StepKindGitWrite ||
		stateChange.Metadata["stateChanging"] != true {
		t.Fatalf("state_change metadata = %+v, want git/write commit state-changing metadata", stateChange.Metadata)
	}
	if got := stringSliceFromValue(stateChange.Metadata["structuredTargetRefs"]); !stringSliceContains(got, "deliverable:version_snapshot") {
		t.Fatalf("structuredTargetRefs = %+v, want deliverable snapshot binding", got)
	}
}

func TestRecordCompletedRuntimeStepEvidenceDoesNotWriteStateChangeForValidationOnlyStep(t *testing.T) {
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
		ID:                 "run-runtime-validation-only-evidence",
		WorkspaceID:        "ws-runtime-validation-only-evidence",
		DefinitionRevision: "rev-runtime-validation-only-evidence",
	}
	step := &runmodel.Step{
		ID:    "run-runtime-validation-only-evidence/step-validate",
		RunID: run.ID,
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Input: map[string]any{
			"coverageWorkItemIds": []string{"cwi-validate"},
			"validationEvidence": map[string]any{
				"mode":            "runtime_behavior",
				"observationType": "runtime_observation",
			},
		},
		Output: map[string]any{"exitCode": 0},
	}

	if err := orch.recordCompletedRuntimeStepEvidence(ctx, run, step, nil, step.Output); err != nil {
		t.Fatalf("record completed runtime step evidence: %v", err)
	}
	records, err := evidenceRepo.ListByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("list evidence records: %v", err)
	}
	for _, record := range records {
		if record.Kind == orchestratormodel.EvidenceKindStateChange {
			t.Fatalf("records = %+v, want no state_change for validation-only step", records)
		}
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

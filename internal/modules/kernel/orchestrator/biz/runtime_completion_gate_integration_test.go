package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	shellcommon "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestRuntimeRepairInputForSatisfiedDoDCoverageMissingIsAssessmentOnly(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-dod-coverage-assessment-repair")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-dod-coverage-assessment-repair", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}
	stepRepo := memory.NewStepRepository()
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(runID + "/graph-001/003-test"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Output: map[string]any{
			"stdout": "TestStream passed; token statistics include prompt tokens and completion tokens.",
		},
	}); err != nil {
		t.Fatalf("seed evidence step: %v", err)
	}
	step := &runmodel.Step{
		ID:    common.StepID(runID + "/follow-up-graph-test"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			"graphFollowUp": true,
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|stage=llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"summary":   "DoD passed but goal coverage is missing.",
				"items": []map[string]any{{
					"item":         "tests pass",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-dod-coverage-assessment-repair/graph-001/003-test"},
				}},
			},
			"runtimeDiagnostics": map[string]any{
				"kind": failureClassDoDCoverageMissing,
				"missingGoalCoverage": []map[string]any{{
					"raw":   "streaming token accounting",
					"kind":  "leaf",
					"state": "missing",
				}},
			},
		},
	}
	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{},
		memory.NewRunRepository(),
		stepRepo,
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		memory.NewApprovalRepository(),
		events.NewMemoryHub(),
	)

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, step, 1, errors.New("missing goal coverage"))

	if input[stepInputKeyAssessmentOnly] != true || input[stepInputKeyRuntimeStage] != runtimeStageVerification || input[stepInputKeySoulStage] != soulStageValidation {
		t.Fatalf("input = %+v, want assessment-only verification repair", input)
	}
	if boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("input = %+v, want no state-changing requirement for coverage correction", input)
	}
	if input["repairObjective"] != "complete_dod_goal_coverage_assessment" {
		t.Fatalf("repairObjective = %v, want coverage assessment correction", input["repairObjective"])
	}
	if missing := mapSliceFromValue(input["missingGoalCoverage"]); len(missing) != 1 {
		t.Fatalf("missingGoalCoverage = %+v, want one item", input["missingGoalCoverage"])
	} else if strings.TrimSpace(stringFromMap(missing[0], "goalRequirementId")) == "" {
		t.Fatalf("missingGoalCoverage = %+v, want stable goalRequirementId", input["missingGoalCoverage"])
	} else if candidates := mapSliceFromValue(missing[0]["evidenceCandidates"]); len(candidates) == 0 {
		t.Fatalf("missingGoalCoverage = %+v, want neutral evidence candidates", input["missingGoalCoverage"])
	}
}

func TestRuntimeRepairInputIncludesLatestClosureEvidenceForCoverageCorrection(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-dod-coverage-latest-closure")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-dod-coverage-latest-closure", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}
	stepRepo := memory.NewStepRepository()
	patchRepo := memory.NewPatchRepository()
	validationRepo := memory.NewValidationResultRepository()
	oldFailedStepID := common.StepID(runID + "/graph-006/002-rerun-targeted-goal-test")
	latestSuccessStepID := common.StepID(runID + "/graph-007/002-rerun-targeted-goal-test")
	for _, step := range []*runmodel.Step{
		{ID: oldFailedStepID, RunID: runID, Kind: shellcommon.StepKindCommand, State: common.StepStateFailed, Output: map[string]any{"stderr": "old failure"}},
		{ID: latestSuccessStepID, RunID: runID, Kind: shellcommon.StepKindCommand, State: common.StepStateCompleted, Output: map[string]any{"stdout": "latest targeted validation passed"}},
	} {
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}
	if err := patchRepo.Create(ctx, &runmodel.Patch{
		ID:          common.PatchID(runID + "/graph-007/001-apply-patch/patch/main-go"),
		WorkspaceID: run.WorkspaceID,
		RunID:       runID,
		StepID:      latestSuccessStepID,
		PatchType:   "unified_diff",
		TargetRef:   "main.go",
		Summary:     "applied target change",
		State:       common.PatchStateApplied,
	}); err != nil {
		t.Fatalf("seed patch: %v", err)
	}
	if err := validationRepo.Create(ctx, &runmodel.ValidationResult{
		ID:              common.ValidationResultID(string(latestSuccessStepID) + "/validation/runtime-step"),
		WorkspaceID:     run.WorkspaceID,
		RunID:           runID,
		StepID:          latestSuccessStepID,
		ValidationKind:  "runtime_step",
		TargetRef:       "main.go",
		State:           common.ValidationResultStatePassed,
		Summary:         "targeted validation passed",
		EvidenceQuality: common.EvidenceQualityValid,
	}); err != nil {
		t.Fatalf("seed validation: %v", err)
	}
	failedAssessment := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-002-of-graph-006-002-rerun-targeted-goal-test"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyAssessmentOnly:        true,
			"repairObjective":                 "complete_dod_goal_coverage_assessment",
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": "goal-req-latest-closure",
				"raw":               "targeted goal validation",
				"evidenceCandidates": []map[string]any{{
					"stepId": string(latestSuccessStepID),
					"kind":   shellcommon.StepKindCommand,
				}},
			}},
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|stage=llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   "old failure still cited",
				"items": []map[string]any{{
					"item":         "targeted goal validation",
					"state":        "satisfied",
					"evidenceRefs": []string{string(oldFailedStepID)},
				}},
				"goalCoverage": map[string]any{
					"satisfied": false,
					"items": []map[string]any{{
						"goalRequirementId": "goal-req-latest-closure",
						"item":              "targeted goal validation",
						"state":             "failed",
						"reason":            "old failure was cited",
					}},
				},
			},
			"runtimeDiagnostics": map[string]any{
				"kind": failureClassDoDCoverageMissing,
				"missingGoalCoverage": []map[string]any{{
					"goalRequirementId": "goal-req-latest-closure",
					"raw":               "targeted goal validation",
					"state":             "missing",
				}},
			},
		},
	}
	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{},
		memory.NewRunRepository(),
		stepRepo,
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		memory.NewApprovalRepository(),
		events.NewMemoryHub(),
	)
	orch.evidence.Patches = patchRepo
	orch.evidence.ValidationResults = validationRepo

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedAssessment, 2, errors.New("missing goal coverage"))

	closure := mapFromValue(input["latestClosureEvidence"])
	if len(closure) == 0 {
		t.Fatalf("latestClosureEvidence missing from repair input: %+v", input)
	}
	latest := mapSliceFromValue(closure["latestEvidence"])
	if len(latest) == 0 || stringFromMap(latest[0], "evidenceRef") != string(latestSuccessStepID) {
		t.Fatalf("latestClosureEvidence = %+v, want latest successful step", closure)
	}
	if patches := mapSliceFromValue(closure["latestPatchEvidence"]); len(patches) == 0 || patches[0]["targetRef"] != "main.go" {
		t.Fatalf("latestPatchEvidence = %+v, want applied patch evidence", closure["latestPatchEvidence"])
	}
	if validations := mapSliceFromValue(closure["latestValidationEvidence"]); len(validations) == 0 || validations[0]["targetRef"] != "main.go" {
		t.Fatalf("latestValidationEvidence = %+v, want passed validation evidence", closure["latestValidationEvidence"])
	}
	superseded := mapSliceFromValue(input["supersededFailureEvidence"])
	if len(superseded) == 0 || stringFromMap(superseded[0], "supersededBy") != string(latestSuccessStepID) {
		t.Fatalf("supersededFailureEvidence = %+v, want old failure mapped to latest success", input["supersededFailureEvidence"])
	}
	if !strings.Contains(strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n"), "latestClosureEvidence") {
		t.Fatalf("repairGuidance = %+v, want latestClosureEvidence guidance", input["repairGuidance"])
	}
}

func TestRuntimeRepairInputTreatsGraphLimitAssessmentAsCoverageCorrection(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-graph-limit-coverage-correction")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-graph-limit-coverage-correction", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}
	stepRepo := memory.NewStepRepository()
	evidenceStepID := common.StepID(runID + "/graph-003/002-run-validation")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     evidenceStepID,
		RunID:  runID,
		Kind:   shellcommon.StepKindCommand,
		State:  common.StepStateCompleted,
		Output: map[string]any{"stdout": "validation passed"},
	}); err != nil {
		t.Fatalf("seed evidence step: %v", err)
	}
	failedAssessment := &runmodel.Step{
		ID:    common.StepID(runID + "/follow-up-graph-limit-assessment"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyAssessmentOnly:     true,
			stepInputKeyGraphLimitFollowUp: true,
			"assessmentOnlyReason":         "graph_follow_up_expansion_limit_exceeded",
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|stage=llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   "coverage missing",
				"items": []map[string]any{{
					"item":         "validation requirement",
					"state":        "failed",
					"evidenceRefs": []string{string(runID)},
				}},
				"goalCoverage": map[string]any{
					"satisfied": false,
					"items": []map[string]any{{
						"goalRequirementId": "goal-req-validation",
						"item":              "validation requirement",
						"state":             "failed",
						"reason":            "missing concrete evidence refs",
					}},
				},
			},
			"runtimeDiagnostics": map[string]any{
				"kind": failureClassDoDCoverageMissing,
				"missingGoalCoverage": []map[string]any{{
					"goalRequirementId": "goal-req-validation",
					"raw":               "validation requirement",
					"state":             "failed",
				}},
			},
		},
	}
	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{},
		memory.NewRunRepository(),
		stepRepo,
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		memory.NewApprovalRepository(),
		events.NewMemoryHub(),
	)

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedAssessment, 1, errors.New("coverage missing"))

	if input[stepInputKeyAssessmentOnly] != true || input["repairObjective"] != "complete_dod_goal_coverage_assessment" {
		t.Fatalf("input = %+v, want graph-limit assessment coverage correction", input)
	}
	missingGoal := mapSliceFromValue(input["missingGoalCoverage"])
	if len(missingGoal) == 0 || len(mapSliceFromValue(missingGoal[0]["evidenceCandidates"])) == 0 {
		t.Fatalf("missingGoalCoverage = %+v, want evidence candidates", input["missingGoalCoverage"])
	}
	missingDoD := mapSliceFromValue(input["missingDoDItems"])
	if len(missingDoD) == 0 || len(mapSliceFromValue(missingDoD[0]["evidenceCandidates"])) == 0 {
		t.Fatalf("missingDoDItems = %+v, want evidence candidates", input["missingDoDItems"])
	}
	if closure := mapFromValue(input["latestClosureEvidence"]); len(closure) == 0 {
		t.Fatalf("latestClosureEvidence missing from coverage correction input: %+v", input)
	}
}

func TestRuntimeRepairInputForMissingDeliverableCoverageIsAssessmentOnly(t *testing.T) {
	runID := common.RunID("run-deliverable-coverage-assessment-repair")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-deliverable-coverage-assessment-repair",
		DoDID:           "dod",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "source_change", "required": true},
							{"id": "validation_report", "required": true},
						},
					},
				},
			},
		},
	}
	step := &runmodel.Step{
		ID:    common.StepID(runID + "/follow-up-graph-test"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			"graphFollowUp": true,
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|stage=llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"summary":   "DoD passed but deliverable coverage is missing.",
				"items": []map[string]any{{
					"item":         "tests pass",
					"state":        "satisfied",
					"evidenceRefs": []string{"run-deliverable-coverage-assessment-repair/graph-001/003-test"},
				}},
			},
			"runtimeDiagnostics": map[string]any{
				"kind": failureClassDoDCoverageMissing,
				"missingDeliverableCoverage": []map[string]any{{
					"id":    "source_change",
					"kind":  "deliverableExpectation",
					"state": "missing",
				}},
			},
		},
	}
	orch := NewRuntimeOrchestrator(
		&stubRuntimeAdapter{},
		memory.NewRunRepository(),
		memory.NewStepRepository(),
		memory.NewWorkspaceRepository(),
		memory.NewDecisionTicketRepository(),
		memory.NewApprovalRepository(),
		events.NewMemoryHub(),
	)

	input := orch.runtimeRepairFollowUpStepInput(context.Background(), run, step, 1, errors.New("missing deliverable coverage"))

	if input[stepInputKeyAssessmentOnly] != true || input[stepInputKeyRuntimeStage] != runtimeStageVerification || input[stepInputKeySoulStage] != soulStageValidation {
		t.Fatalf("input = %+v, want assessment-only verification repair", input)
	}
	if boolValue(input["stateChangingOutputRequired"]) {
		t.Fatalf("input = %+v, want no state-changing requirement for deliverable coverage correction", input)
	}
	if missing := mapSliceFromValue(input["missingDeliverableCoverage"]); len(missing) != 1 || missing[0]["id"] != "source_change" {
		t.Fatalf("missingDeliverableCoverage = %+v, want source_change", input["missingDeliverableCoverage"])
	}
	if required := stringSliceFromValue(input["requiredDeliverableCoverage"]); !reflect.DeepEqual(required, []string{"source_change", "validation_report"}) {
		t.Fatalf("requiredDeliverableCoverage = %+v, want profile required deliverables", input["requiredDeliverableCoverage"])
	}
}

func TestRuntimeRepairInputForFailedAssessmentOnlyDeliverableCoverageSwitchesToEvidenceRepair(t *testing.T) {
	runID := common.RunID("run-deliverable-coverage-evidence-repair")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-deliverable-coverage-evidence-repair",
		DoDID:           "dod",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	step := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001-of-assessment"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyAssessmentOnly:        true,
			"repairObjective":                 "complete_dod_goal_coverage_assessment",
			"assessmentOnlyReason":            "dod_coverage_contract_correction",
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|stage=llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   "deliverable coverage still missing",
				"deliverableCoverage": map[string]any{
					"satisfied": false,
					"items": []map[string]any{{
						"id":     "user_deliverable",
						"state":  "failed",
						"reason": "missing final deliverable evidence",
					}},
				},
			},
			"runtimeDiagnostics": map[string]any{
				"kind": failureClassDoDCoverageMissing,
				"missingDeliverableCoverage": []map[string]any{{
					"id":    "user_deliverable",
					"state": "failed",
				}},
			},
		},
	}

	input := (&RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}).runtimeRepairFollowUpStepInput(context.Background(), run, step, 2, errors.New("missing deliverable coverage"))

	if input[stepInputKeyAssessmentOnly] != nil || input["repairObjective"] != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("repair input = %+v, want concrete evidence repair after failed deliverable assessment correction", input)
	}
	gap := mapFromValue(input["verificationEvidenceGap"])
	if missing := mapSliceFromValue(gap["missingDeliverable"]); len(missing) != 1 || missing[0]["id"] != "user_deliverable" {
		t.Fatalf("verificationEvidenceGap = %+v, want missing deliverable coverage carried forward", gap)
	}
}

func TestRuntimeRepairInputForMissingGoalCoverageWithoutEvidenceCandidatesProducesEvidenceRepair(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-dod-coverage-evidence-repair")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-dod-coverage-evidence-repair", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}
	step := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001-of-planning-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyAssessmentOnly:        true,
			"repairObjective":                 "complete_dod_goal_coverage_assessment",
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|stage=llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"summary":   "DoD passed but goal coverage is still incomplete.",
				"items": []map[string]any{{
					"item":         "tests pass",
					"state":        "satisfied",
					"evidenceRefs": []string{string(runID) + "/graph-001/003-test"},
				}},
			},
			"runtimeDiagnostics": map[string]any{
				"kind": failureClassDoDCoverageMissing,
				"missingGoalCoverage": []map[string]any{{
					"goalRequirementId": "goal-req-001-no-candidate",
					"raw":               "new goal requirement with no current evidence",
					"kind":              "leaf",
					"state":             "missing",
				}},
			},
		},
	}
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, step, 2, errors.New("missing goal coverage"))

	if input[stepInputKeyAssessmentOnly] != nil || input["repairObjective"] != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("repair input = %+v, want concrete evidence repair without assessment-only", input)
	}
	if input["evidenceRepairRequired"] != true {
		t.Fatalf("evidenceRepairRequired = %v, want true", input["evidenceRepairRequired"])
	}
	gap := mapFromValue(input["verificationEvidenceGap"])
	if missing := mapSliceFromValue(gap["missingGoal"]); len(missing) != 1 || missing[0]["goalRequirementId"] != "goal-req-001-no-candidate" {
		t.Fatalf("verificationEvidenceGap = %+v, want missing Goal coverage carried forward", gap)
	}
	if got := input["evidenceRepairSource"]; got != "coverage_gap_without_candidate_evidence" {
		t.Fatalf("evidenceRepairSource = %v, want coverage_gap_without_candidate_evidence", got)
	}
}

func TestRuntimeRepairEvidenceCandidatesRequireCoverageBoundAction(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-coverage-bound-evidence-candidates")
	run := &runmodel.Run{ID: runID, WorkspaceID: "ws-coverage-bound-evidence-candidates", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}
	stepRepo := memory.NewStepRepository()
	unboundStepID := common.StepID(runID + "/graph-001/001-unbound-action")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:     unboundStepID,
		RunID:  runID,
		Kind:   shellcommon.StepKindCommand,
		State:  common.StepStateCompleted,
		Output: map[string]any{"stdout": "latest action completed"},
	}); err != nil {
		t.Fatalf("seed unbound step: %v", err)
	}
	orch := &RuntimeOrchestrator{
		steps:      stepRepo,
		workspaces: memory.NewWorkspaceRepository(),
	}
	missing := missingDoDItemsWithEvidenceCandidates(ctx, orch.steps, run, nil, []map[string]any{{
		"item":  "target alpha requirement",
		"state": "failed",
	}})
	if len(missing) != 1 {
		t.Fatalf("missing = %+v, want one item", missing)
	}
	if candidates := mapSliceFromValue(missing[0]["evidenceCandidates"]); len(candidates) != 0 {
		t.Fatalf("evidenceCandidates = %+v, want unbound latest action filtered", candidates)
	}

	boundStepID := common.StepID(runID + "/graph-002/001-bound-action")
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    boundStepID,
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateCompleted,
		Output: map[string]any{
			"stdout":          "targeted action completed",
			"coverageTargets": []string{"target alpha requirement"},
		},
	}); err != nil {
		t.Fatalf("seed bound step: %v", err)
	}
	missing = missingDoDItemsWithEvidenceCandidates(ctx, orch.steps, run, nil, []map[string]any{{
		"item":  "target alpha requirement",
		"state": "failed",
	}})
	candidates := mapSliceFromValue(missing[0]["evidenceCandidates"])
	if len(candidates) != 1 || stringFromMap(candidates[0], "stepId") != string(boundStepID) {
		t.Fatalf("evidenceCandidates = %+v, want coverage-bound latest action", candidates)
	}
	if got := stringFromMap(candidates[0], "evidenceQuality"); got != "direct" {
		t.Fatalf("evidenceQuality = %q, want direct", got)
	}
}

func TestCompletionGateCoverageRepairFollowUpCanContinue(t *testing.T) {
	runID := common.RunID("run-dod-coverage-assessment-repair-continues")
	step := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001-of-planning-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyAssessmentOnly:        true,
			"repairObjective":                 "complete_dod_goal_coverage_assessment",
			"missingGoalCoverage": []map[string]any{{
				"raw": "streaming token accounting",
				"evidenceCandidates": []map[string]any{{
					"stepId": string(runID) + "/graph-001/003-test",
					"kind":   "shell/command",
				}},
			}},
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|stage=llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"summary":   "DoD passed but goal coverage is still incomplete.",
				"items": []map[string]any{{
					"item":         "tests pass",
					"state":        "satisfied",
					"evidenceRefs": []string{string(runID) + "/graph-001/003-test"},
				}},
				"goalCoverage": map[string]any{
					"satisfied": true,
					"items": []map[string]any{{
						"item":         "covered goal",
						"state":        "satisfied",
						"evidenceRefs": []string{string(runID) + "/graph-001/003-test"},
					}},
				},
			},
			"runtimeDiagnostics": map[string]any{
				"kind": failureClassDoDCoverageMissing,
				"missingGoalCoverage": []map[string]any{{
					"raw":   "streaming token accounting",
					"kind":  "leaf",
					"state": "missing",
				}},
			},
		},
	}

	if shouldCloseCompletionGateFailureWithoutRepair("runtime completion gate failed", step, step.Output) {
		t.Fatalf("coverage repair follow-up should continue through runtime repair budget")
	}
	if input := (&RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}).runtimeRepairFollowUpStepInput(context.Background(), &runmodel.Run{ID: runID, WorkspaceID: "ws-dod-coverage-assessment-repair-continues", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}, step, 2, errors.New("missing goal coverage")); input[stepInputKeyAssessmentOnly] != true || input["repairObjective"] != "complete_dod_goal_coverage_assessment" {
		t.Fatalf("repair input = %+v, want continued assessment-only coverage correction", input)
	} else if missing := mapSliceFromValue(input["missingGoalCoverage"]); len(missing) != 1 || missing[0]["raw"] != "streaming token accounting" {
		t.Fatalf("missingGoalCoverage = %+v, want carried forward missing goal coverage", input["missingGoalCoverage"])
	}
}

func TestCompletionGateAssessmentOnlySchemaInvalidRepairCanContinue(t *testing.T) {
	runID := common.RunID("run-dod-coverage-assessment-schema-invalid")
	step := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-002-of-planning-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyAssessmentOnly:        true,
			stepInputKeyRuntimeStage:          runtimeStageVerification,
			"repairObjective":                 "complete_dod_goal_coverage_assessment",
			"assessmentOnlyReason":            "dod_coverage_contract_correction",
			"convergenceContract": map[string]any{
				"state":           convergenceStateNeedAssessment,
				"runtimeStage":    runtimeStageVerification,
				"repairObjective": "complete_dod_goal_coverage_assessment",
			},
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": "goal-req-usage",
				"raw":               "usage accounting",
				"evidenceCandidates": []map[string]any{{
					"stepId": string(runID) + "/graph-001/003-test",
					"kind":   "shell/command",
				}},
			}},
		},
		Output: map[string]any{
			"stage":                   "llm_schema_invalid",
			failureOutputKeyClass:     failureClassLLMDecisionInvalid,
			failureOutputKeySignature: "class=llm_decision_invalid|kind=llm/workspace_execution|stage=llm_schema_invalid",
			"error":                   "invalid argument: invalid llm decision json: no json object found",
			"reason":                  "invalid argument: invalid llm decision json: no json object found",
		},
	}

	if shouldCloseCompletionGateFailureWithoutRepair("runtime completion gate failed", step, step.Output) {
		t.Fatalf("assessment-only schema invalid repair should continue through runtime repair budget")
	}
	if terminalAssessmentOnlyDecisionContractFailure(step) {
		t.Fatalf("assessment-only schema invalid repair should not be treated as terminal decision contract failure")
	}
	input := (&RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}).runtimeRepairFollowUpStepInput(context.Background(), &runmodel.Run{ID: runID, WorkspaceID: "ws-dod-coverage-assessment-schema-invalid", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}, step, 2, errors.New("invalid llm decision json"))

	if input[stepInputKeyAssessmentOnly] != true ||
		input[stepInputKeyRuntimeStage] != runtimeStageVerification ||
		input["repairObjective"] != "complete_dod_goal_coverage_assessment" ||
		input["schemaInvalidRepairMode"] != "single_valid_json_decision" {
		t.Fatalf("repair input = %+v, want assessment-only schema invalid recovery contract", input)
	}
	if input["stateChangingOutputRequired"] != nil || input["boundedValidationRequired"] != nil {
		t.Fatalf("repair input = %+v, want no executable or validation contract", input)
	}
	if missing := mapSliceFromValue(input["missingGoalCoverage"]); len(missing) != 1 || missing[0]["goalRequirementId"] != "goal-req-usage" {
		t.Fatalf("missingGoalCoverage = %+v, want carried forward assessment scope", input["missingGoalCoverage"])
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedAssessment {
		t.Fatalf("convergenceContract = %+v, want need_assessment", contract)
	}
	recovery := mapFromValue(input["assessmentSchemaInvalidRecovery"])
	if recovery["required"] != true {
		t.Fatalf("assessmentSchemaInvalidRecovery = %+v, want required", recovery)
	}
}

func TestAssessmentSchemaInvalidAfterValidationDriftPreservesValidationEvidenceContract(t *testing.T) {
	runID := common.RunID("run-assessment-schema-invalid-validation-drift")
	failed := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-003-of-assessment"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageVerification,
			"repairObjective":                 repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired":       true,
			"evidenceRepairRequired":          true,
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": "goal-req-001",
				"raw":               "publish verifiable completion evidence",
			}},
			"missingDeliverableCoverage": []map[string]any{{
				"id":    "validation_report",
				"state": "missing",
			}},
			"coverageTargetSet": []map[string]any{{
				"kind":       "goal",
				"targetText": "goal-req-001",
			}},
		},
		Output: map[string]any{
			"stage":                   "llm_schema_invalid",
			failureOutputKeyClass:     failureClassLLMDecisionInvalid,
			failureOutputKeySignature: "class=llm_decision_invalid|kind=llm/workspace_execution|stage=llm_schema_invalid|error=invalid argument: invalid llm decision json: no json object found",
			"error":                   "invalid argument: invalid llm decision json: no json object found",
			"rawContentPreview":       "{\"completionState\":\"completed\",\"dodAssessment\":{\"satisfied\":true,\"goalCoverage\":{\"items\":[]},\"deliverableCoverage\":{\"items\":[]}",
		},
	}

	input := (&RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}).runtimeRepairFollowUpStepInput(context.Background(), &runmodel.Run{ID: runID, WorkspaceID: "ws-assessment-schema-invalid-validation-drift", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}, failed, 3, errors.New("invalid llm decision json"))

	if input[stepInputKeyAssessmentOnly] != nil ||
		input[stepInputKeyRuntimeStage] != runtimeStageVerification ||
		input["repairObjective"] != repairObjectiveConcreteValidationEvidence ||
		input["boundedValidationRequired"] != true ||
		input["evidenceRepairRequired"] != true ||
		input["schemaInvalidRepairMode"] != "single_valid_json_decision" {
		t.Fatalf("repair input = %+v, want concrete validation schema recovery after validation drift", input)
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedValidation {
		t.Fatalf("convergenceContract = %+v, want need_validation", contract)
	}
}

func TestConcreteValidationSchemaInvalidPreservesValidationEvidenceContract(t *testing.T) {
	runID := common.RunID("run-concrete-validation-schema-invalid")
	failed := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001-of-planning"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageVerification,
			"repairObjective":                 repairObjectiveConcreteValidationEvidence,
			"boundedValidationRequired":       true,
			"evidenceRepairRequired":          true,
			"coverageTargetSet": []map[string]any{{
				"kind":       "deliverable",
				"targetText": "version_control_snapshot",
				"state":      "failed",
			}},
			"repairActionPackage": map[string]any{
				"requiredOutput": concreteValidationEvidenceRequiredOutput(nil),
			},
		},
		Output: map[string]any{
			"stage":                   "llm_schema_invalid",
			failureOutputKeyClass:     failureClassLLMDecisionInvalid,
			failureOutputKeySignature: "class=llm_decision_invalid|kind=llm/workspace_execution|stage=llm_schema_invalid|error=invalid argument: invalid llm decision json: no json object found",
			"error":                   "invalid argument: invalid llm decision json: no json object found",
			"rawContentPreview":       "{\"completionState\":\"completed\",\"dodAssessment\":{\"satisfied\":true,\"deliverableCoverage\":{\"items\":[]}",
		},
	}

	input := (&RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}).runtimeRepairFollowUpStepInput(context.Background(), &runmodel.Run{ID: runID, WorkspaceID: "ws-concrete-validation-schema-invalid", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}, failed, 2, errors.New("invalid llm decision json"))

	if input[stepInputKeyAssessmentOnly] != nil ||
		input[stepInputKeyRuntimeStage] != runtimeStageVerification ||
		input["repairObjective"] != repairObjectiveConcreteValidationEvidence ||
		input["boundedValidationRequired"] != true ||
		input["evidenceRepairRequired"] != true {
		t.Fatalf("repair input = %+v, want concrete validation schema recovery contract preserved", input)
	}
	if input["assessmentSchemaInvalidRecovery"] != nil || input["assessmentOnlyReason"] != nil {
		t.Fatalf("repair input = %+v, want no assessment-only schema recovery fields", input)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if !boolValue(required["mustProduceConcreteValidationEvidence"]) ||
		!boolValue(required["mustReturnValidJSON"]) ||
		!boolValue(required["mustNotReuseRejectedJSON"]) {
		t.Fatalf("requiredOutput = %+v, want valid-json concrete validation contract", required)
	}
	guidance := strings.Join(stringListFromValue(input["repairGuidance"]), "\n")
	if strings.Contains(guidance, "Return one valid JSON object containing a completed or failed dodAssessment") ||
		strings.Contains(guidance, "do not return executable work") {
		t.Fatalf("repairGuidance = %s, want executable validation contract not assessment-only", guidance)
	}
}

func TestAssessmentSchemaInvalidBeforeBudgetDoesNotEscalateDuplicateDecision(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-assessment-schema-invalid-duplicate")
	stepRepo := memory.NewStepRepository()
	output := map[string]any{
		"stage":                   "llm_schema_invalid",
		failureOutputKeyClass:     failureClassLLMDecisionInvalid,
		failureOutputKeySignature: "class=llm_decision_invalid|kind=llm/workspace_execution|stage=llm_schema_invalid|error=invalid argument: invalid llm decision json: no json object found",
		"error":                   "invalid argument: invalid llm decision json: no json object found",
		"rawContentPreview":       "{\"completionState\":\"completed\",\"dodAssessment\":{\"satisfied\":true,\"deliverableCoverage\":",
	}
	input := map[string]any{
		stepInputKeyRuntimeRepairFollowUp: true,
		stepInputKeyRuntimeStage:          runtimeStageVerification,
		"repairObjective":                 repairObjectiveConcreteValidationEvidence,
		"missingDeliverableCoverage": []map[string]any{{
			"id": "validation_report",
		}},
	}
	previous := &runmodel.Step{
		ID:     common.StepID(runID + "/repair-001-of-assessment"),
		RunID:  runID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateFailed,
		Input:  cloneStepPayload(input),
		Output: cloneStepPayload(output),
	}
	current := &runmodel.Step{
		ID:     common.StepID(runID + "/repair-002-of-assessment"),
		RunID:  runID,
		Kind:   llmWorkspaceExecutionStepKind,
		State:  common.StepStateFailed,
		Input:  cloneStepPayload(input),
		Output: cloneStepPayload(output),
	}
	if err := stepRepo.Create(ctx, previous); err != nil {
		t.Fatalf("seed previous step: %v", err)
	}
	if err := stepRepo.Create(ctx, current); err != nil {
		t.Fatalf("seed current step: %v", err)
	}
	orch := &RuntimeOrchestrator{
		runtime: &stubRuntimeAdapter{},
		steps:   stepRepo,
		tickets: memory.NewDecisionTicketRepository(),
	}

	if shouldBlockOnDuplicateRuntimeFailureForTest(ctx, orch, &runmodel.Run{ID: runID, WorkspaceID: "ws-assessment-schema-invalid-duplicate", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}, current) {
		t.Fatalf("assessment schema invalid should continue schema recovery before duplicate decision escalation")
	}
}

func TestRuntimeRepairFailedCompletionStateWithAssessmentUsesDoDGate(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	resultRepo := memory.NewResultRepository()
	workspaceID := common.WorkspaceID("ws-failed-completion-assessment-gate")
	runID := common.RunID("run-failed-completion-assessment-gate")
	var repairCalls int
	runtime := &stubRuntimeAdapter{fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
		if !boolValue(request.Input[stepInputKeyRuntimeRepairFollowUp]) {
			return nil, fmt.Errorf("unexpected non-repair request %s", request.StepID)
		}
		repairCalls++
		if repairCalls == 1 {
			return map[string]any{
				"status": "completed",
				stepOutputKeyLLMDecision: map[string]any{
					"completionState": "failed",
					stepOutputKeyDoDAssessment: map[string]any{
						"satisfied": true,
						"summary":   "Base DoD is satisfied, but deliverable coverage is omitted.",
						"items": []map[string]any{{
							"item":         "build passes",
							"state":        "satisfied",
							"evidenceRefs": []string{string(request.StepID)},
						}},
					},
				},
			}, nil
		}
		return map[string]any{
			"status": "completed",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"summary":   "DoD and deliverable coverage passed.",
				"items": []map[string]any{{
					"item":         "build passes",
					"state":        "satisfied",
					"evidenceRefs": []string{string(request.StepID)},
				}},
				"deliverableCoverage": map[string]any{
					"items": []map[string]any{{
						"id":           "single_binary",
						"state":        "satisfied",
						"evidenceRefs": []string{string(request.StepID)},
					}},
				},
			},
		}, nil
	}}
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
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-failed-completion-assessment-gate",
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"dod": map[string]any{
				"items": []string{"build passes"},
			},
			"executionContract": map[string]any{
				"profileContracts": map[string]any{
					"software-dev": map[string]any{
						"deliverableExpectations": []map[string]any{
							{"id": "single_binary", "required": true},
						},
					},
				},
			},
		},
	}
	if err := runRepo.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	err := orch.dispatchStepWithWorkspace(ctx, run, common.StepID(runID+"/repair-001-of-verify"), llmWorkspaceExecutionStepKind, map[string]any{
		stepInputKeyRuntimeRepairFollowUp: true,
		"repairObjective":                 "complete_dod_goal_coverage_assessment",
	}, nil)
	if err != nil {
		t.Fatalf("dispatch repair step: %v", err)
	}
	if repairCalls < 2 {
		t.Fatalf("repairCalls = %d, want follow-up repair after completion gate coverage failure", repairCalls)
	}
	closed, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if closed.State != common.RunStateCompleted {
		t.Fatalf("run = %+v, want completed after coverage repair follow-up", closed)
	}
	result, err := resultRepo.Get(ctx, common.ResultID(runID+"/result/dod-assessment"))
	if err != nil {
		t.Fatalf("get result: %v", err)
	}
	if result.State != common.ResultStateAccepted {
		t.Fatalf("result = %+v, want accepted result", result)
	}
}

func TestCompletionGateAssessmentOnlyFailedCoverageCorrectionSwitchesToConcreteEvidenceRepair(t *testing.T) {
	runID := common.RunID("run-dod-coverage-assessment-contradiction")
	step := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-001-of-planning-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyAssessmentOnly:        true,
			"repairObjective":                 "complete_dod_goal_coverage_assessment",
			"missingGoalCoverage": []map[string]any{{
				"goalRequirementId": "goal-req-001-abcdef12",
				"raw":               "streaming token accounting",
				"evidenceCandidates": []map[string]any{{
					"stepId":     string(runID) + "/graph-001/003-test",
					"kind":       "shell/command",
					"matchScore": 2,
				}},
			}},
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|stage=llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   "Goal coverage marked failed despite candidate evidence.",
				"items": []map[string]any{{
					"item":         "tests pass",
					"state":        "satisfied",
					"evidenceRefs": []string{string(runID) + "/graph-001/003-test"},
				}},
				"goalCoverage": map[string]any{
					"satisfied": false,
					"items": []map[string]any{{
						"goalRequirementId": "goal-req-001-abcdef12",
						"item":              "streaming token accounting",
						"state":             "failed",
						"reason":            "not enough evidence",
					}},
				},
			},
			"runtimeDiagnostics": map[string]any{
				"kind": failureClassDoDCoverageMissing,
				"missingGoalCoverage": []map[string]any{{
					"goalRequirementId": "goal-req-001-abcdef12",
					"raw":               "streaming token accounting",
					"kind":              "leaf",
					"state":             "missing",
				}},
			},
		},
	}

	if shouldCloseCompletionGateFailureWithoutRepair("runtime completion gate failed", step, step.Output) {
		t.Fatalf("assessment-only contradiction with evidence candidates should get another coverage correction attempt")
	}
	input := (&RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}).runtimeRepairFollowUpStepInput(context.Background(), &runmodel.Run{ID: runID, WorkspaceID: "ws-dod-coverage-assessment-contradiction", DoDID: "dod", InitialStepKind: llmWorkspaceExecutionStepKind}, step, 2, errors.New("missing goal coverage"))
	if input[stepInputKeyAssessmentOnly] != nil || input["repairObjective"] != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("repair input = %+v, want concrete evidence repair after failed assessment-only coverage correction", input)
	}
	if input["evidenceRepairRequired"] != true || input["evidenceRepairSource"] != "failed_assessment_only_coverage_gap" {
		t.Fatalf("repair input = %+v, want failed assessment-only evidence repair signal", input)
	}
	if missing := mapSliceFromValue(input["missingGoalCoverage"]); len(missing) != 1 || len(mapSliceFromValue(missing[0]["evidenceCandidates"])) == 0 {
		t.Fatalf("missingGoalCoverage = %+v, want inherited evidence candidates", input["missingGoalCoverage"])
	}
	if guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n"); !strings.Contains(guidance, "verification evidence") {
		t.Fatalf("repairGuidance = %+v, want concrete verification evidence guidance", input["repairGuidance"])
	}
}

func TestRuntimeRepairInputEscalatesRepeatedValidationAfterResourceSurfaceEvidence(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-repeated-validation-resource-surface")
	stepRepo := memory.NewStepRepository()
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID(runID + "/graph-001/001-list-workspace"),
		RunID: runID,
		Kind:  "file/list",
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyGraphRootStepID: string(runID) + "/step-1",
			stepInputKeyGraphNodeID:     "list-workspace",
		},
		Output: map[string]any{
			"status":    "completed",
			"stage":     "file_list",
			"path":      ".",
			"entries":   []map[string]any{},
			"truncated": false,
		},
	}); err != nil {
		t.Fatalf("seed file/list step: %v", err)
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-002-of-planning-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageVerification,
			stepInputKeyRepairOfRootStepID:    string(runID) + "/step-1",
			"repairObjective":                 repairObjectiveConcreteValidationEvidence,
			"evidenceRepairRequired":          true,
			stepInputKeyGraphEvidence: []map[string]any{{
				"stepId": string(runID) + "/graph-001/001-list-workspace",
				"kind":   "file/list",
				"output": map[string]any{
					"status":    "completed",
					"stage":     "file_list",
					"path":      ".",
					"entries":   []map[string]any{},
					"truncated": false,
				},
			}},
		},
		Output: map[string]any{
			"stage":                   "llm_dod_assessment",
			failureOutputKeyClass:     failureClassDoDCoverageMissing,
			failureOutputKeySignature: "class=dod_coverage_missing|kind=llm/workspace_execution|stage=llm_dod_assessment|error=invalid argument: concrete validation evidence repair must return bounded shell/command or mcp/tool_call executiongraph; plain failed dodassessment does not satisfy repairactionpackage.requiredoutput",
			"error":                   "invalid argument: concrete validation evidence repair must return bounded shell/command or mcp/tool_call executionGraph; plain failed dodAssessment does not satisfy repairActionPackage.requiredOutput",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   "coverage still missing after validation-only repair",
			},
			"runtimeDiagnostics": map[string]any{
				"kind": failureClassDoDCoverageMissing,
				"missingDoDItems": []map[string]any{{
					"item":  "deliver source artifact",
					"state": "failed",
				}},
			},
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("seed failed repair step: %v", err)
	}

	input := (&RuntimeOrchestrator{
		steps:      stepRepo,
		workspaces: memory.NewWorkspaceRepository(),
	}).runtimeRepairFollowUpStepInput(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-repeated-validation-resource-surface",
		DoDID:           "dod",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}, failedStep, 3, errors.New("plain failed dodAssessment does not satisfy repairActionPackage.requiredOutput"))

	if input["repairObjective"] != repairObjectiveStateChangingOutput || input["stateChangingOutputRequired"] != true {
		t.Fatalf("repair input = %+v, want repeated validation repair to escalate to state-changing output", input)
	}
	if input["validationRepairEscalatedToImplementation"] != true {
		t.Fatalf("validationRepairEscalatedToImplementation = %v, want true", input["validationRepairEscalatedToImplementation"])
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change", contract)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want state-changing patch/apply contract", required)
	}
}

func TestCompletionGateEvidenceQualityGapRepairFollowUpContinuesToConcreteEvidenceRepair(t *testing.T) {
	runID := common.RunID("run-dod-evidence-quality-repair-follow-up")
	step := &runmodel.Step{
		ID:    common.StepID(runID + "/repair-002-of-graph-004-001-verify"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			"repairObjective":                 repairObjectiveConcreteValidationEvidence,
			"evidenceRepairRequired":          true,
		},
		Output: map[string]any{
			"stage": "llm_dod_assessment",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": true,
				"summary":   "All items are marked satisfied but cite generic evidence.",
				"items": []map[string]any{{
					"item":         "request forwarding returns the expected response",
					"state":        "satisfied",
					"evidenceRefs": []string{string(runID) + "/graph-005/003-verify-tests"},
				}},
			},
			"runtimeDiagnostics": map[string]any{
				"kind":                       "dod_evidence_quality_gap",
				"scope":                      "dod_item",
				"claimAlignedEvidenceNeeded": true,
				"missingDoDItems": []map[string]any{{
					"kind":         "leaf",
					"item":         "request forwarding returns the expected response",
					"state":        "failed",
					"source":       "claimEvidenceAlignment",
					"evidenceRefs": []string{string(runID) + "/graph-005/003-verify-tests"},
					"reason":       "requires evidence text or coverageTargets aligned with the satisfied claim",
				}},
			},
		},
	}

	if shouldCloseCompletionGateFailureWithoutRepair("runtime completion gate failed", step, step.Output) {
		t.Fatalf("evidence-quality repair follow-up should continue through bounded runtime repair")
	}
	input := (&RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}).runtimeRepairFollowUpStepInput(context.Background(), &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-dod-evidence-quality-repair-follow-up",
		DoDID:           "dod",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}, step, 3, errors.New("claim-aligned evidence missing"))
	if input["repairObjective"] != repairObjectiveConcreteValidationEvidence || input["evidenceRepairRequired"] != true {
		t.Fatalf("repair input = %+v, want concrete validation evidence repair", input)
	}
	if input[stepInputKeyAssessmentOnly] != nil {
		t.Fatalf("repair input = %+v, want executable evidence repair, not assessment-only", input)
	}
	missing := mapSliceFromValue(input["missingDoDItems"])
	if len(missing) != 1 || missing[0]["source"] != "claimEvidenceAlignment" {
		t.Fatalf("missingDoDItems = %+v, want claim-alignment evidence gap", missing)
	}
	if gap := mapFromValue(input["verificationEvidenceGap"]); !boolValue(gap["claimAlignedEvidenceNeeded"]) {
		t.Fatalf("verificationEvidenceGap = %+v, want claim-aligned evidence signal", gap)
	}
}

func TestCompletionGateConvertedDecisionFollowUpDoesNotCloseRun(t *testing.T) {
	step := &runmodel.Step{
		ID:    "run-converted-decision/repair-003",
		RunID: "run-converted-decision",
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRuntimeStage:          runtimeStageVerification,
			stepInputKeyRuntimeStageMode:      "strict_convergence",
			"repairObjective":                 repairObjectiveConcreteValidationEvidence,
		},
		Output: map[string]any{
			"stage":                                "llm_decision_required",
			"convertedDecisionGate":                "convergence_contract",
			"continuableDecisionConvertedToRepair": true,
			failureOutputKeyClass:                  failureClassDoDCoverageMissing,
			failureOutputKeySignature:              "class=dod_coverage_missing|stage=llm_decision_required",
			stepOutputKeyDoDAssessment: map[string]any{
				"satisfied": false,
				"summary":   "decision converted to repair contract violation",
				"items": []map[string]any{{
					"item":  "bounded evidence",
					"state": "failed",
				}},
			},
		},
	}

	if shouldCloseCompletionGateFailureWithoutRepair("runtime completion gate failed", step, step.Output) {
		t.Fatalf("converted decision repair follow-up should continue through runtime repair")
	}
}

func TestRuntimeOrchestrator_StateChangingProviderRetryViolationContinuesRecovery(t *testing.T) {
	ctx := context.Background()
	workspaceRepo := memory.NewWorkspaceRepository()
	runRepo := memory.NewRunRepository()
	stepRepo := memory.NewStepRepository()
	ticketRepo := memory.NewDecisionTicketRepository()
	approvalRepo := memory.NewApprovalRepository()
	resultRepo := memory.NewResultRepository()
	validationRepo := memory.NewValidationResultRepository()
	checkpointRepo := memory.NewRuntimeCheckpointRepository()
	runtime := &stubRuntimeAdapter{
		fnRequest: func(request runtimemodel.StepExecutionRequest) (map[string]any, error) {
			stepID := string(request.StepID)
			if strings.Contains(stepID, "/repair-001-of-planning-001-2") {
				return map[string]any{
					"status": "completed",
					stepOutputKeyDoDAssessment: map[string]any{
						"satisfied": false,
						"summary":   "state-changing repair could not produce a valid patch",
						"items": []map[string]any{{
							"item":         "runtime contract repair",
							"state":        "failed",
							"reason":       "LLM kept returning read-only executionGraph",
							"evidenceRefs": []string{stepID},
						}},
					},
				}, nil
			}
			return nil, fmt.Errorf("unexpected step %s", stepID)
		},
	}
	orch := NewRuntimeOrchestratorWithResolutionsAndEvidence(
		runtime,
		runRepo,
		stepRepo,
		workspaceRepo,
		ticketRepo,
		nil,
		approvalRepo,
		RuntimeEvidenceRepositories{
			Results:            resultRepo,
			ValidationResults:  validationRepo,
			RuntimeCheckpoints: checkpointRepo,
		},
		events.NewMemoryHub(),
	)

	workspaceID := common.WorkspaceID("ws-state-changing-retry-closure")
	runID := common.RunID("run-state-changing-retry-closure")
	if err := workspaceRepo.Create(ctx, &workspacemodel.Workspace{ID: workspaceID, Title: "ws", State: common.WorkspaceStateRunning, ActiveRunID: runID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := runRepo.Create(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-state-changing-retry-closure",
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID("run-state-changing-retry-closure/provider-retry-002-of-repair-001-of-planning-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyProviderRetryFollowUp: true,
			stepInputKeyProviderRetryAttempt:  2,
			stepInputKeyProviderRetryRootStep: "run-state-changing-retry-closure/repair-001-of-planning-001",
			stepInputKeyRepairOfStepID:        "run-state-changing-retry-closure/provider-retry-001-of-planning-001",
			stepInputKeyRepairOfRootStepID:    "run-state-changing-retry-closure/step-1",
			"stateChangingOutputRequired":     true,
			"repairObjective":                 "produce_state_changing_output",
		},
		Output: map[string]any{
			"status":                  "completed",
			"stage":                   "llm_execution_graph_plan",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=llm/workspace_execution|stage=llm_execution_graph_plan|error=invalid argument: state-changing llm repair step returned read-only executiongraph without patch/apply",
			"error":                   "invalid argument: state-changing llm repair step returned read-only executionGraph without patch/apply",
			stepOutputKeyExecutionGraph: map[string]any{"nodes": []any{
				map[string]any{"id": "inspect", "kind": "file/list"},
			}},
		},
	}
	if err := stepRepo.Create(ctx, failedStep); err != nil {
		t.Fatalf("seed failed step: %v", err)
	}
	if err := stepRepo.Create(ctx, &runmodel.Step{
		ID:    common.StepID("run-state-changing-retry-closure/repair-001-of-planning-001"),
		RunID: runID,
		Kind:  llmWorkspaceExecutionStepKind,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeRepairFollowUp: true,
			stepInputKeyRepairOfStepID:        "run-state-changing-retry-closure/provider-retry-001-of-planning-001",
			stepInputKeyRepairOfRootStepID:    "run-state-changing-retry-closure/step-1",
		},
		Output: map[string]any{
			"status": "failed",
			"stage":  "llm_provider_timeout",
			"error":  "context deadline exceeded",
		},
	}); err != nil {
		t.Fatalf("seed existing repair step: %v", err)
	}

	if err := orch.dispatchRuntimeRepairFollowUp(ctx, &runmodel.Run{
		ID:              runID,
		WorkspaceID:     workspaceID,
		DoDID:           "dod-state-changing-retry-closure",
		State:           common.RunStateRunning,
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}, failedStep, errors.New("invalid argument: state-changing llm repair step returned read-only executionGraph without patch/apply")); err != nil {
		t.Fatalf("dispatchRuntimeRepairFollowUp: %v", err)
	}

	sawFollowUpRepair := false
	for _, call := range runtime.calls {
		if call == common.StepID("run-state-changing-retry-closure/repair-001-of-planning-001-2") {
			sawFollowUpRepair = true
			break
		}
	}
	if !sawFollowUpRepair {
		t.Fatalf("runtime calls = %+v, want follow-up repair after provider retry contract violation", runtime.calls)
	}
	runItem, err := runRepo.Get(ctx, runID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if runItem.State == common.RunStateRunning {
		t.Fatalf("run state = running, want recovery closure to terminal/blocking state")
	}
	results, err := resultRepo.ListByRun(ctx, runID)
	if err != nil {
		t.Fatalf("list results: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("results empty, want rejected result from failed repair assessment")
	}
}

func TestRuntimeRepairInputRequiresStateChangingForMissingWorkspaceFile(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-missing-file-state-changing-repair")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-missing-file-state-changing-repair",
		DoDID:           "dod-missing-file-state-changing-repair",
		InitialStepKind: llmWorkspaceExecutionStepKind,
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-001/002-inspect-main"),
		RunID: runID,
		Kind:  "file/read",
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyGraphNodeID:     "inspect_main",
			stepInputKeyGraphRootStepID: string(runID) + "/step-1",
		},
		Output: map[string]any{
			"status":                  "failed",
			"stage":                   "file_read_invalid_path",
			"error":                   "lstat /workspace/main.go: no such file or directory",
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime_failed|kind=file/read|stage=file_read_invalid_path|node=inspect_main|error=lstat /workspace/main.go: no such file or directory",
		},
	}
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 1, errors.New("lstat /workspace/main.go: no such file or directory"))
	if input["stateChangingOutputRequired"] != true ||
		input["repairObjective"] != "produce_state_changing_output" {
		t.Fatalf("repair input = %+v, want state-changing repair for missing workspace file", input)
	}
	guidance := strings.Join(stringSliceFromValue(input["repairGuidance"]), "\n")
	if !strings.Contains(guidance, "create it with patch/apply file_write") {
		t.Fatalf("repair guidance = %q, want missing file creation guidance", guidance)
	}
}

func TestRuntimeRepairInputUsesEvidenceAcquisitionForExecutionShellFailureWithoutConcreteTarget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-shell-failure-evidence-acquisition")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-shell-failure-evidence-acquisition",
		DoDID:           "dod-shell-failure-evidence-acquisition",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-003/003-run-go-test"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:       runtimeStageExecution,
			stepInputKeyGraphNodeID:        "run_go_test",
			stepInputKeyGraphRootStepID:    string(runID) + "/step-1",
			stepInputKeyRepairOfRootStepID: string(runID) + "/step-1",
		},
		Output: map[string]any{
			"status":                  "failed",
			"error":                   "exit status 1",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=run_go_test|exitCode=1",
		},
	}
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 1, errors.New("exit status 1"))
	if input["stateChangingOutputRequired"] != nil ||
		input["repairObjective"] != repairObjectiveAcquireFailureEvidence ||
		input["concreteFailureEvidenceRequired"] != true {
		t.Fatalf("repair input = %+v, want evidence acquisition without concrete failure target", input)
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedEvidence {
		t.Fatalf("convergenceContract = %+v, want need_evidence for shell failure without concrete target", contract)
	}
	if input[stepInputKeyRepairOfStepID] != string(failedStep.ID) ||
		input["failedGraphNodeId"] != "run_go_test" {
		t.Fatalf("repair input = %+v, want failed graph node lineage", input)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != false || required["mustProduceConcreteFailureEvidence"] != true {
		t.Fatalf("requiredOutput = %+v, want concrete evidence acquisition contract", required)
	}
}

func TestRuntimeRepairInputRequiresStateChangingForExecutionShellFailureWithConcreteTarget(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-shell-failure-state-changing-repair")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-shell-failure-state-changing-repair",
		DoDID:           "dod-shell-failure-state-changing-repair",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-003/003-run-go-test"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:       runtimeStageExecution,
			stepInputKeyGraphNodeID:        "run_go_test",
			stepInputKeyGraphRootStepID:    string(runID) + "/step-1",
			stepInputKeyRepairOfRootStepID: string(runID) + "/step-1",
		},
		Output: map[string]any{
			"status":                  "failed",
			"stderr":                  "./main_test.go:91:2: cannot assign to struct field config.Providers[\"openai\"].BaseURL in map",
			"error":                   "exit status 1",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|failureTargets=main_test.go|node=run_go_test|exitCode=1",
		},
	}
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 1, errors.New("exit status 1"))
	if input["stateChangingOutputRequired"] != true ||
		input["repairObjective"] != repairObjectiveStateChangingOutput {
		t.Fatalf("repair input = %+v, want concrete target shell failure to require state-changing repair", input)
	}
	contract := mapFromValue(input["convergenceContract"])
	if stringFromMap(contract, "state") != convergenceStateNeedStateChange {
		t.Fatalf("convergenceContract = %+v, want need_state_change for concrete target shell failure", contract)
	}
	required := mapFromValue(mapFromValue(input["repairActionPackage"])["requiredOutput"])
	if required["mustIncludeStateChangingNode"] != true ||
		!stringSliceContains(stringSliceFromValue(required["acceptedKinds"]), "patch/apply") {
		t.Fatalf("requiredOutput = %+v, want patch/apply state-changing contract", required)
	}
}

func TestRuntimeRepairInputAllowsBoundedRetryForExecutionTimeout(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-shell-timeout-retry")
	run := &runmodel.Run{
		ID:              runID,
		WorkspaceID:     "ws-shell-timeout-retry",
		DoDID:           "dod-shell-timeout-retry",
		InitialStepKind: llmWorkspaceExecutionStepKind,
		ExecutionContext: map[string]any{
			"executionContract": map[string]any{"stateChangePolicy": "must_produce_state_change"},
		},
	}
	failedStep := &runmodel.Step{
		ID:    common.StepID(string(runID) + "/graph-003/003-verify"),
		RunID: runID,
		Kind:  shellcommon.StepKindCommand,
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyRuntimeStage:    runtimeStageExecution,
			stepInputKeyGraphNodeID:     "verify",
			stepInputKeyGraphRootStepID: string(runID) + "/step-1",
		},
		Output: map[string]any{
			"status":                  "timeout",
			"error":                   "context deadline exceeded",
			failureOutputKeyClass:     failureClassShellCommand,
			failureOutputKeySignature: "class=shell_command_failed|kind=shell/command|node=verify|exitCode=-1|error=context deadline exceeded",
		},
	}
	orch := &RuntimeOrchestrator{
		steps:      memory.NewStepRepository(),
		workspaces: memory.NewWorkspaceRepository(),
	}

	input := orch.runtimeRepairFollowUpStepInput(ctx, run, failedStep, 1, errors.New("context deadline exceeded"))
	if input["stateChangingOutputRequired"] != nil {
		t.Fatalf("repair input = %+v, want timeout repair not to require state-changing output", input)
	}
	if input["repairObjective"] != repairObjectiveTransientExecutionRetry || input["executionTimeoutRetryAllowed"] != true {
		t.Fatalf("repair input = %+v, want transient execution retry objective", input)
	}
}

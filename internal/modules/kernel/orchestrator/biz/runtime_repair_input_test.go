package orchestrator

import (
	"errors"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeRepairBaseInputProjectsFailedStepContract(t *testing.T) {
	run := &runmodel.Run{ID: "run-repair-base", WorkspaceID: "ws-repair-base"}
	failed := &runmodel.Step{
		ID:    "run-repair-base/graph-001-build",
		RunID: run.ID,
		Kind:  "tool_call",
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyExecutionContextPackage: map[string]any{"large": "omitted"},
			"target":                            "src/app.go",
		},
		Output: map[string]any{
			"stderr": "compile failed",
		},
	}

	input := runtimeRepairBaseInput(
		runtimeRepairBaseInputDeps{policy: RuntimePolicy{MaxRuntimeRepairAttempts: 7}},
		run,
		failed,
		2,
		errors.New("exit status 1"),
	)

	if input[stepInputKeyRuntimeRepairFollowUp] != true {
		t.Fatalf("runtimeRepairFollowUp = %v, want true", input[stepInputKeyRuntimeRepairFollowUp])
	}
	if input[stepInputKeyRuntimeRepairAttempt] != 2 {
		t.Fatalf("runtimeRepairAttempt = %v, want 2", input[stepInputKeyRuntimeRepairAttempt])
	}
	if input["maxRuntimeRepairAttempts"] != 7 {
		t.Fatalf("maxRuntimeRepairAttempts = %v, want 7", input["maxRuntimeRepairAttempts"])
	}
	if input["failureReason"] != "exit status 1" {
		t.Fatalf("failureReason = %v, want exit status 1", input["failureReason"])
	}
	if input["failedStepId"] != string(failed.ID) || input[stepInputKeyRepairOfStepID] != string(failed.ID) {
		t.Fatalf("failed step lineage not projected: %v", input)
	}
	if input[stepInputKeyRepairOfRootStepID] != string(failed.ID) {
		t.Fatalf("repair root = %v, want failed step id", input[stepInputKeyRepairOfRootStepID])
	}
	if input[stepInputKeyRecoveryAction] != string(runtimeRecoveryActionRuntimeRepair) {
		t.Fatalf("recovery action = %v, want runtime repair", input[stepInputKeyRecoveryAction])
	}
	failedInput := mapFromValue(input["failedStepInput"])
	if failedInput["target"] != "src/app.go" {
		t.Fatalf("failedStepInput target = %v, want src/app.go", failedInput["target"])
	}
	if _, ok := failedInput[stepInputKeyExecutionContextPackage]; ok {
		t.Fatalf("failedStepInput retained execution context package: %v", failedInput)
	}
	failedOutput := mapFromValue(input["failedStepOutput"])
	if failedOutput["stderr"] != "compile failed" {
		t.Fatalf("failedStepOutput stderr = %v, want compile failed", failedOutput["stderr"])
	}
}

func TestRuntimeRepairBaseInputWithoutFailedStepUsesDefaults(t *testing.T) {
	input := runtimeRepairBaseInput(runtimeRepairBaseInputDeps{}, &runmodel.Run{ID: "run-repair-base-default"}, nil, 0, nil)

	if input[stepInputKeyRuntimeRepairFollowUp] != true {
		t.Fatalf("runtimeRepairFollowUp = %v, want true", input[stepInputKeyRuntimeRepairFollowUp])
	}
	if input["maxRuntimeRepairAttempts"] != defaultMaxRuntimeRepairAttempts {
		t.Fatalf("maxRuntimeRepairAttempts = %v, want default %d", input["maxRuntimeRepairAttempts"], defaultMaxRuntimeRepairAttempts)
	}
	if _, ok := input["failedStepId"]; ok {
		t.Fatalf("failedStepId should be absent without failed step: %v", input)
	}
}

func TestApplyRuntimeRepairFailureProjectionAddsDiagnosticsAndCoverageContext(t *testing.T) {
	coverageContext := coverageWorkItemContextFromItems("rev-1", []map[string]any{{
		"id":                    "cwi-001",
		"definitionRevision":    "rev-1",
		"source":                "goal",
		"sourceScope":           "goal_leaf",
		"sourceRequirementId":   "goal-req-001",
		"sourcePath":            "goal.leaves[0]",
		"targetText":            "must produce a bounded state change",
		"requiredEvidenceKinds": []string{"state_change", "validation"},
		"status":                "open",
	}}, nil, nil)
	input := map[string]any{}
	failed := &runmodel.Step{
		ID:    "run-repair-failure/step-1",
		RunID: "run-repair-failure",
		Kind:  "llm",
		State: common.StepStateFailed,
		Output: map[string]any{
			"runtimeDiagnostics": map[string]any{
				"kind":                     "dod_evidence_quality_gap",
				coverageWorkItemContextKey: coverageContext,
			},
		},
	}

	applyRuntimeRepairFailureProjection(input, failed)

	if stringFromMap(input, "repairObjective") != repairObjectiveConcreteValidationEvidence {
		t.Fatalf("repairObjective = %q, want concrete validation evidence", stringFromMap(input, "repairObjective"))
	}
	if input["evidenceRepairRequired"] != true {
		t.Fatalf("evidenceRepairRequired = %v, want true", input["evidenceRepairRequired"])
	}
	if len(mapFromValue(input["verificationEvidenceGap"])) == 0 {
		t.Fatalf("verificationEvidenceGap missing from input: %v", input)
	}
	if context := mapFromValue(input[coverageWorkItemContextKey]); len(mapSliceFromValue(context["unresolvedItems"])) != 1 {
		t.Fatalf("coverageWorkItemContext = %+v, want one unresolved item", input[coverageWorkItemContextKey])
	}
	unresolved := mapSliceFromValue(input["unresolvedCoverageWorkItems"])
	if len(unresolved) != 1 || stringFromMap(unresolved[0], "sourceRequirementId") != "goal-req-001" {
		t.Fatalf("unresolvedCoverageWorkItems = %+v, want goal-req-001", unresolved)
	}
	guidance := stringSliceFromValue(input["repairGuidance"])
	if len(guidance) == 0 {
		t.Fatalf("repairGuidance missing after projection")
	}
}

func TestApplyRuntimeRepairMissingCoverageProjectionPreservesExistingDeliverableCoverage(t *testing.T) {
	input := map[string]any{
		"missingDeliverableCoverage": []map[string]any{{
			"id": "already-present",
		}},
	}
	failed := &runmodel.Step{
		ID:    "run-repair-coverage/step-1",
		RunID: "run-repair-coverage",
		Kind:  "llm",
		State: common.StepStateFailed,
		Output: map[string]any{
			"runtimeDiagnostics": map[string]any{
				"missingDoDItems": []map[string]any{{
					"id":   "dod-001",
					"item": "must verify the bounded outcome",
				}},
				"missingDeliverableCoverage": []map[string]any{{
					"id": "from-failure",
				}},
			},
		},
	}

	applyRuntimeRepairMissingCoverageProjection(t.Context(), nil, input, &runmodel.Run{ID: "run-repair-coverage"}, failed, false)

	missingDoD := mapSliceFromValue(input["missingDoDItems"])
	if len(missingDoD) != 1 || stringFromMap(missingDoD[0], "id") != "dod-001" {
		t.Fatalf("missingDoDItems = %+v, want dod-001", missingDoD)
	}
	deliverable := mapSliceFromValue(input["missingDeliverableCoverage"])
	if len(deliverable) != 1 || stringFromMap(deliverable[0], "id") != "already-present" {
		t.Fatalf("missingDeliverableCoverage = %+v, want preserved existing item", deliverable)
	}
}

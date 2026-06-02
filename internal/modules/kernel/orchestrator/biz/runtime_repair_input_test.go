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

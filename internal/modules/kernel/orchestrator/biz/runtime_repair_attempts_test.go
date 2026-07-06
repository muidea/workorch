package orchestrator

import (
	"context"
	"fmt"
	"testing"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memory "github.com/muidea/workorch/internal/pkg/repository/memory"
)

func TestRuntimeOrchestrator_RuntimeRepairAttemptCountIncludesDispatchedProviderInfrastructureFailures(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-runtime-repair-budget-provider-failures")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}

	seedSteps := []*runmodel.Step{
		{
			ID:    common.StepID(runID + "/provider-misrouted-repair"),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateSucceeded,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				failureOutputKeyClass:             failureClassProviderTimeout,
			},
			Output: map[string]any{"status": "completed"},
		},
		{
			ID:    common.StepID(runID + "/repair-provider-timeout"),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateFailed,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				failureOutputKeyClass:             failureClassRuntime,
			},
			Output: map[string]any{
				"stage":               "llm_provider_timeout",
				failureOutputKeyClass: failureClassProviderTimeout,
			},
		},
		{
			ID:    common.StepID(runID + "/real-runtime-repair"),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateSucceeded,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				failureOutputKeyClass:             failureClassRuntime,
			},
			Output: map[string]any{
				"stage":               "llm_execution_graph_plan",
				failureOutputKeyClass: failureClassRuntime,
			},
		},
	}
	for _, step := range seedSteps {
		if err := stepRepo.Create(ctx, step); err != nil {
			t.Fatalf("seed step %s: %v", step.ID, err)
		}
	}

	if got := orch.repairManager().runtimeRepairAttemptCount(ctx, runID); got != len(seedSteps) {
		t.Fatalf("runtimeRepairAttemptCount = %d, want every dispatched runtime repair attempt counted", got)
	}
}

func TestRuntimeOrchestrator_RuntimeRepairAttemptCountIsScopedByFailureSignature(t *testing.T) {
	ctx := context.Background()
	runID := common.RunID("run-runtime-repair-budget-scoped-signature")
	stepRepo := memory.NewStepRepository()
	orch := &RuntimeOrchestrator{steps: stepRepo}
	for attempt := 1; attempt <= defaultMaxRuntimeRepairAttempts; attempt++ {
		if err := stepRepo.Create(ctx, &runmodel.Step{
			ID:    common.StepID(fmt.Sprintf("%s/repair-a-%d", runID, attempt)),
			RunID: runID,
			Kind:  llmcommon.StepKindWorkspaceExecution,
			State: common.StepStateSucceeded,
			Input: map[string]any{
				stepInputKeyRuntimeRepairFollowUp: true,
				stepInputKeyRuntimeRepairAttempt:  attempt,
				stepInputKeyRepairOfFailureClass:  failureClassRuntime,
				stepInputKeyRepairOfFailureSig:    "class=runtime|target=a",
			},
			Output: map[string]any{"status": "completed"},
		}); err != nil {
			t.Fatalf("seed scoped repair %d: %v", attempt, err)
		}
	}

	failedA := &runmodel.Step{
		ID:    common.StepID(runID + "/block/a"),
		RunID: runID,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime|target=a",
		},
	}
	failedB := &runmodel.Step{
		ID:    common.StepID(runID + "/block/b"),
		RunID: runID,
		State: common.StepStateFailed,
		Output: map[string]any{
			failureOutputKeyClass:     failureClassRuntime,
			failureOutputKeySignature: "class=runtime|target=b",
		},
	}

	if got := orch.repairManager().runtimeRepairAttemptCountForStep(ctx, runID, failedA); got != defaultMaxRuntimeRepairAttempts {
		t.Fatalf("attempt count for A = %d, want exhausted scope", got)
	}
	if got := orch.repairManager().runtimeRepairAttemptCountForStep(ctx, runID, failedB); got != 0 {
		t.Fatalf("attempt count for B = %d, want independent scope", got)
	}
}

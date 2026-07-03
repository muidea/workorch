package readmodelprojection

import (
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCurrentRunStatusStepTerminalRunPrefersFailedTerminalOverStaleCheckpoint(t *testing.T) {
	run := &runmodel.Run{ID: "run-terminal-selection", State: common.RunStateFailed}
	steps := []*runmodel.Step{
		{
			ID:    "run-terminal-selection/001-completed",
			RunID: run.ID,
			State: common.StepStateCompleted,
		},
		{
			ID:    "run-terminal-selection/002-failed",
			RunID: run.ID,
			State: common.StepStateFailed,
			Input: map[string]any{"previousStepId": "run-terminal-selection/001-completed"},
		},
	}
	checkpoints := []*runmodel.RuntimeCheckpoint{{
		ID:            "checkpoint-stale",
		RunID:         run.ID,
		CurrentStepID: "run-terminal-selection/001-completed",
	}}

	got := CurrentRunStatusStep(run, steps, checkpoints, nil, nil)
	if got == nil || got.ID != "run-terminal-selection/002-failed" {
		t.Fatalf("CurrentRunStatusStep = %+v, want latest failed terminal step", got)
	}
}

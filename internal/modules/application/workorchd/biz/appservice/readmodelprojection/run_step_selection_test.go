package readmodelprojection

import (
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCurrentRunStatusStepTerminalRunPrefersFailedTerminalOverStaleCheckpoint(t *testing.T) {
	run := &runmodel.Run{ID: "run-terminal-selection", State: common.RunStateFailed}
	steps := []*runmodel.Step{
		{
			ID:    "run-terminal-selection/001-completed",
			RunID: run.ID,
			State: common.StepStateSucceeded,
		},
		{
			ID:    "run-terminal-selection/002-failed",
			RunID: run.ID,
			State: common.StepStateFailed,
			Input: mustWorkspaceRunEventPayload(t, map[string]any{"previousStepId": "run-terminal-selection/001-completed"}),
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

func TestCurrentRunStatusStepTerminalRunUsesCheckpointOrderForLexicallyEarlierFailure(t *testing.T) {
	run := &runmodel.Run{ID: "run-terminal-checkpoint-order", State: common.RunStateFailed}
	steps := []*runmodel.Step{
		{
			ID:    "run-terminal-checkpoint-order/follow-up-final",
			RunID: run.ID,
			State: common.StepStateFailed,
			Input: mustWorkspaceRunEventPayload(t, map[string]any{"previousStepId": "run-terminal-checkpoint-order/graph-010"}),
		},
		{
			ID:    "run-terminal-checkpoint-order/graph-005-old",
			RunID: run.ID,
			State: common.StepStateFailed,
		},
		{
			ID:    "run-terminal-checkpoint-order/graph-010",
			RunID: run.ID,
			State: common.StepStateSucceeded,
		},
	}
	checkpoints := []*runmodel.RuntimeCheckpoint{
		{
			ID:            "checkpoint-old",
			RunID:         run.ID,
			CurrentStepID: "run-terminal-checkpoint-order/graph-005-old",
			OccurredAt:    "2026-07-03T08:40:00Z",
		},
		{
			ID:            "checkpoint-final-runtime-result",
			RunID:         run.ID,
			CurrentStepID: "run-terminal-checkpoint-order/follow-up-final",
			OccurredAt:    "2026-07-03T08:57:52Z",
		},
		{
			ID:            "checkpoint-stale-run-state",
			RunID:         run.ID,
			CurrentStepID: "run-terminal-checkpoint-order/graph-010",
			OccurredAt:    "2026-07-03T08:57:56Z",
		},
	}

	got := CurrentRunStatusStep(run, steps, checkpoints, nil, nil)
	if got == nil || got.ID != "run-terminal-checkpoint-order/follow-up-final" {
		t.Fatalf("CurrentRunStatusStep = %+v, want checkpoint-ordered final failure", got)
	}
}

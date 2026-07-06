package runtimecore

import (
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestMapRunProjectsLineageAndLifecycleTimestamps(t *testing.T) {
	item := &runmodel.Run{
		ID:          "run-successor",
		WorkspaceID: "ws-run-lineage",
		GoalID:      "goal-run-lineage",
		DoDID:       "dod-run-lineage",
		Lineage: runmodel.RunLineage{
			PredecessorRunID: "run-predecessor",
			SuccessorRunID:   "run-next",
			LineageState:     "active",
		},
		State:     common.RunStateActive,
		CreatedAt: "2026-07-06T03:00:00Z",
		StartedAt: "2026-07-06T03:01:00Z",
		EndedAt:   "2026-07-06T03:02:00Z",
	}

	got := MapRun(item)
	if got.Lineage.PredecessorRunID != "run-predecessor" ||
		got.Lineage.SuccessorRunID != "run-next" ||
		got.Lineage.LineageState != "active" ||
		got.CreatedAt != item.CreatedAt ||
		got.StartedAt != item.StartedAt ||
		got.EndedAt != item.EndedAt {
		t.Fatalf("mapped run = %+v, want lineage and timestamps projected", got)
	}
}

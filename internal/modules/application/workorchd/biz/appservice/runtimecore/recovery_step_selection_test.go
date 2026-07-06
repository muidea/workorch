package runtimecore

import (
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestTerminalStepModelTreatsGraphRootAsReferenced(t *testing.T) {
	runID := common.RunID("run-terminal-graph-root")
	root := &runmodel.Step{
		ID:    common.StepID(runID + "/contract-retry-001"),
		RunID: runID,
		State: common.StepStateSucceeded,
	}
	leaf := &runmodel.Step{
		ID:    common.StepID(runID + "/graph-006/001-fix-main"),
		RunID: runID,
		State: common.StepStateSucceeded,
		Input: map[string]any{
			"graphRootStepId": string(root.ID),
			"graphNodeId":     "fix-main",
		},
	}
	olderUnorderedRoot := &runmodel.Step{
		ID:    common.StepID(runID + "/step-1"),
		RunID: runID,
		State: common.StepStateSucceeded,
	}
	olderLeaf := &runmodel.Step{
		ID:    common.StepID(runID + "/graph-001/001-read"),
		RunID: runID,
		State: common.StepStateSucceeded,
		Input: map[string]any{
			"graphRootStepId": string(olderUnorderedRoot.ID),
			"graphNodeId":     "read",
		},
	}

	got := TerminalStepModel([]*runmodel.Step{root, leaf, olderLeaf, olderUnorderedRoot})

	if got == nil || got.ID != leaf.ID {
		t.Fatalf("terminal step = %+v, want latest graph leaf %s instead of referenced roots", got, leaf.ID)
	}
}

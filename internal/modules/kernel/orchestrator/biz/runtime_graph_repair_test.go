package orchestrator

import (
	"testing"

	runModel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestLatestFailedRepairableGraphStepSkipsSupersededFailure(t *testing.T) {
	runID := common.RunID("run-graph-superseded-failure")
	oldFailure := &runModel.Step{
		ID:    common.StepID(runID + "/graph-004/002-rerun-lint"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateFailed,
		Input: map[string]any{
			stepInputKeyGraphNodeID:     "rerun-lint",
			stepInputKeyGraphRootStepID: string(runID) + "/step-1",
			"targetRef":                 "web/src",
		},
	}
	laterSuccess := &runModel.Step{
		ID:    common.StepID(runID + "/graph-005/002-rerun-lint"),
		RunID: runID,
		Kind:  "shell/command",
		State: common.StepStateCompleted,
		Input: map[string]any{
			stepInputKeyGraphNodeID:     "rerun-lint",
			stepInputKeyGraphRootStepID: string(runID) + "/step-1",
			"targetRef":                 "web/src",
		},
	}
	if got := latestFailedRepairableGraphStep([]*runModel.Step{oldFailure, laterSuccess}); got != nil {
		t.Fatalf("latestFailedRepairableGraphStep = %+v, want nil because later success superseded the failure", got)
	}
}

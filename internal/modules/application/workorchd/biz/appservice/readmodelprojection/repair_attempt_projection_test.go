package readmodelprojection

import (
	"testing"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestRepairAttemptProjectionPreservesCountLowerBound(t *testing.T) {
	steps := []projectioncontract.StepView{
		{ID: "repair-a", Input: map[string]any{"runtimeRepairFollowUp": true, "runtimeRepairAttempt": 1}},
		{ID: "repair-b", Input: map[string]any{"runtimeRepairFollowUp": true, "runtimeRepairAttempt": 1}},
		{ID: "ordinary", Input: map[string]any{}},
	}
	count, attempt, stepID := repairAttemptProjection(steps)
	if count != 2 || attempt != 2 || stepID != "repair-b" {
		t.Fatalf("count=%d attempt=%d step=%s", count, attempt, stepID)
	}
	steps[0].Input["runtimeRepairAttempt"] = 5
	count, attempt, stepID = repairAttemptProjection(steps)
	if count != 2 || attempt != 5 || stepID != "repair-a" {
		t.Fatalf("显式较大 attempt 不得降低: count=%d attempt=%d step=%s", count, attempt, stepID)
	}
}

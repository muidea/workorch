package legacy

import (
	"context"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestMinimalRuntimeAdapter_ExecuteStepReturnsCompletedPayload(t *testing.T) {
	adapter := NewMinimalRuntimeAdapter()

	output, err := adapter.ExecuteStep(
		context.Background(),
		common.RunID("run-1"),
		common.StepID("run-1/step-1"),
	)
	if err != nil {
		t.Fatalf("execute step: %v", err)
	}
	if output["runId"] != "run-1" {
		t.Fatalf("runId = %v, want run-1", output["runId"])
	}
	if output["stepId"] != "run-1/step-1" {
		t.Fatalf("stepId = %v, want run-1/step-1", output["stepId"])
	}
	if output["status"] != "completed" {
		t.Fatalf("status = %v, want completed", output["status"])
	}
}

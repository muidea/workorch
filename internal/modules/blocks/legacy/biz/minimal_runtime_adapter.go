package legacy

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

type MinimalRuntimeAdapter struct{}

func NewMinimalRuntimeAdapter() *MinimalRuntimeAdapter { return &MinimalRuntimeAdapter{} }

func (a *MinimalRuntimeAdapter) ExecuteStep(ctx context.Context, runID common.RunID, stepID common.StepID) (map[string]any, error) {
	_ = ctx
	return map[string]any{
		"runId":  string(runID),
		"stepId": string(stepID),
		"status": "completed",
	}, nil
}

package orchestrator

import (
	"context"

	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
)

type PatchRuntimeAdapter struct{}

func NewPatchRuntimeAdapter() *PatchRuntimeAdapter {
	return &PatchRuntimeAdapter{}
}

func (a *PatchRuntimeAdapter) ExecuteRuntimeStep(_ context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	patch, err := runtimePatchRequestFromMap(request.Input)
	if err != nil {
		return runtimemodel.StepExecutionResult{
			Output: map[string]any{
				"status": "failed",
				"stage":  "patch_apply_invalid",
				"error":  err.Error(),
			},
		}, err
	}
	return runtimemodel.StepExecutionResult{
		Output: map[string]any{
			"status": "completed",
			"stage":  "patch_apply",
			"patches": []map[string]any{
				{
					"patchType": patch.PatchType,
					"targetRef": patch.TargetRef,
					"content":   patch.Content,
					"diff":      patch.Diff,
					"summary":   patch.Summary,
				},
			},
		},
	}, nil
}

var _ RuntimeAdapter = (*PatchRuntimeAdapter)(nil)

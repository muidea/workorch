package orchestrator

import (
	"context"
	"strings"

	runtimemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type PatchRuntimeAdapter struct{}

func NewPatchRuntimeAdapter() *PatchRuntimeAdapter {
	return &PatchRuntimeAdapter{}
}

func (a *PatchRuntimeAdapter) ExecuteRuntimeStep(_ context.Context, request runtimemodel.StepExecutionRequest) (runtimemodel.StepExecutionResult, error) {
	switch strings.TrimSpace(request.Kind) {
	case "patch/preview":
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
		preview, err := previewWorkspacePatch(request.WorkspaceDeliverableDirectory, patch)
		if err != nil {
			return runtimemodel.StepExecutionResult{
				Output: map[string]any{
					"status": "failed",
					"stage":  "patch_preview",
					"error":  err.Error(),
				},
			}, err
		}
		return runtimemodel.StepExecutionResult{
			Output: map[string]any{
				"status":       "completed",
				"stage":        "patch_preview",
				"patchPreview": preview,
			},
		}, nil
	case "patch/revert":
		patchID := strings.TrimSpace(stringFromAny(request.Input["patchId"]))
		if patchID == "" {
			return runtimemodel.StepExecutionResult{
				Output: map[string]any{
					"status": "failed",
					"stage":  "patch_revert_invalid",
					"error":  common.ErrInvalidArgument.Error(),
				},
			}, common.ErrInvalidArgument
		}
		return runtimemodel.StepExecutionResult{
			Output: map[string]any{
				"status": "completed",
				"stage":  "patch_revert",
				"patchRevert": map[string]any{
					"patchId":    patchID,
					"approvalId": strings.TrimSpace(stringFromAny(request.Input["approvalId"])),
					"strict":     boolFromAnyDefault(request.Input["strict"], true),
				},
			},
		}, nil
	case "", "patch/apply":
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
						"patchType":  patch.PatchType,
						"targetRef":  patch.TargetRef,
						"content":    patch.Content,
						"diff":       patch.Diff,
						"oldText":    patch.OldText,
						"newText":    patch.NewText,
						"summary":    patch.Summary,
						"baseHash":   patch.BaseHash,
						"occurrence": patch.Occurrence,
						"approvalId": string(patch.ApprovalID),
					},
				},
			},
		}, nil
	default:
		return runtimemodel.StepExecutionResult{
			Output: map[string]any{
				"status": "failed",
				"stage":  "patch_apply_invalid",
				"error":  common.ErrInvalidArgument.Error(),
			},
		}, common.ErrInvalidArgument
	}
}

var _ RuntimeAdapter = (*PatchRuntimeAdapter)(nil)

func boolFromAnyDefault(value any, fallback bool) bool {
	if value == nil {
		return fallback
	}
	typed, ok := value.(bool)
	if !ok {
		return fallback
	}
	return typed
}

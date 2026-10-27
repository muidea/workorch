package memory

import (
	"context"

	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (r *RunRepository) ObserveWorkspacePreparationHistory(ctx context.Context, workspaceID common.WorkspaceID) (runmodel.WorkspacePreparationHistory, error) {
	return r.readWorkspacePreparation(ctx, workspaceID, "", nil)
}

func (r *RunRepository) ReadWorkspaceExecutionIntentReceipt(ctx context.Context, workspaceID common.WorkspaceID, requestID string) (*runmodel.WorkspaceExecutionIntentReceipt, error) {
	var receipt *runmodel.WorkspaceExecutionIntentReceipt
	_, err := r.readWorkspacePreparation(ctx, workspaceID, requestID, &receipt)
	return receipt, err
}

func (r *RunRepository) readWorkspacePreparation(ctx context.Context, workspaceID common.WorkspaceID, requestID string, receipt **runmodel.WorkspaceExecutionIntentReceipt) (runmodel.WorkspacePreparationHistory, error) {
	if ctx == nil || runmodel.ValidatePreparationHistoryWorkspaceID(workspaceID) != nil {
		return runmodel.WorkspacePreparationHistory{}, common.ErrInvalidArgument
	}
	if err := validateRuntimeWorkspaceScope(ctx, workspaceID); err != nil {
		return runmodel.WorkspacePreparationHistory{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return runmodel.WorkspacePreparationHistory{}, err
	}
	if err := validateMemoryRunInventory(r.items); err != nil {
		return runmodel.WorkspacePreparationHistory{}, err
	}
	var retirement *runmodel.WorkspaceRetirementFence
	if value, ok := r.workspaceRetirements[workspaceID]; ok {
		retirement = &value
	}
	items := []*runmodel.Run{}
	for _, item := range r.items {
		if item != nil && item.WorkspaceID == workspaceID {
			items = append(items, item)
		}
	}
	history, err := runmodel.CompileWorkspacePreparationHistory(workspaceID, r.workspaceGenerations[workspaceID], retirement, items)
	if err == nil && receipt != nil {
		*receipt, err = runmodel.CompileExecutionIntentReceipt(workspaceID, requestID, items)
	}
	return history, err
}

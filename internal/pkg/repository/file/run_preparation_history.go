package file

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
	items, err := loadRunMap(r.path)
	if err != nil {
		return runmodel.WorkspacePreparationHistory{}, err
	}
	heads, err := loadMap[common.WorkspaceID, runWorkspacePersistenceHead](r.workspaceGenerationPath())
	if err != nil {
		return runmodel.WorkspacePreparationHistory{}, err
	}
	if err := validatePersistedRunGenerationWatermarks(items, heads); err != nil {
		return runmodel.WorkspacePreparationHistory{}, err
	}
	head := heads[workspaceID]
	runs := []*runmodel.Run{}
	for _, item := range items {
		if item.WorkspaceID == workspaceID {
			value := item
			runs = append(runs, &value)
		}
	}
	history, err := runmodel.CompileWorkspacePreparationHistory(workspaceID, head.Generation, head.Retirement, runs)
	if err == nil && receipt != nil {
		*receipt, err = runmodel.CompileExecutionIntentReceipt(workspaceID, requestID, runs)
	}
	return history, err
}

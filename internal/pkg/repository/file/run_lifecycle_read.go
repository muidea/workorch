package file

import (
	"context"
	run "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (r *RunRepository) GetRuntimeLifecycle(_ context.Context, workspaceID common.WorkspaceID, id common.RunID) (run.RuntimeLifecycleState, error) {
	if workspaceID == "" || id == "" {
		return run.RuntimeLifecycleState{}, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items, err := loadRunMap(r.path)
	if err != nil {
		return run.RuntimeLifecycleState{}, err
	}
	item, found := items[id]
	if !found || item.WorkspaceID != workspaceID {
		return run.RuntimeLifecycleState{}, common.ErrNotFound
	}
	return run.RuntimeLifecycleFromRun(&item)
}

func (r *RunRepository) GetRuntimeStateWriteBase(_ context.Context, workspaceID common.WorkspaceID, id common.RunID) (run.RuntimeStateWriteBase, error) {
	if workspaceID == "" || id == "" {
		return run.RuntimeStateWriteBase{}, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items, err := loadRunMap(r.path)
	if err != nil {
		return run.RuntimeStateWriteBase{}, err
	}
	item, found := items[id]
	if !found || item.WorkspaceID != workspaceID {
		return run.RuntimeStateWriteBase{}, common.ErrNotFound
	}
	return run.RuntimeStateWriteBaseFromRun(&item)
}

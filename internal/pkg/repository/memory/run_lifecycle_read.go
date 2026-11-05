package memory

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
	if err := validateMemoryRunInventory(r.items); err != nil {
		return run.RuntimeLifecycleState{}, err
	}
	item, found := r.items[workspaceScopedKey(workspaceID, string(id))]
	if !found {
		return run.RuntimeLifecycleState{}, common.ErrNotFound
	}
	return run.RuntimeLifecycleFromRun(item)
}

func (r *RunRepository) GetRuntimeStateWriteBase(_ context.Context, workspaceID common.WorkspaceID, id common.RunID) (run.RuntimeStateWriteBase, error) {
	if workspaceID == "" || id == "" {
		return run.RuntimeStateWriteBase{}, common.ErrInvalidArgument
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := validateMemoryRunInventory(r.items); err != nil {
		return run.RuntimeStateWriteBase{}, err
	}
	item, found := r.items[workspaceScopedKey(workspaceID, string(id))]
	if !found {
		return run.RuntimeStateWriteBase{}, common.ErrNotFound
	}
	return run.RuntimeStateWriteBaseFromRun(item)
}

package memory

import (
	"context"
	run "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"sort"
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

func (r *RunRepository) ListInventory(_ context.Context, workspaceID common.WorkspaceID) ([]run.RunInventoryItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if err := validateMemoryRunInventory(r.items); err != nil {
		return nil, err
	}
	items := r.items
	ret := make([]run.RunInventoryItem, 0, len(items))
	for _, item := range items {
		if workspaceID != "" && item.WorkspaceID != workspaceID {
			continue
		}
		ret = append(ret, run.InventoryFromRun(item))
	}
	sort.Slice(ret, func(i, j int) bool {
		if ret[i].WorkspaceID != ret[j].WorkspaceID {
			return ret[i].WorkspaceID < ret[j].WorkspaceID
		}
		return ret[i].ID < ret[j].ID
	})
	return ret, nil
}

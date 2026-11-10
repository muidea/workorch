package file

import (
	"context"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"sort"
)

// ListIDs projects the owner inventory before any authoring/preparation graph
// is cloned. Original storage integrity remains enforced by the load boundary.
func (r *WorkspaceRepository) ListIDs(_ context.Context) ([]common.WorkspaceID, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items, err := loadWorkspacePersistenceMap(r.path)
	if err != nil {
		return nil, err
	}
	ret := make([]common.WorkspaceID, 0, len(items))
	for id, item := range items {
		if item.Deletion != nil && item.Deletion.Completed && item.Deletion.Phase == workspace.WorkspaceDeletionPhaseCompleted {
			continue
		}
		ret = append(ret, id)
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i] < ret[j] })
	return ret, nil
}

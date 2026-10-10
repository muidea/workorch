package workspace

import (
	"context"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func workspaceDeletionRejectsMutation(item *workspacemodel.Workspace) error {
	if item == nil {
		return common.ErrNotFound
	}
	if item.Deletion == nil {
		return nil
	}
	if err := item.Deletion.Validate(item.ID); err != nil {
		return common.ErrConflict
	}
	if item.Deletion.BlocksIngress() {
		return common.ErrConflict
	}
	return nil
}

func (m *Biz) workspaceDeletionReadHidden(ctx context.Context, workspaceID common.WorkspaceID) (bool, error) {
	if m == nil || m.workspaces == nil || workspaceID == "" {
		return false, common.ErrInvalidArgument
	}
	item, err := m.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	if item.Deletion != nil {
		if err := item.Deletion.Validate(item.ID); err != nil {
			return false, common.ErrConflict
		}
	}
	return workspaceDeletionCompleted(item), nil
}

func workspaceDeletionCompleted(item *workspacemodel.Workspace) bool {
	return item != nil && item.Deletion != nil && item.Deletion.Completed && item.Deletion.Phase == workspacemodel.WorkspaceDeletionPhaseCompleted
}

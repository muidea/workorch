package runtimecore

import (
	"context"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (r *Adapter) GetWorkspaceByID(ctx context.Context, workspaceID common.WorkspaceID) (*workspacemodel.Workspace, error) {
	if r == nil {
		return nil, common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.snapshot); err != nil {
		return nil, err
	}
	return r.snapshot.GetWorkspaceByID(ctx, workspaceID)
}

func (r *Adapter) EnsureWorkspace(ctx context.Context, workspaceID common.WorkspaceID, title string) error {
	if r == nil {
		return common.ErrInvalidArgument
	}
	if err := validateRequiredDependencies(r.actions.Workspace); err != nil {
		return err
	}
	return r.actions.Workspace.EnsureWorkspace(ctx, workspaceID, title)
}

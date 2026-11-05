package workorchrepo

import (
	"context"
	run "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (r *workspaceScopedRunRepository) GetRuntimeLifecycle(ctx context.Context, workspaceID common.WorkspaceID, id common.RunID) (run.RuntimeLifecycleState, error) {
	if workspaceID == "" || id == "" {
		return run.RuntimeLifecycleState{}, common.ErrInvalidArgument
	}
	if scoped := common.RuntimeWorkspaceIDFromContext(ctx); scoped != "" && scoped != workspaceID {
		return run.RuntimeLifecycleState{}, common.ErrConflict
	}
	return r.store.runRepo(r.store.runPath(r.store.runRoot(workspaceID, id), workspaceScopedRunFile)).GetRuntimeLifecycle(ctx, workspaceID, id)
}

func (r *workspaceScopedRunRepository) GetRuntimeStateWriteBase(ctx context.Context, workspaceID common.WorkspaceID, id common.RunID) (run.RuntimeStateWriteBase, error) {
	if workspaceID == "" || id == "" {
		return run.RuntimeStateWriteBase{}, common.ErrInvalidArgument
	}
	if scoped := common.RuntimeWorkspaceIDFromContext(ctx); scoped != "" && scoped != workspaceID {
		return run.RuntimeStateWriteBase{}, common.ErrConflict
	}
	return r.store.runRepo(r.store.runPath(r.store.runRoot(workspaceID, id), workspaceScopedRunFile)).GetRuntimeStateWriteBase(ctx, workspaceID, id)
}

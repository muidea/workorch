package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspacecore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func loadWorkspaceSnapshotModels(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID, cachedRunSnapshot *readmodelcore.RunSnapshot) (workspaceSnapshotModels, error) {
	var models workspaceSnapshotModels
	if cachedRunSnapshot != nil && cachedRunSnapshot.Workspace.ID == string(workspaceID) {
		models.Workspace = workspacecore.WorkspaceModelFromView(cachedRunSnapshot.Workspace)
	}

	tasks := workspaceSnapshotBaseLoadTasks(ctx, readers, workspaceID, &models)
	if err := readmodelcore.RunTasks(tasks...); err != nil {
		return workspaceSnapshotModels{}, err
	}
	if err := models.loadRunDerivedItems(ctx, readers, workspaceID); err != nil {
		return workspaceSnapshotModels{}, err
	}
	decisions, err := DecisionsForWorkspaceItems(ctx, readers.Resolutions, readers.AutoGovernanceTraces, workspaceID, models.Tickets)
	if err != nil {
		return workspaceSnapshotModels{}, err
	}
	approvals, err := ApprovalsForWorkspaceItems(ctx, readers.AutoGovernanceTraces, workspaceID, models.ApprovalsRaw)
	if err != nil {
		return workspaceSnapshotModels{}, err
	}
	models.Decisions = decisions
	models.Approvals = approvals
	return models, nil
}

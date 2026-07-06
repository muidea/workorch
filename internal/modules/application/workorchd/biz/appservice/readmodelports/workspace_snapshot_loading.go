package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func loadWorkspaceSnapshotModels(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID, cachedRunSnapshot *readmodelcore.RunSnapshot, options workspaceSnapshotLoadOptions) (workspaceSnapshotModels, error) {
	var models workspaceSnapshotModels

	tasks := workspaceSnapshotBaseLoadTasks(ctx, readers, workspaceID, cachedRunSnapshot, options, &models)
	if err := readmodelcore.RunTasks(tasks...); err != nil {
		return workspaceSnapshotModels{}, err
	}
	projectionRun := models.selectProjectionRun()
	if err := models.loadRunDerivedItems(ctx, readers, workspaceID, projectionRun, options); err != nil {
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

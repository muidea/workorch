package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/pkg/common"
)

func WorkspaceSnapshot(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID, cachedRunSnapshot *readmodelcore.RunSnapshot) (readmodelcore.WorkspaceSnapshot, error) {
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	models, err := loadWorkspaceSnapshotModels(ctx, readers, workspaceID, cachedRunSnapshot)
	if err != nil {
		return readmodelcore.WorkspaceSnapshot{}, err
	}

	projectionRun := models.selectProjectionRun()
	ret := readmodelprojection.WorkspaceSnapshotFromModels(models.projectionInput(projectionRun))
	if projectionRun == nil {
		return ret, nil
	}

	var runSnapshot readmodelcore.RunSnapshot
	if cachedRunSnapshot != nil &&
		cachedRunSnapshot.Run.ID == string(projectionRun.ID) &&
		cachedRunSnapshot.Run.WorkspaceID == string(workspaceID) {
		runSnapshot = *cachedRunSnapshot
	} else {
		runSnapshot, err = RunSnapshot(ctx, effectiveWorkspaceRunSnapshotReaders(readers), projectionRun.ID)
		if err != nil {
			return readmodelcore.WorkspaceSnapshot{}, err
		}
	}
	return readmodelprojection.WorkspaceSnapshotWithRunSnapshot(ret, &runSnapshot), nil
}

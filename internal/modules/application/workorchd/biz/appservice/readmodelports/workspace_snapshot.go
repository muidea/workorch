package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/pkg/common"
)

func WorkspaceSnapshot(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID, cachedRunSnapshot *readmodelcore.RunSnapshotState) (readmodelcore.WorkspaceSnapshotState, error) {
	return workspaceSnapshot(ctx, readers, workspaceID, cachedRunSnapshot, workspaceSnapshotObservabilityLoadOptions())
}

func WorkspaceProjectionSnapshot(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID, cachedRunSnapshot *readmodelcore.RunSnapshotState) (readmodelcore.WorkspaceSnapshotState, error) {
	return WorkspaceSummarySnapshot(ctx, readers, workspaceID, cachedRunSnapshot)
}

func WorkspaceDetailSnapshot(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID, cachedRunSnapshot *readmodelcore.RunSnapshotState) (readmodelcore.WorkspaceSnapshotState, error) {
	return workspaceSnapshot(ctx, readers, workspaceID, cachedRunSnapshot, workspaceSnapshotDetailLoadOptions())
}

func WorkspaceSummarySnapshot(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID, cachedRunSnapshot *readmodelcore.RunSnapshotState) (readmodelcore.WorkspaceSnapshotState, error) {
	snapshot, err := workspaceSnapshot(ctx, readers, workspaceID, cachedRunSnapshot, workspaceSnapshotSummaryLoadOptions())
	if err != nil {
		return readmodelcore.WorkspaceSnapshotState{}, err
	}
	return readmodelprojection.WorkspaceProjection().Summary(snapshot), nil
}

func WorkspaceObservabilitySnapshot(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID, cachedRunSnapshot *readmodelcore.RunSnapshotState) (readmodelcore.WorkspaceSnapshotState, error) {
	return workspaceSnapshot(ctx, readers, workspaceID, cachedRunSnapshot, workspaceSnapshotObservabilityLoadOptions())
}

func workspaceSnapshot(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID, cachedRunSnapshot *readmodelcore.RunSnapshotState, options workspaceSnapshotLoadOptions) (readmodelcore.WorkspaceSnapshotState, error) {
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	models, err := loadWorkspaceSnapshotModels(ctx, readers, workspaceID, cachedRunSnapshot, options)
	if err != nil {
		return readmodelcore.WorkspaceSnapshotState{}, err
	}

	projectionRun := models.selectProjectionRun()
	ret := readmodelprojection.WorkspaceProjection().SnapshotFromModels(models.projectionInput(projectionRun))
	if projectionRun == nil {
		return ret, nil
	}
	if !options.includeProjectionRunSnapshot {
		return ret, nil
	}

	var selectedRunSnapshot readmodelcore.RunSnapshotState
	if cachedRunSnapshot != nil &&
		cachedRunSnapshot.Run.ID == string(projectionRun.ID) &&
		cachedRunSnapshot.Run.WorkspaceID == string(workspaceID) {
		selectedRunSnapshot = *cachedRunSnapshot
	} else {
		selectedRunSnapshot, err = runSnapshot(ctx, effectiveWorkspaceRunSnapshotReaders(readers), projectionRun.ID, options.projectionRunLoadMode, false)
		if err != nil {
			return readmodelcore.WorkspaceSnapshotState{}, err
		}
	}
	ret = readmodelprojection.WorkspaceProjection().SnapshotWithRunSnapshot(ret, &selectedRunSnapshot)
	if cachedRunSnapshot != nil &&
		cachedRunSnapshot.Run.WorkspaceID == string(workspaceID) &&
		cachedRunSnapshot.TaskProgress.WorkspaceID != "" {
		ret.TaskProgress = cachedRunSnapshot.TaskProgress
	}
	return ret, nil
}

package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/pkg/common"
)

func WorkspaceTaskProgress(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID) (readmodelcore.TaskProgress, error) {
	snapshot, err := WorkspaceSnapshot(ctx, readers, workspaceID, nil)
	if err != nil {
		return readmodelcore.TaskProgress{}, err
	}
	return readmodelprojection.WorkspaceSnapshotTaskProgress(snapshot), nil
}

func WorkspaceContextBundle(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID) (readmodelcore.ContextBundle, error) {
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	snapshot, err := WorkspaceSnapshot(ctx, readers, workspaceID, nil)
	if err != nil {
		return readmodelcore.ContextBundle{}, err
	}
	if snapshot.TaskProgress.RunID == "" {
		return readmodelprojection.WorkspaceContextBundleFromSnapshots(snapshot, nil), nil
	}
	runSnapshot, err := RunSnapshot(ctx, effectiveWorkspaceRunSnapshotReaders(readers), common.RunID(snapshot.TaskProgress.RunID))
	if err != nil {
		return readmodelcore.ContextBundle{}, err
	}
	return readmodelprojection.WorkspaceContextBundleFromSnapshots(snapshot, &runSnapshot), nil
}

func WorkspaceTaskDecompositions(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID) ([]readmodelcore.TaskDecomposition, error) {
	snapshot, err := WorkspaceSnapshot(ctx, readers, workspaceID, nil)
	if err != nil {
		return nil, err
	}
	if len(snapshot.TaskDecompositions) > 0 || snapshot.TaskProgress.RunID == "" {
		return readmodelprojection.WorkspaceTaskDecompositionsFromSnapshots(snapshot, nil), nil
	}
	runSnapshot, err := RunSnapshot(ctx, effectiveWorkspaceRunSnapshotReaders(readers), common.RunID(snapshot.TaskProgress.RunID))
	if err != nil {
		return nil, err
	}
	return readmodelprojection.WorkspaceTaskDecompositionsFromSnapshots(snapshot, &runSnapshot), nil
}

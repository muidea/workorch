package readmodelports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func WorkspaceTaskProgress(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID) (projectioncontract.TaskProgress, error) {
	snapshot, err := WorkspaceSnapshot(ctx, readers, workspaceID, nil)
	if err != nil {
		return projectioncontract.TaskProgress{}, err
	}
	return readmodelprojection.WorkspaceProjection().SnapshotTaskProgress(snapshot), nil
}

func WorkspaceContextBundle(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID) (projectioncontract.ContextBundle, error) {
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	snapshot, err := WorkspaceSnapshot(ctx, readers, workspaceID, nil)
	if err != nil {
		return projectioncontract.ContextBundle{}, err
	}
	if snapshot.TaskProgress.RunID == "" {
		return readmodelprojection.ContextProjection().WorkspaceBundle(snapshot, nil), nil
	}
	runSnapshot, err := RunSnapshot(ctx, effectiveWorkspaceRunSnapshotReaders(readers), common.RunID(snapshot.TaskProgress.RunID))
	if err != nil {
		return projectioncontract.ContextBundle{}, err
	}
	return readmodelprojection.ContextProjection().WorkspaceBundle(snapshot, &runSnapshot), nil
}

func WorkspaceTaskDecompositions(ctx context.Context, readers WorkspaceSnapshotReaders, workspaceID common.WorkspaceID) ([]projectioncontract.TaskDecomposition, error) {
	snapshot, err := WorkspaceSnapshot(ctx, readers, workspaceID, nil)
	if err != nil {
		return nil, err
	}
	if len(snapshot.TaskDecompositions) > 0 || snapshot.TaskProgress.RunID == "" {
		return readmodelprojection.WorkspaceProjection().TaskDecompositionsFromSnapshots(snapshot, nil), nil
	}
	runSnapshot, err := RunSnapshot(ctx, effectiveWorkspaceRunSnapshotReaders(readers), common.RunID(snapshot.TaskProgress.RunID))
	if err != nil {
		return nil, err
	}
	return readmodelprojection.WorkspaceProjection().TaskDecompositionsFromSnapshots(snapshot, &runSnapshot), nil
}

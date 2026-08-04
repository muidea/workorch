package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func readWorkspaceSnapshot(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID, cachedRunSnapshot *readmodelcore.RunSnapshot) (readmodelcore.WorkspaceSnapshot, error) {
	return readmodelports.WorkspaceSnapshot(ctx, workspaceSnapshotReaders(deps), workspaceID, cachedRunSnapshot)
}

func readWorkspaceDetailSnapshot(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID, cachedRunSnapshot *readmodelcore.RunSnapshot) (readmodelcore.WorkspaceSnapshot, error) {
	return readmodelports.WorkspaceDetailSnapshot(ctx, workspaceSnapshotReaders(deps), workspaceID, cachedRunSnapshot)
}

func readWorkspaceProjectionSnapshot(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID, cachedRunSnapshot *readmodelcore.RunSnapshot) (readmodelcore.WorkspaceSnapshot, error) {
	return readmodelports.WorkspaceProjectionSnapshot(ctx, workspaceSnapshotReaders(deps), workspaceID, cachedRunSnapshot)
}

func readWorkspaceSummarySnapshot(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID, cachedRunSnapshot *readmodelcore.RunSnapshot) (readmodelcore.WorkspaceSnapshot, error) {
	return readmodelports.WorkspaceSummarySnapshot(ctx, workspaceSnapshotReaders(deps), workspaceID, cachedRunSnapshot)
}

func readWorkspaceTaskProgress(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID) (projectioncontract.TaskProgress, error) {
	return readmodelports.WorkspaceTaskProgress(ctx, workspaceSnapshotReaders(deps), workspaceID)
}

func readWorkspaceContextBundle(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID) (readmodelcore.ContextBundle, error) {
	return readmodelports.WorkspaceContextBundle(ctx, workspaceSnapshotReaders(deps), workspaceID)
}

func readWorkspaceTaskDecompositions(ctx context.Context, deps readModelDependencies, workspaceID common.WorkspaceID) ([]projectioncontract.TaskDecomposition, error) {
	return readmodelports.WorkspaceTaskDecompositions(ctx, workspaceSnapshotReaders(deps), workspaceID)
}

func workspaceSnapshotReaders(deps readModelDependencies) readmodelports.WorkspaceSnapshotReaders {
	candidates := ownerReaderCandidatesFromReadModelDependencies(deps)
	return candidates.WorkspaceSnapshotReaders(deps.ownerReaders, deps.published)
}

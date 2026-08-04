package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadWorkspaceSnapshotWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string) (ret readmodelcore.WorkspaceSnapshot, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (readmodelcore.WorkspaceSnapshot, error) {
		return readWorkspaceSnapshot(ctx, deps, common.WorkspaceID(workspaceID), nil)
	})
}

func ReadWorkspaceSnapshotWithRunSnapshotWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string, runSnapshot readmodelcore.RunSnapshot) (ret readmodelcore.WorkspaceSnapshot, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (readmodelcore.WorkspaceSnapshot, error) {
		return readWorkspaceSnapshot(ctx, deps, common.WorkspaceID(workspaceID), &runSnapshot)
	})
}

func ReadWorkspaceDetailSnapshotWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string) (ret readmodelcore.WorkspaceSnapshot, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (readmodelcore.WorkspaceSnapshot, error) {
		return readWorkspaceDetailSnapshot(ctx, deps, common.WorkspaceID(workspaceID), nil)
	})
}

func ReadWorkspaceDetailSnapshotWithRunSnapshotWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string, runSnapshot readmodelcore.RunSnapshot) (ret readmodelcore.WorkspaceSnapshot, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (readmodelcore.WorkspaceSnapshot, error) {
		return readWorkspaceDetailSnapshot(ctx, deps, common.WorkspaceID(workspaceID), &runSnapshot)
	})
}

func ReadWorkspaceProjectionSnapshotWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string) (ret readmodelcore.WorkspaceSnapshot, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (readmodelcore.WorkspaceSnapshot, error) {
		return readWorkspaceProjectionSnapshot(ctx, deps, common.WorkspaceID(workspaceID), nil)
	})
}

func ReadWorkspaceSummarySnapshotWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string) (ret readmodelcore.WorkspaceSnapshot, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (readmodelcore.WorkspaceSnapshot, error) {
		return readWorkspaceSummarySnapshot(ctx, deps, common.WorkspaceID(workspaceID), nil)
	})
}

func ReadWorkspaceProjectionSnapshotWithRunSnapshotWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string, runSnapshot readmodelcore.RunSnapshot) (ret readmodelcore.WorkspaceSnapshot, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (readmodelcore.WorkspaceSnapshot, error) {
		return readWorkspaceProjectionSnapshot(ctx, deps, common.WorkspaceID(workspaceID), &runSnapshot)
	})
}

func ReadWorkspaceProjectionSnapshotWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (readmodelcore.WorkspaceSnapshot, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.WorkspaceSnapshot, error) {
		return ReadWorkspaceProjectionSnapshotWithSession(ctx, session, workspaceID)
	})
}

func ReadWorkspaceDetailSnapshotWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (readmodelcore.WorkspaceSnapshot, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.WorkspaceSnapshot, error) {
		return ReadWorkspaceDetailSnapshotWithSession(ctx, session, workspaceID)
	})
}

func ReadWorkspaceSnapshotWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (readmodelcore.WorkspaceSnapshot, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.WorkspaceSnapshot, error) {
		return ReadWorkspaceSnapshotWithSession(ctx, session, workspaceID)
	})
}

func ReadWorkspaceSnapshotFromEnv(ctx context.Context, workspaceID string) (readmodelcore.WorkspaceSnapshot, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.WorkspaceSnapshot, error) {
		return ReadWorkspaceSnapshotWithSession(ctx, session, workspaceID)
	})
}

func ReadWorkspaceTaskProgressWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string) (ret readmodelcore.TaskProgress, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (readmodelcore.TaskProgress, error) {
		return readWorkspaceTaskProgress(ctx, deps, common.WorkspaceID(workspaceID))
	})
}

func ReadWorkspaceTaskProgressWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (readmodelcore.TaskProgress, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.TaskProgress, error) {
		return ReadWorkspaceTaskProgressWithSession(ctx, session, workspaceID)
	})
}

func ReadWorkspaceTaskProgressFromEnv(ctx context.Context, workspaceID string) (readmodelcore.TaskProgress, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.TaskProgress, error) {
		return ReadWorkspaceTaskProgressWithSession(ctx, session, workspaceID)
	})
}

func ReadWorkspaceContextBundleWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string) (ret readmodelcore.ContextBundle, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (readmodelcore.ContextBundle, error) {
		return readWorkspaceContextBundle(ctx, deps, common.WorkspaceID(workspaceID))
	})
}

func ReadWorkspaceContextBundleWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (readmodelcore.ContextBundle, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.ContextBundle, error) {
		return ReadWorkspaceContextBundleWithSession(ctx, session, workspaceID)
	})
}

func ReadWorkspaceContextBundleFromEnv(ctx context.Context, workspaceID string) (readmodelcore.ContextBundle, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (readmodelcore.ContextBundle, error) {
		return ReadWorkspaceContextBundleWithSession(ctx, session, workspaceID)
	})
}

func ReadWorkspaceTaskDecompositionsWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string) (ret []readmodelcore.TaskDecomposition, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) ([]readmodelcore.TaskDecomposition, error) {
		return readWorkspaceTaskDecompositions(ctx, deps, common.WorkspaceID(workspaceID))
	})
}

func ReadWorkspaceTaskDecompositionsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]readmodelcore.TaskDecomposition, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) ([]readmodelcore.TaskDecomposition, error) {
		return ReadWorkspaceTaskDecompositionsWithSession(ctx, session, workspaceID)
	})
}

func ReadWorkspaceTaskDecompositionsFromEnv(ctx context.Context, workspaceID string) ([]readmodelcore.TaskDecomposition, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) ([]readmodelcore.TaskDecomposition, error) {
		return ReadWorkspaceTaskDecompositionsWithSession(ctx, session, workspaceID)
	})
}

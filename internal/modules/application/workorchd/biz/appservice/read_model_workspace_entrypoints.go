package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadWorkspaceSnapshotWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret WorkspaceSnapshot, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (WorkspaceSnapshot, error) {
		return readWorkspaceSnapshot(ctx, deps, common.WorkspaceID(workspaceID), nil)
	})
}

func ReadWorkspaceSnapshotWithRunSnapshotWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, runSnapshot RunSnapshot) (ret WorkspaceSnapshot, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (WorkspaceSnapshot, error) {
		return readWorkspaceSnapshot(ctx, deps, common.WorkspaceID(workspaceID), &runSnapshot)
	})
}

func ReadWorkspaceProjectionSnapshotWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret WorkspaceSnapshot, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (WorkspaceSnapshot, error) {
		return readWorkspaceProjectionSnapshot(ctx, deps, common.WorkspaceID(workspaceID), nil)
	})
}

func ReadWorkspaceProjectionSnapshotWithRunSnapshotWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, runSnapshot RunSnapshot) (ret WorkspaceSnapshot, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (WorkspaceSnapshot, error) {
		return readWorkspaceProjectionSnapshot(ctx, deps, common.WorkspaceID(workspaceID), &runSnapshot)
	})
}

func ReadWorkspaceProjectionSnapshotWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (WorkspaceSnapshot, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (WorkspaceSnapshot, error) {
		return ReadWorkspaceProjectionSnapshotWithSession(ctx, session, workspaceID)
	})
}

func ReadWorkspaceSnapshotWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (WorkspaceSnapshot, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (WorkspaceSnapshot, error) {
		return ReadWorkspaceSnapshotWithSession(ctx, session, workspaceID)
	})
}

func ReadWorkspaceSnapshotFromEnv(ctx context.Context, workspaceID string) (WorkspaceSnapshot, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (WorkspaceSnapshot, error) {
		return ReadWorkspaceSnapshotWithSession(ctx, session, workspaceID)
	})
}

func ReadWorkspaceTaskProgressWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret TaskProgress, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (TaskProgress, error) {
		return readWorkspaceTaskProgress(ctx, deps, common.WorkspaceID(workspaceID))
	})
}

func ReadWorkspaceTaskProgressWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (TaskProgress, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (TaskProgress, error) {
		return ReadWorkspaceTaskProgressWithSession(ctx, session, workspaceID)
	})
}

func ReadWorkspaceTaskProgressFromEnv(ctx context.Context, workspaceID string) (TaskProgress, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (TaskProgress, error) {
		return ReadWorkspaceTaskProgressWithSession(ctx, session, workspaceID)
	})
}

func ReadWorkspaceContextBundleWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret ContextBundle, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (ContextBundle, error) {
		return readWorkspaceContextBundle(ctx, deps, common.WorkspaceID(workspaceID))
	})
}

func ReadWorkspaceContextBundleWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (ContextBundle, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (ContextBundle, error) {
		return ReadWorkspaceContextBundleWithSession(ctx, session, workspaceID)
	})
}

func ReadWorkspaceContextBundleFromEnv(ctx context.Context, workspaceID string) (ContextBundle, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (ContextBundle, error) {
		return ReadWorkspaceContextBundleWithSession(ctx, session, workspaceID)
	})
}

func ReadWorkspaceTaskDecompositionsWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret []TaskDecomposition, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) ([]TaskDecomposition, error) {
		return readWorkspaceTaskDecompositions(ctx, deps, common.WorkspaceID(workspaceID))
	})
}

func ReadWorkspaceTaskDecompositionsWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) ([]TaskDecomposition, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]TaskDecomposition, error) {
		return ReadWorkspaceTaskDecompositionsWithSession(ctx, session, workspaceID)
	})
}

func ReadWorkspaceTaskDecompositionsFromEnv(ctx context.Context, workspaceID string) ([]TaskDecomposition, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) ([]TaskDecomposition, error) {
		return ReadWorkspaceTaskDecompositionsWithSession(ctx, session, workspaceID)
	})
}

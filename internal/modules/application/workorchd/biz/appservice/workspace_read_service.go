package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadWorkspaceWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret Workspace, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) (Workspace, error) {
		snapshot, err := readWorkspaceSnapshot(ctx, deps, common.WorkspaceID(workspaceID), nil)
		if err != nil {
			return Workspace{}, err
		}
		return snapshot.Workspace, nil
	})
}

func ReadWorkspaceWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (Workspace, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Workspace, error) {
		return ReadWorkspaceWithSession(ctx, session, workspaceID)
	})
}

func ReadWorkspaceFromEnv(ctx context.Context, workspaceID string) (Workspace, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (Workspace, error) {
		return ReadWorkspaceWithSession(ctx, session, workspaceID)
	})
}

func ListWorkspacesWithSession(ctx context.Context, session *RuntimeSession) (ret []Workspace, err error) {
	return withReadModelSession(ctx, session, func(ctx context.Context, deps readModelDependencies) ([]Workspace, error) {
		return readmodelports.ListProjectedWorkspaces(ctx, deps.workspaces, deps.runs, deps.results, deps.eventRecords)
	})
}

func ListWorkspacesWithBackend(ctx context.Context, backend string, dataRoot string) ([]Workspace, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) ([]Workspace, error) {
		return ListWorkspacesWithSession(ctx, session)
	})
}

func ListWorkspacesFromEnv(ctx context.Context) ([]Workspace, error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) ([]Workspace, error) {
		return ListWorkspacesWithSession(ctx, session)
	})
}

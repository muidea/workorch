package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	"github.com/muidea/workorch/internal/pkg/events"
)

func CreateWorkspaceWithSession(ctx context.Context, session *RuntimeSession, title string, description string, deliverableDirectory string) (ret Workspace, err error) {
	return CreateWorkspaceWithTemplateWithSession(ctx, session, CreateWorkspaceCommand{
		Title:                title,
		Description:          description,
		DeliverableDirectory: deliverableDirectory,
	})
}

func CreateWorkspaceWithTemplateWithSession(ctx context.Context, session *RuntimeSession, cmd CreateWorkspaceCommand) (ret Workspace, err error) {
	return withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (Workspace, error) {
		return workspaceports.CreateWorkspaceView(ctx, hub, workspaceports.CreateWorkspaceViewRequest{
			RepositoryBackend: session.RepositoryBackend(),
			RepositoryDataDir: session.RepositoryDataDir(),
			Command:           cmd,
		})
	})
}

func CreateWorkspaceWithBackend(ctx context.Context, backend string, dataRoot string, title string, description string, deliverableDirectory string) (Workspace, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Workspace, error) {
		return CreateWorkspaceWithSession(ctx, session, title, description, deliverableDirectory)
	})
}

func CreateWorkspaceWithTemplateWithBackend(ctx context.Context, backend string, dataRoot string, cmd CreateWorkspaceCommand) (Workspace, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (Workspace, error) {
		return CreateWorkspaceWithTemplateWithSession(ctx, session, cmd)
	})
}

func CreateWorkspaceFromEnv(ctx context.Context, title string, description string, deliverableDirectory string) (ret Workspace, err error) {
	return withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (Workspace, error) {
		return CreateWorkspaceWithSession(ctx, session, title, description, deliverableDirectory)
	})
}

func DeleteWorkspaceWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, cmd DeleteWorkspaceCommand) (err error) {
	_, err = withEventHubSession(ctx, session, func(ctx context.Context, hub events.Hub) (struct{}, error) {
		return struct{}{}, workspaceports.DeleteWorkspaceView(ctx, hub, workspaceports.DeleteWorkspaceViewRequest{
			RepositoryBackend: session.RepositoryBackend(),
			RepositoryDataDir: session.RepositoryDataDir(),
			WorkspaceID:       workspaceID,
			Command:           cmd,
		})
	})
	return err
}

func DeleteWorkspaceWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd DeleteWorkspaceCommand) error {
	_, err := withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *RuntimeSession) (struct{}, error) {
		if err := DeleteWorkspaceWithSession(ctx, session, workspaceID, cmd); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}

func DeleteWorkspaceFromEnv(ctx context.Context, workspaceID string, cmd DeleteWorkspaceCommand) error {
	_, err := withEnvSession(ctx, func(ctx context.Context, session *RuntimeSession) (struct{}, error) {
		if err := DeleteWorkspaceWithSession(ctx, session, workspaceID, cmd); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}

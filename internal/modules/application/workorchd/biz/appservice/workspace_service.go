package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/workspaceports"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func CreateWorkspaceWithSession(ctx context.Context, session *runtimecore.Session, title string, description string, deliverableDirectory string) (ret appmodelcore.Workspace, err error) {
	return CreateWorkspaceWithTemplateWithSession(ctx, session, workorchmanagementcontract.WorkspaceCreateRequest{
		Title:                title,
		Description:          description,
		DeliverableDirectory: deliverableDirectory,
	})
}

func CreateWorkspaceWithTemplateWithSession(ctx context.Context, session *runtimecore.Session, cmd workorchmanagementcontract.WorkspaceCreateRequest) (ret appmodelcore.Workspace, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (appmodelcore.Workspace, error) {
		return workspaceports.CreateWorkspaceView(ctx, hub, workspaceports.CreateWorkspaceViewRequest{
			RepositoryBackend: session.RepositoryBackend(),
			RepositoryDataDir: session.RepositoryDataDir(),
			Command:           cmd,
		})
	})
}

func CreateWorkspaceWithBackend(ctx context.Context, backend string, dataRoot string, title string, description string, deliverableDirectory string) (appmodelcore.Workspace, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (appmodelcore.Workspace, error) {
		return CreateWorkspaceWithSession(ctx, session, title, description, deliverableDirectory)
	})
}

func CreateWorkspaceWithTemplateWithBackend(ctx context.Context, backend string, dataRoot string, cmd workorchmanagementcontract.WorkspaceCreateRequest) (appmodelcore.Workspace, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (appmodelcore.Workspace, error) {
		return CreateWorkspaceWithTemplateWithSession(ctx, session, cmd)
	})
}

func CreateWorkspaceFromEnv(ctx context.Context, title string, description string, deliverableDirectory string) (ret appmodelcore.Workspace, err error) {
	return withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (appmodelcore.Workspace, error) {
		return CreateWorkspaceWithSession(ctx, session, title, description, deliverableDirectory)
	})
}

func DeleteWorkspaceWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string, cmd appmodelcore.DeleteWorkspaceCommand) (err error) {
	_, err = withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (struct{}, error) {
		return struct{}{}, workspaceports.DeleteWorkspaceView(ctx, hub, workspaceports.DeleteWorkspaceViewRequest{
			RepositoryBackend: session.RepositoryBackend(),
			RepositoryDataDir: session.RepositoryDataDir(),
			WorkspaceID:       workspaceID,
			Command:           cmd,
		})
	})
	return err
}

func DeleteWorkspaceWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd appmodelcore.DeleteWorkspaceCommand) error {
	_, err := withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (struct{}, error) {
		if err := DeleteWorkspaceWithSession(ctx, session, workspaceID, cmd); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}

func RefreshWorkspaceTemplateBindingWithSession(ctx context.Context, session *runtimecore.Session, workspaceID string) (ret appmodelcore.Workspace, err error) {
	return withCommandHubSession(ctx, session, func(ctx context.Context, hub magicevent.Hub) (appmodelcore.Workspace, error) {
		return workspaceports.RefreshWorkspaceTemplateBinding(ctx, hub, workspaceID)
	})
}

func RefreshWorkspaceTemplateBindingWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (appmodelcore.Workspace, error) {
	return withBackendSession(ctx, backend, dataRoot, func(ctx context.Context, session *runtimecore.Session) (appmodelcore.Workspace, error) {
		return RefreshWorkspaceTemplateBindingWithSession(ctx, session, workspaceID)
	})
}

func DeleteWorkspaceFromEnv(ctx context.Context, workspaceID string, cmd appmodelcore.DeleteWorkspaceCommand) error {
	_, err := withEnvSession(ctx, func(ctx context.Context, session *runtimecore.Session) (struct{}, error) {
		if err := DeleteWorkspaceWithSession(ctx, session, workspaceID, cmd); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}

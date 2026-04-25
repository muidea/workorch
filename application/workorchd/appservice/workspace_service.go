package appservice

import (
	"context"
	"errors"

	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Workspace struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	State       string `json:"state"`
	ActiveRunID string `json:"activeRunId,omitempty"`
}

func CreateWorkspaceWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, title string) (ret Workspace, err error) {
	service, repo, err := workspaceDependenciesFromSession(session)
	if err != nil {
		return Workspace{}, err
	}
	defer func() {
		err = errors.Join(err, session.Close(ctx))
	}()

	id := common.WorkspaceID(workspaceID)
	if err := service.CreateWorkspace(ctx, workspacemodule.CreateWorkspaceCommand{
		WorkspaceID: id,
		Title:       title,
	}); err != nil {
		return Workspace{}, err
	}

	item, err := repo.Get(ctx, id)
	if err != nil {
		return Workspace{}, err
	}
	return mapWorkspace(item), nil
}

func CreateWorkspaceWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, title string) (Workspace, error) {
	session, err := NewRuntimeSessionWithBackend(backend, dataRoot)
	if err != nil {
		return Workspace{}, err
	}
	return CreateWorkspaceWithSession(ctx, session, workspaceID, title)
}

func CreateWorkspaceFromEnv(ctx context.Context, workspaceID string, title string) (Workspace, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Workspace{}, err
	}
	return CreateWorkspaceWithSession(ctx, session, workspaceID, title)
}

func ReadWorkspaceWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret Workspace, err error) {
	_, repo, err := workspaceDependenciesFromSession(session)
	if err != nil {
		return Workspace{}, err
	}
	defer func() {
		err = errors.Join(err, session.Close(ctx))
	}()

	item, err := repo.Get(ctx, common.WorkspaceID(workspaceID))
	if err != nil {
		return Workspace{}, err
	}
	return mapWorkspace(item), nil
}

func ReadWorkspaceWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (Workspace, error) {
	session, err := NewRuntimeSessionWithBackend(backend, dataRoot)
	if err != nil {
		return Workspace{}, err
	}
	return ReadWorkspaceWithSession(ctx, session, workspaceID)
}

func ReadWorkspaceFromEnv(ctx context.Context, workspaceID string) (Workspace, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Workspace{}, err
	}
	return ReadWorkspaceWithSession(ctx, session, workspaceID)
}

func ListWorkspacesWithSession(ctx context.Context, session *RuntimeSession) (ret []Workspace, err error) {
	_, repo, err := workspaceDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, session.Close(ctx))
	}()

	items, err := repo.List(ctx)
	if err != nil {
		return nil, err
	}
	ret = make([]Workspace, 0, len(items))
	for _, item := range items {
		ret = append(ret, mapWorkspace(item))
	}
	return ret, nil
}

func ListWorkspacesWithBackend(ctx context.Context, backend string, dataRoot string) ([]Workspace, error) {
	session, err := NewRuntimeSessionWithBackend(backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListWorkspacesWithSession(ctx, session)
}

func ListWorkspacesFromEnv(ctx context.Context) ([]Workspace, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return nil, err
	}
	return ListWorkspacesWithSession(ctx, session)
}

func workspaceDependenciesFromSession(session *RuntimeSession) (workspacemodule.Service, repository.WorkspaceRepository, error) {
	if session == nil || session.Bootstrap == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	exports := session.Bootstrap.Exports()
	if exports.Services.WorkspaceService == nil || exports.Repositories.Workspaces == nil {
		return nil, nil, common.ErrInvalidArgument
	}
	return exports.Services.WorkspaceService, exports.Repositories.Workspaces, nil
}

func mapWorkspace(item *workspacemodel.Workspace) Workspace {
	if item == nil {
		return Workspace{}
	}
	return Workspace{
		ID:          string(item.ID),
		Title:       item.Title,
		State:       string(item.State),
		ActiveRunID: string(item.ActiveRunID),
	}
}

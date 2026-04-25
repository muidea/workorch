package appservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	workspacemodule "github.com/muidea/workorch/internal/modules/kernel/workspace"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Workspace struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	WorkingDirectory string `json:"workingDirectory"`
	State            string `json:"state"`
	ActiveRunID      string `json:"activeRunId,omitempty"`
}

func CreateWorkspaceWithSession(ctx context.Context, session *RuntimeSession, title string, description string, workingDirectory string) (ret Workspace, err error) {
	service, repo, err := workspaceDependenciesFromSession(session)
	if err != nil {
		return Workspace{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	for attempt := 0; attempt < 8; attempt++ {
		id := generateWorkspaceID()
		resolvedWorkingDirectory, err := resolveWorkspaceWorkingDirectory(id, workingDirectory)
		if err != nil {
			return Workspace{}, err
		}
		if err := service.CreateWorkspace(ctx, workspacemodule.CreateWorkspaceCommand{
			WorkspaceID:      id,
			Title:            title,
			Description:      description,
			WorkingDirectory: resolvedWorkingDirectory,
		}); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return Workspace{}, err
		}

		item, err := repo.Get(ctx, id)
		if err != nil {
			return Workspace{}, err
		}
		return mapWorkspace(item), nil
	}
	return Workspace{}, common.ErrConflict
}

func createWorkspaceWithIDWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, title string, description string, workingDirectory string) (ret Workspace, err error) {
	service, repo, err := workspaceDependenciesFromSession(session)
	if err != nil {
		return Workspace{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	id := common.WorkspaceID(workspaceID)
	resolvedWorkingDirectory, err := resolveWorkspaceWorkingDirectory(id, workingDirectory)
	if err != nil {
		return Workspace{}, err
	}
	if err := service.CreateWorkspace(ctx, workspacemodule.CreateWorkspaceCommand{
		WorkspaceID:      id,
		Title:            title,
		Description:      description,
		WorkingDirectory: resolvedWorkingDirectory,
	}); err != nil {
		return Workspace{}, err
	}

	item, err := repo.Get(ctx, id)
	if err != nil {
		return Workspace{}, err
	}
	return mapWorkspace(item), nil
}

func CreateWorkspaceWithBackend(ctx context.Context, backend string, dataRoot string, title string, description string, workingDirectory string) (Workspace, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Workspace{}, err
	}
	return CreateWorkspaceWithSession(ctx, session, title, description, workingDirectory)
}

func createWorkspaceWithIDWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, title string, description string, workingDirectory string) (Workspace, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Workspace{}, err
	}
	return createWorkspaceWithIDWithSession(ctx, session, workspaceID, title, description, workingDirectory)
}

func CreateWorkspaceFromEnv(ctx context.Context, title string, description string, workingDirectory string) (Workspace, error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Workspace{}, err
	}
	return CreateWorkspaceWithSession(ctx, session, title, description, workingDirectory)
}

func ReadWorkspaceWithSession(ctx context.Context, session *RuntimeSession, workspaceID string) (ret Workspace, err error) {
	_, repo, err := workspaceDependenciesFromSession(session)
	if err != nil {
		return Workspace{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	item, err := repo.Get(ctx, common.WorkspaceID(workspaceID))
	if err != nil {
		return Workspace{}, err
	}
	return mapWorkspace(item), nil
}

func ReadWorkspaceWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string) (Workspace, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
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
	defer session.CloseOnReturn(ctx, &err)

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
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
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
	exports, err := session.Exports()
	if err != nil {
		return nil, nil, err
	}
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
		ID:               string(item.ID),
		Title:            item.Title,
		Description:      item.Description,
		WorkingDirectory: item.WorkingDirectory,
		State:            string(item.State),
		ActiveRunID:      string(item.ActiveRunID),
	}
}

func generateWorkspaceID() common.WorkspaceID {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return common.WorkspaceID("ws-" + hex.EncodeToString(raw[:]))
	}
	return common.WorkspaceID(fmt.Sprintf("ws-%d", time.Now().UTC().UnixNano()))
}

func resolveWorkspaceWorkingDirectory(id common.WorkspaceID, workingDirectory string) (string, error) {
	workingDirectory = strings.TrimSpace(workingDirectory)
	if workingDirectory != "" {
		return workingDirectory, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, "workspaces", string(id)), nil
}

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
	ID               string            `json:"id"`
	Title            string            `json:"title"`
	Description      string            `json:"description"`
	WorkingDirectory string            `json:"workingDirectory"`
	State            string            `json:"state"`
	ActiveRunID      string            `json:"activeRunId,omitempty"`
	DefaultSoulID    string            `json:"defaultSoulId,omitempty"`
	SoulBindings     map[string]string `json:"soulBindings,omitempty"`
}

func CreateWorkspaceWithSession(ctx context.Context, session *RuntimeSession, title string, description string, workingDirectory string) (ret Workspace, err error) {
	service, repo, err := workspaceDependenciesFromSession(session)
	if err != nil {
		return Workspace{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	for attempt := 0; attempt < 8; attempt++ {
		id := generateWorkspaceID()
		layout, err := resolveWorkspaceDirectoryLayout(session, id, workingDirectory)
		if err != nil {
			return Workspace{}, err
		}
		if err := ensureWorkspaceDirectoryLayout(layout); err != nil {
			return Workspace{}, err
		}
		if err := service.CreateWorkspace(ctx, workspacemodule.CreateWorkspaceCommand{
			WorkspaceID:      id,
			Title:            title,
			Description:      description,
			WorkingDirectory: layout.WorkingDirectory,
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
	layout, err := resolveWorkspaceDirectoryLayout(session, id, workingDirectory)
	if err != nil {
		return Workspace{}, err
	}
	if err := ensureWorkspaceDirectoryLayout(layout); err != nil {
		return Workspace{}, err
	}
	if err := service.CreateWorkspace(ctx, workspacemodule.CreateWorkspaceCommand{
		WorkspaceID:      id,
		Title:            title,
		Description:      description,
		WorkingDirectory: layout.WorkingDirectory,
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
		DefaultSoulID:    string(item.DefaultSoulID),
		SoulBindings:     mapSoulBindingIDs(item.SoulBindings),
	}
}

func generateWorkspaceID() common.WorkspaceID {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return common.WorkspaceID("ws-" + hex.EncodeToString(raw[:]))
	}
	return common.WorkspaceID(fmt.Sprintf("ws-%d", time.Now().UTC().UnixNano()))
}

type workspaceDirectoryLayout struct {
	WorkspaceRoot    string
	WorkingDirectory string
	Managed          bool
}

func resolveWorkspaceDirectoryLayout(session *RuntimeSession, id common.WorkspaceID, workingDirectory string) (workspaceDirectoryLayout, error) {
	workingDirectory = strings.TrimSpace(workingDirectory)
	if workingDirectory != "" {
		return workspaceDirectoryLayout{WorkingDirectory: workingDirectory}, nil
	}
	if strings.EqualFold(session.RepositoryBackend(), "file") && strings.TrimSpace(session.RepositoryDataDir()) != "" {
		workspaceRoot := filepath.Join(session.RepositoryDataDir(), "workspaces", string(id))
		return workspaceDirectoryLayout{
			WorkspaceRoot:    workspaceRoot,
			WorkingDirectory: filepath.Join(workspaceRoot, "working"),
			Managed:          true,
		}, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return workspaceDirectoryLayout{}, err
	}
	return workspaceDirectoryLayout{WorkingDirectory: filepath.Join(wd, "workspaces", string(id))}, nil
}

func ensureWorkspaceDirectoryLayout(layout workspaceDirectoryLayout) error {
	if strings.TrimSpace(layout.WorkingDirectory) == "" {
		return common.ErrInvalidArgument
	}
	if !layout.Managed {
		return os.MkdirAll(layout.WorkingDirectory, 0o755)
	}
	for _, dir := range []string{
		layout.WorkingDirectory,
		filepath.Join(layout.WorkspaceRoot, "artifacts"),
		filepath.Join(layout.WorkspaceRoot, "runs"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

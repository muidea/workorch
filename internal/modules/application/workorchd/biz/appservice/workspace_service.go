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

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
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
	ContextHints     map[string]any    `json:"contextHints,omitempty"`
	EvidencePolicies []map[string]any  `json:"evidencePolicies,omitempty"`
}

type CreateWorkspaceCommand struct {
	Title            string `json:"title"`
	Description      string `json:"description,omitempty"`
	WorkingDirectory string `json:"workingDirectory,omitempty"`
	TemplateID       string `json:"templateId,omitempty"`
}

func CreateWorkspaceWithSession(ctx context.Context, session *RuntimeSession, title string, description string, workingDirectory string) (ret Workspace, err error) {
	return CreateWorkspaceWithTemplateWithSession(ctx, session, CreateWorkspaceCommand{
		Title:            title,
		Description:      description,
		WorkingDirectory: workingDirectory,
	})
}

func CreateWorkspaceWithTemplateWithSession(ctx context.Context, session *RuntimeSession, cmd CreateWorkspaceCommand) (ret Workspace, err error) {
	service, repo, err := workspaceDependenciesFromSession(session)
	if err != nil {
		return Workspace{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	for attempt := 0; attempt < 8; attempt++ {
		id := generateWorkspaceID()
		layout, err := resolveWorkspaceDirectoryLayout(session, id, cmd.WorkingDirectory)
		if err != nil {
			return Workspace{}, err
		}
		if err := ensureWorkspaceDirectoryLayout(layout); err != nil {
			return Workspace{}, err
		}
		if err := service.CreateWorkspace(ctx, workspacemodule.CreateWorkspaceCommand{
			WorkspaceID:      id,
			Title:            cmd.Title,
			Description:      cmd.Description,
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
		item, err = applyWorkspaceTemplateOnCreate(ctx, session, repo, item, cmd.TemplateID)
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

func CreateWorkspaceWithTemplateWithBackend(ctx context.Context, backend string, dataRoot string, cmd CreateWorkspaceCommand) (Workspace, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Workspace{}, err
	}
	return CreateWorkspaceWithTemplateWithSession(ctx, session, cmd)
}

func applyWorkspaceTemplateOnCreate(ctx context.Context, session *RuntimeSession, workspaces repository.WorkspaceRepository, item *workspacemodel.Workspace, templateID string) (*workspacemodel.Workspace, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return item, nil
	}
	if workspaces == nil || item == nil {
		return nil, common.ErrInvalidArgument
	}
	templateRepo, err := workspaceTemplateRepositoryFromSession(session)
	if err != nil {
		return nil, err
	}
	templateItem, err := templateRepo.Get(ctx, templateID)
	if err != nil {
		return nil, err
	}
	template := workspaceTemplateFromRepository(templateItem)
	if err := validateWorkspaceTemplateSouls(ctx, session, template); err != nil {
		return nil, err
	}
	bindings, err := normalizeSoulBindings(template.SoulBindings)
	if err != nil {
		return nil, err
	}
	item.DefaultSoulID = common.SoulID(strings.TrimSpace(template.DefaultSoulID))
	item.SoulBindings = bindings
	item.ContextHints = cloneContextMap(template.ContextHints)
	item.EvidencePolicies = cloneContextMapSlice(template.EvidencePolicies)
	if err := workspaces.Update(ctx, item); err != nil {
		return nil, err
	}
	return workspaces.Get(ctx, item.ID)
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
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return Workspace{}, err
	}
	defer session.CloseOnReturn(ctx, &err)

	snapshot, err := readWorkspaceSnapshot(ctx, deps, common.WorkspaceID(workspaceID), nil)
	if err != nil {
		return Workspace{}, err
	}
	return snapshot.Workspace, nil
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
	deps, err := readModelDependenciesFromSession(session)
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)

	var items []*workspacemodel.Workspace
	var runItems []*runmodel.Run
	var eventRecordItems []*eventstreammodel.EventRecord
	if err := runReadModelTasks(
		func() error {
			var err error
			items, err = deps.workspaces.List(ctx)
			return err
		},
		func() error {
			var err error
			runItems, err = deps.runs.List(ctx)
			return err
		},
		func() error {
			var err error
			eventRecordItems, err = deps.eventRecords.List(ctx, repository.EventRecordQuery{})
			return err
		},
	); err != nil {
		return nil, err
	}
	ret = make([]Workspace, 0, len(items))
	for _, item := range items {
		resultItems, err := deps.results.ListByWorkspace(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		projectionRun := selectWorkspaceProjectionRun(item, runItems, resultItems, eventRecordItems)
		ret = append(ret, mapWorkspaceWithRunProjection(item, projectionRun))
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

type DeleteWorkspaceCommand struct {
	Purge bool `json:"purge,omitempty"`
}

func DeleteWorkspaceWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, cmd DeleteWorkspaceCommand) (err error) {
	service, repo, err := workspaceDependenciesFromSession(session)
	if err != nil {
		return err
	}
	exports, err := session.Exports()
	if err != nil {
		return err
	}
	defer session.CloseOnReturn(ctx, &err)

	id := common.WorkspaceID(strings.TrimSpace(workspaceID))
	if id == "" {
		return common.ErrInvalidArgument
	}
	workspace, err := repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if cmd.Purge {
		if err := purgeWorkspaceRecords(ctx, exports.Repositories, id); err != nil {
			return err
		}
	}
	if err := service.DeleteWorkspace(ctx, id); err != nil {
		return err
	}
	if !cmd.Purge {
		return nil
	}
	if err := purgeWorkspaceEventRecords(ctx, exports.Repositories, id); err != nil {
		return err
	}
	return purgeWorkspaceDirectories(session, id, workspace.WorkingDirectory)
}

func DeleteWorkspaceWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd DeleteWorkspaceCommand) error {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return err
	}
	return DeleteWorkspaceWithSession(ctx, session, workspaceID, cmd)
}

func DeleteWorkspaceFromEnv(ctx context.Context, workspaceID string, cmd DeleteWorkspaceCommand) error {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return err
	}
	return DeleteWorkspaceWithSession(ctx, session, workspaceID, cmd)
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

func purgeWorkspaceRecords(ctx context.Context, repos bootstrap.RepositoryExports, workspaceID common.WorkspaceID) error {
	if repos.Goals != nil {
		items, err := repos.Goals.ListByWorkspaceID(ctx, workspaceID)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := ignoreNotFound(repos.Goals.Delete(ctx, item.ID)); err != nil {
				return err
			}
		}
	}
	if repos.DoDs != nil {
		items, err := repos.DoDs.ListByWorkspaceID(ctx, workspaceID)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := ignoreNotFound(repos.DoDs.Delete(ctx, item.ID)); err != nil {
				return err
			}
		}
	}
	if repos.MemoryItems != nil {
		items, err := repos.MemoryItems.ListByWorkspaceID(ctx, workspaceID)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := ignoreNotFound(repos.MemoryItems.Delete(ctx, item.ID)); err != nil {
				return err
			}
		}
	}
	runIDs, err := workspaceRunIDs(ctx, repos, workspaceID)
	if err != nil {
		return err
	}
	for _, runID := range runIDs {
		if err := purgeRunRecords(ctx, repos, runID); err != nil {
			return err
		}
	}
	if repos.Tickets != nil {
		tickets, err := repos.Tickets.List(ctx)
		if err != nil {
			return err
		}
		for _, item := range tickets {
			if item.WorkspaceID != workspaceID {
				continue
			}
			if repos.Resolutions != nil {
				if err := ignoreNotFound(repos.Resolutions.DeleteByTicketID(ctx, item.ID)); err != nil {
					return err
				}
			}
			if err := ignoreNotFound(repos.Tickets.Delete(ctx, item.ID)); err != nil {
				return err
			}
		}
	}
	if repos.Approvals != nil {
		items, err := repos.Approvals.List(ctx)
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.WorkspaceID != workspaceID {
				continue
			}
			if err := ignoreNotFound(repos.Approvals.Delete(ctx, item.ID)); err != nil {
				return err
			}
		}
	}
	if repos.Results != nil {
		items, err := repos.Results.ListByWorkspace(ctx, workspaceID)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := ignoreNotFound(repos.Results.Delete(ctx, item.ID)); err != nil {
				return err
			}
		}
	}
	if repos.Artifacts != nil {
		items, err := repos.Artifacts.ListByWorkspace(ctx, workspaceID)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := ignoreNotFound(repos.Artifacts.Delete(ctx, item.ID)); err != nil {
				return err
			}
		}
	}
	if repos.EventRecords != nil {
		if err := purgeWorkspaceEventRecords(ctx, repos, workspaceID); err != nil {
			return err
		}
	}
	for _, runID := range runIDs {
		if repos.Runs == nil {
			continue
		}
		if err := ignoreNotFound(repos.Runs.Delete(ctx, runID)); err != nil {
			return err
		}
	}
	return nil
}

func purgeWorkspaceEventRecords(ctx context.Context, repos bootstrap.RepositoryExports, workspaceID common.WorkspaceID) error {
	if repos.EventRecords == nil {
		return nil
	}
	items, err := repos.EventRecords.List(ctx, repository.EventRecordQuery{WorkspaceID: workspaceID})
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.WorkspaceID != workspaceID {
			continue
		}
		if err := ignoreNotFound(repos.EventRecords.Delete(ctx, item.ID)); err != nil {
			return err
		}
	}
	return nil
}

func workspaceRunIDs(ctx context.Context, repos bootstrap.RepositoryExports, workspaceID common.WorkspaceID) ([]common.RunID, error) {
	if repos.Runs == nil {
		return nil, nil
	}
	runs, err := repos.Runs.List(ctx)
	if err != nil {
		return nil, err
	}
	ret := make([]common.RunID, 0, len(runs))
	for _, item := range runs {
		if item.WorkspaceID == workspaceID {
			ret = append(ret, item.ID)
		}
	}
	return ret, nil
}

func purgeRunRecords(ctx context.Context, repos bootstrap.RepositoryExports, runID common.RunID) error {
	if repos.Steps != nil {
		items, err := repos.Steps.ListByRun(ctx, runID)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := ignoreNotFound(repos.Steps.Delete(ctx, item.ID)); err != nil {
				return err
			}
		}
	}
	if repos.ToolCalls != nil {
		items, err := repos.ToolCalls.ListByRun(ctx, runID)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := ignoreNotFound(repos.ToolCalls.Delete(ctx, item.ID)); err != nil {
				return err
			}
		}
	}
	if repos.Patches != nil {
		items, err := repos.Patches.ListByRun(ctx, runID)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := ignoreNotFound(repos.Patches.Delete(ctx, item.ID)); err != nil {
				return err
			}
		}
	}
	if repos.ValidationResults != nil {
		items, err := repos.ValidationResults.ListByRun(ctx, runID)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := ignoreNotFound(repos.ValidationResults.Delete(ctx, item.ID)); err != nil {
				return err
			}
		}
	}
	if repos.RuntimeCheckpoints != nil {
		items, err := repos.RuntimeCheckpoints.ListByRun(ctx, runID)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := ignoreNotFound(repos.RuntimeCheckpoints.Delete(ctx, item.ID)); err != nil {
				return err
			}
		}
	}
	return nil
}

func purgeWorkspaceDirectories(session *RuntimeSession, workspaceID common.WorkspaceID, workingDirectory string) error {
	paths := make([]string, 0, 2)
	if strings.EqualFold(session.RepositoryBackend(), "file") && strings.TrimSpace(session.RepositoryDataDir()) != "" {
		paths = append(paths, filepath.Join(session.RepositoryDataDir(), "workspaces", string(workspaceID)))
	}
	paths = append(paths, workingDirectory)

	seen := map[string]struct{}{}
	for _, item := range paths {
		path, err := safeWorkspaceRemovalPath(session, workspaceID, item)
		if err != nil {
			return err
		}
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

func safeWorkspaceRemovalPath(session *RuntimeSession, workspaceID common.WorkspaceID, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if abs == string(filepath.Separator) {
		return "", common.ErrInvalidArgument
	}
	home, _ := os.UserHomeDir()
	wd, _ := os.Getwd()
	protected := []string{home, wd}
	if session != nil && strings.TrimSpace(session.RepositoryDataDir()) != "" {
		protected = append(protected, session.RepositoryDataDir())
	}
	for _, item := range protected {
		if item == "" {
			continue
		}
		itemAbs, err := filepath.Abs(item)
		if err != nil {
			return "", err
		}
		if abs == filepath.Clean(itemAbs) {
			return "", common.ErrInvalidArgument
		}
	}
	return abs, nil
}

func ignoreNotFound(err error) error {
	if errors.Is(err, common.ErrNotFound) {
		return nil
	}
	return err
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
		ContextHints:     cloneContextMap(item.ContextHints),
		EvidencePolicies: cloneContextMapSlice(item.EvidencePolicies),
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

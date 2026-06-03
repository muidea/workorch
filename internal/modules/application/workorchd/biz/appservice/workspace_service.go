package appservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	eventstreammodel "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Workspace struct {
	ID                   string            `json:"id"`
	Title                string            `json:"title"`
	Description          string            `json:"description"`
	DeliverableDirectory string            `json:"deliverableDirectory"`
	InternalDirectory    string            `json:"-"`
	State                string            `json:"state"`
	ActiveRunID          string            `json:"activeRunId,omitempty"`
	BlockingTicketID     string            `json:"blockingTicketId,omitempty"`
	BlockingApprovalID   string            `json:"blockingApprovalId,omitempty"`
	TemplateID           string            `json:"templateId,omitempty"`
	ProfileID            string            `json:"profileId,omitempty"`
	DefaultSoulID        string            `json:"defaultSoulId,omitempty"`
	SoulBindings         map[string]string `json:"soulBindings,omitempty"`
	ContextHints         map[string]any    `json:"contextHints,omitempty"`
	EvidencePolicies     []map[string]any  `json:"evidencePolicies,omitempty"`
}

type CreateWorkspaceCommand struct {
	Title                string `json:"title"`
	Description          string `json:"description,omitempty"`
	DeliverableDirectory string `json:"deliverableDirectory,omitempty"`
	TemplateID           string `json:"templateId,omitempty"`
}

func CreateWorkspaceWithSession(ctx context.Context, session *RuntimeSession, title string, description string, deliverableDirectory string) (ret Workspace, err error) {
	return CreateWorkspaceWithTemplateWithSession(ctx, session, CreateWorkspaceCommand{
		Title:                title,
		Description:          description,
		DeliverableDirectory: deliverableDirectory,
	})
}

func CreateWorkspaceWithTemplateWithSession(ctx context.Context, session *RuntimeSession, cmd CreateWorkspaceCommand) (ret Workspace, err error) {
	hub, err := workspaceDependenciesFromSession(session)
	if err != nil {
		return Workspace{}, err
	}

	for attempt := 0; attempt < 8; attempt++ {
		id := generateWorkspaceID()
		layout, err := resolveWorkspaceDirectoryLayout(session, id, cmd.DeliverableDirectory)
		if err != nil {
			return Workspace{}, err
		}
		if err := ensureWorkspaceDirectoryLayout(layout); err != nil {
			return Workspace{}, err
		}
		if err := publishOwnerCommand(ctx, hub, events.TopicWorkspaceCommandCreate, "workspace", string(id), id, "", map[string]any{
			events.PayloadKeyWorkspaceID:          string(id),
			events.PayloadKeyTitle:                cmd.Title,
			events.PayloadKeyDescription:          cmd.Description,
			events.PayloadKeyDeliverableDirectory: layout.DeliverableDirectory,
			events.PayloadKeyInternalDirectory:    layout.InternalDirectory,
		}); err != nil {
			if errors.Is(err, common.ErrConflict) {
				continue
			}
			return Workspace{}, err
		}

		item, err := readWorkspaceByOwnerCommand(ctx, hub, id)
		if err != nil {
			return Workspace{}, err
		}
		item, err = applyWorkspaceTemplateOnCreate(ctx, session, item, cmd.TemplateID)
		if err != nil {
			return Workspace{}, err
		}
		return mapWorkspace(item), nil
	}
	return Workspace{}, common.ErrConflict
}

func createWorkspaceWithIDWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, title string, description string, deliverableDirectory string) (ret Workspace, err error) {
	hub, err := workspaceDependenciesFromSession(session)
	if err != nil {
		return Workspace{}, err
	}

	id := common.WorkspaceID(workspaceID)
	layout, err := resolveWorkspaceDirectoryLayout(session, id, deliverableDirectory)
	if err != nil {
		return Workspace{}, err
	}
	if err := ensureWorkspaceDirectoryLayout(layout); err != nil {
		return Workspace{}, err
	}
	if err := publishOwnerCommand(ctx, hub, events.TopicWorkspaceCommandCreate, "workspace", string(id), id, "", map[string]any{
		events.PayloadKeyWorkspaceID:          string(id),
		events.PayloadKeyTitle:                title,
		events.PayloadKeyDescription:          description,
		events.PayloadKeyDeliverableDirectory: layout.DeliverableDirectory,
		events.PayloadKeyInternalDirectory:    layout.InternalDirectory,
	}); err != nil {
		return Workspace{}, err
	}

	item, err := readWorkspaceByOwnerCommand(ctx, hub, id)
	if err != nil {
		return Workspace{}, err
	}
	return mapWorkspace(item), nil
}

func CreateWorkspaceWithBackend(ctx context.Context, backend string, dataRoot string, title string, description string, deliverableDirectory string) (Workspace, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Workspace{}, err
	}
	return CreateWorkspaceWithSession(ctx, session, title, description, deliverableDirectory)
}

func CreateWorkspaceWithTemplateWithBackend(ctx context.Context, backend string, dataRoot string, cmd CreateWorkspaceCommand) (Workspace, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Workspace{}, err
	}
	return CreateWorkspaceWithTemplateWithSession(ctx, session, cmd)
}

func applyWorkspaceTemplateOnCreate(ctx context.Context, session *RuntimeSession, item *workspacemodel.Workspace, templateID string) (*workspacemodel.Workspace, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return item, nil
	}
	if item == nil {
		return nil, common.ErrInvalidArgument
	}
	exports, err := session.Exports()
	if err != nil {
		return nil, err
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
	item.TemplateID = strings.TrimSpace(template.ID)
	item.ProfileID = strings.TrimSpace(template.ID)
	item.SoulBindings = bindings
	item.ContextHints = cloneContextMap(template.ContextHints)
	item.EvidencePolicies = cloneContextMapSlice(template.EvidencePolicies)
	if workspaceTemplateRequestsGitInit(item.ContextHints) {
		if err := initializeWorkspaceGitRepository(ctx, item.DeliverableDirectory); err != nil {
			return nil, err
		}
	}
	if err := ensureWorkspaceManagedConfigs(item.InternalDirectory, item.ContextHints); err != nil {
		return nil, err
	}
	if err := publishOwnerCommand(ctx, exports.Infrastructure.EventHub, events.TopicWorkspaceCommandUpdate, "workspace", string(item.ID), item.ID, "", map[string]any{
		events.PayloadKeyWorkspace: item,
	}); err != nil {
		return nil, err
	}
	response, err := publishOwnerCommandWithResponse(ctx, exports.Infrastructure.EventHub, events.TopicWorkspaceCommandGet, "workspace", string(item.ID), item.ID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(item.ID),
	})
	if err != nil {
		return nil, err
	}
	updated, ok := response.Payload[events.PayloadKeyWorkspace].(*workspacemodel.Workspace)
	if !ok || updated == nil {
		return nil, common.ErrInvalidArgument
	}
	return updated, nil
}

func workspaceTemplateRequestsGitInit(contextHints map[string]any) bool {
	if len(contextHints) == 0 {
		return false
	}
	versionControl, ok := contextHints["versionControl"].(map[string]any)
	if !ok {
		return false
	}
	gitConfig, ok := versionControl["git"].(map[string]any)
	if !ok {
		return false
	}
	value, ok := gitConfig["initializeOnCreate"].(bool)
	return ok && value
}

func initializeWorkspaceGitRepository(ctx context.Context, deliverableDirectory string) error {
	deliverableDirectory = strings.TrimSpace(deliverableDirectory)
	if deliverableDirectory == "" {
		return common.ErrInvalidArgument
	}
	if _, err := os.Stat(filepath.Join(deliverableDirectory, ".git")); err == nil {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("%w: git binary is required to initialize workspace version control", common.ErrInvalidArgument)
	}
	gitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := runWorkspaceGitCommand(gitCtx, gitPath, deliverableDirectory, "init", "-q"); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"config", "user.name", "WorkOrch"},
		{"config", "user.email", "workorch@example.local"},
		{"config", "core.hooksPath", "/dev/null"},
	} {
		if err := runWorkspaceGitCommand(gitCtx, gitPath, deliverableDirectory, args...); err != nil {
			return err
		}
	}
	return nil
}

func ensureWorkspaceManagedConfigs(internalDirectory string, contextHints map[string]any) error {
	internalDirectory = strings.TrimSpace(internalDirectory)
	if internalDirectory == "" || len(contextHints) == 0 {
		return nil
	}
	configs, ok := contextHints["managedWorkspaceConfigs"].([]any)
	if !ok || len(configs) == 0 {
		return nil
	}
	for _, raw := range configs {
		config, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if err := ensureWorkspaceManagedConfig(internalDirectory, config); err != nil {
			return err
		}
	}
	return nil
}

func ensureWorkspaceManagedConfig(internalDirectory string, config map[string]any) error {
	relPath := strings.TrimSpace(stringValueFromMap(config, "path"))
	if relPath == "" {
		return common.ErrInvalidArgument
	}
	cleanRelPath := filepath.Clean(relPath)
	if filepath.IsAbs(cleanRelPath) || cleanRelPath == "." || strings.HasPrefix(cleanRelPath, ".."+string(os.PathSeparator)) || cleanRelPath == ".." {
		return common.ErrInvalidArgument
	}
	target := filepath.Join(internalDirectory, cleanRelPath)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	format := strings.ToLower(strings.TrimSpace(stringValueFromMap(config, "format")))
	switch format {
	case "", "json":
		return ensureWorkspaceManagedJSONConfig(target, config["content"], boolValueFromMap(config, "mergeMissing"))
	default:
		return fmt.Errorf("%w: unsupported workspace managed config format %q", common.ErrInvalidArgument, format)
	}
}

func ensureWorkspaceManagedJSONConfig(target string, content any, mergeMissing bool) error {
	defaults, ok := content.(map[string]any)
	if !ok {
		return common.ErrInvalidArgument
	}
	if existingRaw, err := os.ReadFile(target); err == nil {
		if !mergeMissing {
			return nil
		}
		existing := map[string]any{}
		if len(strings.TrimSpace(string(existingRaw))) > 0 {
			if err := json.Unmarshal(existingRaw, &existing); err != nil {
				return err
			}
		}
		merged := mergeMissingMap(existing, defaults)
		return writeStableJSONFile(target, merged)
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeStableJSONFile(target, defaults)
}

func mergeMissingMap(existing map[string]any, defaults map[string]any) map[string]any {
	if existing == nil {
		existing = map[string]any{}
	}
	for key, value := range defaults {
		if current, ok := existing[key]; ok {
			currentMap, currentOK := current.(map[string]any)
			defaultMap, defaultOK := value.(map[string]any)
			if currentOK && defaultOK {
				existing[key] = mergeMissingMap(currentMap, defaultMap)
			}
			continue
		}
		existing[key] = cloneContextValue(value)
	}
	return existing
}

func stringValueFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, _ := values[key].(string)
	return value
}

func boolValueFromMap(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	value, _ := values[key].(bool)
	return value
}

func writeStableJSONFile(target string, payload any) error {
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(target, raw, 0o644)
}

func runWorkspaceGitCommand(ctx context.Context, gitPath string, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, gitPath, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: git %s failed: %s", common.ErrInvalidArgument, strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

func createWorkspaceWithIDWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, title string, description string, deliverableDirectory string) (Workspace, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Workspace{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	return createWorkspaceWithIDWithSession(ctx, session, workspaceID, title, description, deliverableDirectory)
}

func CreateWorkspaceFromEnv(ctx context.Context, title string, description string, deliverableDirectory string) (ret Workspace, err error) {
	session, err := NewRuntimeSessionFromEnv()
	if err != nil {
		return Workspace{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	return CreateWorkspaceWithSession(ctx, session, title, description, deliverableDirectory)
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
	hub, err := workspaceDependenciesFromSession(session)
	if err != nil {
		return err
	}
	defer session.CloseOnReturn(ctx, &err)

	id := common.WorkspaceID(strings.TrimSpace(workspaceID))
	if id == "" {
		return common.ErrInvalidArgument
	}
	workspace, err := readWorkspaceByOwnerCommand(ctx, hub, id)
	if err != nil {
		return err
	}
	if cmd.Purge {
		if err := purgeWorkspaceRecordsWithEventHub(ctx, hub, id); err != nil {
			return err
		}
	}
	if err := publishOwnerCommand(ctx, hub, events.TopicWorkspaceCommandDelete, "workspace", string(id), id, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(id),
	}); err != nil {
		return err
	}
	if !cmd.Purge {
		return nil
	}
	if err := publishOwnerCommand(ctx, hub, events.TopicEventStreamCommandPurgeWorkspace, "workspace", string(id), id, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(id),
	}); err != nil {
		return err
	}
	return purgeWorkspaceDirectories(session, id, workspace.DeliverableDirectory, workspace.InternalDirectory)
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

func workspaceDependenciesFromSession(session *RuntimeSession) (events.Hub, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	if exports.Infrastructure.EventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	return exports.Infrastructure.EventHub, nil
}

func purgeWorkspaceRecordsWithEventHub(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	if hub == nil || workspaceID == "" {
		return common.ErrInvalidArgument
	}
	runIDs, err := workspaceRunIDsByEventHub(ctx, hub, workspaceID)
	if err != nil {
		return err
	}
	for _, runID := range runIDs {
		if err := publishOwnerCommand(ctx, hub, events.TopicOrchestratorCommandPurgeRun, "run", string(runID), workspaceID, runID, map[string]any{
			events.PayloadKeyRunID: string(runID),
		}); err != nil {
			return err
		}
	}
	if err := purgeWorkspaceMemoryItems(ctx, hub, workspaceID); err != nil {
		return err
	}
	if err := purgeWorkspaceDecisionTickets(ctx, hub, workspaceID); err != nil {
		return err
	}
	if err := purgeWorkspaceApprovals(ctx, hub, workspaceID); err != nil {
		return err
	}
	if err := purgeWorkspaceResultsAndArtifacts(ctx, hub, workspaceID); err != nil {
		return err
	}
	for _, runID := range runIDs {
		if err := publishOwnerCommand(ctx, hub, events.TopicRunCommandDelete, "run", string(runID), workspaceID, runID, map[string]any{
			events.PayloadKeyRunID: string(runID),
		}); err != nil {
			return err
		}
	}
	return nil
}

func workspaceRunIDsByEventHub(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]common.RunID, error) {
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicRunCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	runs, ok := response.Payload[events.PayloadKeyRuns].([]*runmodel.Run)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]common.RunID, 0, len(runs))
	for _, item := range runs {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		ret = append(ret, item.ID)
	}
	return ret, nil
}

func purgeWorkspaceMemoryItems(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicMemoryCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return err
	}
	items, ok := response.Payload[events.PayloadKeyMemoryItems].([]*memorymodel.MemoryItem)
	if !ok {
		return common.ErrInvalidArgument
	}
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		if err := publishOwnerCommand(ctx, hub, events.TopicMemoryCommandDelete, "memory", string(item.ID), workspaceID, item.RunID, map[string]any{
			events.PayloadKeyMemoryID: string(item.ID),
		}); err != nil {
			return err
		}
	}
	return nil
}

func purgeWorkspaceDecisionTickets(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicDecisionCommandListTicketsByWS, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return err
	}
	items, ok := response.Payload[events.PayloadKeyTickets].([]*decisionmodel.Ticket)
	if !ok {
		return common.ErrInvalidArgument
	}
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		_ = publishOwnerCommand(ctx, hub, events.TopicDecisionCommandDeleteResolution, "decision_ticket", string(item.ID), workspaceID, item.RunID, map[string]any{
			events.PayloadKeyBlockingTicketID: string(item.ID),
		})
		if err := publishOwnerCommand(ctx, hub, events.TopicDecisionCommandDeleteTicket, "decision_ticket", string(item.ID), workspaceID, item.RunID, map[string]any{
			events.PayloadKeyBlockingTicketID: string(item.ID),
		}); err != nil {
			return err
		}
	}
	return nil
}

func purgeWorkspaceApprovals(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	response, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicApprovalCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*approvalmodel.Approval)
	if !ok {
		return common.ErrInvalidArgument
	}
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		if err := publishOwnerCommand(ctx, hub, events.TopicApprovalCommandDelete, "approval", string(item.ID), workspaceID, item.RunID, map[string]any{
			events.PayloadKeyApprovalID: string(item.ID),
		}); err != nil {
			return err
		}
	}
	return nil
}

func purgeWorkspaceResultsAndArtifacts(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	artifactResponse, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicArtifactCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return err
	}
	artifacts, ok := artifactResponse.Payload[events.PayloadKeyItems].([]*resultmodel.Artifact)
	if !ok {
		return common.ErrInvalidArgument
	}
	for _, item := range artifacts {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		if err := publishOwnerCommand(ctx, hub, events.TopicArtifactCommandDelete, "artifact", string(item.ID), workspaceID, item.RunID, map[string]any{
			events.PayloadKeyArtifactID: string(item.ID),
		}); err != nil {
			return err
		}
	}
	resultResponse, err := publishOwnerCommandWithResponse(ctx, hub, events.TopicResultCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return err
	}
	results, ok := resultResponse.Payload[events.PayloadKeyItems].([]*resultmodel.Result)
	if !ok {
		return common.ErrInvalidArgument
	}
	for _, item := range results {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		if err := publishOwnerCommand(ctx, hub, events.TopicResultCommandDelete, "result", string(item.ID), workspaceID, item.RunID, map[string]any{
			events.PayloadKeyResultID: string(item.ID),
		}); err != nil {
			return err
		}
	}
	return nil
}

func publishOwnerCommand(ctx context.Context, hub events.Hub, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, payload map[string]any) error {
	_, err := publishOwnerCommandWithResponse(ctx, hub, topic, subjectType, subjectID, workspaceID, runID, payload)
	return err
}

func publishMemoryRecallTraceUpsert(ctx context.Context, hub events.Hub, trace *memorymodel.MemoryRecallTrace) error {
	if trace == nil {
		return common.ErrInvalidArgument
	}
	return publishOwnerCommand(ctx, hub, events.TopicMemoryRecallTraceCommandUpsert, "memory_recall", string(trace.ID), trace.WorkspaceID, trace.RunID, map[string]any{
		events.PayloadKeyMemoryRecallTrace: trace,
		events.PayloadKeyTraceID:           string(trace.ID),
	})
}

func publishOwnerCommandWithResponse(ctx context.Context, hub events.Hub, topic string, subjectType string, subjectID string, workspaceID common.WorkspaceID, runID common.RunID, payload map[string]any) (events.Envelope, error) {
	if hub == nil || topic == "" {
		return events.Envelope{}, common.ErrInvalidArgument
	}
	err := hub.Publish(ctx, events.Envelope{
		Topic:        topic,
		SourceModule: "application/workorchd/appservice",
		SubjectType:  subjectType,
		SubjectID:    subjectID,
		WorkspaceID:  string(workspaceID),
		RunID:        string(runID),
		Payload:      payload,
	})
	if response, ok := events.ResponseEnvelope(err); ok {
		return response, nil
	}
	if err != nil {
		return events.Envelope{}, err
	}
	return events.Envelope{}, common.ErrInvalidArgument
}

func purgeWorkspaceDirectories(session *RuntimeSession, workspaceID common.WorkspaceID, deliverableDirectory string, internalDirectory string) error {
	paths := make([]string, 0, 2)
	if strings.TrimSpace(internalDirectory) != "" {
		paths = append(paths, internalDirectory)
	} else if strings.EqualFold(session.RepositoryBackend(), "file") && strings.TrimSpace(session.RepositoryDataDir()) != "" {
		paths = append(paths, filepath.Join(session.RepositoryDataDir(), "internal", "workspaces", string(workspaceID)))
	}
	paths = append(paths, deliverableDirectory)

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

func mapWorkspace(item *workspacemodel.Workspace) Workspace {
	if item == nil {
		return Workspace{}
	}
	return Workspace{
		ID:                   string(item.ID),
		Title:                item.Title,
		Description:          item.Description,
		DeliverableDirectory: item.DeliverableDirectory,
		InternalDirectory:    item.InternalDirectory,
		State:                string(item.State),
		ActiveRunID:          string(item.ActiveRunID),
		TemplateID:           item.TemplateID,
		ProfileID:            item.ProfileID,
		DefaultSoulID:        string(item.DefaultSoulID),
		SoulBindings:         mapSoulBindingIDs(item.SoulBindings),
		ContextHints:         cloneContextMap(item.ContextHints),
		EvidencePolicies:     cloneContextMapSlice(item.EvidencePolicies),
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
	DeliverableRoot      string
	DeliverableDirectory string
	InternalDirectory    string
	Managed              bool
}

func resolveWorkspaceDirectoryLayout(session *RuntimeSession, id common.WorkspaceID, deliverableDirectory string) (workspaceDirectoryLayout, error) {
	deliverableDirectory = strings.TrimSpace(deliverableDirectory)
	internalDirectory, err := resolveWorkspaceInternalDirectory(session, id)
	if err != nil {
		return workspaceDirectoryLayout{}, err
	}
	if strings.EqualFold(session.RepositoryBackend(), "file") && strings.TrimSpace(session.RepositoryDataDir()) != "" {
		if deliverableDirectory != "" {
			return workspaceDirectoryLayout{
				DeliverableDirectory: deliverableDirectory,
				InternalDirectory:    internalDirectory,
				Managed:              true,
			}, nil
		}
		deliverableRoot := filepath.Join(filepath.Dir(session.RepositoryDataDir()), ".workorch-workspaces")
		return workspaceDirectoryLayout{
			DeliverableRoot:      deliverableRoot,
			DeliverableDirectory: filepath.Join(deliverableRoot, string(id)),
			InternalDirectory:    internalDirectory,
			Managed:              true,
		}, nil
	}
	if deliverableDirectory != "" {
		return workspaceDirectoryLayout{DeliverableDirectory: deliverableDirectory, InternalDirectory: internalDirectory}, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return workspaceDirectoryLayout{}, err
	}
	return workspaceDirectoryLayout{
		DeliverableDirectory: filepath.Join(wd, ".workorch-workspaces", string(id)),
		InternalDirectory:    internalDirectory,
	}, nil
}

func resolveWorkspaceInternalDirectory(session *RuntimeSession, id common.WorkspaceID) (string, error) {
	if session != nil && strings.EqualFold(session.RepositoryBackend(), "file") && strings.TrimSpace(session.RepositoryDataDir()) != "" {
		return filepath.Join(session.RepositoryDataDir(), "internal", "workspaces", string(id)), nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, ".workorch-data", "internal", "workspaces", string(id)), nil
}

func ensureWorkspaceDirectoryLayout(layout workspaceDirectoryLayout) error {
	if strings.TrimSpace(layout.DeliverableDirectory) == "" || strings.TrimSpace(layout.InternalDirectory) == "" {
		return common.ErrInvalidArgument
	}
	if !layout.Managed {
		if err := os.MkdirAll(layout.DeliverableDirectory, 0o755); err != nil {
			return err
		}
		return os.MkdirAll(layout.InternalDirectory, 0o755)
	}
	for _, dir := range []string{
		layout.DeliverableDirectory,
		layout.InternalDirectory,
		filepath.Join(layout.InternalDirectory, "config"),
		filepath.Join(layout.InternalDirectory, "runs"),
		filepath.Join(layout.InternalDirectory, "artifacts"),
		filepath.Join(layout.InternalDirectory, "events"),
		filepath.Join(layout.InternalDirectory, "llm-calls"),
		filepath.Join(layout.InternalDirectory, "indexes"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

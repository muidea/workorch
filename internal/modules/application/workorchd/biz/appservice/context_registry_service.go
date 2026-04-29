package appservice

import (
	"context"
	"errors"
	"strings"

	mcpmodule "github.com/muidea/workorch/internal/modules/kernel/mcp"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodule "github.com/muidea/workorch/internal/modules/kernel/memory"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	skillmodule "github.com/muidea/workorch/internal/modules/kernel/skill"
	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	soulmodule "github.com/muidea/workorch/internal/modules/kernel/soul"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Soul struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Description          string   `json:"description,omitempty"`
	BusinessDomains      []string `json:"businessDomains,omitempty"`
	SoulMD               string   `json:"soulMd"`
	AllowedSkillRefs     []string `json:"allowedSkillRefs,omitempty"`
	AllowedMCPServerRefs []string `json:"allowedMCPServerRefs,omitempty"`
	AllowedToolKinds     []string `json:"allowedToolKinds,omitempty"`
}

type Skill struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Version               string   `json:"version"`
	Description           string   `json:"description,omitempty"`
	TriggerRules          []string `json:"triggerRules,omitempty"`
	Instruction           string   `json:"instruction"`
	ReferenceRefs         []string `json:"referenceRefs,omitempty"`
	RequiredToolKinds     []string `json:"requiredToolKinds,omitempty"`
	RequiredMCPServerRefs []string `json:"requiredMCPServerRefs,omitempty"`
	Constraints           []string `json:"constraints,omitempty"`
	Source                string   `json:"source,omitempty"`
	Scope                 string   `json:"scope,omitempty"`
	WorkspaceID           string   `json:"workspaceId,omitempty"`
	Status                string   `json:"status,omitempty"`
	SourceRepository      string   `json:"sourceRepository,omitempty"`
	SourcePath            string   `json:"sourcePath,omitempty"`
	TargetPath            string   `json:"targetPath,omitempty"`
	LocalPath             string   `json:"localPath,omitempty"`
	RepoPath              string   `json:"repoPath,omitempty"`
	GlobalPath            string   `json:"globalPath,omitempty"`
	ProjectPath           string   `json:"projectPath,omitempty"`
	ContentHash           string   `json:"contentHash,omitempty"`
	SourceHash            string   `json:"sourceHash,omitempty"`
	AppliedHash           string   `json:"appliedHash,omitempty"`
	ActualHash            string   `json:"actualHash,omitempty"`
	Agent                 string   `json:"agent,omitempty"`
	ManagedBy             string   `json:"managedBy,omitempty"`
	SourceSkillID         string   `json:"sourceSkillId,omitempty"`
}

type MCPServer struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Transport             string   `json:"transport"`
	Endpoint              string   `json:"endpoint,omitempty"`
	Enabled               bool     `json:"enabled"`
	AllowedTools          []string `json:"allowedTools,omitempty"`
	AllowedResources      []string `json:"allowedResources,omitempty"`
	AllowedPrompts        []string `json:"allowedPrompts,omitempty"`
	DefaultApprovalPolicy string   `json:"defaultApprovalPolicy,omitempty"`
	SessionPolicy         string   `json:"sessionPolicy,omitempty"`
	TimeoutMillis         int      `json:"timeoutMillis,omitempty"`
}

type MCPTool struct {
	ServerID string `json:"serverId"`
	Name     string `json:"name"`
}

type MCPResource struct {
	ServerID string `json:"serverId"`
	URI      string `json:"uri"`
}

type MCPPrompt struct {
	ServerID string `json:"serverId"`
	Name     string `json:"name"`
}

type MemoryItem struct {
	ID                  string   `json:"id"`
	WorkspaceID         string   `json:"workspaceId"`
	RunID               string   `json:"runId,omitempty"`
	SourceType          string   `json:"sourceType"`
	SourceRef           string   `json:"sourceRef,omitempty"`
	MemoryType          string   `json:"memoryType"`
	Content             string   `json:"content,omitempty"`
	Summary             string   `json:"summary"`
	Tags                []string `json:"tags,omitempty"`
	Importance          int      `json:"importance,omitempty"`
	Confidence          int      `json:"confidence,omitempty"`
	Visibility          string   `json:"visibility,omitempty"`
	EmbeddingProviderID string   `json:"embeddingProviderId,omitempty"`
	EmbeddingModel      string   `json:"embeddingModel,omitempty"`
	EmbeddingText       string   `json:"embeddingText,omitempty"`
	VectorReady         bool     `json:"vectorReady"`
	VectorDimension     int      `json:"vectorDimension,omitempty"`
	VectorScore         float64  `json:"vectorScore,omitempty"`
}

type CreateSoulCommand = Soul
type UpdateSoulCommand = Soul
type CreateSkillCommand = Skill
type CreateMCPServerCommand = MCPServer

type CreateMemoryItemCommand struct {
	MemoryID            string    `json:"memoryId,omitempty"`
	RunID               string    `json:"runId,omitempty"`
	SourceType          string    `json:"sourceType"`
	SourceRef           string    `json:"sourceRef,omitempty"`
	MemoryType          string    `json:"memoryType"`
	Content             string    `json:"content,omitempty"`
	Summary             string    `json:"summary"`
	Tags                []string  `json:"tags,omitempty"`
	Importance          int       `json:"importance,omitempty"`
	Confidence          int       `json:"confidence,omitempty"`
	Visibility          string    `json:"visibility,omitempty"`
	EmbeddingProviderID string    `json:"embeddingProviderId,omitempty"`
	EmbeddingModel      string    `json:"embeddingModel,omitempty"`
	EmbeddingText       string    `json:"embeddingText,omitempty"`
	Vector              []float64 `json:"vector,omitempty"`
}

func CreateSoulWithSession(ctx context.Context, session *RuntimeSession, cmd CreateSoulCommand) (ret Soul, err error) {
	exports, err := session.Exports()
	if err != nil {
		return Soul{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Services.SoulService == nil || exports.Repositories.Souls == nil {
		return Soul{}, common.ErrInvalidArgument
	}
	if err := exports.Services.SoulService.CreateSoul(ctx, soulmodule.CreateSoulCommand{
		SoulID:               common.SoulID(cmd.ID),
		Name:                 cmd.Name,
		Description:          cmd.Description,
		BusinessDomains:      cloneStrings(cmd.BusinessDomains),
		SoulMD:               cmd.SoulMD,
		AllowedSkillRefs:     cloneStrings(cmd.AllowedSkillRefs),
		AllowedMCPServerRefs: cloneStrings(cmd.AllowedMCPServerRefs),
		AllowedToolKinds:     cloneStrings(cmd.AllowedToolKinds),
	}); err != nil {
		return Soul{}, err
	}
	item, err := exports.Repositories.Souls.Get(ctx, common.SoulID(cmd.ID))
	if err != nil {
		return Soul{}, err
	}
	return mapSoul(item), nil
}

func CreateSoulWithBackend(ctx context.Context, backend string, dataRoot string, cmd CreateSoulCommand) (Soul, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Soul{}, err
	}
	return CreateSoulWithSession(ctx, session, cmd)
}

func UpdateSoulWithSession(ctx context.Context, session *RuntimeSession, soulID string, cmd UpdateSoulCommand) (ret Soul, err error) {
	exports, err := session.Exports()
	if err != nil {
		return Soul{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Services.SoulService == nil || exports.Repositories.Souls == nil {
		return Soul{}, common.ErrInvalidArgument
	}
	soulID = strings.TrimSpace(soulID)
	if soulID == "" {
		return Soul{}, common.ErrInvalidArgument
	}
	if strings.TrimSpace(cmd.ID) != "" && strings.TrimSpace(cmd.ID) != soulID {
		return Soul{}, common.ErrInvalidArgument
	}
	cmd.ID = soulID
	if err := exports.Services.SoulService.UpdateSoul(ctx, soulmodule.UpdateSoulCommand{
		SoulID:               common.SoulID(cmd.ID),
		Name:                 cmd.Name,
		Description:          cmd.Description,
		BusinessDomains:      cloneStrings(cmd.BusinessDomains),
		SoulMD:               cmd.SoulMD,
		AllowedSkillRefs:     cloneStrings(cmd.AllowedSkillRefs),
		AllowedMCPServerRefs: cloneStrings(cmd.AllowedMCPServerRefs),
		AllowedToolKinds:     cloneStrings(cmd.AllowedToolKinds),
	}); err != nil {
		return Soul{}, err
	}
	item, err := exports.Repositories.Souls.Get(ctx, common.SoulID(cmd.ID))
	if err != nil {
		return Soul{}, err
	}
	return mapSoul(item), nil
}

func UpdateSoulWithBackend(ctx context.Context, backend string, dataRoot string, soulID string, cmd UpdateSoulCommand) (Soul, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Soul{}, err
	}
	return UpdateSoulWithSession(ctx, session, soulID, cmd)
}

func ReadSoulWithSession(ctx context.Context, session *RuntimeSession, soulID string) (ret Soul, err error) {
	exports, err := session.Exports()
	if err != nil {
		return Soul{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Repositories.Souls == nil {
		return Soul{}, common.ErrInvalidArgument
	}
	item, err := exports.Repositories.Souls.Get(ctx, common.SoulID(soulID))
	if err != nil {
		return Soul{}, err
	}
	return mapSoul(item), nil
}

func ReadSoulWithBackend(ctx context.Context, backend string, dataRoot string, soulID string) (Soul, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Soul{}, err
	}
	return ReadSoulWithSession(ctx, session, soulID)
}

func ListSoulsWithSession(ctx context.Context, session *RuntimeSession) (ret []Soul, err error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Repositories.Souls == nil {
		return nil, common.ErrInvalidArgument
	}
	items, err := exports.Repositories.Souls.List(ctx)
	if err != nil {
		return nil, err
	}
	ret = make([]Soul, 0, len(items))
	for _, item := range items {
		ret = append(ret, mapSoul(item))
	}
	return ret, nil
}

func ListSoulsWithBackend(ctx context.Context, backend string, dataRoot string) ([]Soul, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListSoulsWithSession(ctx, session)
}

func CreateSkillWithSession(ctx context.Context, session *RuntimeSession, cmd CreateSkillCommand) (ret Skill, err error) {
	exports, err := session.Exports()
	if err != nil {
		return Skill{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Services.SkillService == nil || exports.Repositories.Skills == nil {
		return Skill{}, common.ErrInvalidArgument
	}
	if err := exports.Services.SkillService.CreateSkill(ctx, skillmodule.CreateSkillCommand{
		SkillID:               common.SkillID(cmd.ID),
		Name:                  cmd.Name,
		Version:               cmd.Version,
		Description:           cmd.Description,
		TriggerRules:          cloneStrings(cmd.TriggerRules),
		Instruction:           cmd.Instruction,
		ReferenceRefs:         cloneStrings(cmd.ReferenceRefs),
		RequiredToolKinds:     cloneStrings(cmd.RequiredToolKinds),
		RequiredMCPServerRefs: cloneStrings(cmd.RequiredMCPServerRefs),
		Constraints:           cloneStrings(cmd.Constraints),
	}); err != nil {
		return Skill{}, err
	}
	item, err := exports.Repositories.Skills.Get(ctx, common.SkillID(cmd.ID))
	if err != nil {
		return Skill{}, err
	}
	return mapSkill(item), nil
}

func CreateSkillWithBackend(ctx context.Context, backend string, dataRoot string, cmd CreateSkillCommand) (Skill, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Skill{}, err
	}
	return CreateSkillWithSession(ctx, session, cmd)
}

func ReadSkillWithSession(ctx context.Context, session *RuntimeSession, skillID string, query ...SkillQuery) (ret Skill, err error) {
	exports, err := session.Exports()
	if err != nil {
		return Skill{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Repositories.Skills == nil {
		return Skill{}, common.ErrInvalidArgument
	}
	item, err := exports.Repositories.Skills.Get(ctx, common.SkillID(skillID))
	if err != nil {
		if !errors.Is(err, common.ErrNotFound) {
			return Skill{}, err
		}
		return readSkillHubSkill(ctx, session, skillID, firstSkillQuery(query))
	}
	return mapSkill(item), nil
}

func ReadSkillWithBackend(ctx context.Context, backend string, dataRoot string, skillID string, query ...SkillQuery) (Skill, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Skill{}, err
	}
	return ReadSkillWithSession(ctx, session, skillID, query...)
}

func ListSkillsWithSession(ctx context.Context, session *RuntimeSession, query ...SkillQuery) (ret []Skill, err error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Repositories.Skills == nil {
		return nil, common.ErrInvalidArgument
	}
	items, err := exports.Repositories.Skills.List(ctx)
	if err != nil {
		return nil, err
	}
	ret = make([]Skill, 0, len(items))
	for _, item := range items {
		ret = append(ret, mapSkill(item))
	}
	ret = append(ret, listSkillHubSkillsForQuery(ctx, session, firstSkillQuery(query))...)
	return ret, nil
}

func ListSkillsWithBackend(ctx context.Context, backend string, dataRoot string, query ...SkillQuery) ([]Skill, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListSkillsWithSession(ctx, session, query...)
}

func firstSkillQuery(query []SkillQuery) SkillQuery {
	if len(query) == 0 {
		return SkillQuery{}
	}
	return query[0]
}

func listSkillHubSkillsForQuery(ctx context.Context, session *RuntimeSession, query SkillQuery) []Skill {
	ret, err := listSkillHubGlobalSkills(ctx)
	if err != nil {
		ret = nil
	}
	workspaceID := strings.TrimSpace(query.WorkspaceID)
	if workspaceID == "" {
		return ret
	}
	workspaceSkills, err := listSkillHubWorkspaceSkillsForWorkspace(ctx, session, workspaceID)
	if err != nil {
		return ret
	}
	return append(ret, workspaceSkills...)
}

func listSkillHubWorkspaceSkillsForWorkspace(ctx context.Context, session *RuntimeSession, workspaceID string) ([]Skill, error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	if exports.Repositories.Workspaces == nil {
		return nil, common.ErrInvalidArgument
	}
	workspace, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(strings.TrimSpace(workspaceID)))
	if err != nil {
		return nil, err
	}
	return listSkillHubWorkspaceSkills(ctx, string(workspace.ID), workspace.WorkingDirectory)
}

func readSkillHubSkill(ctx context.Context, session *RuntimeSession, skillID string, query SkillQuery) (Skill, error) {
	scope, refWorkspaceID, sourceSkillID, isSkillHubRef := parseSkillHubSkillRef(skillID)
	if isSkillHubRef {
		switch scope {
		case skillScopeGlobal:
			items, err := listSkillHubGlobalSkills(ctx)
			if err != nil {
				return Skill{}, err
			}
			return findSkillByRef(items, skillID, sourceSkillID)
		case skillScopeWorkspace:
			workspaceID := firstNonEmpty(refWorkspaceID, query.WorkspaceID)
			if strings.TrimSpace(workspaceID) == "" {
				return Skill{}, common.ErrInvalidArgument
			}
			items, err := listSkillHubWorkspaceSkillsForWorkspace(ctx, session, workspaceID)
			if err != nil {
				return Skill{}, err
			}
			return findSkillByRef(items, skillID, sourceSkillID)
		}
		return Skill{}, common.ErrNotFound
	}

	if strings.TrimSpace(query.WorkspaceID) != "" {
		items, err := listSkillHubWorkspaceSkillsForWorkspace(ctx, session, query.WorkspaceID)
		if err == nil {
			if item, err := findSkillByRef(items, skillID, skillID); err == nil {
				return item, nil
			}
		}
	}
	items, err := listSkillHubGlobalSkills(ctx)
	if err == nil {
		if item, err := findSkillByRef(items, skillID, skillID); err == nil {
			return item, nil
		}
	}
	return Skill{}, common.ErrNotFound
}

func findSkillByRef(items []Skill, id string, sourceSkillID string) (Skill, error) {
	id = strings.TrimSpace(id)
	sourceSkillID = strings.TrimSpace(sourceSkillID)
	for _, item := range items {
		if item.ID == id || (sourceSkillID != "" && item.SourceSkillID == sourceSkillID) {
			return item, nil
		}
	}
	return Skill{}, common.ErrNotFound
}

func CreateMCPServerWithSession(ctx context.Context, session *RuntimeSession, cmd CreateMCPServerCommand) (ret MCPServer, err error) {
	exports, err := session.Exports()
	if err != nil {
		return MCPServer{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Services.MCPService == nil || exports.Repositories.MCPServers == nil {
		return MCPServer{}, common.ErrInvalidArgument
	}
	if err := exports.Services.MCPService.CreateServer(ctx, mcpmodule.CreateServerCommand{
		ServerID:              common.MCPServerID(cmd.ID),
		Name:                  cmd.Name,
		Transport:             cmd.Transport,
		Endpoint:              cmd.Endpoint,
		Enabled:               cmd.Enabled,
		AllowedTools:          cloneStrings(cmd.AllowedTools),
		AllowedResources:      cloneStrings(cmd.AllowedResources),
		AllowedPrompts:        cloneStrings(cmd.AllowedPrompts),
		DefaultApprovalPolicy: cmd.DefaultApprovalPolicy,
		SessionPolicy:         cmd.SessionPolicy,
		TimeoutMillis:         cmd.TimeoutMillis,
	}); err != nil {
		return MCPServer{}, err
	}
	item, err := exports.Repositories.MCPServers.Get(ctx, common.MCPServerID(cmd.ID))
	if err != nil {
		return MCPServer{}, err
	}
	return mapMCPServer(item), nil
}

func CreateMCPServerWithBackend(ctx context.Context, backend string, dataRoot string, cmd CreateMCPServerCommand) (MCPServer, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return MCPServer{}, err
	}
	return CreateMCPServerWithSession(ctx, session, cmd)
}

func ReadMCPServerWithSession(ctx context.Context, session *RuntimeSession, serverID string) (ret MCPServer, err error) {
	exports, err := session.Exports()
	if err != nil {
		return MCPServer{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Repositories.MCPServers == nil {
		return MCPServer{}, common.ErrInvalidArgument
	}
	item, err := exports.Repositories.MCPServers.Get(ctx, common.MCPServerID(serverID))
	if err != nil {
		return MCPServer{}, err
	}
	return mapMCPServer(item), nil
}

func ReadMCPServerWithBackend(ctx context.Context, backend string, dataRoot string, serverID string) (MCPServer, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return MCPServer{}, err
	}
	return ReadMCPServerWithSession(ctx, session, serverID)
}

func ListMCPServersWithSession(ctx context.Context, session *RuntimeSession) (ret []MCPServer, err error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Repositories.MCPServers == nil {
		return nil, common.ErrInvalidArgument
	}
	items, err := exports.Repositories.MCPServers.List(ctx)
	if err != nil {
		return nil, err
	}
	ret = make([]MCPServer, 0, len(items))
	for _, item := range items {
		ret = append(ret, mapMCPServer(item))
	}
	return ret, nil
}

func ListMCPServersWithBackend(ctx context.Context, backend string, dataRoot string) ([]MCPServer, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListMCPServersWithSession(ctx, session)
}

func ListMCPServerToolsWithSession(ctx context.Context, session *RuntimeSession, serverID string) (ret []MCPTool, err error) {
	server, err := ReadMCPServerWithSession(ctx, session, serverID)
	if err != nil {
		return nil, err
	}
	ret = make([]MCPTool, 0, len(server.AllowedTools))
	for _, tool := range server.AllowedTools {
		ret = append(ret, MCPTool{ServerID: server.ID, Name: tool})
	}
	return ret, nil
}

func ListMCPServerToolsWithBackend(ctx context.Context, backend string, dataRoot string, serverID string) ([]MCPTool, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListMCPServerToolsWithSession(ctx, session, serverID)
}

func ListMCPServerResourcesWithSession(ctx context.Context, session *RuntimeSession, serverID string) (ret []MCPResource, err error) {
	server, err := ReadMCPServerWithSession(ctx, session, serverID)
	if err != nil {
		return nil, err
	}
	ret = make([]MCPResource, 0, len(server.AllowedResources))
	for _, resource := range server.AllowedResources {
		ret = append(ret, MCPResource{ServerID: server.ID, URI: resource})
	}
	return ret, nil
}

func ListMCPServerResourcesWithBackend(ctx context.Context, backend string, dataRoot string, serverID string) ([]MCPResource, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListMCPServerResourcesWithSession(ctx, session, serverID)
}

func ListMCPServerPromptsWithSession(ctx context.Context, session *RuntimeSession, serverID string) (ret []MCPPrompt, err error) {
	server, err := ReadMCPServerWithSession(ctx, session, serverID)
	if err != nil {
		return nil, err
	}
	ret = make([]MCPPrompt, 0, len(server.AllowedPrompts))
	for _, prompt := range server.AllowedPrompts {
		ret = append(ret, MCPPrompt{ServerID: server.ID, Name: prompt})
	}
	return ret, nil
}

func ListMCPServerPromptsWithBackend(ctx context.Context, backend string, dataRoot string, serverID string) ([]MCPPrompt, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListMCPServerPromptsWithSession(ctx, session, serverID)
}

func CreateMemoryItemWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, cmd CreateMemoryItemCommand) (ret MemoryItem, err error) {
	exports, err := session.Exports()
	if err != nil {
		return MemoryItem{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Services.MemoryService == nil || exports.Repositories.MemoryItems == nil || exports.Repositories.Workspaces == nil {
		return MemoryItem{}, common.ErrInvalidArgument
	}
	if _, err := exports.Repositories.Workspaces.Get(ctx, common.WorkspaceID(workspaceID)); err != nil {
		return MemoryItem{}, err
	}
	memoryID := cmd.MemoryID
	if memoryID == "" {
		memoryID = generateContextID("memory")
	}
	if err := exports.Services.MemoryService.CreateMemoryItem(ctx, memorymodule.CreateMemoryItemCommand{
		MemoryID:            common.MemoryID(memoryID),
		WorkspaceID:         common.WorkspaceID(workspaceID),
		RunID:               common.RunID(cmd.RunID),
		SourceType:          cmd.SourceType,
		SourceRef:           cmd.SourceRef,
		MemoryType:          cmd.MemoryType,
		Content:             cmd.Content,
		Summary:             cmd.Summary,
		Tags:                cloneStrings(cmd.Tags),
		Importance:          cmd.Importance,
		Confidence:          cmd.Confidence,
		Visibility:          cmd.Visibility,
		EmbeddingProviderID: cmd.EmbeddingProviderID,
		EmbeddingModel:      cmd.EmbeddingModel,
		EmbeddingText:       cmd.EmbeddingText,
		Vector:              cloneFloats(cmd.Vector),
	}); err != nil {
		return MemoryItem{}, err
	}
	item, err := exports.Repositories.MemoryItems.Get(ctx, common.MemoryID(memoryID))
	if err != nil {
		return MemoryItem{}, err
	}
	return mapMemoryItem(item), nil
}

func CreateMemoryItemWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, cmd CreateMemoryItemCommand) (MemoryItem, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return MemoryItem{}, err
	}
	return CreateMemoryItemWithSession(ctx, session, workspaceID, cmd)
}

func ListMemoryByWorkspaceWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, query string) (ret []MemoryItem, err error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Repositories.MemoryItems == nil {
		return nil, common.ErrInvalidArgument
	}
	var items []*memorymodel.MemoryItem
	if query != "" {
		items, err = exports.Repositories.MemoryItems.Search(ctx, repository.MemorySearchQuery{
			WorkspaceID: common.WorkspaceID(workspaceID),
			Query:       query,
		})
	} else {
		items, err = exports.Repositories.MemoryItems.ListByWorkspaceID(ctx, common.WorkspaceID(workspaceID))
	}
	if err != nil {
		return nil, err
	}
	ret = make([]MemoryItem, 0, len(items))
	for _, item := range items {
		ret = append(ret, mapMemoryItem(item))
	}
	return ret, nil
}

func ListMemoryByWorkspaceWithBackend(ctx context.Context, backend string, dataRoot string, workspaceID string, query string) ([]MemoryItem, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListMemoryByWorkspaceWithSession(ctx, session, workspaceID, query)
}

func mapSoul(item *soulmodel.Soul) Soul {
	if item == nil {
		return Soul{}
	}
	return Soul{
		ID:                   string(item.ID),
		Name:                 item.Name,
		Description:          item.Description,
		BusinessDomains:      cloneStrings(item.BusinessDomains),
		SoulMD:               item.SoulMD,
		AllowedSkillRefs:     cloneStrings(item.AllowedSkillRefs),
		AllowedMCPServerRefs: cloneStrings(item.AllowedMCPServerRefs),
		AllowedToolKinds:     cloneStrings(item.AllowedToolKinds),
	}
}

func mapSkill(item *skillmodel.Skill) Skill {
	if item == nil {
		return Skill{}
	}
	return Skill{
		ID:                    string(item.ID),
		Name:                  item.Name,
		Version:               item.Version,
		Description:           item.Description,
		TriggerRules:          cloneStrings(item.TriggerRules),
		Instruction:           item.Instruction,
		ReferenceRefs:         cloneStrings(item.ReferenceRefs),
		RequiredToolKinds:     cloneStrings(item.RequiredToolKinds),
		RequiredMCPServerRefs: cloneStrings(item.RequiredMCPServerRefs),
		Constraints:           cloneStrings(item.Constraints),
		Source:                skillSourceRepository,
		Scope:                 skillScopeRepository,
		ManagedBy:             skillManagedByWorkorch,
		SourceSkillID:         string(item.ID),
	}
}

func mapMCPServer(item *mcpmodel.Server) MCPServer {
	if item == nil {
		return MCPServer{}
	}
	return MCPServer{
		ID:                    string(item.ID),
		Name:                  item.Name,
		Transport:             item.Transport,
		Endpoint:              item.Endpoint,
		Enabled:               item.Enabled,
		AllowedTools:          cloneStrings(item.AllowedTools),
		AllowedResources:      cloneStrings(item.AllowedResources),
		AllowedPrompts:        cloneStrings(item.AllowedPrompts),
		DefaultApprovalPolicy: item.DefaultApprovalPolicy,
		SessionPolicy:         item.SessionPolicy,
		TimeoutMillis:         item.TimeoutMillis,
	}
}

func mapMemoryItem(item *memorymodel.MemoryItem) MemoryItem {
	if item == nil {
		return MemoryItem{}
	}
	return MemoryItem{
		ID:                  string(item.ID),
		WorkspaceID:         string(item.WorkspaceID),
		RunID:               string(item.RunID),
		SourceType:          item.SourceType,
		SourceRef:           item.SourceRef,
		MemoryType:          item.MemoryType,
		Content:             item.Content,
		Summary:             item.Summary,
		Tags:                cloneStrings(item.Tags),
		Importance:          item.Importance,
		Confidence:          item.Confidence,
		Visibility:          item.Visibility,
		EmbeddingProviderID: item.EmbeddingProviderID,
		EmbeddingModel:      item.EmbeddingModel,
		EmbeddingText:       item.EmbeddingText,
		VectorReady:         len(item.Vector) > 0,
		VectorDimension:     len(item.Vector),
		VectorScore:         item.VectorScore,
	}
}

func generateContextID(prefix string) string {
	id := string(generateWorkspaceID())
	if suffix, ok := strings.CutPrefix(id, "ws"); ok {
		return prefix + suffix
	}
	return prefix + "-" + id
}

func cloneStrings(input []string) []string {
	if input == nil {
		return nil
	}
	return append([]string(nil), input...)
}

func cloneFloats(input []float64) []float64 {
	if input == nil {
		return nil
	}
	return append([]float64(nil), input...)
}

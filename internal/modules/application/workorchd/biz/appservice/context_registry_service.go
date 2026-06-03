package appservice

import (
	"context"
	"errors"
	"strings"

	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	skillmodel "github.com/muidea/workorch/internal/modules/kernel/skill/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Soul struct {
	ID                          string   `json:"id"`
	Name                        string   `json:"name"`
	Description                 string   `json:"description,omitempty"`
	BusinessDomains             []string `json:"businessDomains,omitempty"`
	SoulMD                      string   `json:"soulMd"`
	AllowedSkillRefs            []string `json:"allowedSkillRefs,omitempty"`
	AllowedMCPServerRefs        []string `json:"allowedMCPServerRefs,omitempty"`
	AllowedCapabilityRefs       []string `json:"allowedCapabilityRefs,omitempty"`
	AllowedCapabilityCategories []string `json:"allowedCapabilityCategories,omitempty"`
	AllowedToolKinds            []string `json:"allowedToolKinds,omitempty"`
}

type Skill struct {
	ID                     string   `json:"id"`
	Name                   string   `json:"name"`
	Version                string   `json:"version"`
	Description            string   `json:"description,omitempty"`
	TriggerRules           []string `json:"triggerRules,omitempty"`
	Instruction            string   `json:"instruction"`
	ReferenceRefs          []string `json:"referenceRefs,omitempty"`
	RequiredToolKinds      []string `json:"requiredToolKinds,omitempty"`
	RequiredMCPServerRefs  []string `json:"requiredMCPServerRefs,omitempty"`
	RequiredCapabilityRefs []string `json:"requiredCapabilityRefs,omitempty"`
	Constraints            []string `json:"constraints,omitempty"`
	Source                 string   `json:"source,omitempty"`
	Scope                  string   `json:"scope,omitempty"`
	WorkspaceID            string   `json:"workspaceId,omitempty"`
	Status                 string   `json:"status,omitempty"`
	SourceRepository       string   `json:"sourceRepository,omitempty"`
	SourcePath             string   `json:"sourcePath,omitempty"`
	TargetPath             string   `json:"targetPath,omitempty"`
	LocalPath              string   `json:"localPath,omitempty"`
	RepoPath               string   `json:"repoPath,omitempty"`
	GlobalPath             string   `json:"globalPath,omitempty"`
	ProjectPath            string   `json:"projectPath,omitempty"`
	ContentHash            string   `json:"contentHash,omitempty"`
	SourceHash             string   `json:"sourceHash,omitempty"`
	AppliedHash            string   `json:"appliedHash,omitempty"`
	ActualHash             string   `json:"actualHash,omitempty"`
	Agent                  string   `json:"agent,omitempty"`
	ManagedBy              string   `json:"managedBy,omitempty"`
	SourceSkillID          string   `json:"sourceSkillId,omitempty"`
}

type MCPServer struct {
	ID                    string            `json:"id"`
	Name                  string            `json:"name"`
	Transport             string            `json:"transport"`
	Endpoint              string            `json:"endpoint,omitempty"`
	Command               string            `json:"command,omitempty"`
	Args                  []string          `json:"args,omitempty"`
	Env                   map[string]string `json:"env,omitempty"`
	Enabled               bool              `json:"enabled"`
	AllowedTools          []string          `json:"allowedTools,omitempty"`
	AllowedResources      []string          `json:"allowedResources,omitempty"`
	AllowedPrompts        []string          `json:"allowedPrompts,omitempty"`
	DefaultApprovalPolicy string            `json:"defaultApprovalPolicy,omitempty"`
	SessionPolicy         string            `json:"sessionPolicy,omitempty"`
	TimeoutMillis         int               `json:"timeoutMillis,omitempty"`
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

type MCPCapability struct {
	ID             string         `json:"id"`
	ServerID       string         `json:"serverId"`
	Kind           string         `json:"kind"`
	Name           string         `json:"name"`
	DisplayName    string         `json:"displayName"`
	Description    string         `json:"description,omitempty"`
	Category       string         `json:"category,omitempty"`
	Tags           []string       `json:"tags,omitempty"`
	UsageHint      string         `json:"usageHint,omitempty"`
	InputSummary   string         `json:"inputSummary,omitempty"`
	OutputSummary  string         `json:"outputSummary,omitempty"`
	StageHints     []string       `json:"stageHints,omitempty"`
	RiskLevel      string         `json:"riskLevel,omitempty"`
	Schema         map[string]any `json:"schema,omitempty"`
	URI            string         `json:"uri,omitempty"`
	Enabled        bool           `json:"enabled"`
	ApprovalPolicy string         `json:"approvalPolicy,omitempty"`
	Status         string         `json:"status,omitempty"`
}

type MCPCapabilityQuery struct {
	ServerID  string
	Kind      string
	Category  string
	Stage     string
	RiskLevel string
	Enabled   *bool
	Query     string
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
type UpdateMCPServerCommand = MCPServer
type CreateMCPCapabilityCommand = MCPCapability
type UpdateMCPCapabilityCommand = MCPCapability

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

type UpdateMemoryItemCommand = CreateMemoryItemCommand

type MemoryQuery struct {
	WorkspaceID string
	Query       string
	Tags        []string
	Types       []string
	Limit       int
}

func CreateSoulWithSession(ctx context.Context, session *RuntimeSession, cmd CreateSoulCommand) (ret Soul, err error) {
	exports, err := session.Exports()
	if err != nil {
		return Soul{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Infrastructure.EventHub == nil {
		return Soul{}, common.ErrInvalidArgument
	}
	response, err := publishContextRegistryCommandWithResponse(ctx, exports.Infrastructure.EventHub, events.TopicSoulCommandCreate, "soul", cmd.ID, map[string]any{
		events.PayloadKeySoul: soulModelFromCommand(cmd),
	})
	if err != nil {
		return Soul{}, err
	}
	item, ok := response.Payload[events.PayloadKeySoul].(*soulmodel.Soul)
	if !ok {
		return Soul{}, common.ErrInvalidArgument
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
	if exports.Infrastructure.EventHub == nil {
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
	response, err := publishContextRegistryCommandWithResponse(ctx, exports.Infrastructure.EventHub, events.TopicSoulCommandUpdate, "soul", cmd.ID, map[string]any{
		events.PayloadKeySoul: soulModelFromCommand(cmd),
	})
	if err != nil {
		return Soul{}, err
	}
	item, ok := response.Payload[events.PayloadKeySoul].(*soulmodel.Soul)
	if !ok {
		return Soul{}, common.ErrInvalidArgument
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

func DeleteSoulWithSession(ctx context.Context, session *RuntimeSession, soulID string) (err error) {
	exports, err := session.Exports()
	if err != nil {
		return err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Infrastructure.EventHub == nil {
		return common.ErrInvalidArgument
	}
	soulID = strings.TrimSpace(soulID)
	if soulID == "" {
		return common.ErrInvalidArgument
	}
	return publishContextRegistryCommand(ctx, exports.Infrastructure.EventHub, events.TopicSoulCommandDelete, "soul", soulID, map[string]any{
		events.PayloadKeySoulID: soulID,
	})
}

func DeleteSoulWithBackend(ctx context.Context, backend string, dataRoot string, soulID string) error {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return err
	}
	return DeleteSoulWithSession(ctx, session, soulID)
}

func ReadSoulWithSession(ctx context.Context, session *RuntimeSession, soulID string) (ret Soul, err error) {
	exports, err := session.Exports()
	if err != nil {
		return Soul{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Infrastructure.EventHub == nil {
		return Soul{}, common.ErrInvalidArgument
	}
	response, err := publishContextRegistryCommandWithResponse(ctx, exports.Infrastructure.EventHub, events.TopicSoulCommandGet, "soul", soulID, map[string]any{
		events.PayloadKeySoulID: soulID,
	})
	if err != nil {
		return Soul{}, err
	}
	item, ok := response.Payload[events.PayloadKeySoul].(*soulmodel.Soul)
	if !ok {
		return Soul{}, common.ErrInvalidArgument
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
	if exports.Infrastructure.EventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishContextRegistryCommandWithResponse(ctx, exports.Infrastructure.EventHub, events.TopicSoulCommandList, "soul", "all", nil)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeySouls].([]*soulmodel.Soul)
	if !ok {
		return nil, common.ErrInvalidArgument
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
	if exports.Infrastructure.EventHub == nil {
		return Skill{}, common.ErrInvalidArgument
	}
	response, err := publishContextRegistryCommandWithResponse(ctx, exports.Infrastructure.EventHub, events.TopicSkillCommandCreate, "skill", cmd.ID, map[string]any{
		events.PayloadKeySkill: skillModelFromCommand(cmd),
	})
	if err != nil {
		return Skill{}, err
	}
	item, ok := response.Payload[events.PayloadKeySkill].(*skillmodel.Skill)
	if !ok {
		return Skill{}, common.ErrInvalidArgument
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
	if exports.Infrastructure.EventHub == nil {
		return Skill{}, common.ErrInvalidArgument
	}
	response, err := publishContextRegistryCommandWithResponse(ctx, exports.Infrastructure.EventHub, events.TopicSkillCommandGet, "skill", skillID, map[string]any{
		events.PayloadKeySkillID: skillID,
	})
	if err != nil {
		if !errors.Is(err, common.ErrNotFound) {
			return Skill{}, err
		}
		return readSkillHubSkill(ctx, session, skillID, firstSkillQuery(query))
	}
	item, ok := response.Payload[events.PayloadKeySkill].(*skillmodel.Skill)
	if !ok {
		return Skill{}, common.ErrInvalidArgument
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
	if exports.Infrastructure.EventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishContextRegistryCommandWithResponse(ctx, exports.Infrastructure.EventHub, events.TopicSkillCommandList, "skill", "all", nil)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeySkills].([]*skillmodel.Skill)
	if !ok {
		return nil, common.ErrInvalidArgument
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
	if exports.Infrastructure.EventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	workspace, err := readWorkspaceByOwnerCommand(ctx, exports.Infrastructure.EventHub, common.WorkspaceID(strings.TrimSpace(workspaceID)))
	if err != nil {
		return nil, err
	}
	return listSkillHubWorkspaceSkills(ctx, string(workspace.ID), workspace.DeliverableDirectory)
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
	if exports.Infrastructure.EventHub == nil {
		return MCPServer{}, common.ErrInvalidArgument
	}
	response, err := publishContextRegistryCommandWithResponse(ctx, exports.Infrastructure.EventHub, events.TopicMCPCommandCreateServer, "mcp_server", cmd.ID, map[string]any{
		events.PayloadKeyServer: mcpServerModelFromCommand(cmd),
	})
	if err != nil {
		return MCPServer{}, err
	}
	item, ok := response.Payload[events.PayloadKeyServer].(*mcpmodel.Server)
	if !ok {
		return MCPServer{}, common.ErrInvalidArgument
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

func UpdateMCPServerWithSession(ctx context.Context, session *RuntimeSession, serverID string, cmd UpdateMCPServerCommand) (ret MCPServer, err error) {
	exports, err := session.Exports()
	if err != nil {
		return MCPServer{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Infrastructure.EventHub == nil {
		return MCPServer{}, common.ErrInvalidArgument
	}
	cmd.ID = strings.TrimSpace(cmd.ID)
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		return MCPServer{}, common.ErrInvalidArgument
	}
	if cmd.ID != "" && cmd.ID != serverID {
		return MCPServer{}, common.ErrInvalidArgument
	}
	cmd.ID = serverID
	env := cloneStringMap(cmd.Env)
	if cmd.Env == nil {
		existing, err := getMCPServerByCommand(ctx, exports.Infrastructure.EventHub, serverID)
		if err != nil {
			return MCPServer{}, err
		}
		env = cloneStringMap(existing.Env)
	}
	response, err := publishContextRegistryCommandWithResponse(ctx, exports.Infrastructure.EventHub, events.TopicMCPCommandUpdateServer, "mcp_server", cmd.ID, map[string]any{
		events.PayloadKeyServer: mcpServerModelFromCommand(UpdateMCPServerCommand{
			ID:                    cmd.ID,
			Name:                  cmd.Name,
			Transport:             cmd.Transport,
			Endpoint:              cmd.Endpoint,
			Command:               cmd.Command,
			Args:                  cloneStrings(cmd.Args),
			Env:                   env,
			Enabled:               cmd.Enabled,
			AllowedTools:          cloneStrings(cmd.AllowedTools),
			AllowedResources:      cloneStrings(cmd.AllowedResources),
			AllowedPrompts:        cloneStrings(cmd.AllowedPrompts),
			DefaultApprovalPolicy: cmd.DefaultApprovalPolicy,
			SessionPolicy:         cmd.SessionPolicy,
			TimeoutMillis:         cmd.TimeoutMillis,
		}),
	})
	if err != nil {
		return MCPServer{}, err
	}
	item, ok := response.Payload[events.PayloadKeyServer].(*mcpmodel.Server)
	if !ok {
		return MCPServer{}, common.ErrInvalidArgument
	}
	return mapMCPServer(item), nil
}

func UpdateMCPServerWithBackend(ctx context.Context, backend string, dataRoot string, serverID string, cmd UpdateMCPServerCommand) (MCPServer, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return MCPServer{}, err
	}
	return UpdateMCPServerWithSession(ctx, session, serverID, cmd)
}

func ReadMCPServerWithSession(ctx context.Context, session *RuntimeSession, serverID string) (ret MCPServer, err error) {
	exports, err := session.Exports()
	if err != nil {
		return MCPServer{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Infrastructure.EventHub == nil {
		return MCPServer{}, common.ErrInvalidArgument
	}
	item, err := getMCPServerByCommand(ctx, exports.Infrastructure.EventHub, serverID)
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
	if exports.Infrastructure.EventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishContextRegistryCommandWithResponse(ctx, exports.Infrastructure.EventHub, events.TopicMCPCommandListServers, "mcp_server", "all", nil)
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyServers].([]*mcpmodel.Server)
	if !ok {
		return nil, common.ErrInvalidArgument
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

func CreateMCPCapabilityWithSession(ctx context.Context, session *RuntimeSession, cmd CreateMCPCapabilityCommand) (ret MCPCapability, err error) {
	exports, err := session.Exports()
	if err != nil {
		return MCPCapability{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Infrastructure.EventHub == nil {
		return MCPCapability{}, common.ErrInvalidArgument
	}
	response, err := publishContextRegistryCommandWithResponse(ctx, exports.Infrastructure.EventHub, events.TopicMCPCommandCreateCapability, "mcp_capability", cmd.ID, map[string]any{
		events.PayloadKeyCapability: mcpCapabilityModelFromCommand(cmd),
	})
	if err != nil {
		return MCPCapability{}, err
	}
	item, ok := response.Payload[events.PayloadKeyCapability].(*mcpmodel.Capability)
	if !ok {
		return MCPCapability{}, common.ErrInvalidArgument
	}
	return mapMCPCapability(item), nil
}

func CreateMCPCapabilityWithBackend(ctx context.Context, backend string, dataRoot string, cmd CreateMCPCapabilityCommand) (MCPCapability, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return MCPCapability{}, err
	}
	return CreateMCPCapabilityWithSession(ctx, session, cmd)
}

func UpdateMCPCapabilityWithSession(ctx context.Context, session *RuntimeSession, capabilityID string, cmd UpdateMCPCapabilityCommand) (ret MCPCapability, err error) {
	exports, err := session.Exports()
	if err != nil {
		return MCPCapability{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Infrastructure.EventHub == nil {
		return MCPCapability{}, common.ErrInvalidArgument
	}
	cmd.ID = capabilityID
	response, err := publishContextRegistryCommandWithResponse(ctx, exports.Infrastructure.EventHub, events.TopicMCPCommandUpdateCapability, "mcp_capability", cmd.ID, map[string]any{
		events.PayloadKeyCapability: mcpCapabilityModelFromCommand(cmd),
	})
	if err != nil {
		return MCPCapability{}, err
	}
	item, ok := response.Payload[events.PayloadKeyCapability].(*mcpmodel.Capability)
	if !ok {
		return MCPCapability{}, common.ErrInvalidArgument
	}
	return mapMCPCapability(item), nil
}

func UpdateMCPCapabilityWithBackend(ctx context.Context, backend string, dataRoot string, capabilityID string, cmd UpdateMCPCapabilityCommand) (MCPCapability, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return MCPCapability{}, err
	}
	return UpdateMCPCapabilityWithSession(ctx, session, capabilityID, cmd)
}

func ReadMCPCapabilityWithSession(ctx context.Context, session *RuntimeSession, capabilityID string) (ret MCPCapability, err error) {
	exports, err := session.Exports()
	if err != nil {
		return MCPCapability{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Infrastructure.EventHub == nil {
		return MCPCapability{}, common.ErrInvalidArgument
	}
	response, err := publishContextRegistryCommandWithResponse(ctx, exports.Infrastructure.EventHub, events.TopicMCPCommandGetCapability, "mcp_capability", capabilityID, map[string]any{
		events.PayloadKeyCapabilityID: capabilityID,
	})
	if err != nil {
		return MCPCapability{}, err
	}
	item, ok := response.Payload[events.PayloadKeyCapability].(*mcpmodel.Capability)
	if !ok {
		return MCPCapability{}, common.ErrInvalidArgument
	}
	return mapMCPCapability(item), nil
}

func ReadMCPCapabilityWithBackend(ctx context.Context, backend string, dataRoot string, capabilityID string) (MCPCapability, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return MCPCapability{}, err
	}
	return ReadMCPCapabilityWithSession(ctx, session, capabilityID)
}

func ListMCPCapabilitiesWithSession(ctx context.Context, session *RuntimeSession, query MCPCapabilityQuery) (ret []MCPCapability, err error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Infrastructure.EventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := publishContextRegistryCommandWithResponse(ctx, exports.Infrastructure.EventHub, events.TopicMCPCommandListCapabilities, "mcp_capability", "all", map[string]any{
		events.PayloadKeyCapabilityQuery: mcpCapabilityQueryPayload(query),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyCapabilities].([]*mcpmodel.Capability)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	ret = make([]MCPCapability, 0, len(items))
	for _, item := range items {
		ret = append(ret, mapMCPCapability(item))
	}
	return ret, nil
}

func ListMCPCapabilitiesWithBackend(ctx context.Context, backend string, dataRoot string, query MCPCapabilityQuery) ([]MCPCapability, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListMCPCapabilitiesWithSession(ctx, session, query)
}

func CreateMemoryItemWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, cmd CreateMemoryItemCommand) (ret MemoryItem, err error) {
	exports, err := session.Exports()
	if err != nil {
		return MemoryItem{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Infrastructure.EventHub == nil {
		return MemoryItem{}, common.ErrInvalidArgument
	}
	if _, err := publishContextRegistryCommandWithResponse(ctx, exports.Infrastructure.EventHub, events.TopicWorkspaceCommandGet, "workspace", workspaceID, map[string]any{
		events.PayloadKeyWorkspaceID: workspaceID,
	}); err != nil {
		return MemoryItem{}, err
	}
	memoryID := cmd.MemoryID
	if memoryID == "" {
		memoryID = generateContextID("memory")
	}
	if err := publishMemoryItemCommand(ctx, exports.Infrastructure.EventHub, events.TopicMemoryCommandCreate, memoryItemFromCommand(memoryID, workspaceID, cmd)); err != nil {
		return MemoryItem{}, err
	}
	item, err := getMemoryItemByCommand(ctx, exports.Infrastructure.EventHub, memoryID)
	if err != nil {
		return MemoryItem{}, err
	}
	if err := publishMemoryEvent(ctx, events.TopicMemoryCreated, item); err != nil {
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

func ReadMemoryItemWithSession(ctx context.Context, session *RuntimeSession, memoryID string) (ret MemoryItem, err error) {
	exports, err := session.Exports()
	if err != nil {
		return MemoryItem{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Infrastructure.EventHub == nil {
		return MemoryItem{}, common.ErrInvalidArgument
	}
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return MemoryItem{}, common.ErrInvalidArgument
	}
	item, err := getMemoryItemByCommand(ctx, exports.Infrastructure.EventHub, memoryID)
	if err != nil {
		return MemoryItem{}, err
	}
	return mapMemoryItem(item), nil
}

func ReadMemoryItemWithBackend(ctx context.Context, backend string, dataRoot string, memoryID string) (MemoryItem, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return MemoryItem{}, err
	}
	return ReadMemoryItemWithSession(ctx, session, memoryID)
}

func UpdateMemoryItemWithSession(ctx context.Context, session *RuntimeSession, memoryID string, cmd UpdateMemoryItemCommand) (ret MemoryItem, err error) {
	exports, err := session.Exports()
	if err != nil {
		return MemoryItem{}, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Infrastructure.EventHub == nil {
		return MemoryItem{}, common.ErrInvalidArgument
	}
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return MemoryItem{}, common.ErrInvalidArgument
	}
	if strings.TrimSpace(cmd.MemoryID) != "" && strings.TrimSpace(cmd.MemoryID) != memoryID {
		return MemoryItem{}, common.ErrInvalidArgument
	}
	existing, err := getMemoryItemByCommand(ctx, exports.Infrastructure.EventHub, memoryID)
	if err != nil {
		return MemoryItem{}, err
	}
	merged := mergeMemoryUpdateCommand(existing, cmd)
	if err := publishMemoryItemCommand(ctx, exports.Infrastructure.EventHub, events.TopicMemoryCommandUpdate, memoryItemFromMergedCommand(memoryID, existing.WorkspaceID, merged)); err != nil {
		return MemoryItem{}, err
	}
	item, err := getMemoryItemByCommand(ctx, exports.Infrastructure.EventHub, memoryID)
	if err != nil {
		return MemoryItem{}, err
	}
	if err := publishMemoryEvent(ctx, events.TopicMemoryUpdated, item); err != nil {
		return MemoryItem{}, err
	}
	return mapMemoryItem(item), nil
}

func UpdateMemoryItemWithBackend(ctx context.Context, backend string, dataRoot string, memoryID string, cmd UpdateMemoryItemCommand) (MemoryItem, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return MemoryItem{}, err
	}
	return UpdateMemoryItemWithSession(ctx, session, memoryID, cmd)
}

func DeleteMemoryItemWithSession(ctx context.Context, session *RuntimeSession, memoryID string) (err error) {
	exports, err := session.Exports()
	if err != nil {
		return err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Infrastructure.EventHub == nil {
		return common.ErrInvalidArgument
	}
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return common.ErrInvalidArgument
	}
	item, err := getMemoryItemByCommand(ctx, exports.Infrastructure.EventHub, memoryID)
	if err != nil {
		return err
	}
	if err := publishContextRegistryCommand(ctx, exports.Infrastructure.EventHub, events.TopicMemoryCommandDelete, "memory", memoryID, map[string]any{
		events.PayloadKeyMemoryID: memoryID,
	}); err != nil {
		return err
	}
	return publishMemoryEvent(ctx, events.TopicMemoryDeleted, item)
}

func DeleteMemoryItemWithBackend(ctx context.Context, backend string, dataRoot string, memoryID string) error {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return err
	}
	return DeleteMemoryItemWithSession(ctx, session, memoryID)
}

func ListMemoryByWorkspaceWithSession(ctx context.Context, session *RuntimeSession, workspaceID string, query string) (ret []MemoryItem, err error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Infrastructure.EventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	var items []*memorymodel.MemoryItem
	if query != "" {
		items, err = searchMemoryByCommand(ctx, exports.Infrastructure.EventHub, repository.MemorySearchQuery{
			WorkspaceID: common.WorkspaceID(workspaceID),
			Query:       query,
		})
	} else {
		response, responseErr := publishContextRegistryCommandWithResponse(ctx, exports.Infrastructure.EventHub, events.TopicMemoryCommandListByWorkspace, "workspace", workspaceID, map[string]any{
			events.PayloadKeyWorkspaceID: workspaceID,
		})
		if responseErr != nil {
			err = responseErr
		} else {
			var ok bool
			items, ok = response.Payload[events.PayloadKeyMemoryItems].([]*memorymodel.MemoryItem)
			if !ok {
				err = common.ErrInvalidArgument
			}
		}
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

func SearchMemoryWithSession(ctx context.Context, session *RuntimeSession, query MemoryQuery) (ret []MemoryItem, err error) {
	exports, err := session.Exports()
	if err != nil {
		return nil, err
	}
	defer session.CloseOnReturn(ctx, &err)
	if exports.Infrastructure.EventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	if strings.TrimSpace(query.WorkspaceID) == "" {
		return nil, common.ErrInvalidArgument
	}
	items, err := searchMemoryByCommand(ctx, exports.Infrastructure.EventHub, repository.MemorySearchQuery{
		WorkspaceID: common.WorkspaceID(strings.TrimSpace(query.WorkspaceID)),
		Query:       strings.TrimSpace(query.Query),
		Tags:        cloneStrings(query.Tags),
		Types:       cloneStrings(query.Types),
		Limit:       query.Limit,
	})
	if err != nil {
		return nil, err
	}
	ret = make([]MemoryItem, 0, len(items))
	for _, item := range items {
		ret = append(ret, mapMemoryItem(item))
	}
	return ret, nil
}

func SearchMemoryWithBackend(ctx context.Context, backend string, dataRoot string, query MemoryQuery) ([]MemoryItem, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return SearchMemoryWithSession(ctx, session, query)
}

func mergeMemoryUpdateCommand(existing *memorymodel.MemoryItem, cmd UpdateMemoryItemCommand) UpdateMemoryItemCommand {
	if existing == nil {
		return cmd
	}
	ret := UpdateMemoryItemCommand{
		MemoryID:            string(existing.ID),
		RunID:               string(existing.RunID),
		SourceType:          existing.SourceType,
		SourceRef:           existing.SourceRef,
		MemoryType:          existing.MemoryType,
		Content:             existing.Content,
		Summary:             existing.Summary,
		Tags:                cloneStrings(existing.Tags),
		Importance:          existing.Importance,
		Confidence:          existing.Confidence,
		Visibility:          existing.Visibility,
		EmbeddingProviderID: existing.EmbeddingProviderID,
		EmbeddingModel:      existing.EmbeddingModel,
		EmbeddingText:       existing.EmbeddingText,
		Vector:              cloneFloats(existing.Vector),
	}
	if strings.TrimSpace(cmd.RunID) != "" {
		ret.RunID = strings.TrimSpace(cmd.RunID)
	}
	if strings.TrimSpace(cmd.SourceType) != "" {
		ret.SourceType = strings.TrimSpace(cmd.SourceType)
	}
	if strings.TrimSpace(cmd.SourceRef) != "" {
		ret.SourceRef = strings.TrimSpace(cmd.SourceRef)
	}
	if strings.TrimSpace(cmd.MemoryType) != "" {
		ret.MemoryType = strings.TrimSpace(cmd.MemoryType)
	}
	if cmd.Content != "" {
		ret.Content = cmd.Content
	}
	if strings.TrimSpace(cmd.Summary) != "" {
		ret.Summary = strings.TrimSpace(cmd.Summary)
	}
	if cmd.Tags != nil {
		ret.Tags = cloneStrings(cmd.Tags)
	}
	if cmd.Importance != 0 {
		ret.Importance = cmd.Importance
	}
	if cmd.Confidence != 0 {
		ret.Confidence = cmd.Confidence
	}
	if strings.TrimSpace(cmd.Visibility) != "" {
		ret.Visibility = strings.TrimSpace(cmd.Visibility)
	}
	if strings.TrimSpace(cmd.EmbeddingProviderID) != "" {
		ret.EmbeddingProviderID = strings.TrimSpace(cmd.EmbeddingProviderID)
	}
	if strings.TrimSpace(cmd.EmbeddingModel) != "" {
		ret.EmbeddingModel = strings.TrimSpace(cmd.EmbeddingModel)
	}
	if strings.TrimSpace(cmd.EmbeddingText) != "" {
		ret.EmbeddingText = strings.TrimSpace(cmd.EmbeddingText)
	}
	if cmd.Vector != nil {
		ret.Vector = cloneFloats(cmd.Vector)
	}
	return ret
}

func publishMemoryEvent(ctx context.Context, topic string, item *memorymodel.MemoryItem) error {
	eventHub := EventHubFromContext(ctx)
	if eventHub == nil || item == nil {
		return nil
	}
	return eventHub.Publish(ctx, events.Envelope{
		Version:      events.WorkorchEventVersion,
		Topic:        topic,
		SourceModule: "workorchd/appservice",
		SubjectType:  "memory",
		SubjectID:    string(item.ID),
		WorkspaceID:  string(item.WorkspaceID),
		RunID:        string(item.RunID),
		Payload:      events.MemoryPayload(item.MemoryType, item.Summary, item.Content, item.SourceType, item.SourceRef, item.Tags, item.Importance, item.Confidence, item.Visibility),
	})
}

func mapSoul(item *soulmodel.Soul) Soul {
	if item == nil {
		return Soul{}
	}
	return Soul{
		ID:                          string(item.ID),
		Name:                        item.Name,
		Description:                 item.Description,
		BusinessDomains:             cloneStrings(item.BusinessDomains),
		SoulMD:                      item.SoulMD,
		AllowedSkillRefs:            cloneStrings(item.AllowedSkillRefs),
		AllowedMCPServerRefs:        cloneStrings(item.AllowedMCPServerRefs),
		AllowedCapabilityRefs:       cloneStrings(item.AllowedCapabilityRefs),
		AllowedCapabilityCategories: cloneStrings(item.AllowedCapabilityCategories),
		AllowedToolKinds:            cloneStrings(item.AllowedToolKinds),
	}
}

func mapSkill(item *skillmodel.Skill) Skill {
	if item == nil {
		return Skill{}
	}
	return Skill{
		ID:                     string(item.ID),
		Name:                   item.Name,
		Version:                item.Version,
		Description:            item.Description,
		TriggerRules:           cloneStrings(item.TriggerRules),
		Instruction:            item.Instruction,
		ReferenceRefs:          cloneStrings(item.ReferenceRefs),
		RequiredToolKinds:      cloneStrings(item.RequiredToolKinds),
		RequiredMCPServerRefs:  cloneStrings(item.RequiredMCPServerRefs),
		RequiredCapabilityRefs: cloneStrings(item.RequiredCapabilityRefs),
		Constraints:            cloneStrings(item.Constraints),
		Source:                 skillSourceRepository,
		Scope:                  skillScopeRepository,
		ManagedBy:              skillManagedByWorkorch,
		SourceSkillID:          string(item.ID),
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
		Command:               item.Command,
		Args:                  cloneStrings(item.Args),
		Env:                   redactStringMap(item.Env),
		Enabled:               item.Enabled,
		AllowedTools:          cloneStrings(item.AllowedTools),
		AllowedResources:      cloneStrings(item.AllowedResources),
		AllowedPrompts:        cloneStrings(item.AllowedPrompts),
		DefaultApprovalPolicy: item.DefaultApprovalPolicy,
		SessionPolicy:         item.SessionPolicy,
		TimeoutMillis:         item.TimeoutMillis,
	}
}

func mapMCPCapability(item *mcpmodel.Capability) MCPCapability {
	if item == nil {
		return MCPCapability{}
	}
	return MCPCapability{
		ID:             string(item.ID),
		ServerID:       string(item.ServerID),
		Kind:           item.Kind,
		Name:           item.Name,
		DisplayName:    item.DisplayName,
		Description:    item.Description,
		Category:       item.Category,
		Tags:           cloneStrings(item.Tags),
		UsageHint:      item.UsageHint,
		InputSummary:   item.InputSummary,
		OutputSummary:  item.OutputSummary,
		StageHints:     cloneStrings(item.StageHints),
		RiskLevel:      item.RiskLevel,
		Schema:         cloneMap(item.Schema),
		URI:            item.URI,
		Enabled:        item.Enabled,
		ApprovalPolicy: item.ApprovalPolicy,
		Status:         item.Status,
	}
}

func getMCPServerByCommand(ctx context.Context, hub events.Hub, serverID string) (*mcpmodel.Server, error) {
	response, err := publishContextRegistryCommandWithResponse(ctx, hub, events.TopicMCPCommandGetServer, "mcp_server", serverID, map[string]any{
		events.PayloadKeyServerID: serverID,
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyServer].(*mcpmodel.Server)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func publishContextRegistryCommand(ctx context.Context, hub events.Hub, topic string, subjectType string, subjectID string, payload map[string]any) error {
	_, err := publishContextRegistryCommandWithResponse(ctx, hub, topic, subjectType, subjectID, payload)
	return err
}

func publishContextRegistryCommandWithResponse(ctx context.Context, hub events.Hub, topic string, subjectType string, subjectID string, payload map[string]any) (events.Envelope, error) {
	if hub == nil || topic == "" {
		return events.Envelope{}, common.ErrInvalidArgument
	}
	err := hub.Publish(ctx, events.Envelope{
		Topic:        topic,
		SourceModule: "application/workorchd/context-registry",
		SubjectType:  subjectType,
		SubjectID:    subjectID,
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

func soulModelFromCommand(cmd Soul) *soulmodel.Soul {
	return &soulmodel.Soul{
		ID:                          common.SoulID(cmd.ID),
		Name:                        cmd.Name,
		Description:                 cmd.Description,
		BusinessDomains:             cloneStrings(cmd.BusinessDomains),
		SoulMD:                      cmd.SoulMD,
		AllowedSkillRefs:            cloneStrings(cmd.AllowedSkillRefs),
		AllowedMCPServerRefs:        cloneStrings(cmd.AllowedMCPServerRefs),
		AllowedCapabilityRefs:       cloneStrings(cmd.AllowedCapabilityRefs),
		AllowedCapabilityCategories: cloneStrings(cmd.AllowedCapabilityCategories),
		AllowedToolKinds:            cloneStrings(cmd.AllowedToolKinds),
	}
}

func skillModelFromCommand(cmd Skill) *skillmodel.Skill {
	return &skillmodel.Skill{
		ID:                     common.SkillID(cmd.ID),
		Name:                   cmd.Name,
		Version:                cmd.Version,
		Description:            cmd.Description,
		TriggerRules:           cloneStrings(cmd.TriggerRules),
		Instruction:            cmd.Instruction,
		ReferenceRefs:          cloneStrings(cmd.ReferenceRefs),
		RequiredToolKinds:      cloneStrings(cmd.RequiredToolKinds),
		RequiredMCPServerRefs:  cloneStrings(cmd.RequiredMCPServerRefs),
		RequiredCapabilityRefs: cloneStrings(cmd.RequiredCapabilityRefs),
		Constraints:            cloneStrings(cmd.Constraints),
	}
}

func mcpServerModelFromCommand(cmd MCPServer) *mcpmodel.Server {
	return &mcpmodel.Server{
		ID:                    common.MCPServerID(cmd.ID),
		Name:                  cmd.Name,
		Transport:             cmd.Transport,
		Endpoint:              cmd.Endpoint,
		Command:               cmd.Command,
		Args:                  cloneStrings(cmd.Args),
		Env:                   cloneStringMap(cmd.Env),
		Enabled:               cmd.Enabled,
		AllowedTools:          cloneStrings(cmd.AllowedTools),
		AllowedResources:      cloneStrings(cmd.AllowedResources),
		AllowedPrompts:        cloneStrings(cmd.AllowedPrompts),
		DefaultApprovalPolicy: cmd.DefaultApprovalPolicy,
		SessionPolicy:         cmd.SessionPolicy,
		TimeoutMillis:         cmd.TimeoutMillis,
	}
}

func mcpCapabilityModelFromCommand(cmd MCPCapability) *mcpmodel.Capability {
	return &mcpmodel.Capability{
		ID:             common.MCPCapabilityID(cmd.ID),
		ServerID:       common.MCPServerID(cmd.ServerID),
		Kind:           cmd.Kind,
		Name:           cmd.Name,
		DisplayName:    cmd.DisplayName,
		Description:    cmd.Description,
		Category:       cmd.Category,
		Tags:           cloneStrings(cmd.Tags),
		UsageHint:      cmd.UsageHint,
		InputSummary:   cmd.InputSummary,
		OutputSummary:  cmd.OutputSummary,
		StageHints:     cloneStrings(cmd.StageHints),
		RiskLevel:      cmd.RiskLevel,
		Schema:         cloneMap(cmd.Schema),
		URI:            cmd.URI,
		Enabled:        cmd.Enabled,
		ApprovalPolicy: cmd.ApprovalPolicy,
		Status:         cmd.Status,
	}
}

func mcpCapabilityQueryPayload(query MCPCapabilityQuery) map[string]any {
	ret := map[string]any{}
	if value := strings.TrimSpace(query.ServerID); value != "" {
		ret[events.PayloadKeyServerID] = value
	}
	if value := strings.TrimSpace(query.Kind); value != "" {
		ret[events.PayloadKeyKind] = value
	}
	if value := strings.TrimSpace(query.Category); value != "" {
		ret[events.PayloadKeyCategory] = value
	}
	if value := strings.TrimSpace(query.Stage); value != "" {
		ret[events.PayloadKeyStage] = value
	}
	if value := strings.TrimSpace(query.RiskLevel); value != "" {
		ret[events.PayloadKeyRisk] = value
	}
	if query.Enabled != nil {
		ret[events.PayloadKeyEnabled] = *query.Enabled
	}
	if value := strings.TrimSpace(query.Query); value != "" {
		ret[events.PayloadKeyQuery] = value
	}
	return ret
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

func memoryItemFromCommand(memoryID string, workspaceID string, cmd CreateMemoryItemCommand) *memorymodel.MemoryItem {
	return &memorymodel.MemoryItem{
		ID:                  common.MemoryID(memoryID),
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
	}
}

func memoryItemFromMergedCommand(memoryID string, workspaceID common.WorkspaceID, cmd UpdateMemoryItemCommand) *memorymodel.MemoryItem {
	return &memorymodel.MemoryItem{
		ID:                  common.MemoryID(memoryID),
		WorkspaceID:         workspaceID,
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
	}
}

func publishMemoryItemCommand(ctx context.Context, hub events.Hub, topic string, item *memorymodel.MemoryItem) error {
	if item == nil {
		return common.ErrInvalidArgument
	}
	return publishContextRegistryCommand(ctx, hub, topic, "memory", string(item.ID), map[string]any{
		events.PayloadKeyMemoryItem: item,
	})
}

func getMemoryItemByCommand(ctx context.Context, hub events.Hub, memoryID string) (*memorymodel.MemoryItem, error) {
	response, err := publishContextRegistryCommandWithResponse(ctx, hub, events.TopicMemoryCommandGet, "memory", memoryID, map[string]any{
		events.PayloadKeyMemoryID: memoryID,
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyMemoryItem].(*memorymodel.MemoryItem)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func searchMemoryByCommand(ctx context.Context, hub events.Hub, query repository.MemorySearchQuery) ([]*memorymodel.MemoryItem, error) {
	response, err := publishContextRegistryCommandWithResponse(ctx, hub, events.TopicMemoryCommandSearch, "memory", "search", map[string]any{
		events.PayloadKeyMemorySearchQuery: query,
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyMemoryItems].([]*memorymodel.MemoryItem)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
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

func redactStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		if value == "" {
			out[key] = ""
			continue
		}
		out[key] = "redacted"
	}
	return out
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneFloats(input []float64) []float64 {
	if input == nil {
		return nil
	}
	return append([]float64(nil), input...)
}

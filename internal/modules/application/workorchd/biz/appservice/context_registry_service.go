package appservice

import (
	"context"
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
	SystemInstruction    string   `json:"systemInstruction"`
	PlanningPolicy       string   `json:"planningPolicy,omitempty"`
	DecisionPolicy       string   `json:"decisionPolicy,omitempty"`
	ApprovalPolicy       string   `json:"approvalPolicy,omitempty"`
	ValidationPolicy     string   `json:"validationPolicy,omitempty"`
	AllowedSkillRefs     []string `json:"allowedSkillRefs,omitempty"`
	AllowedMCPServerRefs []string `json:"allowedMCPServerRefs,omitempty"`
	AllowedToolKinds     []string `json:"allowedToolKinds,omitempty"`
	OutputContract       string   `json:"outputContract,omitempty"`
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
}

type MCPServer struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Transport             string   `json:"transport"`
	Endpoint              string   `json:"endpoint,omitempty"`
	Enabled               bool     `json:"enabled"`
	AllowedTools          []string `json:"allowedTools,omitempty"`
	AllowedResources      []string `json:"allowedResources,omitempty"`
	DefaultApprovalPolicy string   `json:"defaultApprovalPolicy,omitempty"`
	TimeoutMillis         int      `json:"timeoutMillis,omitempty"`
}

type MCPTool struct {
	ServerID string `json:"serverId"`
	Name     string `json:"name"`
}

type MemoryItem struct {
	ID          string   `json:"id"`
	WorkspaceID string   `json:"workspaceId"`
	RunID       string   `json:"runId,omitempty"`
	SourceType  string   `json:"sourceType"`
	SourceRef   string   `json:"sourceRef,omitempty"`
	MemoryType  string   `json:"memoryType"`
	Content     string   `json:"content,omitempty"`
	Summary     string   `json:"summary"`
	Tags        []string `json:"tags,omitempty"`
	Importance  int      `json:"importance,omitempty"`
	Confidence  int      `json:"confidence,omitempty"`
	Visibility  string   `json:"visibility,omitempty"`
}

type CreateSoulCommand = Soul
type CreateSkillCommand = Skill
type CreateMCPServerCommand = MCPServer

type CreateMemoryItemCommand struct {
	MemoryID   string   `json:"memoryId,omitempty"`
	RunID      string   `json:"runId,omitempty"`
	SourceType string   `json:"sourceType"`
	SourceRef  string   `json:"sourceRef,omitempty"`
	MemoryType string   `json:"memoryType"`
	Content    string   `json:"content,omitempty"`
	Summary    string   `json:"summary"`
	Tags       []string `json:"tags,omitempty"`
	Importance int      `json:"importance,omitempty"`
	Confidence int      `json:"confidence,omitempty"`
	Visibility string   `json:"visibility,omitempty"`
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
		SystemInstruction:    cmd.SystemInstruction,
		PlanningPolicy:       cmd.PlanningPolicy,
		DecisionPolicy:       cmd.DecisionPolicy,
		ApprovalPolicy:       cmd.ApprovalPolicy,
		ValidationPolicy:     cmd.ValidationPolicy,
		AllowedSkillRefs:     cloneStrings(cmd.AllowedSkillRefs),
		AllowedMCPServerRefs: cloneStrings(cmd.AllowedMCPServerRefs),
		AllowedToolKinds:     cloneStrings(cmd.AllowedToolKinds),
		OutputContract:       cmd.OutputContract,
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

func ReadSkillWithSession(ctx context.Context, session *RuntimeSession, skillID string) (ret Skill, err error) {
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
		return Skill{}, err
	}
	return mapSkill(item), nil
}

func ReadSkillWithBackend(ctx context.Context, backend string, dataRoot string, skillID string) (Skill, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return Skill{}, err
	}
	return ReadSkillWithSession(ctx, session, skillID)
}

func ListSkillsWithSession(ctx context.Context, session *RuntimeSession) (ret []Skill, err error) {
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
	return ret, nil
}

func ListSkillsWithBackend(ctx context.Context, backend string, dataRoot string) ([]Skill, error) {
	session, err := newRuntimeSessionWithBackend(ctx, backend, dataRoot)
	if err != nil {
		return nil, err
	}
	return ListSkillsWithSession(ctx, session)
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
		DefaultApprovalPolicy: cmd.DefaultApprovalPolicy,
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
		MemoryID:    common.MemoryID(memoryID),
		WorkspaceID: common.WorkspaceID(workspaceID),
		RunID:       common.RunID(cmd.RunID),
		SourceType:  cmd.SourceType,
		SourceRef:   cmd.SourceRef,
		MemoryType:  cmd.MemoryType,
		Content:     cmd.Content,
		Summary:     cmd.Summary,
		Tags:        cloneStrings(cmd.Tags),
		Importance:  cmd.Importance,
		Confidence:  cmd.Confidence,
		Visibility:  cmd.Visibility,
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
		SystemInstruction:    item.SystemInstruction,
		PlanningPolicy:       item.PlanningPolicy,
		DecisionPolicy:       item.DecisionPolicy,
		ApprovalPolicy:       item.ApprovalPolicy,
		ValidationPolicy:     item.ValidationPolicy,
		AllowedSkillRefs:     cloneStrings(item.AllowedSkillRefs),
		AllowedMCPServerRefs: cloneStrings(item.AllowedMCPServerRefs),
		AllowedToolKinds:     cloneStrings(item.AllowedToolKinds),
		OutputContract:       item.OutputContract,
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
		DefaultApprovalPolicy: item.DefaultApprovalPolicy,
		TimeoutMillis:         item.TimeoutMillis,
	}
}

func mapMemoryItem(item *memorymodel.MemoryItem) MemoryItem {
	if item == nil {
		return MemoryItem{}
	}
	return MemoryItem{
		ID:          string(item.ID),
		WorkspaceID: string(item.WorkspaceID),
		RunID:       string(item.RunID),
		SourceType:  item.SourceType,
		SourceRef:   item.SourceRef,
		MemoryType:  item.MemoryType,
		Content:     item.Content,
		Summary:     item.Summary,
		Tags:        cloneStrings(item.Tags),
		Importance:  item.Importance,
		Confidence:  item.Confidence,
		Visibility:  item.Visibility,
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

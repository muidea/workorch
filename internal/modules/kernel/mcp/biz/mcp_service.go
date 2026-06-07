package mcp

import (
	"context"
	"net/url"
	"strings"

	mcpcontracts "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/contracts"
	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type Service struct {
	servers      repository.MCPServerRepository
	capabilities repository.MCPCapabilityRepository
}

func NewService(servers repository.MCPServerRepository, capabilities repository.MCPCapabilityRepository) *Service {
	return &Service{servers: servers, capabilities: capabilities}
}

func (s *Service) CreateServer(ctx context.Context, cmd mcpcontracts.CreateServerCommand) error {
	if err := validateServerCommand(cmd); err != nil {
		return err
	}
	server := serverFromCommand(cmd)
	if err := s.servers.Create(ctx, server); err != nil {
		return err
	}
	return s.ensureDefaultCapabilities(ctx, server)
}

func (s *Service) UpdateServer(ctx context.Context, cmd mcpcontracts.UpdateServerCommand) error {
	if err := validateServerCommand(cmd); err != nil {
		return err
	}
	server := serverFromCommand(cmd)
	if err := s.servers.Update(ctx, server); err != nil {
		return err
	}
	return s.ensureDefaultCapabilities(ctx, server)
}

func (s *Service) GetServer(ctx context.Context, id common.MCPServerID) (*mcpmodel.Server, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.servers.Get(ctx, id)
}

func (s *Service) ListServers(ctx context.Context) ([]*mcpmodel.Server, error) {
	return s.servers.List(ctx)
}

func (s *Service) CreateCapability(ctx context.Context, cmd mcpcontracts.CreateCapabilityCommand) (*mcpmodel.Capability, error) {
	capability, err := s.capabilityFromCommand(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if err := s.capabilities.Create(ctx, capability); err != nil {
		return nil, err
	}
	return s.capabilities.Get(ctx, capability.ID)
}

func (s *Service) UpdateCapability(ctx context.Context, cmd mcpcontracts.UpdateCapabilityCommand) (*mcpmodel.Capability, error) {
	capability, err := s.capabilityFromCommand(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if err := s.capabilities.Update(ctx, capability); err != nil {
		return nil, err
	}
	return s.capabilities.Get(ctx, capability.ID)
}

func (s *Service) GetCapability(ctx context.Context, id common.MCPCapabilityID) (*mcpmodel.Capability, error) {
	if id == "" {
		return nil, common.ErrInvalidArgument
	}
	return s.capabilities.Get(ctx, id)
}

func (s *Service) ListCapabilities(ctx context.Context, query mcpcontracts.CapabilityQuery) ([]*mcpmodel.Capability, error) {
	items, err := s.capabilities.List(ctx)
	if err != nil {
		return nil, err
	}
	ret := make([]*mcpmodel.Capability, 0, len(items))
	for _, item := range items {
		if !matchesCapabilityQuery(item, query) {
			continue
		}
		ret = append(ret, item)
	}
	return ret, nil
}

func validateServerCommand(cmd mcpcontracts.CreateServerCommand) error {
	if cmd.ServerID == "" || strings.TrimSpace(cmd.Name) == "" || strings.TrimSpace(cmd.Transport) == "" {
		return common.ErrInvalidArgument
	}
	if cmd.TimeoutMillis < 0 {
		return common.ErrInvalidArgument
	}
	if hasBlank(cmd.AllowedTools) || hasBlank(cmd.AllowedResources) || hasBlank(cmd.AllowedPrompts) || hasBlankMapKey(cmd.Env) {
		return common.ErrInvalidArgument
	}
	switch strings.TrimSpace(cmd.SessionPolicy) {
	case "", "transient", "long_lived":
	default:
		return common.ErrInvalidArgument
	}
	return nil
}

func (s *Service) capabilityFromCommand(ctx context.Context, cmd mcpcontracts.CreateCapabilityCommand) (*mcpmodel.Capability, error) {
	if cmd.ServerID == "" || strings.TrimSpace(cmd.Kind) == "" || strings.TrimSpace(cmd.Name) == "" {
		return nil, common.ErrInvalidArgument
	}
	if !isCapabilityKind(cmd.Kind) {
		return nil, common.ErrInvalidArgument
	}
	if _, err := s.servers.Get(ctx, cmd.ServerID); err != nil {
		return nil, err
	}
	id := cmd.ID
	if id == "" {
		id = DefaultCapabilityID(cmd.ServerID, cmd.Kind, cmd.Name)
	}
	displayName := strings.TrimSpace(cmd.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(cmd.Name)
	}
	return &mcpmodel.Capability{
		ID:             id,
		ServerID:       cmd.ServerID,
		Kind:           strings.TrimSpace(cmd.Kind),
		Name:           strings.TrimSpace(cmd.Name),
		DisplayName:    displayName,
		Description:    strings.TrimSpace(cmd.Description),
		Category:       strings.TrimSpace(cmd.Category),
		Tags:           compactStrings(cmd.Tags),
		UsageHint:      strings.TrimSpace(cmd.UsageHint),
		InputSummary:   strings.TrimSpace(cmd.InputSummary),
		OutputSummary:  strings.TrimSpace(cmd.OutputSummary),
		StageHints:     compactStrings(cmd.StageHints),
		RiskLevel:      strings.TrimSpace(cmd.RiskLevel),
		Schema:         cloneMap(cmd.Schema),
		URI:            strings.TrimSpace(cmd.URI),
		Enabled:        cmd.Enabled,
		ApprovalPolicy: strings.TrimSpace(cmd.ApprovalPolicy),
		Status:         strings.TrimSpace(cmd.Status),
	}, nil
}

func (s *Service) ensureDefaultCapabilities(ctx context.Context, server *mcpmodel.Server) error {
	for _, capability := range defaultCapabilities(server) {
		existing, err := s.capabilities.Get(ctx, capability.ID)
		if err == nil {
			existing.ServerID = capability.ServerID
			existing.Kind = capability.Kind
			existing.Name = capability.Name
			if strings.TrimSpace(existing.DisplayName) == "" {
				existing.DisplayName = capability.DisplayName
			}
			if strings.TrimSpace(existing.ApprovalPolicy) == "" {
				existing.ApprovalPolicy = capability.ApprovalPolicy
			}
			existing.Enabled = server.Enabled
			if updateErr := s.capabilities.Update(ctx, existing); updateErr != nil {
				return updateErr
			}
			continue
		}
		if err != common.ErrNotFound {
			return err
		}
		if err := s.capabilities.Create(ctx, capability); err != nil {
			return err
		}
	}
	return nil
}

func defaultCapabilities(server *mcpmodel.Server) []*mcpmodel.Capability {
	ret := make([]*mcpmodel.Capability, 0, len(server.AllowedTools)+len(server.AllowedResources)+len(server.AllowedPrompts))
	for _, name := range compactStrings(server.AllowedTools) {
		ret = append(ret, defaultCapability(server, mcpmodel.CapabilityKindTool, name, ""))
	}
	for _, uri := range compactStrings(server.AllowedResources) {
		ret = append(ret, defaultCapability(server, mcpmodel.CapabilityKindResource, uri, uri))
	}
	for _, name := range compactStrings(server.AllowedPrompts) {
		ret = append(ret, defaultCapability(server, mcpmodel.CapabilityKindPrompt, name, ""))
	}
	return ret
}

func defaultCapability(server *mcpmodel.Server, kind string, name string, uri string) *mcpmodel.Capability {
	displayName := defaultCapabilityDisplayName(kind, name)
	ret := &mcpmodel.Capability{
		ID:             DefaultCapabilityID(server.ID, kind, name),
		ServerID:       server.ID,
		Kind:           kind,
		Name:           name,
		DisplayName:    displayName,
		URI:            uri,
		Enabled:        server.Enabled,
		ApprovalPolicy: server.DefaultApprovalPolicy,
		Status:         "draft",
		RiskLevel:      "read_only",
	}
	applyBuiltinCapabilityMetadata(ret)
	return ret
}

func applyBuiltinCapabilityMetadata(item *mcpmodel.Capability) {
	if item == nil {
		return
	}
	switch {
	case item.Kind == mcpmodel.CapabilityKindResource && item.Name == "workspace://acceptance":
		item.DisplayName = "工作空间验收资料"
		item.Description = "读取当前工作空间验收资料，用于 Run 规划、验证和结果审查。"
		item.Category = "验收"
		item.Tags = []string{"acceptance", "workspace"}
		item.UsageHint = "当执行需要读取工作空间验收上下文或 DoD 资料时使用。"
		item.OutputSummary = "返回工作空间验收资料。"
		item.StageHints = []string{"planning", "validation"}
	case item.Kind == mcpmodel.CapabilityKindPrompt && item.Name == "workorch.acceptance":
		item.DisplayName = "验收提示词"
		item.Description = "获取 workorch 验收提示词，用于统一 MCP 驱动执行的验收语义。"
		item.Category = "验收"
		item.Tags = []string{"acceptance", "prompt"}
		item.UsageHint = "当执行需要统一验收提示词时使用。"
		item.OutputSummary = "返回验收提示词内容。"
		item.StageHints = []string{"planning", "validation"}
	}
}

func DefaultCapabilityID(serverID common.MCPServerID, kind string, name string) common.MCPCapabilityID {
	return common.MCPCapabilityID(string(serverID) + ":" + strings.TrimSpace(kind) + ":" + url.PathEscape(strings.TrimSpace(name)))
}

func defaultCapabilityDisplayName(kind string, name string) string {
	switch kind {
	case mcpmodel.CapabilityKindTool:
		return "工具 " + name
	case mcpmodel.CapabilityKindResource:
		return "资源 " + name
	case mcpmodel.CapabilityKindPrompt:
		return "提示词 " + name
	default:
		return name
	}
}

func matchesCapabilityQuery(item *mcpmodel.Capability, query mcpcontracts.CapabilityQuery) bool {
	if query.ServerID != "" && item.ServerID != query.ServerID {
		return false
	}
	if strings.TrimSpace(query.Kind) != "" && item.Kind != strings.TrimSpace(query.Kind) {
		return false
	}
	if strings.TrimSpace(query.Category) != "" && item.Category != strings.TrimSpace(query.Category) {
		return false
	}
	if strings.TrimSpace(query.RiskLevel) != "" && item.RiskLevel != strings.TrimSpace(query.RiskLevel) {
		return false
	}
	if query.Enabled != nil && item.Enabled != *query.Enabled {
		return false
	}
	if strings.TrimSpace(query.Stage) != "" && !containsString(item.StageHints, strings.TrimSpace(query.Stage)) {
		return false
	}
	q := strings.ToLower(strings.TrimSpace(query.Query))
	if q != "" && !capabilityContains(item, q) {
		return false
	}
	return true
}

func capabilityContains(item *mcpmodel.Capability, q string) bool {
	values := []string{string(item.ID), string(item.ServerID), item.Kind, item.Name, item.DisplayName, item.Description, item.Category, item.UsageHint, item.InputSummary, item.OutputSummary, item.RiskLevel, item.Status}
	values = append(values, item.Tags...)
	values = append(values, item.StageHints...)
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), q) {
			return true
		}
	}
	return false
}

func isCapabilityKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case mcpmodel.CapabilityKindTool, mcpmodel.CapabilityKindResource, mcpmodel.CapabilityKindPrompt:
		return true
	default:
		return false
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func serverFromCommand(cmd mcpcontracts.CreateServerCommand) *mcpmodel.Server {
	return &mcpmodel.Server{
		ID:                    cmd.ServerID,
		Name:                  cmd.Name,
		Transport:             cmd.Transport,
		Endpoint:              cmd.Endpoint,
		Command:               strings.TrimSpace(cmd.Command),
		Args:                  compactStrings(cmd.Args),
		Env:                   cloneStringMap(cmd.Env),
		Enabled:               cmd.Enabled,
		AllowedTools:          cloneStrings(cmd.AllowedTools),
		AllowedResources:      cloneStrings(cmd.AllowedResources),
		AllowedPrompts:        cloneStrings(cmd.AllowedPrompts),
		DefaultApprovalPolicy: cmd.DefaultApprovalPolicy,
		SessionPolicy:         strings.TrimSpace(cmd.SessionPolicy),
		TimeoutMillis:         cmd.TimeoutMillis,
	}
}

func hasBlank(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
	}
	return false
}

func hasBlankMapKey(values map[string]string) bool {
	for key := range values {
		if strings.TrimSpace(key) == "" {
			return true
		}
	}
	return false
}

func cloneStrings(input []string) []string {
	if input == nil {
		return nil
	}
	return append([]string(nil), input...)
}

func compactStrings(input []string) []string {
	if input == nil {
		return nil
	}
	ret := make([]string, 0, len(input))
	seen := map[string]struct{}{}
	for _, value := range input {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ret = append(ret, value)
	}
	return ret
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		ret[key] = value
	}
	return ret
}

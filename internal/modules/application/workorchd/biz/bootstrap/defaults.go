package bootstrap

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	mcpmodel "github.com/muidea/workorch/internal/modules/kernel/mcp/pkg/models"
	soulmodel "github.com/muidea/workorch/internal/modules/kernel/soul/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

//go:embed defaults/templates.json defaults/souls.json defaults/souls/*/*.md defaults/mcp_servers.json defaults/mcp_capabilities.json
var embeddedDefaults embed.FS

type embeddedTemplate struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Description      string            `json:"description,omitempty"`
	GoalText         string            `json:"goalText,omitempty"`
	DoDItems         []string          `json:"doDItems,omitempty"`
	DefaultSoulID    string            `json:"defaultSoulId,omitempty"`
	SoulBindings     map[string]string `json:"soulBindings,omitempty"`
	ContextHints     map[string]any    `json:"contextHints,omitempty"`
	EvidencePolicies []map[string]any  `json:"evidencePolicies,omitempty"`
}

type embeddedSoul struct {
	ID                          string   `json:"id"`
	Name                        string   `json:"name"`
	Description                 string   `json:"description"`
	BusinessDomains             []string `json:"businessDomains"`
	AllowedSkillRefs            []string `json:"allowedSkillRefs,omitempty"`
	AllowedMCPServerRefs        []string `json:"allowedMCPServerRefs,omitempty"`
	AllowedCapabilityRefs       []string `json:"allowedCapabilityRefs,omitempty"`
	AllowedCapabilityCategories []string `json:"allowedCapabilityCategories,omitempty"`
	AllowedToolKinds            []string `json:"allowedToolKinds,omitempty"`
	SoulMDRef                   string   `json:"soulMDRef"`
	SoulMD                      string   `json:"-"`
}

type embeddedMCPServer struct {
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

type embeddedMCPCapability struct {
	ID             string         `json:"id"`
	ServerID       string         `json:"serverId"`
	Kind           string         `json:"kind"`
	Name           string         `json:"name"`
	DisplayName    string         `json:"displayName,omitempty"`
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

func loadEmbeddedTemplates() ([]embeddedTemplate, error) {
	data, err := embeddedDefaults.ReadFile("defaults/templates.json")
	if err != nil {
		return nil, err
	}
	var items []embeddedTemplate
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func loadEmbeddedSoulMD(item embeddedSoul) (string, error) {
	ref := strings.TrimSpace(item.SoulMDRef)
	if ref == "" {
		return "", fmt.Errorf("%w: embedded soul %q soulMDRef is required", common.ErrInvalidArgument, item.ID)
	}
	if err := validateEmbeddedSoulMDRef(ref); err != nil {
		return "", fmt.Errorf("%w: embedded soul %q has invalid soulMDRef %q: %v", common.ErrInvalidArgument, item.ID, ref, err)
	}
	data, err := embeddedDefaults.ReadFile("defaults/" + ref)
	if err != nil {
		return "", err
	}
	body := strings.TrimSpace(string(data))
	if body == "" {
		return "", fmt.Errorf("%w: embedded soul %q soul markdown is empty", common.ErrInvalidArgument, item.ID)
	}
	return body, nil
}

func validateEmbeddedSoulMDRef(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("empty ref")
	}
	if strings.HasPrefix(ref, "/") {
		return fmt.Errorf("absolute path is not allowed")
	}
	if !strings.HasPrefix(ref, "souls/") {
		return fmt.Errorf("must be under souls/")
	}
	if !strings.HasSuffix(ref, ".md") {
		return fmt.Errorf("must use .md extension")
	}
	parts := strings.Split(ref, "/")
	if len(parts) != 3 {
		return fmt.Errorf("must use souls/<template>/<stage>.md")
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("invalid path segment %q", part)
		}
	}
	return nil
}

func loadEmbeddedSouls() ([]embeddedSoul, error) {
	data, err := embeddedDefaults.ReadFile("defaults/souls.json")
	if err != nil {
		return nil, err
	}
	var items []embeddedSoul
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	for idx := range items {
		body, err := loadEmbeddedSoulMD(items[idx])
		if err != nil {
			return nil, err
		}
		items[idx].SoulMD = body
	}
	return items, nil
}

func loadEmbeddedMCPServers() ([]embeddedMCPServer, error) {
	data, err := embeddedDefaults.ReadFile("defaults/mcp_servers.json")
	if err != nil {
		return nil, err
	}
	var items []embeddedMCPServer
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func loadEmbeddedMCPCapabilities() ([]embeddedMCPCapability, error) {
	data, err := embeddedDefaults.ReadFile("defaults/mcp_capabilities.json")
	if err != nil {
		return nil, err
	}
	var items []embeddedMCPCapability
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func EnsureBuiltinDefaults(ctx context.Context, soulRepo repository.SoulRepository, templateRepo repository.WorkspaceTemplateRepository, mcpServerRepo repository.MCPServerRepository, mcpCapabilityRepo repository.MCPCapabilityRepository) error {
	if soulRepo == nil || templateRepo == nil {
		return nil
	}

	souls, err := loadEmbeddedSouls()
	if err != nil {
		return err
	}
	for _, s := range souls {
		if s.ID == "" {
			continue
		}
		item := &soulmodel.Soul{
			ID:                          common.SoulID(s.ID),
			Name:                        s.Name,
			Description:                 s.Description,
			BusinessDomains:             s.BusinessDomains,
			SoulMD:                      s.SoulMD,
			AllowedSkillRefs:            s.AllowedSkillRefs,
			AllowedMCPServerRefs:        s.AllowedMCPServerRefs,
			AllowedCapabilityRefs:       s.AllowedCapabilityRefs,
			AllowedCapabilityCategories: s.AllowedCapabilityCategories,
			AllowedToolKinds:            s.AllowedToolKinds,
		}
		if err := upsertBuiltinSoul(ctx, soulRepo, item); err != nil {
			return err
		}
	}

	templates, err := loadEmbeddedTemplates()
	if err != nil {
		return err
	}
	for _, t := range templates {
		if t.ID == "" {
			continue
		}
		if err := upsertBuiltinWorkspaceTemplate(ctx, templateRepo, workspaceTemplateFromEmbedded(t)); err != nil {
			return err
		}
	}
	if mcpServerRepo != nil {
		servers, err := loadEmbeddedMCPServers()
		if err != nil {
			return err
		}
		for _, server := range servers {
			if server.ID == "" {
				continue
			}
			if err := upsertBuiltinMCPServer(ctx, mcpServerRepo, mcpServerFromEmbedded(server)); err != nil {
				return err
			}
		}
	}
	if mcpCapabilityRepo != nil {
		capabilities, err := loadEmbeddedMCPCapabilities()
		if err != nil {
			return err
		}
		for _, capability := range capabilities {
			if capability.ID == "" {
				continue
			}
			if err := upsertBuiltinMCPCapability(ctx, mcpCapabilityRepo, mcpCapabilityFromEmbedded(capability)); err != nil {
				return err
			}
		}
	}
	return nil
}

func upsertBuiltinSoul(ctx context.Context, repo repository.SoulRepository, item *soulmodel.Soul) error {
	if item == nil || item.ID == "" {
		return nil
	}
	if err := repo.Update(ctx, item); err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return repo.Create(ctx, item)
		}
		return err
	}
	return nil
}

func upsertBuiltinWorkspaceTemplate(ctx context.Context, repo repository.WorkspaceTemplateRepository, item *repository.WorkspaceTemplate) error {
	if item == nil || strings.TrimSpace(item.ID) == "" {
		return nil
	}
	if err := repo.Update(ctx, item); err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return repo.Create(ctx, item)
		}
		return err
	}
	return nil
}

func upsertBuiltinMCPServer(ctx context.Context, repo repository.MCPServerRepository, item *mcpmodel.Server) error {
	if item == nil || item.ID == "" {
		return nil
	}
	if err := repo.Update(ctx, item); err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return repo.Create(ctx, item)
		}
		return err
	}
	return nil
}

func upsertBuiltinMCPCapability(ctx context.Context, repo repository.MCPCapabilityRepository, item *mcpmodel.Capability) error {
	if item == nil || item.ID == "" {
		return nil
	}
	if err := repo.Update(ctx, item); err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return repo.Create(ctx, item)
		}
		return err
	}
	return nil
}

func workspaceTemplateFromEmbedded(t embeddedTemplate) *repository.WorkspaceTemplate {
	return &repository.WorkspaceTemplate{
		ID:               t.ID,
		Name:             t.Name,
		Description:      t.Description,
		GoalText:         t.GoalText,
		DoDItems:         filterEmptyStrings(t.DoDItems),
		DefaultSoulID:    t.DefaultSoulID,
		SoulBindings:     t.SoulBindings,
		ContextHints:     cloneContextMap(t.ContextHints),
		EvidencePolicies: cloneEmbeddedEvidencePolicies(t.EvidencePolicies),
	}
}

func mcpServerFromEmbedded(item embeddedMCPServer) *mcpmodel.Server {
	return &mcpmodel.Server{
		ID:                    common.MCPServerID(item.ID),
		Name:                  strings.TrimSpace(item.Name),
		Transport:             strings.TrimSpace(item.Transport),
		Endpoint:              strings.TrimSpace(item.Endpoint),
		Command:               strings.TrimSpace(item.Command),
		Args:                  filterEmptyStrings(item.Args),
		Env:                   cloneStringMap(item.Env),
		Enabled:               item.Enabled,
		AllowedTools:          filterEmptyStrings(item.AllowedTools),
		AllowedResources:      filterEmptyStrings(item.AllowedResources),
		AllowedPrompts:        filterEmptyStrings(item.AllowedPrompts),
		DefaultApprovalPolicy: strings.TrimSpace(item.DefaultApprovalPolicy),
		SessionPolicy:         strings.TrimSpace(item.SessionPolicy),
		TimeoutMillis:         item.TimeoutMillis,
	}
}

func mcpCapabilityFromEmbedded(item embeddedMCPCapability) *mcpmodel.Capability {
	return &mcpmodel.Capability{
		ID:             common.MCPCapabilityID(item.ID),
		ServerID:       common.MCPServerID(item.ServerID),
		Kind:           strings.TrimSpace(item.Kind),
		Name:           strings.TrimSpace(item.Name),
		DisplayName:    strings.TrimSpace(item.DisplayName),
		Description:    strings.TrimSpace(item.Description),
		Category:       strings.TrimSpace(item.Category),
		Tags:           filterEmptyStrings(item.Tags),
		UsageHint:      strings.TrimSpace(item.UsageHint),
		InputSummary:   strings.TrimSpace(item.InputSummary),
		OutputSummary:  strings.TrimSpace(item.OutputSummary),
		StageHints:     filterEmptyStrings(item.StageHints),
		RiskLevel:      strings.TrimSpace(item.RiskLevel),
		Schema:         cloneContextMap(item.Schema),
		URI:            strings.TrimSpace(item.URI),
		Enabled:        item.Enabled,
		ApprovalPolicy: strings.TrimSpace(item.ApprovalPolicy),
		Status:         strings.TrimSpace(item.Status),
	}
}

func cloneContextMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	ret := make(map[string]any, len(input))
	for key, value := range input {
		ret[key] = cloneEmbeddedEvidencePolicyValue(value)
	}
	return ret
}

func cloneEmbeddedEvidencePolicies(input []map[string]any) []map[string]any {
	if input == nil {
		return nil
	}
	ret := make([]map[string]any, 0, len(input))
	for _, item := range input {
		cloned := make(map[string]any, len(item))
		for key, value := range item {
			cloned[key] = cloneEmbeddedEvidencePolicyValue(value)
		}
		ret = append(ret, cloned)
	}
	return ret
}

func cloneEmbeddedEvidencePolicyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, value := range typed {
			cloned[key] = cloneEmbeddedEvidencePolicyValue(value)
		}
		return cloned
	case []map[string]any:
		return cloneEmbeddedEvidencePolicies(typed)
	case []any:
		ret := make([]any, 0, len(typed))
		for _, item := range typed {
			ret = append(ret, cloneEmbeddedEvidencePolicyValue(item))
		}
		return ret
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	ret := make(map[string]string, len(input))
	for key, value := range input {
		ret[key] = value
	}
	return ret
}

func filterEmptyStrings(items []string) []string {
	filtered := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

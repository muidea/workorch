package workorchdefaults

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

//go:embed defaults/templates.json defaults/souls.json defaults/souls/*/*.md defaults/mcp_servers.json defaults/mcp_capabilities.json
var embeddedDefaults embed.FS

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

// SoulDefault, WorkspaceTemplateDefault, MCPServerDefault and
// MCPCapabilityDefault are immutable bootstrap declarations. This package only
// decodes embedded data; each resource Block owns validation and persistence.
type SoulDefault struct {
	ID                          common.SoulID
	Name                        string
	Description                 string
	BusinessDomains             []string
	SoulMD                      string
	AllowedSkillRefs            []string
	AllowedMCPServerRefs        []string
	AllowedCapabilityRefs       []string
	AllowedCapabilityCategories []string
	AllowedToolKinds            []string
}

type WorkspaceTemplateDefault struct {
	ID               string
	Name             string
	Description      string
	GoalText         string
	DoDItems         []string
	DefaultSoulID    string
	SoulBindings     map[string]string
	ContextHints     workorcheventpayload.Payload
	EvidencePolicies []workorcheventpayload.Payload
}

type MCPServerDefault struct {
	ID                    common.MCPServerID
	Name                  string
	Transport             string
	Endpoint              string
	Command               string
	Args                  []string
	Env                   map[string]string
	Enabled               bool
	AllowedTools          []string
	AllowedResources      []string
	AllowedPrompts        []string
	DefaultApprovalPolicy string
	SessionPolicy         string
	TimeoutMillis         int
}

type MCPCapabilityDefault struct {
	ID             common.MCPCapabilityID
	ServerID       common.MCPServerID
	Kind           string
	Name           string
	DisplayName    string
	Description    string
	Category       string
	Tags           []string
	UsageHint      string
	InputSummary   string
	OutputSummary  string
	StageHints     []string
	RiskLevel      string
	Schema         workorcheventpayload.Payload
	URI            string
	Enabled        bool
	ApprovalPolicy string
	Status         string
}

func loadEmbeddedTemplates() ([]workorchmanagementcontract.WorkspaceTemplate, error) {
	data, err := embeddedDefaults.ReadFile("defaults/templates.json")
	if err != nil {
		return nil, err
	}
	var items []workorchmanagementcontract.WorkspaceTemplate
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

func loadEmbeddedMCPServers() ([]workorchmanagementcontract.MCPServer, error) {
	data, err := embeddedDefaults.ReadFile("defaults/mcp_servers.json")
	if err != nil {
		return nil, err
	}
	var items []workorchmanagementcontract.MCPServer
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

func BuiltinSoulDefaults() ([]SoulDefault, error) {
	items, err := loadEmbeddedSouls()
	if err != nil {
		return nil, err
	}
	defaults := make([]SoulDefault, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		defaults = append(defaults, SoulDefault{ID: common.SoulID(item.ID), Name: item.Name, Description: item.Description, BusinessDomains: append([]string(nil), item.BusinessDomains...), SoulMD: item.SoulMD, AllowedSkillRefs: append([]string(nil), item.AllowedSkillRefs...), AllowedMCPServerRefs: append([]string(nil), item.AllowedMCPServerRefs...), AllowedCapabilityRefs: append([]string(nil), item.AllowedCapabilityRefs...), AllowedCapabilityCategories: append([]string(nil), item.AllowedCapabilityCategories...), AllowedToolKinds: append([]string(nil), item.AllowedToolKinds...)})
	}
	return defaults, nil
}

func BuiltinWorkspaceTemplateDefaults() ([]WorkspaceTemplateDefault, error) {
	items, err := loadEmbeddedTemplates()
	if err != nil {
		return nil, err
	}
	defaults := make([]WorkspaceTemplateDefault, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		value, err := workspaceTemplateDefaultFromEmbedded(item)
		if err != nil {
			return nil, err
		}
		defaults = append(defaults, value)
	}
	return defaults, nil
}

func BuiltinMCPServerDefaults() ([]MCPServerDefault, error) {
	items, err := loadEmbeddedMCPServers()
	if err != nil {
		return nil, err
	}
	defaults := make([]MCPServerDefault, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		defaults = append(defaults, MCPServerDefault{ID: common.MCPServerID(item.ID), Name: strings.TrimSpace(item.Name), Transport: strings.TrimSpace(item.Transport), Endpoint: strings.TrimSpace(item.Endpoint), Command: strings.TrimSpace(item.Command), Args: filterEmptyStrings(item.Args), Env: cloneStringMap(item.Env), Enabled: item.Enabled, AllowedTools: filterEmptyStrings(item.AllowedTools), AllowedResources: filterEmptyStrings(item.AllowedResources), AllowedPrompts: filterEmptyStrings(item.AllowedPrompts), DefaultApprovalPolicy: strings.TrimSpace(item.DefaultApprovalPolicy), SessionPolicy: strings.TrimSpace(item.SessionPolicy), TimeoutMillis: item.TimeoutMillis})
	}
	return defaults, nil
}

func BuiltinMCPCapabilityDefaults() ([]MCPCapabilityDefault, error) {
	items, err := loadEmbeddedMCPCapabilities()
	if err != nil {
		return nil, err
	}
	defaults := make([]MCPCapabilityDefault, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		value, err := mcpCapabilityDefaultFromEmbedded(item)
		if err != nil {
			return nil, err
		}
		defaults = append(defaults, value)
	}
	return defaults, nil
}

func workspaceTemplateDefaultFromEmbedded(t workorchmanagementcontract.WorkspaceTemplate) (WorkspaceTemplateDefault, error) {
	contextHints, err := workorcheventpayload.FromMap(t.ContextHints)
	if err != nil {
		return WorkspaceTemplateDefault{}, fmt.Errorf("decode embedded workspace template %q context hints: %w", t.ID, err)
	}
	evidencePolicies, err := workorcheventpayload.FromMapSlice(t.EvidencePolicies)
	if err != nil {
		return WorkspaceTemplateDefault{}, fmt.Errorf("decode embedded workspace template %q evidence policies: %w", t.ID, err)
	}
	return WorkspaceTemplateDefault{
		ID:               t.ID,
		Name:             t.Name,
		Description:      t.Description,
		GoalText:         t.GoalText,
		DoDItems:         filterEmptyStrings(t.DoDItems),
		DefaultSoulID:    t.DefaultSoulID,
		SoulBindings:     t.SoulBindings,
		ContextHints:     contextHints,
		EvidencePolicies: evidencePolicies,
	}, nil
}

func mcpCapabilityDefaultFromEmbedded(item embeddedMCPCapability) (MCPCapabilityDefault, error) {
	schema, err := workorcheventpayload.FromMap(item.Schema)
	if err != nil {
		return MCPCapabilityDefault{}, fmt.Errorf("decode embedded mcp capability %q schema: %w", item.ID, err)
	}
	return MCPCapabilityDefault{
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
		Schema:         schema,
		URI:            strings.TrimSpace(item.URI),
		Enabled:        item.Enabled,
		ApprovalPolicy: strings.TrimSpace(item.ApprovalPolicy),
		Status:         strings.TrimSpace(item.Status),
	}, nil
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

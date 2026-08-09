package workorchdefaults

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
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
	ID               string         `json:"id"`
	ServerID         string         `json:"serverId"`
	Kind             string         `json:"kind"`
	Name             string         `json:"name"`
	DisplayName      string         `json:"displayName,omitempty"`
	Description      string         `json:"description,omitempty"`
	Category         string         `json:"category,omitempty"`
	Tags             []string       `json:"tags,omitempty"`
	UsageHint        string         `json:"usageHint,omitempty"`
	InputSummary     string         `json:"inputSummary,omitempty"`
	OutputSummary    string         `json:"outputSummary,omitempty"`
	StageHints       []string       `json:"stageHints,omitempty"`
	RiskLevel        string         `json:"riskLevel,omitempty"`
	EffectScope      string         `json:"effectScope,omitempty"`
	IdempotencyClass string         `json:"idempotencyClass,omitempty"`
	Schema           map[string]any `json:"schema,omitempty"`
	URI              string         `json:"uri,omitempty"`
	Enabled          bool           `json:"enabled"`
	ApprovalPolicy   string         `json:"approvalPolicy,omitempty"`
	Status           string         `json:"status,omitempty"`
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

// BuiltinSoulDefaults returns immutable owner-neutral management DTOs. Each
// resource owner remains responsible for typed IDs, validation and persistence.
func BuiltinSoulDefaults() ([]workorchmanagementcontract.Soul, error) {
	items, err := loadEmbeddedSouls()
	if err != nil {
		return nil, err
	}
	defaults := make([]workorchmanagementcontract.Soul, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		defaults = append(defaults, workorchmanagementcontract.Soul{ID: item.ID, Name: item.Name, Description: item.Description, BusinessDomains: append([]string(nil), item.BusinessDomains...), SoulMD: item.SoulMD, AllowedSkillRefs: append([]string(nil), item.AllowedSkillRefs...), AllowedMCPServerRefs: append([]string(nil), item.AllowedMCPServerRefs...), AllowedCapabilityRefs: append([]string(nil), item.AllowedCapabilityRefs...), AllowedCapabilityCategories: append([]string(nil), item.AllowedCapabilityCategories...), AllowedToolKinds: append([]string(nil), item.AllowedToolKinds...)})
	}
	return defaults, nil
}

func BuiltinWorkspaceTemplateDefaults() ([]workorchmanagementcontract.WorkspaceTemplate, error) {
	items, err := loadEmbeddedTemplates()
	if err != nil {
		return nil, err
	}
	defaults := make([]workorchmanagementcontract.WorkspaceTemplate, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		item.DoDItems = filterEmptyStrings(item.DoDItems)
		item.SoulBindings = cloneStringMap(item.SoulBindings)
		item.ContextHints = cloneContextMap(item.ContextHints)
		item.EvidencePolicies = cloneEmbeddedEvidencePolicies(item.EvidencePolicies)
		defaults = append(defaults, item)
	}
	return defaults, nil
}

func BuiltinMCPServerDefaults() ([]workorchmanagementcontract.MCPServer, error) {
	items, err := loadEmbeddedMCPServers()
	if err != nil {
		return nil, err
	}
	defaults := make([]workorchmanagementcontract.MCPServer, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		item.Name = strings.TrimSpace(item.Name)
		item.Transport = strings.TrimSpace(item.Transport)
		item.Endpoint = strings.TrimSpace(item.Endpoint)
		item.Command = strings.TrimSpace(item.Command)
		item.Args = filterEmptyStrings(item.Args)
		item.Env = cloneStringMap(item.Env)
		item.AllowedTools = filterEmptyStrings(item.AllowedTools)
		item.AllowedResources = filterEmptyStrings(item.AllowedResources)
		item.AllowedPrompts = filterEmptyStrings(item.AllowedPrompts)
		item.DefaultApprovalPolicy = strings.TrimSpace(item.DefaultApprovalPolicy)
		item.SessionPolicy = strings.TrimSpace(item.SessionPolicy)
		defaults = append(defaults, item)
	}
	return defaults, nil
}

func BuiltinMCPCapabilityDefaults() ([]workorchmanagementcontract.MCPCapability, error) {
	items, err := loadEmbeddedMCPCapabilities()
	if err != nil {
		return nil, err
	}
	defaults := make([]workorchmanagementcontract.MCPCapability, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		defaults = append(defaults, workorchmanagementcontract.MCPCapability{
			ID: item.ID, ServerID: item.ServerID, Kind: strings.TrimSpace(item.Kind), Name: strings.TrimSpace(item.Name),
			DisplayName: strings.TrimSpace(item.DisplayName), Description: strings.TrimSpace(item.Description), Category: strings.TrimSpace(item.Category),
			Tags: filterEmptyStrings(item.Tags), UsageHint: strings.TrimSpace(item.UsageHint), InputSummary: strings.TrimSpace(item.InputSummary),
			OutputSummary: strings.TrimSpace(item.OutputSummary), StageHints: filterEmptyStrings(item.StageHints), RiskLevel: strings.TrimSpace(item.RiskLevel),
			EffectScope: strings.TrimSpace(item.EffectScope), IdempotencyClass: strings.TrimSpace(item.IdempotencyClass),
			Schema: cloneContextMap(item.Schema), URI: strings.TrimSpace(item.URI), Enabled: item.Enabled,
			ApprovalPolicy: strings.TrimSpace(item.ApprovalPolicy), Status: strings.TrimSpace(item.Status),
		})
	}
	return defaults, nil
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

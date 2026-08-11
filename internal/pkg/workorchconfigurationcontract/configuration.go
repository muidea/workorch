// Package workorchconfigurationcontract defines complete, read-only and
// redacted configuration projections shared by server and clients.
package workorchconfigurationcontract

import (
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type ServiceConfiguration struct {
	Server     workorchconfig.ServerConfig     `json:"server"`
	Repository workorchconfig.RepositoryConfig `json:"repository"`
	Runtime    RuntimeConfiguration            `json:"runtime"`
	LLM        LLMConfiguration                `json:"llm"`
	WebSearch  WebSearchConfiguration          `json:"webSearch"`
	Governance GovernanceConfiguration         `json:"governance"`
}

type RuntimeConfiguration struct {
	DefaultSoulID                                string                                `json:"defaultSoulId,omitempty"`
	MaxRuntimeRepairAttempts                     int                                   `json:"maxRuntimeRepairAttempts"`
	MaxProviderRetryAttempts                     int                                   `json:"maxProviderRetryAttempts"`
	MaxContractViolationRetriesPerStep           int                                   `json:"maxContractViolationRetriesPerStep"`
	MaxContractViolationRetriesPerRun            int                                   `json:"maxContractViolationRetriesPerRun"`
	MaxRepeatedContractViolationSignature        int                                   `json:"maxRepeatedContractViolationSignature"`
	MaxConsecutiveContractViolationNoShapeChange int                                   `json:"maxConsecutiveContractViolationNoShapeChange"`
	ShellCommandTimeout                          string                                `json:"shellCommandTimeout"`
	Git                                          RuntimeGitConfiguration               `json:"git"`
	LLMOutput                                    workorchconfig.LLMOutputRuntimeConfig `json:"llmOutput"`
}

type RuntimeGitConfiguration struct {
	DefaultTimeout string `json:"defaultTimeout,omitempty"`
}

type LLMConfiguration struct {
	AIProxy          AIProxyConfigurationView                          `json:"aiProxy"`
	DefaultChat      runtimecontract.ProviderModelTarget               `json:"defaultChat"`
	DefaultEmbedding *runtimecontract.ProviderModelTarget              `json:"defaultEmbedding,omitempty"`
	Budget           workorchconfig.LLMContextBudgetPolicyConfig       `json:"budget"`
	Catalog          workorchmanagementcontract.ProviderCatalogSummary `json:"catalog"`
	RestartRequired  bool                                              `json:"restartRequired"`
}

type AIProxyConfigurationView struct {
	BaseURL                 string `json:"baseUrl"`
	APIKeyConfigured        bool   `json:"apiKeyConfigured"`
	Timeout                 string `json:"timeout,omitempty"`
	CatalogRequestTimeout   string `json:"catalogRequestTimeout,omitempty"`
	CatalogTTL              string `json:"catalogTTL,omitempty"`
	CatalogMaxStaleAge      string `json:"catalogMaxStaleAge,omitempty"`
	MaxCatalogResponseBytes int    `json:"maxCatalogResponseBytes,omitempty"`
	MaxCatalogModels        int    `json:"maxCatalogModels,omitempty"`
}

type WebSearchConfiguration struct {
	DefaultProviderID string                           `json:"defaultProviderId"`
	Providers         []WebSearchProviderConfiguration `json:"providers"`
	ProviderTemplates []WebSearchProviderTemplate      `json:"providerTemplates"`
}

type WebSearchProviderConfiguration struct {
	ID               string            `json:"id"`
	Provider         string            `json:"provider"`
	SearchEndpoint   string            `json:"searchEndpoint,omitempty"`
	FetchEndpoint    string            `json:"fetchEndpoint,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	Timeout          string            `json:"timeout,omitempty"`
	MaxResults       int               `json:"maxResults,omitempty"`
	MaxFetchBytes    int64             `json:"maxFetchBytes,omitempty"`
	MaxRedirects     int               `json:"maxRedirects,omitempty"`
	AllowDomains     []string          `json:"allowDomains,omitempty"`
	DenyDomains      []string          `json:"denyDomains,omitempty"`
	Disabled         bool              `json:"disabled"`
	Ready            bool              `json:"ready"`
	Reason           string            `json:"reason,omitempty"`
	Default          bool              `json:"default"`
	APIKeyConfigured bool              `json:"apiKeyConfigured"`
}

type WebSearchProviderTemplate struct {
	TemplateID     string `json:"templateId"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	ID             string `json:"id"`
	Provider       string `json:"provider"`
	Timeout        string `json:"timeout,omitempty"`
	MaxResults     int    `json:"maxResults,omitempty"`
	MaxFetchBytes  int64  `json:"maxFetchBytes,omitempty"`
	MaxRedirects   int    `json:"maxRedirects,omitempty"`
	SearchEndpoint string `json:"searchEndpoint,omitempty"`
	FetchEndpoint  string `json:"fetchEndpoint,omitempty"`
}

type GovernanceConfiguration struct {
	AutoTicket   GovernanceAutoTicketConfiguration     `json:"autoTicket"`
	AutoApproval GovernanceAutoApprovalConfiguration   `json:"autoApproval"`
	Budget       workorchconfig.GovernanceBudgetConfig `json:"budget"`
}

type GovernanceAutoTicketConfiguration struct {
	Enabled       bool    `json:"enabled"`
	MinConfidence float64 `json:"minConfidence"`
	MaxPerRun     int     `json:"maxPerRun"`
}

type GovernanceAutoApprovalConfiguration struct {
	Enabled             bool     `json:"enabled"`
	MinConfidence       float64  `json:"minConfidence"`
	MaxPerRun           int      `json:"maxPerRun"`
	AllowReadOnly       bool     `json:"allowReadOnly"`
	AllowWorkspaceWrite bool     `json:"allowWorkspaceWrite"`
	RequireHumanFor     []string `json:"requireHumanFor,omitempty"`
}

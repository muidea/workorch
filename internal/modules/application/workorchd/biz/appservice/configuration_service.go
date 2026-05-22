package appservice

import (
	"fmt"
	"sort"
	"strings"

	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	webbiz "github.com/muidea/workorch/internal/modules/blocks/web/biz"
	webcommon "github.com/muidea/workorch/internal/modules/blocks/web/pkg/common"
	orchestratorbiz "github.com/muidea/workorch/internal/modules/kernel/orchestrator/biz"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/dialoglanguage"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

type ServiceConfiguration struct {
	Server     ServerConfiguration     `json:"server"`
	Repository RepositoryConfiguration `json:"repository"`
	Runtime    RuntimeConfiguration    `json:"runtime"`
	LLM        LLMConfiguration        `json:"llm"`
	WebSearch  WebSearchConfiguration  `json:"webSearch"`
	Governance GovernanceConfiguration `json:"governance"`
}

type ServerConfiguration struct {
	Addr                  string `json:"addr"`
	ReadHeaderTimeout     string `json:"readHeaderTimeout"`
	ReadTimeout           string `json:"readTimeout"`
	WriteTimeout          string `json:"writeTimeout"`
	IdleTimeout           string `json:"idleTimeout"`
	ShutdownTimeout       string `json:"shutdownTimeout"`
	MaxConcurrentRuns     int64  `json:"maxConcurrentRuns"`
	DefaultDialogLanguage string `json:"defaultDialogLanguage,omitempty"`
}

type RepositoryConfiguration struct {
	Backend string `json:"backend"`
	DataDir string `json:"dataDir"`
}

type RuntimeConfiguration struct {
	DefaultSoulID            string `json:"defaultSoulId,omitempty"`
	MaxRuntimeRepairAttempts int    `json:"maxRuntimeRepairAttempts"`
	MaxProviderRetryAttempts int    `json:"maxProviderRetryAttempts"`
	ShellCommandTimeout      string `json:"shellCommandTimeout"`
}

type LLMConfiguration struct {
	DefaultProviderID          string                            `json:"defaultProviderId"`
	DefaultEmbeddingProviderID string                            `json:"defaultEmbeddingProviderId,omitempty"`
	ModelCapabilities          []LLMModelCapabilityConfiguration `json:"modelCapabilities,omitempty"`
	Providers                  []LLMProviderConfiguration        `json:"providers"`
	ProviderTemplates          []LLMProviderTemplate             `json:"providerTemplates"`
}

type LLMModelCapabilityConfiguration struct {
	ID                  string `json:"id"`
	Provider            string `json:"provider,omitempty"`
	ModelPattern        string `json:"modelPattern"`
	ContextWindowTokens int    `json:"contextWindowTokens"`
}

type LLMProviderConfiguration struct {
	ID                string                             `json:"id"`
	Provider          string                             `json:"provider,omitempty"`
	BaseURL           string                             `json:"baseUrl,omitempty"`
	Model             string                             `json:"model,omitempty"`
	Timeout           string                             `json:"timeout,omitempty"`
	Disabled          bool                               `json:"disabled"`
	Ready             bool                               `json:"ready"`
	Reason            string                             `json:"reason,omitempty"`
	Default           bool                               `json:"default"`
	APIKeyConfigured  bool                               `json:"apiKeyConfigured"`
	ChatCompletions   bool                               `json:"chatCompletions"`
	Streaming         bool                               `json:"streaming"`
	NonChatOperations []string                           `json:"nonChatOperations,omitempty"`
	FailoverEnabled   bool                               `json:"failoverEnabled"`
	FailoverProviders []string                           `json:"failoverProviders,omitempty"`
	InferenceProfile  llmmodels.ProviderInferenceProfile `json:"inferenceProfile,omitempty"`
	ContextProfile    llmmodels.ProviderContextProfile   `json:"contextProfile,omitempty"`
}

type LLMProviderTemplate struct {
	TemplateID        string   `json:"templateId"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	ID                string   `json:"id"`
	Provider          string   `json:"provider"`
	BaseURL           string   `json:"baseUrl,omitempty"`
	Model             string   `json:"model,omitempty"`
	Timeout           string   `json:"timeout,omitempty"`
	Disabled          bool     `json:"disabled"`
	ChatCompletions   bool     `json:"chatCompletions"`
	Streaming         bool     `json:"streaming"`
	NonChatOperations []string `json:"nonChatOperations,omitempty"`
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
	AutoTicket   GovernanceAutoTicketConfiguration   `json:"autoTicket"`
	AutoApproval GovernanceAutoApprovalConfiguration `json:"autoApproval"`
	Budget       GovernanceBudgetConfiguration       `json:"budget"`
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

type GovernanceBudgetConfiguration struct {
	MaxConsecutiveFailures int `json:"maxConsecutiveFailures"`
	MaxRepeatedSamePrompt  int `json:"maxRepeatedSamePrompt"`
}

type UpdateLLMConfigurationCommand struct {
	DefaultProviderID          *string                            `json:"defaultProviderId,omitempty"`
	DefaultEmbeddingProviderID *string                            `json:"defaultEmbeddingProviderId,omitempty"`
	ModelCapabilities          *[]LLMModelCapabilityConfiguration `json:"modelCapabilities,omitempty"`
	Providers                  *[]UpdateLLMProviderConfiguration  `json:"providers,omitempty"`
}

type UpdateServerConfigurationCommand struct {
	DefaultDialogLanguage *string `json:"defaultDialogLanguage,omitempty"`
}

type UpdateLLMProviderConfiguration struct {
	ID                string                             `json:"id"`
	Provider          *string                            `json:"provider,omitempty"`
	BaseURL           *string                            `json:"baseUrl,omitempty"`
	APIKey            *string                            `json:"apiKey,omitempty"`
	Model             *string                            `json:"model,omitempty"`
	Timeout           *string                            `json:"timeout,omitempty"`
	Disabled          *bool                              `json:"disabled,omitempty"`
	ChatCompletions   *bool                              `json:"chatCompletions,omitempty"`
	Streaming         *bool                              `json:"streaming,omitempty"`
	NonChatOperations []string                           `json:"nonChatOperations,omitempty"`
	FailoverEnabled   *bool                              `json:"failoverEnabled,omitempty"`
	FailoverProviders []string                           `json:"failoverProviders,omitempty"`
	InferenceProfile  *UpdateLLMProviderInferenceProfile `json:"inferenceProfile,omitempty"`
	ContextProfile    *UpdateLLMProviderContextProfile   `json:"contextProfile,omitempty"`
}

type UpdateLLMProviderInferenceProfile struct {
	Temperature          *float64 `json:"temperature,omitempty"`
	MaxOutputTokens      *int     `json:"maxOutputTokens,omitempty"`
	ReasoningEffort      *string  `json:"reasoningEffort,omitempty"`
	ThinkingBudgetTokens *int     `json:"thinkingBudgetTokens,omitempty"`
}

type UpdateLLMProviderContextProfile struct {
	ProfileMode             *string  `json:"profileMode,omitempty"`
	ContextWindowTokens     *int     `json:"contextWindowTokens,omitempty"`
	MaxInputTokens          *int     `json:"maxInputTokens,omitempty"`
	ReservedOutputTokens    *int     `json:"reservedOutputTokens,omitempty"`
	SafetyRatio             *float64 `json:"safetyRatio,omitempty"`
	ContextUtilizationRatio *float64 `json:"contextUtilizationRatio,omitempty"`
	BytesHardLimit          *int     `json:"bytesHardLimit,omitempty"`
	MaxRequestBytes         *int     `json:"maxRequestBytes,omitempty"`
	EstimatedTokensPerByte  *float64 `json:"estimatedTokensPerByte,omitempty"`
	InputTokenPrice         *float64 `json:"inputTokenPrice,omitempty"`
	OutputTokenPrice        *float64 `json:"outputTokenPrice,omitempty"`
	Currency                *string  `json:"currency,omitempty"`
}

type UpdateWebSearchConfigurationCommand struct {
	DefaultProviderID *string                                 `json:"defaultProviderId,omitempty"`
	Providers         *[]UpdateWebSearchProviderConfiguration `json:"providers,omitempty"`
}

type UpdateWebSearchProviderConfiguration struct {
	ID             string            `json:"id"`
	Provider       *string           `json:"provider,omitempty"`
	SearchEndpoint *string           `json:"searchEndpoint,omitempty"`
	FetchEndpoint  *string           `json:"fetchEndpoint,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	APIKey         *string           `json:"apiKey,omitempty"`
	Timeout        *string           `json:"timeout,omitempty"`
	MaxResults     *int              `json:"maxResults,omitempty"`
	MaxFetchBytes  *int64            `json:"maxFetchBytes,omitempty"`
	MaxRedirects   *int              `json:"maxRedirects,omitempty"`
	AllowDomains   []string          `json:"allowDomains,omitempty"`
	DenyDomains    []string          `json:"denyDomains,omitempty"`
	Disabled       *bool             `json:"disabled,omitempty"`
}

func ReadServiceConfiguration() (ServiceConfiguration, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return ServiceConfiguration{}, err
	}
	defaultProviderID, providerConfigs, err := llmbiz.LoadProviderConfigs(cfg.LLM.DefaultProvider, cfg.LLMProviderValueSets())
	if err != nil {
		return ServiceConfiguration{}, err
	}
	providers := make([]LLMProviderConfiguration, 0, len(providerConfigs))
	for id, providerConfig := range providerConfigs {
		redacted := llmbiz.RedactedProviderConfig(providerConfig)
		rawProvider := cfg.LLM.Providers[id]
		providers = append(providers, LLMProviderConfiguration{
			ID:                redacted.ID,
			Provider:          redacted.Provider,
			BaseURL:           redacted.BaseURL,
			Model:             redacted.Model,
			Timeout:           redacted.Timeout,
			Disabled:          redacted.Disabled,
			Ready:             redacted.Ready,
			Reason:            redacted.Reason,
			Default:           id == defaultProviderID,
			APIKeyConfigured:  rawProvider.APIKey != "",
			ChatCompletions:   redacted.Capabilities.ChatCompletions,
			Streaming:         redacted.Capabilities.Streaming,
			NonChatOperations: append([]string(nil), redacted.Capabilities.NonChatOperations...),
			FailoverEnabled:   redacted.Failover.Enabled,
			FailoverProviders: append([]string(nil), redacted.Failover.Providers...),
			InferenceProfile:  redacted.InferenceProfile,
			ContextProfile:    redacted.ContextProfile,
		})
	}
	sort.Slice(providers, func(i, j int) bool {
		if providers[i].Default != providers[j].Default {
			return providers[i].Default
		}
		return providers[i].ID < providers[j].ID
	})
	defaultWebProviderID, webProviderConfigs, err := webbiz.LoadProviderConfigs(cfg.WebSearch.DefaultProvider, cfg.WebSearchProviderValueSets())
	if err != nil {
		return ServiceConfiguration{}, err
	}
	webProviders := make([]WebSearchProviderConfiguration, 0, len(webProviderConfigs))
	for id, providerConfig := range webProviderConfigs {
		rawProvider := cfg.WebSearch.Providers[id]
		webProviders = append(webProviders, WebSearchProviderConfiguration{
			ID:               id,
			Provider:         providerConfig.Provider,
			SearchEndpoint:   providerConfig.SearchEndpoint,
			FetchEndpoint:    providerConfig.FetchEndpoint,
			Headers:          cloneStringMap(rawProvider.Headers),
			Timeout:          rawProvider.Timeout,
			MaxResults:       rawProvider.MaxResults,
			MaxFetchBytes:    rawProvider.MaxFetchBytes,
			MaxRedirects:     rawProvider.MaxRedirects,
			AllowDomains:     append([]string(nil), rawProvider.AllowDomains...),
			DenyDomains:      append([]string(nil), rawProvider.DenyDomains...),
			Disabled:         rawProvider.Disabled != nil && *rawProvider.Disabled,
			Ready:            !providerConfig.Unavailable,
			Reason:           providerConfig.UnavailableReason,
			Default:          id == defaultWebProviderID,
			APIKeyConfigured: rawProvider.APIKey != "",
		})
	}
	sort.Slice(webProviders, func(i, j int) bool {
		if webProviders[i].Default != webProviders[j].Default {
			return webProviders[i].Default
		}
		return webProviders[i].ID < webProviders[j].ID
	})
	return ServiceConfiguration{
		Server: ServerConfiguration{
			Addr:                  cfg.Server.Addr,
			ReadHeaderTimeout:     cfg.Server.ReadHeaderTimeout,
			ReadTimeout:           cfg.Server.ReadTimeout,
			WriteTimeout:          cfg.Server.WriteTimeout,
			IdleTimeout:           cfg.Server.IdleTimeout,
			ShutdownTimeout:       cfg.Server.ShutdownTimeout,
			MaxConcurrentRuns:     cfg.Server.MaxConcurrentRuns,
			DefaultDialogLanguage: strings.TrimSpace(cfg.Server.DefaultDialogLanguage),
		},
		Repository: RepositoryConfiguration{
			Backend: cfg.Repository.Backend,
			DataDir: cfg.Repository.DataDir,
		},
		Runtime: RuntimeConfiguration{
			DefaultSoulID:            strings.TrimSpace(cfg.Runtime.DefaultSoulID),
			MaxRuntimeRepairAttempts: orchestratorbiz.NormalizeRuntimePolicy(orchestratorbiz.RuntimePolicy{MaxRuntimeRepairAttempts: cfg.Runtime.MaxRuntimeRepairAttempts}).MaxRuntimeRepairAttempts,
			MaxProviderRetryAttempts: orchestratorbiz.NormalizeRuntimePolicy(orchestratorbiz.RuntimePolicy{MaxProviderRetryAttempts: cfg.Runtime.MaxProviderRetryAttempts}).MaxProviderRetryAttempts,
			ShellCommandTimeout:      runtimeShellCommandTimeout(cfg.Runtime.ShellCommandTimeout),
		},
		LLM: LLMConfiguration{
			DefaultProviderID:          defaultProviderID,
			DefaultEmbeddingProviderID: strings.TrimSpace(cfg.LLM.DefaultEmbeddingProvider),
			ModelCapabilities:          modelCapabilityConfigurations(cfg.LLM.ModelCapabilities),
			Providers:                  providers,
			ProviderTemplates:          DefaultLLMProviderTemplates(),
		},
		WebSearch: WebSearchConfiguration{
			DefaultProviderID: defaultWebProviderID,
			Providers:         webProviders,
			ProviderTemplates: DefaultWebSearchProviderTemplates(),
		},
		Governance: GovernanceConfiguration{
			AutoTicket: GovernanceAutoTicketConfiguration{
				Enabled:       isEnabled(cfg.Governance.AutoTicket.Enabled),
				MinConfidence: cfg.Governance.AutoTicket.MinConfidence,
				MaxPerRun:     cfg.Governance.AutoTicket.MaxPerRun,
			},
			AutoApproval: GovernanceAutoApprovalConfiguration{
				Enabled:             isEnabled(cfg.Governance.AutoApproval.Enabled),
				MinConfidence:       cfg.Governance.AutoApproval.MinConfidence,
				MaxPerRun:           cfg.Governance.AutoApproval.MaxPerRun,
				AllowReadOnly:       isEnabled(cfg.Governance.AutoApproval.AllowReadOnly),
				AllowWorkspaceWrite: isEnabled(cfg.Governance.AutoApproval.AllowWorkspaceWrite),
				RequireHumanFor:     append([]string(nil), cfg.Governance.AutoApproval.RequireHumanFor...),
			},
			Budget: GovernanceBudgetConfiguration{
				MaxConsecutiveFailures: cfg.Governance.Budget.MaxConsecutiveFailures,
				MaxRepeatedSamePrompt:  cfg.Governance.Budget.MaxRepeatedSamePrompt,
			},
		},
	}, nil
}

func runtimeShellCommandTimeout(configured string) string {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		return configured
	}
	return "60s"
}

func DefaultLLMProviderTemplates() []LLMProviderTemplate {
	return []LLMProviderTemplate{
		{
			TemplateID:      llmcommon.ProviderOpenAI,
			Name:            "OpenAI API",
			Description:     "使用 OpenAI 官方 SDK 调用 Chat Completions；DeepSeek、MiniMax 等可直接使用 OpenAI SDK 的服务应选择该模式并配置各自 baseURL。",
			ID:              "openai-main",
			Provider:        llmcommon.ProviderOpenAI,
			BaseURL:         "https://api.openai.com/v1",
			Model:           "gpt-4.1-mini",
			Timeout:         "60s",
			ChatCompletions: true,
		},
		{
			TemplateID:      llmcommon.ProviderAnthropic,
			Name:            "Anthropic API",
			Description:     "使用 Anthropic 官方 SDK 调用 Messages API；当前不作为 Embedding Provider。",
			ID:              "anthropic-main",
			Provider:        llmcommon.ProviderAnthropic,
			BaseURL:         "https://api.anthropic.com",
			Model:           "claude-sonnet-4-5-20250929",
			Timeout:         "60s",
			ChatCompletions: true,
		},
		{
			TemplateID:      llmcommon.ProviderCompatible,
			Name:            "OpenAI Compatible API",
			Description:     "用于未接入官方 SDK、但暴露 OpenAI-compatible HTTP 接口的本地或自定义服务，继续走轻量 HTTP 兼容实现。",
			ID:              "compatible-main",
			Provider:        llmcommon.ProviderCompatible,
			BaseURL:         "http://127.0.0.1:11434/v1",
			Model:           "deepseek-chat",
			Timeout:         "60s",
			ChatCompletions: true,
		},
	}
}

func DefaultWebSearchProviderTemplates() []WebSearchProviderTemplate {
	return []WebSearchProviderTemplate{
		{
			TemplateID:    webcommon.ProviderHTTP,
			Name:          "HTTP Web Search Gateway",
			Description:   "通过受治理的 HTTP 搜索/读取网关执行 web/search 与 web/fetch；网关负责真实搜索源和网页正文抽取。",
			ID:            "web-http-main",
			Provider:      webcommon.ProviderHTTP,
			Timeout:       "20s",
			MaxResults:    8,
			MaxFetchBytes: 262144,
			MaxRedirects:  3,
		},
	}
}

func UpdateServerConfiguration(cmd UpdateServerConfigurationCommand) (ServiceConfiguration, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return ServiceConfiguration{}, err
	}
	if cmd.DefaultDialogLanguage == nil {
		return ReadServiceConfiguration()
	}
	language := strings.TrimSpace(*cmd.DefaultDialogLanguage)
	if language != "" {
		if !dialoglanguage.IsSupported(language) {
			return ServiceConfiguration{}, fmt.Errorf("%w: defaultDialogLanguage only supports zh-CN or en", common.ErrInvalidArgument)
		}
		language = dialoglanguage.NormalizeTag(language)
	}
	cfg.Server.DefaultDialogLanguage = language
	if err := workorchconfig.Save(cfg); err != nil {
		return ServiceConfiguration{}, err
	}
	if err := workorchconfig.Reload(); err != nil {
		return ServiceConfiguration{}, err
	}
	return ReadServiceConfiguration()
}

func UpdateLLMConfiguration(cmd UpdateLLMConfigurationCommand) (ServiceConfiguration, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return ServiceConfiguration{}, err
	}
	providers := cloneLLMProviderConfigs(cfg.LLM.Providers)
	if cmd.Providers != nil {
		providers, err = updatedLLMProviders(cfg.LLM.Providers, *cmd.Providers)
		if err != nil {
			return ServiceConfiguration{}, err
		}
	}
	modelCapabilities := cloneLLMModelCapabilityConfigs(cfg.LLM.ModelCapabilities)
	if cmd.ModelCapabilities != nil {
		modelCapabilities, err = updatedLLMModelCapabilities(*cmd.ModelCapabilities)
		if err != nil {
			return ServiceConfiguration{}, err
		}
	}
	defaultProviderID := strings.TrimSpace(cfg.LLM.DefaultProvider)
	if cmd.DefaultProviderID != nil {
		defaultProviderID = strings.TrimSpace(*cmd.DefaultProviderID)
	}
	if defaultProviderID == "" {
		return ServiceConfiguration{}, fmt.Errorf("%w: defaultProviderId is required", common.ErrInvalidArgument)
	}
	if _, ok := providers[defaultProviderID]; !ok {
		return ServiceConfiguration{}, fmt.Errorf("%w: defaultProviderId must reference an LLM provider", common.ErrInvalidArgument)
	}
	defaultEmbeddingProviderID := strings.TrimSpace(cfg.LLM.DefaultEmbeddingProvider)
	if cmd.DefaultEmbeddingProviderID != nil {
		defaultEmbeddingProviderID = strings.TrimSpace(*cmd.DefaultEmbeddingProviderID)
	}
	if defaultEmbeddingProviderID != "" {
		provider, ok := providers[defaultEmbeddingProviderID]
		if !ok {
			return ServiceConfiguration{}, fmt.Errorf("%w: defaultEmbeddingProviderId must reference an LLM provider", common.ErrInvalidArgument)
		}
		if !providerSupportsOperation(provider, llmcommon.ProviderOperationEmbeddings) {
			return ServiceConfiguration{}, fmt.Errorf("%w: defaultEmbeddingProviderId must reference a provider with embeddings capability", common.ErrInvalidArgument)
		}
		loadedProvider, err := loadUpdatedProviderConfig(defaultEmbeddingProviderID, provider)
		if err != nil {
			return ServiceConfiguration{}, err
		}
		if loadedProvider.Unavailable {
			return ServiceConfiguration{}, fmt.Errorf("%w: defaultEmbeddingProviderId references an unavailable provider: %s", common.ErrInvalidArgument, loadedProvider.UnavailableReason)
		}
	}
	cfg.LLM.DefaultProvider = defaultProviderID
	cfg.LLM.DefaultEmbeddingProvider = defaultEmbeddingProviderID
	cfg.LLM.ModelCapabilities = modelCapabilities
	cfg.LLM.Providers = providers
	if err := workorchconfig.Save(cfg); err != nil {
		return ServiceConfiguration{}, err
	}
	if err := workorchconfig.Reload(); err != nil {
		return ServiceConfiguration{}, err
	}
	return ReadServiceConfiguration()
}

func UpdateWebSearchConfiguration(cmd UpdateWebSearchConfigurationCommand) (ServiceConfiguration, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return ServiceConfiguration{}, err
	}
	providers := cloneWebSearchProviderConfigs(cfg.WebSearch.Providers)
	if cmd.Providers != nil {
		providers, err = updatedWebSearchProviders(cfg.WebSearch.Providers, *cmd.Providers)
		if err != nil {
			return ServiceConfiguration{}, err
		}
	}
	if len(providers) == 0 {
		cfg.WebSearch.DefaultProvider = ""
		cfg.WebSearch.Providers = map[string]workorchconfig.WebSearchProviderConfig{}
		if err := workorchconfig.Save(cfg); err != nil {
			return ServiceConfiguration{}, err
		}
		if err := workorchconfig.Reload(); err != nil {
			return ServiceConfiguration{}, err
		}
		return ReadServiceConfiguration()
	}
	defaultProviderID := strings.TrimSpace(cfg.WebSearch.DefaultProvider)
	if cmd.DefaultProviderID != nil {
		defaultProviderID = strings.TrimSpace(*cmd.DefaultProviderID)
	}
	if defaultProviderID == "" {
		return ServiceConfiguration{}, fmt.Errorf("%w: defaultProviderId is required", common.ErrInvalidArgument)
	}
	if _, ok := providers[defaultProviderID]; !ok {
		return ServiceConfiguration{}, fmt.Errorf("%w: defaultProviderId must reference a Web Search provider", common.ErrInvalidArgument)
	}
	if _, loaded, err := webbiz.LoadProviderConfigs(defaultProviderID, workorchconfig.Config{
		WebSearch: workorchconfig.WebSearchConfig{
			DefaultProvider: defaultProviderID,
			Providers:       providers,
		},
	}.WebSearchProviderValueSets()); err != nil {
		return ServiceConfiguration{}, err
	} else if provider := loaded[defaultProviderID]; provider.Unavailable {
		return ServiceConfiguration{}, fmt.Errorf("%w: defaultProviderId references an unavailable Web Search provider: %s", common.ErrInvalidArgument, provider.UnavailableReason)
	}
	cfg.WebSearch.DefaultProvider = defaultProviderID
	cfg.WebSearch.Providers = providers
	if err := workorchconfig.Save(cfg); err != nil {
		return ServiceConfiguration{}, err
	}
	if err := workorchconfig.Reload(); err != nil {
		return ServiceConfiguration{}, err
	}
	return ReadServiceConfiguration()
}

func loadUpdatedProviderConfig(providerID string, provider workorchconfig.LLMProviderConfig) (llmmodels.ProviderConfig, error) {
	values := workorchconfig.Config{
		LLM: workorchconfig.LLMConfig{
			Providers: map[string]workorchconfig.LLMProviderConfig{
				providerID: provider,
			},
		},
	}.LLMProviderValueSets()[providerID]
	return llmbiz.LoadProviderConfig(providerID, values)
}

func providerSupportsOperation(provider workorchconfig.LLMProviderConfig, operation string) bool {
	for _, item := range provider.NonChatOperations {
		if strings.TrimSpace(item) == operation {
			return true
		}
	}
	return false
}

func updatedLLMProviders(existing map[string]workorchconfig.LLMProviderConfig, updates []UpdateLLMProviderConfiguration) (map[string]workorchconfig.LLMProviderConfig, error) {
	if len(updates) == 0 {
		return nil, fmt.Errorf("%w: at least one LLM provider is required", common.ErrInvalidArgument)
	}
	ret := make(map[string]workorchconfig.LLMProviderConfig, len(updates))
	for _, update := range updates {
		id := strings.TrimSpace(update.ID)
		if id == "" {
			return nil, fmt.Errorf("%w: LLM provider id is required", common.ErrInvalidArgument)
		}
		if _, exists := ret[id]; exists {
			return nil, fmt.Errorf("%w: duplicate LLM provider id %q", common.ErrInvalidArgument, id)
		}
		next := cloneLLMProviderConfig(existing[id])
		if update.Provider != nil {
			next.Provider = strings.TrimSpace(*update.Provider)
		}
		if update.BaseURL != nil {
			next.BaseURL = strings.TrimSpace(*update.BaseURL)
		}
		if update.Model != nil {
			next.Model = strings.TrimSpace(*update.Model)
		}
		if update.Timeout != nil {
			next.Timeout = strings.TrimSpace(*update.Timeout)
		}
		if update.APIKey != nil {
			if apiKey := strings.TrimSpace(*update.APIKey); apiKey != "" {
				next.APIKey = apiKey
			}
		}
		if update.Disabled != nil {
			next.Disabled = boolPtr(*update.Disabled)
		} else if next.Disabled == nil {
			next.Disabled = boolPtr(false)
		}
		if update.ChatCompletions != nil {
			next.ChatCompletions = boolPtr(*update.ChatCompletions)
		} else if next.ChatCompletions == nil {
			next.ChatCompletions = boolPtr(true)
		}
		if update.Streaming != nil {
			next.Streaming = boolPtr(*update.Streaming)
		} else if next.Streaming == nil {
			next.Streaming = boolPtr(false)
		}
		if update.NonChatOperations != nil {
			next.NonChatOperations = normalizedProviderOperations(update.NonChatOperations)
		}
		if update.FailoverEnabled != nil {
			next.FailoverEnabled = boolPtr(*update.FailoverEnabled)
		} else if next.FailoverEnabled == nil {
			next.FailoverEnabled = boolPtr(false)
		}
		if update.FailoverProviders != nil {
			next.FailoverProviders = normalizedProviderOperations(update.FailoverProviders)
		}
		if update.InferenceProfile != nil {
			applyLLMProviderInferenceProfileUpdate(&next, update.InferenceProfile)
		}
		if update.ContextProfile != nil {
			applyLLMProviderContextProfileUpdate(&next, update.ContextProfile)
		}
		simplifyAutoLLMProviderContextProfile(&next)
		ret[id] = next
	}
	return ret, nil
}

func applyLLMProviderInferenceProfileUpdate(next *workorchconfig.LLMProviderConfig, update *UpdateLLMProviderInferenceProfile) {
	if next == nil || update == nil {
		return
	}
	if update.Temperature != nil {
		next.Temperature = cloneFloat64Ptr(update.Temperature)
	}
	if update.MaxOutputTokens != nil {
		next.MaxOutputTokens = *update.MaxOutputTokens
	}
	if update.ReasoningEffort != nil {
		next.ReasoningEffort = strings.TrimSpace(*update.ReasoningEffort)
	}
	if update.ThinkingBudgetTokens != nil {
		next.ThinkingBudgetTokens = *update.ThinkingBudgetTokens
	}
}

func applyLLMProviderContextProfileUpdate(next *workorchconfig.LLMProviderConfig, update *UpdateLLMProviderContextProfile) {
	if next == nil || update == nil {
		return
	}
	if update.ProfileMode != nil {
		next.ContextProfileMode = strings.TrimSpace(*update.ProfileMode)
	}
	if update.ContextWindowTokens != nil {
		next.ContextWindowTokens = *update.ContextWindowTokens
	}
	if update.MaxInputTokens != nil {
		next.MaxInputTokens = *update.MaxInputTokens
	}
	if update.ReservedOutputTokens != nil {
		next.ReservedOutputTokens = *update.ReservedOutputTokens
	}
	if update.SafetyRatio != nil {
		next.SafetyRatio = *update.SafetyRatio
	}
	if update.ContextUtilizationRatio != nil {
		next.ContextUtilizationRatio = *update.ContextUtilizationRatio
	}
	if update.BytesHardLimit != nil {
		next.BytesHardLimit = *update.BytesHardLimit
	}
	if update.MaxRequestBytes != nil {
		next.MaxRequestBytes = *update.MaxRequestBytes
	}
	if update.EstimatedTokensPerByte != nil {
		next.EstimatedTokensPerByte = *update.EstimatedTokensPerByte
	}
	if update.InputTokenPrice != nil {
		next.InputTokenPrice = *update.InputTokenPrice
	}
	if update.OutputTokenPrice != nil {
		next.OutputTokenPrice = *update.OutputTokenPrice
	}
	if update.Currency != nil {
		next.Currency = strings.TrimSpace(*update.Currency)
	}
}

func simplifyAutoLLMProviderContextProfile(next *workorchconfig.LLMProviderConfig) {
	if next == nil || strings.EqualFold(strings.TrimSpace(next.ContextProfileMode), "manual") {
		return
	}
	if strings.EqualFold(strings.TrimSpace(next.ContextProfileMode), "auto") {
		next.ContextProfileMode = ""
	}
	next.MaxInputTokens = 0
	next.ReservedOutputTokens = 0
	next.SafetyRatio = 0
	next.ContextUtilizationRatio = 0
	next.BytesHardLimit = 0
	next.MaxRequestBytes = 0
	next.EstimatedTokensPerByte = 0
}

func normalizedProviderOperations(values []string) []string {
	ret := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		ret = append(ret, value)
	}
	return ret
}

func modelCapabilityConfigurations(source map[string]workorchconfig.LLMModelCapabilityConfig) []LLMModelCapabilityConfiguration {
	ret := make([]LLMModelCapabilityConfiguration, 0, len(source))
	for id, capability := range source {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		ret = append(ret, LLMModelCapabilityConfiguration{
			ID:                  id,
			Provider:            strings.TrimSpace(capability.Provider),
			ModelPattern:        strings.TrimSpace(capability.ModelPattern),
			ContextWindowTokens: capability.ContextWindowTokens,
		})
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i].ID < ret[j].ID })
	return ret
}

func updatedLLMModelCapabilities(updates []LLMModelCapabilityConfiguration) (map[string]workorchconfig.LLMModelCapabilityConfig, error) {
	ret := make(map[string]workorchconfig.LLMModelCapabilityConfig, len(updates))
	for _, update := range updates {
		id := strings.TrimSpace(update.ID)
		if id == "" {
			return nil, fmt.Errorf("%w: LLM model capability id is required", common.ErrInvalidArgument)
		}
		if _, exists := ret[id]; exists {
			return nil, fmt.Errorf("%w: duplicate LLM model capability id %q", common.ErrInvalidArgument, id)
		}
		modelPattern := strings.TrimSpace(update.ModelPattern)
		if modelPattern == "" {
			return nil, fmt.Errorf("%w: LLM model capability modelPattern is required", common.ErrInvalidArgument)
		}
		if update.ContextWindowTokens <= 0 {
			return nil, fmt.Errorf("%w: LLM model capability contextWindowTokens must be positive", common.ErrInvalidArgument)
		}
		ret[id] = workorchconfig.LLMModelCapabilityConfig{
			Provider:            strings.TrimSpace(update.Provider),
			ModelPattern:        modelPattern,
			ContextWindowTokens: update.ContextWindowTokens,
		}
	}
	return ret, nil
}

func updatedWebSearchProviders(existing map[string]workorchconfig.WebSearchProviderConfig, updates []UpdateWebSearchProviderConfiguration) (map[string]workorchconfig.WebSearchProviderConfig, error) {
	ret := make(map[string]workorchconfig.WebSearchProviderConfig, len(updates))
	for _, update := range updates {
		id := strings.TrimSpace(update.ID)
		if id == "" {
			return nil, fmt.Errorf("%w: Web Search provider id is required", common.ErrInvalidArgument)
		}
		if _, exists := ret[id]; exists {
			return nil, fmt.Errorf("%w: duplicate Web Search provider id %q", common.ErrInvalidArgument, id)
		}
		next := cloneWebSearchProviderConfig(existing[id])
		if update.Provider != nil {
			next.Provider = strings.TrimSpace(*update.Provider)
		}
		if update.SearchEndpoint != nil {
			next.SearchEndpoint = strings.TrimSpace(*update.SearchEndpoint)
		}
		if update.FetchEndpoint != nil {
			next.FetchEndpoint = strings.TrimSpace(*update.FetchEndpoint)
		}
		if update.APIKey != nil {
			if apiKey := strings.TrimSpace(*update.APIKey); apiKey != "" {
				next.APIKey = apiKey
			}
		}
		if update.Headers != nil {
			next.Headers = cloneStringMap(update.Headers)
		}
		if update.Timeout != nil {
			next.Timeout = strings.TrimSpace(*update.Timeout)
		}
		if update.MaxResults != nil {
			next.MaxResults = *update.MaxResults
		}
		if update.MaxFetchBytes != nil {
			next.MaxFetchBytes = *update.MaxFetchBytes
		}
		if update.MaxRedirects != nil {
			next.MaxRedirects = *update.MaxRedirects
		}
		if update.AllowDomains != nil {
			next.AllowDomains = normalizedProviderOperations(update.AllowDomains)
		}
		if update.DenyDomains != nil {
			next.DenyDomains = normalizedProviderOperations(update.DenyDomains)
		}
		if update.Disabled != nil {
			next.Disabled = boolPtr(*update.Disabled)
		} else if next.Disabled == nil {
			next.Disabled = boolPtr(false)
		}
		ret[id] = next
	}
	return ret, nil
}

func boolPtr(value bool) *bool {
	return &value
}

func isEnabled(value *bool) bool {
	return value != nil && *value
}

func cloneLLMProviderConfigs(source map[string]workorchconfig.LLMProviderConfig) map[string]workorchconfig.LLMProviderConfig {
	if len(source) == 0 {
		return map[string]workorchconfig.LLMProviderConfig{}
	}
	ret := make(map[string]workorchconfig.LLMProviderConfig, len(source))
	for id, provider := range source {
		ret[id] = cloneLLMProviderConfig(provider)
	}
	return ret
}

func cloneLLMModelCapabilityConfigs(source map[string]workorchconfig.LLMModelCapabilityConfig) map[string]workorchconfig.LLMModelCapabilityConfig {
	if len(source) == 0 {
		return map[string]workorchconfig.LLMModelCapabilityConfig{}
	}
	ret := make(map[string]workorchconfig.LLMModelCapabilityConfig, len(source))
	for id, capability := range source {
		ret[id] = workorchconfig.LLMModelCapabilityConfig{
			Provider:            capability.Provider,
			ModelPattern:        capability.ModelPattern,
			ContextWindowTokens: capability.ContextWindowTokens,
		}
	}
	return ret
}

func cloneLLMProviderConfig(source workorchconfig.LLMProviderConfig) workorchconfig.LLMProviderConfig {
	return workorchconfig.LLMProviderConfig{
		Provider:                source.Provider,
		BaseURL:                 source.BaseURL,
		APIKey:                  source.APIKey,
		Model:                   source.Model,
		Timeout:                 source.Timeout,
		Disabled:                cloneBoolPtr(source.Disabled),
		ChatCompletions:         cloneBoolPtr(source.ChatCompletions),
		Streaming:               cloneBoolPtr(source.Streaming),
		NonChatOperations:       append([]string(nil), source.NonChatOperations...),
		FailoverEnabled:         cloneBoolPtr(source.FailoverEnabled),
		FailoverProviders:       append([]string(nil), source.FailoverProviders...),
		Temperature:             cloneFloat64Ptr(source.Temperature),
		MaxOutputTokens:         source.MaxOutputTokens,
		ReasoningEffort:         source.ReasoningEffort,
		ThinkingBudgetTokens:    source.ThinkingBudgetTokens,
		ContextProfileMode:      source.ContextProfileMode,
		ContextWindowTokens:     source.ContextWindowTokens,
		MaxInputTokens:          source.MaxInputTokens,
		ReservedOutputTokens:    source.ReservedOutputTokens,
		SafetyRatio:             source.SafetyRatio,
		ContextUtilizationRatio: source.ContextUtilizationRatio,
		BytesHardLimit:          source.BytesHardLimit,
		MaxRequestBytes:         source.MaxRequestBytes,
		EstimatedTokensPerByte:  source.EstimatedTokensPerByte,
		InputTokenPrice:         source.InputTokenPrice,
		OutputTokenPrice:        source.OutputTokenPrice,
		Currency:                source.Currency,
	}
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneWebSearchProviderConfigs(source map[string]workorchconfig.WebSearchProviderConfig) map[string]workorchconfig.WebSearchProviderConfig {
	if len(source) == 0 {
		return map[string]workorchconfig.WebSearchProviderConfig{}
	}
	ret := make(map[string]workorchconfig.WebSearchProviderConfig, len(source))
	for id, provider := range source {
		ret[id] = cloneWebSearchProviderConfig(provider)
	}
	return ret
}

func cloneWebSearchProviderConfig(source workorchconfig.WebSearchProviderConfig) workorchconfig.WebSearchProviderConfig {
	return workorchconfig.WebSearchProviderConfig{
		Provider:       source.Provider,
		SearchEndpoint: source.SearchEndpoint,
		FetchEndpoint:  source.FetchEndpoint,
		APIKey:         source.APIKey,
		Headers:        cloneStringMap(source.Headers),
		Timeout:        source.Timeout,
		MaxResults:     source.MaxResults,
		MaxFetchBytes:  source.MaxFetchBytes,
		MaxRedirects:   source.MaxRedirects,
		AllowDomains:   append([]string(nil), source.AllowDomains...),
		DenyDomains:    append([]string(nil), source.DenyDomains...),
		Disabled:       cloneBoolPtr(source.Disabled),
	}
}

func cloneBoolPtr(source *bool) *bool {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}

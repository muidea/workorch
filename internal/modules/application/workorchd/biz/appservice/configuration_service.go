package appservice

import (
	"fmt"
	"sort"
	"strings"

	llmbiz "github.com/muidea/workorch/internal/modules/blocks/llm/biz"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

type ServiceConfiguration struct {
	Server     ServerConfiguration     `json:"server"`
	Repository RepositoryConfiguration `json:"repository"`
	LLM        LLMConfiguration        `json:"llm"`
}

type ServerConfiguration struct {
	Addr              string `json:"addr"`
	ReadHeaderTimeout string `json:"readHeaderTimeout"`
	ReadTimeout       string `json:"readTimeout"`
	WriteTimeout      string `json:"writeTimeout"`
	IdleTimeout       string `json:"idleTimeout"`
	ShutdownTimeout   string `json:"shutdownTimeout"`
	MaxConcurrentRuns int64  `json:"maxConcurrentRuns"`
}

type RepositoryConfiguration struct {
	Backend string `json:"backend"`
	DataDir string `json:"dataDir"`
}

type LLMConfiguration struct {
	DefaultProviderID          string                     `json:"defaultProviderId"`
	DefaultEmbeddingProviderID string                     `json:"defaultEmbeddingProviderId,omitempty"`
	Providers                  []LLMProviderConfiguration `json:"providers"`
}

type LLMProviderConfiguration struct {
	ID                string   `json:"id"`
	Provider          string   `json:"provider,omitempty"`
	BaseURL           string   `json:"baseUrl,omitempty"`
	Model             string   `json:"model,omitempty"`
	Timeout           string   `json:"timeout,omitempty"`
	Disabled          bool     `json:"disabled"`
	Ready             bool     `json:"ready"`
	Reason            string   `json:"reason,omitempty"`
	Default           bool     `json:"default"`
	APIKeyConfigured  bool     `json:"apiKeyConfigured"`
	ChatCompletions   bool     `json:"chatCompletions"`
	Streaming         bool     `json:"streaming"`
	NonChatOperations []string `json:"nonChatOperations,omitempty"`
}

type UpdateLLMConfigurationCommand struct {
	DefaultProviderID          string                           `json:"defaultProviderId"`
	DefaultEmbeddingProviderID string                           `json:"defaultEmbeddingProviderId,omitempty"`
	Providers                  []UpdateLLMProviderConfiguration `json:"providers"`
}

type UpdateLLMProviderConfiguration struct {
	ID                string   `json:"id"`
	Provider          string   `json:"provider,omitempty"`
	BaseURL           string   `json:"baseUrl,omitempty"`
	APIKey            string   `json:"apiKey,omitempty"`
	Model             string   `json:"model,omitempty"`
	Timeout           string   `json:"timeout,omitempty"`
	Disabled          bool     `json:"disabled"`
	ChatCompletions   *bool    `json:"chatCompletions,omitempty"`
	Streaming         *bool    `json:"streaming,omitempty"`
	NonChatOperations []string `json:"nonChatOperations,omitempty"`
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
		})
	}
	sort.Slice(providers, func(i, j int) bool {
		if providers[i].Default != providers[j].Default {
			return providers[i].Default
		}
		return providers[i].ID < providers[j].ID
	})
	return ServiceConfiguration{
		Server: ServerConfiguration{
			Addr:              cfg.Server.Addr,
			ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout,
			ReadTimeout:       cfg.Server.ReadTimeout,
			WriteTimeout:      cfg.Server.WriteTimeout,
			IdleTimeout:       cfg.Server.IdleTimeout,
			ShutdownTimeout:   cfg.Server.ShutdownTimeout,
			MaxConcurrentRuns: cfg.Server.MaxConcurrentRuns,
		},
		Repository: RepositoryConfiguration{
			Backend: cfg.Repository.Backend,
			DataDir: cfg.Repository.DataDir,
		},
		LLM: LLMConfiguration{
			DefaultProviderID:          defaultProviderID,
			DefaultEmbeddingProviderID: strings.TrimSpace(cfg.LLM.DefaultEmbeddingProvider),
			Providers:                  providers,
		},
	}, nil
}

func UpdateLLMConfiguration(cmd UpdateLLMConfigurationCommand) (ServiceConfiguration, error) {
	cfg, err := workorchconfig.Current()
	if err != nil {
		return ServiceConfiguration{}, err
	}
	providers, err := updatedLLMProviders(cfg.LLM.Providers, cmd.Providers)
	if err != nil {
		return ServiceConfiguration{}, err
	}
	defaultProviderID := strings.TrimSpace(cmd.DefaultProviderID)
	if defaultProviderID == "" {
		return ServiceConfiguration{}, fmt.Errorf("%w: defaultProviderId is required", common.ErrInvalidArgument)
	}
	if _, ok := providers[defaultProviderID]; !ok {
		return ServiceConfiguration{}, fmt.Errorf("%w: defaultProviderId must reference an LLM provider", common.ErrInvalidArgument)
	}
	defaultEmbeddingProviderID := strings.TrimSpace(cmd.DefaultEmbeddingProviderID)
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
	cfg.LLM.Providers = providers
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
		apiKey := strings.TrimSpace(update.APIKey)
		if apiKey == "" && existing != nil {
			apiKey = existing[id].APIKey
		}
		disabled := update.Disabled
		chatCompletions := true
		streaming := false
		if existing != nil {
			if previous := existing[id].ChatCompletions; previous != nil {
				chatCompletions = *previous
			}
			if previous := existing[id].Streaming; previous != nil {
				streaming = *previous
			}
		}
		if update.ChatCompletions != nil {
			chatCompletions = *update.ChatCompletions
		}
		if update.Streaming != nil {
			streaming = *update.Streaming
		}
		ret[id] = workorchconfig.LLMProviderConfig{
			Provider:          strings.TrimSpace(update.Provider),
			BaseURL:           strings.TrimSpace(update.BaseURL),
			APIKey:            apiKey,
			Model:             strings.TrimSpace(update.Model),
			Timeout:           strings.TrimSpace(update.Timeout),
			Disabled:          &disabled,
			ChatCompletions:   &chatCompletions,
			Streaming:         &streaming,
			NonChatOperations: normalizedProviderOperations(update.NonChatOperations),
		}
	}
	return ret, nil
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

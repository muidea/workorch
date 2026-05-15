package appservice

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	llmmodels "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestReadServiceConfigurationRedactsLLMProviders(t *testing.T) {
	configDir := t.TempDir()
	content := `
endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"
readHeaderTimeout = "5s"
readTimeout = "15s"
writeTimeout = "180s"
idleTimeout = "60s"
shutdownTimeout = "5s"
maxConcurrentRuns = 2

[repository]
backend = "file"
dataDir = ".workorch-data"

[runtime]
defaultSoulId = "soul-system"
maxProviderRetryAttempts = 12
shellCommandTimeout = "90s"

[llm]
defaultProvider = "local"
defaultEmbeddingProvider = "local"

[llm.providers.local]
provider = "compatible"
baseURL = "http://127.0.0.1:11434/v1"
apiKey = "secret-value"
model = "local-model"
timeout = "60s"
disabled = false
streaming = true
nonChatOperations = ["embeddings"]
inputTokenPrice = 0.000001
outputTokenPrice = 0.000002
currency = "usd"

[webSearch]
defaultProvider = "search"

[webSearch.providers.search]
provider = "http"
searchEndpoint = "https://search.example.test/search"
fetchEndpoint = "https://search.example.test/fetch"
apiKey = "web-secret"
timeout = "30s"
maxResults = 7
maxFetchBytes = 131072
maxRedirects = 2
allowDomains = ["example.test"]
`
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	cfg, err := ReadServiceConfiguration()
	if err != nil {
		t.Fatalf("read service configuration: %v", err)
	}
	if cfg.Server.Addr != "127.0.0.1:18080" || cfg.Repository.Backend != "file" {
		t.Fatalf("configuration = %+v, want server and repository config", cfg)
	}
	if cfg.Runtime.DefaultSoulID != "soul-system" ||
		cfg.Runtime.MaxProviderRetryAttempts != 12 ||
		cfg.Runtime.ShellCommandTimeout != "90s" {
		t.Fatalf("runtime configuration = %+v, want system runtime settings", cfg.Runtime)
	}
	if cfg.LLM.DefaultProviderID != "local" || cfg.LLM.DefaultEmbeddingProviderID != "local" || len(cfg.LLM.Providers) != 1 {
		t.Fatalf("llm configuration = %+v, want local provider", cfg.LLM)
	}
	if len(cfg.LLM.ProviderTemplates) != 3 {
		t.Fatalf("provider templates = %+v, want openai/anthropic/compatible templates", cfg.LLM.ProviderTemplates)
	}
	provider := cfg.LLM.Providers[0]
	if !provider.Ready || !provider.Default || !provider.APIKeyConfigured || provider.Model != "local-model" ||
		!provider.ChatCompletions || !provider.Streaming || len(provider.NonChatOperations) != 1 || provider.NonChatOperations[0] != "embeddings" {
		t.Fatalf("provider = %+v, want ready redacted local provider", provider)
	}
	if provider.ContextProfile.Currency != "USD" ||
		provider.ContextProfile.InputTokenPrice != 0.000001 ||
		provider.ContextProfile.OutputTokenPrice != 0.000002 {
		t.Fatalf("provider context profile = %+v, want pricing fields exposed", provider.ContextProfile)
	}
	if cfg.WebSearch.DefaultProviderID != "search" || len(cfg.WebSearch.Providers) != 1 || len(cfg.WebSearch.ProviderTemplates) != 1 {
		t.Fatalf("web search configuration = %+v, want search provider and template", cfg.WebSearch)
	}
	webProvider := cfg.WebSearch.Providers[0]
	if !webProvider.Ready || !webProvider.Default || !webProvider.APIKeyConfigured ||
		webProvider.SearchEndpoint != "https://search.example.test/search" ||
		webProvider.FetchEndpoint != "https://search.example.test/fetch" ||
		webProvider.MaxResults != 7 || webProvider.MaxFetchBytes != 131072 || len(webProvider.AllowDomains) != 1 {
		t.Fatalf("web provider = %+v, want ready redacted web provider", webProvider)
	}
}

func TestUpdateLLMConfigurationPersistsAndReloadsProviders(t *testing.T) {
	configDir := t.TempDir()
	content := `
endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"

[repository]
backend = "memory"

[llm]
defaultProvider = "local"

[llm.providers.local]
provider = "compatible"
baseURL = "http://127.0.0.1:11434/v1"
apiKey = "secret-value"
model = "local-model"
timeout = "60s"
disabled = false
`
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	cfg, err := UpdateLLMConfiguration(UpdateLLMConfigurationCommand{
		DefaultProviderID:          "local",
		DefaultEmbeddingProviderID: "local",
		Providers: []UpdateLLMProviderConfiguration{{
			ID:                "local",
			Provider:          "compatible",
			BaseURL:           "http://127.0.0.1:11434/v1",
			Model:             "next-model",
			Timeout:           "45s",
			NonChatOperations: []string{"embeddings", "embeddings"},
			ContextProfile: &llmmodels.ProviderContextProfile{
				InputTokenPrice:        0.000001,
				OutputTokenPrice:       0.000002,
				Currency:               "USD",
				EstimatedTokensPerByte: 0.25,
			},
		}},
	})
	if err != nil {
		t.Fatalf("update llm configuration: %v", err)
	}
	if cfg.LLM.DefaultProviderID != "local" || cfg.LLM.DefaultEmbeddingProviderID != "local" || len(cfg.LLM.Providers) != 1 {
		t.Fatalf("configuration = %+v, want local provider", cfg.LLM)
	}
	provider := cfg.LLM.Providers[0]
	if provider.Model != "next-model" || !provider.APIKeyConfigured || !provider.ChatCompletions ||
		len(provider.NonChatOperations) != 1 || provider.NonChatOperations[0] != "embeddings" {
		t.Fatalf("provider = %+v, want updated model and preserved api key", provider)
	}
	if provider.ContextProfile.InputTokenPrice != 0.000001 ||
		provider.ContextProfile.OutputTokenPrice != 0.000002 ||
		provider.ContextProfile.Currency != "USD" ||
		provider.ContextProfile.EstimatedTokensPerByte != 0.25 {
		t.Fatalf("provider context profile = %+v, want updated profile pricing", provider.ContextProfile)
	}

	current, err := workorchconfig.Current()
	if err != nil {
		t.Fatalf("current config: %v", err)
	}
	if current.LLM.Providers["local"].Model != "next-model" || current.LLM.Providers["local"].APIKey != "secret-value" {
		t.Fatalf("current llm config = %+v, want persisted provider update", current.LLM.Providers["local"])
	}
	if current.LLM.DefaultEmbeddingProvider != "local" {
		t.Fatalf("current default embedding provider = %q, want local", current.LLM.DefaultEmbeddingProvider)
	}
	if current.LLM.Providers["local"].ChatCompletions == nil || !*current.LLM.Providers["local"].ChatCompletions ||
		len(current.LLM.Providers["local"].NonChatOperations) != 1 {
		t.Fatalf("current llm capabilities = %+v, want persisted provider capabilities", current.LLM.Providers["local"])
	}
	if current.LLM.Providers["local"].InputTokenPrice != 0.000001 ||
		current.LLM.Providers["local"].OutputTokenPrice != 0.000002 ||
		current.LLM.Providers["local"].Currency != "USD" ||
		current.LLM.Providers["local"].EstimatedTokensPerByte != 0.25 {
		t.Fatalf("current llm context profile = %+v, want persisted profile pricing", current.LLM.Providers["local"])
	}
}

func TestUpdateLLMConfigurationRejectsMissingDefaultProvider(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(`endpointName = "workorch"`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	_, err := UpdateLLMConfiguration(UpdateLLMConfigurationCommand{
		DefaultProviderID: "missing",
		Providers: []UpdateLLMProviderConfiguration{{
			ID:       "local",
			Provider: "compatible",
		}},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("update err = %v, want invalid argument", err)
	}
}

func TestUpdateLLMConfigurationRejectsEmbeddingProviderWithoutCapability(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(`endpointName = "workorch"`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	_, err := UpdateLLMConfiguration(UpdateLLMConfigurationCommand{
		DefaultProviderID:          "local",
		DefaultEmbeddingProviderID: "local",
		Providers: []UpdateLLMProviderConfiguration{{
			ID:       "local",
			Provider: "compatible",
		}},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("update err = %v, want invalid argument", err)
	}
}

func TestUpdateLLMConfigurationRejectsUnavailableEmbeddingProvider(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(`endpointName = "workorch"`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	_, err := UpdateLLMConfiguration(UpdateLLMConfigurationCommand{
		DefaultProviderID:          "local",
		DefaultEmbeddingProviderID: "local",
		Providers: []UpdateLLMProviderConfiguration{{
			ID:                "local",
			Provider:          "compatible",
			Model:             "embedding-model",
			NonChatOperations: []string{"embeddings"},
		}},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("update err = %v, want invalid argument", err)
	}
}

func TestUpdateWebSearchConfigurationPersistsAndReloadsProviders(t *testing.T) {
	configDir := t.TempDir()
	content := `
endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"

[repository]
backend = "memory"

[webSearch]
defaultProvider = "search"

[webSearch.providers.search]
provider = "http"
searchEndpoint = "https://old.example.test/search"
fetchEndpoint = "https://old.example.test/fetch"
apiKey = "web-secret"
timeout = "20s"
maxResults = 5
`
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	cfg, err := UpdateWebSearchConfiguration(UpdateWebSearchConfigurationCommand{
		DefaultProviderID: "search",
		Providers: []UpdateWebSearchProviderConfiguration{{
			ID:             "search",
			Provider:       "http",
			SearchEndpoint: "https://search.example.test/search",
			FetchEndpoint:  "https://search.example.test/fetch",
			Timeout:        "25s",
			MaxResults:     9,
			MaxFetchBytes:  262144,
			MaxRedirects:   3,
			AllowDomains:   []string{"example.test", "example.test"},
			DenyDomains:    []string{"blocked.example.test"},
		}},
	})
	if err != nil {
		t.Fatalf("update web search configuration: %v", err)
	}
	if cfg.WebSearch.DefaultProviderID != "search" || len(cfg.WebSearch.Providers) != 1 {
		t.Fatalf("configuration = %+v, want search provider", cfg.WebSearch)
	}
	provider := cfg.WebSearch.Providers[0]
	if !provider.Ready || !provider.APIKeyConfigured || provider.SearchEndpoint != "https://search.example.test/search" ||
		provider.MaxResults != 9 || len(provider.AllowDomains) != 1 {
		t.Fatalf("provider = %+v, want updated provider and preserved api key", provider)
	}

	current, err := workorchconfig.Current()
	if err != nil {
		t.Fatalf("current config: %v", err)
	}
	if current.WebSearch.Providers["search"].APIKey != "web-secret" ||
		current.WebSearch.Providers["search"].SearchEndpoint != "https://search.example.test/search" {
		t.Fatalf("current web search config = %+v, want persisted provider update", current.WebSearch.Providers["search"])
	}
}

func TestUpdateWebSearchConfigurationRejectsUnavailableDefaultProvider(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(`endpointName = "workorch"`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	_, err := UpdateWebSearchConfiguration(UpdateWebSearchConfigurationCommand{
		DefaultProviderID: "search",
		Providers: []UpdateWebSearchProviderConfiguration{{
			ID:       "search",
			Provider: "http",
		}},
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("update err = %v, want invalid argument", err)
	}
}

func TestUpdateWebSearchConfigurationCanClearOptionalProviders(t *testing.T) {
	configDir := t.TempDir()
	content := `
endpointName = "workorch"

[webSearch]
defaultProvider = "search"

[webSearch.providers.search]
provider = "http"
searchEndpoint = "https://search.example.test/search"
`
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	cfg, err := UpdateWebSearchConfiguration(UpdateWebSearchConfigurationCommand{})
	if err != nil {
		t.Fatalf("clear web search configuration: %v", err)
	}
	if cfg.WebSearch.DefaultProviderID != "" || len(cfg.WebSearch.Providers) != 0 {
		t.Fatalf("web search configuration = %+v, want cleared optional providers", cfg.WebSearch)
	}
}

package appservice

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

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
maxRuntimeRepairAttempts = 11
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

[webSearch.providers.search.headers]
X-WorkOrch-Client = "workorch"

[governance.autoTicket]
enabled = true
minConfidence = 0.82
maxPerRun = 5

[governance.autoApproval]
enabled = true
minConfidence = 0.9
maxPerRun = 3
allowReadOnly = true
allowWorkspaceWrite = true
requireHumanFor = ["secret_access", "external_write"]

[governance.budget]
maxConsecutiveFailures = 2
maxRepeatedSamePrompt = 1
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
		cfg.Runtime.MaxRuntimeRepairAttempts != 11 ||
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
	if cfg.WebSearch.DefaultProviderID != "search" || len(cfg.WebSearch.Providers) != 1 || len(cfg.WebSearch.ProviderTemplates) != 1 {
		t.Fatalf("web search configuration = %+v, want search provider and template", cfg.WebSearch)
	}
	webProvider := cfg.WebSearch.Providers[0]
	if !webProvider.Ready || !webProvider.Default || !webProvider.APIKeyConfigured ||
		webProvider.SearchEndpoint != "https://search.example.test/search" ||
		webProvider.FetchEndpoint != "https://search.example.test/fetch" ||
		webProvider.MaxResults != 7 || webProvider.MaxFetchBytes != 131072 || len(webProvider.AllowDomains) != 1 ||
		webProvider.Headers["X-WorkOrch-Client"] != "workorch" {
		t.Fatalf("web provider = %+v, want ready redacted web provider", webProvider)
	}
	if !cfg.Governance.AutoTicket.Enabled ||
		cfg.Governance.AutoTicket.MinConfidence != 0.82 ||
		cfg.Governance.AutoTicket.MaxPerRun != 5 ||
		!cfg.Governance.AutoApproval.Enabled ||
		!cfg.Governance.AutoApproval.AllowReadOnly ||
		!cfg.Governance.AutoApproval.AllowWorkspaceWrite ||
		len(cfg.Governance.AutoApproval.RequireHumanFor) != 2 ||
		cfg.Governance.Budget.MaxConsecutiveFailures != 2 ||
		cfg.Governance.Budget.MaxRepeatedSamePrompt != 1 {
		t.Fatalf("governance configuration = %+v, want auto governance settings", cfg.Governance)
	}
}

func TestUpdateLLMConfigurationPersistsAndReloadsProviders(t *testing.T) {
	configDir := t.TempDir()
	content := `
endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"

[repository]
backend = "file"
dataDir = ".workorch-data"

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
		DefaultProviderID:          stringPtr("local"),
		DefaultEmbeddingProviderID: stringPtr("local"),
		Providers: llmProviderUpdates(UpdateLLMProviderConfiguration{
			ID:                "local",
			Provider:          stringPtr("compatible"),
			BaseURL:           stringPtr("http://127.0.0.1:11434/v1"),
			Model:             stringPtr("next-model"),
			Timeout:           stringPtr("45s"),
			NonChatOperations: []string{"embeddings", "embeddings"},
			ContextProfile: &UpdateLLMProviderContextProfile{
				EstimatedTokensPerByte: float64Ptr(0.25),
			},
		}),
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
	if provider.ContextProfile.EstimatedTokensPerByte != 0.25 {
		t.Fatalf("provider context profile = %+v, want updated profile", provider.ContextProfile)
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
	if current.LLM.Providers["local"].EstimatedTokensPerByte != 0 {
		t.Fatalf("current llm context profile = %+v, want auto profile to omit manual token estimate", current.LLM.Providers["local"])
	}
}

func TestUpdateServerConfigurationOmittedFieldsDoNotClearExistingValues(t *testing.T) {
	configDir := t.TempDir()
	content := `
endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"
defaultDialogLanguage = "zh-CN"
`
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	cfg, err := UpdateServerConfiguration(UpdateServerConfigurationCommand{})
	if err != nil {
		t.Fatalf("update server configuration: %v", err)
	}
	if cfg.Server.DefaultDialogLanguage != "zh-CN" {
		t.Fatalf("default dialog language = %q, want preserved zh-CN", cfg.Server.DefaultDialogLanguage)
	}
}

func TestUpdateLLMConfigurationOmittedProviderFieldsPreserveExistingValues(t *testing.T) {
	configDir := t.TempDir()
	content := `
endpointName = "workorch"

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
chatCompletions = true
streaming = true
nonChatOperations = ["embeddings", "rerank"]
failoverEnabled = true
failoverProviders = ["backup"]
contextWindowTokens = 32768
maxInputTokens = 28672
reservedOutputTokens = 4096
safetyRatio = 0.8
bytesHardLimit = 98304
estimatedTokensPerByte = 0.25
`
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	cfg, err := UpdateLLMConfiguration(UpdateLLMConfigurationCommand{
		Providers: llmProviderUpdates(UpdateLLMProviderConfiguration{
			ID:     "local",
			Model:  stringPtr("next-model"),
			APIKey: stringPtr(""),
		}),
	})
	if err != nil {
		t.Fatalf("update llm configuration: %v", err)
	}
	provider := cfg.LLM.Providers[0]
	if provider.Model != "next-model" ||
		provider.Provider != "compatible" ||
		provider.BaseURL != "http://127.0.0.1:11434/v1" ||
		provider.Timeout != "1m0s" ||
		provider.Disabled ||
		!provider.ChatCompletions ||
		!provider.Streaming ||
		len(provider.NonChatOperations) != 2 ||
		!provider.FailoverEnabled ||
		len(provider.FailoverProviders) != 1 ||
		provider.ContextProfile.ContextWindowTokens != 32768 {
		t.Fatalf("provider = %+v, want omitted fields preserved", provider)
	}
	current, err := workorchconfig.Current()
	if err != nil {
		t.Fatalf("current config: %v", err)
	}
	if current.LLM.Providers["local"].APIKey != "secret-value" {
		t.Fatalf("api key was not preserved")
	}
}

func TestUpdateLLMConfigurationCanExplicitlyClearSlices(t *testing.T) {
	configDir := t.TempDir()
	content := `
endpointName = "workorch"

[llm]
defaultProvider = "local"

[llm.providers.local]
provider = "compatible"
baseURL = "http://127.0.0.1:11434/v1"
model = "local-model"
nonChatOperations = ["embeddings"]
failoverProviders = ["backup"]
`
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	cfg, err := UpdateLLMConfiguration(UpdateLLMConfigurationCommand{
		Providers: llmProviderUpdates(UpdateLLMProviderConfiguration{
			ID:                "local",
			NonChatOperations: []string{},
			FailoverProviders: []string{},
		}),
	})
	if err != nil {
		t.Fatalf("update llm configuration: %v", err)
	}
	provider := cfg.LLM.Providers[0]
	if len(provider.NonChatOperations) != 0 || len(provider.FailoverProviders) != 0 {
		t.Fatalf("provider = %+v, want explicit empty slices persisted", provider)
	}
}

func TestUpdateLLMConfigurationPartialInferenceProfilePreservesExistingValues(t *testing.T) {
	configDir := t.TempDir()
	content := `
endpointName = "workorch"

[llm]
defaultProvider = "local"

[llm.providers.local]
provider = "compatible"
baseURL = "http://127.0.0.1:11434/v1"
model = "local-model"
temperature = 0.4
maxOutputTokens = 8192
reasoningEffort = "high"
thinkingBudgetTokens = 4096
`
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	cfg, err := UpdateLLMConfiguration(UpdateLLMConfigurationCommand{
		Providers: llmProviderUpdates(UpdateLLMProviderConfiguration{
			ID: "local",
			InferenceProfile: &UpdateLLMProviderInferenceProfile{
				ReasoningEffort: stringPtr("medium"),
			},
		}),
	})
	if err != nil {
		t.Fatalf("update llm configuration: %v", err)
	}
	provider := cfg.LLM.Providers[0]
	if provider.InferenceProfile.ReasoningEffort != "medium" ||
		provider.InferenceProfile.MaxOutputTokens != 8192 ||
		provider.InferenceProfile.ThinkingBudgetTokens != 4096 ||
		provider.InferenceProfile.Temperature == nil ||
		*provider.InferenceProfile.Temperature != 0.4 {
		t.Fatalf("provider inference profile = %+v, want partial update with preserved values", provider.InferenceProfile)
	}
}

func TestUpdateLLMConfigurationPartialContextProfilePreservesExistingValues(t *testing.T) {
	configDir := t.TempDir()
	content := `
endpointName = "workorch"

[llm]
defaultProvider = "local"

[llm.providers.local]
provider = "compatible"
baseURL = "http://127.0.0.1:11434/v1"
model = "local-model"
contextProfileMode = "manual"
contextWindowTokens = 32768
maxInputTokens = 28672
reservedOutputTokens = 4096
safetyRatio = 0.8
bytesHardLimit = 98304
estimatedTokensPerByte = 0.25
`
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	cfg, err := UpdateLLMConfiguration(UpdateLLMConfigurationCommand{
		Providers: llmProviderUpdates(UpdateLLMProviderConfiguration{
			ID: "local",
			ContextProfile: &UpdateLLMProviderContextProfile{
				ProfileMode:         stringPtr("auto"),
				ContextWindowTokens: intPtr(65536),
			},
		}),
	})
	if err != nil {
		t.Fatalf("update llm configuration: %v", err)
	}
	provider := cfg.LLM.Providers[0]
	if provider.ContextProfile.ProfileMode != "auto" ||
		provider.ContextProfile.ContextWindowTokens != 65536 ||
		provider.ContextProfile.MaxInputTokens != 46080 ||
		provider.ContextProfile.ReservedOutputTokens != 4096 ||
		provider.ContextProfile.BytesHardLimit != 184320 {
		t.Fatalf("provider context profile = %+v, want auto budget", provider.ContextProfile)
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
		DefaultProviderID: stringPtr("missing"),
		Providers: llmProviderUpdates(UpdateLLMProviderConfiguration{
			ID:       "local",
			Provider: stringPtr("compatible"),
		}),
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
		DefaultProviderID:          stringPtr("local"),
		DefaultEmbeddingProviderID: stringPtr("local"),
		Providers: llmProviderUpdates(UpdateLLMProviderConfiguration{
			ID:       "local",
			Provider: stringPtr("compatible"),
		}),
	})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("update err = %v, want invalid argument", err)
	}
}

func TestUpdateLLMConfigurationCanClearInvalidDefaultEmbeddingProvider(t *testing.T) {
	configDir := t.TempDir()
	content := `
endpointName = "workorch"

[llm]
defaultProvider = "local"
defaultEmbeddingProvider = "missing"

[llm.providers.local]
provider = "compatible"
baseURL = "http://127.0.0.1:11434/v1"
model = "local-model"
`
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	cfg, err := UpdateLLMConfiguration(UpdateLLMConfigurationCommand{
		DefaultEmbeddingProviderID: stringPtr(""),
	})
	if err != nil {
		t.Fatalf("clear default embedding provider: %v", err)
	}
	if cfg.LLM.DefaultEmbeddingProviderID != "" {
		t.Fatalf("default embedding provider = %q, want cleared", cfg.LLM.DefaultEmbeddingProviderID)
	}
	current, err := workorchconfig.Current()
	if err != nil {
		t.Fatalf("current config: %v", err)
	}
	if current.LLM.DefaultEmbeddingProvider != "" {
		t.Fatalf("persisted default embedding provider = %q, want cleared", current.LLM.DefaultEmbeddingProvider)
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
		DefaultProviderID:          stringPtr("local"),
		DefaultEmbeddingProviderID: stringPtr("local"),
		Providers: llmProviderUpdates(UpdateLLMProviderConfiguration{
			ID:                "local",
			Provider:          stringPtr("compatible"),
			Model:             stringPtr("embedding-model"),
			NonChatOperations: []string{"embeddings"},
		}),
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
backend = "file"
dataDir = ".workorch-data"

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
		DefaultProviderID: stringPtr("search"),
		Providers: webSearchProviderUpdates(UpdateWebSearchProviderConfiguration{
			ID:             "search",
			Provider:       stringPtr("http"),
			SearchEndpoint: stringPtr("https://search.example.test/search"),
			FetchEndpoint:  stringPtr("https://search.example.test/fetch"),
			Timeout:        stringPtr("25s"),
			MaxResults:     intPtr(9),
			MaxFetchBytes:  int64Ptr(262144),
			MaxRedirects:   intPtr(3),
			AllowDomains:   []string{"example.test", "example.test"},
			DenyDomains:    []string{"blocked.example.test"},
		}),
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

func TestUpdateWebSearchConfigurationOmittedProviderFieldsPreserveExistingValues(t *testing.T) {
	configDir := t.TempDir()
	content := `
endpointName = "workorch"

[webSearch]
defaultProvider = "search"

[webSearch.providers.search]
provider = "http"
searchEndpoint = "https://old.example.test/search"
fetchEndpoint = "https://old.example.test/fetch"
apiKey = "web-secret"
timeout = "20s"
maxResults = 5
maxFetchBytes = 131072
maxRedirects = 2
allowDomains = ["example.test"]
denyDomains = ["blocked.example.test"]
disabled = false

[webSearch.providers.search.headers]
X-WorkOrch-Client = "workorch"
`
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	cfg, err := UpdateWebSearchConfiguration(UpdateWebSearchConfigurationCommand{
		Providers: webSearchProviderUpdates(UpdateWebSearchProviderConfiguration{
			ID:      "search",
			Timeout: stringPtr("25s"),
			APIKey:  stringPtr(""),
		}),
	})
	if err != nil {
		t.Fatalf("update web search configuration: %v", err)
	}
	provider := cfg.WebSearch.Providers[0]
	if provider.Provider != "http" ||
		provider.SearchEndpoint != "https://old.example.test/search" ||
		provider.FetchEndpoint != "https://old.example.test/fetch" ||
		provider.Headers["X-WorkOrch-Client"] != "workorch" ||
		provider.Timeout != "25s" ||
		provider.MaxResults != 5 ||
		provider.MaxFetchBytes != 131072 ||
		provider.MaxRedirects != 2 ||
		len(provider.AllowDomains) != 1 ||
		len(provider.DenyDomains) != 1 ||
		provider.Disabled {
		t.Fatalf("provider = %+v, want omitted fields preserved", provider)
	}
	current, err := workorchconfig.Current()
	if err != nil {
		t.Fatalf("current config: %v", err)
	}
	if current.WebSearch.Providers["search"].APIKey != "web-secret" {
		t.Fatalf("api key was not preserved")
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
		DefaultProviderID: stringPtr("search"),
		Providers: webSearchProviderUpdates(UpdateWebSearchProviderConfiguration{
			ID:       "search",
			Provider: stringPtr("http"),
		}),
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

	cfg, err := UpdateWebSearchConfiguration(UpdateWebSearchConfigurationCommand{
		Providers: webSearchProviderUpdates(),
	})
	if err != nil {
		t.Fatalf("clear web search configuration: %v", err)
	}
	if cfg.WebSearch.DefaultProviderID != "" || len(cfg.WebSearch.Providers) != 0 {
		t.Fatalf("web search configuration = %+v, want cleared optional providers", cfg.WebSearch)
	}
}

func stringPtr(value string) *string {
	return &value
}

func float64Ptr(value float64) *float64 {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func llmProviderUpdates(values ...UpdateLLMProviderConfiguration) *[]UpdateLLMProviderConfiguration {
	return &values
}

func webSearchProviderUpdates(values ...UpdateWebSearchProviderConfiguration) *[]UpdateWebSearchProviderConfiguration {
	return &values
}

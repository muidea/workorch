package appservice

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func validLLMTOML() string {
	return `
[llm.aiProxy]
baseURL = "http://127.0.0.1:8080"
apiKey = "test-key"
timeout = "180s"
catalogRequestTimeout = "5s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "gpt-4o"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25
`
}

func TestReadServiceConfigurationRedactsAIProxySecrets(t *testing.T) {
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

[runtime.git]
defaultTimeout = "45s"

[runtime.llmOutput]
maxPatchContentBytes = 4096
maxGraphBytes = 16384
maxGraphFileWrites = 2
schemaInvalidLargePreviewThreshold = 8192
maxArtifactChunkBytes = 4096
previousChunkTailBytes = 128
maxChunkRetryAttempts = 4
maxArtifactPlanAttempts = 3
maxArtifactReplanAttempts = 2
` + validLLMTOML() + `

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
	if cfg.LLM.AIProxy.BaseURL != "http://127.0.0.1:8080" || !cfg.LLM.AIProxy.APIKeyConfigured {
		t.Fatalf("aiProxy = %+v, want redacted configured ai-proxy", cfg.LLM.AIProxy)
	}
	if cfg.LLM.DefaultChat.ProviderID != "ai-proxy" || cfg.LLM.DefaultChat.ModelID != "gpt-4o" {
		t.Fatalf("defaultChat = %+v, want ai-proxy/gpt-4o", cfg.LLM.DefaultChat)
	}
	if cfg.WebSearch.DefaultProviderID != "search" || len(cfg.WebSearch.Providers) != 1 {
		t.Fatalf("web search configuration = %+v, want search provider", cfg.WebSearch)
	}
	if !cfg.Governance.AutoTicket.Enabled || cfg.Governance.Budget.MaxConsecutiveFailures != 2 {
		t.Fatalf("governance configuration = %+v, want auto governance settings", cfg.Governance)
	}
}

func TestUpdateLLMConfigurationPersistsAIProxyAndDefaultTarget(t *testing.T) {
	configDir := t.TempDir()
	content := `
endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"

[repository]
backend = "file"
dataDir = ".workorch-data"
` + validLLMTOML()
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	baseURL := "http://127.0.0.1:9090"
	modelID := "gpt-4.1"
	cfg, err := UpdateLLMConfiguration(UpdateLLMConfigurationCommand{
		AIProxy: &workorchconfig.AIProxyProviderConfig{
			BaseURL:                 baseURL,
			Timeout:                 "180s",
			CatalogRequestTimeout:   "5s",
			CatalogTTL:              "5m",
			CatalogMaxStaleAge:      "24h",
			MaxCatalogResponseBytes: 1048576,
			MaxCatalogModels:        1000,
		},
		DefaultChat: &workorchconfig.ProviderModelTargetConfig{
			ProviderID: "ai-proxy",
			ModelID:    modelID,
		},
	})
	if err != nil {
		t.Fatalf("update llm configuration: %v", err)
	}
	if cfg.LLM.AIProxy.BaseURL != baseURL || cfg.LLM.DefaultChat.ModelID != modelID {
		t.Fatalf("updated llm = %+v, want persisted ai-proxy target", cfg.LLM)
	}
	if !cfg.LLM.AIProxy.APIKeyConfigured {
		t.Fatalf("api key should remain configured after partial update")
	}
	current, err := workorchconfig.Current()
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if current.LLM.AIProxy.APIKey != "test-key" || current.LLM.DefaultChat.ModelID != modelID {
		t.Fatalf("canonical llm = %+v, want preserved api key and new model", current.LLM)
	}
}

func TestUpdateLLMConfigurationRejectsReservedCustomProviderID(t *testing.T) {
	configDir := t.TempDir()
	content := `
endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"

[repository]
backend = "file"
dataDir = ".workorch-data"
` + validLLMTOML()
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	custom := map[string]workorchconfig.CustomLLMProviderConfig{
		"ai-proxy": {
			Provider:   "openai",
			APISurface: "openai_compatible_http",
			BaseURL:    "https://api.openai.com",
			Models: map[string]workorchconfig.CustomLLMModelConfig{
				"gpt-4o": {ContextWindowTokens: 128000, MaxOutputTokens: 4096, Operations: []string{workorchconfig.LLMOperationChatCompletions}},
			},
		},
	}
	_, err := UpdateLLMConfiguration(UpdateLLMConfigurationCommand{CustomProviders: &custom})
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("UpdateLLMConfiguration error = %v, want reserved provider rejection", err)
	}
}

func TestUpdateServerConfigurationOmittedFieldsDoNotClearExistingValues(t *testing.T) {
	configDir := t.TempDir()
	content := `
endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"
maxConcurrentRuns = 2
defaultDialogLanguage = "en-US"

[repository]
backend = "file"
dataDir = ".workorch-data"
` + validLLMTOML()
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	lang := "zh-CN"
	cfg, err := UpdateServerConfiguration(UpdateServerConfigurationCommand{DefaultDialogLanguage: &lang})
	if err != nil {
		t.Fatalf("update server configuration: %v", err)
	}
	if cfg.Server.DefaultDialogLanguage != lang || cfg.Server.MaxConcurrentRuns != 2 || cfg.Server.Addr != "127.0.0.1:18080" {
		t.Fatalf("server configuration = %+v, want language update with preserved server fields", cfg.Server)
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


[llm.aiProxy]
baseURL = "http://127.0.0.1:8080"
apiKey = "test-key"
timeout = "180s"
catalogRequestTimeout = "5s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "gpt-4o"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25

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


[llm.aiProxy]
baseURL = "http://127.0.0.1:8080"
apiKey = "test-key"
timeout = "180s"
catalogRequestTimeout = "5s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "gpt-4o"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25

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
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(`endpointName = "workorch"

[llm.aiProxy]
baseURL = "http://127.0.0.1:8080"
apiKey = "test-key"
timeout = "180s"
catalogRequestTimeout = "5s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "gpt-4o"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25
`), 0o600); err != nil {
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


[llm.aiProxy]
baseURL = "http://127.0.0.1:8080"
apiKey = "test-key"
timeout = "180s"
catalogRequestTimeout = "5s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "gpt-4o"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25

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

func webSearchProviderUpdates(values ...UpdateWebSearchProviderConfiguration) *[]UpdateWebSearchProviderConfiguration {
	return &values
}

func TestUpdateLLMConfigurationClearAIProxyAPIKey(t *testing.T) {
	useWorkorchTestConfig(t, "file", t.TempDir())
	// Seed an API key via update.
	baseURL := "http://127.0.0.1:18080"
	_, err := UpdateLLMConfiguration(UpdateLLMConfigurationCommand{
		AIProxy: &workorchconfig.AIProxyProviderConfig{
			BaseURL:                 baseURL,
			APIKey:                  "secret-to-clear",
			Timeout:                 "180s",
			CatalogRequestTimeout:   "5s",
			CatalogTTL:              "5m",
			CatalogMaxStaleAge:      "24h",
			MaxCatalogResponseBytes: 1048576,
			MaxCatalogModels:        1000,
		},
		DefaultChat: &workorchconfig.ProviderModelTargetConfig{ProviderID: "custom-test", ModelID: "model-1"},
	})
	if err != nil {
		// custom-test may already be present from useWorkorchTestConfig; still try clear path with current config.
		t.Logf("seed update: %v", err)
	}
	cfg, err := UpdateLLMConfiguration(UpdateLLMConfigurationCommand{
		AIProxy: &workorchconfig.AIProxyProviderConfig{
			BaseURL:                 baseURL,
			Timeout:                 "180s",
			CatalogRequestTimeout:   "5s",
			CatalogTTL:              "5m",
			CatalogMaxStaleAge:      "24h",
			MaxCatalogResponseBytes: 1048576,
			MaxCatalogModels:        1000,
		},
		ClearAIProxyAPIKey: true,
		DefaultChat:        &workorchconfig.ProviderModelTargetConfig{ProviderID: "custom-test", ModelID: "model-1"},
	})
	if err != nil {
		t.Fatalf("clear api key update: %v", err)
	}
	if cfg.LLM.AIProxy.APIKeyConfigured {
		t.Fatalf("apiKeyConfigured = true after clear, want false")
	}
	current, err := workorchconfig.Current()
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if strings.TrimSpace(current.LLM.AIProxy.APIKey) != "" {
		t.Fatalf("persisted api key = %q, want empty after clear", current.LLM.AIProxy.APIKey)
	}
}

func TestUpdateLLMConfigurationRejectsClearWithNonEmptyAIProxyAPIKey(t *testing.T) {
	useWorkorchTestConfig(t, "file", t.TempDir())
	baseURL := "http://127.0.0.1:18080"
	_, err := UpdateLLMConfiguration(UpdateLLMConfigurationCommand{
		AIProxy: &workorchconfig.AIProxyProviderConfig{
			BaseURL:                 baseURL,
			APIKey:                  "new-secret",
			Timeout:                 "180s",
			CatalogRequestTimeout:   "5s",
			CatalogTTL:              "5m",
			CatalogMaxStaleAge:      "24h",
			MaxCatalogResponseBytes: 1048576,
			MaxCatalogModels:        1000,
		},
		ClearAIProxyAPIKey: true,
		DefaultChat:        &workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
	})
	if err == nil {
		t.Fatal("expected clearAIProxyApiKey with non-empty apiKey to fail")
	}
}

func TestUpdateLLMConfigurationRejectsClearWithNonEmptyCustomProviderAPIKey(t *testing.T) {
	useWorkorchTestConfig(t, "file", t.TempDir())
	custom := map[string]workorchconfig.CustomLLMProviderConfig{
		"custom-clear": {
			Provider:   "compatible",
			APISurface: "openai_compatible_http",
			BaseURL:    "http://127.0.0.1:9",
			APIKey:     "new-custom-secret",
			Timeout:    "30s",
			Models: map[string]workorchconfig.CustomLLMModelConfig{
				"model-1": {ContextWindowTokens: 128000, MaxOutputTokens: 8192, Operations: []string{"chat_completions"}},
			},
		},
	}
	_, err := UpdateLLMConfiguration(UpdateLLMConfigurationCommand{
		CustomProviders:            &custom,
		ClearCustomProviderAPIKeys: []string{"custom-clear"},
		DefaultChat:                &workorchconfig.ProviderModelTargetConfig{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
	})
	if err == nil {
		t.Fatal("expected clearCustomProviderApiKeys with non-empty custom apiKey to fail")
	}
}

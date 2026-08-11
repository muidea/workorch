package appservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/configcore"
	llmevents "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
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
maxDocumentRecoveryAttempts = 4
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

func TestUpdateLLMConfigurationPersistsModelAndPreservesFileOwnedAIProxy(t *testing.T) {
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

	modelID := "gpt-4.1"
	cfg, err := UpdateLLMConfiguration(configcore.UpdateLLMConfigurationCommand{
		DefaultChatModelID: &modelID,
	})
	if err != nil {
		t.Fatalf("update llm configuration: %v", err)
	}
	if cfg.LLM.AIProxy.BaseURL != "http://127.0.0.1:8080" || cfg.LLM.DefaultChat.ModelID != modelID {
		t.Fatalf("updated llm = %+v, want file-owned ai-proxy and persisted model", cfg.LLM)
	}
	if !cfg.LLM.AIProxy.APIKeyConfigured {
		t.Fatalf("api key should remain configured after partial update")
	}
	if !cfg.LLM.RestartRequired {
		t.Fatal("LLM model update must remain staged until restart")
	}
	persistedConfig, err := workorchconfig.Persisted()
	if err != nil {
		t.Fatalf("persisted: %v", err)
	}
	if persistedConfig.LLM.AIProxy.BaseURL != "http://127.0.0.1:8080" || persistedConfig.LLM.AIProxy.Timeout != "180s" || persistedConfig.LLM.AIProxy.APIKey != "test-key" || persistedConfig.LLM.DefaultChat.ProviderID != "ai-proxy" || persistedConfig.LLM.DefaultChat.ModelID != modelID {
		t.Fatalf("persisted llm = %+v, want staged provider settings", persistedConfig.LLM)
	}
	if err := workorchconfig.Reload(); err != nil {
		t.Fatalf("restart reload: %v", err)
	}
	current, err := workorchconfig.Current()
	if err != nil {
		t.Fatalf("current after restart: %v", err)
	}
	if current.LLM.DefaultChat.ModelID != modelID {
		t.Fatalf("restarted llm = %+v, want persisted model", current.LLM)
	}
}

func TestUpdateLLMConfigurationStagesAIProxyAPIKeyUntilRestart(t *testing.T) {
	configDir := t.TempDir()
	content := `
endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"

[repository]
backend = "file"
dataDir = ".workorch-data"
` + validLLMTOML()
	configPath := filepath.Join(configDir, "application.toml")
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_ = workorchconfig.Close()
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })

	apiKey := "rotated-key"
	updated, err := UpdateLLMConfiguration(configcore.UpdateLLMConfigurationCommand{APIKey: &apiKey})
	if err != nil {
		t.Fatalf("rotate ai-proxy api key: %v", err)
	}
	if !updated.LLM.AIProxy.APIKeyConfigured {
		t.Fatal("rotated api key is not reported as configured")
	}
	if !updated.LLM.RestartRequired {
		t.Fatal("rotated api key must require restart")
	}
	persistedConfig, err := workorchconfig.Persisted()
	if err != nil {
		t.Fatalf("persisted after rotation: %v", err)
	}
	if persistedConfig.LLM.AIProxy.APIKey != "rotated-key" {
		t.Fatalf("persisted api key = %q, want staged key", persistedConfig.LLM.AIProxy.APIKey)
	}
	if err := workorchconfig.Reload(); err != nil {
		t.Fatalf("restart reload: %v", err)
	}
	current, err := workorchconfig.Current()
	if err != nil {
		t.Fatalf("current after restart: %v", err)
	}
	if current.LLM.AIProxy.APIKey != "rotated-key" {
		t.Fatalf("restarted api key = %q, want rotated key", current.LLM.AIProxy.APIKey)
	}
	persisted, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	if strings.Contains(string(persisted), `apiKey = "test-key"`) || !strings.Contains(string(persisted), `apiKey = 'rotated-key'`) {
		t.Fatalf("persisted configuration did not atomically replace the ai-proxy api key")
	}
}

func TestLLMConfigurationProjectionAndCatalogRefreshUseActiveOwnerRevision(t *testing.T) {
	useWorkorchTestConfig(t, "file", t.TempDir())
	ctx := context.Background()
	active, err := workorchconfig.Current()
	if err != nil {
		t.Fatal(err)
	}
	activeRevision, err := workorchconfig.LLMConfigurationRevision(active.LLM)
	if err != nil {
		t.Fatal(err)
	}
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	observer := magicevent.NewSimpleObserver(llmevents.CommandDestination, hub)
	observer.Subscribe(llmevents.TopicLLMCommandGetActiveConfiguration, func(_ magicevent.Event, result magicevent.Result) {
		result.Set(llmevents.GetActiveConfigurationResult{
			ProviderID:            workorchconfig.BuiltinAIProxyProviderID,
			ConfigurationRevision: activeRevision,
		}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(llmevents.TopicLLMCommandGetActiveConfiguration) })

	apiKey := "staged-key"
	updated, err := UpdateLLMConfiguration(configcore.UpdateLLMConfigurationCommand{APIKey: &apiKey})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.LLM.RestartRequired {
		t.Fatal("LLM update response must be restart-pending")
	}

	projection, err := ReadServiceConfigurationWithCatalog(ctx, hub)
	if err != nil {
		t.Fatal(err)
	}
	if !projection.LLM.RestartRequired {
		t.Fatal("configuration projection must compare persisted config with owner snapshot")
	}
	if _, err := RefreshLLMModelCatalog(ctx, hub); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("refresh error = %v, want restart-required invalid argument", err)
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
	cfg, err := UpdateServerConfiguration(configcore.UpdateServerConfigurationCommand{DefaultDialogLanguage: &lang})
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

	cfg, err := UpdateWebSearchConfiguration(configcore.UpdateWebSearchConfigurationCommand{
		DefaultProviderID: stringPtr("search"),
		Providers: webSearchProviderUpdates(configcore.UpdateWebSearchProviderConfiguration{
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

	cfg, err := UpdateWebSearchConfiguration(configcore.UpdateWebSearchConfigurationCommand{
		Providers: webSearchProviderUpdates(configcore.UpdateWebSearchProviderConfiguration{
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

	_, err := UpdateWebSearchConfiguration(configcore.UpdateWebSearchConfigurationCommand{
		DefaultProviderID: stringPtr("search"),
		Providers: webSearchProviderUpdates(configcore.UpdateWebSearchProviderConfiguration{
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

	cfg, err := UpdateWebSearchConfiguration(configcore.UpdateWebSearchConfigurationCommand{
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

func webSearchProviderUpdates(values ...configcore.UpdateWebSearchProviderConfiguration) *[]configcore.UpdateWebSearchProviderConfiguration {
	return &values
}

func TestUpdateLLMConfigurationClearAIProxyAPIKey(t *testing.T) {
	useWorkorchTestConfig(t, "file", t.TempDir())
	// Seed an API key via update.
	apiKey := "secret-to-clear"
	_, err := UpdateLLMConfiguration(configcore.UpdateLLMConfigurationCommand{
		APIKey: &apiKey,
	})
	if err != nil {
		t.Fatalf("seed api key: %v", err)
	}
	cfg, err := UpdateLLMConfiguration(configcore.UpdateLLMConfigurationCommand{
		ClearAPIKey: true,
	})
	if err != nil {
		t.Fatalf("clear api key update: %v", err)
	}
	if cfg.LLM.AIProxy.APIKeyConfigured {
		t.Fatalf("apiKeyConfigured = true after clear, want false")
	}
	if !cfg.LLM.RestartRequired {
		t.Fatal("cleared api key must require restart")
	}
	persistedConfig, err := workorchconfig.Persisted()
	if err != nil {
		t.Fatalf("persisted: %v", err)
	}
	if strings.TrimSpace(persistedConfig.LLM.AIProxy.APIKey) != "" {
		t.Fatalf("persisted api key = %q, want cleared staged key", persistedConfig.LLM.AIProxy.APIKey)
	}
	if err := workorchconfig.Reload(); err != nil {
		t.Fatalf("restart reload: %v", err)
	}
	current, err := workorchconfig.Current()
	if err != nil {
		t.Fatalf("current after restart: %v", err)
	}
	if strings.TrimSpace(current.LLM.AIProxy.APIKey) != "" {
		t.Fatalf("restarted api key = %q, want empty", current.LLM.AIProxy.APIKey)
	}
}

func TestUpdateLLMConfigurationRejectsClearWithNonEmptyAIProxyAPIKey(t *testing.T) {
	useWorkorchTestConfig(t, "file", t.TempDir())
	apiKey := "new-secret"
	_, err := UpdateLLMConfiguration(configcore.UpdateLLMConfigurationCommand{
		APIKey:      &apiKey,
		ClearAPIKey: true,
	})
	if err == nil {
		t.Fatal("expected clearApiKey with non-empty apiKey to fail")
	}
}

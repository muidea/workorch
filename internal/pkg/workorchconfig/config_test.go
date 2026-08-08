package workorchconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestCurrentReadsMagicCommonApplicationConfig(t *testing.T) {
	configDir := writeApplicationConfig(t, `
endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"
readHeaderTimeout = "5s"
readTimeout = "15s"
writeTimeout = "30s"
idleTimeout = "60s"
shutdownTimeout = "5s"
maxConcurrentRuns = 2
staleDispatchedStepMinAge = "120s"

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

[llm.aiProxy]
baseURL = "http://127.0.0.1:8080"
apiKey = "proxy-key"
timeout = "180s"
catalogRequestTimeout = "5s"
catalogTTL = "5m"
catalogMaxStaleAge = "24h"
maxCatalogResponseBytes = 1048576
maxCatalogModels = 1000

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "gpt-4o"

[llm.defaultEmbedding]
providerID = "ai-proxy"
modelID = "text-embedding-3-large"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25

[webSearch]
defaultProvider = "search"

[webSearch.providers.search]
provider = "http"
searchEndpoint = "https://search.example.test/search"
fetchEndpoint = "https://search.example.test/fetch"
apiKey = "search-key"
timeout = "20s"
maxResults = 8
maxFetchBytes = 262144
maxRedirects = 3
allowDomains = ["example.test"]
denyDomains = ["blocked.example.test"]
disabled = false

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
requireHumanFor = ["ssh_write", "secret_access"]

[governance.budget]
maxConsecutiveFailures = 2
maxRepeatedSamePrompt = 1
	`)
	_ = Close()
	t.Setenv(EnvConfigDir, configDir)
	t.Cleanup(func() { _ = Close() })

	cfg, err := Current()
	if err != nil {
		t.Fatalf("current config: %v", err)
	}
	if cfg.Server.Addr != "127.0.0.1:18080" || cfg.Repository.Backend != "file" || cfg.LLM.AIProxy.BaseURL != "http://127.0.0.1:8080" {
		t.Fatalf("config = %+v, want framework application config", cfg)
	}
	if cfg.Server.MaxConcurrentRuns != 2 || cfg.Server.StaleDispatchedStepMinAge != "120s" {
		t.Fatalf("server config = %+v, want max concurrent runs and stale dispatched min age", cfg.Server)
	}
	if cfg.Runtime.DefaultSoulID != "soul-system" ||
		cfg.Runtime.MaxRuntimeRepairAttempts != 11 ||
		cfg.Runtime.MaxProviderRetryAttempts != 12 ||
		cfg.Runtime.ShellCommandTimeout != "90s" {
		t.Fatalf("runtime config = %+v, want configured runtime settings", cfg.Runtime)
	}
	if !GitRuntimeEnabled(cfg.Runtime.Git) || !GitRuntimeWriteEnabled(cfg.Runtime.Git) || cfg.Runtime.Git.DefaultTimeout != "45s" {
		t.Fatalf("runtime git config = %+v, want configured git runtime settings", cfg.Runtime.Git)
	}
	if cfg.Runtime.LLMOutput.MaxPatchContentBytes != 4096 ||
		cfg.Runtime.LLMOutput.MaxGraphBytes != 16384 ||
		cfg.Runtime.LLMOutput.MaxGraphFileWrites != 2 ||
		cfg.Runtime.LLMOutput.SchemaInvalidLargePreviewThreshold != 8192 ||
		cfg.Runtime.LLMOutput.MaxArtifactChunkBytes != 4096 ||
		cfg.Runtime.LLMOutput.PreviousChunkTailBytes != 128 ||
		cfg.Runtime.LLMOutput.MaxChunkRetryAttempts != 4 ||
		cfg.Runtime.LLMOutput.MaxArtifactPlanAttempts != 3 ||
		cfg.Runtime.LLMOutput.MaxArtifactReplanAttempts != 2 {
		t.Fatalf("runtime llmOutput config = %+v, want configured thresholds", cfg.Runtime.LLMOutput)
	}
	if cfg.LLM.DefaultChat.ProviderID != BuiltinAIProxyProviderID || cfg.LLM.DefaultChat.ModelID != "gpt-4o" {
		t.Fatalf("default chat = %+v, want ai-proxy/gpt-4o", cfg.LLM.DefaultChat)
	}
	if cfg.LLM.DefaultEmbedding == nil || cfg.LLM.DefaultEmbedding.ModelID != "text-embedding-3-large" {
		t.Fatalf("default embedding = %+v, want text-embedding-3-large", cfg.LLM.DefaultEmbedding)
	}
	if cfg.LLM.Budget.ContextUtilizationRatio != 0.75 ||
		cfg.LLM.Budget.EstimatedTokensPerByte != 0.25 {
		t.Fatalf("budget = %+v, want configured budget policy", cfg.LLM.Budget)
	}
	if cfg.WebSearch.DefaultProvider != "search" ||
		cfg.WebSearch.Providers["search"].SearchEndpoint != "https://search.example.test/search" ||
		cfg.WebSearch.Providers["search"].Headers["X-WorkOrch-Client"] != "workorch" {
		t.Fatalf("web search config = %+v, want provider config", cfg.WebSearch)
	}
	if cfg.Governance.AutoTicket.Enabled == nil || !*cfg.Governance.AutoTicket.Enabled ||
		cfg.Governance.AutoTicket.MinConfidence != 0.82 ||
		cfg.Governance.AutoApproval.Enabled == nil || !*cfg.Governance.AutoApproval.Enabled ||
		cfg.Governance.AutoApproval.AllowReadOnly == nil || !*cfg.Governance.AutoApproval.AllowReadOnly ||
		len(cfg.Governance.AutoApproval.RequireHumanFor) != 2 ||
		cfg.Governance.Budget.MaxConsecutiveFailures != 2 {
		t.Fatalf("governance config = %+v, want auto governance settings", cfg.Governance)
	}
}

func TestGitRuntimeIsCoreCapability(t *testing.T) {
	if !GitRuntimeEnabled(GitRuntimeConfig{}) {
		t.Fatal("git runtime should be enabled by default")
	}
	enabled := true
	if !GitRuntimeEnabled(GitRuntimeConfig{Enabled: &enabled}) {
		t.Fatal("git runtime should remain enabled for legacy explicit true")
	}
	disabled := false
	if !GitRuntimeEnabled(GitRuntimeConfig{Enabled: &disabled}) {
		t.Fatal("git runtime should ignore legacy false because it is a core capability")
	}
	if !GitRuntimeWriteEnabled(GitRuntimeConfig{}) {
		t.Fatal("git runtime write should be enabled by default")
	}
	if !GitRuntimeWriteEnabled(GitRuntimeConfig{WriteEnabled: &enabled}) {
		t.Fatal("git runtime write should remain enabled for legacy explicit true")
	}
	if !GitRuntimeWriteEnabled(GitRuntimeConfig{WriteEnabled: &disabled}) {
		t.Fatal("git runtime write should ignore legacy false because it is a core capability")
	}
}

func TestSaveWritesApplicationConfigAndReloadsFrameworkConfig(t *testing.T) {
	configDir := writeApplicationConfig(t, validLLMApplicationTOML())
	_ = Close()
	t.Setenv(EnvConfigDir, configDir)
	t.Cleanup(func() { _ = Close() })

	cfg, err := Current()
	if err != nil {
		t.Fatalf("current config: %v", err)
	}
	cfg.Runtime.MaxRuntimeRepairAttempts = 15
	cfg.Runtime.MaxProviderRetryAttempts = 14
	cfg.Runtime.ShellCommandTimeout = "120s"
	cfg.Runtime.LLMOutput = LLMOutputRuntimeConfig{
		MaxPatchContentBytes:               8192,
		MaxGraphBytes:                      24576,
		MaxGraphFileWrites:                 2,
		SchemaInvalidLargePreviewThreshold: 4096,
		MaxArtifactChunkBytes:              4096,
		PreviousChunkTailBytes:             256,
		MaxChunkRetryAttempts:              4,
		MaxArtifactPlanAttempts:            3,
		MaxArtifactReplanAttempts:          2,
	}
	cfg.LLM.DefaultChat = ProviderModelTargetConfig{ProviderID: BuiltinAIProxyProviderID, ModelID: "gpt-4.1"}
	cfg.LLM.DefaultEmbedding = &ProviderModelTargetConfig{ProviderID: BuiltinAIProxyProviderID, ModelID: "text-embedding-3-small"}
	temperature := 0.0
	cfg.LLM.CustomProviders = map[string]CustomLLMProviderConfig{
		"direct-anthropic": {
			Provider:   "anthropic",
			APISurface: "anthropic-messages",
			BaseURL:    "https://api.anthropic.com",
			APIKey:     "anthropic-key",
			Timeout:    "180s",
			Models: map[string]CustomLLMModelConfig{
				"claude-sonnet-4-20250514": {
					ContextWindowTokens: 200000,
					MaxOutputTokens:     8192,
					Operations:          []string{LLMOperationChatCompletions},
					Temperature:         &temperature,
				},
			},
		},
	}
	cfg.WebSearch.DefaultProvider = "search"
	cfg.WebSearch.Providers = map[string]WebSearchProviderConfig{
		"search": {
			Provider:       "http",
			SearchEndpoint: "https://search.example.test/search",
			FetchEndpoint:  "https://search.example.test/fetch",
			APIKey:         "search-key",
			Timeout:        "20s",
			MaxResults:     8,
		},
	}
	trueValue := true
	cfg.Governance.AutoTicket = GovernanceAutoTicketConfig{
		Enabled:       &trueValue,
		MinConfidence: 0.85,
		MaxPerRun:     4,
	}
	cfg.Governance.AutoApproval = GovernanceAutoApprovalConfig{
		Enabled:             &trueValue,
		MinConfidence:       0.92,
		MaxPerRun:           2,
		AllowReadOnly:       &trueValue,
		AllowWorkspaceWrite: &trueValue,
		RequireHumanFor:     []string{"ssh_write", "production_operation"},
	}
	cfg.Governance.Budget = GovernanceBudgetConfig{
		MaxConsecutiveFailures: 3,
		MaxRepeatedSamePrompt:  1,
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := Reload(); err != nil {
		t.Fatalf("reload config: %v", err)
	}

	next, err := Current()
	if err != nil {
		t.Fatalf("current after reload: %v", err)
	}
	if next.LLM.DefaultChat.ModelID != "gpt-4.1" ||
		next.LLM.DefaultEmbedding == nil ||
		next.LLM.DefaultEmbedding.ModelID != "text-embedding-3-small" {
		t.Fatalf("next config = %+v, want saved llm targets", next.LLM)
	}
	custom, ok := next.LLM.CustomProviders["direct-anthropic"]
	if !ok || custom.Models["claude-sonnet-4-20250514"].MaxOutputTokens != 8192 {
		t.Fatalf("next custom providers = %+v, want saved custom model", next.LLM.CustomProviders)
	}
	if next.Runtime.MaxRuntimeRepairAttempts != 15 || next.Runtime.MaxProviderRetryAttempts != 14 || next.Runtime.ShellCommandTimeout != "120s" {
		t.Fatalf("next runtime config = %+v, want saved runtime settings", next.Runtime)
	}
	if next.Runtime.LLMOutput.MaxPatchContentBytes != 8192 ||
		next.Runtime.LLMOutput.MaxGraphBytes != 24576 ||
		next.Runtime.LLMOutput.MaxGraphFileWrites != 2 ||
		next.Runtime.LLMOutput.SchemaInvalidLargePreviewThreshold != 4096 ||
		next.Runtime.LLMOutput.MaxArtifactChunkBytes != 4096 ||
		next.Runtime.LLMOutput.PreviousChunkTailBytes != 256 ||
		next.Runtime.LLMOutput.MaxChunkRetryAttempts != 4 ||
		next.Runtime.LLMOutput.MaxArtifactPlanAttempts != 3 ||
		next.Runtime.LLMOutput.MaxArtifactReplanAttempts != 2 {
		t.Fatalf("next runtime llmOutput config = %+v, want saved thresholds", next.Runtime.LLMOutput)
	}
	if next.WebSearch.DefaultProvider != "search" ||
		next.WebSearch.Providers["search"].FetchEndpoint != "https://search.example.test/fetch" {
		t.Fatalf("next web search config = %+v, want saved web search provider", next.WebSearch)
	}
	if next.Governance.AutoTicket.Enabled == nil || !*next.Governance.AutoTicket.Enabled ||
		next.Governance.AutoTicket.MaxPerRun != 4 ||
		next.Governance.AutoApproval.MaxPerRun != 2 ||
		len(next.Governance.AutoApproval.RequireHumanFor) != 2 ||
		next.Governance.Budget.MaxConsecutiveFailures != 3 {
		t.Fatalf("next governance config = %+v, want saved governance config", next.Governance)
	}
}

func TestCurrentAppliesLLMOutputRuntimeDefaults(t *testing.T) {
	configDir := writeApplicationConfig(t, validLLMApplicationTOML())
	_ = Close()
	t.Setenv(EnvConfigDir, configDir)
	t.Cleanup(func() { _ = Close() })

	cfg, err := Current()
	if err != nil {
		t.Fatalf("current config: %v", err)
	}
	if cfg.Runtime.LLMOutput != DefaultLLMOutputRuntimeConfig() {
		t.Fatalf("llmOutput defaults = %+v, want %+v", cfg.Runtime.LLMOutput, DefaultLLMOutputRuntimeConfig())
	}
}

func TestCurrentRejectsInvalidLLMOutputRuntimeConfig(t *testing.T) {
	configDir := writeApplicationConfig(t, `
endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"

[repository]
backend = "file"
dataDir = ".workorch-data"

[llm.aiProxy]
baseURL = "http://127.0.0.1:8080"

[llm.defaultChat]
providerID = "ai-proxy"
modelID = "gpt-4o"

[runtime.llmOutput]
maxPatchContentBytes = 200000
maxGraphBytes = 131072
	`)
	_ = Close()
	t.Setenv(EnvConfigDir, configDir)
	t.Cleanup(func() { _ = Close() })

	if _, err := Current(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Current error = %v, want invalid argument", err)
	}
}

func TestCurrentRejectsLegacyLLMFields(t *testing.T) {
	configDir := writeApplicationConfig(t, `
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
model = "local-model"
	`)
	_ = Close()
	t.Setenv(EnvConfigDir, configDir)
	t.Cleanup(func() { _ = Close() })

	_, err := Current()
	if err == nil {
		t.Fatal("Current error = nil, want legacy llm field rejection")
	}
	if !errors.Is(err, common.ErrInvalidArgument) && !strings.Contains(err.Error(), "legacy") && !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Current error = %v, want legacy/unknown field rejection", err)
	}
}

func TestCurrentRejectsRemovedLLMBudgetCaps(t *testing.T) {
	for name, removedField := range map[string]string{
		"default output tokens": "defaultMaxOutputTokens = 4096",
		"max request bytes":     "maxRequestBytes = 4194304",
	} {
		t.Run(name, func(t *testing.T) {
			configDir := writeApplicationConfig(t, fmt.Sprintf(`
endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"

[repository]
backend = "file"
dataDir = ".workorch-data"

[llm.budget]
contextUtilizationRatio = 0.75
estimatedTokensPerByte = 0.25
%s
	`, removedField))
			_ = Close()
			t.Setenv(EnvConfigDir, configDir)
			t.Cleanup(func() { _ = Close() })

			if _, err := Current(); err == nil {
				t.Fatal("Current error = nil, want removed budget cap rejection")
			}
		})
	}
}

func TestValidateLLMConfigRejectsReservedCustomProviderID(t *testing.T) {
	cfg := DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8080"
	cfg.DefaultChat = ProviderModelTargetConfig{ProviderID: BuiltinAIProxyProviderID, ModelID: "gpt-4o"}
	cfg.CustomProviders = map[string]CustomLLMProviderConfig{
		BuiltinAIProxyProviderID: {
			Provider:   "openai",
			APISurface: "openai-compatible",
			BaseURL:    "https://api.openai.com",
			Models: map[string]CustomLLMModelConfig{
				"gpt-4o": {
					ContextWindowTokens: 128000,
					MaxOutputTokens:     4096,
					Operations:          []string{LLMOperationChatCompletions},
				},
			},
		},
	}
	if err := ValidateLLMConfig(cfg); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ValidateLLMConfig error = %v, want reserved custom provider rejection", err)
	}
}

func TestValidateLLMConfigRejectsAIProxyTimeoutAboveSharedCeiling(t *testing.T) {
	cfg := DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8080"
	cfg.AIProxy.Timeout = "1800s"
	cfg.DefaultChat = ProviderModelTargetConfig{ProviderID: BuiltinAIProxyProviderID, ModelID: "gpt-4o"}
	err := ValidateLLMConfig(cfg)
	if err == nil || !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ValidateLLMConfig error = %v, want timeout ceiling rejection", err)
	}
	if !strings.Contains(err.Error(), `llm.aiProxy.timeout "1800s" must not exceed 10m`) {
		t.Fatalf("ValidateLLMConfig error = %v, want actual timeout and ceiling", err)
	}
}

func TestDefaultAIProxyTimeoutUsesSharedDefault(t *testing.T) {
	cfg := DefaultAIProxyProviderConfig()
	if cfg.Timeout != DefaultAIProxyTimeout {
		t.Fatalf("default ai-proxy timeout = %q, want %q", cfg.Timeout, DefaultAIProxyTimeout)
	}
}

func TestValidateLLMConfigRejectsPartialAIProxyOverride(t *testing.T) {
	cfg := DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8080"
	cfg.DefaultChat = ProviderModelTargetConfig{ProviderID: BuiltinAIProxyProviderID, ModelID: "gpt-4o"}
	cfg.AIProxy.ModelOverrides = map[string]AIProxyModelOverrideConfig{
		"gpt-4o": {ContextWindowTokens: 128000},
	}
	if err := ValidateLLMConfig(cfg); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ValidateLLMConfig error = %v, want partial override rejection", err)
	}
}

func TestReloadAdvancesRevision(t *testing.T) {
	configDir := writeApplicationConfig(t, validLLMApplicationTOML())
	_ = Close()
	t.Setenv(EnvConfigDir, configDir)
	t.Cleanup(func() { _ = Close() })

	if _, err := Current(); err != nil {
		t.Fatalf("current config: %v", err)
	}
	before := Revision()
	if err := Reload(); err != nil {
		t.Fatalf("reload config: %v", err)
	}
	after := Revision()
	if after <= before {
		t.Fatalf("revision before=%d after=%d, want reload to advance revision", before, after)
	}
}

func TestConfigureDirectorySetsFrameworkConfigPath(t *testing.T) {
	configDir := t.TempDir()
	ConfigureDirectory(configDir)

	if got := os.Getenv(EnvConfigDir); got != configDir {
		t.Fatalf("%s = %q, want %q", EnvConfigDir, got, configDir)
	}
}

func TestWebSearchProviderValuesUsesFrameworkConfigFields(t *testing.T) {
	disabled := false
	cfg := Config{
		WebSearch: WebSearchConfig{
			DefaultProvider: "search",
			Providers: map[string]WebSearchProviderConfig{
				"search": {
					Provider:       "http",
					SearchEndpoint: "https://search.example.test/search",
					FetchEndpoint:  "https://search.example.test/fetch",
					APIKey:         "secret",
					Headers:        map[string]string{"X-Test": "true"},
					Timeout:        "20s",
					MaxResults:     8,
					MaxFetchBytes:  262144,
					MaxRedirects:   3,
					AllowDomains:   []string{"example.test"},
					DenyDomains:    []string{"blocked.example.test"},
					Disabled:       &disabled,
				},
			},
		},
	}

	values := cfg.WebSearchProviderValues()
	if values["provider"] != "http" ||
		values["searchEndpoint"] != "https://search.example.test/search" ||
		values["fetchEndpoint"] != "https://search.example.test/fetch" ||
		values["apiKey"] != "secret" ||
		values["maxResults"] != "8" ||
		values["maxFetchBytes"] != "262144" ||
		values["maxRedirects"] != "3" ||
		values["allowDomains"] != "example.test" ||
		values["denyDomains"] != "blocked.example.test" ||
		values["disabled"] != "false" ||
		values["header.X-Test"] != "true" {
		t.Fatalf("provider values = %+v, want web search provider bridge values", values)
	}
}

func validLLMApplicationTOML() string {
	return `
endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"

[repository]
backend = "file"
dataDir = ".workorch-data"

[llm.aiProxy]
baseURL = "http://127.0.0.1:8080"
apiKey = "proxy-key"
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

func writeApplicationConfig(t *testing.T, content string) string {
	t.Helper()

	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write application config: %v", err)
	}
	return configDir
}

func TestNormalizeLLMConfigCanonicalizesMapKeys(t *testing.T) {
	cfg := DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8080"
	cfg.DefaultChat = ProviderModelTargetConfig{ProviderID: BuiltinAIProxyProviderID, ModelID: "gpt-4o"}
	cfg.AIProxy.ModelOverrides = map[string]AIProxyModelOverrideConfig{
		"  gpt-4o  ": {
			ContextWindowTokens: 128000,
			MaxOutputTokens:     8192,
			Operations:          []string{" chat_completions ", "embeddings"},
		},
	}
	cfg.CustomProviders = map[string]CustomLLMProviderConfig{
		" custom-a ": {
			Provider:   "compatible",
			APISurface: "openai_compatible_http",
			BaseURL:    "http://127.0.0.1:9",
			Timeout:    "30s",
			Models: map[string]CustomLLMModelConfig{
				" model-1 ": {
					ContextWindowTokens: 128000,
					MaxOutputTokens:     8192,
					Operations:          []string{"chat_completions"},
				},
			},
		},
	}
	normalized, err := NormalizeLLMConfig(cfg)
	if err != nil {
		t.Fatalf("NormalizeLLMConfig: %v", err)
	}
	if _, ok := normalized.AIProxy.ModelOverrides["gpt-4o"]; !ok {
		t.Fatalf("model overrides keys = %#v, want trimmed gpt-4o", normalized.AIProxy.ModelOverrides)
	}
	if ops := normalized.AIProxy.ModelOverrides["gpt-4o"].Operations; len(ops) != 2 || ops[0] != "chat_completions" {
		t.Fatalf("override operations = %#v, want trimmed canonical ops", ops)
	}
	provider, ok := normalized.CustomProviders["custom-a"]
	if !ok {
		t.Fatalf("custom providers keys = %#v, want trimmed custom-a", normalized.CustomProviders)
	}
	if _, ok := provider.Models["model-1"]; !ok {
		t.Fatalf("custom model keys = %#v, want trimmed model-1", provider.Models)
	}
}

func TestNormalizeLLMConfigRejectsTrimDuplicateCustomProviderKeys(t *testing.T) {
	cfg := DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8080"
	cfg.DefaultChat = ProviderModelTargetConfig{ProviderID: BuiltinAIProxyProviderID, ModelID: "gpt-4o"}
	cfg.CustomProviders = map[string]CustomLLMProviderConfig{
		"custom-a": {
			Provider: "compatible", APISurface: "openai_compatible_http", BaseURL: "http://127.0.0.1:9", Timeout: "30s",
			Models: map[string]CustomLLMModelConfig{"model-1": {ContextWindowTokens: 128000, MaxOutputTokens: 8192, Operations: []string{"chat_completions"}}},
		},
		" custom-a ": {
			Provider: "compatible", APISurface: "openai_compatible_http", BaseURL: "http://127.0.0.1:10", Timeout: "30s",
			Models: map[string]CustomLLMModelConfig{"model-2": {ContextWindowTokens: 64000, MaxOutputTokens: 4096, Operations: []string{"chat_completions"}}},
		},
	}
	if _, err := NormalizeLLMConfig(cfg); err == nil {
		t.Fatal("expected trim-duplicate custom provider keys to fail before overwrite")
	}
}

func TestNormalizeLLMConfigRejectsCaseFoldDuplicateCustomProviderKeys(t *testing.T) {
	cfg := DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8080"
	cfg.DefaultChat = ProviderModelTargetConfig{ProviderID: BuiltinAIProxyProviderID, ModelID: "gpt-4o"}
	cfg.CustomProviders = map[string]CustomLLMProviderConfig{
		"Custom-A": {
			Provider: "compatible", APISurface: "openai_compatible_http", BaseURL: "http://127.0.0.1:9", Timeout: "30s",
			Models: map[string]CustomLLMModelConfig{"model-1": {ContextWindowTokens: 128000, MaxOutputTokens: 8192, Operations: []string{"chat_completions"}}},
		},
		"custom-a": {
			Provider: "compatible", APISurface: "openai_compatible_http", BaseURL: "http://127.0.0.1:10", Timeout: "30s",
			Models: map[string]CustomLLMModelConfig{"model-2": {ContextWindowTokens: 64000, MaxOutputTokens: 4096, Operations: []string{"chat_completions"}}},
		},
	}
	if _, err := NormalizeLLMConfig(cfg); err == nil {
		t.Fatal("expected case-fold-duplicate custom provider keys to fail before overwrite")
	}
}

func TestNormalizeLLMConfigRejectsTrimDuplicateModelOverrideKeys(t *testing.T) {
	cfg := DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8080"
	cfg.DefaultChat = ProviderModelTargetConfig{ProviderID: BuiltinAIProxyProviderID, ModelID: "gpt-4o"}
	cfg.AIProxy.ModelOverrides = map[string]AIProxyModelOverrideConfig{
		"gpt-4o":   {ContextWindowTokens: 128000, MaxOutputTokens: 8192, Operations: []string{"chat_completions"}},
		" gpt-4o ": {ContextWindowTokens: 64000, MaxOutputTokens: 4096, Operations: []string{"chat_completions"}},
	}
	if _, err := NormalizeLLMConfig(cfg); err == nil {
		t.Fatal("expected trim-duplicate model override keys to fail before overwrite")
	}
}

func TestNormalizeLLMConfigRejectsTrimDuplicateCustomModelKeys(t *testing.T) {
	cfg := DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8080"
	cfg.DefaultChat = ProviderModelTargetConfig{ProviderID: BuiltinAIProxyProviderID, ModelID: "gpt-4o"}
	cfg.CustomProviders = map[string]CustomLLMProviderConfig{
		"custom-a": {
			Provider: "compatible", APISurface: "openai_compatible_http", BaseURL: "http://127.0.0.1:9", Timeout: "30s",
			Models: map[string]CustomLLMModelConfig{
				"model-1":   {ContextWindowTokens: 128000, MaxOutputTokens: 8192, Operations: []string{"chat_completions"}},
				" model-1 ": {ContextWindowTokens: 64000, MaxOutputTokens: 4096, Operations: []string{"chat_completions"}},
			},
		},
	}
	if _, err := NormalizeLLMConfig(cfg); err == nil {
		t.Fatal("expected trim-duplicate custom model keys to fail before overwrite")
	}
}

func TestNormalizeLLMConfigPreservesCaseDistinctModelIDs(t *testing.T) {
	cfg := DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8080"
	cfg.DefaultChat = ProviderModelTargetConfig{ProviderID: BuiltinAIProxyProviderID, ModelID: "DeepSeek-V4-Flash"}
	cfg.AIProxy.ModelOverrides = map[string]AIProxyModelOverrideConfig{
		"DeepSeek-V4-Flash": {ContextWindowTokens: 1000000, MaxOutputTokens: 65536, Operations: []string{"chat_completions"}},
		"deepseek-v4-flash": {ContextWindowTokens: 1000000, MaxOutputTokens: 65536, Operations: []string{"chat_completions"}},
	}
	cfg.CustomProviders = map[string]CustomLLMProviderConfig{
		"custom-a": {
			Provider: "compatible", APISurface: "openai_compatible_http", BaseURL: "http://127.0.0.1:9", Timeout: "30s",
			Models: map[string]CustomLLMModelConfig{
				"DeepSeek-V4-Flash": {ContextWindowTokens: 1000000, MaxOutputTokens: 65536, Operations: []string{"chat_completions"}},
				"deepseek-v4-flash": {ContextWindowTokens: 1000000, MaxOutputTokens: 65536, Operations: []string{"chat_completions"}},
			},
		},
	}

	normalized, err := NormalizeLLMConfig(cfg)
	if err != nil {
		t.Fatalf("NormalizeLLMConfig: %v", err)
	}
	if len(normalized.AIProxy.ModelOverrides) != 2 || len(normalized.CustomProviders["custom-a"].Models) != 2 {
		t.Fatalf("case-distinct model IDs were collapsed: %+v", normalized)
	}
}

package workorchconfig

import (
	"errors"
	"os"
	"path/filepath"
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
enabled = true
writeEnabled = true
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

[llm]
defaultProvider = "local"
defaultEmbeddingProvider = "local"

[llm.providers.local]
provider = "compatible"
baseURL = "http://127.0.0.1:11434/v1"
apiKey = "local-key"
model = "local-model"
timeout = "60s"
chatCompletions = true
streaming = true
nonChatOperations = ["embeddings"]
contextWindowTokens = 32768
maxInputTokens = 16384
reservedOutputTokens = 4096
safetyRatio = 0.8
bytesHardLimit = 98304
estimatedTokensPerByte = 0.25
	disabled = false

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
	if cfg.Server.Addr != "127.0.0.1:18080" || cfg.Repository.Backend != "file" || cfg.LLM.Providers["local"].Provider != "compatible" {
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
	if !GitRuntimeEnabled(cfg.Runtime.Git) ||
		!GitRuntimeWriteEnabled(cfg.Runtime.Git) ||
		cfg.Runtime.Git.DefaultTimeout != "45s" {
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
	if cfg.LLM.DefaultProvider != "local" {
		t.Fatalf("default provider = %q, want local", cfg.LLM.DefaultProvider)
	}
	if cfg.LLM.DefaultEmbeddingProvider != "local" {
		t.Fatalf("default embedding provider = %q, want local", cfg.LLM.DefaultEmbeddingProvider)
	}
	if cfg.LLM.Providers["local"].Disabled == nil || *cfg.LLM.Providers["local"].Disabled {
		t.Fatalf("llm disabled = %+v, want false pointer", cfg.LLM.Providers["local"].Disabled)
	}
	if cfg.LLM.Providers["local"].Streaming == nil || !*cfg.LLM.Providers["local"].Streaming ||
		len(cfg.LLM.Providers["local"].NonChatOperations) != 1 ||
		cfg.LLM.Providers["local"].NonChatOperations[0] != "embeddings" {
		t.Fatalf("llm provider capabilities = %+v, want streaming embeddings", cfg.LLM.Providers["local"])
	}
	if cfg.LLM.Providers["local"].ContextWindowTokens != 32768 ||
		cfg.LLM.Providers["local"].MaxInputTokens != 16384 ||
		cfg.LLM.Providers["local"].ReservedOutputTokens != 4096 ||
		cfg.LLM.Providers["local"].SafetyRatio != 0.8 ||
		cfg.LLM.Providers["local"].BytesHardLimit != 98304 ||
		cfg.LLM.Providers["local"].EstimatedTokensPerByte != 0.25 {
		t.Fatalf("llm provider profile = %+v, want context profile values", cfg.LLM.Providers["local"])
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

func TestGitRuntimeWriteEnabledRequiresExplicitOptIn(t *testing.T) {
	if GitRuntimeWriteEnabled(GitRuntimeConfig{}) {
		t.Fatal("git runtime write should require explicit opt-in")
	}
	enabled := true
	if !GitRuntimeWriteEnabled(GitRuntimeConfig{WriteEnabled: &enabled}) {
		t.Fatal("git runtime write should respect explicit true")
	}
	disabled := false
	if GitRuntimeWriteEnabled(GitRuntimeConfig{WriteEnabled: &disabled}) {
		t.Fatal("git runtime write should respect explicit false")
	}
}

func TestSaveWritesApplicationConfigAndReloadsFrameworkConfig(t *testing.T) {
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
apiKey = "local-key"
model = "local-model"
timeout = "60s"
contextWindowTokens = 32768
maxInputTokens = 16384
reservedOutputTokens = 4096
safetyRatio = 0.8
bytesHardLimit = 98304
estimatedTokensPerByte = 0.25
disabled = false
	`)
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
	disabled := false
	cfg.LLM.DefaultProvider = "reasoning"
	cfg.LLM.DefaultEmbeddingProvider = "reasoning"
	cfg.LLM.Providers = map[string]LLMProviderConfig{
		"reasoning": {
			Provider:               "openai",
			APIKey:                 "reasoning-key",
			Model:                  "reasoning-model",
			Timeout:                "45s",
			Disabled:               &disabled,
			Temperature:            float64Ptr(0),
			MaxOutputTokens:        65536,
			ReasoningEffort:        "high",
			ThinkingBudgetTokens:   32768,
			ContextWindowTokens:    64000,
			MaxInputTokens:         32000,
			ReservedOutputTokens:   4096,
			SafetyRatio:            0.8,
			BytesHardLimit:         131072,
			EstimatedTokensPerByte: 0.25,
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
	if next.LLM.DefaultProvider != "reasoning" ||
		next.LLM.DefaultEmbeddingProvider != "reasoning" ||
		next.LLM.Providers["reasoning"].Model != "reasoning-model" {
		t.Fatalf("next config = %+v, want saved llm provider", next.LLM)
	}
	if next.LLM.Providers["reasoning"].ContextWindowTokens != 64000 ||
		next.LLM.Providers["reasoning"].MaxInputTokens != 32000 ||
		next.LLM.Providers["reasoning"].BytesHardLimit != 131072 {
		t.Fatalf("next llm provider profile = %+v, want saved context profile", next.LLM.Providers["reasoning"])
	}
	if next.LLM.Providers["reasoning"].Temperature == nil ||
		*next.LLM.Providers["reasoning"].Temperature != 0 ||
		next.LLM.Providers["reasoning"].MaxOutputTokens != 65536 ||
		next.LLM.Providers["reasoning"].ReasoningEffort != "high" ||
		next.LLM.Providers["reasoning"].ThinkingBudgetTokens != 32768 {
		t.Fatalf("next llm inference profile = %+v, want saved inference profile", next.LLM.Providers["reasoning"])
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
	configDir := writeApplicationConfig(t, `
endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"

[repository]
backend = "file"
dataDir = ".workorch-data"
	`)
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

[runtime.llmOutput]
maxPatchContentBytes = 50000
maxGraphBytes = 32768
	`)
	_ = Close()
	t.Setenv(EnvConfigDir, configDir)
	t.Cleanup(func() { _ = Close() })

	if _, err := Current(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Current error = %v, want invalid argument", err)
	}
}

func TestReloadAdvancesRevision(t *testing.T) {
	configDir := writeApplicationConfig(t, `
endpointName = "workorch"

[server]
addr = "127.0.0.1:18080"

[repository]
backend = "file"
dataDir = ".workorch-data"
	`)
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

func TestLLMProviderValuesUsesFrameworkConfigFields(t *testing.T) {
	disabled := true
	cacheMarker := true
	cfg := Config{
		LLM: LLMConfig{
			DefaultProvider: "reasoning",
			Providers: map[string]LLMProviderConfig{
				"reasoning": {
					Provider:               "openai",
					APISurface:             "openai_sdk",
					BaseURL:                "https://example.test/v1",
					APIKey:                 "secret",
					Model:                  "model",
					Timeout:                "30s",
					Disabled:               &disabled,
					Streaming:              &disabled,
					NonChatOperations:      []string{"embeddings"},
					Temperature:            float64Ptr(0),
					MaxOutputTokens:        65536,
					ReasoningEffort:        "high",
					ThinkingBudgetTokens:   32768,
					CacheMode:              "prompt_cache_key",
					PromptCacheKey:         &cacheMarker,
					CacheUsageFields:       []string{"cached_tokens"},
					ContextProfileMode:     "manual",
					ContextWindowTokens:    32768,
					MaxInputTokens:         8192,
					ReservedOutputTokens:   2048,
					SafetyRatio:            0.8,
					BytesHardLimit:         65536,
					EstimatedTokensPerByte: 0.25,
				},
			},
		},
	}

	values := cfg.LLMProviderValues()
	if values["provider"] != "openai" ||
		values["apiSurface"] != "openai_sdk" ||
		values["disabled"] != "true" ||
		values["streaming"] != "true" ||
		values["nonChatOperations"] != "embeddings" ||
		values["temperature"] != "0" ||
		values["maxOutputTokens"] != "65536" ||
		values["reasoningEffort"] != "high" ||
		values["thinkingBudgetTokens"] != "32768" ||
		values["cacheMode"] != "prompt_cache_key" ||
		values["promptCacheKey"] != "true" ||
		values["cacheUsageFields"] != "cached_tokens" ||
		values["contextProfileMode"] != "manual" ||
		values["contextWindowTokens"] != "32768" ||
		values["maxInputTokens"] != "8192" ||
		values["reservedOutputTokens"] != "2048" ||
		values["safetyRatio"] != "0.8" ||
		values["bytesHardLimit"] != "65536" ||
		values["estimatedTokensPerByte"] != "0.25" {
		t.Fatalf("provider values = %+v, want llm provider bridge values", values)
	}
	valueSets := cfg.LLMProviderValueSets()
	if valueSets["reasoning"]["model"] != "model" {
		t.Fatalf("provider value sets = %+v, want reasoning model", valueSets)
	}
}

func TestLLMProviderValueSetsResolveConfiguredModelCapabilities(t *testing.T) {
	cfg := Config{
		LLM: LLMConfig{
			DefaultProvider: "reasoning",
			ModelCapabilities: map[string]LLMModelCapabilityConfig{
				"codex-large": {
					Provider:            "compatible",
					ModelPattern:        "*codex*",
					ContextWindowTokens: 1_000_000,
				},
			},
			Providers: map[string]LLMProviderConfig{
				"reasoning": {
					Provider:           "compatible",
					Model:              "gpt-5.3-codex",
					ContextProfileMode: "auto",
				},
			},
		},
	}

	values := cfg.LLMProviderValues()
	if values["contextWindowTokens"] != "1000000" ||
		values["contextCapabilitySource"] != "model_registry" {
		t.Fatalf("provider values = %+v, want model registry capability projected", values)
	}
}

func TestLLMProviderValuesOmitsAdvancedContextFieldsInAutoMode(t *testing.T) {
	cfg := Config{
		LLM: LLMConfig{
			DefaultProvider: "reasoning",
			Providers: map[string]LLMProviderConfig{
				"reasoning": {
					Provider:                "openai",
					Model:                   "model",
					ContextProfileMode:      "auto",
					ContextWindowTokens:     204800,
					MaxInputTokens:          159040,
					ReservedOutputTokens:    4096,
					SafetyRatio:             0.8,
					ContextUtilizationRatio: 0.75,
					BytesHardLimit:          98304,
					MaxRequestBytes:         4194304,
					EstimatedTokensPerByte:  0.25,
				},
			},
		},
	}

	values := cfg.LLMProviderValues()
	if values["contextProfileMode"] != "auto" ||
		values["contextWindowTokens"] != "204800" {
		t.Fatalf("provider values = %+v, want auto mode with context window", values)
	}
	for _, key := range []string{"maxInputTokens", "reservedOutputTokens", "safetyRatio", "contextUtilizationRatio", "bytesHardLimit", "maxRequestBytes", "estimatedTokensPerByte"} {
		if _, exists := values[key]; exists {
			t.Fatalf("provider values = %+v, want %s omitted in auto mode", values, key)
		}
	}
}

func float64Ptr(value float64) *float64 {
	return &value
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

func writeApplicationConfig(t *testing.T, content string) string {
	t.Helper()

	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write application config: %v", err)
	}
	return configDir
}

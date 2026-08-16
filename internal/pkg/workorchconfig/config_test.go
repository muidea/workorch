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
maxDocumentRecoveryAttempts = 4

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
	if cfg.Runtime.LLMOutput.MaxDocumentRecoveryAttempts != 4 {
		t.Fatalf("runtime llmOutput config = %+v, want configured document recovery limit", cfg.Runtime.LLMOutput)
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
		MaxDocumentRecoveryAttempts: 4,
	}
	cfg.LLM.DefaultChat = ProviderModelTargetConfig{ProviderID: BuiltinAIProxyProviderID, ModelID: "gpt-4.1"}
	cfg.LLM.DefaultEmbedding = &ProviderModelTargetConfig{ProviderID: BuiltinAIProxyProviderID, ModelID: "text-embedding-3-small"}
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
	if next.Runtime.MaxRuntimeRepairAttempts != 15 || next.Runtime.MaxProviderRetryAttempts != 14 || next.Runtime.ShellCommandTimeout != "120s" {
		t.Fatalf("next runtime config = %+v, want saved runtime settings", next.Runtime)
	}
	if next.Runtime.LLMOutput.MaxDocumentRecoveryAttempts != 4 {
		t.Fatalf("next runtime llmOutput config = %+v, want saved document recovery limit", next.Runtime.LLMOutput)
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
maxDocumentRecoveryAttempts = -1
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

func TestValidateLLMConfigRejectsNonAIProxyTarget(t *testing.T) {
	cfg := DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8080"
	cfg.DefaultChat = ProviderModelTargetConfig{ProviderID: "direct-provider", ModelID: "gpt-4o"}
	if err := ValidateLLMConfig(cfg); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("ValidateLLMConfig error = %v, want non-ai-proxy target rejection", err)
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

func TestLLMConfigurationRevisionCoversStagedLLMFields(t *testing.T) {
	cfg := DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8090"
	cfg.AIProxy.APIKey = "old-key"
	cfg.DefaultChat = ProviderModelTargetConfig{ProviderID: BuiltinAIProxyProviderID, ModelID: "model-a"}
	cfg.AIProxy.ModelOverrides = map[string]AIProxyModelOverrideConfig{
		"model-a": {ContextWindowTokens: 128000, MaxOutputTokens: 8192, Operations: []string{LLMOperationResponses}},
	}
	first, err := LLMConfigurationRevision(cfg)
	if err != nil {
		t.Fatal(err)
	}

	updated := CloneLLMConfig(cfg)
	updated.AIProxy.APIKey = "new-key" // same semantic shape, different secret
	second, err := LLMConfigurationRevision(updated)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("API key rotation must change LLM configuration revision")
	}

	updated = CloneLLMConfig(cfg)
	updated.DefaultChat.ModelID = "model-b"
	third, err := LLMConfigurationRevision(updated)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("default model change must change LLM configuration revision")
	}

	updated = CloneLLMConfig(cfg)
	updated.Budget.ContextUtilizationRatio = 0.7
	fourth, err := LLMConfigurationRevision(updated)
	if err != nil {
		t.Fatal(err)
	}
	if first == fourth {
		t.Fatal("budget policy change must change LLM configuration revision")
	}
}

func TestCloneLLMConfigDoesNotShareMutableValues(t *testing.T) {
	cfg := DefaultLLMConfig()
	cfg.DefaultEmbedding = &ProviderModelTargetConfig{ProviderID: BuiltinAIProxyProviderID, ModelID: "embed-a"}
	cfg.AIProxy.ModelOverrides = map[string]AIProxyModelOverrideConfig{
		"model-a": {Operations: []string{LLMOperationResponses}},
	}
	cloned := CloneLLMConfig(cfg)
	cloned.DefaultEmbedding.ModelID = "embed-b"
	override := cloned.AIProxy.ModelOverrides["model-a"]
	override.Operations[0] = LLMOperationEmbeddings
	cloned.AIProxy.ModelOverrides["model-a"] = override
	if cfg.DefaultEmbedding.ModelID != "embed-a" || cfg.AIProxy.ModelOverrides["model-a"].Operations[0] != LLMOperationResponses {
		t.Fatalf("CloneLLMConfig retained caller-owned storage: %+v", cfg)
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

func TestSaveReplacesApplicationConfigAtomically(t *testing.T) {
	configDir := writeApplicationConfig(t, validLLMApplicationTOML())
	t.Setenv(EnvConfigDir, configDir)
	t.Cleanup(func() { _ = Close() })

	cfg, err := Persisted()
	if err != nil {
		t.Fatal(err)
	}
	cfg.LLM.DefaultChat.ModelID = "gpt-atomic"
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	persisted, err := Persisted()
	if err != nil {
		t.Fatal(err)
	}
	if persisted.LLM.AIProxy.BaseURL != "http://127.0.0.1:8080" || persisted.LLM.DefaultChat.ModelID != "gpt-atomic" {
		t.Fatalf("persisted config = %+v, want complete atomically replaced LLM section", persisted.LLM)
	}
	temporaryFiles, err := filepath.Glob(filepath.Join(configDir, ".application.toml-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporaryFiles) != 0 {
		t.Fatalf("temporary config files remain after save: %v", temporaryFiles)
	}
}

func TestNormalizeLLMConfigCanonicalizesMapKeys(t *testing.T) {
	cfg := DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8080"
	cfg.DefaultChat = ProviderModelTargetConfig{ProviderID: BuiltinAIProxyProviderID, ModelID: "gpt-4o"}
	cfg.AIProxy.ModelOverrides = map[string]AIProxyModelOverrideConfig{
		"  gpt-4o  ": {
			ContextWindowTokens: 128000,
			MaxOutputTokens:     8192,
			Operations:          []string{" responses ", "embeddings"},
		},
	}
	normalized, err := NormalizeLLMConfig(cfg)
	if err != nil {
		t.Fatalf("NormalizeLLMConfig: %v", err)
	}
	if _, ok := normalized.AIProxy.ModelOverrides["gpt-4o"]; !ok {
		t.Fatalf("model overrides keys = %#v, want trimmed gpt-4o", normalized.AIProxy.ModelOverrides)
	}
	if ops := normalized.AIProxy.ModelOverrides["gpt-4o"].Operations; len(ops) != 2 || ops[0] != "responses" {
		t.Fatalf("override operations = %#v, want trimmed canonical ops", ops)
	}
}

func TestNormalizeLLMConfigRejectsTrimDuplicateModelOverrideKeys(t *testing.T) {
	cfg := DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8080"
	cfg.DefaultChat = ProviderModelTargetConfig{ProviderID: BuiltinAIProxyProviderID, ModelID: "gpt-4o"}
	cfg.AIProxy.ModelOverrides = map[string]AIProxyModelOverrideConfig{
		"gpt-4o":   {ContextWindowTokens: 128000, MaxOutputTokens: 8192, Operations: []string{"responses"}},
		" gpt-4o ": {ContextWindowTokens: 64000, MaxOutputTokens: 4096, Operations: []string{"responses"}},
	}
	if _, err := NormalizeLLMConfig(cfg); err == nil {
		t.Fatal("expected trim-duplicate model override keys to fail before overwrite")
	}
}

func TestNormalizeLLMConfigPreservesCaseDistinctModelIDs(t *testing.T) {
	cfg := DefaultLLMConfig()
	cfg.AIProxy.BaseURL = "http://127.0.0.1:8080"
	cfg.DefaultChat = ProviderModelTargetConfig{ProviderID: BuiltinAIProxyProviderID, ModelID: "DeepSeek-V4-Flash"}
	cfg.AIProxy.ModelOverrides = map[string]AIProxyModelOverrideConfig{
		"DeepSeek-V4-Flash": {ContextWindowTokens: 1000000, MaxOutputTokens: 65536, Operations: []string{"responses"}},
		"deepseek-v4-flash": {ContextWindowTokens: 1000000, MaxOutputTokens: 65536, Operations: []string{"responses"}},
	}
	normalized, err := NormalizeLLMConfig(cfg)
	if err != nil {
		t.Fatalf("NormalizeLLMConfig: %v", err)
	}
	if len(normalized.AIProxy.ModelOverrides) != 2 {
		t.Fatalf("case-distinct model IDs were collapsed: %+v", normalized)
	}
}

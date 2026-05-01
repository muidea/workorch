package workorchconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCurrentReadsMagicCommonApplicationConfig(t *testing.T) {
	configDir := writeApplicationConfig(t, `
endpointName = "workorchd"

[server]
addr = "127.0.0.1:18080"
readHeaderTimeout = "5s"
readTimeout = "15s"
writeTimeout = "30s"
idleTimeout = "60s"
shutdownTimeout = "5s"
maxConcurrentRuns = 2

[repository]
backend = "file"
dataDir = ".workorch-data"

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
	disabled = false
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
	if cfg.Server.MaxConcurrentRuns != 2 {
		t.Fatalf("max concurrent runs = %d, want 2", cfg.Server.MaxConcurrentRuns)
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
}

func TestSaveWritesApplicationConfigAndReloadsFrameworkConfig(t *testing.T) {
	configDir := writeApplicationConfig(t, `
endpointName = "workorchd"

[server]
addr = "127.0.0.1:18080"

[repository]
backend = "memory"

[llm]
defaultProvider = "local"

[llm.providers.local]
provider = "compatible"
baseURL = "http://127.0.0.1:11434/v1"
apiKey = "local-key"
model = "local-model"
timeout = "60s"
disabled = false
	`)
	_ = Close()
	t.Setenv(EnvConfigDir, configDir)
	t.Cleanup(func() { _ = Close() })

	cfg, err := Current()
	if err != nil {
		t.Fatalf("current config: %v", err)
	}
	disabled := false
	cfg.LLM.DefaultProvider = "reasoning"
	cfg.LLM.DefaultEmbeddingProvider = "reasoning"
	cfg.LLM.Providers = map[string]LLMProviderConfig{
		"reasoning": {
			Provider: "openai",
			APIKey:   "reasoning-key",
			Model:    "reasoning-model",
			Timeout:  "45s",
			Disabled: &disabled,
		},
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
}

func TestReloadAdvancesRevision(t *testing.T) {
	configDir := writeApplicationConfig(t, `
endpointName = "workorchd"

[server]
addr = "127.0.0.1:18080"

[repository]
backend = "memory"
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
	cfg := Config{
		LLM: LLMConfig{
			DefaultProvider: "reasoning",
			Providers: map[string]LLMProviderConfig{
				"reasoning": {
					Provider:          "openai",
					BaseURL:           "https://example.test/v1",
					APIKey:            "secret",
					Model:             "model",
					Timeout:           "30s",
					Disabled:          &disabled,
					Streaming:         &disabled,
					NonChatOperations: []string{"embeddings"},
				},
			},
		},
	}

	values := cfg.LLMProviderValues()
	if values["provider"] != "openai" || values["disabled"] != "true" || values["streaming"] != "true" || values["nonChatOperations"] != "embeddings" {
		t.Fatalf("provider values = %+v, want llm provider bridge values", values)
	}
	valueSets := cfg.LLMProviderValueSets()
	if valueSets["reasoning"]["model"] != "model" {
		t.Fatalf("provider value sets = %+v, want reasoning model", valueSets)
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

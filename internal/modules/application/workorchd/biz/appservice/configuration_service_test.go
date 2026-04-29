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
endpointName = "workorchd"

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
}

func TestUpdateLLMConfigurationPersistsAndReloadsProviders(t *testing.T) {
	configDir := t.TempDir()
	content := `
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
}

func TestUpdateLLMConfigurationRejectsMissingDefaultProvider(t *testing.T) {
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(`endpointName = "workorchd"`), 0o600); err != nil {
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
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(`endpointName = "workorchd"`), 0o600); err != nil {
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
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(`endpointName = "workorchd"`), 0o600); err != nil {
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

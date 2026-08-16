package configcore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestUpdateRuntimeConfigurationPersistsRepairBudgetAndPreservesOmittedFields(t *testing.T) {
	useRuntimeUpdateTestConfig(t)

	maxRuntimeRepairAttempts := 3
	maxDocumentRecoveryAttempts := 4
	item, err := UpdateRuntimeConfiguration(UpdateRuntimeConfigurationCommand{
		MaxRuntimeRepairAttempts: &maxRuntimeRepairAttempts,
		LLMOutput: &UpdateRuntimeLLMOutputConfiguration{
			MaxDocumentRecoveryAttempts: &maxDocumentRecoveryAttempts,
		},
	})
	if err != nil {
		t.Fatalf("UpdateRuntimeConfiguration: %v", err)
	}
	if item.Runtime.MaxRuntimeRepairAttempts != 3 {
		t.Fatalf("maxRuntimeRepairAttempts = %d, want 3", item.Runtime.MaxRuntimeRepairAttempts)
	}
	if item.Runtime.MaxProviderRetryAttempts != 7 {
		t.Fatalf("maxProviderRetryAttempts = %d, want preserved 7", item.Runtime.MaxProviderRetryAttempts)
	}
	if item.Runtime.ShellCommandTimeout != "45s" {
		t.Fatalf("shellCommandTimeout = %q, want preserved 45s", item.Runtime.ShellCommandTimeout)
	}
	if item.Runtime.Git.DefaultTimeout != "35s" {
		t.Fatalf("git runtime config = %+v, want preserved git runtime timeout", item.Runtime.Git)
	}
	if item.Runtime.LLMOutput.MaxDocumentRecoveryAttempts != 4 {
		t.Fatalf("maxDocumentRecoveryAttempts = %d, want 4", item.Runtime.LLMOutput.MaxDocumentRecoveryAttempts)
	}

	readBack, err := workorchconfig.Current()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if readBack.Runtime.MaxRuntimeRepairAttempts != 3 ||
		readBack.Runtime.LLMOutput.MaxDocumentRecoveryAttempts != 4 ||
		!workorchconfig.GitRuntimeWriteEnabled(readBack.Runtime.Git) ||
		readBack.Runtime.Git.DefaultTimeout != "35s" {
		t.Fatalf("persisted runtime config = %+v, want updated repair and document recovery budgets", readBack.Runtime)
	}
}

func TestUpdateRuntimeConfigurationRejectsInvalidValues(t *testing.T) {
	useRuntimeUpdateTestConfig(t)

	zero := 0
	if _, err := UpdateRuntimeConfiguration(UpdateRuntimeConfigurationCommand{MaxRuntimeRepairAttempts: &zero}); err == nil {
		t.Fatal("UpdateRuntimeConfiguration accepted zero maxRuntimeRepairAttempts")
	}
	invalidTimeout := "not-a-duration"
	if _, err := UpdateRuntimeConfiguration(UpdateRuntimeConfigurationCommand{ShellCommandTimeout: &invalidTimeout}); err == nil {
		t.Fatal("UpdateRuntimeConfiguration accepted invalid shellCommandTimeout")
	}
	negativeDocumentRecoveryAttempts := -1
	if _, err := UpdateRuntimeConfiguration(UpdateRuntimeConfigurationCommand{
		LLMOutput: &UpdateRuntimeLLMOutputConfiguration{MaxDocumentRecoveryAttempts: &negativeDocumentRecoveryAttempts},
	}); err == nil {
		t.Fatal("UpdateRuntimeConfiguration accepted invalid llmOutput maxDocumentRecoveryAttempts")
	}
}

func useRuntimeUpdateTestConfig(t *testing.T) {
	t.Helper()
	_ = workorchconfig.Close()
	configDir := t.TempDir()
	content := `endpointName = "workorch"

[server]
addr = "127.0.0.1:0"
maxConcurrentRuns = 1

[repository]
backend = "file"
dataDir = ".workorch-data"

[runtime]
maxRuntimeRepairAttempts = 5
maxProviderRetryAttempts = 7
shellCommandTimeout = "45s"

[runtime.git]
defaultTimeout = "35s"

[runtime.llmOutput]
maxDocumentRecoveryAttempts = 2

[llm.aiProxy]
baseURL = "http://127.0.0.1:8080"
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
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
}

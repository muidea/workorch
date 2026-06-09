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
	maxGraphBytes := 65536
	item, err := UpdateRuntimeConfiguration(UpdateRuntimeConfigurationCommand{
		MaxRuntimeRepairAttempts: &maxRuntimeRepairAttempts,
		LLMOutput: &UpdateRuntimeLLMOutputConfiguration{
			MaxGraphBytes: &maxGraphBytes,
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
	if item.Runtime.LLMOutput.MaxGraphBytes != 65536 {
		t.Fatalf("maxGraphBytes = %d, want 65536", item.Runtime.LLMOutput.MaxGraphBytes)
	}
	if item.Runtime.LLMOutput.MaxPatchContentBytes != 8192 {
		t.Fatalf("maxPatchContentBytes = %d, want preserved 8192", item.Runtime.LLMOutput.MaxPatchContentBytes)
	}

	readBack, err := workorchconfig.Current()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if readBack.Runtime.MaxRuntimeRepairAttempts != 3 || readBack.Runtime.LLMOutput.MaxGraphBytes != 65536 {
		t.Fatalf("persisted runtime config = %+v, want updated repair budget and graph bytes", readBack.Runtime)
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
	negativeGraphBytes := -1
	if _, err := UpdateRuntimeConfiguration(UpdateRuntimeConfigurationCommand{
		LLMOutput: &UpdateRuntimeLLMOutputConfiguration{MaxGraphBytes: &negativeGraphBytes},
	}); err == nil {
		t.Fatal("UpdateRuntimeConfiguration accepted invalid llmOutput maxGraphBytes")
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

[runtime.llmOutput]
maxPatchContentBytes = 8192
maxGraphBytes = 32768
maxGraphFileWrites = 3
schemaInvalidLargePreviewThreshold = 4096
maxArtifactChunkBytes = 4096
previousChunkTailBytes = 256
maxChunkRetryAttempts = 2
maxArtifactPlanAttempts = 2
maxArtifactReplanAttempts = 2
`
	if err := os.WriteFile(filepath.Join(configDir, "application.toml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv(workorchconfig.EnvConfigDir, configDir)
	t.Cleanup(func() { _ = workorchconfig.Close() })
}

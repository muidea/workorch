package configcore

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func TestConfigurationUpdatesRejectUnknownPersistedServerFieldWithoutOverwrite(t *testing.T) {
	tests := map[string]func() error{
		"llm update": func() error {
			modelID := "gpt-updated"
			_, err := UpdateLLMConfiguration(UpdateLLMConfigurationCommand{DefaultChatModelID: &modelID})
			return err
		},
		"representative runtime update": func() error {
			maxRuntimeRepairAttempts := 3
			_, err := UpdateRuntimeConfiguration(UpdateRuntimeConfigurationCommand{MaxRuntimeRepairAttempts: &maxRuntimeRepairAttempts})
			return err
		},
	}

	for name, update := range tests {
		t.Run(name, func(t *testing.T) {
			useRuntimeUpdateTestConfig(t)
			configPath, before := addRetiredServerRunLimit(t)

			err := update()
			if !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("update error = %v, want invalid persisted configuration", err)
			}

			after, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("read application config after rejected update: %v", err)
			}
			if !bytes.Equal(after, before) {
				t.Fatal("rejected update overwrote the persisted configuration")
			}
		})
	}
}

func addRetiredServerRunLimit(t *testing.T) (string, []byte) {
	t.Helper()

	configPath, err := workorchconfig.ApplicationConfigPath()
	if err != nil {
		t.Fatalf("resolve application config: %v", err)
	}
	payload, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read application config: %v", err)
	}
	serverAddr := "addr = \"127.0.0.1:0\""
	retiredField := "max" + "ConcurrentRuns"
	updated := strings.Replace(string(payload), serverAddr, fmt.Sprintf("%s\n%s = 2", serverAddr, retiredField), 1)
	if updated == string(payload) {
		t.Fatal("server fixture was not updated with the retired field")
	}
	if err := os.WriteFile(configPath, []byte(updated), 0o600); err != nil {
		t.Fatalf("write application config with retired field: %v", err)
	}
	return configPath, []byte(updated)
}

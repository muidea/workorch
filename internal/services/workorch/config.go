package workorch

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/pelletier/go-toml/v2"
)

func ConfigureDirectory(configDir string) {
	workorchconfig.ConfigureDirectory(ResolveConfigDir(configDir))
}

func ResolveConfigDir(flagDir string) string {
	if flagDir != "" {
		return flagDir
	}
	if env := os.Getenv("WORKORCH_CONFIG_DIR"); env != "" {
		return env
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".workorch")
	}
	return ""
}

func EnsureConfigFile(dir string) error {
	if dir == "" {
		return nil
	}
	configPath := filepath.Join(dir, "application.toml")
	if _, err := os.Stat(configPath); err == nil {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}
	defaultCfg := map[string]any{
		"endpointName": "workorch",
		"server": map[string]any{
			"addr":              ":8080",
			"readHeaderTimeout": "5s",
			"readTimeout":       "15s",
			"writeTimeout":      "180s",
			"idleTimeout":       "60s",
			"shutdownTimeout":   "5s",
		},
		"repository": map[string]any{
			"backend": "file",
			"dataDir": ".workorch-data",
		},
	}
	data, err := toml.Marshal(defaultCfg)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0o644)
}

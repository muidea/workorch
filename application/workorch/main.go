package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/muidea/magicCommon/framework/application"
	"github.com/muidea/magicCommon/framework/service"
	"github.com/pelletier/go-toml/v2"
	"github.com/muidea/workorch/internal/modules/application/workorch/service/cli"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"

	_ "github.com/muidea/workorch/internal/initiators/workorchruntime"
	_ "github.com/muidea/workorch/internal/modules/application/workorch"
	_ "github.com/muidea/workorch/internal/modules/blocks/approval"
	_ "github.com/muidea/workorch/internal/modules/blocks/llm"
	_ "github.com/muidea/workorch/internal/modules/blocks/mcp"
	_ "github.com/muidea/workorch/internal/modules/blocks/shell"
	_ "github.com/muidea/workorch/internal/modules/blocks/ssh"
	_ "github.com/muidea/workorch/internal/modules/kernel/decision"
	_ "github.com/muidea/workorch/internal/modules/kernel/eventstream"
	_ "github.com/muidea/workorch/internal/modules/kernel/mcp"
	_ "github.com/muidea/workorch/internal/modules/kernel/memory"
	_ "github.com/muidea/workorch/internal/modules/kernel/orchestrator"
	_ "github.com/muidea/workorch/internal/modules/kernel/run"
	_ "github.com/muidea/workorch/internal/modules/kernel/skill"
	_ "github.com/muidea/workorch/internal/modules/kernel/soul"
	_ "github.com/muidea/workorch/internal/modules/kernel/workspace"
)

func main() {
	configDir, cobraArgs := extractConfigDir(os.Args[1:])
	if err := configure(configDir); err != nil {
		log.Fatal(err)
	}
	// Override os.Args so cobra only sees its own flags
	os.Args = append([]string{os.Args[0]}, cobraArgs...)

	if err := application.Startup(service.DefaultService()); err != nil {
		log.Fatal(err)
	}
	defer application.Shutdown()

	if err := application.Run(); err != nil {
		application.Shutdown()
		log.Fatal(err)
	}

	if err := cli.Execute(); err != nil {
		log.Fatal(err)
	}
}

func extractConfigDir(args []string) (string, []string) {
	configDir := ""
	var remaining []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--config-dir" || args[i] == "-config-dir" {
			if i+1 < len(args) {
				configDir = args[i+1]
				i++
			}
			continue
		}
		remaining = append(remaining, args[i])
	}
	return configDir, remaining
}

func configure(configDir string) error {
	dir := resolveConfigDir(configDir)
	workorchconfig.ConfigureDirectory(dir)
	return ensureConfigFile(dir)
}

// resolveConfigDir determines the config directory in priority order:
// 1. --config-dir flag
// 2. WORKORCH_CONFIG_DIR env var
// 3. ~/.workorch/ (global user config)
func resolveConfigDir(flagDir string) string {
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

// ensureConfigFile creates a minimal default config if none exists.
func ensureConfigFile(dir string) error {
	if dir == "" {
		return nil
	}
	configPath := filepath.Join(dir, "application.toml")
	if _, err := os.Stat(configPath); err == nil {
		return nil // config exists
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}
	defaultCfg := map[string]any{
		"endpointName": "workorchd",
		"server": map[string]any{
			"addr":              ":8080",
			"readHeaderTimeout": "5s",
			"readTimeout":       "15s",
			"writeTimeout":      "180s",
			"idleTimeout":       "60s",
			"shutdownTimeout":   "5s",
		},
		"repository": map[string]any{
			"backend": "memory",
			"dataDir": ".workorch-data",
		},
	}
	data, err := toml.Marshal(defaultCfg)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

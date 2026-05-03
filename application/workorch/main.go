package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/muidea/magicCommon/framework/application"
	"github.com/muidea/magicCommon/framework/service"
	"github.com/pelletier/go-toml/v2"
	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
	"github.com/muidea/workorch/internal/modules/application/workorch/service/cli"
	appservice "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
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
	os.Args = append([]string{os.Args[0]}, cobraArgs...)

	// Check if remote mode was requested before initializing local runtime.
	remoteAddr := extractRemoteAddr(os.Args[1:])
	if remoteAddr != "" {
		be := backend.NewRemote(remoteAddr)
		if err := cli.ExecuteWithBackend(be, backend.WorkspaceInfo{}); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Local mode: initialize embedded runtime.
	if err := application.Startup(service.DefaultService()); err != nil {
		log.Fatal(err)
	}
	defer application.Shutdown()

	if err := application.Run(); err != nil {
		application.Shutdown()
		log.Fatal(err)
	}

	session, err := appservice.NewRuntimeSessionFromInitiator()
	if err != nil {
		log.Fatalf("create runtime session: %v", err)
	}
	defer session.Close(context.Background())

	exports, _ := session.Exports()
	be := backend.NewEventHub(session, exports.Infrastructure.EventHub)

	// Resolve default workspace from current working directory.
	cwd, _ := os.Getwd()
	defaultWS := resolveDefaultWorkspace(context.Background(), be, cwd)

	if err := cli.ExecuteWithBackend(be, defaultWS); err != nil {
		log.Fatal(err)
	}
}

func extractRemoteAddr(args []string) string {
	for i, a := range args {
		if a == "--remote" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
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

// resolveDefaultWorkspace finds or creates a Workspace bound to the given directory.
func resolveDefaultWorkspace(ctx context.Context, be backend.Backend, workingDir string) backend.WorkspaceInfo {
	if workingDir == "" {
		return backend.WorkspaceInfo{}
	}
	list, _ := be.Workspace().List(ctx)
	for _, ws := range list {
		if ws.WorkingDirectory == workingDir {
			return ws
		}
	}
	ws, err := be.Workspace().Create(ctx, backend.CreateWorkspaceRequest{
		Title:            filepath.Base(workingDir),
		Description:      workingDir,
		WorkingDirectory: workingDir,
	})
	if err != nil {
		return backend.WorkspaceInfo{}
	}
	return *ws
}

func ensureConfigFile(dir string) error {
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

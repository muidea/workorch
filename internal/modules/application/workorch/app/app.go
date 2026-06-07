package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
	cliconfig "github.com/muidea/workorch/internal/modules/application/workorch/biz/config"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/pelletier/go-toml/v2"
)

type Mode string

const (
	ModeInteractive Mode = "interactive"
	ModeServer      Mode = "server"
	ModeRemote      Mode = "remote"
)

type Options struct {
	ProgramName string
	Args        []string
	ForceServer bool
}

const defaultWorkspaceTemplateID = "generic-governed"
const defaultApplicationShutdownTimeout = 30 * time.Second

type StartupArgs struct {
	ConfigDir string
	CobraArgs []string
	Remote    string
	Mode      Mode
}

func Run(ctx context.Context, opts Options) error {
	if ctx == nil {
		ctx = context.Background()
	}
	parsed, err := ParseStartupArgs(opts.Args, opts.ForceServer)
	if err != nil {
		return err
	}
	svc, err := NewService(opts.ProgramName, parsed)
	if err != nil {
		return err
	}
	if err := svc.Startup(ctx); err != nil {
		return err
	}
	runErr := svc.Run(ctx)
	return errors.Join(runErr, svc.Shutdown(context.Background()))
}

func ParseStartupArgs(args []string, forceServer bool) (StartupArgs, error) {
	ret := StartupArgs{Mode: ModeInteractive}
	if forceServer {
		ret.Mode = ModeServer
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "serve", "server":
			ret.Mode = ModeServer
			continue
		case "--server", "-server", "--daemon", "-daemon":
			ret.Mode = ModeServer
			continue
		case "--config-dir", "-config-dir":
			if i+1 >= len(args) {
				return ret, fmt.Errorf("%s requires a value", arg)
			}
			ret.ConfigDir = args[i+1]
			i++
			continue
		case "--remote":
			if i+1 >= len(args) {
				return ret, fmt.Errorf("%s requires a value", arg)
			}
			ret.Remote = args[i+1]
			ret.Mode = ModeRemote
			ret.CobraArgs = append(ret.CobraArgs, arg, args[i+1])
			i++
			continue
		}
		ret.CobraArgs = append(ret.CobraArgs, arg)
	}
	return ret, nil
}

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

func ExtractCLIConfigPath(args []string) string {
	for i, a := range args {
		if a == "--config" && i+1 < len(args) {
			return args[i+1]
		}
	}
	path, _ := cliconfig.Path()
	return path
}

func ResolveDefaultWorkspace(ctx context.Context, be backend.Backend, workingDir string, cliConfigPath string) backend.WorkspaceInfo {
	if workingDir == "" {
		return backend.WorkspaceInfo{}
	}
	list, _ := be.Workspace().List(ctx)
	for _, ws := range list {
		if ws.DeliverableDirectory == workingDir {
			return ws
		}
	}
	if cfg, err := cliconfig.Load(cliConfigPath); err == nil && cfg.Defaults.LastActiveWorkspaceID != "" {
		for _, ws := range list {
			if string(ws.ID) == cfg.Defaults.LastActiveWorkspaceID {
				return ws
			}
		}
		if ws, err := be.Workspace().Get(ctx, common.WorkspaceID(cfg.Defaults.LastActiveWorkspaceID)); err == nil && ws != nil {
			return *ws
		}
	}
	ws, err := be.Workspace().Create(ctx, backend.CreateWorkspaceRequest{
		Title:                filepath.Base(workingDir),
		Description:          workingDir,
		DeliverableDirectory: workingDir,
		TemplateID:           defaultWorkspaceTemplateID,
	})
	if err != nil {
		return backend.WorkspaceInfo{}
	}
	return *ws
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

func withArgs(programName string, args []string, fn func() error) error {
	if programName == "" {
		programName = os.Args[0]
	}
	previous := os.Args
	os.Args = append([]string{programName}, args...)
	defer func() {
		os.Args = previous
	}()
	return fn()
}

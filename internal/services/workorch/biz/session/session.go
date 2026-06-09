package session

import (
	"context"

	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
	cliconfig "github.com/muidea/workorch/internal/services/workorch/biz/config"
)

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
	return backend.WorkspaceInfo{}
}

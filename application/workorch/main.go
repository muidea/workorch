package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/muidea/workorch/internal/modules/application/workorch/app"
	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
	"github.com/muidea/workorch/internal/pkg/common"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, app.Options{
		ProgramName: os.Args[0],
		Args:        os.Args[1:],
	}); err != nil {
		log.Fatal(err)
	}
}

func resolveDefaultWorkspace(ctx context.Context, be backend.Backend, workingDir string, cliConfigPath string) backend.WorkspaceInfo {
	return app.ResolveDefaultWorkspace(ctx, be, workingDir, cliConfigPath)
}

func backendWorkspaceID(id string) common.WorkspaceID {
	return common.WorkspaceID(id)
}

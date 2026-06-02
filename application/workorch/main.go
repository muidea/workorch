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

	_ "github.com/muidea/workorch/internal/initiators/workorchruntime"
	_ "github.com/muidea/workorch/internal/modules/application/workorch"
	_ "github.com/muidea/workorch/internal/modules/blocks/file"
	_ "github.com/muidea/workorch/internal/modules/blocks/git"
	_ "github.com/muidea/workorch/internal/modules/blocks/llm"
	_ "github.com/muidea/workorch/internal/modules/blocks/mcp"
	_ "github.com/muidea/workorch/internal/modules/blocks/shell"
	_ "github.com/muidea/workorch/internal/modules/blocks/ssh"
	_ "github.com/muidea/workorch/internal/modules/blocks/web"
	_ "github.com/muidea/workorch/internal/modules/kernel/approval"
	_ "github.com/muidea/workorch/internal/modules/kernel/decision"
	_ "github.com/muidea/workorch/internal/modules/kernel/eventstream"
	_ "github.com/muidea/workorch/internal/modules/kernel/governance"
	_ "github.com/muidea/workorch/internal/modules/kernel/mcp"
	_ "github.com/muidea/workorch/internal/modules/kernel/memory"
	_ "github.com/muidea/workorch/internal/modules/kernel/orchestrator"
	_ "github.com/muidea/workorch/internal/modules/kernel/run"
	_ "github.com/muidea/workorch/internal/modules/kernel/skill"
	_ "github.com/muidea/workorch/internal/modules/kernel/soul"
	_ "github.com/muidea/workorch/internal/modules/kernel/workspace"
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

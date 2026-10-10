package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/muidea/workorch/internal/services/workorch"

	_ "github.com/muidea/workorch/internal/initiators/workorchruntime"
	_ "github.com/muidea/workorch/internal/modules/application/workorchd"
	_ "github.com/muidea/workorch/internal/modules/blocks/approval"
	_ "github.com/muidea/workorch/internal/modules/blocks/decision"
	_ "github.com/muidea/workorch/internal/modules/blocks/eventstream"
	_ "github.com/muidea/workorch/internal/modules/blocks/file"
	_ "github.com/muidea/workorch/internal/modules/blocks/git"
	_ "github.com/muidea/workorch/internal/modules/blocks/llm"
	_ "github.com/muidea/workorch/internal/modules/blocks/mcp"
	_ "github.com/muidea/workorch/internal/modules/blocks/mcpregistry"
	_ "github.com/muidea/workorch/internal/modules/blocks/memory"
	_ "github.com/muidea/workorch/internal/modules/blocks/result"
	_ "github.com/muidea/workorch/internal/modules/blocks/run"
	_ "github.com/muidea/workorch/internal/modules/blocks/shell"
	_ "github.com/muidea/workorch/internal/modules/blocks/skill"
	_ "github.com/muidea/workorch/internal/modules/blocks/soul"
	_ "github.com/muidea/workorch/internal/modules/blocks/ssh"
	_ "github.com/muidea/workorch/internal/modules/blocks/web"
	_ "github.com/muidea/workorch/internal/modules/kernel/governance"
	_ "github.com/muidea/workorch/internal/modules/kernel/orchestrator"
	_ "github.com/muidea/workorch/internal/modules/kernel/system"
	_ "github.com/muidea/workorch/internal/modules/kernel/workspace"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := workorch.Run(ctx, workorch.Options{
		ProgramName: os.Args[0],
		Args:        os.Args[1:],
	}); err != nil {
		log.Fatal(err)
	}
}

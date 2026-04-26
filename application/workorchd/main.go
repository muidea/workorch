package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/muidea/magicCommon/framework/application"
	"github.com/muidea/magicCommon/framework/service"
	workorchdmodule "github.com/muidea/workorch/internal/modules/application/workorchd"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"

	_ "github.com/muidea/workorch/internal/initiators/workorchruntime"
	_ "github.com/muidea/workorch/internal/modules/blocks/approval"
	_ "github.com/muidea/workorch/internal/modules/blocks/legacy"
	_ "github.com/muidea/workorch/internal/modules/blocks/llm"
	_ "github.com/muidea/workorch/internal/modules/blocks/shell"
	_ "github.com/muidea/workorch/internal/modules/kernel/decision"
	_ "github.com/muidea/workorch/internal/modules/kernel/mcp"
	_ "github.com/muidea/workorch/internal/modules/kernel/memory"
	_ "github.com/muidea/workorch/internal/modules/kernel/orchestrator"
	_ "github.com/muidea/workorch/internal/modules/kernel/run"
	_ "github.com/muidea/workorch/internal/modules/kernel/skill"
	_ "github.com/muidea/workorch/internal/modules/kernel/soul"
	_ "github.com/muidea/workorch/internal/modules/kernel/workspace"
)

func main() {
	if err := configure(os.Args[1:]); err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := application.Startup(service.DefaultService()); err != nil {
		log.Fatal(err)
	}
	defer application.Shutdown()

	if err := application.Run(); err != nil {
		application.Shutdown()
		log.Fatal(err)
	}

	select {
	case err, ok := <-workorchdmodule.Done():
		if ok && err != nil {
			application.Shutdown()
			log.Fatal(err)
		}
	case <-ctx.Done():
	}
}

func configure(args []string) error {
	flags := flag.NewFlagSet("workorchd", flag.ContinueOnError)
	configDir := flags.String("config-dir", "", "path to workorchd configuration directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	workorchconfig.ConfigureDirectory(*configDir)
	return nil
}

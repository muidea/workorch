package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/muidea/magicCommon/framework/application"
	"github.com/muidea/magicCommon/framework/service"
	workorchdmodule "github.com/muidea/workorch/internal/modules/application/workorchd"

	_ "github.com/muidea/workorch/internal/initiators/workorchruntime"
	_ "github.com/muidea/workorch/internal/modules/blocks/approval"
	_ "github.com/muidea/workorch/internal/modules/blocks/legacy"
	_ "github.com/muidea/workorch/internal/modules/kernel/decision"
	_ "github.com/muidea/workorch/internal/modules/kernel/orchestrator"
	_ "github.com/muidea/workorch/internal/modules/kernel/run"
	_ "github.com/muidea/workorch/internal/modules/kernel/workspace"
)

func main() {
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

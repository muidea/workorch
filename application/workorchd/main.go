package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/muidea/magicCommon/framework/application"
	"github.com/muidea/workorch/application/workorchd/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	config, err := server.ConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	svc := server.NewService(config)
	if err := application.Startup(svc); err != nil {
		log.Fatal(err)
	}
	defer application.Shutdown()

	if err := application.Run(); err != nil {
		application.Shutdown()
		log.Fatal(err)
	}

	select {
	case err, ok := <-svc.Done():
		if ok && err != nil {
			application.Shutdown()
			log.Fatal(err)
		}
	case <-ctx.Done():
	}
}

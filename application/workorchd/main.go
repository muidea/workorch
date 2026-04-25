package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/muidea/workorch/application/workorchd/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	config, err := server.ConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}

	svc := server.NewHTTPServer(config)
	if err := svc.Validate(context.Background()); err != nil {
		log.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Start()
	}()

	select {
	case err := <-errCh:
		if err != nil {
			log.Fatal(err)
		}
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	defer cancel()
	if err := svc.Shutdown(shutdownCtx); err != nil {
		log.Fatal(err)
	}

	if err := <-errCh; err != nil {
		log.Fatal(err)
	}
}

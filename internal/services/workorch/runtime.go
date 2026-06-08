package workorch

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/muidea/magicCommon/framework/application"
	frameworkservice "github.com/muidea/magicCommon/framework/service"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchdeps"
)

const defaultApplicationShutdownTimeout = 30 * time.Second

type frameworkRuntime struct {
	configDir string
	started   bool
}

func (s *frameworkRuntime) Startup(ctx context.Context) error {
	ConfigureDirectory(s.configDir)
	if err := EnsureConfigFile(s.configDir); err != nil {
		return err
	}
	err := application.StartupWithOptions(ctx, frameworkservice.DefaultService(), application.Options{
		ConfigDir:   s.configDir,
		ServiceName: "workorch",
	})
	if err != nil {
		return err
	}
	s.started = true
	return nil
}

func (s *frameworkRuntime) Run(ctx context.Context) error {
	if !s.started {
		return common.ErrInvalidArgument
	}
	if err := application.Run(ctx); err != nil {
		return err
	}
	return nil
}

func (s *frameworkRuntime) Shutdown(ctx context.Context) error {
	if !s.started {
		return nil
	}
	timeout := applicationShutdownTimeout()
	if timeout <= 0 {
		application.Shutdown(ctx)
		s.started = false
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	application.Shutdown(shutdownCtx)
	s.started = false
	if err := shutdownCtx.Err(); err != nil {
		slog.Error("workorch application shutdown context finished", "timeout", timeout, "err", err)
	}
	return nil
}

func applicationShutdownTimeout() time.Duration {
	timeout := defaultApplicationShutdownTimeout
	cfg, err := workorchconfig.Current()
	if err != nil {
		return timeout
	}
	raw := strings.TrimSpace(cfg.Server.ShutdownTimeout)
	if raw == "" {
		return timeout
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return timeout
	}
	return parsed + 10*time.Second
}

func waitServer(ctx context.Context) error {
	done, err := queryServerDoneChannel(ctx)
	if err != nil {
		return err
	}
	select {
	case err, ok := <-done:
		if ok {
			return err
		}
		return nil
	case <-ctx.Done():
		return nil
	}
}

func queryServerDoneChannel(ctx context.Context) (<-chan error, error) {
	runtime, err := workorchdeps.ResolveRuntimeHelper("application/workorch server done")
	if err != nil {
		return nil, err
	}
	hub := runtime.EventHub()
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	publishErr := hub.Publish(ctx, events.Envelope{
		Topic:        events.TopicWorkorchDCommandDone,
		SourceModule: "workorch/app",
		SubjectType:  "workorchd",
		SubjectID:    "server",
	})
	response, ok := events.ResponseEnvelope(publishErr)
	if !ok {
		if publishErr != nil {
			return nil, publishErr
		}
		return nil, common.ErrInvalidArgument
	}
	done, ok := response.Payload[events.PayloadKeyDoneChannel].(<-chan error)
	if !ok || done == nil {
		return nil, common.ErrInvalidArgument
	}
	return done, nil
}

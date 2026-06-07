package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/muidea/magicCommon/framework/application"
	frameworkservice "github.com/muidea/magicCommon/framework/service"
	"github.com/muidea/workorch/internal/modules/application/workorch/biz/backend"
	"github.com/muidea/workorch/internal/modules/application/workorch/service/cli"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchdeps"
)

type Service = frameworkservice.LifecycleService

func NewService(programName string, args StartupArgs) (Service, error) {
	switch args.Mode {
	case ModeRemote:
		return &remoteService{programName: programName, args: args}, nil
	case ModeServer:
		return &serverService{
			framework: frameworkRuntime{
				configDir: ResolveConfigDir(args.ConfigDir),
			},
		}, nil
	case ModeInteractive:
		return &interactiveService{
			programName: programName,
			args:        args,
			framework: frameworkRuntime{
				configDir: ResolveConfigDir(args.ConfigDir),
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported workorch mode: %s", args.Mode)
	}
}

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

type remoteService struct {
	programName string
	args        StartupArgs
	be          backend.Backend
}

func (s *remoteService) Startup(context.Context) error {
	ConfigureDirectory(s.args.ConfigDir)
	s.be = backend.NewRemote(s.args.Remote)
	return nil
}

func (s *remoteService) Run(context.Context) error {
	return withArgs(s.programName, s.args.CobraArgs, func() error {
		return cli.ExecuteWithBackend(s.be, backend.WorkspaceInfo{})
	})
}

func (s *remoteService) Shutdown(context.Context) error {
	return nil
}

type serverService struct {
	framework frameworkRuntime
}

func (s *serverService) Startup(ctx context.Context) error {
	return s.framework.Startup(ctx)
}

func (s *serverService) Run(ctx context.Context) error {
	if err := s.framework.Run(ctx); err != nil {
		return err
	}
	return waitServer(ctx)
}

func (s *serverService) Shutdown(ctx context.Context) error {
	return s.framework.Shutdown(ctx)
}

type interactiveService struct {
	programName    string
	args           StartupArgs
	framework      frameworkRuntime
	be             backend.Backend
	closeBackend   func(context.Context) error
	restoreLogging func()
}

func (s *interactiveService) Startup(ctx context.Context) error {
	s.restoreLogging = installInteractiveLogging(s.framework.configDir)
	if err := s.framework.Startup(ctx); err != nil {
		s.closeLogging()
		return err
	}
	return nil
}

func (s *interactiveService) Run(ctx context.Context) error {
	if err := s.framework.Run(ctx); err != nil {
		return err
	}
	be, closeBackend, err := backend.NewEmbeddedEventHub()
	if err != nil {
		return err
	}
	s.be = be
	s.closeBackend = closeBackend
	workingDir, _ := os.Getwd()
	defaultWS := ResolveDefaultWorkspace(context.Background(), be, workingDir, ExtractCLIConfigPath(s.args.CobraArgs))
	return withArgs(s.programName, s.args.CobraArgs, func() error {
		return cli.ExecuteWithBackend(be, defaultWS)
	})
}

func (s *interactiveService) Shutdown(ctx context.Context) error {
	var err error
	if s.closeBackend != nil {
		err = errors.Join(err, s.closeBackend(ctx))
		s.closeBackend = nil
		s.be = nil
	}
	err = errors.Join(err, s.framework.Shutdown(ctx))
	s.closeLogging()
	return err
}

func (s *interactiveService) closeLogging() {
	if s.restoreLogging == nil {
		return
	}
	s.restoreLogging()
	s.restoreLogging = nil
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

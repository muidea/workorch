package workorch

import (
	"context"
	"errors"
	"fmt"
	"os"

	frameworkservice "github.com/muidea/magicCommon/framework/service"
	"github.com/muidea/workorch/internal/services/workorch/biz/backends"
	"github.com/muidea/workorch/internal/services/workorch/biz/session"
	"github.com/muidea/workorch/internal/services/workorch/service/cli"
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
		defaultWS := resolveDefaultWorkspace(context.Background(), s.be, s.args.CobraArgs)
		return cli.ExecuteWithBackend(s.be, defaultWS)
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
	defaultWS := resolveDefaultWorkspace(context.Background(), be, s.args.CobraArgs)
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

func resolveDefaultWorkspace(ctx context.Context, be backend.Backend, args []string) backend.WorkspaceInfo {
	workingDir, _ := os.Getwd()
	return session.ResolveDefaultWorkspace(ctx, be, workingDir, session.ExtractCLIConfigPath(args))
}

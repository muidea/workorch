package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

var (
	newBootstrapWithBackend              = bootstrap.NewWithBackend
	newBootstrapWithBackendAndCommandHub = bootstrap.NewWithBackendAndCommandHub
)

func NewRuntimeSession(bootstrapRuntime *bootstrap.Runtime) *runtimecore.Session {
	return runtimecore.NewSession(bootstrapRuntime, runtimecore.NewManagedRuntime)
}

func NewRuntimeSessionFromExports(exports bootstrap.Exports) (*runtimecore.Session, error) {
	return runtimecore.NewSessionFromExports(exports, runtimecore.NewManagedRuntime)
}

func NewRuntimeSessionFromCommandHub(commandHub magicevent.Hub, backgroundRoutine magictask.BackgroundRoutine) (*runtimecore.Session, error) {
	session, err := NewRuntimeSessionFromExports(bootstrap.Exports{
		Infrastructure: bootstrap.InfrastructureExports{
			CommandHub:        commandHub,
			BackgroundRoutine: backgroundRoutine,
			LoadConfig:        workorchconfig.Current,
		},
	})
	if err != nil {
		return nil, err
	}
	if cfg, cfgErr := workorchconfig.Current(); cfgErr == nil {
		session.SetRepositoryConfig(cfg.Repository.Backend, cfg.Repository.DataDir)
	}
	return session, nil
}

func NewRuntimeSessionWithBackend(backend string, dataRoot string) (*runtimecore.Session, error) {
	return runtimecore.NewSessionWithBackend(backend, dataRoot, runtimecore.NewManagedRuntime, func() *bootstrap.Runtime {
		return newBootstrapWithBackend(backend, dataRoot)
	})
}

func NewRuntimeSessionWithBackendAndCommandHub(backend string, dataRoot string, commandHub magicevent.Hub) (*runtimecore.Session, error) {
	return runtimecore.NewSessionWithBackendAndCommandHub(
		backend,
		dataRoot,
		commandHub,
		runtimecore.NewManagedRuntime,
		func(commandHub magicevent.Hub) *bootstrap.Runtime {
			return newBootstrapWithBackendAndCommandHub(backend, dataRoot, commandHub)
		},
	)
}

func NewRuntimeSessionFromEnv() (*runtimecore.Session, error) {
	return runtimecore.NewSessionFromEnv(
		workorchconfig.Current,
		runtimecore.NewManagedRuntime,
		func(cfg workorchconfig.Config) *bootstrap.Runtime {
			return newBootstrapWithBackend(cfg.Repository.Backend, cfg.Repository.DataDir)
		},
	)
}

func ContextWithCommandHub(ctx context.Context, commandHub magicevent.Hub) context.Context {
	return runtimecore.ContextWithCommandHub(ctx, commandHub)
}

func CommandHubFromContext(ctx context.Context) magicevent.Hub {
	return runtimecore.CommandHubFromContext(ctx)
}

func newRuntimeSessionWithBackend(ctx context.Context, backend string, dataRoot string) (*runtimecore.Session, error) {
	return NewRuntimeSessionWithBackendAndCommandHub(backend, dataRoot, CommandHubFromContext(ctx))
}

func buildBootstrapRuntime(factory func() *bootstrap.Runtime) (runtime *bootstrap.Runtime, err error) {
	return runtimecore.BuildBootstrapRuntime(factory)
}

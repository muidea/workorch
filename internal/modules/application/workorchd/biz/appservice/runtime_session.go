package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimeports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"
)

type RuntimeSession = runtimecore.Session

var (
	newBootstrapWithBackend                           = bootstrap.NewWithBackend
	newBootstrapWithBackendRuntimePolicyAndCommandHub = bootstrap.NewWithBackendRuntimePolicyAndCommandHub
	newBootstrapWithBackendAndRuntimePolicy           = bootstrap.NewWithBackendAndRuntimePolicy
)

func NewRuntimeSession(bootstrapRuntime *bootstrap.Runtime) *RuntimeSession {
	return runtimecore.NewSession(bootstrapRuntime, runtimeports.NewManagedRuntimeFromCommandHub)
}

func NewRuntimeSessionFromExports(exports bootstrap.Exports) (*RuntimeSession, error) {
	return runtimecore.NewSessionFromExports(exports, runtimeports.NewManagedRuntimeFromCommandHub)
}

func NewRuntimeSessionFromCommandHub(commandHub magicevent.Hub, backgroundRoutine magictask.BackgroundRoutine) (*RuntimeSession, error) {
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

func NewRuntimeSessionWithBackend(backend string, dataRoot string) (*RuntimeSession, error) {
	return runtimecore.NewSessionWithBackend(backend, dataRoot, runtimeports.NewManagedRuntimeFromCommandHub, func() *bootstrap.Runtime {
		return newBootstrapWithBackend(backend, dataRoot)
	})
}

func NewRuntimeSessionWithBackendAndCommandHub(backend string, dataRoot string, commandHub magicevent.Hub) (*RuntimeSession, error) {
	return runtimecore.NewSessionWithBackendAndCommandHub(
		backend,
		dataRoot,
		commandHub,
		workorchconfig.Current,
		runtimeports.NewManagedRuntimeFromCommandHub,
		func(runtimePolicy workorchruntimepolicy.RuntimePolicy, commandHub magicevent.Hub) *bootstrap.Runtime {
			return newBootstrapWithBackendRuntimePolicyAndCommandHub(backend, dataRoot, runtimePolicy, commandHub)
		},
	)
}

func NewRuntimeSessionFromEnv() (*RuntimeSession, error) {
	return runtimecore.NewSessionFromEnv(
		workorchconfig.Current,
		runtimeports.NewManagedRuntimeFromCommandHub,
		func(cfg workorchconfig.Config) *bootstrap.Runtime {
			return newBootstrapWithBackendAndRuntimePolicy(
				cfg.Repository.Backend,
				cfg.Repository.DataDir,
				workorchruntimepolicy.FromConfig(cfg),
			)
		},
	)
}

func ContextWithCommandHub(ctx context.Context, commandHub magicevent.Hub) context.Context {
	return runtimecore.ContextWithCommandHub(ctx, commandHub)
}

func CommandHubFromContext(ctx context.Context) magicevent.Hub {
	return runtimecore.CommandHubFromContext(ctx)
}

func newRuntimeSessionWithBackend(ctx context.Context, backend string, dataRoot string) (*RuntimeSession, error) {
	return NewRuntimeSessionWithBackendAndCommandHub(backend, dataRoot, CommandHubFromContext(ctx))
}

func buildBootstrapRuntime(factory func() *bootstrap.Runtime) (runtime *bootstrap.Runtime, err error) {
	return runtimecore.BuildBootstrapRuntime(factory)
}

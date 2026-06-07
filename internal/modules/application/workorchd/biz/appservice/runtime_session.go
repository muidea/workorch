package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimeports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"
)

type RuntimeSession = runtimecore.Session

func NewRuntimeSession(bootstrapRuntime *bootstrap.Runtime) *RuntimeSession {
	return runtimecore.NewSession(bootstrapRuntime, runtimeports.NewManagedRuntimeFromEventHub)
}

func NewRuntimeSessionFromExports(exports bootstrap.Exports) (*RuntimeSession, error) {
	return runtimecore.NewSessionFromExports(exports, runtimeports.NewManagedRuntimeFromEventHub)
}

func NewRuntimeSessionFromInitiator() (*RuntimeSession, error) {
	return runtimecore.NewSessionFromInitiator(
		workorchconfig.Current,
		exportsFromInitiator,
		runtimeports.NewManagedRuntimeFromEventHub,
	)
}

func NewRuntimeSessionWithBackend(backend string, dataRoot string) (*RuntimeSession, error) {
	return runtimecore.NewSessionWithBackend(backend, dataRoot, runtimeports.NewManagedRuntimeFromEventHub, func() *bootstrap.Runtime {
		return bootstrap.NewWithBackend(backend, dataRoot)
	})
}

func NewRuntimeSessionWithBackendAndEventHub(backend string, dataRoot string, eventHub events.Hub) (*RuntimeSession, error) {
	return runtimecore.NewSessionWithBackendAndEventHub(
		backend,
		dataRoot,
		eventHub,
		workorchconfig.Current,
		runtimeports.NewManagedRuntimeFromEventHub,
		func(runtimePolicy workorchruntimepolicy.RuntimePolicy, eventHub events.Hub) *bootstrap.Runtime {
			return bootstrap.NewWithBackendRuntimePolicyAndEventHub(backend, dataRoot, runtimePolicy, eventHub)
		},
	)
}

func NewRuntimeSessionFromEnv() (*RuntimeSession, error) {
	return runtimecore.NewSessionFromEnv(
		workorchconfig.Current,
		runtimeports.NewManagedRuntimeFromEventHub,
		func(cfg workorchconfig.Config) *bootstrap.Runtime {
			return bootstrap.NewWithBackendAndRuntimePolicy(
				cfg.Repository.Backend,
				cfg.Repository.DataDir,
				workorchruntimepolicy.FromConfig(cfg),
			)
		},
	)
}

func ContextWithEventHub(ctx context.Context, eventHub events.Hub) context.Context {
	return runtimecore.ContextWithEventHub(ctx, eventHub)
}

func EventHubFromContext(ctx context.Context) events.Hub {
	return runtimecore.EventHubFromContext(ctx)
}

func newRuntimeSessionWithBackend(ctx context.Context, backend string, dataRoot string) (*RuntimeSession, error) {
	return NewRuntimeSessionWithBackendAndEventHub(backend, dataRoot, EventHubFromContext(ctx))
}

func exportsFromInitiator() (bootstrap.Exports, error) {
	return bootstrap.ExportsFromInitiator("application/workorchd runtime session")
}

func buildBootstrapRuntime(factory func() *bootstrap.Runtime) (runtime *bootstrap.Runtime, err error) {
	return runtimecore.BuildBootstrapRuntime(factory)
}

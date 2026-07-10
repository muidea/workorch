package appservice

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimecore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/runtimeports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"
)

type RuntimeSession = runtimecore.Session

var (
	newBootstrapWithBackend                          = bootstrap.NewWithBackend
	newBootstrapWithBackendRuntimePolicyAndEventHubs = bootstrap.NewWithBackendRuntimePolicyAndEventHubs
	newBootstrapWithBackendAndRuntimePolicy          = bootstrap.NewWithBackendAndRuntimePolicy
)

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
		return newBootstrapWithBackend(backend, dataRoot)
	})
}

func NewRuntimeSessionWithBackendAndEventHubs(backend string, dataRoot string, eventHub events.Hub, commandHub magicevent.Hub) (*RuntimeSession, error) {
	return runtimecore.NewSessionWithBackendAndEventHubs(
		backend,
		dataRoot,
		eventHub,
		commandHub,
		workorchconfig.Current,
		runtimeports.NewManagedRuntimeFromEventHub,
		func(runtimePolicy workorchruntimepolicy.RuntimePolicy, eventHub events.Hub, commandHub magicevent.Hub) *bootstrap.Runtime {
			return newBootstrapWithBackendRuntimePolicyAndEventHubs(backend, dataRoot, runtimePolicy, eventHub, commandHub)
		},
	)
}

func NewRuntimeSessionFromEnv() (*RuntimeSession, error) {
	return runtimecore.NewSessionFromEnv(
		workorchconfig.Current,
		runtimeports.NewManagedRuntimeFromEventHub,
		func(cfg workorchconfig.Config) *bootstrap.Runtime {
			return newBootstrapWithBackendAndRuntimePolicy(
				cfg.Repository.Backend,
				cfg.Repository.DataDir,
				workorchruntimepolicy.FromConfig(cfg),
			)
		},
	)
}

func EventHubFromContext(ctx context.Context) events.Hub {
	return runtimecore.EventHubFromContext(ctx)
}

func ContextWithEventHubs(ctx context.Context, eventHub events.Hub, commandHub magicevent.Hub) context.Context {
	return runtimecore.ContextWithEventHubs(ctx, eventHub, commandHub)
}

func CommandHubFromContext(ctx context.Context) magicevent.Hub {
	return runtimecore.CommandHubFromContext(ctx)
}

func newRuntimeSessionWithBackend(ctx context.Context, backend string, dataRoot string) (*RuntimeSession, error) {
	return NewRuntimeSessionWithBackendAndEventHubs(backend, dataRoot, EventHubFromContext(ctx), CommandHubFromContext(ctx))
}

func exportsFromInitiator() (bootstrap.Exports, error) {
	return bootstrap.ExportsFromInitiator("application/workorchd runtime session")
}

func buildBootstrapRuntime(factory func() *bootstrap.Runtime) (runtime *bootstrap.Runtime, err error) {
	return runtimecore.BuildBootstrapRuntime(factory)
}

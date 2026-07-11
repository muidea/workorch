package bootstrap

import (
	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
	"github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"
)

func NewFromEnv() *Runtime {
	cfg, err := workorchconfig.Current()
	if err != nil {
		panic("load workorch config: " + err.Error())
	}
	return newWithBackendRuntimePolicyAndCommandHub(
		cfg.Repository.Backend,
		cfg.Repository.DataDir,
		workorchruntimepolicy.FromConfig(cfg),
		nil,
	)
}

func NewWithBackend(backend string, dataRoot string) *Runtime {
	return newWithBackendRuntimePolicyAndCommandHub(backend, dataRoot, workorchruntimepolicy.RuntimePolicy{}, nil)
}

func NewWithBackendAndRuntimePolicy(backend string, dataRoot string, runtimePolicy workorchruntimepolicy.RuntimePolicy) *Runtime {
	return newWithBackendRuntimePolicyAndCommandHub(backend, dataRoot, runtimePolicy, nil)
}

func NewWithBackendAndCommandHub(backend string, dataRoot string, commandHub magicevent.Hub) *Runtime {
	return newWithBackendRuntimePolicyAndCommandHub(backend, dataRoot, workorchruntimepolicy.RuntimePolicy{}, commandHub)
}

func NewWithBackendRuntimePolicyAndCommandHub(backend string, dataRoot string, runtimePolicy workorchruntimepolicy.RuntimePolicy, commandHub magicevent.Hub) *Runtime {
	return newWithBackendRuntimePolicyAndCommandHub(backend, dataRoot, runtimePolicy, commandHub)
}

// newWithBackendRuntimePolicyAndCommandHub builds application-owned runtime
// infrastructure only. Owner repositories and module fixtures belong to the
// framework runtime or the application testkit, never to production exports.
func newWithBackendRuntimePolicyAndCommandHub(backend string, dataRoot string, runtimePolicy workorchruntimepolicy.RuntimePolicy, commandHub magicevent.Hub) *Runtime {
	_ = runtimePolicy
	_ = dataRoot
	backend = workorchrepo.NormalizeRepositoryBackend(backend)
	if backend != "memory" && backend != "file" {
		panic("unsupported repository backend: " + backend)
	}

	if commandHub == nil {
		commandHub = magicevent.NewHub(1024)
	}
	backgroundRoutine := magictask.NewBackgroundRoutine(1024)

	return &Runtime{
		exports: Exports{
			Infrastructure: InfrastructureExports{
				CommandHub:        commandHub,
				BackgroundRoutine: backgroundRoutine,
				LoadConfig:        workorchconfig.Current,
			},
		},
		background: backgroundRoutine,
	}
}

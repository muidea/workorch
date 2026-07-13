package bootstrap

import (
	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

func NewFromEnv() *Runtime {
	cfg, err := workorchconfig.Current()
	if err != nil {
		panic("load workorch config: " + err.Error())
	}
	return newWithBackendAndCommandHub(cfg.Repository.Backend, cfg.Repository.DataDir, nil)
}

func NewWithBackend(backend string, dataRoot string) *Runtime {
	return newWithBackendAndCommandHub(backend, dataRoot, nil)
}

func NewWithBackendAndCommandHub(backend string, dataRoot string, commandHub magicevent.Hub) *Runtime {
	return newWithBackendAndCommandHub(backend, dataRoot, commandHub)
}

// newWithBackendAndCommandHub builds application-owned runtime
// infrastructure only. Owner repositories and module fixtures belong to the
// framework runtime or the application testkit, never to production exports.
func newWithBackendAndCommandHub(backend string, dataRoot string, commandHub magicevent.Hub) *Runtime {
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

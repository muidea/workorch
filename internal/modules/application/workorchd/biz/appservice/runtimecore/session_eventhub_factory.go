package runtimecore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"
)

func NewSessionWithBackendAndEventHub(
	backend string,
	dataRoot string,
	eventHub events.Hub,
	loadConfig func() (workorchconfig.Config, error),
	runtimeFactory ManagedRuntimeFactory,
	bootstrapFactory func(workorchruntimepolicy.RuntimePolicy, events.Hub) *bootstrap.Runtime,
) (*Session, error) {
	runtimePolicy := workorchruntimepolicy.FromConfig(workorchconfig.Config{})
	if loadConfig != nil {
		if cfg, err := loadConfig(); err == nil {
			runtimePolicy = workorchruntimepolicy.FromConfig(cfg)
		}
	}
	bootstrapRuntime, err := BuildBootstrapRuntime(func() *bootstrap.Runtime {
		return bootstrapFactory(runtimePolicy, eventHub)
	})
	if err != nil {
		return nil, err
	}
	session := NewSession(bootstrapRuntime, runtimeFactory)
	session.SetRepositoryConfig(backend, dataRoot)
	return session, nil
}

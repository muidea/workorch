package runtimecore

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"github.com/muidea/workorch/internal/pkg/workorchruntimepolicy"
)

func NewSessionWithBackendAndEventHubs(
	backend string,
	dataRoot string,
	eventHub events.Hub,
	commandHub magicevent.Hub,
	loadConfig func() (workorchconfig.Config, error),
	runtimeFactory ManagedRuntimeFactory,
	bootstrapFactory func(workorchruntimepolicy.RuntimePolicy, events.Hub, magicevent.Hub) *bootstrap.Runtime,
) (*Session, error) {
	runtimePolicy := workorchruntimepolicy.FromConfig(workorchconfig.Config{})
	if loadConfig != nil {
		if cfg, err := loadConfig(); err == nil {
			runtimePolicy = workorchruntimepolicy.FromConfig(cfg)
		}
	}
	bootstrapRuntime, err := BuildBootstrapRuntime(func() *bootstrap.Runtime {
		return bootstrapFactory(runtimePolicy, eventHub, commandHub)
	})
	if err != nil {
		return nil, err
	}
	session := NewSession(bootstrapRuntime, runtimeFactory)
	session.SetRepositoryConfig(backend, dataRoot)
	return session, nil
}

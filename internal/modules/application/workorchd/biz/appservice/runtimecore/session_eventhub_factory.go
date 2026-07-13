package runtimecore

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
)

func NewSessionWithBackendAndCommandHub(
	backend string,
	dataRoot string,
	commandHub magicevent.Hub,
	runtimeFactory ManagedRuntimeFactory,
	bootstrapFactory func(magicevent.Hub) *bootstrap.Runtime,
) (*Session, error) {
	bootstrapRuntime, err := BuildBootstrapRuntime(func() *bootstrap.Runtime {
		return bootstrapFactory(commandHub)
	})
	if err != nil {
		return nil, err
	}
	session := NewSession(bootstrapRuntime, runtimeFactory)
	session.SetRepositoryConfig(backend, dataRoot)
	return session, nil
}

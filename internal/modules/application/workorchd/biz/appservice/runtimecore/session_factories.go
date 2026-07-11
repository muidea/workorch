package runtimecore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
)

func NewSessionWithBackend(
	backend string,
	dataRoot string,
	runtimeFactory ManagedRuntimeFactory,
	bootstrapFactory func() *bootstrap.Runtime,
) (*Session, error) {
	bootstrapRuntime, err := BuildBootstrapRuntime(bootstrapFactory)
	if err != nil {
		return nil, err
	}
	session := NewSession(bootstrapRuntime, runtimeFactory)
	session.SetRepositoryConfig(backend, dataRoot)
	return session, nil
}

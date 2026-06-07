package runtimecore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func NewSessionFromInitiator(
	loadConfig func() (workorchconfig.Config, error),
	initiatorExports func() (bootstrap.Exports, error),
	runtimeFactory ManagedRuntimeFactory,
) (*Session, error) {
	exports, err := initiatorExports()
	if err != nil {
		return nil, err
	}
	session, sessionErr := NewSessionFromExports(exports, runtimeFactory)
	if sessionErr != nil {
		return nil, sessionErr
	}
	if loadConfig != nil {
		if cfg, err := loadConfig(); err == nil {
			session.SetRepositoryConfig(cfg.Repository.Backend, cfg.Repository.DataDir)
		}
	}
	return session, nil
}

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

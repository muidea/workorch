package runtimecore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func NewSessionFromEnv(
	loadConfig func() (workorchconfig.Config, error),
	runtimeFactory ManagedRuntimeFactory,
	bootstrapFactory func(workorchconfig.Config) *bootstrap.Runtime,
) (*Session, error) {
	if loadConfig == nil {
		return nil, common.ErrInvalidArgument
	}
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}
	bootstrapRuntime, err := BuildBootstrapRuntime(func() *bootstrap.Runtime {
		return bootstrapFactory(cfg)
	})
	if err != nil {
		return nil, err
	}
	session := NewSession(bootstrapRuntime, runtimeFactory)
	session.SetRepositoryConfig(cfg.Repository.Backend, cfg.Repository.DataDir)
	return session, nil
}

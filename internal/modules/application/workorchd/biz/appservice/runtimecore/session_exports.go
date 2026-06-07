package runtimecore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

func (s *Session) Exports() (bootstrap.Exports, error) {
	if s == nil {
		return bootstrap.Exports{}, common.ErrInvalidArgument
	}
	if s.Bootstrap == nil {
		if !HasRuntimeExports(s.exports) {
			return bootstrap.Exports{}, common.ErrInvalidArgument
		}
		return s.exports, nil
	}
	return s.Bootstrap.Exports(), nil
}

func (s *Session) RequiredEventHub() (events.Hub, error) {
	exports, err := s.Exports()
	if err != nil {
		return nil, err
	}
	if exports.Infrastructure.EventHub == nil {
		return nil, common.ErrInvalidArgument
	}
	return exports.Infrastructure.EventHub, nil
}

func (s *Session) ConfigLoader() func() (workorchconfig.Config, error) {
	exports, err := s.Exports()
	if err != nil {
		return nil
	}
	if exports.Infrastructure.LoadConfig != nil {
		return exports.Infrastructure.LoadConfig
	}
	return func() (workorchconfig.Config, error) {
		return workorchconfig.Config{}, nil
	}
}

func (s *Session) SetRepositoryConfig(backend string, dataRoot string) {
	if s == nil {
		return
	}
	cfg := ResolveRepositoryConfig(backend, dataRoot)
	s.repositoryBackend = cfg.Backend
	s.repositoryDataDir = cfg.DataDir
}

func (s *Session) RepositoryBackend() string {
	if s == nil {
		return ""
	}
	return s.repositoryBackend
}

func (s *Session) RepositoryDataDir() string {
	if s == nil {
		return ""
	}
	return s.repositoryDataDir
}

func HasRuntimeExports(exports bootstrap.Exports) bool {
	return exports.Infrastructure.EventHub != nil
}

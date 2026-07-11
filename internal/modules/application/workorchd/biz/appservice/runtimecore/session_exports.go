package runtimecore

import (
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/common"
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

func (s *Session) RequiredCommandHub() (magicevent.Hub, error) {
	exports, err := s.Exports()
	if err != nil {
		return nil, err
	}
	if exports.Infrastructure.CommandHub == nil {
		return nil, common.ErrInvalidArgument
	}
	return exports.Infrastructure.CommandHub, nil
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
	return ValidateRuntimeExports(exports) == nil
}

func ValidateRuntimeExports(exports bootstrap.Exports) error {
	required := []struct {
		name  string
		value any
	}{
		{"Infrastructure.CommandHub", exports.Infrastructure.CommandHub},
	}
	for _, item := range required {
		if item.value == nil {
			return fmt.Errorf("%w: missing runtime export %s", common.ErrInvalidArgument, item.name)
		}
	}
	return nil
}

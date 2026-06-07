package runtimecore

import (
	"context"
	"errors"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
)

func (s *Session) Borrow() *Session {
	if s == nil {
		return &Session{borrowed: true}
	}
	if s.Bootstrap == nil {
		return &Session{
			Runtime:           newManagedRuntimeFromExports(s.exports, s.runtimeFactory),
			exports:           s.exports,
			borrowed:          true,
			repositoryBackend: s.repositoryBackend,
			repositoryDataDir: s.repositoryDataDir,
			runtimeFactory:    s.runtimeFactory,
		}
	}
	return &Session{
		Bootstrap:         s.Bootstrap,
		Runtime:           newManagedRuntimeFromBootstrap(s.Bootstrap, s.runtimeFactory),
		borrowed:          true,
		repositoryBackend: s.repositoryBackend,
		repositoryDataDir: s.repositoryDataDir,
		runtimeFactory:    s.runtimeFactory,
	}
}

func (s *Session) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if s.Runtime != nil {
		s.Runtime.Close()
		s.Runtime = nil
	}
	if s.Bootstrap == nil {
		if s.borrowed {
			s.exports = bootstrap.Exports{}
		}
		return nil
	}
	if s.borrowed {
		s.Bootstrap = nil
		return nil
	}
	err := s.Bootstrap.Shutdown(ctx)
	s.Bootstrap = nil
	return err
}

func (s *Session) CloseOnReturn(ctx context.Context, errp *error) {
	if errp == nil {
		return
	}
	*errp = errors.Join(*errp, s.Close(ctx))
}

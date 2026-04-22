package appservice

import (
	"context"

	"github.com/muidea/workorch/application/workorchd/bootstrap"
)

type RuntimeSession struct {
	Bootstrap *bootstrap.Runtime
	Runtime   *ManagedRuntime
}

func NewRuntimeSession(bootstrapRuntime *bootstrap.Runtime) *RuntimeSession {
	if bootstrapRuntime == nil {
		return &RuntimeSession{}
	}
	return &RuntimeSession{
		Bootstrap: bootstrapRuntime,
		Runtime:   NewRuntimeFromBootstrap(bootstrapRuntime),
	}
}

func NewRuntimeSessionWithBackend(backend string, dataRoot string) *RuntimeSession {
	return NewRuntimeSession(bootstrap.NewWithBackend(backend, dataRoot))
}

func NewRuntimeSessionFromEnv() *RuntimeSession {
	return NewRuntimeSession(bootstrap.NewFromEnv())
}

func (s *RuntimeSession) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if s.Runtime != nil {
		s.Runtime.Close()
		s.Runtime = nil
	}
	if s.Bootstrap == nil {
		return nil
	}
	err := s.Bootstrap.Shutdown(ctx)
	s.Bootstrap = nil
	return err
}

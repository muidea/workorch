package appservice

import (
	"context"
	"fmt"
	"strings"

	"github.com/muidea/workorch/application/workorchd/bootstrap"
	"github.com/muidea/workorch/internal/pkg/common"
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

func NewRuntimeSessionWithBackend(backend string, dataRoot string) (*RuntimeSession, error) {
	bootstrapRuntime, err := buildBootstrapRuntime(func() *bootstrap.Runtime {
		return bootstrap.NewWithBackend(backend, dataRoot)
	})
	if err != nil {
		return nil, err
	}
	return NewRuntimeSession(bootstrapRuntime), nil
}

func NewRuntimeSessionFromEnv() (*RuntimeSession, error) {
	bootstrapRuntime, err := buildBootstrapRuntime(bootstrap.NewFromEnv)
	if err != nil {
		return nil, err
	}
	return NewRuntimeSession(bootstrapRuntime), nil
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

func buildBootstrapRuntime(factory func() *bootstrap.Runtime) (runtime *bootstrap.Runtime, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			message := fmt.Sprint(recovered)
			if strings.HasPrefix(message, "unsupported repository backend: ") {
				err = fmt.Errorf("%w: %s", common.ErrInvalidArgument, message)
				runtime = nil
				return
			}
			panic(recovered)
		}
	}()
	return factory(), nil
}

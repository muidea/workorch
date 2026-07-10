package runtimecore

import (
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/bootstrap"
	"github.com/muidea/workorch/internal/pkg/events"
)

type ManagedRuntimeFactory func(events.Hub, magicevent.Hub) *ManagedRuntime

type Session struct {
	Bootstrap         *bootstrap.Runtime
	Runtime           *ManagedRuntime
	exports           bootstrap.Exports
	borrowed          bool
	repositoryBackend string
	repositoryDataDir string
	runtimeFactory    ManagedRuntimeFactory
}

func NewSession(bootstrapRuntime *bootstrap.Runtime, runtimeFactory ManagedRuntimeFactory) *Session {
	if bootstrapRuntime == nil {
		return &Session{runtimeFactory: runtimeFactory}
	}
	return &Session{
		Bootstrap:      bootstrapRuntime,
		Runtime:        newManagedRuntimeFromBootstrap(bootstrapRuntime, runtimeFactory),
		runtimeFactory: runtimeFactory,
	}
}

func NewSessionFromExports(exports bootstrap.Exports, runtimeFactory ManagedRuntimeFactory) (*Session, error) {
	if err := ValidateRuntimeExports(exports); err != nil {
		return nil, err
	}
	return &Session{
		Runtime:        newManagedRuntimeFromExports(exports, runtimeFactory),
		exports:        exports,
		borrowed:       true,
		runtimeFactory: runtimeFactory,
	}, nil
}

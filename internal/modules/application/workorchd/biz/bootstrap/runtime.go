package bootstrap

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

type Runtime struct {
	exports    Exports
	background magictask.BackgroundRoutine
}

type InfrastructureExports struct {
	CommandHub        magicevent.Hub
	BackgroundRoutine magictask.BackgroundRoutine
	LoadConfig        func() (workorchconfig.Config, error)
}

type Exports struct {
	Infrastructure InfrastructureExports
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	if r.exports.Infrastructure.CommandHub != nil {
		r.exports.Infrastructure.CommandHub.Terminate(ctx)
	}
	if r.background != nil {
		r.background.Shutdown(ctx)
		r.background = nil
	}
	return nil
}

func (r *Runtime) Exports() Exports {
	if r == nil {
		return Exports{}
	}
	return r.exports
}

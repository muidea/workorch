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

// DrainBackground stops the shared BackgroundRoutine. Call only after module
// workers that run on this routine have been cancelled (module Stop), otherwise
// Shutdown waits forever for governance/orchestrator loops that still need cancel.
func (r *Runtime) DrainBackground(ctx context.Context) {
	if r == nil || r.background == nil {
		return
	}
	r.background.Shutdown(ctx)
	r.background = nil
	if r.exports.Infrastructure.BackgroundRoutine != nil {
		r.exports.Infrastructure.BackgroundRoutine = nil
	}
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	// 1) Terminate EventHub first. For the testkit runtimeHub this also stops
	// owner modules (cancel governance workers, unsubscribe observers).
	// 2) Then drain BackgroundRoutine so cancelled AsyncFunction loops can exit
	// and no drive/config task re-enters a dead hub.
	if r.exports.Infrastructure.CommandHub != nil {
		r.exports.Infrastructure.CommandHub.Terminate(ctx)
		r.exports.Infrastructure.CommandHub = nil
	}
	r.DrainBackground(ctx)
	return nil
}

func (r *Runtime) Exports() Exports {
	if r == nil {
		return Exports{}
	}
	return r.exports
}

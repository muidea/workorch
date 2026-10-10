package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"sync"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
)

type Runtime struct {
	exports    Exports
	background magictask.BackgroundRoutine
	shutdownMu sync.Mutex
}

type InfrastructureExports struct {
	CommandHub        magicevent.Hub
	BackgroundRoutine magictask.BackgroundRoutine
	LoadConfig        func() (workorchconfig.Config, error)
}

type Exports struct {
	Infrastructure InfrastructureExports
}

// 局部装配显式提供生命周期，不扫描全局 plugin 注册表。
type stagedRuntime interface {
	Quiesce(context.Context) error
	Release(context.Context) error
}

// DrainBackground 先排空局部 owner，再排空共享调度器；失败保留未释放资源。
func (r *Runtime) DrainBackground(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.shutdownMu.Lock()
	defer r.shutdownMu.Unlock()
	if ctx == nil {
		return fmt.Errorf("shutdown context is required")
	}
	if stages, ok := r.exports.Infrastructure.CommandHub.(stagedRuntime); ok {
		if err := stages.Quiesce(ctx); err != nil {
			return err
		}
	}
	return r.drainBackground(ctx)
}

func (r *Runtime) drainBackground(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("shutdown context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.background != nil && !r.background.Shutdown(ctx) {
		if err := ctx.Err(); err != nil {
			return err
		}
		return fmt.Errorf("background tasks are still running")
	}
	r.background = nil
	r.exports.Infrastructure.BackgroundRoutine = nil
	return nil
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.shutdownMu.Lock()
	defer r.shutdownMu.Unlock()
	if ctx == nil {
		return fmt.Errorf("shutdown context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	hub := r.exports.Infrastructure.CommandHub
	drain, draining := hub.(magicevent.DrainingHub)
	if hub != nil && !draining {
		return fmt.Errorf("runtime requires a draining Hub")
	}
	stages, staged := hub.(stagedRuntime)
	if staged {
		if err := stages.Quiesce(ctx); err != nil {
			return err
		}
	}
	if err := r.drainBackground(ctx); err != nil {
		return err
	}
	if draining {
		if err := drain.Drain(ctx); err != nil {
			return errors.Join(err, ctx.Err())
		}
	}
	if staged {
		if err := stages.Release(ctx); err != nil {
			return err
		}
	}
	if draining {
		if err := drain.TerminateChecked(ctx); err != nil {
			return err
		}
		r.exports.Infrastructure.CommandHub = nil
	}
	return nil
}

func (r *Runtime) Exports() Exports {
	if r == nil {
		return Exports{}
	}
	return r.exports
}

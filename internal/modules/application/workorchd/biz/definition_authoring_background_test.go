package biz

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/muidea/magicCommon/task"
)

type authoringTestRoutine struct {
	task.BackgroundRoutine
	queued task.Task
	err    error
}

func (r *authoringTestRoutine) AsyncTaskContext(_ context.Context, work task.Task) error {
	if r.err != nil {
		return r.err
	}
	r.queued = work
	return nil
}

func TestAuthoringTaskLifecycleDrainsQueuedCancellationAndRejectsLateWork(t *testing.T) {
	lifecycle := newAuthoringTaskLifecycle()
	routine := &authoringTestRoutine{}
	var calls atomic.Int32
	if err := lifecycle.schedule(context.Background(), routine, func(context.Context) { calls.Add(1) }); err != nil {
		t.Fatal(err)
	}
	stopped := make(chan struct{})
	go func() {
		lifecycle.stop()
		close(stopped)
	}()
	<-lifecycle.ctx.Done()
	select {
	case <-stopped:
		t.Fatal("stop did not wait for accepted task cleanup")
	default:
	}
	routine.queued.Run()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("cancelled queued task did not drain")
	}
	if calls.Load() != 0 {
		t.Fatal("cancelled queued task executed")
	}
	if err := lifecycle.schedule(context.Background(), routine, func(context.Context) {}); err == nil {
		t.Fatal("task accepted after stop")
	}
	lifecycle.stop()
}

func TestAuthoringTaskLifecycleBoundsAcceptedTasks(t *testing.T) {
	lifecycle := newAuthoringTaskLifecycle()
	routine := &authoringTestRoutine{}
	queued := []task.Task{}
	for range 32 {
		if err := lifecycle.schedule(context.Background(), routine, func(context.Context) {}); err != nil {
			t.Fatal(err)
		}
		queued = append(queued, routine.queued)
	}
	if err := lifecycle.schedule(context.Background(), routine, func(context.Context) {}); err == nil {
		t.Fatal("accepted task bound exceeded")
	}
	for _, work := range queued {
		work.Run()
	}
	lifecycle.stop()
}

func TestAuthoringTaskLifecycleSchedulingFailureDoesNotLeakDrain(t *testing.T) {
	lifecycle := newAuthoringTaskLifecycle()
	routine := &authoringTestRoutine{err: errors.New("queue unavailable")}
	if err := lifecycle.schedule(context.Background(), routine, func(context.Context) {}); err == nil {
		t.Fatal("scheduling failure hidden")
	}
	lifecycle.stop()
}

func TestAuthoringTaskLifecycleRealWorkerCancellationAndDrain(t *testing.T) {
	lifecycle := newAuthoringTaskLifecycle()
	routine := task.NewBackgroundRoutine(1)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if !routine.Shutdown(ctx) {
			t.Error("真实后台 worker 未停止")
		}
	}()
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		lifecycle.cancel()
		close(release)
		lifecycle.stop()
	}()
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	if err := lifecycle.schedule(requestCtx, routine, func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-release
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("真实 worker 未执行已接受任务")
	}
	cancelRequest()
	if lifecycle.ctx.Err() != nil {
		t.Fatal("HTTP 请求取消影响任务生命周期")
	}
	stopped := make(chan struct{})
	go func() { lifecycle.stop(); close(stopped) }()
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("stop 未取消运行任务")
	}
	select {
	case <-stopped:
		t.Fatal("stop 未等待真实任务清理")
	default:
	}
	if err := lifecycle.schedule(context.Background(), routine, func(context.Context) {}); err == nil {
		t.Fatal("停机后仍接受任务")
	}
	release <- struct{}{}
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("真实任务清理后 stop 未返回")
	}
}

func TestAuthoringTaskLifecycleDoesNotUseHTTPRequestCancellation(t *testing.T) {
	lifecycle := newAuthoringTaskLifecycle()
	routine := &authoringTestRoutine{}
	ctx, cancel := context.WithCancel(context.Background())
	called := false
	if err := lifecycle.schedule(ctx, routine, func(taskCtx context.Context) {
		if taskCtx.Err() != nil {
			t.Fatal("accepted task inherited request cancellation")
		}
		called = true
	}); err != nil {
		t.Fatal(err)
	}
	cancel()
	routine.queued.Run()
	lifecycle.stop()
	if !called {
		t.Fatal("accepted task disappeared when HTTP request ended")
	}
}

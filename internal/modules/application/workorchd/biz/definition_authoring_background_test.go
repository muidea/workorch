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

type authoringDrainCompletionCutContext struct {
	context.Context
	release func()
}

func (c authoringDrainCompletionCutContext) Done() <-chan struct{} {
	// 等待方已进入 select，真实退出恰在取消分支求值时发生。
	c.release()
	return c.Context.Done()
}

func TestAuthoringTaskLifecycleCompletedDrainWinsConcurrentCancellation(t *testing.T) {
	for range 64 {
		lifecycle := newAuthoringTaskLifecycle()
		_, release, err := lifecycle.accept(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		lifecycle.beginStop(context.Background())
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		lifecycle.drainCtx = authoringDrainCompletionCutContext{Context: cancelled, release: release}
		if err := lifecycle.stopContext(context.Background()); err != nil {
			t.Fatalf("真实任务已退出，取消分支不得误报排空失败：%v", err)
		}
		if lifecycle.active != 0 {
			t.Fatal("任务未实际退出")
		}
	}
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

func TestAuthoringTaskLifecycleDirectCallsShareBoundedSettlementDrain(t *testing.T) {
	lifecycle := newAuthoringTaskLifecycle()
	caller, cancelCaller := context.WithCancel(context.Background())
	callCtx, release, err := lifecycle.accept(caller)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	nested, nestedRelease, err := lifecycle.accept(callCtx)
	if err != nil {
		t.Fatal(err)
	}
	nestedRelease()
	if nested != callCtx || lifecycle.active != 1 {
		t.Fatal("nested accepted call was counted twice")
	}
	settlement, cancelSettlement := authoringSettlementContext(callCtx)
	defer cancelSettlement()
	cancelCaller()
	if settlement.Err() != nil {
		t.Fatal("caller cancellation discarded known-result settlement")
	}
	stopCtx, cancelStop := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancelStop()
	if err := lifecycle.stopContext(stopCtx); err == nil {
		t.Fatal("undrained direct call reported successful stop")
	}
	select {
	case <-settlement.Done():
	case <-time.After(time.Second):
		t.Fatal("settlement exceeded shared stop deadline")
	}
	if _, _, err := lifecycle.accept(context.Background()); err == nil {
		t.Fatal("direct call accepted after stop")
	}
	if _, _, err := lifecycle.accept(callCtx); err == nil {
		t.Fatal("nested call reused accepted scope after stop")
	}
	started := time.Now()
	if err := lifecycle.stopContext(context.Background()); err == nil || time.Since(started) > time.Second {
		t.Fatal("duplicate stop refreshed drain deadline")
	}
	release()
	if err := lifecycle.stopContext(context.Background()); err != nil {
		t.Fatal("released call did not drain")
	}
}

func TestAuthoringTaskLifecycleStopCallerCancellationBoundsExistingSettlement(t *testing.T) {
	lifecycle := newAuthoringTaskLifecycle()
	callCtx, release, err := lifecycle.accept(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	settlement, cancelSettlement := authoringSettlementContext(callCtx)
	defer cancelSettlement()
	stopCtx, cancelStop := context.WithCancel(context.Background())
	lifecycle.beginStop(stopCtx)
	cancelStop()
	select {
	case <-settlement.Done():
	case <-time.After(time.Second):
		t.Fatal("Stop 调用方取消后仍继续结果结算")
	}
	if err := lifecycle.stopContext(context.Background()); err == nil {
		t.Fatal("未排空调用却报告成功")
	}
	release()
	if err := lifecycle.stopContext(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAuthoringSettlementContextReusesOneKnownResultDeadline(t *testing.T) {
	caller, cancel := context.WithCancel(context.Background())
	cancel()
	settlement, finish := authoringSettlementContext(caller)
	defer finish()
	ctx := context.WithValue(settlement, authoringSettlementKey{}, settlement)
	reused, finishNested := authoringSettlementContext(ctx)
	finishNested()
	first, _ := settlement.Deadline()
	second, _ := reused.Deadline()
	if reused != settlement || first != second || reused.Err() != nil {
		t.Fatal("parser／CAS／readback 刷新了结算期限或继承 caller 取消")
	}
	finish()
	if reused.Err() == nil {
		t.Fatal("复用期限失去原 scope 的取消 fence")
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

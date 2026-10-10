package biz

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestLifecycleNil(t *testing.T) {
	var lifecycle *Lifecycle
	lifecycle.BeginShutdown()
	if err := lifecycle.Reset(); !errors.Is(err, ErrNilLifecycle) {
		t.Fatalf("Reset error = %v", err)
	}
	for _, enter := range []func() (func(), error){lifecycle.Enter, lifecycle.EnterDrain} {
		leave, err := enter()
		if leave != nil || !errors.Is(err, ErrNilLifecycle) {
			t.Fatalf("nil admission = %v, %v", leave != nil, err)
		}
	}
	if err := lifecycle.Quiesce(context.Background()); !errors.Is(err, ErrNilLifecycle) {
		t.Fatalf("Quiesce error = %v", err)
	}
	var value Lifecycle
	value.BeginShutdown()
	if err := value.Quiesce(nil); !errors.Is(err, ErrNilLifecycleContext) {
		t.Fatalf("nil context error = %v", err)
	}
}

func TestLifecycleFreshAndExplicitDrain(t *testing.T) {
	var lifecycle Lifecycle
	leave, err := lifecycle.Enter()
	if err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Quiesce(context.Background()); !errors.Is(err, ErrLifecycleNotClosing) {
		t.Fatalf("open generation Quiesce error = %v", err)
	}
	// 即使 fresh 尚未返回，关门也必须立即完成且可重复调用。
	lifecycle.BeginShutdown()
	lifecycle.BeginShutdown()
	if rejected, err := lifecycle.Enter(); rejected != nil || !errors.Is(err, ErrLifecycleClosing) {
		t.Fatalf("fresh admission after shutdown = %v, %v", rejected != nil, err)
	}
	drainLeave, err := lifecycle.EnterDrain()
	if err != nil {
		t.Fatal(err)
	}
	leave()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := lifecycle.Quiesce(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("active drain Quiesce error = %v", err)
	}
	drainLeave()
	if err := lifecycle.Quiesce(context.Background()); err != nil {
		t.Fatal(err)
	}

	// 本 owner 曾归零不能拒绝上游已接纳工作的 nested query/receipt/CAS；
	// 具体 owner 验证资格，基础 guard 只提供显式 drain 入口。
	drainLeave, err = lifecycle.EnterDrain()
	if err != nil {
		t.Fatal(err)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	if err := lifecycle.Quiesce(ctx2); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled drain wait = %v", err)
	}
	drainLeave()
	if err := lifecycle.Quiesce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rejected, err := lifecycle.Enter(); rejected != nil || !errors.Is(err, ErrLifecycleClosing) {
		t.Fatalf("Quiesce reopened fresh admission = %v, %v", rejected != nil, err)
	}
}

func TestLifecycleDuplicateLeaveDoesNotReleaseOtherCalls(t *testing.T) {
	var lifecycle Lifecycle
	first, err := lifecycle.Enter()
	if err != nil {
		t.Fatal(err)
	}
	second, err := lifecycle.Enter()
	if err != nil {
		t.Fatal(err)
	}
	lifecycle.BeginShutdown()
	var duplicates sync.WaitGroup
	for range 32 {
		duplicates.Go(first)
	}
	duplicates.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := lifecycle.Quiesce(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("duplicate leave released another call: %v", err)
	}
	second()
	second()
	if err := lifecycle.Quiesce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Reset(); err != nil {
		t.Fatal(err)
	}
	next, err := lifecycle.Enter()
	if err != nil {
		t.Fatal(err)
	}
	// 旧代次释放 closure 不能影响新代次计数。
	first()
	second()
	lifecycle.BeginShutdown()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel2()
	if err := lifecycle.Quiesce(ctx2); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stale leave released new generation: %v", err)
	}
	next()
	if err := lifecycle.Quiesce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleTimeoutPreservesInflightAndAllowsRetry(t *testing.T) {
	var lifecycle Lifecycle
	leave, err := lifecycle.Enter()
	if err != nil {
		t.Fatal(err)
	}
	lifecycle.BeginShutdown()
	for range 2 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		err := lifecycle.Quiesce(ctx)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("unfinished work unexpectedly drained: %v", err)
		}
		if err := lifecycle.Reset(); !errors.Is(err, ErrLifecycleNotDrained) {
			t.Fatalf("timeout allowed generation reset: %v", err)
		}
	}
	leave()
	if err := lifecycle.Quiesce(context.Background()); err != nil {
		t.Fatalf("retry after actual completion: %v", err)
	}
}

func TestLifecycleResetRequiresClosedDrainedGeneration(t *testing.T) {
	var lifecycle Lifecycle
	if err := lifecycle.Reset(); err != nil {
		t.Fatalf("initial Start: %v", err)
	}
	if err := lifecycle.Reset(); !errors.Is(err, ErrLifecycleNotDrained) {
		t.Fatalf("duplicate Start on open generation: %v", err)
	}
	leave, err := lifecycle.Enter()
	if err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Reset(); !errors.Is(err, ErrLifecycleNotDrained) {
		t.Fatalf("active open generation reset: %v", err)
	}
	lifecycle.BeginShutdown()
	if err := lifecycle.Reset(); !errors.Is(err, ErrLifecycleNotDrained) {
		t.Fatalf("active closing generation reset: %v", err)
	}
	leave()
	if err := lifecycle.Quiesce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Reset(); err != nil {
		t.Fatalf("drained generation restart: %v", err)
	}
	next, err := lifecycle.Enter()
	if err != nil {
		t.Fatalf("fresh admission after Reset: %v", err)
	}
	next()
	if err := lifecycle.Reset(); !errors.Is(err, ErrLifecycleNotDrained) {
		t.Fatalf("completed but still open generation reset: %v", err)
	}
	lifecycle.BeginShutdown()
	if err := lifecycle.Quiesce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleClosingAndRegistrationRace(t *testing.T) {
	for range 100 {
		var lifecycle Lifecycle
		gate := make(chan struct{})
		release := make(chan struct{})
		decisions := make(chan error, 32)
		var calls sync.WaitGroup
		for range 32 {
			calls.Go(func() {
				<-gate
				leave, err := lifecycle.Enter()
				decisions <- err
				if err == nil {
					<-release
					leave()
				}
			})
		}
		closed := make(chan struct{})
		go func() {
			<-gate
			lifecycle.BeginShutdown()
			close(closed)
		}()
		close(gate)
		<-closed
		accepted := 0
		for range 32 {
			if err := <-decisions; err == nil {
				accepted++
			} else if !errors.Is(err, ErrLifecycleClosing) {
				t.Fatalf("unexpected admission error: %v", err)
			}
		}
		if leave, err := lifecycle.Enter(); leave != nil || !errors.Is(err, ErrLifecycleClosing) {
			t.Fatalf("fresh admitted after closing linearized: %v", err)
		}
		lifecycle.mu.Lock()
		inflight := lifecycle.inflight
		lifecycle.mu.Unlock()
		if inflight != accepted {
			t.Fatalf("registered calls = %d, actual accepted = %d", inflight, accepted)
		}
		if accepted > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
			err := lifecycle.Quiesce(ctx)
			cancel()
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("closing race missed accepted work: %v", err)
			}
		}
		close(release)
		calls.Wait()
		if err := lifecycle.Quiesce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLifecycleTracksQueuedTaskUntilActualCompletion(t *testing.T) {
	var lifecycle Lifecycle
	// 在队列接受任务前注册，等待方超时不调用 leave。
	leave, err := lifecycle.Enter()
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	finish := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer leave()
		<-start
		<-finish
	}()
	lifecycle.BeginShutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	err = lifecycle.Quiesce(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued work considered complete: %v", err)
	}
	close(start)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Millisecond)
	err = lifecycle.Quiesce(ctx2)
	cancel2()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("running work considered complete: %v", err)
	}
	close(finish)
	<-done
	if err := lifecycle.Quiesce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleMultipleQuiescers(t *testing.T) {
	var lifecycle Lifecycle
	leave, err := lifecycle.Enter()
	if err != nil {
		t.Fatal(err)
	}
	lifecycle.BeginShutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	results := make(chan error, 16)
	for range 16 {
		go func() { results <- lifecycle.Quiesce(ctx) }()
	}
	leave()
	for range 16 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

// hookContext 精确控制 Quiesce 复核时机，不依赖 goroutine 调度或 sleep。
type lifecycleHookContext struct {
	context.Context
	errHook  func()
	doneHook func()
}

func (c lifecycleHookContext) Err() error {
	if c.errHook != nil {
		c.errHook()
	}
	return c.Context.Err()
}

func (c lifecycleHookContext) Done() <-chan struct{} {
	if c.doneHook != nil {
		c.doneHook()
	}
	return c.Context.Done()
}

func TestLifecycleQuiesceRechecksAfterDrainReentry(t *testing.T) {
	var lifecycle Lifecycle
	leave, err := lifecycle.Enter()
	if err != nil {
		t.Fatal(err)
	}
	lifecycle.BeginShutdown()
	firstWait := make(chan struct{})
	secondCheck := make(chan struct{})
	drainWait := make(chan struct{})
	resume := make(chan struct{})
	checks, waits := 0, 0
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	hooked := lifecycleHookContext{
		Context: ctx,
		errHook: func() {
			checks++
			if checks == 2 {
				close(secondCheck)
				<-resume
			}
		},
		doneHook: func() {
			waits++
			if waits == 1 {
				close(firstWait)
			} else if waits == 2 {
				close(drainWait)
			}
		},
	}
	result := make(chan error, 1)
	go func() { result <- lifecycle.Quiesce(hooked) }()
	<-firstWait
	leave()
	<-secondCheck
	drainLeave, err := lifecycle.EnterDrain()
	if err != nil {
		t.Fatal(err)
	}
	close(resume)
	select {
	case <-drainWait:
	case err := <-result:
		t.Fatalf("old idle wake falsely declared drain complete: %v", err)
	case <-ctx.Done():
		t.Fatal("Quiesce did not recheck the active drain")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("active drain cancellation: %v", err)
	}
	drainLeave()
	if err := lifecycle.Quiesce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleQuiesceCannotSucceedForResetGeneration(t *testing.T) {
	var lifecycle Lifecycle
	lifecycle.BeginShutdown()
	check := make(chan struct{})
	resume := make(chan struct{})
	var once sync.Once
	ctx := lifecycleHookContext{Context: context.Background(), errHook: func() {
		once.Do(func() {
			close(check)
			<-resume
		})
	}}
	result := make(chan error, 1)
	go func() { result <- lifecycle.Quiesce(ctx) }()
	<-check
	if err := lifecycle.Reset(); err != nil {
		t.Fatal(err)
	}
	lifecycle.BeginShutdown()
	close(resume)
	if err := <-result; !errors.Is(err, ErrLifecycleGenerationChanged) {
		t.Fatalf("old waiter reported new generation drained: %v", err)
	}
	if err := lifecycle.Quiesce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

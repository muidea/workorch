package biz

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"

	"github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
)

func TestBaseOwnsOnlyInjectedRuntimeAndSubscriptions(t *testing.T) {
	ctx := context.Background()
	hub := event.NewHub(16)
	background := task.NewBackgroundRoutine(4)
	t.Cleanup(func() {
		background.Shutdown(ctx)
		hub.Terminate(ctx)
	})

	base := New("unit/base", hub, background)
	if base.ID() != "unit/base" || base.EventHub() != hub || base.BackgroundRoutine() != background {
		t.Fatal("base did not retain its injected owner-neutral runtime")
	}

	called := 0
	base.SubscribeFunc("/unit/query", func(_ event.Event, result event.Result) {
		called++
		if result != nil {
			result.Set("ok", nil)
		}
	})
	result := base.SendEvent(event.NewEvent("/unit/query", "test", "unit/base", event.NewHeader(), struct{}{}))
	if result == nil || result.Error() != nil {
		t.Fatalf("SendEvent result = %#v", result)
	}
	value, err := result.Get()
	if err != nil || value != "ok" || called != 1 {
		t.Fatalf("SendEvent value=%v err=%v called=%d", value, err, called)
	}

	base.UnsubscribeFunc("/unit/query")
	result = base.SendEvent(event.NewEvent("/unit/query", "test", "unit/base", event.NewHeader(), struct{}{}))
	if result == nil || called != 1 {
		t.Fatalf("unsubscribe result=%#v called=%d", result, called)
	}
}

func TestBaseKeepsIndependentObserversForSharedDestinationMatcher(t *testing.T) {
	ctx := context.Background()
	hub := event.NewHub(16)
	background := task.NewBackgroundRoutine(4)
	t.Cleanup(func() {
		background.Shutdown(ctx)
		hub.Terminate(ctx)
	})

	first := New("unit/first", hub, background)
	second := New("unit/second", hub, background)
	var firstCalls atomic.Int32
	var secondCalls atomic.Int32
	first.SubscribeMatchedFunc("unit/first/consumer", "/run/consumers/#", "/run/state/changed", func(_ event.Event, _ event.Result) {
		firstCalls.Add(1)
	})
	second.SubscribeMatchedFunc("unit/second/consumer", "/run/consumers/#", "/run/state/changed", func(_ event.Event, _ event.Result) {
		secondCalls.Add(1)
	})

	hub.Send(event.NewEvent("/run/state/changed", "run", "/run/consumers/#", event.NewHeader(), struct{}{}))
	if firstCalls.Load() != 1 || secondCalls.Load() != 1 {
		t.Fatalf("shared matcher calls = %d/%d, want 1/1", firstCalls.Load(), secondCalls.Load())
	}

	first.UnsubscribeMatchedFunc("unit/first/consumer", "/run/state/changed")
	hub.Send(event.NewEvent("/run/state/changed", "run", "/run/consumers/#", event.NewHeader(), struct{}{}))
	if firstCalls.Load() != 1 || secondCalls.Load() != 2 {
		t.Fatalf("after first unsubscribe calls = %d/%d, want 1/2", firstCalls.Load(), secondCalls.Load())
	}
}

func TestBaseSubmitsBackgroundWork(t *testing.T) {
	ctx := context.Background()
	hub := event.NewHub(16)
	background := task.NewBackgroundRoutine(4)
	t.Cleanup(func() {
		background.Shutdown(ctx)
		hub.Terminate(ctx)
	})

	base := New("unit/base", hub, background)
	called := make(chan struct{}, 1)
	if err := base.SyncTask(func() { called <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
	case <-ctx.Done():
		t.Fatal("background task was not submitted")
	}
}

func TestBaseRejectsInvalidBackgroundWork(t *testing.T) {
	operations := []struct {
		name string
		run  func(*Base, func()) error
	}{
		{"sync", func(b *Base, f func()) error { return b.SyncTask(f) }},
		{"sync_timeout", func(b *Base, f func()) error { return b.SyncTaskWithTimeOut(f, time.Second) }},
		{"async", func(b *Base, f func()) error { return b.AsyncTask(f) }},
		{"async_context", func(b *Base, f func()) error { return b.AsyncTaskContext(context.Background(), f) }},
		{"timer", func(b *Base, f func()) error { return b.Timer(context.Background(), time.Second, 0, f) }},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			// The fake would return nil if invalid work reached the task API.
			base := New("owner", nil, rejectedBackground{})
			assertTaskErrorCode(t, operation.run(&base, nil), cd.IllegalParam)
			missing := New("owner", nil, nil)
			assertTaskErrorCode(t, operation.run(&missing, func() {}), cd.ServiceUnavailable)
			assertTaskErrorCode(t, operation.run(nil, func() {}), cd.ServiceUnavailable)
		})
	}
}

func TestBaseSubmitsAsyncAndTimerWorkToRealBackground(t *testing.T) {
	background := newTestBackground(t, 1)
	base := New("owner", nil, background)
	called := make(chan struct{}, 1)
	for _, submit := range []func(func()) error{
		base.AsyncTask,
		func(f func()) error { return base.AsyncTaskContext(context.Background(), f) },
	} {
		if err := submit(func() { called <- struct{}{} }); err != nil {
			t.Fatal(err)
		}
		waitTaskSignal(t, called)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var once sync.Once
	if err := base.Timer(ctx, time.Millisecond, 0, func() { once.Do(func() { close(called) }) }); err != nil {
		t.Fatal(err)
	}
	waitTaskSignal(t, called)
}

func TestBaseReturnsRealSyncTaskPanic(t *testing.T) {
	background := newTestBackground(t, 1)
	base := New("owner", nil, background)
	for _, run := range []func(func()) error{
		base.SyncTask,
		func(f func()) error { return base.SyncTaskWithTimeOut(f, time.Second) },
	} {
		err := run(func() { panic("original callback panic") })
		assertTaskErrorCode(t, err, cd.Unexpected)
		if !strings.Contains(err.Error(), "original callback panic") {
			t.Fatalf("panic diagnostic replaced: %v", err)
		}
	}
	if err := base.SyncTask(func() {}); err != nil {
		t.Fatalf("worker unavailable after panic: %v", err)
	}
}

func TestBaseSyncTimeoutDoesNotClaimAcceptedTaskExited(t *testing.T) {
	background := newTestBackground(t, 1)
	base := New("owner", nil, background)
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	err := base.SyncTaskWithTimeOut(func() {
		close(started)
		<-release
		close(finished)
	}, 20*time.Millisecond)
	assertTaskErrorCode(t, err, cd.Timeout)
	waitTaskSignal(t, started)
	select {
	case <-finished:
		t.Fatal("completion timeout canceled the accepted task")
	default:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if background.Shutdown(ctx) {
		t.Fatal("shutdown reported successful drain while timed-out task is running")
	}
	unblock()
	waitTaskSignal(t, finished)
}

func TestBaseSyncCompletionBudgetDoesNotBoundAdmission(t *testing.T) {
	background := newTestBackground(t, 1)
	base := New("owner", nil, background)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	if err := base.AsyncTask(func() { close(started); <-release }); err != nil {
		t.Fatal(err)
	}
	waitTaskSignal(t, started)
	fillBackgroundQueue(t, &base)
	result, finished := make(chan error, 1), make(chan struct{})
	go func() {
		result <- base.SyncTaskWithTimeOut(func() { close(finished) }, 0)
	}()
	select {
	case err := <-result:
		t.Fatalf("completion budget incorrectly bounded full-queue admission: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	unblock()
	select {
	case err := <-result:
		if err != nil {
			assertTaskErrorCode(t, err, cd.Timeout)
		}
	case <-time.After(time.Second):
		t.Fatal("sync task did not return after admission")
	}
	waitTaskSignal(t, finished)
}

func TestBaseAsyncContextBoundsAdmissionNotAcceptedTask(t *testing.T) {
	background := newTestBackground(t, 1)
	base := New("owner", nil, background)
	ctx, cancel := context.WithCancel(context.Background())
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	defer cancel()
	if err := base.AsyncTaskContext(ctx, func() { close(started); <-release; close(finished) }); err != nil {
		t.Fatal(err)
	}
	waitTaskSignal(t, started)
	cancel()
	fillBackgroundQueue(t, &base)
	admissionCtx, cancelAdmission := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelAdmission()
	var rejectedRan atomic.Bool
	if err := base.AsyncTaskContext(admissionCtx, func() { rejectedRan.Store(true) }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded admission error = %v, want deadline", err)
	}
	select {
	case <-finished:
		t.Fatal("admission context canceled an accepted callback")
	default:
	}
	unblock()
	waitTaskSignal(t, finished)
	if err := base.SyncTask(func() {}); err != nil {
		t.Fatal(err)
	}
	if rejectedRan.Load() {
		t.Fatal("rejected task was admitted")
	}
}

func TestBaseTimerCancellationDoesNotClaimCallbackExited(t *testing.T) {
	background := newTestBackground(t, 1)
	base := New("owner", nil, background)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var first atomic.Bool
	if err := base.Timer(ctx, time.Millisecond, 0, func() {
		if first.CompareAndSwap(false, true) {
			close(started)
			<-release
			close(finished)
		}
	}); err != nil {
		t.Fatal(err)
	}
	waitTaskSignal(t, started)
	cancel()
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelShutdown()
	if background.Shutdown(shutdownCtx) {
		t.Fatal("timer cancellation falsely reported callback drain")
	}
	unblock()
	waitTaskSignal(t, finished)
}

func TestBaseReturnsRealBackgroundValidationAndClosedErrors(t *testing.T) {
	background := newTestBackground(t, 1)
	base := New("owner", nil, background)
	callback := func() {}
	assertTaskErrorCode(t, base.SyncTaskWithTimeOut(callback, -2), cd.IllegalParam)
	if err := base.SyncTaskWithTimeOut(callback, -1); err != nil {
		t.Fatal(err)
	}
	for _, call := range []func() error{
		func() error { return base.AsyncTaskContext(nil, callback) },
		func() error { return base.Timer(nil, time.Second, 0, callback) },
		func() error { return base.Timer(context.Background(), 0, 0, callback) },
	} {
		if err := call(); err == nil {
			t.Fatal("invalid task parameters were accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := base.AsyncTaskContext(ctx, callback); err == nil {
		t.Fatal("canceled admission accepted")
	}
	if err := base.Timer(ctx, time.Second, 0, callback); !errors.Is(err, context.Canceled) {
		t.Fatalf("timer cancellation error = %v", err)
	}
	if !background.Shutdown(context.Background()) {
		t.Fatal("background did not drain")
	}
	for _, call := range []func() error{
		func() error { return base.SyncTask(callback) },
		func() error { return base.SyncTaskWithTimeOut(callback, time.Second) },
		func() error { return base.AsyncTask(callback) },
		func() error { return base.AsyncTaskContext(context.Background(), callback) },
		func() error { return base.Timer(context.Background(), time.Second, 0, callback) },
	} {
		if err := call(); err == nil {
			t.Fatal("closed background submission error swallowed")
		}
	}
}

func newTestBackground(t *testing.T, capacity int) task.BackgroundRoutine {
	t.Helper()
	background := task.NewBackgroundRoutine(capacity)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if !background.Shutdown(ctx) {
			t.Error("background did not really drain")
		}
	})
	return background
}

func waitTaskSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal("task signal not observed")
	}
}

func assertTaskErrorCode(t *testing.T, err error, code cd.Code) {
	t.Helper()
	var frameworkErr *cd.Error
	if !errors.As(err, &frameworkErr) || frameworkErr.Code != code {
		t.Fatalf("error = %v, want framework code %d", err, code)
	}
}

// With a capacity-one worker blocked, the dispatcher can hold one waiting task
// and the admission channel one more. Both slots must be occupied before testing
// bounded admission; each fill has its own guard against a broken fixture.
func fillBackgroundQueue(t *testing.T, base *Base) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for range 2 {
		if err := base.AsyncTaskContext(ctx, func() {}); err != nil {
			t.Fatalf("fill background queue: %v", err)
		}
	}
}

func TestBaseOwnsDestinationMatchedSubscriptions(t *testing.T) {
	ctx := context.Background()
	hub := event.NewHub(16)
	background := task.NewBackgroundRoutine(4)
	t.Cleanup(func() {
		background.Shutdown(ctx)
		hub.Terminate(ctx)
	})

	base := New("unit/base", hub, background)
	called := 0
	base.SubscribeMatchedFunc("unit/base/prerequisite", "kernel/prerequisite", "/unit/prerequisite", func(_ event.Event, result event.Result) {
		called++
		if result != nil {
			result.Set("ok", nil)
		}
	})
	result := base.SendEvent(event.NewEvent("/unit/prerequisite", "test", "kernel/prerequisite", event.NewHeader(), struct{}{}))
	if result == nil || result.Error() != nil || called != 1 {
		t.Fatalf("matched subscription result=%#v called=%d", result, called)
	}

	base.UnsubscribeMatchedFunc("unit/base/prerequisite", "/unit/prerequisite")
	base.SendEvent(event.NewEvent("/unit/prerequisite", "test", "kernel/prerequisite", event.NewHeader(), struct{}{}))
	if called != 1 {
		t.Fatalf("matched subscription was not removed, called=%d", called)
	}
}

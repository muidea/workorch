package biz

import (
	"context"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
)

type rejectedSubscriptionHub struct {
	event.Hub
	subscribeErr, unsubscribeErr *cd.Error
}

type rejectedBackground struct {
	task.BackgroundRoutine
	err error
}

func (b rejectedBackground) SyncTask(task.Task) error                           { return b.err }
func (b rejectedBackground) AsyncTask(task.Task) error                          { return b.err }
func (b rejectedBackground) SyncTaskWithTimeOut(task.Task, time.Duration) error { return b.err }
func (b rejectedBackground) AsyncTaskContext(context.Context, task.Task) error  { return b.err }
func (b rejectedBackground) Timer(context.Context, task.Task, time.Duration, time.Duration) error {
	return b.err
}

func TestBackgroundTaskFailureIsReturnedUnchanged(t *testing.T) {
	for _, mode := range []string{"sync", "sync_timeout", "async", "async_context", "timer"} {
		t.Run(mode, func(t *testing.T) {
			rejection := cd.NewError(cd.ResourceExhausted, "injected task rejection")
			base := New("owner", nil, rejectedBackground{err: rejection})
			run := func() { t.Fatal("rejected task executed") }
			var err error
			switch mode {
			case "sync":
				err = base.SyncTask(run)
			case "sync_timeout":
				err = base.SyncTaskWithTimeOut(run, time.Second)
			case "async":
				err = base.AsyncTask(run)
			case "async_context":
				err = base.AsyncTaskContext(context.Background(), run)
			case "timer":
				err = base.Timer(context.Background(), time.Second, 0, run)
			}
			if err != rejection {
				t.Fatalf("task error swallowed or replaced: %v", err)
			}
		})
	}
}

func (h *rejectedSubscriptionHub) Subscribe(topic string, observer event.Observer) *cd.Error {
	if h.subscribeErr != nil {
		return h.subscribeErr
	}
	return h.Hub.Subscribe(topic, observer)
}

func (h *rejectedSubscriptionHub) Unsubscribe(topic string, observer event.Observer) *cd.Error {
	if h.unsubscribeErr != nil {
		return h.unsubscribeErr
	}
	return h.Hub.Unsubscribe(topic, observer)
}

func TestCheckedSubscriptionFailurePreservesErrorAndRetryableState(t *testing.T) {
	for _, mode := range []string{"observer", "callback", "matched_callback"} {
		t.Run(mode, func(t *testing.T) {
			hub := &rejectedSubscriptionHub{Hub: event.NewHub(16)}
			defer hub.Terminate(context.Background())
			base := New("owner", hub, nil)
			const topic = "/checked/operation"
			handler := func(_ event.Event, result event.Result) { result.Set("handled", nil) }
			var subscribe, unsubscribe func() error
			switch mode {
			case "observer":
				observer := event.NewSimpleObserver("owner", hub.Hub)
				if err := observer.Subscribe(topic, handler); err != nil {
					t.Fatal(err)
				}
				if err := hub.Hub.Unsubscribe(topic, observer); err != nil {
					t.Fatal(err)
				}
				subscribe = func() error { return base.SubscribeChecked(topic, observer) }
				unsubscribe = func() error { return base.UnsubscribeChecked(topic, observer) }
			case "callback":
				subscribe = func() error { return base.SubscribeFuncChecked(topic, handler) }
				unsubscribe = func() error { return base.UnsubscribeFuncChecked(topic) }
			case "matched_callback":
				subscribe = func() error { return base.SubscribeMatchedFuncChecked("observer", "owner", topic, handler) }
				unsubscribe = func() error { return base.UnsubscribeMatchedFuncChecked("observer", topic) }
			}
			assertRoute := func(present bool) {
				t.Helper()
				value, err := hub.Send(event.NewEvent(topic, "caller", "owner", nil, nil)).Get()
				if present && (err != nil || value != "handled") || !present && err == nil {
					t.Fatalf("routing present=%v value=%v error=%v", present, value, err)
				}
			}
			rejection := cd.NewError(cd.ResourceExhausted, "injected checked subscription rejection")
			hub.subscribeErr = rejection
			if err := subscribe(); err != rejection {
				t.Fatalf("subscribe error replaced: %v", err)
			}
			assertRoute(false)
			hub.subscribeErr = nil
			if err := subscribe(); err != nil {
				t.Fatal(err)
			}
			assertRoute(true)
			hub.unsubscribeErr = rejection
			if err := unsubscribe(); err != rejection {
				t.Fatalf("unsubscribe error replaced: %v", err)
			}
			assertRoute(true)
			hub.unsubscribeErr = nil
			if err := unsubscribe(); err != nil {
				t.Fatal(err)
			}
			assertRoute(false)
			if err := unsubscribe(); err != nil {
				t.Fatalf("duplicate cleanup failed: %v", err)
			}
		})
	}
}

func TestCheckedSubscriptionHelpersRejectMissingRuntime(t *testing.T) {
	base := New("owner", nil, nil)
	callback := func(event.Event, event.Result) {}
	for _, call := range []func() error{
		func() error { return base.SubscribeChecked("topic", nil) },
		func() error { return base.UnsubscribeChecked("topic", nil) },
		func() error { return base.SubscribeFuncChecked("topic", callback) },
		func() error { return base.SubscribeMatchedFuncChecked("observer", "owner", "topic", callback) },
	} {
		if err := call(); err == nil {
			t.Fatal("missing runtime reported subscription success")
		}
	}
	if err := base.UnsubscribeFuncChecked("topic"); err != nil {
		t.Fatalf("unacquired simple subscription cleanup = %v", err)
	}
	if err := base.UnsubscribeMatchedFuncChecked("observer", "topic"); err != nil {
		t.Fatalf("unacquired matched subscription cleanup = %v", err)
	}
}

func TestRequiredSubscriptionFailurePreservesFrameworkErrorAndObserverState(t *testing.T) {
	for _, mode := range []string{"observer", "callback", "matched_callback"} {
		t.Run(mode, func(t *testing.T) {
			hub := &rejectedSubscriptionHub{Hub: event.NewHub(16)}
			defer hub.Terminate(context.Background())
			base := New("owner", hub, nil)
			const topic = "/required/operation"
			handler := func(_ event.Event, result event.Result) { result.Set("handled", nil) }
			var subscribe, unsubscribe func()
			switch mode {
			case "observer":
				observer := event.NewSimpleObserver("owner", hub.Hub)
				if err := observer.Subscribe(topic, handler); err != nil {
					t.Fatal(err)
				}
				if err := hub.Hub.Unsubscribe(topic, observer); err != nil {
					t.Fatal(err)
				}
				subscribe = func() { base.Subscribe(topic, observer) }
				unsubscribe = func() { base.Unsubscribe(topic, observer) }
			case "callback":
				subscribe = func() { base.SubscribeFunc(topic, handler) }
				unsubscribe = func() { base.UnsubscribeFunc(topic) }
			case "matched_callback":
				subscribe = func() { base.SubscribeMatchedFunc("observer", "owner", topic, handler) }
				unsubscribe = func() { base.UnsubscribeMatchedFunc("observer", topic) }
			}
			rejection := cd.NewError(cd.ResourceExhausted, "injected subscription rejection")
			assertPanic := func(call func()) {
				t.Helper()
				defer func() {
					if got := recover(); got != rejection {
						t.Fatalf("required operation swallowed or replaced framework error: %v", got)
					}
				}()
				call()
			}
			assertRoute := func(present bool) {
				t.Helper()
				value, err := hub.Send(event.NewEvent(topic, "caller", "owner", nil, nil)).Get()
				if present && (err != nil || value != "handled") || !present && err == nil {
					t.Fatalf("routing present=%v value=%v error=%v", present, value, err)
				}
			}
			hub.subscribeErr = rejection
			assertPanic(subscribe)
			assertRoute(false)
			hub.subscribeErr = nil
			subscribe()
			assertRoute(true)
			hub.unsubscribeErr = rejection
			assertPanic(unsubscribe)
			assertRoute(true)
			hub.unsubscribeErr = nil
			unsubscribe()
			assertRoute(false)
		})
	}
}

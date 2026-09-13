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

func (b rejectedBackground) SyncTask(task.Task) error  { return b.err }
func (b rejectedBackground) AsyncTask(task.Task) error { return b.err }
func (b rejectedBackground) Timer(context.Context, task.Task, time.Duration, time.Duration) error {
	return b.err
}

func TestRequiredBackgroundTaskFailureIsNotSwallowed(t *testing.T) {
	for _, mode := range []string{"sync", "async", "timer"} {
		t.Run(mode, func(t *testing.T) {
			rejection := cd.NewError(cd.ResourceExhausted, "injected task rejection")
			base := New("owner", nil, rejectedBackground{err: rejection})
			defer func() {
				if got := recover(); got != rejection {
					t.Fatalf("task error swallowed or replaced: %v", got)
				}
			}()
			run := func() { t.Fatal("rejected task executed") }
			switch mode {
			case "sync":
				base.SyncTask(run)
			case "async":
				base.AsyncTask(run)
			case "timer":
				base.Timer(context.Background(), time.Second, 0, run)
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

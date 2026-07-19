package biz

import (
	"context"
	"testing"

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
	base.SyncTask(func() { called <- struct{}{} })
	select {
	case <-called:
	case <-ctx.Done():
		t.Fatal("background task was not submitted")
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

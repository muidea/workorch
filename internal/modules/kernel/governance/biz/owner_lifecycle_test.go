package governance

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	"github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
)

type lifecycleHub struct {
	event.Hub
	subs           int
	failAt         int
	unsubscribeErr *cd.Error
	panicValue     any
	removed        []string
}

func (h *lifecycleHub) Subscribe(topic string, observer event.Observer) *cd.Error {
	h.subs++
	if h.panicValue != nil {
		panic(h.panicValue)
	}
	if h.subs == h.failAt {
		return cd.NewError(cd.ResourceExhausted, "subscribe failed")
	}
	return h.Hub.Subscribe(topic, observer)
}
func (h *lifecycleHub) Unsubscribe(topic string, observer event.Observer) *cd.Error {
	if h.panicValue != nil {
		panic(h.panicValue)
	}
	if h.unsubscribeErr != nil {
		return h.unsubscribeErr
	}
	if err := h.Hub.Unsubscribe(topic, observer); err != nil {
		return err
	}
	h.removed = append(h.removed, topic)
	return nil
}
func lifecycleService(hub event.Hub, background task.BackgroundRoutine) *Service {
	return NewEventService(memory.NewAutoGovernanceTraceRepository(), hub, nil, background)
}
func TestOwnerQuiesceTimeoutPreservesWorkerAndQueuedReceipts(t *testing.T) {
	hub := event.NewHub(16)
	background := task.NewBackgroundRoutine(2)
	owner := lifecycleService(hub, background)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() {
		once.Do(func() { close(release) })
		owner.StopChecked(context.Background())
		background.Shutdown(context.Background())
		hub.Terminate(context.Background())
	})
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	owner.enqueueGovernanceWork(func(context.Context) { close(started); <-release })
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not run")
	}
	var queued atomic.Bool
	owner.enqueueGovernanceWork(func(context.Context) { queued.Store(true) })
	done, queue, cancel := owner.workDone, owner.workQueue, owner.workCancel
	owner.BeginShutdown()
	owner.enqueueGovernanceWork(func(context.Context) { t.Error("fresh work after shutdown") })
	if err := owner.Start(); !errors.Is(err, basebiz.ErrLifecycleClosing) {
		t.Fatalf("Start reopened owner: %v", err)
	}
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stop()
	if err := owner.StopChecked(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unfinished shutdown = %v", err)
	}
	if owner.workDone != done || owner.workQueue != queue || owner.workCancel == nil || cancel == nil || len(owner.unsubscribes) != 2 || len(owner.topics) != 1 {
		t.Fatal("timeout lost handles or subscriptions")
	}
	leave, err := owner.enterOwner(true)
	if err != nil {
		t.Fatal(err)
	}
	leave()
	once.Do(func() { close(release) })
	if err := owner.StopChecked(context.Background()); err != nil {
		t.Fatal(err)
	}
	if queued.Load() {
		t.Fatal("canceled queued work performed new owner I/O")
	}
	if owner.workDone != nil || owner.workCancel != nil || len(owner.unsubscribes) != 0 || len(owner.topics) != 0 {
		t.Fatal("real exit did not release ledger")
	}
}
func TestOwnerPartialSubscriptionFailureAndCancelFailureRetainLedger(t *testing.T) {
	hub := &lifecycleHub{Hub: event.NewHub(16), failAt: 2}
	defer hub.Terminate(context.Background())
	owner := lifecycleService(hub, nil)
	if err := owner.Start(); err == nil {
		t.Fatal("partial Start succeeded")
	}
	if len(owner.unsubscribes) != 1 || owner.workDone == nil {
		t.Fatal("partial Start lost acquired state")
	}
	rejection := cd.NewError(cd.ResourceExhausted, "unsubscribe failed")
	hub.unsubscribeErr = rejection
	if err := owner.StopChecked(context.Background()); err != rejection {
		t.Fatalf("cancel error = %v", err)
	}
	if len(owner.unsubscribes) != 1 || owner.workDone == nil {
		t.Fatal("cancel failure cleared state")
	}
	hub.unsubscribeErr = nil
	if err := owner.StopChecked(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(hub.removed) != 1 {
		t.Fatalf("cleanup = %v", hub.removed)
	}
}
func TestOwnerUnknownHubPanicLeavesCleanupUnlockedAndRetryable(t *testing.T) {
	hub := &lifecycleHub{Hub: event.NewHub(16)}
	defer hub.Terminate(context.Background())
	owner := lifecycleService(hub, nil)
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	hub.panicValue = "unknown cancel panic"
	func() {
		defer func() {
			if recover() != hub.panicValue {
				t.Fatal("unknown panic replaced")
			}
		}()
		_ = owner.StopChecked(context.Background())
	}()
	if len(owner.topics) == 0 || len(owner.unsubscribes) == 0 {
		t.Fatal("panic lost cleanup ledger")
	}
	hub.panicValue = nil
	if err := owner.StopChecked(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(hub.removed) != 3 {
		t.Fatalf("cleanup order/count = %v", hub.removed)
	}
}

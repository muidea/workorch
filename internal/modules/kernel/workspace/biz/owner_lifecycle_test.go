package workspace

import (
	"context"
	"errors"
	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	events "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"

	"sync"
	"testing"
	"time"
)

type lifecycleFailureHub struct {
	magicevent.Hub
	subscribeCalls, unsubscribeCalls, failSubscribe, failUnsubscribe int
	failure                                                          *cd.Error
	panicUnsubscribe                                                 bool
}

func (h *lifecycleFailureHub) Subscribe(topic string, observer magicevent.Observer) *cd.Error {
	h.subscribeCalls++
	if h.subscribeCalls == h.failSubscribe {
		return h.failure
	}
	return h.Hub.Subscribe(topic, observer)
}
func (h *lifecycleFailureHub) Unsubscribe(topic string, observer magicevent.Observer) *cd.Error {
	h.unsubscribeCalls++
	if h.panicUnsubscribe {
		panic("unexpected unsubscribe panic")
	}
	if h.unsubscribeCalls == h.failUnsubscribe {
		return h.failure
	}
	return h.Hub.Unsubscribe(topic, observer)
}
func lifecycleEvent(ctx context.Context, topic, lane string) magicevent.Event {
	ev := magicevent.NewEventWithContext(topic, "test", ModuleID, nil, ctx, struct{}{})
	ev.BindLaneKey(lane)
	return ev
}
func lifecycleOwner(t *testing.T, hub magicevent.Hub) *Biz {
	t.Helper()
	b, err := New(hub, nil, Dependencies{EventHub: hub, OwnerRepositories: memoryrepo.NewWorkspaceOwnerRepositorySet().OwnerRepositories()})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestOwnerLifecyclePartialSubscriptionLedger(t *testing.T) {
	ctx := context.Background()
	probeHub := magicevent.NewHub(16)
	probe := lifecycleOwner(t, probeHub)
	if err := probe.Start(); err != nil {
		t.Fatal(err)
	}
	count := len(probe.subscriptions)
	if err := probe.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
	probeHub.Terminate(ctx)
	for n := 1; n <= count; n++ {
		t.Run(string(rune('A'+n-1)), func(t *testing.T) {
			hub := &lifecycleFailureHub{Hub: magicevent.NewHub(16), failSubscribe: n, failure: cd.NewError(cd.ResourceExhausted, "subscribe rejected")}
			defer hub.Terminate(ctx)
			b := lifecycleOwner(t, hub)
			if err := b.Start(); !errors.Is(err, hub.failure) {
				t.Fatalf("Start error = %v", err)
			}
			if b.started || len(b.subscriptions) != n-1 || b.manager == nil {
				t.Fatal("partial start lost ledger or dependency")
			}
			if n > 1 {
				if err := b.Start(); !errors.Is(err, common.ErrConflict) {
					t.Fatalf("partial restart = %v", err)
				}
				if hub.subscribeCalls != n {
					t.Fatal("partial restart duplicated topics")
				}
			}
			if err := b.Teardown(ctx); err != nil {
				t.Fatal(err)
			}
			if hub.unsubscribeCalls != n-1 || b.manager != nil {
				t.Fatal("partial cleanup did not release exactly acquired topics")
			}
		})
	}
	for n := 1; n <= count; n++ {
		hub := &lifecycleFailureHub{Hub: magicevent.NewHub(16), failUnsubscribe: n, failure: cd.NewError(cd.Unexpected, "unsubscribe rejected")}
		b := lifecycleOwner(t, hub)
		if err := b.Start(); err != nil {
			t.Fatal(err)
		}
		if err := b.Teardown(ctx); !errors.Is(err, hub.failure) {
			t.Fatalf("Teardown error = %v", err)
		}
		if len(b.subscriptions) != count-n+1 || b.manager == nil {
			t.Fatal("failed cleanup lost remaining resources")
		}
		hub.failUnsubscribe = 0
		if err := b.Teardown(ctx); err != nil {
			t.Fatal(err)
		}
		if len(b.subscriptions) != 0 || b.manager != nil {
			t.Fatal("retry failed to release resources")
		}
		hub.Terminate(ctx)
	}
}
func TestOwnerLifecycleWaitsForActualHandlerExit(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	b := lifecycleOwner(t, hub)
	entered, release, exited := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(func() { unblock(); _ = b.Teardown(ctx); hub.Terminate(ctx) })
	if err := b.subscribeOwner(events.TopicWorkspaceCommandCreate, func(_ magicevent.Event, result magicevent.Result) {
		close(entered)
		<-release
		result.Set(struct{}{}, nil)
		close(exited)
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := b.subscribeOwner(events.TopicWorkspaceCommandGet, func(_ magicevent.Event, result magicevent.Result) { result.Set(struct{}{}, nil) }, false); err != nil {
		t.Fatal(err)
	}
	callCtx, cancel := context.WithCancel(ctx)
	sent := make(chan struct{})
	go func() {
		defer close(sent)
		hub.Send(lifecycleEvent(callCtx, events.TopicWorkspaceCommandCreate, "active-lane"))
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not enter")
	}
	cancel()
	select {
	case <-sent:
		t.Fatal("Send cancellation incorrectly completed an executing handler")
	default:
	}
	b.BeginShutdown(ctx)
	short, stop := context.WithTimeout(ctx, 10*time.Millisecond)
	defer stop()
	if err := b.Teardown(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Teardown error = %v", err)
	}
	if b.manager == nil || len(b.subscriptions) != 2 {
		t.Fatal("timeout released resources before actual exit")
	}
	queryCtx, queryCancel := context.WithTimeout(ctx, time.Second)
	defer queryCancel()
	query := hub.Send(lifecycleEvent(queryCtx, events.TopicWorkspaceCommandGet, "query-lane"))
	if err := query.Error(); err != nil {
		t.Fatalf("drain query = %v", err)
	}
	fresh := hub.Send(lifecycleEvent(queryCtx, events.TopicWorkspaceCommandCreate, "fresh-lane"))
	if err := fresh.Error(); !errors.Is(err, basebiz.ErrLifecycleClosing) {
		t.Fatalf("fresh mutation = %v", err)
	}
	if err := b.Start(); !errors.Is(err, basebiz.ErrLifecycleClosing) {
		t.Fatalf("Start reopened closing owner = %v", err)
	}
	unblock()
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("handler did not exit")
	}
	retryCtx, retryCancel := context.WithTimeout(ctx, time.Second)
	defer retryCancel()
	if err := b.Teardown(retryCtx); err != nil {
		t.Fatal(err)
	}
	if _, err := b.enterOwner(true); !errors.Is(err, basebiz.ErrLifecycleClosing) {
		t.Fatalf("released drain = %v", err)
	}
}

type queuedProjectionBackground struct {
	task.BackgroundRoutine
	callback func()
	timer    task.Task
	ctx      context.Context
}

func (b *queuedProjectionBackground) Timer(ctx context.Context, callback task.Task, _, _ time.Duration) error {
	b.ctx, b.timer = ctx, callback
	return nil
}
func (b *queuedProjectionBackground) AsyncFunction(callback func()) error {
	b.callback = callback
	return nil
}

func TestOwnerLifecycleTracksAcceptedQueueUntilActualCallbackExit(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	b := lifecycleOwner(t, hub)
	background := &queuedProjectionBackground{}
	b.Base = basebiz.New(ModuleID, hub, background)
	t.Cleanup(func() {
		if background.callback != nil {
			background.callback()
			background.callback = nil
		}
		_ = b.Teardown(ctx)
		hub.Terminate(ctx)
	})
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	if background.callback == nil || background.timer == nil {
		t.Fatal("startup scheduling was not accepted")
	}
	b.BeginShutdown(ctx)
	if background.ctx.Err() != context.Canceled {
		t.Fatal("shutdown did not cancel future scheduling")
	}
	short, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if err := b.Teardown(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued work Stop = %v", err)
	}
	if b.manager == nil || b.runProjectionCancel == nil || len(b.subscriptions) == 0 {
		t.Fatal("queued callback lost dependencies")
	}
	background.timer.Run()
	background.callback()
	background.callback = nil
	if err := b.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
	background.timer.Run()
	if b.manager != nil || b.runProjectionCtx != nil {
		t.Fatal("actual completion did not release dependencies")
	}
}

func TestOwnerLifecycleWaitsForActualNestedInventorySend(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	b := lifecycleOwner(t, hub)
	background := &queuedProjectionBackground{}
	b.Base = basebiz.New(ModuleID, hub, background)
	entered, release, exited := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	observer := magicevent.NewSimpleObserverWithMatchID("workspace-inventory-test", runevents.CommandDestination, hub)
	if err := observer.Subscribe(runevents.TopicRunCommandLifecycleFactInventory, func(_ magicevent.Event, result magicevent.Result) {
		close(entered)
		<-release
		result.Set(runevents.QueryRunLifecycleFactInventoryResult{}, nil)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unblock(); <-exited; _ = b.Teardown(ctx); hub.Terminate(ctx) })
	if err := b.Start(); err != nil {
		close(exited)
		t.Fatal(err)
	}
	go func() { defer close(exited); background.callback() }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("inventory Send did not enter")
	}
	b.BeginShutdown(ctx)
	short, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if err := b.Teardown(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("active inventory Stop = %v", err)
	}
	select {
	case <-exited:
		t.Fatal("cancellation falsely completed nested Send")
	default:
	}
	if b.manager == nil || b.runProjectionCancel == nil {
		t.Fatal("nested Send lost dependencies")
	}
	unblock()
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("inventory callback did not exit")
	}
	if err := b.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerLifecycleTracksLatePostUntilActualExit(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	b := lifecycleOwner(t, hub)
	entered, release, exited := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(func() { unblock(); _ = b.Teardown(ctx); hub.Terminate(ctx) })
	if err := b.subscribeOwner(runevents.TopicRunStateChanged, func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			t.Error("Post unexpectedly has Result")
		}
		close(entered)
		<-release
		close(exited)
	}, true); err != nil {
		t.Fatal(err)
	}
	b.BeginShutdown(ctx)
	hub.Post(magicevent.NewEventWithContext(runevents.TopicRunStateChanged, runevents.CommandDestination, runevents.NotificationDestination, nil, ctx, struct{}{}))
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("late Post did not enter drain")
	}
	short, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if err := b.Teardown(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Post Stop = %v", err)
	}
	unblock()
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("Post did not exit")
	}
	if err := b.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerLifecycleUnknownPanicRetainsRetryableLedger(t *testing.T) {
	ctx := context.Background()
	hub := &lifecycleFailureHub{Hub: magicevent.NewHub(16), panicUnsubscribe: true}
	defer hub.Terminate(ctx)
	b := lifecycleOwner(t, hub)
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	count := len(b.subscriptions)
	func() {
		defer func() {
			if recover() != "unexpected unsubscribe panic" {
				t.Fatal("unknown panic was swallowed or changed")
			}
		}()
		_ = b.Teardown(ctx)
	}()
	if b.releasing || b.manager == nil || len(b.subscriptions) != count {
		t.Fatal("panic poisoned cleanup claim or lost ledger")
	}
	hub.panicUnsubscribe = false
	if err := b.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
}

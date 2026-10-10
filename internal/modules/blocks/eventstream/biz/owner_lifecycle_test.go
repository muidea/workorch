package eventstream

import (
	"context"
	"errors"
	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	events "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
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
	ev := magicevent.NewEventWithContext(topic, "test", "blocks/eventstream", nil, ctx, struct{}{})
	ev.BindLaneKey(lane)
	return ev
}
func lifecycleOwner(t *testing.T, hub magicevent.Hub) *Biz {
	t.Helper()
	b := New(Dependencies{EventHub: hub, OwnerRepositories: memoryrepo.NewEventStreamOwnerRepositorySet().OwnerRepositories()})
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
	if err := probe.StopChecked(ctx); err != nil {
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
			if b.started || len(b.subscriptions) != n-1 || b.repo == nil {
				t.Fatal("partial start lost ledger or dependency")
			}
			if n > 1 {
				if err := b.Start(); !errors.Is(err, common.ErrConflict) {
					t.Fatalf("partial restart = %v", err)
				}
				if hub.subscribeCalls != n {
					t.Fatal("partial restart duplicated subscriptions")
				}
			}
			if err := b.StopChecked(ctx); err != nil {
				t.Fatal(err)
			}
			if hub.unsubscribeCalls != n-1 || b.repo != nil {
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
		if err := b.StopChecked(ctx); !errors.Is(err, hub.failure) {
			t.Fatalf("Teardown error = %v", err)
		}
		if len(b.subscriptions) != count-n+1 || b.repo == nil {
			t.Fatal("failed cleanup lost remaining resources")
		}
		hub.failUnsubscribe = 0
		if err := b.StopChecked(ctx); err != nil {
			t.Fatal(err)
		}
		if len(b.subscriptions) != 0 || b.repo != nil {
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
	t.Cleanup(func() { unblock(); _ = b.StopChecked(ctx); hub.Terminate(ctx) })
	if err := b.subscribeOwnerCommand(events.TopicEventStreamCommandSealWorkspaceDeletion, func(_ magicevent.Event, result magicevent.Result) {
		close(entered)
		<-release
		result.Set(struct{}{}, nil)
		close(exited)
	}); err != nil {
		t.Fatal(err)
	}
	if err := b.subscribeOwnerCommand(events.TopicEventStreamCommandList, func(_ magicevent.Event, result magicevent.Result) { result.Set(struct{}{}, nil) }); err != nil {
		t.Fatal(err)
	}
	callCtx, cancel := context.WithCancel(ctx)
	sent := make(chan struct{})
	go func() {
		defer close(sent)
		hub.Send(lifecycleEvent(callCtx, events.TopicEventStreamCommandSealWorkspaceDeletion, "active-lane"))
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
	if err := b.StopChecked(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Teardown error = %v", err)
	}
	if b.repo == nil || len(b.subscriptions) != 2 {
		t.Fatal("timeout released resources before actual exit")
	}
	queryCtx, queryCancel := context.WithTimeout(ctx, time.Second)
	defer queryCancel()
	query := hub.Send(lifecycleEvent(queryCtx, events.TopicEventStreamCommandList, "query-lane"))
	if err := query.Error(); err != nil {
		t.Fatalf("drain query = %v", err)
	}
	fresh := hub.Send(lifecycleEvent(queryCtx, events.TopicEventStreamCommandSealWorkspaceDeletion, "fresh-lane"))
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
	if err := b.StopChecked(retryCtx); err != nil {
		t.Fatal(err)
	}
	if _, err := b.enterOwner(true); !errors.Is(err, basebiz.ErrLifecycleClosing) {
		t.Fatalf("released drain = %v", err)
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
		_ = b.StopChecked(ctx)
	}()
	if b.releasing || b.repo == nil || len(b.subscriptions) != count {
		t.Fatal("panic poisoned cleanup claim or lost ledger")
	}
	hub.panicUnsubscribe = false
	if err := b.StopChecked(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerLifecycleTracksPostNotificationUntilActualExit(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	b := lifecycleOwner(t, hub)
	entered, release, exited := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(func() { unblock(); _ = b.StopChecked(ctx); hub.Terminate(ctx) })
	const observerID = "eventstream-post-drain"
	if err := b.subscribeOwnerNotification(observerID, observerID, "audit-post-test", func(_ magicevent.Event, result magicevent.Result) {
		if result != nil {
			t.Error("Post unexpectedly supplied a synchronous result")
		}
		close(entered)
		<-release
		close(exited)
	}); err != nil {
		t.Fatal(err)
	}
	b.BeginShutdown(ctx)
	hub.Post(magicevent.NewEventWithContext("audit-post-test", "test", observerID, nil, ctx, struct{}{}))
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("late audit did not enter")
	}
	short, stop := context.WithTimeout(ctx, 10*time.Millisecond)
	defer stop()
	if err := b.StopChecked(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Post drain = %v", err)
	}
	if b.repo == nil || len(b.subscriptions) != 1 {
		t.Fatal("Post timeout lost resources")
	}
	unblock()
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("Post did not exit")
	}
	if err := b.StopChecked(ctx); err != nil {
		t.Fatal(err)
	}
}

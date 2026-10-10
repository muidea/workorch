package orchestrator

import (
	"context"
	"errors"
	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	events "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"

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
	b := New()
	deps := moduleContractDependencies()
	deps.CommandHub.Terminate(context.Background())
	deps.CommandHub = hub
	if err := b.Configure(deps); err != nil {
		t.Fatal(err)
	}
	b.Base = basebiz.New(ModuleID, hub, nil)

	return b
}
func TestOwnerLifecyclePartialSubscriptionLedger(t *testing.T) {
	ctx := context.Background()
	probeHub := magicevent.NewHub(16)
	probe := lifecycleOwner(t, probeHub)
	if err := probe.subscribeRuntimeIngress(); err != nil {
		t.Fatal(err)
	}
	count := len(probe.subscriptions)
	if err := probe.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	probeHub.Terminate(ctx)
	for n := 1; n <= count; n++ {
		t.Run(string(rune('A'+n-1)), func(t *testing.T) {
			hub := &lifecycleFailureHub{Hub: magicevent.NewHub(16), failSubscribe: n, failure: cd.NewError(cd.ResourceExhausted, "subscribe rejected")}
			defer hub.Terminate(ctx)
			b := lifecycleOwner(t, hub)
			if err := b.subscribeRuntimeIngress(); !errors.Is(err, hub.failure) {
				t.Fatalf("Start error = %v", err)
			}
			if b.started || len(b.subscriptions) != n-1 || b.runtimeController == nil {
				t.Fatal("partial start lost ledger or dependency")
			}
			if n > 1 {
				if err := b.subscribeRuntimeIngress(); !errors.Is(err, common.ErrConflict) {
					t.Fatalf("partial restart = %v", err)
				}
				if hub.subscribeCalls != n {
					t.Fatal("partial restart duplicated topics")
				}
			}
			if err := b.Stop(ctx); err != nil {
				t.Fatal(err)
			}
			if hub.unsubscribeCalls != n-1 || b.runtimeController != nil {
				t.Fatal("partial cleanup did not release exactly acquired topics")
			}
		})
	}
	for n := 1; n <= count; n++ {
		hub := &lifecycleFailureHub{Hub: magicevent.NewHub(16), failUnsubscribe: n, failure: cd.NewError(cd.Unexpected, "unsubscribe rejected")}
		b := lifecycleOwner(t, hub)
		if err := b.subscribeRuntimeIngress(); err != nil {
			t.Fatal(err)
		}
		if err := b.Stop(ctx); !errors.Is(err, hub.failure) {
			t.Fatalf("Teardown error = %v", err)
		}
		if len(b.subscriptions) != count-n+1 || b.runtimeController == nil {
			t.Fatal("failed cleanup lost remaining resources")
		}
		hub.failUnsubscribe = 0
		if err := b.Stop(ctx); err != nil {
			t.Fatal(err)
		}
		if len(b.subscriptions) != 0 || b.runtimeController != nil {
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
	t.Cleanup(func() { unblock(); _ = b.Stop(ctx); hub.Terminate(ctx) })
	if err := b.subscribeOwner(events.TopicOrchestratorDriveRuntime, func(_ magicevent.Event, result magicevent.Result) {
		close(entered)
		<-release
		result.Set(struct{}{}, nil)
		close(exited)
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := b.subscribeOwner(events.TopicOrchestratorCommandListToolCallsByRun, func(_ magicevent.Event, result magicevent.Result) { result.Set(struct{}{}, nil) }, false); err != nil {
		t.Fatal(err)
	}
	callCtx, cancel := context.WithCancel(ctx)
	sent := make(chan struct{})
	go func() {
		defer close(sent)
		hub.Send(lifecycleEvent(callCtx, events.TopicOrchestratorDriveRuntime, "active-lane"))
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
	if err := b.Stop(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Teardown error = %v", err)
	}
	if b.runtimeController == nil || len(b.subscriptions) != 2 {
		t.Fatal("timeout released resources before actual exit")
	}
	queryCtx, queryCancel := context.WithTimeout(ctx, time.Second)
	defer queryCancel()
	query := hub.Send(lifecycleEvent(queryCtx, events.TopicOrchestratorCommandListToolCallsByRun, "query-lane"))
	if err := query.Error(); err != nil {
		t.Fatalf("drain query = %v", err)
	}
	fresh := hub.Send(lifecycleEvent(queryCtx, events.TopicOrchestratorDriveRuntime, "fresh-lane"))
	if err := fresh.Error(); !errors.Is(err, basebiz.ErrLifecycleClosing) {
		t.Fatalf("fresh mutation = %v", err)
	}
	if _, err := b.enterOwner(false); !errors.Is(err, basebiz.ErrLifecycleClosing) {
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
	if err := b.Stop(retryCtx); err != nil {
		t.Fatal(err)
	}
	if _, err := b.enterOwner(true); !errors.Is(err, basebiz.ErrLifecycleClosing) {
		t.Fatalf("released drain = %v", err)
	}
}

func TestOwnerLifecycleAcceptedQueueAndSubmissionFailure(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	defer hub.Terminate(ctx)
	b := lifecycleOwner(t, hub)
	failure := errors.New("submission rejected")
	if err := b.scheduleOwnerWork(func(func()) error { return failure }, func() { t.Error("rejected callback ran") }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	var queued func()
	entered, release, exited := make(chan struct{}), make(chan struct{}), make(chan struct{})
	if err := b.scheduleOwnerWork(func(callback func()) error { queued = callback; return nil }, func() { close(entered); <-release }); err != nil {
		t.Fatal(err)
	}
	b.BeginShutdown(ctx)
	short, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	if err := b.Stop(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued Stop=%v", err)
	}
	cancel()
	if b.runtimeController == nil {
		t.Fatal("queued work lost dependencies")
	}
	go func() { defer close(exited); queued() }()
	<-entered
	short, cancel = context.WithTimeout(ctx, 10*time.Millisecond)
	if err := b.Stop(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("active callback Stop=%v", err)
	}
	cancel()
	close(release)
	<-exited
	if err := b.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.scheduleOwnerWork(func(func()) error { t.Error("released owner submitted work"); return nil }, func() {}); !errors.Is(err, basebiz.ErrLifecycleClosing) {
		t.Fatal(err)
	}
}

func TestOwnerLifecycleUnknownPanicRetainsRetryableLedger(t *testing.T) {
	ctx := context.Background()
	hub := &lifecycleFailureHub{Hub: magicevent.NewHub(16), panicUnsubscribe: true}
	defer hub.Terminate(ctx)
	b := lifecycleOwner(t, hub)
	if err := b.subscribeRuntimeIngress(); err != nil {
		t.Fatal(err)
	}
	count := len(b.subscriptions)
	func() {
		defer func() {
			if recover() != "unexpected unsubscribe panic" {
				t.Fatal("unknown panic was swallowed or changed")
			}
		}()
		_ = b.Stop(ctx)
	}()
	if b.releasing || b.runtimeController == nil || len(b.subscriptions) != count {
		t.Fatal("panic poisoned cleanup claim or lost ledger")
	}
	hub.panicUnsubscribe = false
	if err := b.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

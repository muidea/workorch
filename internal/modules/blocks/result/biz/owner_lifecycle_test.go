package result

import (
	"context"
	"errors"
	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	events "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	"sync/atomic"

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
	b, err := New(hub, nil, Dependencies{EventHub: hub, OwnerRepositories: memoryrepo.NewResultOwnerRepositorySet().OwnerRepositories()})
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
	count := len(probe.topics)
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
			if b.started || len(b.topics) != n-1 || b.service == nil {
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
			if hub.unsubscribeCalls != n-1 || b.service != nil {
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
		if len(b.topics) != count-n+1 || b.service == nil {
			t.Fatal("failed cleanup lost remaining resources")
		}
		hub.failUnsubscribe = 0
		if err := b.Teardown(ctx); err != nil {
			t.Fatal(err)
		}
		if len(b.topics) != 0 || b.service != nil {
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
	if err := b.subscribeOwner(events.TopicResultCommandSealWorkspaceDeletion, func(_ magicevent.Event, result magicevent.Result) {
		close(entered)
		<-release
		result.Set(struct{}{}, nil)
		close(exited)
	}); err != nil {
		t.Fatal(err)
	}
	if err := b.subscribeOwner(events.TopicResultCommandGet, func(_ magicevent.Event, result magicevent.Result) { result.Set(struct{}{}, nil) }); err != nil {
		t.Fatal(err)
	}
	callCtx, cancel := context.WithCancel(ctx)
	sent := make(chan struct{})
	go func() {
		defer close(sent)
		hub.Send(lifecycleEvent(callCtx, events.TopicResultCommandSealWorkspaceDeletion, "active-lane"))
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
	if b.service == nil || len(b.topics) != 2 {
		t.Fatal("timeout released resources before actual exit")
	}
	queryCtx, queryCancel := context.WithTimeout(ctx, time.Second)
	defer queryCancel()
	query := hub.Send(lifecycleEvent(queryCtx, events.TopicResultCommandGet, "query-lane"))
	if err := query.Error(); err != nil {
		t.Fatalf("drain query = %v", err)
	}
	fresh := hub.Send(lifecycleEvent(queryCtx, events.TopicResultCommandSealWorkspaceDeletion, "fresh-lane"))
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
func TestOwnerLifecycleUnknownPanicRetainsRetryableLedger(t *testing.T) {
	ctx := context.Background()
	hub := &lifecycleFailureHub{Hub: magicevent.NewHub(16), panicUnsubscribe: true}
	defer hub.Terminate(ctx)
	b := lifecycleOwner(t, hub)
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	count := len(b.topics)
	func() {
		defer func() {
			if recover() != "unexpected unsubscribe panic" {
				t.Fatal("unknown panic was swallowed or changed")
			}
		}()
		_ = b.Teardown(ctx)
	}()
	if b.releasing || b.service == nil || len(b.topics) != count {
		t.Fatal("panic poisoned cleanup claim or lost ledger")
	}
	hub.panicUnsubscribe = false
	if err := b.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
}

type capturedRecoveryTimer struct {
	task.BackgroundRoutine
	callback task.Task
	ctx      context.Context
}

func (b *capturedRecoveryTimer) Timer(ctx context.Context, callback task.Task, _, _ time.Duration) error {
	b.ctx, b.callback = ctx, callback
	return nil
}

type blockedRecoveryRepository struct {
	repository.ResultRepository
	entered, release chan struct{}
	calls            atomic.Int32
}

func (r *blockedRecoveryRepository) ListForRecovery(context.Context, string, int) ([]*resultmodel.Result, string, error) {
	if r.calls.Add(1) == 1 {
		return nil, "", nil
	}
	close(r.entered)
	<-r.release
	return nil, "", nil
}

func TestOwnerLifecycleWaitsForActualRecoveryCallbackAndRejectsQueuedReplay(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	b := lifecycleOwner(t, hub)
	background := &capturedRecoveryTimer{}
	b.Base = basebiz.New(ModuleID, hub, background)
	repo := &blockedRecoveryRepository{ResultRepository: b.service.results, entered: make(chan struct{}), release: make(chan struct{})}
	b.service.results = repo
	var once sync.Once
	unblock := func() { once.Do(func() { close(repo.release) }) }
	t.Cleanup(func() { unblock(); _ = b.Teardown(ctx); hub.Terminate(ctx) })
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	if background.callback == nil {
		t.Fatal("recovery callback not registered")
	}
	done := make(chan struct{})
	go func() { defer close(done); background.callback.Run() }()
	select {
	case <-repo.entered:
	case <-time.After(time.Second):
		t.Fatal("recovery did not enter")
	}
	b.BeginShutdown(ctx)
	if !errors.Is(background.ctx.Err(), context.Canceled) {
		t.Fatal("recovery scheduling context not canceled")
	}
	short, stop := context.WithTimeout(ctx, 10*time.Millisecond)
	defer stop()
	if err := b.Teardown(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("recovery drain = %v", err)
	}
	if b.service == nil || b.notificationRecoveryCancel == nil || len(b.topics) != 10 {
		t.Fatal("timeout forgot recovery dependencies")
	}
	// 已排队但未执行的 callback 仍可能到达，关闭后不能再进入 repository。
	background.callback.Run()
	if repo.calls.Load() != 2 {
		t.Fatal("closing queued callback re-entered repository")
	}
	unblock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("recovery did not return")
	}
	if err := b.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
	background.callback.Run()
	if repo.calls.Load() != 2 {
		t.Fatal("released callback touched repository")
	}
}

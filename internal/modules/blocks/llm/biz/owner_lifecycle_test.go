package llm

import (
	"context"
	"errors"
	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	events "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchconfig"
	"io"
	"net/http"
	"strings"
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
	configured := workorchconfig.DefaultLLMConfig()
	configured.AIProxy.BaseURL = "http://127.0.0.1:8090"
	configured.DefaultChat = workorchconfig.ProviderModelTargetConfig{ProviderID: workorchconfig.BuiltinAIProxyProviderID, ModelID: "test-model"}
	b, err := New(hub, nil, Dependencies{LoadLLMConfig: func() (workorchconfig.LLMConfig, error) { return configured, nil }})
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
			if b.started || len(b.subscriptions) != n-1 || b.catalogService == nil {
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
			if err := b.Teardown(ctx); err != nil {
				t.Fatal(err)
			}
			if hub.unsubscribeCalls != n-1 || b.catalogService != nil {
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
		if len(b.subscriptions) != count-n+1 || b.catalogService == nil {
			t.Fatal("failed cleanup lost remaining resources")
		}
		hub.failUnsubscribe = 0
		if err := b.Teardown(ctx); err != nil {
			t.Fatal(err)
		}
		if len(b.subscriptions) != 0 || b.catalogService != nil {
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
	if err := b.subscribeOwner(events.TopicAcquireRuntimeCapabilities, func(_ magicevent.Event, result magicevent.Result) {
		close(entered)
		<-release
		result.Set(struct{}{}, nil)
		close(exited)
	}, false); err != nil {
		t.Fatal(err)
	}
	if err := b.subscribeOwner(events.TopicLLMCommandGetActiveConfiguration, func(_ magicevent.Event, result magicevent.Result) { result.Set(struct{}{}, nil) }, false); err != nil {
		t.Fatal(err)
	}
	callCtx, cancel := context.WithCancel(ctx)
	sent := make(chan struct{})
	go func() {
		defer close(sent)
		hub.Send(lifecycleEvent(callCtx, events.TopicAcquireRuntimeCapabilities, "active-lane"))
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
	if b.catalogService == nil || len(b.subscriptions) != 2 {
		t.Fatal("timeout released resources before actual exit")
	}
	queryCtx, queryCancel := context.WithTimeout(ctx, time.Second)
	defer queryCancel()
	query := hub.Send(lifecycleEvent(queryCtx, events.TopicLLMCommandGetActiveConfiguration, "query-lane"))
	if err := query.Error(); err != nil {
		t.Fatalf("drain query = %v", err)
	}
	fresh := hub.Send(lifecycleEvent(queryCtx, events.TopicAcquireRuntimeCapabilities, "fresh-lane"))
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
	count := len(b.subscriptions)
	func() {
		defer func() {
			if recover() != "unexpected unsubscribe panic" {
				t.Fatal("unknown panic was swallowed or changed")
			}
		}()
		_ = b.Teardown(ctx)
	}()
	if b.releasing || b.catalogService == nil || len(b.subscriptions) != count {
		t.Fatal("panic poisoned cleanup claim or lost ledger")
	}
	hub.panicUnsubscribe = false
	if err := b.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
}

type lifecycleRoundTripper func(*http.Request) (*http.Response, error)

func (f lifecycleRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestOwnerLifecycleWaitsForActualCatalogTransportExit(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	b := lifecycleOwner(t, hub)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var calls atomic.Int32
	b.catalogService.client = &http.Client{Transport: lifecycleRoundTripper(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		close(entered)
		<-release
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":[]}`)), Request: req}, nil
	})}
	t.Cleanup(func() { unblock(); _ = b.Teardown(ctx); hub.Terminate(ctx) })
	if err := b.Start(); err != nil {
		t.Fatal(err)
	}
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sent := make(chan struct{})
	go func() {
		defer close(sent)
		ev := magicevent.NewEventWithContext(events.TopicLLMCommandGetProviderModelCatalog, llmApplicationOwnerSource, events.CommandDestination, nil, callCtx, events.GetProviderModelCatalogCommand{})
		ev.BindLaneKey("catalog-active")
		hub.Send(ev)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("catalog transport did not enter")
	}
	cancel()
	b.BeginShutdown(ctx)
	short, stop := context.WithTimeout(ctx, 10*time.Millisecond)
	defer stop()
	if err := b.Teardown(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("actual transport drain = %v", err)
	}
	if b.catalogService == nil || !b.activeLLMConfigurationInitialized || len(b.subscriptions) != 11 {
		t.Fatal("timeout released live transport dependencies")
	}
	query := magicevent.NewEventWithContext(events.TopicLLMCommandGetActiveConfiguration, llmApplicationOwnerSource, events.CommandDestination, nil, ctx, events.GetActiveConfigurationCommand{})
	query.BindLaneKey("config-drain")
	if err := hub.Send(query).Error(); err != nil {
		t.Fatalf("frozen configuration drain = %v", err)
	}
	fresh := magicevent.NewEventWithContext(events.TopicLLMCommandGetProviderModelCatalog, llmApplicationOwnerSource, events.CommandDestination, nil, ctx, events.GetProviderModelCatalogCommand{})
	fresh.BindLaneKey("catalog-fresh")
	if err := hub.Send(fresh).Error(); !errors.Is(err, basebiz.ErrLifecycleClosing) {
		t.Fatalf("new catalog refresh = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatal("shutdown admitted a second physical request")
	}
	select {
	case <-sent:
		t.Fatal("cancellation fabricated actual transport completion")
	default:
	}
	unblock()
	select {
	case <-sent:
	case <-time.After(time.Second):
		t.Fatal("transport did not return")
	}
	if err := b.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
}

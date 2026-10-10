package mcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	frameworkdef "github.com/muidea/magicCommon/def"
	"log/slog"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	modulecommon "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/common"
	mcpevents "github.com/muidea/workorch/internal/modules/blocks/mcp/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type mcpLifecycleHub struct {
	magicevent.Hub
	subscriptions   atomic.Int32
	unsubscriptions atomic.Int32
}

func (h *mcpLifecycleHub) Subscribe(id string, observer magicevent.Observer) *frameworkdef.Error {
	if id == mcpevents.TopicAcquireRuntimeCapabilities {
		h.subscriptions.Add(1)
	}
	return h.Hub.Subscribe(id, observer)
}

func (h *mcpLifecycleHub) Unsubscribe(id string, observer magicevent.Observer) *frameworkdef.Error {
	if id == mcpevents.TopicAcquireRuntimeCapabilities {
		h.unsubscriptions.Add(1)
	}
	return h.Hub.Unsubscribe(id, observer)
}

type mcpLifecycleEvent struct {
	magicevent.Event
	dataReads atomic.Int32
}

func (e *mcpLifecycleEvent) Data() any {
	e.dataReads.Add(1)
	return e.Event.Data()
}

func TestMCPOwnerAcquiresCapabilitiesThroughTypedEventHubContract(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	request := runtimecontract.RuntimeCapabilityAcquisitionRequest{
		WorkspaceID: "ws-mcp-capability", RunID: "run-mcp-capability", AuthoritySnapshotID: "authority-mcp-capability",
		PrerequisiteID: "prerequisite-mcp-capability", OwnerModuleID: ModuleID,
		CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredKinds: []string{modulecommon.StepKindResourceRead},
	}
	result := hub.Send(magicevent.NewEventWithContext(
		mcpevents.TopicAcquireRuntimeCapabilities, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, mcpevents.AcquireRuntimeCapabilitiesCommand(request),
	))
	data, getErr := result.Get()
	if getErr != nil {
		t.Fatal(getErr)
	}
	response, ok := data.(mcpevents.AcquireRuntimeCapabilitiesResult)
	if !ok {
		t.Fatalf("capability response type = %T", data)
	}
	if err := response.Acquisition.ValidateForRequest(request); err != nil {
		t.Fatalf("validate MCP capability acquisition: %v", err)
	}
}

func TestMCPOwnerLifecycleFencesSubscriptionsAndCapabilityDelivery(t *testing.T) {
	ctx := context.Background()
	hub := &mcpLifecycleHub{Hub: magicevent.NewHub(8)}
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	if !owner.started {
		t.Fatal("MCP owner did not enter started lifecycle state")
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	if got := hub.subscriptions.Load(); got != 1 {
		t.Fatalf("capability subscriptions after duplicate Start = %d, want 1", got)
	}
	if strings.Contains(logs.String(), "duplicate eventID") {
		t.Fatalf("duplicate Start reached SimpleObserver registration: %s", logs.String())
	}

	request := runtimecontract.RuntimeCapabilityAcquisitionRequest{
		WorkspaceID: "ws-mcp-lifecycle", RunID: "run-mcp-lifecycle", AuthoritySnapshotID: "authority-mcp-lifecycle",
		PrerequisiteID: "prerequisite-mcp-lifecycle", OwnerModuleID: ModuleID,
		CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredKinds: []string{modulecommon.StepKindResourceRead},
	}
	newEvent := func() *mcpLifecycleEvent {
		return &mcpLifecycleEvent{Event: magicevent.NewEventWithContext(
			mcpevents.TopicAcquireRuntimeCapabilities, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
			magicevent.NewHeader(), ctx, mcpevents.AcquireRuntimeCapabilitiesCommand(request),
		)}
	}
	delivered := newEvent()
	result := hub.Send(delivered)
	if result == nil || result.Error() != nil || delivered.dataReads.Load() != 1 {
		t.Fatalf("started delivery result=%v reads=%d, want one successful delivery", result, delivered.dataReads.Load())
	}

	owner.Teardown(ctx)
	if owner.started {
		t.Fatal("MCP owner remained started after Teardown")
	}
	stopped := newEvent()
	hub.Send(stopped)
	if got := stopped.dataReads.Load(); got != 0 {
		t.Fatalf("post-Stop delivery reads = %d, want 0", got)
	}
	owner.Teardown(ctx)
	if got := hub.unsubscriptions.Load(); got != 1 {
		t.Fatalf("capability unsubscriptions after duplicate Teardown = %d, want 1", got)
	}
	if strings.Contains(logs.String(), "not exist eventID") {
		t.Fatalf("duplicate Teardown reached SimpleObserver unregistration: %s", logs.String())
	}

	restarted, err := New(hub, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Start(); err != nil {
		t.Fatal(err)
	}
	restartedEvent := newEvent()
	result = hub.Send(restartedEvent)
	if result == nil || result.Error() != nil || restartedEvent.dataReads.Load() != 1 {
		t.Fatalf("restart delivery result=%v reads=%d, want one successful delivery", result, restartedEvent.dataReads.Load())
	}
	if got := hub.subscriptions.Load(); got != 2 {
		t.Fatalf("capability subscriptions after fresh-owner restart = %d, want 2", got)
	}
	restarted.Teardown(ctx)
}

// Faults wrap the real Hub: Base still owns its real SimpleObserver and both
// successful subscribe/unsubscribe calls wait for the real control receipt.
type resourceLedgerHub struct {
	magicevent.Hub
	subscribeCalls, unsubscribeCalls int
	failSubscribe, failUnsubscribe   int
	failure                          *frameworkdef.Error
	cancelAfterUnsubscribe           context.CancelFunc
}

func (h *resourceLedgerHub) Subscribe(topic string, observer magicevent.Observer) *frameworkdef.Error {
	h.subscribeCalls++
	if h.subscribeCalls == h.failSubscribe {
		return h.failure
	}
	return h.Hub.Subscribe(topic, observer)
}

func (h *resourceLedgerHub) Unsubscribe(topic string, observer magicevent.Observer) *frameworkdef.Error {
	h.unsubscribeCalls++
	if h.unsubscribeCalls == h.failUnsubscribe {
		return h.failure
	}
	err := h.Hub.Unsubscribe(topic, observer)
	if err == nil && h.cancelAfterUnsubscribe != nil {
		h.cancelAfterUnsubscribe()
	}
	return err
}

func newResourceLedgerOwner(hub magicevent.Hub) *Biz {
	return &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter(nil)}
}

func TestMCPOwnerResourceLedgerEveryStartFailureCanTeardown(t *testing.T) {
	for n := 1; n <= 6; n++ {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			ctx := context.Background()
			failure := frameworkdef.NewError(frameworkdef.ResourceExhausted, "required subscription failed")
			hub := &resourceLedgerHub{Hub: magicevent.NewHub(8), failSubscribe: n, failure: failure}
			t.Cleanup(func() { hub.Terminate(ctx) })
			owner := newResourceLedgerOwner(hub)
			adapter := owner.runtimeAdapter
			if err := owner.Start(); err != failure {
				t.Fatalf("Start error = %v, want exact checked failure", err)
			}
			if owner.started || len(owner.subscribedTopics) != n-1 || owner.runtimeAdapter != adapter {
				t.Fatalf("partial Start lost resource accounting: started=%t topics=%v adapter=%p", owner.started, owner.subscribedTopics, owner.runtimeAdapter)
			}
			wantReads := int32(0)
			if n > 1 {
				wantReads = 1
			}
			if got := resourceLedgerCapabilityReads(hub); got != wantReads {
				t.Fatalf("partial Start capability reads = %d, want %d", got, wantReads)
			}
			if err := owner.Teardown(ctx); err != nil {
				t.Fatal(err)
			}
			if got := resourceLedgerCapabilityReads(hub); got != 0 {
				t.Fatalf("cleanup left live subscription: reads=%d", got)
			}
			if hub.unsubscribeCalls != n-1 || len(owner.subscribedTopics) != 0 || owner.runtimeAdapter != nil {
				t.Fatalf("partial Start cleanup = calls %d topics %v adapter %p", hub.unsubscribeCalls, owner.subscribedTopics, owner.runtimeAdapter)
			}
			if err := owner.Teardown(ctx); err != nil {
				t.Fatal(err)
			}
			if got := resourceLedgerCapabilityReads(hub); got != 0 {
				t.Fatalf("cleanup left live subscription: reads=%d", got)
			}
			if hub.unsubscribeCalls != n-1 {
				t.Fatal("duplicate teardown repeated resource release")
			}
		})
	}
}

func TestMCPOwnerResourceLedgerStartRetrySkipsCompletedTopics(t *testing.T) {
	ctx := context.Background()
	failure := frameworkdef.NewError(frameworkdef.ResourceExhausted, "required subscription failed")
	hub := &resourceLedgerHub{Hub: magicevent.NewHub(8), failSubscribe: 3, failure: failure}
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := newResourceLedgerOwner(hub)
	if err := owner.Start(); err != failure {
		t.Fatalf("Start error = %v", err)
	}
	prefix := append([]string(nil), owner.subscribedTopics...)
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	if !owner.started || hub.subscribeCalls != 6+1 || !reflect.DeepEqual(prefix, owner.subscribedTopics[:len(prefix)]) {
		t.Fatalf("Start retry duplicated successful topics: calls=%d topics=%v", hub.subscribeCalls, owner.subscribedTopics)
	}
	if err := owner.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestMCPOwnerResourceLedgerEveryTeardownFailureRetainsDependenciesAndRetries(t *testing.T) {
	for n := 1; n <= 6; n++ {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			ctx := context.Background()
			failure := frameworkdef.NewError(frameworkdef.ResourceExhausted, "required unsubscribe failed")
			hub := &resourceLedgerHub{Hub: magicevent.NewHub(8), failUnsubscribe: n, failure: failure}
			t.Cleanup(func() { hub.Terminate(ctx) })
			owner := newResourceLedgerOwner(hub)
			if err := owner.Start(); err != nil {
				t.Fatal(err)
			}
			adapter := owner.runtimeAdapter
			topics := append([]string(nil), owner.subscribedTopics...)
			if err := owner.Teardown(ctx); err != failure {
				t.Fatalf("Teardown error = %v, want exact checked failure", err)
			}
			if got := resourceLedgerCapabilityReads(hub); got != 1 {
				t.Fatalf("failed unsubscribe lost remaining handler: reads=%d", got)
			}
			remaining := 6 - n + 1
			if !reflect.DeepEqual(owner.subscribedTopics, topics[:remaining]) || owner.runtimeAdapter != adapter {
				t.Fatalf("failed cleanup discarded live resources: topics=%v adapter=%p", owner.subscribedTopics, owner.runtimeAdapter)
			}
			if err := owner.Teardown(ctx); err != nil {
				t.Fatal(err)
			}
			if hub.unsubscribeCalls != 6+1 || owner.runtimeAdapter != nil || owner.started || len(owner.subscribedTopics) != 0 {
				t.Fatalf("retry did not finish remaining resources: calls=%d topics=%v adapter=%p", hub.unsubscribeCalls, owner.subscribedTopics, owner.runtimeAdapter)
			}
			if err := owner.Teardown(ctx); err != nil {
				t.Fatal(err)
			}
			if hub.unsubscribeCalls != 6+1 {
				t.Fatal("completed resources released again")
			}
		})
	}
}

func TestMCPOwnerResourceLedgerCancellationRetainsUnreleasedResources(t *testing.T) {
	hub := &resourceLedgerHub{Hub: magicevent.NewHub(8)}
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	owner := newResourceLedgerOwner(hub)
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	adapter := owner.runtimeAdapter
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := owner.Teardown(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled teardown = %v", err)
	}
	if hub.unsubscribeCalls != 0 || owner.runtimeAdapter != adapter {
		t.Fatal("pre-canceled cleanup released dependencies")
	}
	interrupted, interrupt := context.WithCancel(context.Background())
	defer interrupt()
	hub.cancelAfterUnsubscribe = interrupt
	if err := owner.Teardown(interrupted); !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted teardown = %v", err)
	}
	if hub.unsubscribeCalls != 1 || len(owner.subscribedTopics) != 6-1 || owner.runtimeAdapter != adapter {
		t.Fatal("cancellation after completion lost exact resource receipt")
	}
	hub.cancelAfterUnsubscribe = nil
	if err := owner.Teardown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hub.unsubscribeCalls != 6 {
		t.Fatal("retry repeated completed unsubscribe")
	}
}

func TestMCPOwnerResourceLedgerUnknownPanicRemainsVisible(t *testing.T) {
	marker := &struct{}{}
	defer func() {
		if value := recover(); value != marker {
			t.Fatalf("panic = %v, want exact unknown panic", value)
		}
	}()
	_ = checkedLifecycleSubscription(func() { panic(marker) })
}

type resourceLedgerSessionInput struct{ closes atomic.Int32 }

func (i *resourceLedgerSessionInput) Write(data []byte) (int, error) { return len(data), nil }
func (i *resourceLedgerSessionInput) Close() error                   { i.closes.Add(1); return nil }

func TestMCPOwnerResourceLedgerClosesAdapterOnlyAfterCompleteUnsubscribeOnce(t *testing.T) {
	ctx := context.Background()
	hub := &resourceLedgerHub{Hub: magicevent.NewHub(8), failUnsubscribe: 2, failure: frameworkdef.NewError(frameworkdef.ResourceExhausted, "unsubscribe failure")}
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := newResourceLedgerOwner(hub)
	adapter := owner.runtimeAdapter
	input := &resourceLedgerSessionInput{}
	done := make(chan error)
	close(done)
	session := &stdioSession{stdin: input, done: done}
	adapter.sessions["resource-ledger"] = session
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	if err := owner.Teardown(ctx); err != hub.failure {
		t.Fatalf("Teardown error = %v", err)
	}
	if input.closes.Load() != 0 || adapter.sessions["resource-ledger"] != session || owner.runtimeAdapter != adapter {
		t.Fatal("failed unsubscribe closed live adapter/session")
	}
	if err := owner.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
	if input.closes.Load() != 1 || len(adapter.sessions) != 0 || owner.runtimeAdapter != nil {
		t.Fatal("successful unsubscribe did not close adapter/session exactly once")
	}
	// A sentinel inserted into the detached adapter detects any second Close,
	// independently of stdioSession's own closeOnce protection.
	sentinelInput := &resourceLedgerSessionInput{}
	adapter.sessions["detached-sentinel"] = &stdioSession{stdin: sentinelInput, done: done}
	if err := owner.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
	if sentinelInput.closes.Load() != 0 || input.closes.Load() != 1 {
		t.Fatal("duplicate teardown called detached adapter Close")
	}
	adapter.Close()
}

func resourceLedgerCapabilityReads(hub magicevent.Hub) int32 {
	ev := &mcpLifecycleEvent{Event: magicevent.NewEventWithContext(
		mcpevents.TopicAcquireRuntimeCapabilities, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), context.Background(), nil,
	)}
	hub.Send(ev)
	return ev.dataReads.Load()
}

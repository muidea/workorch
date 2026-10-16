package shell

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
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	shellevents "github.com/muidea/workorch/internal/modules/blocks/shell/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type shellLifecycleHub struct {
	magicevent.Hub
	subscriptions   atomic.Int32
	unsubscriptions atomic.Int32
}

func (h *shellLifecycleHub) Subscribe(id string, observer magicevent.Observer) *frameworkdef.Error {
	if id == shellevents.TopicAcquireRuntimeCapabilities {
		h.subscriptions.Add(1)
	}
	return h.Hub.Subscribe(id, observer)
}

func (h *shellLifecycleHub) Unsubscribe(id string, observer magicevent.Observer) *frameworkdef.Error {
	if id == shellevents.TopicAcquireRuntimeCapabilities {
		h.unsubscriptions.Add(1)
	}
	return h.Hub.Unsubscribe(id, observer)
}

type shellLifecycleEvent struct {
	magicevent.Event
	dataReads atomic.Int32
}

func (e *shellLifecycleEvent) Data() any {
	e.dataReads.Add(1)
	return e.Event.Data()
}

func TestShellOwnerAcquiresManagedValidationCapabilityThroughTypedEventHubContract(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Teardown(ctx) })

	request := runtimecontract.RuntimeCapabilityAcquisitionRequest{
		WorkspaceID: "ws-shell-capability", RunID: "run-shell-capability", AuthoritySnapshotID: "authority-shell-capability",
		PrerequisiteID: "prerequisite-shell-capability", OwnerModuleID: ModuleID,
		CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	result := hub.Send(magicevent.NewEventWithContext(
		shellevents.TopicAcquireRuntimeCapabilities, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), ctx, shellevents.AcquireRuntimeCapabilitiesCommand(request),
	))
	data, err := result.Get()
	if err != nil {
		t.Fatal(err)
	}
	response, ok := data.(shellevents.AcquireRuntimeCapabilitiesResult)
	if !ok {
		t.Fatalf("capability response type = %T", data)
	}
	if err := response.Acquisition.ValidateForRequest(request); err != nil {
		t.Fatalf("validate Shell capability acquisition: %v", err)
	}
	if facts := response.Acquisition.Capabilities.PrerequisiteFacts; len(facts) != 1 || facts[0].Requirement.OwnerModuleID != runtimecontract.RuntimeModuleFile {
		t.Fatalf("managed-validation prerequisite facts = %+v, want one File-owner manifest fact", facts)
	}
}

func TestShellOwnerLifecycleFencesSubscriptionsAndCapabilityDelivery(t *testing.T) {
	ctx := context.Background()
	hub := &shellLifecycleHub{Hub: magicevent.NewHub(8)}
	t.Cleanup(func() { hub.Terminate(ctx) })
	owner := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}

	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	if !owner.started {
		t.Fatal("Shell owner did not enter started lifecycle state")
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
		WorkspaceID: "ws-shell-lifecycle", RunID: "run-shell-lifecycle", AuthoritySnapshotID: "authority-shell-lifecycle",
		PrerequisiteID: "prerequisite-shell-lifecycle", OwnerModuleID: ModuleID,
		CapabilityClass: runtimecontract.RuntimeCapabilityClassValidation, RequiredKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
	}
	newEvent := func() *shellLifecycleEvent {
		return &shellLifecycleEvent{Event: magicevent.NewEventWithContext(
			shellevents.TopicAcquireRuntimeCapabilities, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
			magicevent.NewHeader(), ctx, shellevents.AcquireRuntimeCapabilitiesCommand(request),
		)}
	}
	delivered := newEvent()
	result := hub.Send(delivered)
	if result == nil || result.Error() != nil || delivered.dataReads.Load() != 1 {
		t.Fatalf("started delivery result=%v reads=%d, want one successful delivery", result, delivered.dataReads.Load())
	}

	owner.Teardown(ctx)
	if owner.started {
		t.Fatal("Shell owner remained started after Teardown")
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

	restarted := &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
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
	return &Biz{Base: basebiz.New(ModuleID, hub, nil), runtimeAdapter: NewRuntimeAdapter()}
}

func TestShellOwnerResourceLedgerEveryStartFailureCanTeardown(t *testing.T) {
	for n := 1; n <= 9; n++ {
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

func TestShellOwnerResourceLedgerStartRetrySkipsCompletedTopics(t *testing.T) {
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
	if !owner.started || hub.subscribeCalls != 9+1 || !reflect.DeepEqual(prefix, owner.subscribedTopics[:len(prefix)]) {
		t.Fatalf("Start retry duplicated successful topics: calls=%d topics=%v", hub.subscribeCalls, owner.subscribedTopics)
	}
	if err := owner.Teardown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestShellOwnerResourceLedgerEveryTeardownFailureRetainsDependenciesAndRetries(t *testing.T) {
	for n := 1; n <= 9; n++ {
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
			if got := resourceLedgerCapabilityReads(hub); got != 0 {
				t.Fatalf("停机后仍接受 fresh capability query: reads=%d", got)
			}
			remaining := 9 - n + 1
			if !reflect.DeepEqual(owner.subscribedTopics, topics[:remaining]) || owner.runtimeAdapter != adapter {
				t.Fatalf("failed cleanup discarded live resources: topics=%v adapter=%p", owner.subscribedTopics, owner.runtimeAdapter)
			}
			if err := owner.Teardown(ctx); err != nil {
				t.Fatal(err)
			}
			if hub.unsubscribeCalls != 9+1 || owner.runtimeAdapter != nil || owner.started || len(owner.subscribedTopics) != 0 {
				t.Fatalf("retry did not finish remaining resources: calls=%d topics=%v adapter=%p", hub.unsubscribeCalls, owner.subscribedTopics, owner.runtimeAdapter)
			}
			if err := owner.Teardown(ctx); err != nil {
				t.Fatal(err)
			}
			if hub.unsubscribeCalls != 9+1 {
				t.Fatal("completed resources released again")
			}
		})
	}
}

func TestShellOwnerResourceLedgerCancellationRetainsUnreleasedResources(t *testing.T) {
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
	if hub.unsubscribeCalls != 1 || len(owner.subscribedTopics) != 9-1 || owner.runtimeAdapter != adapter {
		t.Fatal("cancellation after completion lost exact resource receipt")
	}
	hub.cancelAfterUnsubscribe = nil
	if err := owner.Teardown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hub.unsubscribeCalls != 9 {
		t.Fatal("retry repeated completed unsubscribe")
	}
}

func resourceLedgerCapabilityReads(hub magicevent.Hub) int32 {
	ev := &shellLifecycleEvent{Event: magicevent.NewEventWithContext(
		shellevents.TopicAcquireRuntimeCapabilities, runtimecontract.RuntimeModuleOrchestrator, ModuleID,
		magicevent.NewHeader(), context.Background(), nil,
	)}
	hub.Send(ev)
	return ev.dataReads.Load()
}

func TestShellOwnerQuiescenceWaitsForRealHandlerAndRetainsRetryDependencies(t *testing.T) {
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	owner := newResourceLedgerOwner(hub)
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	adapter := owner.runtimeAdapter
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	handler := owner.lifecycleHandler(func(ev magicevent.Event, result magicevent.Result) {
		close(entered)
		<-release
		result.Set(nil, nil)
	}, false)
	callCtx, cancelCall := context.WithCancel(context.Background())
	ev := magicevent.NewEventWithContext("quiescence-test", runtimecontract.RuntimeModuleOrchestrator, ModuleID, nil, callCtx, nil)
	go func() { defer close(done); handler(ev, magicevent.NewResult(ev.ID(), ev.Source(), ev.Destination())) }()
	<-entered
	cancelCall()
	if err := owner.BeginShutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	budget, cancelBudget := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancelBudget()
	if err := owner.Quiesce(budget); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Quiesce = %v, want timeout while canceled handler still runs", err)
	}
	if err := owner.Teardown(budget); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Teardown = %v", err)
	}
	if owner.runtimeAdapter != adapter || len(owner.subscribedTopics) == 0 {
		t.Fatal("timeout released dependencies")
	}
	var freshCalls int
	fresh := owner.lifecycleHandler(func(magicevent.Event, magicevent.Result) { freshCalls++ }, false)
	result := magicevent.NewResult(ev.ID(), ev.Source(), ev.Destination())
	fresh(ev, result)
	if freshCalls != 0 || result.Error() == nil {
		t.Fatal("BeginShutdown accepted new root")
	}
	if err := owner.Start(); !errors.Is(err, basebiz.ErrLifecycleClosing) {
		t.Fatalf("Start reopened shutdown generation: %v", err)
	}
	close(release)
	<-done
	if err := owner.Quiesce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := owner.Teardown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestShellOwnerQuiescenceExplicitReceiptDrainStillWaitsForCompletion(t *testing.T) {
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	owner := newResourceLedgerOwner(hub)
	if err := owner.BeginShutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	receipt := owner.lifecycleHandler(func(magicevent.Event, magicevent.Result) {
		close(entered)
		<-release
	}, true)
	ev := magicevent.NewEvent("receipt-drain-test", runtimecontract.RuntimeModuleOrchestrator, ModuleID, nil, nil)
	go func() { defer close(done); receipt(ev, magicevent.NewResult(ev.ID(), ev.Source(), ev.Destination())) }()
	<-entered
	budget, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	if err := owner.Quiesce(budget); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("drain not counted: %v", err)
	}
	close(release)
	<-done
	if err := owner.Quiesce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

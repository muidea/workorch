package run

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	plugincommon "github.com/muidea/magicCommon/framework/plugin/common"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type runLifecycleHub struct {
	magicevent.Hub
	mu               sync.Mutex
	failSubscribe    int
	subscribeCalls   int
	failUnsubscribe  int
	unsubscribeCalls int
	unsubscribed     []string
	subscribed       []string
	panicValue       any
}

func (h *runLifecycleHub) Subscribe(topic string, observer magicevent.Observer) *cd.Error {
	h.mu.Lock()
	h.subscribeCalls++
	fail := h.subscribeCalls == h.failSubscribe
	h.mu.Unlock()
	if fail {
		return cd.WrapError(cd.ResourceExhausted, common.ErrConflict, "subscription rejected")
	}
	if err := h.Hub.Subscribe(topic, observer); err != nil {
		return err
	}
	h.mu.Lock()
	h.subscribed = append(h.subscribed, topic)
	h.mu.Unlock()
	return nil
}
func (h *runLifecycleHub) Unsubscribe(topic string, observer magicevent.Observer) *cd.Error {
	h.mu.Lock()
	h.unsubscribeCalls++
	fail := h.unsubscribeCalls == h.failUnsubscribe
	panicValue := h.panicValue
	h.mu.Unlock()
	if fail {
		if panicValue != nil {
			panic(panicValue)
		}
		return cd.WrapError(cd.ResourceExhausted, common.ErrConflict, "unsubscribe rejected")
	}
	if err := h.Hub.Unsubscribe(topic, observer); err != nil {
		return err
	}
	h.mu.Lock()
	h.unsubscribed = append(h.unsubscribed, topic)
	h.mu.Unlock()
	return nil
}

func TestRunLifecyclePartialSubscriptionsRemainReachableAndReleaseInReverse(t *testing.T) {
	// Every setup/start acquisition cut, including the first command and final
	// execution ingress, must retain precisely the successful release prefix.
	for failAt := 1; failAt <= 22; failAt++ {
		t.Run(string(rune('A'+failAt-1)), func(t *testing.T) {
			hub := &runLifecycleHub{Hub: newRunModuleTestHub(t), failSubscribe: failAt}
			module := New()
			err := module.Configure(runTestDependencies(memory.NewRunRepository(), nil, nil, hub, nil))
			if err == nil {
				err = module.Start()
			}
			if err == nil {
				t.Fatalf("subscription cut %d did not fail", failAt)
			}
			if module.bizPtr == nil {
				t.Fatal("partial owner was discarded")
			}
			if err := module.Configure(runTestDependencies(memory.NewRunRepository(), nil, nil, hub, nil)); !errors.Is(err, common.ErrConflict) {
				t.Fatalf("overwrote partial owner: %v", err)
			}
			if err := module.Stop(context.Background()); err != nil {
				t.Fatal(err)
			}
			if module.bizPtr != nil {
				t.Fatal("successful release retained owner")
			}
			if len(hub.unsubscribed) != failAt-1 {
				t.Fatalf("released %d acquired topics, want %d", len(hub.unsubscribed), failAt-1)
			}
			for i, topic := range hub.unsubscribed {
				if topic != hub.subscribed[len(hub.subscribed)-1-i] {
					t.Fatalf("release order %d: %s, acquisitions=%v", i, topic, hub.subscribed)
				}
			}
		})
	}
}

func TestRunLifecycleFailedUnsubscribeKeepsOwnerAndResumesLedger(t *testing.T) {
	hub := &runLifecycleHub{Hub: newRunModuleTestHub(t), failUnsubscribe: 3}
	module := New()
	if err := module.Configure(runTestDependencies(memory.NewRunRepository(), nil, nil, hub, nil)); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	owner := module.bizPtr
	manager := plugincommon.NewPluginMgr("run-checked-teardown")
	if err := manager.Register(module); err != nil {
		t.Fatal(err)
	}
	if err := manager.TeardownChecked(context.Background()); err == nil || !errors.Is(err, common.ErrConflict) {
		t.Fatalf("manager lost teardown failure: %v", err)
	}
	if module.bizPtr != owner {
		t.Fatal("failed teardown discarded owner")
	}
	if err := manager.TeardownChecked(context.Background()); err != nil {
		t.Fatal(err)
	}
	if module.bizPtr != nil {
		t.Fatal("retry did not release owner")
	}
	seen := map[string]bool{}
	for _, topic := range hub.unsubscribed {
		if seen[topic] {
			t.Fatalf("repeated successful unsubscribe %s", topic)
		}
		seen[topic] = true
	}
	if len(seen) != 22 || hub.unsubscribeCalls != 23 {
		t.Fatalf("release ledger: topics=%d calls=%d", len(seen), hub.unsubscribeCalls)
	}
	if err := manager.TeardownChecked(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hub.unsubscribeCalls != 23 {
		t.Fatal("duplicate manager teardown repeated release")
	}
}

func TestRunLifecycleUnknownSubscriptionPanicPreservedWithoutLockedCleanup(t *testing.T) {
	sentinel := &struct{ name string }{"unknown panic"}
	hub := &runLifecycleHub{Hub: newRunModuleTestHub(t), failUnsubscribe: 1, panicValue: sentinel}
	module := New()
	if err := module.Configure(runTestDependencies(memory.NewRunRepository(), nil, nil, hub, nil)); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if value := recover(); value != sentinel {
				t.Fatalf("unknown panic changed: %v", value)
			}
		}()
		_ = module.Stop(context.Background())
	}()
	done := make(chan error, 1)
	go func() { done <- module.Stop(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("panic left commandMu locked")
	}
}

func TestRunLifecycleBeginShutdownKeepsReadinessAndAcceptedCASUntilRealHandlerExit(t *testing.T) {
	ctx := context.Background()
	base := memory.NewRunRepository()
	bootstrap := runtimecontract.RuntimeBootstrap{}
	state := runtimecontract.RuntimeStateRecord{StateRevision: 1}
	item := &runmodel.Run{ID: "run-shutdown", WorkspaceID: "ws-shutdown", State: common.RunStateActive, RuntimeBootstrap: &bootstrap, RuntimeState: &state}
	if err := base.Create(ctx, item); err != nil {
		t.Fatal(err)
	}
	runs := &blockingRunUpdateRepository{RunRepository: base, started: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(runs.release) }) }
	defer release()
	hub := newRunModuleTestHub(t)
	module := New()
	if err := module.Configure(runTestDependencies(runs, nil, nil, hub, nil)); err != nil {
		t.Fatal(err)
	}
	if err := module.Start(); err != nil {
		t.Fatal(err)
	}
	owner := module.bizPtr
	commandDone := make(chan error, 1)
	go func() {
		revision := uint64(1)
		_, err := sendRunTestCommand(ctx, hub, runevents.TopicRunCommandStoreRuntimeStateRecord, runevents.StoreRuntimeStateRecordCommand{WorkspaceID: item.WorkspaceID, RunID: item.ID, RuntimeState: &state, ExpectedRuntimeStateRevision: &revision})
		commandDone <- err
	}()
	select {
	case <-runs.started:
	case <-time.After(time.Second):
		t.Fatal("CAS handler not running")
	}
	beginDone := make(chan struct{})
	go func() { module.BeginShutdown(ctx); close(beginDone) }()
	select {
	case <-beginDone:
	case <-time.After(time.Second):
		t.Fatal("BeginShutdown waited for CAS")
	}
	if err := module.Start(); err == nil {
		t.Fatal("shutdown reopened Start")
	}
	readinessEvent := magicevent.NewEventWithContext(runevents.TopicRunCommandRuntimeStateRecordReadiness, "kernel/orchestrator", ModuleID, magicevent.NewHeader(), ctx, runevents.QueryRuntimeStateRecordReadinessCommand{})
	readinessEvent.BindLaneKey("run-owner-readiness")
	readiness := hub.Send(readinessEvent)
	data, err := readiness.Get()
	if err != nil || !reflect.DeepEqual(data, runevents.QueryRuntimeStateRecordReadinessResult{Ready: true}) {
		t.Fatalf("accepted CAS readiness disabled: data=%+v err=%v", data, err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		budget, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
		err := module.Quiesce(budget)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) || module.bizPtr != owner {
			t.Fatalf("quiescence attempt %d: %v", attempt, err)
		}
	}
	release()
	if err := <-commandDone; err != nil {
		t.Fatal(err)
	}
	if err := module.Quiesce(ctx); err != nil {
		t.Fatal(err)
	}
	// Queries remain legal even after local zero-inflight quiescence.
	if _, err := sendRunTestCommand(ctx, hub, runevents.TopicRunCommandGet, runevents.GetRunCommand{WorkspaceID: item.WorkspaceID, RunID: item.ID}); err != nil {
		t.Fatal(err)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

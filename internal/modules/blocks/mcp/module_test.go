package mcp

import (
	"context"
	"errors"
	frameworkdef "github.com/muidea/magicCommon/def"
	plugincommon "github.com/muidea/magicCommon/framework/plugin/common"
	"testing"

	mcpbiz "github.com/muidea/workorch/internal/modules/blocks/mcp/biz"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestMCPModuleIdentityConfigureStartStopContract(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	defer rawHub.Terminate(ctx)

	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if err := module.Configure(mcpbiz.Dependencies{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Configure without EventHub err = %v, want invalid argument", err)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}
	if err := module.Configure(mcpbiz.Dependencies{EventHub: rawHub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.bizPtr == nil || module.bizPtr.EventHub() == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
}

type moduleResourceLedgerHub struct {
	magicevent.Hub
	subscribeCalls  int
	failSubscribe   int
	failUnsubscribe bool
	failure         *frameworkdef.Error
}

func (h *moduleResourceLedgerHub) Subscribe(topic string, observer magicevent.Observer) *frameworkdef.Error {
	h.subscribeCalls++
	if h.subscribeCalls == h.failSubscribe {
		return h.failure
	}
	return h.Hub.Subscribe(topic, observer)
}
func (h *moduleResourceLedgerHub) Unsubscribe(topic string, observer magicevent.Observer) *frameworkdef.Error {
	if h.failUnsubscribe {
		return h.failure
	}
	return h.Hub.Unsubscribe(topic, observer)
}

func TestMCPModuleResourceLedgerRetainsSameOwnerAfterPartialStartAndFailedStop(t *testing.T) {

	ctx := context.Background()
	hub := &moduleResourceLedgerHub{Hub: magicevent.NewHub(8), failSubscribe: 3, failure: frameworkdef.NewError(frameworkdef.ResourceExhausted, "subscription failure")}
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := New()
	if err := module.Configure(mcpbiz.Dependencies{EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	owner := module.bizPtr
	if err := module.Start(); err != hub.failure {
		t.Fatalf("Start error = %v", err)
	}
	if module.bizPtr != owner {
		t.Fatal("failed Start lost owner")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := module.Stop(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop error = %v", err)
	}
	if module.bizPtr != owner {
		t.Fatal("canceled Stop lost owner")
	}
	hub.failUnsubscribe = true
	if err := module.Stop(ctx); err != hub.failure {
		t.Fatalf("Stop error = %v", err)
	}
	if module.bizPtr != owner {
		t.Fatal("failed Stop lost owner")
	}
	hub.failUnsubscribe = false
	if err := module.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if module.bizPtr != nil {
		t.Fatal("successful Stop retained released owner")
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestMCPModuleResourceLedgerPluginGuardReportsTeardownFailureAndRetries(t *testing.T) {

	ctx := context.Background()
	hub := &moduleResourceLedgerHub{Hub: magicevent.NewHub(8), failSubscribe: 3, failure: frameworkdef.NewError(frameworkdef.ResourceExhausted, "subscription failure")}
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := New()
	manager := plugincommon.NewPluginMgr("capability-resource-ledger-test")
	if err := manager.Register(module); err != nil {
		t.Fatal(err)
	}
	if err := manager.Setup(ctx, hub, nil); err != nil {
		t.Fatal(err)
	}
	owner := module.bizPtr
	if err := manager.Run(ctx); err == nil {
		t.Fatal("plugin Run hid subscription failure")
	}
	hub.failUnsubscribe = true
	if err := manager.TeardownChecked(ctx); err == nil {
		t.Fatal("plugin guard hid incomplete teardown")
	}
	if module.bizPtr != owner {
		t.Fatal("plugin guard lost retryable owner")
	}
	hub.failUnsubscribe = false
	if err := manager.TeardownChecked(ctx); err != nil {
		t.Fatal(err)
	}
	if module.bizPtr != nil {
		t.Fatal("plugin retry retained released owner")
	}
	if err := manager.TeardownChecked(ctx); err != nil {
		t.Fatal(err)
	}
}

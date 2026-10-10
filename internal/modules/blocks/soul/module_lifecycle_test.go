package soul

import (
	"context"
	"errors"
	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	plugincommon "github.com/muidea/magicCommon/framework/plugin/common"
	ownerbiz "github.com/muidea/workorch/internal/modules/blocks/soul/biz"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	"testing"
)

type stagedOwnerFailureHub struct {
	magicevent.Hub
	calls           int
	failSubscribe   bool
	failUnsubscribe bool
	cause           *cd.Error
}

func (h *stagedOwnerFailureHub) Subscribe(topic string, observer magicevent.Observer) *cd.Error {
	h.calls++
	if h.failSubscribe && h.calls == 2 {
		return h.cause
	}
	return h.Hub.Subscribe(topic, observer)
}
func (h *stagedOwnerFailureHub) Unsubscribe(topic string, observer magicevent.Observer) *cd.Error {
	if h.failUnsubscribe {
		return h.cause
	}
	return h.Hub.Unsubscribe(topic, observer)
}
func TestModuleStagedLifecycleRetainsOwnerAndErrorCause(t *testing.T) {
	ctx := context.Background()
	hub := &stagedOwnerFailureHub{Hub: magicevent.NewHub(16), failSubscribe: true, cause: cd.NewError(cd.ResourceExhausted, "test subscription failure")}
	defer hub.Terminate(ctx)
	m := New()
	deps := ownerbiz.Dependencies{Souls: memory.NewSoulRepository(), EventHub: hub}
	if err := m.Configure(deps); err != nil {
		t.Fatal(err)
	}
	owner := m.bizPtr
	if err := m.Configure(deps); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("Configure overwrote owner: %v", err)
	}
	if err := m.Run(ctx); !errors.Is(err, hub.cause) {
		t.Fatalf("Run lost cause: %v", err)
	}
	if m.bizPtr != owner {
		t.Fatal("partial Run lost owner")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	m.BeginShutdown(canceled)
	if err := m.Quiesce(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("Quiesce lost cancellation cause: %v", err)
	}
	if err := m.Stop(canceled); !errors.Is(err, context.Canceled) || m.bizPtr != owner {
		t.Fatalf("canceled Stop = %v", err)
	}
	manager := plugincommon.NewPluginMgr("staged-asset-test")
	if err := manager.Register(m); err != nil {
		t.Fatal(err)
	}
	hub.failUnsubscribe = true
	if err := manager.TeardownChecked(ctx); !errors.Is(err, hub.cause) {
		t.Fatalf("guard lost cleanup cause: %v", err)
	}
	if m.bizPtr != owner {
		t.Fatal("failed guarded cleanup lost owner")
	}
	hub.failUnsubscribe = false
	if err := manager.TeardownChecked(ctx); err != nil {
		t.Fatal(err)
	}
	if m.bizPtr != nil {
		t.Fatal("successful cleanup retained owner")
	}
	if err := m.Configure(deps); err != nil {
		t.Fatal(err)
	}
	if m.bizPtr == owner {
		t.Fatal("new generation reused released owner")
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

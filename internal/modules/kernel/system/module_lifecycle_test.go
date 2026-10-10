package system

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	plugincommon "github.com/muidea/magicCommon/framework/plugin/common"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	soulevents "github.com/muidea/workorch/internal/modules/blocks/soul/pkg/events"
	systembiz "github.com/muidea/workorch/internal/modules/kernel/system/biz"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestSystemStagedLifecycleWaitsForActualEnsureAndRetainsOwner(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	m := New()
	deps := systembiz.Dependencies{EventHub: hub}
	if err := m.Configure(deps); err != nil {
		t.Fatal(err)
	}
	owner := m.bizPtr
	if err := m.Configure(deps); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("owner overwritten: %v", err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	observer := magicevent.NewSimpleObserverWithMatchID("system-staged-test", soulevents.CommandDestination, hub)
	cause := cd.NewError(cd.ResourceExhausted, "ensure failed after actual return")
	if err := observer.Subscribe(soulevents.TopicSoulCommandEnsureDefaults, func(_ magicevent.Event, result magicevent.Result) {
		close(entered)
		<-release
		result.Set(nil, cause)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		unblock()
		_ = m.Stop(ctx)
		observer.Unsubscribe(soulevents.TopicSoulCommandEnsureDefaults)
		hub.Terminate(ctx)
	})
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan *cd.Error, 1)
	go func() { done <- m.Run(callCtx) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("ensure did not enter")
	}
	cancel()
	m.BeginShutdown(ctx)
	short, stop := context.WithTimeout(ctx, 10*time.Millisecond)
	defer stop()
	if err := m.Quiesce(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Quiesce = %v", err)
	}
	if err := m.Stop(short); !errors.Is(err, context.DeadlineExceeded) || m.bizPtr != owner {
		t.Fatalf("Stop prematurely released owner: %v", err)
	}
	if err := m.Start(ctx); !errors.Is(err, basebiz.ErrLifecycleClosing) {
		t.Fatalf("new Start = %v", err)
	}
	manager := plugincommon.NewPluginMgr("system-staged")
	if err := manager.Register(m); err != nil {
		t.Fatal(err)
	}
	if err := manager.TeardownChecked(short); !errors.Is(err, context.DeadlineExceeded) || m.bizPtr != owner {
		t.Fatalf("guard = %v", err)
	}
	select {
	case <-done:
		t.Fatal("cancellation fabricated ensure completion")
	default:
	}
	unblock()
	select {
	case err := <-done:
		if !errors.Is(err, cause) {
			t.Fatalf("Run lost cause: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ensure did not return")
	}
	if err := manager.TeardownChecked(ctx); err != nil {
		t.Fatal(err)
	}
	if m.bizPtr != nil {
		t.Fatal("released owner retained")
	}
	if err := m.Configure(deps); err != nil {
		t.Fatal(err)
	}
	if m.bizPtr == owner {
		t.Fatal("released generation reused")
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

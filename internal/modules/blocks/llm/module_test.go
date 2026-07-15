package llm

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestLLMModuleIdentityConfigureStartStopContract(t *testing.T) {
	ctx := context.Background()
	rawHub := magicevent.NewHub(16)
	defer rawHub.Terminate(ctx)

	module := New()
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start before Configure err = %v, want invalid argument", err)
	}
	if err := module.Configure(Dependencies{EventHub: rawHub}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.catalogService == nil || module.rawEventHub == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	firstObserver := module.observer
	firstSubscriptionCount := len(module.topics)
	if firstObserver == nil || firstSubscriptionCount == 0 {
		t.Fatalf("observer=%v subscriptions=%d, want provider/runtime V2 subscriptions", firstObserver != nil, firstSubscriptionCount)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if module.observer != firstObserver || len(module.topics) != firstSubscriptionCount {
		t.Fatalf("second Start changed observer/subscriptions: observer changed=%v subscriptions=%d want %d", module.observer != firstObserver, len(module.topics), firstSubscriptionCount)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.rawEventHub != nil || module.observer != nil || module.prerequisiteObserver != nil || len(module.topics) != 0 {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
}

func TestLLMModuleStartRequiresEventHub(t *testing.T) {
	module := New()
	if err := module.Configure(Dependencies{}); err != nil {
		t.Fatalf("Configure with empty dependencies: %v", err)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start without EventHub err = %v, want invalid argument", err)
	}
}

package file

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestFileModuleIdentityConfigureStartStopContract(t *testing.T) {
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
	if module.runtimeAdapter == nil || module.eventHub == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	firstObserver := module.capabilityObserver
	if firstObserver == nil {
		t.Fatal("capability observer is nil after Start")
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if module.capabilityObserver != firstObserver {
		t.Fatal("second Start replaced capability observer")
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.runtimeAdapter != nil || module.eventHub != nil || module.capabilityObserver != nil || module.prerequisiteObserver != nil {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestFileModuleStartRequiresEventHub(t *testing.T) {
	module := New()
	if err := module.Configure(Dependencies{}); err != nil {
		t.Fatalf("Configure with empty dependencies: %v", err)
	}
	if err := module.Start(); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Start without EventHub err = %v, want invalid argument", err)
	}
}

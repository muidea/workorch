package workorchd

import (
	"context"
	"errors"
	"testing"

	"github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
)

func TestModuleSetupReturnsConfigError(t *testing.T) {
	t.Setenv("WORKORCH_SERVER_READ_TIMEOUT", "not-a-duration")
	mod := New()

	if err := mod.Setup(event.NewHub(8), task.NewBackgroundRoutine(8)); err == nil {
		t.Fatal("expected setup error")
	}
}

func TestModuleRunRequiresSetup(t *testing.T) {
	mod := New()

	if err := mod.Run(); err == nil {
		t.Fatal("expected run error")
	}
}

func TestModuleTeardownIsIdempotent(t *testing.T) {
	mod := New()
	mod.Teardown()
	mod.Teardown()
}

func TestDoneReturnsClosedChannelBeforeSetup(t *testing.T) {
	mod := New()

	select {
	case _, ok := <-mod.Done():
		if ok {
			t.Fatal("expected closed done channel")
		}
	default:
		t.Fatal("expected done channel to be closed before setup")
	}
}

func TestModuleSetupAndTeardownWithEphemeralAddress(t *testing.T) {
	t.Setenv("WORKORCH_SERVER_ADDR", "127.0.0.1:0")
	t.Setenv("WORKORCH_REPOSITORY_BACKEND", "memory")
	mod := New()

	if err := mod.Setup(event.NewHub(8), task.NewBackgroundRoutine(8)); err != nil {
		t.Fatalf("setup module: %v", err)
	}
	mod.Teardown()
}

func TestModuleDoneReportsServerCloseAsCleanShutdown(t *testing.T) {
	t.Setenv("WORKORCH_SERVER_ADDR", "127.0.0.1:0")
	t.Setenv("WORKORCH_REPOSITORY_BACKEND", "memory")
	mod := New()

	if err := mod.Setup(event.NewHub(8), task.NewBackgroundRoutine(8)); err != nil {
		t.Fatalf("setup module: %v", err)
	}
	if err := mod.Run(); err != nil {
		t.Fatalf("run module: %v", err)
	}
	mod.Teardown()

	select {
	case err, ok := <-mod.Done():
		if ok && err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("done error = %v, want nil or context canceled", err)
		}
	default:
		t.Fatal("expected done channel to close after teardown")
	}
}

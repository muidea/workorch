package bootstrap

import (
	"context"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
)

func TestRuntimeShutdownTerminatesHub(t *testing.T) {
	hub := magicevent.NewHub(4)
	rt := &Runtime{
		exports: Exports{Infrastructure: InfrastructureExports{CommandHub: hub}},
	}

	err := rt.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("shutdown err = %v, want nil", err)
	}
}

func TestRuntimeShutdownDrainsBackground(t *testing.T) {
	background := magictask.NewBackgroundRoutine(4)
	done := make(chan bool, 1)
	rt := &Runtime{background: background}
	if err := background.AsyncFunction(func() {
		time.Sleep(10 * time.Millisecond)
		done <- true
	}); err != nil {
		t.Fatalf("schedule background task: %v", err)
	}

	if err := rt.Shutdown(context.Background()); err != nil {
		t.Fatalf("runtime shutdown: %v", err)
	}
	select {
	case completed := <-done:
		if !completed {
			t.Fatal("background task did not complete")
		}
	default:
		t.Fatal("runtime shutdown returned before background task completed")
	}
}

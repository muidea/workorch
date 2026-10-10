package bootstrap

import (
	"context"
	"errors"
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

func TestRuntimeShutdownTimeoutRetainsSharedResourcesAndRetries(t *testing.T) {
	ctx := context.Background()
	background := magictask.NewBackgroundRoutine(4)
	hub := magicevent.NewHub(4)
	entered, release := make(chan struct{}), make(chan struct{})
	rt := &Runtime{background: background, exports: Exports{Infrastructure: InfrastructureExports{CommandHub: hub, BackgroundRoutine: background}}}
	if err := background.AsyncFunction(func() { close(entered); <-release }); err != nil {
		t.Fatal(err)
	}
	<-entered
	short, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	err := rt.Shutdown(short)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown=%v", err)
	}
	if rt.background != background || rt.Exports().Infrastructure.CommandHub != hub || rt.Exports().Infrastructure.BackgroundRoutine != background {
		t.Fatal("timeout discarded shared resources")
	}
	observer := magicevent.NewSimpleObserver("still-alive", hub)
	if err := observer.Subscribe("query", func(_ magicevent.Event, result magicevent.Result) { result.Set(struct{}{}, nil) }); err != nil {
		t.Fatal(err)
	}
	if err := hub.Send(magicevent.NewEventWithContext("query", "test", "still-alive", nil, ctx, nil)).Error(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := rt.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if rt.background != nil || rt.Exports().Infrastructure.CommandHub != nil {
		t.Fatal("successful shutdown retained resources")
	}
	if err := rt.Shutdown(ctx); err != nil {
		t.Fatal(err)
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

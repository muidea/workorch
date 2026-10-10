package testkit

import (
	"context"
	"errors"
	"testing"
	"time"

	cd "github.com/muidea/magicCommon/def"
	magicevent "github.com/muidea/magicCommon/event"
	filemodule "github.com/muidea/workorch/internal/modules/blocks/file"
)

type startupFaultHub struct {
	magicevent.DrainingHub
	failure          error
	panicOnSubscribe bool
	failOwner        string
	calls            int
}

func (h *startupFaultHub) Subscribe(topic string, observer magicevent.Observer) *cd.Error {
	if h.failOwner != "" && observer.ID() != h.failOwner {
		return h.DrainingHub.Subscribe(topic, observer)
	}
	h.calls++
	if h.calls == 3 {
		if h.panicOnSubscribe {
			panic(h.failure)
		}
		return cd.WrapError(cd.Unexpected, h.failure, "subscribe rejected")
	}
	return h.DrainingHub.Subscribe(topic, observer)
}

func TestPartialRealOwnerStartupRetainsRuntimeOnErrorAndPanic(t *testing.T) {
	for _, panicOnSubscribe := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "panic"}[panicOnSubscribe], func(t *testing.T) {
			ctx := context.Background()
			raw := magicevent.NewHub(16).(magicevent.DrainingHub)
			defer raw.Terminate(ctx)
			failure := errors.New("startup subscription failed")
			fault := &startupFaultHub{DrainingHub: raw, failure: failure, panicOnSubscribe: panicOnSubscribe}
			runtime, err := TryNewRuntime("memory", t.TempDir(), fault)
			if !errors.Is(err, failure) || runtime == nil {
				t.Fatalf("runtime=%v error=%v", runtime, err)
			}
			h := runtime.Exports().Infrastructure.CommandHub.(*runtimeHub)
			if len(h.modules) != 17 || h.repos.Workspaces == nil {
				t.Fatalf("partial startup ledger owners=%d repositories=%v", len(h.modules), h.repos.Workspaces != nil)
			}
			if err := runtime.Shutdown(ctx); err != nil {
				t.Fatal(err)
			}
			if len(h.modules) != 0 || h.repos.Workspaces != nil {
				t.Fatal("successful release retained local dependencies")
			}
			if err := runtime.Shutdown(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPartialRealOwnerStartRetainsAllConfiguredOwners(t *testing.T) {
	for _, panicOnSubscribe := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "panic"}[panicOnSubscribe], func(t *testing.T) {
			ctx := context.Background()
			raw := magicevent.NewHub(16).(magicevent.DrainingHub)
			defer raw.Terminate(ctx)
			failure := errors.New("File Start subscription failed")
			fault := &startupFaultHub{DrainingHub: raw, failure: failure, failOwner: filemodule.ModuleID, panicOnSubscribe: panicOnSubscribe}
			runtime, err := TryNewRuntime("memory", t.TempDir(), fault)
			if !errors.Is(err, failure) || runtime == nil {
				t.Fatalf("runtime=%v error=%v", runtime, err)
			}
			h := runtime.Exports().Infrastructure.CommandHub.(*runtimeHub)
			if len(h.modules) != 20 || fault.calls != 3 {
				t.Fatalf("owners=%d File subscriptions=%d", len(h.modules), fault.calls)
			}
			if err := runtime.Shutdown(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRealRuntimeHubDrainTimeoutRetainsOwnersUntilActualHandlerExit(t *testing.T) {
	ctx := context.Background()
	runtime := NewRuntime("memory", t.TempDir(), nil)
	h := runtime.Exports().Infrastructure.CommandHub.(*runtimeHub)
	entered, release := make(chan struct{}), make(chan struct{})
	observer := magicevent.NewSimpleObserver("blocked-drain", h)
	if err := observer.Subscribe("blocked-drain", func(_ magicevent.Event, _ magicevent.Result) {
		close(entered)
		<-release
	}); err != nil {
		t.Fatal(err)
	}
	h.Post(magicevent.NewEventWithContext("blocked-drain", "test", "blocked-drain", nil, ctx, nil))
	<-entered
	short, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	err := runtime.Shutdown(short)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) || len(h.modules) != 20 || h.repos.Workspaces == nil {
		t.Fatalf("shutdown=%v owners=%d", err, len(h.modules))
	}
	close(release)
	if err := runtime.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if len(h.modules) != 0 || runtime.Exports().Infrastructure.CommandHub != nil {
		t.Fatal("retry did not release drained runtime")
	}
}

type stagedOwnerProbe struct {
	begins, stops int
	quiesceError  *cd.Error
	stopError     error
}

func (*stagedOwnerProbe) Start() error                        { return nil }
func (p *stagedOwnerProbe) BeginShutdown(context.Context)     { p.begins++ }
func (p *stagedOwnerProbe) Quiesce(context.Context) *cd.Error { return p.quiesceError }
func (p *stagedOwnerProbe) Stop(context.Context) error        { p.stops++; return p.stopError }

func TestRuntimeHubStagesRetainFailedOwnersAndReleaseOnce(t *testing.T) {
	ctx := context.Background()
	raw := magicevent.NewHub(8)
	defer raw.Terminate(ctx)
	h := newRuntimeHub(raw)
	failure := errors.New("owner is still running")
	first, last := &stagedOwnerProbe{}, &stagedOwnerProbe{quiesceError: cd.WrapError(cd.Unexpected, failure, "quiesce")}
	h.modules = []lifecycle{first, last}
	if err := h.Quiesce(ctx); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if first.begins != 1 || last.begins != 1 || first.stops != 0 || last.stops != 0 || len(h.modules) != 2 {
		t.Fatal("failed barrier released owners or missed BeginShutdown")
	}
	if err := h.TerminateChecked(ctx); err == nil {
		t.Fatal("Terminate bypassed owner release")
	}
	last.quiesceError = nil
	if err := h.Quiesce(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.Release(ctx); err == nil {
		t.Fatal("Release bypassed Hub drain")
	}
	if err := h.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	last.stopError = failure
	if err := h.Release(ctx); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if len(h.modules) != 2 || first.stops != 0 {
		t.Fatal("failed release discarded dependency ledger")
	}
	last.stopError = nil
	first.stopError = failure
	if err := h.Release(ctx); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if len(h.modules) != 1 || h.modules[0] != first {
		t.Fatal("successful release was not recorded")
	}
	stops := last.stops
	first.stopError = nil
	if err := h.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if last.stops != stops || len(h.modules) != 0 {
		t.Fatal("retry repeated completed release")
	}
	if err := h.TerminateChecked(ctx); err != nil {
		t.Fatal(err)
	}
}

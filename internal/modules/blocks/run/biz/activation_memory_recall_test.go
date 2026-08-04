package run

import (
	"context"
	"testing"
	"time"

	memorybiz "github.com/muidea/workorch/internal/modules/blocks/memory/biz"

	magicevent "github.com/muidea/magicCommon/event"
	basebiz "github.com/muidea/workorch/internal/modules/base/biz"
	memorymodule "github.com/muidea/workorch/internal/modules/blocks/memory"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestPublishRuntimeActivationMemoryRecallUsesTypedMemoryOwnerCommand(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	biz := &Biz{Base: basebiz.New(ModuleID, hub, nil)}

	received := make(chan memoryevents.UpsertMemoryRecallTraceCommand, 1)
	observerID := "/run/consumers/activation-memory-recall-test"
	// The production command is addressed to the memory owner.  Keep the
	// observer's logical ID local to the test, but match the concrete owner
	// destination so this regression exercises the same typed EventHub route
	// as the real handoff.
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, memoryevents.CommandDestination, hub)
	observer.Subscribe(memoryevents.TopicMemoryRecallTraceCommandUpsert, func(event magicevent.Event, result magicevent.Result) {
		if command, ok := event.Data().(memoryevents.UpsertMemoryRecallTraceCommand); ok {
			received <- command
		}
		if result != nil {
			result.Set(memoryevents.MemoryCommandResult{}, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(memoryevents.TopicMemoryRecallTraceCommandUpsert) })

	bootstrap := &runtimecontract.RuntimeBootstrap{Activation: runtimecontract.RuntimeActivationContext{
		Memories: []runtimecontract.RuntimeMemoryContext{{ID: "memory-activation", Summary: "selected project context", VectorScore: 0.91}},
	}}
	biz.publishRuntimeActivationMemoryRecall(ctx, runevents.CreateRunCommand{
		WorkspaceID: "ws-activation-memory", RunID: "run-activation-memory", RuntimeBootstrap: bootstrap,
	})

	select {
	case command := <-received:
		if command.TraceID != "run-activation-memory/memory-recall/activation" || command.WorkspaceID != "ws-activation-memory" || command.RunID != "run-activation-memory" {
			t.Fatalf("trace identity = %+v", command)
		}
		if len(command.SelectedMemoryIDs) != 1 || command.SelectedMemoryIDs[0] != "memory-activation" || len(command.RetrievedMemoryIDs) != 1 || command.RetrievedMemoryIDs[0] != "memory-activation" {
			t.Fatalf("memory selection = %+v", command)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for typed memory recall command")
	}
}

func TestRuntimeActivationMemoryRecallSkipsEmptySelection(t *testing.T) {
	hub := magicevent.NewHub(4)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	biz := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
	received := make(chan struct{}, 1)
	observerID := "/run/consumers/activation-memory-recall-empty-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, hub)
	observer.Subscribe(memoryevents.TopicMemoryRecallTraceCommandUpsert, func(_ magicevent.Event, result magicevent.Result) {
		received <- struct{}{}
		if result != nil {
			result.Set(memoryevents.MemoryCommandResult{}, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(memoryevents.TopicMemoryRecallTraceCommandUpsert) })
	biz.publishRuntimeActivationMemoryRecall(context.Background(), runevents.CreateRunCommand{
		WorkspaceID: "ws-empty-memory", RunID: "run-empty-memory",
		RuntimeBootstrap: &runtimecontract.RuntimeBootstrap{Activation: runtimecontract.RuntimeActivationContext{}},
	})
	select {
	case <-received:
		t.Fatal("empty activation unexpectedly emitted a memory recall command")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestPublishRuntimeActivationMemoryRecallPersistsThroughMemoryOwner(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	traces := memoryrepo.NewMemoryRecallTraceRepository()
	memoryOwner := memorymodule.New()
	if err := memoryOwner.Configure(memorybiz.Dependencies{
		MemoryItems:        memoryrepo.NewMemoryItemRepository(),
		MemoryRecallTraces: traces,
		EventHub:           hub,
	}); err != nil {
		t.Fatalf("configure memory owner: %v", err)
	}
	if err := memoryOwner.Start(); err != nil {
		t.Fatalf("start memory owner: %v", err)
	}
	t.Cleanup(func() {
		_ = memoryOwner.Stop(ctx)
		hub.Terminate(ctx)
	})

	runOwner := &Biz{Base: basebiz.New(ModuleID, hub, nil)}
	runOwner.publishRuntimeActivationMemoryRecall(ctx, runevents.CreateRunCommand{
		WorkspaceID: "ws-activation-memory-persist", RunID: "run-activation-memory-persist",
		RuntimeBootstrap: &runtimecontract.RuntimeBootstrap{Activation: runtimecontract.RuntimeActivationContext{
			Memories: []runtimecontract.RuntimeMemoryContext{{ID: "memory-persist", Summary: "selected", VectorScore: 0.8}},
		}},
	})

	stored, err := traces.Get(ctx, "run-activation-memory-persist/memory-recall/activation")
	if err != nil {
		t.Fatalf("stored activation recall trace: %v", err)
	}
	if stored.WorkspaceID != "ws-activation-memory-persist" || stored.RunID != "run-activation-memory-persist" ||
		len(stored.SelectedMemoryIDs) != 1 || stored.SelectedMemoryIDs[0] != "memory-persist" {
		t.Fatalf("stored activation recall trace = %+v", stored)
	}
}

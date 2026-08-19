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
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
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
			command := event.Data().(memoryevents.UpsertMemoryRecallTraceCommand)
			result.Set(memoryevents.UpsertMemoryRecallTraceResult{
				WorkspaceID: command.WorkspaceID, RunID: command.RunID, TraceID: command.TraceID,
				Disposition: memoryevents.MemoryRecallTraceRecorded,
			}, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(memoryevents.TopicMemoryRecallTraceCommandUpsert) })

	selection := testRuntimeMemorySelection("memory-activation", "selected project context", 0.91)
	selection.CandidateMemoryIDs = append(selection.CandidateMemoryIDs, "memory-excluded")
	selection.ExcludedMemoryIDs = []common.MemoryID{"memory-excluded"}
	selection.SelectionScores = append(selection.SelectionScores, workorchmemorycontract.RecallSelectionScore{
		ID: "memory-excluded", Score: 0, Reason: "excluded_type_quota",
	})
	selection.TypeQuota = []workorchmemorycontract.TypeQuotaEntry{{Type: "project_context", Quota: 1}}
	bootstrap := &runtimecontract.RuntimeBootstrap{Activation: runtimecontract.RuntimeActivationContext{
		Memories:        []runtimecontract.RuntimeMemoryContext{{ID: "memory-activation", Summary: "selected project context", VectorScore: 0.91}},
		MemorySelection: selection,
	}}
	biz.publishRuntimeActivationMemoryRecall(ctx, runevents.CreateRunCommand{
		WorkspaceID: "ws-activation-memory", RunID: "run-activation-memory", RuntimeBootstrap: bootstrap,
	}, "2026-08-19T00:00:00Z")

	select {
	case command := <-received:
		if command.TraceID != "run-activation-memory/memory-recall/activation" || command.WorkspaceID != "ws-activation-memory" || command.RunID != "run-activation-memory" {
			t.Fatalf("trace identity = %+v", command)
		}
		if len(command.SelectedMemoryIDs) != 1 || command.SelectedMemoryIDs[0] != "memory-activation" || len(command.RetrievedMemoryIDs) != 1 || command.RetrievedMemoryIDs[0] != "memory-activation" {
			t.Fatalf("memory selection = %+v", command)
		}
		if len(command.CandidateMemoryIDs) != 2 || command.CandidateMemoryIDs[1] != "memory-excluded" || len(command.ExcludedMemoryIDs) != 1 || command.ExcludedMemoryIDs[0] != "memory-excluded" ||
			len(command.SelectionScores) != 2 || len(command.TypeQuota) != 1 || command.ContextBudget.UsedItems != 1 {
			t.Fatalf("memory audit trajectory = %+v", command)
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
			result.Set(memoryevents.UpsertMemoryRecallTraceResult{}, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(memoryevents.TopicMemoryRecallTraceCommandUpsert) })
	biz.publishRuntimeActivationMemoryRecall(context.Background(), runevents.CreateRunCommand{
		WorkspaceID: "ws-empty-memory", RunID: "run-empty-memory",
		RuntimeBootstrap: &runtimecontract.RuntimeBootstrap{Activation: runtimecontract.RuntimeActivationContext{}},
	}, "2026-08-19T00:00:00Z")
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
			Memories:        []runtimecontract.RuntimeMemoryContext{{ID: "memory-persist", Summary: "selected", VectorScore: 0.8}},
			MemorySelection: testRuntimeMemorySelection("memory-persist", "selected", 0.8),
		}},
	}, "2026-08-19T00:00:00Z")

	stored, err := traces.Get(ctx, "run-activation-memory-persist/memory-recall/activation")
	if err != nil {
		t.Fatalf("stored activation recall trace: %v", err)
	}
	if stored.WorkspaceID != "ws-activation-memory-persist" || stored.RunID != "run-activation-memory-persist" ||
		len(stored.SelectedMemoryIDs) != 1 || stored.SelectedMemoryIDs[0] != "memory-persist" {
		t.Fatalf("stored activation recall trace = %+v", stored)
	}
}

func TestRuntimeActivationMemoryRecallReDrivesFromPersistedRunOnStartup(t *testing.T) {
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

	runs := memoryrepo.NewRunRepository()
	selection := testRuntimeMemorySelection("memory-startup", "startup-selected", 0.87)
	bootstrap := &runtimecontract.RuntimeBootstrap{Activation: runtimecontract.RuntimeActivationContext{
		Memories:        []runtimecontract.RuntimeMemoryContext{{ID: "memory-startup", Summary: "startup-selected", VectorScore: 0.87}},
		MemorySelection: selection,
	}}
	persisted := &runmodel.Run{
		ID: "run-memory-startup", WorkspaceID: "ws-memory-startup", GoalID: "goal-memory-startup", DoDID: "dod-memory-startup",
		DefinitionRevision: "definition-memory-startup", RuntimeBootstrap: bootstrap, State: common.RunStatePending,
		CreatedAt: "2026-08-19T01:02:03Z",
	}
	if err := runs.Create(ctx, persisted); err != nil {
		t.Fatalf("persist run before recovery: %v", err)
	}
	runOwner := New()
	if err := runOwner.Configure(Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatalf("configure run owner: %v", err)
	}
	t.Cleanup(func() {
		_ = runOwner.Stop(ctx)
		_ = memoryOwner.Stop(ctx)
		hub.Terminate(ctx)
	})
	if err := runOwner.Start(); err != nil {
		t.Fatalf("start run owner: %v", err)
	}

	traceID := common.MemoryRecallTraceID("run-memory-startup/memory-recall/activation")
	workspaceCtx := common.ContextWithRuntimeWorkspaceID(ctx, persisted.WorkspaceID)
	before, err := traces.Get(workspaceCtx, traceID)
	if err != nil {
		t.Fatalf("startup-recovered activation recall trace: %v", err)
	}
	if before.OccurredAt != persisted.CreatedAt || before.WorkspaceID != persisted.WorkspaceID || before.RunID != persisted.ID ||
		len(before.CandidateMemoryIDs) != 1 || before.CandidateMemoryIDs[0] != "memory-startup" {
		t.Fatalf("startup-recovered activation recall trace = %+v", before)
	}

	// A second SM-4 recovery pass must remain an exact, zero-rewrite replay at
	// the Memory owner rather than changing the already-recorded evidence.
	runOwner.recoverPendingWorkspaceExecutionSuccessors()
	after, err := traces.Get(workspaceCtx, traceID)
	if err != nil {
		t.Fatalf("replayed activation recall trace: %v", err)
	}
	if !memorymodel.SameMemoryRecallTrace(before, after) {
		t.Fatalf("startup replay changed trace: before=%+v after=%+v", before, after)
	}
}

func testRuntimeMemorySelection(id common.MemoryID, summary string, vectorScore float64) *workorchmemorycontract.RecallSelectionEvidence {
	return &workorchmemorycontract.RecallSelectionEvidence{
		Policy: "test_selection", Status: "selected", SelectionPolicyVersion: "test/v1",
		CandidateMemoryIDs: []common.MemoryID{id}, SelectedMemoryIDs: []common.MemoryID{id}, RetrievedMemoryIDs: []common.MemoryID{id},
		SelectionScores:     []workorchmemorycontract.RecallSelectionScore{{ID: id, Score: 1, Reason: "selected_within_context_budget"}},
		RetrievedMemoryHits: []workorchmemorycontract.RecallHit{{ID: id, Summary: summary, VectorScore: vectorScore}},
		ContextBudget: workorchmemorycontract.RecallContextBudget{
			MaxItems: 8, MaxBytes: 16384, MaxEstimatedTokens: 4096, UsedItems: 1, UsedBytes: len(summary), EstimatedTokens: 1,
		},
	}
}

package run

import (
	"context"
	"errors"
	"testing"
	"time"

	memorybiz "github.com/muidea/workorch/internal/modules/blocks/memory/biz"

	cd "github.com/muidea/magicCommon/def"
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

func TestActivationMemoryRecallObservabilityFailureDoesNotRollbackCommittedRun(t *testing.T) {
	tests := []struct {
		name         string
		installOwner func(*testing.T, magicevent.Hub) func() bool
	}{
		{name: "missing subscriber"},
		{
			name: "memory owner error",
			installOwner: func(t *testing.T, hub magicevent.Hub) func() bool {
				t.Helper()
				called := make(chan struct{}, 1)
				observer := magicevent.NewSimpleObserverWithMatchID("/run/tests/memory-owner-error", memoryevents.CommandDestination, hub)
				observer.Subscribe(memoryevents.TopicMemoryRecallTraceCommandUpsert, func(_ magicevent.Event, result magicevent.Result) {
					called <- struct{}{}
					result.Set(nil, &cd.Error{Code: cd.Unexpected, Message: "memory owner unavailable", Cause: errors.New("memory owner unavailable")})
				})
				t.Cleanup(func() { observer.Unsubscribe(memoryevents.TopicMemoryRecallTraceCommandUpsert) })
				return func() bool {
					select {
					case <-called:
						return true
					default:
						return false
					}
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-memory-observability")
			hub := magicevent.NewHub(8)
			t.Cleanup(func() { hub.Terminate(context.Background()) })
			var ownerCalled func() bool
			if test.installOwner != nil {
				ownerCalled = test.installOwner(t, hub)
			}
			runs := memoryrepo.NewRunRepository()
			owner := New()
			if err := owner.Configure(Dependencies{Runs: runs, EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			intent := activationMemoryReplacementIntent(t, "ws-memory-observability", "restart-memory-observability")
			created, err := owner.createWorkspaceExecutionReplacement(ctx, intent, "")
			if err != nil {
				t.Fatalf("create Run with failed Memory observability handoff: %v", err)
			}
			if ownerCalled != nil && !ownerCalled() {
				t.Fatal("Memory owner error path was not exercised")
			}
			stored, err := runs.GetByWorkspaceID(ctx, intent.WorkspaceID, created.ID)
			if err != nil {
				t.Fatalf("committed Run was rolled back: %v", err)
			}
			if stored.ActivationRequestID != intent.RequestID || stored.RuntimeBootstrap == nil || stored.RuntimeBootstrap.Activation.MemorySelection == nil {
				t.Fatalf("committed Run lost sealed activation input: %+v", stored)
			}
		})
	}
}

func activationMemoryReplacementIntent(t *testing.T, workspaceID common.WorkspaceID, requestID string) runmodel.ReplacementIntent {
	t.Helper()
	bootstrap := restartTestBootstrap(t, workspaceID)
	bootstrap.Activation.Memories = []runtimecontract.RuntimeMemoryContext{{
		ID: "memory-observability", Summary: "selected before Run creation", VectorScore: 0.9,
	}}
	bootstrap.Activation.MemorySelection = testRuntimeMemorySelection("memory-observability", "selected before Run creation", 0.9)
	seed, err := runtimecontract.CompileRuntimeDefinitionSeed(bootstrap.Definition, bootstrap.Activation, bootstrap.CoverageItems, bootstrap.Slices)
	if err != nil {
		t.Fatalf("compile Memory activation definition seed: %v", err)
	}
	bootstrap.DefinitionSeed = seed
	if err := bootstrap.Validate(); err != nil {
		t.Fatalf("Memory activation bootstrap: %v", err)
	}
	return runmodel.ReplacementIntent{
		RequestID: requestID, WorkspaceID: workspaceID, GoalID: bootstrap.Definition.GoalID, DoDID: bootstrap.Definition.DoDID,
		DefinitionRevision: bootstrap.Definition.Revision, StartupInputHash: bootstrap.Definition.StartupInputHash,
		Bootstrap: &bootstrap, ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
		Kind: runtimecontract.RuntimeTerminationSuperseded, Reason: "workspace execution restart/" + requestID,
		State: runmodel.ReplacementIntentPending,
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

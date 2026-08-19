package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	memorybiz "github.com/muidea/workorch/internal/modules/blocks/memory/biz"

	magicevent "github.com/muidea/magicCommon/event"
	memorycommon "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/common"
	memoryevents "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/events"
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	memoryrepo "github.com/muidea/workorch/internal/pkg/repository/memory"
	"github.com/muidea/workorch/internal/pkg/workorchmemorycontract"
	"github.com/muidea/workorch/internal/pkg/workorchrepo"
)

func TestModuleIdentityConfigureStartStopContract(t *testing.T) {
	module := New()
	if ModuleID != "blocks/memory" {
		t.Fatalf("ModuleID = %q, want relocated stateful block ID", ModuleID)
	}
	if module.ID() != ModuleID {
		t.Fatalf("ID = %q, want %q", module.ID(), ModuleID)
	}

	if err := module.Configure(memoryModuleContractDependencies()); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if module.bizPtr == nil || module.bizPtr.EventHub() == nil {
		t.Fatalf("module not fully configured: %+v", module)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if module.bizPtr != nil {
		t.Fatalf("module was not cleared after Stop: %+v", module)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModuleConfigureRequiresMemoryItems(t *testing.T) {
	if err := New().Configure(memorybiz.Dependencies{}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Configure err = %v, want invalid argument", err)
	}
	if err := New().Configure(memorybiz.Dependencies{
		MemoryItems: memoryrepo.NewMemoryItemRepository(),
		EventHub:    magicevent.NewHub(4),
	}); !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("Configure without recall repository err = %v, want invalid argument", err)
	}
}

func TestMemoryRecallTraceCommandsAcceptContractPayloads(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	traces := memoryrepo.NewMemoryRecallTraceRepository()
	module := New()
	if err := module.Configure(memorybiz.Dependencies{
		MemoryItems:        memoryrepo.NewMemoryItemRepository(),
		MemoryRecallTraces: traces,
		EventHub:           hub,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := module.Stop(context.Background()); err != nil {
			t.Errorf("Stop: %v", err)
		}
	})
	traceID := common.MemoryRecallTraceID("trace-contract")
	const observerID = "/memory/consumers/blocks/memory/module-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, hub)
	notifications := make(chan memoryevents.MemoryRecallRecordedData, 1)
	observer.Subscribe(memoryevents.TopicMemoryRecallRecorded, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(memoryevents.MemoryRecallRecordedData); ok {
			notifications <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(memoryevents.TopicMemoryRecallRecorded) })

	response := hub.Send(magicevent.NewEventWithContext(
		memoryevents.TopicMemoryRecallTraceCommandUpsert,
		"test",
		memoryevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		memoryevents.UpsertMemoryRecallTraceCommand{
			TraceID:            traceID,
			WorkspaceID:        "ws-trace-contract",
			RunID:              "run-trace-contract",
			Stage:              "planning",
			Source:             "activation",
			Policy:             "test_selection",
			Status:             "selected",
			CandidateMemoryIDs: []common.MemoryID{"mem-selected", "mem-excluded"},
			SelectedMemoryIDs:  []common.MemoryID{"mem-selected"},
			ExcludedMemoryIDs:  []common.MemoryID{"mem-excluded"},
			TypeQuota:          []workorchmemorycontract.TypeQuotaEntry{{Type: "episodic", Quota: 1}},
			SelectionScores: []workorchmemorycontract.RecallSelectionScore{{
				ID:     "mem-selected",
				Score:  100,
				Reason: "selected_within_context_budget",
			}, {
				ID:     "mem-excluded",
				Score:  10,
				Reason: "excluded_low_value_process_memory",
			}},
			RetrievedMemoryIDs: []common.MemoryID{"mem-selected"},
			RetrievedMemoryHits: []workorchmemorycontract.RecallHit{{
				ID:              "mem-selected",
				Summary:         "hit",
				VectorScore:     0.91,
				VectorDimension: 4,
			}},
			ContextBudget: workorchmemorycontract.RecallContextBudget{
				MaxItems: 8, MaxBytes: 16 * 1024, MaxEstimatedTokens: 4096,
				UsedItems: 1, UsedBytes: 3, EstimatedTokens: 1,
			},
		},
	))
	if response == nil {
		t.Fatal("upsert response is nil")
	}
	if response.Error() != nil {
		t.Fatalf("upsert error = %v", response.Error())
	}
	select {
	case data := <-notifications:
		if data.TraceID != traceID || data.WorkspaceID != "ws-trace-contract" || data.RunID != "run-trace-contract" || data.Stage != "planning" {
			t.Fatalf("recall notification = %+v, want typed trace scope", data)
		}
		if data.SelectionEvidence == nil || len(data.SelectionEvidence.CandidateMemoryIDs) != 2 || len(data.SelectionEvidence.ExcludedMemoryIDs) != 1 || data.SelectionEvidence.ContextBudget.UsedItems != 1 {
			t.Fatalf("recall notification selection evidence = %+v", data.SelectionEvidence)
		}
	case <-time.After(time.Second):
		t.Fatal("typed memory recall notification not received")
	}
	stored, err := traces.Get(ctx, traceID)
	if err != nil {
		t.Fatalf("stored trace: %v", err)
	}
	if stored.Stage != "planning" || stored.TypeQuota["episodic"] != 1 || len(stored.RetrievedMemoryHits) != 1 || stored.RetrievedMemoryHits[0].ID != "mem-selected" {
		t.Fatalf("stored trace = %+v, want command fields", stored)
	}
	if len(stored.SelectedMemoryIDs) != 1 || stored.SelectedMemoryIDs[0] != "mem-selected" ||
		len(stored.ExcludedMemoryIDs) != 1 || stored.ExcludedMemoryIDs[0] != "mem-excluded" ||
		len(stored.SelectionScores) != 2 || stored.SelectionScores[1].ID != "mem-excluded" ||
		stored.SelectionScores[1].Reason != "excluded_low_value_process_memory" {
		t.Fatalf("stored trace = %+v, want selected/excluded recall audit fields", stored)
	}

	response = hub.Send(magicevent.NewEventWithContext(
		memoryevents.TopicMemoryRecallTraceCommandDelete,
		"test",
		memoryevents.CommandDestination,
		magicevent.NewHeader(),
		ctx,
		memoryevents.DeleteMemoryRecallTraceCommand{WorkspaceID: "ws-trace-contract", TraceID: traceID},
	))
	if response == nil {
		t.Fatal("delete response is nil")
	}
	if response.Error() != nil {
		t.Fatalf("delete error = %v", response.Error())
	}
	if _, err := traces.Get(ctx, traceID); err == nil {
		t.Fatalf("trace still exists after delete")
	}
}

func TestMemoryRecallTraceOwnerEnforcesScopeAndImmutableReplay(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	traces := memoryrepo.NewMemoryRecallTraceRepository()
	module := New()
	if err := module.Configure(memorybiz.Dependencies{
		MemoryItems: memoryrepo.NewMemoryItemRepository(), MemoryRecallTraces: traces, EventHub: hub,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		_ = module.Stop(ctx)
		hub.Terminate(ctx)
	})

	const observerID = "/memory/consumers/blocks/memory/replay-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, hub)
	notifications := make(chan memoryevents.MemoryRecallRecordedData, 4)
	observer.Subscribe(memoryevents.TopicMemoryRecallRecorded, func(event magicevent.Event, result magicevent.Result) {
		if data, ok := event.Data().(memoryevents.MemoryRecallRecordedData); ok {
			notifications <- data
		}
		if result != nil {
			result.Set(nil, nil)
		}
	})
	t.Cleanup(func() { observer.Unsubscribe(memoryevents.TopicMemoryRecallRecorded) })

	command := memoryevents.UpsertMemoryRecallTraceCommand{
		TraceID: "trace-replay", WorkspaceID: "ws-a", RunID: "run-a", Source: "recovery",
		Policy: "recovery", Status: "inherited", Reason: "stable", OccurredAt: "2026-08-19T00:00:00Z",
	}
	first := sendMemoryOwnerResult[memoryevents.UpsertMemoryRecallTraceResult](t, ctx, hub, memoryevents.TopicMemoryRecallTraceCommandUpsert, command)
	if first.Disposition != memoryevents.MemoryRecallTraceRecorded {
		t.Fatalf("first disposition = %q, want recorded", first.Disposition)
	}
	select {
	case <-notifications:
	case <-time.After(time.Second):
		t.Fatal("first recall notification not received")
	}

	duplicate := sendMemoryOwnerResult[memoryevents.UpsertMemoryRecallTraceResult](t, ctx, hub, memoryevents.TopicMemoryRecallTraceCommandUpsert, command)
	if duplicate.Disposition != memoryevents.MemoryRecallTraceDuplicate {
		t.Fatalf("duplicate disposition = %q, want duplicate", duplicate.Disposition)
	}
	select {
	case data := <-notifications:
		t.Fatalf("duplicate replay emitted notification: %+v", data)
	case <-time.After(75 * time.Millisecond):
	}

	conflict := command
	conflict.Reason = "mutated history"
	response := hub.Send(magicevent.NewEventWithContext(memoryevents.TopicMemoryRecallTraceCommandUpsert, "test", memoryevents.CommandDestination, magicevent.NewHeader(), ctx, conflict))
	if response == nil || response.Error() == nil {
		t.Fatal("conflicting recall replay was accepted")
	}
	stored, err := traces.Get(common.ContextWithRuntimeWorkspaceID(ctx, "ws-a"), command.TraceID)
	if err != nil {
		t.Fatalf("read stable recall trace: %v", err)
	}
	if stored.Reason != command.Reason {
		t.Fatalf("conflicting replay overwrote trace: %+v", stored)
	}

	for _, payload := range []any{
		memoryevents.GetMemoryRecallTraceCommand{TraceID: command.TraceID},
		memoryevents.ListMemoryRecallTracesCommand{},
		memoryevents.DeleteMemoryRecallTraceCommand{TraceID: command.TraceID},
	} {
		topic := memoryevents.TopicMemoryRecallTraceCommandGet
		switch payload.(type) {
		case memoryevents.ListMemoryRecallTracesCommand:
			topic = memoryevents.TopicMemoryRecallTraceCommandList
		case memoryevents.DeleteMemoryRecallTraceCommand:
			topic = memoryevents.TopicMemoryRecallTraceCommandDelete
		}
		response := hub.Send(magicevent.NewEventWithContext(topic, "test", memoryevents.CommandDestination, magicevent.NewHeader(), ctx, payload))
		if response == nil || response.Error() == nil {
			t.Fatalf("empty-scope %s was accepted", topic)
		}
	}
	mismatchedContext := common.ContextWithRuntimeWorkspaceID(ctx, "ws-b")
	mismatchedScope := hub.Send(magicevent.NewEventWithContext(
		memoryevents.TopicMemoryRecallTraceCommandGet, "test", memoryevents.CommandDestination, magicevent.NewHeader(), mismatchedContext,
		memoryevents.GetMemoryRecallTraceCommand{WorkspaceID: "ws-a", TraceID: command.TraceID},
	))
	if mismatchedScope == nil || !errors.Is(mismatchedScope.Error(), common.ErrConflict) {
		t.Fatalf("mismatched command/context scope error = %v, want conflict", mismatchedScope)
	}

	wrongScope := hub.Send(magicevent.NewEventWithContext(
		memoryevents.TopicMemoryRecallTraceCommandDelete, "test", memoryevents.CommandDestination, magicevent.NewHeader(), ctx,
		memoryevents.DeleteMemoryRecallTraceCommand{WorkspaceID: "ws-b", TraceID: command.TraceID},
	))
	if wrongScope == nil || wrongScope.Error() == nil {
		t.Fatal("cross-workspace trace delete was accepted")
	}
	if _, err := traces.Get(common.ContextWithRuntimeWorkspaceID(ctx, "ws-a"), command.TraceID); err != nil {
		t.Fatalf("cross-workspace delete removed trace: %v", err)
	}

	secondWorkspace := command
	secondWorkspace.WorkspaceID = "ws-b"
	secondWorkspace.RunID = "run-b"
	secondWorkspace.Reason = "workspace b"
	second := sendMemoryOwnerResult[memoryevents.UpsertMemoryRecallTraceResult](t, ctx, hub, memoryevents.TopicMemoryRecallTraceCommandUpsert, secondWorkspace)
	if second.Disposition != memoryevents.MemoryRecallTraceRecorded {
		t.Fatalf("same trace ID in another workspace disposition = %q", second.Disposition)
	}
	for workspaceID, wantReason := range map[common.WorkspaceID]string{"ws-a": "stable", "ws-b": "workspace b"} {
		result := sendMemoryOwnerResult[memoryevents.GetMemoryRecallTraceResult](t, ctx, hub, memoryevents.TopicMemoryRecallTraceCommandGet, memoryevents.GetMemoryRecallTraceCommand{WorkspaceID: workspaceID, TraceID: command.TraceID})
		if result.Trace == nil {
			t.Fatalf("workspace %s trace is nil", workspaceID)
		}
		model, err := result.Trace.Model()
		if err != nil || model.Reason != wantReason {
			t.Fatalf("workspace %s trace = %+v, err=%v", workspaceID, model, err)
		}
	}
}

func TestMemoryRecallTraceOwnerPreservesScopeWithFileBackend(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	repositories := workorchrepo.NewWorkspaceScopedFileRepositorySet(t.TempDir())
	module := New()
	if err := module.Configure(memorybiz.Dependencies{
		MemoryItems: repositories.MemoryItems, MemoryRecallTraces: repositories.MemoryRecallTraces, EventHub: hub,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		_ = module.Stop(ctx)
		hub.Terminate(ctx)
	})

	base := memoryevents.UpsertMemoryRecallTraceCommand{TraceID: "trace-local", WorkspaceID: "ws-file-a", RunID: "run-a", Source: "recovery", Reason: "workspace a"}
	sendMemoryOwnerResult[memoryevents.UpsertMemoryRecallTraceResult](t, ctx, hub, memoryevents.TopicMemoryRecallTraceCommandUpsert, base)
	other := base
	other.WorkspaceID, other.RunID, other.Reason = "ws-file-b", "run-b", "workspace b"
	sendMemoryOwnerResult[memoryevents.UpsertMemoryRecallTraceResult](t, ctx, hub, memoryevents.TopicMemoryRecallTraceCommandUpsert, other)
	for workspaceID, wantReason := range map[common.WorkspaceID]string{"ws-file-a": "workspace a", "ws-file-b": "workspace b"} {
		result := sendMemoryOwnerResult[memoryevents.GetMemoryRecallTraceResult](t, ctx, hub, memoryevents.TopicMemoryRecallTraceCommandGet, memoryevents.GetMemoryRecallTraceCommand{WorkspaceID: workspaceID, TraceID: base.TraceID})
		model, err := result.Trace.Model()
		if err != nil || model.Reason != wantReason {
			t.Fatalf("file workspace %s trace = %+v, err=%v", workspaceID, model, err)
		}
	}

	onlyA := base
	onlyA.TraceID, onlyA.Reason = "trace-only-a", "only workspace a"
	sendMemoryOwnerResult[memoryevents.UpsertMemoryRecallTraceResult](t, ctx, hub, memoryevents.TopicMemoryRecallTraceCommandUpsert, onlyA)
	wrongDelete := hub.Send(magicevent.NewEventWithContext(
		memoryevents.TopicMemoryRecallTraceCommandDelete, "test", memoryevents.CommandDestination, magicevent.NewHeader(), ctx,
		memoryevents.DeleteMemoryRecallTraceCommand{WorkspaceID: "ws-file-b", TraceID: onlyA.TraceID},
	))
	if wrongDelete == nil || wrongDelete.Error() == nil {
		t.Fatal("file backend cross-workspace trace delete was accepted")
	}
	result := sendMemoryOwnerResult[memoryevents.GetMemoryRecallTraceResult](t, ctx, hub, memoryevents.TopicMemoryRecallTraceCommandGet, memoryevents.GetMemoryRecallTraceCommand{WorkspaceID: "ws-file-a", TraceID: onlyA.TraceID})
	if result.Trace == nil {
		t.Fatal("file backend cross-workspace delete removed source trace")
	}
}

func TestMemoryItemOwnerEnforcesWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	module := New()
	if err := module.Configure(memorybiz.Dependencies{
		MemoryItems: memoryrepo.NewMemoryItemRepository(), MemoryRecallTraces: memoryrepo.NewMemoryRecallTraceRepository(), EventHub: hub,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		_ = module.Stop(ctx)
		hub.Terminate(ctx)
	})

	base := memoryevents.CreateMemoryCommand{
		MemoryID: "memory-local", WorkspaceID: "ws-item-a", SourceType: memorycommon.MemorySourceUser,
		MemoryType: memorycommon.MemoryTypeProjectContext, Summary: "workspace a",
	}
	sendMemoryOwnerResult[memoryevents.CreateMemoryResult](t, ctx, hub, memoryevents.TopicMemoryCommandCreate, base)
	other := base
	other.WorkspaceID, other.Summary = "ws-item-b", "workspace b"
	sendMemoryOwnerResult[memoryevents.CreateMemoryResult](t, ctx, hub, memoryevents.TopicMemoryCommandCreate, other)
	for workspaceID, wantSummary := range map[common.WorkspaceID]string{"ws-item-a": "workspace a", "ws-item-b": "workspace b"} {
		result := sendMemoryOwnerResult[memoryevents.GetMemoryResult](t, ctx, hub, memoryevents.TopicMemoryCommandGet, memoryevents.GetMemoryCommand{WorkspaceID: workspaceID, MemoryID: base.MemoryID})
		if result.Item == nil || result.Item.WorkspaceID != workspaceID || result.Item.Summary != wantSummary {
			t.Fatalf("workspace %s memory = %+v", workspaceID, result.Item)
		}
	}

	for _, command := range []struct {
		topic string
		data  any
	}{
		{memoryevents.TopicMemoryCommandCreate, memoryevents.CreateMemoryCommand{MemoryID: "memory-empty-create", SourceType: memorycommon.MemorySourceUser, MemoryType: memorycommon.MemoryTypeProjectContext, Summary: "empty scope"}},
		{memoryevents.TopicMemoryCommandGet, memoryevents.GetMemoryCommand{MemoryID: base.MemoryID}},
		{memoryevents.TopicMemoryCommandListByWorkspace, memoryevents.ListMemoryByWorkspaceCommand{}},
		{memoryevents.TopicMemoryCommandSearch, memoryevents.SearchMemoryCommand{}},
		{memoryevents.TopicMemoryCommandUpdate, memoryevents.UpdateMemoryCommand{MemoryID: base.MemoryID, SourceType: memorycommon.MemorySourceUser, MemoryType: memorycommon.MemoryTypeProjectContext, Summary: "empty scope"}},
		{memoryevents.TopicMemoryCommandDelete, memoryevents.DeleteMemoryCommand{MemoryID: base.MemoryID}},
	} {
		response := hub.Send(magicevent.NewEventWithContext(command.topic, "test", memoryevents.CommandDestination, magicevent.NewHeader(), ctx, command.data))
		if response == nil || !errors.Is(response.Error(), common.ErrInvalidArgument) {
			t.Fatalf("empty-scope %s error = %v, want invalid argument", command.topic, response)
		}
	}

	mismatchedContext := common.ContextWithRuntimeWorkspaceID(ctx, "ws-item-b")
	response := hub.Send(magicevent.NewEventWithContext(
		memoryevents.TopicMemoryCommandGet, "test", memoryevents.CommandDestination, magicevent.NewHeader(), mismatchedContext,
		memoryevents.GetMemoryCommand{WorkspaceID: "ws-item-a", MemoryID: base.MemoryID},
	))
	if response == nil || !errors.Is(response.Error(), common.ErrConflict) {
		t.Fatalf("mismatched item command/context error = %v, want conflict", response)
	}

	onlyA := base
	onlyA.MemoryID, onlyA.Summary = "memory-only-a", "only workspace a"
	sendMemoryOwnerResult[memoryevents.CreateMemoryResult](t, ctx, hub, memoryevents.TopicMemoryCommandCreate, onlyA)
	wrongDelete := hub.Send(magicevent.NewEventWithContext(
		memoryevents.TopicMemoryCommandDelete, "test", memoryevents.CommandDestination, magicevent.NewHeader(), ctx,
		memoryevents.DeleteMemoryCommand{WorkspaceID: "ws-item-b", MemoryID: onlyA.MemoryID},
	))
	if wrongDelete == nil || wrongDelete.Error() == nil {
		t.Fatal("cross-workspace memory delete was accepted")
	}
	result := sendMemoryOwnerResult[memoryevents.GetMemoryResult](t, ctx, hub, memoryevents.TopicMemoryCommandGet, memoryevents.GetMemoryCommand{WorkspaceID: "ws-item-a", MemoryID: onlyA.MemoryID})
	if result.Item == nil || result.Item.Summary != onlyA.Summary {
		t.Fatalf("cross-workspace delete changed source memory: %+v", result.Item)
	}
}

func TestMemoryOwnerCommandsPublishLifecycleNotifications(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(32)
	t.Cleanup(func() { hub.Terminate(ctx) })
	module := New()
	if err := module.Configure(memorybiz.Dependencies{
		MemoryItems:        memoryrepo.NewMemoryItemRepository(),
		MemoryRecallTraces: memoryrepo.NewMemoryRecallTraceRepository(),
		EventHub:           hub,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = module.Stop(ctx) })

	const observerID = "/memory/consumers/blocks/memory/owner-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, hub)
	received := make(chan struct {
		topic string
		data  memoryevents.MemoryChangedData
	}, 3)
	for _, topic := range []string{memoryevents.TopicMemoryCreated, memoryevents.TopicMemoryUpdated, memoryevents.TopicMemoryDeleted} {
		topic := topic
		observer.Subscribe(topic, func(event magicevent.Event, result magicevent.Result) {
			if data, ok := event.Data().(memoryevents.MemoryChangedData); ok {
				received <- struct {
					topic string
					data  memoryevents.MemoryChangedData
				}{topic: topic, data: data}
			}
			if result != nil {
				result.Set(nil, nil)
			}
		})
	}
	t.Cleanup(func() {
		observer.Unsubscribe(memoryevents.TopicMemoryCreated)
		observer.Unsubscribe(memoryevents.TopicMemoryUpdated)
		observer.Unsubscribe(memoryevents.TopicMemoryDeleted)
	})

	command := memoryevents.CreateMemoryCommand{
		MemoryID: "memory-owner-notify", WorkspaceID: "ws-memory-owner-notify", RunID: "run-memory-owner-notify",
		SourceType: memorycommon.MemorySourceRun, SourceRef: "step-memory-owner-notify",
		MemoryType: memorycommon.MemoryTypeRunSummary, Summary: "created memory", Content: "created content",
		Tags: []string{"owner"}, Importance: 50, Confidence: 60, Visibility: "workspace",
	}
	sendMemoryOwnerCommand(t, ctx, hub, memoryevents.TopicMemoryCommandCreate, command)
	created := receiveMemoryNotification(t, received)
	if created.topic != memoryevents.TopicMemoryCreated || created.data.MemoryID != command.MemoryID || created.data.Summary != command.Summary {
		t.Fatalf("created notification = %+v", created)
	}

	command.Summary = "updated memory"
	sendMemoryOwnerCommand(t, ctx, hub, memoryevents.TopicMemoryCommandUpdate, memoryevents.UpdateMemoryCommand(command))
	updated := receiveMemoryNotification(t, received)
	if updated.topic != memoryevents.TopicMemoryUpdated || updated.data.Summary != command.Summary {
		t.Fatalf("updated notification = %+v", updated)
	}

	sendMemoryOwnerCommand(t, ctx, hub, memoryevents.TopicMemoryCommandDelete, memoryevents.DeleteMemoryCommand{WorkspaceID: command.WorkspaceID, MemoryID: command.MemoryID})
	deleted := receiveMemoryNotification(t, received)
	if deleted.topic != memoryevents.TopicMemoryDeleted || deleted.data.MemoryID != command.MemoryID || deleted.data.WorkspaceID != command.WorkspaceID {
		t.Fatalf("deleted notification = %+v", deleted)
	}
}

func TestMemoryOwnerDoesNotPublishWhenRepositoryCommitFails(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	commitErr := errors.New("memory repository unavailable")
	module := New()
	if err := module.Configure(memorybiz.Dependencies{
		MemoryItems:        &failingMemoryItemRepository{MemoryItemRepository: memoryrepo.NewMemoryItemRepository(), failures: 1, err: commitErr},
		MemoryRecallTraces: &failingMemoryRecallTraceRepository{MemoryRecallTraceRepository: memoryrepo.NewMemoryRecallTraceRepository(), failures: 1, err: commitErr},
		EventHub:           hub,
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := module.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		_ = module.Stop(ctx)
		hub.Terminate(ctx)
	})

	const observerID = "/memory/consumers/blocks/memory/commit-failure-test"
	observer := magicevent.NewSimpleObserverWithMatchID(observerID, observerID, hub)
	notifications := make(chan string, 2)
	for _, topic := range []string{memoryevents.TopicMemoryCreated, memoryevents.TopicMemoryRecallRecorded} {
		topic := topic
		observer.Subscribe(topic, func(_ magicevent.Event, result magicevent.Result) {
			notifications <- topic
			if result != nil {
				result.Set(nil, nil)
			}
		})
	}
	t.Cleanup(func() {
		observer.Unsubscribe(memoryevents.TopicMemoryCreated)
		observer.Unsubscribe(memoryevents.TopicMemoryRecallRecorded)
	})

	commands := []struct {
		topic string
		data  any
	}{
		{memoryevents.TopicMemoryCommandCreate, memoryevents.CreateMemoryCommand{
			MemoryID: "memory-failed", WorkspaceID: "ws-failed", SourceType: memorycommon.MemorySourceUser,
			MemoryType: memorycommon.MemoryTypeProjectContext, Summary: "must not publish",
		}},
		{memoryevents.TopicMemoryRecallTraceCommandUpsert, memoryevents.UpsertMemoryRecallTraceCommand{
			TraceID: "trace-failed", WorkspaceID: "ws-failed", RunID: "run-failed", Source: "recovery",
		}},
	}
	for _, command := range commands {
		response := hub.Send(magicevent.NewEventWithContext(command.topic, "test", memoryevents.CommandDestination, magicevent.NewHeader(), ctx, command.data))
		if response == nil || !errors.Is(response.Error(), commitErr) {
			t.Fatalf("%s error = %v, want repository failure", command.topic, response)
		}
	}
	select {
	case topic := <-notifications:
		t.Fatalf("repository failure published %s", topic)
	case <-time.After(75 * time.Millisecond):
	}
	for _, command := range commands {
		response := hub.Send(magicevent.NewEventWithContext(command.topic, "test", memoryevents.CommandDestination, magicevent.NewHeader(), ctx, command.data))
		if response == nil || response.Error() != nil {
			t.Fatalf("recovered %s error = %v", command.topic, response)
		}
	}
	seen := map[string]int{}
	for len(seen) < 2 {
		select {
		case topic := <-notifications:
			seen[topic]++
		case <-time.After(time.Second):
			t.Fatalf("recovered notifications = %#v", seen)
		}
	}
	if seen[memoryevents.TopicMemoryCreated] != 1 || seen[memoryevents.TopicMemoryRecallRecorded] != 1 {
		t.Fatalf("recovered notifications = %#v, want one each", seen)
	}
}

func sendMemoryOwnerCommand[Command any](t *testing.T, ctx context.Context, hub magicevent.Hub, topic string, command Command) {
	t.Helper()
	result := hub.Send(magicevent.NewEventWithContext(topic, "test", memoryevents.CommandDestination, magicevent.NewHeader(), ctx, command))
	if result == nil || result.Error() != nil {
		t.Fatalf("send %s: %v", topic, result)
	}
}

func sendMemoryOwnerResult[Result any, Command any](t *testing.T, ctx context.Context, hub magicevent.Hub, topic string, command Command) Result {
	t.Helper()
	response := hub.Send(magicevent.NewEventWithContext(topic, "test", memoryevents.CommandDestination, magicevent.NewHeader(), ctx, command))
	if response == nil || response.Error() != nil {
		t.Fatalf("send %s: %v", topic, response)
	}
	data, err := response.Get()
	if err != nil {
		t.Fatalf("read %s response: %v", topic, err)
	}
	ret, ok := data.(Result)
	if !ok {
		t.Fatalf("%s result type = %T", topic, data)
	}
	return ret
}

func receiveMemoryNotification(t *testing.T, received <-chan struct {
	topic string
	data  memoryevents.MemoryChangedData
}) struct {
	topic string
	data  memoryevents.MemoryChangedData
} {
	t.Helper()
	select {
	case item := <-received:
		return item
	case <-time.After(time.Second):
		t.Fatal("typed memory notification not received")
		return struct {
			topic string
			data  memoryevents.MemoryChangedData
		}{}
	}
}

func memoryModuleContractDependencies() memorybiz.Dependencies {
	return memorybiz.Dependencies{
		MemoryItems:        memoryrepo.NewMemoryItemRepository(),
		MemoryRecallTraces: memoryrepo.NewMemoryRecallTraceRepository(),
		EventHub:           magicevent.NewHub(16),
	}
}

type failingMemoryItemRepository struct {
	repository.MemoryItemRepository
	failures int
	err      error
}

func (r *failingMemoryItemRepository) Create(ctx context.Context, item *memorymodel.MemoryItem) error {
	if r.failures > 0 {
		r.failures--
		return r.err
	}
	return r.MemoryItemRepository.Create(ctx, item)
}

type failingMemoryRecallTraceRepository struct {
	repository.MemoryRecallTraceRepository
	failures int
	err      error
}

func (r *failingMemoryRecallTraceRepository) Create(ctx context.Context, item *memorymodel.MemoryRecallTrace) error {
	if r.failures > 0 {
		r.failures--
		return r.err
	}
	return r.MemoryRecallTraceRepository.Create(ctx, item)
}

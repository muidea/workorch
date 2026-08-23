package run

import (
	"context"
	"strings"
	"testing"
	"time"

	magicevent "github.com/muidea/magicCommon/event"
	magictask "github.com/muidea/magicCommon/task"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestReplacementRecoveryPersistsTerminationAndSuccessorStages(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-delivery")
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	runs := memory.NewRunRepository()
	predecessor := replacementRecoveryTestRun(t, "ws-delivery", "request-1")
	if err := runs.Create(ctx, predecessor); err != nil {
		t.Fatal(err)
	}
	observeReplacementTermination(t, hub, runs, predecessor.WorkspaceID, predecessor.ID, "terminal-1", 9)

	biz := New()
	if err := biz.Configure(Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := biz.deliverReplacementTermination(predecessor.WorkspaceID, predecessor.ID); err != nil {
		t.Fatalf("deliver termination: %v", err)
	}
	stored, err := runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ReplacementIntent == nil || stored.ReplacementIntent.State != runmodel.ReplacementIntentTerminationAcknowledged ||
		stored.ReplacementIntent.TerminalDeliveryID != "terminal-1" || stored.ReplacementIntent.RuntimeStateRevision != 9 ||
		stored.ReplacementIntent.Recovery.Stage != runmodel.ReplacementRecoverySuccessorMaterialization || stored.ReplacementIntent.Recovery.State != runmodel.ReplacementRecoveryScheduled {
		t.Fatalf("termination acknowledgement did not seal successor recovery: %+v", stored.ReplacementIntent)
	}
	if err := biz.driveReplacementRecoveryGeneration(stored.WorkspaceID, stored.ID, stored.ReplacementIntent.Recovery.Generation); err != nil {
		t.Fatalf("materialize successor: %v", err)
	}
	stored, err = runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ReplacementIntent.State != runmodel.ReplacementIntentCompleted || stored.ReplacementIntent.Recovery.State != runmodel.ReplacementRecoveryCompleted ||
		stored.ReplacementIntent.SuccessorRunID == "" || stored.ReplacementIntent.AcknowledgedAt == "" || stored.Lineage.SuccessorRunID != stored.ReplacementIntent.SuccessorRunID {
		t.Fatalf("replacement recovery did not complete: %+v", stored.ReplacementIntent)
	}
	successor, err := runs.GetByWorkspaceID(ctx, stored.WorkspaceID, stored.ReplacementIntent.SuccessorRunID)
	if err != nil || successor.State != common.RunStateActive || successor.Lineage.PredecessorRunID != stored.ID {
		t.Fatalf("successor = %+v err=%v", successor, err)
	}

	// The completed owner ledger makes duplicate process-local drive a no-op.
	if err := biz.driveReplacementRecoveryGeneration(stored.WorkspaceID, stored.ID, stored.ReplacementIntent.Recovery.Generation); err != nil {
		t.Fatalf("duplicate completed recovery: %v", err)
	}
	items, err := runs.ListByWorkspaceID(ctx, stored.WorkspaceID)
	if err != nil || len(items) != 2 {
		t.Fatalf("duplicate recovery created another Run: items=%+v err=%v", items, err)
	}
}

func TestReplacementRecoveryFailurePersistsNextGeneration(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-failure")
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	runs := memory.NewRunRepository()
	predecessor := replacementRecoveryTestRun(t, "ws-failure", "request-failure")
	if err := runs.Create(ctx, predecessor); err != nil {
		t.Fatal(err)
	}
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, func(_ magicevent.Event, result magicevent.Result) {
		result.Set("not-a-termination-result", nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeTermination) })
	biz := New()
	if err := biz.Configure(Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}

	if err := biz.deliverReplacementTermination(predecessor.WorkspaceID, predecessor.ID); err == nil {
		t.Fatal("wrong termination result was accepted")
	}
	stored, err := runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
	if err != nil {
		t.Fatal(err)
	}
	lease := stored.ReplacementIntent.Recovery
	if stored.ReplacementIntent.State != runmodel.ReplacementIntentPending || lease.State != runmodel.ReplacementRecoveryScheduled ||
		lease.Generation != 2 || lease.Attempt != 1 || !strings.Contains(lease.LastError, "invalid Runtime termination acknowledgement") || lease.DueAt == "" {
		t.Fatalf("persistent retry lease = %+v intent=%+v", lease, stored.ReplacementIntent)
	}
}

func TestReplacementRecoveryFreshOwnerRecoversRunningGeneration(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-recovery")
	hub := magicevent.NewHub(16)
	background := magictask.NewBackgroundRoutine(8)
	t.Cleanup(func() {
		background.Shutdown(context.Background())
		hub.Terminate(context.Background())
	})
	runs := memory.NewRunRepository()
	predecessor := replacementRecoveryTestRun(t, "ws-recovery", "request-recovery")
	predecessor.ReplacementIntent.Recovery.State = runmodel.ReplacementRecoveryRunning
	predecessor.ReplacementIntent.Recovery.DueAt = ""
	if !predecessor.ReplacementIntent.ValidFor(predecessor) {
		t.Fatalf("running recovery fixture is invalid: %+v", predecessor.ReplacementIntent)
	}
	if err := runs.Create(ctx, predecessor); err != nil {
		t.Fatal(err)
	}
	observeReplacementTermination(t, hub, runs, predecessor.WorkspaceID, predecessor.ID, "terminal-recovery", 11)

	biz := New()
	if err := biz.Configure(Dependencies{Runs: runs, EventHub: hub, BackgroundRoutine: background}); err != nil {
		t.Fatal(err)
	}
	biz.recoverPendingWorkspaceExecutionSuccessors()

	deadline := time.Now().Add(3 * time.Second)
	for {
		stored, err := runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.ReplacementIntent.State == runmodel.ReplacementIntentCompleted {
			if stored.ReplacementIntent.Recovery.Generation < 3 {
				t.Fatalf("fresh owner did not advance recovered generation: %+v", stored.ReplacementIntent.Recovery)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("fresh owner did not complete persisted recovery: %+v", stored.ReplacementIntent)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestReplacementRecoveryStopCancelsWakeupAndFreshOwnerRemountsPersistedLease(t *testing.T) {
	ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-stop-wakeup")
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(context.Background()) })
	runs := memory.NewRunRepository()
	predecessor := replacementRecoveryTestRun(t, "ws-stop-wakeup", "request-stop-wakeup")
	predecessor.ReplacementIntent.Recovery.DueAt = time.Now().UTC().Add(30 * time.Second).Format(time.RFC3339Nano)
	if err := runs.Create(ctx, predecessor); err != nil {
		t.Fatal(err)
	}

	startOwner := func(t *testing.T) (*Biz, magictask.BackgroundRoutine) {
		t.Helper()
		background := magictask.NewBackgroundRoutine(8)
		owner := New()
		if err := owner.Configure(Dependencies{Runs: runs, EventHub: hub, BackgroundRoutine: background}); err != nil {
			t.Fatalf("configure Run owner: %v", err)
		}
		if err := owner.Start(); err != nil {
			t.Fatalf("start Run owner: %v", err)
		}
		return owner, background
	}
	wakeupMounted := func(owner *Biz) bool {
		owner.replacementRecoveryMu.Lock()
		defer owner.replacementRecoveryMu.Unlock()
		return owner.replacementRecoveryTasks[replacementRecoveryTaskKey(predecessor.WorkspaceID, predecessor.ID)] == predecessor.ReplacementIntent.Recovery.Generation
	}
	waitForWakeup := func(t *testing.T, owner *Biz) {
		t.Helper()
		deadline := time.Now().Add(time.Second)
		for !wakeupMounted(owner) {
			if time.Now().After(deadline) {
				t.Fatal("persisted replacement recovery wakeup was not mounted")
			}
			time.Sleep(time.Millisecond)
		}
	}

	firstOwner, firstBackground := startOwner(t)
	waitForWakeup(t, firstOwner)
	started := time.Now()
	if err := firstOwner.Stop(context.Background()); err != nil {
		t.Fatalf("stop first Run owner: %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Run owner Stop waited for the durable lease due time: %v", elapsed)
	}
	if wakeupMounted(firstOwner) {
		t.Fatal("stopped Run owner retained a process-local replacement wakeup")
	}
	if !firstBackground.Shutdown(context.Background()) {
		t.Fatal("shutdown first background routine")
	}
	stored, err := runs.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ReplacementIntent == nil || stored.ReplacementIntent.Recovery.State != runmodel.ReplacementRecoveryScheduled || stored.ReplacementIntent.Recovery.Generation != predecessor.ReplacementIntent.Recovery.Generation {
		t.Fatalf("Stop mutated persisted replacement recovery authority: %+v", stored.ReplacementIntent)
	}

	secondOwner, secondBackground := startOwner(t)
	waitForWakeup(t, secondOwner)
	if err := secondOwner.Stop(context.Background()); err != nil {
		t.Fatalf("stop fresh Run owner: %v", err)
	}
	if !secondBackground.Shutdown(context.Background()) {
		t.Fatal("shutdown second background routine")
	}
}

func replacementRecoveryTestRun(t *testing.T, workspaceID common.WorkspaceID, requestID string) *runmodel.Run {
	t.Helper()
	bootstrap := restartTestBootstrap(t, workspaceID)
	item := &runmodel.Run{
		ID: "run-001", WorkspaceID: workspaceID, GoalID: "goal-old", DoDID: "dod-old",
		DefinitionRevision: "def-old", StartupInputHash: "startup-old", State: common.RunStateActive,
		Lineage: runmodel.RunLineage{LineageState: "active"},
	}
	intent := runmodel.ReplacementIntent{
		RequestID: requestID, WorkspaceID: workspaceID, GoalID: "goal-1", DoDID: "dod-1",
		DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
		Kind: runtimecontract.RuntimeTerminationSuperseded, Reason: "workspace execution restart/" + requestID,
		ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
		State:               runmodel.ReplacementIntentPending,
	}
	intent.Recovery = newReplacementRecoveryLease(intent, runmodel.ReplacementRecoveryAdministrativeTermination, 1, time.Now().UTC().Format(time.RFC3339Nano))
	item.ReplacementIntent = &intent
	if !intent.ValidFor(item) {
		t.Fatalf("replacement fixture is invalid: %+v", intent)
	}
	return item
}

func observeReplacementTermination(t *testing.T, hub magicevent.Hub, runs *memory.RunRepository, workspaceID common.WorkspaceID, runID common.RunID, terminalDeliveryID string, stateRevision uint64) {
	t.Helper()
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(orchestratorevents.RequestRuntimeTerminationCommand)
		if !ok || ev.Source() != ModuleID || cmd.WorkspaceID != workspaceID || cmd.RunID != runID || cmd.Kind != runtimecontract.RuntimeTerminationSuperseded {
			t.Errorf("termination command = source=%q data=%+v", ev.Source(), ev.Data())
			result.Set(nil, nil)
			return
		}
		ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), workspaceID)
		for attempt := 0; attempt < 8; attempt++ {
			current, err := runs.GetByWorkspaceID(ctx, workspaceID, runID)
			if err != nil {
				t.Errorf("load predecessor: %v", err)
				result.Set(nil, nil)
				return
			}
			next := runmodel.CloneRun(current)
			next.State = common.RunStateSuperseded
			next.Lineage.LineageState = "superseded"
			next.TerminalDelivery = &runtimecontract.RuntimeTerminalDeliveryIdentity{
				WorkspaceID: workspaceID, RunID: runID, TerminalDeliveryID: terminalDeliveryID,
			}
			if err := runs.CompareAndSwap(ctx, next, current.Revision); err == nil {
				result.Set(orchestratorevents.RequestRuntimeTerminationResult{TerminalDeliveryID: terminalDeliveryID, WorkspaceReleased: true, StateRevision: stateRevision}, nil)
				return
			}
		}
		t.Errorf("could not persist fake terminal Run")
		result.Set(nil, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeTermination) })
}

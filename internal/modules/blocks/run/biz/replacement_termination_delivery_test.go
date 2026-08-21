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

type conflictOnceRunRepository struct {
	*memory.RunRepository
	remaining int
}

func (r *conflictOnceRunRepository) CompareAndSwap(ctx context.Context, item *runmodel.Run, revision uint64) error {
	if r.remaining > 0 {
		r.remaining--
		return common.ErrConflict
	}
	return r.RunRepository.CompareAndSwap(ctx, item, revision)
}

type conflictByRunIDRepository struct {
	*memory.RunRepository
	runID     common.RunID
	remaining int
}

func (r *conflictByRunIDRepository) CompareAndSwap(ctx context.Context, item *runmodel.Run, revision uint64) error {
	if item != nil && item.ID == r.runID && r.remaining > 0 {
		r.remaining--
		return common.ErrConflict
	}
	return r.RunRepository.CompareAndSwap(ctx, item, revision)
}

func TestReplacementTerminationDeliveryPersistsConcreteAcknowledgement(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := &conflictOnceRunRepository{RunRepository: memory.NewRunRepository(), remaining: 1}
	successor := replacementTerminationTestRun("ws-delivery", "run-002", "run-001", "request-1")
	if err := runs.Create(ctx, successor); err != nil {
		t.Fatal(err)
	}
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(orchestratorevents.RequestRuntimeTerminationCommand)
		if !ok || ev.Source() != ModuleID || cmd.WorkspaceID != successor.WorkspaceID || cmd.RunID != "run-001" || cmd.Kind != runtimecontract.RuntimeTerminationSuperseded {
			t.Errorf("termination command = source=%q data=%+v", ev.Source(), ev.Data())
			result.Set(nil, nil)
			return
		}
		result.Set(orchestratorevents.RequestRuntimeTerminationResult{TerminalDeliveryID: "terminal-1", WorkspaceReleased: true, StateRevision: 9}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeTermination) })

	biz := New()
	if err := biz.Configure(Dependencies{Runs: runs, EventHub: hub}); err != nil {
		t.Fatal(err)
	}
	if err := biz.deliverReplacementTermination(successor.WorkspaceID, successor.ID); err != nil {
		t.Fatalf("deliver termination: %v", err)
	}
	stored, err := runs.GetByWorkspaceID(ctx, successor.WorkspaceID, successor.ID)
	if err != nil {
		t.Fatal(err)
	}
	intent := stored.ReplacementIntent
	if intent == nil || intent.State != runmodel.ReplacementIntentCompleted || intent.AttemptCount != 1 || intent.TerminalDeliveryID != "terminal-1" || intent.RuntimeStateRevision != 9 || intent.AcknowledgedAt == "" || intent.LastError != "" {
		t.Fatalf("acknowledged intent = %+v", intent)
	}
	if err := biz.deliverReplacementTermination(successor.WorkspaceID, successor.ID); err != nil {
		t.Fatalf("idempotent acknowledged delivery: %v", err)
	}
	stored, _ = runs.GetByWorkspaceID(ctx, successor.WorkspaceID, successor.ID)
	if stored.ReplacementIntent.AttemptCount != 1 {
		t.Fatalf("acknowledged intent was redelivered: %+v", stored.ReplacementIntent)
	}
}

func TestReplacementTerminationStartupRecoveryKeepsWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(16)
	background := magictask.NewBackgroundRoutine(8)
	t.Cleanup(func() {
		background.Shutdown(ctx)
		hub.Terminate(ctx)
	})
	runs := memory.NewRunRepository()
	for _, workspaceID := range []common.WorkspaceID{"ws-recovery-a", "ws-recovery-b"} {
		item := replacementTerminationTestRun(workspaceID, "run-002", "run-001", "request-"+string(workspaceID))
		if err := runs.Create(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	delivered := make(chan string, 2)
	observer := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	observer.Subscribe(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, func(ev magicevent.Event, result magicevent.Result) {
		cmd := ev.Data().(orchestratorevents.RequestRuntimeTerminationCommand)
		delivered <- string(cmd.WorkspaceID) + "/" + string(cmd.RunID)
		result.Set(orchestratorevents.RequestRuntimeTerminationResult{TerminalDeliveryID: "terminal-" + string(cmd.WorkspaceID), WorkspaceReleased: true, StateRevision: 7}, nil)
	})
	t.Cleanup(func() { observer.Unsubscribe(orchestratorevents.TopicOrchestratorRequestRuntimeTermination) })
	biz := New()
	if err := biz.Configure(Dependencies{Runs: runs, EventHub: hub, BackgroundRoutine: background}); err != nil {
		t.Fatal(err)
	}
	biz.recoverPendingWorkspaceExecutionSuccessors()
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case scope := <-delivered:
			seen[scope] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("recovery deliveries = %+v", seen)
		}
	}
	for _, scope := range []string{"ws-recovery-a/run-001", "ws-recovery-b/run-001"} {
		if !seen[scope] {
			t.Fatalf("missing workspace-scoped delivery %q in %+v", scope, seen)
		}
	}
	for _, workspaceID := range []common.WorkspaceID{"ws-recovery-a", "ws-recovery-b"} {
		deadline := time.Now().Add(2 * time.Second)
		for {
			item, err := runs.GetByWorkspaceID(ctx, workspaceID, "run-001")
			if err != nil {
				t.Fatal(err)
			}
			if item.ReplacementIntent.State == runmodel.ReplacementIntentCompleted {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("workspace %s intent not acknowledged: %+v", workspaceID, item.ReplacementIntent)
			}
			time.Sleep(time.Millisecond)
		}
	}
}

func TestReplacementTerminationDeliveryRejectsWrongResultAndKeepsPendingEvidence(t *testing.T) {
	ctx := context.Background()
	hub := magicevent.NewHub(8)
	t.Cleanup(func() { hub.Terminate(ctx) })
	runs := memory.NewRunRepository()
	successor := replacementTerminationTestRun("ws-wrong-result", "run-002", "run-001", "request-wrong")
	successor.ReplacementIntent.State = runmodel.ReplacementIntentPending
	successor.ReplacementIntent.SuccessorRunID = ""
	successor.ReplacementIntent.TerminalDeliveryID = ""
	if err := runs.Create(ctx, successor); err != nil {
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
	if err := biz.deliverReplacementTermination(successor.WorkspaceID, successor.ID); err == nil {
		t.Fatal("wrong termination result was accepted")
	}
	stored, _ := runs.GetByWorkspaceID(ctx, successor.WorkspaceID, successor.ID)
	intent := stored.ReplacementIntent
	if intent == nil || intent.State != runmodel.ReplacementIntentPending || intent.AttemptCount != 1 || !strings.Contains(intent.LastError, "invalid Runtime termination acknowledgement") {
		t.Fatalf("pending failure evidence = %+v", intent)
	}
}

func TestReplacementTerminalDeliveryRecoversEverySuccessorPersistenceCutPoint(t *testing.T) {
	tests := []struct {
		name       string
		failRunID  common.RunID
		failCount  int
		wantLinked bool
	}{
		{name: "terminal delivery persisted before successor create"},
		{name: "successor created before predecessor linkage", failRunID: "run-001", failCount: 8},
		{name: "predecessor linked before successor activation", failRunID: "run-002", failCount: 3, wantLinked: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := common.ContextWithRuntimeWorkspaceID(context.Background(), "ws-cut-point")
			hub := magicevent.NewHub(16)
			t.Cleanup(func() { hub.Terminate(context.Background()) })
			baseRuns := memory.NewRunRepository()
			predecessor := terminalReplacementTestRun(t, "ws-cut-point", "restart-cut-point")
			if err := baseRuns.Create(ctx, predecessor); err != nil {
				t.Fatal(err)
			}

			if test.failRunID != "" {
				faultingRuns := &conflictByRunIDRepository{
					RunRepository: baseRuns, runID: test.failRunID, remaining: test.failCount,
				}
				faultingOwner := New()
				if err := faultingOwner.Configure(Dependencies{Runs: faultingRuns, EventHub: hub}); err != nil {
					t.Fatal(err)
				}
				if _, err := faultingOwner.materializeReplacementAfterTerminalDelivery(ctx, predecessor, predecessor.TerminalDelivery.TerminalDeliveryID); err == nil {
					t.Fatal("fault-injected materialization unexpectedly completed")
				}
				stored, err := baseRuns.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
				if err != nil {
					t.Fatal(err)
				}
				if gotLinked := stored.ReplacementIntent.SuccessorRunID != ""; gotLinked != test.wantLinked {
					t.Fatalf("pre-restart successor linkage = %t, want %t; intent=%+v", gotLinked, test.wantLinked, stored.ReplacementIntent)
				}
				successor, err := baseRuns.GetByWorkspaceID(ctx, predecessor.WorkspaceID, "run-002")
				if err != nil || successor.State != common.RunStatePending {
					t.Fatalf("persisted cut-point successor = %+v err=%v", successor, err)
				}
			}

			restartedOwner := New()
			if err := restartedOwner.Configure(Dependencies{Runs: baseRuns, EventHub: hub}); err != nil {
				t.Fatal(err)
			}
			restartedOwner.recoverPendingWorkspaceExecutionSuccessors()

			items, err := baseRuns.ListByWorkspaceID(ctx, predecessor.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 2 {
				t.Fatalf("recovery created %d Runs, want predecessor plus one successor: %+v", len(items), items)
			}
			storedPredecessor, err := baseRuns.GetByWorkspaceID(ctx, predecessor.WorkspaceID, predecessor.ID)
			if err != nil {
				t.Fatal(err)
			}
			successorID := storedPredecessor.ReplacementIntent.SuccessorRunID
			if successorID != "run-002" || storedPredecessor.Lineage.SuccessorRunID != successorID || storedPredecessor.ReplacementIntent.State != runmodel.ReplacementIntentSuccessorCreated {
				t.Fatalf("recovered predecessor lineage = %+v intent=%+v", storedPredecessor.Lineage, storedPredecessor.ReplacementIntent)
			}
			successor, err := baseRuns.GetByWorkspaceID(ctx, predecessor.WorkspaceID, successorID)
			if err != nil {
				t.Fatal(err)
			}
			if successor.State != common.RunStateActive || successor.Lineage.PredecessorRunID != predecessor.ID || successor.ActivationRequestID != "restart-cut-point" {
				t.Fatalf("recovered successor = %+v", successor)
			}

			// Repeating startup recovery must not create or reactivate another Run.
			restartedOwner.recoverPendingWorkspaceExecutionSuccessors()
			items, err = baseRuns.ListByWorkspaceID(ctx, predecessor.WorkspaceID)
			if err != nil || len(items) != 2 {
				t.Fatalf("duplicate recovery Runs = %+v err=%v", items, err)
			}
		})
	}
}

func terminalReplacementTestRun(t *testing.T, workspaceID common.WorkspaceID, requestID string) *runmodel.Run {
	t.Helper()
	bootstrap := restartTestBootstrap(t, workspaceID)
	terminal := &runtimecontract.RuntimeTerminalDeliveryIdentity{
		WorkspaceID: workspaceID, RunID: "run-001", TerminalDeliveryID: "terminal-cut-point",
	}
	return &runmodel.Run{
		ID: "run-001", WorkspaceID: workspaceID, GoalID: "goal-old", DoDID: "dod-old",
		DefinitionRevision: "def-old", StartupInputHash: "startup-old", State: common.RunStateSuperseded,
		Lineage: runmodel.RunLineage{LineageState: "superseded"}, TerminalDelivery: terminal,
		ReplacementIntent: &runmodel.ReplacementIntent{
			RequestID: requestID, WorkspaceID: workspaceID, GoalID: "goal-1", DoDID: "dod-1",
			DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
			Kind: runtimecontract.RuntimeTerminationSuperseded, Reason: "workspace execution restart/" + requestID,
			ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
			State:               runmodel.ReplacementIntentPending,
		},
	}
}

func replacementTerminationTestRun(workspaceID common.WorkspaceID, successorRunID, predecessorRunID common.RunID, requestID string) *runmodel.Run {
	bootstrap := runtimecontract.RuntimeBootstrap{Definition: runtimecontract.RuntimeDefinitionContext{StartupInputHash: "startup-input-test", GoalID: "goal-1", DoDID: "dod-1", Revision: "def-1"}}
	item := &runmodel.Run{
		ID: predecessorRunID, WorkspaceID: workspaceID, State: common.RunStateActive,
		Lineage: runmodel.RunLineage{SuccessorRunID: successorRunID, LineageState: "active"},
	}
	item.ReplacementIntent = &runmodel.ReplacementIntent{
		RequestID: requestID, WorkspaceID: workspaceID, GoalID: "goal-1", DoDID: "dod-1", DefinitionRevision: "def-1", StartupInputHash: "startup-input-test", Bootstrap: &bootstrap,
		Kind: runtimecontract.RuntimeTerminationSuperseded, Reason: "workspace execution restart/" + requestID,
		ReconciliationScope: runtimecontract.RuntimeCWIReconciliationScopeWorkspaceFull,
		State:               runmodel.ReplacementIntentSuccessorCreated, SuccessorRunID: successorRunID, TerminalDeliveryID: "terminal-before-ack",
	}
	return item
}

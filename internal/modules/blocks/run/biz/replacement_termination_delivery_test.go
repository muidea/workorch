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
	intent := stored.ReplacementTerminationIntent
	if intent == nil || intent.State != runmodel.ReplacementTerminationIntentAcknowledged || intent.AttemptCount != 1 || intent.TerminalDeliveryID != "terminal-1" || intent.RuntimeStateRevision != 9 || intent.AcknowledgedAt == "" || intent.LastError != "" {
		t.Fatalf("acknowledged intent = %+v", intent)
	}
	if err := biz.deliverReplacementTermination(successor.WorkspaceID, successor.ID); err != nil {
		t.Fatalf("idempotent acknowledged delivery: %v", err)
	}
	stored, _ = runs.GetByWorkspaceID(ctx, successor.WorkspaceID, successor.ID)
	if stored.ReplacementTerminationIntent.AttemptCount != 1 {
		t.Fatalf("acknowledged intent was redelivered: %+v", stored.ReplacementTerminationIntent)
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
			item, err := runs.GetByWorkspaceID(ctx, workspaceID, "run-002")
			if err != nil {
				t.Fatal(err)
			}
			if item.ReplacementTerminationIntent.State == runmodel.ReplacementTerminationIntentAcknowledged {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("workspace %s intent not acknowledged: %+v", workspaceID, item.ReplacementTerminationIntent)
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
	intent := stored.ReplacementTerminationIntent
	if intent == nil || intent.State != runmodel.ReplacementTerminationIntentPending || intent.AttemptCount != 1 || !strings.Contains(intent.LastError, "invalid Runtime termination acknowledgement") {
		t.Fatalf("pending failure evidence = %+v", intent)
	}
}

func replacementTerminationTestRun(workspaceID common.WorkspaceID, successorRunID, predecessorRunID common.RunID, requestID string) *runmodel.Run {
	item := &runmodel.Run{
		ID: successorRunID, WorkspaceID: workspaceID, State: common.RunStatePending,
		ActivationRequestID: requestID,
		Lineage:             runmodel.RunLineage{PredecessorRunID: predecessorRunID, LineageState: "active"},
	}
	item.ReplacementTerminationIntent = &runmodel.ReplacementTerminationIntent{
		RequestID: "workspace-execution/restart/" + requestID, PredecessorRunID: predecessorRunID,
		Kind: runtimecontract.RuntimeTerminationSuperseded, Reason: "workspace execution restart/" + requestID,
		State: runmodel.ReplacementTerminationIntentPending,
	}
	return item
}

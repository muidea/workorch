package workspace

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func newWorkspaceLifecycleBiz(t *testing.T) (*Biz, *memory.WorkspaceRepository) {
	t.Helper()
	workspaces := memory.NewWorkspaceRepository()
	hub := magicevent.NewHub(16)
	biz, err := New(hub, task.NewBackgroundRoutine(4), Dependencies{
		Workspaces: workspaces,
		Goals:      memory.NewGoalRepository(),
		DoDs:       memory.NewDoDRepository(),
		Templates:  memory.NewWorkspaceTemplateRepository(),
		EventHub:   hub,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if err := biz.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { biz.Teardown(context.Background()) })
	return biz, workspaces
}

func TestReduceRunProjectionFactSetsActiveRun(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}
	tr, err := reduceRunProjectionFact(current, workspaceevents.ApplyRunProjectionFactCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "blocks/run", CausationID: "cause-1", RuntimeStateRevision: 1, RuntimeStateRef: "runtime-state/ws-1/run-1/1", RunState: common.RunStateActive,
	})
	if err != nil || tr.Noop || tr.ActiveRunID != "run-1" || tr.State != common.WorkspaceStateActive {
		t.Fatalf("transition=%+v err=%v", tr, err)
	}
}

func TestReduceRunProjectionFactAcceptsInitialActiveBeforeSM2Initialization(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}
	tr, err := reduceRunProjectionFact(current, workspaceevents.ApplyRunProjectionFactCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "blocks/run", CausationID: "run-state/run-1/0",
		RunState: common.RunStateActive,
	})
	if err != nil || tr.Noop || tr.ActiveRunID != "run-1" {
		t.Fatalf("initial activation transition=%+v err=%v", tr, err)
	}
}

func TestReduceRunProjectionFactDuplicateIsNoop(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-1"}
	tr, err := reduceRunProjectionFact(current, workspaceevents.ApplyRunProjectionFactCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "blocks/run", CausationID: "cause-1", RuntimeStateRevision: 1, RuntimeStateRef: "runtime-state/ws-1/run-1/1", RunState: common.RunStateActive,
	})
	if err != nil || !tr.Noop {
		t.Fatalf("expected noop, got %+v err=%v", tr, err)
	}
}

func TestReduceRunProjectionFactDoesNotStealOtherLiveRun(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-live"}
	_, err := reduceRunProjectionFact(current, workspaceevents.ApplyRunProjectionFactCommand{
		WorkspaceID: "ws-1", RunID: "run-other", SourceOwner: "blocks/run", CausationID: "cause-1", RuntimeStateRevision: 1, RuntimeStateRef: "runtime-state/ws-1/run-other/1", RunState: common.RunStateActive,
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
}

func freshActivationSlotReleaseFact(runID common.RunID) workspaceevents.ReleaseFreshActivationRunSlotFactCommand {
	return workspaceevents.ReleaseFreshActivationRunSlotFactCommand{
		WorkspaceID:                        "ws-1",
		RunID:                              runID,
		SourceOwner:                        "blocks/run",
		CausationID:                        "run-state/" + string(runID) + "/7/fresh_activation_required",
		RuntimeStateRevision:               7,
		RuntimeStateRef:                    "runtime-state/ws-1/" + string(runID) + "/7",
		RuntimeActivationDisposition:       runmodel.RuntimeActivationDispositionFreshActivationRequired,
		ActualRuntimeStateSchemaVersion:    runtimecontract.RuntimeStateSchemaVersionV2,
		SupportedRuntimeStateSchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
	}
}

func TestReduceFreshActivationRunSlotFactReleasesOnlyMatchingActiveRun(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateBlockedOnApproval, ActiveRunID: "run-old"}
	tr, err := reduceFreshActivationRunSlotFact(current, freshActivationSlotReleaseFact("run-old"))
	if err != nil || tr.Noop || !tr.ClearActive || tr.State != common.WorkspaceStateActive {
		t.Fatalf("transition=%+v err=%v", tr, err)
	}
	next := applyWorkspaceProjection(current, tr)
	if next.ActiveRunID != "" || next.State != common.WorkspaceStateActive {
		t.Fatalf("released workspace=%+v", next)
	}

	duplicate, err := reduceFreshActivationRunSlotFact(next, freshActivationSlotReleaseFact("run-old"))
	if err != nil || !duplicate.Noop {
		t.Fatalf("duplicate=%+v err=%v", duplicate, err)
	}
}

func TestReduceFreshActivationRunSlotFactCannotReleaseNewerRun(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-new"}
	tr, err := reduceFreshActivationRunSlotFact(current, freshActivationSlotReleaseFact("run-old"))
	if err != nil || !tr.Noop || tr.ClearActive {
		t.Fatalf("stale transition=%+v err=%v", tr, err)
	}
}

func TestWorkspaceFreshActivationNotificationDoesNotReleaseSlotWithoutExplicitRestart(t *testing.T) {
	biz, workspaces := newWorkspaceLifecycleBiz(t)
	ctx := context.Background()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-old"}); err != nil {
		t.Fatal(err)
	}
	biz.handleRunStateChanged(magicevent.NewEventWithContext(
		runevents.TopicRunStateChanged, runevents.CommandDestination, runevents.NotificationDestination, magicevent.NewHeader(), ctx,
		runevents.RunStateChangedData{
			WorkspaceID:                        "ws-1",
			RunID:                              "run-old",
			State:                              common.RunStateActive,
			CausationID:                        "run-state/run-old/7/fresh_activation_required",
			RuntimeStateRevision:               7,
			RuntimeStateRef:                    "runtime-state/ws-1/run-old/7",
			RuntimeActivationDisposition:       runmodel.RuntimeActivationDispositionFreshActivationRequired,
			ActualRuntimeStateSchemaVersion:    runtimecontract.RuntimeStateSchemaVersionV2,
			SupportedRuntimeStateSchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
		},
	), nil)
	stored, err := workspaces.Get(ctx, "ws-1")
	if err != nil || stored.ActiveRunID != "run-old" {
		t.Fatalf("fresh notification changed workspace=%+v err=%v; only explicit restart may release the slot", stored, err)
	}
}

func TestReduceWorkspaceTerminalDeliveryReleasesOwnRun(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-1"}
	tr, err := reduceWorkspaceTerminalDelivery(current, workspaceevents.ApplyRuntimeTerminalDeliveryCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "terminal-delivery/td-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1",
		Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"},
	})
	if err != nil || tr.Noop || !tr.ClearActive {
		t.Fatalf("transition=%+v err=%v", tr, err)
	}
}

func TestReduceWorkspaceTerminalDeliveryDoesNotReleaseOtherLiveRun(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-live"}
	tr, err := reduceWorkspaceTerminalDelivery(current, workspaceevents.ApplyRuntimeTerminalDeliveryCommand{
		WorkspaceID: "ws-1", RunID: "run-old", SourceOwner: "kernel/orchestrator",
		CausationID: "terminal-delivery/td-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1",
		Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"},
	})
	if !errors.Is(err, common.ErrConflict) || tr.Noop {
		t.Fatalf("expected fenced conflict for other live run, got %+v err=%v", tr, err)
	}
}

func TestReduceWorkspaceTerminalDeliveryDuplicateReleaseIsNoop(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: ""}
	tr, err := reduceWorkspaceTerminalDelivery(current, workspaceevents.ApplyRuntimeTerminalDeliveryCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		CausationID: "terminal-delivery/td-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1",
		Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"},
	})
	if err != nil || tr.Noop || tr.TerminalDelivery == nil {
		t.Fatalf("expected initial release fence, got %+v err=%v", tr, err)
	}
}

func TestWorkspaceLifecycleHandlerProjectionAndRelease(t *testing.T) {
	biz, workspaces := newWorkspaceLifecycleBiz(t)
	ctx := context.Background()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}

	proj, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceCommandApplyRunProjectionFact, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
		workspaceevents.ApplyRunProjectionFactCommand{
			WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "blocks/run", CausationID: "cause-1", RuntimeStateRevision: 1, RuntimeStateRef: "runtime-state/ws-1/run-1/1", RunState: common.RunStateActive,
		},
	))
	if err != nil || !proj.Transitioned || proj.Workspace.ActiveRunID != "run-1" {
		t.Fatalf("projection=%+v err=%v", proj, err)
	}

	// Duplicate projection is idempotent.
	dup, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceCommandApplyRunProjectionFact, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
		workspaceevents.ApplyRunProjectionFactCommand{
			WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "blocks/run", CausationID: "cause-1", RuntimeStateRevision: 1, RuntimeStateRef: "runtime-state/ws-1/run-1/1", RunState: common.RunStateActive,
		},
	))
	if err != nil || dup.Transitioned {
		t.Fatalf("duplicate projection=%+v err=%v", dup, err)
	}

	term, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceCommandApplyTerminalDelivery, "kernel/orchestrator", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
		workspaceevents.ApplyRuntimeTerminalDeliveryCommand{
			WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
			CausationID: "terminal-delivery/td-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1",
			Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"},
		},
	))
	if err != nil || !term.Transitioned || term.Workspace.ActiveRunID != "" {
		t.Fatalf("terminal=%+v err=%v", term, err)
	}
	if term.DeliveryAck == nil || term.DeliveryAck.AcknowledgedState != runtimecontract.RuntimeDeliveryAcknowledgedWorkspaceReleased {
		t.Fatalf("ack=%+v", term.DeliveryAck)
	}

	// Repeated release is idempotent.
	again, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceCommandApplyTerminalDelivery, "kernel/orchestrator", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
		workspaceevents.ApplyRuntimeTerminalDeliveryCommand{
			WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
			CausationID: "terminal-delivery/td-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1",
			Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"},
		},
	))
	if err != nil || again.Transitioned {
		t.Fatalf("repeated release=%+v err=%v", again, err)
	}
}

func TestWorkspaceLifecycleHandlerReleasesMatchingFreshActivationRunSlot(t *testing.T) {
	biz, workspaces := newWorkspaceLifecycleBiz(t)
	ctx := context.Background()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-old"}); err != nil {
		t.Fatal(err)
	}
	released, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceCommandReleaseFreshActivationRunSlot, workspaceevents.FreshActivationFactForwarderSource, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
		freshActivationSlotReleaseFact("run-old"),
	))
	if err != nil || !released.Transitioned || released.Workspace.ActiveRunID != "" || released.Workspace.State != common.WorkspaceStateActive {
		t.Fatalf("fresh release=%+v err=%v", released, err)
	}
	// A delayed old fact is harmless after another owner-local projection has
	// assigned the slot to a newer Run.
	if err := workspaces.Update(ctx, &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-new", Revision: released.Workspace.Revision}); err != nil {
		t.Fatalf("seed new active run: %v", err)
	}
	stale, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceCommandReleaseFreshActivationRunSlot, workspaceevents.FreshActivationFactForwarderSource, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
		freshActivationSlotReleaseFact("run-old"),
	))
	if err != nil || stale.Transitioned || stale.Workspace.ActiveRunID != "run-new" {
		t.Fatalf("stale fresh release=%+v err=%v", stale, err)
	}
}

func TestWorkspaceLifecycleHandlerRejectsForgedCrossOwnerSources(t *testing.T) {
	biz, workspaces := newWorkspaceLifecycleBiz(t)
	ctx := context.Background()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	_, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceCommandApplyRunProjectionFact, "blocks/run", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
		workspaceevents.ApplyRunProjectionFactCommand{
			WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "blocks/run", CausationID: "run-state/run-1/0", RunState: common.RunStateActive,
		},
	))
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("forged Run projection source err=%v", err)
	}
	_, err = biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceCommandReleaseFreshActivationRunSlot, "blocks/run", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
		freshActivationSlotReleaseFact("run-1"),
	))
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("forged fresh activation release source err=%v", err)
	}
	_, err = biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceCommandApplyTerminalDelivery, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
		workspaceevents.ApplyRuntimeTerminalDeliveryCommand{
			WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator", CausationID: "terminal-delivery/td-1",
			TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1",
			Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"},
		},
	))
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("forged terminal source err=%v", err)
	}
	stored, err := workspaces.Get(ctx, "ws-1")
	if err != nil || stored.ActiveRunID != "" {
		t.Fatalf("forged source changed workspace=%+v err=%v", stored, err)
	}
}

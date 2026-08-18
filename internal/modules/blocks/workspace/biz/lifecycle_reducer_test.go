package workspace

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository/memory"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func workspaceTerminalDeliveryTestCommand(identity runtimecontract.RuntimeTerminalDeliveryIdentity, causationID string) workspaceevents.RuntimeTerminalDeliveryFact {
	return workspaceevents.RuntimeTerminalDeliveryFact{RuntimeTerminalDeliveryFact: runtimecontract.RuntimeTerminalDeliveryFact{
		RuntimeTerminalDeliveryIdentity: identity,
		SourceOwner:                     "kernel/orchestrator",
		CausationID:                     causationID,
	}}
}

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
	tr, err := reduceRunProjectionFact(current, workspaceevents.RunProjectionFact{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "blocks/run", CausationID: "cause-1", RuntimeStateRevision: 1, RuntimeStateRef: "runtime-state/ws-1/run-1/1", RunState: common.RunStateActive,
	})
	if err != nil || tr.Noop || tr.ActiveRunID != "run-1" || tr.State != common.WorkspaceStateActive {
		t.Fatalf("transition=%+v err=%v", tr, err)
	}
}

func TestReduceRunProjectionFactAcceptsInitialActiveBeforeSM2Initialization(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}
	tr, err := reduceRunProjectionFact(current, workspaceevents.RunProjectionFact{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "blocks/run", CausationID: "run-state/run-1/0",
		RunState: common.RunStateActive,
	})
	if err != nil || tr.Noop || tr.ActiveRunID != "run-1" {
		t.Fatalf("initial activation transition=%+v err=%v", tr, err)
	}
}

func TestReduceRunProjectionFactDuplicateIsNoop(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-1"}
	tr, err := reduceRunProjectionFact(current, workspaceevents.RunProjectionFact{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "blocks/run", CausationID: "cause-1", RuntimeStateRevision: 1, RuntimeStateRef: "runtime-state/ws-1/run-1/1", RunState: common.RunStateActive,
	})
	if err != nil || !tr.Noop {
		t.Fatalf("expected noop, got %+v err=%v", tr, err)
	}
}

func TestReduceRunProjectionFactDoesNotStealOtherLiveRun(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-live"}
	_, err := reduceRunProjectionFact(current, workspaceevents.RunProjectionFact{
		WorkspaceID: "ws-1", RunID: "run-other", SourceOwner: "blocks/run", CausationID: "cause-1", RuntimeStateRevision: 1, RuntimeStateRef: "runtime-state/ws-1/run-other/1", RunState: common.RunStateActive,
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err=%v", err)
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
			WorkspaceID:                  "ws-1",
			RunID:                        "run-old",
			State:                        common.RunStateActive,
			CausationID:                  "run-state/run-old/7/fresh_activation_required",
			RuntimeStateRevision:         7,
			RuntimeStateRef:              "runtime-state/ws-1/run-old/7",
			RuntimeActivationDisposition: runtimecontract.RuntimeActivationDispositionFreshActivationRequired,
			RuntimeStateIncompatibility: runtimecontract.RuntimeStateIncompatibility{
				Reason:                 runtimecontract.RuntimeStateIncompatibilityReasonSchemaUnsupported,
				ActualSchemaVersion:    2,
				SupportedSchemaVersion: runtimecontract.RuntimeStateSchemaVersionCurrent,
			},
		},
	), nil)
	stored, err := workspaces.Get(ctx, "ws-1")
	if err != nil || stored.ActiveRunID != "run-old" {
		t.Fatalf("fresh notification changed workspace=%+v err=%v; only explicit restart may release the slot", stored, err)
	}
}

func TestReduceWorkspaceTerminalDeliveryReleasesOwnRun(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-1"}
	tr, err := reduceWorkspaceTerminalDelivery(current, workspaceTerminalDeliveryTestCommand(runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}}, "terminal-delivery/td-1"))
	if err != nil || tr.Noop || !tr.ClearActive {
		t.Fatalf("transition=%+v err=%v", tr, err)
	}
}

func TestReduceWorkspaceTerminalDeliveryDoesNotReleaseOtherLiveRun(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-live"}
	tr, err := reduceWorkspaceTerminalDelivery(current, workspaceTerminalDeliveryTestCommand(runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-old", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}}, "terminal-delivery/td-1"))
	if !errors.Is(err, common.ErrConflict) || tr.Noop {
		t.Fatalf("expected fenced conflict for other live run, got %+v err=%v", tr, err)
	}
}

func TestReduceWorkspaceTerminalDeliveryDuplicateReleaseIsNoop(t *testing.T) {
	identity := runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}}
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, TerminalDelivery: &identity}
	tr, err := reduceWorkspaceTerminalDelivery(current, workspaceTerminalDeliveryTestCommand(identity, "terminal-delivery/td-1"))
	if err != nil || !tr.Noop {
		t.Fatalf("duplicate transition=%+v err=%v, want noop", tr, err)
	}
}

func TestReduceWorkspaceTerminalDeliveryRejectsConflictingPersistedFence(t *testing.T) {
	identity := runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}}
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, TerminalDelivery: &identity}
	conflicting := identity
	conflicting.TerminalDeliveryID = "td-2"
	tr, err := reduceWorkspaceTerminalDelivery(current, workspaceTerminalDeliveryTestCommand(conflicting, "terminal-delivery/td-2"))
	if !errors.Is(err, common.ErrConflict) || tr.Noop || tr.TerminalDelivery != nil || tr.ClearActive {
		t.Fatalf("transition=%+v err=%v, want conflicting fence rejection", tr, err)
	}
}

func TestReduceWorkspaceTerminalDeliveryRejectsUnownedEmptySlot(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}
	tr, err := reduceWorkspaceTerminalDelivery(current, workspaceTerminalDeliveryTestCommand(runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}}, "terminal-delivery/td-1"))
	if !errors.Is(err, common.ErrConflict) || tr.TerminalDelivery != nil || tr.ClearActive {
		t.Fatalf("transition=%+v err=%v, want conflict without acknowledgement", tr, err)
	}
}

func TestWorkspaceLifecycleHandlerProjectionAndRelease(t *testing.T) {
	biz, workspaces := newWorkspaceLifecycleBiz(t)
	ctx := context.Background()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}

	proj, err := biz.applyRunProjection(ctx,
		workspaceevents.RunProjectionFact{
			WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "blocks/run", CausationID: "cause-1", RuntimeStateRevision: 1, RuntimeStateRef: "runtime-state/ws-1/run-1/1", RunState: common.RunStateActive,
		},
	)
	if err != nil || !proj.Transitioned || proj.Workspace.ActiveRunID != "run-1" {
		t.Fatalf("projection=%+v err=%v", proj, err)
	}

	// Duplicate projection is idempotent.
	dup, err := biz.applyRunProjection(ctx,
		workspaceevents.RunProjectionFact{
			WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "blocks/run", CausationID: "cause-1", RuntimeStateRevision: 1, RuntimeStateRef: "runtime-state/ws-1/run-1/1", RunState: common.RunStateActive,
		},
	)
	if err != nil || dup.Transitioned {
		t.Fatalf("duplicate projection=%+v err=%v", dup, err)
	}

	term, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceFactTerminalDelivery, "kernel/orchestrator", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
		workspaceTerminalDeliveryTestCommand(runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}}, "terminal-delivery/td-1"),
	))
	if err != nil || !term.Transitioned || term.Workspace.ActiveRunID != "" {
		t.Fatalf("terminal=%+v err=%v", term, err)
	}
	if term.DeliveryAck == nil || term.DeliveryAck.AcknowledgedState != runtimecontract.RuntimeDeliveryAcknowledgedWorkspaceReleased {
		t.Fatalf("ack=%+v", term.DeliveryAck)
	}

	// Repeated release is idempotent.
	again, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceFactTerminalDelivery, "kernel/orchestrator", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
		workspaceTerminalDeliveryTestCommand(runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}}, "terminal-delivery/td-1"),
	))
	if err != nil || again.Transitioned {
		t.Fatalf("repeated release=%+v err=%v", again, err)
	}
}

func TestWorkspaceLifecycleHandlerRejectsForgedCrossOwnerSources(t *testing.T) {
	biz, workspaces := newWorkspaceLifecycleBiz(t)
	ctx := context.Background()
	if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}); err != nil {
		t.Fatal(err)
	}
	_, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceFactTerminalDelivery, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
		workspaceTerminalDeliveryTestCommand(runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}}, "terminal-delivery/td-1"),
	))
	if !errors.Is(err, common.ErrInvalidArgument) {
		t.Fatalf("forged terminal source err=%v", err)
	}
	stored, err := workspaces.Get(ctx, "ws-1")
	if err != nil || stored.ActiveRunID != "" {
		t.Fatalf("forged source changed workspace=%+v err=%v", stored, err)
	}
}

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
		Workspaces:              workspaces,
		DeletionMaterialCleaner: workspaces,
		Goals:                   memory.NewGoalRepository(),
		DoDs:                    memory.NewDoDRepository(),
		Templates:               memory.NewWorkspaceTemplateRepository(),
		EventHub:                hub,
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

func workspaceRunProjectionTestFact(runID common.RunID, runRevision, runtimeRevision uint64, state common.RunState) workspaceRunProjectionFact {
	return workspaceRunProjectionGenerationTestFact(1, runID, runRevision, runtimeRevision, state)
}

func workspaceRunProjectionGenerationTestFact(generation uint64, runID common.RunID, runRevision, runtimeRevision uint64, state common.RunState) workspaceRunProjectionFact {
	ref := ""
	if runtimeRevision > 0 {
		ref = runevents.RuntimeStateRef("ws-1", runID, runtimeRevision)
	}
	return workspaceRunProjectionFact{
		WorkspaceID: "ws-1", WorkspaceRunGeneration: generation, RunID: runID, RunRevision: runRevision, SourceOwner: runevents.CommandDestination,
		CausationID: runevents.RunStateChangedCausationID(runID, generation, runRevision, runtimeRevision), RunState: state,
		RuntimeStateRevision: runtimeRevision, RuntimeStateRef: ref,
	}
}

func TestRunProjectionGenerationFencesTwoTerminalRunsAndVeryLateFirstRun(t *testing.T) {
	firstFact := workspaceRunProjectionGenerationTestFact(1, "run-001", 2, 1, common.RunStateActive)
	firstTransition, err := reduceRunProjectionFact(&workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}, firstFact)
	if err != nil {
		t.Fatal(err)
	}
	firstActive := applyWorkspaceProjection(&workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}, firstTransition)
	firstDelivery := workspaceTerminalDeliveryTestCommand(runtimecontract.RuntimeTerminalDeliveryIdentity{
		WorkspaceID: "ws-1", RunID: "run-001", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1",
		DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1",
		Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "first"},
	}, "terminal-delivery/td-1")
	firstTerminalTransition, err := reduceWorkspaceTerminalDelivery(firstActive, firstDelivery)
	if err != nil {
		t.Fatal(err)
	}
	firstTerminal := applyWorkspaceProjection(firstActive, firstTerminalTransition)

	// The Run repository persists the Workspace watermark before the Run row,
	// so response loss may leave an intentional generation gap. Handoff uses
	// monotonic ordering and must not require adjacency.
	secondFact := workspaceRunProjectionGenerationTestFact(3, "run-003", 2, 1, common.RunStateActive)
	secondTransition, err := reduceRunProjectionFact(firstTerminal, secondFact)
	if err != nil {
		t.Fatal(err)
	}
	secondActive := applyWorkspaceProjection(firstTerminal, secondTransition)
	secondDelivery := workspaceTerminalDeliveryTestCommand(runtimecontract.RuntimeTerminalDeliveryIdentity{
		WorkspaceID: "ws-1", RunID: "run-003", TerminalDeliveryID: "td-2", AuthoritySnapshotID: "auth-2",
		DefinitionRevision: "def-2", EvidenceRevision: "evidence-2", DispatchID: "dispatch-2",
		Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "second"},
	}, "terminal-delivery/td-2")
	secondTerminalTransition, err := reduceWorkspaceTerminalDelivery(secondActive, secondDelivery)
	if err != nil {
		t.Fatal(err)
	}
	secondTerminal := applyWorkspaceProjection(secondActive, secondTerminalTransition)

	veryLateFirst := workspaceRunProjectionGenerationTestFact(1, "run-001", 99, 98, common.RunStateActive)
	lateTransition, err := reduceRunProjectionFact(secondTerminal, veryLateFirst)
	if err != nil || !lateTransition.Noop {
		t.Fatalf("very-late first generation transition=%+v err=%v", lateTransition, err)
	}
	if secondTerminal.ActiveRunID != "" || secondTerminal.RunProjectionCursor.WorkspaceRunGeneration != 3 ||
		secondTerminal.RunProjectionCursor.RunID != "run-003" || secondTerminal.TerminalDelivery == nil || secondTerminal.TerminalDelivery.RunID != "run-003" {
		t.Fatalf("second terminal fence drifted: %+v", secondTerminal)
	}
}

func TestRunProjectionGenerationRejectsIdentityAndHandoffConflicts(t *testing.T) {
	first := workspaceProjectedRunTestState("run-1")
	if _, err := reduceRunProjectionFact(first, workspaceRunProjectionGenerationTestFact(1, "run-other", 2, 1, common.RunStateActive)); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("same generation/different Run err=%v, want conflict", err)
	}
	if _, err := reduceRunProjectionFact(first, workspaceRunProjectionGenerationTestFact(2, "run-2", 2, 1, common.RunStateActive)); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("new generation without terminal fence err=%v, want conflict", err)
	}
	identity := runtimecontract.RuntimeTerminalDeliveryIdentity{
		WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1",
		DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1",
		Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"},
	}
	terminalTransition, err := reduceWorkspaceTerminalDelivery(first, workspaceTerminalDeliveryTestCommand(identity, "terminal-delivery/td-1"))
	if err != nil {
		t.Fatal(err)
	}
	terminal := applyWorkspaceProjection(first, terminalTransition)
	if _, err := reduceRunProjectionFact(terminal, workspaceRunProjectionGenerationTestFact(2, "run-1", 3, 2, common.RunStateActive)); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("same Run/higher generation err=%v, want conflict", err)
	}
}

func workspaceRunStateChangedTestData(runID common.RunID, runRevision, runtimeRevision uint64, state common.RunState) runevents.RunStateChangedData {
	fact := workspaceRunProjectionTestFact(runID, runRevision, runtimeRevision, state)
	return runevents.RunStateChangedData{
		WorkspaceID: fact.WorkspaceID, WorkspaceRunGeneration: fact.WorkspaceRunGeneration,
		RunID: fact.RunID, RunRevision: fact.RunRevision, State: fact.RunState,
		CausationID: fact.CausationID, RuntimeStateRevision: fact.RuntimeStateRevision, RuntimeStateRef: fact.RuntimeStateRef,
	}
}

func workspaceProjectedRunTestState(runID common.RunID) *workspacemodel.Workspace {
	fact := workspaceRunProjectionTestFact(runID, 2, 1, common.RunStateActive)
	return applyWorkspaceProjection(&workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}, workspaceProjectionTransition{
		ActiveRunID: runID,
		State:       common.WorkspaceStateActive,
		RunProjection: &workspacemodel.WorkspaceRunProjectionCursor{
			WorkspaceRunGeneration: fact.WorkspaceRunGeneration,
			RunID:                  fact.RunID, RunRevision: fact.RunRevision, RuntimeStateRevision: fact.RuntimeStateRevision,
			RuntimeStateRef: fact.RuntimeStateRef, CausationID: fact.CausationID,
		},
	})
}

func TestWorkspaceRunProjectionFactValidationStaysOwnerPrivate(t *testing.T) {
	fact := workspaceRunProjectionTestFact("run-1", 2, 1, common.RunStateActive)
	if err := fact.validate(); err != nil {
		t.Fatalf("valid projection fact: %v", err)
	}
	preRuntime := workspaceRunProjectionTestFact("run-1", 1, 0, common.RunStateActive)
	if err := preRuntime.validate(); err != nil {
		t.Fatalf("valid pre-runtime activation projection: %v", err)
	}
	preRuntime.RunState = common.RunStateBlockedOnDecision
	if err := preRuntime.validate(); err == nil {
		t.Fatal("pre-runtime governance projection must be rejected")
	}
	fact.RunState = common.RunStateCompleted
	if err := fact.validate(); err == nil {
		t.Fatal("terminal Run state must not enter Workspace projection reducer")
	}
}

func TestReduceRunProjectionFactSetsActiveRun(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}
	tr, err := reduceRunProjectionFact(current, workspaceRunProjectionTestFact("run-1", 2, 1, common.RunStateActive))
	if err != nil || tr.Noop || tr.ActiveRunID != "run-1" || tr.State != common.WorkspaceStateActive {
		t.Fatalf("transition=%+v err=%v", tr, err)
	}
}

func TestReduceRunProjectionFactAcceptsInitialActiveBeforeSM2Initialization(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}
	tr, err := reduceRunProjectionFact(current, workspaceRunProjectionTestFact("run-1", 1, 0, common.RunStateActive))
	if err != nil || tr.Noop || tr.ActiveRunID != "run-1" {
		t.Fatalf("initial activation transition=%+v err=%v", tr, err)
	}
}

func TestReduceRunProjectionFactDuplicateIsNoop(t *testing.T) {
	fact := workspaceRunProjectionTestFact("run-1", 2, 1, common.RunStateActive)
	current := applyWorkspaceProjection(&workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive}, workspaceProjectionTransition{
		ActiveRunID: "run-1", State: common.WorkspaceStateActive,
		RunProjection: &workspacemodel.WorkspaceRunProjectionCursor{WorkspaceRunGeneration: fact.WorkspaceRunGeneration, RunID: fact.RunID, RunRevision: fact.RunRevision, RuntimeStateRevision: fact.RuntimeStateRevision, RuntimeStateRef: fact.RuntimeStateRef, CausationID: fact.CausationID},
	})
	tr, err := reduceRunProjectionFact(current, fact)
	if err != nil || !tr.Noop {
		t.Fatalf("expected noop, got %+v err=%v", tr, err)
	}
}

func TestReduceRunProjectionFactDoesNotStealOtherLiveRun(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-live"}
	_, err := reduceRunProjectionFact(current, workspaceRunProjectionTestFact("run-other", 1, 1, common.RunStateActive))
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestWorkspaceRunNotificationsNeverReleaseActiveSlot(t *testing.T) {
	values := []struct {
		name string
		data runevents.RunStateChangedData
	}{
		{name: "ordinary duplicate", data: workspaceRunStateChangedTestData("run-old", 8, 7, common.RunStateActive)},
		{name: "own governance projection", data: workspaceRunStateChangedTestData("run-old", 9, 8, common.RunStateBlockedOnDecision)},
		{name: "conflicting live run", data: workspaceRunStateChangedTestData("run-other", 2, 1, common.RunStateActive)},
	}
	for _, value := range values {
		t.Run(value.name, func(t *testing.T) {
			biz, workspaces := newWorkspaceLifecycleBiz(t)
			ctx := context.Background()
			if err := workspaces.Create(ctx, &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-old"}); err != nil {
				t.Fatal(err)
			}
			biz.handleRunStateChanged(magicevent.NewEventWithContext(
				runevents.TopicRunStateChanged, runevents.CommandDestination, runevents.NotificationDestination, magicevent.NewHeader(), ctx, value.data,
			), nil)
			stored, err := workspaces.Get(ctx, "ws-1")
			if err != nil || stored.ActiveRunID != "run-old" {
				t.Fatalf("notification changed active slot workspace=%+v err=%v", stored, err)
			}
		})
	}
}

func TestReduceWorkspaceTerminalDeliveryReleasesOwnRun(t *testing.T) {
	current := workspaceProjectedRunTestState("run-1")
	tr, err := reduceWorkspaceTerminalDelivery(current, workspaceTerminalDeliveryTestCommand(runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}}, "terminal-delivery/td-1"))
	if err != nil || tr.Noop || !tr.ClearActive {
		t.Fatalf("transition=%+v err=%v", tr, err)
	}
}

func TestReduceWorkspaceTerminalDeliveryDoesNotReleaseOtherLiveRun(t *testing.T) {
	current := workspaceProjectedRunTestState("run-live")
	tr, err := reduceWorkspaceTerminalDelivery(current, workspaceTerminalDeliveryTestCommand(runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-old", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}}, "terminal-delivery/td-1"))
	if !errors.Is(err, common.ErrConflict) || tr.Noop {
		t.Fatalf("expected fenced conflict for other live run, got %+v err=%v", tr, err)
	}
}

func TestReduceWorkspaceTerminalDeliveryDuplicateReleaseIsNoop(t *testing.T) {
	identity := runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}}
	projected := workspaceProjectedRunTestState("run-1")
	transition, err := reduceWorkspaceTerminalDelivery(projected, workspaceTerminalDeliveryTestCommand(identity, "terminal-delivery/td-1"))
	if err != nil {
		t.Fatal(err)
	}
	current := applyWorkspaceProjection(projected, transition)
	tr, err := reduceWorkspaceTerminalDelivery(current, workspaceTerminalDeliveryTestCommand(identity, "terminal-delivery/td-1"))
	if err != nil || !tr.Noop {
		t.Fatalf("duplicate transition=%+v err=%v, want noop", tr, err)
	}
}

func TestReduceWorkspaceTerminalDeliveryRejectsConflictingPersistedFence(t *testing.T) {
	identity := runtimecontract.RuntimeTerminalDeliveryIdentity{WorkspaceID: "ws-1", RunID: "run-1", TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", DispatchID: "dispatch-1", Outcome: runtimecontract.TerminalOutcome{Kind: runtimecontract.TerminalOutcomeKindCompleted, Code: "completed", Summary: "done"}}
	projected := workspaceProjectedRunTestState("run-1")
	transition, err := reduceWorkspaceTerminalDelivery(projected, workspaceTerminalDeliveryTestCommand(identity, "terminal-delivery/td-1"))
	if err != nil {
		t.Fatal(err)
	}
	current := applyWorkspaceProjection(projected, transition)
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

	fact := workspaceRunProjectionTestFact("run-1", 2, 1, common.RunStateActive)
	proj, err := biz.applyRunProjection(ctx, fact)
	if err != nil || !proj.Transitioned || proj.Workspace.ActiveRunID != "run-1" {
		t.Fatalf("projection=%+v err=%v", proj, err)
	}

	// Duplicate projection is idempotent.
	dup, err := biz.applyRunProjection(ctx, fact)
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

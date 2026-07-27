package workspace

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/magicCommon/task"
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
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "blocks/run", CausationID: "cause-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", RunState: common.RunStateActive,
	})
	if err != nil || tr.Noop || tr.ActiveRunID != "run-1" || tr.State != common.WorkspaceStateActive {
		t.Fatalf("transition=%+v err=%v", tr, err)
	}
}

func TestReduceRunProjectionFactDuplicateIsNoop(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-1"}
	tr, err := reduceRunProjectionFact(current, workspaceevents.ApplyRunProjectionFactCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "blocks/run", CausationID: "cause-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", RunState: common.RunStateActive,
	})
	if err != nil || !tr.Noop {
		t.Fatalf("expected noop, got %+v err=%v", tr, err)
	}
}

func TestReduceRunProjectionFactDoesNotStealOtherLiveRun(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-live"}
	_, err := reduceRunProjectionFact(current, workspaceevents.ApplyRunProjectionFactCommand{
		WorkspaceID: "ws-1", RunID: "run-other", SourceOwner: "blocks/run", CausationID: "cause-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", RunState: common.RunStateActive,
	})
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestReduceWorkspaceTerminalDeliveryReleasesOwnRun(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-1"}
	tr, err := reduceWorkspaceTerminalDelivery(current, workspaceevents.ApplyRuntimeTerminalDeliveryCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1",
		Action: runtimecontract.RuntimeActionComplete, TerminalCode: "completed", TerminalSummary: "done",
	})
	if err != nil || tr.Noop || !tr.ClearActive {
		t.Fatalf("transition=%+v err=%v", tr, err)
	}
}

func TestReduceWorkspaceTerminalDeliveryDoesNotReleaseOtherLiveRun(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: "run-live"}
	tr, err := reduceWorkspaceTerminalDelivery(current, workspaceevents.ApplyRuntimeTerminalDeliveryCommand{
		WorkspaceID: "ws-1", RunID: "run-old", SourceOwner: "kernel/orchestrator",
		TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1",
		Action: runtimecontract.RuntimeActionComplete, TerminalCode: "completed", TerminalSummary: "done",
	})
	if !errors.Is(err, common.ErrConflict) || tr.Noop {
		t.Fatalf("expected fenced conflict for other live run, got %+v err=%v", tr, err)
	}
}

func TestReduceWorkspaceTerminalDeliveryDuplicateReleaseIsNoop(t *testing.T) {
	current := &workspacemodel.Workspace{ID: "ws-1", State: common.WorkspaceStateActive, ActiveRunID: ""}
	tr, err := reduceWorkspaceTerminalDelivery(current, workspaceevents.ApplyRuntimeTerminalDeliveryCommand{
		WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
		TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1",
		Action: runtimecontract.RuntimeActionComplete, TerminalCode: "completed", TerminalSummary: "done",
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
		workspaceevents.TopicWorkspaceCommandApplyRunProjectionFact, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
		workspaceevents.ApplyRunProjectionFactCommand{
			WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "blocks/run", CausationID: "cause-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", RunState: common.RunStateActive,
		},
	))
	if err != nil || !proj.Transitioned || proj.Workspace.ActiveRunID != "run-1" {
		t.Fatalf("projection=%+v err=%v", proj, err)
	}

	// Duplicate projection is idempotent.
	dup, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceCommandApplyRunProjectionFact, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
		workspaceevents.ApplyRunProjectionFactCommand{
			WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "blocks/run", CausationID: "cause-1", AuthoritySnapshotID: "auth-1", DefinitionRevision: "def-1", EvidenceRevision: "evidence-1", RunState: common.RunStateActive,
		},
	))
	if err != nil || dup.Transitioned {
		t.Fatalf("duplicate projection=%+v err=%v", dup, err)
	}

	term, err := biz.executeLifecycleCommand(magicevent.NewEventWithContext(
		workspaceevents.TopicWorkspaceCommandApplyTerminalDelivery, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
		workspaceevents.ApplyRuntimeTerminalDeliveryCommand{
			WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
			TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1",
			Action: runtimecontract.RuntimeActionComplete, TerminalCode: "completed", TerminalSummary: "done",
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
		workspaceevents.TopicWorkspaceCommandApplyTerminalDelivery, "test", workspaceevents.CommandDestination, magicevent.NewHeader(), ctx,
		workspaceevents.ApplyRuntimeTerminalDeliveryCommand{
			WorkspaceID: "ws-1", RunID: "run-1", SourceOwner: "kernel/orchestrator",
			TerminalDeliveryID: "td-1", AuthoritySnapshotID: "auth-1",
			Action: runtimecontract.RuntimeActionComplete, TerminalCode: "completed", TerminalSummary: "done",
		},
	))
	if err != nil || again.Transitioned {
		t.Fatalf("repeated release=%+v err=%v", again, err)
	}
}

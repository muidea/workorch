package runports

import (
	"context"
	"testing"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRunViewProjectsRunModel(t *testing.T) {
	t.Parallel()

	item := &runmodel.Run{
		ID:                 "run-view",
		WorkspaceID:        "ws-run-view",
		GoalID:             "goal-run-view",
		DoDID:              "dod-run-view",
		DefinitionRevision: "goal:goal-run-view|dod:dod-run-view",
		InitialStepKind:    "llm/workspace_execution",
		State:              common.RunStateActive,
		Priority:           7,
		BlockingTicketID:   "ticket-run-view",
		BlockingApprovalID: "approval-run-view",
		FailureReason:      "seeded failure",
	}

	got := RunView(item)
	if got.ID != string(item.ID) || got.WorkspaceID != string(item.WorkspaceID) {
		t.Fatalf("run view IDs = %+v, want seeded run/workspace IDs", got)
	}
	if got.GoalID != string(item.GoalID) || got.DoDID != string(item.DoDID) {
		t.Fatalf("definition IDs = goal:%q dod:%q, want seeded IDs", got.GoalID, got.DoDID)
	}
	if got.DefinitionRevision != item.DefinitionRevision {
		t.Fatalf("definitionRevision = %q, want %q", got.DefinitionRevision, item.DefinitionRevision)
	}
	if got.State != string(item.State) || got.Priority != item.Priority {
		t.Fatalf("state/priority = %q/%d, want %q/%d", got.State, got.Priority, item.State, item.Priority)
	}
	if got.BlockingTicketID != string(item.BlockingTicketID) || got.BlockingApprovalID != string(item.BlockingApprovalID) {
		t.Fatalf("blocking IDs = ticket:%q approval:%q, want seeded IDs", got.BlockingTicketID, got.BlockingApprovalID)
	}
	if got.FailureReason != item.FailureReason {
		t.Fatalf("failure reason = %q, want %q", got.FailureReason, item.FailureReason)
	}
}

func TestRunViewWithFailureReasonOverridesFailureReason(t *testing.T) {
	t.Parallel()

	got := RunViewWithFailureReason(&runmodel.Run{
		ID:            "run-view-failure",
		WorkspaceID:   "ws-run-view-failure",
		State:         common.RunStateFailed,
		FailureReason: "original",
	}, "override")
	if got.FailureReason != "override" {
		t.Fatalf("failure reason = %q, want override", got.FailureReason)
	}
}

func TestRunViewsProjectsSlice(t *testing.T) {
	t.Parallel()

	got := RunViews([]*runmodel.Run{
		{ID: "run-view-a", WorkspaceID: "ws-run-view", State: common.RunStatePending},
		{ID: "run-view-b", WorkspaceID: "ws-run-view", State: common.RunStateActive},
	})
	if len(got) != 2 || got[0].ID != "run-view-a" || got[1].ID != "run-view-b" {
		t.Fatalf("run views = %+v, want two projected views", got)
	}
}

func TestReadRunViewUsesOwnerReadAndProjectsView(t *testing.T) {
	ctx := context.Background()
	runtime := newOwnerRuntime()
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := testRuntimeExports(runtime)
	run := &runmodel.Run{
		ID:          "run-view-read",
		WorkspaceID: "ws-run-view-read",
		State:       common.RunStatePending,
	}
	if err := exports.Repositories.Runs.Create(ctx, run); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	got, err := ReadRunView(ctx, exports.Infrastructure.CommandHub, run.ID)
	if err != nil {
		t.Fatalf("ReadRunView: %v", err)
	}
	if got.ID != string(run.ID) || got.WorkspaceID != string(run.WorkspaceID) {
		t.Fatalf("run view = %+v, want seeded run identity", got)
	}
}

func TestListRunViewsByWorkspaceUsesOwnerReadAndProjectsViews(t *testing.T) {
	ctx := context.Background()
	runtime := newOwnerRuntime()
	t.Cleanup(func() {
		if err := runtime.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown runtime: %v", err)
		}
	})
	exports := testRuntimeExports(runtime)
	for _, item := range []*runmodel.Run{
		{ID: "run-view-list-a", WorkspaceID: "ws-run-view-list-a", State: common.RunStatePending},
		{ID: "run-view-list-b", WorkspaceID: "ws-run-view-list-b", State: common.RunStatePending},
	} {
		if err := exports.Repositories.Runs.Create(ctx, item); err != nil {
			t.Fatalf("seed run %s: %v", item.ID, err)
		}
	}

	got, err := ListRunViewsByWorkspace(ctx, exports.Infrastructure.CommandHub, "ws-run-view-list-b")
	if err != nil {
		t.Fatalf("ListRunViewsByWorkspace: %v", err)
	}
	if len(got) != 1 || got[0].ID != "run-view-list-b" || got[0].WorkspaceID != "ws-run-view-list-b" {
		t.Fatalf("run views = %+v, want filtered run-view-list-b", got)
	}
}

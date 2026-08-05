package tui

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestSnapshotGovernanceFiltersCurrentDisplayRun(t *testing.T) {
	snapshot := &projectioncontract.WorkspaceSnapshot{
		Workspace: projectioncontract.Workspace{
			ID:          "ws-a",
			ActiveRunID: "run-current",
		},
		Runs: []projectioncontract.Run{
			{ID: "run-old", WorkspaceID: "ws-a", State: string(common.RunStateBlockedOnDecision)},
			{ID: "run-current", WorkspaceID: "ws-a", State: string(common.RunStateActive)},
		},
		Decisions: []projectioncontract.Decision{
			{ID: "ticket-old", WorkspaceID: "ws-a", RunID: "run-old", State: "open"},
			{ID: "ticket-current", WorkspaceID: "ws-a", RunID: "run-current", State: "open"},
			{ID: "ticket-other-ws", WorkspaceID: "ws-b", RunID: "run-current", State: "open"},
		},
		Approvals: []projectioncontract.Approval{
			{ID: "approval-old", WorkspaceID: "ws-a", RunID: "run-old", State: "pending"},
			{ID: "approval-current", WorkspaceID: "ws-a", RunID: "run-current", State: "pending"},
			{ID: "approval-other-ws", WorkspaceID: "ws-b", RunID: "run-current", State: "pending"},
		},
	}

	run := selectSnapshotDisplayRun(snapshot)
	if run == nil || run.ID != "run-current" {
		t.Fatalf("display run = %+v, want current active run", run)
	}
	decisions := filterSnapshotDecisionsForRun(snapshot, run)
	if len(decisions) != 1 || decisions[0].ID != "ticket-current" {
		t.Fatalf("decisions = %+v, want current workspace/run decision only", decisions)
	}
	approvals := filterSnapshotApprovalsForRun(snapshot, run)
	if len(approvals) != 1 || approvals[0].ID != "approval-current" {
		t.Fatalf("approvals = %+v, want current workspace/run approval only", approvals)
	}
}

func TestSnapshotGovernanceWithoutActiveOrOutcomeRunIgnoresHistoricalItems(t *testing.T) {
	snapshot := &projectioncontract.WorkspaceSnapshot{
		Workspace: projectioncontract.Workspace{ID: "ws-a"},
		Runs: []projectioncontract.Run{
			{ID: "run-old", WorkspaceID: "ws-a", State: string(common.RunStateBlockedOnDecision)},
		},
		Decisions: []projectioncontract.Decision{
			{ID: "ticket-old", WorkspaceID: "ws-a", RunID: "run-old", State: "open"},
		},
		Approvals: []projectioncontract.Approval{
			{ID: "approval-old", WorkspaceID: "ws-a", RunID: "run-old", State: "pending"},
		},
	}

	run := selectSnapshotDisplayRun(snapshot)
	if run != nil {
		t.Fatalf("display run = %+v, want nil without active or outcome run", run)
	}
	if decisions := filterSnapshotDecisionsForRun(snapshot, run); len(decisions) != 0 {
		t.Fatalf("decisions = %+v, want historical decisions ignored", decisions)
	}
	if approvals := filterSnapshotApprovalsForRun(snapshot, run); len(approvals) != 0 {
		t.Fatalf("approvals = %+v, want historical approvals ignored", approvals)
	}
}

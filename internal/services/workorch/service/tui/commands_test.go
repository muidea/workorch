package tui

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
	backend "github.com/muidea/workorch/internal/pkg/workorchport"
)

func TestSnapshotGovernanceFiltersCurrentDisplayRun(t *testing.T) {
	snapshot := &backend.WorkspaceSnapshotInfo{
		Workspace: backend.WorkspaceInfo{
			ID:          "ws-a",
			ActiveRunID: "run-current",
		},
		Runs: []backend.RunInfo{
			{ID: "run-old", WorkspaceID: "ws-a", State: common.RunStateBlockedOnDecision},
			{ID: "run-current", WorkspaceID: "ws-a", State: common.RunStateRunning},
		},
		Decisions: []backend.DecisionInfo{
			{ID: "ticket-old", WorkspaceID: "ws-a", RunID: "run-old", State: "open"},
			{ID: "ticket-current", WorkspaceID: "ws-a", RunID: "run-current", State: "open"},
			{ID: "ticket-other-ws", WorkspaceID: "ws-b", RunID: "run-current", State: "open"},
		},
		Approvals: []backend.ApprovalInfo{
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
	snapshot := &backend.WorkspaceSnapshotInfo{
		Workspace: backend.WorkspaceInfo{ID: "ws-a"},
		Runs: []backend.RunInfo{
			{ID: "run-old", WorkspaceID: "ws-a", State: common.RunStateBlockedOnDecision},
		},
		Decisions: []backend.DecisionInfo{
			{ID: "ticket-old", WorkspaceID: "ws-a", RunID: "run-old", State: "open"},
		},
		Approvals: []backend.ApprovalInfo{
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

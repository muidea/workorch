package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkspaceSnapshotTaskProgressUsesExistingProgress(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-progress"},
		TaskProgress: readmodelcore.TaskProgress{
			WorkspaceID: "ws-progress",
			State:       "custom",
		},
	}

	got := WorkspaceSnapshotTaskProgress(snapshot)
	if got.State != "custom" || got.WorkspaceID != "ws-progress" {
		t.Fatalf("progress = %+v, want existing workspace progress", got)
	}
}

func TestWorkspaceTaskProgressIgnoresHistoricalBlockersWithoutActiveRun(t *testing.T) {
	t.Parallel()

	got := MapLocalizedWorkspaceTaskProgress(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{
			ID:    "ws-startup-recovered",
			State: string(common.WorkspaceStateActive),
		},
		Decisions: []readmodelcore.Decision{{
			ID:          "ticket-historical",
			WorkspaceID: "ws-startup-recovered",
			RunID:       "run-historical",
			State:       "open",
			Question:    "historical decision",
		}},
		Approvals: []readmodelcore.Approval{{
			ID:          "approval-historical",
			WorkspaceID: "ws-startup-recovered",
			RunID:       "run-historical",
			State:       "pending",
			Reason:      "historical approval",
		}},
	})

	if got.Recovery.Mode != "idle" || got.Recovery.NextAction != "start_workspace_execution" {
		t.Fatalf("recovery = %+v, want idle start action", got.Recovery)
	}
	if len(got.Blockers) != 0 {
		t.Fatalf("blockers = %+v, want no current blockers without active run", got.Blockers)
	}
}

func TestWorkspaceTaskProgressOnlyUsesActiveRunBlockers(t *testing.T) {
	t.Parallel()

	got := MapLocalizedWorkspaceTaskProgress(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{
			ID:          "ws-active-blocker",
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-current",
		},
		Decisions: []readmodelcore.Decision{
			{
				ID:          "ticket-old",
				WorkspaceID: "ws-active-blocker",
				RunID:       "run-old",
				State:       "open",
				Question:    "old decision",
			},
			{
				ID:          "ticket-dismissed",
				WorkspaceID: "ws-active-blocker",
				RunID:       "run-current",
				State:       string(common.DecisionTicketStateSuperseded),
				Question:    "dismissed decision",
			},
			{
				ID:          "ticket-current",
				WorkspaceID: "ws-active-blocker",
				RunID:       "run-current",
				State:       "open",
				Question:    "current decision",
			},
		},
	})

	if got.Recovery.BlockingTicketID != "ticket-current" || got.Recovery.Mode != "wait_decision" {
		t.Fatalf("recovery = %+v, want current active run decision", got.Recovery)
	}
	if len(got.Blockers) != 1 || got.Blockers[0].ID != "ticket-current" {
		t.Fatalf("blockers = %+v, want only current active run blocker", got.Blockers)
	}
}

func TestWorkspaceTaskProgressRequiresSameScopeOpenGovernanceBlockers(t *testing.T) {
	t.Parallel()

	got := MapLocalizedWorkspaceTaskProgress(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{
			ID:          "ws-current",
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-shared",
		},
		Decisions: []readmodelcore.Decision{
			{
				ID:          "ticket-other-workspace",
				WorkspaceID: "ws-other",
				RunID:       "run-shared",
				State:       string(common.DecisionTicketStateOpen),
			},
			{
				ID:          "ticket-not-open",
				WorkspaceID: "ws-current",
				RunID:       "run-shared",
				State:       string(common.DecisionTicketStateResolved),
			},
		},
		Approvals: []readmodelcore.Approval{
			{
				ID:          "approval-other-workspace",
				WorkspaceID: "ws-other",
				RunID:       "run-shared",
				State:       string(common.ApprovalStatePending),
			},
			{
				ID:          "approval-not-pending",
				WorkspaceID: "ws-current",
				RunID:       "run-shared",
				State:       string(common.ApprovalStateApproved),
			},
		},
	})

	if got.Recovery.Mode == "wait_decision" || got.Recovery.Mode == "wait_approval" ||
		got.Recovery.BlockingTicketID != "" || got.Recovery.BlockingApprovalID != "" ||
		len(got.Blockers) != 0 {
		t.Fatalf("progress = %+v, want no governance blockers without same-scope open ticket or pending approval", got)
	}
}

func TestWorkspaceGovernanceProjectionClearsResolvedDecisionBlocker(t *testing.T) {
	t.Parallel()

	got := ProjectWorkspaceSnapshotDetail(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{
			ID:               "ws-resolved-blocker",
			State:            string(common.WorkspaceStateActive),
			ActiveRunID:      "run-current",
			BlockingTicketID: "ticket-resolved",
		},
		Run: &readmodelcore.Run{
			ID:               "run-current",
			WorkspaceID:      "ws-resolved-blocker",
			State:            string(common.RunStateBlockedOnDecision),
			BlockingTicketID: "ticket-resolved",
		},
		TaskProgress: readmodelcore.TaskProgress{
			WorkspaceID:      "ws-resolved-blocker",
			RunID:            "run-current",
			State:            string(common.RunStateBlockedOnDecision),
			BlockingTicketID: "ticket-resolved",
			Recovery: readmodelcore.RecoveryPlan{
				Mode:             "wait_decision",
				NextAction:       "resolve_decision_ticket",
				BlockingTicketID: "ticket-resolved",
			},
		},
		Decisions: []readmodelcore.Decision{{
			ID:          "ticket-resolved",
			WorkspaceID: "ws-resolved-blocker",
			RunID:       "run-current",
			State:       string(common.DecisionTicketStateResolved),
		}},
	})

	if got.Workspace.State != string(common.WorkspaceStateActive) ||
		got.Workspace.BlockingTicketID != "" ||
		got.TaskProgress.BlockingTicketID != "" ||
		got.TaskProgress.Recovery.BlockingTicketID != "" ||
		got.TaskProgress.Recovery.Mode == "wait_decision" {
		t.Fatalf("snapshot = %+v progress = %+v, want resolved decision not projected as normal blocker", got.Workspace, got.TaskProgress)
	}
}

func TestWorkspaceGovernanceProjectionKeepsOpenDecisionBlocker(t *testing.T) {
	t.Parallel()

	got := ProjectWorkspaceSnapshotDetail(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{
			ID:               "ws-open-blocker",
			State:            string(common.WorkspaceStateActive),
			ActiveRunID:      "run-current",
			BlockingTicketID: "ticket-open",
		},
		Run: &readmodelcore.Run{
			ID:               "run-current",
			WorkspaceID:      "ws-open-blocker",
			State:            string(common.RunStateBlockedOnDecision),
			BlockingTicketID: "ticket-open",
		},
		TaskProgress: readmodelcore.TaskProgress{
			WorkspaceID: "ws-open-blocker",
			RunID:       "run-current",
			State:       string(common.RunStateBlockedOnDecision),
		},
		Decisions: []readmodelcore.Decision{{
			ID:          "ticket-open",
			WorkspaceID: "ws-open-blocker",
			RunID:       "run-current",
			State:       string(common.DecisionTicketStateOpen),
		}},
	})

	if got.Workspace.State != string(common.WorkspaceStateActive) ||
		got.Workspace.BlockingTicketID != "ticket-open" ||
		got.TaskProgress.BlockingTicketID != "ticket-open" ||
		got.TaskProgress.Recovery.Mode != "wait_decision" {
		t.Fatalf("snapshot = %+v progress = %+v, want open decision blocker retained", got.Workspace, got.TaskProgress)
	}
}

func TestWorkspaceContextBundleFromSnapshotsPrefersRunSnapshot(t *testing.T) {
	t.Parallel()

	workspaceSnapshot := readmodelcore.WorkspaceSnapshot{
		Workspace:    readmodelcore.Workspace{ID: "ws-context"},
		TaskProgress: readmodelcore.TaskProgress{WorkspaceID: "ws-context", RunID: "run-context"},
	}
	runSnapshot := &readmodelcore.RunSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-context"},
		Run:       readmodelcore.Run{ID: "run-context", WorkspaceID: "ws-context"},
		TaskProgress: readmodelcore.TaskProgress{
			WorkspaceID: "ws-context",
			RunID:       "run-context",
			State:       "running",
		},
	}

	got := WorkspaceContextBundleFromSnapshots(workspaceSnapshot, runSnapshot)
	if got.Mode != "run_recovery" || got.RunID != "run-context" {
		t.Fatalf("context bundle = %+v, want run recovery bundle", got)
	}
}

func TestWorkspaceSnapshotWithContextBundleEmbedsProjectionContext(t *testing.T) {
	t.Parallel()

	got := WorkspaceSnapshotWithContextBundle(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-context"},
		TaskProgress: readmodelcore.TaskProgress{
			WorkspaceID: "ws-context",
			State:       "running",
		},
	})
	if got.ContextBundle == nil {
		t.Fatal("ContextBundle is nil, want embedded workspace context projection")
	}
	if got.ContextBundle.WorkspaceID != "ws-context" || got.ContextBundle.Mode != "workspace" {
		t.Fatalf("context bundle = %+v, want workspace scoped projection", got.ContextBundle)
	}
}

func TestWorkspaceSnapshotWithRunSnapshotRefreshesContextBundle(t *testing.T) {
	t.Parallel()

	got := WorkspaceSnapshotWithRunSnapshot(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-context"},
		TaskProgress: readmodelcore.TaskProgress{
			WorkspaceID: "ws-context",
			State:       "queued",
		},
	}, &readmodelcore.RunSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-context"},
		Run:       readmodelcore.Run{ID: "run-context", WorkspaceID: "ws-context"},
		TaskProgress: readmodelcore.TaskProgress{
			WorkspaceID: "ws-context",
			RunID:       "run-context",
			State:       "running",
		},
	})
	if got.ContextBundle == nil {
		t.Fatal("ContextBundle is nil, want embedded run context projection")
	}
	if got.ContextBundle.Mode != "run_recovery" || got.ContextBundle.RunID != "run-context" {
		t.Fatalf("context bundle = %+v, want run recovery context projection", got.ContextBundle)
	}
}

func TestWorkspaceTaskDecompositionsFromSnapshots(t *testing.T) {
	t.Parallel()

	existing := []readmodelcore.TaskDecomposition{{ID: "workspace-decomposition"}}
	if got := WorkspaceTaskDecompositionsFromSnapshots(readmodelcore.WorkspaceSnapshot{
		TaskDecompositions: existing,
		TaskProgress:       readmodelcore.TaskProgress{RunID: "run-ignored"},
	}, &readmodelcore.RunSnapshot{TaskDecompositions: []readmodelcore.TaskDecomposition{{ID: "run-decomposition"}}}); len(got) != 1 || got[0].ID != "workspace-decomposition" {
		t.Fatalf("existing decompositions = %+v, want workspace decomposition", got)
	}

	runSnapshot := &readmodelcore.RunSnapshot{
		TaskDecompositions: []readmodelcore.TaskDecomposition{{ID: "run-decomposition"}},
	}
	if got := WorkspaceTaskDecompositionsFromSnapshots(readmodelcore.WorkspaceSnapshot{
		TaskProgress: readmodelcore.TaskProgress{RunID: "run-decomposition"},
	}, runSnapshot); len(got) != 1 || got[0].ID != "run-decomposition" {
		t.Fatalf("run decompositions = %+v, want run decomposition", got)
	}

	if got := WorkspaceTaskDecompositionsFromSnapshots(readmodelcore.WorkspaceSnapshot{}, nil); got != nil {
		t.Fatalf("decompositions = %+v, want nil without run progress", got)
	}
}

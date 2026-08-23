package readmodelprojection

import (
	"testing"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestWorkspaceSnapshotTaskProgressUsesExistingProgress(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-progress"},
		TaskProgress: projectioncontract.TaskProgress{
			WorkspaceID: "ws-progress",
			State:       "custom",
		},
	}

	got := workspaceSnapshotTaskProgress(snapshot)
	if got.State != "custom" || got.WorkspaceID != "ws-progress" {
		t.Fatalf("progress = %+v, want existing workspace progress", got)
	}
}

func TestWorkspaceTaskProgressIgnoresHistoricalBlockersWithoutActiveRun(t *testing.T) {
	t.Parallel()

	got := mapLocalizedWorkspaceTaskProgress(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:    "ws-startup-recovered",
			State: string(common.WorkspaceStateActive),
		},
		Decisions: []projectioncontract.Decision{{
			ID:          "ticket-historical",
			WorkspaceID: "ws-startup-recovered",
			RunID:       "run-historical",
			State:       "open",
			Question:    "historical decision",
		}},
		Approvals: []projectioncontract.Approval{{
			ID:          "approval-historical",
			WorkspaceID: "ws-startup-recovered",
			RunID:       "run-historical",
			State:       "pending",
			Reason:      "historical approval",
		}},
	})

	if got.Recovery.Mode != "idle" {
		t.Fatalf("recovery = %+v, want factual idle mode", got.Recovery)
	}
	if len(got.Blockers) != 0 {
		t.Fatalf("blockers = %+v, want no current blockers without active run", got.Blockers)
	}
}

func TestWorkspaceTaskProgressOnlyUsesActiveRunBlockers(t *testing.T) {
	t.Parallel()

	got := mapLocalizedWorkspaceTaskProgress(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:          "ws-active-blocker",
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-current",
		},
		Run: &appmodelcore.Run{
			ID: "run-current", WorkspaceID: "ws-active-blocker", State: string(common.RunStateBlockedOnDecision), BlockingTicketID: "ticket-current",
		},
		Decisions: []projectioncontract.Decision{
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
				ID:          "ticket-extra",
				WorkspaceID: "ws-active-blocker",
				RunID:       "run-current",
				State:       "open",
				Question:    "noncanonical same-run decision",
			},
			{
				ID:                     "ticket-current",
				WorkspaceID:            "ws-active-blocker",
				RunID:                  "run-current",
				State:                  "open",
				Question:               "current decision",
				AllowedRecoveryActions: []string{"revise_definition_and_restart"},
				DefinitionClarification: &projectioncontract.DefinitionClarification{
					Class:          "runtime",
					BindingState:   "open",
					RequiredAction: "revise_definition_and_restart",
				},
			},
		},
	})

	if got.Recovery.BlockingTicketID != "ticket-current" || got.Recovery.Mode != "wait_decision" {
		t.Fatalf("recovery = %+v, want current active run decision", got.Recovery)
	}
	if len(got.Blockers) != 1 || got.Blockers[0].ID != "ticket-current" {
		t.Fatalf("blockers = %+v, want only current active run blocker", got.Blockers)
	}
	if got.Recovery.BlockingResource == nil ||
		got.Recovery.BlockingResource.OwnerKind != "decision" ||
		got.Recovery.BlockingResource.ResourceID != "ticket-current" ||
		got.Recovery.BlockingResource.WorkspaceID != "ws-active-blocker" ||
		got.Recovery.BlockingResource.RunID != "run-current" ||
		got.Recovery.BlockingResource.RequiredAction != "revise_definition_and_restart" ||
		len(got.Recovery.BlockingResource.AllowedUserActions) != 1 ||
		got.Recovery.SuggestedNavigation != "workspace_definition" {
		t.Fatalf("recovery blocker = %+v, want exact Decision-owned definition binding", got.Recovery)
	}
}

func TestWorkspaceTaskProgressRequiresSameScopeOpenGovernanceBlockers(t *testing.T) {
	t.Parallel()

	got := mapLocalizedWorkspaceTaskProgress(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:          "ws-current",
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-shared",
		},
		Decisions: []projectioncontract.Decision{
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
		Approvals: []projectioncontract.Approval{
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

	got := projectWorkspaceSnapshotDetail(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:          "ws-resolved-blocker",
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-current",
		},
		Run: &appmodelcore.Run{
			ID:               "run-current",
			WorkspaceID:      "ws-resolved-blocker",
			State:            string(common.RunStateBlockedOnDecision),
			BlockingTicketID: "ticket-resolved",
		},
		TaskProgress: projectioncontract.TaskProgress{
			WorkspaceID:      "ws-resolved-blocker",
			RunID:            "run-current",
			State:            string(common.RunStateBlockedOnDecision),
			BlockingTicketID: "ticket-resolved",
			Recovery: projectioncontract.RecoveryPlan{
				Mode:             "wait_decision",
				BlockingTicketID: "ticket-resolved",
			},
		},
		Decisions: []projectioncontract.Decision{{
			ID:          "ticket-resolved",
			WorkspaceID: "ws-resolved-blocker",
			RunID:       "run-current",
			State:       string(common.DecisionTicketStateResolved),
		}},
	})

	if got.Workspace.State != string(common.WorkspaceStateActive) || got.Workspace.ActiveRunID != "run-current" ||
		got.Run == nil || got.Run.BlockingTicketID != "ticket-resolved" ||
		got.TaskProgress.BlockingTicketID != "ticket-resolved" || got.TaskProgress.Recovery.BlockingTicketID != "ticket-resolved" ||
		got.GovernanceDisplay.DecisionID != "" || got.ConsistencyDiagnostic == nil || got.ConsistencyDiagnostic.Code != "owner_governance_reference_unresolved" {
		t.Fatalf("snapshot = %+v progress = %+v display=%+v diagnostic=%+v, want owner fields unchanged with a read-side mismatch", got.Workspace, got.TaskProgress, got.GovernanceDisplay, got.ConsistencyDiagnostic)
	}
}

func TestWorkspaceGovernanceProjectionKeepsOpenDecisionBlocker(t *testing.T) {
	t.Parallel()

	got := projectWorkspaceSnapshotDetail(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:          "ws-open-blocker",
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-current",
		},
		Run: &appmodelcore.Run{
			ID:               "run-current",
			WorkspaceID:      "ws-open-blocker",
			State:            string(common.RunStateBlockedOnDecision),
			BlockingTicketID: "ticket-open",
		},
		TaskProgress: projectioncontract.TaskProgress{
			WorkspaceID: "ws-open-blocker",
			RunID:       "run-current",
			State:       string(common.RunStateBlockedOnDecision),
		},
		Decisions: []projectioncontract.Decision{{
			ID:          "ticket-open",
			WorkspaceID: "ws-open-blocker",
			RunID:       "run-current",
			State:       string(common.DecisionTicketStateOpen),
		}},
	})

	if got.Workspace.State != string(common.WorkspaceStateActive) || got.Workspace.ActiveRunID != "run-current" ||
		got.Run == nil || got.Run.BlockingTicketID != "ticket-open" ||
		got.GovernanceDisplay.DecisionID != "ticket-open" || got.GovernanceDisplay.PromptMode != "decision" ||
		got.TaskProgress.BlockingTicketID != "" || got.TaskProgress.Recovery.Mode != "" || got.ConsistencyDiagnostic != nil {
		t.Fatalf("snapshot = %+v progress = %+v display=%+v diagnostic=%+v, want canonical owner state plus separate decision display", got.Workspace, got.TaskProgress, got.GovernanceDisplay, got.ConsistencyDiagnostic)
	}
}

func TestWorkspaceContextBundleFromSnapshotsPrefersRunSnapshot(t *testing.T) {
	t.Parallel()

	workspaceSnapshot := readmodelcore.WorkspaceSnapshotState{
		Workspace:    appmodelcore.Workspace{ID: "ws-context"},
		TaskProgress: projectioncontract.TaskProgress{WorkspaceID: "ws-context", RunID: "run-context"},
	}
	runSnapshot := &readmodelcore.RunSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-context"},
		Run:       appmodelcore.Run{ID: "run-context", WorkspaceID: "ws-context"},
		TaskProgress: projectioncontract.TaskProgress{
			WorkspaceID: "ws-context",
			RunID:       "run-context",
			State:       "running",
		},
	}

	got := workspaceContextBundleFromSnapshots(workspaceSnapshot, runSnapshot)
	if got.Mode != "run_recovery" || got.RunID != "run-context" {
		t.Fatalf("context bundle = %+v, want run recovery bundle", got)
	}
}

func TestWorkspaceSnapshotWithContextBundleEmbedsProjectionContext(t *testing.T) {
	t.Parallel()

	got := workspaceSnapshotWithContextBundle(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-context"},
		TaskProgress: projectioncontract.TaskProgress{
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

	got := workspaceSnapshotWithRunSnapshot(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-context"},
		TaskProgress: projectioncontract.TaskProgress{
			WorkspaceID: "ws-context",
			State:       "queued",
		},
	}, &readmodelcore.RunSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-context"},
		Run:       appmodelcore.Run{ID: "run-context", WorkspaceID: "ws-context"},
		TaskProgress: projectioncontract.TaskProgress{
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

	existing := []projectioncontract.TaskDecomposition{{ID: "workspace-decomposition"}}
	if got := workspaceTaskDecompositionsFromSnapshots(readmodelcore.WorkspaceSnapshotState{
		TaskDecompositions: existing,
		TaskProgress:       projectioncontract.TaskProgress{RunID: "run-ignored"},
	}, &readmodelcore.RunSnapshotState{TaskDecompositions: []projectioncontract.TaskDecomposition{{ID: "run-decomposition"}}}); len(got) != 1 || got[0].ID != "workspace-decomposition" {
		t.Fatalf("existing decompositions = %+v, want workspace decomposition", got)
	}

	runSnapshot := &readmodelcore.RunSnapshotState{
		TaskDecompositions: []projectioncontract.TaskDecomposition{{ID: "run-decomposition"}},
	}
	if got := workspaceTaskDecompositionsFromSnapshots(readmodelcore.WorkspaceSnapshotState{
		TaskProgress: projectioncontract.TaskProgress{RunID: "run-decomposition"},
	}, runSnapshot); len(got) != 1 || got[0].ID != "run-decomposition" {
		t.Fatalf("run decompositions = %+v, want run decomposition", got)
	}

	if got := workspaceTaskDecompositionsFromSnapshots(readmodelcore.WorkspaceSnapshotState{}, nil); got != nil {
		t.Fatalf("decompositions = %+v, want nil without run progress", got)
	}
}

package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestNormalizeWorkspaceGovernanceRequiresOpenDecisionTicket(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:               "ws-governance-normalize",
			State:            string(common.WorkspaceStateActive),
			ActiveRunID:      "run-governance-normalize",
			BlockingTicketID: "ticket-missing",
		},
		Run: &appmodelcore.Run{
			ID:               "run-governance-normalize",
			WorkspaceID:      "ws-governance-normalize",
			State:            string(common.RunStateBlockedOnDecision),
			BlockingTicketID: "ticket-missing",
		},
	}

	got := normalizeWorkspaceGovernanceProjection(snapshot)
	if got.Workspace.State != string(common.WorkspaceStateActive) ||
		got.Workspace.BlockingTicketID != "" ||
		got.TaskProgress.BlockingTicketID != "" ||
		workspacePromptMode(got) == "decision" {
		t.Fatalf("normalized snapshot = %+v, want active workspace without decision prompt when ticket is not open", got)
	}
}

func TestNormalizeWorkspaceGovernanceRequiresPendingApproval(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:                 "ws-approval-normalize",
			State:              string(common.WorkspaceStateBlockedOnApproval),
			ActiveRunID:        "run-approval-normalize",
			BlockingApprovalID: "approval-missing",
		},
		Run: &appmodelcore.Run{
			ID:                 "run-approval-normalize",
			WorkspaceID:        "ws-approval-normalize",
			State:              string(common.RunStateBlockedOnApproval),
			BlockingApprovalID: "approval-missing",
		},
	}

	got := normalizeWorkspaceGovernanceProjection(snapshot)
	if got.Workspace.State == string(common.WorkspaceStateBlockedOnApproval) ||
		got.Workspace.BlockingApprovalID != "" ||
		got.TaskProgress.BlockingApprovalID != "" ||
		workspacePromptMode(got) == "approval" {
		t.Fatalf("normalized snapshot = %+v, want no blocked approval without pending approval", got)
	}
}

func TestProjectWorkspaceSnapshotSummaryKeepsBlockingDecision(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:               "ws-governance-summary",
			State:            string(common.WorkspaceStateActive),
			ActiveRunID:      "run-governance-summary",
			BlockingTicketID: "ticket-open",
		},
		Run: &appmodelcore.Run{
			ID:               "run-governance-summary",
			WorkspaceID:      "ws-governance-summary",
			State:            string(common.RunStateBlockedOnDecision),
			BlockingTicketID: "ticket-open",
		},
		Decisions: []projectioncontract.Decision{{
			ID:          "ticket-open",
			WorkspaceID: "ws-governance-summary",
			RunID:       "run-governance-summary",
			State:       string(common.DecisionTicketStateOpen),
		}},
	}

	got := projectWorkspaceSnapshotSummary(snapshot)
	if got.Workspace.State != string(common.WorkspaceStateActive) ||
		got.Workspace.BlockingTicketID != "ticket-open" ||
		workspacePromptMode(got) != "decision" {
		t.Fatalf("summary governance = workspace:%+v mode=%s, want active decision prompt", got.Workspace, workspacePromptMode(got))
	}
	if len(got.Decisions) != 1 || got.Decisions[0].ID != "ticket-open" {
		t.Fatalf("summary decisions = %+v, want blocking decision retained", got.Decisions)
	}
}

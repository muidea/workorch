package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestNormalizeWorkspaceGovernanceRequiresOpenDecisionTicket(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{
			ID:               "ws-governance-normalize",
			State:            string(common.WorkspaceStateWaitingDecision),
			ActiveRunID:      "run-governance-normalize",
			BlockingTicketID: "ticket-missing",
		},
		Run: &readmodelcore.Run{
			ID:               "run-governance-normalize",
			WorkspaceID:      "ws-governance-normalize",
			State:            string(common.RunStateBlockedOnDecision),
			BlockingTicketID: "ticket-missing",
		},
	}

	got := NormalizeWorkspaceGovernanceProjection(snapshot)
	if got.Workspace.State == string(common.WorkspaceStateWaitingDecision) ||
		got.Workspace.BlockingTicketID != "" ||
		got.TaskProgress.BlockingTicketID != "" ||
		WorkspacePromptMode(got) == "decision" {
		t.Fatalf("normalized snapshot = %+v, want no waiting_decision without open ticket", got)
	}
}

func TestProjectWorkspaceSnapshotSummaryKeepsBlockingDecision(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{
			ID:               "ws-governance-summary",
			State:            string(common.WorkspaceStateRunning),
			ActiveRunID:      "run-governance-summary",
			BlockingTicketID: "ticket-open",
		},
		Run: &readmodelcore.Run{
			ID:               "run-governance-summary",
			WorkspaceID:      "ws-governance-summary",
			State:            string(common.RunStateBlockedOnDecision),
			BlockingTicketID: "ticket-open",
		},
		Decisions: []readmodelcore.Decision{{
			ID:          "ticket-open",
			WorkspaceID: "ws-governance-summary",
			RunID:       "run-governance-summary",
			State:       string(common.DecisionTicketStateOpen),
		}},
	}

	got := ProjectWorkspaceSnapshotSummary(snapshot)
	if got.Workspace.State != string(common.WorkspaceStateWaitingDecision) ||
		got.Workspace.BlockingTicketID != "ticket-open" ||
		WorkspacePromptMode(got) != "decision" {
		t.Fatalf("summary governance = workspace:%+v mode=%s, want waiting decision", got.Workspace, WorkspacePromptMode(got))
	}
	if len(got.Decisions) != 1 || got.Decisions[0].ID != "ticket-open" {
		t.Fatalf("summary decisions = %+v, want blocking decision retained", got.Decisions)
	}
}

package readmodelprojection

import (
	"reflect"
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
			ID:          "ws-governance-normalize",
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-governance-normalize",
		},
		Run: &appmodelcore.Run{
			ID:               "run-governance-normalize",
			WorkspaceID:      "ws-governance-normalize",
			State:            string(common.RunStateBlockedOnDecision),
			BlockingTicketID: "ticket-missing",
		},
	}

	got := normalizeWorkspaceGovernanceProjection(snapshot)
	if !reflect.DeepEqual(got.Workspace, snapshot.Workspace) || got.Run == nil || got.Run.State != string(common.RunStateBlockedOnDecision) ||
		got.Run.BlockingTicketID != "ticket-missing" || got.GovernanceDisplay.DecisionID != "" || workspacePromptMode(got) == "decision" ||
		got.ConsistencyDiagnostic == nil || got.ConsistencyDiagnostic.Code != "owner_governance_reference_unresolved" {
		t.Fatalf("projection = %+v, want immutable owner fields plus unresolved decision diagnostic", got)
	}
}

func TestNormalizeWorkspaceGovernanceRequiresPendingApproval(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:          "ws-approval-normalize",
			State:       string(common.WorkspaceStateBlockedOnApproval),
			ActiveRunID: "run-approval-normalize",
		},
		Run: &appmodelcore.Run{
			ID:                 "run-approval-normalize",
			WorkspaceID:        "ws-approval-normalize",
			State:              string(common.RunStateBlockedOnApproval),
			BlockingApprovalID: "approval-missing",
		},
	}

	got := normalizeWorkspaceGovernanceProjection(snapshot)
	if !reflect.DeepEqual(got.Workspace, snapshot.Workspace) || got.Run == nil || got.Run.State != string(common.RunStateBlockedOnApproval) ||
		got.Run.BlockingApprovalID != "approval-missing" || got.GovernanceDisplay.ApprovalID != "" || workspacePromptMode(got) == "approval" ||
		got.ConsistencyDiagnostic == nil || got.ConsistencyDiagnostic.Code != "owner_governance_reference_unresolved" {
		t.Fatalf("projection = %+v, want immutable owner fields plus unresolved approval diagnostic", got)
	}
}

func TestProjectWorkspaceSnapshotSummaryKeepsBlockingDecision(t *testing.T) {
	t.Parallel()

	snapshot := readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{
			ID:          "ws-governance-summary",
			State:       string(common.WorkspaceStateActive),
			ActiveRunID: "run-governance-summary",
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
		got.Workspace.ActiveRunID != "run-governance-summary" ||
		got.Run != nil ||
		got.GovernanceDisplay.DecisionID != "ticket-open" ||
		workspacePromptMode(got) != "decision" {
		t.Fatalf("summary governance = workspace:%+v display:%+v mode=%s, want bounded summary with active decision display", got.Workspace, got.GovernanceDisplay, workspacePromptMode(got))
	}
	if len(got.Decisions) != 1 || got.Decisions[0].ID != "ticket-open" {
		t.Fatalf("summary decisions = %+v, want blocking decision retained", got.Decisions)
	}
}

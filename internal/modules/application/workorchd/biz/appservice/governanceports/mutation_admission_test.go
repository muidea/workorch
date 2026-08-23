package governanceports

import (
	"context"
	"errors"
	"testing"

	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestResolveDecisionViewRejectsNonCanonicalOpenResourceWithoutMutation(t *testing.T) {
	testHub := newGovernanceMagicTestHub(t)
	testHub.subscribeCanonicalGovernance(governanceCanonicalFixture{
		WorkspaceID: "ws-decision-stale", RunID: "run-decision-stale", RunState: common.RunStateBlockedOnDecision, BlockingTicketID: "ticket-canonical",
	})
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandGetTicket, func(any) any {
		return decisionevents.GetTicketResult{Ticket: &decisionmodel.Ticket{
			ID: "ticket-extra", WorkspaceID: "ws-decision-stale", RunID: "run-decision-stale", State: common.DecisionTicketStateOpen,
		}}
	})
	mutationCount := 0
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandEnsureResolved, func(any) any {
		mutationCount++
		return decisionevents.EnsureResolvedResult{}
	})

	_, err := ResolveDecisionView(context.Background(), testHub.rawHub, nil, "ticket-extra", appmodelcore.ResolveDecisionCommand{
		ResolutionType:    "provide_missing_info",
		ResolutionPayload: mustGovernancePayload(t, map[string]any{"answer": "continue"}),
	}, "ws-decision-stale")
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("ResolveDecisionView err = %v, want typed conflict", err)
	}
	if mutationCount != 0 {
		t.Fatalf("non-canonical open Decision produced %d mutations, want zero", mutationCount)
	}
}

func TestResolveApprovalViewRejectsNonCanonicalPendingResourceWithoutMutation(t *testing.T) {
	testHub := newGovernanceMagicTestHub(t)
	testHub.subscribeCanonicalGovernance(governanceCanonicalFixture{
		WorkspaceID: "ws-approval-stale", RunID: "run-approval-stale", RunState: common.RunStateBlockedOnApproval, BlockingApprovalID: "approval-canonical",
	})
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandListByWorkspace, func(any) approvalTestCommandResult {
		return approvalTestCommandResult{Approvals: []*approvalmodel.Approval{{
			ID: "approval-extra", WorkspaceID: "ws-approval-stale", RunID: "run-approval-stale", State: common.ApprovalStatePending,
		}}}
	})
	mutationCount := 0
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandEnsureApproved, func(any) approvalTestCommandResult {
		mutationCount++
		return approvalTestCommandResult{}
	})

	_, err := ResolveApprovalView(context.Background(), testHub.rawHub, nil, "approval-extra", "approve", "ws-approval-stale")
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("ResolveApprovalView err = %v, want typed conflict", err)
	}
	if mutationCount != 0 {
		t.Fatalf("non-canonical pending Approval produced %d mutations, want zero", mutationCount)
	}
}

func TestResolveDecisionViewRejectsDefinitionAdmissionAndClarification(t *testing.T) {
	tests := []struct {
		name   string
		ticket *decisionmodel.Ticket
	}{
		{name: "typed binding", ticket: &decisionmodel.Ticket{DefinitionClarification: &decisionmodel.DefinitionClarificationBinding{Class: decisionmodel.DefinitionClarificationRuntime}}},
		{name: "workspace clarification kind", ticket: &decisionmodel.Ticket{DecisionKind: "workspace_definition_clarification"}},
		{name: "definition admission failure class", ticket: &decisionmodel.Ticket{FailureClass: "definition_admission"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testHub := newGovernanceMagicTestHub(t)
			ticket := *tc.ticket
			ticket.ID = "ticket-definition"
			ticket.WorkspaceID = "ws-definition"
			ticket.RunID = "run-definition"
			ticket.State = common.DecisionTicketStateOpen
			testHub.subscribeDecision(decisionevents.TopicDecisionCommandGetTicket, func(any) any {
				return decisionevents.GetTicketResult{Ticket: &ticket}
			})
			mutationCount := 0
			testHub.subscribeDecision(decisionevents.TopicDecisionCommandEnsureResolved, func(any) any {
				mutationCount++
				return decisionevents.EnsureResolvedResult{}
			})

			_, err := ResolveDecisionView(context.Background(), testHub.rawHub, nil, ticket.ID, appmodelcore.ResolveDecisionCommand{
				ResolutionType:    "provide_missing_info",
				ResolutionPayload: mustGovernancePayload(t, map[string]any{"answer": "continue"}),
			}, ticket.WorkspaceID)
			if !errors.Is(err, common.ErrConflict) {
				t.Fatalf("ResolveDecisionView err = %v, want dedicated-definition conflict", err)
			}
			if mutationCount != 0 {
				t.Fatalf("definition ticket produced %d generic mutations, want zero", mutationCount)
			}
		})
	}
}

func TestBatchResolveDecisionViewsPreflightsAllTargetsBeforeMutation(t *testing.T) {
	testHub := newGovernanceMagicTestHub(t)
	testHub.subscribeCanonicalGovernance(
		governanceCanonicalFixture{WorkspaceID: "ws-batch-valid", RunID: "run-batch-valid", RunState: common.RunStateBlockedOnDecision, BlockingTicketID: "ticket-batch-valid"},
		governanceCanonicalFixture{WorkspaceID: "ws-batch-stale", RunID: "run-batch-stale", RunState: common.RunStateBlockedOnDecision, BlockingTicketID: "ticket-other"},
	)
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandGetTicket, func(data any) any {
		cmd := data.(decisionevents.GetTicketCommand)
		switch cmd.TicketID {
		case "ticket-batch-valid":
			return decisionevents.GetTicketResult{Ticket: &decisionmodel.Ticket{ID: cmd.TicketID, WorkspaceID: "ws-batch-valid", RunID: "run-batch-valid", State: common.DecisionTicketStateOpen}}
		case "ticket-batch-stale":
			return decisionevents.GetTicketResult{Ticket: &decisionmodel.Ticket{ID: cmd.TicketID, WorkspaceID: "ws-batch-stale", RunID: "run-batch-stale", State: common.DecisionTicketStateOpen}}
		default:
			t.Fatalf("unexpected ticket read %q", cmd.TicketID)
			return nil
		}
	})
	mutationCount := 0
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandEnsureResolved, func(any) any {
		mutationCount++
		return decisionevents.EnsureResolvedResult{}
	})

	_, err := BatchResolveDecisionViews(context.Background(), testHub.rawHub, nil, appmodelcore.BatchResolveDecisionCommand{
		TicketIDs:         []string{"ticket-batch-valid", "ticket-batch-stale"},
		ResolutionType:    "provide_missing_info",
		ResolutionPayload: mustGovernancePayload(t, map[string]any{"answer": "continue"}),
	}, "")
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("BatchResolveDecisionViews err = %v, want typed conflict", err)
	}
	if mutationCount != 0 {
		t.Fatalf("failed Decision batch produced %d mutations, want zero", mutationCount)
	}
}

func TestBatchResolveDecisionViewsRejectsDefinitionTicketBeforeMutation(t *testing.T) {
	testHub := newGovernanceMagicTestHub(t)
	testHub.subscribeCanonicalGovernance(governanceCanonicalFixture{
		WorkspaceID: "ws-definition-batch-valid", RunID: "run-definition-batch-valid", RunState: common.RunStateBlockedOnDecision, BlockingTicketID: "ticket-definition-batch-valid",
	})
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandGetTicket, func(data any) any {
		cmd := data.(decisionevents.GetTicketCommand)
		if cmd.TicketID == "ticket-definition-batch-valid" {
			return decisionevents.GetTicketResult{Ticket: &decisionmodel.Ticket{ID: cmd.TicketID, WorkspaceID: "ws-definition-batch-valid", RunID: "run-definition-batch-valid", State: common.DecisionTicketStateOpen}}
		}
		return decisionevents.GetTicketResult{Ticket: &decisionmodel.Ticket{
			ID: cmd.TicketID, WorkspaceID: "ws-definition-batch", RunID: "run-definition-batch", State: common.DecisionTicketStateOpen,
			DecisionKind: "runtime_definition_clarification",
		}}
	})
	mutationCount := 0
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandEnsureResolved, func(any) any {
		mutationCount++
		return decisionevents.EnsureResolvedResult{}
	})

	_, err := BatchResolveDecisionViews(context.Background(), testHub.rawHub, nil, appmodelcore.BatchResolveDecisionCommand{
		TicketIDs:         []string{"ticket-definition-batch-valid", "ticket-definition-batch"},
		ResolutionType:    "provide_missing_info",
		ResolutionPayload: mustGovernancePayload(t, map[string]any{"answer": "continue"}),
	}, "")
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("BatchResolveDecisionViews err = %v, want dedicated-definition conflict", err)
	}
	if mutationCount != 0 {
		t.Fatalf("definition Decision batch produced %d mutations, want zero", mutationCount)
	}
}

func TestBatchResolveApprovalViewsPreflightsAllTargetsBeforeMutation(t *testing.T) {
	testHub := newGovernanceMagicTestHub(t)
	testHub.subscribeCanonicalGovernance(
		governanceCanonicalFixture{WorkspaceID: "ws-approval-batch-valid", RunID: "run-approval-batch-valid", RunState: common.RunStateBlockedOnApproval, BlockingApprovalID: "approval-batch-valid"},
		governanceCanonicalFixture{WorkspaceID: "ws-approval-batch-stale", RunID: "run-approval-batch-stale", RunState: common.RunStateBlockedOnApproval, BlockingApprovalID: "approval-other"},
	)
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandList, func(any) approvalTestCommandResult {
		return approvalTestCommandResult{Approvals: []*approvalmodel.Approval{
			{ID: "approval-batch-valid", WorkspaceID: "ws-approval-batch-valid", RunID: "run-approval-batch-valid", State: common.ApprovalStatePending},
			{ID: "approval-batch-stale", WorkspaceID: "ws-approval-batch-stale", RunID: "run-approval-batch-stale", State: common.ApprovalStatePending},
		}}
	})
	mutationCount := 0
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandEnsureApproved, func(any) approvalTestCommandResult {
		mutationCount++
		return approvalTestCommandResult{}
	})

	_, err := BatchResolveApprovalViews(context.Background(), testHub.rawHub, nil, appmodelcore.BatchResolveApprovalCommand{
		ApprovalIDs: []string{"approval-batch-valid", "approval-batch-stale"}, Action: "approve",
	}, "")
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("BatchResolveApprovalViews err = %v, want typed conflict", err)
	}
	if mutationCount != 0 {
		t.Fatalf("failed Approval batch produced %d mutations, want zero", mutationCount)
	}
}

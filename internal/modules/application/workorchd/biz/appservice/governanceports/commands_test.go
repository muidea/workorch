package governanceports

import (
	"context"
	"testing"

	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"

	approvalcontracts "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/contracts"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisioncontracts "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/contracts"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestApprovalsForQueryFiltersOwnerCommandResults(t *testing.T) {
	hub := events.NewMemoryHub()
	hub.Subscribe(approvalevents.TopicApprovalCommandListByRun, func(_ context.Context, envelope events.Envelope) error {
		if got := envelope.Payload[events.PayloadKeyRunID]; got != "run-001" {
			t.Fatalf("run payload = %v, want run-001", got)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyItems: []*approvalmodel.Approval{
					{ID: "approval-001", WorkspaceID: "ws-001", RunID: "run-001"},
					{ID: "approval-other-workspace", WorkspaceID: "ws-other", RunID: "run-001"},
					{ID: "approval-other-run", WorkspaceID: "ws-001", RunID: "run-other"},
					nil,
				},
			},
		})
	})

	items, err := ApprovalsForQuery(context.Background(), hub, " ws-001 ", " run-001 ")
	if err != nil {
		t.Fatalf("ApprovalsForQuery err = %v", err)
	}
	if len(items) != 1 || items[0].ID != "approval-001" {
		t.Fatalf("ApprovalsForQuery items = %#v, want only approval-001", items)
	}
}

func TestDecisionTicketsForQueryFiltersOwnerCommandResults(t *testing.T) {
	hub := events.NewMemoryHub()
	hub.Subscribe(decisionevents.TopicDecisionCommandListTicketsByRun, func(_ context.Context, envelope events.Envelope) error {
		if got := envelope.Payload[events.PayloadKeyRunID]; got != "run-001" {
			t.Fatalf("run payload = %v, want run-001", got)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyTickets: []*decisionmodel.Ticket{
					{ID: "ticket-001", WorkspaceID: "ws-001", RunID: "run-001"},
					{ID: "ticket-other-workspace", WorkspaceID: "ws-other", RunID: "run-001"},
					{ID: "ticket-other-run", WorkspaceID: "ws-001", RunID: "run-other"},
					nil,
				},
			},
		})
	})

	items, err := DecisionTicketsForQuery(context.Background(), hub, " ws-001 ", " run-001 ")
	if err != nil {
		t.Fatalf("DecisionTicketsForQuery err = %v", err)
	}
	if len(items) != 1 || items[0].ID != "ticket-001" {
		t.Fatalf("DecisionTicketsForQuery items = %#v, want only ticket-001", items)
	}
}

func TestResolutionForTicketReadsOnlyResolvedTickets(t *testing.T) {
	hub := events.NewMemoryHub()
	readCount := 0
	hub.Subscribe(decisionevents.TopicDecisionCommandGetResolution, func(_ context.Context, envelope events.Envelope) error {
		readCount++
		if got := envelope.Payload[events.PayloadKeyBlockingTicketID]; got != "ticket-resolved" {
			t.Fatalf("ticket payload = %v, want ticket-resolved", got)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyResolution: &decisionmodel.Resolution{
					ID:       "resolution-001",
					TicketID: "ticket-resolved",
					Type:     "provide_missing_info",
				},
			},
		})
	})

	openResolution, err := ResolutionForTicket(context.Background(), hub, &decisionmodel.Ticket{
		ID:    "ticket-open",
		State: common.DecisionTicketStateOpen,
	})
	if err != nil {
		t.Fatalf("open ResolutionForTicket err = %v", err)
	}
	if openResolution != nil || readCount != 0 {
		t.Fatalf("open resolution = %#v readCount = %d, want nil and no owner read", openResolution, readCount)
	}

	resolvedResolution, err := ResolutionForTicket(context.Background(), hub, &decisionmodel.Ticket{
		ID:    "ticket-resolved",
		State: common.DecisionTicketStateResolved,
	})
	if err != nil {
		t.Fatalf("resolved ResolutionForTicket err = %v", err)
	}
	if resolvedResolution == nil || resolvedResolution.ID != "resolution-001" || readCount != 1 {
		t.Fatalf("resolved resolution = %#v readCount = %d, want resolution-001 and one owner read", resolvedResolution, readCount)
	}
}

func TestResolveDecisionPublishesContractCommand(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	hub.Subscribe(decisionevents.TopicDecisionCommandEnsureResolved, func(_ context.Context, envelope events.Envelope) error {
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(decisioncontracts.ResolveTicketCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want ResolveTicketCommand", envelope.Payload[events.PayloadKeyCommand])
		}
		if cmd.TicketID != "ticket-contract" || cmd.WorkspaceID != "ws-contract" || cmd.RunID != "run-contract" || cmd.Type != "provide_missing_info" {
			t.Fatalf("command = %+v", cmd)
		}
		if cmd.ResolutionID != "resolution-ws-contract-ticket-contract-provide_missing_info" {
			t.Fatalf("resolutionID = %q, want workspace-scoped id", cmd.ResolutionID)
		}
		if cmd.Payload["answer"] != "42" {
			t.Fatalf("command payload = %#v", cmd.Payload)
		}
		if _, ok := envelope.Payload[events.PayloadKeyBlockingTicketID]; ok {
			t.Fatalf("legacy ticket id payload still present")
		}
		ticket := &decisionmodel.Ticket{ID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, State: common.DecisionTicketStateResolved}
		resolution := &decisionmodel.Resolution{ID: cmd.ResolutionID, TicketID: cmd.TicketID, Type: cmd.Type, Payload: cmd.Payload}
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response", Payload: map[string]any{
			events.PayloadKeyTicket:     ticket,
			events.PayloadKeyResolution: resolution,
		}})
	})

	ticket, resolution, err := ResolveDecision(ctx, hub, &decisionmodel.Ticket{
		ID:          "ticket-contract",
		WorkspaceID: "ws-contract",
		RunID:       "run-contract",
		StepID:      "step-contract",
	}, "provide_missing_info", map[string]any{"answer": "42"})
	if err != nil {
		t.Fatalf("ResolveDecision: %v", err)
	}
	if ticket.ID != "ticket-contract" || resolution.Type != "provide_missing_info" {
		t.Fatalf("ticket/resolution = %+v / %+v", ticket, resolution)
	}
}

func TestResolveDecisionScopesResolutionIDByWorkspace(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	var resolutionIDs []common.ResolutionID
	hub.Subscribe(decisionevents.TopicDecisionCommandEnsureResolved, func(_ context.Context, envelope events.Envelope) error {
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(decisioncontracts.ResolveTicketCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want ResolveTicketCommand", envelope.Payload[events.PayloadKeyCommand])
		}
		resolutionIDs = append(resolutionIDs, cmd.ResolutionID)
		ticket := &decisionmodel.Ticket{ID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, State: common.DecisionTicketStateResolved}
		resolution := &decisionmodel.Resolution{ID: cmd.ResolutionID, TicketID: cmd.TicketID, Type: cmd.Type, Payload: cmd.Payload}
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response", Payload: map[string]any{
			events.PayloadKeyTicket:     ticket,
			events.PayloadKeyResolution: resolution,
		}})
	})

	for _, workspaceID := range []common.WorkspaceID{"ws-scope-a", "ws-scope-b"} {
		_, _, err := ResolveDecision(ctx, hub, &decisionmodel.Ticket{
			ID:          "ticket-same",
			WorkspaceID: workspaceID,
			RunID:       "run-same",
			StepID:      "run-same/decision",
		}, "replan", map[string]any{"operator": "scope-test"})
		if err != nil {
			t.Fatalf("ResolveDecision(%s): %v", workspaceID, err)
		}
	}
	if len(resolutionIDs) != 2 {
		t.Fatalf("resolutionIDs = %v, want two IDs", resolutionIDs)
	}
	if resolutionIDs[0] != "resolution-ws-scope-a-ticket-same-replan" || resolutionIDs[1] != "resolution-ws-scope-b-ticket-same-replan" {
		t.Fatalf("resolutionIDs = %v, want workspace-scoped IDs", resolutionIDs)
	}
	if resolutionIDs[0] == resolutionIDs[1] {
		t.Fatalf("resolutionIDs collided: %v", resolutionIDs)
	}
}

func TestResolveApprovalPublishesContractCommand(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	hub.Subscribe(approvalevents.TopicApprovalCommandEnsureApproved, func(_ context.Context, envelope events.Envelope) error {
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(approvalcontracts.ResolveApprovalCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want ResolveApprovalCommand", envelope.Payload[events.PayloadKeyCommand])
		}
		if cmd.ApprovalID != "approval-contract" {
			t.Fatalf("command = %+v", cmd)
		}
		if _, ok := envelope.Payload[events.PayloadKeyApprovalID]; ok {
			t.Fatalf("legacy approval id payload still present")
		}
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response", Payload: map[string]any{
			events.PayloadKeyApproval: &approvalmodel.Approval{ID: cmd.ApprovalID, State: common.ApprovalStateApproved},
		}})
	})

	item, err := ApproveApproval(ctx, hub, "approval-contract")
	if err != nil {
		t.Fatalf("ApproveApproval: %v", err)
	}
	if item.ID != "approval-contract" || item.State != common.ApprovalStateApproved {
		t.Fatalf("approval = %+v", item)
	}
}

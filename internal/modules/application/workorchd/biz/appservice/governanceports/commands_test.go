package governanceports

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func mustGovernancePayload(t *testing.T, input map[string]any) workorcheventpayload.Payload {
	t.Helper()
	payload, err := workorcheventpayload.FromMap(input)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestApprovalsForQueryFiltersOwnerCommandResults(t *testing.T) {
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandListByRun, func(data any) approvalevents.ApprovalCommandResult {
		cmd, ok := data.(approvalevents.ListApprovalsByRunCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want ListApprovalsByRunCommand", data)
		}
		if cmd.RunID != "run-001" {
			t.Fatalf("run command = %q, want run-001", cmd.RunID)
		}
		if cmd.WorkspaceID != "ws-001" {
			t.Fatalf("workspace command = %q, want ws-001", cmd.WorkspaceID)
		}
		return approvalevents.ApprovalCommandResult{
			Approvals: []*approvalmodel.Approval{
				{ID: "approval-001", WorkspaceID: "ws-001", RunID: "run-001"},
				{ID: "approval-other-workspace", WorkspaceID: "ws-other", RunID: "run-001"},
				{ID: "approval-other-run", WorkspaceID: "ws-001", RunID: "run-other"},
				nil,
			},
		}
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
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandListTicketsByRun, func(data any) decisionevents.DecisionCommandResult {
		cmd, ok := data.(decisionevents.ListTicketsByRunCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want ListTicketsByRunCommand", data)
		}
		if cmd.RunID != "run-001" {
			t.Fatalf("run command = %q, want run-001", cmd.RunID)
		}
		if cmd.WorkspaceID != "ws-001" {
			t.Fatalf("workspace command = %q, want ws-001", cmd.WorkspaceID)
		}
		return decisionevents.DecisionCommandResult{
			Tickets: []*decisionmodel.Ticket{
				{ID: "ticket-001", WorkspaceID: "ws-001", RunID: "run-001"},
				{ID: "ticket-other-workspace", WorkspaceID: "ws-other", RunID: "run-001"},
				{ID: "ticket-other-run", WorkspaceID: "ws-001", RunID: "run-other"},
				nil,
			},
		}
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
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	readCount := 0
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandGetResolution, func(data any) decisionevents.DecisionCommandResult {
		readCount++
		cmd, ok := data.(decisionevents.GetResolutionCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want GetResolutionCommand", data)
		}
		if cmd.TicketID != "ticket-resolved" {
			t.Fatalf("ticket command = %v, want ticket-resolved", cmd.TicketID)
		}
		return decisionevents.DecisionCommandResult{
			Resolution: &decisionmodel.Resolution{
				ID:       "resolution-001",
				TicketID: "ticket-resolved",
				Type:     "provide_missing_info",
			},
		}
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
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandEnsureResolved, func(data any) decisionevents.DecisionCommandResult {
		cmd, ok := data.(decisionevents.EnsureResolvedCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want EnsureResolvedCommand", data)
		}
		if cmd.TicketID != "ticket-contract" || cmd.WorkspaceID != "ws-contract" || cmd.RunID != "run-contract" || cmd.Type != "provide_missing_info" {
			t.Fatalf("command = %+v", cmd)
		}
		if cmd.ResolutionID != "resolution-ws-contract-ticket-contract-provide_missing_info" {
			t.Fatalf("resolutionID = %q, want workspace-scoped id", cmd.ResolutionID)
		}
		if answer, _ := workorcheventpayload.New(cmd.Payload...).String("answer"); answer != "42" {
			t.Fatalf("command payload = %#v", cmd.Payload)
		}
		ticket := &decisionmodel.Ticket{ID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, State: common.DecisionTicketStateResolved}
		resolution := &decisionmodel.Resolution{ID: cmd.ResolutionID, TicketID: cmd.TicketID, Type: cmd.Type, Payload: workorcheventpayload.New(cmd.Payload...)}
		return decisionevents.DecisionCommandResult{Ticket: ticket, Resolution: resolution}
	})

	ticket, resolution, err := ResolveDecision(ctx, hub, &decisionmodel.Ticket{
		ID:          "ticket-contract",
		WorkspaceID: "ws-contract",
		RunID:       "run-contract",
		StepID:      "step-contract",
	}, "provide_missing_info", mustGovernancePayload(t, map[string]any{"answer": "42"}))
	if err != nil {
		t.Fatalf("ResolveDecision: %v", err)
	}
	if ticket.ID != "ticket-contract" || resolution.Type != "provide_missing_info" {
		t.Fatalf("ticket/resolution = %+v / %+v", ticket, resolution)
	}
}

func TestResolveDecisionScopesResolutionIDByWorkspace(t *testing.T) {
	ctx := context.Background()
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	var resolutionIDs []common.ResolutionID
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandEnsureResolved, func(data any) decisionevents.DecisionCommandResult {
		cmd, ok := data.(decisionevents.EnsureResolvedCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want EnsureResolvedCommand", data)
		}
		resolutionIDs = append(resolutionIDs, cmd.ResolutionID)
		ticket := &decisionmodel.Ticket{ID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, State: common.DecisionTicketStateResolved}
		resolution := &decisionmodel.Resolution{ID: cmd.ResolutionID, TicketID: cmd.TicketID, Type: cmd.Type, Payload: workorcheventpayload.New(cmd.Payload...)}
		return decisionevents.DecisionCommandResult{Ticket: ticket, Resolution: resolution}
	})

	for _, workspaceID := range []common.WorkspaceID{"ws-scope-a", "ws-scope-b"} {
		_, _, err := ResolveDecision(ctx, hub, &decisionmodel.Ticket{
			ID:          "ticket-same",
			WorkspaceID: workspaceID,
			RunID:       "run-same",
			StepID:      "run-same/decision",
		}, "replan", mustGovernancePayload(t, map[string]any{"operator": "scope-test"}))
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
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandEnsureApproved, func(data any) approvalevents.ApprovalCommandResult {
		cmd, ok := data.(approvalevents.EnsureApprovedCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want EnsureApprovedCommand", data)
		}
		if cmd.ApprovalID != "approval-contract" {
			t.Fatalf("command = %+v", cmd)
		}
		return approvalevents.ApprovalCommandResult{Approval: &approvalmodel.Approval{ID: cmd.ApprovalID, State: common.ApprovalStateApproved}}
	})

	item, err := ApproveApproval(ctx, hub, "approval-contract")
	if err != nil {
		t.Fatalf("ApproveApproval: %v", err)
	}
	if item.ID != "approval-contract" || item.State != common.ApprovalStateApproved {
		t.Fatalf("approval = %+v", item)
	}
}

func TestResolveApprovalItemPublishesScopedContractCommand(t *testing.T) {
	ctx := context.Background()
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandEnsureApproved, func(data any) approvalevents.ApprovalCommandResult {
		cmd, ok := data.(approvalevents.EnsureApprovedCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want EnsureApprovedCommand", data)
		}
		if cmd.ApprovalID != "approval-scoped-contract" || cmd.WorkspaceID != "ws-scoped-contract" || cmd.RunID != "run-scoped-contract" {
			t.Fatalf("command = %+v, want scoped approval command", cmd)
		}
		return approvalevents.ApprovalCommandResult{
			Approval: &approvalmodel.Approval{
				ID:          cmd.ApprovalID,
				WorkspaceID: cmd.WorkspaceID,
				RunID:       cmd.RunID,
				State:       common.ApprovalStateApproved,
			},
		}
	})

	item, err := ApproveApprovalItem(ctx, hub, &approvalmodel.Approval{
		ID:          "approval-scoped-contract",
		WorkspaceID: "ws-scoped-contract",
		RunID:       "run-scoped-contract",
		State:       common.ApprovalStatePending,
	})
	if err != nil {
		t.Fatalf("ApproveApprovalItem: %v", err)
	}
	if item.ID != "approval-scoped-contract" || item.WorkspaceID != "ws-scoped-contract" || item.RunID != "run-scoped-contract" || item.State != common.ApprovalStateApproved {
		t.Fatalf("approval = %+v, want scoped approved item", item)
	}
}

type governanceMagicTestHub struct {
	rawHub           magicevent.Hub
	decisionObserver magicevent.SimpleObserver
	approvalObserver magicevent.SimpleObserver
}

func newGovernanceMagicTestHub(t *testing.T) *governanceMagicTestHub {
	t.Helper()
	rawHub := magicevent.NewHub(16)
	ret := &governanceMagicTestHub{
		rawHub:           rawHub,
		decisionObserver: magicevent.NewSimpleObserver(decisionevents.CommandDestination, rawHub),
		approvalObserver: magicevent.NewSimpleObserver(approvalevents.CommandDestination, rawHub),
	}
	t.Cleanup(func() {
		rawHub.Terminate(context.Background())
	})
	return ret
}

func (h *governanceMagicTestHub) subscribeDecision(topic string, handler func(any) decisionevents.DecisionCommandResult) {
	h.decisionObserver.Subscribe(topic, func(ev magicevent.Event, result magicevent.Result) {
		result.Set(handler(ev.Data()), nil)
	})
}

func (h *governanceMagicTestHub) subscribeApproval(topic string, handler func(any) approvalevents.ApprovalCommandResult) {
	h.approvalObserver.Subscribe(topic, func(ev magicevent.Event, result magicevent.Result) {
		result.Set(handler(ev.Data()), nil)
	})
}

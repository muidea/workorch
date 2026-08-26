package governanceports

import (
	"context"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
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
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandListByRun, func(data any) approvalTestCommandResult {
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
		return approvalTestCommandResult{
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
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandListTicketsByRun, func(data any) any {
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
		return decisionevents.ListTicketsByRunResult{
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
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandGetResolution, func(data any) any {
		readCount++
		cmd, ok := data.(decisionevents.GetResolutionCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want GetResolutionCommand", data)
		}
		if cmd.TicketID != "ticket-resolved" {
			t.Fatalf("ticket command = %v, want ticket-resolved", cmd.TicketID)
		}
		return decisionevents.GetResolutionResult{
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
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandEnsureResolved, func(data any) any {
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
		return decisionevents.EnsureResolvedResult{Ticket: ticket, Resolution: resolution}
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
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandEnsureResolved, func(data any) any {
		cmd, ok := data.(decisionevents.EnsureResolvedCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want EnsureResolvedCommand", data)
		}
		resolutionIDs = append(resolutionIDs, cmd.ResolutionID)
		ticket := &decisionmodel.Ticket{ID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, State: common.DecisionTicketStateResolved}
		resolution := &decisionmodel.Resolution{ID: cmd.ResolutionID, TicketID: cmd.TicketID, Type: cmd.Type, Payload: workorcheventpayload.New(cmd.Payload...)}
		return decisionevents.EnsureResolvedResult{Ticket: ticket, Resolution: resolution}
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
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandEnsureApproved, func(data any) approvalTestCommandResult {
		cmd, ok := data.(approvalevents.EnsureApprovedCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want EnsureApprovedCommand", data)
		}
		if cmd.ApprovalID != "approval-contract" || cmd.WorkspaceID != "ws-contract" || cmd.RunID != "run-contract" {
			t.Fatalf("command = %+v", cmd)
		}
		return approvalTestCommandResult{Approval: &approvalmodel.Approval{ID: cmd.ApprovalID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, State: common.ApprovalStateApproved}}
	})

	item, err := ApproveApprovalItem(ctx, hub, &approvalmodel.Approval{ID: "approval-contract", WorkspaceID: "ws-contract", RunID: "run-contract"})
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
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandEnsureApproved, func(data any) approvalTestCommandResult {
		cmd, ok := data.(approvalevents.EnsureApprovedCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want EnsureApprovedCommand", data)
		}
		if cmd.ApprovalID != "approval-scoped-contract" || cmd.WorkspaceID != "ws-scoped-contract" || cmd.RunID != "run-scoped-contract" {
			t.Fatalf("command = %+v, want scoped approval command", cmd)
		}
		return approvalTestCommandResult{
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
	t                 *testing.T
	rawHub            magicevent.Hub
	decisionObserver  magicevent.SimpleObserver
	approvalObserver  magicevent.SimpleObserver
	workspaceObserver magicevent.SimpleObserver
	runObserver       magicevent.SimpleObserver
}

type governanceCanonicalFixture struct {
	WorkspaceID        common.WorkspaceID
	RunID              common.RunID
	RunState           common.RunState
	BlockingTicketID   common.TicketID
	BlockingApprovalID common.ApprovalID
}

// approvalTestCommandResult is intentionally test-local. Production Approval
// topics expose distinct result types; this helper only keeps fixture setup
// compact while converting each topic to its concrete contract below.
type approvalTestCommandResult struct {
	Approval  *approvalmodel.Approval
	Approvals []*approvalmodel.Approval
}

func newGovernanceMagicTestHub(t *testing.T) *governanceMagicTestHub {
	t.Helper()
	rawHub := magicevent.NewHub(16)
	ret := &governanceMagicTestHub{
		t:                 t,
		rawHub:            rawHub,
		decisionObserver:  magicevent.NewSimpleObserver(decisionevents.CommandDestination, rawHub),
		approvalObserver:  magicevent.NewSimpleObserver(approvalevents.CommandDestination, rawHub),
		workspaceObserver: magicevent.NewSimpleObserver(workspaceevents.CommandDestination, rawHub),
		runObserver:       magicevent.NewSimpleObserver(runevents.CommandDestination, rawHub),
	}
	t.Cleanup(func() {
		rawHub.Terminate(context.Background())
	})
	return ret
}

func (h *governanceMagicTestHub) subscribeCanonicalGovernance(fixtures ...governanceCanonicalFixture) {
	byWorkspace := make(map[common.WorkspaceID]governanceCanonicalFixture, len(fixtures))
	byRun := make(map[common.RunID]governanceCanonicalFixture, len(fixtures))
	for _, fixture := range fixtures {
		byWorkspace[fixture.WorkspaceID] = fixture
		byRun[fixture.RunID] = fixture
	}
	h.workspaceObserver.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(workspaceevents.GetWorkspaceCommand)
		if !ok {
			h.t.Fatalf("workspace command payload type = %T, want GetWorkspaceCommand", ev.Data())
		}
		fixture, ok := byWorkspace[cmd.WorkspaceID]
		if !ok {
			h.t.Fatalf("unexpected workspace governance admission read: %q", cmd.WorkspaceID)
		}
		result.Set(workspaceevents.WorkspaceResult{Workspace: &workspaceevents.Workspace{
			ID:          fixture.WorkspaceID,
			ActiveRunID: fixture.RunID,
		}}, nil)
	})
	h.runObserver.Subscribe(runevents.TopicRunCommandGet, func(ev magicevent.Event, result magicevent.Result) {
		cmd, ok := ev.Data().(runevents.GetRunCommand)
		if !ok {
			h.t.Fatalf("run command payload type = %T, want GetRunCommand", ev.Data())
		}
		fixture, ok := byRun[cmd.RunID]
		if !ok || cmd.WorkspaceID != fixture.WorkspaceID {
			h.t.Fatalf("unexpected run governance admission read: workspace=%q run=%q", cmd.WorkspaceID, cmd.RunID)
		}
		result.Set(runevents.GetRunResult{Run: &runmodel.Run{
			ID:                 fixture.RunID,
			WorkspaceID:        fixture.WorkspaceID,
			State:              fixture.RunState,
			BlockingTicketID:   fixture.BlockingTicketID,
			BlockingApprovalID: fixture.BlockingApprovalID,
		}}, nil)
	})
}

func (h *governanceMagicTestHub) subscribeDecision(topic string, handler func(any) any) {
	h.decisionObserver.Subscribe(topic, func(ev magicevent.Event, result magicevent.Result) {
		result.Set(handler(ev.Data()), nil)
	})
}

func (h *governanceMagicTestHub) subscribeApproval(topic string, handler func(any) approvalTestCommandResult) {
	h.approvalObserver.Subscribe(topic, func(ev magicevent.Event, result magicevent.Result) {
		response := handler(ev.Data())
		switch topic {
		case approvalevents.TopicApprovalCommandGet:
			result.Set(approvalevents.GetApprovalResult{Approval: response.Approval}, nil)
		case approvalevents.TopicApprovalCommandList:
			result.Set(approvalevents.ListApprovalsResult{Approvals: response.Approvals}, nil)
		case approvalevents.TopicApprovalCommandListByRun:
			result.Set(approvalevents.ListApprovalsByRunResult{Approvals: response.Approvals}, nil)
		case approvalevents.TopicApprovalCommandListByWorkspace:
			result.Set(approvalevents.ListApprovalsByWorkspaceResult{Approvals: response.Approvals}, nil)
		case approvalevents.TopicApprovalCommandEnsureApproved:
			result.Set(approvalevents.EnsureApprovedResult{Approval: response.Approval}, nil)
		case approvalevents.TopicApprovalCommandEnsureRejected:
			result.Set(approvalevents.EnsureRejectedResult{Approval: response.Approval}, nil)
		default:
			panic("unsupported approval test topic: " + topic)
		}
	})
}

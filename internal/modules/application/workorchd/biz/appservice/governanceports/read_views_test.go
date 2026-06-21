package governanceports

import (
	"context"
	"testing"

	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	approvalcontracts "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/contracts"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisioncontracts "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/contracts"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

type governanceTraceReaderFunc func(context.Context, repository.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error)

func (fn governanceTraceReaderFunc) List(ctx context.Context, query repository.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error) {
	return fn(ctx, query)
}

func TestDecisionPageProjectsResolutionAndAutoGovernance(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-governance-read")
	hub.Subscribe(decisionevents.TopicDecisionCommandListTicketsByWS, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyTickets: []*decisionmodel.Ticket{
					{
						ID:          "ticket-001",
						WorkspaceID: workspaceID,
						RunID:       "run-001",
						State:       common.DecisionTicketStateResolved,
						CreatedAt:   "2026-06-05T01:00:00Z",
					},
				},
			},
		})
	})
	hub.Subscribe(decisionevents.TopicDecisionCommandGetResolution, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyResolution: &decisionmodel.Resolution{
					ID:       "resolution-001",
					TicketID: common.TicketID("ticket-001"),
					Type:     "provide_missing_info",
				},
			},
		})
	})
	traces := governanceTraceReaderFunc(func(_ context.Context, query repository.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error) {
		if query.ItemType != "ticket" || query.ItemID != "ticket-001" {
			t.Fatalf("trace query = %+v, want ticket-001", query)
		}
		return []*governancemodel.AutoGovernanceTrace{{
			ID:          "trace-001",
			WorkspaceID: workspaceID,
			RunID:       "run-001",
			ItemType:    "ticket",
			ItemID:      "ticket-001",
			Action:      "auto_ticket",
			Status:      "created",
			OccurredAt:  "2026-06-05T01:01:00Z",
		}}, nil
	})

	page, err := DecisionPage(ctx, hub, traces, appmodelcore.DecisionQuery{WorkspaceID: string(workspaceID), Limit: 10})
	if err != nil {
		t.Fatalf("DecisionPage err = %v", err)
	}
	if len(page.Decisions) != 1 {
		t.Fatalf("decisions = %#v, want one item", page.Decisions)
	}
	item := page.Decisions[0]
	if item.ID != "ticket-001" || item.ResolutionID != "resolution-001" || item.ResolutionType != "provide_missing_info" {
		t.Fatalf("decision = %+v, want resolution projection", item)
	}
	if item.AutoGovernance == nil || item.AutoGovernance.TraceID != "trace-001" {
		t.Fatalf("auto governance = %+v, want trace-001", item.AutoGovernance)
	}
	if page.Limit != 10 || page.NextCursorAfterID != "ticket-001" {
		t.Fatalf("page = %+v, want limit 10 and next cursor ticket-001", page)
	}
}

func TestResolveDecisionViewResolvesOpenTicket(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-governance-read")
	hub.Subscribe(decisionevents.TopicDecisionCommandGetTicket, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyTicket: &decisionmodel.Ticket{
					ID:          "ticket-001",
					WorkspaceID: workspaceID,
					RunID:       "run-001",
					State:       common.DecisionTicketStateOpen,
				},
			},
		})
	})
	hub.Subscribe(decisionevents.TopicDecisionCommandEnsureResolved, func(_ context.Context, envelope events.Envelope) error {
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(decisioncontracts.ResolveTicketCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want ResolveTicketCommand", envelope.Payload[events.PayloadKeyCommand])
		}
		if cmd.Type != "provide_missing_info" {
			t.Fatalf("resolution type = %q, want provide_missing_info", cmd.Type)
		}
		if cmd.Payload["answer"] != "resume" {
			t.Fatalf("payload = %#v, want answer resume", cmd.Payload)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyTicket: &decisionmodel.Ticket{
					ID:          "ticket-001",
					WorkspaceID: workspaceID,
					RunID:       "run-001",
					State:       common.DecisionTicketStateResolved,
				},
				events.PayloadKeyResolution: &decisionmodel.Resolution{
					ID:       "resolution-001",
					TicketID: "ticket-001",
					Type:     cmd.Type,
					Payload:  cmd.Payload,
				},
			},
		})
	})

	item, err := ResolveDecisionView(ctx, hub, nil, "ticket-001", appmodelcore.ResolveDecisionCommand{
		ResolutionType:    "provide_missing_info",
		ResolutionPayload: map[string]any{"answer": "resume"},
	}, workspaceID)
	if err != nil {
		t.Fatalf("ResolveDecisionView err = %v", err)
	}
	if item.ID != "ticket-001" || item.State != string(common.DecisionTicketStateResolved) || item.ResolutionID != "resolution-001" {
		t.Fatalf("item = %+v, want resolved ticket with resolution", item)
	}
}

func TestDecisionViewReadsTicketWithExplicitScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-governance-detail-scope")
	hub.Subscribe(decisionevents.TopicDecisionCommandGetTicket, func(_ context.Context, envelope events.Envelope) error {
		if envelope.WorkspaceID != string(workspaceID) {
			t.Fatalf("get ticket envelope workspace = %q, want %s", envelope.WorkspaceID, workspaceID)
		}
		if envelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) {
			t.Fatalf("get ticket payload workspace = %v, want %s", envelope.Payload[events.PayloadKeyWorkspaceID], workspaceID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyTicket: &decisionmodel.Ticket{
					ID:          "ticket-detail-scope",
					WorkspaceID: workspaceID,
					RunID:       "run-001",
					State:       common.DecisionTicketStateOpen,
				},
			},
		})
	})

	item, err := DecisionView(ctx, hub, nil, "ticket-detail-scope", workspaceID)
	if err != nil {
		t.Fatalf("DecisionView err = %v", err)
	}
	if item.ID != "ticket-detail-scope" || item.WorkspaceID != string(workspaceID) {
		t.Fatalf("item = %+v, want scoped decision view", item)
	}
}

func TestResolveDecisionViewWithoutScopePrefersOpenDuplicateTicket(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	hub.Subscribe(decisionevents.TopicDecisionCommandListTickets, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyTickets: []*decisionmodel.Ticket{
					{
						ID:          "ticket-duplicate",
						WorkspaceID: "ws-old",
						RunID:       "run-002",
						State:       common.DecisionTicketStateResolved,
					},
					{
						ID:          "ticket-duplicate",
						WorkspaceID: "ws-current",
						RunID:       "run-002",
						State:       common.DecisionTicketStateOpen,
					},
				},
			},
		})
	})
	hub.Subscribe(decisionevents.TopicDecisionCommandEnsureResolved, func(_ context.Context, envelope events.Envelope) error {
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(decisioncontracts.ResolveTicketCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want ResolveTicketCommand", envelope.Payload[events.PayloadKeyCommand])
		}
		if cmd.WorkspaceID != "ws-current" || cmd.TicketID != "ticket-duplicate" {
			t.Fatalf("command scope = workspace:%q ticket:%q, want current open ticket", cmd.WorkspaceID, cmd.TicketID)
		}
		ticket := &decisionmodel.Ticket{ID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, State: common.DecisionTicketStateResolved}
		resolution := &decisionmodel.Resolution{ID: cmd.ResolutionID, TicketID: cmd.TicketID, Type: cmd.Type, Payload: cmd.Payload}
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response", Payload: map[string]any{
			events.PayloadKeyTicket:     ticket,
			events.PayloadKeyResolution: resolution,
		}})
	})

	item, err := ResolveDecisionView(ctx, hub, nil, "ticket-duplicate", appmodelcore.ResolveDecisionCommand{
		ResolutionType:    "replan",
		ResolutionPayload: map[string]any{"operator": "test"},
	}, "")
	if err != nil {
		t.Fatalf("ResolveDecisionView err = %v", err)
	}
	if item.WorkspaceID != "ws-current" || item.State != string(common.DecisionTicketStateResolved) {
		t.Fatalf("item = %+v, want resolved current workspace ticket", item)
	}
}

func TestBatchResolveDecisionViewsReadsTicketsWithExplicitScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-governance-scope")
	ticketIDs := []string{"ticket-scope-001", "ticket-scope-002"}
	hub.Subscribe(decisionevents.TopicDecisionCommandGetTicket, func(_ context.Context, envelope events.Envelope) error {
		if envelope.WorkspaceID != string(workspaceID) {
			t.Fatalf("get ticket envelope workspace = %q, want %s", envelope.WorkspaceID, workspaceID)
		}
		if envelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) {
			t.Fatalf("get ticket payload workspace = %v, want %s", envelope.Payload[events.PayloadKeyWorkspaceID], workspaceID)
		}
		ticketID := common.TicketID(envelope.Payload[events.PayloadKeyBlockingTicketID].(string))
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyTicket: &decisionmodel.Ticket{
					ID:          ticketID,
					WorkspaceID: workspaceID,
					RunID:       "run-001",
					State:       common.DecisionTicketStateOpen,
				},
			},
		})
	})
	resolvedCount := 0
	hub.Subscribe(decisionevents.TopicDecisionCommandEnsureResolved, func(_ context.Context, envelope events.Envelope) error {
		resolvedCount++
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(decisioncontracts.ResolveTicketCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want ResolveTicketCommand", envelope.Payload[events.PayloadKeyCommand])
		}
		if cmd.WorkspaceID != workspaceID || cmd.RunID != "run-001" {
			t.Fatalf("decision command scope = workspace:%q run:%q, want %s/run-001", cmd.WorkspaceID, cmd.RunID, workspaceID)
		}
		ticket := &decisionmodel.Ticket{ID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, State: common.DecisionTicketStateResolved}
		resolution := &decisionmodel.Resolution{ID: cmd.ResolutionID, TicketID: cmd.TicketID, Type: cmd.Type, Payload: cmd.Payload}
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response", Payload: map[string]any{
			events.PayloadKeyTicket:     ticket,
			events.PayloadKeyResolution: resolution,
		}})
	})

	items, err := BatchResolveDecisionViews(ctx, hub, nil, appmodelcore.BatchResolveDecisionCommand{
		TicketIDs:         ticketIDs,
		ResolutionType:    "provide_missing_info",
		ResolutionPayload: map[string]any{"answer": "resume"},
	}, workspaceID)
	if err != nil {
		t.Fatalf("BatchResolveDecisionViews err = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %#v, want two decisions", items)
	}
	if resolvedCount != 2 {
		t.Fatalf("resolved count = %d, want 2", resolvedCount)
	}
	for idx, item := range items {
		if item.ID != ticketIDs[idx] || item.WorkspaceID != string(workspaceID) || item.State != string(common.DecisionTicketStateResolved) {
			t.Fatalf("item[%d] = %+v, want resolved %s", idx, item, ticketIDs[idx])
		}
	}
}

func TestApprovalViewsForQueryProjectsAutoGovernance(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-governance-read")
	hub.Subscribe(approvalevents.TopicApprovalCommandListByRun, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyItems: []*approvalmodel.Approval{
					{
						ID:          "approval-001",
						WorkspaceID: workspaceID,
						RunID:       "run-001",
						State:       common.ApprovalStatePending,
					},
				},
			},
		})
	})
	traces := governanceTraceReaderFunc(func(_ context.Context, query repository.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error) {
		if query.ItemType != "approval" || query.ItemID != "approval-001" {
			t.Fatalf("trace query = %+v, want approval-001", query)
		}
		return []*governancemodel.AutoGovernanceTrace{{
			ID:          "trace-approval-001",
			WorkspaceID: workspaceID,
			RunID:       "run-001",
			ItemType:    "approval",
			ItemID:      "approval-001",
			Action:      "auto_approval",
			Status:      "created",
			OccurredAt:  "2026-06-05T01:01:00Z",
		}}, nil
	})

	items, err := ApprovalViewsForQuery(ctx, hub, traces, "", "run-001")
	if err != nil {
		t.Fatalf("ApprovalViewsForQuery err = %v", err)
	}
	if len(items) != 1 || items[0].ID != "approval-001" {
		t.Fatalf("items = %#v, want approval-001", items)
	}
	if items[0].AutoGovernance == nil || items[0].AutoGovernance.TraceID != "trace-approval-001" {
		t.Fatalf("auto governance = %+v, want trace-approval-001", items[0].AutoGovernance)
	}
}

func TestApprovalViewReadsApprovalWithExplicitScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-governance-detail-scope")
	hub.Subscribe(approvalevents.TopicApprovalCommandGet, func(_ context.Context, envelope events.Envelope) error {
		if envelope.WorkspaceID != string(workspaceID) {
			t.Fatalf("get approval envelope workspace = %q, want %s", envelope.WorkspaceID, workspaceID)
		}
		if envelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) {
			t.Fatalf("get approval payload workspace = %v, want %s", envelope.Payload[events.PayloadKeyWorkspaceID], workspaceID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyApproval: &approvalmodel.Approval{
					ID:          "approval-detail-scope",
					WorkspaceID: workspaceID,
					RunID:       "run-001",
					State:       common.ApprovalStatePending,
				},
			},
		})
	})

	item, err := ApprovalView(ctx, hub, nil, "approval-detail-scope", workspaceID)
	if err != nil {
		t.Fatalf("ApprovalView err = %v", err)
	}
	if item.ID != "approval-detail-scope" || item.WorkspaceID != string(workspaceID) {
		t.Fatalf("item = %+v, want scoped approval view", item)
	}
}

func TestResolveApprovalViewReadsApprovalWithExplicitScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-governance-scope")
	hub.Subscribe(approvalevents.TopicApprovalCommandGet, func(_ context.Context, envelope events.Envelope) error {
		if envelope.WorkspaceID != string(workspaceID) {
			t.Fatalf("get approval envelope workspace = %q, want %s", envelope.WorkspaceID, workspaceID)
		}
		if envelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) {
			t.Fatalf("get approval payload workspace = %v, want %s", envelope.Payload[events.PayloadKeyWorkspaceID], workspaceID)
		}
		approvalID := common.ApprovalID(envelope.Payload[events.PayloadKeyApprovalID].(string))
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyApproval: &approvalmodel.Approval{
					ID:          approvalID,
					WorkspaceID: workspaceID,
					RunID:       "run-001",
					State:       common.ApprovalStatePending,
				},
			},
		})
	})
	hub.Subscribe(approvalevents.TopicApprovalCommandEnsureApproved, func(_ context.Context, envelope events.Envelope) error {
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(approvalcontracts.ResolveApprovalCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want ResolveApprovalCommand", envelope.Payload[events.PayloadKeyCommand])
		}
		if cmd.WorkspaceID != workspaceID || cmd.RunID != "run-001" {
			t.Fatalf("approval command scope = workspace:%q run:%q, want %s/run-001", cmd.WorkspaceID, cmd.RunID, workspaceID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyApproval: &approvalmodel.Approval{
					ID:          cmd.ApprovalID,
					WorkspaceID: cmd.WorkspaceID,
					RunID:       cmd.RunID,
					State:       common.ApprovalStateApproved,
				},
			},
		})
	})

	item, err := ResolveApprovalView(ctx, hub, nil, "approval-scope-001", "approve", workspaceID)
	if err != nil {
		t.Fatalf("ResolveApprovalView err = %v", err)
	}
	if item.ID != "approval-scope-001" || item.WorkspaceID != string(workspaceID) || item.State != string(common.ApprovalStateApproved) {
		t.Fatalf("item = %+v, want scoped approved approval", item)
	}
}

func TestBatchResolveApprovalViewsApprovesPendingItems(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-governance-read")
	approvalIDs := []string{"approval-001", "approval-002"}
	hub.Subscribe(approvalevents.TopicApprovalCommandGet, func(_ context.Context, envelope events.Envelope) error {
		if envelope.WorkspaceID != string(workspaceID) {
			t.Fatalf("get approval envelope workspace = %q, want %s", envelope.WorkspaceID, workspaceID)
		}
		if envelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) {
			t.Fatalf("get approval payload workspace = %v, want %s", envelope.Payload[events.PayloadKeyWorkspaceID], workspaceID)
		}
		approvalID := common.ApprovalID(envelope.Payload[events.PayloadKeyApprovalID].(string))
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyApproval: &approvalmodel.Approval{
					ID:          approvalID,
					WorkspaceID: workspaceID,
					RunID:       "run-001",
					State:       common.ApprovalStatePending,
				},
			},
		})
	})
	approvedCount := 0
	hub.Subscribe(approvalevents.TopicApprovalCommandEnsureApproved, func(_ context.Context, envelope events.Envelope) error {
		approvedCount++
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(approvalcontracts.ResolveApprovalCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want ResolveApprovalCommand", envelope.Payload[events.PayloadKeyCommand])
		}
		approvalID := cmd.ApprovalID
		if cmd.WorkspaceID != workspaceID || cmd.RunID != "run-001" {
			t.Fatalf("approval command scope = workspace:%q run:%q, want %s/run-001", cmd.WorkspaceID, cmd.RunID, workspaceID)
		}
		if envelope.WorkspaceID != string(workspaceID) || envelope.RunID != "run-001" {
			t.Fatalf("approval envelope scope = workspace:%q run:%q, want %s/run-001", envelope.WorkspaceID, envelope.RunID, workspaceID)
		}
		if envelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) || envelope.Payload[events.PayloadKeyRunID] != "run-001" {
			t.Fatalf("approval payload scope = workspace:%v run:%v, want %s/run-001", envelope.Payload[events.PayloadKeyWorkspaceID], envelope.Payload[events.PayloadKeyRunID], workspaceID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyApproval: &approvalmodel.Approval{
					ID:          approvalID,
					WorkspaceID: workspaceID,
					RunID:       "run-001",
					State:       common.ApprovalStateApproved,
				},
			},
		})
	})

	items, err := BatchResolveApprovalViews(ctx, hub, nil, appmodelcore.BatchResolveApprovalCommand{
		ApprovalIDs: approvalIDs,
		Action:      "approve",
	}, workspaceID)
	if err != nil {
		t.Fatalf("BatchResolveApprovalViews err = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %#v, want two approvals", items)
	}
	for idx, item := range items {
		if item.ID != approvalIDs[idx] || item.State != string(common.ApprovalStateApproved) {
			t.Fatalf("item[%d] = %+v, want approved %s", idx, item, approvalIDs[idx])
		}
	}
	if approvedCount != 2 {
		t.Fatalf("approved count = %d, want 2", approvedCount)
	}
}

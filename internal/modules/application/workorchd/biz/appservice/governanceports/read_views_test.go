package governanceports

import (
	"context"
	"testing"

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
	hub.Subscribe(events.TopicDecisionCommandListTicketsByWS, func(_ context.Context, envelope events.Envelope) error {
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
	hub.Subscribe(events.TopicDecisionCommandGetResolution, func(_ context.Context, envelope events.Envelope) error {
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
	hub.Subscribe(events.TopicDecisionCommandGetTicket, func(_ context.Context, envelope events.Envelope) error {
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
	hub.Subscribe(events.TopicDecisionCommandEnsureResolved, func(_ context.Context, envelope events.Envelope) error {
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(decisioncontracts.ResolveTicketCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want ResolveTicketCommand", envelope.Payload[events.PayloadKeyCommand])
		}
		if cmd.Type != "provide_missing_info" {
			t.Fatalf("resolution type = %q, want provide_missing_info", cmd.Type)
		}
		if cmd.Payload["answer"] != "continue" {
			t.Fatalf("payload = %#v, want answer continue", cmd.Payload)
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
		ResolutionPayload: map[string]any{"answer": "continue"},
	}, workspaceID)
	if err != nil {
		t.Fatalf("ResolveDecisionView err = %v", err)
	}
	if item.ID != "ticket-001" || item.State != string(common.DecisionTicketStateResolved) || item.ResolutionID != "resolution-001" {
		t.Fatalf("item = %+v, want resolved ticket with resolution", item)
	}
}

func TestResolveDecisionViewWithoutScopePrefersOpenDuplicateTicket(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	hub.Subscribe(events.TopicDecisionCommandListTickets, func(_ context.Context, envelope events.Envelope) error {
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
	hub.Subscribe(events.TopicDecisionCommandEnsureResolved, func(_ context.Context, envelope events.Envelope) error {
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

func TestApprovalViewsForQueryProjectsAutoGovernance(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-governance-read")
	hub.Subscribe(events.TopicApprovalCommandListByRun, func(_ context.Context, envelope events.Envelope) error {
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

func TestBatchResolveApprovalViewsApprovesPendingItems(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-governance-read")
	approvalIDs := []string{"approval-001", "approval-002"}
	hub.Subscribe(events.TopicApprovalCommandGet, func(_ context.Context, envelope events.Envelope) error {
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
	hub.Subscribe(events.TopicApprovalCommandEnsureApproved, func(_ context.Context, envelope events.Envelope) error {
		approvedCount++
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(approvalcontracts.ResolveApprovalCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want ResolveApprovalCommand", envelope.Payload[events.PayloadKeyCommand])
		}
		approvalID := cmd.ApprovalID
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

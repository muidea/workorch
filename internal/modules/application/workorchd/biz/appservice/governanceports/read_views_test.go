package governanceports

import (
	"context"
	"errors"
	"testing"

	approvalevents "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/events"
	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	approvalcontracts "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/contracts"
	approvalmodel "github.com/muidea/workorch/internal/modules/kernel/approval/pkg/models"
	decisioncontracts "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/contracts"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
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
	if page.Limit != 10 || page.NextCursorAfterID != string(workspaceID)+"\x00ticket-001" {
		t.Fatalf("page = %+v, want limit 10 and scoped next cursor", page)
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
	resumePublished := false
	hub.Subscribe(orchestratorevents.TopicOrchestratorCommandResumeRun, func(_ context.Context, envelope events.Envelope) error {
		resumePublished = true
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response"})
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
	if resumePublished {
		t.Fatal("governance port published direct orchestrator resume; continuation must be owned by workorchd scheduler")
	}
}

func TestResolveDecisionViewRejectsNonSemanticMissingInfoPayload(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]any
	}{
		{name: "empty explicit answer", payload: map[string]any{"answer": ""}},
		{name: "whitespace explicit answer", payload: map[string]any{"answer": "   "}},
		{name: "slash only", payload: map[string]any{"answer": "/"}},
		{name: "decision command prefix only", payload: map[string]any{"answer": "/decision answer"}},
		{name: "run command prefix only", payload: map[string]any{"answer": "/run resume"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			hub := events.NewMemoryHub()
			workspaceID := common.WorkspaceID("ws-governance-invalid-missing-info")
			hub.Subscribe(decisionevents.TopicDecisionCommandGetTicket, func(_ context.Context, envelope events.Envelope) error {
				return events.Response(events.Envelope{
					Topic: envelope.Topic + "/response",
					Payload: map[string]any{
						events.PayloadKeyTicket: &decisionmodel.Ticket{
							ID:          "ticket-invalid-missing-info",
							WorkspaceID: workspaceID,
							RunID:       "run-invalid-missing-info",
							State:       common.DecisionTicketStateOpen,
						},
					},
				})
			})
			resolved := false
			hub.Subscribe(decisionevents.TopicDecisionCommandEnsureResolved, func(_ context.Context, envelope events.Envelope) error {
				resolved = true
				return events.Response(events.Envelope{Topic: envelope.Topic + "/response"})
			})

			_, err := ResolveDecisionView(ctx, hub, nil, "ticket-invalid-missing-info", appmodelcore.ResolveDecisionCommand{
				ResolutionType:    "provide_missing_info",
				ResolutionPayload: tc.payload,
			}, workspaceID)
			if !errors.Is(err, common.ErrInvalidArgument) {
				t.Fatalf("ResolveDecisionView err = %v, want invalid argument", err)
			}
			if resolved {
				t.Fatal("invalid missing-info payload resolved the ticket")
			}
		})
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
	resumeCount := 0
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
	hub.Subscribe(orchestratorevents.TopicOrchestratorCommandResumeRun, func(_ context.Context, envelope events.Envelope) error {
		resumeCount++
		return events.Response(events.Envelope{Topic: envelope.Topic + "/response"})
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
	if resumeCount != 0 {
		t.Fatalf("direct orchestrator resume count = %d, want 0; continuation must be owned by workorchd scheduler", resumeCount)
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

func TestResolveApprovalViewWithCommandPublishesGovernanceSemanticContext(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-approval-semantic")
	hub.Subscribe(approvalevents.TopicApprovalCommandGet, func(_ context.Context, envelope events.Envelope) error {
		approvalID := common.ApprovalID(envelope.Payload[events.PayloadKeyApprovalID].(string))
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyApproval: &approvalmodel.Approval{
					ID:          approvalID,
					WorkspaceID: workspaceID,
					RunID:       "run-approval-semantic",
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
		semantic, ok := cmd.Context["governanceSemantic"].(map[string]any)
		if !ok {
			t.Fatalf("governanceSemantic = %#v, want map", cmd.Context["governanceSemantic"])
		}
		if semantic["intent"] != "approve_execution" || semantic["rawInput"] != "同意继续" || semantic["source"] != "tui" {
			t.Fatalf("governanceSemantic = %#v, want approve rawInput/source", semantic)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyApproval: &approvalmodel.Approval{
					ID:          cmd.ApprovalID,
					WorkspaceID: cmd.WorkspaceID,
					RunID:       cmd.RunID,
					State:       common.ApprovalStateApproved,
					Context:     cmd.Context,
				},
			},
		})
	})

	item, err := ResolveApprovalViewWithCommand(ctx, hub, nil, "approval-semantic-001", appmodelcore.ResolveApprovalCommand{
		Action: "approve",
		Payload: map[string]any{
			"rawInput": "同意继续",
			"source":   "tui",
		},
	}, workspaceID)
	if err != nil {
		t.Fatalf("ResolveApprovalViewWithCommand err = %v", err)
	}
	semantic, ok := item.Context["governanceSemantic"].(map[string]any)
	if !ok || semantic["intent"] != "approve_execution" {
		t.Fatalf("item semantic = %#v, want approve", item.Context["governanceSemantic"])
	}
}

func TestResolveApprovalViewWithoutScopeResolvesUniquePendingApproval(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-approval-unique")
	hub.Subscribe(approvalevents.TopicApprovalCommandList, func(_ context.Context, envelope events.Envelope) error {
		if envelope.WorkspaceID != "" {
			t.Fatalf("list approval envelope workspace = %q, want empty no-scope list", envelope.WorkspaceID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyItems: []*approvalmodel.Approval{
					{
						ID:          "approval-unique",
						WorkspaceID: workspaceID,
						RunID:       "run-approval-unique",
						State:       common.ApprovalStatePending,
					},
				},
			},
		})
	})
	hub.Subscribe(approvalevents.TopicApprovalCommandEnsureApproved, func(_ context.Context, envelope events.Envelope) error {
		cmd, ok := envelope.Payload[events.PayloadKeyCommand].(approvalcontracts.ResolveApprovalCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want ResolveApprovalCommand", envelope.Payload[events.PayloadKeyCommand])
		}
		if envelope.WorkspaceID != string(workspaceID) || envelope.RunID != "run-approval-unique" {
			t.Fatalf("approval envelope scope = workspace:%q run:%q, want %s/run-approval-unique", envelope.WorkspaceID, envelope.RunID, workspaceID)
		}
		if cmd.WorkspaceID != workspaceID || cmd.RunID != "run-approval-unique" {
			t.Fatalf("approval command scope = workspace:%q run:%q, want %s/run-approval-unique", cmd.WorkspaceID, cmd.RunID, workspaceID)
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

	item, err := ResolveApprovalView(ctx, hub, nil, "approval-unique", "approve", "")
	if err != nil {
		t.Fatalf("ResolveApprovalView err = %v", err)
	}
	if item.ID != "approval-unique" || item.WorkspaceID != string(workspaceID) || item.State != string(common.ApprovalStateApproved) {
		t.Fatalf("item = %+v, want unique scoped approval", item)
	}
}

func TestResolveApprovalViewWithoutScopeConflictsOnDuplicatePendingApprovals(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	hub.Subscribe(approvalevents.TopicApprovalCommandList, func(_ context.Context, envelope events.Envelope) error {
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyItems: []*approvalmodel.Approval{
					{
						ID:          "approval-duplicate",
						WorkspaceID: "ws-approval-duplicate-a",
						RunID:       "run-001",
						State:       common.ApprovalStatePending,
					},
					{
						ID:          "approval-duplicate",
						WorkspaceID: "ws-approval-duplicate-b",
						RunID:       "run-001",
						State:       common.ApprovalStatePending,
					},
				},
			},
		})
	})
	hub.Subscribe(approvalevents.TopicApprovalCommandEnsureApproved, func(context.Context, events.Envelope) error {
		t.Fatal("approval mutation should not be published for duplicate no-scope approval")
		return nil
	})

	_, err := ResolveApprovalView(ctx, hub, nil, "approval-duplicate", "approve", "")
	if err == nil || !errors.Is(err, common.ErrConflict) {
		t.Fatalf("ResolveApprovalView err = %v, want conflict", err)
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

package governanceports

import (
	"context"
	"errors"
	"testing"

	magicevent "github.com/muidea/magicCommon/event"
	approvalevents "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/events"
	decisioncommon "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/common"
	decisionevents "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	approvalmodel "github.com/muidea/workorch/internal/modules/blocks/approval/pkg/models"
	decisionmodel "github.com/muidea/workorch/internal/modules/blocks/decision/pkg/models"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

type governanceTraceReaderFunc func(context.Context, repository.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error)

func (fn governanceTraceReaderFunc) List(ctx context.Context, query repository.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error) {
	return fn(ctx, query)
}

func TestDecisionPageProjectsResolutionAndAutoGovernance(t *testing.T) {
	ctx := context.Background()
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	workspaceID := common.WorkspaceID("ws-governance-read")
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandListTicketsByWS, func(data any) decisionevents.DecisionCommandResult {
		cmd, ok := data.(decisionevents.ListTicketsByWorkspaceCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want ListTicketsByWorkspaceCommand", data)
		}
		if cmd.WorkspaceID != workspaceID {
			t.Fatalf("command workspace = %q, want %s", cmd.WorkspaceID, workspaceID)
		}
		return decisionevents.DecisionCommandResult{
			Tickets: []*decisionmodel.Ticket{
				{
					ID:          "ticket-001",
					WorkspaceID: workspaceID,
					RunID:       "run-001",
					State:       common.DecisionTicketStateResolved,
					CreatedAt:   "2026-06-05T01:00:00Z",
				},
			},
		}
	})
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandGetResolution, func(data any) decisionevents.DecisionCommandResult {
		cmd, ok := data.(decisionevents.GetResolutionCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want GetResolutionCommand", data)
		}
		if cmd.TicketID != "ticket-001" {
			t.Fatalf("command ticket = %q, want ticket-001", cmd.TicketID)
		}
		return decisionevents.DecisionCommandResult{
			Resolution: &decisionmodel.Resolution{
				ID:       "resolution-001",
				TicketID: common.TicketID("ticket-001"),
				Type:     "provide_missing_info",
			},
		}
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
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	workspaceID := common.WorkspaceID("ws-governance-read")
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandGetTicket, func(data any) decisionevents.DecisionCommandResult {
		cmd, ok := data.(decisionevents.GetTicketCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want GetTicketCommand", data)
		}
		if cmd.WorkspaceID != workspaceID || cmd.TicketID != "ticket-001" {
			t.Fatalf("command = %+v, want scoped ticket", cmd)
		}
		return decisionevents.DecisionCommandResult{
			Ticket: &decisionmodel.Ticket{
				ID:          "ticket-001",
				WorkspaceID: workspaceID,
				RunID:       "run-001",
				State:       common.DecisionTicketStateOpen,
			},
		}
	})
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandEnsureResolved, func(data any) decisionevents.DecisionCommandResult {
		cmd, ok := data.(decisionevents.EnsureResolvedCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want EnsureResolvedCommand", data)
		}
		if cmd.Type != "provide_missing_info" {
			t.Fatalf("resolution type = %q, want provide_missing_info", cmd.Type)
		}
		if answer, _ := workorcheventpayload.New(cmd.Payload...).String("answer"); answer != "resume" {
			t.Fatalf("payload = %#v, want answer resume", cmd.Payload)
		}
		return decisionevents.DecisionCommandResult{
			Ticket: &decisionmodel.Ticket{
				ID:          "ticket-001",
				WorkspaceID: workspaceID,
				RunID:       "run-001",
				State:       common.DecisionTicketStateResolved,
			},
			Resolution: &decisionmodel.Resolution{
				ID:       "resolution-001",
				TicketID: "ticket-001",
				Type:     cmd.Type,
				Payload:  workorcheventpayload.New(cmd.Payload...),
			},
		}
	})
	resumePublished := false
	resumeObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	resumeObserver.Subscribe(orchestratorevents.TopicOrchestratorCommandResumeRun, func(_ magicevent.Event, result magicevent.Result) {
		resumePublished = true
		result.Set(orchestratorevents.OrchestratorCommandResult{}, nil)
	})
	defer resumeObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommandResumeRun)

	item, err := ResolveDecisionView(ctx, hub, nil, "ticket-001", appmodelcore.ResolveDecisionCommand{
		ResolutionType:    "provide_missing_info",
		ResolutionPayload: mustGovernancePayload(t, map[string]any{"answer": "resume"}),
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

func TestResolveDecisionViewNormalizesRuntimeV2ReplanToAllowedContinuation(t *testing.T) {
	ctx := context.Background()
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	workspaceID := common.WorkspaceID("ws-governance-runtime-replan")
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandGetTicket, func(data any) decisionevents.DecisionCommandResult {
		cmd, ok := data.(decisionevents.GetTicketCommand)
		if !ok || cmd.WorkspaceID != workspaceID || cmd.TicketID != "ticket-runtime-replan" {
			t.Fatalf("get ticket command = %#v", data)
		}
		return decisionevents.DecisionCommandResult{Ticket: &decisionmodel.Ticket{
			ID:                     "ticket-runtime-replan",
			WorkspaceID:            workspaceID,
			RunID:                  "run-runtime-replan",
			DecisionKind:           "runtime_v2_governance",
			State:                  common.DecisionTicketStateOpen,
			AllowedRecoveryActions: []string{decisioncommon.ResolutionTypeProvideMissingInfo},
		}}
	})
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandEnsureResolved, func(data any) decisionevents.DecisionCommandResult {
		cmd, ok := data.(decisionevents.EnsureResolvedCommand)
		if !ok {
			t.Fatalf("resolve command payload = %T", data)
		}
		if cmd.Type != decisioncommon.ResolutionTypeProvideMissingInfo {
			t.Fatalf("normalized resolution type = %q, want continue resolution", cmd.Type)
		}
		payload := workorcheventpayload.New(cmd.Payload...)
		if requested, _ := payload.String("requestedResolutionType"); requested != decisioncommon.ResolutionTypeReplan {
			t.Fatalf("requestedResolutionType = %q, want replan", requested)
		}
		if intent, _ := payload.String("runtimeContinuationIntent"); intent != "resume" {
			t.Fatalf("runtimeContinuationIntent = %q, want resume", intent)
		}
		return decisionevents.DecisionCommandResult{
			Ticket:     &decisionmodel.Ticket{ID: "ticket-runtime-replan", WorkspaceID: workspaceID, RunID: "run-runtime-replan", DecisionKind: "runtime_v2_governance", State: common.DecisionTicketStateResolved},
			Resolution: &decisionmodel.Resolution{ID: "resolution-runtime-replan", TicketID: "ticket-runtime-replan", Type: cmd.Type, Payload: payload},
		}
	})

	item, err := ResolveDecisionView(ctx, hub, nil, "ticket-runtime-replan", appmodelcore.ResolveDecisionCommand{
		ResolutionType:    decisioncommon.ResolutionTypeReplan,
		ResolutionPayload: mustGovernancePayload(t, map[string]any{"answer": "please retry with the revised plan"}),
	}, workspaceID)
	if err != nil {
		t.Fatalf("ResolveDecisionView err = %v", err)
	}
	if item.State != string(common.DecisionTicketStateResolved) || item.ResolutionType != decisioncommon.ResolutionTypeProvideMissingInfo {
		t.Fatalf("decision = %+v, want resolved continuation", item)
	}
}

func TestResolveDecisionViewReturnsActionableConflictForUnsupportedRuntimeV2Replan(t *testing.T) {
	ctx := context.Background()
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandGetTicket, func(data any) decisionevents.DecisionCommandResult {
		return decisionevents.DecisionCommandResult{Ticket: &decisionmodel.Ticket{
			ID:                     "ticket-runtime-replan-conflict",
			WorkspaceID:            "ws-governance-runtime-replan-conflict",
			RunID:                  "run-runtime-replan-conflict",
			DecisionKind:           "runtime_v2_governance",
			State:                  common.DecisionTicketStateOpen,
			AllowedRecoveryActions: []string{decisioncommon.ResolutionTypeRejectCurrentPath},
		}}
	})
	resolved := false
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandEnsureResolved, func(any) decisionevents.DecisionCommandResult {
		resolved = true
		return decisionevents.DecisionCommandResult{}
	})

	_, err := ResolveDecisionView(ctx, hub, nil, "ticket-runtime-replan-conflict", appmodelcore.ResolveDecisionCommand{ResolutionType: decisioncommon.ResolutionTypeReplan}, "ws-governance-runtime-replan-conflict")
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("ResolveDecisionView err = %v, want conflict", err)
	}
	var actionConflict *decisioncommon.ResolutionActionConflictError
	if !errors.As(err, &actionConflict) || actionConflict.TicketID != "ticket-runtime-replan-conflict" || actionConflict.RequestedType != decisioncommon.ResolutionTypeReplan || len(actionConflict.AllowedActions) != 1 || actionConflict.AllowedActions[0] != decisioncommon.ResolutionTypeRejectCurrentPath {
		t.Fatalf("action conflict = %#v, want ticket-specific remediation", actionConflict)
	}
	if resolved {
		t.Fatal("unsupported replan resolved the ticket")
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
			testHub := newGovernanceMagicTestHub(t)
			hub := testHub.rawHub
			workspaceID := common.WorkspaceID("ws-governance-invalid-missing-info")
			testHub.subscribeDecision(decisionevents.TopicDecisionCommandGetTicket, func(data any) decisionevents.DecisionCommandResult {
				cmd, ok := data.(decisionevents.GetTicketCommand)
				if !ok {
					t.Fatalf("command payload type = %T, want GetTicketCommand", data)
				}
				if cmd.WorkspaceID != workspaceID || cmd.TicketID != "ticket-invalid-missing-info" {
					t.Fatalf("command = %+v, want invalid missing info ticket", cmd)
				}
				return decisionevents.DecisionCommandResult{
					Ticket: &decisionmodel.Ticket{
						ID:          "ticket-invalid-missing-info",
						WorkspaceID: workspaceID,
						RunID:       "run-invalid-missing-info",
						State:       common.DecisionTicketStateOpen,
					},
				}
			})
			resolved := false
			testHub.subscribeDecision(decisionevents.TopicDecisionCommandEnsureResolved, func(data any) decisionevents.DecisionCommandResult {
				resolved = true
				return decisionevents.DecisionCommandResult{}
			})

			_, err := ResolveDecisionView(ctx, hub, nil, "ticket-invalid-missing-info", appmodelcore.ResolveDecisionCommand{
				ResolutionType:    "provide_missing_info",
				ResolutionPayload: mustGovernancePayload(t, tc.payload),
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
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	workspaceID := common.WorkspaceID("ws-governance-detail-scope")
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandGetTicket, func(data any) decisionevents.DecisionCommandResult {
		cmd, ok := data.(decisionevents.GetTicketCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want GetTicketCommand", data)
		}
		if cmd.WorkspaceID != workspaceID {
			t.Fatalf("get ticket command workspace = %q, want %s", cmd.WorkspaceID, workspaceID)
		}
		return decisionevents.DecisionCommandResult{
			Ticket: &decisionmodel.Ticket{
				ID:          "ticket-detail-scope",
				WorkspaceID: workspaceID,
				RunID:       "run-001",
				State:       common.DecisionTicketStateOpen,
			},
		}
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
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandListTickets, func(data any) decisionevents.DecisionCommandResult {
		if _, ok := data.(decisionevents.ListTicketsCommand); !ok {
			t.Fatalf("command payload type = %T, want ListTicketsCommand", data)
		}
		return decisionevents.DecisionCommandResult{
			Tickets: []*decisionmodel.Ticket{
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
		}
	})
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandEnsureResolved, func(data any) decisionevents.DecisionCommandResult {
		cmd, ok := data.(decisionevents.EnsureResolvedCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want EnsureResolvedCommand", data)
		}
		if cmd.WorkspaceID != "ws-current" || cmd.TicketID != "ticket-duplicate" {
			t.Fatalf("command scope = workspace:%q ticket:%q, want current open ticket", cmd.WorkspaceID, cmd.TicketID)
		}
		ticket := &decisionmodel.Ticket{ID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, State: common.DecisionTicketStateResolved}
		resolution := &decisionmodel.Resolution{ID: cmd.ResolutionID, TicketID: cmd.TicketID, Type: cmd.Type, Payload: workorcheventpayload.New(cmd.Payload...)}
		return decisionevents.DecisionCommandResult{Ticket: ticket, Resolution: resolution}
	})

	item, err := ResolveDecisionView(ctx, hub, nil, "ticket-duplicate", appmodelcore.ResolveDecisionCommand{
		ResolutionType:    "replan",
		ResolutionPayload: mustGovernancePayload(t, map[string]any{"operator": "test"}),
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
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	workspaceID := common.WorkspaceID("ws-governance-scope")
	ticketIDs := []string{"ticket-scope-001", "ticket-scope-002"}
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandGetTicket, func(data any) decisionevents.DecisionCommandResult {
		cmd, ok := data.(decisionevents.GetTicketCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want GetTicketCommand", data)
		}
		if cmd.WorkspaceID != workspaceID {
			t.Fatalf("get ticket command workspace = %q, want %s", cmd.WorkspaceID, workspaceID)
		}
		return decisionevents.DecisionCommandResult{
			Ticket: &decisionmodel.Ticket{
				ID:          cmd.TicketID,
				WorkspaceID: workspaceID,
				RunID:       "run-001",
				State:       common.DecisionTicketStateOpen,
			},
		}
	})
	resolvedCount := 0
	resumeCount := 0
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandEnsureResolved, func(data any) decisionevents.DecisionCommandResult {
		resolvedCount++
		cmd, ok := data.(decisionevents.EnsureResolvedCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want EnsureResolvedCommand", data)
		}
		if cmd.WorkspaceID != workspaceID || cmd.RunID != "run-001" {
			t.Fatalf("decision command scope = workspace:%q run:%q, want %s/run-001", cmd.WorkspaceID, cmd.RunID, workspaceID)
		}
		ticket := &decisionmodel.Ticket{ID: cmd.TicketID, WorkspaceID: cmd.WorkspaceID, RunID: cmd.RunID, State: common.DecisionTicketStateResolved}
		resolution := &decisionmodel.Resolution{ID: cmd.ResolutionID, TicketID: cmd.TicketID, Type: cmd.Type, Payload: workorcheventpayload.New(cmd.Payload...)}
		return decisionevents.DecisionCommandResult{Ticket: ticket, Resolution: resolution}
	})
	resumeObserver := magicevent.NewSimpleObserver(orchestratorevents.CommandDestination, hub)
	resumeObserver.Subscribe(orchestratorevents.TopicOrchestratorCommandResumeRun, func(_ magicevent.Event, result magicevent.Result) {
		resumeCount++
		result.Set(orchestratorevents.OrchestratorCommandResult{}, nil)
	})
	defer resumeObserver.Unsubscribe(orchestratorevents.TopicOrchestratorCommandResumeRun)

	items, err := BatchResolveDecisionViews(ctx, hub, nil, appmodelcore.BatchResolveDecisionCommand{
		TicketIDs:         ticketIDs,
		ResolutionType:    "provide_missing_info",
		ResolutionPayload: mustGovernancePayload(t, map[string]any{"answer": "resume"}),
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

func TestBatchResolveDecisionViewsRejectsMixedAllowedActionsBeforeWrites(t *testing.T) {
	ctx := context.Background()
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	workspaceID := common.WorkspaceID("ws-governance-actions")
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandGetTicket, func(data any) decisionevents.DecisionCommandResult {
		cmd := data.(decisionevents.GetTicketCommand)
		actions := []string{"provide_missing_info"}
		if cmd.TicketID == "ticket-action-b" {
			actions = []string{"replan"}
		}
		return decisionevents.DecisionCommandResult{Ticket: &decisionmodel.Ticket{
			ID: cmd.TicketID, WorkspaceID: workspaceID, RunID: "run-actions", State: common.DecisionTicketStateOpen,
			AllowedRecoveryActions: actions,
		}}
	})
	resolvedCount := 0
	testHub.subscribeDecision(decisionevents.TopicDecisionCommandEnsureResolved, func(_ any) decisionevents.DecisionCommandResult {
		resolvedCount++
		return decisionevents.DecisionCommandResult{}
	})
	_, err := BatchResolveDecisionViews(ctx, hub, nil, appmodelcore.BatchResolveDecisionCommand{
		TicketIDs: []string{"ticket-action-a", "ticket-action-b"}, ResolutionType: "provide_missing_info",
		ResolutionPayload: mustGovernancePayload(t, map[string]any{"answer": "continue"}),
	}, workspaceID)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("mixed allowed action batch err = %v, want ticket action conflict", err)
	}
	var actionConflict *decisioncommon.ResolutionActionConflictError
	if !errors.As(err, &actionConflict) || actionConflict.TicketID != "ticket-action-b" ||
		actionConflict.RequestedType != decisioncommon.ResolutionTypeProvideMissingInfo ||
		len(actionConflict.AllowedActions) != 1 || actionConflict.AllowedActions[0] != decisioncommon.ResolutionTypeReplan {
		t.Fatalf("mixed allowed action conflict = %#v, want ticket-specific remediation", actionConflict)
	}
	if resolvedCount != 0 {
		t.Fatalf("mixed allowed action batch wrote %d resolutions, want zero", resolvedCount)
	}
}

func TestApprovalViewsForQueryProjectsAutoGovernance(t *testing.T) {
	ctx := context.Background()
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	workspaceID := common.WorkspaceID("ws-governance-read")
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandListByRun, func(data any) approvalevents.ApprovalCommandResult {
		cmd, ok := data.(approvalevents.ListApprovalsByRunCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want ListApprovalsByRunCommand", data)
		}
		if cmd.RunID != "run-001" {
			t.Fatalf("command run = %q, want run-001", cmd.RunID)
		}
		return approvalevents.ApprovalCommandResult{
			Approvals: []*approvalmodel.Approval{
				{
					ID:          "approval-001",
					WorkspaceID: workspaceID,
					RunID:       "run-001",
					State:       common.ApprovalStatePending,
				},
			},
		}
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
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	workspaceID := common.WorkspaceID("ws-governance-detail-scope")
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandGet, func(data any) approvalevents.ApprovalCommandResult {
		cmd, ok := data.(approvalevents.GetApprovalCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want GetApprovalCommand", data)
		}
		if cmd.WorkspaceID != workspaceID {
			t.Fatalf("get approval command workspace = %q, want %s", cmd.WorkspaceID, workspaceID)
		}
		return approvalevents.ApprovalCommandResult{
			Approval: &approvalmodel.Approval{
				ID:          "approval-detail-scope",
				WorkspaceID: workspaceID,
				RunID:       "run-001",
				State:       common.ApprovalStatePending,
			},
		}
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
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	workspaceID := common.WorkspaceID("ws-governance-scope")
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandGet, func(data any) approvalevents.ApprovalCommandResult {
		cmd, ok := data.(approvalevents.GetApprovalCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want GetApprovalCommand", data)
		}
		if cmd.WorkspaceID != workspaceID {
			t.Fatalf("get approval command workspace = %q, want %s", cmd.WorkspaceID, workspaceID)
		}
		return approvalevents.ApprovalCommandResult{
			Approval: &approvalmodel.Approval{
				ID:          cmd.ApprovalID,
				WorkspaceID: workspaceID,
				RunID:       "run-001",
				State:       common.ApprovalStatePending,
			},
		}
	})
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandEnsureApproved, func(data any) approvalevents.ApprovalCommandResult {
		cmd, ok := data.(approvalevents.EnsureApprovedCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want EnsureApprovedCommand", data)
		}
		if cmd.WorkspaceID != workspaceID || cmd.RunID != "run-001" {
			t.Fatalf("approval command scope = workspace:%q run:%q, want %s/run-001", cmd.WorkspaceID, cmd.RunID, workspaceID)
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
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	workspaceID := common.WorkspaceID("ws-approval-semantic")
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandGet, func(data any) approvalevents.ApprovalCommandResult {
		cmd, ok := data.(approvalevents.GetApprovalCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want GetApprovalCommand", data)
		}
		return approvalevents.ApprovalCommandResult{
			Approval: &approvalmodel.Approval{
				ID:          cmd.ApprovalID,
				WorkspaceID: workspaceID,
				RunID:       "run-approval-semantic",
				State:       common.ApprovalStatePending,
			},
		}
	})
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandEnsureApproved, func(data any) approvalevents.ApprovalCommandResult {
		cmd, ok := data.(approvalevents.EnsureApprovedCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want EnsureApprovedCommand", data)
		}
		contextPayload := workorcheventpayload.New(cmd.Context...)
		semantic, ok := contextPayload.Object("governanceSemantic")
		if !ok {
			t.Fatalf("governanceSemantic missing from %#v", contextPayload)
		}
		intent, _ := semantic.String("intent")
		rawInput, _ := semantic.String("rawInput")
		source, _ := semantic.String("source")
		if intent != "approve_execution" || rawInput != "同意继续" || source != "tui" {
			t.Fatalf("governanceSemantic = %#v, want approve rawInput/source", semantic)
		}
		return approvalevents.ApprovalCommandResult{
			Approval: &approvalmodel.Approval{
				ID:          cmd.ApprovalID,
				WorkspaceID: cmd.WorkspaceID,
				RunID:       cmd.RunID,
				State:       common.ApprovalStateApproved,
				Context:     contextPayload,
			},
		}
	})

	item, err := ResolveApprovalViewWithCommand(ctx, hub, nil, "approval-semantic-001", appmodelcore.ResolveApprovalCommand{
		Action: "approve",
		Payload: mustGovernancePayload(t, map[string]any{
			"rawInput": "同意继续",
			"source":   "tui",
		}),
	}, workspaceID)
	if err != nil {
		t.Fatalf("ResolveApprovalViewWithCommand err = %v", err)
	}
	semantic, ok := item.Context.Object("governanceSemantic")
	intent, _ := semantic.String("intent")
	if !ok || intent != "approve_execution" {
		t.Fatalf("item semantic = %#v, want approve", item.Context)
	}
}

func TestResolveApprovalViewWithoutScopeResolvesUniquePendingApproval(t *testing.T) {
	ctx := context.Background()
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	workspaceID := common.WorkspaceID("ws-approval-unique")
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandList, func(data any) approvalevents.ApprovalCommandResult {
		if _, ok := data.(approvalevents.ListApprovalsCommand); !ok {
			t.Fatalf("command payload type = %T, want ListApprovalsCommand", data)
		}
		return approvalevents.ApprovalCommandResult{
			Approvals: []*approvalmodel.Approval{
				{
					ID:          "approval-unique",
					WorkspaceID: workspaceID,
					RunID:       "run-approval-unique",
					State:       common.ApprovalStatePending,
				},
			},
		}
	})
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandEnsureApproved, func(data any) approvalevents.ApprovalCommandResult {
		cmd, ok := data.(approvalevents.EnsureApprovedCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want EnsureApprovedCommand", data)
		}
		if cmd.WorkspaceID != workspaceID || cmd.RunID != "run-approval-unique" {
			t.Fatalf("approval command scope = workspace:%q run:%q, want %s/run-approval-unique", cmd.WorkspaceID, cmd.RunID, workspaceID)
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
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandList, func(data any) approvalevents.ApprovalCommandResult {
		if _, ok := data.(approvalevents.ListApprovalsCommand); !ok {
			t.Fatalf("command payload type = %T, want ListApprovalsCommand", data)
		}
		return approvalevents.ApprovalCommandResult{
			Approvals: []*approvalmodel.Approval{
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
		}
	})
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandEnsureApproved, func(data any) approvalevents.ApprovalCommandResult {
		t.Fatal("approval mutation should not be published for duplicate no-scope approval")
		return approvalevents.ApprovalCommandResult{}
	})

	_, err := ResolveApprovalView(ctx, hub, nil, "approval-duplicate", "approve", "")
	if err == nil || !errors.Is(err, common.ErrConflict) {
		t.Fatalf("ResolveApprovalView err = %v, want conflict", err)
	}
}

func TestBatchResolveApprovalViewsApprovesPendingItems(t *testing.T) {
	ctx := context.Background()
	testHub := newGovernanceMagicTestHub(t)
	hub := testHub.rawHub
	workspaceID := common.WorkspaceID("ws-governance-read")
	approvalIDs := []string{"approval-001", "approval-002"}
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandGet, func(data any) approvalevents.ApprovalCommandResult {
		cmd, ok := data.(approvalevents.GetApprovalCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want GetApprovalCommand", data)
		}
		if cmd.WorkspaceID != workspaceID {
			t.Fatalf("get approval command workspace = %q, want %s", cmd.WorkspaceID, workspaceID)
		}
		return approvalevents.ApprovalCommandResult{
			Approval: &approvalmodel.Approval{
				ID:          cmd.ApprovalID,
				WorkspaceID: workspaceID,
				RunID:       "run-001",
				State:       common.ApprovalStatePending,
			},
		}
	})
	approvedCount := 0
	testHub.subscribeApproval(approvalevents.TopicApprovalCommandEnsureApproved, func(data any) approvalevents.ApprovalCommandResult {
		approvedCount++
		cmd, ok := data.(approvalevents.EnsureApprovedCommand)
		if !ok {
			t.Fatalf("command payload type = %T, want EnsureApprovedCommand", data)
		}
		approvalID := cmd.ApprovalID
		if cmd.WorkspaceID != workspaceID || cmd.RunID != "run-001" {
			t.Fatalf("approval command scope = workspace:%q run:%q, want %s/run-001", cmd.WorkspaceID, cmd.RunID, workspaceID)
		}
		return approvalevents.ApprovalCommandResult{
			Approval: &approvalmodel.Approval{
				ID:          approvalID,
				WorkspaceID: workspaceID,
				RunID:       "run-001",
				State:       common.ApprovalStateApproved,
			},
		}
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

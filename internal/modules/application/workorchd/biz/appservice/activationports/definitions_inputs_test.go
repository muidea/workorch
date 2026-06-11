package activationports

import (
	"context"
	"testing"

	decisionevents "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/events"
	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func TestReadWorkspaceExecutionContextInputsReadsDefinitionsWithExplicitScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-activation-input-scope")
	goalID := common.GoalID("goal-activation-input-scope")
	dodID := common.DoDID("dod-activation-input-scope")

	hub.Subscribe(workspaceevents.TopicWorkspaceCommandGetGoal, func(_ context.Context, envelope events.Envelope) error {
		if envelope.WorkspaceID != string(workspaceID) {
			t.Fatalf("goal envelope workspace = %q, want %s", envelope.WorkspaceID, workspaceID)
		}
		if envelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) {
			t.Fatalf("goal payload workspace = %v, want %s", envelope.Payload[events.PayloadKeyWorkspaceID], workspaceID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyGoal: &workspacemodel.Goal{
					ID:          goalID,
					WorkspaceID: workspaceID,
					Text:        "scoped goal",
				},
			},
		})
	})
	hub.Subscribe(workspaceevents.TopicWorkspaceCommandGetDoD, func(_ context.Context, envelope events.Envelope) error {
		if envelope.WorkspaceID != string(workspaceID) {
			t.Fatalf("dod envelope workspace = %q, want %s", envelope.WorkspaceID, workspaceID)
		}
		if envelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) {
			t.Fatalf("dod payload workspace = %v, want %s", envelope.Payload[events.PayloadKeyWorkspaceID], workspaceID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyDoD: &workspacemodel.DoD{
					ID:          dodID,
					WorkspaceID: workspaceID,
					Items:       []string{"scoped dod"},
					State:       common.DoDStateReady,
				},
			},
		})
	})
	hub.Subscribe(workspaceevents.TopicWorkspaceCommandGet, func(_ context.Context, envelope events.Envelope) error {
		if envelope.WorkspaceID != string(workspaceID) {
			t.Fatalf("workspace envelope workspace = %q, want %s", envelope.WorkspaceID, workspaceID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyWorkspace: &workspacemodel.Workspace{
					ID:    workspaceID,
					Title: "scoped workspace",
					State: common.WorkspaceStateActive,
				},
			},
		})
	})

	inputs, err := ReadWorkspaceExecutionContextInputs(ctx, hub, workspaceID, goalID, dodID)
	if err != nil {
		t.Fatalf("ReadWorkspaceExecutionContextInputs err = %v", err)
	}
	if inputs.Workspace.ID != workspaceID || inputs.Goal.WorkspaceID != workspaceID || inputs.DoD.WorkspaceID != workspaceID {
		t.Fatalf("inputs = %+v, want workspace scoped inputs", inputs)
	}
}

func TestWorkspaceGovernanceHistoryReadsRunAndResolutionWithWorkspaceScope(t *testing.T) {
	ctx := context.Background()
	hub := events.NewMemoryHub()
	workspaceID := common.WorkspaceID("ws-governance-history-scope")
	runID := common.RunID("run-001")
	ticketID := common.TicketID("run-001-step-1-decision")
	goalID := common.GoalID("goal-history")
	dodID := common.DoDID("dod-history")

	assertScope := func(kind string, envelope events.Envelope) {
		t.Helper()
		if envelope.WorkspaceID != string(workspaceID) {
			t.Fatalf("%s envelope workspace = %q, want %s", kind, envelope.WorkspaceID, workspaceID)
		}
		if envelope.Payload[events.PayloadKeyWorkspaceID] != string(workspaceID) {
			t.Fatalf("%s payload workspace = %v, want %s", kind, envelope.Payload[events.PayloadKeyWorkspaceID], workspaceID)
		}
	}

	hub.Subscribe(decisionevents.TopicDecisionCommandListTicketsByWS, func(_ context.Context, envelope events.Envelope) error {
		assertScope("ticket list", envelope)
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyTickets: []*decisionmodel.Ticket{{
					ID:          ticketID,
					WorkspaceID: workspaceID,
					RunID:       runID,
					StepID:      common.StepID("run-001-step-1"),
					State:       common.DecisionTicketStateResolved,
					Reason:      "history",
					Question:    "question",
				}},
			},
		})
	})
	hub.Subscribe(runevents.TopicRunCommandGet, func(_ context.Context, envelope events.Envelope) error {
		assertScope("run get", envelope)
		if envelope.RunID != string(runID) || envelope.Payload[events.PayloadKeyRunID] != string(runID) {
			t.Fatalf("run get scope = run:%q payload:%v, want %s", envelope.RunID, envelope.Payload[events.PayloadKeyRunID], runID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyRun: &runmodel.Run{
					ID:          runID,
					WorkspaceID: workspaceID,
					GoalID:      goalID,
					DoDID:       dodID,
				},
			},
		})
	})
	hub.Subscribe(decisionevents.TopicDecisionCommandGetResolution, func(_ context.Context, envelope events.Envelope) error {
		assertScope("resolution get", envelope)
		if envelope.Payload[events.PayloadKeyBlockingTicketID] != string(ticketID) {
			t.Fatalf("resolution ticket payload = %v, want %s", envelope.Payload[events.PayloadKeyBlockingTicketID], ticketID)
		}
		return events.Response(events.Envelope{
			Topic: envelope.Topic + "/response",
			Payload: map[string]any{
				events.PayloadKeyResolution: &decisionmodel.Resolution{
					ID:       common.ResolutionID("resolution-1"),
					TicketID: ticketID,
					Type:     "selected",
					Payload:  map[string]any{"choice": "ok"},
				},
			},
		})
	})

	history := WorkspaceGovernanceHistory(ctx, hub, workspaceID, goalID, dodID, 1)
	if len(history) != 1 {
		t.Fatalf("history length = %d, want 1: %+v", len(history), history)
	}
	if history[0]["ticketId"] != string(ticketID) || history[0]["runId"] != string(runID) {
		t.Fatalf("history = %+v, want scoped ticket/run", history[0])
	}
}

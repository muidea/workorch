package readmodelprojection

import (
	"testing"

	decisionmodel "github.com/muidea/workorch/internal/modules/kernel/decision/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestMapDecisionProjectsFailureRecoveryFields(t *testing.T) {
	item := &decisionmodel.Ticket{
		ID:                     "ticket-runtime-recovery",
		WorkspaceID:            "ws-runtime-recovery",
		RunID:                  "run-runtime-recovery",
		Reason:                 "runtime repair exhausted",
		State:                  common.DecisionTicketStateResolved,
		FailureClass:           "model_runtime_failed",
		FailureSignature:       "model-signature",
		AllowedRecoveryActions: []string{"resume"},
		ResolvedAt:             "2026-07-06T01:02:03Z",
		ResolvedBy:             "auto-governance",
		Context: map[string]any{
			"failureClass":           "runtime_failed",
			"failureSignature":       "class=runtime_failed|kind=llm/workspace_execution",
			"allowedRecoveryActions": []any{"replan", "terminate"},
		},
	}
	got := MapDecision(item, nil, nil)
	if got.FailureClass != "model_runtime_failed" ||
		got.FailureSignature != "model-signature" ||
		len(got.AllowedRecoveryActions) != 1 ||
		got.AllowedRecoveryActions[0] != "resume" ||
		got.ResolvedAt != "2026-07-06T01:02:03Z" ||
		got.ResolvedBy != "auto-governance" {
		t.Fatalf("projected decision recovery fields = %+v", got)
	}
	view := ProjectDecisionView(got)
	if view.Context == nil ||
		view.FailureClass != got.FailureClass ||
		len(view.AllowedRecoveryActions) != 1 ||
		view.ResolvedAt != got.ResolvedAt ||
		view.ResolvedBy != got.ResolvedBy {
		t.Fatalf("decision view lost recovery fields: %+v", view)
	}
}

func TestMapDecisionFallsBackToContextRecoveryFields(t *testing.T) {
	item := &decisionmodel.Ticket{
		ID:          "ticket-runtime-recovery-context",
		WorkspaceID: "ws-runtime-recovery-context",
		RunID:       "run-runtime-recovery-context",
		Reason:      "runtime repair exhausted",
		State:       common.DecisionTicketStateOpen,
		Context: map[string]any{
			"failureClass":           "runtime_failed",
			"failureSignature":       "class=runtime_failed|kind=llm/workspace_execution",
			"allowedRecoveryActions": []any{"replan", "terminate"},
		},
	}
	got := MapDecision(item, nil, nil)
	if got.FailureClass != "runtime_failed" ||
		got.FailureSignature != "class=runtime_failed|kind=llm/workspace_execution" ||
		len(got.AllowedRecoveryActions) != 2 ||
		got.AllowedRecoveryActions[0] != "replan" ||
		got.AllowedRecoveryActions[1] != "terminate" {
		t.Fatalf("fallback decision recovery fields = %+v", got)
	}
}

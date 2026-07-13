package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func TestMapConvergenceStatusProjectsCompactRunState(t *testing.T) {
	t.Parallel()

	got := MapConvergenceStatus(readmodelcore.RunSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-a"},
		Run:       readmodelcore.Run{ID: "run-a", WorkspaceID: "ws-a", State: "running"},
		GapBaseline: &readmodelcore.GapBaseline{
			CurrentFocus:         map[string]any{"id": "focus-a"},
			AcceptedEvidenceRefs: []string{"evidence-a", "evidence-a"},
			RejectedEvidenceRefs: []string{"rejected-a"},
			TodoItems: []readmodelcore.GapTodoItem{{
				Reason:                "missing accepted evidence",
				RequiredEvidenceKinds: []string{"validation", "state_change", "validation"},
			}},
			NextAction: readmodelcore.GapNextAction{Kind: "validate"},
		},
		Steps: []readmodelcore.StepView{
			{
				ID: "step-old-denied",
				Input: map[string]any{
					"decisionEligibilityDenied":       true,
					"decisionEligibilityDeniedReason": "older_reason",
				},
			},
			{
				ID: "step-latest-denied",
				Input: map[string]any{
					"decisionEligibilityDenied":       true,
					"decisionEligibilityDeniedReason": "continuable_next_action_available",
					"decisionNotEligibleReason":       "legacy_alias_reason",
				},
			},
		},
	})
	if got == nil {
		t.Fatal("ConvergenceStatus is nil")
	}
	if got.WorkspaceID != "ws-a" || got.RunID != "run-a" || got.RunState != "running" {
		t.Fatalf("identity = %+v, want ws-a/run-a/running", got)
	}
	if got.CurrentFocusID != "focus-a" || got.NextActionKind != "validate" || got.CurrentGap != "missing accepted evidence" {
		t.Fatalf("focus/action/gap = %+v, want compact gap status", got)
	}
	if len(got.MissingEvidenceKinds) != 2 || got.MissingEvidenceKinds[0] != "validation" || got.MissingEvidenceKinds[1] != "state_change" {
		t.Fatalf("MissingEvidenceKinds = %+v, want compact unique evidence kinds", got.MissingEvidenceKinds)
	}
	if got.AcceptedEvidenceCount != 1 || got.RejectedEvidenceCount != 1 {
		t.Fatalf("evidence counts = %d/%d, want 1/1", got.AcceptedEvidenceCount, got.RejectedEvidenceCount)
	}
	if got.DecisionCause != "" {
		t.Fatalf("DecisionCause = %q, want empty for active run", got.DecisionCause)
	}
	if !got.DecisionEligibilityDenied ||
		got.DecisionNotEligibleReason != "continuable_next_action_available" ||
		got.DecisionNotEligibleStepID != "step-latest-denied" {
		t.Fatalf("decision eligibility fields = %+v, want latest denied reason projection", got)
	}
}

func TestMapConvergenceStatusClassifiesGovernanceDecision(t *testing.T) {
	t.Parallel()

	got := MapConvergenceStatus(readmodelcore.RunSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-a"},
		Run:       readmodelcore.Run{ID: "run-a", WorkspaceID: "ws-a", State: "waiting_decision"},
	})
	if got == nil {
		t.Fatal("ConvergenceStatus is nil")
	}
	if got.DecisionCause != "governance_required" {
		t.Fatalf("ConvergenceStatus = %+v, want governance decision cause", got)
	}
}

func TestMapWorkspaceConvergenceStatusAddsHeartbeatSignals(t *testing.T) {
	t.Parallel()

	got := MapWorkspaceConvergenceStatus(readmodelcore.WorkspaceSnapshot{
		Workspace:    readmodelcore.Workspace{ID: "ws-a", ActiveRunID: "run-a"},
		Runs:         []readmodelcore.Run{{ID: "run-a", WorkspaceID: "ws-a", State: "running"}},
		RunHeartbeat: readmodelcore.RunHeartbeat{RunID: "run-a", LatestEffectiveProgressAt: "2026-06-26T01:00:00Z", Stalled: true, IneffectiveRetry: true},
	})
	if got == nil {
		t.Fatal("ConvergenceStatus is nil")
	}
	if got.LatestEffectiveProgressAt != "2026-06-26T01:00:00Z" || !got.Stalled || !got.IneffectiveRetry {
		t.Fatalf("heartbeat fields = %+v, want compact heartbeat problem signals", got)
	}
}

package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestMapConvergenceStatusProjectsCompactRunState(t *testing.T) {
	t.Parallel()

	got := mapConvergenceStatus(readmodelcore.RunSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-a"},
		Run:       appmodelcore.Run{ID: "run-a", WorkspaceID: "ws-a", State: "running"},
		GapBaseline: &projectioncontract.GapBaseline{
			CurrentFocus:         map[string]any{"id": "focus-a"},
			AcceptedEvidenceRefs: []string{"evidence-a", "evidence-a"},
			RejectedEvidenceRefs: []string{"rejected-a"},
			TodoItems: []projectioncontract.GapTodoItem{{
				Reason:                "missing accepted evidence",
				RequiredEvidenceKinds: []string{"validation", "state_change", "validation"},
			}},
		},
		Steps: []projectioncontract.StepView{
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
	if got.CurrentFocusID != "focus-a" || got.CurrentGap != "missing accepted evidence" {
		t.Fatalf("focus/gap = %+v, want compact factual gap status", got)
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

	got := mapConvergenceStatus(readmodelcore.RunSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-a"},
		Run:       appmodelcore.Run{ID: "run-a", WorkspaceID: "ws-a", State: "waiting_decision"},
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

	got := mapWorkspaceConvergenceStatus(readmodelcore.WorkspaceSnapshotState{
		Workspace:    appmodelcore.Workspace{ID: "ws-a", ActiveRunID: "run-a"},
		Runs:         []appmodelcore.Run{{ID: "run-a", WorkspaceID: "ws-a", State: "running"}},
		RunHeartbeat: projectioncontract.RunHeartbeat{RunID: "run-a", LatestEffectiveProgressAt: "2026-06-26T01:00:00Z", Stalled: true, IneffectiveRetry: true},
	})
	if got == nil {
		t.Fatal("ConvergenceStatus is nil")
	}
	if got.LatestEffectiveProgressAt != "2026-06-26T01:00:00Z" || !got.Stalled || !got.IneffectiveRetry {
		t.Fatalf("heartbeat fields = %+v, want compact heartbeat problem signals", got)
	}
}

// RVC-15: ConvergenceStatus projects coverage/slice/disposition/repair/activation fields
// from Acceptance + Runtime + AssessmentOutcome without reverse-driving authority.
func TestMapConvergenceStatusProjectsPositiveProgressFields(t *testing.T) {
	t.Parallel()

	got := mapConvergenceStatus(readmodelcore.RunSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-progress"},
		Run:       appmodelcore.Run{ID: "run-progress", WorkspaceID: "ws-progress", State: "running"},
		Acceptance: projectioncontract.AcceptanceProjection{
			ActiveSliceID: "slice-1",
			CurrentFocus: &projectioncontract.CoverageFocus{
				ID: "focus-1", SliceID: "slice-1",
				AcceptedEvidenceRefs:  []string{"ev-1", "ev-2"},
				RejectedEvidenceRefs:  []string{"ev-bad"},
				RequiredEvidenceKinds: []string{"validation"},
				FailureClass:          "repairable_gap",
				FailureSignature:      "missing validation for cwi-2",
			},
			Budget: &projectioncontract.ConvergenceBudget{
				FocusID: "focus-1", SliceID: "slice-1",
				MaxRepairAttemptsPerFocus: 10,
				RepairAttempts:            3,
				RepeatedFailureSignatures: 2,
				LastDecisionReason:        "retry validation",
			},
		},
		TaskProgress: projectioncontract.TaskProgress{
			Runtime: &projectioncontract.RuntimeProgress{
				CurrentFocusID:        "focus-1",
				ConvergenceState:      "need_validation",
				AcceptedEvidenceCount: 2,
				RejectedEvidenceCount: 1,
				MissingEvidenceKinds:  []string{"validation"},
			},
			AssessmentOutcome: projectioncontract.AssessmentOutcome{
				State:                  "repairable_gap",
				SatisfiedCoverageCount: 4,
				RemainingCoverageCount: 2,
			},
		},
	})
	if got == nil {
		t.Fatal("ConvergenceStatus is nil")
	}
	if got.CurrentSliceID != "slice-1" || got.CurrentFocusID != "focus-1" {
		t.Fatalf("slice/focus = %+v", got)
	}
	if got.ConvergenceState != "need_validation" {
		t.Fatalf("ConvergenceState = %q", got.ConvergenceState)
	}
	if got.ClosedCoverageItemCount != 4 || got.RemainingCoverageItemCount != 2 {
		t.Fatalf("coverage counts = %d/%d", got.ClosedCoverageItemCount, got.RemainingCoverageItemCount)
	}
	if got.AssessmentDisposition != "repairable_gap" {
		t.Fatalf("AssessmentDisposition = %q", got.AssessmentDisposition)
	}
	if got.RepairAttempts != 3 || got.MaxRepairAttempts != 10 || got.RepeatedFailureSignatureCount != 2 {
		t.Fatalf("repair budget = attempts=%d max=%d repeat=%d", got.RepairAttempts, got.MaxRepairAttempts, got.RepeatedFailureSignatureCount)
	}
	if got.LatestAcceptedEvidenceRef != "ev-2" {
		t.Fatalf("LatestAcceptedEvidenceRef = %q", got.LatestAcceptedEvidenceRef)
	}
	if got.GapSummary == "" {
		t.Fatalf("GapSummary empty: %+v", got)
	}
	if got.NextActivationRequired {
		t.Fatalf("NextActivationRequired unexpectedly true for active run: %+v", got)
	}
}

func TestMapConvergenceStatusMarksNextActivationWhenTerminalWithRemainingCoverage(t *testing.T) {
	t.Parallel()

	got := mapConvergenceStatus(readmodelcore.RunSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-term"},
		Run:       appmodelcore.Run{ID: "run-term", WorkspaceID: "ws-term", State: "failed"},
		TaskProgress: projectioncontract.TaskProgress{
			AssessmentOutcome: projectioncontract.AssessmentOutcome{
				State:                  "repairable_gap",
				SatisfiedCoverageCount: 1,
				RemainingCoverageCount: 3,
			},
		},
	})
	if got == nil {
		t.Fatal("ConvergenceStatus is nil")
	}
	if !got.NextActivationRequired {
		t.Fatalf("NextActivationRequired = false, want true for terminal with remaining coverage: %+v", got)
	}
	if got.ClosedCoverageItemCount != 1 || got.RemainingCoverageItemCount != 3 {
		t.Fatalf("coverage counts = %d/%d", got.ClosedCoverageItemCount, got.RemainingCoverageItemCount)
	}
}

func TestMapConvergenceStatusDoesNotReportRequiredEvidenceAsMissingDuringAssessment(t *testing.T) {
	status := mapConvergenceStatus(readmodelcore.RunSnapshotState{
		Run: appmodelcore.Run{ID: "run-a", WorkspaceID: "ws-a", State: "active"},
		Acceptance: projectioncontract.AcceptanceProjection{CurrentFocus: &projectioncontract.CoverageFocus{
			ID: "focus-1", RequiredEvidenceKinds: []string{"validation"}, AcceptedEvidenceRefs: []string{"evidence-1"}, State: "need_assessment",
		}},
		TaskProgress: projectioncontract.TaskProgress{Runtime: &projectioncontract.RuntimeProgress{CurrentFocusID: "focus-1", ConvergenceState: "need_assessment", AcceptedEvidenceCount: 1}},
	})
	if status == nil || len(status.MissingEvidenceKinds) != 0 || status.ConvergenceState != "need_assessment" {
		t.Fatalf("convergence status = %+v", status)
	}
}

package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
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
		RuntimeExecution: &readmodelcore.RuntimeExecutionState{
			ContractViolation: &runtimemodel.ContractViolation{
				ViolatedRule:         "coverage_assessment_incomplete",
				Recoverability:       "recompute_authority",
				RejectedOutputRef:    "run-a/step-a/rejected-output",
				Attempt:              2,
				MaxAttempts:          2,
				SignatureRepeatCount: 1,
				CorrectionMode:       "forced_narrow",
				BudgetScope:          map[string]any{"lineageType": "contract_violation_retry", "focusId": "focus-a"},
				CorrectionContract: map[string]any{
					"requiredOutput": map[string]any{
						"acceptedKinds": []any{"validation/managed_session"},
					},
				},
			},
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
	if got.LatestContractViolationRule != "coverage_assessment_incomplete" || got.LatestRecovery != "recompute_authority" {
		t.Fatalf("violation/recovery = %q/%q", got.LatestContractViolationRule, got.LatestRecovery)
	}
	if got.LatestRejectedDiagnosticRef != "run-a/step-a/rejected-output" ||
		got.DecisionCause != "" ||
		got.RetryAttempt != 2 ||
		got.RetryMaxAttempts != 2 ||
		got.RetrySignatureRepeatCount != 1 ||
		!got.NarrowedRetryAttempted ||
		!got.ManagedValidationAvailable ||
		got.RetryBudgetScope["focusId"] != "focus-a" {
		t.Fatalf("retry diagnostics = %+v, want retry exhaustion projection", got)
	}
	if !got.DecisionEligibilityDenied ||
		got.DecisionNotEligibleReason != "continuable_next_action_available" ||
		got.DecisionNotEligibleStepID != "step-latest-denied" {
		t.Fatalf("decision eligibility fields = %+v, want latest denied reason projection", got)
	}
}

func TestMapConvergenceStatusPrefersOwnerAuthorityNextAction(t *testing.T) {
	t.Parallel()

	got := MapConvergenceStatus(readmodelcore.RunSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-authority-status"},
		Run: readmodelcore.Run{
			ID:          "run-authority-status",
			WorkspaceID: "ws-authority-status",
			State:       "running",
			ExecutionContext: map[string]any{
				"convergenceAuthority": map[string]any{
					"currentFocus": map[string]any{"id": "focus-authority"},
					"nextAction":   map[string]any{"kind": "validate"},
				},
			},
		},
		GapBaseline: &readmodelcore.GapBaseline{
			CurrentFocus: map[string]any{"id": "focus-stale"},
			NextAction:   readmodelcore.GapNextAction{Kind: "assessment"},
		},
	})
	if got == nil {
		t.Fatal("ConvergenceStatus is nil")
	}
	if got.CurrentFocusID != "focus-authority" || got.NextActionKind != "validate" {
		t.Fatalf("ConvergenceStatus = %+v, want owner convergenceAuthority focus/nextAction", got)
	}
}

func TestMapConvergenceStatusUsesOwnerAuthorityMissingEvidenceKinds(t *testing.T) {
	t.Parallel()

	got := MapConvergenceStatus(readmodelcore.RunSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-authority-missing"},
		Run: readmodelcore.Run{
			ID:          "run-authority-missing",
			WorkspaceID: "ws-authority-missing",
			State:       "running",
			ExecutionContext: map[string]any{
				"convergenceAuthority": map[string]any{
					"currentFocus": map[string]any{
						"id":                   "cwi-036",
						"missingEvidenceKinds": []any{"validation"},
					},
					"nextAction": map[string]any{"kind": "validate"},
				},
			},
		},
		GapBaseline: &readmodelcore.GapBaseline{
			CurrentFocus: map[string]any{"id": "cwi-036"},
			NextAction:   readmodelcore.GapNextAction{Kind: "validate"},
			TodoItems: []readmodelcore.GapTodoItem{{
				RequiredEvidenceKinds: []string{"validation", "state_change"},
			}},
		},
		Acceptance: readmodelcore.AcceptanceProjection{
			CurrentFocus: &readmodelcore.CoverageFocus{
				ID:                    "cwi-036",
				RequiredEvidenceKinds: []string{"validation", "state_change"},
			},
		},
	})
	if got == nil {
		t.Fatal("ConvergenceStatus is nil")
	}
	if got.CurrentFocusID != "cwi-036" || got.NextActionKind != "validate" {
		t.Fatalf("ConvergenceStatus = %+v, want owner authority focus/action", got)
	}
	if len(got.MissingEvidenceKinds) != 1 || got.MissingEvidenceKinds[0] != "validation" {
		t.Fatalf("MissingEvidenceKinds = %+v, want owner authority validation only", got.MissingEvidenceKinds)
	}
}

func TestMapConvergenceStatusClassifiesRetryExhaustionDecision(t *testing.T) {
	t.Parallel()

	got := MapConvergenceStatus(readmodelcore.RunSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-a"},
		Run:       readmodelcore.Run{ID: "run-a", WorkspaceID: "ws-a", State: "waiting_decision"},
		RuntimeExecution: &readmodelcore.RuntimeExecutionState{
			ContractViolation: &runtimemodel.ContractViolation{
				ViolatedRule:         "state_changing_required_missing",
				Recoverability:       "decision_required",
				RejectedOutputRef:    "run-a/step-a/rejected-output",
				Attempt:              2,
				MaxAttempts:          2,
				SignatureRepeatCount: 1,
				CorrectionMode:       "forced_narrow",
				BudgetScope:          map[string]any{"lineageType": "contract_violation_retry", "focusId": "focus-a"},
				ExpectedContract: map[string]any{
					"acceptedKinds": []any{"validation/managed_session"},
				},
			},
		},
	})
	if got == nil {
		t.Fatal("ConvergenceStatus is nil")
	}
	if got.DecisionCause != "retry_exhaustion" ||
		got.LatestContractViolationRule != "state_changing_required_missing" ||
		got.LatestRecovery != "decision_required" ||
		got.LatestRejectedDiagnosticRef != "run-a/step-a/rejected-output" ||
		got.RetryAttempt != 2 ||
		got.RetryMaxAttempts != 2 ||
		got.RetrySignatureRepeatCount != 1 ||
		!got.NarrowedRetryAttempted ||
		!got.ManagedValidationAvailable ||
		got.RetryBudgetScope["lineageType"] != "contract_violation_retry" ||
		got.RetryBudgetScope["focusId"] != "focus-a" {
		t.Fatalf("ConvergenceStatus = %+v, want retry exhaustion diagnostics", got)
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

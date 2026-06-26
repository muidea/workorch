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
				ViolatedRule:   "coverage_assessment_incomplete",
				Recoverability: "recompute_authority",
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

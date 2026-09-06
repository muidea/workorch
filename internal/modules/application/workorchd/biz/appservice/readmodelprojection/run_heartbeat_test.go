package readmodelprojection

import (
	"testing"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestMapRunHeartbeatProjectsLatestProgressEvidence(t *testing.T) {
	t.Parallel()

	got := mapRunHeartbeat(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-a", ActiveRunID: "run-a"},
		Runs: []appmodelcore.Run{
			{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateActive)},
		},
		TaskProgress: projectioncontract.TaskProgress{
			WorkspaceID:      "ws-a",
			RunID:            "run-a",
			State:            string(common.RunStateActive),
			CurrentStepID:    "step-a",
			CurrentStepKind:  "llm/workspace_execution",
			CurrentStepState: "running",
			EvidenceRefs:     []string{"older-evidence", "latest-evidence"},
			RepairLoop:       projectioncontract.RepairLoop{RepeatedFailureReason: "same decision"},
		},
		EventRecords: []projectioncontract.EventRecord{
			{Topic: "old", RunID: "run-a", Revision: 1, OccurredAt: "2026-06-10T00:00:00Z"},
			{Topic: "latest", RunID: "run-a", Revision: 2, OccurredAt: "2026-06-10T00:02:00Z"},
		},
		LLMUsageTraces: []projectioncontract.LLMUsageTrace{
			{RunID: "run-a", CompletedAt: "2026-06-10T00:03:00Z"},
		},
		Results: []projectioncontract.Result{
			{RunID: "run-a", UpdatedAt: "2026-06-10T00:04:00Z"},
		},
		Artifacts: []projectioncontract.Artifact{
			{RunID: "run-a", UpdatedAt: "2026-06-10T00:05:00Z"},
		},
		Patches: []projectioncontract.Patch{
			{RunID: "run-a", AppliedAt: "2026-06-10T00:06:00Z"},
		},
	})

	if got.WorkspaceID != "ws-a" || got.RunID != "run-a" || got.State != string(common.RunStateActive) {
		t.Fatalf("identity = %+v, want ws-a/run-a running", got)
	}
	if got.LatestStepID != "step-a" || got.LatestStepKind != "llm/workspace_execution" || got.LatestStepState != "running" {
		t.Fatalf("step = %+v, want current step metadata", got)
	}
	if got.LatestEvidenceRef != "latest-evidence" || got.LatestEventTopic != "latest" || got.LatestEventRevision != 2 {
		t.Fatalf("evidence/event = %+v, want latest evidence and event", got)
	}
	if got.LatestEffectiveProgressAt != "" || got.LatestActivityAt != "2026-06-10T00:06:00Z" {
		t.Fatalf("activity without accepted owner evidence is not effective progress: %+v", got)
	}
	if got.ActivityState != "ineffective_retry" || !got.IneffectiveRetry || got.Stalled {
		t.Fatalf("activity = %+v, want ineffective retry without stalled", got)
	}
}

func TestMapRunHeartbeatMarksLiveRunWithNoActivity(t *testing.T) {
	t.Parallel()

	got := mapRunHeartbeat(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-a", ActiveRunID: "run-a"},
		Runs: []appmodelcore.Run{
			{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateActive)},
		},
	})
	if got.ActivityState != "no_activity" || !got.Stalled {
		t.Fatalf("heartbeat = %+v, want no_activity stalled live run", got)
	}
}

func TestMapRunHeartbeatMarksConvergenceBudgetEscalationIneffectiveRetry(t *testing.T) {
	t.Parallel()

	got := mapRunHeartbeat(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-a", ActiveRunID: "run-a"},
		Runs: []appmodelcore.Run{
			{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateActive)},
		},
		Acceptance: projectioncontract.AcceptanceProjection{
			Budget: &projectioncontract.ConvergenceBudget{
				State:    "escalated",
				Decision: "budget_exhausted",
			},
		},
	})
	if got.ActivityState != "ineffective_retry" || !got.IneffectiveRetry {
		t.Fatalf("heartbeat = %+v, want budget escalation ineffective retry", got)
	}
}

func TestMapRunHeartbeatMarksRuntimeNoProgressTerminalAsIneffectiveRetry(t *testing.T) {
	snapshot := readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-v2"},
		Run: &appmodelcore.Run{
			ID: "run-v2", WorkspaceID: "ws-v2", State: string(common.RunStateFailed),
		},
		TaskProgress: projectioncontract.TaskProgress{RunID: "run-v2", Runtime: &projectioncontract.RuntimeProgress{
			FailureClass: string(runtimecontract.FailureClassNoProgressExhausted), TerminalCode: "no_progress_exhausted",
		}},
	}
	got := mapRunHeartbeatAt(snapshot, time.Now().UTC())
	if !got.IneffectiveRetry || got.Stalled {
		t.Fatalf("heartbeat = %+v, want terminal ineffective retry without stalled", got)
	}
}

func TestRuntimeAuthorityExhaustionIsNotProjectedAsIneffectiveRetry(t *testing.T) {
	for _, failureClass := range []runtimecontract.FailureClass{
		runtimecontract.FailureClassCandidateAuthorityExhausted,
		// Compatibility with aggregates persisted before the class split.
		runtimecontract.FailureClassCandidateExhausted,
	} {
		if runtimeRetryExhausted(&projectioncontract.RuntimeProgress{
			FailureClass: string(failureClass), TerminalCode: "candidate_authority_exhausted",
		}) {
			t.Fatalf("zero-retry authority exhaustion %q was projected as ineffective retry", failureClass)
		}
	}
	if !runtimeRetryExhausted(&projectioncontract.RuntimeProgress{
		FailureClass: string(runtimecontract.FailureClassCandidateExhausted), TerminalCode: "candidate_retry_exhausted",
	}) {
		t.Fatal("actual candidate retry exhaustion lost its ineffective-retry projection")
	}
}

func TestMapRunHeartbeatDoesNotTreatAssessmentActionAsBudgetEscalation(t *testing.T) {
	got := mapRunHeartbeat(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-a", ActiveRunID: "run-a"},
		Runs:      []appmodelcore.Run{{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateActive)}},
		Acceptance: projectioncontract.AcceptanceProjection{Budget: &projectioncontract.ConvergenceBudget{
			Decision: string(runtimecontract.ConvergenceStateNeedAssessment),
		}},
	})
	if got.IneffectiveRetry || got.ActivityState == "ineffective_retry" {
		t.Fatalf("assessment action was treated as ineffective retry: %+v", got)
	}
}

func TestMapRunHeartbeatDoesNotInferRetryFromPriorConvergenceProjection(t *testing.T) {
	t.Parallel()

	got := mapRunHeartbeat(readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-a", ActiveRunID: "run-a"},
		Runs: []appmodelcore.Run{
			{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateBlockedOnDecision)},
		},
		ConvergenceStatus: &projectioncontract.ConvergenceStatus{
			DecisionCause: "retry_exhaustion",
		},
	})
	if got.ActivityState != "waiting_decision" || got.IneffectiveRetry {
		t.Fatalf("heartbeat = %+v, want owner waiting decision without inferred retry signal", got)
	}
}

func TestMapRunHeartbeatMarksLiveRunStalledAfterProgressThreshold(t *testing.T) {
	t.Parallel()

	got := mapRunHeartbeatAt(runHeartbeatSnapshotWithProgress("2026-06-10T00:00:00Z", string(common.RunStateActive)), mustParseTime(t, "2026-06-10T00:11:00Z"))
	if got.ActivityState != "stalled" || !got.Stalled {
		t.Fatalf("heartbeat = %+v, want stalled live run", got)
	}
	if got.Summary != "no run activity for 11m0s since 2026-06-10T00:00:00Z" {
		t.Fatalf("summary = %q, want stalled duration summary", got.Summary)
	}
}

func TestMapRunHeartbeatKeepsRecentProgressRunning(t *testing.T) {
	t.Parallel()

	got := mapRunHeartbeatAt(runHeartbeatSnapshotWithProgress("2026-06-10T00:00:00Z", string(common.RunStateActive)), mustParseTime(t, "2026-06-10T00:09:59Z"))
	if got.ActivityState != "running" || got.Stalled {
		t.Fatalf("heartbeat = %+v, want recent progress running", got)
	}
}

func TestMapRunHeartbeatIgnoresInvalidProgressTimeForStalledThreshold(t *testing.T) {
	t.Parallel()

	got := mapRunHeartbeatAt(runHeartbeatSnapshotWithProgress("not-a-time", string(common.RunStateActive)), mustParseTime(t, "2026-06-10T00:11:00Z"))
	if got.ActivityState != "running" || got.Stalled {
		t.Fatalf("heartbeat = %+v, want invalid progress time to remain running", got)
	}
}

func TestMapRunHeartbeatDoesNotMarkTerminalRunStalledByClock(t *testing.T) {
	t.Parallel()

	got := mapRunHeartbeatAt(runHeartbeatSnapshotWithProgress("2026-06-10T00:00:00Z", string(common.RunStateCompleted)), mustParseTime(t, "2026-06-10T00:30:00Z"))
	if got.ActivityState != string(common.RunStateCompleted) || got.Stalled {
		t.Fatalf("heartbeat = %+v, want terminal completed without stalled", got)
	}
}

func runHeartbeatSnapshotWithProgress(progressAt string, runState string) readmodelcore.WorkspaceSnapshotState {
	return readmodelcore.WorkspaceSnapshotState{
		Workspace: appmodelcore.Workspace{ID: "ws-a", ActiveRunID: "run-a"},
		Runs: []appmodelcore.Run{
			{ID: "run-a", WorkspaceID: "ws-a", State: runState},
		},
		EventRecords: []projectioncontract.EventRecord{
			{Topic: "progress", RunID: "run-a", Revision: 1, OccurredAt: progressAt},
		},
	}
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()
	ret, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return ret
}

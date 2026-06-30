package readmodelprojection

import (
	"testing"
	"time"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func TestMapRunHeartbeatProjectsLatestProgressEvidence(t *testing.T) {
	t.Parallel()

	got := MapRunHeartbeat(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-a", ActiveRunID: "run-a"},
		Runs: []readmodelcore.Run{
			{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateRunning)},
		},
		TaskProgress: readmodelcore.TaskProgress{
			WorkspaceID:      "ws-a",
			RunID:            "run-a",
			State:            string(common.RunStateRunning),
			CurrentStepID:    "step-a",
			CurrentStepKind:  "llm/workspace_execution",
			CurrentStepState: "running",
			EvidenceRefs:     []string{"older-evidence", "latest-evidence"},
			RepairLoop:       readmodelcore.RepairLoop{RepeatedFailureReason: "same decision"},
		},
		EventRecords: []readmodelcore.EventRecord{
			{Topic: "old", RunID: "run-a", Revision: 1, OccurredAt: "2026-06-10T00:00:00Z"},
			{Topic: "latest", RunID: "run-a", Revision: 2, OccurredAt: "2026-06-10T00:02:00Z"},
		},
		LLMUsageTraces: []readmodelcore.LLMUsageTrace{
			{RunID: "run-a", CompletedAt: "2026-06-10T00:03:00Z"},
		},
		Results: []readmodelcore.Result{
			{RunID: "run-a", UpdatedAt: "2026-06-10T00:04:00Z"},
		},
		Artifacts: []readmodelcore.Artifact{
			{RunID: "run-a", UpdatedAt: "2026-06-10T00:05:00Z"},
		},
		Patches: []readmodelcore.Patch{
			{RunID: "run-a", AppliedAt: "2026-06-10T00:06:00Z"},
		},
	})

	if got.WorkspaceID != "ws-a" || got.RunID != "run-a" || got.State != string(common.RunStateRunning) {
		t.Fatalf("identity = %+v, want ws-a/run-a running", got)
	}
	if got.LatestStepID != "step-a" || got.LatestStepKind != "llm/workspace_execution" || got.LatestStepState != "running" {
		t.Fatalf("step = %+v, want current step metadata", got)
	}
	if got.LatestEvidenceRef != "latest-evidence" || got.LatestEventTopic != "latest" || got.LatestEventRevision != 2 {
		t.Fatalf("evidence/event = %+v, want latest evidence and event", got)
	}
	if got.LatestEffectiveProgressAt != "2026-06-10T00:06:00Z" {
		t.Fatalf("latest effective progress = %q, want patch timestamp", got.LatestEffectiveProgressAt)
	}
	if got.ActivityState != "ineffective_retry" || !got.IneffectiveRetry || got.Stalled {
		t.Fatalf("activity = %+v, want ineffective retry without stalled", got)
	}
}

func TestMapRunHeartbeatMarksLiveRunWithNoActivity(t *testing.T) {
	t.Parallel()

	got := MapRunHeartbeat(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-a", ActiveRunID: "run-a"},
		Runs: []readmodelcore.Run{
			{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateRunning)},
		},
	})
	if got.ActivityState != "no_activity" || !got.Stalled {
		t.Fatalf("heartbeat = %+v, want no_activity stalled live run", got)
	}
}

func TestMapRunHeartbeatMarksConvergenceBudgetEscalationIneffectiveRetry(t *testing.T) {
	t.Parallel()

	got := MapRunHeartbeat(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-a", ActiveRunID: "run-a"},
		Runs: []readmodelcore.Run{
			{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateRunning)},
		},
		Acceptance: readmodelcore.AcceptanceProjection{
			Budget: &readmodelcore.ConvergenceBudget{
				State:    "escalated",
				Decision: "escalate_decision",
			},
		},
	})
	if got.ActivityState != "ineffective_retry" || !got.IneffectiveRetry {
		t.Fatalf("heartbeat = %+v, want budget escalation ineffective retry", got)
	}
}

func TestMapRunHeartbeatMarksWaitingDecisionRetryExhaustionIneffectiveRetry(t *testing.T) {
	t.Parallel()

	got := MapRunHeartbeat(readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-a", ActiveRunID: "run-a"},
		Runs: []readmodelcore.Run{
			{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateBlockedOnDecision)},
		},
		ConvergenceStatus: &readmodelcore.ConvergenceStatus{
			DecisionCause: "retry_exhaustion",
		},
	})
	if got.ActivityState != "waiting_decision" || !got.IneffectiveRetry {
		t.Fatalf("heartbeat = %+v, want waiting decision with ineffective retry signal", got)
	}
}

func TestMapRunHeartbeatMarksLiveRunStalledAfterProgressThreshold(t *testing.T) {
	t.Parallel()

	got := mapRunHeartbeatAt(runHeartbeatSnapshotWithProgress("2026-06-10T00:00:00Z", string(common.RunStateRunning)), mustParseTime(t, "2026-06-10T00:11:00Z"))
	if got.ActivityState != "stalled" || !got.Stalled {
		t.Fatalf("heartbeat = %+v, want stalled live run", got)
	}
	if got.Summary != "no effective progress for 11m0s since 2026-06-10T00:00:00Z" {
		t.Fatalf("summary = %q, want stalled duration summary", got.Summary)
	}
}

func TestMapRunHeartbeatKeepsRecentProgressRunning(t *testing.T) {
	t.Parallel()

	got := mapRunHeartbeatAt(runHeartbeatSnapshotWithProgress("2026-06-10T00:00:00Z", string(common.RunStateRunning)), mustParseTime(t, "2026-06-10T00:09:59Z"))
	if got.ActivityState != "running" || got.Stalled {
		t.Fatalf("heartbeat = %+v, want recent progress running", got)
	}
}

func TestMapRunHeartbeatIgnoresInvalidProgressTimeForStalledThreshold(t *testing.T) {
	t.Parallel()

	got := mapRunHeartbeatAt(runHeartbeatSnapshotWithProgress("not-a-time", string(common.RunStateRunning)), mustParseTime(t, "2026-06-10T00:11:00Z"))
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

func runHeartbeatSnapshotWithProgress(progressAt string, runState string) readmodelcore.WorkspaceSnapshot {
	return readmodelcore.WorkspaceSnapshot{
		Workspace: readmodelcore.Workspace{ID: "ws-a", ActiveRunID: "run-a"},
		Runs: []readmodelcore.Run{
			{ID: "run-a", WorkspaceID: "ws-a", State: runState},
		},
		EventRecords: []readmodelcore.EventRecord{
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

package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRepairLoopUsesExactCurrentFailureCountWithoutLatchingDisplayFlags(t *testing.T) {
	for _, count := range []int{1, 2} {
		state := runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{
			Repair:  runtimecontract.RuntimeRepairLedger{RepairAttempts: 2, SignatureCounts: []runtimecontract.RuntimeFailureSignatureCount{{Signature: "old", Count: 4}, {Signature: "current", Count: count}}},
			Failure: &runtimecontract.FailureFact{Class: runtimecontract.FailureClassExecutionFailure, Signature: "current", Message: "current failure"},
		}}
		summary := mapRuntimeProgress(state)
		progress := projectioncontract.TaskProgress{State: "active", Runtime: &summary,
			RepairLoop: projectioncontract.RepairLoop{RepeatedFailureReason: "prior projection", Escalated: true}}
		applyRuntimeProgressToRepairLoop(&progress, summary, state)
		if (progress.RepairLoop.RepeatedFailureReason != "") != (count > 1) || progress.RepairLoop.Escalated {
			t.Fatalf("count=%d loop=%+v", count, progress.RepairLoop)
		}
		if got := heartbeatIneffectiveRetry(readmodelcore.WorkspaceSnapshotState{TaskProgress: progress,
			ConvergenceStatus: &projectioncontract.ConvergenceStatus{IneffectiveRetry: true, DecisionCause: "retry_exhaustion"}}); got != (count > 1) {
			t.Fatalf("count=%d stale display flag became authority: %t", count, got)
		}
	}
}

func TestRuntimeProgressTimestampRequiresAcceptedExactOwnerReceipt(t *testing.T) {
	receipt := runtimecontract.RuntimeOperationReceipt{ExecutionKey: "effect-key", Fence: 1, OperationID: "effect", Outcome: runtimecontract.RuntimeOperationOutcomeAccepted,
		ObservedEffectScope: runtimecontract.RuntimeEffectWorkspaceDurable, Changed: true, CompletedAt: "2026-06-10T00:01:00Z"}
	state := runtimecontract.RuntimeStateRecord{Facts: runtimecontract.RuntimeFacts{WorkspaceID: "ws-a", RunID: "run-a",
		Evidence: runtimecontract.EvidenceLedger{Accepted: []runtimecontract.EvidenceFact{{ID: "evidence", DispatchID: "dispatch", OperationID: "effect", Kind: runtimecontract.EvidenceKindStateChange}}}},
		CompletedDispatches: []runtimecontract.RuntimeDispatchRecord{{ID: "dispatch", WorkspaceID: "ws-a", RunID: "run-a", OperationExecutions: []runtimecontract.RuntimeOperationExecutionRecord{{
			OperationID: "effect", ExecutionKey: "effect-key", Fence: 1, State: runtimecontract.OperationExecutionEvidenceCommitted, OwnerReceiptHash: runtimecontract.StableRuntimeOperationReceiptHash(receipt),
		}}}}, OperationReceipts: []runtimecontract.RuntimeOperationReceipt{receipt},
	}
	if got := latestRuntimeAcceptedEvidenceAt(state); got != receipt.CompletedAt {
		t.Fatalf("accepted causation-only repair did not count as progress: %q", got)
	}
	for name, mutate := range map[string]func(*runtimecontract.RuntimeStateRecord){
		"rejected": func(s *runtimecontract.RuntimeStateRecord) {
			s.Facts.Evidence.Rejected = s.Facts.Evidence.Accepted
			s.Facts.Evidence.Accepted = nil
		},
		"receipt drift": func(s *runtimecontract.RuntimeStateRecord) { s.OperationReceipts[0].AfterHash = "changed" },
		"owner committed only": func(s *runtimecontract.RuntimeStateRecord) {
			s.CompletedDispatches[0].OperationExecutions[0].State = runtimecontract.OperationExecutionOwnerCommitted
		},
		"other run": func(s *runtimecontract.RuntimeStateRecord) { s.CompletedDispatches[0].RunID = "run-old" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := state.Clone()
			mutate(&changed)
			if got := latestRuntimeAcceptedEvidenceAt(changed); got != "" {
				t.Fatalf("invalid progress timestamp: %q", got)
			}
		})
	}
}

func TestHeartbeatSeparatesActivityEffectiveProgressAndTerminalOutcome(t *testing.T) {
	snapshot := readmodelcore.WorkspaceSnapshotState{Workspace: appmodelcore.Workspace{ID: "ws-a"},
		Run:          &appmodelcore.Run{ID: "run-a", WorkspaceID: "ws-a", State: string(common.RunStateActive)},
		TaskProgress: projectioncontract.TaskProgress{RunID: "run-a", Runtime: &projectioncontract.RuntimeProgress{LatestAcceptedEvidenceAt: "2026-06-10T00:01:00Z"}},
		EventRecords: []projectioncontract.EventRecord{{RunID: "run-a", OccurredAt: "2026-06-10T00:12:00Z"}, {RunID: "", OccurredAt: "2026-06-10T00:20:00Z"}},
		Results:      []projectioncontract.Result{{RunID: "run-old", State: "accepted", CreatedAt: "2026-06-10T00:22:00Z"}, {RunID: "run-a", State: "rejected", CreatedAt: "2026-06-10T00:13:00Z"}},
	}
	heartbeat := mapRunHeartbeatAt(snapshot, mustParseTime(t, "2026-06-10T00:14:00Z"))
	if heartbeat.LatestEffectiveProgressAt != "2026-06-10T00:01:00Z" || heartbeat.LatestActivityAt != "2026-06-10T00:13:00Z" || heartbeat.Stalled || heartbeat.IneffectiveRetry {
		t.Fatalf("activity or other-run outcome promoted to effective progress: %+v", heartbeat)
	}
	for _, runID := range []string{"", "run-old"} {
		stale := snapshot
		stale.TaskProgress.RunID = runID
		stale.Acceptance = projectioncontract.AcceptanceProjection{Audit: &projectioncontract.RuntimeAudit{RunID: "run-old"}, Budget: &projectioncontract.ConvergenceBudget{Decision: "budget_exhausted"}}
		stale.ConvergenceStatus = &projectioncontract.ConvergenceStatus{IneffectiveRetry: true, DecisionCause: "retry_exhaustion"}
		projected := mapRunHeartbeatAt(stale, mustParseTime(t, "2026-06-10T00:14:00Z"))
		if projected.LatestEffectiveProgressAt != "" || projected.IneffectiveRetry {
			t.Fatalf("unscoped/old-run projection leaked progress or retry: %+v", projected)
		}
	}
	snapshot.Run.State = string(common.RunStateFailed)
	heartbeat = mapRunHeartbeatAt(snapshot, mustParseTime(t, "2026-06-10T00:30:00Z"))
	if heartbeat.ActivityState != "failed" || heartbeat.Stalled {
		t.Fatalf("earlier progress hid terminal failure: %+v", heartbeat)
	}
}

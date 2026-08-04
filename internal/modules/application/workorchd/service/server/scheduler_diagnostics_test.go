package server

import (
	"testing"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestSchedulerRecommendedActionsUseResumeTerminology(t *testing.T) {
	tests := []struct {
		name           string
		kind           string
		reason         string
		recoverability string
		want           []string
	}{
		{
			name:           "run continuation",
			kind:           schedulerKindRunContinuation,
			recoverability: schedulerRecoverabilityManualRetry,
			want:           []string{"run_resume"},
		},
		{
			name:           "workspace activation transient",
			kind:           schedulerKindWorkspaceActivation,
			recoverability: schedulerRecoverabilityManualRetry,
			want:           []string{"run_resume"},
		},
		{
			name:           "workspace activation bootstrap rejected",
			kind:           schedulerKindWorkspaceActivation,
			reason:         schedulerReasonWorkspaceActivationFailed,
			recoverability: schedulerRecoverabilityInspect,
			want:           []string{"repair_profile_or_definition", "workspace_restart"},
		},
		{
			name:           "workspace restart",
			kind:           schedulerKindWorkspaceRestart,
			recoverability: schedulerRecoverabilityManualRetry,
			want:           []string{"workspace_restart"},
		},
		{
			name:           "slot unavailable",
			reason:         schedulerReasonRunSlotUnavailable,
			recoverability: schedulerRecoverabilityManualRetry,
			want:           []string{"run_resume", "workspace_restart"},
		},
		{
			name:           "no action",
			kind:           schedulerKindRunContinuation,
			recoverability: schedulerRecoverabilityNoAction,
			want:           nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := schedulerRecommendedActions(tt.kind, tt.reason, tt.recoverability)
			if len(got) != len(tt.want) {
				t.Fatalf("actions = %v, want %v", got, tt.want)
			}
			for idx := range got {
				if got[idx] != tt.want[idx] {
					t.Fatalf("actions = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestSchedulerDiagnosticsPreservesContinuationReadback(t *testing.T) {
	store := newSchedulerDiagnosticsStore()
	store.recordContinuation(schedulerDiagnosticInput{
		WorkspaceID: "ws-readback",
		RunID:       "run-readback",
		Kind:        schedulerKindRunContinuation,
		State:       schedulerDiagnosticStateFailed,
		Reason:      schedulerReasonOwnerReadbackNotAdvanced,
		Readback: &projectioncontract.ContinuationReadbackDiagnostic{
			WorkspaceID:             "ws-readback",
			RunID:                   "run-readback",
			Action:                  "resume",
			Found:                   true,
			OldRunState:             "blocked_on_decision",
			NewRunState:             "blocked_on_decision",
			OldBlockingTicketID:     "ticket-old",
			NewBlockingTicketID:     "ticket-old",
			Reason:                  "blocked_on_resolved_decision",
			ProjectionRevision:      7,
			SourceEventRevision:     8,
			ProjectedSourceRevision: 8,
		},
	})

	diagnostics := store.snapshot("ws-readback", "run-readback")
	if diagnostics == nil || diagnostics.LatestContinuation == nil || diagnostics.LatestContinuation.Readback == nil {
		t.Fatalf("diagnostics = %+v, want latest continuation readback", diagnostics)
	}
	readback := diagnostics.LatestContinuation.Readback
	if readback.Action != "resume" ||
		readback.NewRunState != "blocked_on_decision" ||
		readback.NewBlockingTicketID != "ticket-old" ||
		readback.Reason != "blocked_on_resolved_decision" ||
		readback.ProjectionRevision != 7 {
		t.Fatalf("readback = %+v, want preserved readback state diff", readback)
	}
	diagnostics.LatestContinuation.Readback.NewRunState = "mutated"
	fresh := store.snapshot("ws-readback", "run-readback")
	if fresh.LatestContinuation.Readback.NewRunState != "blocked_on_decision" {
		t.Fatalf("stored readback mutated through clone: %+v", fresh.LatestContinuation.Readback)
	}
}

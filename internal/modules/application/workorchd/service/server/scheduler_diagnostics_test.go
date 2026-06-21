package server

import "testing"

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
			name:           "governance continuation",
			kind:           schedulerKindGovernanceContinuation,
			recoverability: schedulerRecoverabilityManualRetry,
			want:           []string{"run_resume"},
		},
		{
			name:           "workspace activation",
			kind:           schedulerKindWorkspaceActivation,
			recoverability: schedulerRecoverabilityManualRetry,
			want:           []string{"run_resume"},
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

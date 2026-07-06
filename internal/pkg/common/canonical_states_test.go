package common

import "testing"

func TestCanonicalGovernanceAndEvidenceStates(t *testing.T) {
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"decision open", CanonicalDecisionTicketState(DecisionTicketStateOpen), DecisionTicketStateOpen},
		{"decision unknown preserved", CanonicalDecisionTicketState(DecisionTicketState("custom")), DecisionTicketState("custom")},
		{"approval approved", CanonicalApprovalState(ApprovalStateApproved), ApprovalStateApproved},
		{"result rejected", CanonicalResultState(ResultStateRejected), ResultStateRejected},
		{"tool completed", CanonicalToolCallState(ToolCallStateCompleted), ToolCallStateCompleted},
		{"patch applied", CanonicalPatchState(PatchStateApplied), PatchStateApplied},
		{"validation warning", CanonicalValidationResultState(ValidationResultStateWarning), ValidationResultStateWarning},
		{"artifact continuation applying", CanonicalArtifactContinuationState(ArtifactContinuationStateApplying), ArtifactContinuationStateApplying},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("canonical state = %v, want %v", tt.got, tt.want)
			}
		})
	}
}

package server

import "testing"

func TestSchedulerDiagnosticsPreservesImmutableAdmissionFailure(t *testing.T) {
	store := newSchedulerDiagnosticsStore()
	store.recordAdmissionFailure(schedulerDiagnosticInput{
		WorkspaceID:        "ws-admission",
		Kind:               schedulerKindWorkspaceActivation,
		Outcome:            schedulerDiagnosticOutcomeAdmissionRejected,
		Reason:             schedulerReasonWorkspaceActivationFailed,
		Recoverability:     schedulerRecoverabilityInspect,
		RecommendedActions: []string{"repair_profile_or_definition", "workspace_restart"},
	})

	diagnostics := store.snapshot("ws-admission", "")
	if diagnostics == nil || diagnostics.LatestAdmissionFailure == nil {
		t.Fatalf("diagnostics = %+v, want latest admission failure", diagnostics)
	}
	latest := diagnostics.LatestAdmissionFailure
	if latest.Outcome != schedulerDiagnosticOutcomeAdmissionRejected || latest.Recoverability != schedulerRecoverabilityInspect || len(latest.RecommendedActions) != 2 {
		t.Fatalf("admission failure = %+v", latest)
	}
	diagnostics.LatestAdmissionFailure.RecommendedActions[0] = "mutated"
	fresh := store.snapshot("ws-admission", "")
	if fresh.LatestAdmissionFailure.RecommendedActions[0] != "repair_profile_or_definition" {
		t.Fatalf("stored admission observation mutated through clone: %+v", fresh.LatestAdmissionFailure)
	}
}

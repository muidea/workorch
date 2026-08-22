package workorchprojectioncontract

import "testing"

func TestCloneSchedulerDiagnosticsDetachesNestedCollections(t *testing.T) {
	input := &SchedulerDiagnostics{
		LatestAdmissionFailure: &ExecutionAdmissionFailureDiagnostic{
			RecommendedActions: []string{"resume"},
		},
		RecentAdmissionFailures: []ExecutionAdmissionFailureDiagnostic{{
			RecommendedActions: []string{"restart"},
		}},
		AdmissionFailureCounts: []ExecutionAdmissionFailureCount{{Kind: "failure", Count: 1}},
	}

	got := CloneSchedulerDiagnostics(input)
	input.LatestAdmissionFailure.RecommendedActions[0] = "mutated"
	input.RecentAdmissionFailures[0].RecommendedActions[0] = "mutated"
	input.AdmissionFailureCounts[0].Count = 2

	if got.LatestAdmissionFailure.RecommendedActions[0] != "resume" {
		t.Fatalf("latest admission failure shares mutable storage: %+v", got.LatestAdmissionFailure)
	}
	if got.RecentAdmissionFailures[0].RecommendedActions[0] != "restart" {
		t.Fatalf("recent admission failure shares mutable storage: %+v", got.RecentAdmissionFailures[0])
	}
	if got.AdmissionFailureCounts[0].Count != 1 {
		t.Fatalf("count slice shares mutable storage: %+v", got.AdmissionFailureCounts)
	}
}

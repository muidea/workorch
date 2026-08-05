package workorchprojectioncontract

import "testing"

func TestCloneSchedulerDiagnosticsDetachesNestedCollections(t *testing.T) {
	input := &SchedulerDiagnostics{
		LatestContinuation: &ContinuationDiagnostic{
			RecommendedActions: []string{"resume"},
			Readback:           &ContinuationReadbackDiagnostic{Reason: "original"},
		},
		RecentContinuations: []ContinuationDiagnostic{{
			RecommendedActions: []string{"restart"},
			Readback:           &ContinuationReadbackDiagnostic{Reason: "recent"},
		}},
		ContinuationFailureCounts: []ContinuationDiagnosticCount{{Kind: "failure", Count: 1}},
	}

	got := CloneSchedulerDiagnostics(input)
	input.LatestContinuation.RecommendedActions[0] = "mutated"
	input.LatestContinuation.Readback.Reason = "mutated"
	input.RecentContinuations[0].RecommendedActions[0] = "mutated"
	input.RecentContinuations[0].Readback.Reason = "mutated"
	input.ContinuationFailureCounts[0].Count = 2

	if got.LatestContinuation.RecommendedActions[0] != "resume" || got.LatestContinuation.Readback.Reason != "original" {
		t.Fatalf("latest continuation shares mutable storage: %+v", got.LatestContinuation)
	}
	if got.RecentContinuations[0].RecommendedActions[0] != "restart" || got.RecentContinuations[0].Readback.Reason != "recent" {
		t.Fatalf("recent continuation shares mutable storage: %+v", got.RecentContinuations[0])
	}
	if got.ContinuationFailureCounts[0].Count != 1 {
		t.Fatalf("count slice shares mutable storage: %+v", got.ContinuationFailureCounts)
	}
}

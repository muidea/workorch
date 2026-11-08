package orchestrator

import "testing"

// The strict decoder produces the diagnostic; SM-2 exhausts the original
// correction budget, then real file-backed Result read-back and all three
// owners acknowledge the same rejected terminal. No hand-built nil fixture.
func TestDriveRuntimeSemanticConstraintFailureFileDeliveryAndRecovery(t *testing.T) {
	baseline := runValidationInputReviewMainline(t, "foreign_review", "")
	for _, cut := range []string{"after_result_persistence", "after_workspace_persistence", "after_run_persistence"} {
		t.Run(cut, func(t *testing.T) {
			if got := runValidationInputReviewMainline(t, "foreign_review", cut); got != baseline {
				t.Fatalf("terminal recovery repeated provider calls: %d != %d", got, baseline)
			}
		})
	}
}

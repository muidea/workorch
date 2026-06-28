package readmodelprojection

import "testing"

func TestRuntimeResourceHealthFromStepsSummarizesValidationResources(t *testing.T) {
	steps := []StepView{{
		ID:    "run-resource-health/verify",
		Kind:  "shell/command",
		State: "completed",
		Output: map[string]any{
			"validationResources": []map[string]any{{
				"kind":   "process",
				"status": "released",
			}, {
				"kind":         "scratch_dir",
				"status":       "cleanup_failed",
				"cleanupError": "permission denied",
			}},
		},
	}}

	health := RuntimeResourceHealthFromSteps(steps)
	if health["resourceCount"] != 2 || health["stepsWithResources"] != 1 || health["released"] != false {
		t.Fatalf("health = %+v, want counted degraded resource health", health)
	}
	byKind, _ := health["byKind"].(map[string]int)
	if byKind["process"] != 1 || byKind["scratch_dir"] != 1 {
		t.Fatalf("byKind = %+v, want process and scratch_dir counts", byKind)
	}
	byStatus, _ := health["byStatus"].(map[string]int)
	if byStatus["released"] != 1 || byStatus["cleanup_failed"] != 1 {
		t.Fatalf("byStatus = %+v, want released and cleanup_failed counts", byStatus)
	}
	issue, _ := health["latestIssue"].(map[string]any)
	if issue["stepId"] != "run-resource-health/verify" ||
		issue["kind"] != "scratch_dir" ||
		issue["status"] != "cleanup_failed" {
		t.Fatalf("latestIssue = %+v, want latest cleanup failure", issue)
	}
}

func TestMapRuntimeExecutionStateIncludesResourceHealthWithoutCheckpoints(t *testing.T) {
	steps := []StepView{{
		ID:    "run-resource-health/verify",
		Kind:  "shell/command",
		State: "completed",
		Output: map[string]any{
			"validationResources": []map[string]any{{
				"kind":   "process",
				"status": "released",
			}},
		},
	}}

	state := MapRuntimeExecutionState(steps, nil, &steps[0])
	if state == nil || state.ResourceHealth == nil {
		t.Fatalf("RuntimeExecutionState = %+v, want resource health projection", state)
	}
	if state.ResourceHealth["resourceCount"] != 1 || state.ResourceHealth["released"] != true {
		t.Fatalf("ResourceHealth = %+v, want released resource summary", state.ResourceHealth)
	}
}

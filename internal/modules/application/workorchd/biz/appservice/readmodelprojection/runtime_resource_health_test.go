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

func TestRuntimeResourceHealthFromStepsUsesPlannedInputResourcesWhenOutputMissing(t *testing.T) {
	steps := []StepView{{
		ID:    "run-resource-health/verify",
		Kind:  "shell/command",
		State: "dispatched",
		Input: map[string]any{
			"validationResources": []map[string]any{{
				"kind":   "process_group",
				"status": "pending",
			}, {
				"kind":   "scratch_dir",
				"status": "pending",
			}},
		},
	}}

	health := RuntimeResourceHealthFromSteps(steps)
	if health["resourceCount"] != 2 || health["stepsWithResources"] != 1 || health["released"] != false {
		t.Fatalf("health = %+v, want planned pending input resources counted", health)
	}
	byKind, _ := health["byKind"].(map[string]int)
	if byKind["process_group"] != 1 || byKind["scratch_dir"] != 1 {
		t.Fatalf("byKind = %+v, want process_group and scratch_dir counts", byKind)
	}
	byStatus, _ := health["byStatus"].(map[string]int)
	if byStatus["pending"] != 2 {
		t.Fatalf("byStatus = %+v, want pending planned resources", byStatus)
	}
	if _, ok := health["latestIssue"]; ok {
		t.Fatalf("latestIssue = %+v, want no issue for non-terminal planned resources", health["latestIssue"])
	}
}

func TestRuntimeResourceHealthFromStepsTreatsClosedTerminalStatusesAsReleased(t *testing.T) {
	steps := []StepView{{
		ID:    "run-resource-health/verify",
		Kind:  "shell/command",
		State: "completed",
		Output: map[string]any{
			"validationResources": []map[string]any{{
				"kind":   "process_group",
				"status": "skipped",
			}, {
				"kind":   "scratch_dir",
				"status": "removed",
			}, {
				"kind":   "scratch_dir",
				"status": "cleanup_skipped",
			}, {
				"kind":   "process_group",
				"status": "failed",
			}, {
				"kind":   "process_group",
				"status": "timeout",
			}},
		},
	}}

	health := RuntimeResourceHealthFromSteps(steps)
	if health["resourceCount"] != 5 || health["stepsWithResources"] != 1 || health["released"] != true {
		t.Fatalf("health = %+v, want closed resource health", health)
	}
	byStatus, _ := health["byStatus"].(map[string]int)
	if byStatus["skipped"] != 1 || byStatus["removed"] != 1 || byStatus["cleanup_skipped"] != 1 ||
		byStatus["failed"] != 1 || byStatus["timeout"] != 1 {
		t.Fatalf("byStatus = %+v, want closed terminal status counts", byStatus)
	}
	if _, ok := health["latestIssue"]; ok {
		t.Fatalf("latestIssue = %+v, want no issue for closed terminal statuses", health["latestIssue"])
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

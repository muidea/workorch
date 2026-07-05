package readmodelprojection

import "strings"

func RuntimeResourceHealthFromSteps(steps []StepView) map[string]any {
	if len(steps) == 0 {
		return nil
	}
	total := 0
	byKind := map[string]int{}
	byStatus := map[string]int{}
	stepsWithResources := map[string]struct{}{}
	closed := 0
	var latestIssue map[string]any
	for _, step := range steps {
		resources := validationResourceRecordsForStep(step)
		if len(resources) == 0 {
			continue
		}
		stepsWithResources[step.ID] = struct{}{}
		for _, resource := range resources {
			total++
			kind := strings.TrimSpace(readModelStringValue(resource["kind"]))
			if kind == "" {
				kind = "unknown"
			}
			status := strings.TrimSpace(readModelStringValue(resource["status"]))
			if status == "" {
				status = "unknown"
			}
			byKind[kind]++
			byStatus[status]++
			if validationResourceStatusClosed(status) {
				closed++
			}
			if validationResourceStatusIssue(status, readModelStepStateTerminal(step.State)) {
				latestIssue = map[string]any{
					"stepId": step.ID,
					"kind":   kind,
					"status": status,
				}
				if cleanupError := strings.TrimSpace(readModelStringValue(resource["cleanupError"])); cleanupError != "" {
					latestIssue["cleanupError"] = cleanupError
				}
			}
		}
	}
	if total == 0 {
		return nil
	}
	health := map[string]any{
		"resourceCount":      total,
		"stepsWithResources": len(stepsWithResources),
		"byKind":             byKind,
		"byStatus":           byStatus,
		"released":           closed == total,
	}
	if byStatus["cleanup_failed"] > 0 {
		health["cleanupFailures"] = byStatus["cleanup_failed"]
	}
	if byStatus["orphaned"] > 0 {
		health["orphaned"] = byStatus["orphaned"]
	}
	if latestIssue != nil {
		health["latestIssue"] = latestIssue
	}
	return health
}

func validationResourceRecordsForStep(step StepView) []map[string]any {
	if resources := MapSliceValue(step.Output["validationResources"]); len(resources) > 0 {
		return resources
	}
	return MapSliceValue(step.Input["validationResources"])
}

func validationResourceStatusClosed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "released", "skipped", "removed", "cleanup_skipped":
		return true
	default:
		return false
	}
}

func validationResourceStatusIssue(status string, ownerTerminal bool) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		status = "unknown"
	}
	if validationResourceStatusClosed(status) {
		return false
	}
	switch status {
	case "pending", "active":
		return ownerTerminal
	default:
		return true
	}
}

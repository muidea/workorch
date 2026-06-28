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
	var latestIssue map[string]any
	for _, step := range steps {
		resources := MapSliceValue(step.Output["validationResources"])
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
			switch status {
			case "released":
			default:
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
		"released":           byStatus["released"] == total,
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

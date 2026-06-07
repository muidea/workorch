package readmodelprojection

func llmContextBudgetProviderID(budget map[string]any, usage map[string]any) string {
	return firstNonEmptyTimelineString(readModelStringValue(budget["providerId"]), readModelStringValue(usage["providerId"]))
}

func llmContextBudgetModel(budget map[string]any, usage map[string]any) string {
	return firstNonEmptyTimelineString(readModelStringValue(budget["model"]), readModelStringValue(usage["model"]))
}

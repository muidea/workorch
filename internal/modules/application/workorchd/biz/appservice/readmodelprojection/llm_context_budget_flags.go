package readmodelprojection

func llmContextBudgetTruncated(budget map[string]any) bool {
	return readModelBoolValue(budget["truncated"])
}

func llmContextBudgetGoalDoDCompressed(budget map[string]any) bool {
	return readModelBoolValue(budget["goalDoDCompressed"])
}

func llmContextBudgetOverBudget(budget map[string]any) bool {
	return readModelBoolValue(budget["overBudget"])
}

func llmContextBudgetP0Overflow(budget map[string]any) bool {
	return readModelBoolValue(budget["p0Overflow"])
}

func llmContextBudgetPromptCaptured(budget map[string]any) bool {
	return readModelBoolValue(budget["promptCaptured"])
}

package readmodelprojection

func llmContextBudgetMaxBytes(budget map[string]any) int {
	return readModelIntValue(budget["maxBytes"])
}

func llmContextBudgetActualBytes(budget map[string]any) int {
	return readModelIntValue(budget["actualBytes"])
}

func llmContextBudgetMaxTokens(budget map[string]any) int {
	return readModelIntValue(budget["maxTokens"])
}

func llmContextBudgetEstimatedInputTokens(budget map[string]any) int {
	return readModelIntValue(budget["estimatedInputTokens"])
}

func llmContextBudgetEstimatedOutputTokens(budget map[string]any) int {
	return readModelIntValue(budget["estimatedOutputTokens"])
}

func llmContextBudgetActualTokens(budget map[string]any) int {
	return readModelIntValue(budget["actualTokens"])
}

func llmContextBudgetShrinkLevel(budget map[string]any) int {
	return readModelIntValue(budget["shrinkLevel"])
}

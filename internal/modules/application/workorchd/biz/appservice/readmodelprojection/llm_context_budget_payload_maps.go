package readmodelprojection

func llmContextBudgetProfile(budget map[string]any) map[string]any {
	return readModelMapValue(budget["profile"])
}

func llmContextBudgetCachePlan(budget map[string]any) map[string]any {
	return readModelMapValue(budget["cachePlan"])
}

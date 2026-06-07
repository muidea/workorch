package readmodelprojection

func llmContextBudgetRules(budget map[string]any) []string {
	return readModelStringSliceValue(budget["rules"])
}

func llmContextBudgetSourceRefs(budget map[string]any) []string {
	return readModelStringSliceValue(budget["sourceRefs"])
}

func llmContextBudgetIncludedSections(budget map[string]any) []string {
	return readModelStringSliceValue(budget["includedSections"])
}

func llmContextBudgetNormalizedSections(budget map[string]any) []string {
	return readModelStringSliceValue(budget["normalizedSections"])
}

func llmContextBudgetCompressedSections(budget map[string]any) []string {
	return readModelStringSliceValue(budget["compressedSections"])
}

func llmContextBudgetOmittedSections(budget map[string]any) []string {
	return readModelStringSliceValue(budget["omittedSections"])
}

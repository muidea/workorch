package readmodelprojection

func llmContextBudgetProfileMode(budget map[string]any) string {
	return readModelStringValue(budget["profileMode"])
}

func llmContextBudgetCapabilitySource(budget map[string]any) string {
	return readModelStringValue(budget["capabilitySource"])
}

func llmContextBudgetContextUtilization(budget map[string]any) float64 {
	return readModelFloatValue(budget["contextUtilizationRatio"])
}

func llmContextBudgetMaxRequestBytes(budget map[string]any) int {
	return readModelIntValue(budget["maxRequestBytes"])
}

func llmContextBudgetStageBudgetRatio(budget map[string]any) float64 {
	return readModelFloatValue(budget["stageBudgetRatio"])
}

func llmContextBudgetStageBudgetSource(budget map[string]any) string {
	return readModelStringValue(budget["stageBudgetSource"])
}

func llmContextBudgetTracePolicy(budget map[string]any) string {
	return readModelStringValue(budget["tracePolicy"])
}

func llmContextBudgetEstimator(budget map[string]any) string {
	return readModelStringValue(budget["estimator"])
}

package readmodelprojection

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func MissingDoDItemsFromStepOutput(output map[string]any) []map[string]any {
	if len(output) == 0 {
		return nil
	}
	diagnostics := readModelMapValue(output["runtimeDiagnostics"])
	if items := readModelMapSliceValue(diagnostics["missingDoDItems"]); len(items) > 0 {
		return items
	}
	if failedOutput := readModelMapValue(output["failedStepOutput"]); len(failedOutput) > 0 {
		if items := MissingDoDItemsFromStepOutput(failedOutput); len(items) > 0 {
			return items
		}
	}
	return nil
}

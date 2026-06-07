package readmodelprojection

var stepOutputLLMAllowedKeys = map[string]struct{}{
	"contextBudget":     {},
	"failureClass":      {},
	"failureReason":     {},
	"llmCallRefs":       {},
	"llmUsage":          {},
	"prompt":            {},
	"repairAttempt":     {},
	"streamDiagnostics": {},
}

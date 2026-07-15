package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

func llmContextBudgetInt(budget workorcheventpayload.Payload, key string) int {
	value, _ := budget.Int64(key)
	return int(value)
}

func llmContextBudgetMaxBytes(budget workorcheventpayload.Payload) int {
	return llmContextBudgetInt(budget, "maxBytes")
}
func llmContextBudgetActualBytes(budget workorcheventpayload.Payload) int {
	return llmContextBudgetInt(budget, "actualBytes")
}
func llmContextBudgetMaxTokens(budget workorcheventpayload.Payload) int {
	return llmContextBudgetInt(budget, "maxTokens")
}
func llmContextBudgetEstimatedInputTokens(budget workorcheventpayload.Payload) int {
	return llmContextBudgetInt(budget, "estimatedInputTokens")
}
func llmContextBudgetEstimatedOutputTokens(budget workorcheventpayload.Payload) int {
	return llmContextBudgetInt(budget, "estimatedOutputTokens")
}
func llmContextBudgetActualTokens(budget workorcheventpayload.Payload) int {
	return llmContextBudgetInt(budget, "actualTokens")
}
func llmContextBudgetShrinkLevel(budget workorcheventpayload.Payload) int {
	return llmContextBudgetInt(budget, "shrinkLevel")
}

package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

func llmContextBudgetStrings(budget workorcheventpayload.Payload, key string) []string {
	value, _ := budget.StringSlice(key)
	return value
}

func llmContextBudgetRules(budget workorcheventpayload.Payload) []string {
	return llmContextBudgetStrings(budget, "rules")
}
func llmContextBudgetSourceRefs(budget workorcheventpayload.Payload) []string {
	return llmContextBudgetStrings(budget, "sourceRefs")
}
func llmContextBudgetIncludedSections(budget workorcheventpayload.Payload) []string {
	return llmContextBudgetStrings(budget, "includedSections")
}
func llmContextBudgetNormalizedSections(budget workorcheventpayload.Payload) []string {
	return llmContextBudgetStrings(budget, "normalizedSections")
}
func llmContextBudgetCompressedSections(budget workorcheventpayload.Payload) []string {
	return llmContextBudgetStrings(budget, "compressedSections")
}
func llmContextBudgetOmittedSections(budget workorcheventpayload.Payload) []string {
	return llmContextBudgetStrings(budget, "omittedSections")
}

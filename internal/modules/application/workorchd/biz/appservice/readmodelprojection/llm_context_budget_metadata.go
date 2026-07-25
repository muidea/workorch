package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

func llmContextBudgetString(budget workorcheventpayload.Payload, key string) string {
	value, _ := budget.String(key)
	return value
}

func llmContextBudgetProfileMode(budget workorcheventpayload.Payload) string {
	return llmContextBudgetString(budget, "profileMode")
}
func llmContextBudgetCapabilitySource(budget workorcheventpayload.Payload) string {
	return llmContextBudgetString(budget, "capabilitySource")
}
func llmContextBudgetContextUtilization(budget workorcheventpayload.Payload) float64 {
	value, _ := budget.Float64("contextUtilizationRatio")
	return value
}
func llmContextBudgetMaxRequestBytes(budget workorcheventpayload.Payload) int {
	return llmContextBudgetInt(budget, "maxRequestBytes")
}
func llmContextBudgetStageBudgetRatio(budget workorcheventpayload.Payload) float64 {
	value, _ := budget.Float64("stageBudgetRatio")
	return value
}
func llmContextBudgetStageBudgetSource(budget workorcheventpayload.Payload) string {
	return llmContextBudgetString(budget, "stageBudgetSource")
}
func llmContextBudgetTracePolicy(budget workorcheventpayload.Payload) string {
	return llmContextBudgetString(budget, "tracePolicy")
}
func llmContextBudgetEstimator(budget workorcheventpayload.Payload) string {
	return llmContextBudgetString(budget, "estimator")
}
func llmContextBudgetContextGateAction(budget workorcheventpayload.Payload) string {
	return llmContextBudgetString(budget, "contextGateAction")
}
func llmContextBudgetContextGateReason(budget workorcheventpayload.Payload) string {
	return llmContextBudgetString(budget, "contextGateReason")
}
func llmContextBudgetBudgetFailureClass(budget workorcheventpayload.Payload) string {
	return llmContextBudgetString(budget, "budgetFailureClass")
}

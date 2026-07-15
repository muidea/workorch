package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

func llmContextBudgetProfile(budget workorcheventpayload.Payload) workorcheventpayload.Payload {
	value, _ := budget.Object("profile")
	return value
}

func llmContextBudgetCachePlan(budget workorcheventpayload.Payload) workorcheventpayload.Payload {
	value, _ := budget.Object("cachePlan")
	return value
}

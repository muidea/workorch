package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

func llmContextBudgetTruncated(budget workorcheventpayload.Payload) bool {
	value, _ := budget.Bool("truncated")
	return value
}

func llmContextBudgetGoalDoDCompressed(budget workorcheventpayload.Payload) bool {
	value, _ := budget.Bool("goalDoDCompressed")
	return value
}

func llmContextBudgetOverBudget(budget workorcheventpayload.Payload) bool {
	value, _ := budget.Bool("overBudget")
	return value
}

func llmContextBudgetP0Overflow(budget workorcheventpayload.Payload) bool {
	value, _ := budget.Bool("p0Overflow")
	return value
}

func llmContextBudgetPromptCaptured(budget workorcheventpayload.Payload) bool {
	value, _ := budget.Bool("promptCaptured")
	return value
}

func llmContextBudgetProviderSubmitted(budget workorcheventpayload.Payload) bool {
	value, _ := budget.Bool("providerSubmitted")
	return value
}

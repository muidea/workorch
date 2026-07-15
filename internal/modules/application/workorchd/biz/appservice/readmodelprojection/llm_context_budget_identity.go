package readmodelprojection

import "github.com/muidea/workorch/internal/pkg/workorcheventpayload"

func llmContextBudgetProviderID(budget workorcheventpayload.Payload, usage workorcheventpayload.Payload) string {
	return firstNonEmptyTimelineString(
		llmContextBudgetString(budget, "providerId"),
		llmContextBudgetString(usage, "providerId"),
		llmContextBudgetString(usage, "providerID"),
	)
}

func llmContextBudgetModel(budget workorcheventpayload.Payload, usage workorcheventpayload.Payload) string {
	return firstNonEmptyTimelineString(
		llmContextBudgetString(budget, "modelId"),
		llmContextBudgetString(budget, "model"),
		llmContextBudgetString(usage, "model"),
		llmContextBudgetString(usage, "modelId"),
	)
}

func llmContextBudgetCapabilityRevision(budget workorcheventpayload.Payload) string {
	return firstNonEmptyTimelineString(
		llmContextBudgetString(budget, "capabilityRevision"),
		llmContextBudgetString(budget, "CapabilityRevision"),
	)
}

func llmContextBudgetConfigurationRevision(budget workorcheventpayload.Payload) string {
	return firstNonEmptyTimelineString(
		llmContextBudgetString(budget, "configurationRevision"),
		llmContextBudgetString(budget, "ConfigurationRevision"),
	)
}

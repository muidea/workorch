package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func SummarizeLLMUsage(items []projectioncontract.LLMUsageTrace) projectioncontract.LLMUsageSummary {
	ret := projectioncontract.LLMUsageSummary{}
	breakdownByKey := map[string]*projectioncontract.LLMUsageProviderModelBreakdown{}
	for _, item := range items {
		addLLMUsageTraceSummary(&ret, breakdownByKey, item)
	}
	ret.ProviderModelBreakdown = sortedLLMUsageProviderModelBreakdowns(breakdownByKey)
	applyLLMUsageSummaryRatios(&ret)
	return ret
}

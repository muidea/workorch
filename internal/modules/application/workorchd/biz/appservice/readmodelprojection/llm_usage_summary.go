package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func SummarizeLLMUsage(items []readmodelcore.LLMUsageTrace) readmodelcore.LLMUsageSummary {
	ret := readmodelcore.LLMUsageSummary{}
	breakdownByKey := map[string]*readmodelcore.LLMUsageProviderModelBreakdown{}
	for _, item := range items {
		addLLMUsageTraceSummary(&ret, breakdownByKey, item)
	}
	ret.ProviderModelBreakdown = sortedLLMUsageProviderModelBreakdowns(breakdownByKey)
	applyLLMUsageSummaryRatios(&ret)
	return ret
}

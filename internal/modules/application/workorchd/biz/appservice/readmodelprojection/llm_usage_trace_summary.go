package readmodelprojection

import (
	"strings"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
)

func addLLMUsageTraceSummary(summary *projectioncontract.LLMUsageSummary, breakdownByKey map[string]*projectioncontract.LLMUsageProviderModelBreakdown, item projectioncontract.LLMUsageTrace) {
	summary.EntryTotal++
	summary.InputTokens += item.InputTokens
	summary.OutputTokens += item.OutputTokens
	summary.TotalTokens += item.TotalTokens
	if item.Estimated {
		summary.EstimatedEntries++
	}
	if strings.TrimSpace(item.UsageSource) == llmcommon.UsageSourceProviderNative {
		summary.ProviderNativeEntries++
	}
	if item.Status == "failed" {
		summary.FailedEntries++
	}
	cacheUsage := llmUsageCacheMetricsFromTrace(item)
	addLLMUsageCacheMetrics(summary, cacheUsage)
	key := llmUsageProviderModelBreakdownKey(item)
	entry, ok := breakdownByKey[key]
	if !ok {
		entry = newLLMUsageProviderModelBreakdown(item)
		breakdownByKey[key] = entry
	}
	addLLMUsageProviderModelBreakdown(entry, item, cacheUsage.cacheReadTokens, cacheUsage.cacheWriteTokens)
}

package readmodelprojection

import (
	"strings"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: llm_usage_cache_flags.go

func llmUsageCacheEstimatedEntry(item projectioncontract.LLMUsageTrace) bool {
	value, _ := item.CacheUsage.Bool("estimated")
	return value
}

func llmUsageCacheProviderNativeEntry(item projectioncontract.LLMUsageTrace) bool {
	value, _ := item.CacheUsage.String("source")
	return strings.TrimSpace(value) == llmcommon.UsageSourceProviderNative
}

// source: llm_usage_trace_summary.go

func addLLMUsageTraceSummary(summary *projectioncontract.LLMUsageSummary, breakdownByKey map[string]*projectioncontract.LLMUsageProviderModelBreakdown, item projectioncontract.LLMUsageTrace) {
	if item.ProviderSubmitted {
		summary.NativeCache.PhysicalCalls++
		read := llmUsagePayloadInt(item.CacheUsage, "cacheReadTokens")
		if item.Estimated || item.UsageSource != llmcommon.UsageSourceProviderNative || !llmUsageCacheMeasurementKnown(item.CacheUsage, "cacheReadTokens") || read < 0 || read > item.InputTokens {
			summary.NativeCache.UnknownCalls++
		} else {
			summary.NativeCache.MeasuredCalls++
			summary.NativeCache.MeasuredInputTokens += item.InputTokens
			summary.NativeCache.CacheReadTokens += read
			summary.NativeCache.UncachedInputTokens += item.InputTokens - read
			if read > 0 {
				summary.NativeCache.HitCalls++
			}
		}
	}
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

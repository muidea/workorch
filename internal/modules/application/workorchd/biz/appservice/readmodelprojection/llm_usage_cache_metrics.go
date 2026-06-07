package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

type llmUsageCacheMetrics struct {
	cacheReadTokens          int
	cacheWriteTokens         int
	cacheCreationTokens      int
	cacheableInputTokens     int
	nonCacheableInputTokens  int
	cacheEstimatedEntry      bool
	cacheProviderNativeEntry bool
}

func llmUsageCacheMetricsFromTrace(item readmodelcore.LLMUsageTrace) llmUsageCacheMetrics {
	ret := llmUsageCacheMetrics{
		cacheReadTokens:         readModelIntValue(item.CacheUsage["cacheReadTokens"]),
		cacheWriteTokens:        readModelIntValue(item.CacheUsage["cacheWriteTokens"]),
		cacheCreationTokens:     readModelIntValue(item.CacheUsage["cacheCreationTokens"]),
		cacheableInputTokens:    readModelIntValue(item.CacheUsage["cacheableInputTokens"]),
		nonCacheableInputTokens: readModelIntValue(item.CacheUsage["nonCacheableInputTokens"]),
	}
	if len(item.CacheUsage) == 0 {
		return ret
	}
	ret.cacheEstimatedEntry = llmUsageCacheEstimatedEntry(item)
	ret.cacheProviderNativeEntry = llmUsageCacheProviderNativeEntry(item)
	return ret
}

func addLLMUsageCacheMetrics(summary *readmodelcore.LLMUsageSummary, metrics llmUsageCacheMetrics) {
	addLLMUsageCacheTokenMetrics(summary, metrics)
	addLLMUsageCacheEntryMetrics(summary, metrics)
}

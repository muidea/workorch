package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func addLLMUsageCacheTokenMetrics(summary *readmodelcore.LLMUsageSummary, metrics llmUsageCacheMetrics) {
	summary.CacheReadTokens += metrics.cacheReadTokens
	summary.CacheWriteTokens += metrics.cacheWriteTokens
	summary.CacheCreationTokens += metrics.cacheCreationTokens
	summary.CacheableInputTokens += metrics.cacheableInputTokens
	summary.NonCacheableInputTokens += metrics.nonCacheableInputTokens
}

func addLLMUsageCacheEntryMetrics(summary *readmodelcore.LLMUsageSummary, metrics llmUsageCacheMetrics) {
	if metrics.cacheEstimatedEntry {
		summary.CacheEstimatedEntries++
	}
	if metrics.cacheProviderNativeEntry {
		summary.CacheProviderEntries++
	}
}

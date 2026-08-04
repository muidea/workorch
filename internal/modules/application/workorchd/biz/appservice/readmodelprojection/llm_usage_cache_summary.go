package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func addLLMUsageCacheTokenMetrics(summary *projectioncontract.LLMUsageSummary, metrics llmUsageCacheMetrics) {
	summary.CacheReadTokens += metrics.cacheReadTokens
	summary.CacheWriteTokens += metrics.cacheWriteTokens
	summary.CacheCreationTokens += metrics.cacheCreationTokens
	summary.CacheableInputTokens += metrics.cacheableInputTokens
	summary.NonCacheableInputTokens += metrics.nonCacheableInputTokens
}

func addLLMUsageCacheEntryMetrics(summary *projectioncontract.LLMUsageSummary, metrics llmUsageCacheMetrics) {
	if metrics.cacheEstimatedEntry {
		summary.CacheEstimatedEntries++
	}
	if metrics.cacheProviderNativeEntry {
		summary.CacheProviderEntries++
	}
}

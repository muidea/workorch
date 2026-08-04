package readmodelprojection

import (
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

type llmUsageCacheMetrics struct {
	cacheReadTokens          int
	cacheWriteTokens         int
	cacheCreationTokens      int
	cacheableInputTokens     int
	nonCacheableInputTokens  int
	cacheEstimatedEntry      bool
	cacheProviderNativeEntry bool
}

func llmUsagePayloadInt(payload workorcheventpayload.Payload, key string) int {
	value, _ := payload.Int64(key)
	return int(value)
}

func llmUsagePayloadBool(payload workorcheventpayload.Payload, key string) bool {
	value, _ := payload.Bool(key)
	return value
}

func llmUsageCacheMetricsFromTrace(item projectioncontract.LLMUsageTrace) llmUsageCacheMetrics {
	ret := llmUsageCacheMetrics{
		cacheReadTokens:         llmUsagePayloadInt(item.CacheUsage, "cacheReadTokens"),
		cacheWriteTokens:        llmUsagePayloadInt(item.CacheUsage, "cacheWriteTokens"),
		cacheCreationTokens:     llmUsagePayloadInt(item.CacheUsage, "cacheCreationTokens"),
		cacheableInputTokens:    llmUsagePayloadInt(item.CacheUsage, "cacheableInputTokens"),
		nonCacheableInputTokens: llmUsagePayloadInt(item.CacheUsage, "nonCacheableInputTokens"),
	}
	if len(item.CacheUsage.Fields) == 0 {
		return ret
	}
	ret.cacheEstimatedEntry = llmUsageCacheEstimatedEntry(item)
	ret.cacheProviderNativeEntry = llmUsageCacheProviderNativeEntry(item)
	return ret
}

func addLLMUsageCacheMetrics(summary *projectioncontract.LLMUsageSummary, metrics llmUsageCacheMetrics) {
	addLLMUsageCacheTokenMetrics(summary, metrics)
	addLLMUsageCacheEntryMetrics(summary, metrics)
}

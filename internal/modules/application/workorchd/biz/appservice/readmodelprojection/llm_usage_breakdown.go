package readmodelprojection

import (
	"strings"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func llmUsageProviderModelBreakdownKey(item projectioncontract.LLMUsageTrace) string {
	return strings.TrimSpace(item.ProviderID) + "|" +
		strings.TrimSpace(item.Provider) + "|" +
		strings.TrimSpace(item.APISurface) + "|" +
		strings.TrimSpace(item.Model) + "|" +
		llmUsageProviderMode(item)
}

func newLLMUsageProviderModelBreakdown(item projectioncontract.LLMUsageTrace) *projectioncontract.LLMUsageProviderModelBreakdown {
	return &projectioncontract.LLMUsageProviderModelBreakdown{
		ProviderID:   strings.TrimSpace(item.ProviderID),
		Provider:     strings.TrimSpace(item.Provider),
		APISurface:   strings.TrimSpace(item.APISurface),
		Model:        strings.TrimSpace(item.Model),
		ProviderMode: llmUsageProviderMode(item),
	}
}

func addLLMUsageProviderModelBreakdown(entry *projectioncontract.LLMUsageProviderModelBreakdown, item projectioncontract.LLMUsageTrace, cacheReadTokens int, cacheWriteTokens int) {
	entry.EntryTotal++
	entry.InputTokens += item.InputTokens
	entry.OutputTokens += item.OutputTokens
	entry.TotalTokens += item.TotalTokens
	entry.CacheReadTokens += cacheReadTokens
	entry.CacheWriteTokens += cacheWriteTokens
	entry.CacheCreationTokens += llmUsagePayloadInt(item.CacheUsage, "cacheCreationTokens")
	if llmUsagePayloadInt(item.CacheMarkers, "renderedStablePrefixBytes") > 0 {
		entry.CacheablePromptCount++
	}
	if cacheReadTokens > 0 {
		entry.ActualCacheHitCount++
	}
	if cacheWriteTokens > 0 || llmUsagePayloadInt(item.CacheUsage, "cacheCreationTokens") > 0 {
		entry.CacheCreationCount++
	}
	if llmUsagePayloadBool(item.CacheMarkers, "promptCacheKeySent") {
		entry.PromptCacheKeySentCount++
	}
	if llmUsagePayloadBool(item.CacheMarkers, "cacheControlSent") {
		entry.CacheControlSentCount++
	}
	if llmUsagePayloadBool(item.CacheMarkers, "autoPrefixCacheExpected") {
		entry.AutoPrefixExpectedCount++
	}
	if item.Estimated {
		entry.EstimatedEntries++
	}
	if item.Status == "failed" {
		entry.FailedEntries++
	}
}

func llmUsageProviderMode(item projectioncontract.LLMUsageTrace) string {
	value, _ := item.CacheMarkers.String("providerMode")
	return strings.TrimSpace(value)
}

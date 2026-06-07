package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func llmUsageProviderModelBreakdownKey(item readmodelcore.LLMUsageTrace) string {
	return strings.TrimSpace(item.ProviderID) + "|" + strings.TrimSpace(item.Provider) + "|" + strings.TrimSpace(item.Model)
}

func newLLMUsageProviderModelBreakdown(item readmodelcore.LLMUsageTrace) *readmodelcore.LLMUsageProviderModelBreakdown {
	return &readmodelcore.LLMUsageProviderModelBreakdown{
		ProviderID: strings.TrimSpace(item.ProviderID),
		Provider:   strings.TrimSpace(item.Provider),
		Model:      strings.TrimSpace(item.Model),
	}
}

func addLLMUsageProviderModelBreakdown(entry *readmodelcore.LLMUsageProviderModelBreakdown, item readmodelcore.LLMUsageTrace, cacheReadTokens int, cacheWriteTokens int) {
	entry.EntryTotal++
	entry.InputTokens += item.InputTokens
	entry.OutputTokens += item.OutputTokens
	entry.TotalTokens += item.TotalTokens
	entry.CacheReadTokens += cacheReadTokens
	entry.CacheWriteTokens += cacheWriteTokens
	if item.Estimated {
		entry.EstimatedEntries++
	}
	if item.Status == "failed" {
		entry.FailedEntries++
	}
}

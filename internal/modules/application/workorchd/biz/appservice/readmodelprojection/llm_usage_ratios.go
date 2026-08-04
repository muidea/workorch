package readmodelprojection

import projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

func applyLLMUsageSummaryRatios(summary *projectioncontract.LLMUsageSummary) {
	summary.CacheReadInputRatio = readModelRatio(summary.CacheReadTokens, summary.InputTokens)
	summary.CacheableInputRatio = readModelRatio(summary.CacheableInputTokens, summary.InputTokens)
	summary.NonCacheableInputRatio = readModelRatio(summary.NonCacheableInputTokens, summary.InputTokens)
	for idx := range summary.ProviderModelBreakdown {
		summary.ProviderModelBreakdown[idx].CacheReadInputRatio = readModelRatio(summary.ProviderModelBreakdown[idx].CacheReadTokens, summary.ProviderModelBreakdown[idx].InputTokens)
	}
}

func readModelRatio(part, total int) float64 {
	if total <= 0 || part <= 0 {
		return 0
	}
	return float64(part) / float64(total)
}

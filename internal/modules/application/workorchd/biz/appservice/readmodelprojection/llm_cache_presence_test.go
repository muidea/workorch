package readmodelprojection

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/llmaudit"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestCacheSummaryPreservesUnknownAndKnownZero(t *testing.T) {
	traces := []projectioncontract.LLMUsageTrace{
		{InputTokens: 100},
		{InputTokens: 100, CacheUsage: llmaudit.CacheUsage{CacheReadTokensKnown: true, CacheWriteTokensKnown: true}.Payload()},
		{InputTokens: 100, CacheUsage: llmaudit.CacheUsage{CacheReadTokensKnown: true, CacheReadTokens: 40, CacheWriteTokensKnown: true, CacheWriteTokens: 60}.Payload()},
		// Historical positive data can be counted; historical zero lacked presence.
		{InputTokens: 100, CacheUsage: workorcheventpayload.New(workorcheventpayload.IntField("cacheReadTokens", 10), workorcheventpayload.IntField("cacheWriteTokens", 0))},
	}
	summary := projectioncontract.LLMUsageSummary{}
	breakdown := projectioncontract.LLMUsageProviderModelBreakdown{}
	for _, trace := range traces {
		metrics := llmUsageCacheMetricsFromTrace(trace)
		addLLMUsageCacheMetrics(&summary, metrics)
		addLLMUsageProviderModelBreakdown(&breakdown, trace, metrics.cacheReadTokens, metrics.cacheWriteTokens)
	}
	if summary.CacheReadKnownEntries != 3 || summary.CacheWriteKnownEntries != 2 || summary.CacheReadTokens != 50 || summary.CacheWriteTokens != 60 {
		t.Fatalf("summary=%+v", summary)
	}
	if breakdown.EntryTotal != 4 || breakdown.CacheReadKnownEntries != 3 || breakdown.CacheWriteKnownEntries != 2 || breakdown.CacheReadTokens != 50 || breakdown.CacheWriteTokens != 60 {
		t.Fatalf("breakdown=%+v", breakdown)
	}
	copy := summary
	copy.CacheReadKnownEntries++
	if llmUsageSummaryEqual(summary, copy) {
		t.Fatal("presence-only change hidden by projection equality")
	}
}

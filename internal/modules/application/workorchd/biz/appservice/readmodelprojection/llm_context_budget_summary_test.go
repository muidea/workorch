package readmodelprojection

import (
	"math"
	"testing"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func TestSummarizeLLMContextBudgetsAggregatesTraceMetadata(t *testing.T) {
	got := SummarizeLLMContextBudgets([]readmodelcore.LLMContextBudgetTrace{
		{
			StepID:               "step-a",
			EstimatedInputTokens: 1000,
			ShrinkLevel:          1,
			Truncated:            true,
			OverBudget:           true,
			ContextUtilization:   0.50,
			MaxRequestBytes:      4096,
			PromptCaptured:       true,
		},
		{
			StepID:               "step-b",
			EstimatedInputTokens: 2000,
			ActualTokens:         2500,
			ShrinkLevel:          3,
			P0Overflow:           true,
			GoalDoDCompressed:    true,
			ContextUtilization:   0.75,
			MaxRequestBytes:      8192,
		},
	})
	if got.TraceTotal != 2 ||
		got.TruncatedCount != 1 ||
		got.OverBudgetCount != 1 ||
		got.P0OverflowCount != 1 ||
		got.GoalDoDCompressedCount != 1 ||
		got.PromptCapturedCount != 1 ||
		got.MaxShrinkLevel != 3 ||
		got.MaxInputTokens != 2500 ||
		got.MaxRequestBytes != 8192 {
		t.Fatalf("summary = %+v, want aggregated context budget metrics", got)
	}
	if got.MaxContextUtilization != 0.75 || got.AvgContextUtilization != 0.625 {
		t.Fatalf("utilization = max %.3f avg %.3f, want 0.750/0.625", got.MaxContextUtilization, got.AvgContextUtilization)
	}
}

func TestSummarizeLLMContextBudgetsAddsRecentWindowTrend(t *testing.T) {
	items := make([]readmodelcore.LLMContextBudgetTrace, 0, 12)
	for idx := 0; idx < 12; idx++ {
		utilization := 0.20
		if idx >= 2 {
			utilization = 0.80
		}
		items = append(items, readmodelcore.LLMContextBudgetTrace{
			StepID:               "step",
			EstimatedInputTokens: 1000 + idx,
			Truncated:            idx >= 10,
			OverBudget:           idx == 11,
			ContextUtilization:   utilization,
		})
	}
	got := SummarizeLLMContextBudgets(items)
	if got.RecentWindowSize != llmContextBudgetRecentWindowSize ||
		got.RecentTraceTotal != llmContextBudgetRecentWindowSize ||
		got.RecentTruncatedCount != 2 ||
		got.RecentOverBudgetCount != 1 ||
		got.RecentMaxInputTokens != 1011 ||
		got.UtilizationTrend != "increasing" {
		t.Fatalf("recent summary = %+v, want recent window and increasing trend", got)
	}
	if math.Abs(got.RecentAvgUtilization-0.80) > 0.0001 || math.Abs(got.RecentMaxUtilization-0.80) > 0.0001 {
		t.Fatalf("recent utilization = max %.3f avg %.3f, want 0.800/0.800", got.RecentMaxUtilization, got.RecentAvgUtilization)
	}
}

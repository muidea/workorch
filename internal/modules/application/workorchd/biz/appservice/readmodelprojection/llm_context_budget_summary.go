package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

const llmContextBudgetRecentWindowSize = 10

func SummarizeLLMContextBudgets(items []readmodelcore.LLMContextBudgetTrace) readmodelcore.LLMContextBudgetSummary {
	ret := readmodelcore.LLMContextBudgetSummary{}
	if len(items) == 0 {
		return ret
	}
	var utilizationTotal float64
	var utilizationCount int
	for _, item := range items {
		ret.TraceTotal++
		if item.Truncated {
			ret.TruncatedCount++
		}
		if item.OverBudget {
			ret.OverBudgetCount++
		}
		if item.P0Overflow {
			ret.P0OverflowCount++
		}
		if item.GoalDoDCompressed {
			ret.GoalDoDCompressedCount++
		}
		if item.PromptCaptured {
			ret.PromptCapturedCount++
		}
		if item.ShrinkLevel > ret.MaxShrinkLevel {
			ret.MaxShrinkLevel = item.ShrinkLevel
		}
		if item.EstimatedInputTokens > ret.MaxInputTokens {
			ret.MaxInputTokens = item.EstimatedInputTokens
		}
		if item.ActualTokens > ret.MaxInputTokens {
			ret.MaxInputTokens = item.ActualTokens
		}
		if item.MaxRequestBytes > ret.MaxRequestBytes {
			ret.MaxRequestBytes = item.MaxRequestBytes
		}
		if item.ContextUtilization > 0 {
			utilizationTotal += item.ContextUtilization
			utilizationCount++
			if item.ContextUtilization > ret.MaxContextUtilization {
				ret.MaxContextUtilization = item.ContextUtilization
			}
		}
	}
	if utilizationCount > 0 {
		ret.AvgContextUtilization = utilizationTotal / float64(utilizationCount)
	}
	applyLLMContextBudgetRecentWindowSummary(&ret, items)
	return ret
}

func applyLLMContextBudgetRecentWindowSummary(summary *readmodelcore.LLMContextBudgetSummary, items []readmodelcore.LLMContextBudgetTrace) {
	if summary == nil || len(items) == 0 {
		return
	}
	windowSize := llmContextBudgetRecentWindowSize
	if len(items) < windowSize {
		windowSize = len(items)
	}
	recentStart := len(items) - windowSize
	recent := summarizeLLMContextBudgetWindow(items[recentStart:])
	summary.RecentWindowSize = windowSize
	summary.RecentTraceTotal = recent.traceTotal
	summary.RecentTruncatedCount = recent.truncatedCount
	summary.RecentOverBudgetCount = recent.overBudgetCount
	summary.RecentMaxUtilization = recent.maxUtilization
	summary.RecentAvgUtilization = recent.avgUtilization
	summary.RecentMaxInputTokens = recent.maxInputTokens
	previousStart := recentStart - windowSize
	if previousStart < 0 {
		previousStart = 0
	}
	if previousStart < recentStart {
		previous := summarizeLLMContextBudgetWindow(items[previousStart:recentStart])
		summary.UtilizationTrend = llmContextBudgetUtilizationTrend(previous.avgUtilization, recent.avgUtilization)
	}
}

type llmContextBudgetWindowSummary struct {
	traceTotal      int
	truncatedCount  int
	overBudgetCount int
	maxUtilization  float64
	avgUtilization  float64
	maxInputTokens  int
}

func summarizeLLMContextBudgetWindow(items []readmodelcore.LLMContextBudgetTrace) llmContextBudgetWindowSummary {
	ret := llmContextBudgetWindowSummary{}
	var utilizationTotal float64
	var utilizationCount int
	for _, item := range items {
		ret.traceTotal++
		if item.Truncated {
			ret.truncatedCount++
		}
		if item.OverBudget {
			ret.overBudgetCount++
		}
		if item.EstimatedInputTokens > ret.maxInputTokens {
			ret.maxInputTokens = item.EstimatedInputTokens
		}
		if item.ActualTokens > ret.maxInputTokens {
			ret.maxInputTokens = item.ActualTokens
		}
		if item.ContextUtilization > 0 {
			utilizationTotal += item.ContextUtilization
			utilizationCount++
			if item.ContextUtilization > ret.maxUtilization {
				ret.maxUtilization = item.ContextUtilization
			}
		}
	}
	if utilizationCount > 0 {
		ret.avgUtilization = utilizationTotal / float64(utilizationCount)
	}
	return ret
}

func llmContextBudgetUtilizationTrend(previous float64, recent float64) string {
	if previous <= 0 || recent <= 0 {
		return ""
	}
	delta := recent - previous
	switch {
	case delta > 0.05:
		return "increasing"
	case delta < -0.05:
		return "decreasing"
	default:
		return "stable"
	}
}

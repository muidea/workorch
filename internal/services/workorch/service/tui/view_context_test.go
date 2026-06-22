package tui

import (
	"strings"
	"testing"

	backend "github.com/muidea/workorch/internal/pkg/workorchport"
)

func TestRenderLLMContextCompact(t *testing.T) {
	section := renderLLMContextCompact(map[string]any{
		"providerId":           "reasoning",
		"model":                "test-model",
		"estimatedInputTokens": 1200,
		"maxTokens":            4096,
		"shrinkLevel":          2,
		"goalDoDCompressed":    false,
		"includedSections":     []any{"p0", "turn-01", "turn-02"},
		"normalizedSections":   []any{"turn-03"},
		"compressedSections":   []any{"p2"},
	}, backend.LLMContextBudgetSummaryInfo{
		TraceTotal:            3,
		MaxShrinkLevel:        2,
		MaxContextUtilization: 0.75,
		AvgContextUtilization: 0.50,
		RecentWindowSize:      3,
		RecentTraceTotal:      3,
		RecentAvgUtilization:  0.60,
		UtilizationTrend:      "increasing",
	}, 120)
	if section == "" {
		t.Fatal("section empty, want llm context summary")
	}
	if !strings.Contains(section, "goal/dod protected") ||
		!strings.Contains(section, "traces 3") ||
		!strings.Contains(section, "max util 75%") ||
		!strings.Contains(section, "recent 3/3 avg 60% increasing") ||
		!strings.Contains(section, "included:") ||
		!strings.Contains(section, "normalized:") {
		t.Fatalf("section = %q, want context budget details", section)
	}
}

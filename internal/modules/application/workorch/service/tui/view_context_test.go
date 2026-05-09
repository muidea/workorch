package tui

import (
	"strings"
	"testing"
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
	}, 120)
	if section == "" {
		t.Fatal("section empty, want llm context summary")
	}
	if !strings.Contains(section, "goal/dod protected") || !strings.Contains(section, "included:") || !strings.Contains(section, "normalized:") {
		t.Fatalf("section = %q, want context budget details", section)
	}
}

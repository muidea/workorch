package tui

import (
	"strings"
	"testing"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func TestLLMUsageCompactQueryAvailability(t *testing.T) {
	for _, test := range []struct {
		availability string
		want         string
	}{
		{availability: "not_loaded", want: "用量未加载"},
		{availability: "complete", want: "查询的 LLM 用量窗口内没有记录"},
		{availability: "partial", want: "用量数据不完整"},
	} {
		got := renderLLMUsageCompact(projectioncontract.LLMUsageSummary{Availability: test.availability}, 100)
		if !strings.Contains(got, test.want) {
			t.Fatalf("%s: %q", test.availability, got)
		}
	}
	if got := renderLLMUsageCompact(projectioncontract.LLMUsageSummary{Availability: "partial", EntryTotal: 2}, 100); !strings.Contains(got, "数据不完整") {
		t.Fatalf("partial: %q", got)
	}
}

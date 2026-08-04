package readmodelprojection

import (
	"sort"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func sortLLMUsageTraces(items []projectioncontract.LLMUsageTrace) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].OccurredAt == items[j].OccurredAt {
			return items[i].ID < items[j].ID
		}
		return items[i].OccurredAt < items[j].OccurredAt
	})
}

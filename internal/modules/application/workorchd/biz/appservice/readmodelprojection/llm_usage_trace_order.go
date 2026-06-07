package readmodelprojection

import (
	"sort"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func sortLLMUsageTraces(items []readmodelcore.LLMUsageTrace) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].OccurredAt == items[j].OccurredAt {
			return items[i].ID < items[j].ID
		}
		return items[i].OccurredAt < items[j].OccurredAt
	})
}

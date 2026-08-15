package readmodelprojection

import (
	"sort"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

// WD5 mechanical consolidation: same import contract; source list follows.

// source: context_section_compaction.go

func compactContextSections(items []projectioncontract.ContextSection) []projectioncontract.ContextSection {
	ret := make([]projectioncontract.ContextSection, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		item.EvidenceRefs = compactUniqueStrings(item.EvidenceRefs)
		ret = append(ret, item)
	}
	sort.SliceStable(ret, func(i, j int) bool {
		if ret[i].Priority == ret[j].Priority {
			return ret[i].ID < ret[j].ID
		}
		return ret[i].Priority > ret[j].Priority
	})
	if len(ret) == 0 {
		return nil
	}
	return ret
}

// source: llm_usage_breakdown_order.go

func sortedLLMUsageProviderModelBreakdowns(items map[string]*projectioncontract.LLMUsageProviderModelBreakdown) []projectioncontract.LLMUsageProviderModelBreakdown {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.LLMUsageProviderModelBreakdown, 0, len(items))
	for _, item := range items {
		ret = append(ret, *item)
	}
	sort.SliceStable(ret, func(i, j int) bool {
		left := ret[i]
		right := ret[j]
		if left.ProviderID != right.ProviderID {
			return left.ProviderID < right.ProviderID
		}
		if left.Provider != right.Provider {
			return left.Provider < right.Provider
		}
		if left.APISurface != right.APISurface {
			return left.APISurface < right.APISurface
		}
		if left.Model != right.Model {
			return left.Model < right.Model
		}
		if left.ProviderMode != right.ProviderMode {
			return left.ProviderMode < right.ProviderMode
		}
		return false
	})
	return ret
}

// source: llm_usage_trace_order.go

func sortLLMUsageTraces(items []projectioncontract.LLMUsageTrace) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].OccurredAt == items[j].OccurredAt {
			return items[i].ID < items[j].ID
		}
		return items[i].OccurredAt < items[j].OccurredAt
	})
}

// source: timeline_sort.go

func sortRuntimeTimelineItems(items []projectioncontract.RuntimeTimelineItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].OccurredAt == items[j].OccurredAt {
			return items[i].Source < items[j].Source
		}
		return items[i].OccurredAt < items[j].OccurredAt
	})
}

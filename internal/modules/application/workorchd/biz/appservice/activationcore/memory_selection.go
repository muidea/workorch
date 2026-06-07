package activationcore

import (
	"strings"

	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func VisibleMemoryItems(items []*memorymodel.MemoryItem, limit int) []*memorymodel.MemoryItem {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	ret := make([]*memorymodel.MemoryItem, 0, min(len(items), limit))
	for _, item := range items {
		if len(ret) >= limit {
			break
		}
		if MemoryVisibleToLLM(item) {
			ret = append(ret, item)
		}
	}
	return ret
}

func ExcludedMemoryItems(candidates []*memorymodel.MemoryItem, selected []*memorymodel.MemoryItem) []*memorymodel.MemoryItem {
	if len(candidates) == 0 {
		return nil
	}
	selectedIDs := map[common.MemoryID]struct{}{}
	for _, item := range selected {
		if item != nil && item.ID != "" {
			selectedIDs[item.ID] = struct{}{}
		}
	}
	ret := make([]*memorymodel.MemoryItem, 0, len(candidates)-len(selectedIDs))
	for _, item := range candidates {
		if item == nil || item.ID == "" {
			continue
		}
		if _, ok := selectedIDs[item.ID]; ok {
			continue
		}
		ret = append(ret, item)
	}
	return ret
}

func MemoryVisibleToLLM(item *memorymodel.MemoryItem) bool {
	if item == nil {
		return false
	}
	if strings.TrimSpace(item.MemoryType) != "tool_feedback" {
		return true
	}
	if MemoryHasTag(item, "accepted_input") {
		return true
	}
	return !MemoryHasTag(item, "provider_retry")
}

func MemoryHasTag(item *memorymodel.MemoryItem, tag string) bool {
	if item == nil {
		return false
	}
	tag = strings.TrimSpace(strings.ToLower(tag))
	if tag == "" {
		return false
	}
	for _, candidate := range item.Tags {
		if strings.TrimSpace(strings.ToLower(candidate)) == tag {
			return true
		}
	}
	return false
}

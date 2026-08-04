package readmodelprojection

import (
	memorymodel "github.com/muidea/workorch/internal/modules/blocks/memory/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func MapMemoryRecallTraces(items []*memorymodel.MemoryRecallTrace) []projectioncontract.MemoryRecallTrace {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.MemoryRecallTrace, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, mapMemoryRecallTrace(item))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

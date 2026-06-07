package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	memorymodel "github.com/muidea/workorch/internal/modules/kernel/memory/pkg/models"
)

func MapMemoryRecallTraces(items []*memorymodel.MemoryRecallTrace) []readmodelcore.MemoryRecallTrace {
	if len(items) == 0 {
		return nil
	}
	ret := make([]readmodelcore.MemoryRecallTrace, 0, len(items))
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

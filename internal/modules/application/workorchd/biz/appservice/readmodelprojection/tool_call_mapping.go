package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	runtimeevidencemodel "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/models"
)

func MapToolCalls(items []*runtimeevidencemodel.ToolCall) []projectioncontract.ToolCall {
	if len(items) == 0 {
		return nil
	}
	ret := make([]projectioncontract.ToolCall, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		projected := projectToolCallBase(item)
		projected.Arguments = cloneReadModelMap(item.Arguments.Map())
		projected.Output = cloneReadModelMap(item.Output.Map())
		ret = append(ret, projected)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

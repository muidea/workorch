package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func MapToolCalls(items []*runmodel.ToolCall) []readmodelcore.ToolCall {
	if len(items) == 0 {
		return nil
	}
	ret := make([]readmodelcore.ToolCall, 0, len(items))
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

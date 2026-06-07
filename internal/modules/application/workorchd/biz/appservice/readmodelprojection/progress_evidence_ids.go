package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
)

func IDsFromSteps(items []presenter.StepView) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func IDsFromToolCalls(items []readmodelcore.ToolCall) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

func IDsFromCheckpoints(items []readmodelcore.RuntimeCheckpoint) []string {
	ret := make([]string, 0, len(items))
	for _, item := range items {
		ret = append(ret, item.ID)
	}
	return compactUniqueStrings(ret)
}

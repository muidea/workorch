package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

type StepView = readmodelcore.StepView

func MapStep(item *runmodel.Step) *StepView {
	if item == nil {
		return nil
	}
	return &StepView{
		ID:     string(item.ID),
		Kind:   llmcommon.CanonicalRuntimeBlockKind(item.Kind),
		State:  string(item.State),
		Input:  cloneStepViewMap(item.Input),
		Output: cloneStepViewMap(item.Output),
	}
}

func MapSteps(items []*runmodel.Step) []StepView {
	if len(items) == 0 {
		return nil
	}
	out := make([]StepView, 0, len(items))
	for _, item := range items {
		mapped := MapStep(item)
		if mapped == nil {
			continue
		}
		out = append(out, *mapped)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneStepViewMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = cloneStepViewValue(v)
	}
	return out
}

func cloneStepViewSlice(input []any) []any {
	if input == nil {
		return nil
	}
	out := make([]any, len(input))
	for i, v := range input {
		out[i] = cloneStepViewValue(v)
	}
	return out
}

func cloneStepViewValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneStepViewMap(typed)
	case []any:
		return cloneStepViewSlice(typed)
	default:
		return typed
	}
}

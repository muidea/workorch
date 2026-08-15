package readmodelprojection

import (
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func mapStep(item *runmodel.Step) *projectioncontract.StepView {
	if item == nil {
		return nil
	}
	input := item.Input.Map()
	output := item.Output.Map()
	return &projectioncontract.StepView{
		ID:            string(item.ID),
		Kind:          llmcommon.CanonicalRuntimeBlockKind(item.Kind),
		State:         string(common.CanonicalStepState(item.State)),
		Input:         cloneStepViewMap(input),
		Output:        cloneStepViewMap(output),
		ContractAudit: cloneStepViewMap(projectStepContractAudit(input, output)),
	}
}

func mapSteps(items []*runmodel.Step) []projectioncontract.StepView {
	if len(items) == 0 {
		return nil
	}
	out := make([]projectioncontract.StepView, 0, len(items))
	for _, item := range items {
		mapped := mapStep(item)
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

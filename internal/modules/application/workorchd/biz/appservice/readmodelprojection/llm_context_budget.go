package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
)

func MapLLMContextBudgetTraces(steps []presenter.StepView) []readmodelcore.LLMContextBudgetTrace {
	if len(steps) == 0 {
		return nil
	}
	ret := make([]readmodelcore.LLMContextBudgetTrace, 0)
	for _, step := range steps {
		if step.Output == nil || step.Kind != llmcommon.StepKindWorkspaceExecution {
			continue
		}
		budget := readModelMapValue(step.Output["contextBudget"])
		if len(budget) == 0 {
			continue
		}
		usage := readModelMapValue(step.Output["llmUsage"])
		ret = append(ret, llmContextBudgetTraceFromStep(step, budget, usage))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

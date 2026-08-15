package readmodelprojection

import (
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"

	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorcheventpayload"
)

func mapLLMContextBudgetTraces(steps []projectioncontract.StepView) []projectioncontract.LLMContextBudgetTrace {
	if len(steps) == 0 {
		return nil
	}
	ret := make([]projectioncontract.LLMContextBudgetTrace, 0)
	for _, step := range steps {
		if step.Output == nil || step.Kind != llmcommon.StepKindWorkspaceExecution {
			continue
		}
		output, err := workorcheventpayload.FromMap(step.Output)
		if err != nil {
			continue
		}
		budget, ok := output.Object("contextBudget")
		if !ok || len(budget.Fields) == 0 {
			continue
		}
		usage, _ := output.Object("llmUsage")
		ret = append(ret, llmContextBudgetTraceFromStep(step, budget, usage))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

package readmodelprojection

import (
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func ProjectStepView(item projectioncontract.StepView) projectioncontract.StepView {
	kind := llmcommon.CanonicalRuntimeBlockKind(item.Kind)
	return projectioncontract.StepView{
		ID:            item.ID,
		Kind:          kind,
		State:         item.State,
		Output:        ProjectStepOutput(kind, item.Output),
		ContractAudit: cloneStepViewMap(projectStepContractAudit(item.Input, item.Output, item.ContractAudit)),
	}
}

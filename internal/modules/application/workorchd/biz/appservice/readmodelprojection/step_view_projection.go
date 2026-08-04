package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
)

func ProjectStepView(item readmodelcore.StepView) readmodelcore.StepView {
	kind := llmcommon.CanonicalRuntimeBlockKind(item.Kind)
	return readmodelcore.StepView{
		ID:            item.ID,
		Kind:          kind,
		State:         item.State,
		Output:        ProjectStepOutput(kind, item.Output),
		ContractAudit: cloneStepViewMap(projectStepContractAudit(item.Input, item.Output, item.ContractAudit)),
	}
}

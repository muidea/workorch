package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"
)

func ProjectStepView(item presenter.StepView) presenter.StepView {
	kind := llmcommon.CanonicalRuntimeBlockKind(item.Kind)
	return presenter.StepView{
		ID:     item.ID,
		Kind:   kind,
		State:  item.State,
		Output: ProjectStepOutput(kind, item.Output),
	}
}

package readmodelprojection

import llmcommon "github.com/muidea/workorch/internal/modules/blocks/llm/pkg/common"

func ProjectStepView(item StepView) StepView {
	kind := llmcommon.CanonicalRuntimeBlockKind(item.Kind)
	return StepView{
		ID:     item.ID,
		Kind:   kind,
		State:  item.State,
		Output: ProjectStepOutput(kind, item.Output),
	}
}

package readmodelprojection

import (
	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func MapGitOperationTraces(steps []workorchprojectioncontract.StepView) []workorchprojectioncontract.GitOperationTrace {
	if len(steps) == 0 {
		return nil
	}
	ret := make([]workorchprojectioncontract.GitOperationTrace, 0)
	for _, step := range steps {
		if step.Kind != gitcommon.StepKindRead && step.Kind != gitcommon.StepKindWrite {
			continue
		}
		if len(step.Output) == 0 {
			continue
		}
		ret = append(ret, projectGitOperationTrace(step))
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

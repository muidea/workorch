package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	gitcommon "github.com/muidea/workorch/internal/modules/blocks/git/pkg/common"
)

func MapGitOperationTraces(steps []presenter.StepView) []readmodelcore.GitOperationTrace {
	if len(steps) == 0 {
		return nil
	}
	ret := make([]readmodelcore.GitOperationTrace, 0)
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

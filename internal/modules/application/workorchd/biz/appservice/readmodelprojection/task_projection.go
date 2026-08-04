package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func MapTaskDecompositions(snapshot readmodelcore.RunSnapshot) []projectioncontract.TaskDecomposition {
	if len(snapshot.Steps) == 0 {
		return nil
	}
	stepByNodeRoot := taskExecutionStepsByRoot(snapshot.Steps)
	ret := make([]projectioncontract.TaskDecomposition, 0)
	for _, step := range snapshot.Steps {
		item, ok := taskDecompositionFromStep(snapshot, step, stepByNodeRoot[step.ID])
		if !ok {
			continue
		}
		ret = append(ret, item)
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}

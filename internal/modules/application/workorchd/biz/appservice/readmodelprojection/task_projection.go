package readmodelprojection

import "github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"

func MapTaskDecompositions(snapshot readmodelcore.RunSnapshot) []readmodelcore.TaskDecomposition {
	if len(snapshot.Steps) == 0 {
		return nil
	}
	stepByNodeRoot := taskExecutionStepsByRoot(snapshot.Steps)
	ret := make([]readmodelcore.TaskDecomposition, 0)
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

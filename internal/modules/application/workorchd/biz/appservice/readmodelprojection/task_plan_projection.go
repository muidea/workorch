package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
)

func MapTaskPlans(snapshot readmodelcore.RunSnapshot, decompositions []readmodelcore.TaskDecomposition) []readmodelcore.TaskPlan {
	if len(decompositions) == 0 {
		return nil
	}
	stepByID := map[string]presenter.StepView{}
	for _, step := range snapshot.Steps {
		stepByID[step.ID] = step
	}
	occurrences := TimelineStepOccurrences(snapshot)
	grouped := map[string][]readmodelcore.TaskPlan{}
	for _, decomposition := range decompositions {
		if strings.TrimSpace(decomposition.RootStepID) == "" {
			continue
		}
		step, hasStep := stepByID[decomposition.RootStepID]
		plan := projectTaskPlan(snapshot, decomposition, step, hasStep, occurrences)
		grouped[plan.AnchorStepID] = append(grouped[plan.AnchorStepID], plan)
	}
	if len(grouped) == 0 {
		return nil
	}
	ret := taskPlanRevisions(grouped)
	if len(ret) == 0 {
		return nil
	}
	return ret
}

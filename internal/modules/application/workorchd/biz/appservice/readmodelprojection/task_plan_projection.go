package readmodelprojection

import (
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func MapTaskPlans(snapshot readmodelcore.RunSnapshot, decompositions []readmodelcore.TaskDecomposition) []readmodelcore.TaskPlan {
	if len(decompositions) == 0 {
		return nil
	}
	stepByID := map[string]readmodelcore.StepView{}
	for _, step := range snapshot.Steps {
		stepByID[step.ID] = step
	}
	occurrences := TimelineStepOccurrences(snapshot)
	grouped := map[string][]readmodelcore.TaskPlan{}
	knownAnchors := map[string]bool{}
	for _, decomposition := range decompositions {
		if strings.TrimSpace(decomposition.RootStepID) == "" {
			continue
		}
		step, hasStep := stepByID[decomposition.RootStepID]
		plan := projectTaskPlan(snapshot, decomposition, step, hasStep, occurrences)
		grouped[plan.AnchorStepID] = append(grouped[plan.AnchorStepID], plan)
		knownAnchors[plan.AnchorStepID] = true
		knownAnchors[decomposition.RootStepID] = true
	}
	for _, step := range snapshot.Steps {
		plan, ok := taskRetryPlanFromStep(snapshot, step, occurrences, knownAnchors)
		if !ok {
			continue
		}
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

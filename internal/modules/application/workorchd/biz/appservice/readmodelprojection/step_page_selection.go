package readmodelprojection

import (
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func selectRunStepPageItems(items []*runmodel.Step, limit int, cursorAfterID string) ([]projectioncontract.StepView, bool) {
	started := cursorAfterID == ""
	selected := make([]projectioncontract.StepView, 0, min(limit, len(items)))
	for _, item := range items {
		if item == nil {
			continue
		}
		if !started {
			if string(item.ID) == cursorAfterID {
				started = true
			}
			continue
		}
		if len(selected) >= limit {
			return selected, true
		}
		mapped := MapStep(item)
		if mapped == nil {
			continue
		}
		selected = append(selected, ProjectStepView(*mapped))
	}
	return selected, false
}

package readmodelprojection

import runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"

func selectRunStepPageItems(items []*runmodel.Step, limit int, cursorAfterID string) ([]StepView, bool) {
	started := cursorAfterID == ""
	selected := make([]StepView, 0, min(limit, len(items)))
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

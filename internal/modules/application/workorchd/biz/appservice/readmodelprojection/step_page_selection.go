package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/pkg/presenter"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
)

func selectRunStepPageItems(items []*runmodel.Step, limit int, cursorAfterID string) ([]presenter.StepView, bool) {
	started := cursorAfterID == ""
	selected := make([]presenter.StepView, 0, min(limit, len(items)))
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
		mapped := presenter.MapStep(item)
		if mapped == nil {
			continue
		}
		selected = append(selected, ProjectStepView(*mapped))
	}
	return selected, false
}

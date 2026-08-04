package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func projectRunSnapshotStepDetails(ret *readmodelcore.RunSnapshot) {
	if ret == nil {
		return
	}

	if ret.Step != nil {
		projected := ProjectStepView(*ret.Step)
		ret.Step = &projected
	}
	if len(ret.Steps) > 0 {
		limit := min(len(ret.Steps), stepPageLimit)
		projected := make([]readmodelcore.StepView, 0, limit)
		for _, item := range ret.Steps[:limit] {
			projected = append(projected, ProjectStepView(item))
		}
		ret.Steps = projected
	}
}

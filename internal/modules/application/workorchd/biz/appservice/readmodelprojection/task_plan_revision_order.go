package readmodelprojection

import (
	"sort"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func sortTaskPlanRevisions(plans []readmodelcore.TaskPlan) {
	sort.SliceStable(plans, func(i, j int) bool {
		leftAt := strings.TrimSpace(plans[i].OccurredAt)
		rightAt := strings.TrimSpace(plans[j].OccurredAt)
		if leftAt == rightAt {
			return plans[i].RevisionStepID < plans[j].RevisionStepID
		}
		if leftAt == "" {
			return false
		}
		if rightAt == "" {
			return true
		}
		return leftAt < rightAt
	})
}

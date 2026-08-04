package readmodelprojection

import (
	"sort"
	"strings"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func sortTaskPlanRevisions(plans []projectioncontract.TaskPlan) {
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

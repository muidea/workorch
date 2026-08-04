package readmodelprojection

import (
	"fmt"
	"sort"

	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func taskPlanRevisions(grouped map[string][]projectioncontract.TaskPlan) []projectioncontract.TaskPlan {
	ret := make([]projectioncontract.TaskPlan, 0)
	anchors := make([]string, 0, len(grouped))
	for anchor := range grouped {
		anchors = append(anchors, anchor)
	}
	sort.Strings(anchors)
	for _, anchor := range anchors {
		plans := grouped[anchor]
		sortTaskPlanRevisions(plans)
		for idx := range plans {
			plans[idx].Revision = idx + 1
			plans[idx].ID = fmt.Sprintf("%s/plan/%d", plans[idx].AnchorStepID, plans[idx].Revision)
			if plans[idx].Revision > 1 && plans[idx].State == "accepted" {
				plans[idx].State = "revised"
			}
			ret = append(ret, plans[idx])
		}
	}
	return ret
}

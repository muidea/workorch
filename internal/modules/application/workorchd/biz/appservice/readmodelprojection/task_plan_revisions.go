package readmodelprojection

import (
	"fmt"
	"sort"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
)

func taskPlanRevisions(grouped map[string][]readmodelcore.TaskPlan) []readmodelcore.TaskPlan {
	ret := make([]readmodelcore.TaskPlan, 0)
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

package readmodelprojection

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func CurrentProgressMilestone(items []projectioncontract.ProgressMilestone) string {
	for _, item := range items {
		if item.State != "completed" {
			return item.ID
		}
	}
	if len(items) == 0 {
		return ""
	}
	return items[len(items)-1].ID
}

func BlockerReason(items []readmodelcore.ProgressBlocker, kind string, id string) string {
	for _, item := range items {
		if item.Kind == kind && item.ID == id {
			return item.Reason
		}
	}
	return ""
}

package definitioncore

import (
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func MapGoal(item *workspacemodel.Goal) appmodelcore.Goal {
	if item == nil {
		return appmodelcore.Goal{}
	}
	return appmodelcore.Goal{
		ID:          string(item.ID),
		WorkspaceID: string(item.WorkspaceID),
		Text:        item.Text,
	}
}

func MapGoals(items []*workspacemodel.Goal) []appmodelcore.Goal {
	ret := make([]appmodelcore.Goal, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, MapGoal(item))
	}
	return ret
}

func MapDoD(item *workspacemodel.DoD) appmodelcore.DoD {
	if item == nil {
		return appmodelcore.DoD{}
	}
	return appmodelcore.DoD{
		ID:          string(item.ID),
		WorkspaceID: string(item.WorkspaceID),
		Items:       append([]string(nil), item.Items...),
		State:       string(item.State),
	}
}

func MapDoDs(items []*workspacemodel.DoD) []appmodelcore.DoD {
	ret := make([]appmodelcore.DoD, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, MapDoD(item))
	}
	return ret
}

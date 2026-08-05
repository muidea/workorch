package workorchport

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func GoalInfoFromManagement(item workorchmanagementcontract.Goal) GoalInfo {
	return GoalInfo{ID: common.GoalID(item.ID), WorkspaceID: common.WorkspaceID(item.WorkspaceID), Text: item.Text}
}

func GoalInfosFromManagement(items []workorchmanagementcontract.Goal) []GoalInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]GoalInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, GoalInfoFromManagement(item))
	}
	return ret
}

func DoDInfoFromManagement(item workorchmanagementcontract.DoD) DoDInfo {
	return DoDInfo{
		ID: common.DoDID(item.ID), WorkspaceID: common.WorkspaceID(item.WorkspaceID),
		Items: cloneProjectionStrings(item.Items), State: item.State,
	}
}

func DoDInfosFromManagement(items []workorchmanagementcontract.DoD) []DoDInfo {
	if len(items) == 0 {
		return nil
	}
	ret := make([]DoDInfo, 0, len(items))
	for _, item := range items {
		ret = append(ret, DoDInfoFromManagement(item))
	}
	return ret
}

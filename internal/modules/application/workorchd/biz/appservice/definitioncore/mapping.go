package definitioncore

import (
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func MapGoal(item *workspacemodel.Goal) workorchmanagementcontract.Goal {
	if item == nil {
		return workorchmanagementcontract.Goal{}
	}
	return workorchmanagementcontract.Goal{
		ID:                     string(item.ID),
		WorkspaceID:            string(item.WorkspaceID),
		Text:                   item.Text,
		DefinitionRevision:     item.DefinitionRevision,
		PreviousDefinitionHash: item.PreviousDefinitionHash,
		PreviousGoalID:         string(item.PreviousGoalID),
		LineageState:           item.LineageState,
	}
}

func MapGoals(items []*workspacemodel.Goal) []workorchmanagementcontract.Goal {
	ret := make([]workorchmanagementcontract.Goal, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, MapGoal(item))
	}
	return ret
}

func MapDoD(item *workspacemodel.DoD) workorchmanagementcontract.DoD {
	if item == nil {
		return workorchmanagementcontract.DoD{}
	}
	return workorchmanagementcontract.DoD{
		ID:                     string(item.ID),
		WorkspaceID:            string(item.WorkspaceID),
		Items:                  append([]string(nil), item.Items...),
		State:                  string(item.State),
		DefinitionRevision:     item.DefinitionRevision,
		PreviousDefinitionHash: item.PreviousDefinitionHash,
		PreviousDoDID:          string(item.PreviousDoDID),
		LineageState:           item.LineageState,
	}
}

func MapDoDs(items []*workspacemodel.DoD) []workorchmanagementcontract.DoD {
	ret := make([]workorchmanagementcontract.DoD, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		ret = append(ret, MapDoD(item))
	}
	return ret
}

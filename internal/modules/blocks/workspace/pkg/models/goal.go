package workspace

import "github.com/muidea/workorch/internal/pkg/common"

type Goal struct {
	ID                     common.GoalID
	WorkspaceID            common.WorkspaceID
	Text                   string
	DefinitionRevision     string
	PreviousDefinitionHash string
	PreviousGoalID         common.GoalID
	LineageState           string
}

func CloneGoal(item *Goal) *Goal {
	if item == nil {
		return nil
	}
	ret := *item
	return &ret
}

func CloneGoals(items []*Goal) []*Goal {
	if items == nil {
		return nil
	}
	ret := make([]*Goal, 0, len(items))
	for _, item := range items {
		ret = append(ret, CloneGoal(item))
	}
	return ret
}

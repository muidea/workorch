package definitioncore

import (
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type GoalConsolidationPlan struct {
	CanonicalID common.GoalID
	Text        string
	ObsoleteIDs []common.GoalID
}

type DoDConsolidationPlan struct {
	CanonicalID common.DoDID
	Items       []string
	ObsoleteIDs []common.DoDID
}

func GoalIDsExcept(items []*workspacemodel.Goal, workspaceID common.WorkspaceID, retainedID common.GoalID) []common.GoalID {
	ret := make([]common.GoalID, 0, len(items))
	for _, item := range items {
		if item == nil || item.ID == retainedID || item.WorkspaceID != workspaceID {
			continue
		}
		ret = append(ret, item.ID)
	}
	return ret
}

func DoDIDsExcept(items []*workspacemodel.DoD, workspaceID common.WorkspaceID, retainedID common.DoDID) []common.DoDID {
	ret := make([]common.DoDID, 0, len(items))
	for _, item := range items {
		if item == nil || item.ID == retainedID || item.WorkspaceID != workspaceID {
			continue
		}
		ret = append(ret, item.ID)
	}
	return ret
}

func WorkspaceGoalConsolidationPlan(existing []*workspacemodel.Goal, workspaceID common.WorkspaceID, appended string) (GoalConsolidationPlan, error) {
	canonical := LatestWorkspaceGoal(existing)
	if canonical == nil {
		return GoalConsolidationPlan{}, common.ErrInvalidArgument
	}
	merged := MergedGoalText(existing, appended)
	if merged == "" {
		return GoalConsolidationPlan{}, common.ErrInvalidArgument
	}
	return GoalConsolidationPlan{
		CanonicalID: canonical.ID,
		Text:        merged,
		ObsoleteIDs: GoalIDsExcept(existing, workspaceID, canonical.ID),
	}, nil
}

func WorkspaceDoDConsolidationPlan(existing []*workspacemodel.DoD, workspaceID common.WorkspaceID, appended []string) (DoDConsolidationPlan, error) {
	canonical := LatestWorkspaceDoD(existing)
	if canonical == nil {
		return DoDConsolidationPlan{}, common.ErrInvalidArgument
	}
	merged := MergedDoDItems(existing, appended)
	if len(merged) == 0 {
		return DoDConsolidationPlan{}, common.ErrInvalidArgument
	}
	return DoDConsolidationPlan{
		CanonicalID: canonical.ID,
		Items:       merged,
		ObsoleteIDs: DoDIDsExcept(existing, workspaceID, canonical.ID),
	}, nil
}

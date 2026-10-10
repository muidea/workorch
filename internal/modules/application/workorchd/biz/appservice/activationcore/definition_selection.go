package activationcore

import (
	"context"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func SelectWorkspaceExecutionGoal(
	ctx context.Context,
	listByWorkspaceID func(context.Context, common.WorkspaceID) ([]*workspacemodel.Goal, error),
	workspaceID common.WorkspaceID,
	requestedID common.GoalID,
) (*workspacemodel.Goal, error) {
	items, err := listByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	items, err = OrderedWorkspaceGoals(items)
	if err != nil {
		return nil, err
	}
	latest := LatestWorkspaceGoal(items)
	if requestedID != "" {
		for _, item := range items {
			if item != nil && item.ID == requestedID {
				if latest != nil {
					return latest, nil
				}
				return item, nil
			}
		}
		return nil, common.ErrConflict
	}
	if latest == nil {
		return nil, common.ErrInvalidArgument
	}
	return latest, nil
}

func SelectWorkspaceExecutionDoD(
	ctx context.Context,
	listByWorkspaceID func(context.Context, common.WorkspaceID) ([]*workspacemodel.DoD, error),
	workspaceID common.WorkspaceID,
	requestedID common.DoDID,
) (*workspacemodel.DoD, error) {
	items, err := listByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	items, err = OrderedWorkspaceDoDs(items)
	if err != nil {
		return nil, err
	}
	readyItems := make([]*workspacemodel.DoD, 0, len(items))
	for _, item := range items {
		if item != nil && item.State == common.DoDStateReady {
			readyItems = append(readyItems, item)
		}
	}
	latest := LatestWorkspaceDoD(readyItems)
	if requestedID != "" {
		for _, item := range readyItems {
			if item.ID == requestedID {
				if latest != nil {
					return latest, nil
				}
				return item, nil
			}
		}
		return nil, common.ErrConflict
	}
	if latest == nil {
		return nil, common.ErrInvalidArgument
	}
	return latest, nil
}

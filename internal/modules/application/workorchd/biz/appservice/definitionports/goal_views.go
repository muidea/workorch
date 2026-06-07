package definitionports

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/appmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitioncore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelports"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func DefineOrConsolidateWorkspaceGoalView(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, text string) (appmodelcore.Goal, error) {
	eventRecords := readmodelports.EventRecordReader{Hub: hub}
	item, err := DefineOrConsolidateWorkspaceGoal(ctx, hub, eventRecords, workspaceID, text)
	if err != nil {
		return appmodelcore.Goal{}, err
	}
	return definitioncore.MapGoal(item), nil
}

func UpdateGoalView(ctx context.Context, hub events.Hub, goalID common.GoalID, text string) (appmodelcore.Goal, error) {
	text = strings.TrimSpace(text)
	if goalID == "" || text == "" {
		return appmodelcore.Goal{}, common.ErrInvalidArgument
	}
	text = definitioncore.FormatGoalDefinitionText(text)
	if text == "" {
		return appmodelcore.Goal{}, common.ErrInvalidArgument
	}
	item, err := UpdateGoal(ctx, hub, "", goalID, text)
	if err != nil {
		return appmodelcore.Goal{}, err
	}
	return definitioncore.MapGoal(item), nil
}

func ReadGoalView(ctx context.Context, hub events.Hub, goalID common.GoalID) (appmodelcore.Goal, error) {
	item, err := ReadGoal(ctx, hub, goalID)
	if err != nil {
		return appmodelcore.Goal{}, err
	}
	return definitioncore.MapGoal(item), nil
}

func ListConsolidatedWorkspaceGoalViews(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]appmodelcore.Goal, error) {
	eventRecords := readmodelports.EventRecordReader{Hub: hub}
	items, err := ListConsolidatedWorkspaceGoals(ctx, hub, eventRecords, workspaceID)
	if err != nil {
		return nil, err
	}
	return definitioncore.MapGoals(items), nil
}

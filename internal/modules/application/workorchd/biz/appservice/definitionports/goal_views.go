package definitionports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitioncore"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func DefineOrConsolidateWorkspaceGoalView(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, text string) (workorchmanagementcontract.Goal, error) {
	item, err := DefineOrConsolidateWorkspaceGoal(ctx, hub, workspaceID, text)
	if err != nil {
		return workorchmanagementcontract.Goal{}, err
	}
	return definitioncore.MapGoal(item), nil
}

func UpdateGoalView(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, goalID common.GoalID, text string) (workorchmanagementcontract.Goal, error) {
	text = strings.TrimSpace(text)
	if workspaceID == "" || goalID == "" || text == "" {
		return workorchmanagementcontract.Goal{}, common.ErrInvalidArgument
	}
	text = definitioncore.FormatGoalDefinitionText(text)
	if text == "" {
		return workorchmanagementcontract.Goal{}, common.ErrInvalidArgument
	}
	item, err := UpdateGoal(ctx, hub, workspaceID, goalID, text)
	if err != nil {
		return workorchmanagementcontract.Goal{}, err
	}
	return definitioncore.MapGoal(item), nil
}

func ReadGoalView(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, goalID common.GoalID) (workorchmanagementcontract.Goal, error) {
	var err error
	ctx, err = common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return workorchmanagementcontract.Goal{}, err
	}
	item, err := ReadGoal(ctx, hub, goalID)
	if err != nil {
		return workorchmanagementcontract.Goal{}, err
	}
	return definitioncore.MapGoal(item), nil
}

func ListConsolidatedWorkspaceGoalViews(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]workorchmanagementcontract.Goal, error) {
	items, err := ListConsolidatedWorkspaceGoals(ctx, hub, workspaceID)
	if err != nil {
		return nil, err
	}
	return definitioncore.MapGoals(items), nil
}

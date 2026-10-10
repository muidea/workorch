package definitionquality

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitioncore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionports"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func CheckGoalDoDQuality(ctx context.Context, hub magicevent.Hub, cmd Command) (Check, error) {
	if hub == nil {
		return Check{}, common.ErrInvalidArgument
	}
	workspaceID := common.WorkspaceID(strings.TrimSpace(cmd.WorkspaceID))
	if workspaceID == "" {
		return Check{}, common.ErrInvalidArgument
	}
	scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)

	goalText, resolvedGoalID, err := ResolveGoal(
		scopedCtx,
		func(ctx context.Context, id common.GoalID) (string, common.GoalID, error) {
			item, err := definitionports.ReadGoal(ctx, hub, id)
			if err != nil {
				return "", "", err
			}
			return strings.TrimSpace(item.Text), item.ID, nil
		},
		func(ctx context.Context, workspaceID common.WorkspaceID) (string, common.GoalID, error) {
			item, err := activationcore.SelectWorkspaceExecutionGoal(ctx, func(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
				return definitionports.ListGoalsByWorkspace(ctx, hub, workspaceID)
			}, workspaceID, "")
			if err != nil {
				return "", "", err
			}
			if item == nil {
				return "", "", nil
			}
			return strings.TrimSpace(item.Text), item.ID, nil
		},
		workspaceID,
		common.GoalID(strings.TrimSpace(cmd.GoalID)),
		cmd.GoalText,
	)
	if err != nil {
		return Check{}, err
	}
	dodItems, resolvedDoDID, err := ResolveDoD(
		scopedCtx,
		func(ctx context.Context, id common.DoDID) ([]string, common.DoDID, error) {
			item, err := definitionports.ReadDoD(ctx, hub, id)
			if err != nil {
				return nil, "", err
			}
			return definitioncore.NormalizeDoDItems(item.Items), item.ID, nil
		},
		func(ctx context.Context, workspaceID common.WorkspaceID) ([]string, common.DoDID, error) {
			item, err := activationcore.SelectWorkspaceExecutionDoD(ctx, func(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
				return definitionports.ListDoDsByWorkspace(ctx, hub, workspaceID)
			}, workspaceID, "")
			if err != nil {
				return nil, "", err
			}
			if item == nil {
				return nil, "", nil
			}
			return definitioncore.NormalizeDoDItems(item.Items), item.ID, nil
		},
		workspaceID,
		common.DoDID(strings.TrimSpace(cmd.DoDID)),
		cmd.DoDItems,
	)
	if err != nil {
		return Check{}, err
	}

	return Evaluate(workspaceID, resolvedGoalID, resolvedDoDID, goalText, dodItems), nil
}

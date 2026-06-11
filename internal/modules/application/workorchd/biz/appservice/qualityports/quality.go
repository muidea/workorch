package qualityports

import (
	"context"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitioncore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitionports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/qualitycore"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func CheckGoalDoDQuality(ctx context.Context, hub events.Hub, cmd qualitycore.Command) (qualitycore.Check, error) {
	if hub == nil {
		return qualitycore.Check{}, common.ErrInvalidArgument
	}
	workspaceID := common.WorkspaceID(strings.TrimSpace(cmd.WorkspaceID))
	if workspaceID == "" {
		return qualitycore.Check{}, common.ErrInvalidArgument
	}
	scopedCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)

	goalText, resolvedGoalID, err := qualitycore.ResolveGoal(
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
			}, nil, workspaceID, "")
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
		return qualitycore.Check{}, err
	}
	dodItems, resolvedDoDID, err := qualitycore.ResolveDoD(
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
			}, nil, workspaceID, "")
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
		return qualitycore.Check{}, err
	}

	return qualitycore.Evaluate(workspaceID, resolvedGoalID, resolvedDoDID, goalText, dodItems), nil
}

package definitionports

import (
	"context"
	"errors"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitioncore"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ConsolidateGoal(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, canonicalID common.GoalID, text string, obsoleteIDs []common.GoalID) (*workspacemodel.Goal, error) {
	updated, err := UpdateGoal(ctx, hub, workspaceID, canonicalID, text)
	if err != nil {
		return nil, err
	}
	for _, goalID := range obsoleteIDs {
		if goalID == "" || goalID == canonicalID {
			continue
		}
		if err := DeleteGoal(ctx, hub, workspaceID, goalID); err != nil && !errors.Is(err, common.ErrNotFound) {
			return nil, err
		}
	}
	return updated, nil
}

func ConsolidateWorkspaceGoal(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, existing []*workspacemodel.Goal, appended string) (*workspacemodel.Goal, error) {
	plan, err := definitioncore.WorkspaceGoalConsolidationPlan(existing, workspaceID, appended)
	if err != nil {
		return nil, err
	}
	return ConsolidateGoal(ctx, hub, workspaceID, plan.CanonicalID, plan.Text, plan.ObsoleteIDs)
}

func DefineOrConsolidateWorkspaceGoal(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, text string) (*workspacemodel.Goal, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, common.ErrInvalidArgument
	}
	text = definitioncore.FormatGoalDefinitionText(text)
	if text == "" {
		return nil, common.ErrInvalidArgument
	}
	existing, err := ListGoalsByWorkspace(ctx, hub, workspaceID)
	if err != nil {
		return nil, err
	}
	existing, err = activationcore.OrderedWorkspaceGoals(existing)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		return ConsolidateWorkspaceGoal(ctx, hub, workspaceID, existing, text)
	}
	return defineNewWorkspaceGoal(ctx, hub, workspaceID, text)
}

func ListConsolidatedWorkspaceGoals(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
	items, err := ListGoalsByWorkspace(ctx, hub, workspaceID)
	if err != nil {
		return nil, err
	}
	items, err = activationcore.OrderedWorkspaceGoals(items)
	if err != nil {
		return nil, err
	}
	if len(items) <= 1 {
		return items, nil
	}
	item, err := ConsolidateWorkspaceGoal(ctx, hub, workspaceID, items, "")
	if err != nil {
		return nil, err
	}
	return []*workspacemodel.Goal{item}, nil
}

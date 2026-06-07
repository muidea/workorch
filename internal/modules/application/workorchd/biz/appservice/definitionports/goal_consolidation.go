package definitionports

import (
	"context"
	"errors"
	"strings"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/activationcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/definitioncore"
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ConsolidateGoal(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, canonicalID common.GoalID, text string, obsoleteIDs []common.GoalID) (*workspacemodel.Goal, error) {
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

func ConsolidateWorkspaceGoal(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID, existing []*workspacemodel.Goal, appended string) (*workspacemodel.Goal, error) {
	plan, err := definitioncore.WorkspaceGoalConsolidationPlan(existing, workspaceID, appended)
	if err != nil {
		return nil, err
	}
	return ConsolidateGoal(ctx, hub, workspaceID, plan.CanonicalID, plan.Text, plan.ObsoleteIDs)
}

func DefineOrConsolidateWorkspaceGoal(ctx context.Context, hub events.Hub, eventRecords activationcore.EventRecordLister, workspaceID common.WorkspaceID, text string) (*workspacemodel.Goal, error) {
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
	existing = activationcore.OrderedWorkspaceGoals(ctx, eventRecords, workspaceID, existing)
	if len(existing) > 0 {
		return ConsolidateWorkspaceGoal(ctx, hub, workspaceID, existing, text)
	}
	return defineNewWorkspaceGoal(ctx, hub, workspaceID, text)
}

func ListConsolidatedWorkspaceGoals(ctx context.Context, hub events.Hub, eventRecords activationcore.EventRecordLister, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
	items, err := ListGoalsByWorkspace(ctx, hub, workspaceID)
	if err != nil {
		return nil, err
	}
	items = activationcore.OrderedWorkspaceGoals(ctx, eventRecords, workspaceID, items)
	if len(items) <= 1 {
		return items, nil
	}
	item, err := ConsolidateWorkspaceGoal(ctx, hub, workspaceID, items, "")
	if err != nil {
		return nil, err
	}
	return []*workspacemodel.Goal{item}, nil
}

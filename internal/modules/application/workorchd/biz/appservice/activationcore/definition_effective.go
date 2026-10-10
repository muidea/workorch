package activationcore

import (
	"context"
	"strings"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func EffectiveWorkspaceGoal(
	ctx context.Context,
	listByWorkspaceID func(context.Context, common.WorkspaceID) ([]*workspacemodel.Goal, error),
	workspaceID common.WorkspaceID,
	selected *workspacemodel.Goal,
) (*workspacemodel.Goal, []map[string]any, error) {
	if listByWorkspaceID == nil || selected == nil {
		return selected, nil, nil
	}
	items, err := listByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	items, err = OrderedWorkspaceGoals(items)
	if err != nil {
		return nil, nil, err
	}
	contexts := make([]map[string]any, 0, len(items))
	parts := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		contexts = append(contexts, GoalContext(item))
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		parts = append(parts, text)
	}
	effective := *selected
	if latest := LatestWorkspaceGoal(items); latest != nil {
		effective = *latest
	}
	if len(parts) > 0 {
		effective.Text = strings.Join(parts, "\n")
	}
	return &effective, contexts, nil
}

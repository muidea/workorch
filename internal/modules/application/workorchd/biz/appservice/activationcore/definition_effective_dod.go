package activationcore

import (
	"context"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/doditems"
)

func EffectiveWorkspaceDoD(
	ctx context.Context,
	listByWorkspaceID func(context.Context, common.WorkspaceID) ([]*workspacemodel.DoD, error),
	eventRecords EventRecordLister,
	workspaceID common.WorkspaceID,
	selected *workspacemodel.DoD,
) (*workspacemodel.DoD, []map[string]any, error) {
	if listByWorkspaceID == nil || selected == nil {
		return selected, nil, nil
	}
	items, err := listByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	items = OrderedWorkspaceDoDs(ctx, eventRecords, workspaceID, items)
	contexts := make([]map[string]any, 0, len(items))
	mergedItems := make([]string, 0)
	seen := map[string]struct{}{}
	for _, item := range items {
		if item == nil || item.WorkspaceID != workspaceID || (item.State != common.DoDStateReady && item.State != common.DoDStateSatisfied) {
			continue
		}
		normalized := doditems.Normalize(item.Items)
		if len(normalized) == 0 {
			continue
		}
		contexts = append(contexts, DoDContext(item))
		for _, entry := range normalized {
			if _, ok := seen[entry]; ok {
				continue
			}
			seen[entry] = struct{}{}
			mergedItems = append(mergedItems, entry)
		}
	}
	effective := *selected
	if latest := LatestWorkspaceDoD(items); latest != nil {
		effective = *latest
	}
	if len(mergedItems) > 0 {
		effective.Items = mergedItems
		effective.State = common.DoDStateReady
	}
	return &effective, contexts, nil
}

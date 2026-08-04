package activationcore

import (
	"context"

	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"

	eventstreammodel "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/models"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/workorchquerycontract"
)

type EventRecordLister interface {
	List(ctx context.Context, query workorchquerycontract.EventRecordQuery) ([]*eventstreammodel.EventRecord, error)
}

func OrderedWorkspaceGoals(ctx context.Context, eventRecords EventRecordLister, workspaceID common.WorkspaceID, items []*workspacemodel.Goal) []*workspacemodel.Goal {
	return OrderedWorkspaceDefinitions(ctx, eventRecords, workspaceID, workspaceevents.TopicGoalDefined, items, func(item *workspacemodel.Goal) common.GoalID {
		if item == nil {
			return ""
		}
		return item.ID
	})
}

func OrderedWorkspaceDoDs(ctx context.Context, eventRecords EventRecordLister, workspaceID common.WorkspaceID, items []*workspacemodel.DoD) []*workspacemodel.DoD {
	return OrderedWorkspaceDefinitions(ctx, eventRecords, workspaceID, workspaceevents.TopicDoDDefined, items, func(item *workspacemodel.DoD) common.DoDID {
		if item == nil {
			return ""
		}
		return item.ID
	})
}

package governancecore

import (
	"context"
	"errors"

	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	projectioncontract "github.com/muidea/workorch/internal/pkg/workorchprojectioncontract"
)

func LatestAutoGovernanceSummary(
	ctx context.Context,
	traces AutoGovernanceTraceReader,
	workspaceID common.WorkspaceID,
	runID common.RunID,
	itemType string,
	itemID string,
) (*projectioncontract.AutoGovernanceSummary, error) {
	if traces == nil || itemType == "" || itemID == "" {
		return nil, nil
	}
	list, err := traces.List(ctx, governanceevents.AutoGovernanceTraceQuery{
		WorkspaceID: workspaceID,
		RunID:       runID,
		ItemType:    itemType,
		ItemID:      itemID,
		Limit:       10,
	})
	if err != nil {
		if errors.Is(err, common.ErrInvalidArgument) {
			return nil, nil
		}
		return nil, err
	}
	if len(list) == 0 {
		return nil, err
	}
	latest := LatestAutoGovernanceTrace(list)
	if latest == nil {
		return nil, nil
	}
	return MapAutoGovernanceSummary(latest), nil
}

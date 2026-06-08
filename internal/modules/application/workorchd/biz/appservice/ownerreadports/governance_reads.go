package ownerreadports

import (
	"context"

	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"

	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func ListAutoGovernanceTracesByOwnerCommand(ctx context.Context, hub events.Hub, query repository.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, governanceevents.TopicGovernanceAutoCommandListTraces, "auto_governance_trace", "list", query.WorkspaceID, query.RunID, map[string]any{
		events.PayloadKeyQuery: query,
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeyItems].([]*governancemodel.AutoGovernanceTrace)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"

	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func ListAutoGovernanceTracesByOwnerCommand(ctx context.Context, hub magicevent.Hub, query repository.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[governanceevents.ListAutoGovernanceTracesCommand, governanceevents.GovernanceCommandResult](ctx, hub, governanceevents.TopicGovernanceAutoCommandListTraces, governanceevents.CommandDestination, "auto_governance_trace", "list", governanceevents.ListAutoGovernanceTracesCommand{Query: query})
	if err != nil {
		return nil, err
	}
	return response.Traces, nil
}

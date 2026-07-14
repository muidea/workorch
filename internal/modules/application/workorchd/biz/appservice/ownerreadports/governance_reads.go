package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	governanceevents "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	governancemodel "github.com/muidea/workorch/internal/modules/kernel/governance/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/repository"
)

func ListAutoGovernanceTracesByOwnerCommand(ctx context.Context, hub magicevent.Hub, query repository.AutoGovernanceTraceQuery) ([]*governancemodel.AutoGovernanceTrace, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(governanceevents.TopicGovernanceAutoCommandListTraces, sourceModule, governanceevents.CommandDestination, magicevent.NewHeader(), ctx, governanceevents.ListAutoGovernanceTracesCommand{Query: query})
	ownerevent.BindLane(event, governanceevents.TopicGovernanceAutoCommandListTraces, "auto_governance_trace", "list", query.WorkspaceID, query.RunID, query.StepID)
	response, err := ownerevent.Governance(hub.Send(event), governanceevents.TopicGovernanceAutoCommandListTraces)
	if err != nil {
		return nil, err
	}
	return response.Traces, nil
}

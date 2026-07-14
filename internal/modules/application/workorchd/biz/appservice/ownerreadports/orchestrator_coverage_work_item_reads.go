package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	"github.com/muidea/workorch/internal/pkg/common"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func ListCoverageWorkItemsByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*orchestratormodel.CoverageWorkItem, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun, sourceModule, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.ListCoverageWorkItemsByRunCommand{WorkspaceID: workspaceID, RunID: runID})
	ownerevent.BindLane(event, orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun, "run", string(runID), workspaceID, runID, "")
	response, err := ownerevent.Orchestrator(hub.Send(event), orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun)
	if err != nil {
		return nil, err
	}
	return response.CoverageWorkItems, nil
}

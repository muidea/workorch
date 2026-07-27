package runports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func CancelRun(ctx context.Context, hub magicevent.Hub, item *runmodel.Run) error {
	if item == nil || item.ID == "" {
		return common.ErrInvalidArgument
	}
	if hub == nil {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorRequestRuntimeTermination, "application/workorchd/appservice", orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.RequestRuntimeTerminationCommand{WorkspaceID: item.WorkspaceID, RunID: item.ID, Kind: runtimecontract.RuntimeTerminationCancelled, RequestedBy: "application/workorchd/appservice", Reason: "application cancellation"})
	ownerevent.BindLane(event, orchestratorevents.TopicOrchestratorRequestRuntimeTermination, "run", string(item.ID), item.WorkspaceID, item.ID, "")
	response := hub.Send(event)
	if response == nil {
		return common.ErrInvalidArgument
	}
	return response.Error()
}

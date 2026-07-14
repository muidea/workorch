package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadResultByOwnerCommand(ctx context.Context, hub magicevent.Hub, resultID common.ResultID) (*resultmodel.Result, error) {
	if hub == nil || resultID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	commandCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	event := magicevent.NewEventWithContext(resultevents.TopicResultCommandGet, sourceModule, resultevents.CommandDestination, magicevent.NewHeader(), commandCtx, resultevents.GetResultCommand{ResultID: resultID})
	ownerevent.BindLane(event, resultevents.TopicResultCommandGet, "result", string(resultID), workspaceID, "", "")
	response, err := ownerevent.Result(hub.Send(event), resultevents.TopicResultCommandGet)
	if err != nil {
		return nil, err
	}
	if response.Result == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Result, nil
}

package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	resultevents "github.com/muidea/workorch/internal/modules/blocks/result/pkg/events"

	resultmodel "github.com/muidea/workorch/internal/modules/blocks/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadResultByOwnerCommand(ctx context.Context, hub magicevent.Hub, resultID common.ResultID) (*resultmodel.Result, error) {
	if hub == nil || resultID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	response, err := sendMagicCommand[resultevents.GetResultCommand, resultevents.ResultCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, resultevents.TopicResultCommandGet, resultevents.CommandDestination, "result", string(resultID), resultevents.GetResultCommand{ResultID: resultID})
	if err != nil {
		return nil, err
	}
	if response.Result == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Result, nil
}

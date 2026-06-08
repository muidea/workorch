package ownerreadports

import (
	"context"

	resultevents "github.com/muidea/workorch/internal/modules/kernel/result/pkg/events"

	resultcontracts "github.com/muidea/workorch/internal/modules/kernel/result/pkg/contracts"
	resultmodel "github.com/muidea/workorch/internal/modules/kernel/result/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadResultByOwnerCommand(ctx context.Context, hub events.Hub, resultID common.ResultID) (*resultmodel.Result, error) {
	if hub == nil || resultID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, resultevents.TopicResultCommandGet, "result", string(resultID), "", "", map[string]any{
		events.PayloadKeyCommand: resultcontracts.GetResultCommand{ResultID: resultID},
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyResult].(*resultmodel.Result)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

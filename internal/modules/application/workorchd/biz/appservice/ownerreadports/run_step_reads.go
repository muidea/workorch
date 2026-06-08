package ownerreadports

import (
	"context"

	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadStepByOwnerCommand(ctx context.Context, hub events.Hub, stepID common.StepID) (*runmodel.Step, error) {
	if hub == nil || stepID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, runevents.TopicRunCommandGetStep, "step", string(stepID), "", "", map[string]any{
		events.PayloadKeyStepID: string(stepID),
	})
	if err != nil {
		return nil, err
	}
	item, ok := response.Payload[events.PayloadKeyStep].(*runmodel.Step)
	if !ok || item == nil {
		return nil, common.ErrInvalidArgument
	}
	return item, nil
}

func ListStepsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*runmodel.Step, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := PublishCommandWithResponse(ctx, hub, runevents.TopicRunCommandListStepsByRun, "run", string(runID), "", runID, map[string]any{
		events.PayloadKeyRunID: string(runID),
	})
	if err != nil {
		return nil, err
	}
	items, ok := response.Payload[events.PayloadKeySteps].([]*runmodel.Step)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	return items, nil
}

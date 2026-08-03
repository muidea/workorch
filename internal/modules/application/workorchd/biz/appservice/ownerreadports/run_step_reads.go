package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadStepByOwnerCommand(ctx context.Context, hub magicevent.Hub, stepID common.StepID) (*runmodel.Step, error) {
	if hub == nil || stepID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	commandCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandGetStep, sourceModule, runevents.CommandDestination, magicevent.NewHeader(), commandCtx, runevents.GetStepCommand{
		WorkspaceID: workspaceID,
		StepID:      stepID,
	})
	ownerevent.BindLane(event, runevents.Contracts(), runevents.TopicRunCommandGetStep, "step", string(stepID), workspaceID, "", stepID)
	response, err := ownerevent.Run(hub.Send(event), runevents.TopicRunCommandGetStep)
	if err != nil {
		return nil, err
	}
	if response.Step == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Step, nil
}

func ListStepsByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*runmodel.Step, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	commandCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandListStepsByRun, sourceModule, runevents.CommandDestination, magicevent.NewHeader(), commandCtx, runevents.ListStepsByRunCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
	})
	ownerevent.BindLane(event, runevents.Contracts(), runevents.TopicRunCommandListStepsByRun, "run", string(runID), workspaceID, runID, "")
	response, err := ownerevent.Run(hub.Send(event), runevents.TopicRunCommandListStepsByRun)
	if err != nil {
		return nil, err
	}
	return response.Steps, nil
}

func ListStepSummariesByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*runmodel.Step, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	commandCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandListStepSummariesByRun, sourceModule, runevents.CommandDestination, magicevent.NewHeader(), commandCtx, runevents.ListStepSummariesByRunCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
	})
	ownerevent.BindLane(event, runevents.Contracts(), runevents.TopicRunCommandListStepSummariesByRun, "run", string(runID), workspaceID, runID, "")
	response, err := ownerevent.Run(hub.Send(event), runevents.TopicRunCommandListStepSummariesByRun)
	if err != nil {
		return nil, err
	}
	return response.Steps, nil
}

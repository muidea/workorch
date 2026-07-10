package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadStepByOwnerCommand(ctx context.Context, hub magicevent.Hub, stepID common.StepID) (*runmodel.Step, error) {
	if hub == nil || stepID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	response, err := sendMagicCommand[runevents.GetStepCommand, runevents.RunCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, runevents.TopicRunCommandGetStep, runevents.CommandDestination, "step", string(stepID), runevents.GetStepCommand{
		WorkspaceID: workspaceID,
		StepID:      stepID,
	})
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
	response, err := sendMagicCommand[runevents.ListStepsByRunCommand, runevents.RunCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, runevents.TopicRunCommandListStepsByRun, runevents.CommandDestination, "run", string(runID), runevents.ListStepsByRunCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
	})
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
	response, err := sendMagicCommand[runevents.ListStepSummariesByRunCommand, runevents.RunCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, runevents.TopicRunCommandListStepSummariesByRun, runevents.CommandDestination, "run", string(runID), runevents.ListStepSummariesByRunCommand{
		WorkspaceID: workspaceID,
		RunID:       runID,
	})
	if err != nil {
		return nil, err
	}
	return response.Steps, nil
}

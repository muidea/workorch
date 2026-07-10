package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadRunByOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) (*runmodel.Run, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := common.RuntimeWorkspaceIDFromContext(ctx)
	if workspaceID != "" {
		return ReadRunByWorkspaceOwnerCommand(ctx, hub, workspaceID, runID)
	}
	response, err := sendMagicCommand[runevents.GetRunCommand, runevents.RunCommandResult](ctx, hub, runevents.TopicRunCommandGet, runevents.CommandDestination, "run", string(runID), runevents.GetRunCommand{RunID: runID})
	if err != nil {
		return nil, err
	}
	if response.Run == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Run, nil
}

func ReadRunByWorkspaceOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID, runID common.RunID) (*runmodel.Run, error) {
	if hub == nil || workspaceID == "" || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[runevents.GetRunCommand, runevents.RunCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, runevents.TopicRunCommandGet, runevents.CommandDestination, "run", string(runID), runevents.GetRunCommand{WorkspaceID: workspaceID, RunID: runID})
	if err != nil {
		return nil, err
	}
	if response.Run == nil {
		return nil, common.ErrInvalidArgument
	}
	return response.Run, nil
}

func ListRunsByOwnerCommand(ctx context.Context, hub magicevent.Hub) ([]*runmodel.Run, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[runevents.ListRunsCommand, runevents.RunCommandResult](ctx, hub, runevents.TopicRunCommandList, runevents.CommandDestination, "run", "all", runevents.ListRunsCommand{})
	if err != nil {
		return nil, err
	}
	return response.Runs, nil
}

func ListRecoverableRuntimeGapRunCandidatesByOwnerCommand(ctx context.Context, hub magicevent.Hub) ([]*runmodel.Run, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[runevents.ListRunsCommand, runevents.RunCommandResult](ctx, hub, runevents.TopicRunCommandList, runevents.CommandDestination, "run", "runtime-gap-candidates", runevents.ListRunsCommand{
		States:  []common.RunState{common.RunStateActive},
		Compact: true,
	})
	if err != nil {
		return nil, err
	}
	return response.Runs, nil
}

func ListRunsByWorkspaceOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	response, err := sendMagicCommand[runevents.ListRunsByWorkspaceCommand, runevents.RunCommandResult](common.ContextWithRuntimeWorkspaceID(ctx, workspaceID), hub, runevents.TopicRunCommandListByWorkspace, runevents.CommandDestination, "workspace", string(workspaceID), runevents.ListRunsByWorkspaceCommand{WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	return response.Runs, nil
}

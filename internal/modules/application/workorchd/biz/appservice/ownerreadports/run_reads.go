package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
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
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandGet, sourceModule, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.GetRunCommand{RunID: runID})
	ownerevent.BindLane(event, runevents.Contracts(), runevents.TopicRunCommandGet, "run", string(runID), "", runID, "")
	response, err := ownerevent.Run(hub.Send(event), runevents.TopicRunCommandGet)
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
	commandCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandGet, sourceModule, runevents.CommandDestination, magicevent.NewHeader(), commandCtx, runevents.GetRunCommand{WorkspaceID: workspaceID, RunID: runID})
	ownerevent.BindLane(event, runevents.Contracts(), runevents.TopicRunCommandGet, "run", string(runID), workspaceID, runID, "")
	response, err := ownerevent.Run(hub.Send(event), runevents.TopicRunCommandGet)
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
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandList, sourceModule, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.ListRunsCommand{})
	ownerevent.BindLane(event, runevents.Contracts(), runevents.TopicRunCommandList, "run", "all", "", "", "")
	response, err := ownerevent.Run(hub.Send(event), runevents.TopicRunCommandList)
	if err != nil {
		return nil, err
	}
	return response.Runs, nil
}

func ListRecoverableRuntimeGapRunCandidatesByOwnerCommand(ctx context.Context, hub magicevent.Hub) ([]*runmodel.Run, error) {
	if hub == nil {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandList, sourceModule, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.ListRunsCommand{
		States:  []common.RunState{common.RunStateActive},
		Compact: true,
	})
	ownerevent.BindLane(event, runevents.Contracts(), runevents.TopicRunCommandList, "run", "runtime-gap-candidates", "", "", "")
	response, err := ownerevent.Run(hub.Send(event), runevents.TopicRunCommandList)
	if err != nil {
		return nil, err
	}
	return response.Runs, nil
}

func ListRunsByWorkspaceOwnerCommand(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	commandCtx := common.ContextWithRuntimeWorkspaceID(ctx, workspaceID)
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandListByWorkspace, sourceModule, runevents.CommandDestination, magicevent.NewHeader(), commandCtx, runevents.ListRunsByWorkspaceCommand{WorkspaceID: workspaceID})
	ownerevent.BindLane(event, runevents.Contracts(), runevents.TopicRunCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := ownerevent.Run(hub.Send(event), runevents.TopicRunCommandListByWorkspace)
	if err != nil {
		return nil, err
	}
	return response.Runs, nil
}

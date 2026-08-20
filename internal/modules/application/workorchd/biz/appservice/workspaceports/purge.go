package workspaceports

import (
	"context"
	"errors"

	magicevent "github.com/muidea/magicCommon/event"
	eventstreamevents "github.com/muidea/workorch/internal/modules/blocks/eventstream/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/blocks/run/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	"github.com/muidea/workorch/internal/pkg/common"
)

func PurgeWorkspaceRecords(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) error {
	if hub == nil || workspaceID == "" {
		return common.ErrInvalidArgument
	}
	runIDs, err := WorkspaceRunIDs(ctx, hub, workspaceID)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			runIDs = nil
		} else {
			return err
		}
	}
	for _, runID := range runIDs {
		event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommandPurgeRun, sourceModule, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.PurgeRunCommand{WorkspaceID: workspaceID, RunID: runID})
		ownerevent.BindLane(event, orchestratorevents.Contracts(), orchestratorevents.TopicOrchestratorCommandPurgeRun, "run", string(runID), workspaceID, runID, "")
		if _, err := ownerevent.Orchestrator(hub.Send(event), orchestratorevents.TopicOrchestratorCommandPurgeRun); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	if err := purgeWorkspaceMemoryItems(ctx, hub, workspaceID); err != nil {
		return err
	}
	if err := purgeWorkspaceDecisionTickets(ctx, hub, workspaceID); err != nil {
		return err
	}
	if err := purgeWorkspaceApprovals(ctx, hub, workspaceID); err != nil {
		return err
	}
	if err := purgeWorkspaceResultsAndArtifacts(ctx, hub, workspaceID); err != nil {
		return err
	}
	for _, runID := range runIDs {
		event := magicevent.NewEventWithContext(runevents.TopicRunCommandDelete, sourceModule, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.DeleteRunCommand{
			WorkspaceID: workspaceID,
			RunID:       runID,
		})
		ownerevent.BindLane(event, runevents.Contracts(), runevents.TopicRunCommandDelete, "run", string(runID), workspaceID, runID, "")
		if _, err := ownerevent.Run[runevents.DeleteRunResult](hub.Send(event), runevents.TopicRunCommandDelete); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	return nil
}

func WorkspaceRunIDs(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]common.RunID, error) {
	if hub == nil || workspaceID == "" {
		return nil, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(runevents.TopicRunCommandListByWorkspace, sourceModule, runevents.CommandDestination, magicevent.NewHeader(), ctx, runevents.ListRunsByWorkspaceCommand{WorkspaceID: workspaceID})
	ownerevent.BindLane(event, runevents.Contracts(), runevents.TopicRunCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", "")
	response, err := ownerevent.Run[runevents.ListRunsByWorkspaceResult](hub.Send(event), runevents.TopicRunCommandListByWorkspace)
	if err != nil {
		return nil, err
	}
	ret := make([]common.RunID, 0, len(response.Runs))
	for _, item := range response.Runs {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		ret = append(ret, item.ID)
	}
	return ret, nil
}

func PurgeWorkspaceEventStream(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) error {
	if hub == nil || workspaceID == "" {
		return common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(eventstreamevents.TopicEventStreamCommandPurgeWorkspace, sourceModule, eventstreamevents.CommandDestination, magicevent.NewHeader(), ctx, eventstreamevents.PurgeWorkspaceCommand{WorkspaceID: workspaceID})
	ownerevent.BindLane(event, eventstreamevents.Contracts(), eventstreamevents.TopicEventStreamCommandPurgeWorkspace, "workspace", string(workspaceID), workspaceID, "", "")
	_, err := ownerevent.EventStream[eventstreamevents.PurgeWorkspaceResult](hub.Send(event), eventstreamevents.TopicEventStreamCommandPurgeWorkspace)
	if errors.Is(err, common.ErrNotFound) || errors.Is(err, common.ErrConflict) {
		return nil
	}
	return err
}

package workspaceports

import (
	"context"
	"errors"

	eventstreamevents "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/events"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	runevents "github.com/muidea/workorch/internal/modules/kernel/run/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	orchestratorcontracts "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/contracts"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func PurgeWorkspaceRecords(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
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
		if err := ownercommandports.Publish(ctx, hub, orchestratorevents.TopicOrchestratorCommandPurgeRun, "run", string(runID), workspaceID, runID, map[string]any{
			events.PayloadKeyCommand: orchestratorcontracts.PurgeRunCommand{RunID: runID},
		}); err != nil && !errors.Is(err, common.ErrNotFound) {
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
		if _, err := sendWorkspaceMagicCommand[runevents.DeleteRunCommand, runevents.RunCommandResult](ctx, hub, runevents.TopicRunCommandDelete, runevents.CommandDestination, "run", string(runID), workspaceID, runID, runevents.DeleteRunCommand{
			WorkspaceID: workspaceID,
			RunID:       runID,
		}); err != nil && !errors.Is(err, common.ErrNotFound) {
			return err
		}
	}
	return nil
}

func WorkspaceRunIDs(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]common.RunID, error) {
	response, err := sendWorkspaceMagicCommand[runevents.ListRunsByWorkspaceCommand, runevents.RunCommandResult](ctx, hub, runevents.TopicRunCommandListByWorkspace, runevents.CommandDestination, "workspace", string(workspaceID), workspaceID, "", runevents.ListRunsByWorkspaceCommand{WorkspaceID: workspaceID})
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

func PurgeWorkspaceEventStream(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	_, err := sendWorkspaceMagicCommand[eventstreamevents.PurgeWorkspaceCommand, eventstreamevents.EventStreamCommandResult](ctx, hub, eventstreamevents.TopicEventStreamCommandPurgeWorkspace, eventstreamevents.CommandDestination, "workspace", string(workspaceID), workspaceID, "", eventstreamevents.PurgeWorkspaceCommand{WorkspaceID: workspaceID})
	if errors.Is(err, common.ErrNotFound) || errors.Is(err, common.ErrConflict) {
		return nil
	}
	return err
}

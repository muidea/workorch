package workspaceports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownercommandports"
	eventstreamcontracts "github.com/muidea/workorch/internal/modules/kernel/eventstream/pkg/contracts"
	orchestratorcontracts "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/contracts"
	runcontracts "github.com/muidea/workorch/internal/modules/kernel/run/pkg/contracts"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func PurgeWorkspaceRecords(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	if hub == nil || workspaceID == "" {
		return common.ErrInvalidArgument
	}
	runIDs, err := WorkspaceRunIDs(ctx, hub, workspaceID)
	if err != nil {
		return err
	}
	for _, runID := range runIDs {
		if err := ownercommandports.Publish(ctx, hub, events.TopicOrchestratorCommandPurgeRun, "run", string(runID), workspaceID, runID, map[string]any{
			events.PayloadKeyCommand: orchestratorcontracts.PurgeRunCommand{RunID: runID},
		}); err != nil {
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
		if err := ownercommandports.Publish(ctx, hub, events.TopicRunCommandDelete, "run", string(runID), workspaceID, runID, map[string]any{
			events.PayloadKeyCommand: runcontracts.DeleteRunCommand{RunID: runID},
		}); err != nil {
			return err
		}
	}
	return nil
}

func WorkspaceRunIDs(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]common.RunID, error) {
	response, err := ownercommandports.PublishWithResponse(ctx, hub, events.TopicRunCommandListByWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyWorkspaceID: string(workspaceID),
	})
	if err != nil {
		return nil, err
	}
	runs, ok := response.Payload[events.PayloadKeyRuns].([]*runmodel.Run)
	if !ok {
		return nil, common.ErrInvalidArgument
	}
	ret := make([]common.RunID, 0, len(runs))
	for _, item := range runs {
		if item == nil || item.WorkspaceID != workspaceID {
			continue
		}
		ret = append(ret, item.ID)
	}
	return ret, nil
}

func PurgeWorkspaceEventStream(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) error {
	return ownercommandports.Publish(ctx, hub, events.TopicEventStreamCommandPurgeWorkspace, "workspace", string(workspaceID), workspaceID, "", map[string]any{
		events.PayloadKeyCommand: eventstreamcontracts.PurgeWorkspaceCommand{WorkspaceID: workspaceID},
	})
}

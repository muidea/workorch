package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	"github.com/muidea/workorch/internal/pkg/common"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func ListArtifactContinuationsByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*orchestratormodel.ArtifactContinuation, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun, sourceModule, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.ListArtifactContinuationsByRunCommand{WorkspaceID: workspaceID, RunID: runID})
	ownerevent.BindLane(event, orchestratorevents.Contracts(), orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun, "run", string(runID), workspaceID, runID, "")
	response, err := ownerevent.Orchestrator(hub.Send(event), orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun)
	if err != nil {
		return nil, err
	}
	return response.ArtifactContinuations, nil
}

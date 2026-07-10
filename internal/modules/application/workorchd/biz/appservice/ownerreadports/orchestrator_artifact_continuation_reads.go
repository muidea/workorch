package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

	"github.com/muidea/workorch/internal/pkg/common"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func ListArtifactContinuationsByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*orchestratormodel.ArtifactContinuation, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	response, err := sendMagicCommand[orchestratorevents.ListArtifactContinuationsByRunCommand, orchestratorevents.OrchestratorCommandResult](ctx, hub, orchestratorevents.TopicOrchestratorCommandListArtifactContinuationsByRun, orchestratorevents.CommandDestination, "run", string(runID), orchestratorevents.ListArtifactContinuationsByRunCommand{WorkspaceID: workspaceID, RunID: runID})
	if err != nil {
		return nil, err
	}
	return response.ArtifactContinuations, nil
}

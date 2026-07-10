package ownerreadports

import (
	"context"

	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
	orchestratormodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func ListCoverageWorkItemsByRunOwnerCommand(ctx context.Context, hub events.Hub, runID common.RunID) ([]*orchestratormodel.CoverageWorkItem, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	response, err := sendMagicCommand[orchestratorevents.ListCoverageWorkItemsByRunCommand, orchestratorevents.OrchestratorCommandResult](ctx, hub, orchestratorevents.TopicOrchestratorCommandListCoverageWorkItemsByRun, orchestratorevents.CommandDestination, "run", string(runID), orchestratorevents.ListCoverageWorkItemsByRunCommand{WorkspaceID: workspaceID, RunID: runID})
	if err != nil {
		return nil, err
	}
	return response.CoverageWorkItems, nil
}

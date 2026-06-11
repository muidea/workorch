package runports

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/events"
)

func ReadRun(ctx context.Context, hub events.Hub, runID common.RunID) (*runmodel.Run, error) {
	return ownerreadports.ReadRunByOwnerCommand(ctx, hub, runID)
}

func ListRuns(ctx context.Context, hub events.Hub) ([]*runmodel.Run, error) {
	return ownerreadports.ListRunsByOwnerCommand(ctx, hub)
}

func ListRunsByWorkspace(ctx context.Context, hub events.Hub, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	if workspaceID == "" {
		return ListRuns(ctx, hub)
	}
	return ownerreadports.ListRunsByWorkspaceOwnerCommand(ctx, hub, workspaceID)
}

func ListStepsByRun(ctx context.Context, hub events.Hub, runID common.RunID) ([]*runmodel.Step, error) {
	return ownerreadports.ListStepsByRunOwnerCommand(ctx, hub, runID)
}

func ListRuntimeCheckpointsByRun(ctx context.Context, hub events.Hub, runID common.RunID) ([]*runmodel.RuntimeCheckpoint, error) {
	return ownerreadports.ListRuntimeCheckpointsByRunOwnerCommand(ctx, hub, runID)
}

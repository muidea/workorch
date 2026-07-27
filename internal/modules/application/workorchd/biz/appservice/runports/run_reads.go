package runports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadRun(ctx context.Context, hub magicevent.Hub, runID common.RunID) (*runmodel.Run, error) {
	return ownerreadports.ReadRunByOwnerCommand(ctx, hub, runID)
}

func ListRuns(ctx context.Context, hub magicevent.Hub) ([]*runmodel.Run, error) {
	return ownerreadports.ListRunsByOwnerCommand(ctx, hub)
}

func ListRecoverableRuntimeGapRunCandidates(ctx context.Context, hub magicevent.Hub) ([]*runmodel.Run, error) {
	return ownerreadports.ListRecoverableRuntimeGapRunCandidatesByOwnerCommand(ctx, hub)
}

func ListRunsByWorkspace(ctx context.Context, hub magicevent.Hub, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	if workspaceID == "" {
		return ListRuns(ctx, hub)
	}
	return ownerreadports.ListRunsByWorkspaceOwnerCommand(ctx, hub, workspaceID)
}

func ListStepsByRun(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*runmodel.Step, error) {
	return ownerreadports.ListStepsByRunOwnerCommand(ctx, hub, runID)
}

func ListStepSummariesByRun(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*runmodel.Step, error) {
	return ownerreadports.ListStepSummariesByRunOwnerCommand(ctx, hub, runID)
}

func ListRuntimeCheckpointsByRun(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*runmodel.RuntimeCheckpoint, error) {
	return ownerreadports.ListRuntimeCheckpointsByRunOwnerCommand(ctx, hub, runID)
}

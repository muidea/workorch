package readmodelports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

type WorkspaceReader struct {
	Hub magicevent.Hub
}

func (r WorkspaceReader) Get(ctx context.Context, id common.WorkspaceID) (*workspacemodel.Workspace, error) {
	return ownerreadports.ReadWorkspaceByOwnerCommand(ctx, r.Hub, id)
}

func (r WorkspaceReader) List(ctx context.Context) ([]*workspacemodel.Workspace, error) {
	return ownerreadports.ListWorkspacesByOwnerCommand(ctx, r.Hub)
}

type GoalReader struct {
	Hub magicevent.Hub
}

func (r GoalReader) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.Goal, error) {
	return ownerreadports.ListGoalsByWorkspaceOwnerCommand(ctx, r.Hub, workspaceID)
}

type DoDReader struct {
	Hub magicevent.Hub
}

func (r DoDReader) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*workspacemodel.DoD, error) {
	return ownerreadports.ListDoDsByWorkspaceOwnerCommand(ctx, r.Hub, workspaceID)
}

type RunReader struct {
	Hub magicevent.Hub
}

func (r RunReader) Get(ctx context.Context, id common.RunID) (*runmodel.Run, error) {
	return ownerreadports.ReadRunByOwnerCommand(ctx, r.Hub, id)
}

func (r RunReader) GetByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID, id common.RunID) (*runmodel.Run, error) {
	return ownerreadports.ReadRunByWorkspaceOwnerCommand(ctx, r.Hub, workspaceID, id)
}

func (r RunReader) List(ctx context.Context) ([]*runmodel.Run, error) {
	return ownerreadports.ListRunsByOwnerCommand(ctx, r.Hub)
}

func (r RunReader) ListByWorkspaceID(ctx context.Context, workspaceID common.WorkspaceID) ([]*runmodel.Run, error) {
	return ownerreadports.ListRunsByWorkspaceOwnerCommand(ctx, r.Hub, workspaceID)
}

type StepReader struct {
	Hub magicevent.Hub
}

func (r StepReader) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.Step, error) {
	return ownerreadports.ListStepsByRunOwnerCommand(ctx, r.Hub, runID)
}

type RuntimeCheckpointReader struct {
	Hub magicevent.Hub
}

func (r RuntimeCheckpointReader) ListByRun(ctx context.Context, runID common.RunID) ([]*runmodel.RuntimeCheckpoint, error) {
	return ownerreadports.ListRuntimeCheckpointsByRunOwnerCommand(ctx, r.Hub, runID)
}

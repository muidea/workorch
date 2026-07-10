package readmodelports

import (
	"context"
	"strings"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerreadports"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelcore"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/readmodelprojection"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ReadRunStepsPage(ctx context.Context, hub magicevent.Hub, query readmodelcore.StepQuery) (readmodelcore.StepPage, error) {
	ctx, runID, err := runScopedContext(ctx, hub, query.WorkspaceID, query.RunID)
	if err != nil {
		return readmodelcore.StepPage{}, err
	}
	items, err := ownerreadports.ListStepSummariesByRunOwnerCommand(ctx, hub, runID)
	if err != nil {
		return readmodelcore.StepPage{}, err
	}
	return readmodelprojection.PageRunSteps(items, query), nil
}

func ReadRunToolCallsPage(ctx context.Context, hub magicevent.Hub, query readmodelcore.ToolCallQuery) (readmodelcore.ToolCallPage, error) {
	ctx, runID, err := runScopedContext(ctx, hub, query.WorkspaceID, query.RunID)
	if err != nil {
		return readmodelcore.ToolCallPage{}, err
	}
	items, err := ownerreadports.ListToolCallsByRunOwnerCommand(ctx, hub, runID)
	if err != nil {
		return readmodelcore.ToolCallPage{}, err
	}
	return readmodelprojection.PageRunToolCalls(items, query), nil
}

func ReadRunRuntimeCheckpointsPage(ctx context.Context, hub magicevent.Hub, query readmodelcore.RuntimeCheckpointQuery) (readmodelcore.RuntimeCheckpointPage, error) {
	ctx, runID, err := runScopedContext(ctx, hub, query.WorkspaceID, query.RunID)
	if err != nil {
		return readmodelcore.RuntimeCheckpointPage{}, err
	}
	items, err := ownerreadports.ListRuntimeCheckpointsByRunOwnerCommand(ctx, hub, runID)
	if err != nil {
		return readmodelcore.RuntimeCheckpointPage{}, err
	}
	return readmodelprojection.PageRunRuntimeCheckpoints(items, query), nil
}

func runScopedContext(ctx context.Context, hub magicevent.Hub, workspaceID string, runID string) (context.Context, common.RunID, error) {
	if hub == nil {
		return ctx, "", common.ErrInvalidArgument
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ctx, "", common.ErrInvalidArgument
	}
	ctx = common.ContextWithRuntimeWorkspaceID(ctx, common.WorkspaceID(strings.TrimSpace(workspaceID)))
	id := common.RunID(runID)
	run, err := ownerreadports.ReadRunByOwnerCommand(ctx, hub, id)
	if err != nil {
		return ctx, "", err
	}
	if run == nil {
		return ctx, "", common.ErrInvalidArgument
	}
	return common.ContextWithRuntimeWorkspaceID(ctx, run.WorkspaceID), id, nil
}

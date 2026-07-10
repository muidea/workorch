package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

	runmodel "github.com/muidea/workorch/internal/modules/kernel/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ListToolCallsByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*runmodel.ToolCall, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	response, err := sendMagicCommand[orchestratorevents.ListToolCallsByRunCommand, orchestratorevents.OrchestratorCommandResult](ctx, hub, orchestratorevents.TopicOrchestratorCommandListToolCallsByRun, orchestratorevents.CommandDestination, "run", string(runID), orchestratorevents.ListToolCallsByRunCommand{WorkspaceID: workspaceID, RunID: runID})
	if err != nil {
		return nil, err
	}
	return response.ToolCalls, nil
}

func ListPatchesByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*runmodel.Patch, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	response, err := sendMagicCommand[orchestratorevents.ListPatchesByRunCommand, orchestratorevents.OrchestratorCommandResult](ctx, hub, orchestratorevents.TopicOrchestratorCommandListPatchesByRun, orchestratorevents.CommandDestination, "run", string(runID), orchestratorevents.ListPatchesByRunCommand{WorkspaceID: workspaceID, RunID: runID})
	if err != nil {
		return nil, err
	}
	return response.Patches, nil
}

func ListValidationResultsByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*runmodel.ValidationResult, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	response, err := sendMagicCommand[orchestratorevents.ListValidationResultsByRunCommand, orchestratorevents.OrchestratorCommandResult](ctx, hub, orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun, orchestratorevents.CommandDestination, "run", string(runID), orchestratorevents.ListValidationResultsByRunCommand{WorkspaceID: workspaceID, RunID: runID})
	if err != nil {
		return nil, err
	}
	return response.ValidationResults, nil
}

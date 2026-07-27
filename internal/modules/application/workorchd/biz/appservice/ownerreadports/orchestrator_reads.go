package ownerreadports

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	orchestratorevents "github.com/muidea/workorch/internal/modules/kernel/orchestrator/pkg/events"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	runmodel "github.com/muidea/workorch/internal/modules/blocks/run/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

func ListToolCallsByRunOwnerCommand(ctx context.Context, hub magicevent.Hub, runID common.RunID) ([]*runmodel.ToolCall, error) {
	if hub == nil || runID == "" {
		return nil, common.ErrInvalidArgument
	}
	workspaceID := workspaceScopeFromContext(ctx)
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommandListToolCallsByRun, sourceModule, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.ListToolCallsByRunCommand{WorkspaceID: workspaceID, RunID: runID})
	ownerevent.BindLane(event, orchestratorevents.TopicOrchestratorCommandListToolCallsByRun, "run", string(runID), workspaceID, runID, "")
	response, err := ownerevent.Orchestrator(hub.Send(event), orchestratorevents.TopicOrchestratorCommandListToolCallsByRun)
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
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommandListPatchesByRun, sourceModule, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.ListPatchesByRunCommand{WorkspaceID: workspaceID, RunID: runID})
	ownerevent.BindLane(event, orchestratorevents.TopicOrchestratorCommandListPatchesByRun, "run", string(runID), workspaceID, runID, "")
	response, err := ownerevent.Orchestrator(hub.Send(event), orchestratorevents.TopicOrchestratorCommandListPatchesByRun)
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
	event := magicevent.NewEventWithContext(orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun, sourceModule, orchestratorevents.CommandDestination, magicevent.NewHeader(), ctx, orchestratorevents.ListValidationResultsByRunCommand{WorkspaceID: workspaceID, RunID: runID})
	ownerevent.BindLane(event, orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun, "run", string(runID), workspaceID, runID, "")
	response, err := ownerevent.Orchestrator(hub.Send(event), orchestratorevents.TopicOrchestratorCommandListValidationResultsByRun)
	if err != nil {
		return nil, err
	}
	return response.ValidationResults, nil
}

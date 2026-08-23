package biz

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	"github.com/muidea/workorch/internal/pkg/events"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// workspaceCommandGateway is a send-only, concrete Workspace-owner gateway.
// It can compile a typed restart bootstrap from the runtime component, but it
// cannot select a lifecycle state or mutate any owner aggregate itself.
type workspaceCommandGateway struct {
	hub     magicevent.Hub
	runtime *runtimeComponent
}

func (g *workspaceCommandGateway) submit(ctx context.Context, command workorchdevents.WorkspaceRunRequestedCommand) (workorchdevents.WorkspaceRunRequestedResult, error) {
	if g == nil || g.hub == nil || !g.runtime.ready() {
		return workorchdevents.WorkspaceRunRequestedResult{}, errRuntimeComponentUnavailable
	}
	workspaceID := common.WorkspaceID(command.WorkspaceID)
	if workspaceID == "" || command.RequestID == "" {
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrInvalidArgument
	}
	ctx, err := common.BindRuntimeWorkspaceContext(ctx, workspaceID)
	if err != nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	intent := workspaceevents.WorkspaceExecutionIntentCommand{WorkspaceID: workspaceID, RequestID: command.RequestID}
	switch command.Intent {
	case "resume":
		intent.Kind = runtimecontract.RuntimeExecutionIntentResume
	case "restart":
		intent.Kind = runtimecontract.RuntimeExecutionIntentRestart
		bootstrap, err := appservice.CompileWorkspaceExecutionRestartBootstrapWithSession(ctx, g.runtime.borrow(), string(workspaceID))
		if err != nil {
			return workorchdevents.WorkspaceRunRequestedResult{}, err
		}
		intent.Bootstrap = bootstrap
	default:
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrInvalidArgument
	}
	event := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandExecutionIntent, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, intent)
	event.BindLaneKey(events.EventHubLaneKeyFor(workspaceevents.Contracts(), event.ID(), eventcontract.LaneScope{SubjectType: "workspace", SubjectID: string(workspaceID), WorkspaceID: string(workspaceID)}))
	response := g.hub.Send(event)
	if response == nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrConflict
	}
	if err := response.Error(); err != nil {
		if err.Cause != nil {
			return workorchdevents.WorkspaceRunRequestedResult{}, err.Cause
		}
		return workorchdevents.WorkspaceRunRequestedResult{}, err
	}
	data, resultErr := response.Get()
	if resultErr != nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, resultErr
	}
	result, ok := data.(workspaceevents.WorkspaceExecutionIntentResult)
	if !ok || result.WorkspaceID != workspaceID || result.RequestID != command.RequestID || result.Kind != intent.Kind {
		return workorchdevents.WorkspaceRunRequestedResult{}, common.ErrConflict
	}
	if err := result.Validate(); err != nil {
		return workorchdevents.WorkspaceRunRequestedResult{}, fmt.Errorf("%w: Workspace execution intent result: %v", common.ErrConflict, err)
	}
	return workorchdevents.WorkspaceRunRequestedResult(runtimecontract.RuntimeExecutionIntentAdmission(result)), nil
}

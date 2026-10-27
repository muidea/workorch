package biz

import (
	"context"
	"fmt"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
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
	if g == nil || g.hub == nil {
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
		if !g.runtime.ready() {
			return workorchdevents.WorkspaceRunRequestedResult{}, errRuntimeComponentUnavailable
		}
		intent.Kind = runtimecontract.RuntimeExecutionIntentResume
	case "restart":
		intent.Kind = runtimecontract.RuntimeExecutionIntentRestart
		original, err := g.recoverOriginal(ctx, workspaceevents.RecoverWorkspaceExecutionIntentCommand{WorkspaceID: workspaceID, RequestID: command.RequestID, Kind: intent.Kind})
		if err != nil {
			return workorchdevents.WorkspaceRunRequestedResult{}, err
		}
		if original.Admission != nil {
			return workorchdevents.WorkspaceRunRequestedResult(*original.Admission), nil
		}
		if !g.runtime.ready() {
			return workorchdevents.WorkspaceRunRequestedResult{}, errRuntimeComponentUnavailable
		}
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

func (g *workspaceCommandGateway) recoverOriginal(ctx context.Context, c workspaceevents.RecoverWorkspaceExecutionIntentCommand) (workspaceevents.RecoverWorkspaceExecutionIntentResult, error) {
	if err := c.Validate(); err != nil {
		return workspaceevents.RecoverWorkspaceExecutionIntentResult{}, err
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceCommandRecoverExecutionIntent, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, c)
	ev.BindLaneKey(events.EventHubLaneKeyFor(workspaceevents.Contracts(), ev.ID(), eventcontract.LaneScope{SubjectType: "workspace", SubjectID: string(c.WorkspaceID), WorkspaceID: string(c.WorkspaceID)}))
	response := g.hub.Send(ev)
	if response == nil {
		return workspaceevents.RecoverWorkspaceExecutionIntentResult{}, common.ErrConflict
	}
	if err := response.Error(); err != nil {
		if err.Cause != nil {
			return workspaceevents.RecoverWorkspaceExecutionIntentResult{}, err.Cause
		}
		return workspaceevents.RecoverWorkspaceExecutionIntentResult{}, err
	}
	data, err := response.Get()
	if err != nil {
		return workspaceevents.RecoverWorkspaceExecutionIntentResult{}, err
	}
	r, ok := data.(workspaceevents.RecoverWorkspaceExecutionIntentResult)
	if !ok || r.ValidateFor(c) != nil {
		return workspaceevents.RecoverWorkspaceExecutionIntentResult{}, common.ErrConflict
	}
	return r, nil
}

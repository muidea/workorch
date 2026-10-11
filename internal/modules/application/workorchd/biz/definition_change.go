package biz

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// 待审候选准备不接确认、admission 或执行意图，也不暴露 HTTP 写入口。
func (b *Biz) PrepareDefinitionChange(ctx context.Context, command workspaceevents.PrepareDefinitionChangeCommand) (workspaceevents.PrepareDefinitionChangeResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil || command.Validate() != nil {
		return workspaceevents.PrepareDefinitionChangeResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return workspaceevents.PrepareDefinitionChangeResult{}, err
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDefinitionChangePrepare, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID)}))
	response, err := ownerevent.WorkspaceOwner[workspaceevents.PrepareDefinitionChangeResult](b.SendEvent(ev), ev.ID())
	if err != nil {
		return workspaceevents.PrepareDefinitionChangeResult{}, err
	}
	if err := response.ValidateFor(command); err != nil {
		return workspaceevents.PrepareDefinitionChangeResult{}, err
	}
	response.Candidate = response.Candidate.Clone()
	return response, nil
}

func (b *Biz) QueryDefinitionChange(ctx context.Context, command workspaceevents.QueryDefinitionChangeCommand) (workspaceevents.QueryDefinitionChangeResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil || command.WorkspaceID == "" {
		return workspaceevents.QueryDefinitionChangeResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return workspaceevents.QueryDefinitionChangeResult{}, err
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceDefinitionChangeQuery, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), ev.ID(), eventcontract.LaneScope{WorkspaceID: string(command.WorkspaceID)}))
	response, err := ownerevent.WorkspaceOwner[workspaceevents.QueryDefinitionChangeResult](b.SendEvent(ev), ev.ID())
	if err != nil {
		return workspaceevents.QueryDefinitionChangeResult{}, err
	}
	if err := response.ValidateFor(command); err != nil {
		return workspaceevents.QueryDefinitionChangeResult{}, err
	}
	if response.Candidate != nil {
		candidate := response.Candidate.Clone()
		response.Candidate = &candidate
	}
	return response, nil
}

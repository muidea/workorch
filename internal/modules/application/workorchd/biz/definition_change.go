package biz

import (
	"context"

	magicevent "github.com/muidea/magicCommon/event"
	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/ownerevent"
	workspaceevents "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/events"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
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

// 派生与六审仍只是待审材料，不执行精确批准或定义应用。
func (b *Biz) DeriveDefinitionChangeAcceptance(ctx context.Context, id common.WorkspaceID, inputHash string) (workspace.DefinitionDerivedAcceptance, error) {
	view, err := b.QueryDefinitionChange(ctx, workspaceevents.QueryDefinitionChangeCommand{WorkspaceID: id})
	if err != nil {
		return workspace.DefinitionDerivedAcceptance{}, err
	}
	if inputHash == "" || view.Candidate == nil || view.Candidate.Record.InputHash != inputHash {
		return workspace.DefinitionDerivedAcceptance{}, common.ErrConflict
	}
	return b.deriveDefinitionAcceptance(ctx, id, inputHash)
}

func (b *Biz) CompileDefinitionChangeSpecification(ctx context.Context, id common.WorkspaceID, inputHash string) (workspace.GoalSpecification, error) {
	view, err := b.QueryDefinitionChange(ctx, workspaceevents.QueryDefinitionChangeCommand{WorkspaceID: id})
	if err != nil {
		return workspace.GoalSpecification{}, err
	}
	if inputHash == "" || view.Candidate == nil || view.Candidate.Record.InputHash != inputHash {
		return workspace.GoalSpecification{}, common.ErrConflict
	}
	return b.compileAuthoringSpecification(ctx, id, inputHash)
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

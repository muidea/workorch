package biz

import (
	"context"
	"fmt"
	"reflect"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"

	magicevent "github.com/muidea/magicCommon/event"
	workspaceevents "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/eventcontract"
	workorchevents "github.com/muidea/workorch/internal/pkg/events"
)

// BeginDefinitionSpecification 请求 Workspace 封存编译输入，不进行准入或调用 provider。
func (b *Biz) BeginDefinitionSpecification(ctx context.Context, command workspaceevents.BeginDefinitionSpecificationCommand) (workspaceevents.BeginDefinitionSpecificationResult, error) {
	if b == nil || b.EventHub() == nil || ctx == nil {
		return workspaceevents.BeginDefinitionSpecificationResult{}, common.ErrInvalidArgument
	}
	if err := ctx.Err(); err != nil {
		return workspaceevents.BeginDefinitionSpecificationResult{}, err
	}
	command = command.Normalized()
	if err := command.Validate(); err != nil {
		return workspaceevents.BeginDefinitionSpecificationResult{}, err
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceSpecificationBegin, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceSpecificationBegin, eventcontract.LaneScope{WorkspaceID: string(command.Candidate.WorkspaceID)}))
	result := b.SendEvent(ev)
	if result == nil {
		return workspaceevents.BeginDefinitionSpecificationResult{}, fmt.Errorf("%w: missing compilation begin result", common.ErrConflict)
	}
	if err := result.Error(); err != nil {
		return workspaceevents.BeginDefinitionSpecificationResult{}, err
	}
	data, err := result.Get()
	if err != nil {
		return workspaceevents.BeginDefinitionSpecificationResult{}, err
	}
	response, ok := data.(workspaceevents.BeginDefinitionSpecificationResult)
	if !ok {
		return workspaceevents.BeginDefinitionSpecificationResult{}, fmt.Errorf("%w: unexpected compilation begin result", common.ErrConflict)
	}
	if err := response.ValidateFor(command); err != nil {
		return workspaceevents.BeginDefinitionSpecificationResult{}, err
	}
	response.Snapshot = response.Snapshot.Clone()
	inputs := (workspace.GoalSpecification{Resources: response.Resources, Dimensions: response.Dimensions}).Clone()
	response.Resources, response.Dimensions = inputs.Resources, inputs.Dimensions
	return response, nil
}

func (b *Biz) reserveDefinitionSpecification(ctx context.Context, command workspaceevents.ReserveDefinitionSpecificationRequestCommand) (workspaceevents.DefinitionSpecificationRequestResult, error) {
	if err := ctx.Err(); err != nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, err
	}
	if err := command.Validate(); err != nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, err
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceSpecificationReserve, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceSpecificationReserve, eventcontract.LaneScope{WorkspaceID: string(command.Identity.WorkspaceID)}))
	result := b.SendEvent(ev)
	if result == nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, fmt.Errorf("%w: missing compilation reserve result", common.ErrConflict)
	}
	if err := result.Error(); err != nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, err
	}
	data, err := result.Get()
	if err != nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, err
	}
	response, ok := data.(workspaceevents.DefinitionSpecificationRequestResult)
	if !ok {
		return workspaceevents.DefinitionSpecificationRequestResult{}, fmt.Errorf("%w: unexpected compilation reserve result", common.ErrConflict)
	}
	if err := response.ValidateFor(command.Identity); err != nil {
		return workspaceevents.DefinitionSpecificationRequestResult{}, err
	}
	return response.Clone(), nil
}

func (b *Biz) persistDefinitionSpecificationCompletion(ctx context.Context, command workspaceevents.CompleteDefinitionSpecificationRequestCommand) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := command.Validate(); err != nil {
		return err
	}
	ev := magicevent.NewEventWithContext(workspaceevents.TopicWorkspaceSpecificationComplete, ModuleID, workspaceevents.CommandDestination, magicevent.NewHeader(), ctx, command)
	ev.BindLaneKey(workorchevents.EventHubLaneKeyFor(workspaceevents.Contracts(), workspaceevents.TopicWorkspaceSpecificationComplete, eventcontract.LaneScope{WorkspaceID: string(command.Identity.WorkspaceID)}))
	result := b.SendEvent(ev)
	if result == nil {
		return fmt.Errorf("%w: missing compilation completion result", common.ErrConflict)
	}
	if err := result.Error(); err != nil {
		return err
	}
	data, err := result.Get()
	if err != nil {
		return err
	}
	response, ok := data.(workspaceevents.DefinitionSpecificationRequestResult)
	if !ok {
		return fmt.Errorf("%w: unexpected compilation completion result", common.ErrConflict)
	}
	if err := response.ValidateFor(command.Identity); err != nil {
		return err
	}
	if response.Disposition != workspaceevents.DefinitionSpecificationAlreadyCompleted {
		return fmt.Errorf("%w: compilation completion not persisted", common.ErrConflict)
	}
	if !reflect.DeepEqual(response.Completed.Normalized(), command.Completion.Normalized()) {
		return fmt.Errorf("%w: persisted compilation completion differs from submitted result", common.ErrConflict)
	}
	return nil
}

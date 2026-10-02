package appservice

import (
	magicevent "github.com/muidea/magicCommon/event"
	workorchdevents "github.com/muidea/workorch/internal/modules/application/workorchd/pkg/events"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (d *CLICommandDispatcher) handleTypedCLIDefinition(event magicevent.Event, result magicevent.Result) {
	ctx := ContextWithCommandHub(event.Context(), d.EventHub())
	switch command := event.Data().(type) {
	case workorchdevents.CLIGoalDefineCommand, workorchdevents.CLIGoalUpdateCommand,
		workorchdevents.CLIDoDDefineCommand, workorchdevents.CLIDoDUpdateCommand:
		setRetiredDefinitionWrite(result)
	case workorchdevents.CLIGoalListCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		items, err := ListGoalsWithSession(ctx, d.borrow(), string(command.WorkspaceID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIGoalListResult{Goals: items}, nil)
	case workorchdevents.CLIGoalGetCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		item, err := ReadGoalWithSession(ctx, d.borrow(), string(command.WorkspaceID), string(command.GoalID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIGoalGetResult{Goal: item}, nil)
	case workorchdevents.CLIDoDListCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		items, err := ListDoDsWithSession(ctx, d.borrow(), string(command.WorkspaceID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIDoDListResult{DoDs: items}, nil)
	case workorchdevents.CLIDoDGetCommand:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, command.WorkspaceID)
		item, err := ReadDoDWithSession(ctx, d.borrow(), string(command.WorkspaceID), string(command.DoDID))
		if setTypedCLIError(result, err) {
			return
		}
		result.Set(workorchdevents.CLIDoDGetResult{DoD: item}, nil)
	default:
		setInvalidTypedCLICommand(result)
	}
}

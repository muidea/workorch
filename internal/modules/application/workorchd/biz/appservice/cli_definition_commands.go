package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (d *CLICommandDispatcher) handleDefinitionCommand(ctx context.Context, cmd events.CLICommandPayload) (map[string]any, []map[string]any, error) {
	switch cmd.Command {
	case events.CommandGoalDefine:
		item, err := DefineGoalWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), clicore.StringValue(cmd.Payload, "text"))
		return clicore.ResourceMap(item), nil, err
	case events.CommandGoalList:
		items, err := ListGoalsWithSession(ctx, d.borrow(), string(cmd.WorkspaceID))
		return nil, clicore.ResourceMaps(items), err
	case events.CommandGoalGet:
		item, err := ReadGoalWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case events.CommandGoalUpdate:
		item, err := UpdateGoalWithSession(ctx, d.borrow(), cmd.SubjectID, clicore.StringValue(cmd.Payload, "text"))
		return clicore.ResourceMap(item), nil, err
	case events.CommandDoDDefine:
		item, err := DefineDoDWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), clicore.StringSliceValue(cmd.Payload, "items"))
		return clicore.ResourceMap(item), nil, err
	case events.CommandDoDList:
		items, err := ListDoDsWithSession(ctx, d.borrow(), string(cmd.WorkspaceID))
		return nil, clicore.ResourceMaps(items), err
	case events.CommandDoDGet:
		item, err := ReadDoDWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case events.CommandDoDUpdate:
		item, err := UpdateDoDWithSession(ctx, d.borrow(), cmd.SubjectID, clicore.StringSliceValue(cmd.Payload, "items"))
		return clicore.ResourceMap(item), nil, err
	default:
		return unsupportedCLICommand(cmd)
	}
}

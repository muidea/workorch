package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
)

func (d *CLICommandDispatcher) handleDefinitionCommand(ctx context.Context, cmd clicore.Command) (map[string]any, []map[string]any, error) {
	switch cmd.Command {
	case clicore.CommandGoalDefine:
		item, err := DefineGoalWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), clicore.StringValue(cmd.Payload, "text"))
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandGoalList:
		items, err := ListGoalsWithSession(ctx, d.borrow(), string(cmd.WorkspaceID))
		return nil, clicore.ResourceMaps(items), err
	case clicore.CommandGoalGet:
		item, err := ReadGoalWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandGoalUpdate:
		item, err := UpdateGoalWithSession(ctx, d.borrow(), cmd.SubjectID, clicore.StringValue(cmd.Payload, "text"))
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandDoDDefine:
		item, err := DefineDoDWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), clicore.StringSliceValue(cmd.Payload, "items"))
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandDoDList:
		items, err := ListDoDsWithSession(ctx, d.borrow(), string(cmd.WorkspaceID))
		return nil, clicore.ResourceMaps(items), err
	case clicore.CommandDoDGet:
		item, err := ReadDoDWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandDoDUpdate:
		item, err := UpdateDoDWithSession(ctx, d.borrow(), cmd.SubjectID, clicore.StringSliceValue(cmd.Payload, "items"))
		return clicore.ResourceMap(item), nil, err
	default:
		return unsupportedCLICommand(cmd)
	}
}

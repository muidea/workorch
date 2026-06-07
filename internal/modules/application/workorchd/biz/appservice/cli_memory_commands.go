package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (d *CLICommandDispatcher) handleMemoryCommand(ctx context.Context, cmd events.CLICommandPayload) (map[string]any, []map[string]any, error) {
	switch cmd.Command {
	case events.CommandMemoryCreate:
		item, err := CreateMemoryItemWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), clicore.MemoryCommandFromPayload(cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case events.CommandMemoryGet:
		item, err := ReadMemoryItemWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case events.CommandMemoryUpdate:
		item, err := UpdateMemoryItemWithSession(ctx, d.borrow(), cmd.SubjectID, clicore.MemoryCommandFromPayload(cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case events.CommandMemoryDelete:
		err := DeleteMemoryItemWithSession(ctx, d.borrow(), cmd.SubjectID)
		return nil, nil, err
	case events.CommandMemoryList:
		items, err := SearchMemoryWithSession(ctx, d.borrow(), MemoryQuery{WorkspaceID: string(cmd.WorkspaceID)})
		return nil, clicore.ResourceMaps(items), err
	case events.CommandMemorySearch:
		items, err := SearchMemoryWithSession(ctx, d.borrow(), clicore.MemorySearchQueryFromPayload(string(cmd.WorkspaceID), cmd.Payload))
		return nil, clicore.ResourceMaps(items), err
	default:
		return unsupportedCLICommand(cmd)
	}
}

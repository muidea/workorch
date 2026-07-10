package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (d *CLICommandDispatcher) handleMemoryCommand(ctx context.Context, cmd clicore.Command) (map[string]any, []map[string]any, error) {
	switch cmd.Command {
	case clicore.CommandMemoryCreate:
		item, err := CreateMemoryItemWithSession(ctx, d.borrow(), string(cmd.WorkspaceID), clicore.MemoryCommandFromPayload(cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandMemoryGet:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
		item, err := ReadMemoryItemWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandMemoryUpdate:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
		item, err := UpdateMemoryItemWithSession(ctx, d.borrow(), cmd.SubjectID, clicore.MemoryCommandFromPayload(cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandMemoryDelete:
		ctx = common.ContextWithRuntimeWorkspaceID(ctx, cmd.WorkspaceID)
		err := DeleteMemoryItemWithSession(ctx, d.borrow(), cmd.SubjectID)
		return nil, nil, err
	case clicore.CommandMemoryList:
		items, err := SearchMemoryWithSession(ctx, d.borrow(), MemoryQuery{WorkspaceID: string(cmd.WorkspaceID)})
		return nil, clicore.ResourceMaps(items), err
	case clicore.CommandMemorySearch:
		items, err := SearchMemoryWithSession(ctx, d.borrow(), clicore.MemorySearchQueryFromPayload(string(cmd.WorkspaceID), cmd.Payload))
		return nil, clicore.ResourceMaps(items), err
	default:
		return unsupportedCLICommand(cmd)
	}
}

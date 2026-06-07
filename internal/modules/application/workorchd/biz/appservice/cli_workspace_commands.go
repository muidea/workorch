package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (d *CLICommandDispatcher) handleWorkspaceCommand(ctx context.Context, cmd events.CLICommandPayload) (map[string]any, []map[string]any, error) {
	switch cmd.Command {
	case events.CommandWorkspaceCreate:
		item, err := CreateWorkspaceWithTemplateWithSession(ctx, d.borrow(), clicore.CreateWorkspaceCommandFromPayload(cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case events.CommandWorkspaceGet:
		item, err := ReadWorkspaceWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case events.CommandWorkspaceList:
		items, err := ListWorkspacesWithSession(ctx, d.borrow())
		return nil, clicore.ResourceMaps(items), err
	case events.CommandWorkspaceSnapshot:
		item, err := ReadWorkspaceSnapshotWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case events.CommandWorkspaceDelete:
		err := DeleteWorkspaceWithSession(ctx, d.borrow(), cmd.SubjectID, DeleteWorkspaceCommand{Purge: clicore.BoolValue(cmd.Payload, "purge")})
		return nil, nil, err
	default:
		return unsupportedCLICommand(cmd)
	}
}

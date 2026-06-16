package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
	"github.com/muidea/workorch/internal/pkg/common"
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
		if d.viewReader == nil {
			return nil, nil, common.ErrInvalidArgument
		}
		items, err := d.viewReader.ListWorkspaceView(ctx)
		return nil, clicore.ResourceMaps(items), err
	case events.CommandWorkspaceSnapshot:
		if d.viewReader == nil {
			return nil, nil, common.ErrInvalidArgument
		}
		item, err := d.viewReader.ReadWorkspaceViewSnapshot(ctx, cmd.SubjectID)
		if err != nil {
			return nil, nil, err
		}
		item = ProjectWorkspaceSnapshotDetail(item)
		return clicore.ResourceMap(item), nil, err
	case events.CommandWorkspaceDelete:
		err := DeleteWorkspaceWithSession(ctx, d.borrow(), cmd.SubjectID, DeleteWorkspaceCommand{Purge: clicore.BoolValue(cmd.Payload, "purge")})
		return nil, nil, err
	default:
		return unsupportedCLICommand(cmd)
	}
}

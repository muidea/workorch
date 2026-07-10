package appservice

import (
	"context"
	"fmt"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
	"github.com/muidea/workorch/internal/pkg/common"
)

func (d *CLICommandDispatcher) handleWorkspaceCommand(ctx context.Context, cmd clicore.Command) (map[string]any, []map[string]any, error) {
	switch cmd.Command {
	case clicore.CommandWorkspaceCreate:
		item, err := CreateWorkspaceWithTemplateWithSession(ctx, d.borrow(), clicore.CreateWorkspaceCommandFromPayload(cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandWorkspaceGet:
		if err := validateCLIWorkspaceObjectScope(cmd); err != nil {
			return nil, nil, err
		}
		item, err := ReadWorkspaceWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandWorkspaceList:
		if d.viewReader == nil {
			return nil, nil, common.ErrInvalidArgument
		}
		items, err := d.viewReader.ListWorkspaceView(ctx)
		return nil, clicore.ResourceMaps(items), err
	case clicore.CommandWorkspaceSnapshot:
		if err := validateCLIWorkspaceObjectScope(cmd); err != nil {
			return nil, nil, err
		}
		if d.viewReader == nil {
			return nil, nil, common.ErrInvalidArgument
		}
		item, err := d.viewReader.ReadWorkspaceViewSnapshot(ctx, cmd.SubjectID)
		if err != nil {
			return nil, nil, err
		}
		item = ProjectWorkspaceSnapshotDetail(item)
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandWorkspaceDelete:
		if err := validateCLIWorkspaceObjectScope(cmd); err != nil {
			return nil, nil, err
		}
		err := DeleteWorkspaceWithSession(ctx, d.borrow(), cmd.SubjectID, DeleteWorkspaceCommand{Purge: clicore.BoolValue(cmd.Payload, "purge")})
		return nil, nil, err
	default:
		return unsupportedCLICommand(cmd)
	}
}

func validateCLIWorkspaceObjectScope(cmd clicore.Command) error {
	if cmd.WorkspaceID == "" || cmd.SubjectID == "" {
		return fmt.Errorf("%w: workspace command %s requires workspace scope and subject id", common.ErrInvalidArgument, cmd.Command)
	}
	if string(cmd.WorkspaceID) != cmd.SubjectID {
		return fmt.Errorf("%w: workspace command %s scope %s does not match subject %s", common.ErrConflict, cmd.Command, cmd.WorkspaceID, cmd.SubjectID)
	}
	return nil
}

package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
)

func (d *CLICommandDispatcher) handleTemplateCommand(ctx context.Context, cmd clicore.Command) (map[string]any, []map[string]any, error) {
	switch cmd.Command {
	case clicore.CommandTemplateCreate:
		item, err := CreateWorkspaceTemplateWithSession(ctx, d.borrow(), clicore.WorkspaceTemplateFromPayload(cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandTemplateGet:
		item, err := ReadWorkspaceTemplateWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandTemplateList:
		items, err := ListWorkspaceTemplatesWithSession(ctx, d.borrow())
		return nil, clicore.ResourceMaps(items), err
	case clicore.CommandTemplateUpdate:
		item, err := UpdateWorkspaceTemplateWithSession(ctx, d.borrow(), cmd.SubjectID, clicore.WorkspaceTemplateFromPayload(cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case clicore.CommandTemplateDelete:
		err := DeleteWorkspaceTemplateWithSession(ctx, d.borrow(), cmd.SubjectID)
		return nil, nil, err
	default:
		return unsupportedCLICommand(cmd)
	}
}

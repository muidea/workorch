package appservice

import (
	"context"

	"github.com/muidea/workorch/internal/modules/application/workorchd/biz/appservice/clicore"
	"github.com/muidea/workorch/internal/pkg/events"
)

func (d *CLICommandDispatcher) handleTemplateCommand(ctx context.Context, cmd events.CLICommandPayload) (map[string]any, []map[string]any, error) {
	switch cmd.Command {
	case events.CommandTemplateCreate:
		item, err := CreateWorkspaceTemplateWithSession(ctx, d.borrow(), *clicore.DecodeOrZero[WorkspaceTemplate](cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case events.CommandTemplateGet:
		item, err := ReadWorkspaceTemplateWithSession(ctx, d.borrow(), cmd.SubjectID)
		return clicore.ResourceMap(item), nil, err
	case events.CommandTemplateList:
		items, err := ListWorkspaceTemplatesWithSession(ctx, d.borrow())
		return nil, clicore.ResourceMaps(items), err
	case events.CommandTemplateUpdate:
		item, err := UpdateWorkspaceTemplateWithSession(ctx, d.borrow(), cmd.SubjectID, *clicore.DecodeOrZero[WorkspaceTemplate](cmd.Payload))
		return clicore.ResourceMap(item), nil, err
	case events.CommandTemplateDelete:
		err := DeleteWorkspaceTemplateWithSession(ctx, d.borrow(), cmd.SubjectID)
		return nil, nil, err
	default:
		return unsupportedCLICommand(cmd)
	}
}
